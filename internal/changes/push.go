package changes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// Push and open a pull request (changes.spec.md story 4a): the owner
// pushes one of Evie's draft Changes in a GitHub Repository. Verify runs
// first; the owner's push goes ahead after a failure and the pull request
// says so. The pull request goes into the branch the Change was built
// from, with Evie's notes. Pushing again sends new commits and keeps the
// pull request.

// ErrNotPushable is a Change that cannot be pushed; the error says why.
var ErrNotPushable = errors.New("cannot push this Change")

// VerifyOutcome is the Workspace's Verify command run before a push.
type VerifyOutcome struct {
	Command string `json:"command"`
	Passed  bool   `json:"passed"`
	// Output is the end of the run's output.
	Output string `json:"output,omitempty"`
}

// Verifier runs the Workspace's Verify command on a build branch's
// worktree, sandboxed without network, as the merge gate does. It answers
// nil when the Workspace has no Verify command.
type Verifier interface {
	Verify(ctx context.Context, ws memory.WorkspaceID, c Change) (*VerifyOutcome, error)
}

// PullRequestOpener is what a push needs from GitHub.
type PullRequestOpener interface {
	PullRequests(ctx context.Context, owner, name string) ([]github.PullRequest, error)
	CreatePullRequest(ctx context.Context, owner, name, head, base, title, body string) (github.PullRequest, error)
}

// Pusher pushes Evie's draft Changes and opens their pull requests.
type Pusher struct {
	Lister *Lister
	// Service carries the token to git and lets one action at a time
	// change a folder's Repository.
	Service *Service
	// GitHub is nil without a token.
	GitHub PullRequestOpener
	// Verifier is nil when Verify cannot run here.
	Verifier Verifier
	// Syncer, when set, records the new pull request at once.
	Syncer interface {
		Sync(ctx context.Context, ws memory.WorkspaceID, repo Repository) (SyncResult, error)
	}
}

// PushResult is a pushed Change and its pull request.
type PushResult struct {
	Change      Change `json:"change"`
	PullRequest int    `json:"pull_request"`
	URL         string `json:"url"`
	// Existing is a pull request the branch already had.
	Existing bool           `json:"existing,omitempty"`
	Verify   *VerifyOutcome `json:"verify,omitempty"`
}

// Push pushes Evie's draft Change on branch and opens its pull request.
func (p *Pusher) Push(ctx context.Context, ws memory.WorkspaceID, folder, branch string) (PushResult, error) {
	if p.GitHub == nil {
		return PushResult{}, ErrNoToken
	}
	release, err := p.Service.claim(folder)
	if err != nil {
		return PushResult{}, err
	}
	defer release()
	repo, list, err := p.Lister.List(ctx, ws, folder)
	if err != nil {
		return PushResult{}, err
	}
	if repo.Kind != GitHub {
		return PushResult{}, fmt.Errorf("%w: the Repository is not on GitHub; publish it first", ErrNotPushable)
	}
	c, ok := changeOn(list, branch)
	switch {
	case !ok:
		return PushResult{}, fmt.Errorf("%w: no Change of Evie's is on branch %s", ErrNotPushable, branch)
	case c.State == Merged || c.State == Closed:
		return PushResult{}, fmt.Errorf("%w: %s is already %s", ErrNotPushable, branch, c.State)
	case c.Worktree == "":
		return PushResult{}, fmt.Errorf("%w: %s's worktree was removed", ErrNotPushable, branch)
	}
	base := c.Base
	if base == "" {
		base = repo.BaseBranch
	}
	onGitHub, err := p.Service.remoteGit(ctx, folder, "ls-remote", "--heads", "origin", "refs/heads/"+base)
	if err != nil {
		return PushResult{}, fmt.Errorf("read GitHub's branches: %w", err)
	}
	if onGitHub == "" {
		return PushResult{}, fmt.Errorf("%w: %s was built from %s, which GitHub does not have; push %s first", ErrNotPushable, branch, base, base)
	}
	var outcome *VerifyOutcome
	if p.Verifier != nil {
		if outcome, err = p.Verifier.Verify(ctx, ws, c); err != nil {
			return PushResult{}, fmt.Errorf("run Verify on %s: %w", branch, err)
		}
	}
	if _, err := p.Service.remoteGit(ctx, folder, "push", "origin", "refs/heads/"+branch+":refs/heads/"+branch); err != nil {
		return PushResult{}, fmt.Errorf("%w: pushing %s failed: %v", ErrPushFailed, branch, err)
	}
	pull, existing, err := p.openOrFind(ctx, repo, c, base, outcome)
	if err != nil {
		return PushResult{}, err
	}
	result := PushResult{Change: c, PullRequest: pull.Number, URL: pull.HTMLURL, Existing: existing, Verify: outcome}
	if p.Syncer != nil {
		// The pull request shows on the Change at once; a failed sync only
		// delays that to the next one.
		if _, err := p.Syncer.Sync(ctx, ws, repo); err == nil {
			if _, list, err := p.Lister.List(ctx, ws, folder); err == nil {
				if synced, ok := changeOn(list, branch); ok {
					result.Change = synced
				}
			}
		}
	}
	return result, nil
}

// openOrFind is the branch's open pull request, or a new one.
func (p *Pusher) openOrFind(ctx context.Context, repo Repository, c Change, base string, outcome *VerifyOutcome) (github.PullRequest, bool, error) {
	open, err := p.GitHub.PullRequests(ctx, repo.Owner, repo.Name)
	if err != nil {
		return github.PullRequest{}, false, err
	}
	for _, pull := range open {
		if pull.Head.Ref == c.Branch {
			return pull, true, nil
		}
	}
	pull, err := p.GitHub.CreatePullRequest(ctx, repo.Owner, repo.Name, c.Branch, base, c.Title, pullRequestBody(c, outcome))
	if err != nil {
		return github.PullRequest{}, false, fmt.Errorf("pushed %s, but opening its pull request failed: %w. Push again to retry", c.Branch, err)
	}
	return pull, false, nil
}

// pullRequestBody is Evie's notes, then where the Change came from and how
// Verify ended, with the end of its output when it failed.
func pullRequestBody(c Change, outcome *VerifyOutcome) string {
	var b strings.Builder
	if c.Notes != "" {
		b.WriteString(c.Notes)
		b.WriteString("\n\n")
	}
	b.WriteString("---\n")
	switch {
	case c.Origin != nil && c.Origin.Workflow != "":
		fmt.Fprintf(&b, "Made by Evie in workflow %s, step %s.\n", c.Origin.Workflow, c.Origin.Step)
	default:
		b.WriteString("Made by Evie from a chat.\n")
	}
	if outcome != nil {
		if outcome.Passed {
			fmt.Fprintf(&b, "\nVerify passed: `%s`\n", outcome.Command)
		} else {
			fmt.Fprintf(&b, "\nVerify failed: `%s`. Pushed anyway by the owner. The end of its output:\n\n```text\n%s\n```\n", outcome.Command, strings.TrimSpace(outcome.Output))
		}
	}
	return b.String()
}

func changeOn(list []Change, branch string) (Change, bool) {
	for _, c := range list {
		if c.Author == ByEvie && c.Branch == branch {
			return c, true
		}
	}
	return Change{}, false
}
