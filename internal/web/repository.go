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

// A Workspace's Repository for Settings → Repository (changes.spec.md
// story 1): read from the Workspace's current folder on every request,
// never stored.

// Repositories reads and publishes the Repository at a Workspace folder,
// and checks the GitHub token.
type Repositories interface {
	Repository(ctx context.Context, folder string) (changes.Repository, error)
	Publish(context.Context, changes.Publish) (changes.Repository, error)
	Account(context.Context) changes.GitHubStatus
}

// WithRepositories adds the Repository routes.
func WithRepositories(server *Server, repositories Repositories) *Server {
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
	// Publish to GitHub (story 2a): a local Repository becomes a new
	// repository on the token owner's account, with its base branch pushed.
	mux.Handle("/api/workspaces/repository/publish", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
			Name        string             `json:"name"`
			Private     bool               `json:"private"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) || request.Name == "" || len(request.Name) > 100 {
			managementJSONError(w, http.StatusBadRequest, "invalid_repository_request", "publish takes a Workspace ID, a repository name and whether it is private")
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
		repository, err := s.repositories.Publish(r.Context(), changes.Publish{Folder: folder.Path, Name: request.Name, Private: request.Private})
		if err != nil {
			status, code := publishError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, repository)
	}))
	// The token check alone, for the forms that create repositories.
	mux.Handle("/api/github/status", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		if status, err := decodeManagementEmptyObject(w, r); err != nil {
			managementJSONError(w, status, "invalid_github_request", "status takes an empty object")
			return
		}
		writeJSON(w, http.StatusOK, s.repositories.Account(r.Context()))
	}))
}

// publishError maps a refused or failed publish to a status; every message
// it carries is already free of the token.
func publishError(err error) (int, string) {
	switch {
	case errors.Is(err, github.ErrInvalidName):
		return http.StatusBadRequest, "invalid_repository_name"
	case errors.Is(err, changes.ErrNoToken), errors.Is(err, github.ErrUnauthorized):
		return http.StatusConflict, "github_not_connected"
	case errors.Is(err, changes.ErrNotPublishable):
		return http.StatusConflict, "not_publishable"
	case errors.Is(err, changes.ErrBusy):
		return http.StatusConflict, "repository_busy"
	case errors.Is(err, github.ErrUnprocessable), errors.Is(err, github.ErrForbidden):
		return http.StatusUnprocessableEntity, "github_refused"
	case errors.Is(err, changes.ErrPushFailed):
		return http.StatusBadGateway, "push_failed"
	}
	return http.StatusBadGateway, "publish_failed"
}
