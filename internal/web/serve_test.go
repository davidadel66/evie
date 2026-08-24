package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

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
	events      []memory.Event
	appendErrAt int
	appendCalls int
	appendErr   error
	afterAppend func(memory.EventInput)
}

func (f *fakeHistory) Append(_ context.Context, _ memory.TurnLease, input memory.EventInput) (memory.Event, error) {
	f.appendCalls++
	if f.appendErrAt == f.appendCalls {
		if f.appendErr != nil {
			return memory.Event{}, f.appendErr
		}
		return memory.Event{}, errors.New("assistant persistence failed")
	}
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
	if f.afterAppend != nil {
		f.afterAppend(input)
	}
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
	}, webTestTurnOwner{}))
	return srv, srv.Handler()
}

type webTestTurnOwner struct {
	acquireErr error
	conflict   bool
	leaseLost  bool
}

func (o webTestTurnOwner) Acquire(context.Context, time.Duration) (memory.TurnLease, error) {
	if o.acquireErr != nil {
		return memory.TurnLease{}, o.acquireErr
	}
	return memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 1}, nil
}
func (webTestTurnOwner) Heartbeat(context.Context, memory.TurnLease, time.Duration) (memory.TurnLease, error) {
	return memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: 1}, nil
}
func (webTestTurnOwner) Authorize(context.Context, memory.TurnLease) error { return nil }
func (webTestTurnOwner) Release(context.Context, memory.TurnLease) error   { return nil }
func (o webTestTurnOwner) IsConflict(error) bool                           { return o.conflict }
func (o webTestTurnOwner) IsSessionInactive(error) bool                    { return false }
func (o webTestTurnOwner) IsLeaseLost(error) bool                          { return o.leaseLost }

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

type asyncWebClient struct {
	deltaWriteStarted <-chan struct{}
	callbackDone      chan<- struct{}
}

func (c asyncWebClient) ChatStream(_ context.Context, _ openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	go func() {
		handlers.OnContent("async")
		close(c.callbackDone)
	}()
	<-c.deltaWriteStarted
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{
		Message: openrouter.Message{Role: "assistant", Content: "complete"},
	}}}, nil
}

type blockingDeltaWriter struct {
	*httptest.ResponseRecorder
	deltaStarted chan struct{}
	releaseDelta <-chan struct{}
	once         sync.Once
}

func (w *blockingDeltaWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "event: delta\n") {
		w.once.Do(func() { close(w.deltaStarted) })
		<-w.releaseDelta
	}
	return w.ResponseRecorder.Write(data)
}

func (w *blockingDeltaWriter) Flush() {}

func TestChatWaitsForAdmittedProviderCallbackBeforeAssistantAndTurnDone(t *testing.T) {
	deltaStarted := make(chan struct{})
	releaseDelta := make(chan struct{})
	callbackDone := make(chan struct{})
	client := asyncWebClient{deltaWriteStarted: deltaStarted, callbackDone: callbackDone}
	session := agent.New(client, "test", &fakeHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	w := &blockingDeltaWriter{
		ResponseRecorder: httptest.NewRecorder(),
		deltaStarted:     deltaStarted,
		releaseDelta:     releaseDelta,
	}
	done := make(chan struct{})
	go func() {
		NewServer(session).Handler().ServeHTTP(w, chatRequest(`{"message":"hi"}`))
		close(done)
	}()
	select {
	case <-deltaStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("asynchronous delta did not reach the SSE writer")
	}
	select {
	case <-done:
		t.Fatal("HTTP turn completed while asynchronous provider callback was active")
	default:
	}
	close(releaseDelta)
	select {
	case <-callbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("asynchronous delta callback did not return")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP turn did not finish after asynchronous callback joined")
	}
	assertSSEOrder(t, w.Body.String(),
		"event: delta\ndata: {\"text\":\"async\"}",
		"event: assistant_done\ndata: {\"content\":\"complete\"}",
		"event: turn_done\ndata: {}",
	)
}

type concurrentWebClient struct {
	reasoningWriteStarted <-chan struct{}
	contentStarted        chan struct{}
	callbacksDone         chan struct{}
	allowReturn           <-chan struct{}
}

func (c concurrentWebClient) ChatStream(_ context.Context, _ openrouter.ChatRequest, handlers openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	go handlers.OnReasoning("thinking")
	<-c.reasoningWriteStarted
	go func() {
		close(c.contentStarted)
		handlers.OnContent("answer")
		close(c.callbacksDone)
	}()
	<-c.contentStarted
	<-c.allowReturn
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{
		Message: openrouter.Message{Role: "assistant", Content: "complete"},
	}}}, nil
}

type blockingReasoningWriter struct {
	*httptest.ResponseRecorder
	reasoningStarted chan struct{}
	releaseReasoning <-chan struct{}
	once             sync.Once
}

func (w *blockingReasoningWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "event: reasoning\n") {
		w.once.Do(func() { close(w.reasoningStarted) })
		<-w.releaseReasoning
	}
	return w.ResponseRecorder.Write(data)
}

func (w *blockingReasoningWriter) Flush() {}

func TestChatSerializesConcurrentProviderCallbacksBeforeTurnDone(t *testing.T) {
	reasoningStarted := make(chan struct{})
	releaseReasoning := make(chan struct{})
	contentStarted := make(chan struct{})
	callbacksDone := make(chan struct{})
	allowReturn := make(chan struct{})
	client := concurrentWebClient{
		reasoningWriteStarted: reasoningStarted,
		contentStarted:        contentStarted,
		callbacksDone:         callbacksDone,
		allowReturn:           allowReturn,
	}
	session := agent.New(client, "test", &fakeHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	w := &blockingReasoningWriter{
		ResponseRecorder: httptest.NewRecorder(),
		reasoningStarted: reasoningStarted,
		releaseReasoning: releaseReasoning,
	}
	done := make(chan struct{})
	go func() {
		NewServer(session).Handler().ServeHTTP(w, chatRequest(`{"message":"hi"}`))
		close(done)
	}()
	select {
	case <-contentStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent content callback did not start")
	}
	select {
	case <-done:
		t.Fatal("HTTP turn completed before concurrent callbacks joined")
	default:
	}
	close(releaseReasoning)
	select {
	case <-callbacksDone:
	case <-time.After(5 * time.Second):
		t.Fatal("serialized SSE callbacks did not finish")
	}
	close(allowReturn)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP turn did not finish after concurrent callbacks were released")
	}
	assertSSEOrder(t, w.Body.String(),
		"event: reasoning\ndata: {\"text\":\"thinking\"}",
		"event: reasoning_done\ndata: {}",
		"event: delta\ndata: {\"text\":\"answer\"}",
		"event: assistant_done\ndata: {\"content\":\"complete\"}",
		"event: turn_done\ndata: {}",
	)
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

type contentFirstWebClient struct{}

func (contentFirstWebClient) ChatStream(
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

func TestChatSuppressesReasoningThatArrivesAfterContent(t *testing.T) {
	session := agent.New(contentFirstWebClient{}, "test", &fakeHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	recorder := httptest.NewRecorder()
	NewServer(session).Handler().ServeHTTP(recorder, chatRequest(`{"message":"hi"}`))
	body := recorder.Body.String()
	if strings.Contains(body, "event: reasoning") {
		t.Fatalf("late reasoning reopened presentation:\n%s", body)
	}
	assertSSEOrder(t, body,
		"event: delta\ndata: {\"text\":\"answer\"}",
		"event: assistant_done\ndata: {\"content\":\"answer\"}",
		"event: turn_done\ndata: {}",
	)
}

func TestCommittedToolCallingAssistantPrecedesTerminalSSEAndSuppressesTools(t *testing.T) {
	for _, presentation := range []struct {
		name   string
		deltas []string
	}{
		{name: "zero delta"},
		{name: "matching prefix", deltas: []string{"committed"}},
		{name: "divergent content", deltas: []string{"speculative"}},
	} {
		for _, cause := range []string{"caller cancellation", "lease loss"} {
			t.Run(presentation.name+"_"+cause, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				history := &fakeHistory{}
				owner := webTestTurnOwner{}
				if cause == "caller cancellation" {
					history.afterAppend = func(input memory.EventInput) {
						if input.Type == memory.EventAssistantMessage {
							cancel()
						}
					}
				} else {
					history.appendErrAt = 3 // intent append, before ToolCall presentation
					history.appendErr = errors.New("lease fence lost")
					owner.leaseLost = true
				}
				client := &fakeClient{steps: []fakeStep{{
					deltas:  presentation.deltas,
					content: "committed",
					toolCalls: []openrouter.ToolCall{{
						ID: "call", Type: "function",
						Function: openrouter.FunctionCall{Name: "missing", Arguments: `{}`},
					}},
				}}}
				session := agent.New(client, "test", history, memory.ScopeContext{
					OwnerID: memory.LocalOwnerID, SessionID: "test-session",
				}, owner)
				recorder := httptest.NewRecorder()
				events, err := newSSEEvents(recorder)
				if err != nil {
					t.Fatal(err)
				}
				sendErr := session.Send(ctx, "go", events, nil)
				if sendErr == nil {
					t.Fatal("Send succeeded after selected terminal cause")
				}
				events.Error(sendErr.Error())
				events.TurnDone()
				body := recorder.Body.String()
				if strings.Count(body, "event: assistant_done\n") != 1 ||
					strings.Contains(body, "event: tool_call\n") ||
					strings.Contains(body, "event: response_discarded\n") {
					t.Fatalf("terminal SSE contract violated:\n%s", body)
				}
				assertSSEOrder(t, body,
					"event: assistant_done\ndata: {\"content\":\"committed\"}",
					"event: error",
					"event: turn_done",
				)
			})
		}
	}
}

func TestChatStreamsContentDiscardBeforeErrorAndTurnDone(t *testing.T) {
	history := &fakeHistory{appendErrAt: 2}
	client := &fakeClient{steps: []fakeStep{{deltas: []string{"partial"}, content: "partial"}}}
	session := agent.New(client, "test", history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	recorder := httptest.NewRecorder()
	NewServer(session).Handler().ServeHTTP(recorder, chatRequest(`{"message":"hi"}`))
	body := recorder.Body.String()
	assertSSEOrder(t, body,
		"event: delta\ndata: {\"text\":\"partial\"}",
		"event: response_discarded\ndata: {\"reason\":\"assistant_persistence_failed\",\"message\":\"Response interrupted; streamed text was not saved.\"}",
		"event: error",
		"event: turn_done",
	)
}

func TestChatStreamsReasoningDoneBeforeStandaloneDiscard(t *testing.T) {
	history := &fakeHistory{appendErrAt: 2}
	client := &fakeClient{steps: []fakeStep{{reasoning: []string{"thinking"}, content: "unrendered final"}}}
	session := agent.New(client, "test", history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{})
	recorder := httptest.NewRecorder()
	NewServer(session).Handler().ServeHTTP(recorder, chatRequest(`{"message":"hi"}`))
	body := recorder.Body.String()
	assertSSEOrder(t, body,
		"event: reasoning\ndata: {\"text\":\"thinking\"}",
		"event: reasoning_done\ndata: {}",
		"event: response_discarded\ndata: {\"reason\":\"assistant_persistence_failed\",\"message\":\"Response interrupted; streamed text was not saved.\"}",
		"event: error",
		"event: turn_done",
	)
}

func TestDurableLeaseConflictUsesPreStream409JSON(t *testing.T) {
	conflictErr := errors.New("lease held elsewhere")
	session := agent.New(&fakeClient{}, "test", &fakeHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{acquireErr: conflictErr, conflict: true})
	recorder := httptest.NewRecorder()
	NewServer(session).Handler().ServeHTTP(recorder, chatRequest(`{"message":"hi"}`))
	if recorder.Code != http.StatusConflict || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status=%d content-type=%q body=%s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "event:") || !strings.Contains(recorder.Body.String(), `"error"`) {
		t.Fatalf("conflict response=%s", recorder.Body.String())
	}
}

func assertSSEOrder(t *testing.T, body string, values ...string) {
	t.Helper()
	last := -1
	for _, value := range values {
		index := strings.Index(body, value)
		if index <= last {
			t.Fatalf("SSE value %q missing or out of order in:\n%s", value, body)
		}
		last = index
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
