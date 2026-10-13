package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrWorkspaceFolderChanged = errors.New("Workspace folder changed; refresh and try again")

func ensureWorkspaceFolders(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_folder_revisions (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), revision INTEGER NOT NULL CHECK(revision > 0),
 path TEXT NOT NULL, recorded_at TEXT NOT NULL, PRIMARY KEY(workspace_id, revision)
 );`)
	return err
}

func (s *Store) WorkspaceFolder(ctx context.Context, id memory.WorkspaceID) (memory.WorkspaceFolder, error) {
	var folder memory.WorkspaceFolder
	err := s.db.QueryRowContext(ctx, `SELECT
 COALESCE((SELECT path FROM workspace_folder_revisions WHERE workspace_id = w.id ORDER BY revision DESC LIMIT 1), ''),
 COALESCE((SELECT MAX(revision) FROM workspace_folder_revisions WHERE workspace_id = w.id), 0)
 FROM workspaces w WHERE w.id = ?`, id).Scan(&folder.Path, &folder.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		return folder, ErrWorkspaceNotFound
	}
	return folder, err
}

// ActiveWorkspaceFolder is an active Workspace's folder path, "" when it has
// none. A missing or archived Workspace is ErrWorkspaceNotFound, so nothing
// is served from an archived Workspace's folder (workspace-tabs.spec.md).
func (s *Store) ActiveWorkspaceFolder(ctx context.Context, id memory.WorkspaceID) (string, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `SELECT
 COALESCE((SELECT path FROM workspace_folder_revisions WHERE workspace_id = w.id ORDER BY revision DESC LIMIT 1), '')
 FROM workspaces w WHERE w.id = ? AND w.lifecycle_state = ?`, id, memory.WorkspaceActive).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", ErrWorkspaceNotFound, id)
	}
	return path, err
}

// A revision compare-and-insert avoids a stale browser silently replacing a
// newer attachment. Empty path detaches without removing files or history.
// A change or detach is refused while a draft or open Change of the
// Workspace lives in its current folder (ErrUnfinishedChanges), checked in
// the same transaction as the write; setting the same folder again is not
// a change.
func (s *Store) SetWorkspaceFolder(ctx context.Context, id memory.WorkspaceID, expected int64, path string) (memory.WorkspaceFolder, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return memory.WorkspaceFolder{}, err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if path != "" {
		if !filepath.IsAbs(path) {
			return memory.WorkspaceFolder{}, errors.New("Choose an absolute folder path")
		}
		canonical, err := memory.CanonicalProjectRoot(path)
		if err != nil {
			return memory.WorkspaceFolder{}, fmt.Errorf("Folder is unavailable: %w", err)
		}
		path = canonical
	}
	if expected < 0 {
		return memory.WorkspaceFolder{}, ErrWorkspaceFolderChanged
	}
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var current string
		var revision int64
		err := conn.QueryRowContext(ctx, `SELECT
 COALESCE((SELECT path FROM workspace_folder_revisions WHERE workspace_id = w.id ORDER BY revision DESC LIMIT 1), ''),
 COALESCE((SELECT MAX(revision) FROM workspace_folder_revisions WHERE workspace_id = w.id), 0)
 FROM workspaces w WHERE w.id = ? AND w.lifecycle_state = 'active'`, id).Scan(&current, &revision)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && revision != expected) {
			return ErrWorkspaceFolderChanged
		}
		if err != nil {
			return err
		}
		if current != path && !sameFolder(current, path) {
			if err := checkNoUnfinishedChanges(ctx, conn, id, current); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workspace_folder_revisions(workspace_id,revision,path,recorded_at) VALUES(?,?,?,?)`,
			id, expected+1, path, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("save Workspace folder: %w", err)
		}
		return nil
	})
	if err != nil {
		return memory.WorkspaceFolder{}, err
	}
	return memory.WorkspaceFolder{Path: path, Revision: expected + 1}, nil
}

func (h *SessionHistory) WorkingDirectory(ctx context.Context) (string, error) {
	session, err := h.store.GetActiveSession(ctx, h.sessionID)
	if err != nil {
		return "", err
	}
	if session.ParentSessionID != "" {
		return "", nil
	}
	if session.WorkspaceID != "" {
		folder, err := h.store.WorkspaceFolder(ctx, session.WorkspaceID)
		return folder.Path, err
	}
	return session.ProjectRootSnapshot, nil
}
