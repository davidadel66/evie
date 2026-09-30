package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrSessionModelChanged = errors.New("session model changed; refresh and try again")

type SessionModel struct {
	Model    string `json:"model"`
	Revision int64  `json:"revision"`
}

func (s *Store) SessionModel(ctx context.Context, id memory.SessionID) (SessionModel, error) {
	var setting SessionModel
	err := s.db.QueryRowContext(ctx, `SELECT model_id, revision FROM session_model_settings WHERE session_id = ?`, id).Scan(&setting.Model, &setting.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return setting, err
}

// SetSessionModel commits only for an idle owner conversation and the exact
// setting revision that the browser displayed.
func (s *Store) SetSessionModel(ctx context.Context, id memory.SessionID, revision int64, model string) (SessionModel, error) {
	if strings.TrimSpace(model) != model || model == "" || revision < 0 {
		return SessionModel{}, errors.New("invalid model selection")
	}
	var result SessionModel
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var status, parent string
		err := conn.QueryRowContext(ctx, `SELECT status, COALESCE(parent_session_id, '') FROM sessions WHERE id = ?`, id).Scan(&status, &parent)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && status != string(memory.SessionActive)) {
			return ErrSessionNotActive
		}
		if err != nil {
			return err
		}
		if parent != "" {
			return ErrSessionDelegated
		}
		var busy bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM session_turn_leases WHERE session_id = ? AND holder_id IS NOT NULL AND expires_at > ?)`, id, s.now().UTC().Format(turnLeaseTimeFormat)).Scan(&busy); err != nil {
			return err
		}
		if busy {
			return ErrTurnLeaseHeld
		}
		var current int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE((SELECT revision FROM session_model_settings WHERE session_id = ?), 0)`, id).Scan(&current); err != nil {
			return err
		}
		if current != revision {
			return ErrSessionModelChanged
		}
		result = SessionModel{Model: model, Revision: revision + 1}
		_, err = conn.ExecContext(ctx, `INSERT INTO session_model_settings(session_id, model_id, revision) VALUES (?, ?, ?) ON CONFLICT(session_id) DO UPDATE SET model_id = excluded.model_id, revision = excluded.revision`, id, model, result.Revision)
		return err
	})
	if err != nil {
		return SessionModel{}, err
	}
	return result, nil
}
