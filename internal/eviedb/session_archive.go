package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrSessionDelegated = errors.New("eviedb: delegated session lifecycle is managed by its parent")

// ArchiveSession closes an idle owner conversation without changing its scope,
// composition or history. The existing inactive-session fence blocks new turns.
func (s *Store) ArchiveSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	return s.setSessionArchived(ctx, id, true)
}

func (s *Store) RestoreSession(ctx context.Context, id memory.SessionID) (memory.Session, error) {
	return s.setSessionArchived(ctx, id, false)
}

func (s *Store) setSessionArchived(ctx context.Context, id memory.SessionID, archived bool) (memory.Session, error) {
	var session memory.Session
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		session, err = scanSession(conn.QueryRowContext(ctx, `
			SELECT id, workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot,
			       parent_session_id, COALESCE(title, ''), status, created_at, updated_at
			FROM sessions WHERE id = ?
		`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSessionNotActive
		}
		if err != nil {
			return fmt.Errorf("read session lifecycle: %w", err)
		}
		if session.ParentSessionID != "" {
			return ErrSessionDelegated
		}
		now := s.now().UTC()
		var busy bool
		if err := conn.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM session_turn_leases
				WHERE session_id = ? AND holder_id IS NOT NULL AND expires_at > ?
			) OR EXISTS (
				SELECT 1 FROM subagent_executions
				WHERE parent_session_id = ? AND state IN ('admitted', 'running')
			)
		`, id, now.Format(turnLeaseTimeFormat), id).Scan(&busy); err != nil {
			return fmt.Errorf("check session lifecycle boundary: %w", err)
		}
		if busy {
			return ErrTurnLeaseHeld
		}
		status := memory.SessionActive
		if archived {
			status = memory.SessionClosed
		}
		if session.Status == status {
			return nil
		}
		if _, err := conn.ExecContext(ctx, `UPDATE sessions SET status = ?, updated_at = ? WHERE id = ?`,
			status, now.Format(time.RFC3339Nano), id); err != nil {
			return fmt.Errorf("update session lifecycle: %w", err)
		}
		session.Status, session.UpdatedAt = status, now
		return nil
	})
	if err != nil {
		return memory.Session{}, err
	}
	return session, nil
}
