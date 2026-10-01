package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// Harness review Stage 4 (C1, C2, C3, C8): context budgeting regressions.

func budgetTestProfile(t *testing.T, model string, hard, working int64) openrouter.ContextProfile {
	t.Helper()
	profile, err := openrouter.NewExplicitContextProfile(model, hard, working, 16_384)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

// completedTurns returns n completed root turns whose user messages are size
// bytes each, numbered turn-1..turn-n from sequence 1.
func completedTurns(n, size int) []memory.Event {
	var events []memory.Event
	for i := 1; i <= n; i++ {
		events = append(events, completedCompactionTurn(
			fmt.Sprintf("turn-%d", i), int64(2*i-1), strings.Repeat(string(rune('a'+i-1)), size), "answer",
		)...)
	}
	return events
}

func withActiveRoot(events []memory.Event) []memory.Event {
	return append(events, memory.Event{
		ID: "active", Sequence: int64(len(events) + 1), Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "continue",
	})
}

func snapshotPayloads(t *testing.T, events []memory.Event) []memory.ContextSnapshotPayload {
	t.Helper()
	var snapshots []memory.ContextSnapshotPayload
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

// C1: planning reserved a ~97 KB worst-case summary encoding (16 KiB of '<',
// each escaped to six bytes). With the ~25 KB system prompt and tool schemas,
// no prefix could reach the 60 percent target below a ~205k working ceiling,
// so a session over its usable budget was stuck on context_overflow.
func TestAutomaticCompactionPlansOnSmallWorkingCeilings(t *testing.T) {
	for _, working := range []int64{131_072, 200_000} {
		t.Run(fmt.Sprint(working), func(t *testing.T) {
			input := ContextComposeInput{
				Profile: budgetTestProfile(t, "test/model", working, working),
				Events:  withActiveRoot(completedTurns(5, int(working*18/100))),
				Tools:   tools.BuiltinToolset().Schemas(), ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
			}
			plan, required, err := selectAutomaticCompaction(input, NewContextComposer(CanonicalRequestEstimator{}))
			if err != nil || !required || plan.FirstRetained.ID == "" {
				t.Fatalf("plan=%+v required=%v error=%v", plan.FirstRetained.ID, required, err)
			}
		})
	}
}

// C1: on a large window the worst-case reservation covered more history
// than the target needed.
func TestAutomaticCompactionDoesNotOverCompactLargeWindow(t *testing.T) {
	events := completedCompactionTurn("turn-1", 1, strings.Repeat("a", 60_000), "answer")
	events = append(events, completedCompactionTurn("turn-2", 3, strings.Repeat("b", 60_000), "answer")...)
	events = append(events, completedCompactionTurn("turn-3", 5, strings.Repeat("c", 60_000), "answer")...)
	events = append(events, completedCompactionTurn("turn-4", 7, strings.Repeat("d", 30_000), "answer")...)
	input := ContextComposeInput{
		Profile: budgetTestProfile(t, "test/model", 262_144, 262_144), Events: withActiveRoot(events),
		Tools: tools.BuiltinToolset().Schemas(), ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
	}
	plan, required, err := selectAutomaticCompaction(input, NewContextComposer(CanonicalRequestEstimator{}))
	if err != nil || !required {
		t.Fatalf("required=%v error=%v", required, err)
	}
	if plan.CoveredLast.ID != "turn-2-assistant" || plan.FirstRetained.ID != "turn-3" {
		t.Fatalf("plan covers through %s and retains from %s, want the two oldest turns only",
			plan.CoveredLast.ID, plan.FirstRetained.ID)
	}
}

// C1 acceptance: sessions on 128k and 200k working ceilings that previously
// stopped on context_overflow now compact automatically and keep going.
func TestSendCompactsAndContinuesOnSmallWorkingCeilings(t *testing.T) {
	for _, working := range []int64{131_072, 200_000} {
		t.Run(fmt.Sprint(working), func(t *testing.T) {
			history := &fakeHistory{events: completedTurns(5, int(working*70/100))}
			compactor := &fakeClient{steps: []step{assistantStep(validCompactionSummary(), nil)}}
			conversation := &fakeClient{steps: []step{assistantStep("done", nil), assistantStep("still going", nil)}}
			session := NewWithCompactor(conversation, compactor, budgetTestProfile(t, "test/model", working, working), history,
				memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
			if err := session.Send(context.Background(), "continue", &recorder{}, nil); err != nil {
				t.Fatalf("first turn: %v", err)
			}
			if len(compactor.reqs) != 1 || len(conversation.reqs) != 1 ||
				conversation.reqs[0].Messages[1].Content != validCompactionSummary() {
				t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
			}
			if err := session.Send(context.Background(), "and again", &recorder{}, nil); err != nil {
				t.Fatalf("second turn: %v", err)
			}
			if len(conversation.reqs) != 2 {
				t.Fatalf("conversation requests=%d", len(conversation.reqs))
			}
			for _, event := range history.allEvents() {
				if event.Type == memory.EventTurnFailed {
					t.Fatalf("turn failed: %s", event.Content)
				}
			}
		})
	}
}

// C1: the reservation is verified after the fact. A summary whose encoding
// cannot fit the usable budget is never accepted.
func TestAutomaticCompactionRejectsSummaryThatCannotFit(t *testing.T) {
	var oversized strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&oversized, "## %s\nkept\n\n", heading)
	}
	oversized.WriteString(strings.Repeat("<", CompactionSummaryMaxBytes-oversized.Len()))
	if err := validateCompactionSummary(oversized.String()); err != nil {
		t.Fatal(err)
	}
	working := int64(131_072)
	history := &fakeHistory{events: completedTurns(5, int(working*18/100))}
	compactor := &fakeClient{steps: []step{assistantStep(oversized.String(), nil)}}
	conversation := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	session := NewWithCompactor(conversation, compactor, budgetTestProfile(t, "test/model", working, working), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	session.composer = NewContextComposer(CanonicalRequestEstimator{})
	if err := session.Send(context.Background(), "continue", &recorder{}, nil); err == nil {
		t.Fatal("Send unexpectedly succeeded")
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 0 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	events := history.allEvents()
	for _, event := range events {
		if event.Type == memory.EventContextCompacted {
			t.Fatal("a summary that cannot fit became durable")
		}
	}
	terminal := terminalPayloadOf(t, events[len(events)-1])
	if terminal.Classification != memory.ClassificationProviderResponseInvalid || terminal.Stage != memory.StageContextCompaction {
		t.Fatalf("terminal=%+v", terminal)
	}
}

// C1: a summary larger than the reservation that still fits the usable
// budget is accepted; the 60 percent target is a planning preference.
func TestAutomaticCompactionAcceptsSummaryAboveReservationThatFits(t *testing.T) {
	var larger strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&larger, "## %s\nkept\n\n", heading)
	}
	larger.WriteString(strings.Repeat("<", 4_000))
	larger.WriteString(strings.Repeat("x", CompactionSummaryMaxBytes-larger.Len()))
	working := int64(131_072)
	history := &fakeHistory{events: completedTurns(5, int(working*18/100))}
	compactor := &fakeClient{steps: []step{assistantStep(larger.String(), nil)}}
	conversation := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := NewWithCompactor(conversation, compactor, budgetTestProfile(t, "test/model", working, working), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	session.composer = NewContextComposer(CanonicalRequestEstimator{})
	if err := session.Send(context.Background(), "continue", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	snapshots := snapshotPayloads(t, history.allEvents())
	if len(conversation.reqs) != 1 || len(snapshots) != 1 || snapshots[0].ActiveCompactionEventID == "" {
		t.Fatalf("conversation requests=%d snapshots=%+v", len(conversation.reqs), snapshots)
	}
	if snapshots[0].SerializedBytes <= percentageFloor(working, automaticCompactionTargetPercent) {
		t.Fatalf("fixture did not exceed the target: %d bytes", snapshots[0].SerializedBytes)
	}
}

// C1: /context must report what the turn path would send. Compose silently
// dropped old turns, so diagnostics showed headroom the turn never had.
func TestInspectContextReportsTheProjectionTheTurnWouldUse(t *testing.T) {
	working := int64(131_072)
	history := &fakeHistory{events: completedTurns(5, int(working*70/100))}
	session := New(nil, budgetTestProfile(t, "test/model", working, working), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	diagnostics, err := session.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projection := diagnostics.Projection
	if projection.RetainedFirstEventID != "turn-1" {
		t.Fatalf("projection silently dropped history: retained from %s", projection.RetainedFirstEventID)
	}
	if diagnostics.HeadroomBytes != projection.UsableInputBytes-projection.SerializedBytes || diagnostics.HeadroomBytes >= 0 {
		t.Fatalf("headroom=%d usable=%d serialized=%d", diagnostics.HeadroomBytes, projection.UsableInputBytes, projection.SerializedBytes)
	}
	plan := diagnostics.AutomaticCompaction
	if plan == nil || plan.CoveredFirstEventID != "turn-1" || plan.FirstRetainedEventID != "turn-4" {
		t.Fatalf("automatic compaction diagnostics=%+v", plan)
	}
	if !strings.Contains(strings.Join(diagnostics.Warnings, "\n"), "automatic compaction") {
		t.Fatalf("warnings=%v", diagnostics.Warnings)
	}
	if history.appendAttempts != 0 || history.snapshotCount != 0 {
		t.Fatal("inspection appended durable events")
	}
}

func TestInspectContextReportsAnUnrecoverableOverflow(t *testing.T) {
	working := int64(131_072)
	history := &fakeHistory{events: completedTurns(1, 400_000)}
	session := New(nil, budgetTestProfile(t, "test/model", working, working), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	diagnostics, err := session.InspectContext(context.Background())
	if err != nil {
		t.Fatalf("an over-budget history must be reported, not hidden behind an error: %v", err)
	}
	if diagnostics.HeadroomBytes >= 0 || diagnostics.AutomaticCompaction != nil ||
		!strings.Contains(strings.Join(diagnostics.Warnings, "\n"), "context_overflow") {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
}

// memoryDeliveryTally independently re-derives per-turn memory delivery: each
// evidence item and each memory-tool outcome counts once, the first time its
// exact bytes reach the provider in the turn.
type memoryDeliveryTally struct {
	seen  map[string]bool
	total int
}

func (m *memoryDeliveryTally) add(t *testing.T, request openrouter.ChatRequest, outcome func(openrouter.Message) bool) int {
	t.Helper()
	if m.seen == nil {
		m.seen = make(map[string]bool)
	}
	for _, message := range request.Messages {
		switch {
		case message.Role == "user" && strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n"):
			var block struct {
				Evidence []json.RawMessage `json:"evidence"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content, "EVIE_MEMORY_DATA\n")), &block); err != nil {
				t.Fatal(err)
			}
			for _, item := range block.Evidence {
				if m.seen["evidence:"+string(item)] {
					continue
				}
				m.seen["evidence:"+string(item)] = true
				escaped, err := json.Marshal(string(item))
				if err != nil {
					t.Fatal(err)
				}
				m.total += len(escaped) - 2
			}
		case message.Role == "tool" && outcome(message):
			encoded, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			if !m.seen["outcome:"+string(encoded)] {
				m.seen["outcome:"+string(encoded)] = true
				m.total += len(encoded)
			}
		}
	}
	return m.total
}

// distinctMemoryDelivery is the cumulative tally after each request.
func distinctMemoryDelivery(t *testing.T, requests []openrouter.ChatRequest, outcomeIDs map[string]bool) []int {
	t.Helper()
	var tally memoryDeliveryTally
	var cumulative []int
	for _, request := range requests {
		cumulative = append(cumulative, tally.add(t, request, func(message openrouter.Message) bool {
			return outcomeIDs[message.ToolCallID]
		}))
	}
	return cumulative
}

// C2: one memory search followed by ordinary tool calls re-sent the same
// evidence on every request and was charged each time, so the turn died
// around its eleventh request with a misleading context_overflow.
func TestMemoryBudgetChargesRepeatedDeliveryOncePerTurn(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := f.remember(source, memory.MemoryEverywhere, "tourmaline envelope")
	f.refresh()
	check := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "continue_check", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) { return "Checked the current request.", nil }}
	steps := []step{assistantStep("", nil, toolCall("search", "memory_search", `{"query":"tourmaline"}`))}
	for i := 1; i <= 16; i++ {
		steps = append(steps, assistantStep("", nil, toolCall(fmt.Sprintf("check-%d", i), "continue_check", `{}`)))
	}
	steps = append(steps, assistantStep("The tourmaline source is still current.", nil))
	client := &fakeClient{steps: steps}
	if err := f.session(reader, client, check).Send(context.Background(), "Check the envelope, then keep verifying.", &recorder{}, nil); err != nil {
		t.Fatalf("re-sending already delivered memory exhausted the turn: %v", err)
	}
	receipts := investigationReceipts(t, f, reader)
	if len(receipts) != len(client.reqs)-1 || len(receipts) != 17 {
		t.Fatalf("requests=%d receipts=%d", len(client.reqs), len(receipts))
	}
	want := distinctMemoryDelivery(t, client.reqs[1:], map[string]bool{"search": true})
	for i, receipt := range receipts {
		if len(receipt.Evidence) != 1 || receipt.Evidence[0].ClaimID != saved.ClaimID {
			t.Fatalf("receipt %d lost the original source: %+v", i, receipt)
		}
		if receipt.Investigation == nil || receipt.Investigation.CumulativeMemoryBytes != want[i] || want[i] != want[0] {
			t.Fatalf("receipt %d accounting=%+v, want %d (first delivery %d)", i, receipt.Investigation, want[i], want[0])
		}
	}
}

func budgetTestEvidence(id, text string) memory.RetrievalEvidence {
	return memory.RetrievalEvidence{
		ID: id, Kind: "claim", Intent: memory.RetrievalCurrent, Status: memory.SemanticStatusActive,
		CurrentStatus: memory.SemanticStatusActive, Text: text, ScopeKey: "global", Paths: []string{"lexical"},
	}
}

func TestAdmitRequestChargesOnlyUndeliveredMemory(t *testing.T) {
	r := &retrievalTurn{
		status:   memory.RetrievalSuccess,
		evidence: []memory.RetrievalEvidence{budgetTestEvidence("e1", "first <source> text")},
		toolIDs:  map[string]bool{"search": true},
	}
	outcome := openrouter.Message{Role: "tool", ToolCallID: "search", Content: `{"status":"success","matches":1}`}
	request := func() openrouter.ChatRequest {
		data, _ := r.renderProjection()
		return openrouter.ChatRequest{Messages: []openrouter.Message{
			{Role: "system", Content: "system"}, {Role: "user", Content: data}, outcome,
		}}
	}
	first := request()
	if _, _, reduced, err := r.admitRequest(first); err != nil || reduced {
		t.Fatalf("reduced=%v error=%v", reduced, err)
	}
	want := distinctMemoryDelivery(t, []openrouter.ChatRequest{first}, r.toolIDs)
	if r.delivered != want[0] {
		t.Fatalf("delivered=%d, want %d", r.delivered, want[0])
	}
	for i := 0; i < 20; i++ {
		if _, _, reduced, err := r.admitRequest(request()); err != nil || reduced || r.delivered != want[0] {
			t.Fatalf("re-send %d charged again: delivered=%d reduced=%v error=%v", i, r.delivered, reduced, err)
		}
	}
	r.evidence = append(r.evidence, budgetTestEvidence("e2", "second source"))
	second := request()
	if _, _, _, err := r.admitRequest(second); err != nil {
		t.Fatal(err)
	}
	want = distinctMemoryDelivery(t, []openrouter.ChatRequest{first, second}, r.toolIDs)
	if r.delivered != want[1] || want[1] <= want[0] {
		t.Fatalf("delivered=%d, want only the new item charged (%d)", r.delivered, want[1])
	}
}

// calibratingClient reports provider input tokens as a fixed function of the
// canonical request bytes, like a real tokenizer on uniform text.
type calibratingClient struct {
	bytesPerToken float64
	steps         []step
	reqs          []openrouter.ChatRequest
	bytes, tokens []int64
}

func (c *calibratingClient) ChatStream(_ context.Context, req openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	estimate, err := (CanonicalRequestEstimator{}).Estimate(req)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	tokens := int64(math.Ceil(float64(estimate.SerializedBytes) / c.bytesPerToken))
	c.reqs = append(c.reqs, req)
	c.bytes = append(c.bytes, estimate.SerializedBytes)
	c.tokens = append(c.tokens, tokens)
	s := assistantStep("done", nil)
	if len(c.steps) > 0 {
		s, c.steps = c.steps[0], c.steps[1:]
	}
	s.res.Usage = testProviderUsage(tokens, 1, tokens+1)
	return s.res, s.err
}

// expectedTokenRatio restates the documented calibration rule: until eight
// samples exist the ratio is at most 3 bytes per token; afterwards it is the
// minimum of the newest 32 samples, kept between 1 and 6 bytes per token.
func expectedTokenRatio(bytes, tokens []int64) int64 {
	if len(bytes) > 32 {
		bytes, tokens = bytes[len(bytes)-32:], tokens[len(tokens)-32:]
	}
	ratio := int64(math.MaxInt64)
	for i := range bytes {
		ratio = min(ratio, bytes[i]*1000/tokens[i])
	}
	if len(bytes) < 8 {
		ratio = min(ratio, 3000)
	}
	return max(1000, min(ratio, 6000))
}

// C3: tokens were estimated as one per canonical byte, about 4.5 times too
// many, although provider usage is recorded on every assistant event.
func TestSendCalibratesTokenBudgetFromRecordedProviderUsage(t *testing.T) {
	history := &fakeHistory{}
	client := &calibratingClient{bytesPerToken: 4.4}
	session := New(client, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	for i := 0; i < 10; i++ {
		if err := session.Send(context.Background(), fmt.Sprintf("turn %d", i), &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	usableTokens := int64(262_144 - 16_384 - 4_096)
	snapshots := snapshotPayloads(t, history.allEvents())
	if len(snapshots) != 10 {
		t.Fatalf("snapshots=%d", len(snapshots))
	}
	for k, snapshot := range snapshots {
		want := expectedTokenRatio(client.bytes[:k], client.tokens[:k])
		if snapshot.BytesPerTokenMilli != want || snapshot.CalibrationSamples != k ||
			snapshot.UsableInputBytes != usableTokens*want/1000 ||
			snapshot.RoughTokenEstimate != (snapshot.SerializedBytes*1000+want-1)/want ||
			snapshot.EstimatorVersion != CalibratedRequestEstimatorVersion {
			t.Fatalf("snapshot %d: ratio=%d samples=%d usable=%d rough=%d estimator=%s, want ratio %d",
				k, snapshot.BytesPerTokenMilli, snapshot.CalibrationSamples, snapshot.UsableInputBytes,
				snapshot.RoughTokenEstimate, snapshot.EstimatorVersion, want)
		}
	}
	if snapshots[0].BytesPerTokenMilli != 3000 || snapshots[9].BytesPerTokenMilli <= 4000 {
		t.Fatalf("first ratio=%d, calibrated ratio=%d", snapshots[0].BytesPerTokenMilli, snapshots[9].BytesPerTokenMilli)
	}

	// /context uses the same deterministic ratio the next request will use.
	diagnostics, err := session.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := expectedTokenRatio(client.bytes, client.tokens); diagnostics.Projection.BytesPerTokenMilli != want {
		t.Fatalf("diagnostic ratio=%d, want %d", diagnostics.Projection.BytesPerTokenMilli, want)
	}

	// Samples are per model: another model starts again from the floor.
	other := New(&calibratingClient{bytesPerToken: 4.4}, testContextProfile("other/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	if err := other.Send(context.Background(), "switch models", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	snapshots = snapshotPayloads(t, history.allEvents())
	if last := snapshots[len(snapshots)-1]; last.BytesPerTokenMilli != 3000 || last.CalibrationSamples != 0 {
		t.Fatalf("other model ratio=%d samples=%d", last.BytesPerTokenMilli, last.CalibrationSamples)
	}
}

// C3: a denser tokenizer lowers the ratio immediately; the floor is only an
// upper bound before enough samples exist.
func TestSendCalibrationFollowsADenserTokenizerImmediately(t *testing.T) {
	history := &fakeHistory{}
	client := &calibratingClient{bytesPerToken: 2}
	session := New(client, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	for i := 0; i < 2; i++ {
		if err := session.Send(context.Background(), fmt.Sprintf("turn %d", i), &recorder{}, nil); err != nil {
			t.Fatal(err)
		}
	}
	snapshots := snapshotPayloads(t, history.allEvents())
	if want := client.bytes[0] * 1000 / client.tokens[0]; snapshots[1].BytesPerTokenMilli != want || want >= 3000 {
		t.Fatalf("second ratio=%d, want %d", snapshots[1].BytesPerTokenMilli, want)
	}
}

func contextLengthRejection() step {
	return step{err: &openrouter.StreamError{
		Kind: openrouter.StreamProviderError, HTTPStatus: 400, ContextLengthExceeded: true,
		Err: errors.New("api returned status 400"),
	}}
}

// C8: a provider context-length 400 triggers exactly one compact-and-retry.
func TestSendCompactsAndRetriesOnceAfterProviderContextLengthRejection(t *testing.T) {
	history := &fakeHistory{events: completedTurns(3, 60_000)}
	compactor := &fakeClient{steps: []step{assistantStep(validCompactionSummary(), nil)}}
	conversation := &fakeClient{steps: []step{contextLengthRejection(), assistantStep("done", nil), assistantStep("next", nil)}}
	session := NewWithCompactor(conversation, compactor, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	if err := session.Send(context.Background(), "continue", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 2 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	first, err := (CanonicalRequestEstimator{}).Estimate(conversation.reqs[0])
	if err != nil {
		t.Fatal(err)
	}
	retried, err := (CanonicalRequestEstimator{}).Estimate(conversation.reqs[1])
	if err != nil {
		t.Fatal(err)
	}
	if retried.SerializedBytes >= first.SerializedBytes || conversation.reqs[1].Messages[1].Content != validCompactionSummary() {
		t.Fatalf("retry was not compacted: first=%d retried=%d", first.SerializedBytes, retried.SerializedBytes)
	}
	var shape []memory.EventType
	for _, event := range history.allEvents()[len(completedTurns(3, 1)):] {
		shape = append(shape, event.Type)
	}
	if fmt.Sprint(shape) != fmt.Sprint([]memory.EventType{
		memory.EventUserMessage, memory.EventContextSnapshot, memory.EventContextCompacted,
		memory.EventContextSnapshot, memory.EventAssistantMessage,
	}) {
		t.Fatalf("durable shape=%v", shape)
	}
	snapshots := snapshotPayloads(t, history.allEvents())
	// The retry assumes no more bytes per token than the rejected request
	// proved: its bytes over the hard window less the output reserve.
	bound := first.SerializedBytes * 1000 / (300_000 - 16_384)
	if snapshots[1].BytesPerTokenMilli != bound || snapshots[1].ActiveCompactionEventID == "" {
		t.Fatalf("retry snapshot ratio=%d, want %d (%+v)", snapshots[1].BytesPerTokenMilli, bound, snapshots[1])
	}
	if _, err := session.InspectContext(context.Background()); err != nil {
		t.Fatalf("retry shape is not valid durable history: %v", err)
	}
	if err := session.Send(context.Background(), "next turn", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	snapshots = snapshotPayloads(t, history.allEvents())
	if last := snapshots[len(snapshots)-1]; last.BytesPerTokenMilli > bound {
		t.Fatalf("the durable rejection did not bound later calibration: %d > %d", last.BytesPerTokenMilli, bound)
	}
}

func TestSendFailsWithContextOverflowWhenRetryIsAlsoRejected(t *testing.T) {
	history := &fakeHistory{events: completedTurns(3, 60_000)}
	compactor := &fakeClient{steps: []step{assistantStep(validCompactionSummary(), nil), assistantStep(validCompactionSummary(), nil)}}
	conversation := &fakeClient{steps: []step{contextLengthRejection(), contextLengthRejection(), assistantStep("must not run", nil)}}
	session := NewWithCompactor(conversation, compactor, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	err := session.Send(context.Background(), "continue", &recorder{}, nil)
	if !IsContextOverflow(err) {
		t.Fatalf("error=%v, want context overflow", err)
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 2 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	events := history.allEvents()
	last := events[len(events)-1]
	terminal := terminalPayloadOf(t, last)
	if last.Type != memory.EventTurnFailed || terminal.Classification != memory.ClassificationContextOverflow ||
		terminal.Stage != memory.StageProvider || terminal.HTTPStatus != nil || last.Content != terminal.SafeContent() {
		t.Fatalf("terminal event=%+v payload=%+v", last, terminal)
	}
}

func TestSendContextRejectionRecoveryRespectsCompactionFailure(t *testing.T) {
	history := &fakeHistory{events: completedTurns(3, 60_000)}
	compactor := &fakeClient{steps: []step{{err: errors.New("summary unavailable")}}}
	conversation := &fakeClient{steps: []step{contextLengthRejection(), assistantStep("must not run", nil)}}
	session := NewWithCompactor(conversation, compactor, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	if err := session.Send(context.Background(), "continue", &recorder{}, nil); err == nil {
		t.Fatal("Send unexpectedly succeeded")
	}
	if len(compactor.reqs) != 1 || len(conversation.reqs) != 1 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	events := history.allEvents()
	terminal := terminalPayloadOf(t, events[len(events)-1])
	if terminal.Classification != memory.ClassificationProviderError || terminal.Stage != memory.StageContextCompaction {
		t.Fatalf("terminal=%+v", terminal)
	}
}

func TestSendDoesNotRetryOrdinaryBadRequest(t *testing.T) {
	history := &fakeHistory{events: completedTurns(3, 60_000)}
	compactor := &fakeClient{}
	conversation := &fakeClient{steps: []step{{err: &openrouter.StreamError{
		Kind: openrouter.StreamProviderError, HTTPStatus: 400, Err: errors.New("api returned status 400"),
	}}}}
	session := NewWithCompactor(conversation, compactor, testContextProfile("test/model"), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	if err := session.Send(context.Background(), "continue", &recorder{}, nil); err == nil {
		t.Fatal("Send unexpectedly succeeded")
	}
	if len(compactor.reqs) != 0 || len(conversation.reqs) != 1 {
		t.Fatalf("compactor requests=%d conversation requests=%d", len(compactor.reqs), len(conversation.reqs))
	}
	events := history.allEvents()
	terminal := terminalPayloadOf(t, events[len(events)-1])
	if terminal.Classification != memory.ClassificationProviderError || terminal.HTTPStatus == nil || *terminal.HTTPStatus != 400 {
		t.Fatalf("terminal=%+v", terminal)
	}
}
