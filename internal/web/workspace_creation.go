package web

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type workspaceCreationController interface {
	RegisterWorkspaceWithOptions(context.Context, eviedb.WorkspaceRegistration) (memory.Workspace, error)
}

func (s *Server) handleWorkspaceRegister(w http.ResponseWriter, r *http.Request) {
	var request eviedb.WorkspaceRegistration
	if status, err := decodeManagementJSON(w, r, &request); err != nil || strings.TrimSpace(request.DisplayName) == "" {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "body must be one JSON object with a nonblank displayName field")
		return
	}
	var workspace memory.Workspace
	var err error
	if controller, ok := s.contextSessions.(workspaceCreationController); ok {
		workspace, err = controller.RegisterWorkspaceWithOptions(r.Context(), request)
	} else if request.PresetID == "" && request.FolderPath == "" && !request.CreateFolder {
		workspace, err = s.contextSessions.RegisterWorkspace(r.Context(), request.DisplayName)
	} else {
		managementJSONError(w, http.StatusUnprocessableEntity, "workspace_options_unavailable", "Workspace creation options are unavailable")
		return
	}
	if err != nil {
		message := "Workspace could not be created; check the Agent Preset and folder"
		if errors.Is(err, eviedb.ErrWorkspaceFolderInvalid) {
			message = eviedb.ErrWorkspaceFolderInvalid.Error()
		} else if errors.Is(err, eviedb.ErrWorkspaceCreationUncertain) {
			message = eviedb.ErrWorkspaceCreationUncertain.Error()
		}
		managementJSONError(w, http.StatusUnprocessableEntity, "workspace_registration_failed", message)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}
