package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// A Workspace's Changes for the Changes tab (changes.spec.md story 3): its
// Repository and the Changes in it, read from the Workspace's current
// folder on every request.

// ChangeLister lists the Changes of a Workspace's Repository at its folder.
type ChangeLister interface {
	List(ctx context.Context, ws memory.WorkspaceID, folder string) (changes.Repository, []changes.Change, error)
}

// WithChanges adds the Change routes.
func WithChanges(server *Server, lister ChangeLister) *Server {
	server.changeLister = lister
	return server
}

func (s *Server) registerChangeRoutes(mux *http.ServeMux) {
	if s.changeLister == nil {
		return
	}
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if !ok {
		return
	}
	mux.Handle("/api/changes/list", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) {
			managementJSONError(w, http.StatusBadRequest, "invalid_changes_request", "list takes a Workspace ID")
			return
		}
		folder, err := controller.WorkspaceFolder(r.Context(), request.WorkspaceID)
		if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "no such Workspace")
			return
		}
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "changes_unavailable", "the Workspace folder could not be read")
			return
		}
		repository, list, err := s.changeLister.List(r.Context(), request.WorkspaceID, folder.Path)
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "changes_unavailable", "the Workspace's Changes could not be read")
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Repository changes.Repository `json:"repository"`
			Changes    []changes.Change   `json:"changes"`
		}{repository, list})
	}))
}
