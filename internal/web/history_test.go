package web

import (
	"encoding/json"
	"fmt"
	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelectedHistoryRestoresPersistedMessagesAndRejectsOtherSessions(t *testing.T) {
	h := &fakeHistory{events: []memory.Event{
		{ID: "u1", Sequence: 1, Type: memory.EventUserMessage, Content: "remember concise answers"},
		{ID: "a1", Sequence: 2, Type: memory.EventAssistantMessage, Content: "Saved after approval.", Payload: []byte(`{}`)},
		{ID: "internal", Sequence: 3, Type: memory.EventContextSnapshot, Content: "private internal context"},
	}}
	runtime := agent.New(&fakeClient{}, webTestContextProfile("test"), h, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "one"}, webTestTurnOwner{})
	server := NewContextServer(runtime, nil, nil, &fakeContextSessionController{})
	server.activeSession = memory.Session{ID: "one"}
	for _, tc := range []struct {
		body   string
		status int
	}{{`{"sessionId":"one"}`, 200}, {`{"sessionId":"other"}`, 409}} {
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, managementRequest("/api/context-sessions/history", tc.body))
		if rr.Code != tc.status {
			t.Fatalf("status=%d want %d: %s", rr.Code, tc.status, rr.Body.String())
		}
		if tc.status == http.StatusOK && (!strings.Contains(rr.Body.String(), "remember concise answers") || !strings.Contains(rr.Body.String(), "Saved after approval.") || strings.Contains(rr.Body.String(), "private internal")) {
			t.Fatal(rr.Body.String())
		}
	}
}

func TestHistoryPagingAndRecordedToolDecisions(t *testing.T) {
	events := []memory.Event{{ID: "intent", Type: memory.EventToolIntent, ExecutionID: "exec", Payload: []byte(`{"call":{"id":"call","name":"memory_remember_literal","arguments":"{}"}}`)}}
	for i := 0; i < 204; i++ {
		events = append(events, memory.Event{ID: memory.EventID(fmt.Sprintf("u%d", i)), Type: memory.EventUserMessage, Content: fmt.Sprint(i)})
	}
	events = append(events, memory.Event{Type: memory.EventApproval, ExecutionID: "exec", Payload: []byte(`{"decision":"approved"}`)}, memory.Event{Type: memory.EventToolSucceeded, ExecutionID: "exec", Content: "saved"})
	items, err := projectHistory(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 205 || items[0].Approval.State != "approved" || items[0].Approval.RequestID != "" || *items[0].Result != "saved" {
		t.Fatalf("invalid historical tool record: %+v", items[0])
	}
	runtime := agent.New(&fakeClient{}, webTestContextProfile("test"), &fakeHistory{events: events}, memory.ScopeContext{SessionID: "one"}, webTestTurnOwner{})
	server := NewContextServer(runtime, nil, nil, &fakeContextSessionController{})
	server.activeSession = memory.Session{ID: "one"}
	cursor := ""
	seen := map[string]bool{}
	for _, count := range []int{100, 100, 5} {
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, managementRequest("/api/context-sessions/history", fmt.Sprintf(`{"sessionId":"one","before":%q}`, cursor)))
		var page struct {
			Items  []historyItem
			Before string
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if rr.Code != 200 || len(page.Items) != count {
			t.Fatalf("page=%s", rr.Body.String())
		}
		for _, item := range page.Items {
			if seen[item.Key] {
				t.Fatal("duplicate item")
			}
			seen[item.Key] = true
		}
		cursor = page.Before
	}
	if cursor != "" || len(seen) != 205 {
		t.Fatal("missing history")
	}
	server.activeTurns = 1
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/context-sessions/history", `{"sessionId":"one"}`))
	if rr.Code != 409 {
		t.Fatal(rr.Code)
	}
}

func TestChatRejectsAnotherTabsSelectedSession(t *testing.T) {
	h := &fakeHistory{}
	runtime := agent.New(&fakeClient{}, webTestContextProfile("test"), h, memory.ScopeContext{SessionID: "two"}, webTestTurnOwner{})
	server := NewContextServer(runtime, nil, nil, &fakeContextSessionController{})
	server.activeSession = memory.Session{ID: "two"}
	for _, body := range []string{`{"message":"belongs to one","sessionId":"one"}`, `{"message":"missing binding"}`} {
		rr := httptest.NewRecorder()
		server.Handler().ServeHTTP(rr, managementRequest("/api/chat", body))
		if rr.Code != http.StatusConflict || len(h.events) != 0 || server.activeTurns != 0 {
			t.Fatalf("unbound send: %d %s events=%v", rr.Code, rr.Body.String(), h.events)
		}
	}
}
