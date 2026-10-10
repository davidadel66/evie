package changes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
	"github.com/davidadel66/evie/internal/memory"
)

// Sync (changes.spec.md story 3b): while evie serve runs, each Workspace
// with a GitHub Repository is synced every 60 seconds, and again when the
// owner asks. A sync lists the open pull requests, reads the checks on each
// one's head commit, and reads each pull request it last saw open that is
// no longer listed, to record a merge or close done on GitHub. GitHub
// requests per sync: 1 list + 2 per open pull request + 1 per pull request
// that left the list. The Repository is read from git alone.

// SyncEvery is how often Run syncs.
const SyncEvery = 60 * time.Second

// PullRequestClient is what sync reads from GitHub.
type PullRequestClient interface {
	PullRequests(ctx context.Context, owner, name string) ([]github.PullRequest, error)
	PullRequest(ctx context.Context, owner, name string, number int) (github.PullRequest, error)
	WorkflowRuns(ctx context.Context, owner, name, sha string) ([]github.WorkflowRun, error)
	CombinedStatus(ctx context.Context, owner, name, sha string) (github.CombinedStatus, error)
}

// PullRequestStore records each sync's pull requests and reads them back.
type PullRequestStore interface {
	PullRequestRecords
	SaveChangePullRequests(context.Context, memory.WorkspaceID, []eviedb.ChangePullRequest) error
}

// Workspaces lists the active Workspaces with their current folders.
type Workspaces interface {
	ListWorkspaces(ctx context.Context, includeArchived bool) ([]memory.Workspace, error)
}

// SyncConfig is how startup composition builds a Syncer.
type SyncConfig struct {
	// GitHub is nil without a token: a GitHub Repository then cannot sync.
	GitHub PullRequestClient
	Store  PullRequestStore
	// Workspaces is what Run syncs.
	Workspaces Workspaces
	// Every is Run's interval, SyncEvery when zero.
	Every time.Duration
	// Ticker starts Run's ticks and returns how to stop them; nil is
	// time.NewTicker. Tests tick by hand.
	Ticker func(time.Duration) (<-chan time.Time, func())
	// Logf reports a failed sync in Run; nil is log.Printf.
	Logf func(format string, args ...any)
}

// SyncResult is what one sync did.
type SyncResult struct {
	// Synced is false for a Repository not on GitHub, which has nothing to
	// sync, and when a sync of the Workspace was already running.
	Synced         bool `json:"synced"`
	AlreadyRunning bool `json:"already_running,omitempty"`
	// PullRequests is how many open pull requests the sync saw.
	PullRequests int `json:"pull_requests"`
	// Finished is how many it found merged or closed since the last sync.
	Finished int `json:"finished"`
	// Requests is how many GitHub requests it made.
	Requests int `json:"requests"`
}

// Syncer syncs Workspaces' pull requests, one sync at a time per
// Workspace.
type Syncer struct {
	github       PullRequestClient
	store        PullRequestStore
	workspaces   Workspaces
	every        time.Duration
	ticker       func(time.Duration) (<-chan time.Time, func())
	logf         func(format string, args ...any)
	repositories *Service
	mu           sync.Mutex
	running      map[memory.WorkspaceID]bool
}

func NewSyncer(c SyncConfig) *Syncer {
	s := &Syncer{github: c.GitHub, store: c.Store, workspaces: c.Workspaces, every: c.Every, ticker: c.Ticker, logf: c.Logf,
		// Without a GitHub client the Repository is read from git alone,
		// so finding what to sync makes no GitHub request.
		repositories: NewService(Config{}), running: map[memory.WorkspaceID]bool{}}
	if s.every <= 0 {
		s.every = SyncEvery
	}
	if s.ticker == nil {
		s.ticker = func(d time.Duration) (<-chan time.Time, func()) {
			t := time.NewTicker(d)
			return t.C, t.Stop
		}
	}
	if s.logf == nil {
		s.logf = log.Printf
	}
	return s
}

// Run syncs every Workspace with a GitHub Repository when it starts and
// then every interval, until ctx ends. Without a GitHub client it has
// nothing to do.
func (s *Syncer) Run(ctx context.Context) {
	if s.github == nil || s.workspaces == nil {
		return
	}
	ticks, stop := s.ticker(s.every)
	defer stop()
	for {
		s.syncAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

func (s *Syncer) syncAll(ctx context.Context) {
	workspaces, err := s.workspaces.ListWorkspaces(ctx, false)
	if err != nil {
		if ctx.Err() == nil {
			s.logf("changes sync: list Workspaces: %v", err)
		}
		return
	}
	for _, w := range workspaces {
		if ctx.Err() != nil {
			return
		}
		if w.Folder.Path == "" {
			continue
		}
		repo, err := s.repositories.Repository(ctx, w.Folder.Path)
		if err != nil || repo.Kind != GitHub {
			continue
		}
		if _, err := s.Sync(ctx, w.ID, repo); err != nil && ctx.Err() == nil {
			s.logf("changes sync: Workspace %s (%s/%s): %v", w.ID, repo.Owner, repo.Name, err)
		}
	}
}

// Refresh syncs the Workspace whose folder is folder now: what the owner's
// refresh asks for.
func (s *Syncer) Refresh(ctx context.Context, ws memory.WorkspaceID, folder string) (SyncResult, error) {
	repo, err := s.repositories.Repository(ctx, folder)
	if err != nil {
		return SyncResult{}, err
	}
	return s.Sync(ctx, ws, repo)
}

// Sync records the Workspace's pull requests on repo as GitHub has them
// now. Only a GitHub Repository syncs. While a sync of the Workspace runs,
// another returns at once with AlreadyRunning. A sync GitHub refuses
// records nothing.
func (s *Syncer) Sync(ctx context.Context, ws memory.WorkspaceID, repo Repository) (SyncResult, error) {
	if repo.Kind != GitHub {
		return SyncResult{}, nil
	}
	if s.github == nil {
		return SyncResult{}, ErrNoToken
	}
	release, ok := s.claim(ws)
	if !ok {
		return SyncResult{AlreadyRunning: true}, nil
	}
	defer release()
	gh := &countedClient{client: s.github}
	result, err := s.sync(ctx, gh, ws, repo)
	result.Requests = gh.requests
	return result, err
}

func (s *Syncer) claim(ws memory.WorkspaceID) (func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running[ws] {
		return nil, false
	}
	s.running[ws] = true
	return func() {
		s.mu.Lock()
		delete(s.running, ws)
		s.mu.Unlock()
	}, true
}

func (s *Syncer) sync(ctx context.Context, gh *countedClient, ws memory.WorkspaceID, repo Repository) (SyncResult, error) {
	var result SyncResult
	open, err := gh.PullRequests(ctx, repo.Owner, repo.Name)
	if err != nil {
		return result, fmt.Errorf("list the open pull requests of %s/%s: %w", repo.Owner, repo.Name, err)
	}
	listed := map[int]bool{}
	var records []eviedb.ChangePullRequest
	for _, p := range open {
		if listed[p.Number] {
			continue
		}
		listed[p.Number] = true
		record, err := openRecord(ctx, gh, repo, p)
		if err != nil {
			return result, err
		}
		records = append(records, record)
	}
	result.PullRequests = len(records)
	stored, err := s.store.ChangePullRequests(ctx, ws)
	if err != nil {
		return result, err
	}
	for _, old := range stored {
		if old.State != "open" || listed[old.Number] || !sameRepository(old, repo) {
			continue
		}
		p, err := gh.PullRequest(ctx, repo.Owner, repo.Name, old.Number)
		if err != nil {
			return result, fmt.Errorf("read pull request #%d: %w", old.Number, err)
		}
		state := "closed"
		switch {
		case p.Merged || p.MergedAt != nil:
			state = "merged"
		case p.State == "open":
			// Still open, though not listed (past the first hundred).
			record, err := openRecord(ctx, gh, repo, p)
			if err != nil {
				return result, err
			}
			records = append(records, record)
			result.PullRequests++
			continue
		}
		records = append(records, pullRecord(repo, p, state, old.Checks))
		result.Finished++
	}
	if err := s.store.SaveChangePullRequests(ctx, ws, records); err != nil {
		return result, fmt.Errorf("record the pull requests: %w", err)
	}
	result.Synced = true
	return result, nil
}

// openRecord is an open pull request with the checks on its head commit:
// the newest run of each GitHub Actions workflow, then its commit statuses.
// (GitHub's Checks API does not accept fine-grained tokens.)
func openRecord(ctx context.Context, gh PullRequestClient, repo Repository, p github.PullRequest) (eviedb.ChangePullRequest, error) {
	runs, err := gh.WorkflowRuns(ctx, repo.Owner, repo.Name, p.Head.SHA)
	if err != nil {
		return eviedb.ChangePullRequest{}, fmt.Errorf("read the workflow runs of pull request #%d: %w%s", p.Number, err, permissionHint(err, "Actions"))
	}
	status, err := gh.CombinedStatus(ctx, repo.Owner, repo.Name, p.Head.SHA)
	if err != nil {
		return eviedb.ChangePullRequest{}, fmt.Errorf("read the commit statuses of pull request #%d: %w%s", p.Number, err, permissionHint(err, "Commit statuses"))
	}
	var checks []eviedb.ChangePullRequestCheck
	for _, r := range newestRuns(runs) {
		checks = append(checks, eviedb.ChangePullRequestCheck{Name: r.Name, State: string(workflowRunState(r)), URL: r.HTMLURL})
	}
	for _, st := range status.Statuses {
		checks = append(checks, eviedb.ChangePullRequestCheck{Name: st.Context, State: string(statusState(st.State)), URL: st.TargetURL})
	}
	return pullRecord(repo, p, "open", checks), nil
}

func pullRecord(repo Repository, p github.PullRequest, state string, checks []eviedb.ChangePullRequestCheck) eviedb.ChangePullRequest {
	return eviedb.ChangePullRequest{Number: p.Number, Owner: repo.Owner, Repository: repo.Name, Branch: p.Head.Ref, Base: p.Base.Ref,
		Title: p.Title, Body: p.Body, Author: p.User.Login, State: state, Draft: p.Draft, HeadSHA: p.Head.SHA, Mergeable: p.Mergeable,
		URL: p.HTMLURL, Checks: checks, GitHubUpdatedAt: p.UpdatedAt}
}

// newestRuns is each workflow's newest run, in the order the workflows
// first appear: a workflow run again is judged by its latest run.
func newestRuns(runs []github.WorkflowRun) []github.WorkflowRun {
	index := map[string]int{}
	var out []github.WorkflowRun
	for _, r := range runs {
		if i, ok := index[r.Name]; ok {
			if r.ID > out[i].ID {
				out[i] = r
			}
			continue
		}
		index[r.Name] = len(out)
		out = append(out, r)
	}
	return out
}

// workflowRunState maps a workflow run. A completed run with a conclusion
// GitHub may add later, or "stale", counts as pending: it neither passes
// nor fails.
func workflowRunState(r github.WorkflowRun) CheckState {
	switch r.Status {
	case "completed":
		switch r.Conclusion {
		case "success":
			return CheckPassed
		case "failure", "timed_out", "cancelled", "action_required", "startup_failure":
			return CheckFailed
		case "neutral", "skipped":
			return CheckNeutral
		}
		return CheckPending
	case "in_progress":
		return CheckRunning
	}
	return CheckPending
}

// statusState maps a commit status: pending, success, failure or error.
func statusState(state string) CheckState {
	switch state {
	case "success":
		return CheckPassed
	case "failure", "error":
		return CheckFailed
	}
	return CheckPending
}

// countedClient counts the requests one sync makes; a sync runs on one
// goroutine.
type countedClient struct {
	client   PullRequestClient
	requests int
}

func (c *countedClient) PullRequests(ctx context.Context, owner, name string) ([]github.PullRequest, error) {
	c.requests++
	return c.client.PullRequests(ctx, owner, name)
}

func (c *countedClient) PullRequest(ctx context.Context, owner, name string, number int) (github.PullRequest, error) {
	c.requests++
	return c.client.PullRequest(ctx, owner, name, number)
}

func (c *countedClient) WorkflowRuns(ctx context.Context, owner, name, sha string) ([]github.WorkflowRun, error) {
	c.requests++
	return c.client.WorkflowRuns(ctx, owner, name, sha)
}

func (c *countedClient) CombinedStatus(ctx context.Context, owner, name, sha string) (github.CombinedStatus, error) {
	c.requests++
	return c.client.CombinedStatus(ctx, owner, name, sha)
}

// permissionHint names the token permission a forbidden read needs.
func permissionHint(err error, permission string) string {
	if !errors.Is(err, github.ErrForbidden) {
		return ""
	}
	return ". Give the token the " + permission + " permission (read) in its settings on GitHub"
}
