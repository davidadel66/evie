package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var (
	ErrTurnLeaseHeld            = errors.New("eviedb: turn lease is held")
	ErrTurnLeaseLost            = errors.New("eviedb: turn lease is not current")
	ErrTurnLeaseNotHeld         = errors.New("eviedb: turn lease is not held")
	ErrTurnLeaseSessionInactive = errors.New("eviedb: turn lease session is missing or inactive")

	errTurnLeaseWriterClosed       = errors.New("eviedb: turn lease writer is closed")
	errTurnLeaseTransactionControl = errors.New("eviedb: turn lease writer cannot control its transaction")
)

// TurnLeaseWriter is the restricted database surface available to a fenced
// write callback. Transaction commit and rollback remain owned by Store.
type TurnLeaseWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type turnLeaseWriter struct {
	mu     sync.RWMutex
	conn   *sql.Conn
	closed bool
}

func (w *turnLeaseWriter) ExecContext(
	ctx context.Context,
	query string,
	args ...any,
) (sql.Result, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		return nil, errTurnLeaseWriterClosed
	}
	if err := validateTurnLeaseWriteStatement(query); err != nil {
		return nil, err
	}
	return w.conn.ExecContext(ctx, query, args...)
}

func (w *turnLeaseWriter) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
}

func validateTurnLeaseWriteStatement(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return errors.New("turn lease write statement must not be empty")
	}
	if strings.Contains(query, ";") || strings.HasPrefix(query, "--") || strings.HasPrefix(query, "/*") {
		return errTurnLeaseTransactionControl
	}
	fields := strings.Fields(query)
	switch strings.ToUpper(fields[0]) {
	case "BEGIN", "COMMIT", "END", "ROLLBACK", "SAVEPOINT", "RELEASE":
		return errTurnLeaseTransactionControl
	default:
		return nil
	}
}

// Fixed-width UTC text preserves nanosecond ordering in SQLite comparisons.
const turnLeaseTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

func (s *Store) AcquireTurnLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	duration time.Duration,
) (memory.TurnLease, error) {
	var lease memory.TurnLease
	err := withImmediateTransaction(ctx, s.db, func(conn *sql.Conn) error {
		nowText, _, expiresText, err := turnLeaseWindow(sessionID, holderID, s.now(), duration)
		if err != nil {
			return err
		}

		row := conn.QueryRowContext(ctx, `
			INSERT INTO session_turn_leases (
				session_id, holder_id, fencing_token, lease_generation, expires_at
			)
			SELECT sessions.id, ?, 1, 1, ?
			FROM sessions
			WHERE sessions.id = ? AND sessions.status = ?
			ON CONFLICT(session_id) DO UPDATE SET
				holder_id = excluded.holder_id,
				fencing_token = session_turn_leases.fencing_token + 1,
				lease_generation = session_turn_leases.lease_generation + 1,
				expires_at = excluded.expires_at
			WHERE (session_turn_leases.holder_id IS NULL
			   OR session_turn_leases.expires_at <= ?)
			  AND EXISTS (
				SELECT 1 FROM sessions
				WHERE sessions.id = session_turn_leases.session_id
				  AND sessions.status = ?
			  )
			RETURNING session_id, holder_id, fencing_token, lease_generation, expires_at
		`, holderID, expiresText, sessionID, memory.SessionActive, nowText, memory.SessionActive)

		lease, err = scanTurnLease(row)
		if errors.Is(err, sql.ErrNoRows) {
			active, activeErr := turnLeaseSessionActive(ctx, conn, sessionID)
			if activeErr != nil {
				return fmt.Errorf("check turn lease session: %w", activeErr)
			}
			if !active {
				return fmt.Errorf("%w: session %q", ErrTurnLeaseSessionInactive, sessionID)
			}
			return fmt.Errorf("%w: session %q", ErrTurnLeaseHeld, sessionID)
		}
		if err != nil {
			return fmt.Errorf("acquire turn lease: %w", err)
		}
		return nil
	})
	if err != nil {
		return memory.TurnLease{}, err
	}
	return lease, nil
}

func (s *Store) HeartbeatTurnLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	duration time.Duration,
) (memory.TurnLease, error) {
	if err := validateFencingToken(token); err != nil {
		return memory.TurnLease{}, err
	}

	var lease memory.TurnLease
	err := withImmediateTransaction(ctx, s.db, func(conn *sql.Conn) error {
		nowText, _, expiresText, err := turnLeaseWindow(sessionID, holderID, s.now(), duration)
		if err != nil {
			return err
		}

		lease, err = scanTurnLease(conn.QueryRowContext(ctx, `
			UPDATE session_turn_leases
			SET expires_at = max(expires_at, ?)
			WHERE session_id = ?
			  AND holder_id = ?
			  AND fencing_token = ?
			  AND expires_at > ?
			  AND EXISTS (
				SELECT 1 FROM sessions
				WHERE sessions.id = session_turn_leases.session_id
				  AND sessions.status = ?
			  )
			RETURNING session_id, holder_id, fencing_token, lease_generation, expires_at
		`, expiresText, sessionID, holderID, token, nowText, memory.SessionActive))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: session %q", ErrTurnLeaseLost, sessionID)
		}
		if err != nil {
			return fmt.Errorf("heartbeat turn lease: %w", err)
		}
		return nil
	})
	if err != nil {
		return memory.TurnLease{}, err
	}
	return lease, nil
}

func (s *Store) ReleaseTurnLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
) error {
	return withImmediateTransaction(ctx, s.db, func(conn *sql.Conn) error {
		nowText, err := validateTurnLeaseAccess(sessionID, holderID, token, s.now())
		if err != nil {
			return err
		}

		var releasedSessionID string
		err = conn.QueryRowContext(ctx, `
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
	})
}

// WithTurnLeaseWrite runs write in a transaction that remains fenced to the
// supplied lease until commit. The callback is not run for a stale lease, and
// all callback writes are rolled back if the lease expires before commit. The
// callback must perform its database work through the supplied writer.
func (s *Store) WithTurnLeaseWrite(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	write func(TurnLeaseWriter) error,
) error {
	if write == nil {
		return errors.New("turn lease write must not be nil")
	}

	return withImmediateTransaction(ctx, s.db, func(conn *sql.Conn) error {
		nowText, err := validateTurnLeaseAccess(sessionID, holderID, token, s.now())
		if err != nil {
			return err
		}
		if err := fenceTurnLeaseWrite(ctx, conn, sessionID, holderID, token, nowText); err != nil {
			return err
		}

		writer := &turnLeaseWriter{conn: conn}
		writeErr := func() error {
			defer writer.close()
			return write(writer)
		}()
		if writeErr != nil {
			return fmt.Errorf("turn lease write: %w", writeErr)
		}

		// Recheck with fresh time so a callback that outlives the lease cannot
		// commit. BEGIN IMMEDIATE holds SQLite's write lock throughout.
		nowText, err = validateTurnLeaseAccess(sessionID, holderID, token, s.now())
		if err != nil {
			return err
		}
		return fenceTurnLeaseWrite(ctx, conn, sessionID, holderID, token, nowText)
	})
}

func fenceTurnLeaseWrite(
	ctx context.Context,
	conn *sql.Conn,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	nowText string,
) error {
	var authorized int
	err := conn.QueryRowContext(ctx, `
		UPDATE session_turn_leases
		SET fencing_token = fencing_token
		WHERE session_id = ?
		  AND holder_id = ?
		  AND fencing_token = ?
		  AND expires_at > ?
		  AND EXISTS (
			SELECT 1 FROM sessions
			WHERE sessions.id = session_turn_leases.session_id
			  AND sessions.status = ?
		  )
		RETURNING 1
	`, sessionID, holderID, token, nowText, memory.SessionActive).Scan(&authorized)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: session %q", ErrTurnLeaseLost, sessionID)
	}
	if err != nil {
		return fmt.Errorf("fence turn lease write: %w", err)
	}
	return nil
}

// GetTurnLease returns the persisted holder snapshot, including an expired
// holder that has not yet been released or replaced. Only a fenced store
// operation proves that a snapshot still owns the session.
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

func turnLeaseSessionActive(
	ctx context.Context,
	conn *sql.Conn,
	sessionID memory.SessionID,
) (bool, error) {
	var active bool
	err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sessions
			WHERE id = ? AND status = ?
		)
	`, sessionID, memory.SessionActive).Scan(&active)
	return active, err
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
