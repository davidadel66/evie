package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

// runningChat opens chat id in the Workspace with a turn in flight, and
// returns the call that ends that turn.
func runningChat(s *Server, id memory.SessionID, workspace memory.WorkspaceID) func() {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	s.chats[id] = &openChat{agent: &agent.Session{}, session: memory.Session{ID: id, WorkspaceID: workspace}}
	s.turns[id] = &webTurn{stop: func(error) {}}
	return func() {
		s.sessionMu.Lock()
		defer s.sessionMu.Unlock()
		delete(s.turns, id)
	}
}

// openingController opens a new chat for every unscoped selection.
type openingController struct {
	*fakeContextSessionController
	opened int
}

func (c *openingController) SelectSession(_ context.Context, selection ContextSessionSelection) (OpenedContextSession, error) {
	c.selected = append(c.selected, selection)
	id := selection.SessionID
	if id == "" {
		c.opened++
		id = memory.SessionID(fmt.Sprintf("chat-%d", c.opened))
	}
	return OpenedContextSession{Session: memory.Session{ID: id, Status: memory.SessionActive}, Agent: &agent.Session{}}, nil
}

func TestOpenChatsCloseTheLeastRecentlyUsedIdleChat(t *testing.T) {
	controller := &openingController{fakeContextSessionController: &fakeContextSessionController{}}
	server := NewContextServer(nil, nil, nil, controller)
	handler := server.Handler()
	open := func(body string) int {
		t.Helper()
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, managementRequest("/api/context-sessions/select", body))
		return result.Code
	}
	isOpen := func(id memory.SessionID) bool { return server.isOpenChat(id) }
	for range maxOpenChats {
		if code := open(`{"unscoped":true}`); code != http.StatusOK {
			t.Fatal(code)
		}
	}
	server.turns["chat-1"] = &webTurn{}
	if code := open(`{"sessionId":"chat-3"}`); code != http.StatusOK || len(controller.selected) != maxOpenChats {
		t.Fatalf("refocusing an open chat: %d after %d opens", code, len(controller.selected))
	}
	// chat-1 is the oldest but running, chat-3 was just used: chat-2 closes,
	// then chat-4.
	for _, closes := range []memory.SessionID{"chat-2", "chat-4"} {
		if code := open(`{"unscoped":true}`); code != http.StatusOK || isOpen(closes) || !isOpen("chat-1") || !isOpen("chat-3") || len(server.chats) != maxOpenChats {
			t.Fatalf("opening another chat: %d, %s open %t, %d open", code, closes, isOpen(closes), len(server.chats))
		}
	}
	for id := range server.chats {
		server.turns[id] = &webTurn{}
	}
	before := len(controller.selected)
	if code := open(`{"unscoped":true}`); code != http.StatusConflict || len(controller.selected) != before {
		t.Fatalf("all chats busy: %d, controller called %t", code, len(controller.selected) != before)
	}
}

// Changing a chat's model while its history is read is race-free.
func TestModelChangeAndHistoryReadsDoNotRace(t *testing.T) {
	s, _ := modelTestServer()
	handler := s.Handler()
	var wg sync.WaitGroup
	wg.Go(func() {
		revision := int64(0)
		for range 50 {
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, managementRequest("/api/models/select", fmt.Sprintf(`{"sessionId":"session-1","model":"anthropic/test","revision":%d}`, revision)))
			var body struct {
				Revision int64 `json:"revision"`
			}
			if rr.Code == http.StatusOK && json.Unmarshal(rr.Body.Bytes(), &body) == nil {
				revision = body.Revision
			}
		}
	})
	wg.Go(func() {
		for range 50 {
			handler.ServeHTTP(httptest.NewRecorder(), managementRequest("/api/context-sessions/history", `{"sessionId":"session-1"}`))
			handler.ServeHTTP(httptest.NewRecorder(), managementRequest("/api/memory/evidence", `{"sessionId":"session-1","snapshotId":"x"}`))
		}
	})
	wg.Wait()
}

// Model changes and compaction are held per chat: a turn elsewhere does not
// block them.
func TestAnotherChatsTurnDoesNotHoldThisChat(t *testing.T) {
	s, _ := modelTestServer()
	runningChat(s, "other", "")
	handler := s.Handler()
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, managementRequest("/api/models/select", `{"sessionId":"session-1","model":"anthropic/test","revision":0}`))
	if selected.Code != http.StatusOK {
		t.Fatalf("model change beside another chat's turn: %d %s", selected.Code, selected.Body)
	}
	compacted := httptest.NewRecorder()
	handler.ServeHTTP(compacted, managementRequest("/api/compact", `{"sessionId":"session-1"}`))
	if strings.Contains(compacted.Body.String(), "turn_in_progress") {
		t.Fatalf("compaction beside another chat's turn: %d %s", compacted.Code, compacted.Body)
	}
	busy := httptest.NewRecorder()
	handler.ServeHTTP(busy, managementRequest("/api/compact", `{"sessionId":"other"}`))
	if !strings.Contains(busy.Body.String(), "turn_in_progress") {
		t.Fatalf("compaction during the chat's own turn: %d %s", busy.Code, busy.Body)
	}
}

// Archiving a Workspace closes its open chats and leaves others open.
func TestWorkspaceArchiveClosesItsChats(t *testing.T) {
	controller := &fakeWorkspaceArchiveController{}
	server := NewContextServer(nil, nil, nil, controller)
	for id, workspace := range map[memory.SessionID]memory.WorkspaceID{"a1": "a", "a2": "a", "b1": "b"} {
		server.chats[id] = &openChat{agent: &agent.Session{}, session: memory.Session{ID: id, WorkspaceID: workspace}}
	}
	result := httptest.NewRecorder()
	server.Handler().ServeHTTP(result, managementRequest("/api/workspaces/archive", `{"workspaceId":"a"}`))
	if result.Code != http.StatusOK || server.isOpenChat("a1") || server.isOpenChat("a2") || !server.isOpenChat("b1") {
		t.Fatalf("archive: %d, open a1=%t a2=%t b1=%t", result.Code, server.isOpenChat("a1"), server.isOpenChat("a2"), server.isOpenChat("b1"))
	}
}
