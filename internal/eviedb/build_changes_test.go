package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// recordTestBuild stores a finished build attempt of Workspace ws as the
// harness records it, with its worktree in repo; change adjusts it first.
func recordTestBuild(t *testing.T, store *Store, ws memory.WorkspaceID, repo string, change func(*delegation.Attempt)) delegation.Attempt {
	t.Helper()
	ctx := context.Background()
	parent, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, now := uuid.NewString(), time.Now().UTC()
	a := delegation.Attempt{ID: id, State: "succeeded", CreatedAt: now, EndedAt: &now, Child: child,
		Parent:     delegation.Parent{Scope: memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: parent.ID, WorkspaceID: ws}},
		Assignment: delegation.Assignment{Key: "key-" + id, Objective: "Build something"},
		Worktree: &delegation.Worktree{Repo: repo, Path: filepath.Join(repo, ".evie", "worktrees", id), Temp: filepath.Join(repo, ".evie", "tmp", id),
			Branch: "evie/" + id[:8] + "-work", Base: "main"}}
	if change != nil {
		change(&a)
	}
	record, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
		a.ID, parent.ID, a.Assignment.Key, "digest", child.ID, a.State, string(record)); err != nil {
		t.Fatal(err)
	}
	return a
}

// changes.spec.md story 3a (the folder guard moved from story 1): a
// Workspace's folder cannot be changed or detached while a draft or open
// Change of the Workspace lives in it. Merging or removing the build's
// worktree finishes it; another Workspace's builds, the Workspace's builds
// in another folder, and setting the same folder again do not count.
func TestWorkspaceFolderStaysWhileABuildChangeInItIsUnfinished(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ws, err := store.RegisterWorkspace(ctx, "Code")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.RegisterWorkspace(ctx, "Other")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	next, previous := t.TempDir(), t.TempDir()
	if _, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, repo); err != nil {
		t.Fatal(err)
	}
	recordTestBuild(t, store, other.ID, repo, func(a *delegation.Attempt) { a.State, a.EndedAt = "running", nil })
	recordTestBuild(t, store, ws.ID, previous, nil)
	draft := recordTestBuild(t, store, ws.ID, repo, nil)
	// A workflow build worker belongs to its run's Workspace.
	workflow := recordTestBuild(t, store, "", repo, func(a *delegation.Attempt) {
		a.Run = &delegation.RunOwner{RunID: "run-1", WorkspaceID: ws.ID, Workflow: "fix-bugs", Step: "build"}
	})

	for _, path := range []string{next, ""} {
		_, err := store.SetWorkspaceFolder(ctx, ws.ID, 1, path)
		if !errors.Is(err, ErrUnfinishedChanges) || !strings.Contains(err.Error(), "2 ") {
			t.Fatalf("set %q while unfinished = %v", path, err)
		}
	}
	// A stale browser is told to refresh first, as before.
	if _, err := store.SetWorkspaceFolder(ctx, ws.ID, 0, next); !errors.Is(err, ErrWorkspaceFolderChanged) {
		t.Fatalf("stale = %v", err)
	}
	// The same folder again changes nothing a Change depends on.
	if folder, err := store.SetWorkspaceFolder(ctx, ws.ID, 1, repo); err != nil || folder.Revision != 2 || folder.Path != repo {
		t.Fatalf("same folder = %+v %v", folder, err)
	}

	if _, err := store.RecordBuildGate(ctx, draft.ID, func(g *delegation.BuildGate) {
		g.Step, g.Merged = delegation.GateMerged, &delegation.GateMerge{Target: "main", To: "head", At: time.Now().UTC()}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetWorkspaceFolder(ctx, ws.ID, 2, next); !errors.Is(err, ErrUnfinishedChanges) || !strings.Contains(err.Error(), "1 ") {
		t.Fatalf("set with one unfinished = %v", err)
	}
	if err := store.MarkBuildWorktreeRemoved(ctx, workflow.ID); err != nil {
		t.Fatal(err)
	}
	folder, err := store.SetWorkspaceFolder(ctx, ws.ID, 2, next)
	if err != nil || folder.Revision != 3 {
		t.Fatalf("set after merge and removal = %+v %v", folder, err)
	}
	if current, err := store.WorkspaceFolder(ctx, ws.ID); err != nil || current != folder {
		t.Fatalf("current = %+v %v", current, err)
	}
	// The builds left in the earlier folder are not Changes of the new one.
	if _, err := store.SetWorkspaceFolder(ctx, ws.ID, 3, ""); err != nil {
		t.Fatalf("detach = %v", err)
	}
}
