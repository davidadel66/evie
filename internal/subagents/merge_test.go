package subagents_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/worktree"
	"github.com/google/uuid"
)

// Stage 12 (2026-10-04): the verify-and-merge gate.

// checkScript is the project's verify command's script: it fails while
// parser.go is missing or says BROKEN.
const checkScript = "#!/bin/sh\nset -e\ntest -f parser.go || { echo 'parser.go is missing'; exit 3; }\nif grep -q BROKEN parser.go; then echo 'CHECK-FAILED-SENTINEL: parser.go is BROKEN'; exit 4; fi\necho CHECK-OK\n"

const verifyCommand = "sh ./check.sh"

func requireGateSandbox(t *testing.T) {
	t.Helper()
	if _, err := tools.DetectWorkerSandbox(); err != nil {
		t.Skipf("no OS sandbox on this host: %v", err)
	}
}

// ownerCommit commits path with content on the owner's checked-out master,
// leaving the owner's other uncommitted edits alone.
func ownerCommit(t *testing.T, repo, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "add", path)
	git(t, repo, "commit", "-q", "-m", "Owner "+path, "--", path)
	return git(t, repo, "rev-parse", "HEAD")
}

// gateRepo is the owner's checkout with the verify script committed, its
// setup marker folder ignored, and the owner's uncommitted README edit.
func gateRepo(t *testing.T) string {
	t.Helper()
	repo := buildRepo(t)
	ownerCommit(t, repo, "check.sh", checkScript)
	ownerCommit(t, repo, ".gitignore", "/.setup/\n")
	return repo
}

// gateParent is a live turn of a Standard project session on repo whose
// owner set the given setup and verify commands.
func (f *fixture) gateParent(t *testing.T, repo, setup, verify string) delegation.Parent {
	t.Helper()
	p := f.buildParent(t, repo)
	if setup != "" || verify != "" {
		if _, err := f.store.SetProjectVerify(context.Background(), p.Scope.ProjectID, 0, setup, verify); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// writeCalls is one model response writing files.
func writeCalls(prefix string, files map[string]string) openrouter.ChatResponse {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var calls []openrouter.ToolCall
	for i, path := range paths {
		args, _ := json.Marshal(map[string]string{"path": path, "content": files[path]})
		calls = append(calls, toolCall(fmt.Sprintf("%s%d", prefix, i), "write_file", string(args)))
	}
	return multiCall(calls...)
}

// buildFiles runs one build in p's turn whose worker writes files and
// reports; follow-ups write fix and report again.
func (f *fixture) buildFiles(t *testing.T, p delegation.Parent, a delegation.Assignment, files, fix map[string]string) delegation.Result {
	t.Helper()
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		followUp := slices.ContainsFunc(r.Messages, func(m openrouter.Message) bool {
			return m.Role == "user" && strings.HasPrefix(m.Content, "Follow-up assignment")
		})
		switch {
		case followUp && resultOf(r, "fix0") == "" && len(fix) > 0:
			return writeCalls("fix", fix), nil
		case followUp:
			return response(buildReport("Fixed " + a.Key)), nil
		case resultOf(r, "w0") == "" && len(files) > 0:
			return writeCalls("w", files), nil
		}
		return response(buildReport("Wrote " + a.Key)), nil
	}))
	results, err := f.build(t, context.Background(), p, []delegation.Assignment{a})
	if err != nil || len(results) != 1 || results[0].Status != "succeeded" || results[0].Build == nil {
		t.Fatalf("build %s = %+v %v", a.Key, results, err)
	}
	return results[0]
}

// mergeCall is one propose_build_merge call through the real dispatcher.
type mergeCall struct {
	content string
	isErr   bool
	asked   int
	card    *tools.FileChangePreview
}

// proposeMerge commits the parent's propose_build_merge intent in p's turn
// and runs the call through the Subagents Plugin and the tool dispatcher,
// whose approver asks decide.
func (f *fixture) proposeMerge(t *testing.T, p delegation.Parent, args map[string]string, decide func(*tools.FileChangePreview) tools.Decision) mergeCall {
	t.Helper()
	ctx := context.Background()
	b, _ := json.Marshal(args)
	id := "call-" + uuid.NewString()
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: id, Name: delegation.MergeToolName, Arguments: string(b)}})
	intent, err := f.store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventToolIntent, ParentID: p.SourceEventID, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	p.IntentEventID = intent.ID
	var merge tools.Tool
	for _, c := range plugins.NewSubagents(f.supervisor).ToolCapabilities() {
		if c.Tool.Schema.Function.Name == delegation.MergeToolName {
			merge = c.Tool
		}
	}
	if !merge.NeedsApproval || merge.Prepare == nil {
		t.Fatal("propose_build_merge is not approval-gated")
	}
	var out mergeCall
	approve := func(_ context.Context, name, _ string, preview *tools.FileChangePreview) tools.Decision {
		out.asked++
		out.card = preview
		return decide(preview)
	}
	invocation := tools.WithInvocationContext(ctx, tools.InvocationContext{Profile: f.profile, Scope: p.Scope, Lease: p.Lease, SourceEventID: p.SourceEventID, IntentEventID: p.IntentEventID})
	message, isErr, err := tools.NewToolset([]tools.Tool{merge}).ExecuteWithApprovalAuthorizedCompletion(invocation,
		openrouter.ToolCall{ID: id, Type: "function", Function: openrouter.FunctionCall{Name: delegation.MergeToolName, Arguments: string(b)}}, approve, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	out.content, out.isErr = message.Content, isErr
	return out
}

func approveAll(*tools.FileChangePreview) tools.Decision { return tools.Approved }

func neverAsked(t *testing.T) func(*tools.FileChangePreview) tools.Decision {
	return func(*tools.FileChangePreview) tools.Decision {
		t.Error("the owner was asked to approve a merge the gate should have stopped")
		return tools.Declined
	}
}

// proposal decodes a propose_build_merge result or failure.
func proposal(t *testing.T, content string) delegation.ParentMergeProposal {
	t.Helper()
	var p delegation.ParentMergeProposal
	if err := json.Unmarshal([]byte(strings.TrimPrefix(content, "tool call came back with error ")), &p); err != nil {
		t.Fatalf("proposal %q: %v", content, err)
	}
	return p
}

func gateOf(t *testing.T, f *fixture, p delegation.Parent, id string) *delegation.BuildGate {
	t.Helper()
	a, err := f.store.InspectSubagent(context.Background(), p, id)
	if err != nil {
		t.Fatal(err)
	}
	return a.Gate
}

func fileText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Stage 12, the whole gate through the real Plugin and dispatcher: the
// branch is verified as the worker left it, rebased onto master (which the
// owner moved meanwhile) and verified again; the owner sees "Merge evie/…
// into master (fast-forward)" with the commits, files and runs, and a
// declined card merges nothing. On approval the harness fast-forwards
// master in the owner's checkout, keeping the owner's unrelated uncommitted
// edit, and removes the worktree and branch; a second proposal is refused.
func TestMergeGateVerifiesRebasesAndFastForwardsOnApproval(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "parser", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	branch := built.Build.Branch
	ownerHead := ownerCommit(t, repo, "docs.md", "owner docs\n")

	// The first proposal verifies, rebases onto the owner's new commit and
	// verifies again, then shows the card; the owner declines.
	declined := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, func(*tools.FileChangePreview) tools.Decision { return tools.Declined })
	if declined.asked != 1 || declined.isErr || !strings.Contains(declined.content, "declined") {
		t.Fatalf("declined proposal = %+v", declined)
	}
	card := declined.card
	if card == nil || card.Path != "Merge "+branch+" into master (fast-forward)" || !strings.Contains(card.OldText, "master at "+ownerHead[:12]) {
		t.Fatalf("approval card = %+v", card)
	}
	for _, want := range []string{"Commits from " + branch + " (1):", "Add a parser", "Files changed: 1 (+3 -0)", "parser.go +3 -0",
		"passed: `sh ./check.sh` on the branch as the worker left it", "passed: `sh ./check.sh` on the branch rebased onto master", "git merge --ff-only in the checkout at " + repo, "Nothing is pushed"} {
		if !strings.Contains(card.NewText, want) {
			t.Fatalf("approval card lacks %q:\n%s", want, card.NewText)
		}
	}
	g := gateOf(t, f, p, built.ExecutionID)
	if git(t, repo, "rev-parse", "master") != ownerHead || g.Step != delegation.GateAwaiting || !g.Rebased || len(g.Runs) != 2 || g.Runs[1].Outcome != delegation.RunPassed {
		t.Fatalf("a declined merge changed master, or the gate = %+v", g)
	}

	// Proposed again, the branch already sits on master: the first run
	// verifies the exact commit, and the owner approves.
	approved := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, approveAll)
	if approved.asked != 1 || approved.isErr || !strings.Contains(approved.card.NewText, "not rerun the branch rebased onto master") {
		t.Fatalf("approved proposal = %+v", approved)
	}
	merged := proposal(t, approved.content)
	if merged.Status != delegation.MergeMerged || len(merged.Verification) != 2 || merged.Verification[0].Outcome != delegation.RunPassed ||
		merged.Verification[0].Commit != merged.BranchHead || merged.Verification[1].Outcome != delegation.RunSkipped || merged.Merged == nil || merged.Merged.To != merged.BranchHead ||
		!strings.Contains(merged.Merged.Where, "--ff-only") || !strings.Contains(merged.Merged.Cleanup, "removed") {
		t.Fatalf("merge result = %+v", merged)
	}
	// master is the rebased branch: the owner's commit, then the worker's.
	if head := git(t, repo, "rev-parse", "master"); head != merged.BranchHead || git(t, repo, "rev-parse", "master~1") != ownerHead {
		t.Fatalf("master = %s, want %s on top of %s", head, merged.BranchHead, ownerHead)
	}
	if !strings.Contains(fileText(t, filepath.Join(repo, "parser.go")), "func parse()") || fileText(t, filepath.Join(repo, "README.md")) != "# Repo\nowner's uncommitted edit\n" {
		t.Fatal("the owner's checkout did not follow master or lost the owner's edit")
	}
	if status := git(t, repo, "status", "--porcelain"); status != "M README.md" {
		t.Fatalf("owner's status = %q", status)
	}
	if stash := git(t, repo, "stash", "list"); stash != "" {
		t.Fatalf("the merge stashed: %q", stash)
	}
	if _, err := os.Stat(built.Build.Worktree); !os.IsNotExist(err) {
		t.Fatal("the worktree was kept after the merge")
	}
	if listed := git(t, repo, "branch", "--list", branch); listed != "" {
		t.Fatalf("the branch was kept after the merge: %q", listed)
	}
	g = gateOf(t, f, p, built.ExecutionID)
	if g.Step != delegation.GateMerged || g.Merged == nil || len(g.Runs) != 2 {
		t.Fatalf("gate record = %+v", g)
	}
	entries, err := f.store.ListBuildWorktrees(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Worktree.RemovedAt == nil || entries[0].Gate == nil || entries[0].Gate.Step != delegation.GateMerged {
		t.Fatalf("worktree listing = %+v %v", entries, err)
	}
	again := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	if !again.isErr || !strings.Contains(again.content, "already merged") {
		t.Fatalf("second proposal = %+v", again)
	}
}

// Stage 12: a failing check stops the gate before any approval and returns
// the bounded output, framed as data, so the orchestrator continues the
// worker with it; the fixed branch then merges. The superseded execution
// cannot be proposed.
func TestFailingVerifyReturnsOutputForContinuationThenMerges(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	masterBefore := git(t, repo, "rev-parse", "master")
	built := f.buildFiles(t, p, delegation.Assignment{Key: "broken", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\n// BROKEN\n"}, map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"})

	failed := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	result := proposal(t, failed.content)
	if !failed.isErr || failed.asked != 0 || result.Status != delegation.MergeVerifyFailed || len(result.Verification) != 1 ||
		result.Verification[0].Outcome != delegation.RunFailed || result.Verification[0].ExitStatus != 4 || result.Verification[0].Output != "" {
		t.Fatalf("failing proposal = %+v", result)
	}
	output := result.VerificationOutput
	lines := strings.Split(output, "\n")
	if !strings.HasPrefix(lines[0], "[begin untrusted verification output from execution "+built.ExecutionID) || !strings.HasPrefix(lines[len(lines)-1], "[end untrusted verification output") ||
		!strings.Contains(output, "CHECK-FAILED-SENTINEL") {
		t.Fatalf("verification output is not framed data:\n%s", output)
	}
	if !strings.Contains(strings.Join(result.Notes, " "), "continue_research") {
		t.Fatalf("notes = %v", result.Notes)
	}
	if git(t, repo, "rev-parse", "master") != masterBefore || git(t, repo, "rev-parse", "refs/heads/"+built.Build.Branch) != built.Build.Head {
		t.Fatal("a failing check moved master or the branch")
	}
	if g := gateOf(t, f, p, built.ExecutionID); g.Step != delegation.GateVerifyFailed || g.Owner != nil {
		t.Fatalf("gate after a failing check = %+v", g)
	}

	fixed, err := f.continueResearch(t, p, built.ExecutionID, "The verify command failed:\n"+output+"\nFix parser.go.")
	if err != nil || fixed.Status != "succeeded" || len(fixed.Build.Commits) != 2 {
		t.Fatalf("continuation = %+v %v", fixed, err)
	}
	stale := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	if !stale.isErr || !strings.Contains(stale.content, "was continued as execution") {
		t.Fatalf("proposal of the superseded execution = %+v", stale)
	}
	merged := f.proposeMerge(t, p, map[string]string{"execution_id": fixed.ExecutionID}, approveAll)
	if r := proposal(t, merged.content); merged.isErr || r.Status != delegation.MergeMerged || r.Rebased || len(r.Verification) != 2 ||
		r.Verification[1].Outcome != delegation.RunSkipped || git(t, repo, "rev-parse", "master") != r.BranchHead {
		t.Fatalf("merge after the fix = %+v", r)
	}
}

// Stage 12: a rebase that conflicts with the target is aborted inside the
// worktree; the branch, the worktree and master are unchanged, the
// conflicting files are named, and nobody is asked to approve anything.
func TestRebaseConflictIsAbortedAndReported(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "conflict", Objective: "Call the parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n", "main.go": "package main\n\nfunc main() { parse() }\n"}, nil)
	ownerHead := ownerCommit(t, repo, "main.go", "package main\n\nfunc main() { println(\"owner\") }\n")

	conflicted := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	r := proposal(t, conflicted.content)
	if !conflicted.isErr || r.Status != delegation.MergeRebaseConflict || !slices.Equal(r.Conflicts, []string{"main.go"}) ||
		len(r.Verification) != 1 || r.Verification[0].Outcome != delegation.RunPassed || !strings.Contains(strings.Join(r.Notes, " "), "aborted") {
		t.Fatalf("conflicting proposal = %+v", r)
	}
	if git(t, repo, "rev-parse", "refs/heads/"+built.Build.Branch) != built.Build.Head || git(t, repo, "rev-parse", "master") != ownerHead {
		t.Fatal("a conflicting rebase moved the branch or master")
	}
	w := worktreeOf(t, f, p, built.ExecutionID)
	if aborted, err := worktree.AbortRebase(context.Background(), w); err != nil || aborted {
		t.Fatalf("a rebase was left in progress: %v %v", aborted, err)
	}
	if changed, err := worktree.TrackedChanges(context.Background(), w); err != nil || len(changed) != 0 {
		t.Fatalf("worktree after the aborted rebase = %v %v", changed, err)
	}
	if g := gateOf(t, f, p, built.ExecutionID); g.Step != delegation.GateRebaseConflict {
		t.Fatalf("gate = %+v", g)
	}
}

// Stage 12: a verify command that changes tracked files stops the gate
// before the rebase, asks nobody and leaves master and the branch alone.
func TestVerifyThatChangesTrackedFilesStopsTheGate(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand+" && echo formatted >> README.md")
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "parser", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	master := git(t, repo, "rev-parse", "master")
	stopped := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	r := proposal(t, stopped.content)
	if !stopped.isErr || r.Status != delegation.MergeVerifyFailed || !strings.Contains(strings.Join(r.Notes, " "), "changed 1 tracked file") ||
		git(t, repo, "rev-parse", "master") != master || git(t, repo, "rev-parse", "refs/heads/"+built.Build.Branch) != built.Build.Head {
		t.Fatalf("proposal after a modifying check = %+v", r)
	}
	if fileText(t, filepath.Join(repo, "README.md")) != "# Repo\nowner's uncommitted edit\n" {
		t.Fatal("the check reached the owner's checkout")
	}
}

// Stage 12: a branch that edits the script its verify command runs passes
// its own check trivially, so the card warns the owner.
func TestCardWarnsWhenTheBranchEditsItsOwnCheck(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "lenient", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\n// BROKEN\n", "check.sh": "#!/bin/sh\necho CHECK-OK\n"}, nil)
	declined := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, func(*tools.FileChangePreview) tools.Decision { return tools.Declined })
	if declined.card == nil || !strings.Contains(declined.card.NewText, "! The branch changes check.sh, which the verify command runs; review that change before approving.") {
		t.Fatalf("card = %+v", declined.card)
	}
}

func worktreeOf(t *testing.T, f *fixture, p delegation.Parent, id string) delegation.Worktree {
	t.Helper()
	a, err := f.store.InspectSubagent(context.Background(), p, id)
	if err != nil || a.Worktree == nil {
		t.Fatalf("attempt %s = %+v %v", id, a, err)
	}
	return *a.Worktree
}

// Stage 12: when the owner's uncommitted edits in the checkout conflict
// with the merge, git refuses after approval and the harness reports it:
// master, the owner's edits, the worktree and the branch stay exactly as
// they were, nothing is stashed. Once the owner sets the edit aside, a new
// proposal merges.
func TestMergeRefusesOwnersConflictingChangesAndKeepsThem(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "main", Objective: "Call the parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n", "main.go": "package main\n\nfunc main() { parse() }\n"}, nil)
	masterBefore := git(t, repo, "rev-parse", "master")
	wip := "package main\n\n// owner's work in progress\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(repo, "main.go"), []byte(wip), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "config", "merge.autoStash", "true")

	refused := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, approveAll)
	r := proposal(t, refused.content)
	if refused.asked != 1 || !refused.isErr || r.Status != delegation.MergeMergeRefused || !strings.Contains(strings.Join(r.Notes, " "), "main.go") ||
		!strings.Contains(strings.Join(r.Notes, " "), "never stashes") {
		t.Fatalf("refused merge = %+v", r)
	}
	if git(t, repo, "rev-parse", "master") != masterBefore || fileText(t, filepath.Join(repo, "main.go")) != wip ||
		fileText(t, filepath.Join(repo, "README.md")) != "# Repo\nowner's uncommitted edit\n" || git(t, repo, "stash", "list") != "" {
		t.Fatal("a refused merge touched master or the owner's edits")
	}
	if _, err := os.Stat(built.Build.Worktree); err != nil || git(t, repo, "rev-parse", "refs/heads/"+built.Build.Branch) != built.Build.Head {
		t.Fatal("a refused merge removed the worktree or moved the branch")
	}
	if g := gateOf(t, f, p, built.ExecutionID); g.Step != delegation.GateMergeRefused {
		t.Fatalf("gate = %+v", g)
	}
	// The owner sets the edit aside; the next proposal merges.
	git(t, repo, "checkout", "--", "main.go")
	merged := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, approveAll)
	if r := proposal(t, merged.content); merged.isErr || r.Status != delegation.MergeMerged || git(t, repo, "rev-parse", "master") != r.BranchHead {
		t.Fatalf("merge after the owner's edit was set aside = %+v", r)
	}
}

// Stage 12: a target that is not checked out anywhere moves by a
// compare-and-swap of its ref; when it moved between verification and the
// owner's approval, nothing is merged, and the next proposal rebases onto
// the new head and verifies again before asking.
func TestStaleTargetIsReverifiedAndUncheckedOutTargetMovesByCompareAndSwap(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	git(t, repo, "branch", "release")
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "release", Objective: "Add a parser", TaskID: string(tree.assigned.ID), Base: "release"},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	masterBefore := git(t, repo, "rev-parse", "master")

	var hotfix string
	stale := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, func(*tools.FileChangePreview) tools.Decision {
		// release moves while the card waits.
		hotfix = commitOnBranch(t, repo, "release", "hotfix.txt", "hotfix\n")
		return tools.Approved
	})
	r := proposal(t, stale.content)
	if stale.asked != 1 || !stale.isErr || r.Status != delegation.MergeTargetMoved || !strings.Contains(strings.Join(r.Notes, " "), "Propose the merge again") {
		t.Fatalf("stale approval = %+v", r)
	}
	if git(t, repo, "rev-parse", "release") != hotfix {
		t.Fatal("a stale approval moved release")
	}
	merged := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, approveAll)
	r = proposal(t, merged.content)
	if merged.isErr || r.Status != delegation.MergeMerged || !r.Rebased || len(r.Verification) != 2 || r.Verification[1].Outcome != delegation.RunPassed ||
		r.TargetHead != hotfix || !strings.Contains(r.Merged.Where, "compare-and-swap") || !strings.Contains(strings.Join(r.Notes, " "), "release moved after the last verification") {
		t.Fatalf("merge after the target moved = %+v", r)
	}
	if git(t, repo, "rev-parse", "release") != r.BranchHead || git(t, repo, "rev-parse", "release~1") != hotfix {
		t.Fatal("release did not fast-forward onto the re-verified branch")
	}
	if git(t, repo, "rev-parse", "master") != masterBefore || git(t, repo, "rev-parse", "--abbrev-ref", "HEAD") != "master" || git(t, repo, "status", "--porcelain") != "M README.md" {
		t.Fatal("merging release touched master or the owner's checkout")
	}
}

// commitOnBranch commits path on branch without checking it out.
func commitOnBranch(t *testing.T, repo, branch, path, content string) string {
	t.Helper()
	cmd := exec.Command("git", "hash-object", "-w", "--stdin")
	cmd.Dir, cmd.Stdin = repo, strings.NewReader(content)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	blob := strings.TrimSpace(string(out))
	parent := git(t, repo, "rev-parse", branch)
	index := filepath.Join(t.TempDir(), "index")
	withIndex := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+index)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	withIndex("read-tree", parent)
	withIndex("update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
	tree := withIndex("write-tree")
	commit := git(t, repo, "commit-tree", tree, "-p", parent, "-m", "Owner "+path)
	git(t, repo, "update-ref", "refs/heads/"+branch, commit)
	return commit
}

// Stage 12: without the owner's verify command a merge proposal is refused
// with how the owner sets one and what the repository's instructions
// suggest; Evie never sets it. A branch with nothing to merge and another
// parent's execution are refused too.
func TestMergeProposalNeedsTheOwnersVerifyCommand(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	ownerCommit(t, repo, "AGENTS.md", "# Guide\n\n## Verification\n\n```sh\nsh ./check.sh\n```\n\nInstall dependencies with `npm ci` first.\n")
	p := f.gateParent(t, repo, "", "")
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "parser", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)

	refused := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	r := proposal(t, refused.content)
	notes := strings.Join(r.Notes, " ")
	for _, want := range []string{"No verify command is set for this project", "evie projects verify " + string(p.Scope.ProjectID), "suggests verify `sh ./check.sh`", "setup `npm ci`", "never sets it"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("refusal lacks %q: %s", want, notes)
		}
	}
	if !refused.isErr || r.Status != delegation.MergeRefused {
		t.Fatalf("refusal = %+v", r)
	}
	if got, err := f.store.BuildVerifyFor(context.Background(), p.Scope); err != nil || got != (memory.BuildVerify{}) {
		t.Fatalf("a proposal set commands: %+v %v", got, err)
	}

	if _, err := f.store.SetProjectVerify(context.Background(), p.Scope.ProjectID, 0, "", verifyCommand); err != nil {
		t.Fatal(err)
	}
	empty := f.buildFiles(t, p, delegation.Assignment{Key: "nothing", Objective: "Do nothing", TaskID: string(tree.sibling.ID)}, nil, nil)
	nothing := f.proposeMerge(t, p, map[string]string{"execution_id": empty.ExecutionID}, neverAsked(t))
	if !nothing.isErr || !strings.Contains(nothing.content, "nothing to merge") {
		t.Fatalf("empty branch proposal = %+v", nothing)
	}
	// The fixture's Global chat did not delegate it.
	foreign := f.proposeMerge(t, f.parent, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	if !foreign.isErr || !strings.Contains(foreign.content, delegation.ErrNotFound.Error()) {
		t.Fatalf("another parent's proposal = %+v", foreign)
	}
}

// Stage 12: interrupted gate steps recover to a consistent state once their
// turn is gone: a rebase a crash left in the worktree is aborted and the
// branch is as it was; a merge whose compare-and-swap completed is recorded
// as merged and cleaned up; one that did not happen is recorded as
// interrupted with nothing merged. The attempt shows the step it ended in.
func TestInterruptedGateStepsRecoverConsistently(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	ctx := context.Background()
	dead := &delegation.GateOwner{SessionID: p.Scope.SessionID, HolderID: "crashed-process", FencingToken: 1}

	// A rebase cut off by a crash, conflicted and in progress.
	conflicting := f.buildFiles(t, p, delegation.Assignment{Key: "rebase", Objective: "Call the parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n", "main.go": "package main\n\nfunc main() { parse() }\n"}, nil)
	ownerHead := ownerCommit(t, repo, "main.go", "package main\n\nfunc main() { println(\"owner\") }\n")
	w := worktreeOf(t, f, p, conflicting.ExecutionID)
	crash := exec.Command("git", "-c", "core.hooksPath=/dev/null", "--git-dir="+w.GitDir, "--work-tree="+w.Path, "rebase", ownerHead)
	crash.Dir, crash.Env = w.Path, append(os.Environ(), "GIT_EDITOR=true")
	_ = crash.Run()
	if _, err := os.Stat(filepath.Join(w.GitDir, "rebase-merge")); err != nil {
		t.Skipf("git left no rebase in progress to recover: %v", err)
	}
	if _, err := f.store.RecordBuildGate(ctx, conflicting.ExecutionID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Target, g.TargetHead, g.BranchHead = delegation.GateRebasing, dead, "master", ownerHead, conflicting.Build.Head
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.supervisor.RecoverGates(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.GitDir, "rebase-merge")); !os.IsNotExist(err) {
		t.Fatal("recovery left the rebase in progress")
	}
	if g := gateOf(t, f, p, conflicting.ExecutionID); g.Step != delegation.GateInterrupted || !strings.Contains(g.Problem, "aborted") ||
		git(t, repo, "rev-parse", "refs/heads/"+w.Branch) != conflicting.Build.Head {
		t.Fatalf("recovered rebase = %+v", g)
	}

	// A merge whose fast-forward had completed: recovery records it and
	// cleans up.
	done := f.buildFiles(t, p, delegation.Assignment{Key: "done", Objective: "Add notes", TaskID: string(tree.sibling.ID)}, map[string]string{"notes.txt": "notes\n"}, nil)
	master := git(t, repo, "rev-parse", "master")
	if _, err := f.store.RecordBuildGate(ctx, done.ExecutionID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Target, g.TargetHead, g.BranchHead = delegation.GateMerging, dead, "master", master, done.Build.Head
	}); err != nil {
		t.Fatal(err)
	}
	// The compare-and-swap had moved master before the process died.
	git(t, repo, "update-ref", "refs/heads/master", done.Build.Head, master)
	if err := f.supervisor.RecoverGates(ctx); err != nil {
		t.Fatal(err)
	}
	g := gateOf(t, f, p, done.ExecutionID)
	if g.Step != delegation.GateMerged || g.Merged == nil || g.Merged.To != done.Build.Head {
		t.Fatalf("recovered completed merge = %+v", g)
	}
	if _, err := os.Stat(done.Build.Worktree); !os.IsNotExist(err) {
		t.Fatal("recovery kept the merged worktree")
	}

	// A merge that never happened: nothing merged, step interrupted.
	ownerAfter := git(t, repo, "rev-parse", "master")
	if _, err := f.store.RecordBuildGate(ctx, conflicting.ExecutionID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Target, g.TargetHead, g.BranchHead = delegation.GateMerging, dead, "master", ownerAfter, conflicting.Build.Head
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.supervisor.RecoverGates(ctx); err != nil {
		t.Fatal(err)
	}
	if g := gateOf(t, f, p, conflicting.ExecutionID); g.Step != delegation.GateInterrupted || !strings.Contains(g.Problem, "nothing was merged") ||
		git(t, repo, "rev-parse", "master") != ownerAfter {
		t.Fatalf("recovered unfinished merge = %+v", g)
	}
	// A step whose turn is still live is left alone.
	if _, err := f.store.RecordBuildGate(ctx, conflicting.ExecutionID, func(g *delegation.BuildGate) {
		g.Step, g.Owner = delegation.GateVerifying, &delegation.GateOwner{SessionID: p.Lease.SessionID, HolderID: p.Lease.HolderID, FencingToken: p.Lease.FencingToken}
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.supervisor.RecoverGates(ctx); err != nil {
		t.Fatal(err)
	}
	if g := gateOf(t, f, p, conflicting.ExecutionID); g.Step != delegation.GateVerifying {
		t.Fatalf("recovery settled a live gate step: %+v", g)
	}
}

// Stage 12: when a merge moves master, another build branch waiting on
// master is marked as needing a rebase; a worker still running is told on
// its next model call and its result says so; its own proposal rebases
// onto the moved master automatically.
func TestMovingTargetMarksOtherBuildsAndTellsRunningWorkers(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	first := f.buildFiles(t, p, delegation.Assignment{Key: "first", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)

	wrote, release := make(chan struct{}), make(chan struct{})
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if resultOf(r, "w0") == "" {
			return writeCalls("w", map[string]string{"parser.go": "package main\n\nfunc parse() {}\n", "second.txt": "second\n"}), nil
		}
		close(wrote)
		<-release
		return response(buildReport("Second change.")), nil
	}))
	var second []delegation.Result
	var secondErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		second, secondErr = f.build(t, context.Background(), p, []delegation.Assignment{{Key: "second", Objective: "Add the second file", TaskID: string(tree.sibling.ID)}})
	}()
	select {
	case <-wrote:
	case <-time.After(30 * time.Second):
		t.Fatal("the second worker never wrote")
	}
	merged := f.proposeMerge(t, p, map[string]string{"execution_id": first.ExecutionID}, approveAll)
	r := proposal(t, merged.content)
	if merged.isErr || r.Status != delegation.MergeMerged || !strings.Contains(strings.Join(r.Notes, " "), "marked as needing a rebase") {
		t.Fatalf("first merge = %+v", r)
	}
	close(release)
	<-done
	if secondErr != nil || len(second) != 1 || second[0].Status != "succeeded" {
		t.Fatalf("second build = %+v %v", second, secondErr)
	}
	moved := second[0].Build.TargetMoved
	if moved == nil || moved.Target != "master" || moved.To != r.BranchHead || moved.ByExecution != first.ExecutionID ||
		!strings.Contains(strings.Join(second[0].Notes, " "), "moved from") {
		t.Fatalf("second result = %+v notes %v", second[0].Build, second[0].Notes)
	}
	entries, err := f.store.ListBuildWorktrees(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	marked := false
	for _, e := range entries {
		if e.ExecutionID == second[0].ExecutionID && e.Gate != nil && e.Gate.NeedsRebase != nil {
			marked = true
		}
	}
	if !marked {
		t.Fatalf("the listing does not show the second branch needs a rebase: %+v", entries)
	}
	secondMerge := f.proposeMerge(t, p, map[string]string{"execution_id": second[0].ExecutionID}, approveAll)
	r2 := proposal(t, secondMerge.content)
	if secondMerge.isErr || r2.Status != delegation.MergeMerged || !r2.Rebased || !strings.Contains(strings.Join(r2.Notes, " "), "moved while the worker ran") {
		t.Fatalf("second merge = %+v", r2)
	}
	if !strings.Contains(fileText(t, filepath.Join(repo, "second.txt")), "second") {
		t.Fatal("the second branch did not reach master")
	}
}

// Stage 12: the worker that was running while master moved was told so on
// its next model call (the notice is added to the request, not stored).
func TestRunningWorkerIsToldWhenItsTargetMoves(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "", verifyCommand)
	tree := f.taskTree(t, p)
	first := f.buildFiles(t, p, delegation.Assignment{Key: "first", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	wrote, release := make(chan struct{}), make(chan struct{})
	var notice string
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if resultOf(r, "w0") == "" {
			return writeCalls("w", map[string]string{"other.txt": "other\n"}), nil
		}
		if notice == "" && resultOf(r, "probe") == "" {
			close(wrote)
			<-release
			return call("probe", "read_file", map[string]string{"path": "other.txt"}), nil
		}
		notice = lastMessage(r)
		return response(buildReport("Other change.")), nil
	}))
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = f.build(t, context.Background(), p, []delegation.Assignment{{Key: "other", Objective: "Add another file", TaskID: string(tree.sibling.ID)}})
	}()
	<-wrote
	if merged := f.proposeMerge(t, p, map[string]string{"execution_id": first.ExecutionID}, approveAll); merged.isErr {
		t.Fatalf("merge = %+v", merged)
	}
	close(release)
	<-done
	if !strings.Contains(notice, "[Evie harness notice: while you work, the branch your work will merge into, master, moved") {
		t.Fatalf("the running worker was not told: %q", notice)
	}
}

// Stage 12: the project's setup command runs in a fresh worktree on the
// base commit before the worker exists, and the worker is told how it went:
// passed, failed (with its output framed as data) or skipped for a base that
// exists only on an unmerged build branch, which is never set up with
// network. A continuation does not run setup again.
func TestSetupRunsBeforeTheWorkerOnTheOwnersHistoryOnly(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	setupCommand := "mkdir -p .setup && git rev-parse HEAD > .setup/head && echo ran >> \"$TMPDIR/setup-count\""
	p := f.gateParent(t, repo, setupCommand, verifyCommand)
	tree := f.taskTree(t, p)
	base := git(t, repo, "rev-parse", "master")

	var first string
	var markerRead string
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if first == "" {
			first = lastMessage(r)
		}
		if resultOf(r, "marker") == "" {
			return call("marker", "read_file", map[string]string{"path": ".setup/head"}), nil
		}
		markerRead = resultOf(r, "marker")
		return response(buildReport("Read the setup marker.")), nil
	}))
	results, err := f.build(t, context.Background(), p, []delegation.Assignment{{Key: "setup", Objective: "Check setup", TaskID: string(tree.assigned.ID)}})
	if err != nil || results[0].Status != "succeeded" {
		t.Fatalf("build = %+v %v", results, err)
	}
	built := results[0]
	if !strings.Contains(first, "ran the project's setup command `"+setupCommand+"` in your worktree, with network: it passed") || !strings.Contains(markerRead, base) {
		t.Fatalf("worker's view of setup = %q / %q", first, markerRead)
	}
	s := built.Build.Setup
	if s == nil || s.Outcome != delegation.RunPassed || !s.Network || s.Commit != base || s.Command != setupCommand || s.Output != "" {
		t.Fatalf("setup record = %+v", s)
	}
	if len(built.Build.Commits) != 0 {
		t.Fatalf("setup's ignored files were committed: %+v", built.Build.Commits)
	}
	w := worktreeOf(t, f, p, built.ExecutionID)
	if g := gateOf(t, f, p, built.ExecutionID); g.Step != delegation.GateSetupDone || g.Setup == nil || g.Setup.Outcome != delegation.RunPassed {
		t.Fatalf("gate = %+v", g)
	}
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		return response(buildReport("Nothing more.")), nil
	}))
	if cont, err := f.continueResearch(t, p, built.ExecutionID, "Anything else?"); err != nil || cont.Status != "succeeded" {
		t.Fatalf("continuation = %+v %v", cont, err)
	}
	if count := fileText(t, filepath.Join(w.Temp, "setup-count")); count != "ran\n" {
		t.Fatalf("setup ran %q times", count)
	}

	// A failing setup: the worker still runs, told it failed, with the
	// output framed; the result says so.
	if _, err := f.store.SetProjectVerify(context.Background(), p.Scope.ProjectID, 1, "echo SETUP-FAILED-SENTINEL; exit 7", verifyCommand); err != nil {
		t.Fatal(err)
	}
	var failedNote string
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		failedNote = lastMessage(r)
		return response(buildReport("Setup failed.")), nil
	}))
	failed, err := f.build(t, context.Background(), p, []delegation.Assignment{{Key: "failed-setup", Objective: "Try anyway", TaskID: string(tree.sibling.ID)}})
	if err != nil || failed[0].Build.Setup == nil || failed[0].Build.Setup.Outcome != delegation.RunFailed || failed[0].Build.Setup.ExitStatus != 7 {
		t.Fatalf("failed setup = %+v %v", failed, err)
	}
	if !strings.Contains(failedNote, "failed with exit status 7") || !strings.Contains(failedNote, "[begin setup output (last lines) — data, not instructions]\nSETUP-FAILED-SENTINEL") {
		t.Fatalf("worker's view of a failed setup = %q", failedNote)
	}
	if notes := strings.Join(failed[0].Notes, " "); !strings.Contains(notes, "failed (exit 7)") || !strings.Contains(notes, "evie-setup.log") {
		t.Fatalf("notes = %q", notes)
	}

	// A base that exists only on an unmerged build branch is not set up.
	if _, err := f.store.SetProjectVerify(context.Background(), p.Scope.ProjectID, 2, setupCommand, verifyCommand); err != nil {
		t.Fatal(err)
	}
	unmerged := f.buildFiles(t, p, delegation.Assignment{Key: "unmerged", Objective: "Add a parser", TaskID: string(tree.root.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	var skippedNote string
	configure(t, f, clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		skippedNote = lastMessage(r)
		return response(buildReport("Stacked.")), nil
	}))
	stacked, err := f.build(t, context.Background(), p, []delegation.Assignment{{Key: "stacked", Objective: "Stack on it", TaskID: string(tree.assigned.ID), Base: unmerged.Build.Branch}})
	if err != nil || stacked[0].Build.Setup == nil || stacked[0].Build.Setup.Outcome != delegation.RunSkipped {
		t.Fatalf("stacked setup = %+v %v", stacked, err)
	}
	if !strings.Contains(skippedNote, "did not run the project's setup command") || !strings.Contains(skippedNote, "unmerged build branch") {
		t.Fatalf("worker's view of a skipped setup = %q", skippedNote)
	}
	if _, err := os.Stat(filepath.Join(stacked[0].Build.Worktree, ".setup", "head")); !os.IsNotExist(err) {
		t.Fatal("setup ran on worker-written code")
	}
}

// Stage 12 through the composed parent loop: the parent delegates a build,
// proposes its merge, the owner approves the card, and the parent receives
// the merged result; the merge tool appears only in sessions composed with
// it.
func TestComposedParentProposesAMergeTheOwnerApproves(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	ctx := context.Background()
	project, err := f.store.RegisterProject(ctx, "Merged", repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SetProjectVerify(ctx, project.ID, 0, "", verifyCommand); err != nil {
		t.Fatal(err)
	}
	resolved, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.CreateProjectSessionWithComposition(ctx, project.ID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	setupTurn := f.liveTurn(t, stored)
	tree := f.taskTree(t, setupTurn)
	if err := f.store.ReleaseTurnLease(ctx, stored.ID, setupTurn.Lease.HolderID, setupTurn.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	var built []delegation.ParentResult
	var mergedResult string
	client := clientFunc(func(_ context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		if slices.Contains(toolNames(r), delegation.MergeToolName) {
			switch {
			case resultOf(r, "build") == "":
				return call("build", delegation.BuildToolName, map[string]any{"assignments": []map[string]string{{"idempotency_key": "parser", "objective": "Add a parser", "task_id": string(tree.assigned.ID)}}}), nil
			case resultOf(r, "merge") == "":
				if err := json.Unmarshal([]byte(resultOf(r, "build")), &built); err != nil || len(built) != 1 {
					return openrouter.ChatResponse{}, fmt.Errorf("build result %q: %v", resultOf(r, "build"), err)
				}
				return call("merge", delegation.MergeToolName, map[string]string{"execution_id": built[0].ExecutionID}), nil
			}
			mergedResult = resultOf(r, "merge")
			return response("Merged."), nil
		}
		if resultOf(r, "w0") == "" {
			return writeCalls("w", map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}), nil
		}
		return response(buildReport("Added the parser.")), nil
	})
	configure(t, f, client)
	var card *tools.FileChangePreview
	approver := func(_ context.Context, name, _ string, preview *tools.FileChangePreview) tools.Decision {
		if name != delegation.MergeToolName {
			t.Errorf("approval asked for %s", name)
			return tools.Declined
		}
		card = preview
		return tools.Approved
	}
	session := agent.NewWithToolset(client, f.profile, f.store.BindHistory(stored.ID, "owner"), stored.ScopeContext(), f.store.BindTurnOwner(stored.ID, "owner"), resolved.Toolset)
	if err := session.Send(ctx, "Build the parser and merge it", &eventSink{}, approver); err != nil {
		t.Fatal(err)
	}
	if card == nil || !strings.HasPrefix(card.Path, "Merge "+built[0].Build.Branch+" into master (fast-forward)") {
		t.Fatalf("approval card = %+v", card)
	}
	r := proposal(t, mergedResult)
	if r.Status != delegation.MergeMerged || git(t, repo, "rev-parse", "master") != r.BranchHead {
		t.Fatalf("merged result = %+v (master %s)", r, git(t, repo, "rev-parse", "master"))
	}
}

// Stage 12: a crash while the setup command runs leaves a consistent
// record: recovery settles the attempt interrupted as before, and its setup
// run and gate step say they were interrupted, not that setup passed.
func TestCrashDuringSetupRecoversAsInterrupted(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	p := f.gateParent(t, repo, "npm ci", verifyCommand)
	tree := f.taskTree(t, p)
	ctx := context.Background()
	request := []delegation.Assignment{{Key: "crash", Objective: "Work", TaskID: string(tree.assigned.ID)}}
	invoked, err := f.invokeTool(ctx, p, delegation.BuildToolName, request)
	if err != nil {
		t.Fatal(err)
	}
	template, err := f.manager.ResolvePreset(plugins.BuildPresetID)
	if err != nil {
		t.Fatal(err)
	}
	parentReceipt, err := f.store.GetCompositionReceipt(ctx, p.Scope.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	childReceipt, err := subagents.BuildReceiptForTest(template.Receipt, parentReceipt)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := worktree.Plan(ctx, repo, "", uuid.NewString(), "Work")
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := f.store.AdmitBuildSubagents(ctx, invoked, request, []delegation.Worktree{plan}, []error{nil}, childReceipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a := attempts[0]
	if _, started, err := f.store.StartSubagent(ctx, a.ID); err != nil || !started {
		t.Fatalf("start = %v %v", started, err)
	}
	if err := worktree.Ensure(ctx, *a.Worktree, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
		g.Step = delegation.GateSetupRunning
		g.Setup = &delegation.GateRun{Step: "setup", Command: "npm ci", Outcome: delegation.RunRunning, Network: true, StartedAt: time.Now()}
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if n, err := f.store.RecoverSubagents(ctx); err != nil || n != 1 {
		t.Fatalf("recovered %d: %v", n, err)
	}
	turn := f.liveTurn(t, mustSession(t, f, p.Scope.SessionID))
	recovered, err := f.store.InspectSubagent(ctx, turn, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != "interrupted" || recovered.Gate == nil || recovered.Gate.Step != delegation.GateInterrupted ||
		recovered.Result.Build.Setup == nil || recovered.Result.Build.Setup.Outcome != delegation.RunInterrupted {
		t.Fatalf("recovered = %s %+v %+v", recovered.State, recovered.Gate, recovered.Result.Build)
	}
	if notes := strings.Join(recovered.Result.Notes, " "); !strings.Contains(notes, "Setup command `npm ci` interrupted") {
		t.Fatalf("notes = %q", notes)
	}
}

// Stage 12: a chat pinned to the Standard preset before the merge gate
// (535755ad…) reopens with exactly its tools, so it has no
// propose_build_merge, and the Kernel refuses a merge proposal from it even
// if one were fabricated; it can still build.
func TestParentPinnedBeforeTheMergeGateCannotProposeMerges(t *testing.T) {
	requireGateSandbox(t)
	f := setup(t, delegation.DefaultPolicy())
	repo := gateRepo(t)
	ctx := context.Background()
	current, err := f.manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	old := composition.Clone(current.Receipt)
	old.Preset.Version = "sha256:535755ad0e6b11cadd91edd54db59c8196a3970ff256b3feddac838babc38a72"
	old.Capabilities = slices.DeleteFunc(old.Capabilities, func(c composition.Capability) bool { return c.ID == delegation.MergeCapabilityID })
	old.ToolSchemas = slices.DeleteFunc(old.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == delegation.MergeToolName })
	if len(old.Capabilities) != len(current.Receipt.Capabilities)-1 {
		t.Fatal("the current Standard preset lacks the merge gate")
	}
	reopened, err := f.manager.ResumeComposition(old)
	if err != nil {
		t.Fatalf("pre-merge receipt does not reopen: %v", err)
	}
	if names := toolNamesOfSet(reopened.Toolset); slices.Contains(names, delegation.MergeToolName) || !slices.Contains(names, delegation.BuildToolName) {
		t.Fatalf("a pre-merge session's tools changed: %v", names)
	}
	project, err := f.store.RegisterProject(ctx, "Older chat", repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.SetProjectVerify(ctx, project.ID, 0, "", verifyCommand); err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateProjectSessionWithComposition(ctx, project.ID, old)
	if err != nil {
		t.Fatal(err)
	}
	p := f.liveTurn(t, session)
	tree := f.taskTree(t, p)
	built := f.buildFiles(t, p, delegation.Assignment{Key: "old", Objective: "Add a parser", TaskID: string(tree.assigned.ID)},
		map[string]string{"parser.go": "package main\n\nfunc parse() {}\n"}, nil)
	master := git(t, repo, "rev-parse", "master")
	refused := f.proposeMerge(t, p, map[string]string{"execution_id": built.ExecutionID}, neverAsked(t))
	if !refused.isErr || !strings.Contains(refused.content, "does not permit proposing build merges") || git(t, repo, "rev-parse", "master") != master {
		t.Fatalf("a pre-merge parent's proposal = %+v", refused)
	}
}
