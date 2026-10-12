package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// MCP Connection records (workspace-mcp-connections.spec.md). Credentials
// never live here; the Keychain holds them.

var (
	// ErrNoWorkspaceAccessPin is a session created outside a Workspace, or
	// before chats pinned their access revision.
	ErrNoWorkspaceAccessPin            = errors.New("eviedb: the session has no Workspace Access pin")
	ErrWorkspaceAccessRevisionNotFound = errors.New("eviedb: no such Workspace Access revision")
)

func ensureMCPConnections(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS session_workspace_access_pins (
 session_id TEXT PRIMARY KEY NOT NULL REFERENCES sessions(id),
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 access_revision INTEGER NOT NULL CHECK(access_revision >= 0));
 CREATE TRIGGER IF NOT EXISTS session_workspace_access_pins_no_update BEFORE UPDATE ON session_workspace_access_pins BEGIN SELECT RAISE(ABORT,'Workspace Access pins are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS session_workspace_access_pins_no_delete BEFORE DELETE ON session_workspace_access_pins BEGIN SELECT RAISE(ABORT,'Workspace Access pins are immutable'); END;
 CREATE TABLE IF NOT EXISTS mcp_connections (
 id TEXT PRIMARY KEY NOT NULL CHECK(id GLOB 'mcp-*'),
 name TEXT NOT NULL UNIQUE,
 url TEXT NOT NULL CHECK(url GLOB 'https://*'),
 client_id TEXT NOT NULL DEFAULT '',
 status TEXT NOT NULL CHECK(status IN ('not_connected','connected','needs_sign_in','unreachable','keychain_unavailable')),
 status_detail TEXT NOT NULL DEFAULT '',
 oauth_json TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(oauth_json)),
 created_at TEXT NOT NULL,
 updated_at TEXT NOT NULL)`)
	return err
}

func (s *Store) CreateMCPConnection(ctx context.Context, c mcpconn.Connection) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO mcp_connections(id,name,url,client_id,status,status_detail,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.URL, c.ClientID, string(c.Status), c.StatusDetail, c.CreatedAt.UTC().Format(time.RFC3339Nano), c.UpdatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return mcpconn.ErrConnectionExists
	}
	return err
}

func (s *Store) MCPConnection(ctx context.Context, id string) (mcpconn.Connection, error) {
	c, err := scanMCPConnection(s.db.QueryRowContext(ctx, mcpConnectionSelect+` WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return mcpconn.Connection{}, mcpconn.ErrConnectionNotFound
	}
	return c, err
}

func (s *Store) MCPConnections(ctx context.Context) ([]mcpconn.Connection, error) {
	rows, err := s.db.QueryContext(ctx, mcpConnectionSelect+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []mcpconn.Connection{}
	for rows.Next() {
		c, err := scanMCPConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) SetMCPConnectionStatus(ctx context.Context, id string, status mcpconn.Status, detail string, oauth *mcpconn.OAuthMetadata, at time.Time) error {
	var result sql.Result
	var err error
	if oauth != nil {
		encoded, marshalErr := json.Marshal(oauth)
		if marshalErr != nil {
			return marshalErr
		}
		result, err = s.db.ExecContext(ctx, `UPDATE mcp_connections SET status=?, status_detail=?, oauth_json=?, updated_at=? WHERE id=?`,
			string(status), detail, string(encoded), at.UTC().Format(time.RFC3339Nano), id)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE mcp_connections SET status=?, status_detail=?, updated_at=? WHERE id=?`,
			string(status), detail, at.UTC().Format(time.RFC3339Nano), id)
	}
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return mcpconn.ErrConnectionNotFound
	}
	return nil
}

// SessionWorkspaceAccessPin is the Workspace and Workspace Access revision a
// chat pinned at creation (workspace-mcp-connections.spec.md).
func (s *Store) SessionWorkspaceAccessPin(ctx context.Context, session memory.SessionID) (memory.WorkspaceID, int64, error) {
	var ws string
	var revision int64
	err := s.db.QueryRowContext(ctx, `SELECT workspace_id, access_revision FROM session_workspace_access_pins WHERE session_id=?`, session).Scan(&ws, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, ErrNoWorkspaceAccessPin
	}
	return memory.WorkspaceID(ws), revision, err
}

// WorkspaceAccessAt reads one Workspace Access revision; revision 0 is the
// empty access every Workspace starts with.
func (s *Store) WorkspaceAccessAt(ctx context.Context, id memory.WorkspaceID, revision int64) (WorkspaceAccessRevision, error) {
	out := WorkspaceAccessRevision{WorkspaceID: id, Revision: revision, Access: workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{}}}
	if revision == 0 {
		return out, nil
	}
	var encoded, recorded string
	err := s.db.QueryRowContext(ctx, `SELECT access, recorded_at FROM workspace_access_revisions WHERE workspace_id=? AND revision=?`, id, revision).Scan(&encoded, &recorded)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrWorkspaceAccessRevisionNotFound
	}
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(encoded), &out.Access); err != nil {
		return out, fmt.Errorf("decode Workspace Access: %w", err)
	}
	out.RecordedAt, err = parseTime(recorded)
	return out, err
}

const mcpConnectionSelect = `SELECT id,name,url,client_id,status,status_detail,oauth_json,created_at,updated_at FROM mcp_connections`

func scanMCPConnection(row interface{ Scan(...any) error }) (mcpconn.Connection, error) {
	var c mcpconn.Connection
	var status, oauth, created, updated string
	if err := row.Scan(&c.ID, &c.Name, &c.URL, &c.ClientID, &status, &c.StatusDetail, &oauth, &created, &updated); err != nil {
		return mcpconn.Connection{}, err
	}
	c.Status = mcpconn.Status(status)
	if err := json.Unmarshal([]byte(oauth), &c.OAuth); err != nil {
		return mcpconn.Connection{}, err
	}
	var err error
	if c.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return mcpconn.Connection{}, err
	}
	if c.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return mcpconn.Connection{}, err
	}
	return c, nil
}
