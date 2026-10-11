package changes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/worktree"
)

// Merge and close (changes.spec.md story 5): the owner merges or closes
// one of a Workspace's Changes by its ID, from Changes or `evie changes
// merge|close`. In a local Repository a merge goes through the Kernel's
// merge gate: a Change whose gate passed against the current target merges
// at once, any other is verified, rebased and verified again first (story
// 5a). In a GitHub Repository a Change merges through its pull request,
// with GitHub's "Rebase and merge" of the head the owner saw, and the
// owner's own pull requests merge too (story 5b). After any merge, by Evie
// or observed on GitHub, Evie removes its own branch and worktree and
// brings the owner's base branch up to date when it is checked out with no
// uncommitted edits. Closing removes Evie's worktree and branch, and
// closes the pull request; the owner's branches are never deleted.

// ErrNotMerged is a merge or close that did not happen; the error says
// why.
var ErrNotMerged = errors.New("nothing was merged or closed")

var (
	// errNeedsRebase is a merge GitHub refused for a conflict.
	errNeedsRebase = errors.New("it conflicts")
	// errNewCommits is a merge of a head the pull request no longer has.
	errNewCommits = errors.New("new commits arrived")
)

// BuildGate is the Kernel's merge gate acting for the owner.
type BuildGate interface {
	MergeAsOwner(ctx context.Context, ws memory.WorkspaceID, execution string) (delegation.MergeProposal, error)
	CloseAsOwner(ctx context.Context, ws memory.WorkspaceID, execution string) error
}

// PullRequestMerger is what merging and closing on GitHub needs.
type PullRequestMerger interface {
	PullRequest(ctx context.Context, owner, name string, number int) (github.PullRequest, error)
	MergePullRequest(ctx context.Context, owner, name string, number int, sha string) (string, error)
	ClosePullRequest(ctx context.Context, owner, name string, number int) error
}

// MergeRecords is what merging keeps: a conflict GitHub reports (story
// 5b), "Merge when checks pass" and the notices it leaves (story 5c).
type MergeRecords interface {
	RecordChangeMergeable(ctx context.Context, ws memory.WorkspaceID, number int, head string, mergeable bool) error
	ArmChangeMerge(ctx context.Context, ws memory.WorkspaceID, number int) error
	DisarmChangeMerge(ctx context.Context, ws memory.WorkspaceID, number int) (bool, error)
	RecordChangeNotification(ctx context.Context, n eviedb.ChangeNotification) error
}

// Merger merges and closes a Workspace's Changes for the owner.
type Merger struct {
	Lister *Lister
	Gate   BuildGate
	// GitHub is nil without a token: a GitHub Repository's Changes then
	// cannot merge or close.
	GitHub  PullRequestMerger
	Records MergeRecords
	// Syncer, when set, records a merge or close done on GitHub at once.
	Syncer interface {
		Sync(ctx context.Context, ws memory.WorkspaceID, repo Repository) (SyncResult, error)
	}
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
		return m.mergeOnGitHub(ctx, ws, folder, repo, c)
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
	result.Change = m.after(ctx, ws, folder, c)
	return result, nil
}

// CloseResult is a close: the Change after it, and what was cleaned up.
type CloseResult struct {
	Change Change   `json:"change"`
	Notes  []string `json:"notes,omitempty"`
}

// Close closes Change id of Workspace ws, whose Repository is at folder:
// its pull request is closed on GitHub, Evie's worktree and branch are
// removed, and nothing is merged.
func (m *Merger) Close(ctx context.Context, ws memory.WorkspaceID, folder, id string) (CloseResult, error) {
	release, err := m.Lister.Repositories.claim(folder)
	if err != nil {
		return CloseResult{}, err
	}
	defer release()
	repo, c, err := m.find(ctx, ws, folder, id)
	if err != nil {
		return CloseResult{}, err
	}
	if repo.Kind != GitHub {
		if err := m.Gate.CloseAsOwner(ctx, ws, c.ExecutionID); err != nil {
			return CloseResult{Change: c}, err
		}
		return CloseResult{Change: m.after(ctx, ws, folder, c), Notes: []string{"Evie's worktree and branch were removed."}}, nil
	}
	var notes []string
	if p := c.PullRequest; p != nil && p.State == "open" {
		if m.GitHub == nil {
			return CloseResult{Change: c}, ErrNoToken
		}
		if err := m.GitHub.ClosePullRequest(ctx, repo.Owner, repo.Name, p.Number); err != nil {
			return CloseResult{Change: c}, gitHubRefusal(err)
		}
		notes = append(notes, fmt.Sprintf("Pull request #%d was closed on GitHub.", p.Number))
	}
	notes = append(notes, m.cleanUp(ctx, ws, folder, repo, c)...)
	m.sync(ctx, ws, repo)
	return CloseResult{Change: m.after(ctx, ws, folder, c), Notes: notes}, nil
}

// mergeOnGitHub merges c's pull request with GitHub's "Rebase and merge",
// only at the head Evie last showed, then cleans up after it.
func (m *Merger) mergeOnGitHub(ctx context.Context, ws memory.WorkspaceID, folder string, repo Repository, c Change) (MergeResult, error) {
	result := MergeResult{Change: c}
	pr := c.PullRequest
	switch {
	case m.GitHub == nil:
		return result, ErrNoToken
	case pr == nil || pr.State != "open":
		return result, fmt.Errorf("%w: %s has no open pull request; push it first with evie changes push", ErrNotMerged, c.ID)
	}
	pull, err := m.GitHub.PullRequest(ctx, repo.Owner, repo.Name, pr.Number)
	if err != nil {
		return result, gitHubRefusal(err)
	}
	switch {
	case pull.State != "open":
		m.sync(ctx, ws, repo)
		return result, fmt.Errorf("%w: pull request #%d is already %s on GitHub", ErrNotMerged, pr.Number, pull.State)
	case pull.Draft:
		return result, fmt.Errorf("%w: GitHub marks pull request #%d as a draft; mark it ready for review there first", ErrNotMerged, pr.Number)
	case pull.Head.SHA != pr.HeadSHA:
		m.sync(ctx, ws, repo)
		return result, fmt.Errorf("%w: %w: pull request #%d has new commits since Evie last read it; review them before merging", ErrNotMerged, errNewCommits, pr.Number)
	case pull.Mergeable != nil && !*pull.Mergeable:
		return result, m.needsRebase(ctx, ws, repo, c)
	}
	merged, err := m.GitHub.MergePullRequest(ctx, repo.Owner, repo.Name, pr.Number, pr.HeadSHA)
	switch {
	case errors.Is(err, github.ErrNotMergeable) && strings.Contains(strings.ToLower(err.Error()), "not mergeable"):
		return result, m.needsRebase(ctx, ws, repo, c)
	case errors.Is(err, github.ErrHeadChanged):
		m.sync(ctx, ws, repo)
		return result, fmt.Errorf("%w: %w: pull request #%d has new commits since Evie last read it; review them before merging", ErrNotMerged, errNewCommits, pr.Number)
	case err != nil:
		return result, gitHubRefusal(err)
	}
	result.Status = delegation.MergeMerged
	result.Merged = &delegation.GateMerge{Target: c.Base, To: merged, Where: fmt.Sprintf("GitHub's rebase merge of pull request #%d", pr.Number)}
	result.Notes = append([]string{fmt.Sprintf("GitHub merged pull request #%d into %s by rebase.", pr.Number, c.Base)}, m.cleanUp(ctx, ws, folder, repo, c)...)
	result.Notes = append(result.Notes, m.catchUp(ctx, folder, c.Base))
	m.sync(ctx, ws, repo)
	result.Change = m.after(ctx, ws, folder, c)
	return result, nil
}

// needsRebase records that GitHub cannot merge c's head and says so.
func (m *Merger) needsRebase(ctx context.Context, ws memory.WorkspaceID, repo Repository, c Change) error {
	if m.Records != nil {
		_ = m.Records.RecordChangeMergeable(ctx, ws, c.PullRequest.Number, c.PullRequest.HeadSHA, false)
	}
	return fmt.Errorf("%w: %w: pull request #%d conflicts with %s and needs a rebase; once its branch is rebased and pushed, merge it again", ErrNotMerged, errNeedsRebase, c.PullRequest.Number, c.Base)
}

// gitHubRefusal marks GitHub's refusal of a merge or close as one that did
// not happen, with GitHub's reason; any other error is left as it is.
func gitHubRefusal(err error) error {
	if refusedByGitHub(err) || errors.Is(err, github.ErrNotMergeable) || errors.Is(err, github.ErrHeadChanged) {
		return fmt.Errorf("%w: %w", ErrNotMerged, err)
	}
	return err
}

// cleanUp removes Evie's own branch, on GitHub and locally, and its
// worktree after c merged or closed, and says what it did. The owner's
// branches are never deleted.
func (m *Merger) cleanUp(ctx context.Context, ws memory.WorkspaceID, folder string, repo Repository, c Change) []string {
	if c.Author != ByEvie || !strings.HasPrefix(c.Branch, "evie/") {
		return nil
	}
	var notes []string
	if repo.Kind == GitHub {
		svc := m.Lister.Repositories
		ref := "refs/heads/" + c.Branch
		if out, err := svc.remoteGit(ctx, folder, "ls-remote", "--heads", "origin", ref); err == nil && out != "" {
			if _, err := svc.remoteGit(ctx, folder, "push", "--quiet", "origin", "--delete", ref); err != nil {
				notes = append(notes, fmt.Sprintf("Evie's branch %s could not be deleted on GitHub: %v.", c.Branch, err))
			} else {
				notes = append(notes, fmt.Sprintf("Evie's branch %s was deleted on GitHub.", c.Branch))
			}
		}
	}
	if c.Worktree != "" && c.ExecutionID != "" {
		if err := m.Gate.CloseAsOwner(ctx, ws, c.ExecutionID); err != nil {
			notes = append(notes, fmt.Sprintf("Evie's worktree and local branch were kept: %v.", err))
		} else {
			notes = append(notes, "Evie's worktree and local branch were removed.")
		}
	}
	return notes
}

// catchUp fetches the base branch and fast-forwards it in the owner's
// checkout only when the checkout is on it with no uncommitted edits;
// otherwise the Repository shows how far behind it is.
func (m *Merger) catchUp(ctx context.Context, folder, base string) string {
	if base == "" {
		return "The base branch is unknown, so nothing was brought up to date."
	}
	tracking := "refs/remotes/origin/" + base
	if _, err := m.Lister.Repositories.remoteGit(ctx, folder, "fetch", "--quiet", "origin", "refs/heads/"+base+":"+tracking); err != nil {
		return fmt.Sprintf("Fetching %s failed (%v), so your checkout was left as it was.", base, err)
	}
	current, _ := worktree.Git(ctx, folder, "symbolic-ref", "--short", "-q", "HEAD")
	if current != base {
		return fmt.Sprintf("Your checkout is on %s, so %s was not fast-forwarded there.", current, base)
	}
	if status, err := worktree.Git(ctx, folder, "status", "--porcelain", "--untracked-files=no"); err != nil || status != "" {
		return fmt.Sprintf("Your checkout has uncommitted edits, so %s was not fast-forwarded there.", base)
	}
	if _, err := worktree.Git(ctx, folder, "merge", "--ff-only", "--quiet", tracking); err != nil {
		return fmt.Sprintf("%s could not be fast-forwarded in your checkout: %v.", base, err)
	}
	return fmt.Sprintf("%s was fast-forwarded in your checkout.", base)
}

// AfterSync cleans up after merges a sync of a GitHub Repository observed:
// Evie's branch and worktree of each of its Changes merged on GitHub, then
// the base branch each merged into, or the default branch when a pull
// request merged or closed. It skips a folder another action holds; the
// next sync tries again.
func (m *Merger) AfterSync(ctx context.Context, ws memory.WorkspaceID, synced Repository, result SyncResult) {
	if synced.Kind != GitHub || synced.Folder == "" {
		return
	}
	release, err := m.Lister.Repositories.claim(synced.Folder)
	if err != nil {
		return
	}
	defer release()
	// Read again with GitHub, whose default branch is the base.
	repo, list, err := m.Lister.List(ctx, ws, synced.Folder)
	if err != nil || repo.Kind != GitHub {
		return
	}
	bases := map[string]bool{}
	for _, c := range list {
		if c.State == Merged && c.Author == ByEvie && c.Worktree != "" {
			m.cleanUp(ctx, ws, repo.Folder, repo, c)
			bases[c.Base] = true
		}
	}
	if result.Finished > 0 {
		bases[repo.BaseBranch] = true
	}
	for base := range bases {
		m.catchUp(ctx, repo.Folder, base)
	}
	for _, c := range list {
		if c.ArmedAt != nil && c.State == Open && c.PullRequest != nil {
			m.fire(ctx, ws, repo, c)
		}
	}
}

// Arm sets "Merge when checks pass" on Change id's pull request (story
// 5c): each sync then merges it once at least one check exists and every
// check passed, and clears it when a check fails or the merge conflicts.
// It cannot be set while the pull request has no checks, a failed one,
// GitHub's draft marking or a conflict.
func (m *Merger) Arm(ctx context.Context, ws memory.WorkspaceID, folder, id string) (Change, error) {
	release, err := m.Lister.Repositories.claim(folder)
	if err != nil {
		return Change{}, err
	}
	defer release()
	repo, c, err := m.find(ctx, ws, folder, id)
	if err != nil {
		return Change{}, err
	}
	pr := c.PullRequest
	switch {
	case repo.Kind != GitHub:
		return c, fmt.Errorf("%w: \"Merge when checks pass\" is for pull requests on GitHub; merge %s now instead", ErrNotMerged, id)
	case m.Records == nil || m.GitHub == nil:
		return c, ErrNoToken
	case pr == nil || pr.State != "open":
		return c, fmt.Errorf("%w: %s has no open pull request; push it first with evie changes push", ErrNotMerged, id)
	case pr.Draft:
		return c, fmt.Errorf("%w: GitHub marks pull request #%d as a draft; mark it ready for review there first", ErrNotMerged, pr.Number)
	case c.NeedsRebase:
		return c, fmt.Errorf("%w: pull request #%d needs a rebase first", ErrNotMerged, pr.Number)
	case len(c.Checks) == 0:
		return c, fmt.Errorf("%w: pull request #%d has no checks yet, so nothing would tell Evie when to merge it; merge it now, or set this once its checks appear", ErrNotMerged, pr.Number)
	}
	if failed, ok := failedCheck(c.Checks); ok {
		return c, fmt.Errorf("%w: the check %s failed on pull request #%d; fix it first", ErrNotMerged, failed, pr.Number)
	}
	if err := m.Records.ArmChangeMerge(ctx, ws, pr.Number); err != nil {
		return c, err
	}
	return m.after(ctx, ws, folder, c), nil
}

// Disarm clears "Merge when checks pass" on Change id's pull request.
func (m *Merger) Disarm(ctx context.Context, ws memory.WorkspaceID, folder, id string) (Change, error) {
	_, list, err := m.Lister.List(ctx, ws, folder)
	if err != nil {
		return Change{}, err
	}
	c, ok := changeByID(list, id)
	if !ok || c.ArmedAt == nil || c.PullRequest == nil || m.Records == nil {
		return c, fmt.Errorf("%w: \"Merge when checks pass\" is not set on %s", ErrNotMerged, id)
	}
	if _, err := m.Records.DisarmChangeMerge(ctx, ws, c.PullRequest.Number); err != nil {
		return c, err
	}
	return m.after(ctx, ws, folder, c), nil
}

// fire acts on one armed Change after a sync: a failed check clears it, a
// head whose checks all passed merges, and anything else waits. A merge
// GitHub refuses for good clears it; new commits or an error that may pass
// leave it set for the next sync. Each outcome but waiting leaves a notice.
func (m *Merger) fire(ctx context.Context, ws memory.WorkspaceID, repo Repository, c Change) {
	pr := c.PullRequest
	clear := func(kind eviedb.WorkflowNotificationKind, event, detail string) {
		if _, err := m.Records.DisarmChangeMerge(ctx, ws, pr.Number); err != nil {
			return
		}
		_ = m.Records.RecordChangeNotification(ctx, eviedb.ChangeNotification{WorkspaceID: ws, ChangeID: c.ID, Kind: kind,
			Key: c.ID + ":" + event + ":" + pr.HeadSHA, Detail: detail})
	}
	if failed, ok := failedCheck(c.Checks); ok {
		clear(eviedb.ChangeNotifyCleared, "failed:"+failed, fmt.Sprintf("\"Merge when checks pass\" was cleared on pull request #%d, %s: the check %s failed.", pr.Number, c.Title, failed))
		return
	}
	if !checksPassed(c.Checks) {
		return
	}
	_, err := m.mergeOnGitHub(ctx, ws, repo.Folder, repo, c)
	switch {
	case err == nil:
		clear(eviedb.ChangeNotifyMerged, "merged", fmt.Sprintf("Pull request #%d, %s, merged into %s once its checks passed.", pr.Number, c.Title, c.Base))
	case errors.Is(err, errNeedsRebase):
		clear(eviedb.ChangeNotifyNeedsRebase, "needs_rebase", fmt.Sprintf("Pull request #%d, %s, passed its checks but conflicts with %s and needs a rebase; \"Merge when checks pass\" was cleared.", pr.Number, c.Title, c.Base))
	case errors.Is(err, errNewCommits) || !errors.Is(err, ErrNotMerged):
		// The next sync reads the new head's checks, or tries again.
	default:
		clear(eviedb.ChangeNotifyCleared, "refused", fmt.Sprintf("\"Merge when checks pass\" was cleared on pull request #%d, %s: %v", pr.Number, c.Title, err))
	}
}

// failedCheck is the first failed check, if any.
func failedCheck(checks []Check) (string, bool) {
	for _, check := range checks {
		if check.State == CheckFailed {
			return check.Name, true
		}
	}
	return "", false
}

// checksPassed says whether at least one check exists and every check
// finished without failing.
func checksPassed(checks []Check) bool {
	for _, check := range checks {
		if check.State != CheckPassed && check.State != CheckNeutral {
			return false
		}
	}
	return len(checks) > 0
}

func (m *Merger) sync(ctx context.Context, ws memory.WorkspaceID, repo Repository) {
	if m.Syncer != nil {
		_, _ = m.Syncer.Sync(ctx, ws, repo)
	}
}

// after is c as the list shows it now.
func (m *Merger) after(ctx context.Context, ws memory.WorkspaceID, folder string, c Change) Change {
	if _, list, err := m.Lister.List(ctx, ws, folder); err == nil {
		if now, ok := changeByID(list, c.ID); ok {
			return now
		}
	}
	return c
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
	case repo.Kind != GitHub && c.ExecutionID == "":
		return repo, c, fmt.Errorf("%w: %s is not one of Evie's build branches", ErrNotMerged, id)
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
