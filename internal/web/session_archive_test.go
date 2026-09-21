package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type fakeArchiveController struct {
	fakeContextSessionController
	changed []memory.Session
	err     error
	entered chan struct{}
	release chan struct{}
}

func (c *fakeArchiveController) ArchiveSession(_ context.Context, id memory.SessionID) (memory.Session, error) {
	return c.change(id, memory.SessionClosed)
}

func (c *fakeArchiveController) RestoreSession(_ context.Context, id memory.SessionID) (memory.Session, error) {
	return c.change(id, memory.SessionActive)
}

func (c *fakeArchiveController) change(id memory.SessionID, status memory.SessionStatus) (memory.Session, error) {
	if c.entered != nil {
		close(c.entered)
		<-c.release
	}
	if c.err != nil {
		return memory.Session{}, c.err
	}
	session := memory.Session{ID: id, Status: status}
	c.changed = append(c.changed, session)
	return session, nil
}

func TestContextSessionArchiveClearsSelectedConversationAndRestoreDoesNotSelect(t *testing.T) {
	controller := &fakeArchiveController{}
	server := NewContextServer(&agent.Session{}, nil, nil, controller)
	server.activeSession = memory.Session{ID: "session-1"}
	handler := server.Handler()
	archived := httptest.NewRecorder()
	handler.ServeHTTP(archived, managementRequest("/api/context-sessions/archive", `{"sessionId":"session-1"}`))
	if archived.Code != http.StatusOK || !strings.Contains(archived.Body.String(), `"status":"closed"`) ||
		server.activeSession.ID != "" || server.session != nil {
		t.Fatalf("archive status=%d body=%s selected=%+v", archived.Code, archived.Body.String(), server.activeSession)
	}
	staleChat := httptest.NewRecorder()
	handler.ServeHTTP(staleChat, chatRequest(`{"sessionId":"session-1","message":"stale"}`))
	if staleChat.Code != http.StatusConflict {
		t.Fatalf("stale chat status=%d body=%s", staleChat.Code, staleChat.Body.String())
	}
	restored := httptest.NewRecorder()
	handler.ServeHTTP(restored, managementRequest("/api/context-sessions/restore", `{"sessionId":"session-1"}`))
	if restored.Code != http.StatusOK || !strings.Contains(restored.Body.String(), `"status":"active"`) ||
		server.activeSession.ID != "" || server.session != nil || len(controller.selected) != 0 {
		t.Fatalf("restore status=%d body=%s selected=%+v", restored.Code, restored.Body.String(), server.activeSession)
	}
}

func TestContextSessionArchivePreservesOtherSelectionAndFailedSelection(t *testing.T) {
	for _, failure := range []bool{false, true} {
		controller := &fakeArchiveController{}
		id := "other-session"
		if failure {
			controller.err = errors.New("private SQLite path /secret/evie.db")
			id = "selected-session"
		}
		selectedAgent := &agent.Session{}
		server := NewContextServer(selectedAgent, nil, nil, controller)
		server.activeSession = memory.Session{ID: "selected-session"}
		result := httptest.NewRecorder()
		server.Handler().ServeHTTP(result, managementRequest("/api/context-sessions/archive", `{"sessionId":"`+id+`"}`))
		want := http.StatusOK
		if failure {
			want = http.StatusInternalServerError
		}
		if result.Code != want || strings.Contains(result.Body.String(), "/secret/") ||
			server.activeSession.ID != "selected-session" || server.session != selectedAgent || server.selectingSession {
			t.Fatalf("failure=%v status=%d body=%s active=%+v", failure, result.Code, result.Body.String(), server.activeSession)
		}
	}
}

func TestContextSessionArchiveHTTPValidationAndBusyBoundaries(t *testing.T) {
	for _, path := range []string{"/api/context-sessions/archive", "/api/context-sessions/restore"} {
		for _, tc := range []struct {
			body      string
			busy      bool
			selecting bool
			cause     error
			status    int
		}{
			{body: `{}`, status: http.StatusBadRequest},
			{body: `{"sessionId":" "}`, status: http.StatusBadRequest},
			{body: `{"sessionId":"one","other":true}`, status: http.StatusBadRequest},
			{body: `{"sessionId":"one"} {}`, status: http.StatusBadRequest},
			{body: `{"sessionId":"one"}`, busy: true, status: http.StatusConflict},
			{body: `{"sessionId":"one"}`, selecting: true, status: http.StatusConflict},
			{body: `{"sessionId":"one"}`, cause: eviedb.ErrTurnLeaseHeld, status: http.StatusConflict},
			{body: `{"sessionId":"one"}`, cause: eviedb.ErrSessionNotActive, status: http.StatusConflict},
			{body: `{"sessionId":"one"}`, cause: eviedb.ErrSessionDelegated, status: http.StatusUnprocessableEntity},
		} {
			controller := &fakeArchiveController{err: tc.cause}
			server := NewContextServer(nil, nil, nil, controller)
			if tc.busy {
				server.activeTurns = 1
			}
			server.selectingSession = tc.selecting
			result := httptest.NewRecorder()
			server.Handler().ServeHTTP(result, managementRequest(path, tc.body))
			if result.Code != tc.status || len(controller.changed) != 0 {
				t.Fatalf("path=%s case=%+v status=%d body=%s", path, tc, result.Code, result.Body.String())
			}
		}
	}
}

func TestContextSessionArchiveSerializesAgainstChatAndSelection(t *testing.T) {
	controller := &fakeArchiveController{entered: make(chan struct{}), release: make(chan struct{})}
	server := NewContextServer(&agent.Session{}, nil, nil, controller)
	server.activeSession = memory.Session{ID: "session-1"}
	handler := server.Handler()
	done := make(chan struct{})
	archived := httptest.NewRecorder()
	go func() {
		handler.ServeHTTP(archived, managementRequest("/api/context-sessions/archive", `{"sessionId":"session-1"}`))
		close(done)
	}()
	<-controller.entered
	chat := httptest.NewRecorder()
	handler.ServeHTTP(chat, chatRequest(`{"sessionId":"session-1","message":"race"}`))
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, managementRequest("/api/context-sessions/select", `{"unscoped":true}`))
	close(controller.release)
	<-done
	if chat.Code != http.StatusConflict || selected.Code != http.StatusConflict || archived.Code != http.StatusOK || len(controller.selected) != 0 {
		t.Fatalf("chat=%d select=%d archive=%d", chat.Code, selected.Code, archived.Code)
	}
}
