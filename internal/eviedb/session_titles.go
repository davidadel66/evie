package eviedb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

// ensureSessionTitles serializes the additive upgrade and its backfill in one
// immediate transaction. A second process waits, rechecks the schema after it
// owns SQLite's write lock, and observes the completed first upgrade.
func ensureSessionTitles(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		hasTitle, err := sessionTableHasTitle(ctx, conn)
		if err != nil {
			return err
		}
		if !hasTitle {
			if _, err := conn.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN title TEXT`); err != nil {
				return fmt.Errorf("add sessions.title: %w", err)
			}
		}
		return backfillSessionTitles(ctx, conn)
	})
}

func sessionTableHasTitle(ctx context.Context, conn *sql.Conn) (bool, error) {
	rows, err := conn.QueryContext(ctx, `PRAGMA table_info(sessions)`)
	if err != nil {
		return false, fmt.Errorf("inspect sessions schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan sessions schema: %w", err)
		}
		if name == "title" {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read sessions schema: %w", err)
	}
	return false, nil
}

func backfillSessionTitles(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `
		SELECT sessions.id, events.content
		FROM sessions
		JOIN events ON events.session_id = sessions.id
		WHERE sessions.title IS NULL
		  AND events.event_type = ?
		  AND events.role = ?
		  AND events.parent_id IS NULL
		ORDER BY sessions.id, events.sequence
	`, memory.EventUserMessage, memory.RoleUser)
	if err != nil {
		return fmt.Errorf("query title backfill evidence: %w", err)
	}

	titles := make(map[memory.SessionID]string)
	for rows.Next() {
		var id memory.SessionID
		var content string
		if err := rows.Scan(&id, &content); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan title backfill evidence: %w", err)
		}
		if _, found := titles[id]; found {
			continue
		}
		if title := memory.NormalizeSessionTitle(content); title != "" {
			titles[id] = title
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close title backfill evidence: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read title backfill evidence: %w", err)
	}

	for id, title := range titles {
		if _, err := conn.ExecContext(ctx, `
			UPDATE sessions SET title = ? WHERE id = ? AND title IS NULL
		`, title, id); err != nil {
			return fmt.Errorf("backfill title for session %q: %w", id, err)
		}
	}
	return nil
}
