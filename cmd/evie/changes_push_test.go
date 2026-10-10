package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// changes.spec.md story 4a: before a push, the Workspace's Verify command
// runs on the build branch's worktree the way the merge gate runs it,
// sandboxed without network.
func TestVerifyBeforeAPushRunsTheWorkspaceCommandInTheWorktree(t *testing.T) {
	ctx := context.Background()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	repo := filepath.Join(base, "repo")
	cliGit(t, base, "init", "-q", "-b", "main", repo)
	cliGit(t, repo, "commit", "-q", "--allow-empty", "-m", "first")
	db, err := eviedb.OpenDBAt(filepath.Join(base, "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	w, err := store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Code", FolderPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	path, temp := filepath.Join(repo, ".evie", "worktrees", id), filepath.Join(repo, ".evie", "tmp", id)
	for _, dir := range []string{path, temp} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(path, "marker"), []byte("in the worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	attempt := delegation.Attempt{ID: id, State: "succeeded", CreatedAt: time.Now().UTC(), Child: child,
		Parent:     delegation.Parent{Scope: memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: parent.ID, WorkspaceID: w.ID}},
		Assignment: delegation.Assignment{Key: "key", Objective: "Fix the footer"},
		Worktree:   &delegation.Worktree{Repo: repo, Path: path, Temp: temp, Branch: "evie/" + id[:8] + "-footer", Base: "main"}}
	record, _ := json.Marshal(attempt)
	if _, err := db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
		id, parent.ID, "key", "digest", child.ID, attempt.State, string(record)); err != nil {
		t.Fatal(err)
	}
	verifier := storeVerifier{store: store}
	change := changes.Change{ExecutionID: id, Worktree: path}

	if outcome, err := verifier.Verify(ctx, w.ID, change); err != nil || outcome != nil {
		t.Fatalf("no Verify command = %+v, %v", outcome, err)
	}
	set := func(verify string) {
		t.Helper()
		current, err := store.BuildVerifyFor(ctx, memory.ScopeContext{WorkspaceID: w.ID})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SetWorkspaceVerify(ctx, w.ID, current.Revision, "", verify); err != nil {
			t.Fatal(err)
		}
	}
	set("cat marker")
	outcome, err := verifier.Verify(ctx, w.ID, change)
	if err != nil || outcome == nil || !outcome.Passed || outcome.Command != "cat marker" || !strings.Contains(outcome.Output, "in the worktree") {
		t.Fatalf("passing Verify = %+v, %v", outcome, err)
	}
	set("echo broke; exit 3")
	if outcome, err := verifier.Verify(ctx, w.ID, change); err != nil || outcome == nil || outcome.Passed || !strings.Contains(outcome.Output, "broke") {
		t.Fatalf("failing Verify = %+v, %v", outcome, err)
	}
	if _, err := verifier.Verify(ctx, w.ID, changes.Change{ExecutionID: "missing"}); err == nil {
		t.Fatal("a Change with no build attempt ran Verify")
	}
}

// Story 4a: a chat's build in a GitHub Repository becomes a draft Change
// instead of a local merge; the Repository's kind comes from its origin,
// without the token.
func TestGitHubFoldersReadsTheRepositoryKindFromOrigin(t *testing.T) {
	ctx := context.Background()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	repo := filepath.Join(base, "repo")
	cliGit(t, base, "init", "-q", "-b", "main", repo)
	folders := githubFolders{repositories: changes.NewService(changes.Config{})}
	if onGitHub, err := folders.OnGitHub(ctx, repo); err != nil || onGitHub {
		t.Fatalf("local = %v, %v", onGitHub, err)
	}
	cliGit(t, repo, "remote", "add", "origin", "https://github.com/davidadel66/notes.git")
	if onGitHub, err := folders.OnGitHub(ctx, repo); err != nil || !onGitHub {
		t.Fatalf("GitHub = %v, %v", onGitHub, err)
	}
}
