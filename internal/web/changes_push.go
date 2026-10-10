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

// Push (changes.spec.md story 4a): the owner pushes one of Evie's draft
// Changes in a GitHub Repository and opens its pull request.

// ChangePusher pushes Evie's draft Change on a branch of the Repository at
// a Workspace's folder and opens its pull request.
type ChangePusher interface {
	Push(ctx context.Context, ws memory.WorkspaceID, folder, branch string) (changes.PushResult, error)
}

// WithChangePusher adds the push route.
func WithChangePusher(server *Server, pusher ChangePusher) *Server {
	server.changePusher = pusher
	return server
}

func (s *Server) registerChangePushRoutes(mux *http.ServeMux) {
	if s.changePusher == nil {
		return
	}
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if !ok {
		return
	}
	mux.Handle("/api/changes/push", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			WorkspaceID memory.WorkspaceID `json:"workspace_id"`
			Branch      string             `json:"branch"`
		}
		if !decodeCandidateRequest(w, r, &request) {
			return
		}
		if !boundedID(string(request.WorkspaceID)) || !boundedID(request.Branch) {
			managementJSONError(w, http.StatusBadRequest, "invalid_push_request", "push takes a Workspace ID and a branch")
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
		result, err := s.changePusher.Push(r.Context(), request.WorkspaceID, folder.Path, request.Branch)
		if err != nil {
			status, code := changePushError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
}

// changePushError maps a failed push to a status; its message says which
// step failed and never carries the token.
func changePushError(err error) (int, string) {
	switch {
	case errors.Is(err, changes.ErrNotPushable):
		return http.StatusConflict, "not_pushable"
	case errors.Is(err, changes.ErrNoToken), errors.Is(err, github.ErrUnauthorized):
		return http.StatusConflict, "github_not_connected"
	case errors.Is(err, github.ErrNotFound):
		return http.StatusConflict, "github_no_access"
	case errors.Is(err, changes.ErrBusy):
		return http.StatusConflict, "repository_busy"
	case errors.Is(err, github.ErrUnprocessable), errors.Is(err, github.ErrForbidden):
		return http.StatusUnprocessableEntity, "github_refused"
	}
	return http.StatusBadGateway, "push_failed"
}
