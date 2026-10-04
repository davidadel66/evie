package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/worktree"
	"github.com/google/uuid"
)

// `evie workspaces build <id> on|off|default` records the owner's build
// setting, which is off by default (Stage 11); `evie workspaces list` shows it.
func TestWorkspacesBuildCommandShowsAndSetsTheSetting(t *testing.T) {
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
	if err != nil || !strings.Contains(listed, `"build": {`) || !strings.Contains(listed, `"setting": "default"`) {
		t.Fatalf("listed = %s %v", listed, err)
	}
	revision := w.CurrentRevisionID
	for _, tc := range []struct {
		setting memory.WorkspaceBuildSetting
		allowed bool
	}{{memory.WorkspaceBuildOn, true}, {memory.WorkspaceBuildOff, false}, {memory.WorkspaceBuildDefault, false}} {
		out, err := run("workspaces", "build", string(w.ID), string(tc.setting))
		if err != nil {
			t.Fatalf("%s: %v", tc.setting, err)
		}
		var changed memory.Workspace
		if err := json.Unmarshal([]byte(out), &changed); err != nil {
			t.Fatal(err)
		}
		if changed.Build != (memory.WorkspaceBuild{Setting: tc.setting, Allowed: tc.allowed}) || changed.CurrentRevisionID == revision {
			t.Fatalf("%s: %s", tc.setting, out)
		}
		revision = changed.CurrentRevisionID
	}
	for _, args := range [][]string{{"workspaces", "build", string(w.ID)}, {"workspaces", "build", string(w.ID), "always"}, {"workspaces", "build", "missing", "on"}} {
		if _, err := run(args...); err == nil {
			t.Fatalf("%q succeeded", args)
		}
	}
}

// The web API sets the build setting through the real controller.
func TestWebWorkspaceBuildRouteChangesTheStoredSetting(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	w, err := store.RegisterWorkspace(ctx, "Code")
	if err != nil {
		t.Fatal(err)
	}
	handler := web.NewContextServer(nil, nil, nil, newWebContextSessionController(store, sessionCompositionManager(t), nil)).Handler()
	body := `{"workspaceId":"` + string(w.ID) + `","revision":"` + string(w.CurrentRevisionID) + `","setting":"on"}`
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687/api/workspaces/build", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var changed memory.Workspace
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &changed) != nil || changed.Build != (memory.WorkspaceBuild{Setting: memory.WorkspaceBuildOn, Allowed: true}) {
		t.Fatalf("build route=%d %s", recorder.Code, recorder.Body)
	}
}

func cliGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// abandonedBuild records a finished build attempt of state whose worktree
// exists in repo, as a crash and recovery would leave it.
func abandonedBuild(t *testing.T, store *eviedb.Store, db *sql.DB, repo, state string) delegation.Attempt {
	t.Helper()
	ctx := context.Background()
	w, err := worktree.Plan(ctx, repo, "", uuid.NewString(), "Abandoned work")
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.Ensure(ctx, w, true); err != nil {
		t.Fatal(err)
	}
	resolved, err := sessionCompositionManager(t).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	receipt := resolved.Receipt
	parent, err := store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(ctx, parent.ID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	a := delegation.Attempt{ID: filepath.Base(w.Path), Child: child, State: state, CreatedAt: now, EndedAt: &now, Worktree: &w,
		Receipt: composition.Receipt{Preset: composition.PresetIdentity{ID: delegation.WorkerBuild}},
		Result:  &delegation.Result{Status: state, Reason: "original_parent_ownership_ended"}}
	record, _ := json.Marshal(a)
	if _, err := db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
		a.ID, parent.ID, "key-"+a.ID, "digest", child.ID, state, string(record)); err != nil {
		t.Fatal(err)
	}
	return a
}

// `evie worktrees list` shows every build worktree with its execution's
// state and whether it still exists or holds uncommitted work; `evie
// worktrees clean` removes the named or abandoned ones, refusing to drop
// uncommitted work without --force, and keeps branches unless asked.
func TestWorktreesCommandListsAndCleansAbandonedWorktrees(t *testing.T) {
	ctx := context.Background()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cliGit(t, repo, "init", "-q", "-b", "master")
	cliGit(t, repo, "add", ".")
	cliGit(t, repo, "commit", "-q", "-m", "first")
	db, err := eviedb.OpenDBAt(filepath.Join(base, "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	interrupted := abandonedBuild(t, store, db, repo, "interrupted")
	dirty := abandonedBuild(t, store, db, repo, "failed")
	kept := abandonedBuild(t, store, db, repo, "succeeded")
	if err := os.WriteFile(filepath.Join(dirty.Worktree.Path, "wip.txt"), []byte("work"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runWorktreeManagement(ctx, args, &out, store)
		if !handled {
			t.Fatalf("%q was not handled", args)
		}
		return out.String(), err
	}
	listed, err := run("worktrees", "list")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Worktrees []struct {
			ExecutionID string `json:"execution_id"`
			State       string `json:"state"`
			Present     bool   `json:"present"`
			Uncommitted bool   `json:"uncommitted"`
			Branch      string `json:"branch"`
		} `json:"worktrees"`
	}
	if err := json.Unmarshal([]byte(listed), &snapshot); err != nil || len(snapshot.Worktrees) != 3 {
		t.Fatalf("listed = %s %v", listed, err)
	}
	for _, w := range snapshot.Worktrees {
		if !w.Present || w.Uncommitted != (w.ExecutionID == dirty.ID) || w.Branch == "" {
			t.Fatalf("listed worktree = %+v", w)
		}
	}
	out, err := run("worktrees", "clean", "--abandoned")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, interrupted.ID) || !strings.Contains(out, "uncommitted") {
		t.Fatalf("clean --abandoned = %s", out)
	}
	for _, c := range []struct {
		a       delegation.Attempt
		present bool
	}{{interrupted, false}, {dirty, true}, {kept, true}} {
		if _, err := os.Stat(c.a.Worktree.Path); (err == nil) != c.present {
			t.Fatalf("%s present=%v after clean --abandoned: %v", c.a.State, c.present, err)
		}
	}
	if _, err := run("worktrees", "clean", "--force", "--delete-branch", dirty.ID, kept.ID); err != nil {
		t.Fatal(err)
	}
	for _, a := range []delegation.Attempt{dirty, kept} {
		if _, err := os.Stat(a.Worktree.Path); !os.IsNotExist(err) {
			t.Fatalf("%s survived clean --force", a.ID)
		}
		if got := cliGit(t, repo, "branch", "--list", a.Worktree.Branch); got != "" {
			t.Fatalf("branch %s survived --delete-branch", a.Worktree.Branch)
		}
	}
	if got := cliGit(t, repo, "branch", "--list", interrupted.Worktree.Branch); got == "" {
		t.Fatal("cleanup without --delete-branch deleted the branch")
	}
	for _, args := range [][]string{{"worktrees"}, {"worktrees", "clean"}, {"worktrees", "clean", "no-such-execution"}} {
		if _, err := run(args...); err == nil {
			t.Fatalf("%q succeeded", args)
		}
	}
	var buffer bytes.Buffer
	if handled, _ := runWorktreeManagement(ctx, []string{"workspaces", "list"}, &buffer, store); handled {
		t.Fatal("another command was handled")
	}
}
