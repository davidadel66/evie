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
	gate := &cliGate{}
	run := func(args ...string) (string, error) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runChangeManagement(ctx, args, &out, changeCommands{store: store, repositories: func() *changes.Service { return changes.NewService(changes.Config{}) },
			getenv: func(string) string { return "" }, gate: func() (changes.BuildGate, error) { return gate, nil }})
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
	// Story 4a: pushing needs GitHub; without a token the command says so
	// before reading the Workspace's branches.
	if out, err := run("changes", "push", string(w.ID), attempt.Worktree.Branch); err == nil || !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("push without a token = %s %v", out, err)
	}
	if out, err := run("changes", "push", "missing", attempt.Worktree.Branch); err == nil || !strings.Contains(err.Error(), "no Workspace") {
		t.Fatalf("push to an unknown Workspace = %s %v", out, err)
	}
	// Story 5a: merge and close by the ID the list shows, through the merge
	// gate; closing discards the branch, so it needs --yes.
	changeID := "build:" + attempt.Worktree.Branch
	if out, err := run("changes", "merge", string(w.ID), changeID); err != nil || !strings.Contains(out, `"status": "merged"`) || len(gate.merged) != 1 || gate.merged[0] != string(w.ID)+" "+id {
		t.Fatalf("merge = %s %v; gate %v", out, err, gate.merged)
	}
	// Story 5c: "Merge when checks pass" is for GitHub pull requests.
	if _, err := run("changes", "merge", string(w.ID), changeID, "--when-checks-pass"); err == nil || !strings.Contains(err.Error(), "GitHub") {
		t.Fatalf("arming a local Change = %v", err)
	}
	if _, err := run("changes", "disarm", string(w.ID), changeID); err == nil || !strings.Contains(err.Error(), "not set") {
		t.Fatalf("disarming = %v", err)
	}
	if _, err := run("changes", "close", string(w.ID), changeID); err == nil || !strings.Contains(err.Error(), "--yes") || len(gate.closed) != 0 {
		t.Fatalf("close without --yes = %v; gate %v", err, gate.closed)
	}
	if out, err := run("changes", "close", string(w.ID), changeID, "--yes"); err != nil || len(gate.closed) != 1 || !strings.Contains(out, changeID) {
		t.Fatalf("close = %s %v; gate %v", out, err, gate.closed)
	}
	for _, args := range [][]string{{"changes"}, {"changes", "list"}, {"changes", "push", string(w.ID)}, {"changes", "push", string(w.ID), ""}, {"changes", "merge", string(w.ID)}, {"changes", "close", string(w.ID), changeID, "--force"}, {"changes", "merge", string(w.ID), changeID, "--now"}, {"changes", "disarm", string(w.ID)}} {
		if _, err := run(args...); err == nil || !strings.Contains(err.Error(), "usage: evie changes") {
			t.Fatalf("%q = %v", args, err)
		}
	}
	var out2 bytes.Buffer
	if handled, _ := runChangeManagement(ctx, []string{"worktrees", "list"}, &out2, changeCommands{store: store}); handled {
		t.Fatal("another command was handled")
	}
}

// cliGate stands in for the Kernel's merge gate.
type cliGate struct{ merged, closed []string }

func (g *cliGate) MergeAsOwner(_ context.Context, ws memory.WorkspaceID, execution string) (delegation.MergeProposal, error) {
	g.merged = append(g.merged, string(ws)+" "+execution)
	return delegation.MergeProposal{ExecutionID: execution, Status: delegation.MergeMerged, Merged: &delegation.GateMerge{Target: "main"}}, nil
}

func (g *cliGate) CloseAsOwner(_ context.Context, ws memory.WorkspaceID, execution string) error {
	g.closed = append(g.closed, string(ws)+" "+execution)
	return nil
}
