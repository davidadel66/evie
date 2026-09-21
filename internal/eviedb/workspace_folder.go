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

// A revision compare-and-insert avoids a stale browser silently replacing a
// newer attachment. Empty path detaches without removing files or history.
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
	result, err := s.db.ExecContext(ctx, `INSERT INTO workspace_folder_revisions(workspace_id,revision,path,recorded_at)
 SELECT id, ?, ?, ? FROM workspaces WHERE id = ? AND lifecycle_state = 'active'
 AND COALESCE((SELECT MAX(revision) FROM workspace_folder_revisions WHERE workspace_id = ?),0) = ?`,
		expected+1, path, s.now().UTC().Format(time.RFC3339Nano), id, id, expected)
	if err != nil {
		return memory.WorkspaceFolder{}, fmt.Errorf("save Workspace folder: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return memory.WorkspaceFolder{}, err
	}
	if n != 1 {
		return memory.WorkspaceFolder{}, ErrWorkspaceFolderChanged
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
