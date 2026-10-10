package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// Evie's Changes (changes.spec.md story 3a) are read from the build
// attempts that already record their branch, base, origin, merge gate and
// worktree removal: nothing new is stored for them.

// ErrUnfinishedChanges refuses to change or detach a Workspace's folder
// while a draft or open Change of the Workspace lives in it: a build
// worktree there that is neither merged nor removed.
var ErrUnfinishedChanges = errors.New("This Workspace has unfinished Changes")

type attemptQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// WorkspaceBuilds returns the latest attempt on each build worktree that
// belongs to Workspace ws, newest first: delegated from a chat in the
// Workspace, or run by one of its Workflow Runs.
func (s *Store) WorkspaceBuilds(ctx context.Context, ws memory.WorkspaceID) ([]delegation.Attempt, error) {
	return workspaceBuilds(ctx, s.db, ws)
}

// workspaceBuilds reads every build attempt as ListBuildWorktrees does,
// keeping the latest attempt on each worktree (a continuation shares its
// original's), and the Workspace's among them.
func workspaceBuilds(ctx context.Context, q attemptQueryer, ws memory.WorkspaceID) ([]delegation.Attempt, error) {
	rows, err := q.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE CASE WHEN json_valid(record_json) THEN json_type(record_json,'$.worktree') IS NOT NULL END ORDER BY rowid DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	var out []delegation.Attempt
	for rows.Next() {
		a, err := readSubagent(rows)
		if err != nil || a.Worktree == nil || seen[a.Worktree.Path] {
			continue
		}
		seen[a.Worktree.Path] = true
		if buildWorkspace(a) == ws {
			out = append(out, a)
		}
	}
	return out, rows.Err()
}

// buildWorkspace is the Workspace a build attempt belongs to: its Workflow
// Run's, or its parent chat's.
func buildWorkspace(a delegation.Attempt) memory.WorkspaceID {
	if a.Run != nil && a.Run.WorkspaceID != "" {
		return a.Run.WorkspaceID
	}
	return a.Parent.Scope.WorkspaceID
}

// checkNoUnfinishedChanges refuses while Workspace ws has a build worktree
// in folder that is neither merged nor removed.
func checkNoUnfinishedChanges(ctx context.Context, conn *sql.Conn, ws memory.WorkspaceID, folder string) error {
	if folder == "" {
		return nil
	}
	attempts, err := workspaceBuilds(ctx, conn, ws)
	if err != nil {
		return err
	}
	unfinished := 0
	for _, a := range attempts {
		if a.Worktree.RemovedAt != nil || (a.Gate != nil && a.Gate.Merged != nil) || !sameFolder(a.Worktree.Repo, folder) {
			continue
		}
		unfinished++
	}
	if unfinished > 0 {
		return fmt.Errorf("%w in its folder (%d not merged or closed): merge them, or remove their worktrees with `evie worktrees clean`, before changing or detaching the folder", ErrUnfinishedChanges, unfinished)
	}
	return nil
}
