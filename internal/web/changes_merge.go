package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// Merge and close (changes.spec.md story 5a): the owner merges or closes
// one of a Workspace's Changes by its ID.

// ChangeMerger merges and closes the Changes of the Repository at a
// Workspace's folder.
type ChangeMerger interface {
	Merge(ctx context.Context, ws memory.WorkspaceID, folder, id string) (changes.MergeResult, error)
	Close(ctx context.Context, ws memory.WorkspaceID, folder, id string) (changes.CloseResult, error)
	// Arm and Disarm set and clear "Merge when checks pass" (story 5c).
	Arm(ctx context.Context, ws memory.WorkspaceID, folder, id string) (changes.Change, error)
	Disarm(ctx context.Context, ws memory.WorkspaceID, folder, id string) (changes.Change, error)
}

// WithChangeMerger adds the merge and close routes.
func WithChangeMerger(server *Server, merger ChangeMerger) *Server {
	server.changeMerger = merger
	return server
}

func (s *Server) registerChangeMergeRoutes(mux *http.ServeMux) {
	if s.changeMerger == nil {
		return
	}
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if !ok {
		return
	}
	// change reads a request naming a Workspace and a Change, confirmed
	// when confirm is set, and answers with the Workspace's folder, or
	// writes the refusal.
	type changeRequest struct {
		WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		ChangeID    string             `json:"change_id"`
		Confirm     *bool              `json:"confirm,omitempty"`
		// WhenChecksPass sets "Merge when checks pass" instead of merging.
		WhenChecksPass bool `json:"when_checks_pass,omitempty"`
	}
	change := func(w http.ResponseWriter, r *http.Request, confirm bool) (memory.WorkspaceID, string, string, changeRequest, bool) {
		var request changeRequest
		if !decodeCandidateRequest(w, r, &request) {
			return "", "", "", request, false
		}
		if !boundedID(string(request.WorkspaceID)) || !boundedID(request.ChangeID) {
			managementJSONError(w, http.StatusBadRequest, "invalid_change_request", "this takes a Workspace ID and a Change ID")
			return "", "", "", request, false
		}
		if confirm && (request.Confirm == nil || !*request.Confirm) {
			managementJSONError(w, http.StatusBadRequest, "confirmation_required", "closing removes the Change's branch and worktree; confirm it")
			return "", "", "", request, false
		}
		folder, err := controller.WorkspaceFolder(r.Context(), request.WorkspaceID)
		if errors.Is(err, eviedb.ErrWorkspaceNotFound) {
			managementJSONError(w, http.StatusNotFound, "workspace_not_found", "no such Workspace")
			return "", "", "", request, false
		}
		if err != nil {
			managementJSONError(w, http.StatusInternalServerError, "repository_unavailable", "the Workspace folder could not be read")
			return "", "", "", request, false
		}
		return request.WorkspaceID, folder.Path, request.ChangeID, request, true
	}
	// answer writes a Change, or the refusal.
	answer := func(w http.ResponseWriter, c changes.Change, err error) {
		if err != nil {
			status, code := changeMergeError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
	mux.Handle("/api/changes/merge", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		ws, folder, id, request, ok := change(w, r, false)
		if !ok {
			return
		}
		if request.WhenChecksPass {
			armed, err := s.changeMerger.Arm(r.Context(), ws, folder, id)
			answer(w, armed, err)
			return
		}
		result, err := s.changeMerger.Merge(r.Context(), ws, folder, id)
		if errors.Is(err, changes.ErrNotMerged) {
			// When the gate ran, its runs say why nothing merged.
			refused := struct {
				Code   string               `json:"code"`
				Error  string               `json:"error"`
				Result *changes.MergeResult `json:"result,omitempty"`
			}{Code: "not_merged", Error: err.Error()}
			if result.Status != "" {
				refused.Result = &result
			}
			writeJSON(w, http.StatusConflict, refused)
			return
		}
		if err != nil {
			status, code := changeMergeError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	}))
	mux.Handle("/api/changes/close", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		ws, folder, id, _, ok := change(w, r, true)
		if !ok {
			return
		}
		closed, err := s.changeMerger.Close(r.Context(), ws, folder, id)
		if err != nil {
			status, code := changeMergeError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, closed)
	}))
	mux.Handle("/api/changes/disarm", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		ws, folder, id, _, ok := change(w, r, false)
		if !ok {
			return
		}
		disarmed, err := s.changeMerger.Disarm(r.Context(), ws, folder, id)
		answer(w, disarmed, err)
	}))
}

// changeMergeError maps a failed merge or close to a status.
func changeMergeError(err error) (int, string) {
	switch {
	case errors.Is(err, changes.ErrNotMerged):
		return http.StatusConflict, "not_merged"
	case errors.Is(err, changes.ErrBusy):
		return http.StatusConflict, "repository_busy"
	}
	return http.StatusInternalServerError, "merge_failed"
}
