package main

import (
	"bytes"
	"context"
	"encoding/json"
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

// changes.spec.md story 3a: `evie changes list <workspace-id>` prints the
// Workspace's Repository and its Changes, here a workflow build worker's
// branch as a draft linked to its run.
func TestChangesListCommandPrintsTheWorkspaceChanges(t *testing.T) {
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
	workers, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, now := uuid.NewString(), time.Now().UTC()
	attempt := delegation.Attempt{ID: id, State: "running", CreatedAt: now, Child: child,
		Parent:     delegation.Parent{Scope: memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: workers.ID, WorkspaceID: w.ID}},
		Assignment: delegation.Assignment{Key: "key", Objective: "Fix the flaky test"},
		Run:        &delegation.RunOwner{RunID: "run-7", WorkspaceID: w.ID, Workflow: "fix-bugs", Step: "build"},
		Worktree:   &delegation.Worktree{Repo: repo, Path: filepath.Join(repo, ".evie", "worktrees", id), Branch: "evie/" + id[:8] + "-flaky", Base: "main"}}
	record, _ := json.Marshal(attempt)
	if _, err := db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
		id, workers.ID, "key", "digest", child.ID, attempt.State, string(record)); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runChangeManagement(ctx, args, &out, store, func() *changes.Service { return changes.NewService(changes.Config{}) })
		if !handled {
			t.Fatalf("%q was not handled", args)
		}
		return out.String(), err
	}

	out, err := run("changes", "list", string(w.ID))
	var listed struct {
		Repository changes.Repository `json:"repository"`
		Changes    []changes.Change   `json:"changes"`
	}
	if err != nil || json.Unmarshal([]byte(out), &listed) != nil {
		t.Fatalf("list = %s %v", out, err)
	}
	if listed.Repository.Kind != changes.Local || listed.Repository.Folder != repo || len(listed.Changes) != 1 {
		t.Fatalf("listed = %s", out)
	}
	if c := listed.Changes[0]; c.ExecutionID != id || c.ID != "build:"+c.Branch || c.State != changes.Draft || c.Title != "Fix the flaky test" ||
		c.Origin == nil || *c.Origin != (changes.Origin{RunID: "run-7", Workflow: "fix-bugs", Step: "build"}) || c.Worktree != attempt.Worktree.Path {
		t.Fatalf("Change = %s", out)
	}
	if out, err := run("changes", "list", "missing"); err == nil || !strings.Contains(err.Error(), "no Workspace") {
		t.Fatalf("unknown Workspace = %s %v", out, err)
	}
	for _, args := range [][]string{{"changes"}, {"changes", "list"}, {"changes", "push", string(w.ID)}} {
		if _, err := run(args...); err == nil || !strings.Contains(err.Error(), "usage: evie changes") {
			t.Fatalf("%q = %v", args, err)
		}
	}
	var out2 bytes.Buffer
	if handled, _ := runChangeManagement(ctx, []string{"worktrees", "list"}, &out2, store, nil); handled {
		t.Fatal("another command was handled")
	}
}
