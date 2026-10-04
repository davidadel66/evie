package subagents

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/worktree"
)

// The verify-and-merge gate (Stage 12 of docs/memory-architecture.md,
// 2026-10-04), modelled on how the owner's own reviews were run (verify,
// rebase, verify again, approve, fast-forward, clean up) and on cloud coding
// agents' environments (a setup phase with network before an agent phase
// without it).
//
//   - Setup: when the harness creates a build worktree, it runs the
//     project's setup command there on the base commit, in the sandbox
//     variant with network, before the worker exists. Worker-written code
//     never runs with network: a base that exists only on an unmerged build
//     branch is not set up.
//   - Proposal: propose_build_merge verifies the worker's branch offline in
//     its worktree, rebases it onto the target's current head, verifies it
//     again, and returns the summary. Only when both runs pass does the
//     owner get an approval card.
//   - Merge: on approval the harness, never a worker, fast-forwards the
//     target and removes the worktree and branch. Other branches waiting on
//     the same target learn that it moved.

const (
	setupTimeout  = 10 * time.Minute
	verifyTimeout = 20 * time.Minute
	// gateCleanupTimeout bounds removing the worktree and branch after a
	// merge, which happens even if the call was cancelled.
	gateCleanupTimeout = 30 * time.Second
)

// runSetup runs the project's setup command in a fresh build attempt's
// worktree and records how it went, or returns nil when there is none.
func (s *Supervisor) runSetup(ctx context.Context, a delegation.Attempt) *delegation.BuildGate {
	commands, err := s.store.BuildVerifyFor(ctx, a.Parent.Scope)
	if err != nil || strings.TrimSpace(commands.Setup) == "" || a.Worktree == nil {
		return nil
	}
	w := *a.Worktree
	run := delegation.GateRun{Step: "setup", Command: commands.Setup, Commit: w.BaseCommit, Network: true, StartedAt: s.now().UTC(),
		Outcome: delegation.RunRunning, Log: filepath.Join(w.Temp, "evie-setup.log")}
	owned, err := worktree.OwnerHistoryContains(ctx, w.Repo, w.BaseCommit)
	switch {
	case err != nil:
		run.Outcome, run.Detail = delegation.RunError, "the harness could not tell whether the base commit is in the owner's history"
	case !owned:
		run.Outcome, run.Detail = delegation.RunSkipped, "the base commit exists only on an unmerged build branch, and setup has network, so it runs only on the owner's history"
	}
	if run.Outcome != delegation.RunRunning {
		ended := s.now().UTC()
		run.EndedAt, run.Log = &ended, ""
		return s.recordSetup(ctx, a.ID, run, delegation.GateSetupDone)
	}
	if gate := s.recordSetup(ctx, a.ID, run, delegation.GateSetupRunning); gate == nil {
		return nil
	}
	result, err := tools.RunGateCommand(ctx, tools.GateCommand{Fence: tools.BuildFence{Worktree: w.Path, Temp: w.Temp}, Command: commands.Setup,
		Network: true, Timeout: s.setupTimeout, Log: run.Log})
	finishRun(ctx, &run, result, err, s.now().UTC())
	return s.recordSetup(context.WithoutCancel(ctx), a.ID, run, delegation.GateSetupDone)
}

func (s *Supervisor) recordSetup(ctx context.Context, id string, run delegation.GateRun, step string) *delegation.BuildGate {
	recorded, err := s.store.RecordBuildGate(ctx, id, func(g *delegation.BuildGate) {
		g.Step = step
		copied := run
		g.Setup = &copied
	})
	if err != nil {
		return nil
	}
	return recorded.Gate
}

// finishRun records how a gate command ended.
func finishRun(ctx context.Context, run *delegation.GateRun, result tools.GateResult, err error, now time.Time) {
	run.EndedAt = &now
	run.DurationMS = result.Duration.Milliseconds()
	run.Output = result.Tail
	switch {
	case err != nil && ctx.Err() != nil:
		run.Outcome, run.Detail = delegation.RunInterrupted, "the call was cancelled"
	case err != nil:
		run.Outcome, run.Detail, run.Log = delegation.RunError, gateWords(err), ""
	case result.TimedOut:
		run.Outcome, run.ExitStatus = delegation.RunTimeout, -1
	case result.ExitStatus == 0:
		run.Outcome = delegation.RunPassed
	default:
		run.Outcome, run.ExitStatus = delegation.RunFailed, result.ExitStatus
	}
}

// gateWords is a gate error in harness words.
func gateWords(err error) string {
	message := err.Error()
	if len(message) > 300 {
		message = strings.ToValidUTF8(message[:300], "") + "…"
	}
	return message
}

// beginGate marks id's gate as running in this process, or refuses a
// second one.
func (s *Supervisor) beginGate(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gates[id] {
		return fmt.Errorf("a merge gate is already running for execution %q", id)
	}
	s.gates[id] = true
	return nil
}

func (s *Supervisor) endGate(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.gates, id)
}

func (s *Supervisor) gateActive(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gates[id]
}

// moveNotice is the harness notice for a running build attempt whose target
// moved, or "".
func (s *Supervisor) moveNotice(id string) string {
	s.mu.Lock()
	move, ok := s.moved[id]
	s.mu.Unlock()
	if !ok {
		return ""
	}
	return fmt.Sprintf("[Evie harness notice: while you work, the branch your work will merge into, %s, moved from %s to %s (another worker's merge). Keep working on your branch; Evie's merge gate rebases it onto %s and reruns the project's checks before anything is merged. Do not run git commands that write.]",
		move.Target, shortCommit(move.From), shortCommit(move.To), move.Target)
}

func (s *Supervisor) forgetMove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.moved, id)
}

// gateOwner is the parent turn that runs a gate step.
func gateOwner(p delegation.Parent) *delegation.GateOwner {
	return &delegation.GateOwner{SessionID: p.Lease.SessionID, HolderID: p.Lease.HolderID, FencingToken: p.Lease.FencingToken}
}

// gateFailure records a step that ended the proposal and returns it as the
// error the parent reads.
func (s *Supervisor) gateFailure(ctx context.Context, id, step, problem string, p delegation.MergeProposal) error {
	_, _ = s.store.RecordBuildGate(context.WithoutCancel(ctx), id, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Problem = step, nil, problem
		g.Target, g.TargetHead, g.BranchHead, g.Rebased = p.Target, p.TargetHead, p.BranchHead, p.Rebased
		g.Runs = stripOutputs(p.Verification)
	})
	if problem != "" && !slices.Contains(p.Notes, problem) {
		p.Notes = append([]string{problem}, p.Notes...)
	}
	return &delegation.MergeGateError{Proposal: p}
}

// refuse ends a proposal before anything ran.
func refuse(p delegation.MergeProposal, format string, args ...any) error {
	p.Status = delegation.MergeRefused
	p.Notes = append([]string{fmt.Sprintf(format, args...)}, p.Notes...)
	return &delegation.MergeGateError{Proposal: p}
}

func stripOutputs(runs []delegation.GateRun) []delegation.GateRun {
	out := make([]delegation.GateRun, len(runs))
	for i, run := range runs {
		run.Output = ""
		out[i] = run
	}
	return out
}

// ProposeMerge runs the merge gate on one of parent's build attempts: it
// verifies the worker's branch in its worktree, offline in the sandbox,
// rebases it onto target's current head (default: the branch the build
// started from) and verifies it again. It returns the proposal when both
// runs pass; otherwise a MergeGateError carrying the bounded failure output,
// with nothing merged. It refuses an attempt that is not the parent's, not
// a finished build, not its child's latest, or has no commits to merge, and
// a Workspace or project without a verify command.
func (s *Supervisor) ProposeMerge(ctx context.Context, parent delegation.Parent, executionID, target string) (delegation.MergeProposal, error) {
	executionID, target = strings.TrimSpace(executionID), strings.TrimSpace(target)
	if executionID == "" {
		return delegation.MergeProposal{}, errors.New("invalid propose_build_merge arguments: execution_id is blank; use the execution_id of a build result from this conversation")
	}
	if len(executionID) > delegation.MaxKeyBytes || len(target) > delegation.MaxBaseBytes {
		return delegation.MergeProposal{}, fmt.Errorf("invalid propose_build_merge arguments: execution_id is limited to %d bytes and target to %d", delegation.MaxKeyBytes, delegation.MaxBaseBytes)
	}
	a, commands, err := s.store.PrepareBuildGate(ctx, parent, executionID)
	if err != nil {
		return delegation.MergeProposal{}, err
	}
	if a.Gate != nil && delegation.GateInProgress(a.Gate.Step) && !s.gateActive(a.ID) {
		// An earlier gate step was cut off and no gate of this process runs
		// it: settle it first, as recovery would.
		if err := s.recoverGate(ctx, a); err != nil {
			return delegation.MergeProposal{}, err
		}
		if a, commands, err = s.store.PrepareBuildGate(ctx, parent, executionID); err != nil {
			return delegation.MergeProposal{}, err
		}
	}
	w := *a.Worktree
	if target == "" {
		target = w.Base
	}
	p := delegation.MergeProposal{ExecutionID: a.ID, Branch: w.Branch, Target: target}
	if strings.TrimSpace(commands.Verify) == "" {
		return p, refuse(p, "%s", noVerifyCommand(parent, w.Repo))
	}
	if err := s.beginGate(a.ID); err != nil {
		return p, refuse(p, "%v", err)
	}
	defer s.endGate(a.ID)
	if target == w.Branch || strings.HasPrefix(target, "evie/") {
		return p, refuse(p, "%s is a build branch; merge into the owner's branch the build started from (%s) or another owner branch", target, w.Base)
	}
	targetHead, err := worktree.TargetHead(ctx, w.Repo, target)
	if err != nil {
		return p, refuse(p, "%v; the build started from %q, so name the target branch to merge into", err, w.Base)
	}
	if err := worktree.Ensure(ctx, w, false); err != nil {
		return p, refuse(p, "%v", err)
	}
	if aborted, err := worktree.AbortRebase(ctx, w); err != nil {
		return p, refuse(p, "a rebase an interruption left in the worktree could not be aborted: %v", err)
	} else if aborted {
		p.Notes = append(p.Notes, "A rebase an earlier interruption left in the worktree was aborted first.")
	}
	if changed, err := worktree.TrackedChanges(ctx, w); err != nil {
		return p, refuse(p, "the worktree's status could not be read: %v", err)
	} else if len(changed) > 0 {
		return p, refuse(p, "the worktree %s has %d uncommitted change(s) to tracked files, which the harness did not commit; continue the worker or clean the worktree before proposing a merge", w.Path, len(changed))
	}
	branchHead, err := worktree.BranchHead(ctx, w)
	if err != nil {
		return p, refuse(p, "%v", err)
	}
	ahead, err := worktree.Compare(ctx, w.Repo, targetHead, branchHead)
	if err != nil {
		return p, refuse(p, "the branch could not be compared with %s: %v", target, err)
	}
	if len(ahead.Commits) == 0 {
		return p, refuse(p, "%s has no commits that %s lacks, so there is nothing to merge", w.Branch, target)
	}
	if moved := a.Gate; moved != nil && moved.NeedsRebase != nil {
		if by := moved.NeedsRebase.ByExecution; by != "" {
			p.Notes = append(p.Notes, fmt.Sprintf("%s moved while the worker ran (the merge of execution %s); the gate rebases onto its current head.", moved.NeedsRebase.Target, by))
		} else {
			p.Notes = append(p.Notes, fmt.Sprintf("%s moved after the last verification; the gate rebases onto its current head and verifies again.", moved.NeedsRebase.Target))
		}
	}
	owner := gateOwner(parent)
	p.TargetHead, p.BranchHead = targetHead, branchHead

	// 1. Verify the branch as the worker left it.
	if _, err := s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Problem = delegation.GateVerifying, owner, ""
		g.Target, g.TargetHead, g.BranchHead, g.Rebased, g.Runs = target, targetHead, branchHead, false, nil
	}); err != nil {
		return p, err
	}
	first := s.verify(ctx, w, commands.Verify, "verify", branchHead)
	p.Verification = append(p.Verification, first)
	p.Outputs = append(p.Outputs, delegation.RunOutput{Step: "verify on " + shortCommit(branchHead), Output: first.Output})
	if first.Outcome != delegation.RunPassed {
		p.Status = delegation.MergeVerifyFailed
		return p, s.gateFailure(ctx, a.ID, verifyStep(first), verifyProblem(first, "the branch as the worker left it"), p)
	}

	// 2. Rebase onto the target's current head. A verify command that
	// changed tracked files left work no one committed: the gate does not
	// rebase over it.
	if changed, err := worktree.TrackedChanges(ctx, w); err != nil {
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, "the worktree's status could not be read after verification: "+gateWords(err), p)
	} else if len(changed) > 0 {
		p.Status = delegation.MergeVerifyFailed
		return p, s.gateFailure(ctx, a.ID, delegation.GateVerifyFailed, fmt.Sprintf("The verify command changed %d tracked file(s) in the worktree, so the gate stopped before rebasing; nothing was merged. A check should not modify the code it checks: ask the owner to fix the command, or continue the worker so the harness commits those changes with its work.", len(changed)), p)
	}
	if _, err := s.store.AuthorizeBuildGate(ctx, parent, a.ID); err != nil {
		return p, s.gateFailure(ctx, a.ID, delegation.GateInterrupted, "the parent's authority ended during verification", p)
	}
	if targetHead, err = worktree.TargetHead(ctx, w.Repo, target); err != nil {
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, err.Error(), p)
	}
	p.TargetHead = targetHead
	if _, err := s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
		g.Step, g.TargetHead, g.Runs = delegation.GateRebasing, targetHead, stripOutputs(p.Verification)
	}); err != nil {
		return p, err
	}
	rebase, err := worktree.Rebase(ctx, w, targetHead)
	if err != nil {
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, "the rebase failed and was aborted: "+gateWords(err), p)
	}
	if len(rebase.Conflicts) > 0 || rebase.Unlisted > 0 {
		p.Status, p.Conflicts = delegation.MergeRebaseConflict, rebase.Conflicts
		problem := fmt.Sprintf("Rebasing onto %s at %s conflicts in %d file(s); the rebase was aborted, the branch is unchanged and nothing was merged. A worker cannot rebase (it runs no git that writes): delegate the change again with base %q, giving it this result, or resolve the conflict yourself.",
			target, shortCommit(targetHead), len(rebase.Conflicts)+rebase.Unlisted, target)
		return p, s.gateFailure(ctx, a.ID, delegation.GateRebaseConflict, problem, p)
	}
	p.Rebased, p.BranchHead = rebase.Rebased, rebase.Head
	if rebase.Rebased {
		if err := s.store.RecordBuildRebase(context.WithoutCancel(ctx), a.ID, target, targetHead); err != nil {
			return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, "the rebase could not be recorded: "+gateWords(err), p)
		}
		w.Base, w.BaseCommit = target, targetHead
	}

	// 3. Verify again on the rebased branch. When the branch already
	// contained the target's head, the first run verified this commit.
	if rebase.Rebased {
		if _, err := s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
			g.Step, g.BranchHead, g.Rebased = delegation.GateVerifyingAfter, rebase.Head, true
		}); err != nil {
			return p, err
		}
		second := s.verify(ctx, w, commands.Verify, "verify_rebased", rebase.Head)
		p.Verification = append(p.Verification, second)
		p.Outputs = append(p.Outputs, delegation.RunOutput{Step: "verify after rebasing onto " + target + " " + shortCommit(targetHead), Output: second.Output})
		if second.Outcome != delegation.RunPassed {
			p.Status = delegation.MergeVerifyFailed
			return p, s.gateFailure(ctx, a.ID, verifyStep(second), verifyProblem(second, "the branch rebased onto "+target), p)
		}
	} else {
		now := s.now().UTC()
		p.Verification = append(p.Verification, delegation.GateRun{Step: "verify_rebased", Command: commands.Verify, Commit: rebase.Head, Outcome: delegation.RunSkipped,
			Detail: fmt.Sprintf("the branch already contains %s at %s, so the first run verified this exact commit", target, shortCommit(targetHead)), StartedAt: now, EndedAt: &now})
	}

	// The summary the owner approves.
	summary, err := worktree.Compare(ctx, w.Repo, targetHead, p.BranchHead)
	if err != nil {
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, "the branch could not be summarized: "+gateWords(err), p)
	}
	p.Commits, p.Files, p.FilesChanged, p.Insertions, p.Deletions = summary.Commits, summary.Files, summary.FilesChanged, summary.Insertions, summary.Deletions
	p.Warnings = commandWarnings(ctx, w.Repo, targetHead, p.BranchHead, commands)
	if at, found, err := worktree.CheckedOutAt(ctx, w.Repo, target); err == nil && found {
		p.Where = fmt.Sprintf("git merge --ff-only in the checkout at %s, where %s is checked out; uncommitted changes there are kept, and git refuses if they would be overwritten", resolvedPath(at), target)
	} else {
		p.Where = fmt.Sprintf("compare-and-swap of refs/heads/%s from %s to %s (it is not checked out anywhere)", target, shortCommit(targetHead), shortCommit(p.BranchHead))
	}
	p.Status = delegation.MergeReady
	if _, err := s.store.RecordBuildGate(context.WithoutCancel(ctx), a.ID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Problem = delegation.GateAwaiting, nil, ""
		g.Target, g.TargetHead, g.BranchHead, g.Rebased = target, targetHead, p.BranchHead, p.Rebased
		g.Runs, g.NeedsRebase = stripOutputs(p.Verification), nil
	}); err != nil {
		return p, err
	}
	return p, nil
}

func resolvedPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

// verifyStep is the gate step a verification run that did not pass ends.
func verifyStep(run delegation.GateRun) string {
	if run.Outcome == delegation.RunInterrupted {
		return delegation.GateInterrupted
	}
	return delegation.GateVerifyFailed
}

// verifyProblem words a failed verification run.
func verifyProblem(run delegation.GateRun, what string) string {
	switch run.Outcome {
	case delegation.RunFailed:
		return fmt.Sprintf("The verify command failed on %s (exit %d); nothing was merged. Continue the worker with continue_research and the failure output below, then propose the merge again.", what, run.ExitStatus)
	case delegation.RunTimeout:
		return fmt.Sprintf("The verify command did not finish on %s within its time limit and was stopped; nothing was merged.", what)
	case delegation.RunInterrupted:
		return fmt.Sprintf("Verification of %s was interrupted; nothing was merged. Propose the merge again.", what)
	}
	return fmt.Sprintf("The verify command could not run on %s (%s); nothing was merged.", what, run.Detail)
}

// noVerifyCommand refuses a proposal for a scope without a verify command,
// naming how the owner sets one and what the repository's instructions
// suggest. Evie never sets it itself.
func noVerifyCommand(parent delegation.Parent, repo string) string {
	how := "the owner sets one with `evie workspaces verify " + string(parent.Scope.WorkspaceID) + " --verify '<command>'` or in the Workspace's settings"
	if parent.Scope.WorkspaceID == "" {
		how = "the owner sets one with `evie projects verify " + string(parent.Scope.ProjectID) + " --verify '<command>'`"
	}
	message := "No verify command is set for this " + scopeNoun(parent.Scope) + ", so Evie cannot propose a merge: " + how + ". Evie never sets it itself."
	if proposal := worktree.ProposeCommands(repo); proposal.Verify != "" {
		message += fmt.Sprintf(" The repository's %s suggests verify `%s`", proposal.VerifySource, proposal.Verify)
		if proposal.Setup != "" {
			message += fmt.Sprintf(" and setup `%s` (%s)", proposal.Setup, proposal.SetupSource)
		}
		message += "; ask the owner whether to use them."
	}
	return message
}

func scopeNoun(scope memory.ScopeContext) string {
	if scope.WorkspaceID != "" {
		return "Workspace"
	}
	return "project"
}

// verify runs the verify command on commit in w's worktree, offline.
func (s *Supervisor) verify(ctx context.Context, w delegation.Worktree, command, step, commit string) delegation.GateRun {
	run := delegation.GateRun{Step: step, Command: command, Commit: commit, StartedAt: s.now().UTC(), Log: filepath.Join(w.Temp, "evie-"+step+".log")}
	result, err := tools.RunGateCommand(ctx, tools.GateCommand{Fence: tools.BuildFence{Worktree: w.Path, Temp: w.Temp}, Command: command, Timeout: s.verifyTimeout, Log: run.Log})
	finishRun(ctx, &run, result, err, s.now().UTC())
	return run
}

// commandWarnings names files the branch changes that the setup or verify
// command runs: a branch that edits its own checks deserves a look.
func commandWarnings(ctx context.Context, repo, from, to string, commands memory.BuildVerify) []string {
	changed, err := worktree.ChangedPaths(ctx, repo, from, to)
	if err != nil {
		return nil
	}
	var warnings []string
	for _, c := range []struct{ name, command string }{{"verify", commands.Verify}, {"setup", commands.Setup}} {
		for _, path := range commandPaths(c.command) {
			for _, file := range changed {
				if file == path || strings.HasPrefix(file, path+"/") {
					if !worktree.PlainPath(file) {
						// A worker-chosen name is never shown unframed.
						file = path + "/…"
					}
					warning := fmt.Sprintf("The branch changes %s, which the %s command runs; review that change before approving.", file, c.name)
					if !slices.Contains(warnings, warning) {
						warnings = append(warnings, warning)
					}
				}
			}
		}
	}
	return warnings
}

// commandPaths are the repository-relative paths a command names: words
// with a slash, or starting with ./, that are not options or absolute.
func commandPaths(command string) []string {
	var paths []string
	for _, word := range strings.FieldsFunc(command, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ';' || r == '&' || r == '|' || r == '(' || r == ')' || r == '"' || r == '\''
	}) {
		if strings.HasPrefix(word, "-") || filepath.IsAbs(word) || strings.Contains(word, "$") || !(strings.Contains(word, "/") || strings.HasPrefix(word, "./")) {
			continue
		}
		if _, value, ok := strings.Cut(word, "="); ok {
			word = value
		}
		clean := filepath.ToSlash(filepath.Clean(word))
		if clean == "." || strings.HasPrefix(clean, "../") {
			continue
		}
		paths = append(paths, clean)
	}
	return paths
}

// Merge performs the fast-forward the owner approved for proposal, as the
// harness: only if the attempt's gate still awaits exactly this approval,
// the branch is still at the verified head and the target has not moved
// since verification. A moved target is reported, nothing merged, and the
// next proposal rebases and verifies again. After the merge the worktree
// and branch are removed, and other build branches waiting on the target
// learn that it moved.
func (s *Supervisor) Merge(ctx context.Context, parent delegation.Parent, approved delegation.MergeProposal) (delegation.MergeProposal, error) {
	p := approved
	p.Outputs = nil
	a, err := s.store.AuthorizeBuildGate(ctx, parent, approved.ExecutionID)
	if err != nil {
		return p, err
	}
	g := a.Gate
	if g == nil || g.Step != delegation.GateAwaiting || g.Target != approved.Target || g.TargetHead != approved.TargetHead || g.BranchHead != approved.BranchHead {
		return p, refuse(p, "this proposal is no longer the one awaiting approval; nothing was merged. Propose the merge again.")
	}
	if err := s.beginGate(a.ID); err != nil {
		return p, refuse(p, "%v", err)
	}
	defer s.endGate(a.ID)
	w := *a.Worktree
	head, err := worktree.BranchHead(ctx, w)
	if err != nil || head != approved.BranchHead {
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, fmt.Sprintf("%s is no longer at the verified head %s; nothing was merged. Propose the merge again.", w.Branch, shortCommit(approved.BranchHead)), p)
	}
	current, err := worktree.TargetHead(ctx, w.Repo, approved.Target)
	if err != nil {
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, err.Error(), p)
	}
	if current != approved.TargetHead {
		return p, s.targetMoved(ctx, a, p, current)
	}
	if _, err := s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Problem = delegation.GateMerging, gateOwner(parent), ""
	}); err != nil {
		return p, err
	}
	where, err := worktree.FastForward(ctx, w, approved.Target, approved.TargetHead, approved.BranchHead)
	var refused *worktree.MergeRefusedError
	switch {
	case errors.Is(err, worktree.ErrTargetMoved):
		now, _ := worktree.TargetHead(context.WithoutCancel(ctx), w.Repo, approved.Target)
		return p, s.targetMoved(ctx, a, p, now)
	case errors.As(err, &refused):
		p.Status = delegation.MergeMergeRefused
		return p, s.gateFailure(ctx, a.ID, delegation.GateMergeRefused, fmt.Sprintf("Nothing was merged: %s. Evie never stashes, resets or overwrites the owner's changes; once the owner commits or sets them aside, propose the merge again.", refused.Error()), p)
	case err != nil:
		return p, s.gateFailure(ctx, a.ID, delegation.GateRefused, "the merge failed: "+gateWords(err), p)
	}
	return s.completeMerge(context.WithoutCancel(ctx), a, p, delegation.GateMerge{Target: approved.Target, From: approved.TargetHead, To: approved.BranchHead, Where: where, At: s.now().UTC()})
}

// targetMoved records that the target moved between verification and the
// merge: nothing was merged, and the next proposal rebases and verifies
// again.
func (s *Supervisor) targetMoved(ctx context.Context, a delegation.Attempt, p delegation.MergeProposal, now string) error {
	p.Status = delegation.MergeTargetMoved
	move := delegation.TargetMove{Target: p.Target, From: p.TargetHead, To: now, At: s.now().UTC()}
	_, _ = s.store.RecordBuildGate(context.WithoutCancel(ctx), a.ID, func(g *delegation.BuildGate) { g.NeedsRebase = &move })
	return s.gateFailure(ctx, a.ID, delegation.GateTargetMoved, fmt.Sprintf("%s moved from %s to %s after verification, so nothing was merged. Propose the merge again: the gate rebases onto the new head and verifies again before asking the owner.",
		p.Target, shortCommit(p.TargetHead), shortCommit(now)), p)
}

// completeMerge records a merge, removes the worktree and branch, and tells
// other build branches on the same target that it moved.
func (s *Supervisor) completeMerge(ctx context.Context, a delegation.Attempt, p delegation.MergeProposal, merged delegation.GateMerge) (delegation.MergeProposal, error) {
	w := *a.Worktree
	if _, err := s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
		g.Step, g.Owner, g.Merged = delegation.GateMerged, nil, &merged
	}); err != nil {
		return p, fmt.Errorf("the merge happened (%s is now at %s) but could not be recorded: %w", merged.Target, shortCommit(merged.To), err)
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, gateCleanupTimeout)
	defer cancel()
	merged.Cleanup = "the worktree and branch " + w.Branch + " were removed"
	if err := worktree.Remove(cleanupCtx, w, true); err != nil {
		merged.Cleanup = fmt.Sprintf("the worktree and branch were kept (%s); `evie worktrees clean --delete-branch %s` removes them", gateWords(err), a.ID)
	} else if err := s.store.MarkBuildWorktreeRemoved(cleanupCtx, a.ID); err != nil {
		merged.Cleanup = "the worktree and branch were removed, but the record could not be updated: " + gateWords(err)
	}
	_, _ = s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) { g.Merged = &merged })
	p.Status, p.Merged = delegation.MergeMerged, &merged
	p.Notes = append(p.Notes, fmt.Sprintf("Merged %s into %s (fast-forward to %s) by %s; %s.", w.Branch, merged.Target, shortCommit(merged.To), merged.Where, merged.Cleanup))
	move := delegation.TargetMove{Target: merged.Target, From: merged.From, To: merged.To, ByExecution: a.ID, At: merged.At}
	marked, err := s.store.MarkBuildTargetMoved(ctx, w.Repo, move, a.Child.ID)
	if err == nil && len(marked) > 0 {
		// Workers still running are told on their next model call.
		s.mu.Lock()
		for _, other := range marked {
			if !other.Terminal() {
				s.moved[other.ID] = move
			}
		}
		s.mu.Unlock()
		p.Notes = append(p.Notes, fmt.Sprintf("%d other build branch(es) targeting %s were marked as needing a rebase; their next merge proposal rebases onto it.", len(marked), merged.Target))
	}
	return p, nil
}

// RecoverGates settles merge-gate steps that a crash or a lost turn left in
// progress, once their owner's turn is gone and no gate of this process
// runs them: an interrupted rebase is aborted so the branch is as it was;
// an interrupted verification is marked so; an interrupted merge is checked
// against the target, which a fast-forward or compare-and-swap moved
// completely or not at all. Each attempt's record shows the step it ended
// in.
func (s *Supervisor) RecoverGates(ctx context.Context) error {
	candidates, err := s.store.BuildGateRecoveryCandidates(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, a := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.gateActive(a.ID) {
			continue
		}
		if err := s.recoverGate(ctx, a); err != nil {
			failures = append(failures, fmt.Errorf("recover the merge gate of execution %q: %w", a.ID, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Supervisor) recoverGate(ctx context.Context, a delegation.Attempt) error {
	if err := s.beginGate(a.ID); err != nil {
		return nil
	}
	defer s.endGate(a.ID)
	w, g := *a.Worktree, *a.Gate
	interrupted := func(problem string) error {
		_, err := s.store.RecordBuildGate(ctx, a.ID, func(g *delegation.BuildGate) {
			g.Step, g.Owner, g.Problem = delegation.GateInterrupted, nil, problem
		})
		return err
	}
	switch g.Step {
	case delegation.GateSetupRunning:
		return interrupted("The setup run was interrupted before it was recorded.")
	case delegation.GateRebasing:
		if worktree.Exists(w) {
			if _, err := worktree.AbortRebase(ctx, w); err != nil {
				return err
			}
		}
		return interrupted("The rebase was interrupted; the harness aborted it, so the branch is as it was. Propose the merge again.")
	case delegation.GateMerging:
		current, err := worktree.TargetHead(ctx, w.Repo, g.Target)
		if err != nil {
			return interrupted("The merge was interrupted and the target could not be read: " + gateWords(err))
		}
		if current == g.BranchHead && g.BranchHead != "" {
			// The fast-forward completed before the interruption.
			p := delegation.MergeProposal{ExecutionID: a.ID, Branch: w.Branch, Target: g.Target, TargetHead: g.TargetHead, BranchHead: g.BranchHead}
			_, err := s.completeMerge(ctx, a, p, delegation.GateMerge{Target: g.Target, From: g.TargetHead, To: g.BranchHead,
				Where: "found complete after an interruption", At: s.now().UTC()})
			return err
		}
		return interrupted(fmt.Sprintf("The merge was interrupted before %s moved; nothing was merged. Propose the merge again.", g.Target))
	}
	return interrupted("Verification was interrupted; nothing was merged. Propose the merge again.")
}
