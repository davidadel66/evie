package changes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// Push and open a pull request (changes.spec.md story 4a): the owner
// pushes one of Evie's draft Changes in a GitHub Repository. Verify runs
// first; the owner's push goes ahead after a failure and the pull request
// says so. The pull request goes into the branch the Change was built
// from, with Evie's notes. Pushing again sends new commits and keeps the
// pull request.
//
// A workflow pushes the same way through github.open_change (story 4b),
// naming the build by its execution ID and the Repository by its resource,
// with the pull request text it gives; its push needs Verify set and
// passing.

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
	PullRequestsOnBranch(ctx context.Context, owner, name, branch string) ([]github.PullRequest, error)
	CreatePullRequest(ctx context.Context, owner, name, head, base, title, body string) (github.PullRequest, error)
}

// Resource is the Workspace Access resource that names a GitHub
// Repository to workflows: repo:<owner>/<name>.
func (r Repository) Resource() string { return "repo:" + r.Owner + "/" + r.Name }

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
	return p.push(ctx, ws, folder, push{branch: branch})
}

// PushForWorkflow pushes the draft Change built by execution and opens its
// pull request with text, for a workflow run whose step names resource. It
// refuses before anything reaches GitHub unless resource is the
// Repository's, the text is a valid title and description naming no local
// folder, and Verify is set and passes.
func (p *Pusher) PushForWorkflow(ctx context.Context, ws memory.WorkspaceID, folder, resource, execution string, text delegation.ChangeText) (PushResult, error) {
	return p.push(ctx, ws, folder, push{execution: execution, resource: resource, text: &text})
}

// FindPullRequest is the pull request, open or finished, on the branch of
// the Change built by execution, or nil: what reconciling a workflow's
// push asks.
func (p *Pusher) FindPullRequest(ctx context.Context, ws memory.WorkspaceID, folder, resource, execution string) (*github.PullRequest, error) {
	if p.GitHub == nil {
		return nil, ErrNoToken
	}
	repo, c, err := p.find(ctx, ws, folder, push{execution: execution, resource: resource})
	if err != nil {
		return nil, err
	}
	pulls, err := p.GitHub.PullRequestsOnBranch(ctx, repo.Owner, repo.Name, c.Branch)
	if err != nil || len(pulls) == 0 {
		return nil, err
	}
	return &pulls[0], nil
}

// push is one push: the owner's, of a branch, or a workflow's, of a build
// under a resource with its own text and a Verify that must pass.
type push struct {
	branch    string
	execution string
	resource  string
	text      *delegation.ChangeText
}

func (p *Pusher) push(ctx context.Context, ws memory.WorkspaceID, folder string, req push) (PushResult, error) {
	if p.GitHub == nil {
		return PushResult{}, ErrNoToken
	}
	release, err := p.Service.claim(folder)
	if err != nil {
		return PushResult{}, err
	}
	defer release()
	repo, c, err := p.find(ctx, ws, folder, req)
	if err != nil {
		return PushResult{}, err
	}
	switch {
	case c.State == Merged || c.State == Closed:
		return PushResult{}, fmt.Errorf("%w: %s is already %s", ErrNotPushable, c.Branch, c.State)
	case c.Worktree == "":
		return PushResult{}, fmt.Errorf("%w: %s's worktree was removed", ErrNotPushable, c.Branch)
	}
	if req.text != nil {
		if err := req.text.Check(); err != nil || *req.text == (delegation.ChangeText{}) {
			if err == nil {
				err = errors.New("title is blank: give the pull request a title and a description")
			}
			return PushResult{}, fmt.Errorf("%w: %v", ErrNotPushable, err)
		}
		home, _ := os.UserHomeDir()
		if path := req.text.LocalPath(c.Worktree, folder, home); path != "" {
			return PushResult{}, fmt.Errorf("%w: the title or description names the local folder %s; name files by their path in the repository", ErrNotPushable, path)
		}
		c.Title, c.Notes = strings.TrimSpace(req.text.Title), strings.TrimSpace(req.text.Description)
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
		return PushResult{}, fmt.Errorf("%w: %s was built from %s, which GitHub does not have; push %s first", ErrNotPushable, c.Branch, base, base)
	}
	var outcome *VerifyOutcome
	if p.Verifier != nil {
		if outcome, err = p.Verifier.Verify(ctx, ws, c); err != nil {
			return PushResult{}, fmt.Errorf("run Verify on %s: %w", c.Branch, err)
		}
	}
	if req.text != nil {
		// A workflow pushes only a branch that passes Verify.
		switch {
		case outcome == nil:
			return PushResult{}, fmt.Errorf("%w: a workflow pushes only a branch that passes the Workspace's Verify command, and none is set; the owner sets it with evie workspaces verify", ErrNotPushable)
		case !outcome.Passed:
			return PushResult{}, fmt.Errorf("%w: Verify failed (`%s`), and a workflow pushes only a branch that passes it. The end of its output:\n%s", ErrNotPushable, outcome.Command, strings.TrimSpace(outcome.Output))
		}
	}
	if _, err := p.Service.remoteGit(ctx, folder, "push", "origin", "refs/heads/"+c.Branch+":refs/heads/"+c.Branch); err != nil {
		return PushResult{}, fmt.Errorf("%w: pushing %s failed: %v", ErrPushFailed, c.Branch, err)
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
				if synced, ok := changeOn(list, c.Branch); ok {
					result.Change = synced
				}
			}
		}
	}
	return result, nil
}

// find reads the Repository at folder and the Change a push names: by
// branch for the owner, or by build execution under the Repository's
// resource for a workflow.
func (p *Pusher) find(ctx context.Context, ws memory.WorkspaceID, folder string, req push) (Repository, Change, error) {
	repo, list, err := p.Lister.List(ctx, ws, folder)
	if err != nil {
		return repo, Change{}, err
	}
	if repo.Kind != GitHub {
		return repo, Change{}, fmt.Errorf("%w: the Repository is not on GitHub; publish it first", ErrNotPushable)
	}
	if req.execution == "" {
		c, ok := changeOn(list, req.branch)
		if !ok {
			return repo, c, fmt.Errorf("%w: no Change of Evie's is on branch %s", ErrNotPushable, req.branch)
		}
		return repo, c, nil
	}
	if !strings.EqualFold(req.resource, repo.Resource()) {
		return repo, Change{}, fmt.Errorf("%w: the step names %s, but this Workspace's Repository is %s", ErrNotPushable, req.resource, repo.Resource())
	}
	for _, c := range list {
		if c.Author == ByEvie && c.ExecutionID == req.execution {
			return repo, c, nil
		}
	}
	return repo, Change{}, fmt.Errorf("%w: no Change of Evie's in this Workspace was built by execution %s, or a later attempt continued it", ErrNotPushable, req.execution)
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
	if err != nil && !refusedByGitHub(err) {
		// GitHub may have opened it before the answer was lost.
		err = fmt.Errorf("%w: %w", ErrOpenUncertain, err)
	}
	if err != nil {
		return github.PullRequest{}, false, fmt.Errorf("pushed %s, but opening its pull request failed: %w. Push again to retry", c.Branch, err)
	}
	return pull, false, nil
}

// ErrOpenUncertain is a request to open a pull request that failed
// without GitHub refusing it: the pull request may exist. Finding the
// branch's pull request settles it.
var ErrOpenUncertain = errors.New("the pull request may have been opened")

// refusedByGitHub is GitHub's answer that it did not do what was asked.
func refusedByGitHub(err error) bool {
	for _, refusal := range []error{github.ErrUnprocessable, github.ErrForbidden, github.ErrUnauthorized, github.ErrNotFound, github.ErrInvalidRef, github.ErrInvalidName} {
		if errors.Is(err, refusal) {
			return true
		}
	}
	return false
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
