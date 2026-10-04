package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type exploreWorkspaceController struct {
	fakeContextSessionController
	calls    int
	id       memory.WorkspaceID
	revision memory.WorkspaceRevisionID
	setting  memory.WorkspaceExploreSetting
	err      error
}

func (c *exploreWorkspaceController) SetWorkspaceExplore(_ context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID, setting memory.WorkspaceExploreSetting) (memory.Workspace, error) {
	c.calls++
	c.id, c.revision, c.setting = id, revision, setting
	return memory.Workspace{ID: id, CurrentRevisionID: "next", Explore: memory.WorkspaceExplore{Setting: setting, Allowed: setting == memory.WorkspaceExploreOn}}, c.err
}

// The explore toggle is a guarded management route: POST with the exact
// JSON content type from a loopback origin, an explicit setting and the
// revision the owner saw. Turning explore off stays available mid-turn.
func TestWorkspaceExploreHTTPSetsTheSettingBehindTheManagementGuard(t *testing.T) {
	c := &exploreWorkspaceController{}
	s := NewContextServer(nil, nil, nil, c)
	s.activeTurns = 1
	handler := s.Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"off"}`))
	if w.Code != http.StatusOK || c.id != "workspace" || c.revision != "revision" || c.setting != memory.WorkspaceExploreOff || !strings.Contains(w.Body.String(), `"explore":{"setting":"off","allowed":false}`) {
		t.Fatalf("explicit off=%d %s", w.Code, w.Body)
	}
	for _, setting := range []memory.WorkspaceExploreSetting{memory.WorkspaceExploreOn, memory.WorkspaceExploreDefault} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"`+string(setting)+`"}`))
		if w.Code != http.StatusOK || c.setting != setting {
			t.Fatalf("setting %q=%d %s", setting, w.Code, w.Body)
		}
	}

	calls := c.calls
	for _, body := range []string{
		`{"workspaceId":"workspace","revision":"revision"}`,
		`{"workspaceId":"workspace","setting":"on"}`,
		`{"revision":"revision","setting":"on"}`,
		`{"workspaceId":"workspace","revision":"revision","setting":"sometimes"}`,
		`{"workspaceId":"workspace","revision":"revision","enabled":true}`,
	} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/explore", body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s=%d %s", body, w.Code, w.Body)
		}
	}
	// The guard refuses anything a foreign page or form could send.
	get := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6687/api/workspaces/explore", nil)
	form := managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`)
	form.Header.Set("Content-Type", "text/plain")
	foreign := managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`)
	foreign.Header.Set("Origin", "https://evil.example")
	rebound := managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`)
	rebound.Host = "evil.example:6687"
	for _, tc := range []struct {
		request *http.Request
		status  int
	}{{get, http.StatusMethodNotAllowed}, {form, http.StatusForbidden}, {foreign, http.StatusForbidden}, {rebound, http.StatusForbidden}} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, tc.request)
		if w.Code != tc.status {
			t.Fatalf("%s %s=%d, want %d", tc.request.Method, tc.request.Header.Get("Origin"), w.Code, tc.status)
		}
	}
	if c.calls != calls {
		t.Fatal("a refused request reached the controller")
	}

	for _, tc := range []struct {
		err    error
		status int
		text   string
	}{
		{eviedb.ErrChooserStateChanged, http.StatusConflict, "refresh"},
		{eviedb.ErrWorkspaceNotFound, http.StatusConflict, "refresh"},
		{eviedb.ErrWorkspaceExplorePreset, http.StatusUnprocessableEntity, "Standard"},
		{errors.New("secret private path"), http.StatusUnprocessableEntity, "could not be saved"},
	} {
		c.err = tc.err
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.text) || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("error %v=%d %s", tc.err, w.Code, w.Body)
		}
	}
}

// A context controller without the explore setting has no explore route.
func TestWorkspaceExploreHTTPNeedsAController(t *testing.T) {
	handler := NewContextServer(nil, nil, nil, &fakeContextSessionController{}).Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/explore", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`))
	if w.Code == http.StatusOK {
		t.Fatalf("explore route without a controller=%d %s", w.Code, w.Body)
	}
}
