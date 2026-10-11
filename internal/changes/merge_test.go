package changes_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/changes"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/github"
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
	if c, err := m.Close(ctx, "w", dir, "build:evie/1a2b3c4d-fix"); err != nil || c.Change.ID != draft.ID || len(gate.closed) != 1 || gate.closed[0] != "w e1" {
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

	if len(gate.merged) != 0 {
		t.Fatalf("refused merges reached the gate: %v", gate.merged)
	}
}

// mergeRecords records the conflicts a merge reports.
type mergeRecords struct{ recorded []string }

func (r *mergeRecords) RecordChangeMergeable(_ context.Context, ws memory.WorkspaceID, number int, head string, mergeable bool) error {
	r.recorded = append(r.recorded, fmt.Sprintf("%s #%d %s %v", ws, number, head, mergeable))
	return nil
}

func (*mergeRecords) ArmChangeMerge(context.Context, memory.WorkspaceID, int) error { return nil }
func (*mergeRecords) DisarmChangeMerge(context.Context, memory.WorkspaceID, int) (bool, error) {
	return false, nil
}
func (*mergeRecords) RecordChangeNotification(context.Context, eviedb.ChangeNotification) error {
	return nil
}

// trackedRepository is the owner's clone of the fake's "notes" repository
// with a tracked file on main, and an Evie branch per name with one commit
// each, pushed with their pull requests opened.
func trackedRepository(t *testing.T, fake *fakeGitHub, server *httptest.Server, branches ...string) (string, map[string]changes.Change) {
	t.Helper()
	fake.seed("notes", true)
	dir := filepath.Join(t.TempDir(), "notes")
	gitIn(t, fake.root, "clone", "-q", fake.bare("notes"), dir)
	gitIn(t, dir, "remote", "set-url", "origin", cloneURL("notes"))
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "notes.md")
	gitIn(t, dir, "-c", "user.name=Owner", "-c", "user.email=owner@example.com", "commit", "-q", "-m", "Notes")
	gitIn(t, dir, "push", "-q", fake.bare("notes"), "main")
	built := map[string]changes.Change{}
	for i, branch := range branches {
		gitIn(t, dir, "switch", "-q", "-c", branch, "main")
		gitIn(t, dir, "-c", "user.name=Evie", "-c", "user.email=evie@localhost", "commit", "-q", "--allow-empty", "-m", "Change "+branch)
		gitIn(t, dir, "switch", "-q", "main")
		c := changes.Change{ID: "build:" + branch, Branch: branch, Base: "main", State: changes.Draft, Author: changes.ByEvie, Title: "Change " + branch,
			Notes: "Why.", Worktree: dir, ExecutionID: fmt.Sprintf("e%d", i+1), Origin: &changes.Origin{SessionID: "s1"}}
		if _, err := newPusher(server, fixedSource{c}, nil).Push(context.Background(), "w", dir, branch); err != nil {
			t.Fatal(err)
		}
		c.State = changes.Open
		c.PullRequest = &changes.PullRequest{Number: i + 1, State: "open", HeadSHA: head(t, dir, branch)}
		built[branch] = c
	}
	return dir, built
}

// changes.spec.md story 5b: in a GitHub Repository the owner merges a
// Change through its pull request with GitHub's rebase merge of the head
// Evie showed, and closes it there. Afterwards Evie's own branch goes on
// GitHub and locally, with its worktree, and the owner's base branch is
// fast-forwarded only where it is checked out with no uncommitted edits.
// GitHub's refusals say why and merge nothing; a conflict is recorded as
// needing a rebase. The owner's branches are never deleted.
func TestTheOwnerMergesAndClosesOnGitHub(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	const fix, docs = "evie/1a2b3c4d-fix", "evie/2b3c4d5e-docs"
	dir, built := trackedRepository(t, fake, server, fix, docs)
	owner := changes.Change{ID: "pr:3", Branch: "owner-idea", Base: "main", State: changes.Open, Author: changes.ByOwner,
		PullRequest: &changes.PullRequest{Number: 3, State: "open", HeadSHA: head(t, dir, "main")}}
	gitIn(t, dir, "push", "-q", fake.bare("notes"), "main:refs/heads/owner-idea")
	fake.pulls["notes"] = append(fake.pulls["notes"], map[string]any{"number": 3, "state": "open", "title": "Idea", "html_url": "https://github.com/davidadel66/notes/pull/3",
		"head": map[string]any{"ref": "owner-idea", "sha": head(t, dir, "main")}, "base": map[string]any{"ref": "main"}, "user": map[string]any{"login": "davidadel66"}})
	gate, records := &fakeGate{}, &mergeRecords{}
	merger := func(list ...changes.Change) *changes.Merger {
		return &changes.Merger{Lister: &changes.Lister{Repositories: publishService(server), Builds: fixedSource(list)}, Gate: gate,
			GitHub: github.New(server.URL, token, server.Client()), Records: records}
	}
	m := merger(built[fix], built[docs], owner)
	refused := func(name, id, says string) {
		t.Helper()
		if _, err := m.Merge(ctx, "w", dir, id); !errors.Is(err, changes.ErrNotMerged) || !strings.Contains(err.Error(), says) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	mainBefore := head(t, fake.bare("notes"), "refs/heads/main")

	fake.pullDraft = true
	refused("a GitHub draft", built[fix].ID, "draft")
	fake.pullDraft, fake.notMergeable = false, true
	refused("a conflict", built[fix].ID, "needs a rebase")
	if want := fmt.Sprintf("w #1 %s false", built[fix].PullRequest.HeadSHA); len(records.recorded) != 1 || records.recorded[0] != want {
		t.Fatalf("recorded %v, want %s", records.recorded, want)
	}
	fake.notMergeable, fake.mergeFails = false, http.StatusMethodNotAllowed
	refused("rebase merges not allowed", built[fix].ID, "Rebase merges are not allowed")
	fake.mergeFails = 0
	moved := built[fix]
	moved.PullRequest = &changes.PullRequest{Number: 1, State: "open", HeadSHA: strings.Repeat("a", 40)}
	if _, err := merger(moved).Merge(ctx, "w", dir, moved.ID); !errors.Is(err, changes.ErrNotMerged) || !strings.Contains(err.Error(), "new commits") {
		t.Fatalf("a moved head: %v", err)
	}
	if head(t, fake.bare("notes"), "refs/heads/main") != mainBefore || len(gate.closed) != 0 {
		t.Fatal("a refused merge merged or cleaned up")
	}

	// The owner's checkout has an uncommitted edit: main is not touched.
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("notes, edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := m.Merge(ctx, "w", dir, built[fix].ID)
	if err != nil || result.Status != delegation.MergeMerged || result.Merged == nil || result.Merged.To != built[fix].PullRequest.HeadSHA {
		t.Fatalf("merge = %+v, %v", result, err)
	}
	notes := strings.Join(result.Notes, " ")
	if head(t, fake.bare("notes"), "refs/heads/main") != built[fix].PullRequest.HeadSHA || strings.Contains(refs(t, fake.bare("notes")), fix) ||
		len(gate.closed) != 1 || gate.closed[0] != "w e1" || !strings.Contains(notes, "deleted on GitHub") || !strings.Contains(notes, "uncommitted edits") {
		t.Fatalf("after the merge: GitHub %s; closed %v; notes %s", refs(t, fake.bare("notes")), gate.closed, notes)
	}
	if head(t, dir, "main") == built[fix].PullRequest.HeadSHA {
		t.Fatal("main moved in a checkout with uncommitted edits")
	}
	if repo, err := publishService(server).Repository(ctx, dir); err != nil || repo.Behind != 1 {
		t.Fatalf("repository = %+v, %v", repo, err)
	}

	// Closing on GitHub closes the pull request and removes Evie's branch;
	// closing the owner's own pull request keeps the owner's branch.
	closed, err := m.Close(ctx, "w", dir, built[docs].ID)
	if err != nil || fake.pulls["notes"][1]["state"] != "closed" || strings.Contains(refs(t, fake.bare("notes")), docs) || len(gate.closed) != 2 {
		t.Fatalf("close = %+v, %v; GitHub %s", closed, err, refs(t, fake.bare("notes")))
	}
	if _, err := m.Close(ctx, "w", dir, owner.ID); err != nil || fake.pulls["notes"][2]["state"] != "closed" || !strings.Contains(refs(t, fake.bare("notes")), "owner-idea") || len(gate.closed) != 2 {
		t.Fatalf("owner close = %v; GitHub %s; closed %v", err, refs(t, fake.bare("notes")), gate.closed)
	}

	// A merge observed on GitHub: the next sync's cleanup removes Evie's
	// branch and worktree and, with the checkout clean, fast-forwards main.
	gitIn(t, dir, "checkout", "--", "notes.md")
	mergedOnGitHub := built[docs]
	mergedOnGitHub.State = changes.Merged
	gitIn(t, fake.bare("notes"), "update-ref", "refs/heads/main", built[fix].PullRequest.HeadSHA)
	gate.closed = nil
	merger(mergedOnGitHub).AfterSync(ctx, "w", changes.Repository{Kind: changes.GitHub, Folder: dir}, changes.SyncResult{Synced: true, Finished: 1})
	if len(gate.closed) != 1 || gate.closed[0] != "w e2" || head(t, dir, "main") != built[fix].PullRequest.HeadSHA {
		t.Fatalf("after sync: closed %v, main %s", gate.closed, head(t, dir, "main"))
	}
}

// changes.spec.md story 5c: "Merge when checks pass" cannot be set while a
// pull request has no checks. Each sync then merges it once at least one
// check exists and every one passed, clears it when a check fails or the
// merge conflicts, and leaves a notice of each; meanwhile it waits.
func TestMergeWhenChecksPassFiresOnGreenAndClearsOnRed(t *testing.T) {
	ctx := context.Background()
	fake, server := newFakeGitHub(t)
	const fix, docs, clash = "evie/1a2b3c4d-fix", "evie/2b3c4d5e-docs", "evie/3c4d5e6f-clash"
	dir, built := trackedRepository(t, fake, server, fix, docs, clash)
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	w, err := store.RegisterWorkspace(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	ws := w.ID
	synced := func(states map[string]string) {
		t.Helper()
		var records []eviedb.ChangePullRequest
		for _, branch := range []string{fix, docs, clash} {
			c := built[branch]
			record := eviedb.ChangePullRequest{Number: c.PullRequest.Number, Owner: "davidadel66", Repository: "notes", Branch: branch, Base: "main",
				Title: c.Title, Author: "davidadel66", State: "open", HeadSHA: c.PullRequest.HeadSHA, URL: "https://github.com/davidadel66/notes/pull/1", GitHubUpdatedAt: time.Now()}
			if state, ok := states[branch]; ok && state != "" {
				record.Checks = []eviedb.ChangePullRequestCheck{{Name: "ci", State: state}}
			}
			if _, gone := states[branch]; gone || len(states) == 0 {
				records = append(records, record)
			}
		}
		if err := store.SaveChangePullRequests(ctx, ws, records); err != nil {
			t.Fatal(err)
		}
	}
	var drafts []changes.Change
	for _, branch := range []string{fix, docs, clash} {
		c := built[branch]
		c.State, c.PullRequest = changes.Draft, nil
		drafts = append(drafts, c)
	}
	gate := &fakeGate{}
	m := &changes.Merger{Lister: &changes.Lister{Repositories: publishService(server), Builds: fixedSource(drafts),
		PullRequests: changes.PullRequestSource{Store: store, Arms: store}}, Gate: gate, GitHub: github.New(server.URL, token, server.Client()), Records: store}
	repo := changes.Repository{Kind: changes.GitHub, Folder: dir}
	after := func() { m.AfterSync(ctx, ws, repo, changes.SyncResult{Synced: true}) }
	notices := func() map[string]eviedb.WorkflowNotification {
		t.Helper()
		list, err := store.ListWorkflowNotifications(ctx, eviedb.WorkflowNotificationFilter{WorkspaceID: ws, All: true})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]eviedb.WorkflowNotification{}
		for _, n := range list {
			out[n.ChangeID+" "+string(n.Kind)] = n
		}
		return out
	}

	synced(map[string]string{fix: "running", docs: "", clash: "running"})
	if _, err := m.Arm(ctx, ws, dir, "build:"+docs); !errors.Is(err, changes.ErrNotMerged) || !strings.Contains(err.Error(), "no checks") {
		t.Fatalf("arming without checks = %v", err)
	}
	armed, err := m.Arm(ctx, ws, dir, "build:"+fix)
	if err != nil || armed.ArmedAt == nil {
		t.Fatalf("arm = %+v, %v", armed, err)
	}
	if _, err := m.Arm(ctx, ws, dir, "build:"+clash); err != nil {
		t.Fatal(err)
	}
	mainBefore := head(t, fake.bare("notes"), "refs/heads/main")
	after()
	if head(t, fake.bare("notes"), "refs/heads/main") != mainBefore || len(notices()) != 0 {
		t.Fatal("an armed merge fired while its checks ran")
	}

	// Green: it merges, clears and says so.
	synced(map[string]string{fix: "passed", docs: "", clash: "running"})
	after()
	if head(t, fake.bare("notes"), "refs/heads/main") != built[fix].PullRequest.HeadSHA || len(gate.closed) != 1 {
		t.Fatalf("green did not merge: main %s, closed %v", head(t, fake.bare("notes"), "refs/heads/main"), gate.closed)
	}
	if n, ok := notices()["build:"+fix+" change_merged"]; !ok || !strings.Contains(n.Detail, "#1") {
		t.Fatalf("notices = %+v", notices())
	}
	if arms, _ := store.ChangeMergeArms(ctx, ws); len(arms) != 1 {
		t.Fatalf("arms after the merge = %v", arms)
	}

	// A conflict clears it, records the rebase it needs, and says so.
	fake.notMergeable = true
	synced(map[string]string{docs: "", clash: "passed"})
	after()
	if n, ok := notices()["build:"+clash+" change_needs_rebase"]; !ok || !strings.Contains(n.Detail, "rebase") {
		t.Fatalf("notices = %+v", notices())
	}
	if arms, _ := store.ChangeMergeArms(ctx, ws); len(arms) != 0 {
		t.Fatalf("arms after the conflict = %v", arms)
	}
	fake.notMergeable = false

	// Red: a failing check clears it without merging.
	synced(map[string]string{docs: "running"})
	if _, err := m.Arm(ctx, ws, dir, "build:"+docs); err != nil {
		t.Fatal(err)
	}
	synced(map[string]string{docs: "failed"})
	mainBefore = head(t, fake.bare("notes"), "refs/heads/main")
	after()
	if n, ok := notices()["build:"+docs+" change_merge_cleared"]; !ok || !strings.Contains(n.Detail, "ci") || head(t, fake.bare("notes"), "refs/heads/main") != mainBefore {
		t.Fatalf("notices = %+v", notices())
	}
	if _, err := m.Arm(ctx, ws, dir, "build:"+docs); !errors.Is(err, changes.ErrNotMerged) || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("arming with a failed check = %v", err)
	}

	// The owner clears it.
	synced(map[string]string{docs: "running"})
	if _, err := m.Arm(ctx, ws, dir, "build:"+docs); err != nil {
		t.Fatal(err)
	}
	if disarmed, err := m.Disarm(ctx, ws, dir, "build:"+docs); err != nil || disarmed.ArmedAt != nil {
		t.Fatalf("disarm = %+v, %v", disarmed, err)
	}
	if _, err := m.Disarm(ctx, ws, dir, "build:"+docs); !errors.Is(err, changes.ErrNotMerged) {
		t.Fatalf("disarm again = %v", err)
	}
}
