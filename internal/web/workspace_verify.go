package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/worktree"
)

type workspaceVerifyController interface {
	SetWorkspaceVerify(ctx context.Context, id memory.WorkspaceID, expected int64, setup, verify string) (memory.Workspace, error)
	ProposeWorkspaceVerify(ctx context.Context, id memory.WorkspaceID) (worktree.Proposal, error)
}

// handleWorkspaceVerify records the owner's setup and verify commands for a
// Workspace (Stage 12, 2026-10-04): setup runs with network in each fresh
// build worktree, verify gates every merge proposal. Only this guarded route
// and the CLI set them; the revision the owner saw must still be the latest.
func (s *Server) handleWorkspaceVerify(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspaceId"`
		Revision    *int64             `json:"revision"`
		Setup       string             `json:"setup"`
		Verify      string             `json:"verify"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil {
		jsonError(w, status, "body must identify a Workspace and revision, with its setup and verify commands")
		return
	}
	if request.WorkspaceID == "" || request.Revision == nil || *request.Revision < 0 {
		jsonError(w, http.StatusBadRequest, "workspaceId and revision (the commands' revision you saw, 0 when none is set) are required")
		return
	}
	workspace, err := s.contextSessions.(workspaceVerifyController).SetWorkspaceVerify(r.Context(), request.WorkspaceID, *request.Revision, request.Setup, request.Verify)
	if err != nil {
		status, message := http.StatusUnprocessableEntity, "The commands could not be saved. Refresh and try again."
		switch {
		case errors.Is(err, eviedb.ErrBuildVerifyChanged) || errors.Is(err, eviedb.ErrWorkspaceNotFound):
			status, message = http.StatusConflict, "The Workspace changed; refresh it before changing its commands."
		case errors.Is(err, eviedb.ErrBuildVerifyCommand):
			message = eviedb.ErrBuildVerifyCommand.Error()
		}
		managementJSONError(w, status, "workspace_verify_failed", message)
		return
	}
	writeJSON(w, http.StatusOK, workspace)
}

// handleWorkspaceVerifyPropose returns the setup and verify commands Evie
// finds in the Workspace folder's repository instructions. It records
// nothing: the owner decides whether to save them.
func (s *Server) handleWorkspaceVerifyPropose(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspaceId"`
	}
	if status, err := decodeManagementJSON(w, r, &request); err != nil || request.WorkspaceID == "" {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "body must identify a Workspace")
		return
	}
	proposal, err := s.contextSessions.(workspaceVerifyController).ProposeWorkspaceVerify(r.Context(), request.WorkspaceID)
	if err != nil {
		managementJSONError(w, http.StatusUnprocessableEntity, "workspace_verify_propose_failed", "The repository's instructions could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}
