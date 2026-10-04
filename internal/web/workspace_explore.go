package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type workspaceExploreController interface {
	SetWorkspaceExplore(context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID, memory.WorkspaceExploreSetting) (memory.Workspace, error)
}

// handleWorkspaceExplore records the owner's explore setting for a Workspace
// (2026-10-03): "default" follows the Workspace's folder, "on" and "off" are
// explicit. Like research permission, it stays available during a turn so
// explore can always be turned off.
func (s *Server) handleWorkspaceExplore(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID             `json:"workspaceId"`
		Revision    memory.WorkspaceRevisionID     `json:"revision"`
		Setting     memory.WorkspaceExploreSetting `json:"setting"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a Workspace, revision, and explore setting")
		return
	}
	if request.WorkspaceID == "" || request.Revision == "" || !request.Setting.Valid() {
		jsonError(w, http.StatusBadRequest, `workspaceId, revision, and setting ("default", "on" or "off") are required`)
		return
	}
	workspace, err := s.contextSessions.(workspaceExploreController).SetWorkspaceExplore(r.Context(), request.WorkspaceID, request.Revision, request.Setting)
	if err != nil {
		status, message := http.StatusUnprocessableEntity, "Explore permission could not be saved. Refresh and try again."
		switch {
		case errors.Is(err, eviedb.ErrChooserStateChanged) || errors.Is(err, eviedb.ErrWorkspaceNotFound):
			status, message = http.StatusConflict, "Workspace changed; refresh it before changing explore permission."
		case errors.Is(err, eviedb.ErrWorkspaceExplorePreset):
			message = "Explore workers need a Standard Workspace."
		}
		managementJSONError(w, status, "workspace_explore_failed", message)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}
