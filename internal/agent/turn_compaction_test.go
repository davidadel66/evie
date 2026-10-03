package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// workTool returns a deterministic result for each call. Results stay below
// the 4 KiB projection threshold, so pressure projection cannot shrink the
// active turn: only in-turn compaction can.
func workTool(resultBytes int) tools.Tool {
	count := 0
	return tools.Tool{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "work", Description: "Do one step of work.", Parameters: openrouter.Parameter{Type: "object"},
		}},
		Execute: func(context.Context, string) (string, error) {
			count++
			prefix := fmt.Sprintf("RESULT-%03d:", count)
			return prefix + strings.Repeat("r", resultBytes-len(prefix)), nil
		},
	}
}

// agenticClient asks for one work call per response until calls responses
// have asked for tools, then answers. Each call's arguments carry argBytes of
// payload, as a file write would, which projection never shortens.
type agenticClient struct {
	calls    int
	argBytes int
	answered int
	reqs     []openrouter.ChatRequest
	// reject lists 1-based request numbers the provider rejects for context
	// length before any output.
	reject map[int]bool
}

func (c *agenticClient) ChatStream(_ context.Context, req openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.reqs = append(c.reqs, req)
	if c.reject[len(c.reqs)] {
		return openrouter.ChatResponse{}, &openrouter.StreamError{
			Kind: openrouter.StreamProviderError, HTTPStatus: 400, ContextLengthExceeded: true,
			Err: errors.New("api returned status 400"),
		}
	}
	c.answered++
	if c.answered > c.calls || req.ToolChoice == "none" {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
			Role: "assistant", Content: "done",
		}}}}, nil
	}
	args := fmt.Sprintf(`{"step":%d,"data":%q}`, c.answered, strings.Repeat("a", c.argBytes))
	call := toolCall(fmt.Sprintf("call-%03d", c.answered), "work", args)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", ToolCalls: []openrouter.ToolCall{call},
	}}}}, nil
}

// turnSummaryClient answers every compactor call with a valid summary for its
// generation, unless err is set or block makes it wait for cancellation.
type turnSummaryClient struct {
	reqs    []openrouter.ChatRequest
	err     error
	block   bool
	entered chan struct{}
}

func turnWorkSummary(generation int) string {
	return strings.ReplaceAll(validCompactionSummary(), "kept", fmt.Sprintf("in-turn generation %d", generation))
}

func (c *turnSummaryClient) ChatStream(ctx context.Context, req openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.reqs = append(c.reqs, req)
	if c.entered != nil {
		close(c.entered)
		c.entered = nil
	}
	if c.block {
		<-ctx.Done()
		return openrouter.ChatResponse{}, ctx.Err()
	}
	if c.err != nil {
		return openrouter.ChatResponse{}, c.err
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: turnWorkSummary(len(c.reqs)),
	}}}}, nil
}

const longTurnRequest = "Rewrite every module, one step at a time."

// longTurnSession is a byte-budgeted session on a 200,000-byte working
// ceiling: usable input is 195,903 bytes and the compaction target 120,000.
func longTurnSession(t *testing.T, history *fakeHistory, conversation Client, compactor Client, options ...SessionOption) *Session {
	t.Helper()
	session := withByteBudgets(NewWithCompactor(conversation, compactor, automaticTestProfile(t, 200_000), history,
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner()))
	for _, option := range options {
		option(session)
	}
	return session
}

func eventsOfType(events []memory.Event, eventType memory.EventType) []memory.Event {
	var matched []memory.Event
	for _, event := range events {
		if event.Type == eventType {
			matched = append(matched, event)
		}
	}
	return matched
}

func turnCompactedPayloadOf(t *testing.T, event memory.Event) memory.ContextTurnCompactedPayload {
	t.Helper()
	var payload memory.ContextTurnCompactedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// checkTurnRequest verifies one conversational request of the long turn: the
// user request appears verbatim exactly once, an in-turn summary block (when
// present) immediately follows it, and every assistant tool call is followed
// by exactly its own results. It returns whether the request carries a
// summary block and the result labels it carries.
func checkTurnRequest(t *testing.T, index int, req openrouter.ChatRequest) (bool, []string) {
	t.Helper()
	roots, summarized := 0, false
	var results []string
	for i, message := range req.Messages {
		if message.Role == "user" && message.Content == longTurnRequest {
			roots++
			if i+1 < len(req.Messages) && strings.HasPrefix(req.Messages[i+1].Content, turnSummaryOpen) {
				summarized = true
			}
		}
		if message.Role == "user" && strings.HasPrefix(message.Content, turnSummaryOpen) &&
			(i == 0 || req.Messages[i-1].Content != longTurnRequest) {
			t.Fatalf("request %d: in-turn summary does not follow the user request", index)
		}
		if message.Role != "assistant" || len(message.ToolCalls) == 0 {
			continue
		}
		for j, call := range message.ToolCalls {
			if i+1+j >= len(req.Messages) || req.Messages[i+1+j].Role != "tool" || req.Messages[i+1+j].ToolCallID != call.ID {
				t.Fatalf("request %d: tool call %s is not followed by its result", index, call.ID)
			}
			results = append(results, req.Messages[i+1+j].Content[:len("RESULT-000")])
		}
	}
	if roots != 1 {
		t.Fatalf("request %d carries the user request %d times", index, roots)
	}
	return summarized, results
}

// replayTurnSnapshots recomposes every durable request of the session from
// the durable events that preceded its snapshot and requires the exact
// request hash the snapshot recorded.
func replayTurnSnapshots(t *testing.T, session *Session, events []memory.Event) int {
	t.Helper()
	ctx := context.Background()
	folder, err := session.workingFolder(ctx)
	if err != nil {
		t.Fatal(err)
	}
	replayed := 0
	for i, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		prior := events[:i]
		root := memory.EventID("")
		for _, candidate := range prior {
			if candidate.Type == memory.EventUserMessage {
				root = candidate.ID
			}
		}
		summary, _, err := reconstructCompactionChain(prior)
		if err != nil {
			t.Fatalf("snapshot %s: %v", event.ID, err)
		}
		// A compact-and-retry request assumed no more bytes per token than
		// the rejected request for the same trigger proved.
		var rejectedBytes int64
		if i >= 2 && events[i-2].Type == memory.EventContextSnapshot && events[i-2].ParentID == event.ParentID {
			var rejected memory.ContextSnapshotPayload
			if err := json.Unmarshal(events[i-2].Payload, &rejected); err != nil {
				t.Fatal(err)
			}
			rejectedBytes = rejected.SerializedBytes
		}
		standing, standingReceipt, err := session.standingMemory(ctx)
		if err != nil {
			t.Fatal(err)
		}
		workingContext := ""
		if provider, ok := session.history.(workingContextProvider); ok {
			if workingContext, err = provider.WorkingContext(ctx); err != nil {
				t.Fatal(err)
			}
		}
		composed, err := session.composer.Compose(ContextComposeInput{
			EnvironmentNote: workingFolderNote(folder), StandingMemory: standing, StandingMemoryReceipt: standingReceipt,
			Profile: session.profile, Summary: summary, Events: prior, ActiveRootID: root,
			TriggerEventID: event.ParentID, Iteration: snapshot.Iteration,
			Tools: session.modelToolset().Schemas(), Reasoning: session.reasoning, WorkingContext: workingContext,
			RejectedRequestBytes: rejectedBytes,
		})
		if err != nil {
			t.Fatalf("replay snapshot %s: %v", event.ID, err)
		}
		if composed.Snapshot.RequestSHA256 != snapshot.RequestSHA256 ||
			composed.Snapshot.ActiveTurnCompactionEventID != snapshot.ActiveTurnCompactionEventID {
			t.Fatalf("replay of snapshot %s (iteration %d) differs: sha %s want %s, turn compaction %q want %q",
				event.ID, snapshot.Iteration, composed.Snapshot.RequestSHA256, snapshot.RequestSHA256,
				composed.Snapshot.ActiveTurnCompactionEventID, snapshot.ActiveTurnCompactionEventID)
		}
		replayed++
	}
	return replayed
}

// withoutTurnCompactions removes in-turn compactions and the snapshots that
// name them: the history as projection alone would have to send it.
func withoutTurnCompactions(events []memory.Event) []memory.Event {
	var kept []memory.Event
	for _, event := range events {
		if event.Type != memory.EventContextTurnCompacted && event.Type != memory.EventContextSnapshot {
			kept = append(kept, event)
		}
	}
	return kept
}

// Stage 8: one long agentic turn keeps working past the point where
// projection alone would overflow. Its oldest completed tool groups are
// summarized into a labelled block after the verbatim user request, the
// newest groups stay verbatim, and every request is exactly reproducible from
// durable events.
func TestSendCompactsActiveTurnToolGroupsAndKeepsWorking(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000}
	compactor := &turnSummaryClient{}
	session := longTurnSession(t, history, conversation, compactor)
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatal(err)
	}
	events := history.allEvents()
	compactions := eventsOfType(events, memory.EventContextTurnCompacted)
	if len(conversation.reqs) != 31 || len(compactions) < 2 || len(compactor.reqs) != len(compactions) {
		t.Fatalf("conversation requests=%d compactions=%d compactor requests=%d", len(conversation.reqs), len(compactions), len(compactor.reqs))
	}
	if eventsOfType(events, memory.EventContextCompacted) != nil || eventsOfType(events, memory.EventTurnFailed) != nil {
		t.Fatalf("unexpected whole-turn compaction or failure: %v", eventTypes(events))
	}

	// Durable shape: each in-turn compaction follows its provider trigger,
	// is followed by the snapshot that names it, and advances one chain.
	index := make(map[memory.EventID]int, len(events))
	for i, event := range events {
		index[event.ID] = i
	}
	var prior memory.Event
	for generation, compaction := range compactions {
		i := index[compaction.ID]
		payload := turnCompactedPayloadOf(t, compaction)
		var next memory.ContextSnapshotPayload
		if err := json.Unmarshal(events[i+1].Payload, &next); err != nil {
			t.Fatal(err)
		}
		if events[i-1].ID != compaction.ParentID || events[i-1].Type != memory.EventToolSucceeded ||
			events[i+1].Type != memory.EventContextSnapshot || next.ActiveTurnCompactionEventID != compaction.ID {
			t.Fatalf("compaction %d durable shape: %v", generation+1, eventTypes(events[i-1:i+2]))
		}
		if payload.TurnID != events[0].ID || payload.Generation != int64(generation+1) ||
			payload.PromptVersion != memory.ContextTurnCompactionPromptVersion {
			t.Fatalf("compaction %d payload=%+v", generation+1, payload)
		}
		covered, retained := events[index[payload.CoveredLastEventID]], events[index[payload.FirstRetainedEventID]]
		if covered.Type != memory.EventToolSucceeded || retained.Type != memory.EventAssistantMessage {
			t.Fatalf("compaction %d cut is not between whole groups: %s then %s", generation+1, covered.Type, retained.Type)
		}
		if generation == 0 {
			if first := events[index[payload.CoveredFirstEventID]]; first.Type != memory.EventAssistantMessage || first.ParentID != events[0].ID {
				t.Fatalf("first generation does not start at the turn's first group: %+v", first)
			}
		} else {
			priorPayload := turnCompactedPayloadOf(t, prior)
			if payload.PriorCompactionEventID != prior.ID || payload.CoveredFirstEventID != priorPayload.FirstRetainedEventID {
				t.Fatalf("compaction %d does not advance the turn's chain", generation+1)
			}
		}
		prior = compaction
	}

	// The compactor sees the turn's request, the covered groups with their
	// result event IDs, and from the second generation the prior summary.
	for generation, request := range compactor.reqs {
		if request.Messages[0].Content != turnCompactionSystemPrompt || request.Tools != nil {
			t.Fatalf("compactor request %d does not use the in-turn prompt", generation+1)
		}
		transcript := request.Messages[1].Content
		payload := turnCompactedPayloadOf(t, compactions[generation])
		if !strings.Contains(transcript, longTurnRequest) ||
			!strings.Contains(transcript, `"result_event_id":"`+string(payload.CoveredLastEventID)+`"`) {
			t.Fatalf("compactor request %d transcript lacks the request or covered result IDs", generation+1)
		}
		hasPrior := strings.Contains(transcript, `"prior_summary":null`)
		if (generation == 0) != hasPrior {
			t.Fatalf("compactor request %d prior summary presence is wrong", generation+1)
		}
		if generation > 0 && !strings.Contains(transcript, "in-turn generation "+fmt.Sprint(generation)) {
			t.Fatalf("compactor request %d does not carry the prior summary forward", generation+1)
		}
	}

	// Every request keeps the user request verbatim and whole tool groups;
	// after the first compaction it carries the summary and keeps the newest
	// groups verbatim.
	firstCompacted := -1
	for i, request := range conversation.reqs {
		summarized, results := checkTurnRequest(t, i+1, request)
		if summarized && firstCompacted < 0 {
			firstCompacted = i
		}
		if firstCompacted >= 0 && !summarized {
			t.Fatalf("request %d lost the in-turn summary", i+1)
		}
		if i == len(conversation.reqs)-1 {
			if len(results) < turnCompactionRetainedGroups || results[len(results)-1] != "RESULT-030" ||
				results[len(results)-2] != "RESULT-029" || results[0] == "RESULT-001" {
				t.Fatalf("final request results=%v", results)
			}
		}
	}
	if firstCompacted < 0 {
		t.Fatal("no request carried an in-turn summary")
	}
	// Cache stability: between compaction points each request extends the
	// previous one unchanged; only an iteration that compacts rewrites the
	// tail after the user request.
	snapshots := snapshotPayloads(t, events)
	for i := 1; i < len(conversation.reqs); i++ {
		previous, current := conversation.reqs[i-1].Messages, conversation.reqs[i].Messages
		changed := snapshots[i].ActiveTurnCompactionEventID != snapshots[i-1].ActiveTurnCompactionEventID
		prefix := len(previous) <= len(current)
		for j := 0; prefix && j < len(previous); j++ {
			a, _ := json.Marshal(previous[j])
			b, _ := json.Marshal(current[j])
			prefix = string(a) == string(b)
		}
		if prefix == changed {
			t.Fatalf("request %d: extends previous=%v, compacted=%v", i+1, prefix, changed)
		}
	}
	last := conversation.reqs[len(conversation.reqs)-1]
	summaryBlock := ""
	for i, message := range last.Messages {
		if message.Content == longTurnRequest {
			summaryBlock = last.Messages[i+1].Content
		}
	}
	if summaryBlock != turnSummaryMessage(compactions[len(compactions)-1].Content) {
		t.Fatalf("final request summary block is not the latest durable summary")
	}

	// Projection alone cannot send the final request.
	finalSnapshot := eventsOfType(events, memory.EventContextSnapshot)
	trigger := finalSnapshot[len(finalSnapshot)-1].ParentID
	unprojected := withoutTurnCompactions(events)
	for i, event := range unprojected {
		if event.ID == trigger {
			unprojected = unprojected[:i+1]
			break
		}
	}
	_, err := session.composer.Compose(ContextComposeInput{
		Profile: session.profile, Events: unprojected, ActiveRootID: events[0].ID, TriggerEventID: trigger,
		Iteration: 31, Tools: session.modelToolset().Schemas(),
	})
	if !IsContextOverflow(err) {
		t.Fatalf("projection alone would have fit the final request: %v", err)
	}

	if replayed := replayTurnSnapshots(t, session, events); replayed != 31 {
		t.Fatalf("replayed %d snapshots", replayed)
	}
	if _, err := session.InspectContext(context.Background()); err != nil {
		t.Fatalf("/context after in-turn compaction: %v", err)
	}
}

// A turn that has no more than the retained groups, or whose request still
// fits, is never compacted in-turn: compaction is the last resort before a
// request cannot be sent.
func TestInTurnCompactionOnlyRunsWhenTheRequestCannotFit(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 10, argBytes: 10_000}
	compactor := &turnSummaryClient{}
	session := longTurnSession(t, history, conversation, compactor)
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatal(err)
	}
	if len(compactor.reqs) != 0 || eventsOfType(history.allEvents(), memory.EventContextTurnCompacted) != nil {
		t.Fatalf("a fitting turn was compacted in-turn: compactor requests=%d", len(compactor.reqs))
	}

	// Two huge groups cannot be compacted: both are retained verbatim.
	history = &fakeHistory{}
	conversation = &agenticClient{calls: 2, argBytes: 120_000}
	compactor = &turnSummaryClient{}
	session = longTurnSession(t, history, conversation, compactor)
	err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000))
	if !IsContextOverflow(err) || len(compactor.reqs) != 0 {
		t.Fatalf("Send error=%v compactor requests=%d", err, len(compactor.reqs))
	}
}

// firstInTurnCompactionIteration runs the long turn without limits and
// returns the iteration whose request first carried an in-turn summary.
func firstInTurnCompactionIteration(t *testing.T, calls, argBytes, resultBytes int) int {
	t.Helper()
	history := &fakeHistory{}
	session := longTurnSession(t, history, &agenticClient{calls: calls, argBytes: argBytes}, &turnSummaryClient{})
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(resultBytes)); err != nil {
		t.Fatal(err)
	}
	for _, snapshot := range snapshotPayloads(t, history.allEvents()) {
		if snapshot.ActiveTurnCompactionEventID != "" {
			return snapshot.Iteration
		}
	}
	t.Fatal("the long turn never compacted in-turn")
	return 0
}

// The 2026-10-01 memo covers in-turn compaction: after a failed automatic
// compaction no later iteration of the turn calls the compactor, and an
// in-turn compaction failure ends the turn with its own classification.
func TestInTurnCompactionFailureIsRememberedForTheTurn(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000}
	compactor := &turnSummaryClient{err: errors.New("summary unavailable")}
	session := longTurnSession(t, history, conversation, compactor)
	err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000))
	if err == nil || len(compactor.reqs) != 1 {
		t.Fatalf("Send error=%v compactor requests=%d", err, len(compactor.reqs))
	}
	events := history.allEvents()
	last := events[len(events)-1]
	terminal := terminalPayloadOf(t, last)
	if last.Type != memory.EventTurnFailed || terminal.Classification != memory.ClassificationProviderError ||
		terminal.Stage != memory.StageContextCompaction || eventsOfType(events, memory.EventContextTurnCompacted) != nil {
		t.Fatalf("terminal=%+v events=%v", terminal, eventTypes(events))
	}

	// A whole-turn compaction that failed earlier in the turn is remembered
	// too: when a later iteration no longer fits and only an in-turn
	// compaction could help, the compactor is not called again and the turn
	// fails with the remembered classification at composition.
	history = &fakeHistory{events: automaticPressureHistory(100_000)}
	conversation = &agenticClient{calls: 30, argBytes: 10_000}
	compactor = &turnSummaryClient{err: errors.New("summary unavailable")}
	session = longTurnSession(t, history, conversation, compactor)
	err = session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000))
	if err == nil || len(compactor.reqs) != 1 {
		t.Fatalf("Send error=%v compactor requests=%d", err, len(compactor.reqs))
	}
	events = history.allEvents()
	last = events[len(events)-1]
	terminal = terminalPayloadOf(t, last)
	if last.Type != memory.EventTurnFailed || terminal.Classification != memory.ClassificationProviderError ||
		terminal.Stage != memory.StageContextCompose {
		t.Fatalf("terminal=%+v", terminal)
	}
	snapshots := snapshotPayloads(t, events)
	if len(snapshots) < 2 || snapshots[len(snapshots)-1].CompactionFailureCategory != memory.ContextCompactionSummaryProvider {
		t.Fatalf("snapshots did not record the remembered failure: %+v", snapshots)
	}
	// The failing iteration's only plan was an in-turn compaction.
	failing := events[:len(events)-1]
	root := memory.EventID("")
	for _, event := range failing {
		if event.Type == memory.EventUserMessage {
			root = event.ID
		}
	}
	summary, _, err := reconstructCompactionChain(failing)
	if err != nil {
		t.Fatal(err)
	}
	plan, required, err := selectAutomaticCompaction(ContextComposeInput{
		Profile: session.profile, Summary: summary, Events: failing, ActiveRootID: root,
		TriggerEventID: last.ParentID, Iteration: snapshots[len(snapshots)-1].Iteration + 1,
		Tools: session.modelToolset().Schemas(),
	}, session.composer)
	if err != nil || !required || plan.turn == nil {
		t.Fatalf("failing iteration plan: required=%v in-turn=%v error=%v", required, plan.turn != nil, err)
	}
}

// C8: a provider context-length rejection can recover through an in-turn
// compaction. The durable shape is trigger, rejected snapshot, in-turn
// compaction, retry snapshot; the retry is not a new model response.
func TestContextRejectionRecoversThroughInTurnCompaction(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 8, argBytes: 10_000, reject: map[int]bool{6: true}}
	compactor := &turnSummaryClient{}
	session := longTurnSession(t, history, conversation, compactor)
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatal(err)
	}
	events := history.allEvents()
	compactions := eventsOfType(events, memory.EventContextTurnCompacted)
	if len(compactions) != 1 || len(compactor.reqs) != 1 || len(conversation.reqs) != 10 {
		t.Fatalf("compactions=%d compactor=%d conversation=%d", len(compactions), len(compactor.reqs), len(conversation.reqs))
	}
	i := 0
	for i = range events {
		if events[i].ID == compactions[0].ID {
			break
		}
	}
	shape := eventTypes(events[i-2 : i+3])
	if fmt.Sprint(shape) != fmt.Sprint([]memory.EventType{
		memory.EventToolSucceeded, memory.EventContextSnapshot, memory.EventContextTurnCompacted,
		memory.EventContextSnapshot, memory.EventAssistantMessage,
	}) || events[i-1].ParentID != compactions[0].ParentID || events[i+1].ParentID != compactions[0].ParentID {
		t.Fatalf("durable recovery shape=%v", shape)
	}
	var retry memory.ContextSnapshotPayload
	if err := json.Unmarshal(events[i+1].Payload, &retry); err != nil {
		t.Fatal(err)
	}
	if retry.Iteration != 6 || retry.ActiveTurnCompactionEventID != compactions[0].ID || retry.BytesPerTokenMilli == 0 {
		t.Fatalf("retry snapshot=%+v", retry)
	}
	if _, err := session.InspectContext(context.Background()); err != nil {
		t.Fatalf("recovery shape is not valid durable history: %v", err)
	}
	if replayed := replayTurnSnapshots(t, session, events); replayed != 10 {
		t.Fatalf("replayed %d snapshots", replayed)
	}
}

// C8 final pass: an iteration that already compacted in-turn before its
// rejected request has no recovery shape left and fails at once.
func TestContextRejectionAfterSameIterationInTurnCompactionFails(t *testing.T) {
	iteration := firstInTurnCompactionIteration(t, 30, 10_000, 3_000)
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000, reject: map[int]bool{iteration: true}}
	compactor := &turnSummaryClient{}
	session := longTurnSession(t, history, conversation, compactor)
	err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000))
	if !IsContextOverflow(err) || len(compactor.reqs) != 1 || len(conversation.reqs) != iteration {
		t.Fatalf("Send error=%v compactor=%d conversation=%d", err, len(compactor.reqs), len(conversation.reqs))
	}
	events := history.allEvents()
	last := events[len(events)-1]
	terminal := terminalPayloadOf(t, last)
	if terminal.Classification != memory.ClassificationContextOverflow || terminal.Stage != memory.StageProvider {
		t.Fatalf("terminal=%+v", terminal)
	}
	if _, err := session.InspectContext(context.Background()); err != nil {
		t.Fatalf("history invalid after failed recovery: %v", err)
	}
}

// The step limit's final tool-free call can follow an in-turn compaction in
// the same iteration: the request keeps tool_choice none, the summary, and
// the trailing harness note.
func TestStepLimitFinalCallAfterInTurnCompaction(t *testing.T) {
	iteration := firstInTurnCompactionIteration(t, 30, 10_000, 3_000)
	t.Setenv(TurnStepLimitEnv, fmt.Sprint(iteration))
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000}
	compactor := &turnSummaryClient{}
	session := longTurnSession(t, history, conversation, compactor)
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatal(err)
	}
	if len(conversation.reqs) != iteration || len(compactor.reqs) != 1 {
		t.Fatalf("conversation=%d compactor=%d", len(conversation.reqs), len(compactor.reqs))
	}
	final := conversation.reqs[len(conversation.reqs)-1]
	summarized, _ := checkTurnRequest(t, iteration, final)
	if !isStepLimitRequest(final) || !summarized {
		t.Fatalf("final request: step limit=%v summarized=%v", isStepLimitRequest(final), summarized)
	}
}

// A worker's wrap-up wins over in-turn compaction: the wrap-up request is
// fitted by its own reduction and the compactor is never called for it.
func TestWrapUpRequestIsFittedWithoutInTurnCompaction(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 80, argBytes: 200}
	compactor := &turnSummaryClient{}
	wrapUp := WithWrapUp(func(state WrapUpState) (string, bool) {
		return "Evie harness notice: write the report now.", state.RequestBytes > state.UsableBytes
	})
	session := longTurnSession(t, history, conversation, compactor, wrapUp)
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(4_000)); err != nil {
		t.Fatal(err)
	}
	final := conversation.reqs[len(conversation.reqs)-1]
	if len(compactor.reqs) != 0 || final.ToolChoice != "none" || len(conversation.reqs) >= 81 ||
		!strings.Contains(final.Messages[len(final.Messages)-1].Content, "write the report now") {
		t.Fatalf("compactor=%d requests=%d final tool choice=%q", len(compactor.reqs), len(conversation.reqs), final.ToolChoice)
	}
}

// Web cancel during the in-turn compactor call interrupts the turn at
// context_compaction and leaves no summary; the history stays valid.
func TestCancellationDuringInTurnCompactionLeavesNoSummary(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000}
	entered := make(chan struct{})
	compactor := &turnSummaryClient{block: true, entered: entered}
	session := longTurnSession(t, history, conversation, compactor)
	go func() {
		<-entered
		cancel()
	}()
	err := session.Send(ctx, longTurnRequest, &recorder{}, nil, workTool(3_000))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error=%v", err)
	}
	events := history.allEvents()
	last := events[len(events)-1]
	terminal := terminalPayloadOf(t, last)
	if last.Type != memory.EventTurnInterrupted || terminal.Stage != memory.StageContextCompaction ||
		eventsOfType(events, memory.EventContextTurnCompacted) != nil {
		t.Fatalf("terminal=%+v events=%v", terminal, eventTypes(events[len(events)-3:]))
	}
	if _, _, err := reconstructCompactionChain(events); err != nil {
		t.Fatalf("history invalid after cancellation: %v", err)
	}
}

// turnHistoryBuilder writes durable-shaped events for hand-built histories.
type turnHistoryBuilder struct {
	events  []memory.Event
	trigger memory.EventID
	root    memory.Event
}

func (b *turnHistoryBuilder) add(event memory.Event) memory.Event {
	event.SessionID = "test-session"
	event.Sequence = int64(len(b.events) + 1)
	event.FormatVersion = 1
	b.events = append(b.events, event)
	return event
}

func (b *turnHistoryBuilder) rootTurn(id, content string) memory.Event {
	b.root = b.add(memory.Event{ID: memory.EventID(id), Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
	b.trigger = b.root.ID
	return b.root
}

// group appends one assistant event with calls tool calls and their intents
// and results, in order, and returns the assistant and its results.
func (b *turnHistoryBuilder) group(label string, calls, resultBytes int) (memory.Event, []memory.Event) {
	var toolCalls []memory.ToolCall
	for i := 0; i < calls; i++ {
		toolCalls = append(toolCalls, memory.ToolCall{ID: fmt.Sprintf("%s-call-%d", label, i), Name: "work", Arguments: `{}`})
	}
	payload, _ := json.Marshal(memory.AssistantMessagePayload{ToolCalls: toolCalls})
	assistant := b.add(memory.Event{ID: memory.EventID(label + "-assistant"), ParentID: b.trigger,
		Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: payload})
	var results []memory.Event
	for i, call := range toolCalls {
		execution := memory.ExecutionID(fmt.Sprintf("%s-execution-%d", label, i))
		intentPayload, _ := json.Marshal(memory.ToolIntentPayload{Call: call})
		intent := b.add(memory.Event{ID: memory.EventID(fmt.Sprintf("%s-intent-%d", label, i)), ParentID: assistant.ID,
			Type: memory.EventToolIntent, ExecutionID: execution, Payload: intentPayload})
		resultPayload, _ := json.Marshal(memory.ToolResultPayload{ToolCallID: call.ID})
		content := fmt.Sprintf("%s result %d:", label, i) + strings.Repeat("r", resultBytes)
		results = append(results, b.add(memory.Event{ID: memory.EventID(fmt.Sprintf("%s-result-%d", label, i)), ParentID: intent.ID,
			Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: execution, Content: content, Payload: resultPayload}))
	}
	b.trigger = results[len(results)-1].ID
	return assistant, results
}

func (b *turnHistoryBuilder) turnCompaction(id string, generation int64, prior memory.EventID, first, last, retained memory.Event, summary string) memory.Event {
	digest := sha256.Sum256([]byte(summary))
	payload, _ := json.Marshal(memory.ContextTurnCompactedPayload{
		SchemaVersion: memory.ContextTurnCompactedSchemaVersion, TurnID: b.root.ID, Generation: generation,
		PriorCompactionEventID: prior,
		CoveredFirstEventID:    first.ID, CoveredFirstSequence: first.Sequence,
		CoveredLastEventID: last.ID, CoveredLastSequence: last.Sequence,
		FirstRetainedEventID: retained.ID, FirstRetainedSequence: retained.Sequence,
		CanonicalModel: "test/model", PromptVersion: memory.ContextTurnCompactionPromptVersion,
		SummaryBytes: int64(len(summary)), SummarySHA256: fmt.Sprintf("%x", digest),
	})
	return b.add(memory.Event{ID: memory.EventID(id), ParentID: b.trigger, Type: memory.EventContextTurnCompacted,
		Content: summary, Payload: payload})
}

// Both history validators share one in-turn frontier rule: a compaction
// covers a contiguous prefix of whole, complete tool groups of its own turn,
// keeps at least the group that triggered it, immediately follows that
// trigger, and advances the turn's chain exactly.
func TestDurableHistoryValidatesInTurnCompactionFrontier(t *testing.T) {
	type mutation func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event)
	validate := func(mutate mutation) error {
		b := &turnHistoryBuilder{}
		b.rootTurn("root", "do the work")
		var assistants []memory.Event
		var results [][]memory.Event
		for i := 0; i < 4; i++ {
			assistant, groupResults := b.group(fmt.Sprintf("g%d", i+1), 2, 100)
			assistants = append(assistants, assistant)
			results = append(results, groupResults)
		}
		mutate(b, assistants, results)
		return validateDurableContextHistory(b.events)
	}
	summary := validCompactionSummary()
	if err := validate(func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
		b.turnCompaction("tc-1", 1, "", a[0], r[1][1], a[2], summary)
		b.group("g5", 1, 100)
		b.turnCompaction("tc-2", 2, "tc-1", a[2], r[2][1], a[3], summary)
	}); err != nil {
		t.Fatalf("valid in-turn compaction chain rejected: %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate mutation
	}{
		{name: "splits a call from its group", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.turnCompaction("tc-1", 1, "", a[0], r[1][0], a[2], summary)
		}},
		{name: "retained frontier skips a group", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.turnCompaction("tc-1", 1, "", a[0], r[1][1], a[3], summary)
		}},
		{name: "first generation skips the first group", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.turnCompaction("tc-1", 1, "", a[1], r[1][1], a[2], summary)
		}},
		{name: "covers the triggering group", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			// The retained group would start after the compaction itself.
			later := memory.Event{ID: "g5-assistant", Sequence: int64(len(b.events) + 2)}
			b.turnCompaction("tc-1", 1, "", a[0], r[3][1], later, summary)
			b.group("g5", 1, 100)
		}},
		{name: "does not follow its trigger", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.trigger = r[2][1].ID
			b.turnCompaction("tc-1", 1, "", a[0], r[1][1], a[2], summary)
		}},
		{name: "names another turn", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.root.ID = "other-root"
			b.turnCompaction("tc-1", 1, "", a[0], r[1][1], a[2], summary)
		}},
		{name: "second generation without its prior", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.turnCompaction("tc-2", 2, "missing", a[2], r[2][1], a[3], summary)
		}},
		{name: "second generation does not advance", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.turnCompaction("tc-1", 1, "", a[0], r[0][1], a[1], summary)
			b.group("g5", 1, 100)
			b.turnCompaction("tc-2", 2, "tc-1", a[0], r[2][1], a[3], summary)
		}},
		{name: "invalid summary", mutate: func(b *turnHistoryBuilder, a []memory.Event, r [][]memory.Event) {
			b.turnCompaction("tc-1", 1, "", a[0], r[1][1], a[2], "not sectioned")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validate(test.mutate); err == nil {
				t.Fatal("invalid in-turn compaction accepted")
			}
		})
	}
}

// A snapshot names the in-turn compaction whose summary its request carried:
// the latest of its turn, or none.
func TestDurableHistoryRequiresSnapshotsToNameTheActiveInTurnCompaction(t *testing.T) {
	history := &fakeHistory{}
	conversation := &agenticClient{calls: 30, argBytes: 10_000}
	session := longTurnSession(t, history, conversation, &turnSummaryClient{})
	if err := session.Send(context.Background(), longTurnRequest, &recorder{}, nil, workTool(3_000)); err != nil {
		t.Fatal(err)
	}
	events := append([]memory.Event(nil), history.allEvents()...)
	if err := validateDurableContextHistory(events); err != nil {
		t.Fatal(err)
	}
	for i, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.ActiveTurnCompactionEventID == "" {
			continue
		}
		snapshot.ActiveTurnCompactionEventID = ""
		mutated := append([]memory.Event(nil), events...)
		mutated[i].Payload, _ = json.Marshal(snapshot)
		if err := validateDurableContextHistory(mutated); err == nil {
			t.Fatalf("snapshot %s without its active in-turn compaction accepted", event.ID)
		}
		return
	}
	t.Fatal("no snapshot named an in-turn compaction")
}

// A later whole-turn compaction summarizes a turn as the conversation saw it:
// the in-turn summary in place of its covered groups.
func TestWholeTurnCompactionSummarizesTheInTurnView(t *testing.T) {
	b := &turnHistoryBuilder{}
	b.rootTurn("turn-1", "first request")
	a1, _ := b.group("g1", 1, 100)
	_, r2 := b.group("g2", 1, 100)
	a3, _ := b.group("g3", 1, 100)
	summary := strings.ReplaceAll(validCompactionSummary(), "kept", "IN_TURN_SUMMARY")
	b.turnCompaction("tc-1", 1, "", a1, r2[0], a3, summary)
	b.group("g4", 1, 100)
	b.add(memory.Event{ID: "turn-1-answer", ParentID: b.trigger, Type: memory.EventAssistantMessage,
		Role: memory.RoleAssistant, Content: "first answer", Payload: json.RawMessage(`{}`)})
	for i := 2; i <= 4; i++ {
		b.rootTurn(fmt.Sprintf("turn-%d", i), fmt.Sprintf("request %d", i))
		b.add(memory.Event{ID: memory.EventID(fmt.Sprintf("turn-%d-answer", i)), ParentID: b.trigger,
			Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "answer", Payload: json.RawMessage(`{}`)})
	}
	plan, err := selectManualCompaction(b.events, testContextProfile("test/model"), CanonicalRequestEstimator{})
	if err != nil {
		t.Fatal(err)
	}
	transcript := plan.Request.Messages[1].Content
	if !strings.Contains(transcript, "IN_TURN_SUMMARY") || strings.Contains(transcript, "g1 result 0") ||
		!strings.Contains(transcript, "g4 result 0") {
		t.Fatalf("whole-turn compactor transcript does not use the in-turn view")
	}
}

// wrapUpPlaceholders describes only results the request carried: results an
// in-turn compaction covered are not in it.
func TestWrapUpPlaceholdersSkipInTurnCoveredResults(t *testing.T) {
	b := &turnHistoryBuilder{}
	b.rootTurn("root", "work")
	a1, r1 := b.group("g1", 1, 5_000)
	a2, _ := b.group("g2", 1, 5_000)
	b.group("g3", 1, 5_000)
	b.turnCompaction("tc-1", 1, "", a1, r1[0], a2, validCompactionSummary())
	reduced := reduceToolResults(b.events, wrapUpMarker)
	placeholders := wrapUpPlaceholders(b.events, reduced, memory.ContextSnapshotPayload{
		RetainedFirstSequence: 1, RetainedLastSequence: b.events[len(b.events)-1].Sequence,
	})
	for _, placeholder := range placeholders {
		if placeholder.EventID == r1[0].ID {
			t.Fatalf("placeholder for a covered result: %+v", placeholder)
		}
	}
	if len(placeholders) != 2 {
		t.Fatalf("placeholders=%+v", placeholders)
	}
}
