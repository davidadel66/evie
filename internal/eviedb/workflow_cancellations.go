package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Cancellation and the audited rerun override (Stage 14 sub-stage 8 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md, ADR
// 0017, 0021, 0029):
//
//   - workflow_run_cancellations: the owner's cancellation of a run,
//     recorded once with the owner's surface, the reason and the state it
//     interrupted. Recording it moves the run to cancelling without the
//     run's lease, like any owner instruction a holder observes: every
//     holder write checks the run's state in its own transaction, so no
//     node, question, pause, failure or retry starts after it, while a node
//     in flight finishes and is recorded and an effect in flight completes
//     or reaches reconciliation. The holder of the lease then finishes the
//     cancellation (FinishWorkflowCancellation) once no attempt is in
//     flight and no Effect Intent lacks its receipt, withdrawing an open
//     question in the same transaction; the run is cancelled, its completed
//     nodes and effects stay recorded, and its Run Key is free. A trigger
//     refuses cancelling or cancelled without the owner's record, and a
//     cancelling run only ever finishes. Immutable.
//   - workflow_run_overrides: the owner's audited override that reruns a
//     succeeded logical execution (ADR 0021): the new run, the succeeded
//     run it reruns, the logical Run Key both share, the reason and the
//     surface, recorded in the transaction that records the run. The new
//     run's Run Key includes the override, so the database's one-run-per-
//     key rule still holds and its Effect Intents' idempotency keys differ
//     from the earlier run's. Immutable.

var (
	// ErrWorkflowRunCancelling refuses what a cancelling run no longer
	// does: start a node, ask a question, pause, fail or retry.
	ErrWorkflowRunCancelling = errors.New("the workflow run is being cancelled: no further node starts")
	// ErrWorkflowRerunRefused refuses a rerun override that does not name
	// a succeeded run of the same logical execution with a reason.
	ErrWorkflowRerunRefused = errors.New("the rerun override is refused")
)

// MaxWorkflowCancelReason bounds the owner's reason for a cancellation or
// a rerun override.
const MaxWorkflowCancelReason = 1000

// WorkflowRunCancellation is the owner's cancellation of a run.
type WorkflowRunCancellation struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason,omitempty"`
	// Actor is always the owner; Surface the CLI or the web.
	Actor   string `json:"actor"`
	Surface string `json:"surface"`
	// FromState is the state the cancellation interrupted.
	FromState   WorkflowRunState `json:"from_state"`
	RequestedAt time.Time        `json:"requested_at"`
}

// WorkflowRunCancellationRequest is the owner cancelling a run from a
// surface, with an optional reason.
type WorkflowRunCancellationRequest struct {
	RunID   string
	Reason  string
	Surface string
}

// WorkflowRunOverride is the owner's audited rerun of a succeeded logical
// execution.
type WorkflowRunOverride struct {
	RunID string `json:"run_id"`
	// RerunOf is the succeeded run it reruns, LogicalKey the Run Key of
	// the logical execution both share (the new run's own Run Key
	// includes the override).
	RerunOf    string    `json:"rerun_of"`
	LogicalKey string    `json:"logical_key"`
	Reason     string    `json:"reason"`
	Actor      string    `json:"actor"`
	Surface    string    `json:"surface"`
	RecordedAt time.Time `json:"recorded_at"`
}

func ensureWorkflowRunCancellationSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_run_cancellations (
 run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id),
 reason TEXT NOT NULL CHECK(length(reason) <= 1000),
 actor TEXT NOT NULL CHECK(actor = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 from_state TEXT NOT NULL CHECK(from_state IN ('queued','running','waiting_for_human','waiting_for_dependency','needs_reconciliation')),
 requested_at TEXT NOT NULL);
 CREATE TRIGGER IF NOT EXISTS workflow_run_cancellations_no_update BEFORE UPDATE ON workflow_run_cancellations
 BEGIN SELECT RAISE(ABORT,'workflow run cancellations are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_cancellations_no_delete BEFORE DELETE ON workflow_run_cancellations
 BEGIN SELECT RAISE(ABORT,'workflow run cancellations are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_cancelled_by_owner BEFORE UPDATE OF state ON workflow_runs
 WHEN NEW.state IN ('cancelling','cancelled') AND NOT EXISTS(SELECT 1 FROM workflow_run_cancellations c WHERE c.run_id=NEW.id)
 BEGIN SELECT RAISE(ABORT,'only the owner''s recorded cancellation cancels a workflow run'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_cancelling_finishes BEFORE UPDATE OF state ON workflow_runs
 WHEN OLD.state = 'cancelling' AND NEW.state NOT IN ('cancelling','cancelled','succeeded','failed')
 BEGIN SELECT RAISE(ABORT,'a cancelling workflow run only finishes'); END;
 CREATE TABLE IF NOT EXISTS workflow_run_overrides (
 run_id TEXT PRIMARY KEY REFERENCES workflow_runs(id),
 rerun_of TEXT NOT NULL REFERENCES workflow_runs(id),
 logical_key TEXT NOT NULL CHECK(length(logical_key) BETWEEN 1 AND 256),
 reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 actor TEXT NOT NULL CHECK(actor = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 recorded_at TEXT NOT NULL,
 CHECK(rerun_of <> run_id));
 CREATE INDEX IF NOT EXISTS workflow_run_overrides_by_rerun_of ON workflow_run_overrides(rerun_of);
 CREATE TRIGGER IF NOT EXISTS workflow_run_overrides_no_update BEFORE UPDATE ON workflow_run_overrides
 BEGIN SELECT RAISE(ABORT,'workflow rerun overrides are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_overrides_no_delete BEFORE DELETE ON workflow_run_overrides
 BEGIN SELECT RAISE(ABORT,'workflow rerun overrides are immutable'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow cancellation schema: %w", err)
	}
	return nil
}

// readWorkflowRunCancellation reads a run's cancellation; found is false
// when the owner never cancelled it.
func readWorkflowRunCancellation(ctx context.Context, q queryRower, runID string) (WorkflowRunCancellation, bool, error) {
	c := WorkflowRunCancellation{RunID: runID}
	var requested string
	err := q.QueryRowContext(ctx, `SELECT reason,actor,surface,from_state,requested_at FROM workflow_run_cancellations WHERE run_id=?`, runID).
		Scan(&c.Reason, &c.Actor, &c.Surface, &c.FromState, &requested)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return WorkflowRunCancellation{}, false, nil
	case err != nil:
		return WorkflowRunCancellation{}, false, err
	}
	c.RequestedAt, err = parseTime(requested)
	return c, err == nil, err
}

// readWorkflowRunOverride reads the override a run was started under;
// found is false for an ordinary run.
func readWorkflowRunOverride(ctx context.Context, q queryRower, runID string) (WorkflowRunOverride, bool, error) {
	o := WorkflowRunOverride{RunID: runID}
	var recorded string
	err := q.QueryRowContext(ctx, `SELECT rerun_of,logical_key,reason,actor,surface,recorded_at FROM workflow_run_overrides WHERE run_id=?`, runID).
		Scan(&o.RerunOf, &o.LogicalKey, &o.Reason, &o.Actor, &o.Surface, &recorded)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return WorkflowRunOverride{}, false, nil
	case err != nil:
		return WorkflowRunOverride{}, false, err
	}
	o.RecordedAt, err = parseTime(recorded)
	return o, err == nil, err
}

// logicalRunKey is the Run Key of the logical execution a run belongs to:
// its own, or for a rerun the one its override names.
func logicalRunKey(ctx context.Context, q queryRower, run WorkflowRun) (string, error) {
	// A Successor Run continues its predecessor's logical execution
	// (sub-stage 12), recorded with its migration.
	if m, found, err := readMigration(ctx, q, "successor_id", run.ID); err != nil || found {
		return m.LogicalKey, err
	}
	o, rerun, err := readWorkflowRunOverride(ctx, q, run.ID)
	if err != nil || !rerun {
		return run.RunKey, err
	}
	return o.LogicalKey, nil
}

// reasonSuffix renders an optional reason after a colon.
func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return ": " + reason
}

// cancellingError is a cancelling run's error: why it is cancelling, and
// that nothing more starts.
func cancellingError(c WorkflowRunCancellation) string {
	return fmt.Sprintf("cancelling at the owner's request via %s%s; no further node starts", c.Surface, reasonSuffix(c.Reason))
}

// cancellingUnknownError is a cancelling run's error while an effect's
// outcome is unknown: the cancellation finishes once its receipt is
// recorded, never by retrying it.
func cancellingUnknownError(unknown string) string {
	return "cancelling once its effect is reconciled: " + unknown
}

// cancelledError is a cancelled run's error.
func cancelledError(c WorkflowRunCancellation) string {
	return fmt.Sprintf("cancelled by the owner via %s%s", c.Surface, reasonSuffix(c.Reason))
}

// RequestWorkflowRunCancellation records the owner's cancellation of an
// unfinished run once and moves it to cancelling, without the run's lease:
// from then on no node, question, pause, failure or retry starts, and the
// holder of the run's lease (or the next runtime to claim it) finishes the
// cancellation. A run that needs reconciliation keeps its automatic
// reconciliation time; its cancellation finishes once the effect's receipt
// is recorded. A cancelling or cancelled run returns its record unchanged;
// a run that succeeded or failed is refused and left as it is.
func (s *Store) RequestWorkflowRunCancellation(ctx context.Context, req WorkflowRunCancellationRequest) (WorkflowRunCancellation, error) {
	if req.Surface != "cli" && req.Surface != "web" {
		return WorkflowRunCancellation{}, ErrWorkflowApprover
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > MaxWorkflowCancelReason {
		return WorkflowRunCancellation{}, fmt.Errorf("a cancellation's reason is at most %d bytes", MaxWorkflowCancelReason)
	}
	var out WorkflowRunCancellation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		run, err := readWorkflowRun(ctx, conn, req.RunID)
		if err != nil {
			return err
		}
		existing, found, err := readWorkflowRunCancellation(ctx, conn, run.ID)
		if err != nil {
			return err
		}
		if found && (run.State == WorkflowRunCancelling || run.State == WorkflowRunCancelled) {
			out = existing
			return nil
		}
		if run.State.Terminal() || found {
			return fmt.Errorf("%w: %s", ErrWorkflowRunFinished, run.State)
		}
		now := s.now().UTC()
		c := WorkflowRunCancellation{RunID: run.ID, Reason: reason, Actor: "owner", Surface: req.Surface, FromState: run.State, RequestedAt: now}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_cancellations(run_id,reason,actor,surface,from_state,requested_at) VALUES(?,?,?,?,?,?)`,
			c.RunID, c.Reason, c.Actor, c.Surface, string(c.FromState), formatTime(now)); err != nil {
			return fmt.Errorf("record workflow run cancellation: %w", err)
		}
		// A run whose effect is unresolved keeps its automatic
		// reconciliation time; any other is due for its holder at once.
		message, notBefore := cancellingError(c), any(nil)
		switch err := requireEffectsResolved(ctx, conn, run.ID); {
		case errors.Is(err, ErrWorkflowEffectUnresolved):
			if run.NotBefore != nil {
				notBefore = formatLeaseTime(*run.NotBefore)
			}
			if run.State == WorkflowRunNeedsReconciliation {
				message = cancellingUnknownError(run.Error)
			}
		case err != nil:
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state='cancelling',error=?,not_before=?,updated_at=? WHERE id=?`,
			clipText(message, maxEffectText), notBefore, formatTime(now), run.ID); err != nil {
			return fmt.Errorf("cancel workflow run: %w", err)
		}
		out, _, err = readWorkflowRunCancellation(ctx, conn, run.ID)
		return err
	})
	return out, err
}

// FinishWorkflowCancellation ends a cancelling run cancelled, under the
// holder's lease, once nothing is in flight: no attempt running and no
// Effect Intent without its receipt (an effect in flight completes or is
// reconciled first: ErrWorkflowEffectUnresolved). An open question is
// withdrawn unanswered in the same transaction. Completed nodes and
// effects stay recorded, and the run's Run Key is free.
func (s *Store) FinishWorkflowCancellation(ctx context.Context, lease WorkflowRunLease) (WorkflowRun, error) {
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if run.State != WorkflowRunCancelling {
			return fmt.Errorf("%w: the run is %s, not cancelling", ErrWorkflowRunConflict, run.State)
		}
		var inFlight int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND state='running'`, run.ID).Scan(&inFlight); err != nil {
			return err
		}
		if inFlight > 0 {
			return fmt.Errorf("%w: an attempt is in flight", ErrWorkflowRunConflict)
		}
		if err := requireEffectsResolved(ctx, conn, run.ID); err != nil {
			return err
		}
		c, found, err := readWorkflowRunCancellation(ctx, conn, run.ID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: run %s has no cancellation", ErrWorkflowRunConflict, run.ID)
		}
		for {
			seq, _, open, err := openQuestion(ctx, conn, run.ID)
			if err != nil {
				return err
			}
			if !open {
				break
			}
			if err := resolveInterruption(ctx, conn, lease, seq, resolutionRow{kind: WorkflowInterruptionWithdrawn, actor: "runtime",
				detail: fmt.Sprintf("the owner cancelled the run via %s%s", c.Surface, reasonSuffix(c.Reason))}, now); err != nil {
				return err
			}
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET state='cancelled',error=?,not_before=NULL,finished_at=?,updated_at=? WHERE id=?`,
			cancelledError(c), formatTime(now), formatTime(now), run.ID)
		return err
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, lease.RunID)
}

// WorkflowRunState reads a run's state alone: what a holder checks while a
// node's call is in flight, to stop it once the run is cancelled.
func (s *Store) WorkflowRunState(ctx context.Context, id string) (WorkflowRunState, error) {
	var state WorkflowRunState
	err := s.db.QueryRowContext(ctx, `SELECT state FROM workflow_runs WHERE id=?`, id).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", ErrWorkflowRunNotFound, id)
	}
	return state, err
}

// validRerun checks a start's override fields before its transaction.
func validRerun(req WorkflowRunStart) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrWorkflowRerunRefused, fmt.Sprintf(format, args...))
	}
	reason := strings.TrimSpace(req.Reason)
	switch {
	case req.RerunOf == "" && (req.LogicalKey != "" || reason != ""):
		return refuse("a logical Run Key and a reason are recorded only with the run a rerun overrides")
	case req.RerunOf == "":
		return nil
	case reason == "":
		return refuse("a rerun of run %s needs the owner's reason, which is recorded with it", req.RerunOf)
	case len(reason) > MaxWorkflowCancelReason:
		return refuse("a rerun's reason is at most %d bytes", MaxWorkflowCancelReason)
	case req.LogicalKey == "" || len(req.LogicalKey) > 256:
		return refuse("a rerun names the logical Run Key it reruns")
	case req.LogicalKey == req.RunKey:
		return refuse("a rerun's Run Key must include its override, not equal the logical Run Key %s", req.LogicalKey)
	}
	return nil
}

// checkRerun requires, inside the start's transaction, that the run a
// rerun overrides succeeded and belongs to the same Workspace, workflow
// and logical execution.
func checkRerun(ctx context.Context, conn *sql.Conn, req WorkflowRunStart) error {
	refuse := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrWorkflowRerunRefused, fmt.Sprintf(format, args...))
	}
	prior, err := readWorkflowRun(ctx, conn, req.RerunOf)
	if errors.Is(err, ErrWorkflowRunNotFound) {
		return refuse("run %s does not exist", req.RerunOf)
	}
	if err != nil {
		return err
	}
	if prior.WorkspaceID != req.WorkspaceID || prior.Workflow != req.Workflow {
		return refuse("run %s is a run of %s in Workspace %s, not of %s in Workspace %s", prior.ID, prior.Workflow, prior.WorkspaceID, req.Workflow, req.WorkspaceID)
	}
	if prior.State != WorkflowRunSucceeded {
		if prior.State == WorkflowRunFailed || prior.State == WorkflowRunCancelled {
			return refuse("run %s is %s, so its Run Key is free: start it again without an override", prior.ID, prior.State)
		}
		return refuse("run %s is %s: only a succeeded run's key needs an override", prior.ID, prior.State)
	}
	logical, err := logicalRunKey(ctx, conn, prior)
	if err != nil {
		return err
	}
	if logical != req.LogicalKey {
		return refuse("run %s ran the logical execution %s, and these inputs name %s", prior.ID, logical, req.LogicalKey)
	}
	return nil
}
