package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type workspaceResearchController interface {
	SetWorkspaceResearch(context.Context, memory.WorkspaceID, memory.WorkspaceRevisionID, bool) (memory.Workspace, error)
}

func (s *Server) handleWorkspaceResearch(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID         `json:"workspaceId"`
		Revision    memory.WorkspaceRevisionID `json:"revision"`
		Enabled     *bool                      `json:"enabled"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a Workspace, revision, and research permission")
		return
	}
	if request.WorkspaceID == "" || request.Revision == "" || request.Enabled == nil {
		jsonError(w, http.StatusBadRequest, "workspaceId, revision, and enabled are required")
		return
	}
	// Permission revocation must remain available while a foreground turn runs.
	workspace, err := s.contextSessions.(workspaceResearchController).SetWorkspaceResearch(r.Context(), request.WorkspaceID, request.Revision, *request.Enabled)
	if err != nil {
		status, message := http.StatusUnprocessableEntity, "Research permission could not be saved. Check that the Web plugin and Research preset are available, then refresh and try again."
		if errors.Is(err, eviedb.ErrChooserStateChanged) || errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			status, message = http.StatusConflict, "Workspace changed; refresh it before changing research permission."
		}
		managementJSONError(w, status, "workspace_research_failed", message)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}
