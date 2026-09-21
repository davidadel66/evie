package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

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
