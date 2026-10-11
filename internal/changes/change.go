package changes

import (
	"context"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// A Change is one branch of work in a Workspace's Repository, followed from
// draft through review to merged or closed (changes.spec.md, story 3). Two
// sources produce them: Evie's build branches, read from the build
// attempts that already record them (story 3a), and pull requests on
// GitHub, synced into Evie's database (story 3b). List merges the two.

// State is where a Change is in its lifecycle: draft → open → merged or
// closed, and draft → closed.
type State string

const (
	// Draft is a branch that has not passed the merge gate (local) or has
	// no pull request (GitHub).
	Draft State = "draft"
	// Open is a branch that passed the merge gate and awaits review (local)
	// or has an open pull request (GitHub).
	Open   State = "open"
	Merged State = "merged"
	Closed State = "closed"
)

// Author is who made a Change.
type Author string

const (
	ByEvie  Author = "evie"
	ByOwner Author = "owner"
)

// Origin is what produced one of Evie's Changes: a chat session, or a
// workflow run's step.
type Origin struct {
	SessionID string `json:"session_id,omitempty"`
	RunID     string `json:"run_id,omitempty"`
	Workflow  string `json:"workflow,omitempty"`
	Step      string `json:"step,omitempty"`
}

// PullRequest is a Change's pull request on GitHub.
type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	// State is GitHub's: open, merged or closed.
	State string `json:"state"`
	// Draft is GitHub's own draft marking, which blocks merging.
	Draft     bool      `json:"draft,omitempty"`
	HeadSHA   string    `json:"head_sha"`
	Author    string    `json:"author"`
	Mergeable *bool     `json:"mergeable,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CheckState is one check's outcome on a Change's latest commit.
type CheckState string

const (
	CheckPending CheckState = "pending"
	CheckRunning CheckState = "running"
	CheckPassed  CheckState = "passed"
	CheckFailed  CheckState = "failed"
	// CheckNeutral covers neutral and skipped results, which do not fail.
	CheckNeutral CheckState = "neutral"
)

// Check is one check run or commit status on a Change's latest commit.
type Check struct {
	Name  string     `json:"name"`
	State CheckState `json:"state"`
	URL   string     `json:"url,omitempty"`
}

// Change is one branch of work in a Workspace's Repository.
type Change struct {
	// ID is "build:<branch>" for Evie's build branches (stable across
	// continuation attempts) and "pr:<number>" for a pull request no build
	// branch matches.
	ID          string             `json:"id"`
	WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	Branch      string             `json:"branch"`
	Base        string             `json:"base"`
	State       State              `json:"state"`
	NeedsRebase bool               `json:"needs_rebase,omitempty"`
	Author      Author             `json:"author"`
	Origin      *Origin            `json:"origin,omitempty"`
	Title       string             `json:"title"`
	Notes       string             `json:"notes,omitempty"`
	Head        string             `json:"head,omitempty"`
	// Worktree is the build worktree's path while it exists.
	Worktree     string `json:"worktree,omitempty"`
	FilesChanged int    `json:"files_changed,omitempty"`
	Insertions   int    `json:"insertions,omitempty"`
	Deletions    int    `json:"deletions,omitempty"`
	// ExecutionID is Evie's build attempt.
	ExecutionID string       `json:"execution_id,omitempty"`
	PullRequest *PullRequest `json:"pull_request,omitempty"`
	Checks      []Check      `json:"checks,omitempty"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Source reads one kind of a Workspace's Changes.
type Source interface {
	Changes(ctx context.Context, ws memory.WorkspaceID, repo Repository) ([]Change, error)
}

// Lister lists a Workspace's Changes from its sources. A nil source
// contributes nothing.
type Lister struct {
	Repositories *Service
	// Builds is Evie's build branches (story 3a).
	Builds Source
	// PullRequests is the synced pull requests of a GitHub Repository
	// (story 3b); a local Repository has none.
	PullRequests Source
}

// finishedKept is how many merged or closed Changes a list keeps.
const finishedKept = 20

// List reads the Workspace's Repository from its folder and merges its
// Changes: unfinished ones first, most recently updated first, then the
// 20 most recently finished.
func (l *Lister) List(ctx context.Context, ws memory.WorkspaceID, folder string) (Repository, []Change, error) {
	repo, err := l.Repositories.Repository(ctx, folder)
	if err != nil || repo.Kind == None {
		return repo, []Change{}, err
	}
	var all []Change
	if l.Builds != nil {
		builds, err := l.Builds.Changes(ctx, ws, repo)
		if err != nil {
			return repo, nil, err
		}
		all = append(all, builds...)
	}
	if l.PullRequests != nil && repo.Kind == GitHub {
		pulls, err := l.PullRequests.Changes(ctx, ws, repo)
		if err != nil {
			return repo, nil, err
		}
		all = link(all, pulls)
	}
	return repo, ordered(all), nil
}

// link joins each pull request to Evie's build Change on the same branch:
// that Change takes the pull request, its checks, its head and GitHub's
// state, and keeps Evie's title, notes and origin. Of several pull
// requests on one branch, the open one, else the newest, is the branch's.
// Any other pull request is a Change of its own.
func link(builds, pulls []Change) []Change {
	onBranch := map[string]int{}
	for i, c := range builds {
		onBranch[c.Branch] = i
	}
	chosen := map[string]Change{}
	var own []Change
	for _, p := range pulls {
		if _, ok := onBranch[p.Branch]; !ok || p.Branch == "" {
			own = append(own, p)
			continue
		}
		if current, ok := chosen[p.Branch]; !ok || preferred(p, current) {
			chosen[p.Branch] = p
		}
	}
	for branch, p := range chosen {
		c := &builds[onBranch[branch]]
		c.State, c.PullRequest, c.Checks = p.State, p.PullRequest, p.Checks
		c.NeedsRebase = c.NeedsRebase || p.NeedsRebase
		if p.Head != "" {
			c.Head = p.Head
		}
		if p.Base != "" {
			c.Base = p.Base
		}
		if p.UpdatedAt.After(c.UpdatedAt) {
			c.UpdatedAt = p.UpdatedAt
		}
	}
	return append(builds, own...)
}

// preferred says whether pull request p is its branch's over current: an
// open one over a finished one, else the newer.
func preferred(p, current Change) bool {
	if (p.State == Open) != (current.State == Open) {
		return p.State == Open
	}
	return p.UpdatedAt.After(current.UpdatedAt)
}

func ordered(all []Change) []Change {
	unfinished, finished := []Change{}, []Change{}
	for _, c := range all {
		if c.State == Merged || c.State == Closed {
			finished = append(finished, c)
		} else {
			unfinished = append(unfinished, c)
		}
	}
	newest := func(list []Change) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].UpdatedAt.After(list[j].UpdatedAt) })
	}
	newest(unfinished)
	newest(finished)
	if len(finished) > finishedKept {
		finished = finished[:finishedKept]
	}
	return append(unfinished, finished...)
}
