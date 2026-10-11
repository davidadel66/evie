package changes_test

import (
	"context"
	"database/sql"
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

// buildRecords is a real Evie database holding build attempts as the
// harness records them: one row per attempt, its worktree pinned in the
// record.
type buildRecords struct {
	t     *testing.T
	db    *sql.DB
	store *eviedb.Store
	clock time.Time
}

func newBuildRecords(t *testing.T) *buildRecords {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := &buildRecords{t: t, db: db, store: eviedb.NewStore(db), clock: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	r.store.UseClock(func() time.Time { return r.clock }, time.UTC)
	return r
}

// build records a finished build attempt delegated from a chat in ws (no
// Workspace when ws is empty), with its worktree in repo; change adjusts
// the record before it is stored.
func (r *buildRecords) build(ws memory.WorkspaceID, repo, objective string, change func(*delegation.Attempt)) delegation.Attempt {
	r.t.Helper()
	ctx := context.Background()
	parent, err := r.store.CreateGlobalSession(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	child, err := r.store.CreateGlobalSession(ctx)
	if err != nil {
		r.t.Fatal(err)
	}
	id := uuid.NewString()
	ended := r.clock
	a := delegation.Attempt{ID: id, State: "succeeded", CreatedAt: r.clock.Add(-time.Hour), EndedAt: &ended, Child: child,
		Parent:     delegation.Parent{Scope: memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: parent.ID, WorkspaceID: ws}},
		Assignment: delegation.Assignment{Key: "key-" + id, Objective: objective},
		Worktree: &delegation.Worktree{Repo: repo, Path: filepath.Join(repo, ".evie", "worktrees", id), Temp: filepath.Join(repo, ".evie", "tmp", id),
			Branch: "evie/" + id[:8] + "-work", Base: "main", BaseCommit: "base0"}}
	if change != nil {
		change(&a)
	}
	r.insert(a)
	return a
}

func (r *buildRecords) insert(a delegation.Attempt) {
	r.t.Helper()
	record, err := json.Marshal(a)
	if err != nil {
		r.t.Fatal(err)
	}
	parent := a.Parent.Scope.SessionID
	if _, err := r.db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,?,?)`,
		a.ID, parent, a.Assignment.Key, "digest", a.Child.ID, a.State, string(record)); err != nil {
		r.t.Fatal(err)
	}
}

// gate records a merge-gate step on execution id, at the clock's time.
func (r *buildRecords) gate(id string, update func(*delegation.BuildGate)) {
	r.t.Helper()
	if _, err := r.store.RecordBuildGate(context.Background(), id, update); err != nil {
		r.t.Fatal(err)
	}
}

func (r *buildRecords) remove(id string) {
	r.t.Helper()
	if err := r.store.MarkBuildWorktreeRemoved(context.Background(), id); err != nil {
		r.t.Fatal(err)
	}
}

func reported(summary string, limitations []string, subjects ...string) func(*delegation.Attempt) {
	return func(a *delegation.Attempt) {
		report := &delegation.BuildReport{Branch: a.Worktree.Branch, Base: a.Worktree.Base, Head: "head-" + a.ID[:8], FilesChanged: 3, Insertions: 40, Deletions: 7}
		for i, subject := range subjects {
			report.Commits = append(report.Commits, delegation.BuildCommit{SHA: strings.Repeat(string(rune('a'+i)), 40), Subject: subject})
		}
		a.Result = &delegation.Result{ExecutionID: a.ID, Status: a.State, Summary: summary, Limitations: limitations, Build: report,
			Notes: []string{"harness note that is not the owner's to read here"}}
	}
}

// changeByID finds the Change read from build attempt id; the Change
// itself is named by its branch.
func changeByID(t *testing.T, list []changes.Change, id string) changes.Change {
	t.Helper()
	for _, c := range list {
		if c.ExecutionID == id {
			if c.ID != "build:"+c.Branch {
				t.Fatalf("Change %q is not named by its branch %q", c.ID, c.Branch)
			}
			return c
		}
	}
	t.Fatalf("no Change from attempt %s in %+v", id, list)
	return changes.Change{}
}

// changes.spec.md story 3a: Evie's Changes are read from the build
// attempts that already record their branch, base, origin, gate and
// worktree removal. Each build worktree of the Workspace in its Repository
// is one Change, from its latest attempt: draft until it passes the merge
// gate, open while it awaits the owner, then merged or closed.
func TestBuildChangesFollowTheBuildRecordsOfTheWorkspaceRepository(t *testing.T) {
	ctx := context.Background()
	r := newBuildRecords(t)
	repo, elsewhere := repository(t, "main"), repository(t, "main")
	ws, other := memory.WorkspaceID("ws-code"), memory.WorkspaceID("ws-other")

	draft := r.build(ws, repo, "Add the login page\nwith a form", reported("Added the login page.", []string{"No tests for OAuth.", "Styles are rough."}, "Add the login page"))
	open := r.build(ws, repo, "Fix the footer", reported("", nil, "Fix the footer"))
	r.gate(open.ID, func(g *delegation.BuildGate) {
		g.Step, g.Target, g.TargetHead, g.BranchHead = delegation.GateAwaiting, "main", "main1", "verified-head"
	})
	moved := r.build(ws, repo, "Rename the module", reported("", nil, "Rename the module"))
	r.gate(moved.ID, func(g *delegation.BuildGate) {
		g.NeedsRebase = &delegation.TargetMove{Target: "main", From: "main0", To: "main1", ByExecution: open.ID}
	})
	conflict := r.build(ws, repo, "Touch the same lines", reported("", nil, "Touch the same lines"))
	r.gate(conflict.ID, func(g *delegation.BuildGate) { g.Step, g.Target = delegation.GateRebaseConflict, "release" })
	merged := r.build(ws, repo, "Ship it", reported("", nil, "Ship it"))
	r.clock = r.clock.Add(time.Minute)
	r.gate(merged.ID, func(g *delegation.BuildGate) {
		g.Step, g.Target = delegation.GateMerged, "main"
		g.Merged = &delegation.GateMerge{Target: "main", From: "main0", To: "merged-head", Where: "fast-forward", At: r.clock}
	})
	r.clock = r.clock.Add(time.Minute)
	r.remove(merged.ID)
	closed := r.build(ws, repo, "Abandoned idea", reported("", nil, "Abandoned idea"))
	r.clock = r.clock.Add(time.Minute)
	r.remove(closed.ID)
	removedAt := r.clock

	// A workflow build worker belongs to its run's Workspace and names the
	// run and step it came from; still running, it has no report yet.
	long := "Implement the " + strings.Repeat("very ", 20) + "long feature\nsecond line"
	workflow := r.build("", repo, long, func(a *delegation.Attempt) {
		a.State, a.EndedAt = "running", nil
		a.Run = &delegation.RunOwner{RunID: "run-1", WorkspaceID: ws, Workflow: "fix-bugs", Step: "build"}
	})

	// A continuation shares its original's worktree: the latest attempt is
	// the Change, and its title is the first commit made on the branch.
	original := r.build(ws, repo, "Write the parser", reported("", nil, "Write the parser"))
	r.gate(original.ID, func(g *delegation.BuildGate) { g.Step = delegation.GateVerifyFailed })
	continued := r.build(ws, repo, "Fix the failing parser test", func(a *delegation.Attempt) {
		a.Child, a.Worktree, a.Parent = original.Child, original.Worktree, original.Parent
		a.Continues = &delegation.Continues{ExecutionID: original.ID}
		reported("Parser fixed.", nil, "Continue: Fix the failing parser test", "Write the parser")(a)
	})

	// Not this Workspace's Changes: another Workspace's build in the same
	// folder, this Workspace's build in another folder, and a build from a
	// chat outside any Workspace.
	r.build(other, repo, "Other Workspace", nil)
	r.build(ws, elsewhere, "Another folder", nil)
	r.build("", repo, "No Workspace", nil)

	lister := &changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: changes.NewBuildSource(r.store)}
	found, list, err := lister.List(ctx, ws, repo)
	if err != nil || found.Kind != changes.Local {
		t.Fatalf("list = %+v, %v", found, err)
	}
	if len(list) != 8 {
		t.Fatalf("Changes = %d: %+v", len(list), list)
	}

	c := changeByID(t, list, draft.ID)
	if c.State != changes.Draft || c.NeedsRebase || c.Author != changes.ByEvie || c.WorkspaceID != ws || c.ExecutionID != draft.ID ||
		c.Origin == nil || *c.Origin != (changes.Origin{SessionID: string(draft.Parent.Scope.SessionID)}) ||
		c.Branch != draft.Worktree.Branch || c.Base != "main" || c.Head != "head-"+draft.ID[:8] || c.Worktree != draft.Worktree.Path ||
		c.FilesChanged != 3 || c.Insertions != 40 || c.Deletions != 7 || !c.UpdatedAt.Equal(*draft.EndedAt) {
		t.Fatalf("draft = %+v", c)
	}
	if c.Title != "Add the login page" || c.Notes != "Added the login page.\n\nLimitations:\n- No tests for OAuth.\n- Styles are rough." {
		t.Fatalf("draft title and notes = %q %q", c.Title, c.Notes)
	}

	if c := changeByID(t, list, open.ID); c.State != changes.Open || c.NeedsRebase || c.Head != "verified-head" || c.Notes != "" {
		t.Fatalf("open = %+v", c)
	}
	if c := changeByID(t, list, moved.ID); c.State != changes.Draft || !c.NeedsRebase {
		t.Fatalf("target moved = %+v", c)
	}
	if c := changeByID(t, list, conflict.ID); c.State != changes.Draft || !c.NeedsRebase || c.Base != "release" {
		t.Fatalf("rebase conflict = %+v", c)
	}
	if c := changeByID(t, list, merged.ID); c.State != changes.Merged || c.NeedsRebase || c.Head != "merged-head" || c.Worktree != "" ||
		!c.UpdatedAt.Equal(removedAt.Add(-time.Minute)) {
		t.Fatalf("merged = %+v", c)
	}
	if c := changeByID(t, list, closed.ID); c.State != changes.Closed || c.Worktree != "" || !c.UpdatedAt.Equal(removedAt) {
		t.Fatalf("closed = %+v", c)
	}
	c = changeByID(t, list, workflow.ID)
	if c.State != changes.Draft || c.Origin == nil || *c.Origin != (changes.Origin{RunID: "run-1", Workflow: "fix-bugs", Step: "build"}) || c.Head != "" {
		t.Fatalf("workflow = %+v", c)
	}
	if want := []rune(strings.Split(long, "\n")[0]); c.Title != strings.TrimSpace(string(want[:71]))+"…" || len([]rune(c.Title)) > 72 {
		t.Fatalf("workflow title = %q", c.Title)
	}
	if c := changeByID(t, list, continued.ID); c.State != changes.Draft || c.Title != "Write the parser" || c.Notes != "Parser fixed." {
		t.Fatalf("continued = %+v", c)
	}
	for _, c := range list {
		if c.ExecutionID == original.ID || strings.Contains(c.Notes, "harness note") {
			t.Fatalf("listed %+v", c)
		}
	}
	// Unfinished first; then the finished, most recently updated first.
	if list[6].ExecutionID != closed.ID || list[7].ExecutionID != merged.ID {
		t.Fatalf("order = %+v", list)
	}

	// The other Workspace sees only its own Change, and a folder that is
	// not a Repository has none.
	if _, list, err := lister.List(ctx, other, repo); err != nil || len(list) != 1 || list[0].Title != "Other Workspace" {
		t.Fatalf("other Workspace = %+v %v", list, err)
	}
	if _, list, err := lister.List(ctx, ws, t.TempDir()); err != nil || len(list) != 0 {
		t.Fatalf("no Repository = %+v %v", list, err)
	}
}

// Story 4a: in a GitHub Repository open means a pull request, so a build
// whose gate passed is still a draft until it is pushed; in a local one a
// passed gate awaiting the owner is open.
func TestABuildThatPassedTheGateIsADraftUntilItHasAPullRequestOnGitHub(t *testing.T) {
	ctx := context.Background()
	r := newBuildRecords(t)
	repo := repository(t, "main")
	gitIn(t, repo, "remote", "add", "origin", "https://github.com/davidadel66/notes.git")
	ws := memory.WorkspaceID("ws-code")
	passed := r.build(ws, repo, "Fix the footer", reported("", nil, "Fix the footer"))
	r.gate(passed.ID, func(g *delegation.BuildGate) { g.Step, g.Target = delegation.GateAwaiting, "main" })
	read, err := changes.NewService(changes.Config{}).Repository(ctx, repo)
	if err != nil || read.Kind != changes.GitHub {
		t.Fatalf("repository = %+v, %v", read, err)
	}
	list, err := changes.NewBuildSource(r.store).Changes(ctx, ws, read)
	if err != nil {
		t.Fatal(err)
	}
	if c := changeByID(t, list, passed.ID); c.State != changes.Draft {
		t.Fatalf("passed gate on GitHub = %+v", c)
	}
}

// Story 4a (2026-10-10): the title and description Evie gave with its
// merge proposal are the Change's title and notes, and so its pull
// request's; a build proposed without them keeps the commit subject and
// the worker's own words.
func TestAProposalsTitleAndDescriptionAreTheChanges(t *testing.T) {
	ctx := context.Background()
	r := newBuildRecords(t)
	repo := repository(t, "main")
	ws := memory.WorkspaceID("ws-code")
	described := r.build(ws, repo, "Append exactly one new line to README.md containing the text", reported("I appended the line in /Users/me/code/notes/.evie/worktrees/x.", nil, "Append exactly one new line to README.md containing the text"))
	r.gate(described.ID, func(g *delegation.BuildGate) {
		g.Step, g.Target = delegation.GateAwaiting, "main"
		g.Change = &delegation.ChangeText{Title: "Add the integration-test line to the README", Description: "Adds one line to README.md.\n\nVerified with `test -s README.md`."}
	})
	older := r.build(ws, repo, "Fix the footer", reported("Moved the footer below the page.", nil, "Fix the footer"))
	r.gate(older.ID, func(g *delegation.BuildGate) { g.Step, g.Target = delegation.GateAwaiting, "main" })
	read, err := changes.NewService(changes.Config{}).Repository(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	list, err := changes.NewBuildSource(r.store).Changes(ctx, ws, read)
	if err != nil {
		t.Fatal(err)
	}
	if c := changeByID(t, list, described.ID); c.Title != "Add the integration-test line to the README" || c.Notes != "Adds one line to README.md.\n\nVerified with `test -s README.md`." {
		t.Fatalf("described Change = %q / %q", c.Title, c.Notes)
	}
	if c := changeByID(t, list, older.ID); c.Title != "Fix the footer" || c.Notes != "Moved the footer below the page." {
		t.Fatalf("Change without text = %q / %q", c.Title, c.Notes)
	}
}

// Story 6a: a build Evie titled with its merge proposal keeps that title
// and description once linked to its pull request; one it did not takes
// the pull request's.
func TestALinkedBuildKeepsEviesOwnTitleOverThePullRequests(t *testing.T) {
	ctx := context.Background()
	r := newBuildRecords(t)
	repo := repository(t, "main")
	gitIn(t, repo, "remote", "add", "origin", "https://github.com/davidadel66/notes.git")
	ws := memory.WorkspaceID("ws-code")
	titled := r.build(ws, repo, "Append exactly one new line", reported("worker words", nil, "Append exactly one new line"))
	r.gate(titled.ID, func(g *delegation.BuildGate) {
		g.Step = delegation.GateAwaiting
		g.Change = &delegation.ChangeText{Title: "Add the integration-test line", Description: "Adds one line."}
	})
	untitled := r.build(ws, repo, "Make the change the note asks for", reported("worker words", nil, "Make the change the note asks for"))
	pulls := fixedSource{
		{ID: "pr:4", Branch: titled.Worktree.Branch, State: changes.Open, Author: changes.ByOwner, Title: "PR title", Notes: "PR body", PullRequest: &changes.PullRequest{Number: 4, State: "open"}},
		{ID: "pr:6", Branch: untitled.Worktree.Branch, State: changes.Open, Author: changes.ByOwner, Title: "Add CHANGELOG.md", Notes: "The workflow's text.", PullRequest: &changes.PullRequest{Number: 6, State: "open"}},
	}
	_, list, err := (&changes.Lister{Repositories: changes.NewService(changes.Config{}), Builds: changes.NewBuildSource(r.store), PullRequests: pulls}).List(ctx, ws, repo)
	if err != nil {
		t.Fatal(err)
	}
	if c := changeByID(t, list, titled.ID); c.Title != "Add the integration-test line" || c.Notes != "Adds one line." || c.PullRequest == nil {
		t.Fatalf("titled = %q / %q", c.Title, c.Notes)
	}
	if c := changeByID(t, list, untitled.ID); c.Title != "Add CHANGELOG.md" || c.Notes != "The workflow's text." {
		t.Fatalf("untitled = %q / %q", c.Title, c.Notes)
	}
}
