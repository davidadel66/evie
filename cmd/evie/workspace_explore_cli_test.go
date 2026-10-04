package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

// `evie workspaces list` shows each Workspace's explore setting and whether
// it allows explore now; `evie workspaces explore <id> on|off|default`
// changes it as a new revision of the Workspace the owner names.
func TestWorkspacesExploreCommandShowsAndSetsTheSetting(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	w, err := store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Code", FolderPath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runWorkspaceManagement(ctx, args, &out, store)
		if !handled {
			t.Fatalf("%q was not handled", args)
		}
		return out.String(), err
	}
	listed, err := run("workspaces", "list")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Workspaces []memory.Workspace `json:"workspaces"`
	}
	if err := json.Unmarshal([]byte(listed), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].ID != w.ID || snapshot.Workspaces[0].Explore != (memory.WorkspaceExplore{Setting: memory.WorkspaceExploreDefault, Allowed: true}) {
		t.Fatalf("listed = %s", listed)
	}
	revision := w.CurrentRevisionID
	for _, tc := range []struct {
		setting memory.WorkspaceExploreSetting
		allowed bool
	}{{memory.WorkspaceExploreOff, false}, {memory.WorkspaceExploreOn, true}, {memory.WorkspaceExploreDefault, true}} {
		out, err := run("workspaces", "explore", string(w.ID), string(tc.setting))
		if err != nil {
			t.Fatalf("%s: %v", tc.setting, err)
		}
		var changed memory.Workspace
		if err := json.Unmarshal([]byte(out), &changed); err != nil {
			t.Fatal(err)
		}
		if changed.Explore != (memory.WorkspaceExplore{Setting: tc.setting, Allowed: tc.allowed}) || changed.CurrentRevisionID == revision {
			t.Fatalf("%s: %s", tc.setting, out)
		}
		revision = changed.CurrentRevisionID
	}
	for _, args := range [][]string{
		{"workspaces"},
		{"workspaces", "explore", string(w.ID)},
		{"workspaces", "explore", string(w.ID), "sometimes"},
		{"workspaces", "explore", "missing", "on"},
	} {
		if _, err := run(args...); err == nil {
			t.Fatalf("%q succeeded", args)
		}
	}
	if _, err := run("workspaces", "explore", string(w.ID), "maybe"); err == nil || !strings.Contains(err.Error(), "on|off|default") {
		t.Fatalf("usage error = %v", err)
	}
	var out bytes.Buffer
	if handled, _ := runWorkspaceManagement(ctx, []string{"sessions", "inspect", "x"}, &out, store); handled {
		t.Fatal("another command was handled")
	}
}

// The web settings toggle reaches the store through the real controller.
func TestWebWorkspaceExploreRouteChangesTheStoredSetting(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	w, err := store.RegisterWorkspace(ctx, "No folder")
	if err != nil {
		t.Fatal(err)
	}
	handler := web.NewContextServer(nil, nil, nil, newWebContextSessionController(store, sessionCompositionManager(t), nil)).Handler()
	body := `{"workspaceId":"` + string(w.ID) + `","revision":"` + string(w.CurrentRevisionID) + `","setting":"on"}`
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687/api/workspaces/explore", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var changed memory.Workspace
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &changed) != nil || changed.Explore != (memory.WorkspaceExplore{Setting: memory.WorkspaceExploreOn, Allowed: true}) {
		t.Fatalf("explore route=%d %s", recorder.Code, recorder.Body)
	}
	listed, err := store.ListWorkspaces(ctx, false)
	if err != nil || len(listed) != 1 || listed[0].CurrentRevisionID != changed.CurrentRevisionID || listed[0].Explore.Setting != memory.WorkspaceExploreOn {
		t.Fatalf("stored = %+v, %v", listed, err)
	}
}
