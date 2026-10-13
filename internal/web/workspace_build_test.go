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

type buildWorkspaceController struct {
	fakeContextSessionController
	calls    int
	id       memory.WorkspaceID
	revision memory.WorkspaceRevisionID
	setting  memory.WorkspaceBuildSetting
	err      error
}

func (c *buildWorkspaceController) SetWorkspaceBuild(_ context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID, setting memory.WorkspaceBuildSetting) (memory.Workspace, error) {
	c.calls++
	c.id, c.revision, c.setting = id, revision, setting
	return memory.Workspace{ID: id, CurrentRevisionID: "next", Build: memory.WorkspaceBuild{Setting: setting, Allowed: setting == memory.WorkspaceBuildOn}}, c.err
}

// Stage 11: the build permission is a guarded management route like the
// explore setting: POST with the exact JSON content type from a loopback
// origin, an explicit setting and the revision the owner saw. Turning build
// off stays available mid-turn; errors never echo internal text.
func TestWorkspaceBuildHTTPSetsTheSettingBehindTheManagementGuard(t *testing.T) {
	c := &buildWorkspaceController{}
	s := NewContextServer(nil, nil, nil, c)
	runningChat(s, "chat", "workspace")
	handler := s.Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/build", `{"workspaceId":"workspace","revision":"revision","setting":"off"}`))
	if w.Code != http.StatusOK || c.id != "workspace" || c.revision != "revision" || c.setting != memory.WorkspaceBuildOff || !strings.Contains(w.Body.String(), `"build":{"setting":"off","allowed":false}`) {
		t.Fatalf("explicit off=%d %s", w.Code, w.Body)
	}
	calls := c.calls
	for _, body := range []string{
		`{"workspaceId":"workspace","revision":"revision"}`,
		`{"workspaceId":"workspace","setting":"on"}`,
		`{"workspaceId":"workspace","revision":"revision","setting":"always"}`,
		`{"workspaceId":"workspace","revision":"revision","enabled":true}`,
	} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/build", body))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s=%d %s", body, w.Code, w.Body)
		}
	}
	foreign := managementRequest("/api/workspaces/build", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`)
	foreign.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, foreign)
	if w.Code != http.StatusForbidden || c.calls != calls {
		t.Fatalf("foreign origin=%d", w.Code)
	}
	for _, tc := range []struct {
		err    error
		status int
		text   string
	}{
		{eviedb.ErrChooserStateChanged, http.StatusConflict, "refresh"},
		{eviedb.ErrWorkspaceBuildPreset, http.StatusUnprocessableEntity, "Standard"},
		{errors.New("secret private path"), http.StatusUnprocessableEntity, "could not be saved"},
	} {
		c.err = tc.err
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/build", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.text) || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("error %v=%d %s", tc.err, w.Code, w.Body)
		}
	}
	plain := NewContextServer(nil, nil, nil, &fakeContextSessionController{}).Handler()
	w = httptest.NewRecorder()
	plain.ServeHTTP(w, managementRequest("/api/workspaces/build", `{"workspaceId":"workspace","revision":"revision","setting":"on"}`))
	if w.Code == http.StatusOK {
		t.Fatalf("build route without a controller=%d", w.Code)
	}
}
