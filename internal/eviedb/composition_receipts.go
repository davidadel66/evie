package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
)

var (
	ErrCompositionReceiptExists   = errors.New("eviedb: Composition Receipt already exists")
	ErrCompositionReceiptNotFound = errors.New("eviedb: Composition Receipt not found")
)

type compositionReceiptExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// SaveCompositionReceipt appends the one immutable composition identity for a
// session. Validation occurs before SQLite so rejected secret-shaped content
// is never written even transiently.
func (s *Store) SaveCompositionReceipt(
	ctx context.Context,
	sessionID memory.SessionID,
	receipt composition.Receipt,
) error {
	if sessionID == "" {
		return errors.New("session ID must not be empty")
	}
	encoded, err := composition.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("validate Composition Receipt: %w", err)
	}
	err = insertCompositionReceipt(ctx, s.db, sessionID, encoded, s.now())
	if err == nil {
		return nil
	}
	var exists bool
	if lookupErr := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM session_composition_receipts WHERE session_id = ?)
	`, sessionID).Scan(&exists); lookupErr == nil && exists {
		return fmt.Errorf("%w for session %q", ErrCompositionReceiptExists, sessionID)
	}
	return fmt.Errorf("save Composition Receipt for session %q: %w", sessionID, err)
}

func insertCompositionReceipt(
	ctx context.Context,
	executor compositionReceiptExecutor,
	sessionID memory.SessionID,
	encoded []byte,
	recordedAt time.Time,
) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO session_composition_receipts (session_id, receipt_json, recorded_at)
		VALUES (?, ?, ?)
	`, sessionID, string(encoded), recordedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetCompositionReceipt(
	ctx context.Context,
	sessionID memory.SessionID,
) (composition.Receipt, error) {
	if sessionID == "" {
		return composition.Receipt{}, errors.New("session ID must not be empty")
	}
	var encoded string
	err := s.db.QueryRowContext(ctx, `
		SELECT receipt_json FROM session_composition_receipts WHERE session_id = ?
	`, sessionID).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return composition.Receipt{}, fmt.Errorf("%w for session %q", ErrCompositionReceiptNotFound, sessionID)
	}
	if err != nil {
		return composition.Receipt{}, fmt.Errorf("read Composition Receipt for session %q: %w", sessionID, err)
	}
	receipt, err := composition.Unmarshal([]byte(encoded))
	if err != nil {
		return composition.Receipt{}, fmt.Errorf("decode Composition Receipt for session %q: %w", sessionID, err)
	}
	return receipt, nil
}
