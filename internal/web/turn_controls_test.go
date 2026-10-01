package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

const stoppedEvent = "event: error\ndata: {\"message\":\"Turn stopped.\",\"code\":\"turn_stopped\"}\n\n"

// stoppableClient streams one partial delta on its first call and then waits
// for the turn's context, the shape of a long provider response. Later calls
// answer at once so the session can prove it still works after a stop.
type stoppableClient struct {
	mu        sync.Mutex
	calls     int
	streaming chan struct{}
	cancelled chan struct{}
}

func newStoppableClient() *stoppableClient {
	return &stoppableClient{streaming: make(chan struct{}), cancelled: make(chan struct{})}
}

func (c *stoppableClient) ChatStream(ctx context.Context, _ openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.mu.Lock()
	c.calls++
	call := c.calls
	c.mu.Unlock()
	if call > 1 {
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{
			Message: openrouter.Message{Role: "assistant", Content: "next answer"},
		}}}, nil
	}
	h.OnContent("partial")
	close(c.streaming)
	<-ctx.Done()
	close(c.cancelled)
	return openrouter.ChatResponse{}, ctx.Err()
}

func (c *stoppableClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// countingTurnOwner records lease acquisition and release so a stop can be
// shown to give the session back.
type countingTurnOwner struct {
	mu       sync.Mutex
	acquires int
	releases int
}

func (o *countingTurnOwner) Acquire(context.Context, time.Duration) (memory.TurnLease, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.acquires++
	return memory.TurnLease{SessionID: "test-session", HolderID: "holder", FencingToken: memory.FencingToken(o.acquires)}, nil
}

func (o *countingTurnOwner) Heartbeat(_ context.Context, lease memory.TurnLease, _ time.Duration) (memory.TurnLease, error) {
	return lease, nil
}
func (o *countingTurnOwner) Authorize(context.Context, memory.TurnLease) error { return nil }
func (o *countingTurnOwner) Release(context.Context, memory.TurnLease) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.releases++
	return nil
}
func (o *countingTurnOwner) IsConflict(error) bool        { return false }
func (o *countingTurnOwner) IsSessionInactive(error) bool { return false }
func (o *countingTurnOwner) IsLeaseLost(error) bool       { return false }

func (o *countingTurnOwner) counts() (int, int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.acquires, o.releases
}

func controlRequest(path, body string) *http.Request {
	return managementRequest(path, body)
}

// serveAsync runs one request in the background, the way a browser keeps a
// chat stream open while sending a separate stop request.
func serveAsync(h http.Handler, req *http.Request) <-chan *httptest.ResponseRecorder {
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		done <- rec
	}()
	return done
}

func awaitResponse(t *testing.T, done <-chan *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	select {
	case rec := <-done:
		return rec
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
		return nil
	}
}

func awaitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func terminalEvents(t *testing.T, history *fakeHistory) []memory.TurnTerminalPayload {
	t.Helper()
	var terminals []memory.TurnTerminalPayload
	for _, event := range history.events {
		if event.Type != memory.EventTurnInterrupted && event.Type != memory.EventTurnFailed {
			continue
		}
		if event.Type != memory.EventTurnInterrupted {
			t.Fatalf("terminal event type=%s, want %s", event.Type, memory.EventTurnInterrupted)
		}
		var payload memory.TurnTerminalPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		terminals = append(terminals, payload)
	}
	return terminals
}

func assertControlError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var body struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if rec.Code != status || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Code != code || body.Error == "" {
		t.Fatalf("status=%d body=%s, want %d with code %q", rec.Code, rec.Body.String(), status, code)
	}
}

func TestCancelMidStreamInterruptsTurnAndSessionStaysUsable(t *testing.T) {
	client := newStoppableClient()
	history := &fakeHistory{}
	owner := &countingTurnOwner{}
	session := agent.New(client, webTestContextProfile("test"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, owner)
	h := NewServer(session).Handler()

	chat := serveAsync(h, chatRequest(`{"message":"write a long essay"}`))
	awaitSignal(t, client.streaming, "the provider to stream")

	stop := httptest.NewRecorder()
	h.ServeHTTP(stop, controlRequest("/api/cancel", `{}`))
	if stop.Code != http.StatusAccepted || !strings.Contains(stop.Body.String(), `"status":"stopping"`) {
		t.Fatalf("cancel status=%d body=%s", stop.Code, stop.Body.String())
	}
	first := awaitResponse(t, chat)
	awaitSignal(t, client.cancelled, "provider cancellation")

	assertSSEOrder(t, first.Body.String(),
		"event: delta\ndata: {\"text\":\"partial\"}",
		"event: response_discarded\ndata: {\"reason\":\"caller_cancelled\"",
		stoppedEvent,
		"event: turn_done\ndata: {}",
	)
	if !strings.HasSuffix(first.Body.String(), "event: turn_done\ndata: {}\n\n") {
		t.Fatalf("stopped stream must end with turn_done:\n%s", first.Body.String())
	}
	terminals := terminalEvents(t, history)
	if len(terminals) != 1 || terminals[0].Classification != memory.ClassificationCallerCancelled || terminals[0].Stage != memory.StageProvider {
		t.Fatalf("terminal evidence=%+v, want one caller_cancelled interruption at provider", terminals)
	}
	for _, event := range history.events {
		if event.Type == memory.EventAssistantMessage {
			t.Fatal("stopped provider response was committed")
		}
	}
	if acquires, releases := owner.counts(); acquires != 1 || releases != 1 {
		t.Fatalf("lease acquires=%d releases=%d, want the stopped turn's lease released", acquires, releases)
	}

	again := httptest.NewRecorder()
	h.ServeHTTP(again, controlRequest("/api/cancel", `{}`))
	assertControlError(t, again, http.StatusConflict, "no_active_turn")

	next := httptest.NewRecorder()
	h.ServeHTTP(next, chatRequest(`{"message":"short answer please"}`))
	if next.Code != http.StatusOK || !strings.Contains(next.Body.String(), "event: assistant_done\ndata: {\"content\":\"next answer\"") ||
		strings.Contains(next.Body.String(), "event: error") {
		t.Fatalf("next turn status=%d body=%s", next.Code, next.Body.String())
	}
	if acquires, releases := owner.counts(); acquires != 2 || releases != 2 {
		t.Fatalf("lease acquires=%d releases=%d after the next turn", acquires, releases)
	}
}

func TestCancelDuringToolCallCancelsTheTool(t *testing.T) {
	started := make(chan struct{})
	toolErr := make(chan error, 1)
	toolset := tools.NewToolset([]tools.Tool{{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "slow_command", Parameters: openrouter.Parameter{Type: "object"},
		}},
		Execute: func(ctx context.Context, _ string) (string, error) {
			close(started)
			<-ctx.Done()
			toolErr <- ctx.Err()
			return "", ctx.Err()
		},
	}})
	client := &fakeClient{steps: []fakeStep{{toolCalls: []openrouter.ToolCall{{
		ID: "call-1", Type: "function", Function: openrouter.FunctionCall{Name: "slow_command", Arguments: `{}`},
	}}}}}
	history := &fakeHistory{}
	owner := &countingTurnOwner{}
	session := agent.NewWithToolset(client, webTestContextProfile("test"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, owner, toolset)
	h := NewServer(session).Handler()

	chat := serveAsync(h, chatRequest(`{"message":"run the slow thing"}`))
	awaitSignal(t, started, "the tool to start")
	stop := httptest.NewRecorder()
	h.ServeHTTP(stop, controlRequest("/api/cancel", `{}`))
	if stop.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", stop.Code, stop.Body.String())
	}
	rec := awaitResponse(t, chat)

	select {
	case err := <-toolErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("tool context error=%v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("running tool was not cancelled")
	}
	assertSSEOrder(t, rec.Body.String(),
		"event: tool_call\ndata: {\"id\":\"call-1\"",
		stoppedEvent,
		"event: turn_done\ndata: {}",
	)
	terminals := terminalEvents(t, history)
	if len(terminals) != 1 || terminals[0].Classification != memory.ClassificationCallerCancelled || terminals[0].Stage != memory.StageToolExecute {
		t.Fatalf("terminal evidence=%+v, want caller_cancelled at tool_execute", terminals)
	}
	if len(client.reqs) != 1 {
		t.Fatalf("provider requests=%d, want no provider call after the stop", len(client.reqs))
	}
	if acquires, releases := owner.counts(); acquires != 1 || releases != 1 {
		t.Fatalf("lease acquires=%d releases=%d", acquires, releases)
	}
}

func TestCancelWhileApprovalPendingExpiresTheApproval(t *testing.T) {
	executed := make(chan struct{}, 1)
	toolset := tools.NewToolset([]tools.Tool{{
		Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
			Name: "gated_write", Parameters: openrouter.Parameter{Type: "object"},
		}},
		NeedsApproval: true,
		Execute: func(context.Context, string) (string, error) {
			executed <- struct{}{}
			return "written", nil
		},
	}})
	client := &fakeClient{steps: []fakeStep{{toolCalls: []openrouter.ToolCall{{
		ID: "call-1", Type: "function", Function: openrouter.FunctionCall{Name: "gated_write", Arguments: `{}`},
	}}}}}
	history := &fakeHistory{}
	session := agent.NewWithToolset(client, webTestContextProfile("test"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, &countingTurnOwner{}, toolset)
	srv := NewServer(session)
	h := srv.Handler()

	chat := serveAsync(h, chatRequest(`{"message":"write it"}`))
	approvalID := pendingID(t, srv)
	stop := httptest.NewRecorder()
	h.ServeHTTP(stop, controlRequest("/api/cancel", `{}`))
	if stop.Code != http.StatusAccepted {
		t.Fatalf("cancel status=%d body=%s", stop.Code, stop.Body.String())
	}
	rec := awaitResponse(t, chat)

	assertSSEOrder(t, rec.Body.String(), "event: approval_request", stoppedEvent, "event: turn_done\ndata: {}")
	late := httptest.NewRecorder()
	h.ServeHTTP(late, approveRequest(approvalID, true))
	if late.Code != http.StatusNotFound {
		t.Fatalf("late approval status=%d, want the stopped turn's approval gone", late.Code)
	}
	select {
	case <-executed:
		t.Fatal("gated tool ran after the turn was stopped")
	default:
	}
	terminals := terminalEvents(t, history)
	if len(terminals) != 1 || terminals[0].Stage != memory.StageToolApproval {
		t.Fatalf("terminal evidence=%+v, want caller_cancelled at tool_approval", terminals)
	}
}

func approveRequest(id string, approve bool) *http.Request {
	body, _ := json.Marshal(map[string]any{"id": id, "approve": approve})
	return controlRequest("/api/approve", string(body))
}

func TestOnlyACleanOwnerStopIsReportedAsStopped(t *testing.T) {
	stopped, stop := context.WithCancelCause(context.Background())
	stop(errTurnStopped)
	if !stoppedByOwner(stopped, context.Canceled) {
		t.Fatal("owner stop not recognized")
	}
	if stoppedByOwner(stopped, errors.Join(context.Canceled, errors.New("persist terminal event: disk full"))) {
		t.Fatal("a stop whose terminal evidence failed must keep the ordinary error")
	}
	if stoppedByOwner(stopped, nil) {
		t.Fatal("a turn that completed before the stop is not stopped")
	}
	runtime, shutdown := context.WithCancel(context.Background())
	turn, release := context.WithCancelCause(runtime)
	defer release(nil)
	shutdown()
	if stoppedByOwner(turn, context.Canceled) {
		t.Fatal("runtime shutdown reported as an owner stop")
	}
}

func TestCancelWithNoActiveTurnIsAClearNoOp(t *testing.T) {
	client := &fakeClient{}
	h := newTestServer(client)
	for range 2 {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, controlRequest("/api/cancel", `{}`))
		assertControlError(t, rec, http.StatusConflict, "no_active_turn")
	}
	if len(client.reqs) != 0 {
		t.Fatalf("cancel reached the provider: %d requests", len(client.reqs))
	}
}

// contextTurnServer is a Context-managed server whose active conversation is
// session-1, the shape the browser always talks to.
func contextTurnServer(client agent.Client, history *fakeHistory) (*Server, http.Handler) {
	session := agent.New(client, webTestContextProfile("test"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "session-1",
	}, webTestTurnOwner{})
	srv := NewContextServer(session, nil, nil, &fakeContextSessionController{session: session})
	srv.activeSession = memory.Session{ID: "session-1", Status: memory.SessionActive}
	return srv, srv.Handler()
}

func TestCancelCannotStopAnotherSessionsTurn(t *testing.T) {
	client := &fakeClient{
		steps:   []fakeStep{{content: "finished anyway"}},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	_, h := contextTurnServer(client, &fakeHistory{})

	chat := serveAsync(h, chatRequest(`{"message":"hello","sessionId":"session-1"}`))
	<-client.entered
	for _, body := range []string{`{"sessionId":"session-2"}`, `{}`, `{"sessionId":""}`} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, controlRequest("/api/cancel", body))
		assertControlError(t, rec, http.StatusConflict, "no_active_turn")
	}
	close(client.release)
	rec := awaitResponse(t, chat)
	if strings.Contains(rec.Body.String(), "event: error") ||
		!strings.Contains(rec.Body.String(), "event: assistant_done\ndata: {\"content\":\"finished anyway\"") {
		t.Fatalf("turn in another session was disturbed:\n%s", rec.Body.String())
	}
}

func TestCancelStopsTheIdentifiedSessionsTurn(t *testing.T) {
	client := newStoppableClient()
	history := &fakeHistory{}
	_, h := contextTurnServer(client, history)

	chat := serveAsync(h, chatRequest(`{"message":"hello","sessionId":"session-1"}`))
	awaitSignal(t, client.streaming, "the provider to stream")
	for range 2 {
		// Repeated stops while the turn unwinds stay accepted and harmless.
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, controlRequest("/api/cancel", `{"sessionId":"session-1"}`))
		if rec.Code != http.StatusAccepted && rec.Code != http.StatusConflict {
			t.Fatalf("cancel status=%d body=%s", rec.Code, rec.Body.String())
		}
	}
	rec := awaitResponse(t, chat)
	assertSSEOrder(t, rec.Body.String(), stoppedEvent, "event: turn_done\ndata: {}")
	if terminals := terminalEvents(t, history); len(terminals) != 1 {
		t.Fatalf("terminal evidence=%+v", terminals)
	}
}

func TestTurnControlRoutesStayBehindTheGuard(t *testing.T) {
	client := &fakeClient{
		steps:   []fakeStep{{content: "unaffected"}},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := newTestServer(client)
	chat := serveAsync(h, chatRequest(`{"message":"hello"}`))
	<-client.entered

	for _, path := range []string{"/api/cancel", "/api/compact"} {
		get := httptest.NewRecorder()
		h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687"+path, nil))
		if get.Code != http.StatusMethodNotAllowed || get.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("GET %s status=%d allow=%q", path, get.Code, get.Header().Get("Allow"))
		}

		form := controlRequest(path, `{}`)
		form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		plain := controlRequest(path, `{}`)
		plain.Header.Set("Content-Type", "text/plain")
		foreign := controlRequest(path, `{}`)
		foreign.Header.Set("Origin", "http://evil.example")
		rebound := controlRequest(path, `{}`)
		rebound.Host = "rebound.example:6687"
		for name, req := range map[string]*http.Request{"form": form, "plain": plain, "origin": foreign, "host": rebound} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s status=%d, want 403", name, path, rec.Code)
			}
		}
	}

	close(client.release)
	rec := awaitResponse(t, chat)
	if strings.Contains(rec.Body.String(), "event: error") || !strings.Contains(rec.Body.String(), "unaffected") {
		t.Fatalf("rejected control requests disturbed the turn:\n%s", rec.Body.String())
	}
}

func TestCancelRejectsMalformedBodies(t *testing.T) {
	h := newTestServer(&fakeClient{})
	for _, body := range []string{`not json`, `{"sessionId":1}`, `{"session":"x"}`, `{} {}`} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, controlRequest("/api/cancel", body))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q status=%d, want 400", body, rec.Code)
		}
	}
}

func summaryStep() fakeStep {
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		summary.WriteString("## ")
		summary.WriteString(heading)
		summary.WriteString("\nkept\n\n")
	}
	return fakeStep{content: summary.String()}
}

// threeTurns runs enough completed turns through the web path that manual
// compaction has an eligible prefix (it always retains the two newest).
func threeTurns(t *testing.T, h http.Handler, sessionID string) {
	t.Helper()
	for _, message := range []string{"first", "second", "third"} {
		body, _ := json.Marshal(map[string]string{"message": message, "sessionId": sessionID})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, chatRequest(string(body)))
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "event: error") {
			t.Fatalf("setup turn %q status=%d body=%s", message, rec.Code, rec.Body.String())
		}
	}
}

func TestCompactSummarizesTheActiveSession(t *testing.T) {
	client := &fakeClient{steps: []fakeStep{{content: "a1"}, {content: "a2"}, {content: "a3"}, summaryStep()}}
	history := &fakeHistory{}
	_, h := contextTurnServer(client, history)
	threeTurns(t, h, "session-1")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, controlRequest("/api/compact", `{"sessionId":"session-1"}`))
	var body struct {
		Outcome string `json:"outcome"`
		EventID string `json:"eventId"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil || body.Outcome != "compacted" || body.EventID == "" {
		t.Fatalf("compact status=%d body=%s", rec.Code, rec.Body.String())
	}
	last := history.events[len(history.events)-1]
	if last.Type != memory.EventContextCompacted || string(last.ID) != body.EventID {
		t.Fatalf("last durable event=%s %s, want the reported compaction", last.Type, last.ID)
	}
	if len(client.reqs) != 4 || len(client.reqs[3].Tools) != 0 {
		t.Fatalf("provider requests=%d, want three turns plus one tool-free compactor call", len(client.reqs))
	}
}

func TestCompactReportsNothingEligible(t *testing.T) {
	client := &fakeClient{steps: []fakeStep{{content: "only one"}}}
	history := &fakeHistory{}
	_, h := contextTurnServer(client, history)
	chat := httptest.NewRecorder()
	h.ServeHTTP(chat, chatRequest(`{"message":"hi","sessionId":"session-1"}`))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, controlRequest("/api/compact", `{"sessionId":"session-1"}`))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"outcome":"nothing_to_compact"}` {
		t.Fatalf("compact status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(client.reqs) != 1 {
		t.Fatalf("provider requests=%d, want no compactor call", len(client.reqs))
	}
	for _, event := range history.events {
		if event.Type == memory.EventContextCompacted {
			t.Fatal("no-op compaction wrote an event")
		}
	}
}

func TestCompactReportsFailureCategory(t *testing.T) {
	tests := []struct {
		name           string
		step           fakeStep
		classification string
	}{
		{name: "provider", step: fakeStep{err: errors.New("upstream status 503 secret-body")}, classification: "provider_error"},
		{name: "invalid", step: fakeStep{content: "not the seven sections"}, classification: "provider_response_invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &fakeClient{steps: []fakeStep{{content: "a1"}, {content: "a2"}, {content: "a3"}, test.step}}
			history := &fakeHistory{}
			_, h := contextTurnServer(client, history)
			threeTurns(t, h, "session-1")
			before := len(history.events)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, controlRequest("/api/compact", `{"sessionId":"session-1"}`))
			var body struct {
				Code           string `json:"code"`
				Classification string `json:"classification"`
				Error          string `json:"error"`
			}
			if rec.Code != http.StatusBadGateway || json.Unmarshal(rec.Body.Bytes(), &body) != nil ||
				body.Code != "compaction_failed" || body.Classification != test.classification || body.Error == "" {
				t.Fatalf("compact status=%d body=%s", rec.Code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "secret-body") {
				t.Fatalf("compaction failure leaked provider detail: %s", rec.Body.String())
			}
			if len(history.events) != before {
				t.Fatal("failed compaction changed durable history")
			}
		})
	}
}

func TestCompactRefusedWhileATurnIsRunning(t *testing.T) {
	client := &fakeClient{
		steps:   []fakeStep{{content: "done"}},
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	_, h := contextTurnServer(client, &fakeHistory{})
	chat := serveAsync(h, chatRequest(`{"message":"hello","sessionId":"session-1"}`))
	<-client.entered

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, controlRequest("/api/compact", `{"sessionId":"session-1"}`))
	assertControlError(t, rec, http.StatusConflict, "turn_in_progress")

	close(client.release)
	awaitResponse(t, chat)
	if len(client.reqs) != 1 {
		t.Fatalf("provider requests=%d, want no compactor call during the turn", len(client.reqs))
	}
}

func TestCompactRefusesAnotherSessionAndHeldLease(t *testing.T) {
	_, h := contextTurnServer(&fakeClient{}, &fakeHistory{})
	for _, body := range []string{`{"sessionId":"session-2"}`, `{}`} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, controlRequest("/api/compact", body))
		assertControlError(t, rec, http.StatusConflict, "context_session_changed")
	}

	held := agent.New(&fakeClient{}, webTestContextProfile("test"), &fakeHistory{}, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, webTestTurnOwner{acquireErr: errors.New("lease held by the REPL"), conflict: true})
	rec := httptest.NewRecorder()
	NewServer(held).Handler().ServeHTTP(rec, controlRequest("/api/compact", `{}`))
	assertControlError(t, rec, http.StatusConflict, "session_busy")
}
