package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/repoinstructions"
)

const (
	ContextComposerVersion            = "context-composer-v2"
	CanonicalRequestEstimatorVersion  = "canonical-provider-json-bytes-v2"
	CalibratedRequestEstimatorVersion = "calibrated-provider-json-bytes-v3"
)

var ErrContextOverflow = errors.New("agent: request exceeds configured model context")

type contextOverflowError struct {
	serialized int64
	usable     int64
}

func (e *contextOverflowError) Error() string {
	return fmt.Sprintf("%v: %d serialized bytes exceeds %d usable bytes", ErrContextOverflow, e.serialized, e.usable)
}

func (e *contextOverflowError) Unwrap() error { return ErrContextOverflow }

func IsContextOverflow(err error) bool { return errors.Is(err, ErrContextOverflow) }

type RequestEstimate struct {
	SerializedBytes int64
	RoughTokens     int64
	RequestSHA256   string
}

// RequestEstimator is the replaceable hard-bound estimator consumed by the
// composer. Implementations must account for the complete request value.
type RequestEstimator interface {
	Version() string
	Estimate(openrouter.ChatRequest) (RequestEstimate, error)
}

type CanonicalRequestEstimator struct{}

func (CanonicalRequestEstimator) Version() string { return CanonicalRequestEstimatorVersion }

func (CanonicalRequestEstimator) Estimate(request openrouter.ChatRequest) (RequestEstimate, error) {
	encoded, err := openrouter.RequestBytes(request)
	if err != nil {
		return RequestEstimate{}, fmt.Errorf("serialize provider request: %w", err)
	}
	digest := sha256.Sum256(encoded)
	bytes := int64(len(encoded))
	return RequestEstimate{
		SerializedBytes: bytes,
		RoughTokens:     (bytes + 3) / 4,
		RequestSHA256:   hex.EncodeToString(digest[:]),
	}, nil
}

type ContextComposeInput struct {
	RepositoryInstructions       string
	RepositoryInstructionsTurnID memory.EventID
	WorkerInstructions           string
	MemoryData                   string
	MemoryReceipt                *memory.RetrievalReceipt
	Profile                      openrouter.ContextProfile
	Summary                      *ContextSummary
	Events                       []memory.Event
	ActiveRootID                 memory.EventID
	TriggerEventID               memory.EventID
	Iteration                    int
	Tools                        []openrouter.Tool
	// ToolChoice "none" forbids tool calls while keeping Tools in the
	// request, as the final step-limit call requires.
	ToolChoice     string
	Reasoning      *openrouter.ReasoningConfig
	WorkingContext string
	Continuation   map[memory.EventID][]json.RawMessage
	// FinalStepNote, when set, is a trusted harness instruction appended
	// after the conversation as the request's last message.
	FinalStepNote string
	// RejectedRequestBytes, when positive, is the canonical size of this
	// trigger's request that the provider rejected for context length. The
	// one recovery request assumes no more bytes per token than that
	// rejection proved and always plans an automatic compaction.
	RejectedRequestBytes int64
}

// ContextSummary is the validated rolling summary selected by the later
// compaction stage. The composer owns its request position and accounting;
// callers own durable selection and validation.
type ContextSummary struct {
	CompactionEventID    memory.EventID
	FirstRetainedEventID memory.EventID
	Content              string
}

type ComposedContext struct {
	Request  openrouter.ChatRequest
	Snapshot memory.ContextSnapshotPayload
}

type DurableContextSnapshotDiagnostics struct {
	EventID  memory.EventID                `json:"event_id"`
	ParentID memory.EventID                `json:"parent_id"`
	Sequence int64                         `json:"sequence"`
	Manifest memory.ContextSnapshotPayload `json:"manifest"`
}

type ContextDiagnostics struct {
	Profile                openrouter.ContextProfileDiagnostics `json:"profile"`
	LatestSnapshot         *DurableContextSnapshotDiagnostics   `json:"latest_snapshot,omitempty"`
	Projection             memory.ContextSnapshotPayload        `json:"projection"`
	CurrentDurableEventID  memory.EventID                       `json:"current_durable_event_id,omitempty"`
	CurrentDurableSequence int64                                `json:"current_durable_sequence,omitempty"`
	HeadroomBytes          int64                                `json:"headroom_bytes"`
	AutomaticCompaction    *ContextCompactionPlanDiagnostics    `json:"automatic_compaction,omitempty"`
	Warnings               []string                             `json:"warnings,omitempty"`
}

// ContextCompactionPlanDiagnostics names, by content-free event identity, the
// automatic compaction the next request would attempt before it is sent.
type ContextCompactionPlanDiagnostics struct {
	CoveredFirstEventID  memory.EventID `json:"covered_first_event_id"`
	CoveredLastEventID   memory.EventID `json:"covered_last_event_id"`
	FirstRetainedEventID memory.EventID `json:"first_retained_event_id"`
}

type ContextComposer struct {
	estimator RequestEstimator
}

func NewContextComposer(estimator RequestEstimator) *ContextComposer {
	return &ContextComposer{estimator: estimator}
}

type contextProjection struct {
	request           openrouter.ChatRequest
	estimate          RequestEstimate
	conversation      []openrouter.Message
	selectedOriginal  []memory.Event
	selectedProjected []memory.Event
}

type contextPreparation struct {
	profile     openrouter.ContextProfileDiagnostics
	ratio       tokenRatio
	usable      int64
	turns       [][]memory.Event
	activeIndex int
	trigger     memory.Event
	start       int
}

// workingBytes is the working ceiling in canonical request bytes, the base of
// the automatic compaction threshold and target.
func (p contextPreparation) workingBytes() int64 {
	return p.ratio.budgetBytes(p.profile.WorkingTokens)
}

func (c *ContextComposer) prepare(
	input ContextComposeInput,
) (contextPreparation, error) {
	if c == nil || c.estimator == nil {
		return contextPreparation{}, errors.New("context request estimator is not configured")
	}
	if input.Iteration <= 0 {
		return contextPreparation{}, errors.New("context iteration must be positive")
	}
	if input.Summary != nil && (input.Summary.CompactionEventID == "" || strings.TrimSpace(input.Summary.Content) == "") {
		return contextPreparation{}, errors.New("context summary identity and content must be present")
	}
	profile := input.Profile.Diagnostics()
	if err := validateDurableContextHistory(input.Events); err != nil {
		return contextPreparation{}, err
	}
	ratio := contextTokenRatio(c.estimator, input.Events, profileCanonicalModel(profile),
		profile.HardWindowTokens, profile.OutputReserveTokens, input.RejectedRequestBytes)
	usable, err := usableInputBytes(profile, ratio)
	if err != nil {
		return contextPreparation{}, err
	}
	turns, activeIndex, trigger, err := contextRootTurns(input.Events, input.ActiveRootID, input.TriggerEventID)
	if err != nil {
		return contextPreparation{}, err
	}
	start := 0
	if input.Summary != nil && input.Summary.FirstRetainedEventID != "" {
		found := false
		for i, turn := range turns {
			if turn[0].ID == input.Summary.FirstRetainedEventID {
				start = i
				found = true
				break
			}
		}
		if !found {
			return contextPreparation{}, fmt.Errorf("context summary retained root %q is missing", input.Summary.FirstRetainedEventID)
		}
		if start > activeIndex {
			return contextPreparation{}, errors.New("context summary retained frontier is after the active turn")
		}
	}
	return contextPreparation{
		profile: profile, ratio: ratio, usable: usable, turns: turns, activeIndex: activeIndex, trigger: trigger, start: start,
	}, nil
}

func (c *ContextComposer) projectAtStart(
	input ContextComposeInput,
	prepared contextPreparation,
	start int,
) (contextProjection, error) {
	profile, usable, turns := prepared.profile, prepared.usable, prepared.turns
	projection := contextProjection{selectedOriginal: flattenContextTurns(turns[start:])}
	var err error
	projection.selectedProjected, err = applyToolResultGroupLimits(projection.selectedOriginal)
	if err != nil {
		return contextProjection{}, fmt.Errorf("bound durable tool-result groups: %w", err)
	}
	composeProjected := func(projected []memory.Event) error {
		projection.conversation, err = messagesFromEventsWithContinuation(projected, input.Continuation)
		if err != nil {
			return fmt.Errorf("project durable history: %w", err)
		}
		if input.MemoryData != "" {
			rootIndex := -1
			for i, event := range projected {
				if event.ID == input.ActiveRootID {
					rootIndex = i
					break
				}
			}
			if rootIndex < 0 {
				return errors.New("memory projection requires the current root user event")
			}
			tail, tailErr := messagesFromEventsWithContinuation(projected[rootIndex:], input.Continuation)
			if tailErr != nil {
				return tailErr
			}
			position := len(projection.conversation) - len(tail)
			withMemory := append([]openrouter.Message(nil), projection.conversation[:position]...)
			withMemory = append(withMemory, openrouter.Message{Role: "user", Content: input.MemoryData})
			projection.conversation = append(withMemory, projection.conversation[position:]...)
		}
		messages := make([]openrouter.Message, 0, len(projection.conversation)+2)
		instructions := systemPrompt
		if input.WorkerInstructions != "" {
			instructions = input.WorkerInstructions
		}
		messages = append(messages, openrouter.Message{Role: "system", Content: instructions})
		if input.Summary != nil {
			messages = append(messages, openrouter.Message{Role: "system", Content: input.Summary.Content})
		}
		if input.WorkingContext != "" {
			messages = append(messages, openrouter.Message{Role: "user", Content: input.WorkingContext})
		}
		if input.RepositoryInstructions != "" {
			messages = append(messages, openrouter.Message{Role: "user", Content: input.RepositoryInstructions})
		}
		messages = append(messages, projection.conversation...)
		if input.FinalStepNote != "" {
			messages = append(messages, openrouter.Message{Role: "user", Content: input.FinalStepNote})
		}
		projection.request = openrouter.ChatRequest{
			Model:      profile.ConfiguredModel,
			Messages:   messages,
			Tools:      append([]openrouter.Tool(nil), input.Tools...),
			ToolChoice: input.ToolChoice,
			Stream:     true,
			Reasoning:  cloneReasoning(input.Reasoning),
			MaxTokens:  profile.OutputReserveTokens,
		}
		projection.request, err = openrouter.PrepareRequest(projection.request)
		if err != nil {
			return err
		}
		projection.estimate, err = c.estimator.Estimate(projection.request)
		return err
	}
	if err := composeProjected(projection.selectedProjected); err != nil {
		return contextProjection{}, err
	}
	groups, err := completeToolResultGroups(projection.selectedOriginal)
	if err != nil {
		return contextProjection{}, fmt.Errorf("identify durable tool-result groups: %w", err)
	}
	pressureTarget := percentageFloor(usable, 60)
	eligibleGroups := max(0, len(groups)-retainedCompleteToolResultGroups)
	for groupIndex := 0; projection.estimate.SerializedBytes > pressureTarget && groupIndex < eligibleGroups; groupIndex++ {
		for _, resultIndex := range groups[groupIndex].resultIndexes {
			if projection.estimate.SerializedBytes <= pressureTarget {
				break
			}
			if !isPressureProjectableToolResult(projection.selectedOriginal[resultIndex]) {
				continue
			}
			pressureProjection := projectOldToolResult(projection.selectedOriginal[resultIndex])
			if len(pressureProjection) >= len(projection.selectedProjected[resultIndex].Content) {
				continue
			}
			projection.selectedProjected[resultIndex].Content = pressureProjection
			if err := composeProjected(projection.selectedProjected); err != nil {
				return contextProjection{}, err
			}
		}
	}
	return projection, nil
}

func (c *ContextComposer) Compose(input ContextComposeInput) (ComposedContext, error) {
	prepared, err := c.prepare(input)
	if err != nil {
		return ComposedContext{}, err
	}
	usable, activeIndex, start := prepared.usable, prepared.activeIndex, prepared.start
	var projection contextProjection
	for {
		projection, err = c.projectAtStart(input, prepared, start)
		if err != nil {
			if IsContextOverflow(err) && start < activeIndex {
				start++
				continue
			}
			return ComposedContext{}, err
		}
		if projection.estimate.SerializedBytes <= usable {
			break
		}
		if start >= activeIndex {
			return ComposedContext{}, &contextOverflowError{serialized: projection.estimate.SerializedBytes, usable: usable}
		}
		start++
	}
	snapshot, err := c.projectionSnapshot(input, prepared, start, projection)
	if err != nil {
		return ComposedContext{}, err
	}
	if err := snapshot.Validate(); err != nil {
		return ComposedContext{}, fmt.Errorf("validate context snapshot: %w", err)
	}
	return ComposedContext{Request: projection.request, Snapshot: snapshot}, nil
}

// projectionSnapshot is the content-free manifest of one projected request.
// Callers validate it before it can describe a request that is sent.
func (c *ContextComposer) projectionSnapshot(
	input ContextComposeInput,
	prepared contextPreparation,
	start int,
	projection contextProjection,
) (memory.ContextSnapshotPayload, error) {
	profile := prepared.profile
	first := prepared.turns[start][0]
	// A final-step note trails the conversation and is accounted as history.
	historyMessages := len(projection.conversation)
	if input.FinalStepNote != "" {
		historyMessages++
	}
	systemBytes, summaryBytes, historyBytes, toolBytes, settingsBytes, err := contextByteBreakdown(
		projection.request, historyMessages, input.Summary != nil,
	)
	if err != nil {
		return memory.ContextSnapshotPayload{}, err
	}
	snapshot := memory.ContextSnapshotPayload{
		Memory:                       input.MemoryReceipt,
		RepositoryInstructionsTurnID: input.RepositoryInstructionsTurnID,
		SchemaVersion:                memory.ContextSnapshotSchemaVersion,
		ComposerVersion:              ContextComposerVersion,
		EstimatorVersion:             c.estimator.Version(),
		Iteration:                    input.Iteration,
		ConfiguredModel:              profile.ConfiguredModel,
		CanonicalModel:               profileCanonicalModel(profile),
		AdvertisedModel:              profile.AdvertisedModel,
		ProfileSource:                string(profile.Source),
		AdvertisedWindowTokens:       profile.AdvertisedWindowTokens,
		HardWindowTokens:             profile.HardWindowTokens,
		WorkingCeilingTokens:         profile.WorkingTokens,
		OutputReserveTokens:          profile.OutputReserveTokens,
		EstimationMarginTokens:       profile.EstimationMarginTokens,
		BytesPerTokenMilli:           prepared.ratio.milli,
		UsableInputBytes:             prepared.usable,
		SerializedBytes:              projection.estimate.SerializedBytes,
		RoughTokenEstimate:           memory.ContextTokenEstimate(projection.estimate.SerializedBytes, prepared.ratio.milli),
		RequestSHA256:                projection.estimate.RequestSHA256,
		RetainedFirstEventID:         first.ID,
		RetainedFirstSequence:        first.Sequence,
		RetainedLastEventID:          prepared.trigger.ID,
		RetainedLastSequence:         prepared.trigger.Sequence,
		MessageCount:                 len(projection.request.Messages),
		ToolSchemaCount:              len(projection.request.Tools),
		SystemMessageBytes:           systemBytes,
		SummaryMessageBytes:          summaryBytes,
		HistoryMessageBytes:          historyBytes,
		ToolSchemaBytes:              toolBytes,
		RequestSettingsBytes:         settingsBytes,
		Placeholders:                 toolResultPlaceholderManifests(projection.selectedOriginal, projection.selectedProjected),
	}
	if prepared.ratio.milli > 0 {
		snapshot.CalibrationSamples = prepared.ratio.samples
	}
	if input.Summary != nil {
		snapshot.ActiveCompactionEventID = input.Summary.CompactionEventID
	}
	return snapshot, nil
}

func profileCanonicalModel(profile openrouter.ContextProfileDiagnostics) string {
	if profile.CanonicalModel != "" {
		return profile.CanonicalModel
	}
	return profile.ConfiguredModel
}

func percentageFloor(value int64, percent int64) int64 {
	return (value/100)*percent + (value%100)*percent/100
}

// InspectContext performs a point-in-time durable read and a hypothetical
// empty-root composition. It takes no turn lease and writes no session state.
func (s *Session) InspectContext(ctx context.Context) (ContextDiagnostics, error) {
	if s.configurationErr != nil {
		return ContextDiagnostics{}, s.configurationErr
	}
	events, err := s.history.Events(ctx)
	if err != nil {
		return ContextDiagnostics{}, fmt.Errorf("load durable history: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return ContextDiagnostics{}, err
	}
	summary, _, err := reconstructCompactionChain(events)
	if err != nil {
		return ContextDiagnostics{}, fmt.Errorf("reconstruct durable compaction chain: %w", err)
	}

	var latest *DurableContextSnapshotDiagnostics
	var maxSequence int64
	var currentEventID memory.EventID
	for _, event := range events {
		if event.Sequence >= maxSequence {
			maxSequence = event.Sequence
			currentEventID = event.ID
		}
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		if err := validateSnapshotEvent(event); err != nil {
			return ContextDiagnostics{}, err
		}
		var payload memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return ContextDiagnostics{}, fmt.Errorf("decode context snapshot event %q: %w", event.ID, err)
		}
		if latest == nil || event.Sequence > latest.Sequence {
			latest = &DurableContextSnapshotDiagnostics{
				EventID: event.ID, ParentID: event.ParentID, Sequence: event.Sequence, Manifest: payload,
			}
		}
	}

	const hypotheticalRootID memory.EventID = "hypothetical-current-root"
	for _, event := range events {
		if event.ID == hypotheticalRootID {
			return ContextDiagnostics{}, errors.New("durable history conflicts with the hypothetical context diagnostic identity")
		}
	}
	hypothetical := memory.Event{
		ID: hypotheticalRootID, Sequence: maxSequence + 1,
		Type: memory.EventUserMessage, Role: memory.RoleUser,
	}
	projectionEvents := append(append([]memory.Event(nil), events...), hypothetical)
	iteration := 1
	if latest != nil {
		iteration = latest.Manifest.Iteration + 1
	}
	workingContext := ""
	if provider, ok := s.history.(workingContextProvider); ok && s.workerInstructions == "" {
		workingContext, err = provider.WorkingContext(ctx)
		if err != nil {
			return ContextDiagnostics{}, fmt.Errorf("load working context: %w", err)
		}
	}
	repository := ""
	if provider, ok := s.history.(interface {
		PreviewRepositoryInstructions(context.Context) (memory.RepositoryInstructionSnapshot, error)
	}); ok && s.workerInstructions == "" {
		snapshot, loadErr := provider.PreviewRepositoryInstructions(ctx)
		if loadErr != nil {
			return ContextDiagnostics{}, loadErr
		}
		if snapshot.Status == "error" {
			return ContextDiagnostics{}, errors.New(snapshot.Detail)
		}
		repository = repoinstructions.Render(snapshot)
	}
	projection, compaction, planWarnings, err := s.composer.inspectNextRequest(ContextComposeInput{
		RepositoryInstructions: repository,
		Profile:                s.profile, Summary: summary, Events: projectionEvents, ActiveRootID: hypothetical.ID,
		TriggerEventID: hypothetical.ID, Iteration: iteration,
		Tools: s.modelToolset().Schemas(), Reasoning: s.reasoning, WorkingContext: workingContext, WorkerInstructions: s.workerInstructions,
	})
	if err != nil {
		return ContextDiagnostics{}, fmt.Errorf("compose hypothetical context: %w", err)
	}
	diagnostics := ContextDiagnostics{
		Profile: s.profile.Diagnostics(), LatestSnapshot: latest, Projection: projection,
		CurrentDurableEventID: currentEventID, CurrentDurableSequence: maxSequence,
		HeadroomBytes:       projection.UsableInputBytes - projection.SerializedBytes,
		AutomaticCompaction: compaction, Warnings: planWarnings,
	}
	if diagnostics.Profile.Source == openrouter.ContextProfileBuiltinFallback {
		diagnostics.Warnings = append(diagnostics.Warnings, "context profile uses built-in fallback metadata")
	}
	if latest == nil {
		diagnostics.Warnings = append(diagnostics.Warnings, "no durable context snapshot exists yet")
	} else {
		if latest.Sequence < maxSequence {
			diagnostics.Warnings = append(diagnostics.Warnings, "latest context snapshot predates current durable history")
		}
		if !snapshotMatchesProfile(latest.Manifest, diagnostics.Profile) {
			diagnostics.Warnings = append(diagnostics.Warnings, "latest context snapshot used a different context profile")
		}
	}
	return diagnostics, nil
}

// inspectNextRequest describes the request the turn path would build for
// input, using the same planning as a turn. Without pressure it is exactly
// the composed request. Under pressure the turn compacts first or fails, so
// the unchanged projection at the active summary frontier is reported with
// the planned compaction, never a silently trimmed composition.
func (c *ContextComposer) inspectNextRequest(
	input ContextComposeInput,
) (memory.ContextSnapshotPayload, *ContextCompactionPlanDiagnostics, []string, error) {
	prepared, err := c.prepare(input)
	if err != nil {
		return memory.ContextSnapshotPayload{}, nil, nil, err
	}
	plan, required, planErr := selectAutomaticCompaction(input, c)
	if planErr != nil && !errors.Is(planErr, ErrNoLegalAutomaticCompaction) {
		return memory.ContextSnapshotPayload{}, nil, nil, planErr
	}
	if !required {
		composed, err := c.Compose(input)
		if err != nil {
			return memory.ContextSnapshotPayload{}, nil, nil, err
		}
		var warnings []string
		if composed.Snapshot.RetainedFirstEventID != prepared.turns[prepared.start][0].ID {
			warnings = append(warnings, fmt.Sprintf(
				"the next request omits older turns before %s to fit the usable input budget",
				composed.Snapshot.RetainedFirstEventID))
		}
		return composed.Snapshot, nil, warnings, nil
	}
	projection, err := c.projectAtStart(input, prepared, prepared.start)
	if err != nil {
		return memory.ContextSnapshotPayload{}, nil, nil, err
	}
	snapshot, err := c.projectionSnapshot(input, prepared, prepared.start, projection)
	if err != nil {
		return memory.ContextSnapshotPayload{}, nil, nil, err
	}
	if planErr != nil {
		return snapshot, nil, []string{
			"the next request cannot fit: no legal automatic compaction reaches the usable input budget, so the turn would fail with context_overflow",
		}, nil
	}
	return snapshot, &ContextCompactionPlanDiagnostics{
			CoveredFirstEventID: plan.CoveredFirst.ID, CoveredLastEventID: plan.CoveredLast.ID,
			FirstRetainedEventID: plan.FirstRetained.ID,
		}, []string{fmt.Sprintf(
			"the next request will first run automatic compaction through %s, retaining from %s",
			plan.CoveredLast.ID, plan.FirstRetained.ID)}, nil
}

func snapshotMatchesProfile(snapshot memory.ContextSnapshotPayload, profile openrouter.ContextProfileDiagnostics) bool {
	canonical := profile.CanonicalModel
	if canonical == "" {
		canonical = profile.ConfiguredModel
	}
	return snapshot.ConfiguredModel == profile.ConfiguredModel && snapshot.CanonicalModel == canonical &&
		snapshot.ProfileSource == string(profile.Source) && snapshot.HardWindowTokens == profile.HardWindowTokens &&
		snapshot.WorkingCeilingTokens == profile.WorkingTokens &&
		snapshot.OutputReserveTokens == profile.OutputReserveTokens &&
		snapshot.EstimationMarginTokens == profile.EstimationMarginTokens
}

func usableInputBytes(profile openrouter.ContextProfileDiagnostics, ratio tokenRatio) (int64, error) {
	ceiling := min(profile.HardWindowTokens, profile.WorkingTokens)
	if ceiling <= 0 || profile.OutputReserveTokens <= 0 || profile.EstimationMarginTokens <= 0 ||
		profile.OutputReserveTokens > math.MaxInt64-profile.EstimationMarginTokens ||
		profile.OutputReserveTokens+profile.EstimationMarginTokens >= ceiling {
		return 0, errors.New("context profile has no usable input budget")
	}
	return ratio.budgetBytes(ceiling - profile.OutputReserveTokens - profile.EstimationMarginTokens), nil
}

func contextRootTurns(
	events []memory.Event,
	activeRootID memory.EventID,
	triggerID memory.EventID,
) ([][]memory.Event, int, memory.Event, error) {
	var turns [][]memory.Event
	activeIndex := -1
	var trigger memory.Event
	for _, event := range events {
		if event.ID == triggerID {
			trigger = event
		}
		if event.Type == memory.EventUserMessage {
			if event.Role != memory.RoleUser || event.ParentID != "" {
				return nil, 0, memory.Event{}, fmt.Errorf("user event %q is not a root user turn", event.ID)
			}
			turns = append(turns, nil)
			if event.ID == activeRootID {
				activeIndex = len(turns) - 1
			}
		}
		if len(turns) == 0 {
			return nil, 0, memory.Event{}, fmt.Errorf("history event %q precedes the first root user turn", event.ID)
		}
		turns[len(turns)-1] = append(turns[len(turns)-1], event)
	}
	if len(turns) == 0 {
		return nil, 0, memory.Event{}, errors.New("durable history contains no root user turn")
	}
	if activeIndex < 0 {
		return nil, 0, memory.Event{}, fmt.Errorf("active root event %q is missing", activeRootID)
	}
	if activeIndex != len(turns)-1 {
		return nil, 0, memory.Event{}, fmt.Errorf("active root event %q is not the latest root turn", activeRootID)
	}
	if trigger.ID == "" {
		return nil, 0, memory.Event{}, fmt.Errorf("provider trigger event %q is missing", triggerID)
	}
	if trigger.Type != memory.EventUserMessage && trigger.Type != memory.EventToolSucceeded &&
		trigger.Type != memory.EventToolFailed && trigger.Type != memory.EventToolCancelled {
		return nil, 0, memory.Event{}, fmt.Errorf("event %q cannot trigger a conversational provider request", trigger.ID)
	}
	return turns, activeIndex, trigger, nil
}

func flattenContextTurns(turns [][]memory.Event) []memory.Event {
	count := 0
	for _, turn := range turns {
		count += len(turn)
	}
	flattened := make([]memory.Event, 0, count)
	for _, turn := range turns {
		flattened = append(flattened, turn...)
	}
	return flattened
}

func cloneReasoning(reasoning *openrouter.ReasoningConfig) *openrouter.ReasoningConfig {
	if reasoning == nil {
		return nil
	}
	copy := *reasoning
	return &copy
}

func contextByteBreakdown(
	request openrouter.ChatRequest,
	historyMessages int,
	hasSummary bool,
) (int64, int64, int64, int64, int64, error) {
	if openrouter.UsesResponses(request.Model) {
		parts, err := openrouter.ResponseRequestPartSizes(request)
		if err != nil {
			return 0, 0, 0, 0, 0, err
		}
		summaryBytes, historyBytes := int64(0), int64(0)
		start := 1
		if hasSummary {
			summaryBytes = parts.Messages[1]
			start++
		}
		for _, size := range parts.Messages[start:] {
			historyBytes += size
		}
		return parts.Messages[0], summaryBytes, historyBytes, parts.Tools, parts.Settings, nil
	}
	system, err := json.Marshal(request.Messages[0])
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	systemBytes := int64(len(system))
	nextSystem := 1
	summaryBytes := int64(0)
	if hasSummary {
		summary, err := json.Marshal(request.Messages[nextSystem])
		if err != nil {
			return 0, 0, 0, 0, 0, err
		}
		summaryBytes = int64(len(summary))
		nextSystem++
	}
	history, err := json.Marshal(request.Messages[len(request.Messages)-historyMessages:])
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	tools, err := json.Marshal(request.Tools)
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	settings, err := json.Marshal(struct {
		Model     string                      `json:"model"`
		Stream    bool                        `json:"stream"`
		Reasoning *openrouter.ReasoningConfig `json:"reasoning,omitempty"`
		MaxTokens int64                       `json:"max_tokens"`
	}{request.Model, request.Stream, request.Reasoning, request.MaxTokens})
	if err != nil {
		return 0, 0, 0, 0, 0, err
	}
	toolBytes := int64(0)
	if len(request.Tools) > 0 {
		toolBytes = int64(len(tools))
	}
	historyBytes := int64(len(history))
	for _, message := range request.Messages[nextSystem : len(request.Messages)-historyMessages] {
		working, err := json.Marshal(message)
		if err != nil {
			return 0, 0, 0, 0, 0, err
		}
		historyBytes += int64(len(working))
	}
	return systemBytes, summaryBytes, historyBytes, toolBytes, int64(len(settings)), nil
}

func validateDurableContextHistory(events []memory.Event) error {
	type indexedEvent struct {
		event memory.Event
		index int
	}
	byID := make(map[memory.EventID]indexedEvent, len(events))
	for i, event := range events {
		if event.ID == "" {
			return fmt.Errorf("durable history event at index %d has no ID", i)
		}
		if _, exists := byID[event.ID]; exists {
			return fmt.Errorf("durable history repeats event ID %q", event.ID)
		}
		byID[event.ID] = indexedEvent{event: event, index: i}
	}
	for i, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		if err := validateSnapshotEvent(event); err != nil {
			return err
		}
		var payload memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode context snapshot event %q: %w", event.ID, err)
		}
		parent, ok := byID[event.ParentID]
		// The single compact-and-retry after a provider context-length
		// rejection follows the rejected request's snapshot for this trigger.
		retry := ok && parent.index == i-3 && events[i-2].Type == memory.EventContextSnapshot &&
			events[i-2].ParentID == event.ParentID
		if !ok || (parent.index != i-1 && parent.index != i-2 && !retry) {
			return fmt.Errorf("context snapshot event %q does not immediately follow its durable parent", event.ID)
		}
		if parent.index < i-1 {
			compaction := events[i-1]
			if compaction.Type != memory.EventContextCompacted || payload.ActiveCompactionEventID != compaction.ID {
				return fmt.Errorf("context snapshot event %q has an invalid intervening compaction", event.ID)
			}
			compactionPayload, err := decodeContextCompactionEvent(compaction)
			if err != nil || compactionPayload.Trigger != memory.ContextCompactionAutomatic {
				return fmt.Errorf("context snapshot event %q does not follow a valid automatic compaction", event.ID)
			}
			if payload.RetainedFirstEventID != compactionPayload.FirstRetainedEventID {
				return fmt.Errorf("context snapshot event %q retained frontier does not match its automatic compaction", event.ID)
			}
		}
		if parent.event.Type != memory.EventUserMessage && parent.event.Type != memory.EventToolSucceeded &&
			parent.event.Type != memory.EventToolFailed && parent.event.Type != memory.EventToolCancelled {
			return fmt.Errorf("context snapshot event %q parent is not a provider trigger", event.ID)
		}
		if payload.RetainedLastEventID != parent.event.ID || payload.RetainedLastSequence != parent.event.Sequence {
			return fmt.Errorf("context snapshot event %q retained endpoint does not match its parent", event.ID)
		}
		first, ok := byID[payload.RetainedFirstEventID]
		if !ok || first.index >= i || first.event.Sequence != payload.RetainedFirstSequence ||
			first.event.Type != memory.EventUserMessage || first.event.Role != memory.RoleUser || first.event.ParentID != "" ||
			first.event.Sequence > parent.event.Sequence {
			return fmt.Errorf("context snapshot event %q retained starting frontier is not a root user turn", event.ID)
		}
		seenPlaceholders := make(map[memory.EventID]struct{}, len(payload.Placeholders))
		var previousSequence int64
		for _, placeholder := range payload.Placeholders {
			projected, ok := byID[placeholder.EventID]
			if !ok || projected.index >= i || projected.event.Sequence < first.event.Sequence ||
				projected.event.Sequence > parent.event.Sequence ||
				(projected.event.Type != memory.EventToolSucceeded && projected.event.Type != memory.EventToolFailed &&
					projected.event.Type != memory.EventToolCancelled) {
				return fmt.Errorf("context snapshot event %q has an invalid placeholder event %q", event.ID, placeholder.EventID)
			}
			if _, duplicate := seenPlaceholders[placeholder.EventID]; duplicate || projected.event.Sequence <= previousSequence {
				return fmt.Errorf("context snapshot event %q has unordered or repeated placeholders", event.ID)
			}
			digest := sha256.Sum256([]byte(projected.event.Content))
			if placeholder.OriginalBytes != int64(len(projected.event.Content)) ||
				placeholder.SHA256 != hex.EncodeToString(digest[:]) {
				return fmt.Errorf("context snapshot event %q placeholder %q does not match durable content", event.ID, placeholder.EventID)
			}
			seenPlaceholders[placeholder.EventID] = struct{}{}
			previousSequence = projected.event.Sequence
		}
	}
	return nil
}

func validateSnapshotEvent(event memory.Event) error {
	if event.Role != "" || event.ExecutionID != "" || event.Content != "" || event.ParentID == "" {
		return fmt.Errorf("context snapshot event %q has invalid content-bearing envelope fields", event.ID)
	}
	var payload memory.ContextSnapshotPayload
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("decode context snapshot event %q: %w", event.ID, err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode context snapshot event %q trailer", event.ID)
	}
	if err := payload.Validate(); err != nil {
		return fmt.Errorf("validate context snapshot event %q: %w", event.ID, err)
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("canonicalize context snapshot event %q: %w", event.ID, err)
	}
	if !bytes.Equal(event.Payload, canonical) {
		return fmt.Errorf("context snapshot event %q payload is not canonical", event.ID)
	}
	return nil
}
