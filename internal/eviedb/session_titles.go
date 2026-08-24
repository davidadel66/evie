package eviedb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

type sessionTitleUpgradeHooks struct {
	afterFastMissingCheck func()
	afterTransactionOwned func()
	beforeBackfill        func()
}

// ensureSessionTitlesWithHooks avoids the global writer lock for current
// schemas. A legacy opener that observes a missing column serializes the
// additive upgrade, rechecks after acquiring ownership, and backfills exactly
// once. Hooks are private deterministic test coordination seams.
func ensureSessionTitlesWithHooks(ctx context.Context, db *sql.DB, hooks sessionTitleUpgradeHooks) error {
	hasTitle, err := sessionTableHasTitle(ctx, db)
	if err != nil {
		return err
	}
	if hasTitle {
		return nil
	}
	if hooks.afterFastMissingCheck != nil {
		hooks.afterFastMissingCheck()
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		if hooks.afterTransactionOwned != nil {
			hooks.afterTransactionOwned()
		}
		hasTitle, err := sessionTableHasTitle(ctx, conn)
		if err != nil {
			return err
		}
		if hasTitle {
			return nil
		}
		if _, err := conn.ExecContext(ctx, `ALTER TABLE sessions ADD COLUMN title TEXT`); err != nil {
			return fmt.Errorf("add sessions.title: %w", err)
		}
		if hooks.beforeBackfill != nil {
			hooks.beforeBackfill()
		}
		return backfillSessionTitles(ctx, conn)
	})
}

type rowsQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func sessionTableHasTitle(ctx context.Context, queryer rowsQueryer) (bool, error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_info(sessions)`)
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
		SELECT sessions.id, events.event_type, COALESCE(events.role, ''),
		       COALESCE(events.parent_id, ''), events.content
		FROM sessions
		JOIN events ON events.session_id = sessions.id
		WHERE sessions.title IS NULL
		ORDER BY sessions.id, events.sequence
	`)
	if err != nil {
		return fmt.Errorf("query title backfill evidence: %w", err)
	}

	titles := make(map[memory.SessionID]string)
	for rows.Next() {
		var id memory.SessionID
		var eventType memory.EventType
		var role memory.EventRole
		var parentID memory.EventID
		var content string
		if err := rows.Scan(&id, &eventType, &role, &parentID, &content); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan title backfill evidence: %w", err)
		}
		if _, found := titles[id]; found {
			continue
		}
		if title := memory.SessionTitleCandidate(eventType, role, parentID, content); title != "" {
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
