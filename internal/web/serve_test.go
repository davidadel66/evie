package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// fakeClient scripts the provider, same shape as the agent package's
// test fake: each ChatStream call consumes one step. The channels let
// the busy test hold a turn open mid-request.
type fakeClient struct {
	steps   []fakeStep
	entered chan struct{}
	release chan struct{}
}

type fakeStep struct {
	deltas    []string
	reasoning []string // streamed before deltas, as real providers send it
	content   string
	toolCalls []openrouter.ToolCall
	err       error
}

type fakeHistory struct {
	events []memory.Event
}

func (f *fakeHistory) Append(_ context.Context, input memory.EventInput) (memory.Event, error) {
	event := memory.Event{
		ID:            memory.EventID(fmt.Sprintf("event-%d", len(f.events)+1)),
		SessionID:     "test-session",
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

func (f *fakeClient) ChatStream(_ context.Context, req openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.release != nil {
		<-f.release
	}
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
	if s.err != nil {
		return openrouter.ChatResponse{}, s.err
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{
		Message: openrouter.Message{Role: "assistant", Content: s.content, ToolCalls: s.toolCalls},
	}}}, nil
}

func newTestServer(c *fakeClient) http.Handler {
	_, h := newTestServerFull(c)
	return h
}

func newTestServerFull(c *fakeClient) (*Server, http.Handler) {
	srv := NewServer(agent.New(c, "test-model", &fakeHistory{}, memory.ScopeContext{
		OwnerID:   memory.LocalOwnerID,
		SessionID: "test-session",
	}))
	return srv, srv.Handler()
}

// chatRequest builds a well-formed same-origin chat POST; individual
// tests then break one property at a time.
func chatRequest(body string) *http.Request {
	req := httptest.NewRequest("POST", "http://127.0.0.1:6687/api/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func TestChatStreamsATurn(t *testing.T) {
	h := newTestServer(&fakeClient{steps: []fakeStep{{deltas: []string{"Hel", "lo"}, content: "Hello"}}})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, chatRequest(`{"message":"hi"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: delta\ndata: {\"text\":\"Hel\"}",
		"event: delta\ndata: {\"text\":\"lo\"}",
		"event: assistant_done\ndata: {\"content\":\"Hello\"}",
		"event: turn_done\ndata: {}",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q:\n%s", want, body)
		}
	}
	if !strings.HasSuffix(body, "event: turn_done\ndata: {}\n\n") {
		t.Fatalf("stream does not end with turn_done:\n%s", body)
	}
}

func TestChatStreamsReasoningBeforeContent(t *testing.T) {
	h := newTestServer(&fakeClient{steps: []fakeStep{{
		reasoning: []string{"Compute ", "17*23"},
		deltas:    []string{"391"},
		content:   "391",
	}}})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, chatRequest(`{"message":"what is 17*23"}`))

	body := rec.Body.String()
	for _, want := range []string{
		"event: reasoning\ndata: {\"text\":\"Compute \"}",
		"event: reasoning\ndata: {\"text\":\"17*23\"}",
		"event: reasoning_done\ndata: {}",
		"event: delta\ndata: {\"text\":\"391\"}",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q:\n%s", want, body)
		}
	}

	// Wire order: reasoning precedes content; reasoning_done fires
	// exactly once, at the boundary between them.
	if strings.Count(body, "event: reasoning_done") != 1 {
		t.Fatalf("reasoning_done fired %d times, want 1:\n%s", strings.Count(body, "event: reasoning_done"), body)
	}
	firstReasoning := strings.Index(body, "event: reasoning")
	done := strings.Index(body, "event: reasoning_done")
	firstDelta := strings.Index(body, "event: delta")
	if !(firstReasoning < done && done < firstDelta) {
		t.Fatalf("order = reasoning@%d done@%d delta@%d, want reasoning < done < delta:\n%s",
			firstReasoning, done, firstDelta, body)
	}
}

func TestBusySecondChatGets409(t *testing.T) {
	c := &fakeClient{
		steps:   []fakeStep{{content: "done"}},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := newTestServer(c)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		h.ServeHTTP(httptest.NewRecorder(), chatRequest(`{"message":"first"}`))
	}()
	<-c.entered // first turn is inside ChatStream, holding the session lock

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, chatRequest(`{"message":"second"}`))

	if rec.Code != http.StatusConflict {
		t.Fatalf("second chat status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "error") {
		t.Fatalf("409 body = %q, want JSON error", rec.Body.String())
	}

	close(c.release)
	<-firstDone
}

func TestGuardRejects(t *testing.T) {
	h := newTestServer(&fakeClient{})

	// Wrong content type — what an HTML form would send.
	req := chatRequest(`{"message":"hi"}`)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("form content type: status = %d, want 403", rec.Code)
	}

	// Foreign origin — a malicious page's fetch.
	req = chatRequest(`{"message":"hi"}`)
	req.Header.Set("Origin", "http://evil.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: status = %d, want 403", rec.Code)
	}

	// Foreign host — DNS rebinding.
	req = chatRequest(`{"message":"hi"}`)
	req.Host = "rebound.example:6687"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign host: status = %d, want 403", rec.Code)
	}
}

func TestGuardAllowsLoopbackOrigins(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:6687", "http://localhost:5173"} {
		h := newTestServer(&fakeClient{steps: []fakeStep{{content: "ok"}}})
		req := chatRequest(`{"message":"hi"}`)
		req.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("origin %s: status = %d, want 200", origin, rec.Code)
		}
	}
}

func TestBadJSONIs400(t *testing.T) {
	h := newTestServer(&fakeClient{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, chatRequest(`not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSendErrorStreamsErrorThenTurnDone(t *testing.T) {
	h := newTestServer(&fakeClient{steps: []fakeStep{{err: errors.New("provider down")}}})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, chatRequest(`{"message":"hi"}`))

	body := rec.Body.String()
	if !strings.Contains(body, "event: error\n") || !strings.Contains(body, "provider down") {
		t.Fatalf("missing error event:\n%s", body)
	}
	if !strings.HasSuffix(body, "event: turn_done\ndata: {}\n\n") {
		t.Fatalf("error stream must still end with turn_done:\n%s", body)
	}
}

func TestListenAddr(t *testing.T) {
	t.Setenv("EVIE_ADDR", "")
	if addr, err := listenAddr(); err != nil || addr != "127.0.0.1:6687" {
		t.Fatalf("default = %q, %v", addr, err)
	}

	t.Setenv("EVIE_ADDR", "localhost:9999")
	if _, err := listenAddr(); err != nil {
		t.Fatalf("loopback override rejected: %v", err)
	}

	t.Setenv("EVIE_ADDR", "0.0.0.0:6687")
	if _, err := listenAddr(); err == nil {
		t.Fatal("non-loopback bind must be refused")
	}
}
