package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

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
	appendAttempts int
	appendErrAt    int
	appendErr      error
}

func (f *fakeHistory) Append(_ context.Context, input memory.EventInput) (memory.Event, error) {
	f.appendAttempts++
	if f.appendErr != nil && (f.appendErrAt == 0 || f.appendAttempts == f.appendErrAt) {
		return memory.Event{}, f.appendErr
	}
	event := memory.Event{
		ID:            memory.EventID(fmt.Sprintf("event-%d", len(f.events)+1)),
		SessionID:     memory.SessionID("test-session"),
		Sequence:      int64(len(f.events) + 1),
		ParentID:      input.ParentID,
		Type:          input.Type,
		Role:          input.Role,
		ExecutionID:   input.ExecutionID,
		Content:       input.Content,
		Payload:       append([]byte(nil), input.Payload...),
		FormatVersion: 1,
	}
	f.events = append(f.events, event)
	return event, nil
}

func (f *fakeHistory) Events(_ context.Context) ([]memory.Event, error) {
	return append([]memory.Event(nil), f.events...), nil
}

func newTestSession(client Client, model string) *Session {
	return New(client, model, &fakeHistory{}, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: memory.SessionID("test-session"),
	})
}

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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

	if err := s.Send(context.Background(), "hello", &recorder{}, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !providerSawUser {
		t.Fatal("provider was called before the user event was durable")
	}
}

func TestSendStopsWhenUserEventAppendFails(t *testing.T) {
	history := &fakeHistory{appendErr: errors.New("disk full")}
	c := &fakeClient{steps: []step{assistantStep("must not run", nil)}}
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

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
	tool.Execute = func(args string) (string, error) {
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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

	err := s.Send(context.Background(), "go", &recorder{}, nil, echoTool("echo", false, &ran))
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("Send error = %v, want tool intent append failure", err)
	}
	if ran {
		t.Fatal("tool executed after intent append failure")
	}
}

func TestSendStopsWhenToolOutcomeAppendFails(t *testing.T) {
	history := &fakeHistory{appendErrAt: 4, appendErr: errors.New("disk full")}
	ran := false
	c := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("call-1", "echo", `{}`)),
	}}
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

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
	tool.Execute = func(args string) (string, error) {
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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})
	approve := func(name, args string, _ *tools.FileChangePreview) tools.Decision {
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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})
	approve := func(name, args string, _ *tools.FileChangePreview) tools.Decision {
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
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})
	deny := func(name, args string, _ *tools.FileChangePreview) tools.Decision {
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
	events []string
}

func (r *recorder) Delta(text string)            { r.events = append(r.events, "delta:"+text) }
func (r *recorder) Reasoning(text string)        { r.events = append(r.events, "reasoning:"+text) }
func (r *recorder) ReasoningDone()               { r.events = append(r.events, "reasoningdone") }
func (r *recorder) AssistantDone(content string) { r.events = append(r.events, "done:"+content) }
func (r *recorder) ToolCall(id, name, args string) {
	r.events = append(r.events, fmt.Sprintf("call:%s:%s:%s", id, name, args))
}
func (r *recorder) ToolResult(id, content string, isErr bool) {
	r.events = append(r.events, fmt.Sprintf("result:%s:%v:%s", id, isErr, content))
}

func assistantStep(content string, deltas []string, calls ...openrouter.ToolCall) step {
	return step{
		deltas: deltas,
		res: openrouter.ChatResponse{Choices: []openrouter.Choice{{
			Message: openrouter.Message{Role: "assistant", Content: content, ToolCalls: calls},
		}}},
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
		Execute: func(args string) (string, error) {
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
	if len(c.reqs[0].Tools) != len(tools.Schemas()) {
		t.Fatalf("request advertised %d tools, want base registry %d", len(c.reqs[0].Tools), len(tools.Schemas()))
	}
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
	approve := func(name, args string, _ *tools.FileChangePreview) tools.Decision { return tools.Approved }

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
	deny := func(name, args string, _ *tools.FileChangePreview) tools.Decision { return tools.Declined }

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
	if len(history.events) != 1 || history.events[0].Type != memory.EventUserMessage ||
		history.events[0].Content != "hi" {
		t.Fatalf("durable events = %+v", history.events)
	}
}

func TestSendBuildsProviderRequestFromDurableHistory(t *testing.T) {
	history := &fakeHistory{events: []memory.Event{
		{ID: "event-1", SessionID: "test-session", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "earlier question"},
		{ID: "event-2", SessionID: "test-session", Sequence: 2, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "earlier answer", Payload: json.RawMessage(`{}`)},
	}}
	c := &fakeClient{steps: []step{assistantStep("new answer", nil)}}
	s := New(c, "test-model", history, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	})

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

func TestModelResolution(t *testing.T) {
	t.Setenv("EVIE_MODEL", "env-model")
	if s := newTestSession(nil, ""); s.model != "env-model" {
		t.Fatalf("model = %q, want env override", s.model)
	}
	if s := newTestSession(nil, "explicit"); s.model != "explicit" {
		t.Fatalf("model = %q, explicit arg must win", s.model)
	}
	t.Setenv("EVIE_MODEL", "")
	if s := newTestSession(nil, ""); s.model != DefaultModel {
		t.Fatalf("model = %q, want DefaultModel", s.model)
	}
}

func rolesOf(msgs []openrouter.Message) string {
	roles := make([]string, len(msgs))
	for i, m := range msgs {
		roles[i] = m.Role
	}
	return strings.Join(roles, ",")
}
