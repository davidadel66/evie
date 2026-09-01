package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// fakeClient scripts the provider: each ChatStream call consumes the next
// step, replays its deltas through onDelta, and returns its response. The
// optional channels let the busy test hold a turn open mid-request.
type fakeClient struct {
	steps   []step
	reqs    []openrouter.ChatRequest
	entered chan struct{} // signaled when ChatStream is reached
	release chan struct{} // if non-nil, ChatStream waits on it
	onCall  func()
}

type step struct {
	reasoning []string // streamed before deltas, as real providers send it
	deltas    []string
	res       openrouter.ChatResponse
	err       error
}

type fakeHistory struct {
	events         []memory.Event
	ordered        []memory.Event
	snapshotCount  int
	snapshotErr    error
	appendAttempts int
	appendErrAt    int
	appendBlockAt  int
	appendEntered  chan struct{}
	appendErr      error
	eventsErr      error
	afterAppend    func(memory.EventInput)
	onEvents       func()
}

func (f *fakeHistory) Append(ctx context.Context, _ memory.TurnLease, input memory.EventInput) (memory.Event, error) {
	if f.ordered == nil {
		f.ordered = append([]memory.Event(nil), f.events...)
	}
	if input.Type == memory.EventContextSnapshot {
		f.snapshotCount++
		if f.snapshotErr != nil {
			return memory.Event{}, f.snapshotErr
		}
		event := memory.Event{
			ID: memory.EventID(fmt.Sprintf("snapshot-%d", f.snapshotCount)), SessionID: "test-session",
			Sequence: int64(len(f.ordered) + 1), ParentID: input.ParentID, Type: input.Type,
			Payload: append([]byte(nil), input.Payload...), FormatVersion: 1,
		}
		f.ordered = append(f.ordered, event)
		if f.afterAppend != nil {
			f.afterAppend(input)
		}
		return event, nil
	}
	f.appendAttempts++
	if f.appendBlockAt == f.appendAttempts {
		if f.appendEntered != nil {
			close(f.appendEntered)
		}
		<-ctx.Done()
		return memory.Event{}, ctx.Err()
	}
	if f.appendErr != nil && (f.appendErrAt == 0 || f.appendAttempts == f.appendErrAt) {
		return memory.Event{}, f.appendErr
	}
	event := memory.Event{
		ID:            memory.EventID(fmt.Sprintf("event-%d", len(f.events)+1)),
		SessionID:     memory.SessionID("test-session"),
		Sequence:      int64(len(f.ordered) + 1),
		ParentID:      input.ParentID,
		Type:          input.Type,
		Role:          input.Role,
		ExecutionID:   input.ExecutionID,
		Content:       input.Content,
		Payload:       append([]byte(nil), input.Payload...),
		FormatVersion: 1,
	}
	f.events = append(f.events, event)
	f.ordered = append(f.ordered, event)
	if f.afterAppend != nil {
		f.afterAppend(input)
	}
	return event, nil
}

func (f *fakeHistory) Events(_ context.Context) ([]memory.Event, error) {
	if f.onEvents != nil {
		f.onEvents()
	}
	if f.ordered != nil {
		return append([]memory.Event(nil), f.ordered...), f.eventsErr
	}
	return append([]memory.Event(nil), f.events...), f.eventsErr
}

func (f *fakeHistory) allEvents() []memory.Event {
	if f.ordered != nil {
		return f.ordered
	}
	return f.events
}

func newTestSession(client Client, model string) *Session {
	return New(client, testContextProfile(model), &fakeHistory{}, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: memory.SessionID("test-session"),
	}, newFakeTurnOwner())
}

func testContextProfile(model string) openrouter.ContextProfile {
	profile, err := openrouter.NewExplicitContextProfile(model, 300000, 262144, 16384)
	if err != nil {
		panic(err)
	}
	return profile
}

type fakeTurnOwner struct {
	lease memory.TurnLease
}

func newFakeTurnOwner() *fakeTurnOwner {
	return &fakeTurnOwner{lease: memory.TurnLease{
		SessionID: "test-session", HolderID: "test-holder", FencingToken: 1,
		Generation: 1, ExpiresAt: time.Now().Add(time.Minute),
	}}
}

func (o *fakeTurnOwner) Acquire(context.Context, time.Duration) (memory.TurnLease, error) {
	return o.lease, nil
}
func (o *fakeTurnOwner) Heartbeat(context.Context, memory.TurnLease, time.Duration) (memory.TurnLease, error) {
	return o.lease, nil
}
func (o *fakeTurnOwner) Authorize(context.Context, memory.TurnLease) error { return nil }
func (o *fakeTurnOwner) Release(context.Context, memory.TurnLease) error   { return nil }
func (o *fakeTurnOwner) IsConflict(error) bool                             { return false }
func (o *fakeTurnOwner) IsSessionInactive(error) bool                      { return false }
func (o *fakeTurnOwner) IsLeaseLost(error) bool                            { return false }

func (f *fakeClient) ChatStream(_ context.Context, req openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if f.onCall != nil {
		f.onCall()
	}
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
	f.reqs = append(f.reqs, req)
	if len(f.steps) == 0 {
		return openrouter.ChatResponse{}, errors.New("fake: no scripted step left")
	}
	s := f.steps[0]
	f.steps = f.steps[1:]
	for _, r := range s.reasoning {
		if h.OnReasoning != nil {
			h.OnReasoning(r)
		}
	}
	for _, d := range s.deltas {
		if h.OnContent != nil {
			h.OnContent(d)
		}
	}
	return s.res, s.err
}

func TestSendPersistsUserBeforeProviderCall(t *testing.T) {
	history := &fakeHistory{}
	providerSawUser := false
	c := &fakeClient{steps: []step{assistantStep("done", nil)}}
	c.onCall = func() {
		providerSawUser = len(history.events) == 1 &&
			history.events[0].Type == memory.EventUserMessage &&
			history.events[0].Role == memory.RoleUser &&
			history.events[0].Content == "hello"
	}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	if err := s.Send(context.Background(), "hello", &recorder{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !providerSawUser {
		t.Fatal("provider was called before the user event was durable")
	}
}

func TestSendComposesAndSnapshotsEveryProviderIteration(t *testing.T) {
	history := &fakeHistory{}
	providerEventCounts := []int{}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, openrouter.ToolCall{
			ID: "call-1", Type: "function", Function: openrouter.FunctionCall{Name: "echo", Arguments: `{"value":"hi"}`},
		}),
		assistantStep("done", nil),
	}}
	client.onCall = func() { providerEventCounts = append(providerEventCounts, len(history.allEvents())) }
	profile := testContextProfile("test-model")
	session := New(client, profile, history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	extra := echoTool("echo", false, nil)

	if err := session.Send(context.Background(), "go", &recorder{}, nil, extra); err != nil {
		t.Fatal(err)
	}
	wantTypes := []memory.EventType{
		memory.EventUserMessage,
		memory.EventContextSnapshot,
		memory.EventAssistantMessage,
		memory.EventToolIntent,
		memory.EventToolSucceeded,
		memory.EventContextSnapshot,
		memory.EventAssistantMessage,
	}
	allEvents := history.allEvents()
	if len(allEvents) != len(wantTypes) {
		t.Fatalf("events=%+v", allEvents)
	}
	for i, want := range wantTypes {
		if allEvents[i].Type != want {
			t.Fatalf("event %d type=%q, want %q", i, allEvents[i].Type, want)
		}
	}
	if fmt.Sprint(providerEventCounts) != "[2 6]" {
		t.Fatalf("provider observed event counts %v, want snapshots committed first", providerEventCounts)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("provider requests=%d", len(client.reqs))
	}
	toolSchemas := tools.SchemasWith([]tools.Tool{extra})
	wantRequests := []openrouter.ChatRequest{
		{
			Model: "test-model", Stream: true, Reasoning: cloneReasoning(session.reasoning), MaxTokens: 16384,
			Messages: []openrouter.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: "go"},
			},
			Tools: toolSchemas,
		},
		{
			Model: "test-model", Stream: true, Reasoning: cloneReasoning(session.reasoning), MaxTokens: 16384,
			Messages: []openrouter.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: "go"},
				{Role: "assistant", ToolCalls: []openrouter.ToolCall{{
					ID: "call-1", Type: "function", Function: openrouter.FunctionCall{Name: "echo", Arguments: `{"value":"hi"}`},
				}}},
				{Role: "tool", Content: `echo:{"value":"hi"}`, ToolCallID: "call-1"},
			},
			Tools: toolSchemas,
		},
	}
	if !reflect.DeepEqual(client.reqs, wantRequests) {
		t.Fatalf("provider requests mismatch\n got: %#v\nwant: %#v", client.reqs, wantRequests)
	}
	for i, eventIndex := range []int{1, 5} {
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(allEvents[eventIndex].Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		estimate, err := (CanonicalRequestEstimator{}).Estimate(client.reqs[i])
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Iteration != i+1 || snapshot.RequestSHA256 != estimate.RequestSHA256 ||
			snapshot.SerializedBytes != estimate.SerializedBytes {
			t.Fatalf("snapshot %d=%+v estimate=%+v", i, snapshot, estimate)
		}
		if allEvents[eventIndex].Content != "" {
			t.Fatalf("snapshot stored content: %+v", allEvents[eventIndex])
		}
	}
	if allEvents[1].ParentID != allEvents[0].ID || allEvents[5].ParentID != allEvents[4].ID {
		t.Fatalf("snapshot parents=%q,%q", allEvents[1].ParentID, allEvents[5].ParentID)
	}

	firstComposition, err := session.composer.Compose(ContextComposeInput{
		Profile: profile, Events: []memory.Event{allEvents[0]}, ActiveRootID: allEvents[0].ID,
		TriggerEventID: allEvents[0].ID, Iteration: 1, Tools: toolSchemas, Reasoning: session.reasoning,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondComposition, err := session.composer.Compose(ContextComposeInput{
		Profile: profile, Events: []memory.Event{allEvents[0], allEvents[2], allEvents[3], allEvents[4]},
		ActiveRootID: allEvents[0].ID, TriggerEventID: allEvents[4].ID, Iteration: 2,
		Tools: toolSchemas, Reasoning: session.reasoning,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstSnapshotJSON, err := json.Marshal(firstComposition.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshotJSON, err := json.Marshal(secondComposition.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	executionID := allEvents[3].ExecutionID
	if executionID == "" || allEvents[4].ExecutionID != executionID {
		t.Fatalf("tool execution correlation=%q,%q", executionID, allEvents[4].ExecutionID)
	}
	wantEvents := []memory.Event{
		{ID: "event-1", SessionID: "test-session", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go", FormatVersion: 1},
		{ID: "snapshot-1", SessionID: "test-session", Sequence: 2, ParentID: "event-1", Type: memory.EventContextSnapshot, Payload: firstSnapshotJSON, FormatVersion: 1},
		{ID: "event-2", SessionID: "test-session", Sequence: 3, ParentID: "event-1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Payload: json.RawMessage(`{"tool_calls":[{"id":"call-1","name":"echo","arguments":"{\"value\":\"hi\"}"}]}`), FormatVersion: 1},
		{ID: "event-3", SessionID: "test-session", Sequence: 4, ParentID: "event-2", Type: memory.EventToolIntent, ExecutionID: executionID,
			Payload: json.RawMessage(`{"call":{"id":"call-1","name":"echo","arguments":"{\"value\":\"hi\"}"}}`), FormatVersion: 1},
		{ID: "event-4", SessionID: "test-session", Sequence: 5, ParentID: "event-3", Type: memory.EventToolSucceeded, Role: memory.RoleTool,
			ExecutionID: executionID, Content: `echo:{"value":"hi"}`, Payload: json.RawMessage(`{"tool_call_id":"call-1","is_error":false}`), FormatVersion: 1},
		{ID: "snapshot-2", SessionID: "test-session", Sequence: 6, ParentID: "event-4", Type: memory.EventContextSnapshot, Payload: secondSnapshotJSON, FormatVersion: 1},
		{ID: "event-5", SessionID: "test-session", Sequence: 7, ParentID: "event-4", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "done", Payload: json.RawMessage(`{}`), FormatVersion: 1},
	}
	if !reflect.DeepEqual(allEvents, wantEvents) {
		t.Fatalf("ordered durable history mismatch\n got: %#v\nwant: %#v", allEvents, wantEvents)
	}
}

func TestSessionsKeepSeparateToolsetsAcrossProviderIterations(t *testing.T) {
	newSession := func(toolName, result string) (*Session, *fakeClient) {
		client := &fakeClient{steps: []step{
			assistantStep("", nil, toolCall("call-1", toolName, `{}`)),
			assistantStep("done", nil),
		}}
		toolset := tools.NewToolset([]tools.Tool{{
			Schema: openrouter.Tool{
				Type: "function",
				Function: openrouter.Function{
					Name: toolName, Parameters: openrouter.Parameter{Type: "object"},
				},
			},
			Execute: func(context.Context, string) (string, error) { return result, nil },
		}})
		return NewWithToolset(
			client,
			testContextProfile("test-model"),
			&fakeHistory{},
			memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: memory.SessionID(toolName)},
			newFakeTurnOwner(),
			toolset,
		), client
	}

	first, firstClient := newSession("first_only", "first result")
	second, secondClient := newSession("second_only", "second result")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, session := range []*Session{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- session.Send(context.Background(), "go", &recorder{}, nil)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	assertPinned := func(client *fakeClient, wantName, wantResult string) {
		t.Helper()
		if len(client.reqs) != 2 {
			t.Fatalf("%s provider requests = %d, want 2", wantName, len(client.reqs))
		}
		for i, req := range client.reqs {
			if len(req.Tools) != 1 || req.Tools[0].Function.Name != wantName {
				t.Fatalf("%s request %d tools = %+v", wantName, i, req.Tools)
			}
		}
		messages := client.reqs[1].Messages
		if got := messages[len(messages)-1]; got.Role != "tool" || got.Content != wantResult {
			t.Fatalf("%s second request result = %+v", wantName, got)
		}
	}
	assertPinned(firstClient, "first_only", "first result")
	assertPinned(secondClient, "second_only", "second result")
}

func TestSessionToolsetReturnsUnknownForAbsentTool(t *testing.T) {
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "absent", `{}`)),
		assistantStep("done", nil),
	}}
	session := NewWithToolset(
		client,
		testContextProfile("test-model"),
		&fakeHistory{},
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"},
		newFakeTurnOwner(),
		tools.NewToolset(nil),
	)

	if err := session.Send(context.Background(), "go", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 2 || len(client.reqs[0].Tools) != 0 || len(client.reqs[1].Tools) != 0 {
		t.Fatalf("provider requests = %+v", client.reqs)
	}
	result := client.reqs[1].Messages[len(client.reqs[1].Messages)-1]
	if result.Role != "tool" || result.ToolCallID != "call-1" || result.Content != "Unknown Tool Call: absent" {
		t.Fatalf("unknown tool result = %+v", result)
	}
}

func TestSendFailsClosedWhenContextSnapshotCannotPersist(t *testing.T) {
	history := &fakeHistory{snapshotErr: errors.New("disk full")}
	client := &fakeClient{steps: []step{assistantStep("not called", nil)}}
	session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())

	err := session.Send(context.Background(), "hello", &recorder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "persist context snapshot") {
		t.Fatalf("Send error=%v", err)
	}
	if len(client.reqs) != 0 || len(history.events) != 1 {
		t.Fatalf("provider requests=%d events=%+v", len(client.reqs), history.events)
	}
}

func TestSendRecordsContextOverflowBeforeProviderTransport(t *testing.T) {
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{assistantStep("not called", nil)}}
	session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())

	err := session.Send(context.Background(), strings.Repeat("x", 250000), &recorder{}, nil)
	if !errors.Is(err, ErrContextOverflow) {
		t.Fatalf("Send error=%v, want ErrContextOverflow", err)
	}
	if len(client.reqs) != 0 || len(history.events) != 2 {
		t.Fatalf("provider requests=%d events=%+v", len(client.reqs), history.events)
	}
	terminal := history.events[1]
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if terminal.Type != memory.EventTurnFailed || terminal.Content != "The turn could not fit within the configured model context." ||
		payload.Classification != memory.ClassificationContextOverflow || payload.Stage != memory.StageContextCompose {
		t.Fatalf("overflow terminal=%+v payload=%+v", terminal, payload)
	}
}

func TestSendStopsWhenUserEventAppendFails(t *testing.T) {
	history := &fakeHistory{appendErr: errors.New("disk full")}
	c := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	err := s.Send(context.Background(), "hello", &recorder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Send error = %v, want durable append failure", err)
	}
	if len(c.reqs) != 0 {
		t.Fatalf("provider received %d requests after append failure", len(c.reqs))
	}
}

func TestSendPersistsAssistantBeforeToolExecution(t *testing.T) {
	history := &fakeHistory{}
	intentWasDurable := false
	tool := echoTool("echo", false, nil)
	tool.Execute = func(_ context.Context, args string) (string, error) {
		if len(history.events) != 3 || history.events[1].Type != memory.EventAssistantMessage ||
			history.events[2].Type != memory.EventToolIntent ||
			history.events[2].ParentID != history.events[1].ID || history.events[2].ExecutionID == "" {
			return "echo:" + args, nil
		}
		var payload memory.ToolIntentPayload
		if err := json.Unmarshal(history.events[2].Payload, &payload); err == nil &&
			payload.Call.ID == "call-1" && payload.Call.Name == "echo" &&
			payload.Call.Arguments == `{"x":1}` {
			intentWasDurable = true
		}
		return "echo:" + args, nil
	}
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{"x":1}`)),
		assistantStep("done", nil),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	if err := s.Send(context.Background(), "go", &recorder{}, nil, tool); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !intentWasDurable {
		t.Fatal("tool executed before its intent was durable")
	}
	if len(history.events) < 5 {
		t.Fatalf("durable turn events = %+v", history.events)
	}
	intentEvent := history.events[2]
	outcomeEvent := history.events[3]
	if outcomeEvent.Type != memory.EventToolSucceeded || outcomeEvent.Role != memory.RoleTool ||
		outcomeEvent.ParentID != intentEvent.ID || outcomeEvent.ExecutionID != intentEvent.ExecutionID ||
		outcomeEvent.Content != `echo:{"x":1}` {
		t.Errorf("tool outcome event = %+v, intent = %+v", outcomeEvent, intentEvent)
	}
	var outcomePayload memory.ToolResultPayload
	if err := json.Unmarshal(outcomeEvent.Payload, &outcomePayload); err != nil ||
		outcomePayload.ToolCallID != "call-1" || outcomePayload.IsError {
		t.Errorf("tool outcome payload = %+v, error = %v", outcomePayload, err)
	}
	last := history.events[len(history.events)-1]
	if last.Type != memory.EventAssistantMessage || last.Content != "done" {
		t.Errorf("durable turn events = %+v", history.events)
	}
}

func TestSendStopsWhenAssistantEventAppendFails(t *testing.T) {
	history := &fakeHistory{appendErrAt: 2, appendErr: errors.New("disk full")}
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	err := s.Send(context.Background(), "go", &recorder{}, nil, echoTool("echo", false, &ran))
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Send error = %v, want assistant append failure", err)
	}
	if ran {
		t.Fatal("tool executed after assistant append failure")
	}
}

func TestSendStopsWhenToolIntentAppendFails(t *testing.T) {
	history := &fakeHistory{appendErrAt: 3, appendErr: errors.New("disk full")}
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	err := s.Send(context.Background(), "go", &recorder{}, nil, echoTool("echo", false, &ran))
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Send error = %v, want tool intent append failure", err)
	}
	if ran {
		t.Fatal("tool executed after intent append failure")
	}
}

func TestSendParentCancellationDuringToolStopsLaterToolsAndProviderIterations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := echoTool("cancel-turn", false, nil)
	first.Execute = func(ctx context.Context, _ string) (string, error) {
		cancel()
		return "", ctx.Err()
	}
	secondRan := false
	second := echoTool("must-not-run", false, &secondRan)
	c := &fakeClient{steps: []step{
		assistantStep("", nil,
			toolCall("call-1", "cancel-turn", `{}`),
			toolCall("call-2", "must-not-run", `{}`)),
		assistantStep("must not be requested", nil),
	}}
	s := newTestSession(c, "test-model")

	err := s.Send(ctx, "go", &recorder{}, nil, first, second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
	if secondRan {
		t.Fatal("later tool executed after parent cancellation")
	}
	if len(c.reqs) != 1 {
		t.Fatalf("provider requests = %d, want exactly the initial iteration", len(c.reqs))
	}
}

func TestSendParentCancellationDuringProviderDiscardsResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("late", nil, toolCall("call-1", "must-not-run", `{}`)),
	}}
	c.onCall = cancel
	s := newTestSession(c, "test-model")

	err := s.Send(ctx, "go", &recorder{}, nil, echoTool("must-not-run", false, &ran))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
	if ran {
		t.Fatal("tool executed from provider response returned after cancellation")
	}
}

func TestSendCancellationDuringHistoryLoadPreventsProvider(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	history := &fakeHistory{
		eventsErr: errors.New("competing history error"),
		onEvents:  cancel,
	}
	client := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	s := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())

	err := s.Send(ctx, "go", &recorder{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled", err)
	}
	if len(client.reqs) != 0 {
		t.Fatalf("provider requests = %d after history cancellation", len(client.reqs))
	}
}

func TestSendParentCancellationWinsOverCompetingProviderError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeClient{
		steps:  []step{{err: errors.New("competing provider error")}},
		onCall: cancel,
	}
	s := newTestSession(client, "test-model")

	err := s.Send(ctx, "go", &recorder{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send error = %v, want context.Canceled instead of provider error", err)
	}
}

func TestCommittedFinalAssistantRemainsDurableSuccess(t *testing.T) {
	t.Run("assistant append", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		history := &fakeHistory{afterAppend: func(input memory.EventInput) {
			if input.Type == memory.EventAssistantMessage {
				cancel()
			}
		}}
		client := &fakeClient{steps: []step{assistantStep("late final", nil)}}
		events := &recorder{}
		s := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: "test-session",
		}, newFakeTurnOwner())

		err := s.Send(ctx, "go", events, nil)
		if err != nil {
			t.Fatalf("Send error = %v, want durable success", err)
		}
		if len(events.events) != 1 || events.events[0] != "done:late final" {
			t.Fatalf("events after committed assistant = %v", events.events)
		}
	})

	t.Run("AssistantDone callback", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client := &fakeClient{steps: []step{assistantStep("late final", nil)}}
		events := &recorder{onAssistantDone: cancel}
		s := newTestSession(client, "test-model")

		err := s.Send(ctx, "go", events, nil)
		if err != nil {
			t.Fatalf("Send error = %v, want durable success", err)
		}
		if len(client.reqs) != 1 {
			t.Fatalf("provider requests = %d, want one", len(client.reqs))
		}
	})
}

func TestSendCancellationAtToolIntentAndOutcomeBoundariesStopsLaterPhases(t *testing.T) {
	t.Run("intent append", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		history := &fakeHistory{afterAppend: func(input memory.EventInput) {
			if input.Type == memory.EventToolIntent {
				cancel()
			}
		}}
		client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call-1", "echo", `{}`))}}
		ran := false
		events := &recorder{}
		s := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: "test-session",
		}, newFakeTurnOwner())

		err := s.Send(ctx, "go", events, nil, echoTool("echo", false, &ran))
		if !errors.Is(err, context.Canceled) || ran {
			t.Fatalf("Send error = %v, tool ran = %v", err, ran)
		}
		for _, event := range events.events {
			if strings.HasPrefix(event, "call:") {
				t.Fatalf("ToolCall emitted after intent cancellation: %v", events.events)
			}
		}
	})

	t.Run("ToolCall callback", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("call-1", "echo", `{}`))}}
		ran := false
		events := &recorder{onToolCall: cancel}
		s := newTestSession(client, "test-model")

		err := s.Send(ctx, "go", events, nil, echoTool("echo", false, &ran))
		if !errors.Is(err, context.Canceled) || ran {
			t.Fatalf("Send error = %v, tool ran = %v", err, ran)
		}
	})

	t.Run("outcome append", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		history := &fakeHistory{afterAppend: func(input memory.EventInput) {
			if input.Type == memory.EventToolSucceeded {
				cancel()
			}
		}}
		client := &fakeClient{steps: []step{
			assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
			assistantStep("must not run", nil),
		}}
		ran := false
		events := &recorder{}
		s := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: "test-session",
		}, newFakeTurnOwner())

		err := s.Send(ctx, "go", events, nil, echoTool("echo", false, &ran))
		if !errors.Is(err, context.Canceled) || !ran {
			t.Fatalf("Send error = %v, tool ran = %v", err, ran)
		}
		if len(client.reqs) != 1 {
			t.Fatalf("provider requests = %d after outcome cancellation", len(client.reqs))
		}
		for _, event := range events.events {
			if strings.HasPrefix(event, "result:") {
				t.Fatalf("ToolResult emitted after outcome cancellation: %v", events.events)
			}
		}
	})

	t.Run("ToolResult callback", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client := &fakeClient{steps: []step{
			assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
			assistantStep("must not run", nil),
		}}
		events := &recorder{onToolResult: cancel}
		s := newTestSession(client, "test-model")

		err := s.Send(ctx, "go", events, nil, echoTool("echo", false, nil))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Send error = %v, want context.Canceled", err)
		}
		if len(client.reqs) != 1 {
			t.Fatalf("provider requests = %d after ToolResult cancellation", len(client.reqs))
		}
	})
}

func TestSendStopsWhenToolOutcomeAppendFails(t *testing.T) {
	history := &fakeHistory{appendErrAt: 4, appendErr: errors.New("disk full")}
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	err := s.Send(context.Background(), "go", &recorder{}, nil, echoTool("echo", false, &ran))
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Send error = %v, want tool outcome append failure", err)
	}
	if !ran {
		t.Fatal("tool did not run before its outcome append failed")
	}
	if len(c.reqs) != 1 {
		t.Fatalf("provider received %d requests, want only the pre-tool request", len(c.reqs))
	}
}

func TestSendPersistsApprovalBeforeGatedToolExecution(t *testing.T) {
	history := &fakeHistory{}
	approvalWasDurable := false
	tool := echoTool("dangerous", true, nil)
	tool.Execute = func(_ context.Context, args string) (string, error) {
		if len(history.events) != 4 {
			return "ran", nil
		}
		intentEvent := history.events[2]
		approvalEvent := history.events[3]
		if approvalEvent.Type != memory.EventApproval ||
			approvalEvent.ParentID != intentEvent.ID ||
			approvalEvent.ExecutionID != intentEvent.ExecutionID {
			return "ran", nil
		}
		var payload memory.ApprovalPayload
		if err := json.Unmarshal(approvalEvent.Payload, &payload); err == nil &&
			payload.Decision == memory.ApprovalApproved {
			approvalWasDurable = true
		}
		return "ran", nil
	}
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "dangerous", `{}`)),
		assistantStep("done", nil),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())
	approve := func(_ context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		return tools.Approved
	}

	if err := s.Send(context.Background(), "go", &recorder{}, approve, tool); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !approvalWasDurable {
		t.Fatal("gated tool executed before approval was durable")
	}
}

func TestSendStopsWhenApprovalAppendFails(t *testing.T) {
	history := &fakeHistory{appendErrAt: 4, appendErr: errors.New("disk full")}
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "dangerous", `{}`)),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())
	approve := func(_ context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		return tools.Approved
	}

	err := s.Send(
		context.Background(), "go", &recorder{}, approve,
		echoTool("dangerous", true, &ran),
	)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Send error = %v, want approval append failure", err)
	}
	if ran {
		t.Fatal("gated tool ran after approval append failure")
	}
}

func TestSendRecordsDeclinedApprovalAsCancellation(t *testing.T) {
	history := &fakeHistory{}
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "dangerous", `{}`)),
		assistantStep("understood", nil),
	}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())
	deny := func(_ context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		return tools.Declined
	}

	if err := s.Send(
		context.Background(), "go", &recorder{}, deny,
		echoTool("dangerous", true, &ran),
	); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ran {
		t.Fatal("declined tool executed")
	}
	if len(history.events) != 6 {
		t.Fatalf("durable events = %+v", history.events)
	}
	approvalEvent := history.events[3]
	outcomeEvent := history.events[4]
	if approvalEvent.Type != memory.EventApproval ||
		outcomeEvent.Type != memory.EventToolCancelled ||
		outcomeEvent.ParentID != approvalEvent.ID ||
		outcomeEvent.ExecutionID != approvalEvent.ExecutionID {
		t.Errorf("approval = %+v, cancelled outcome = %+v", approvalEvent, outcomeEvent)
	}
	var payload memory.ToolResultPayload
	if err := json.Unmarshal(outcomeEvent.Payload, &payload); err != nil ||
		payload.ToolCallID != "call-1" || payload.IsError {
		t.Errorf("cancelled payload = %+v, error = %v", payload, err)
	}
}

// recorder flattens the event stream into strings so a test asserts order
// and payload with one slice comparison.
type recorder struct {
	events          []string
	onAssistantDone func()
	onToolCall      func()
	onToolResult    func()
}

func (r *recorder) Delta(text string)     { r.events = append(r.events, "delta:"+text) }
func (r *recorder) Reasoning(text string) { r.events = append(r.events, "reasoning:"+text) }
func (r *recorder) ReasoningDone()        { r.events = append(r.events, "reasoningdone") }
func (r *recorder) AssistantDone(content string) {
	r.events = append(r.events, "done:"+content)
	if r.onAssistantDone != nil {
		r.onAssistantDone()
	}
}
func (r *recorder) ToolCall(id, name, args string) {
	r.events = append(r.events, fmt.Sprintf("call:%s:%s:%s", id, name, args))
	if r.onToolCall != nil {
		r.onToolCall()
	}
}
func (r *recorder) ToolResult(id, content string, isErr bool) {
	r.events = append(r.events, fmt.Sprintf("result:%s:%v:%s", id, isErr, content))
	if r.onToolResult != nil {
		r.onToolResult()
	}
}
func (r *recorder) ResponseDiscarded(reason DiscardReason, message string) {
	r.events = append(r.events, fmt.Sprintf("discarded:%s:%s", reason, message))
}

func assistantStep(content string, deltas []string, calls ...openrouter.ToolCall) step {
	return step{
		deltas: deltas,
		res: openrouter.ChatResponse{Choices: []openrouter.Choice{{
			Message: openrouter.Message{Role: "assistant", Content: content, ToolCalls: calls},
		}}},
	}
}

func assistantUsageStep(content string, usage *openrouter.TokenUsage, calls ...openrouter.ToolCall) step {
	step := assistantStep(content, nil, calls...)
	step.res.Usage = usage
	return step
}

func testProviderUsage(input, output, total int64) *openrouter.TokenUsage {
	return &openrouter.TokenUsage{
		InputTokens:  &input,
		OutputTokens: &output,
		TotalTokens:  &total,
	}
}

// reasoningStep scripts an assistant message that thinks first: reasoning
// fragments stream before the content deltas, as providers actually send.
func reasoningStep(content string, reasoning, deltas []string, calls ...openrouter.ToolCall) step {
	s := assistantStep(content, deltas, calls...)
	s.reasoning = reasoning
	return s
}

func toolCall(id, name, args string) openrouter.ToolCall {
	return openrouter.ToolCall{ID: id, Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: args}}
}

// echoTool is the per-turn extra a frontend would pass: hermetic, no side
// effects, optionally gated.
func echoTool(name string, gated bool, ran *bool) tools.Tool {
	return tools.Tool{
		Schema: openrouter.Tool{
			Type:     "function",
			Function: openrouter.Function{Name: name, Parameters: openrouter.Parameter{Type: "object"}},
		},
		Execute: func(_ context.Context, args string) (string, error) {
			if ran != nil {
				*ran = true
			}
			return "echo:" + args, nil
		},
		NeedsApproval: gated,
	}
}

func TestPlainAnswer(t *testing.T) {
	c := &fakeClient{steps: []step{assistantStep("Hello David", []string{"Hello ", "David"})}}
	s := newTestSession(c, "test-model")
	rec := &recorder{}

	if err := s.Send(context.Background(), "hi", rec, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := []string{"delta:Hello ", "delta:David", "done:Hello David"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}

	if roles := rolesOf(c.reqs[0].Messages); roles != "system,user" {
		t.Fatalf("request messages = %s", roles)
	}
	if history := s.history.(*fakeHistory); len(history.events) != 2 ||
		history.events[1].Type != memory.EventAssistantMessage {
		t.Fatalf("durable events = %+v", history.events)
	}
	if len(c.reqs) != 1 || c.reqs[0].Model != "test-model" {
		t.Fatalf("request model = %q", c.reqs[0].Model)
	}
	if c.reqs[0].MaxTokens != 16384 {
		t.Fatalf("request max_tokens = %d, want 16384", c.reqs[0].MaxTokens)
	}
	if len(c.reqs[0].Tools) != len(tools.Schemas()) {
		t.Fatalf("request advertised %d tools, want base registry %d", len(c.reqs[0].Tools), len(tools.Schemas()))
	}
}

func TestSendPersistsSeparateUsageForEveryProviderIteration(t *testing.T) {
	history := &fakeHistory{}
	client := &fakeClient{steps: []step{
		assistantUsageStep("", testProviderUsage(10, 2, 12), toolCall("call-1", "echo", `{}`)),
		assistantUsageStep("done", testProviderUsage(20, 3, 23)),
	}}
	session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	if err := session.Send(context.Background(), "go", &recorder{}, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatal(err)
	}

	var usages []*memory.TokenUsage
	for _, event := range history.events {
		if event.Type != memory.EventAssistantMessage {
			continue
		}
		var payload memory.AssistantMessagePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		usages = append(usages, payload.Usage)
	}
	if len(usages) != 2 || usages[0] == nil || usages[1] == nil ||
		*usages[0].InputTokens != 10 || *usages[0].OutputTokens != 2 || *usages[0].TotalTokens != 12 ||
		*usages[1].InputTokens != 20 || *usages[1].OutputTokens != 3 || *usages[1].TotalTokens != 23 {
		t.Fatalf("durable per-iteration usage=%+v", usages)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("provider requests=%d, want 2", len(client.reqs))
	}
	for i, request := range client.reqs {
		if request.MaxTokens != 16384 {
			t.Fatalf("provider request %d max_tokens=%d, want 16384", i, request.MaxTokens)
		}
	}
	secondRequest, err := json.Marshal(client.reqs[1])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(secondRequest), `"usage"`) || strings.Contains(string(secondRequest), `"input_tokens"`) {
		t.Fatalf("durable usage leaked into provider history request: %s", secondRequest)
	}
}

func TestAssistantEventInputMapsAndNormalizesUsage(t *testing.T) {
	inputCount, outputCount, totalCount := int64(1), int64(2), int64(3)
	reasoningCount, cachedCount, cacheWriteCount := int64(4), int64(5), int64(6)
	complete, err := assistantEventInput(openrouter.Message{
		Role:             "assistant",
		Content:          "ok",
		Reasoning:        "transient reasoning",
		ReasoningDetails: json.RawMessage(`[{"type":"reasoning.text","text":"transient reasoning"}]`),
	}, &openrouter.TokenUsage{
		InputTokens:           &inputCount,
		OutputTokens:          &outputCount,
		TotalTokens:           &totalCount,
		ReasoningOutputTokens: &reasoningCount,
		CachedInputTokens:     &cachedCount,
		CacheWriteInputTokens: &cacheWriteCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantComplete = `{"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3,"reasoning_output_tokens":4,"cached_input_tokens":5,"cache_write_input_tokens":6}}`
	if string(complete.Payload) != wantComplete {
		t.Fatalf("complete durable usage payload=%s, want %s", complete.Payload, wantComplete)
	}
	if strings.Contains(string(complete.Payload), "transient reasoning") ||
		strings.Contains(string(complete.Payload), `"reasoning_details"`) {
		t.Fatalf("transient reasoning crossed durable assistant boundary: %s", complete.Payload)
	}

	negative, zero := int64(-1), int64(0)
	input, err := assistantEventInput(openrouter.Message{Role: "assistant", Content: "ok"}, &openrouter.TokenUsage{
		InputTokens:  &negative,
		OutputTokens: &zero,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(input.Payload) != `{"usage":{"output_tokens":0}}` {
		t.Fatalf("normalized adversarial usage payload=%s", input.Payload)
	}

	empty, err := assistantEventInput(openrouter.Message{Role: "assistant", Content: "ok"}, &openrouter.TokenUsage{})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty.Payload) != `{}` {
		t.Fatalf("empty usage payload=%s, want {}", empty.Payload)
	}
}

func TestAssistantUsageSharesAssistantCommitOrRollbackFate(t *testing.T) {
	t.Run("commit wins cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		history := &fakeHistory{afterAppend: func(input memory.EventInput) {
			if input.Type == memory.EventAssistantMessage {
				cancel()
			}
		}}
		client := &fakeClient{steps: []step{
			assistantUsageStep("accepted", testProviderUsage(4, 5, 9)),
		}}
		session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: "test-session",
		}, newFakeTurnOwner())
		if err := session.Send(ctx, "go", &recorder{}, nil); err != nil {
			t.Fatalf("Send error=%v, want committed success", err)
		}
		if len(history.events) != 2 || history.events[1].Type != memory.EventAssistantMessage {
			t.Fatalf("events=%+v", history.events)
		}
		var payload memory.AssistantMessagePayload
		if err := json.Unmarshal(history.events[1].Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Usage == nil || *payload.Usage.TotalTokens != 9 {
			t.Fatalf("committed usage=%+v", payload.Usage)
		}
	})

	t.Run("assistant append rollback", func(t *testing.T) {
		history := &fakeHistory{appendErrAt: 2, appendErr: errors.New("rolled back")}
		client := &fakeClient{steps: []step{
			assistantUsageStep("not accepted", testProviderUsage(4, 5, 9)),
		}}
		session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: "test-session",
		}, newFakeTurnOwner())
		if err := session.Send(context.Background(), "go", &recorder{}, nil); err == nil {
			t.Fatal("Send succeeded after assistant append rollback")
		}
		if len(history.events) != 1 || history.events[0].Type != memory.EventUserMessage {
			t.Fatalf("rolled-back assistant or usage persisted: %+v", history.events)
		}
	})
}

func TestToolRoundTrip(t *testing.T) {
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("c1", "echo", `{"x":1}`)),
		assistantStep("all done", []string{"all done"}),
	}}
	s := newTestSession(c, "test-model")
	rec := &recorder{}

	if err := s.Send(context.Background(), "go", rec, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := []string{
		"done:", // tool-only assistant message still fires AssistantDone
		`call:c1:echo:{"x":1}`,
		`result:c1:false:echo:{"x":1}`,
		"delta:all done",
		"done:all done",
	}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}

	if roles := rolesOf(c.reqs[1].Messages); roles != "system,user,assistant,tool" {
		t.Fatalf("second request messages = %s", roles)
	}
	// Both requests must advertise the extra alongside the base registry.
	for i, req := range c.reqs {
		if len(req.Tools) != len(tools.Schemas())+1 {
			t.Fatalf("request %d advertised %d tools, want %d", i, len(req.Tools), len(tools.Schemas())+1)
		}
	}
}

func TestToolResultAdmissionCapsDurableAndVisibleContent(t *testing.T) {
	upstream := strings.Repeat("🙂", toolResultAdmissionBytes/4+100)
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("c1", "large", `{}`)),
		assistantStep("done", nil),
	}}
	history := &fakeHistory{}
	session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	recorder := &recorder{}
	large := tools.Tool{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "large", Parameters: openrouter.Parameter{Type: "object"},
		}},
		Execute: func(context.Context, string) (string, error) { return upstream, nil },
	}

	if err := session.Send(context.Background(), "go", recorder, nil, large); err != nil {
		t.Fatal(err)
	}
	var durable string
	for _, event := range history.events {
		if event.Type == memory.EventToolSucceeded {
			durable = event.Content
		}
	}
	if len(durable) != toolResultAdmissionBytes || !utf8.ValidString(durable) {
		t.Fatalf("durable result bytes=%d valid_utf8=%v", len(durable), utf8.ValidString(durable))
	}
	visible := "result:c1:false:" + durable
	if !containsString(recorder.events, visible) {
		t.Fatalf("visible result differs from durable result")
	}
	if got := client.reqs[1].Messages[len(client.reqs[1].Messages)-1].Content; got != durable {
		t.Fatalf("provider result differs from durable result")
	}
}

func TestSessionSendProjectsOldToolResultWithoutChangingDurableHistory(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 154097, 154097, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := []memory.Event{{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run"}}
	sequence := int64(2)
	for group := 1; group <= 5; group++ {
		assistantID := memory.EventID(fmt.Sprintf("assistant-%d", group))
		callID := fmt.Sprintf("call-%d", group)
		resultID := memory.EventID(fmt.Sprintf("result-%d", group))
		events = append(events,
			memory.Event{ID: assistantID, Sequence: sequence, ParentID: "root", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
				Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{{ID: callID, Name: "large", Arguments: `{}`}}})},
			memory.Event{ID: resultID, Sequence: sequence + 1, ParentID: assistantID, Type: memory.EventToolSucceeded, Role: memory.RoleTool,
				Content: strings.Repeat(string(rune('a'+group-1)), 20*1024), Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: callID})},
		)
		sequence += 2
	}
	history := &fakeHistory{events: events}
	client := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := New(client, profile, history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	session.reasoning = nil

	if err := session.Send(context.Background(), "continue", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if history.events[2].Content != strings.Repeat("a", 20*1024) {
		t.Fatal("canonical durable result was changed by request projection")
	}
	var projected, retained string
	for _, message := range client.reqs[0].Messages {
		switch message.ToolCallID {
		case "call-1":
			projected = message.Content
		case "call-5":
			retained = message.Content
		}
	}
	digest := sha256.Sum256([]byte(history.events[2].Content))
	exactProjection := fmt.Sprintf(
		"[older tool result projected: event_id=result-1 original_bytes=20480 sha256=%x]\n<head>\n%s\n<tail>\n%s",
		digest,
		strings.Repeat("a", 512),
		strings.Repeat("a", 512),
	)
	if projected != exactProjection {
		t.Fatalf("provider projection=%q", projected)
	}
	if retained != strings.Repeat("e", 20*1024) {
		t.Fatalf("newest retained result bytes=%d", len(retained))
	}
	var snapshot memory.ContextSnapshotPayload
	for _, event := range history.allEvents() {
		if event.Type == memory.EventContextSnapshot {
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
		}
	}
	secondDigest := sha256.Sum256([]byte(history.events[4].Content))
	if len(snapshot.Placeholders) != 2 || snapshot.Placeholders[0].EventID != "result-1" ||
		snapshot.Placeholders[0].SHA256 != fmt.Sprintf("%x", digest) ||
		snapshot.Placeholders[1].EventID != "result-2" || snapshot.Placeholders[1].SHA256 != fmt.Sprintf("%x", secondDigest) {
		t.Fatalf("snapshot placeholders=%+v", snapshot.Placeholders)
	}
	if snapshot.SerializedBytes > percentageFloor(snapshot.UsableInputBytes, 60) {
		t.Fatalf("snapshot bytes=%d target=%d", snapshot.SerializedBytes, percentageFloor(snapshot.UsableInputBytes, 60))
	}
	wantMessages := []openrouter.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: "run"},
	}
	for group := 1; group <= 5; group++ {
		callID := fmt.Sprintf("call-%d", group)
		wantMessages = append(wantMessages,
			openrouter.Message{Role: "assistant", ToolCalls: []openrouter.ToolCall{{
				ID: callID, Type: "function", Function: openrouter.FunctionCall{Name: "large", Arguments: `{}`},
			}}},
			openrouter.Message{Role: "tool", ToolCallID: callID, Content: strings.Repeat(string(rune('a'+group-1)), 20*1024)},
		)
	}
	wantMessages[3].Content = exactProjection
	wantMessages[5].Content = fmt.Sprintf(
		"[older tool result projected: event_id=result-2 original_bytes=20480 sha256=%x]\n<head>\n%s\n<tail>\n%s",
		secondDigest,
		strings.Repeat("b", 512),
		strings.Repeat("b", 512),
	)
	wantRequest := openrouter.ChatRequest{
		Model: "test/model", Messages: wantMessages, Tools: tools.Schemas(), Stream: true, MaxTokens: 1,
	}
	wantRequest.Messages = append(wantRequest.Messages, openrouter.Message{Role: "user", Content: "continue"})
	if !reflect.DeepEqual(client.reqs[0], wantRequest) {
		got := client.reqs[0]
		if got.Model != wantRequest.Model || got.Stream != wantRequest.Stream || got.MaxTokens != wantRequest.MaxTokens ||
			!reflect.DeepEqual(got.Reasoning, wantRequest.Reasoning) || !reflect.DeepEqual(got.Tools, wantRequest.Tools) ||
			len(got.Messages) != len(wantRequest.Messages) {
			t.Fatalf("provider request envelope mismatch: got messages=%d tools=%d reasoning=%+v; want messages=%d tools=%d reasoning=%+v",
				len(got.Messages), len(got.Tools), got.Reasoning, len(wantRequest.Messages), len(wantRequest.Tools), wantRequest.Reasoning)
		}
		for i := range got.Messages {
			if !reflect.DeepEqual(got.Messages[i], wantRequest.Messages[i]) {
				t.Fatalf("provider message %d mismatch: got role=%s call=%s content_bytes=%d calls=%+v; want role=%s call=%s content_bytes=%d calls=%+v",
					i, got.Messages[i].Role, got.Messages[i].ToolCallID, len(got.Messages[i].Content), got.Messages[i].ToolCalls,
					wantRequest.Messages[i].Role, wantRequest.Messages[i].ToolCallID, len(wantRequest.Messages[i].Content), wantRequest.Messages[i].ToolCalls)
			}
		}
		t.Fatal("provider request mismatch without a differing field")
	}
}

func TestReasoningStreamsThenContent(t *testing.T) {
	c := &fakeClient{steps: []step{
		reasoningStep("391", []string{"Compute ", "17*23"}, []string{"391"}),
	}}
	s := newTestSession(c, "test-model")
	rec := &recorder{}

	if err := s.Send(context.Background(), "what is 17*23", rec, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Every reasoning fragment, in order, then exactly one ReasoningDone
	// at the moment the first content delta arrives, then the content.
	want := []string{
		"reasoning:Compute ",
		"reasoning:17*23",
		"reasoningdone",
		"delta:391",
		"done:391",
	}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
}

type contentFirstReasoningClient struct{}

func (contentFirstReasoningClient) ChatStream(
	_ context.Context,
	_ openrouter.ChatRequest,
	h openrouter.StreamHandlers,
) (openrouter.ChatResponse, error) {
	h.OnContent("answer")
	h.OnReasoning("late reasoning")
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{
		Role: "assistant", Content: "answer", Reasoning: "late reasoning",
	}}}}, nil
}

func TestReasoningAfterContentIsAssembledButNotRendered(t *testing.T) {
	rec := &recorder{}
	if err := newTestSession(contentFirstReasoningClient{}, "test-model").Send(
		context.Background(), "go", rec, nil,
	); err != nil {
		t.Fatal(err)
	}
	want := []string{"delta:answer", "done:answer"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events=%v, want monotonic content presentation %v", rec.events, want)
	}
}

func TestReasoningDoneOncePerMessageAcrossToolRoundTrip(t *testing.T) {
	c := &fakeClient{steps: []step{
		// Tool-only message: no content delta ever ends the thinking.
		reasoningStep("", []string{"need a tool"}, nil, toolCall("c1", "echo", "{}")),
		reasoningStep("done", []string{"now answer"}, []string{"done"}),
	}}
	s := newTestSession(c, "test-model")
	rec := &recorder{}

	if err := s.Send(context.Background(), "go", rec, nil, echoTool("echo", false, nil)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := []string{
		"reasoning:need a tool",
		"reasoningdone", // fired after assembly, before AssistantDone
		"done:",
		`call:c1:echo:{}`,
		`result:c1:false:echo:{}`,
		"reasoning:now answer",
		"reasoningdone", // fired by the first content delta
		"delta:done",
		"done:done",
	}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
}

func TestNoReasoningMeansNoReasoningEvents(t *testing.T) {
	c := &fakeClient{steps: []step{
		assistantStep("plain", []string{"plain"}),
	}}
	s := newTestSession(c, "test-model")
	rec := &recorder{}

	if err := s.Send(context.Background(), "hi", rec, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := []string{"delta:plain", "done:plain"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}
}

func TestGatedExtraApprovedAndDeclined(t *testing.T) {
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("c1", "danger", "{}")),
		assistantStep("ok", nil),
	}}
	s := newTestSession(c, "test-model")
	approve := func(_ context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		return tools.Approved
	}

	if err := s.Send(context.Background(), "go", &recorder{}, approve, echoTool("danger", true, &ran)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !ran {
		t.Fatal("approved gated tool never ran")
	}

	// Fresh session, declining approver.
	ran = false
	c = &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("c1", "danger", "{}")),
		assistantStep("ok", nil),
	}}
	s = newTestSession(c, "test-model")
	rec := &recorder{}
	deny := func(_ context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		return tools.Declined
	}

	if err := s.Send(context.Background(), "go", rec, deny, echoTool("danger", true, &ran)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ran {
		t.Fatal("declined gated tool ran anyway")
	}
	if !strings.Contains(fmt.Sprint(rec.events), "David declined") {
		t.Fatalf("decline not surfaced in events: %v", rec.events)
	}
	last := c.reqs[1].Messages[len(c.reqs[1].Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "David declined") {
		t.Fatalf("decline not recorded in transcript: %+v", last)
	}
}

func TestProviderErrorAbortsTurn(t *testing.T) {
	c := &fakeClient{steps: []step{{err: errors.New("boom")}}}
	s := newTestSession(c, "test-model")

	err := s.Send(context.Background(), "hi", &recorder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want wrapped boom", err)
	}
	// The user event stays durable even though the provider failed.
	history := s.history.(*fakeHistory)
	if len(history.events) != 2 || history.events[0].Type != memory.EventUserMessage ||
		history.events[0].Content != "hi" {
		t.Fatalf("durable events = %+v", history.events)
	}
	if history.events[1].Type != memory.EventTurnFailed || history.events[1].ParentID != history.events[0].ID {
		t.Fatalf("terminal event = %+v", history.events[1])
	}
}

func TestSendBuildsProviderRequestFromDurableHistory(t *testing.T) {
	history := &fakeHistory{events: []memory.Event{
		{ID: "event-1", SessionID: "test-session", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "earlier question"},
		{ID: "event-2", SessionID: "test-session", Sequence: 2, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "earlier answer", Payload: json.RawMessage(`{}`)},
	}}
	c := &fakeClient{steps: []step{assistantStep("new answer", nil)}}
	s := New(c, testContextProfile("test-model"), history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}, newFakeTurnOwner())

	if err := s.Send(context.Background(), "new question", &recorder{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(c.reqs) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(c.reqs))
	}
	got := c.reqs[0].Messages
	if roles := rolesOf(got); roles != "system,user,assistant,user" {
		t.Fatalf("request messages = %s", roles)
	}
	if got[1].Content != "earlier question" || got[2].Content != "earlier answer" ||
		got[3].Content != "new question" {
		t.Fatalf("request messages = %+v", got)
	}
}

func TestNoChoicesIsAnError(t *testing.T) {
	c := &fakeClient{steps: []step{{res: openrouter.ChatResponse{}}}}
	s := newTestSession(c, "test-model")

	if err := s.Send(context.Background(), "hi", &recorder{}, nil); err == nil {
		t.Fatal("empty choices did not error")
	}
}

func TestSecondSendGetsErrBusy(t *testing.T) {
	c := &fakeClient{
		steps:   []step{assistantStep("done", nil)},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	s := newTestSession(c, "test-model")
	firstDone := make(chan error, 1)

	go func() { firstDone <- s.Send(context.Background(), "hi", &recorder{}, nil) }()
	<-c.entered // first turn is now inside ChatStream, holding the lock

	if err := s.Send(context.Background(), "again", &recorder{}, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Send = %v, want ErrBusy", err)
	}

	close(c.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Send: %v", err)
	}
}

func TestSessionExposesImmutableContextDiagnostics(t *testing.T) {
	session := newTestSession(nil, "vendor/model")
	diagnostics := session.ContextProfile()
	diagnostics.HardWindowTokens = 1
	if got := session.ContextProfile().HardWindowTokens; got != 300000 {
		t.Fatalf("session hard window = %d, want immutable 300000", got)
	}
}

func rolesOf(msgs []openrouter.Message) string {
	roles := make([]string, len(msgs))
	for i, m := range msgs {
		roles[i] = m.Role
	}
	return strings.Join(roles, ",")
}
