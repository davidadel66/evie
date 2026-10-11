package subagents_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
)

// workspaceGateParent is a live turn of a Standard chat in a Workspace on
// repo whose owner turned build workers on and set the verify command.
func (f *fixture) workspaceGateParent(t *testing.T, repo string) (delegation.Parent, memory.WorkspaceID) {
	t.Helper()
	ctx := context.Background()
	ws, err := f.store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Code", FolderPath: repo})
	if err != nil {
		t.Fatal(err)
	}
	on, err := f.store.SetWorkspaceBuild(ctx, ws.ID, ws.CurrentRevisionID, memory.WorkspaceBuildOn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SetWorkspaceVerify(ctx, ws.ID, 0, "", verifyCommand); err != nil {
		t.Fatal(err)
	}
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateWorkspaceSessionWithComposition(ctx, ws.ID, on.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	return f.liveTurn(t, session), ws.ID
}

// changes.spec.md story 5a: the owner merges and closes Evie's local
// Changes from Changes or the CLI, without a chat. A draft goes through
// the whole gate first; one whose gate passed and whose target has not
// moved merges at once, through the same merge as an approved chat card.
// Nothing outside the Workspace's own builds can be merged or closed.
func TestTheOwnerMergesAndClosesAWorkspacesBuilds(t *testing.T) {
	requireGateSandbox(t)
	ctx := context.Background()
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p, ws := f.workspaceGateParent(t, repo)
	tree := f.taskTree(t, p)

	// A draft: the gate never ran, and the owner's master moved since.
	draft := f.buildFiles(t, p, delegation.Assignment{Key: "parser", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	ownerHead := ownerCommit(t, repo, "docs.md", "owner docs\n")
	if _, err := f.supervisor.MergeAsOwner(ctx, "another-workspace", draft.ExecutionID); !errors.Is(err, delegation.ErrNotFound) {
		t.Fatalf("another Workspace's merge = %v", err)
	}
	merged, err := f.supervisor.MergeAsOwner(ctx, ws, draft.ExecutionID)
	if err != nil {
		t.Fatal(err)
	}
	if merged.Status != delegation.MergeMerged || merged.Merged == nil || len(merged.Verification) != 2 || !merged.Rebased ||
		git(t, repo, "rev-parse", "master") != merged.BranchHead || git(t, repo, "rev-parse", "master~1") != ownerHead {
		t.Fatalf("draft merge = %+v", merged)
	}
	if g := gateOf(t, f, p, draft.ExecutionID); g.Step != delegation.GateMerged || g.Owner != nil {
		t.Fatalf("gate = %+v", g)
	}
	if _, err := os.Stat(draft.Build.Worktree); !os.IsNotExist(err) || git(t, repo, "branch", "--list", draft.Build.Branch) != "" {
		t.Fatal("the merged worktree or branch was kept")
	}
	if _, err := f.supervisor.MergeAsOwner(ctx, ws, draft.ExecutionID); err == nil || !strings.Contains(err.Error(), "already merged") {
		t.Fatalf("merging again = %v", err)
	}

	// Open: the chat's proposal passed the gate and, in a Workspace, asked
	// nothing (story 6b): the Change waits in the Changes tab, and the
	// owner's merge there reuses the gate's runs without verifying again.
	open := f.buildFiles(t, p, delegation.Assignment{Key: "lexer", Objective: "Add a lexer", TaskID: string(tree.sibling.ID)},
		map[string]string{"lexer.go": "package main\n\nfunc lex() {}\n"}, nil)
	proposed := f.proposeMerge(t, p, map[string]string{"execution_id": open.ExecutionID}, neverAsked(t))
	if proposed.asked != 0 || proposed.isErr || !strings.Contains(proposed.content, `"status":"ready"`) || !strings.Contains(proposed.content, "Changes tab") {
		t.Fatalf("proposal = %+v", proposed)
	}
	verified := gateOf(t, f, p, open.ExecutionID)
	opened, err := f.supervisor.MergeAsOwner(ctx, ws, open.ExecutionID)
	if err != nil || opened.Status != delegation.MergeMerged || git(t, repo, "rev-parse", "master") != verified.BranchHead {
		t.Fatalf("open merge = %+v, %v", opened, err)
	}
	if len(opened.Verification) != len(verified.Runs) || opened.Verification[0].StartedAt != verified.Runs[0].StartedAt {
		t.Fatalf("the open Change was verified again: %+v, gate %+v", opened.Verification, verified.Runs)
	}

	// A failing check stops the owner's merge as it stops a proposal.
	broken := f.buildFiles(t, p, delegation.Assignment{Key: "broken", Objective: "Break the parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\n// BROKEN\nfunc parse() {}\n"}, nil)
	master := git(t, repo, "rev-parse", "master")
	_, err = f.supervisor.MergeAsOwner(ctx, ws, broken.ExecutionID)
	var refused *delegation.MergeGateError
	if !errors.As(err, &refused) || refused.Proposal.Status != delegation.MergeVerifyFailed || git(t, repo, "rev-parse", "master") != master {
		t.Fatalf("broken merge = %v", err)
	}

	// Close removes Evie's worktree and branch, and nothing else.
	if err := f.supervisor.CloseAsOwner(ctx, "another-workspace", broken.ExecutionID); !errors.Is(err, delegation.ErrNotFound) {
		t.Fatalf("another Workspace's close = %v", err)
	}
	if err := f.supervisor.CloseAsOwner(ctx, ws, broken.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(broken.Build.Worktree); !os.IsNotExist(err) || git(t, repo, "branch", "--list", broken.Build.Branch) != "" ||
		git(t, repo, "rev-parse", "master") != master || git(t, repo, "branch", "--show-current") != "master" {
		t.Fatal("close kept Evie's worktree or branch, or touched the owner's")
	}
	if a, err := f.store.InspectSubagent(ctx, p, broken.ExecutionID); err != nil || a.Worktree.RemovedAt == nil {
		t.Fatalf("closed attempt = %+v, %v", a.Worktree, err)
	}
	if err := f.supervisor.CloseAsOwner(ctx, ws, broken.ExecutionID); err == nil {
		t.Fatal("closed twice")
	}
}

// Story 5a: a gate step the owner runs from another process (the CLI) has
// no chat turn, so it holds a claim with a deadline; recovery in evie serve
// leaves it alone until then, and settles it after, as it settles a lost
// turn's.
func TestRecoveryLeavesTheOwnersGateStepUntilItsClaimEnds(t *testing.T) {
	ctx := context.Background()
	f := setup(t, delegation.DefaultPolicy())
	repo := buildRepo(t)
	p, _ := f.workspaceGateParent(t, repo)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "parser", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n"}, nil)
	claim := func(until time.Time) {
		t.Helper()
		if _, err := f.store.RecordBuildGate(ctx, built.ExecutionID, func(g *delegation.BuildGate) {
			g.Step, g.Owner = delegation.GateVerifying, &delegation.GateOwner{Until: &until}
		}); err != nil {
			t.Fatal(err)
		}
	}
	candidates := func() int {
		t.Helper()
		found, err := f.store.BuildGateRecoveryCandidates(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return len(found)
	}
	claim(time.Now().Add(time.Hour))
	if n := candidates(); n != 0 {
		t.Fatalf("a live owner claim is a recovery candidate (%d)", n)
	}
	claim(time.Now().Add(-time.Minute))
	if n := candidates(); n != 1 {
		t.Fatalf("an ended owner claim is not a recovery candidate (%d)", n)
	}
}
