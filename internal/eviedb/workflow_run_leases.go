package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Workflow Run leases and recovery (Stage 14 sub-stage 4 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md, ADR
// 0017, 0025, 0029, 0033). Several Evie processes (the REPL, `evie serve`,
// a one-shot CLI start) share one database, and exactly one of them
// advances a run at a time:
//
//   - workflow_run_leases: one row per run that a runtime ever claimed. A
//     claim names its holder (one runtime instance) and a fencing token that
//     grows with every claim (the database refuses a smaller one); the
//     holder renews it by heartbeat before it expires and releases it when
//     it stops advancing the run. A run can be claimed when it was never
//     claimed, when its lease was released, or when its lease expired; a
//     claim of an expired lease is a takeover.
//   - workflow_run_events: append-only recovery history: takeovers, attempts
//     recorded interrupted, pauses and resumes, scheduled retries, and
//     releases at shutdown.
//
// Every attempt begin and finish, checkpoint and run failure, pause and
// resume is fenced: its transaction requires the holder's exact token,
// unreleased and unexpired, at its start and again before it commits, so a
// holder whose lease expired or was taken over writes nothing. A claim
// records every attempt an earlier lease left in flight as interrupted, in
// its own transaction, before the new holder can do anything else.

var (
	ErrWorkflowRunLeaseHeld = errors.New("another runtime holds the workflow run's lease")
	ErrWorkflowRunLeaseLost = errors.New("the workflow run's lease is not this holder's: it expired, was released or was taken over")
	ErrWorkflowRunNotDue    = errors.New("the workflow run waits for its retry or recheck time")
	// ErrWorkflowRunNotSchedulable is a run waiting for something the
	// scheduler does not resume (a human, a reconciliation, a cancellation).
	ErrWorkflowRunNotSchedulable = errors.New("the workflow run is waiting for something the scheduler does not resume")
)

// WorkflowRunNotDueError is a claim before the run's durable not-before
// time.
type WorkflowRunNotDueError struct{ NotBefore time.Time }

func (e *WorkflowRunNotDueError) Error() string {
	return fmt.Sprintf("%s: not before %s", ErrWorkflowRunNotDue, formatLeaseTime(e.NotBefore))
}
func (e *WorkflowRunNotDueError) Unwrap() error { return ErrWorkflowRunNotDue }

// WorkflowRunLease is one runtime's fenced right to advance one run.
type WorkflowRunLease struct {
	RunID       string     `json:"run_id"`
	Holder      string     `json:"holder"`
	Token       int64      `json:"token"`
	AcquiredAt  time.Time  `json:"acquired_at"`
	HeartbeatAt time.Time  `json:"heartbeat_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
}

// WorkflowRunClaim is a lease just acquired with what recovery found: the
// earlier lease it took over (nil unless that lease expired while held) and
// the attempts that lease left in flight, now recorded interrupted.
type WorkflowRunClaim struct {
	Lease       WorkflowRunLease
	TookOver    *WorkflowRunLease
	Interrupted []WorkflowNodeAttempt
}

// WorkflowRunEventKind is one kind of recovery event.
type WorkflowRunEventKind string

const (
	WorkflowEventLeaseTakenOver     WorkflowRunEventKind = "lease_taken_over"
	WorkflowEventAttemptInterrupted WorkflowRunEventKind = "attempt_interrupted"
	WorkflowEventPaused             WorkflowRunEventKind = "paused"
	WorkflowEventResumed            WorkflowRunEventKind = "resumed"
	WorkflowEventRetryScheduled     WorkflowRunEventKind = "retry_scheduled"
	WorkflowEventLeaseReleased      WorkflowRunEventKind = "lease_released"
)

// WorkflowRunEvent is one recovery event, recorded by the lease holder
// named on it.
type WorkflowRunEvent struct {
	RunID  string               `json:"run_id"`
	Seq    int64                `json:"seq"`
	Kind   WorkflowRunEventKind `json:"kind"`
	Holder string               `json:"holder"`
	Token  int64                `json:"token"`
	// Attempt is the attempt the event concerns, when it concerns one.
	Attempt int64     `json:"attempt,omitempty"`
	Detail  string    `json:"detail"`
	At      time.Time `json:"at"`
}

// Lease and retry times are stored as fixed-width UTC text so SQLite
// compares them in time order.
func formatLeaseTime(t time.Time) string { return t.UTC().Format(turnLeaseTimeFormat) }

func parseLeaseTime(text string) (time.Time, error) { return time.Parse(turnLeaseTimeFormat, text) }

func ensureWorkflowRunLeaseSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_run_leases (
 run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id),
 holder_id TEXT NOT NULL CHECK(length(holder_id) BETWEEN 1 AND 128),
 fencing_token INTEGER NOT NULL CHECK(fencing_token > 0),
 acquired_at TEXT NOT NULL,
 heartbeat_at TEXT NOT NULL,
 expires_at TEXT NOT NULL,
 released_at TEXT);
 CREATE TRIGGER IF NOT EXISTS workflow_run_leases_no_delete BEFORE DELETE ON workflow_run_leases
 BEGIN SELECT RAISE(ABORT,'workflow run leases are kept'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_leases_fenced BEFORE UPDATE OF run_id,fencing_token ON workflow_run_leases
 WHEN NEW.run_id <> OLD.run_id OR NEW.fencing_token < OLD.fencing_token
 BEGIN SELECT RAISE(ABORT,'a workflow run lease''s fencing token only grows'); END;
 CREATE TABLE IF NOT EXISTS workflow_run_events (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq > 0),
 kind TEXT NOT NULL CHECK(kind IN ('lease_taken_over','attempt_interrupted','paused','resumed','retry_scheduled','lease_released')),
 holder_id TEXT NOT NULL,
 fencing_token INTEGER NOT NULL CHECK(fencing_token > 0),
 attempt_seq INTEGER,
 detail TEXT NOT NULL,
 created_at TEXT NOT NULL,
 PRIMARY KEY(run_id, seq));
 CREATE TRIGGER IF NOT EXISTS workflow_run_events_no_update BEFORE UPDATE ON workflow_run_events
 BEGIN SELECT RAISE(ABORT,'workflow run events are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_events_no_delete BEFORE DELETE ON workflow_run_events
 BEGIN SELECT RAISE(ABORT,'workflow run events are immutable'); END;
 CREATE INDEX IF NOT EXISTS workflow_runs_unfinished ON workflow_runs(state) WHERE state IN ('queued','running','waiting_for_dependency');`)
	if err != nil {
		return fmt.Errorf("create workflow run lease schema: %w", err)
	}
	return nil
}

const leaseColumns = `run_id,holder_id,fencing_token,acquired_at,heartbeat_at,expires_at,released_at`

func scanWorkflowRunLease(scanner rowScanner) (WorkflowRunLease, error) {
	var l WorkflowRunLease
	var acquired, heartbeat, expires string
	var released sql.NullString
	if err := scanner.Scan(&l.RunID, &l.Holder, &l.Token, &acquired, &heartbeat, &expires, &released); err != nil {
		return l, err
	}
	var err error
	if l.AcquiredAt, err = parseLeaseTime(acquired); err != nil {
		return l, err
	}
	if l.HeartbeatAt, err = parseLeaseTime(heartbeat); err != nil {
		return l, err
	}
	if l.ExpiresAt, err = parseLeaseTime(expires); err != nil {
		return l, err
	}
	if released.Valid {
		t, err := parseLeaseTime(released.String)
		if err != nil {
			return l, err
		}
		l.ReleasedAt = &t
	}
	return l, nil
}

// readWorkflowRunLease reads a run's lease row; held is false when no
// runtime ever claimed the run.
func readWorkflowRunLease(ctx context.Context, q queryRower, runID string) (WorkflowRunLease, bool, error) {
	l, err := scanWorkflowRunLease(q.QueryRowContext(ctx, `SELECT `+leaseColumns+` FROM workflow_run_leases WHERE run_id=?`, runID))
	if errors.Is(err, sql.ErrNoRows) {
		return WorkflowRunLease{}, false, nil
	}
	return l, err == nil, err
}

func recordWorkflowRunEvent(ctx context.Context, conn *sql.Conn, e WorkflowRunEvent) error {
	var attempt any
	if e.Attempt > 0 {
		attempt = e.Attempt
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_events(run_id,seq,kind,holder_id,fencing_token,attempt_seq,detail,created_at)
 VALUES(?,(SELECT COALESCE(MAX(seq),0)+1 FROM workflow_run_events WHERE run_id=?),?,?,?,?,?,?)`,
		e.RunID, e.RunID, e.Kind, e.Holder, e.Token, attempt, e.Detail, formatLeaseTime(e.At))
	if err != nil {
		return fmt.Errorf("record workflow run event: %w", err)
	}
	return nil
}

func workflowRunEvents(ctx context.Context, conn *sql.Conn, runID string) ([]WorkflowRunEvent, error) {
	rows, err := conn.QueryContext(ctx, `SELECT run_id,seq,kind,holder_id,fencing_token,attempt_seq,detail,created_at FROM workflow_run_events WHERE run_id=? ORDER BY seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRunEvent{}
	for rows.Next() {
		var e WorkflowRunEvent
		var attempt sql.NullInt64
		var at string
		if err := rows.Scan(&e.RunID, &e.Seq, &e.Kind, &e.Holder, &e.Token, &attempt, &e.Detail, &at); err != nil {
			return nil, err
		}
		e.Attempt = attempt.Int64
		if e.At, err = parseLeaseTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func validLeaseRequest(holder string, d time.Duration) error {
	if strings.TrimSpace(holder) == "" || len(holder) > 128 {
		return errors.New("a workflow run lease needs a holder of 1 to 128 bytes")
	}
	if d <= 0 {
		return errors.New("a workflow run lease needs a positive duration")
	}
	return nil
}

// schedulable reports the states a runtime may claim: queued and running
// runs, runs paused waiting for a dependency, which are checked again at
// their recheck time, (sub-stage 6) runs that need reconciliation, which a
// runtime claims to reconcile them automatically or to record the owner's
// reconciliation, (sub-stage 7) runs waiting for a human, which only
// the owner's answering surface claims, to record the answer:
// DueWorkflowRuns never lists them, so no scheduler resumes them, and
// (sub-stage 8) cancelling runs, which a runtime claims to finish the
// owner's cancellation, after restarts too.
func schedulable(state WorkflowRunState) bool {
	return state == WorkflowRunQueued || state == WorkflowRunRunning || state == WorkflowRunWaitingForDependency || state == WorkflowRunNeedsReconciliation ||
		state == WorkflowRunWaitingForHuman || state == WorkflowRunCancelling
}

// AcquireWorkflowRunLease claims a run for holder for d. It refuses a
// finished run, a run waiting for something the scheduler does not resume,
// a run before its not-before time and a run whose lease another holder
// still holds. Claiming an expired lease takes it over; in the same
// transaction every attempt an earlier lease left in flight is recorded
// interrupted, so the new holder starts from a settled checkpoint.
func (s *Store) AcquireWorkflowRunLease(ctx context.Context, runID, holder string, d time.Duration) (WorkflowRunClaim, error) {
	if err := validLeaseRequest(holder, d); err != nil {
		return WorkflowRunClaim{}, err
	}
	var claim WorkflowRunClaim
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		run, err := readWorkflowRun(ctx, conn, runID)
		if err != nil {
			return err
		}
		switch {
		case run.State.Terminal():
			return fmt.Errorf("%w: %s", ErrWorkflowRunFinished, run.State)
		case !schedulable(run.State):
			return fmt.Errorf("%w: %s", ErrWorkflowRunNotSchedulable, run.State)
		case run.NotBefore != nil && run.NotBefore.After(now) && run.State != WorkflowRunNeedsReconciliation && run.State != WorkflowRunCancelling:
			// A run that needs reconciliation, or has an Effect Intent
			// without a receipt that this claim records unknown, can be
			// claimed at any time for the owner's reconciliation; its
			// not-before time only paces automatic reconciliation
			// (DueWorkflowRuns). A cancelling run is never held back by a
			// retry's backoff (sub-stage 8).
			switch err := requireEffectsResolved(ctx, conn, runID); {
			case err == nil:
				return &WorkflowRunNotDueError{NotBefore: *run.NotBefore}
			case !errors.Is(err, ErrWorkflowEffectUnresolved):
				return err
			}
		}
		prev, claimed, err := readWorkflowRunLease(ctx, conn, runID)
		if err != nil {
			return err
		}
		if claimed && prev.ReleasedAt == nil && prev.ExpiresAt.After(now) {
			return fmt.Errorf("%w: %s until %s", ErrWorkflowRunLeaseHeld, prev.Holder, formatLeaseTime(prev.ExpiresAt))
		}
		lease := WorkflowRunLease{RunID: runID, Holder: holder, Token: prev.Token + 1, AcquiredAt: now, HeartbeatAt: now, ExpiresAt: now.Add(d)}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_leases(`+leaseColumns+`) VALUES(?,?,?,?,?,?,NULL)
 ON CONFLICT(run_id) DO UPDATE SET holder_id=excluded.holder_id,fencing_token=excluded.fencing_token,acquired_at=excluded.acquired_at,
 heartbeat_at=excluded.heartbeat_at,expires_at=excluded.expires_at,released_at=NULL`,
			runID, holder, lease.Token, formatLeaseTime(now), formatLeaseTime(now), formatLeaseTime(lease.ExpiresAt)); err != nil {
			return fmt.Errorf("claim workflow run lease: %w", err)
		}
		claim.Lease = lease
		if claimed && prev.ReleasedAt == nil {
			took := prev
			claim.TookOver = &took
			if err := recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: runID, Kind: WorkflowEventLeaseTakenOver, Holder: holder, Token: lease.Token,
				Detail: fmt.Sprintf("holder %s's lease %d expired at %s without being released (last heartbeat %s); %s took the run over",
					prev.Holder, prev.Token, formatLeaseTime(prev.ExpiresAt), formatLeaseTime(prev.HeartbeatAt), holder), At: now}); err != nil {
				return err
			}
		}
		rows, err := conn.QueryContext(ctx, `SELECT `+attemptColumns+` FROM workflow_node_attempts WHERE run_id=? AND state='running' AND lease_token<? ORDER BY seq`, runID, lease.Token)
		if err != nil {
			return err
		}
		var inFlight []WorkflowNodeAttempt
		for rows.Next() {
			a, err := scanWorkflowAttempt(rows)
			if err != nil {
				rows.Close()
				return err
			}
			inFlight = append(inFlight, a)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, a := range inFlight {
			by := "an earlier Evie version"
			if a.LeaseHolder != "" {
				by = fmt.Sprintf("holder %s (lease %d)", a.LeaseHolder, a.LeaseToken)
			}
			a.State, a.Error, a.FinishedAt = WorkflowAttemptInterrupted, fmt.Sprintf("interrupted: %s stopped before the attempt's outcome was recorded", by), &now
			if _, err := conn.ExecContext(ctx, `UPDATE workflow_node_attempts SET state='interrupted',error=?,finished_at=? WHERE run_id=? AND seq=? AND state='running'`,
				a.Error, formatTime(now), runID, a.Seq); err != nil {
				return fmt.Errorf("record interrupted workflow node attempt: %w", err)
			}
			if err := recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: runID, Kind: WorkflowEventAttemptInterrupted, Holder: holder, Token: lease.Token, Attempt: a.Seq,
				Detail: fmt.Sprintf("steps.%s attempt %d from checkpoint %d: %s", a.Step, a.Attempt, a.InputCheckpoint, a.Error), At: now}); err != nil {
				return err
			}
			claim.Interrupted = append(claim.Interrupted, a)
		}
		if len(inFlight) > 0 {
			if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET updated_at=? WHERE id=?`, formatTime(now), runID); err != nil {
				return err
			}
		}
		// An Effect Intent whose attempt ended without its receipt (just
		// interrupted above, or interrupted by an Evie that predates the
		// effect protocol) leaves its outcome unknown: the run needs
		// reconciliation before anything else (sub-stage 6), never the
		// interruption's rerun.
		if err := markOrphanedEffectsUnknown(ctx, conn, lease, now); err != nil {
			return err
		}
		// Every unknown outcome is presented to the owner as an
		// interruption (sub-stage 7), including one an earlier Evie
		// recorded without it.
		return ensureEffectInterruptions(ctx, conn, lease, now)
	})
	if err != nil {
		return WorkflowRunClaim{}, err
	}
	return claim, nil
}

// fenceWorkflowRun requires lease to be the run's current lease: the same
// holder and token, not released and not expired at now.
func fenceWorkflowRun(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, now time.Time) error {
	if lease.RunID == "" || lease.Holder == "" || lease.Token <= 0 {
		return fmt.Errorf("%w: no lease", ErrWorkflowRunLeaseLost)
	}
	var current int
	err := conn.QueryRowContext(ctx, `SELECT 1 FROM workflow_run_leases WHERE run_id=? AND holder_id=? AND fencing_token=? AND released_at IS NULL AND expires_at>?`,
		lease.RunID, lease.Holder, lease.Token, formatLeaseTime(now)).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: run %s, holder %s, lease %d", ErrWorkflowRunLeaseLost, lease.RunID, lease.Holder, lease.Token)
	}
	return err
}

// withWorkflowRunLease runs write in one immediate transaction fenced by
// lease at its start and again, at a fresh time, before it commits: a write
// that outlives its lease does not commit. now is the store's time at the
// start.
func (s *Store) withWorkflowRunLease(ctx context.Context, lease WorkflowRunLease, write func(conn *sql.Conn, now time.Time) error) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		if err := fenceWorkflowRun(ctx, conn, lease, now); err != nil {
			return err
		}
		if err := write(conn, now); err != nil {
			return err
		}
		return fenceWorkflowRun(ctx, conn, lease, s.now().UTC())
	})
}

// HeartbeatWorkflowRunLease renews the holder's current lease for d from
// now; it never shortens it. An expired, released or taken-over lease is
// not renewed.
func (s *Store) HeartbeatWorkflowRunLease(ctx context.Context, lease WorkflowRunLease, d time.Duration) (WorkflowRunLease, error) {
	if err := validLeaseRequest(lease.Holder, d); err != nil {
		return WorkflowRunLease{}, err
	}
	var out WorkflowRunLease
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		var err error
		out, err = scanWorkflowRunLease(conn.QueryRowContext(ctx, `UPDATE workflow_run_leases SET heartbeat_at=?,expires_at=max(expires_at,?)
 WHERE run_id=? AND holder_id=? AND fencing_token=? AND released_at IS NULL AND expires_at>?
 RETURNING `+leaseColumns, formatLeaseTime(now), formatLeaseTime(now.Add(d)), lease.RunID, lease.Holder, lease.Token, formatLeaseTime(now)))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: run %s, holder %s, lease %d", ErrWorkflowRunLeaseLost, lease.RunID, lease.Holder, lease.Token)
		}
		return err
	})
	return out, err
}

// ReleaseWorkflowRunLease gives the run up at once, so any runtime can claim
// it without waiting for expiry. A lease another holder took over is not
// released (ErrWorkflowRunLeaseLost). A reason, given when the holder stops
// while the run still has work, is recorded as a lease_released event.
func (s *Store) ReleaseWorkflowRunLease(ctx context.Context, lease WorkflowRunLease, reason string) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		var released int
		err := conn.QueryRowContext(ctx, `UPDATE workflow_run_leases SET released_at=? WHERE run_id=? AND holder_id=? AND fencing_token=? AND released_at IS NULL RETURNING 1`,
			formatLeaseTime(now), lease.RunID, lease.Holder, lease.Token).Scan(&released)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: run %s, holder %s, lease %d", ErrWorkflowRunLeaseLost, lease.RunID, lease.Holder, lease.Token)
		}
		if err != nil || reason == "" {
			return err
		}
		run, err := readWorkflowRun(ctx, conn, lease.RunID)
		if err != nil || run.State.Terminal() {
			// A finished run has no work left to hand over.
			return err
		}
		return recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: lease.RunID, Kind: WorkflowEventLeaseReleased, Holder: lease.Holder, Token: lease.Token, Detail: reason, At: now})
	})
}

// PauseWorkflowRun pauses a run visibly under the holder's lease: it waits
// for a dependency with reason as its error, and is due again for a check
// after recheck. A pause is recorded once per reason. No attempt may be in
// flight.
func (s *Store) PauseWorkflowRun(ctx context.Context, lease WorkflowRunLease, reason string, recheck time.Duration) (WorkflowRun, error) {
	if strings.TrimSpace(reason) == "" || recheck <= 0 {
		return WorkflowRun{}, errors.New("a paused run needs its reason and a recheck time")
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		// A cancelling run is never paused: a pause would resume by itself
		// and hide the owner's cancellation (sub-stage 8).
		if run.State == WorkflowRunCancelling {
			return ErrWorkflowRunCancelling
		}
		var inFlight int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND state='running'`, lease.RunID).Scan(&inFlight); err != nil {
			return err
		}
		if inFlight > 0 {
			return fmt.Errorf("%w: an attempt is in flight", ErrWorkflowRunConflict)
		}
		// A run whose effect outcome is unknown needs reconciliation; a
		// pause, which resumes by itself, would hide it (ADR 0029).
		if err := requireEffectsResolved(ctx, conn, lease.RunID); err != nil {
			return err
		}
		if run.State == WorkflowRunNeedsReconciliation {
			return fmt.Errorf("%w: the run needs reconciliation", ErrWorkflowRunConflict)
		}
		// A run waiting for the owner's answer stays waiting: a pause would
		// resume by itself (ADR 0029, sub-stage 7).
		if err := requireNoOpenQuestion(ctx, conn, lease.RunID); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state='waiting_for_dependency',error=?,not_before=?,updated_at=? WHERE id=?`,
			reason, formatLeaseTime(now.Add(recheck)), formatTime(now), lease.RunID); err != nil {
			return fmt.Errorf("pause workflow run: %w", err)
		}
		if run.State == WorkflowRunWaitingForDependency && run.Error == reason {
			return nil
		}
		return recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: lease.RunID, Kind: WorkflowEventPaused, Holder: lease.Holder, Token: lease.Token, Detail: reason, At: now})
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, lease.RunID)
}

// authorizeWorkflowRun is run admission for a run that continues (ADR
// 0033: revocations apply despite the pin). The definition must still
// admit runs (authorizeWorkflowActivation: its latest version approved, not
// deactivated, not blocked), and the version the run is pinned to must be
// neither deactivated nor blocked. A run of a superseded version continues
// under its own pins.
func authorizeWorkflowRun(ctx context.Context, conn *sql.Conn, run WorkflowRun) error {
	if _, err := authorizeWorkflowActivation(ctx, conn, run.WorkspaceID, run.Workflow); err != nil {
		return err
	}
	own, err := activationAt(ctx, conn, run.WorkspaceID, run.Workflow, run.Version)
	if err != nil {
		return err
	}
	if own.Deactivated != nil {
		return fmt.Errorf("%w: version %d was deactivated", ErrWorkflowNotActive, own.Version)
	}
	blocks, err := activationBlocks(ctx, conn, own)
	if err != nil {
		return err
	}
	if len(blocks) > 0 {
		return &WorkflowBlockedError{Reasons: blocks}
	}
	return nil
}

// AdmitWorkflowRun applies run admission to a run its holder is about to
// continue, changing nothing.
func (s *Store) AdmitWorkflowRun(ctx context.Context, lease WorkflowRunLease) error {
	return s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, _ time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		return authorizeWorkflowRun(ctx, conn, run)
	})
}

// ResumeWorkflowRun applies run admission to a run its holder is about to
// continue and, in the same transaction, resumes a paused run: it is
// running again from its latest checkpoint, its reason and recheck time
// are cleared, and the resume is recorded. A run that is not paused is
// unchanged.
func (s *Store) ResumeWorkflowRun(ctx context.Context, lease WorkflowRunLease) (WorkflowRun, error) {
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if err := authorizeWorkflowRun(ctx, conn, run); err != nil {
			return err
		}
		if run.State != WorkflowRunWaitingForDependency {
			return nil
		}
		// Automatic resume stays off while an effect's outcome is unknown
		// (ADR 0029).
		if err := requireEffectsResolved(ctx, conn, lease.RunID); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state='running',error='',not_before=NULL,updated_at=? WHERE id=?`, formatTime(now), lease.RunID); err != nil {
			return fmt.Errorf("resume workflow run: %w", err)
		}
		return recordWorkflowRunEvent(ctx, conn, WorkflowRunEvent{RunID: lease.RunID, Kind: WorkflowEventResumed, Holder: lease.Holder, Token: lease.Token,
			Detail: fmt.Sprintf("paused because %s; it continues from checkpoint %d", run.Error, run.Checkpoint), At: now})
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, lease.RunID)
}

// DueWorkflowRuns lists, oldest first and at most limit, the runs a
// scheduler should claim now: queued, running or paused runs past their
// not-before time, and runs needing reconciliation whose automatic
// reconciliation is due (sub-stage 6; one waiting for the owner has no
// not-before time and is never due), and (sub-stage 8) cancelling runs,
// at once unless an effect's outcome is unknown, then when its automatic
// reconciliation is due, whose lease was never taken, was released or
// expired. Every scheduler pass is therefore also the recovery scan: a run
// whose holder died is due once its lease expires.
func (s *Store) DueWorkflowRuns(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		return nil, nil
	}
	now := formatLeaseTime(s.now())
	rows, err := s.db.QueryContext(ctx, `SELECT r.id FROM workflow_runs r LEFT JOIN workflow_run_leases l ON l.run_id=r.id
 WHERE (r.state IN ('queued','running','waiting_for_dependency') AND (r.not_before IS NULL OR r.not_before<=?)
 OR r.state='needs_reconciliation' AND r.not_before IS NOT NULL AND r.not_before<=?
 OR r.state='cancelling' AND (r.not_before IS NOT NULL AND r.not_before<=? OR NOT EXISTS(SELECT 1 FROM workflow_effect_unknowns u WHERE u.run_id=r.id
 AND NOT EXISTS(SELECT 1 FROM workflow_effect_receipts x WHERE x.run_id=u.run_id AND x.intent_seq=u.intent_seq))))
 AND (l.run_id IS NULL OR l.released_at IS NOT NULL OR l.expires_at<=?)
 ORDER BY r.rowid LIMIT ?`, now, now, now, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
