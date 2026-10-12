package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Tool Policies (workspace-mcp-connections.spec.md story 3): per Workspace,
// the reviewed MCP tools David set to Allow, as append-only revisions. Ask
// is the default and is not stored. A policy names the exact reviewed
// selector `<name>@<hash>`, so a changed tool asks again.

var ErrMCPToolPoliciesChanged = errors.New("eviedb: MCP Tool Policies changed; refresh and try again")

func ensureMCPToolPolicies(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workspace_mcp_tool_policies (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 revision INTEGER NOT NULL CHECK(revision > 0),
 policies TEXT NOT NULL CHECK(json_valid(policies)),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, revision));
 CREATE TRIGGER IF NOT EXISTS workspace_mcp_tool_policies_no_update BEFORE UPDATE ON workspace_mcp_tool_policies BEGIN SELECT RAISE(ABORT,'MCP Tool Policy revisions are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workspace_mcp_tool_policies_no_delete BEFORE DELETE ON workspace_mcp_tool_policies BEGIN SELECT RAISE(ABORT,'MCP Tool Policy revisions are immutable'); END;`)
	return err
}

// MCPToolPolicies is a Workspace's latest policy revision: Connection ID →
// selector → allowed.
func (s *Store) MCPToolPolicies(ctx context.Context, ws memory.WorkspaceID) (int64, map[string]map[string]bool, error) {
	policies := map[string]map[string]bool{}
	var revision int64
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT revision, policies FROM workspace_mcp_tool_policies WHERE workspace_id=? ORDER BY revision DESC LIMIT 1`, ws).Scan(&revision, &encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, policies, nil
	}
	if err != nil {
		return 0, nil, err
	}
	if err := json.Unmarshal([]byte(encoded), &policies); err != nil {
		return 0, nil, fmt.Errorf("decode MCP Tool Policies: %w", err)
	}
	return revision, policies, nil
}

// SetMCPToolPolicy records a new revision with one selector allowed or back
// at Ask, when expected is still the latest revision and the Workspace is
// active.
func (s *Store) SetMCPToolPolicy(ctx context.Context, ws memory.WorkspaceID, expected int64, connectionID, selector string, allow bool) (int64, error) {
	var next int64
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var state string
		err := conn.QueryRowContext(ctx, `SELECT lifecycle_state FROM workspaces WHERE id=?`, ws).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkspaceNotFound
		}
		if err != nil {
			return err
		}
		if state != string(memory.WorkspaceActive) {
			return ErrWorkspaceInactive
		}
		policies := map[string]map[string]bool{}
		var current int64
		var encoded string
		err = conn.QueryRowContext(ctx, `SELECT revision, policies FROM workspace_mcp_tool_policies WHERE workspace_id=? ORDER BY revision DESC LIMIT 1`, ws).Scan(&current, &encoded)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return err
		default:
			if err := json.Unmarshal([]byte(encoded), &policies); err != nil {
				return fmt.Errorf("decode MCP Tool Policies: %w", err)
			}
		}
		if current != expected {
			return ErrMCPToolPoliciesChanged
		}
		if allow {
			if policies[connectionID] == nil {
				policies[connectionID] = map[string]bool{}
			}
			policies[connectionID][selector] = true
		} else {
			delete(policies[connectionID], selector)
			if len(policies[connectionID]) == 0 {
				delete(policies, connectionID)
			}
		}
		written, err := json.Marshal(policies)
		if err != nil {
			return err
		}
		next = current + 1
		_, err = conn.ExecContext(ctx, `INSERT INTO workspace_mcp_tool_policies(workspace_id, revision, policies, recorded_at) VALUES(?,?,?,?)`,
			ws, next, string(written), s.now().UTC().Format(time.RFC3339Nano))
		return err
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}
