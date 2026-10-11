package changes_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// fakeGate records the owner's merges and closes and answers as told.
type fakeGate struct {
	merged, closed []string
	refuse         *delegation.MergeProposal
}

func (g *fakeGate) MergeAsOwner(_ context.Context, ws memory.WorkspaceID, execution string) (delegation.MergeProposal, error) {
	g.merged = append(g.merged, string(ws)+" "+execution)
	if g.refuse != nil {
		return *g.refuse, &delegation.MergeGateError{Proposal: *g.refuse}
	}
	return delegation.MergeProposal{ExecutionID: execution, Status: delegation.MergeMerged, Merged: &delegation.GateMerge{Target: "main", To: "abc"}}, nil
}

func (g *fakeGate) CloseAsOwner(_ context.Context, ws memory.WorkspaceID, execution string) error {
	g.closed = append(g.closed, string(ws)+" "+execution)
	return nil
}

// changes.spec.md story 5a: the owner merges and closes Evie's Changes in a
// local Repository by their ID, through the merge gate; a refusal says why
// with the gate's runs. What story 5b covers (anything with a pull
// request, any merge on GitHub) is refused for now.
func TestTheOwnerMergesAndClosesLocalChangesThroughTheGate(t *testing.T) {
	ctx := context.Background()
	dir := repository(t, "main")
	draft := changes.Change{ID: "build:evie/1a2b3c4d-fix", Branch: "evie/1a2b3c4d-fix", State: changes.Draft, Author: changes.ByEvie, ExecutionID: "e1", Worktree: dir}
	merged := changes.Change{ID: "build:evie/2b3c4d5e-old", Branch: "evie/2b3c4d5e-old", State: changes.Merged, Author: changes.ByEvie, ExecutionID: "e2"}
	gate := &fakeGate{}
	m := &changes.Merger{Lister: &changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: fixedSource{draft, merged}}, Gate: gate}

	result, err := m.Merge(ctx, "w", dir, "build:evie/1a2b3c4d-fix")
	if err != nil || result.Status != delegation.MergeMerged || result.Merged == nil || result.Change.ID != draft.ID {
		t.Fatalf("merge = %+v, %v", result, err)
	}
	if len(gate.merged) != 1 || gate.merged[0] != "w e1" {
		t.Fatalf("gate merged %v", gate.merged)
	}
	gate.refuse = &delegation.MergeProposal{ExecutionID: "e1", Status: delegation.MergeVerifyFailed, Notes: []string{"The verify command failed on the branch."},
		Verification: []delegation.GateRun{{Step: "verify", Command: "go test ./...", Outcome: delegation.RunFailed, Output: "--- FAIL: TestFix"}}}
	result, err = m.Merge(ctx, "w", dir, "build:evie/1a2b3c4d-fix")
	if !errors.Is(err, changes.ErrNotMerged) || !strings.Contains(err.Error(), "verify command failed") ||
		result.Status != delegation.MergeVerifyFailed || len(result.Verification) != 1 || result.Verification[0].Output != "--- FAIL: TestFix" {
		t.Fatalf("refused merge = %+v, %v", result, err)
	}
	if c, err := m.Close(ctx, "w", dir, "build:evie/1a2b3c4d-fix"); err != nil || c.ID != draft.ID || len(gate.closed) != 1 || gate.closed[0] != "w e1" {
		t.Fatalf("close = %+v, %v; closed %v", c, err, gate.closed)
	}

	gate.merged, gate.closed = nil, nil
	for name, id := range map[string]string{"unknown": "build:evie/ffffffff-none", "merged": merged.ID, "pull request": "pr:3"} {
		if _, err := m.Merge(ctx, "w", dir, id); !errors.Is(err, changes.ErrNotMerged) {
			t.Fatalf("%s merge = %v", name, err)
		}
		if _, err := m.Close(ctx, "w", dir, id); !errors.Is(err, changes.ErrNotMerged) {
			t.Fatalf("%s close = %v", name, err)
		}
	}

	// In a GitHub Repository merging goes through the pull request (story
	// 5b); a draft without one closes as in a local Repository.
	fake, server := newFakeGitHub(t)
	gh, _ := pushRepository(t, fake)
	onGitHub := draftOn(gh)
	withPull := onGitHub
	withPull.ID, withPull.Branch, withPull.ExecutionID = "build:evie/9a8b7c6d-pr", "evie/9a8b7c6d-pr", "e9"
	withPull.PullRequest = &changes.PullRequest{Number: 4, State: "open"}
	m = &changes.Merger{Lister: &changes.Lister{Repositories: publishService(server), Builds: fixedSource{onGitHub, withPull}}, Gate: gate}
	if _, err := m.Merge(ctx, "w", gh, onGitHub.ID); !errors.Is(err, changes.ErrNotMerged) || !strings.Contains(err.Error(), "pull request") {
		t.Fatalf("GitHub merge = %v", err)
	}
	if _, err := m.Close(ctx, "w", gh, withPull.ID); !errors.Is(err, changes.ErrNotMerged) {
		t.Fatalf("closing a Change with a pull request = %v", err)
	}
	if _, err := m.Close(ctx, "w", gh, onGitHub.ID); err != nil || len(gate.closed) != 1 {
		t.Fatalf("closing a GitHub draft = %v, closed %v", err, gate.closed)
	}
	if len(gate.merged) != 0 {
		t.Fatalf("refused merges reached the gate: %v", gate.merged)
	}
}
