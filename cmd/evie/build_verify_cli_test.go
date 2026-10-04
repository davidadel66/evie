package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

// Stage 12 (2026-10-04): `evie workspaces verify <id>` shows a Workspace's
// setup and verify commands with what its repository's instructions
// suggest, and sets them only from the owner's flags; `evie projects verify`
// does the same for a project, by ID or root. A flag left out keeps its
// command; an empty one clears it.
func TestVerifyCommandsShowProposeAndSetOnlyFromTheOwner(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "AGENTS.md"), []byte("## Verification\n\n```sh\n./scripts/verify-change.sh\n```\n\nInstall with `npm --prefix ui ci` first.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Code", FolderPath: folder})
	if err != nil {
		t.Fatal(err)
	}
	run := func(handle func(context.Context, []string, *bytes.Buffer) (bool, error), args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		handled, err := handle(ctx, args, &out)
		if !handled {
			t.Fatalf("%q was not handled", args)
		}
		return out.String(), err
	}
	workspaces := func(ctx context.Context, args []string, out *bytes.Buffer) (bool, error) {
		return runWorkspaceManagement(ctx, args, out, store)
	}
	projects := func(ctx context.Context, args []string, out *bytes.Buffer) (bool, error) {
		return runProjectManagement(ctx, args, out, store)
	}

	shown, err := run(workspaces, "workspaces", "verify", string(w.ID))
	var view struct {
		Verify   memory.BuildVerify `json:"verify"`
		Proposal struct {
			Setup, Verify, SetupSource, VerifySource string
		} `json:"proposal"`
	}
	if err != nil || json.Unmarshal([]byte(shown), &view) != nil || view.Verify != (memory.BuildVerify{}) || view.Proposal.Verify != "./scripts/verify-change.sh" ||
		view.Proposal.Setup != "npm --prefix ui ci" || view.Proposal.VerifySource != "AGENTS.md" {
		t.Fatalf("shown = %s %v", shown, err)
	}
	if got, _ := store.BuildVerifyFor(ctx, memory.ScopeContext{WorkspaceID: w.ID}); got != (memory.BuildVerify{}) {
		t.Fatalf("showing a proposal set commands: %+v", got)
	}

	set, err := run(workspaces, "workspaces", "verify", string(w.ID), "--setup", "npm --prefix ui ci", "--verify", "./scripts/verify-change.sh")
	if err != nil || !strings.Contains(set, `"setup": "npm --prefix ui ci"`) || !strings.Contains(set, `"revision": 1`) {
		t.Fatalf("set = %s %v", set, err)
	}
	if _, err := run(workspaces, "workspaces", "verify", string(w.ID), "--setup", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.BuildVerifyFor(ctx, memory.ScopeContext{WorkspaceID: w.ID}); got != (memory.BuildVerify{Verify: "./scripts/verify-change.sh", Revision: 2}) {
		t.Fatalf("after clearing setup = %+v", got)
	}
	if listed, err := run(workspaces, "workspaces", "list"); err != nil || !strings.Contains(listed, `"verify": "./scripts/verify-change.sh"`) {
		t.Fatalf("listed = %s %v", listed, err)
	}
	for _, args := range [][]string{{"workspaces", "verify"}, {"workspaces", "verify", "missing", "--verify", "true"}, {"workspaces", "verify", string(w.ID), "--bogus", "x"}, {"workspaces", "verify", string(w.ID), "--verify"}} {
		if _, err := run(workspaces, args...); err == nil {
			t.Fatalf("%q succeeded", args)
		}
	}

	root := t.TempDir()
	project, err := store.RegisterProject(ctx, "Repo", root)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := run(projects, "projects", "verify", root, "--verify", "go test ./..."); err != nil || !strings.Contains(out, `"verify": "go test ./..."`) {
		t.Fatalf("project by root = %s %v", out, err)
	}
	if out, err := run(projects, "projects", "verify", string(project.ID)); err != nil || !strings.Contains(out, `"revision": 1`) {
		t.Fatalf("project by ID = %s %v", out, err)
	}
	if listed, err := run(projects, "projects", "list"); err != nil || !strings.Contains(listed, string(project.ID)) || !strings.Contains(listed, `"verify": "go test ./..."`) {
		t.Fatalf("projects list = %s %v", listed, err)
	}
	if _, err := run(projects, "projects", "verify", "missing", "--verify", "true"); err == nil {
		t.Fatal("an unknown project was configured")
	}
}

// Stage 12: the web routes reach the store through the real controller: the
// proposal reads the Workspace folder's instructions and records nothing,
// and the owner's save records a revision.
func TestWebWorkspaceVerifyRoutesProposeAndSet(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "CLAUDE.md"), []byte("## Checks\n\n```sh\nmake check\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, err := store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Code", FolderPath: folder})
	if err != nil {
		t.Fatal(err)
	}
	handler := web.NewContextServer(nil, nil, nil, newWebContextSessionController(store, sessionCompositionManager(t), nil)).Handler()
	post := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687"+path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	proposed := post("/api/workspaces/verify/propose", `{"workspaceId":"`+string(w.ID)+`"}`)
	if proposed.Code != http.StatusOK || !strings.Contains(proposed.Body.String(), `"verify":"make check"`) {
		t.Fatalf("propose route=%d %s", proposed.Code, proposed.Body)
	}
	if got, _ := store.BuildVerifyFor(ctx, memory.ScopeContext{WorkspaceID: w.ID}); got != (memory.BuildVerify{}) {
		t.Fatalf("a proposal set commands: %+v", got)
	}
	saved := post("/api/workspaces/verify", `{"workspaceId":"`+string(w.ID)+`","revision":0,"setup":"","verify":"make check"}`)
	var changed memory.Workspace
	if saved.Code != http.StatusOK || json.Unmarshal(saved.Body.Bytes(), &changed) != nil || changed.Verify != (memory.BuildVerify{Verify: "make check", Revision: 1}) {
		t.Fatalf("verify route=%d %s", saved.Code, saved.Body)
	}
	if stale := post("/api/workspaces/verify", `{"workspaceId":"`+string(w.ID)+`","revision":0,"setup":"","verify":"true"}`); stale.Code != http.StatusConflict {
		t.Fatalf("stale save=%d %s", stale.Code, stale.Body)
	}
}

// Stage 12: `evie worktrees list` shows each build worktree's last merge
// gate step and problem, how setup went, and whether its target moved.
func TestWorktreesListShowsTheMergeGate(t *testing.T) {
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
	a := abandonedBuild(t, store, db, repo, "succeeded")
	if _, err := store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
		g.Step, g.Problem = delegation.GateVerifyFailed, "The verify command failed"
		g.Setup = &delegation.GateRun{Step: "setup", Command: "npm ci", Outcome: delegation.RunPassed}
		g.NeedsRebase = &delegation.TargetMove{Target: "master", To: "abc123"}
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if _, err := runWorktreeManagement(ctx, []string{"worktrees", "list"}, &out, store); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"gate_step": "verify_failed"`, `"gate_problem": "The verify command failed"`, `"setup": "passed"`, `"needs_rebase": "master moved to abc123"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("listing lacks %s:\n%s", want, out.String())
		}
	}
}
