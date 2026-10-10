package changes

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// Evie's Changes (changes.spec.md story 3a) are read from the build
// attempts that already record their branch, base, origin, merge gate and
// worktree removal; nothing new is stored for them. Each build worktree in
// the Workspace's Repository is one Change, read from its latest attempt.

// BuildRecords reads a Workspace's build attempts: the latest attempt on
// each build worktree that a chat in the Workspace delegated or one of its
// Workflow Runs ran.
type BuildRecords interface {
	WorkspaceBuilds(ctx context.Context, ws memory.WorkspaceID) ([]delegation.Attempt, error)
}

// BuildSource is the Source of Evie's build branches.
type BuildSource struct {
	records BuildRecords
}

func NewBuildSource(records BuildRecords) *BuildSource {
	return &BuildSource{records: records}
}

// Changes lists the Workspace's build branches whose worktree belongs to
// the Repository's folder.
func (b *BuildSource) Changes(ctx context.Context, ws memory.WorkspaceID, repo Repository) ([]Change, error) {
	if repo.Kind == None || repo.Folder == "" {
		return nil, nil
	}
	attempts, err := b.records.WorkspaceBuilds(ctx, ws)
	if err != nil {
		return nil, err
	}
	folder := canonicalPath(repo.Folder)
	var out []Change
	for _, a := range attempts {
		if a.Worktree == nil || canonicalPath(a.Worktree.Repo) != folder {
			continue
		}
		out = append(out, buildChange(ws, a, repo.Kind))
	}
	return out, nil
}

// buildChange reads one Change from its latest build attempt: merged once
// the gate merged it, closed once its worktree was removed without a
// merge, open while the gate awaits the owner's approval in a local
// Repository, and otherwise a draft. In a GitHub Repository open means a
// pull request, which List links in (story 4a).
func buildChange(ws memory.WorkspaceID, a delegation.Attempt, kind Kind) Change {
	w, g := *a.Worktree, a.Gate
	// The branch, not the attempt, names the Change: a continuation
	// attempt keeps the branch and so stays the same Change.
	c := Change{ID: "build:" + w.Branch, WorkspaceID: ws, ExecutionID: a.ID, Author: ByEvie, State: Draft, Branch: w.Branch, Base: w.Base}
	var report *delegation.BuildReport
	if a.Result != nil {
		report = a.Result.Build
	}
	if report != nil {
		c.Head, c.FilesChanged, c.Insertions, c.Deletions = report.Head, report.FilesChanged, report.Insertions, report.Deletions
	}
	if g != nil {
		// The gate's target is the branch the Change merges into, as the
		// gate itself reads it when a target moves.
		if g.Target != "" {
			c.Base = g.Target
		}
		if g.BranchHead != "" {
			c.Head = g.BranchHead
		}
	}
	switch {
	case g != nil && g.Merged != nil:
		c.State = Merged
		if g.Merged.To != "" {
			c.Head = g.Merged.To
		}
	case w.RemovedAt != nil:
		c.State = Closed
	case g != nil && g.Step == delegation.GateAwaiting && kind != GitHub:
		c.State = Open
	}
	if g != nil && (c.State == Draft || c.State == Open) {
		c.NeedsRebase = g.NeedsRebase != nil || g.Step == delegation.GateRebaseConflict
	}
	if w.RemovedAt == nil {
		c.Worktree = w.Path
	}
	if a.Run != nil {
		c.Origin = &Origin{RunID: a.Run.RunID, Workflow: a.Run.Workflow, Step: a.Run.Step}
	} else if a.Parent.Scope.SessionID != "" {
		c.Origin = &Origin{SessionID: string(a.Parent.Scope.SessionID)}
	}
	c.Title = buildTitle(a, report)
	c.Notes = buildNotes(a.Result)
	c.UpdatedAt = buildUpdatedAt(a)
	return c
}

// titleRunes bounds a Change's title as the harness bounds its commit
// subjects.
const titleRunes = 72

// buildTitle is the subject of the first commit made on the branch (the
// harness commits each attempt with its objective's first line), else the
// first line of the objective.
func buildTitle(a delegation.Attempt, report *delegation.BuildReport) string {
	if report != nil && len(report.Commits) > 0 {
		// Commits are listed newest first.
		if title := firstLine(report.Commits[len(report.Commits)-1].Subject); title != "" {
			return title
		}
	}
	if title := firstLine(a.Assignment.Objective); title != "" {
		return title
	}
	return a.Worktree.Branch
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.TrimSpace(line)
	if runes := []rune(line); len(runes) > titleRunes {
		line = strings.TrimSpace(string(runes[:titleRunes-1])) + "…"
	}
	return line
}

// buildNotes are the worker's own words from its report: the Summary, then
// its Limitations. The harness's notes and the full report stay with the
// attempt.
func buildNotes(r *delegation.Result) string {
	if r == nil {
		return ""
	}
	var parts []string
	if summary := strings.TrimSpace(r.Summary); summary != "" {
		parts = append(parts, summary)
	}
	var limitations []string
	for _, l := range r.Limitations {
		if l = strings.TrimSpace(l); l != "" {
			limitations = append(limitations, "- "+l)
		}
	}
	if len(limitations) > 0 {
		parts = append(parts, "Limitations:\n"+strings.Join(limitations, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// buildUpdatedAt is the latest time the attempt's record shows: created,
// started, ended, its last gate step, merged or its worktree removed.
func buildUpdatedAt(a delegation.Attempt) time.Time {
	latest := a.CreatedAt
	later := func(t *time.Time) {
		if t != nil && t.After(latest) {
			latest = *t
		}
	}
	later(a.StartedAt)
	later(a.EndedAt)
	later(a.Worktree.RemovedAt)
	if g := a.Gate; g != nil {
		later(&g.At)
		if g.Merged != nil {
			later(&g.Merged.At)
		}
	}
	return latest
}

// canonicalPath resolves symbolic links so a Repository's folder and a
// worktree's recorded repository compare as the same place.
func canonicalPath(path string) string {
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	return filepath.Clean(path)
}
