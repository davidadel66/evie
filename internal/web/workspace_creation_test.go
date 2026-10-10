package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type optionsWorkspaceController struct {
	fakeContextSessionController
	options eviedb.WorkspaceRegistration
	err     error
}

func (c *optionsWorkspaceController) RegisterWorkspaceWithOptions(_ context.Context, options eviedb.WorkspaceRegistration) (memory.Workspace, error) {
	c.options = options
	return memory.Workspace{ID: "created", DefaultPresetID: options.PresetID, AllowedPresetIDs: []string{options.PresetID}}, c.err
}

func TestWorkspaceCreationHTTPPassesExplicitOptionsAndSanitizesErrors(t *testing.T) {
	controller := &optionsWorkspaceController{}
	handler := NewContextServer(nil, nil, nil, controller).Handler()
	body := `{"displayName":"Learning","presetId":"research","folderPath":"/tmp/learning","createFolder":true}`
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, managementRequest("/api/workspaces/register", body))
	want := eviedb.WorkspaceRegistration{DisplayName: "Learning", PresetID: "research", FolderPath: "/tmp/learning", CreateFolder: true}
	if response.Code != http.StatusCreated || !reflect.DeepEqual(controller.options, want) {
		t.Fatalf("registration = %d %s %+v", response.Code, response.Body.String(), controller.options)
	}
	controller.err = errors.New("private provider token secret-key /private/db")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, managementRequest("/api/workspaces/register", body))
	if response.Code != http.StatusUnprocessableEntity || strings.Contains(response.Body.String(), "secret-key") || strings.Contains(response.Body.String(), "/private") {
		t.Fatalf("unsafe error = %d %s", response.Code, response.Body.String())
	}
}

func TestWorkspaceCreationHTTPLegacyControllerRejectsUnsupportedOptions(t *testing.T) {
	controller := &fakeContextSessionController{}
	handler := NewContextServer(nil, nil, nil, controller).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, managementRequest("/api/workspaces/register", `{"displayName":"Research","presetId":"research"}`))
	if response.Code != http.StatusUnprocessableEntity || len(controller.registered) != 0 {
		t.Fatalf("ignored options = %d %s", response.Code, response.Body.String())
	}
}

// prepareRepositories records the Repository a creation asked for and
// answers it as told.
type prepareRepositories struct {
	stubRepositories
	prepared []changes.Prepare
	folder   string
	err      error
}

func (p *prepareRepositories) Prepare(_ context.Context, request changes.Prepare) (changes.Repository, error) {
	p.prepared = append(p.prepared, request)
	if p.err != nil {
		return changes.Repository{}, p.err
	}
	return changes.Repository{Kind: changes.GitHub, Folder: p.folder}, nil
}

// changes.spec.md story 2b: the Repository is set up before the Workspace
// is registered, at the folder the setup returns; a failed setup registers
// nothing.
func TestWorkspaceCreationSetsUpTheRepositoryFirst(t *testing.T) {
	controller := &optionsWorkspaceController{}
	repositories := &prepareRepositories{folder: "/Users/me/code/notes"}
	handler := WithRepositories(NewContextServer(nil, nil, nil, controller), repositories).Handler()
	register := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, managementRequest("/api/workspaces/register", body))
		return response
	}
	home, _ := os.UserHomeDir()
	res := register(`{"displayName":"notes","presetId":"standard","folderPath":"~/code/notes","repository":{"kind":"github_new","name":"notes","private":true}}`)
	if res.Code != http.StatusCreated || len(repositories.prepared) != 1 {
		t.Fatalf("register = %d %s", res.Code, res.Body)
	}
	if got := repositories.prepared[0]; got.Choice != changes.NewGitHub || got.Folder != filepath.Join(home, "code", "notes") || got.Name != "notes" || !got.Private {
		t.Fatalf("prepared %+v", got)
	}
	if controller.options.FolderPath != "/Users/me/code/notes" || controller.options.CreateFolder {
		t.Fatalf("registered %+v", controller.options)
	}

	controller.options = eviedb.WorkspaceRegistration{}
	repositories.err = fmt.Errorf("%w: /Users/me/code/notes already exists; choose a new folder", changes.ErrFolderUnavailable)
	res = register(`{"displayName":"notes","folderPath":"/Users/me/code/notes","repository":{"kind":"local"}}`)
	if res.Code != http.StatusConflict || !strings.Contains(res.Body.String(), "already exists") || controller.options.DisplayName != "" {
		t.Fatalf("failed setup = %d %s, registered %+v", res.Code, res.Body, controller.options)
	}
	for _, body := range []string{
		`{"displayName":"notes","folderPath":"/x","createFolder":true,"repository":{"kind":"local"}}`,
		`{"displayName":"notes","repository":{"kind":"local"}}`,
		`{"displayName":"notes","folderPath":"/x","repository":{"kind":"svn"}}`,
	} {
		if res := register(body); res.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", body, res.Code, res.Body)
		}
	}
	// No repository choice keeps today's behavior.
	repositories.prepared = nil
	if res := register(`{"displayName":"plain","folderPath":"/tmp"}`); res.Code != http.StatusCreated || len(repositories.prepared) != 0 || controller.options.FolderPath != "/tmp" {
		t.Fatalf("plain = %d %s", res.Code, res.Body)
	}
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, managementRequest("/api/github/repositories", `{}`))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"full_name":"davidadel66/evie"`) {
		t.Fatalf("repositories = %d %s", list.Code, list.Body)
	}
}
