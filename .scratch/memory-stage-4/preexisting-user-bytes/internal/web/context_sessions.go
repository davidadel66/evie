package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type ContextSessionSelection struct {
	SessionID         memory.SessionID           `json:"sessionId,omitempty"`
	WorkspaceID       memory.WorkspaceID         `json:"workspaceId,omitempty"`
	WorkspaceRevision memory.WorkspaceRevisionID `json:"workspaceRevision,omitempty"`
	ProjectID         memory.ProjectID           `json:"projectId,omitempty"`
	Unscoped          bool                       `json:"unscoped,omitempty"`
}

type ContextScopeKind string

const (
	ContextScopeWorkspace ContextScopeKind = "workspace"
	ContextScopeProject   ContextScopeKind = "project"
	ContextScopeUnscoped  ContextScopeKind = "unscoped"
)

type ContextScopeDescriptor struct {
	Kind              ContextScopeKind           `json:"kind"`
	DisplayName       string                     `json:"displayName"`
	WorkspaceID       memory.WorkspaceID         `json:"workspaceId,omitempty"`
	WorkspaceRevision memory.WorkspaceRevisionID `json:"workspaceRevision,omitempty"`
	ProjectID         memory.ProjectID           `json:"projectId,omitempty"`
	ProjectRoot       string                     `json:"projectRoot,omitempty"`
}

type ContextSessionSnapshot struct {
	Workspaces    []memory.Workspace      `json:"workspaces"`
	Projects      []memory.Project        `json:"projects"`
	Sessions      []memory.SessionListing `json:"sessions"`
	ActiveSession *memory.Session         `json:"activeSession,omitempty"`
	ActiveScope   *ContextScopeDescriptor `json:"activeScope,omitempty"`
}

type OpenedContextSession struct {
	Session memory.Session
	Agent   *agent.Session
}

type ContextSessionController interface {
	Snapshot(context.Context) (ContextSessionSnapshot, error)
	RegisterWorkspace(context.Context, string) (memory.Workspace, error)
	SelectSession(context.Context, ContextSessionSelection) (OpenedContextSession, error)
}

func (s *Server) handleContextSessionList(w http.ResponseWriter, r *http.Request) {
	if status, err := decodeManagementEmptyObject(w, r); err != nil {
		jsonError(w, status, "body must be one empty JSON object")
		return
	}
	snapshot, err := s.contextSessions.Snapshot(r.Context())
	if err != nil {
		managementJSONError(w, http.StatusInternalServerError, "context_sessions_unavailable", "Context Scope choices are unavailable")
		return
	}
	if snapshot.Workspaces == nil {
		snapshot.Workspaces = []memory.Workspace{}
	}
	if snapshot.Projects == nil {
		snapshot.Projects = []memory.Project{}
	}
	if snapshot.Sessions == nil {
		snapshot.Sessions = []memory.SessionListing{}
	}
	s.sessionMu.RLock()
	active := s.activeSession
	s.sessionMu.RUnlock()
	if active.ID != "" {
		snapshot.ActiveSession = &active
		descriptor := describeContextScope(active, snapshot)
		snapshot.ActiveScope = &descriptor
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleWorkspaceRegister(w http.ResponseWriter, r *http.Request) {
	var request struct {
		DisplayName string `json:"displayName"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil || strings.TrimSpace(request.DisplayName) == "" {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "body must be one JSON object with a nonblank displayName field")
		return
	}
	workspace, err := s.contextSessions.RegisterWorkspace(r.Context(), request.DisplayName)
	if err != nil {
		managementJSONError(w, http.StatusUnprocessableEntity, "workspace_registration_failed", "Workspace could not be registered")
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (s *Server) handleContextSessionSelect(w http.ResponseWriter, r *http.Request) {
	var selection ContextSessionSelection
	if status, err := decodeManagementJSON(w, r, &selection); err != nil {
		jsonError(w, status, "body must select exactly one existing session, Workspace, project, or unscoped session")
		return
	}
	if !validContextSessionSelection(selection) {
		jsonError(w, http.StatusBadRequest, "body must select exactly one existing session, Workspace, project, or unscoped session")
		return
	}
	s.sessionMu.Lock()
	if s.selectingSession || s.activeTurns > 0 {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "context_session_busy", "finish the current turn before selecting another Context Scope")
		return
	}
	s.selectingSession = true
	s.sessionMu.Unlock()
	defer func() {
		s.sessionMu.Lock()
		s.selectingSession = false
		s.sessionMu.Unlock()
	}()
	opened, err := s.contextSessions.SelectSession(r.Context(), selection)
	if err != nil {
		status := http.StatusUnprocessableEntity
		code := "context_session_selection_failed"
		message := "selected Context Scope could not be opened"
		if errors.Is(err, eviedb.ErrChooserStateChanged) || errors.Is(err, eviedb.ErrSessionNotActive) {
			status = http.StatusConflict
			code = "context_session_state_changed"
			message = "Context Scope choices changed; refresh and try again"
		}
		managementJSONError(w, status, code, message)
		return
	}
	if opened.Agent == nil || opened.Session.ID == "" {
		managementJSONError(w, http.StatusInternalServerError, "context_session_unavailable", "selected session could not be opened")
		return
	}
	snapshot, err := s.contextSessions.Snapshot(r.Context())
	if err != nil {
		managementJSONError(w, http.StatusInternalServerError, "context_sessions_unavailable", "selected Context Scope could not be displayed")
		return
	}
	descriptor := describeContextScope(opened.Session, snapshot)
	s.sessionMu.Lock()
	s.session = opened.Agent
	s.activeSession = opened.Session
	s.sessionMu.Unlock()
	writeJSON(w, http.StatusOK, struct {
		Session memory.Session         `json:"session"`
		Scope   ContextScopeDescriptor `json:"scope"`
	}{opened.Session, descriptor})
}

func validContextSessionSelection(selection ContextSessionSelection) bool {
	variants := 0
	if selection.SessionID != "" {
		variants++
	}
	if selection.WorkspaceID != "" || selection.WorkspaceRevision != "" {
		if selection.WorkspaceID == "" || selection.WorkspaceRevision == "" {
			return false
		}
		variants++
	}
	if selection.ProjectID != "" {
		variants++
	}
	if selection.Unscoped {
		variants++
	}
	return variants == 1
}

func describeContextScope(session memory.Session, snapshot ContextSessionSnapshot) ContextScopeDescriptor {
	if session.WorkspaceID != "" {
		descriptor := ContextScopeDescriptor{
			Kind: ContextScopeWorkspace, DisplayName: string(session.WorkspaceID), WorkspaceID: session.WorkspaceID,
			WorkspaceRevision: session.WorkspaceRevisionSnapshot,
		}
		for _, workspace := range snapshot.Workspaces {
			if workspace.ID == session.WorkspaceID {
				descriptor.DisplayName = memory.WorkspaceDisplayLabel(workspace.DisplayName, workspace.CreatedAt)
				break
			}
		}
		return descriptor
	}
	if session.ProjectID != "" {
		descriptor := ContextScopeDescriptor{
			Kind: ContextScopeProject, DisplayName: string(session.ProjectID), ProjectID: session.ProjectID, ProjectRoot: session.ProjectRootSnapshot,
		}
		for _, project := range snapshot.Projects {
			if project.ID == session.ProjectID {
				descriptor.DisplayName = memory.ProjectDisplayLabel(project.DisplayName, project.CreatedAt)
				break
			}
		}
		return descriptor
	}
	return ContextScopeDescriptor{Kind: ContextScopeUnscoped, DisplayName: "Unscoped"}
}
