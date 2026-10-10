package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// Refresh (changes.spec.md story 3b): opening the Changes tab asks for an
// immediate sync of the Workspace's pull requests from GitHub.

// ChangeSync syncs the pull requests of the Repository at a Workspace's
// folder now.
type ChangeSync interface {
	Refresh(ctx context.Context, ws memory.WorkspaceID, folder string) (changes.SyncResult, error)
}

// WithChangeSync adds the refresh route.
func WithChangeSync(server *Server, sync ChangeSync) *Server {
	server.changeSync = sync
	return server
}

func (s *Server) registerChangeSyncRoutes(mux *http.ServeMux) {
	if s.changeSync == nil {
		return
	}
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if !ok {
		return
	}
	mux.Handle("/api/changes/refresh", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) {
			managementJSONError(w, http.StatusBadRequest, "invalid_refresh_request", "refresh takes a Workspace ID")
			return
		}
		folder, err := controller.WorkspaceFolder(r.Context(), request.WorkspaceID)
		if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "no such Workspace")
			return
		}
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "repository_unavailable", "the Workspace folder could not be read")
			return
		}
		result, err := s.changeSync.Refresh(r.Context(), request.WorkspaceID, folder.Path)
		if err != nil {
			status, code := changeSyncError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
}

// changeSyncError maps a failed sync to a status; its message names the
// GitHub request that was refused and never carries the token.
func changeSyncError(err error) (int, string) {
	switch {
	case errors.Is(err, changes.ErrNoToken), errors.Is(err, github.ErrUnauthorized):
		return http.StatusConflict, "github_not_connected"
	case errors.Is(err, github.ErrNotFound):
		return http.StatusConflict, "github_no_access"
	case errors.Is(err, github.ErrForbidden), errors.Is(err, github.ErrUnprocessable):
		return http.StatusUnprocessableEntity, "github_refused"
	}
	return http.StatusBadGateway, "sync_failed"
}
