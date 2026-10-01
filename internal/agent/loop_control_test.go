package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

var toolPhaseCancellationCases = []struct {
	name  string
	gated bool
	// at is the callback whose start cancels the caller; want is the
	// lifecycle stage the caller interruption must capture.
	at   memory.TurnStage
	want memory.TurnStage
}{
	{name: "authorize preparation", gated: true, at: memory.StageToolPrepare, want: memory.StageToolPrepare},
	{name: "observe approval", gated: true, at: memory.StageToolApproval, want: memory.StageToolApproval},
	{name: "authorize approved execution", gated: true, at: memory.StageToolExecute, want: memory.StageToolExecute},
	{name: "authorize ungated execution", gated: false, at: memory.StageToolExecute, want: memory.StageToolPrepare},
}

func approveAll(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
	return tools.Approved
}

// L5: the caller is cancelled after the tool registry's context check and
// before the callback's own stage check, and no heartbeat goroutine has
// reserved the cause. The callback is therefore the first observer of the
// cancellation; it must not wait for the tool phase that only its own
// goroutine can end.
func TestToolPhaseCallbackCancellationDoesNotWaitOnItsOwnToolPhase(t *testing.T) {
	for _, test := range toolPhaseCancellationCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			coordinator := newTurnCoordinator(ctx)
			coordinator.setStage(memory.StageTurnStart)
			history := &fakeHistory{events: []memory.Event{{
				ID: "root", SessionID: "test-session", Sequence: 1,
				Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go",
			}}}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("call", "target", `{}`)),
				assistantStep("must not run", nil),
			}}
			s := ownedSession(client, history, &scriptedOwner{})
			ran := false
			s.toolset = s.toolset.WithTools([]tools.Tool{echoTool("target", test.gated, &ran)})
			s.timing.beforeToolPhaseCallback = func(stage memory.TurnStage) {
				if stage == test.at {
					cancel()
				}
			}
			lease := memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 7, Generation: 7}
			done := make(chan error, 1)
			go func() {
				done <- s.runOwnedTurn(coordinator, lease, &recorder{}, approveAll, &turnProgress{requestParentID: "root"})
			}()
			var err error
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("tool-phase callback deadlocked waiting for its own tool phase")
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("runOwnedTurn error=%v, want context.Canceled", err)
			}
			if selected := coordinator.result(); selected.kind != causeCallerCancelled || selected.stage != test.want {
				t.Fatalf("cause=%+v, want caller cancellation at %q", selected, test.want)
			}
			if ran || len(client.reqs) != 1 {
				t.Fatalf("tool ran=%v provider calls=%d", ran, len(client.reqs))
			}
			for _, event := range history.events {
				switch event.Type {
				case memory.EventToolSucceeded, memory.EventToolFailed, memory.EventToolCancelled:
					t.Fatalf("tool outcome committed after cancellation: %+v", history.events)
				}
			}
		})
	}
}

// L5 through Send: the cancelled turn returns, records its interruption, and
// leaves the session free for the next turn.
func TestToolPhaseCallbackCancellationReleasesSessionForNextTurn(t *testing.T) {
	for _, test := range toolPhaseCancellationCases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			history := &fakeHistory{}
			client := &fakeClient{steps: []step{
				assistantStep("", nil, toolCall("call", "target", `{}`)),
				assistantStep("next turn", nil),
			}}
			owner := &scriptedOwner{}
			s := ownedSession(client, history, owner)
			ran := false
			target := echoTool("target", test.gated, &ran)
			cancelled := false
			s.timing.beforeToolPhaseCallback = func(stage memory.TurnStage) {
				if stage == test.at && !cancelled {
					cancelled = true
					cancel()
				}
			}
			done := make(chan error, 1)
			go func() { done <- s.Send(ctx, "go", &recorder{}, approveAll, target) }()
			var err error
			select {
			case err = <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("Send deadlocked after caller cancellation in a tool-phase callback")
			}
			if !errors.Is(err, context.Canceled) || ran {
				t.Fatalf("Send error=%v tool ran=%v", err, ran)
			}
			last := history.events[len(history.events)-1]
			if last.Type != memory.EventTurnInterrupted {
				t.Fatalf("last event=%+v, want interruption", last)
			}
			payload := terminalPayloadOf(t, last)
			if payload.Classification != memory.ClassificationCallerCancelled || payload.Stage != test.want {
				t.Fatalf("terminal payload=%+v, want caller_cancelled at %q", payload, test.want)
			}
			if _, _, _, releases := owner.counts(); releases != 1 {
				t.Fatalf("releases=%d", releases)
			}
			if err := s.Send(context.Background(), "again", &recorder{}, approveAll, target); err != nil {
				t.Fatalf("next Send on the same session: %v", err)
			}
		})
	}
}

func countingEchoTool(name string, runs *int) tools.Tool {
	tool := echoTool(name, false, nil)
	tool.Execute = func(_ context.Context, args string) (string, error) {
		*runs++
		return "echo:" + args, nil
	}
	return tool
}

func isStepLimitRequest(req openrouter.ChatRequest) bool {
	last := req.Messages[len(req.Messages)-1]
	return req.ToolChoice == "none" && last.Role == "user" && strings.Contains(last.Content, "Evie harness notice")
}

func snapshotsOf(t *testing.T, events []memory.Event) []memory.ContextSnapshotPayload {
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

// L1: a model that keeps calling tools gets exactly one final request at the
// cap, with tool calls forbidden (schemas kept for providers that replay
// tool-call history) and a harness note, and its answer commits as the turn's
// success.
func TestStepLimitEndsRunawayToolLoopWithToolFreeFinalAnswer(t *testing.T) {
	t.Setenv(TurnStepLimitEnv, "3")
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{"n":1}`)),
		assistantStep("", nil, toolCall("call-2", "echo", `{"n":2}`)),
		assistantStep("Here is what I found so far.", nil),
		assistantStep("must not run", nil),
	}}
	runs := 0
	s := ownedSession(client, history, &scriptedOwner{})
	if err := s.Send(context.Background(), "keep going", &recorder{}, nil, countingEchoTool("echo", &runs)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(client.reqs) != 3 || runs != 2 {
		t.Fatalf("provider calls=%d tool runs=%d", len(client.reqs), runs)
	}
	for i, req := range client.reqs[:2] {
		if len(req.Tools) == 0 || req.ToolChoice != "" || isStepLimitRequest(req) {
			t.Fatalf("request %d restricted tools before the cap", i+1)
		}
	}
	final := client.reqs[2]
	finalNote := final.Messages[len(final.Messages)-1]
	if !isStepLimitRequest(final) || len(final.Tools) == 0 || !strings.Contains(finalNote.Content, "limit of 3 model responses") {
		t.Fatalf("final request tools=%d tool_choice=%q last message=%+v, want schemas kept, tool_choice none and a harness note", len(final.Tools), final.ToolChoice, finalNote)
	}
	if final.Messages[len(final.Messages)-2].Role != "tool" {
		t.Fatalf("harness note must follow the last tool result: %+v", final.Messages)
	}
	last := history.events[len(history.events)-1]
	if last.Type != memory.EventAssistantMessage || last.Content != "Here is what I found so far." {
		t.Fatalf("last durable event=%+v, want final assistant", last)
	}
	snapshots := snapshotsOf(t, history.allEvents())
	if len(snapshots) != 3 || snapshots[2].Iteration != 3 || snapshots[2].ToolSchemaCount != snapshots[1].ToolSchemaCount ||
		snapshots[2].MessageCount != len(final.Messages) || snapshots[1].ToolSchemaCount == 0 {
		t.Fatalf("snapshots=%+v", snapshots)
	}
	estimate, err := (CanonicalRequestEstimator{}).Estimate(final)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.RequestSHA256 != snapshots[2].RequestSHA256 {
		t.Fatal("final snapshot does not describe the request that was sent")
	}
}

// L1: when the tool-free final response still asks for tools, the turn ends
// with a distinct durable classification and nothing from that response is
// committed or executed.
func TestStepLimitFinalToolCallEndsTurnWithStepLimitClassification(t *testing.T) {
	t.Setenv(TurnStepLimitEnv, "2")
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
		assistantStep("Let me check one more thing", []string{"Let me check one more thing"}, toolCall("call-2", "echo", `{}`)),
		assistantStep("must not run", nil),
	}}
	runs := 0
	events := &recorder{}
	owner := &scriptedOwner{}
	s := ownedSession(client, history, owner)
	err := s.Send(context.Background(), "keep going", events, nil, countingEchoTool("echo", &runs))
	if !errors.Is(err, ErrStepLimitExceeded) {
		t.Fatalf("Send error=%v, want ErrStepLimitExceeded", err)
	}
	if len(client.reqs) != 2 || runs != 1 || !isStepLimitRequest(client.reqs[1]) {
		t.Fatalf("provider calls=%d tool runs=%d", len(client.reqs), runs)
	}
	var assistants int
	var outcome memory.Event
	for _, event := range history.events {
		switch event.Type {
		case memory.EventAssistantMessage:
			assistants++
		case memory.EventToolSucceeded:
			outcome = event
		}
	}
	if assistants != 1 {
		t.Fatalf("committed assistants=%d, want only the first", assistants)
	}
	last := history.events[len(history.events)-1]
	payload := terminalPayloadOf(t, last)
	if last.Type != memory.EventTurnFailed || payload.Classification != memory.ClassificationStepLimitExceeded ||
		payload.Stage != memory.StageProvider || payload.HTTPStatus != nil ||
		payload.TurnID != history.events[0].ID || last.ParentID != outcome.ID {
		t.Fatalf("terminal event=%+v payload=%+v", last, payload)
	}
	if err := payload.Validate(last.Type); err != nil || last.Content != payload.SafeContent() || last.Content == "" {
		t.Fatalf("terminal is not safe evidence: err=%v content=%q", err, last.Content)
	}
	want := fmt.Sprintf("discarded:%s:%s", DiscardProviderResponseInvalid, DiscardedResponseMessage)
	if !containsString(events.events, want) {
		t.Fatalf("callbacks=%v, want %q", events.events, want)
	}
	if _, _, _, releases := owner.counts(); releases != 1 {
		t.Fatalf("releases=%d", releases)
	}
}

// L1: the cap counts model responses, not transport retries of one response.
func TestStepLimitCountsModelResponsesNotTransportRetries(t *testing.T) {
	t.Setenv(TurnStepLimitEnv, "2")
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{
		{err: connectionFailure()},
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
		assistantStep("done", nil),
	}}
	runs := 0
	s := ownedSession(client, history, &scriptedOwner{})
	waits := recordProviderRetryWaits(s)
	if err := s.Send(context.Background(), "go", &recorder{}, nil, countingEchoTool("echo", &runs)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(client.reqs) != 3 || len(*waits) != 1 || runs != 1 {
		t.Fatalf("provider calls=%d retry waits=%d tool runs=%d", len(client.reqs), len(*waits), runs)
	}
	if isStepLimitRequest(client.reqs[0]) || isStepLimitRequest(client.reqs[1]) || !isStepLimitRequest(client.reqs[2]) {
		t.Fatal("a transport retry consumed a step")
	}
}

func TestStepLimitConfiguration(t *testing.T) {
	if DefaultTurnStepLimit != 100 || TurnStepLimitEnv != "EVIE_TURN_STEP_LIMIT" {
		t.Fatalf("default=%d env=%q", DefaultTurnStepLimit, TurnStepLimitEnv)
	}
	for _, test := range []struct {
		raw  string
		want int
	}{{raw: "", want: DefaultTurnStepLimit}, {raw: " 7 ", want: 7}, {raw: "1", want: 1}} {
		t.Setenv(TurnStepLimitEnv, test.raw)
		if err := ValidateTurnConfiguration(); err != nil {
			t.Fatalf("%q: %v", test.raw, err)
		}
		if s := ownedSession(&fakeClient{}, &fakeHistory{}, &scriptedOwner{}); s.configurationErr != nil || s.stepLimit != test.want {
			t.Fatalf("%q: limit=%d err=%v", test.raw, s.stepLimit, s.configurationErr)
		}
	}
	for _, raw := range []string{"0", "-1", "abc", "1.5"} {
		t.Setenv(TurnStepLimitEnv, raw)
		if err := ValidateTurnConfiguration(); err == nil || !strings.Contains(err.Error(), TurnStepLimitEnv) {
			t.Fatalf("%q: ValidateTurnConfiguration=%v", raw, err)
		}
		client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
		owner := &scriptedOwner{}
		err := ownedSession(client, &fakeHistory{}, owner).Send(context.Background(), "go", &recorder{}, nil)
		if err == nil || !strings.Contains(err.Error(), TurnStepLimitEnv) || len(client.reqs) != 0 {
			t.Fatalf("%q: Send error=%v provider calls=%d", raw, err, len(client.reqs))
		}
		if acquires, _, _, _ := owner.counts(); acquires != 0 {
			t.Fatalf("%q: acquired a lease with invalid configuration", raw)
		}
	}
}

// L1: delegated sessions run the same loop and guard, and the durable store
// accepts the step-limit terminal evidence.
func TestDelegatedSessionUsesTurnStepLimit(t *testing.T) {
	t.Setenv(TurnStepLimitEnv, "1")
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := standardManager(t, store)
	parent, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(ctx, parent.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call-1", "web_search", `{"query":"x"}`))}}
	session := NewDelegatedWithToolset(client, testContextProfile("test-model"), store.BindHistory(child.ID, "child"), child.ScopeContext(), store.BindTurnOwner(child.ID, "child"), resolved.Toolset, plugins.ResearchInstructions)
	if err := session.Send(ctx, "Assignment: research.", &recorder{}, nil); !errors.Is(err, ErrStepLimitExceeded) {
		t.Fatalf("Send error=%v, want ErrStepLimitExceeded", err)
	}
	if len(client.reqs) != 1 || !isStepLimitRequest(client.reqs[0]) {
		t.Fatalf("requests=%+v", client.reqs)
	}
	events, err := store.LoadEvents(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	last := events[len(events)-1]
	if payload := terminalPayloadOf(t, last); last.Type != memory.EventTurnFailed ||
		payload.Classification != memory.ClassificationStepLimitExceeded || payload.Stage != memory.StageProvider {
		t.Fatalf("last durable event=%+v", last)
	}
}
