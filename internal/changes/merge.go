package changes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// Merge and close (changes.spec.md story 5a): the owner merges or closes
// one of Evie's Changes by its ID, from Changes or `evie changes merge|close`.
// In a local Repository a merge goes through the Kernel's merge gate: a
// Change whose gate passed against the current target merges at once, any
// other is verified, rebased and verified again first. Closing removes
// Evie's worktree and branch. Story 5b adds the same for GitHub, through
// the pull request.

// ErrNotMerged is a merge or close that did not happen; the error says
// why.
var ErrNotMerged = errors.New("nothing was merged or closed")

// BuildGate is the Kernel's merge gate acting for the owner.
type BuildGate interface {
	MergeAsOwner(ctx context.Context, ws memory.WorkspaceID, execution string) (delegation.MergeProposal, error)
	CloseAsOwner(ctx context.Context, ws memory.WorkspaceID, execution string) error
}

// Merger merges and closes a Workspace's Changes for the owner.
type Merger struct {
	Lister *Lister
	Gate   BuildGate
}

// MergeResult is a merge: the Change after it, and the gate's outcome.
type MergeResult struct {
	Change Change `json:"change"`
	// Status is the gate's: merged, or why nothing was.
	Status string                `json:"status"`
	Merged *delegation.GateMerge `json:"merged,omitempty"`
	// Verification is the gate's runs, with the end of a failed run's
	// output.
	Verification []delegation.GateRun `json:"verification,omitempty"`
	Notes        []string             `json:"notes,omitempty"`
}

// Merge merges Change id of Workspace ws, whose Repository is at folder.
func (m *Merger) Merge(ctx context.Context, ws memory.WorkspaceID, folder, id string) (MergeResult, error) {
	release, err := m.Lister.Repositories.claim(folder)
	if err != nil {
		return MergeResult{}, err
	}
	defer release()
	repo, c, err := m.find(ctx, ws, folder, id)
	if err != nil {
		return MergeResult{}, err
	}
	if repo.Kind == GitHub {
		return MergeResult{Change: c}, fmt.Errorf("%w: in a GitHub Repository a Change merges through its pull request, which Evie does not do yet", ErrNotMerged)
	}
	p, err := m.Gate.MergeAsOwner(ctx, ws, c.ExecutionID)
	result := MergeResult{Change: c, Status: p.Status, Merged: p.Merged, Verification: p.Verification, Notes: p.Notes}
	var refused *delegation.MergeGateError
	switch {
	case errors.As(err, &refused):
		return result, fmt.Errorf("%w: %s", ErrNotMerged, strings.Join(p.Notes, " "))
	case err != nil:
		return result, err
	}
	if _, list, err := m.Lister.List(ctx, ws, folder); err == nil {
		if after, ok := changeByID(list, id); ok {
			result.Change = after
		}
	}
	return result, nil
}

// Close closes Change id of Workspace ws, whose Repository is at folder:
// Evie's worktree and branch are removed and nothing is merged.
func (m *Merger) Close(ctx context.Context, ws memory.WorkspaceID, folder, id string) (Change, error) {
	release, err := m.Lister.Repositories.claim(folder)
	if err != nil {
		return Change{}, err
	}
	defer release()
	_, c, err := m.find(ctx, ws, folder, id)
	if err != nil {
		return Change{}, err
	}
	if c.PullRequest != nil {
		return c, fmt.Errorf("%w: %s has pull request #%d, which Evie does not close yet", ErrNotMerged, c.Branch, c.PullRequest.Number)
	}
	if err := m.Gate.CloseAsOwner(ctx, ws, c.ExecutionID); err != nil {
		return c, err
	}
	if _, list, err := m.Lister.List(ctx, ws, folder); err == nil {
		if after, ok := changeByID(list, id); ok {
			return after, nil
		}
	}
	return c, nil
}

// find is the Repository at folder and its unfinished Change of Evie's
// named id.
func (m *Merger) find(ctx context.Context, ws memory.WorkspaceID, folder, id string) (Repository, Change, error) {
	repo, list, err := m.Lister.List(ctx, ws, folder)
	if err != nil {
		return repo, Change{}, err
	}
	c, ok := changeByID(list, id)
	switch {
	case !ok:
		return repo, c, fmt.Errorf("%w: this Workspace has no Change %s", ErrNotMerged, id)
	case c.Author != ByEvie || c.ExecutionID == "":
		return repo, c, fmt.Errorf("%w: %s is a pull request of the owner's, which Evie does not merge or close yet", ErrNotMerged, id)
	case c.State == Merged || c.State == Closed:
		return repo, c, fmt.Errorf("%w: %s is already %s", ErrNotMerged, id, c.State)
	}
	return repo, c, nil
}

func changeByID(list []Change, id string) (Change, bool) {
	for _, c := range list {
		if c.ID == id {
			return c, true
		}
	}
	return Change{}, false
}
