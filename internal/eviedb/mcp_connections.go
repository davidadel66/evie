package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/mcpconn"
)

// MCP Connection records (workspace-mcp-connections.spec.md). Credentials
// never live here; the Keychain holds them.

func ensureMCPConnections(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS mcp_connections (
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
