package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
)

type fakeContextSessionController struct {
	snapshot    ContextSessionSnapshot
	registered  []string
	selected    []ContextSessionSelection
	session     *agent.Session
	registerErr error
	selectErr   error
}

func (f *fakeContextSessionController) Snapshot(context.Context) (ContextSessionSnapshot, error) {
	return f.snapshot, nil
}

func (f *fakeContextSessionController) RegisterWorkspace(_ context.Context, name string) (memory.Workspace, error) {
	f.registered = append(f.registered, name)
	if f.registerErr != nil {
		return memory.Workspace{}, f.registerErr
	}
	return memory.Workspace{ID: "workspace-1", DisplayName: name, State: memory.WorkspaceActive, CurrentRevisionID: "revision-1"}, nil
}

func (f *fakeContextSessionController) SelectSession(_ context.Context, selection ContextSessionSelection) (OpenedContextSession, error) {
	f.selected = append(f.selected, selection)
	if f.selectErr != nil {
		return OpenedContextSession{}, f.selectErr
	}
	return OpenedContextSession{Session: memory.Session{
		ID: "session-1", WorkspaceID: "workspace-1", WorkspaceRevisionSnapshot: "revision-1", Status: memory.SessionActive,
	}, Agent: f.session}, nil
}

func TestContextSessionHTTPDoesNotExposeControllerErrors(t *testing.T) {
	controller := &fakeContextSessionController{
		registerErr: errors.New("sqlite path /secret/evie.db"),
		selectErr:   errors.New("provider token secret-value"),
	}
	handler := NewContextServer(nil, nil, nil, controller).Handler()

	registered := httptest.NewRecorder()
	handler.ServeHTTP(registered, managementRequest("/api/workspaces/register", `{"displayName":"Cairo"}`))
	if registered.Code != http.StatusUnprocessableEntity || strings.Contains(registered.Body.String(), "/secret/") {
		t.Fatalf("registration status=%d body=%s", registered.Code, registered.Body.String())
	}

	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, managementRequest("/api/context-sessions/select", `{"unscoped":true}`))
	if selected.Code != http.StatusUnprocessableEntity || strings.Contains(selected.Body.String(), "secret-value") {
		t.Fatalf("selection status=%d body=%s", selected.Code, selected.Body.String())
	}
}

func TestContextSessionHTTPEncodesEmptyCollectionsAsArrays(t *testing.T) {
	handler := NewContextServer(nil, nil, nil, &fakeContextSessionController{}).Handler()

	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, managementRequest("/api/context-sessions/list", `{}`))

	if listed.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
	for _, collection := range []string{"workspaces", "projects", "sessions"} {
		want := `"` + collection + `":[]`
		if !strings.Contains(listed.Body.String(), want) {
			t.Errorf("list body=%s, want %s", listed.Body.String(), want)
		}
	}
}

func TestWorkspaceHTTPRegistrationExplicitSelectionChatAndResume(t *testing.T) {
	client := &fakeClient{steps: []fakeStep{{content: "inside Cairo"}}}
	controller := &fakeContextSessionController{
		snapshot: ContextSessionSnapshot{Workspaces: []memory.Workspace{{
			ID: "workspace-1", DisplayName: "Cairo's Kitchen", State: memory.WorkspaceActive, CurrentRevisionID: "revision-1",
		}}},
		session: agent.New(client, webTestContextProfile("test"), &fakeHistory{}, memory.ScopeContext{
			OwnerID: memory.LocalOwnerID, SessionID: "session-1", WorkspaceID: "workspace-1", WorkspaceRevision: "revision-1",
		}, webTestTurnOwner{}),
	}
	server := NewContextServer(nil, nil, nil, controller)
	handler := server.Handler()

	registered := httptest.NewRecorder()
	handler.ServeHTTP(registered, managementRequest("/api/workspaces/register", `{"displayName":"Cairo's Kitchen"}`))
	if registered.Code != http.StatusCreated || len(controller.registered) != 1 ||
		!strings.Contains(registered.Body.String(), `"currentRevisionId":"revision-1"`) {
		t.Fatalf("registration status=%d body=%s calls=%v", registered.Code, registered.Body.String(), controller.registered)
	}

	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, managementRequest("/api/context-sessions/select", `{"workspaceId":"workspace-1","workspaceRevision":"revision-1"}`))
	if selected.Code != http.StatusOK || len(controller.selected) != 1 ||
		controller.selected[0].WorkspaceID != "workspace-1" || controller.selected[0].WorkspaceRevision != "revision-1" ||
		!strings.Contains(selected.Body.String(), `"kind":"workspace"`) ||
		!strings.Contains(selected.Body.String(), `"displayName":"Cairo's Kitchen"`) {
		t.Fatalf("selection status=%d body=%s calls=%+v", selected.Code, selected.Body.String(), controller.selected)
	}

	chat := httptest.NewRecorder()
	handler.ServeHTTP(chat, chatRequest(`{"message":"hello"}`))
	if chat.Code != http.StatusOK || !strings.Contains(chat.Body.String(), "inside Cairo") {
		t.Fatalf("chat status=%d body=%s", chat.Code, chat.Body.String())
	}

	resumed := httptest.NewRecorder()
	handler.ServeHTTP(resumed, managementRequest("/api/context-sessions/select", `{"sessionId":"session-1"}`))
	if resumed.Code != http.StatusOK || len(controller.selected) != 2 || controller.selected[1].SessionID != "session-1" {
		t.Fatalf("resume status=%d body=%s calls=%+v", resumed.Code, resumed.Body.String(), controller.selected)
	}
}

func TestContextSessionHTTPRequiresExactlyOneExplicitScopeVariant(t *testing.T) {
	controller := &fakeContextSessionController{}
	handler := NewContextServer(nil, nil, nil, controller).Handler()
	for _, body := range []string{
		`{}`,
		`{"workspaceId":"workspace-1"}`,
		`{"workspaceId":"workspace-1","workspaceRevision":"revision-1","unscoped":true}`,
		`{"sessionId":"session-1","projectId":"project-1"}`,
	} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, managementRequest("/api/context-sessions/select", body))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d response=%s", body, recorder.Code, recorder.Body.String())
		}
	}
	if len(controller.selected) != 0 {
		t.Fatalf("invalid selections reached controller: %+v", controller.selected)
	}
}

func TestContextSessionSelectionWaitsForTheCurrentSessionTurnBoundary(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	client := &fakeClient{entered: entered, release: release, steps: []fakeStep{{content: "done"}}}
	controller := &fakeContextSessionController{session: agent.New(
		client, webTestContextProfile("test"), &fakeHistory{},
		memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "session-1"}, webTestTurnOwner{},
	)}
	server := NewContextServer(controller.session, nil, nil, controller)
	handler := server.Handler()
	chatDone := make(chan struct{})
	go func() {
		handler.ServeHTTP(httptest.NewRecorder(), chatRequest(`{"message":"hold"}`))
		close(chatDone)
	}()
	<-entered

	selection := httptest.NewRecorder()
	handler.ServeHTTP(selection, managementRequest("/api/context-sessions/select", `{"sessionId":"session-1"}`))
	if selection.Code != http.StatusConflict || len(controller.selected) != 0 {
		t.Fatalf("selection status=%d body=%s calls=%+v", selection.Code, selection.Body.String(), controller.selected)
	}
	close(release)
	<-chatDone
}
