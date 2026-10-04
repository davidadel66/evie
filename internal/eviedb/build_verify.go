package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// The merge gate's commands (Stage 12 of docs/memory-architecture.md,
// 2026-10-04). A Workspace or project declares a setup command (optional,
// run with network in a fresh build worktree on its base commit) and a
// verify command (run offline on a worker's branch before and after its
// rebase). Only the owner sets them, through the CLI or the management API;
// Evie may propose values from the repository's instructions but never
// records them. Each change is an immutable revision written with a
// compare-and-set on the expected revision, and the latest applies at once,
// to running chats too, like the Workspace folder: a corrected or removed
// command must not keep running in older chats.

var (
	// ErrBuildVerifyChanged refuses a write whose expected revision is not
	// the latest.
	ErrBuildVerifyChanged = errors.New("the setup and verify commands changed; refresh and try again")
	// ErrBuildVerifyCommand refuses a command that is too long or holds a
	// control character other than a tab or newline.
	ErrBuildVerifyCommand = fmt.Errorf("a setup or verify command must be at most %d bytes with no control characters other than tabs and newlines", memory.MaxBuildCommandBytes)
)

const (
	verifyScopeWorkspace = "workspace"
	verifyScopeProject   = "project"
)

func ensureBuildVerify(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS build_verify_revisions (
 scope_kind TEXT NOT NULL CHECK(scope_kind IN ('workspace','project')),
 scope_id TEXT NOT NULL CHECK(length(scope_id) > 0),
 revision INTEGER NOT NULL CHECK(revision > 0),
 setup_command TEXT NOT NULL,
 verify_command TEXT NOT NULL,
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(scope_kind, scope_id, revision));
 CREATE TRIGGER IF NOT EXISTS build_verify_revisions_no_update BEFORE UPDATE ON build_verify_revisions
 BEGIN SELECT RAISE(ABORT,'build verify revisions are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS build_verify_revisions_no_delete BEFORE DELETE ON build_verify_revisions
 BEGIN SELECT RAISE(ABORT,'build verify revisions are immutable'); END;`)
	if err != nil {
		return fmt.Errorf("create build verify settings: %w", err)
	}
	return nil
}

// cleanBuildCommand trims a command and refuses one the gate cannot run as
// one shell line or script.
func cleanBuildCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if len(command) > memory.MaxBuildCommandBytes {
		return "", ErrBuildVerifyCommand
	}
	for _, r := range command {
		if r < 0x20 && r != '\t' && r != '\n' || r == 0x7f {
			return "", ErrBuildVerifyCommand
		}
	}
	return command, nil
}

func buildVerify(ctx context.Context, q queryRower, kind, id string) (memory.BuildVerify, error) {
	var v memory.BuildVerify
	err := q.QueryRowContext(ctx, `SELECT setup_command,verify_command,revision FROM build_verify_revisions
 WHERE scope_kind=? AND scope_id=? ORDER BY revision DESC LIMIT 1`, kind, id).Scan(&v.Setup, &v.Verify, &v.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.BuildVerify{}, nil
	}
	if err != nil {
		return v, fmt.Errorf("read setup and verify commands: %w", err)
	}
	return v, nil
}

// BuildVerifyFor is the commands that apply to a chat in scope: its
// Workspace's, or its project's; a Global chat has none.
func (s *Store) BuildVerifyFor(ctx context.Context, scope memory.ScopeContext) (memory.BuildVerify, error) {
	switch {
	case scope.WorkspaceID != "":
		return buildVerify(ctx, s.db, verifyScopeWorkspace, string(scope.WorkspaceID))
	case scope.ProjectID != "":
		return buildVerify(ctx, s.db, verifyScopeProject, string(scope.ProjectID))
	}
	return memory.BuildVerify{}, nil
}

// insertBuildVerify records the next revision if expected is still the
// latest.
func insertBuildVerify(ctx context.Context, conn *sql.Conn, kind, id string, expected int64, setup, verify string, now time.Time) (memory.BuildVerify, error) {
	setup, err := cleanBuildCommand(setup)
	if err != nil {
		return memory.BuildVerify{}, err
	}
	verify, err = cleanBuildCommand(verify)
	if err != nil {
		return memory.BuildVerify{}, err
	}
	current, err := buildVerify(ctx, conn, kind, id)
	if err != nil {
		return memory.BuildVerify{}, err
	}
	if expected != current.Revision {
		return memory.BuildVerify{}, ErrBuildVerifyChanged
	}
	next := memory.BuildVerify{Setup: setup, Verify: verify, Revision: expected + 1}
	if _, err := conn.ExecContext(ctx, `INSERT INTO build_verify_revisions(scope_kind,scope_id,revision,setup_command,verify_command,recorded_at) VALUES(?,?,?,?,?,?)`,
		kind, id, next.Revision, next.Setup, next.Verify, now.Format(time.RFC3339Nano)); err != nil {
		return memory.BuildVerify{}, fmt.Errorf("save setup and verify commands: %w", err)
	}
	return next, nil
}

// SetWorkspaceVerify records the owner's setup and verify commands for an
// active Workspace as its next revision, if expected is the latest. Empty
// clears a command.
func (s *Store) SetWorkspaceVerify(ctx context.Context, id memory.WorkspaceID, expected int64, setup, verify string) (memory.Workspace, error) {
	var workspace memory.Workspace
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		workspace, err = scanWorkspace(conn.QueryRowContext(ctx, `SELECT id,display_name,lifecycle_state,current_revision_id,created_at,updated_at FROM workspaces WHERE id=?`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
		}
		if err != nil {
			return err
		}
		if workspace.State != memory.WorkspaceActive {
			return fmt.Errorf("%w: %s is archived", ErrWorkspaceNotFound, id)
		}
		_, err = insertBuildVerify(ctx, conn, verifyScopeWorkspace, string(id), expected, setup, verify, s.now().UTC())
		return err
	})
	if err != nil {
		return memory.Workspace{}, err
	}
	if err := s.loadWorkspacePresets(ctx, &workspace); err != nil {
		return memory.Workspace{}, err
	}
	return s.completeWorkspace(ctx, workspace)
}

// SetProjectVerify records the owner's commands for a registered project
// that is not archived, if expected is the latest revision.
func (s *Store) SetProjectVerify(ctx context.Context, id memory.ProjectID, expected int64, setup, verify string) (memory.BuildVerify, error) {
	var next memory.BuildVerify
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var archived bool
		err := conn.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, id).Scan(&archived)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrProjectNotFound, id)
		}
		if err != nil {
			return err
		}
		if archived {
			return ErrProjectNotActive
		}
		next, err = insertBuildVerify(ctx, conn, verifyScopeProject, string(id), expected, setup, verify, s.now().UTC())
		return err
	})
	return next, err
}
