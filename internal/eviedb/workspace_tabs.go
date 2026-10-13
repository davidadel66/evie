package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// The owner's approvals of what a Workspace tab may read
// (workspace-tabs.spec.md, story 2). An approval pins the tab's exact data
// list by its key, so a changed list has no approval until the owner
// approves it again.
func ensureWorkspaceTabApprovals(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_tab_data_approvals (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id), tab_id TEXT NOT NULL,
 data_key TEXT NOT NULL, approved_at TEXT NOT NULL, PRIMARY KEY(workspace_id, tab_id)
 )`)
	return err
}

var tabDataKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ApproveWorkspaceTabData records that the owner approved this exact data
// list for an active Workspace's tab.
func (s *Store) ApproveWorkspaceTabData(ctx context.Context, workspace memory.WorkspaceID, tab, key string) error {
	if !tabDataKeyPattern.MatchString(key) || tab == "" || len(tab) > 32 {
		return fmt.Errorf("approve tab data: invalid tab or data key")
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO workspace_tab_data_approvals (workspace_id, tab_id, data_key, approved_at)
 SELECT id, ?, ?, ? FROM workspaces WHERE id = ? AND lifecycle_state = ?
 ON CONFLICT(workspace_id, tab_id) DO UPDATE SET data_key = excluded.data_key, approved_at = excluded.approved_at`,
		tab, key, s.now().UTC().Format(time.RFC3339Nano), workspace, memory.WorkspaceActive)
	if err != nil {
		return fmt.Errorf("approve tab data: %w", err)
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("%w: %s", ErrWorkspaceNotFound, workspace)
	}
	return nil
}

// WithdrawWorkspaceTabData removes a tab's approval; it reads nothing until
// approved again.
func (s *Store) WithdrawWorkspaceTabData(ctx context.Context, workspace memory.WorkspaceID, tab string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM workspace_tab_data_approvals WHERE workspace_id = ? AND tab_id = ?`, workspace, tab); err != nil {
		return fmt.Errorf("withdraw tab data: %w", err)
	}
	return nil
}

// WorkspaceTabDataApprovals is each approved tab's data key.
func (s *Store) WorkspaceTabDataApprovals(ctx context.Context, workspace memory.WorkspaceID) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tab_id, data_key FROM workspace_tab_data_approvals WHERE workspace_id = ?`, workspace)
	if err != nil {
		return nil, fmt.Errorf("read tab data approvals: %w", err)
	}
	defer rows.Close()
	approvals := map[string]string{}
	for rows.Next() {
		var tab, key string
		if err := rows.Scan(&tab, &key); err != nil {
			return nil, err
		}
		approvals[tab] = key
	}
	return approvals, rows.Err()
}
