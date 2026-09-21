package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type RepositoryInstructionController interface {
	PreviewRepositoryInstructions(context.Context, memory.WorkspaceID) (memory.RepositoryInstructionSnapshot, error)
	SetRepositoryInstructionSettings(context.Context, memory.WorkspaceID, int64, bool) (memory.RepositoryInstructionSettings, error)
	RepositoryInstructionSnapshot(context.Context, memory.SessionID, memory.EventID) (memory.RepositoryInstructionSnapshot, error)
}
type repositoryInstructionRequest struct {
	WorkspaceID    memory.WorkspaceID `json:"workspaceId"`
	FolderRevision int64              `json:"folderRevision"`
	Revision       int64              `json:"revision"`
	Enabled        *bool              `json:"enabled"`
	SessionID      memory.SessionID   `json:"sessionId"`
	TurnID         memory.EventID     `json:"turnId"`
}

func (s *Server) registerRepositoryInstructionRoutes(mux *http.ServeMux) {
	controller, ok := s.contextSessions.(RepositoryInstructionController)
	if !ok {
		return
	}
	for _, action := range []string{"preview", "settings", "snapshot"} {
		mux.Handle("/api/repository-instructions/"+action, s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
			var req repositoryInstructionRequest
			if status, err := decodeManagementJSON(w, r, &req); err != nil || req.WorkspaceID == "" {
				if err == nil {
					status = 400
				}
				jsonError(w, status, "A Workspace is required")
				return
			}
			if action == "settings" {
				if req.Enabled == nil || req.Revision < 0 {
					jsonError(w, 400, "An instruction setting and revision are required")
					return
				}
				s.sessionMu.Lock()
				defer s.sessionMu.Unlock()
				if s.activeTurns > 0 || s.selectingSession {
					jsonError(w, 409, "Finish the current turn before changing repository instructions")
					return
				}
				result, err := controller.SetRepositoryInstructionSettings(r.Context(), req.WorkspaceID, req.Revision, *req.Enabled)
				if err != nil {
					if errors.Is(err, eviedb.ErrRepositoryInstructionsChanged) {
						jsonError(w, 409, err.Error())
					} else {
						jsonError(w, 422, "Repository instructions could not be saved")
					}
					return
				}
				writeJSON(w, 200, result)
				return
			}
			s.sessionMu.RLock()
			defer s.sessionMu.RUnlock()
			if action == "snapshot" {
				if req.SessionID == "" || req.TurnID == "" {
					jsonError(w, 400, "A conversation and turn are required")
					return
				}
				if req.SessionID != s.activeSession.ID || req.WorkspaceID != s.activeSession.WorkspaceID {
					jsonError(w, 409, "Conversation changed; reopen the instruction details")
					return
				}
				result, err := controller.RepositoryInstructionSnapshot(r.Context(), req.SessionID, req.TurnID)
				if err != nil {
					if errors.Is(err, sql.ErrNoRows) {
						jsonError(w, 404, "No repository instruction snapshot was recorded for this turn.")
					} else {
						jsonError(w, 500, "Instruction snapshot is unavailable")
					}
					return
				}
				if result.WorkspaceID != req.WorkspaceID {
					jsonError(w, 404, "Instruction snapshot is unavailable")
					return
				}
				writeJSON(w, 200, result)
				return
			}
			result, err := controller.PreviewRepositoryInstructions(r.Context(), req.WorkspaceID)
			if err != nil {
				jsonError(w, 404, "Workspace instructions are unavailable")
				return
			}
			if result.Folder.Revision != req.FolderRevision {
				jsonError(w, 409, "Workspace folder changed; reopen the instructions")
				return
			}
			writeJSON(w, 200, result)
		}))
	}
}
