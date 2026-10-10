package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
)

// changes.spec.md story 1: Settings → Repository reads a Workspace's
// Repository from its current folder through one management route.
func TestRepositoryRouteReadsTheWorkspaceFolder(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "evie")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "master"}, {"remote", "add", "origin", "git@github.com:davidadel66/evie.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	handler := WithRepositories(NewContextServer(nil, nil, nil, &folderTestController{store: store}), changes.NewService(nil)).Handler()
	call := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest("/api/workspaces/repository", body))
		return rec
	}
	read := func() changes.Repository {
		t.Helper()
		res := call(`{"workspace_id":"` + string(workspace.ID) + `"}`)
		var r changes.Repository
		if res.Code != 200 || json.Unmarshal(res.Body.Bytes(), &r) != nil {
			t.Fatalf("repository = %d %s", res.Code, res.Body)
		}
		return r
	}
	if r := read(); r.Kind != changes.None || r.Problem != "This Workspace has no folder." {
		t.Fatalf("without a folder = %+v", r)
	}
	if _, err := store.SetWorkspaceFolder(ctx, workspace.ID, 0, repo); err != nil {
		t.Fatal(err)
	}
	if r := read(); r.Kind != changes.GitHub || r.Owner != "davidadel66" || r.Name != "evie" || r.GitHub == nil || r.GitHub.State != changes.NoToken {
		t.Fatalf("with the folder = %+v", r)
	}
	if res := call(`{"workspace_id":"missing"}`); res.Code != 404 {
		t.Fatalf("unknown Workspace = %d %s", res.Code, res.Body)
	}
	for _, body := range []string{`{}`, `{"workspace_id":"x","folder":"/etc"}`} {
		if res := call(body); res.Code != 400 {
			t.Fatalf("%s = %d %s", body, res.Code, res.Body)
		}
	}
}
