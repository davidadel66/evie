package web

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type workspaceCreationController interface {
	RegisterWorkspaceWithOptions(context.Context, eviedb.WorkspaceRegistration) (memory.Workspace, error)
}

// workspaceCreationValidator checks that a registration's Agent Presets
// resolve, without creating anything.
type workspaceCreationValidator interface {
	ValidateWorkspaceRegistration(context.Context, eviedb.WorkspaceRegistration) error
}

// repositoryChoice is the Repository a new Workspace starts with
// (changes.spec.md story 2b); absent or "none" keeps the folder as given.
type repositoryChoice struct {
	Kind    changes.Choice `json:"kind"`
	Owner   string         `json:"owner,omitempty"`
	Name    string         `json:"name,omitempty"`
	Private bool           `json:"private,omitempty"`
}

func (s *Server) handleWorkspaceRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		eviedb.WorkspaceRegistration
		// AllowResearchDelegation shadows the embedded field, so an absent
		// value can be told apart from false: research is on for a new
		// Standard Workspace unless the request turns it off
		// (workspace-research.spec.md, amended 2026-11-02).
		AllowResearchDelegation *bool             `json:"allowResearchDelegation,omitempty"`
		Repository              *repositoryChoice `json:"repository,omitempty"`
	}
	if status, err := decodeManagementJSON(w, r, &body); err != nil || strings.TrimSpace(body.DisplayName) == "" {
		if err == nil {
			status = http.StatusBadRequest
		}
		jsonError(w, status, "body must be one JSON object with a nonblank displayName field")
		return
	}
	request := body.WorkspaceRegistration
	request.FolderPath = expandHome(strings.TrimSpace(request.FolderPath))
	if explicit := body.AllowResearchDelegation; explicit != nil {
		request.AllowResearchDelegation = *explicit
	} else {
		preset := strings.TrimSpace(request.PresetID)
		request.AllowResearchDelegation = preset == "" || preset == "standard"
	}
	// The Repository is set up first and the Workspace registered at its
	// folder after, so a failure never leaves a Workspace pointing at a
	// half-made folder.
	prepared := ""
	if choice := body.Repository; choice != nil && choice.Kind != "" && choice.Kind != "none" {
		switch {
		case choice.Kind != changes.NewLocal && choice.Kind != changes.NewGitHub && choice.Kind != changes.ExistingGitHub:
			managementJSONError(w, http.StatusBadRequest, "invalid_repository_choice", "the repository is none, local, github_new or github_existing")
			return
		case request.FolderPath == "" || request.CreateFolder:
			managementJSONError(w, http.StatusBadRequest, "invalid_repository_choice", "a repository needs a folder path, and makes the folder itself")
			return
		case s.repositories == nil:
			managementJSONError(w, http.StatusUnprocessableEntity, "repositories_unavailable", "repository choices are unavailable")
			return
		}
		// A Workspace refused for its presets must not leave a Repository
		// behind, so the presets are checked before the Repository is made.
		if validator, ok := s.contextSessions.(workspaceCreationValidator); ok {
			if err := validator.ValidateWorkspaceRegistration(r.Context(), request); err != nil {
				managementJSONError(w, http.StatusUnprocessableEntity, "workspace_registration_failed", "Workspace could not be created; check plugin settings")
				return
			}
		}
		repository, err := s.repositories.Prepare(r.Context(), changes.Prepare{Choice: choice.Kind, Folder: request.FolderPath, Owner: choice.Owner, Name: choice.Name, Private: choice.Private})
		if err != nil {
			status, code := repositoryError(err)
			managementJSONError(w, status, code, err.Error())
			return
		}
		request.FolderPath, prepared = repository.Folder, repository.Folder
	}
	var workspace memory.Workspace
	var err error
	if controller, ok := s.contextSessions.(workspaceCreationController); ok {
		workspace, err = controller.RegisterWorkspaceWithOptions(r.Context(), request)
	} else if request.PresetID == "" && request.FolderPath == "" && !request.CreateFolder && (body.AllowResearchDelegation == nil || !*body.AllowResearchDelegation) {
		// A controller without options creates a plain Workspace; only an
		// explicit research request is an option it cannot honor.
		workspace, err = s.contextSessions.RegisterWorkspace(r.Context(), request.DisplayName)
	} else {
		managementJSONError(w, http.StatusUnprocessableEntity, "workspace_options_unavailable", "Workspace creation options are unavailable")
		return
	}
	if err != nil {
		message := "Workspace could not be created; check the folder and plugin settings"
		if errors.Is(err, eviedb.ErrWorkspaceFolderInvalid) {
			message = eviedb.ErrWorkspaceFolderInvalid.Error()
		} else if errors.Is(err, eviedb.ErrWorkspaceCreationUncertain) {
			message = eviedb.ErrWorkspaceCreationUncertain.Error()
		}
		if prepared != "" {
			message = "The repository is ready at " + prepared + ", but the Workspace could not be created: " + message + ". Create the Workspace again with no repository and that folder."
		}
		managementJSONError(w, http.StatusUnprocessableEntity, "workspace_registration_failed", message)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

// expandHome turns ~ and ~/... into the owner's home folder.
func expandHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~"))
}
