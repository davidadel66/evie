package agent

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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
}

type step struct {
	reasoning []string // streamed before deltas, as real providers send it
	deltas    []string
	res       openrouter.ChatResponse
	err       error
}

func (f *fakeClient) ChatStream(req openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
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

// recorder flattens the event stream into strings so a test asserts order
// and payload with one slice comparison.
type recorder struct {
	events []string
}

func (r *recorder) Delta(text string)          { r.events = append(r.events, "delta:"+text) }
func (r *recorder) Reasoning(text string)      { r.events = append(r.events, "reasoning:"+text) }
func (r *recorder) ReasoningDone()             { r.events = append(r.events, "reasoningdone") }
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
	s := New(c, "test-model")
	rec := &recorder{}

	if err := s.Send("hi", rec, nil); err != nil {
		t.Fatalf("Send: %v", err)
	}

	want := []string{"delta:Hello ", "delta:David", "done:Hello David"}
	if fmt.Sprint(rec.events) != fmt.Sprint(want) {
		t.Fatalf("events = %v, want %v", rec.events, want)
	}

	roles := rolesOf(s.messages)
	if roles != "system,user,assistant" {
		t.Fatalf("messages = %s", roles)
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
	s := New(c, "test-model")
	rec := &recorder{}

	if err := s.Send("go", rec, nil, echoTool("echo", false, nil)); err != nil {
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

	if roles := rolesOf(s.messages); roles != "system,user,assistant,tool,assistant" {
		t.Fatalf("messages = %s", roles)
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
	s := New(c, "test-model")
	rec := &recorder{}

	if err := s.Send("what is 17*23", rec, nil); err != nil {
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
	s := New(c, "test-model")
	rec := &recorder{}

	if err := s.Send("go", rec, nil, echoTool("echo", false, nil)); err != nil {
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
	s := New(c, "test-model")
	rec := &recorder{}

	if err := s.Send("hi", rec, nil); err != nil {
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
	s := New(c, "test-model")
	approve := func(name, args string) tools.Decision { return tools.Approved }

	if err := s.Send("go", &recorder{}, approve, echoTool("danger", true, &ran)); err != nil {
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
	s = New(c, "test-model")
	rec := &recorder{}
	deny := func(name, args string) tools.Decision { return tools.Declined }

	if err := s.Send("go", rec, deny, echoTool("danger", true, &ran)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if ran {
		t.Fatal("declined gated tool ran anyway")
	}
	if !strings.Contains(fmt.Sprint(rec.events), "David declined") {
		t.Fatalf("decline not surfaced in events: %v", rec.events)
	}
	last := s.messages[len(s.messages)-2] // tool message sits before the final assistant
	if last.Role != "tool" || !strings.Contains(last.Content, "David declined") {
		t.Fatalf("decline not recorded in transcript: %+v", last)
	}
}

func TestProviderErrorAbortsTurn(t *testing.T) {
	c := &fakeClient{steps: []step{{err: errors.New("boom")}}}
	s := New(c, "test-model")

	err := s.Send("hi", &recorder{}, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want wrapped boom", err)
	}
	// The user message stays: the turn failed, the input wasn't lost.
	if roles := rolesOf(s.messages); roles != "system,user" {
		t.Fatalf("messages = %s", roles)
	}
}

func TestNoChoicesIsAnError(t *testing.T) {
	c := &fakeClient{steps: []step{{res: openrouter.ChatResponse{}}}}
	s := New(c, "test-model")

	if err := s.Send("hi", &recorder{}, nil); err == nil {
		t.Fatal("empty choices did not error")
	}
}

func TestSecondSendGetsErrBusy(t *testing.T) {
	c := &fakeClient{
		steps:   []step{assistantStep("done", nil)},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	s := New(c, "test-model")
	firstDone := make(chan error, 1)

	go func() { firstDone <- s.Send("hi", &recorder{}, nil) }()
	<-c.entered // first turn is now inside ChatStream, holding the lock

	if err := s.Send("again", &recorder{}, nil); !errors.Is(err, ErrBusy) {
		t.Fatalf("second Send = %v, want ErrBusy", err)
	}

	close(c.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Send: %v", err)
	}
}

func TestModelResolution(t *testing.T) {
	t.Setenv("EVIE_MODEL", "env-model")
	if s := New(nil, ""); s.model != "env-model" {
		t.Fatalf("model = %q, want env override", s.model)
	}
	if s := New(nil, "explicit"); s.model != "explicit" {
		t.Fatalf("model = %q, explicit arg must win", s.model)
	}
	t.Setenv("EVIE_MODEL", "")
	if s := New(nil, ""); s.model != DefaultModel {
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
