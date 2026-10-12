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

type fakeWorkspaceArchiveController struct {
	fakeContextSessionController
	changed []memory.Workspace
	err     error
}

func (c *fakeWorkspaceArchiveController) ArchiveWorkspace(_ context.Context, id memory.WorkspaceID) (memory.Workspace, error) {
	return c.change(id, memory.WorkspaceArchived)
}

func (c *fakeWorkspaceArchiveController) RestoreWorkspace(_ context.Context, id memory.WorkspaceID) (memory.Workspace, error) {
	return c.change(id, memory.WorkspaceActive)
}

func (c *fakeWorkspaceArchiveController) change(id memory.WorkspaceID, state memory.WorkspaceState) (memory.Workspace, error) {
	if c.err != nil {
		return memory.Workspace{}, c.err
	}
	workspace := memory.Workspace{ID: id, State: state}
	c.changed = append(c.changed, workspace)
	return workspace, nil
}

func TestWorkspaceArchiveClearsAConversationInsideItAndRestoreDoesNotSelect(t *testing.T) {
	controller := &fakeWorkspaceArchiveController{}
	server := NewContextServer(&agent.Session{}, nil, nil, controller)
	server.activeSession = memory.Session{ID: "session-1", WorkspaceID: "finance"}
	handler := server.Handler()
	archived := httptest.NewRecorder()
	handler.ServeHTTP(archived, managementRequest("/api/workspaces/archive", `{"workspaceId":"finance"}`))
	if archived.Code != http.StatusOK || !strings.Contains(archived.Body.String(), `"state":"archived"`) ||
		server.activeSession.ID != "" || server.session != nil || server.selectingSession {
		t.Fatalf("archive status=%d body=%s selected=%+v", archived.Code, archived.Body.String(), server.activeSession)
	}
	restored := httptest.NewRecorder()
	handler.ServeHTTP(restored, managementRequest("/api/workspaces/restore", `{"workspaceId":"finance"}`))
	if restored.Code != http.StatusOK || !strings.Contains(restored.Body.String(), `"state":"active"`) ||
		server.activeSession.ID != "" || len(controller.selected) != 0 {
		t.Fatalf("restore status=%d body=%s selected=%+v", restored.Code, restored.Body.String(), server.activeSession)
	}
}

func TestWorkspaceArchiveKeepsAConversationElsewhereAndHidesFailures(t *testing.T) {
	for _, failure := range []bool{false, true} {
		controller := &fakeWorkspaceArchiveController{}
		if failure {
			controller.err = errors.New("private SQLite path /secret/evie.db")
		}
		selectedAgent := &agent.Session{}
		server := NewContextServer(selectedAgent, nil, nil, controller)
		server.activeSession = memory.Session{ID: "session-1", WorkspaceID: "general"}
		result := httptest.NewRecorder()
		server.Handler().ServeHTTP(result, managementRequest("/api/workspaces/archive", `{"workspaceId":"finance"}`))
		want := http.StatusOK
		if failure {
			want = http.StatusInternalServerError
		}
		if result.Code != want || strings.Contains(result.Body.String(), "/secret/") ||
			server.activeSession.ID != "session-1" || server.session != selectedAgent || server.selectingSession {
			t.Fatalf("failure=%v status=%d body=%s active=%+v", failure, result.Code, result.Body.String(), server.activeSession)
		}
	}
}

func TestWorkspaceArchiveHTTPValidationAndBusyBoundaries(t *testing.T) {
	for _, path := range []string{"/api/workspaces/archive", "/api/workspaces/restore"} {
		for _, tc := range []struct {
			body      string
			busy      bool
			selecting bool
			cause     error
			status    int
		}{
			{body: `{}`, status: http.StatusBadRequest},
			{body: `{"workspaceId":" "}`, status: http.StatusBadRequest},
			{body: `{"workspaceId":"one","other":true}`, status: http.StatusBadRequest},
			{body: `{"workspaceId":"one"}`, busy: true, status: http.StatusConflict},
			{body: `{"workspaceId":"one"}`, selecting: true, status: http.StatusConflict},
			{body: `{"workspaceId":"one"}`, cause: eviedb.ErrWorkspaceNotFound, status: http.StatusNotFound},
		} {
			controller := &fakeWorkspaceArchiveController{err: tc.cause}
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
