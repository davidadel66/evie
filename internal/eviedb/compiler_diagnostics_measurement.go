package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// RecordCompilerForeground belongs to the bound local History adapter. It is
// not a model tool or an HTTP input and cannot record another session's root.
func (h *SessionHistory) RecordCompilerForeground(ctx context.Context, m memory.CompilerForegroundMeasurement) error {
	if m.RootID == "" || m.StartedAtUnixMS <= 0 || safeDiagnosticOutcome(m.Outcome) != m.Outcome {
		return ErrReviewInvalidRequest
	}
	if (m.TerminalCommittedAtUnixMS == nil) != (m.TerminalCommitNanos == nil) || (m.ResponseFinalizedAtUnixMS == nil) != (m.ResponseFinalizationNanos == nil) {
		return ErrReviewInvalidRequest
	}
	for _, duration := range []*int64{m.TerminalCommitNanos, m.ResponseFinalizationNanos} {
		if duration != nil && *duration < 0 {
			return ErrReviewInvalidRequest
		}
	}
	return h.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var session memory.SessionID
		var typ, role string
		var parent sql.NullString
		if err := conn.QueryRowContext(ctx, `SELECT session_id,event_type,role,parent_id FROM events WHERE id=?`, m.RootID).Scan(&session, &typ, &role, &parent); err != nil {
			return err
		}
		if session != h.sessionID || typ != "user_message" || role != "user" || parent.Valid {
			return ErrOwnerReviewUnauthorized
		}
		_, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_diagnostic_foreground(root_id,session_id,started_at,terminal_at,terminal_ns,finalized_at,finalization_ns,outcome) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(root_id) DO NOTHING`, m.RootID, h.sessionID, m.StartedAtUnixMS, m.TerminalCommittedAtUnixMS, m.TerminalCommitNanos, m.ResponseFinalizedAtUnixMS, m.ResponseFinalizationNanos, m.Outcome)
		return err
	})
}

type compilerAttemptObservation struct {
	claim                           compilerClaim
	inference, validation, database *int64
	databaseStart                   time.Time
}

func measuredCompilerDuration(start time.Time) *int64 {
	n := time.Since(start).Nanoseconds()
	return &n
}
func (s *Store) recordCompilerAttempt(o compilerAttemptObservation, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	outcome := "completed"
	switch {
	case errors.Is(cause, ErrCompilerFence):
		outcome = "stale"
	case errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded):
		outcome = "cancelled"
	case cause != nil:
		outcome = "failed"
	}
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		// A later claim/cancel may have fenced this attempt before the caller saw
		// transport completion. Attribute its time to stale work, never that winner.
		var fence int64
		if err := conn.QueryRowContext(ctx, `SELECT fence FROM memory_compiler_jobs WHERE job_id=?`, o.claim.JobID).Scan(&fence); err != nil {
			return err
		}
		if fence != o.claim.Fence && outcome != "cancelled" {
			outcome = "stale"
		}
		_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_diagnostic_attempts SET inference_ns=?,validation_ns=?,database_ns=?,outcome=? WHERE job_id=? AND fence=? AND outcome='incomplete'`, o.inference, o.validation, o.database, outcome, o.claim.JobID, o.claim.Fence)
		return err
	})
}

// This clock is sampled after publication's transaction commit returns. SQL
// trigger time would precede COMMIT and cannot measure commit/lock latency.
func (s *Store) recordCompilerPublication(job string, fence int64, started time.Time) error {
	completed := time.Now()
	duration := completed.Sub(started).Nanoseconds()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_diagnostic_jobs SET published_at=?,publication_ns=? WHERE job_id=? AND EXISTS(SELECT 1 FROM memory_compiler_jobs j WHERE j.job_id=? AND j.fence=? AND j.state IN('completed_candidates','completed_empty'))`, completed.UnixMilli(), duration, job, job, fence)
		return err
	})
}
