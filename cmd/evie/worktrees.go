package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/worktree"
)

const worktreesUsage = "usage: evie worktrees list | evie worktrees clean [--force] [--delete-branch] <execution-id>... | evie worktrees clean [--force] [--delete-branch] --abandoned"

// worktreeListing is one build worktree as `evie worktrees list` shows it.
type worktreeListing struct {
	ExecutionID string `json:"execution_id"`
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Branch      string `json:"branch"`
	Base        string `json:"base"`
	Path        string `json:"path"`
	Present     bool   `json:"present"`
	Uncommitted bool   `json:"uncommitted"`
	Removed     bool   `json:"removed"`
	Problem     string `json:"problem,omitempty"`
	// The merge gate (Stage 12, 2026-10-04): its last step and problem, how
	// setup went, whether the target moved since, and where it merged.
	GateStep    string `json:"gate_step,omitempty"`
	GateProblem string `json:"gate_problem,omitempty"`
	Setup       string `json:"setup,omitempty"`
	NeedsRebase string `json:"needs_rebase,omitempty"`
	MergedInto  string `json:"merged_into,omitempty"`
}

// abandonedStates are build executions that ended without a report to
// review: their worktrees hold nothing the parent is waiting on.
var abandonedStates = []string{"interrupted", "failed", "cancelled"}

// runWorktreeManagement answers `evie worktrees list`, which shows every
// build worker's worktree with its execution's state, and `evie worktrees
// clean`, which removes the named executions' worktrees, or every abandoned
// one (Stage 11, 2026-10-03). A worktree with uncommitted changes is kept
// unless --force is given, and branches are kept unless --delete-branch is.
// It never touches an execution that is still running.
func runWorktreeManagement(ctx context.Context, args []string, out io.Writer, store *eviedb.Store) (bool, error) {
	if len(args) == 0 || args[0] != "worktrees" {
		return false, nil
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	switch {
	case len(args) == 2 && args[1] == "list":
		entries, err := store.ListBuildWorktrees(ctx)
		if err != nil {
			return true, err
		}
		listed := make([]worktreeListing, 0, len(entries))
		for _, e := range entries {
			row := worktreeListing{ExecutionID: e.ExecutionID, State: e.State, Reason: e.Reason, TaskID: e.TaskID, Branch: e.Worktree.Branch,
				Base: e.Worktree.Base, Path: e.Worktree.Path, Removed: e.Worktree.RemovedAt != nil}
			present, dirty, err := worktree.Status(ctx, e.Worktree)
			row.Present, row.Uncommitted = present, dirty
			if err != nil {
				row.Problem = err.Error()
			}
			if e.Setup != nil {
				row.Setup = e.Setup.Outcome
			}
			if g := e.Gate; g != nil {
				row.GateStep, row.GateProblem = g.Step, g.Problem
				if g.NeedsRebase != nil {
					row.NeedsRebase = fmt.Sprintf("%s moved to %s", g.NeedsRebase.Target, g.NeedsRebase.To)
				}
				if g.Merged != nil {
					row.MergedInto = fmt.Sprintf("%s at %s", g.Merged.Target, g.Merged.To)
				}
			}
			listed = append(listed, row)
		}
		return true, encoder.Encode(struct {
			Worktrees []worktreeListing `json:"worktrees"`
		}{listed})
	case len(args) >= 3 && args[1] == "clean":
		var force, deleteBranch, abandoned bool
		var ids []string
		for _, arg := range args[2:] {
			switch arg {
			case "--force":
				force = true
			case "--delete-branch":
				deleteBranch = true
			case "--abandoned":
				abandoned = true
			default:
				ids = append(ids, arg)
			}
		}
		if abandoned == (len(ids) > 0) {
			return true, errors.New(worktreesUsage)
		}
		if abandoned {
			entries, err := store.ListBuildWorktrees(ctx)
			if err != nil {
				return true, err
			}
			for _, e := range entries {
				if e.Worktree.RemovedAt == nil && slices.Contains(abandonedStates, e.State) {
					ids = append(ids, e.ExecutionID)
				}
			}
		}
		type outcome struct {
			ExecutionID string `json:"execution_id"`
			Removed     bool   `json:"removed"`
			Skipped     string `json:"skipped,omitempty"`
		}
		var outcomes []outcome
		var failures []error
		for _, id := range ids {
			w, err := store.BuildWorktreeFor(ctx, id)
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if w.RemovedAt != nil {
				outcomes = append(outcomes, outcome{ExecutionID: id, Skipped: "already removed"})
				continue
			}
			if _, dirty, err := worktree.Status(ctx, w); err == nil && dirty && !force {
				outcomes = append(outcomes, outcome{ExecutionID: id, Skipped: "the worktree has uncommitted changes; commit them on its branch, or pass --force to discard them"})
				continue
			}
			if err := worktree.Remove(ctx, w, deleteBranch); err != nil {
				failures = append(failures, fmt.Errorf("execution %s: %w", id, err))
				continue
			}
			if err := store.MarkBuildWorktreeRemoved(ctx, id); err != nil {
				failures = append(failures, fmt.Errorf("execution %s: %w", id, err))
				continue
			}
			outcomes = append(outcomes, outcome{ExecutionID: id, Removed: true})
		}
		if outcomes == nil {
			outcomes = []outcome{}
		}
		if err := encoder.Encode(struct {
			Cleaned []outcome `json:"cleaned"`
		}{outcomes}); err != nil {
			return true, err
		}
		return true, errors.Join(failures...)
	}
	return true, errors.New(worktreesUsage)
}
