package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type workspaceBuildController interface {
	SetWorkspaceBuild(context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID, memory.WorkspaceBuildSetting) (memory.Workspace, error)
}

// handleWorkspaceBuild records the owner's build setting for a Workspace
// (Stage 11, 2026-10-03): build workers write code in their own git
// worktrees, so they are off by default and "on" is the owner's explicit
// choice. Like the other permissions it stays available during a turn, so
// build can always be turned off.
func (s *Server) handleWorkspaceBuild(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID           `json:"workspaceId"`
		Revision    memory.WorkspaceRevisionID   `json:"revision"`
		Setting     memory.WorkspaceBuildSetting `json:"setting"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a Workspace, revision, and build setting")
		return
	}
	if request.WorkspaceID == "" || request.Revision == "" || !request.Setting.Valid() {
		jsonError(w, http.StatusBadRequest, `workspaceId, revision, and setting ("default", "on" or "off") are required`)
		return
	}
	workspace, err := s.contextSessions.(workspaceBuildController).SetWorkspaceBuild(r.Context(), request.WorkspaceID, request.Revision, request.Setting)
	if err != nil {
		status, message := http.StatusUnprocessableEntity, "Build permission could not be saved. Refresh and try again."
		switch {
		case errors.Is(err, eviedb.ErrChooserStateChanged) || errors.Is(err, eviedb.ErrWorkspaceNotFound):
			status, message = http.StatusConflict, "Workspace changed; refresh it before changing build permission."
		case errors.Is(err, eviedb.ErrWorkspaceBuildPreset):
			message = "Build workers need a Standard Workspace."
		}
		managementJSONError(w, status, "workspace_build_failed", message)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}
