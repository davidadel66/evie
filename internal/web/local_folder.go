package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localfiles"
	"github.com/davidadel66/evie/internal/memory"
)

type WorkspaceFolderController interface {
	WorkspaceFolder(context.Context, memory.WorkspaceID) (memory.WorkspaceFolder, error)
	SetWorkspaceFolder(context.Context, memory.WorkspaceID, int64, string) (memory.WorkspaceFolder, error)
}

type folderRequest struct {
	WorkspaceID memory.WorkspaceID `json:"workspaceId"`
	Revision    int64              `json:"revision"`
	Path        string             `json:"path"`
	Query       string             `json:"query"`
	View        string             `json:"view"`
	Base        string             `json:"base"`
}

func (s *Server) registerFolderRoutes(mux *http.ServeMux) {
	controller, ok := s.contextSessions.(WorkspaceFolderController)
	if !ok {
		return
	}
	mux.Handle("/api/workspaces/folder", s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
		var req folderRequest
		if status, err := decodeManagementJSON(w, r, &req); err != nil || req.WorkspaceID == "" {
			if err == nil {
				status = 400
			}
			jsonError(w, status, "A Workspace and folder path are required")
			return
		}
		s.sessionMu.Lock()
		defer s.sessionMu.Unlock()
		if s.activeTurns > 0 || s.selectingSession {
			jsonError(w, 409, "Finish the current turn before changing folders")
			return
		}
		folder, err := controller.SetWorkspaceFolder(r.Context(), req.WorkspaceID, req.Revision, req.Path)
		if err != nil {
			if errors.Is(err, eviedb.ErrWorkspaceFolderChanged) {
				jsonError(w, 409, err.Error())
			} else {
				jsonError(w, 422, "The folder could not be attached. Choose an existing absolute folder path.")
			}
			return
		}
		s.stopWorkspaceTerminals(req.WorkspaceID)
		writeJSON(w, 200, folder)
	}))
	for _, action := range []string{"list", "read", "changes", "diff"} {
		mux.Handle("/api/folder/"+action, s.managementRoute(func(w http.ResponseWriter, r *http.Request) {
			var req folderRequest
			if status, err := decodeManagementJSON(w, r, &req); err != nil || req.WorkspaceID == "" {
				if err == nil {
					status = 400
				}
				jsonError(w, status, "A Workspace is required")
				return
			}
			// Hold the selection lock through the read so a replacement cannot race
			// this request into a different folder under an old tab identity.
			s.sessionMu.RLock()
			defer s.sessionMu.RUnlock()
			folder, err := controller.WorkspaceFolder(r.Context(), req.WorkspaceID)
			if err != nil {
				jsonError(w, 404, "Workspace folder is unavailable")
				return
			}
			if folder.Revision != req.Revision {
				jsonError(w, 409, "Workspace folder changed. Reopen Files.")
				return
			}
			if folder.Path == "" {
				jsonError(w, 422, "Attach a folder in this Workspace to browse files")
				return
			}
			var result any
			switch action {
			case "list":
				result, err = localfiles.List(r.Context(), folder.Path, req.Path, req.Query)
			case "read":
				result, err = localfiles.Read(folder.Path, req.Path)
			case "changes":
				result, err = localfiles.Review(r.Context(), folder.Path, req.View, req.Base)
			case "diff":
				result, err = localfiles.Diff(r.Context(), folder.Path, req.Path, req.View, req.Base)
			}
			if err != nil {
				jsonError(w, 422, err.Error())
				return
			}
			writeJSON(w, 200, result)
		}))
	}
}
