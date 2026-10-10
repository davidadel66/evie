package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// A Workspace's Repository for Settings → Repository (changes.spec.md
// story 1): read from the Workspace's current folder on every request,
// never stored.

// RepositoryReader reads the Repository at a Workspace folder.
type RepositoryReader interface {
	Repository(ctx context.Context, folder string) (changes.Repository, error)
}

// WithRepositories adds the Repository route.
func WithRepositories(server *Server, repositories RepositoryReader) *Server {
	server.repositories = repositories
	return server
}

func (s *Server) registerRepositoryRoutes(mux *http.ServeMux) {
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if s.repositories == nil || !ok {
		return
	}
	mux.Handle("/api/workspaces/repository", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) {
			managementJSONError(w, http.StatusBadRequest, "invalid_repository_request", "repository takes a Workspace ID")
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
		repository, err := s.repositories.Repository(r.Context(), folder.Path)
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "repository_unavailable", "git could not read the Workspace folder")
			return
		}
		writeJSON(w, http.StatusOK, repository)
	}))
}
