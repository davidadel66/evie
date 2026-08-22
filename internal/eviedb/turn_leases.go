package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var (
	ErrTurnLeaseHeld    = errors.New("eviedb: turn lease is held")
	ErrTurnLeaseLost    = errors.New("eviedb: turn lease is not current")
	ErrTurnLeaseNotHeld = errors.New("eviedb: turn lease is not held")
)

// Fixed-width UTC text preserves nanosecond ordering in SQLite comparisons.
const turnLeaseTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

func (s *Store) AcquireTurnLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	duration time.Duration,
) (memory.TurnLease, error) {
	now := s.now()
	nowText, expiresAt, expiresText, err := turnLeaseWindow(sessionID, holderID, now, duration)
	if err != nil {
		return memory.TurnLease{}, err
	}

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO session_turn_leases (
			session_id, holder_id, fencing_token, lease_generation, expires_at
		) VALUES (?, ?, 1, 1, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			holder_id = excluded.holder_id,
			fencing_token = session_turn_leases.fencing_token + 1,
			lease_generation = session_turn_leases.lease_generation + 1,
			expires_at = excluded.expires_at
		WHERE session_turn_leases.holder_id IS NULL
		   OR session_turn_leases.expires_at <= ?
		RETURNING session_id, holder_id, fencing_token, lease_generation, expires_at
	`, sessionID, holderID, expiresText, nowText)

	lease, err := scanTurnLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.TurnLease{}, fmt.Errorf("%w: session %q", ErrTurnLeaseHeld, sessionID)
	}
	if err != nil {
		return memory.TurnLease{}, fmt.Errorf("acquire turn lease: %w", err)
	}
	lease.ExpiresAt = expiresAt
	return lease, nil
}

func (s *Store) HeartbeatTurnLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	duration time.Duration,
) (memory.TurnLease, error) {
	now := s.now()
	nowText, _, expiresText, err := turnLeaseWindow(sessionID, holderID, now, duration)
	if err != nil {
		return memory.TurnLease{}, err
	}
	if err := validateFencingToken(token); err != nil {
		return memory.TurnLease{}, err
	}

	row := s.db.QueryRowContext(ctx, `
		UPDATE session_turn_leases
		SET expires_at = max(expires_at, ?)
		WHERE session_id = ?
		  AND holder_id = ?
		  AND fencing_token = ?
		  AND expires_at > ?
		RETURNING session_id, holder_id, fencing_token, lease_generation, expires_at
	`, expiresText, sessionID, holderID, token, nowText)

	lease, err := scanTurnLease(row)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.TurnLease{}, fmt.Errorf("%w: session %q", ErrTurnLeaseLost, sessionID)
	}
	if err != nil {
		return memory.TurnLease{}, fmt.Errorf("heartbeat turn lease: %w", err)
	}
	return lease, nil
}

func (s *Store) ReleaseTurnLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
) error {
	now := s.now()
	nowText, err := validateTurnLeaseAccess(sessionID, holderID, token, now)
	if err != nil {
		return err
	}

	var releasedSessionID string
	err = s.db.QueryRowContext(ctx, `
		UPDATE session_turn_leases
		SET holder_id = NULL, expires_at = NULL
		WHERE session_id = ?
		  AND holder_id = ?
		  AND fencing_token = ?
		  AND expires_at > ?
		RETURNING session_id
	`, sessionID, holderID, token, nowText).Scan(&releasedSessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: session %q", ErrTurnLeaseLost, sessionID)
	}
	if err != nil {
		return fmt.Errorf("release turn lease: %w", err)
	}
	return nil
}

// WithTurnLeaseWrite runs write in a transaction that remains fenced to the
// supplied lease until commit. The callback is not run for a stale lease, and
// all callback writes are rolled back if the lease expires before commit. The
// callback must perform its database work through the supplied transaction.
func (s *Store) WithTurnLeaseWrite(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	write func(*sql.Tx) error,
) error {
	if write == nil {
		return errors.New("turn lease write must not be nil")
	}

	nowText, err := validateTurnLeaseAccess(sessionID, holderID, token, s.now())
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin turn lease write: %w", err)
	}
	defer tx.Rollback()

	if err := fenceTurnLeaseWrite(ctx, tx, sessionID, holderID, token, nowText); err != nil {
		return err
	}
	if err := write(tx); err != nil {
		return fmt.Errorf("turn lease write: %w", err)
	}

	// Recheck with fresh time so a callback that outlives the lease cannot
	// commit. The first fencing UPDATE holds SQLite's write lock throughout.
	nowText, err = validateTurnLeaseAccess(sessionID, holderID, token, s.now())
	if err != nil {
		return err
	}
	if err := fenceTurnLeaseWrite(ctx, tx, sessionID, holderID, token, nowText); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit turn lease write: %w", err)
	}
	return nil
}

func fenceTurnLeaseWrite(
	ctx context.Context,
	tx *sql.Tx,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	nowText string,
) error {
	var authorized int
	err := tx.QueryRowContext(ctx, `
		UPDATE session_turn_leases
		SET fencing_token = fencing_token
		WHERE session_id = ?
		  AND holder_id = ?
		  AND fencing_token = ?
		  AND expires_at > ?
		RETURNING 1
	`, sessionID, holderID, token, nowText).Scan(&authorized)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: session %q", ErrTurnLeaseLost, sessionID)
	}
	if err != nil {
		return fmt.Errorf("fence turn lease write: %w", err)
	}
	return nil
}

func (s *Store) GetTurnLease(ctx context.Context, sessionID memory.SessionID) (memory.TurnLease, error) {
	if strings.TrimSpace(string(sessionID)) == "" {
		return memory.TurnLease{}, errors.New("session ID must not be empty")
	}

	lease, err := scanTurnLease(s.db.QueryRowContext(ctx, `
		SELECT session_id, holder_id, fencing_token, lease_generation, expires_at
		FROM session_turn_leases
		WHERE session_id = ? AND holder_id IS NOT NULL
	`, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return memory.TurnLease{}, fmt.Errorf("%w: session %q", ErrTurnLeaseNotHeld, sessionID)
	}
	if err != nil {
		return memory.TurnLease{}, fmt.Errorf("get turn lease: %w", err)
	}
	return lease, nil
}

func scanTurnLease(scanner rowScanner) (memory.TurnLease, error) {
	var (
		sessionID, holderID, expiresText string
		fencingToken                     int64
		generation                       int64
	)
	if err := scanner.Scan(&sessionID, &holderID, &fencingToken, &generation, &expiresText); err != nil {
		return memory.TurnLease{}, err
	}
	expiresAt, err := time.Parse(turnLeaseTimeFormat, expiresText)
	if err != nil {
		return memory.TurnLease{}, fmt.Errorf("parse turn lease expiry: %w", err)
	}
	return memory.TurnLease{
		SessionID:    memory.SessionID(sessionID),
		HolderID:     memory.LeaseHolderID(holderID),
		FencingToken: memory.FencingToken(fencingToken),
		Generation:   memory.LeaseGeneration(generation),
		ExpiresAt:    expiresAt,
	}, nil
}

func turnLeaseWindow(
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	now time.Time,
	duration time.Duration,
) (string, time.Time, string, error) {
	if strings.TrimSpace(string(sessionID)) == "" {
		return "", time.Time{}, "", errors.New("session ID must not be empty")
	}
	if strings.TrimSpace(string(holderID)) == "" {
		return "", time.Time{}, "", errors.New("lease holder ID must not be empty")
	}
	if now.IsZero() {
		return "", time.Time{}, "", errors.New("lease time must not be zero")
	}
	if duration <= 0 {
		return "", time.Time{}, "", errors.New("lease duration must be positive")
	}

	now = now.UTC()
	expiresAt := now.Add(duration)
	if !expiresAt.After(now) {
		return "", time.Time{}, "", errors.New("lease expiry must be after current time")
	}
	return now.Format(turnLeaseTimeFormat), expiresAt, expiresAt.Format(turnLeaseTimeFormat), nil
}

func validateTurnLeaseAccess(
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	now time.Time,
) (string, error) {
	if strings.TrimSpace(string(sessionID)) == "" {
		return "", errors.New("session ID must not be empty")
	}
	if strings.TrimSpace(string(holderID)) == "" {
		return "", errors.New("lease holder ID must not be empty")
	}
	if err := validateFencingToken(token); err != nil {
		return "", err
	}
	if now.IsZero() {
		return "", errors.New("lease time must not be zero")
	}
	return now.UTC().Format(turnLeaseTimeFormat), nil
}

func validateFencingToken(token memory.FencingToken) error {
	if token <= 0 {
		return errors.New("fencing token must be positive")
	}
	return nil
}
