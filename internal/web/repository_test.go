package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
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
	handler := WithRepositories(NewContextServer(nil, nil, nil, &folderTestController{store: store}), changes.NewService(changes.Config{})).Handler()
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

// stubRepositories records a publish and answers it as told.
type stubRepositories struct {
	published []changes.Publish
	err       error
}

func (s *stubRepositories) Repository(context.Context, string) (changes.Repository, error) {
	return changes.Repository{Kind: changes.Local}, nil
}
func (s *stubRepositories) Publish(_ context.Context, p changes.Publish) (changes.Repository, error) {
	s.published = append(s.published, p)
	if s.err != nil {
		return changes.Repository{}, s.err
	}
	return changes.Repository{Kind: changes.GitHub, Owner: "davidadel66", Name: p.Name}, nil
}
func (s *stubRepositories) Account(context.Context) changes.GitHubStatus {
	return changes.GitHubStatus{State: changes.Connected, Login: "davidadel66"}
}
func (s *stubRepositories) Prepare(context.Context, changes.Prepare) (changes.Repository, error) {
	return changes.Repository{}, errors.New("not used")
}
func (s *stubRepositories) GitHubRepositories(context.Context) ([]github.Repository, error) {
	return []github.Repository{{Name: "evie", FullName: "davidadel66/evie"}}, nil
}

// Story 2a: Publish takes the Workspace's folder, never one from the
// browser, and turns each refusal into its own status.
func TestPublishRouteUsesTheWorkspaceFolderAndMapsRefusals(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if _, err := store.SetWorkspaceFolder(ctx, workspace.ID, 0, folder); err != nil {
		t.Fatal(err)
	}
	stub := &stubRepositories{}
	handler := WithRepositories(NewContextServer(nil, nil, nil, &folderTestController{store: store}), stub).Handler()
	call := func(route, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest(route, body))
		return rec
	}
	publish := `{"workspace_id":"` + string(workspace.ID) + `","name":"notes","private":true}`
	if res := call("/api/workspaces/repository/publish", publish); res.Code != 200 || len(stub.published) != 1 {
		t.Fatalf("publish = %d %s", res.Code, res.Body)
	}
	if got := stub.published[0]; got.Folder == "" || got.Name != "notes" || !got.Private {
		t.Fatalf("published %+v", got)
	}
	if real, _ := filepath.EvalSymlinks(folder); stub.published[0].Folder != folder && stub.published[0].Folder != real {
		t.Fatalf("folder %q, want the Workspace's %q", stub.published[0].Folder, folder)
	}
	for err, want := range map[error]int{
		changes.ErrNoToken: 409,
		fmt.Errorf("%w: origin", changes.ErrNotPublishable): 409,
		changes.ErrBusy:       409,
		github.ErrInvalidName: 400,
		fmt.Errorf("%w: name already exists", github.ErrUnprocessable): 422,
		fmt.Errorf("%w: created", changes.ErrPushFailed):               502,
	} {
		stub.err = err
		if res := call("/api/workspaces/repository/publish", publish); res.Code != want || !strings.Contains(res.Body.String(), err.Error()) {
			t.Fatalf("%v = %d %s, want %d", err, res.Code, res.Body, want)
		}
	}
	for _, body := range []string{`{"workspace_id":"x","name":""}`, `{"workspace_id":"x","name":"n","folder":"/etc"}`} {
		if res := call("/api/workspaces/repository/publish", body); res.Code != 400 {
			t.Fatalf("%s = %d", body, res.Code)
		}
	}
	if res := call("/api/github/status", `{}`); res.Code != 200 || !strings.Contains(res.Body.String(), `"login":"davidadel66"`) {
		t.Fatalf("status = %d %s", res.Code, res.Body)
	}
}
