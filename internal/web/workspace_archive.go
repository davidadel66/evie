package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// workspaceArchiveController archives and restores Workspaces. Archive is
// the owner's "delete": it stops new sessions and runs and keeps sessions,
// memory and history (workspaces.spec.md keeps hard erasure separate).
type workspaceArchiveController interface {
	ArchiveWorkspace(context.Context, memory.WorkspaceID) (memory.Workspace, error)
	RestoreWorkspace(context.Context, memory.WorkspaceID) (memory.Workspace, error)
}

func (s *Server) handleWorkspaceArchive(w http.ResponseWriter, r *http.Request) {
	s.handleWorkspaceLifecycle(w, r, true)
}

func (s *Server) handleWorkspaceRestore(w http.ResponseWriter, r *http.Request) {
	s.handleWorkspaceLifecycle(w, r, false)
}

func (s *Server) handleWorkspaceLifecycle(w http.ResponseWriter, r *http.Request, archived bool) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspaceId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil || strings.TrimSpace(string(request.WorkspaceID)) == "" {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "body must be one JSON object with a nonblank workspaceId field")
		return
	}
	controller := s.contextSessions.(workspaceArchiveController)
	s.sessionMu.Lock()
	if s.selectingSession || s.workspaceBusyLocked(request.WorkspaceID) {
		s.sessionMu.Unlock()
		managementJSONError(w, http.StatusConflict, "context_session_busy", "finish the current turn before changing a workspace")
		return
	}
	s.selectingSession = true
	release := s.holdArchivingLocked(func(chat *openChat) bool { return chat.session.WorkspaceID == request.WorkspaceID })
	s.sessionMu.Unlock()
	defer func() {
		s.sessionMu.Lock()
		s.selectingSession = false
		release()
		s.sessionMu.Unlock()
	}()
	change := controller.RestoreWorkspace
	if archived {
		change = controller.ArchiveWorkspace
	}
	workspace, err := change(r.Context(), request.WorkspaceID)
	if err != nil {
		if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "Workspace not found; refresh and try again")
			return
		}
		managementJSONError(w, http.StatusInternalServerError, "workspace_archive_failed", "Workspace archival could not be changed")
		return
	}
	// Its open chats leave with an archived Workspace, as a chat does when it
	// is archived itself.
	if archived {
		s.sessionMu.Lock()
		s.closeWorkspaceLocked(request.WorkspaceID)
		s.sessionMu.Unlock()
	}
	writeJSON(w, http.StatusOK, struct {
		Workspace memory.Workspace `json:"workspace"`
	}{workspace})
}
