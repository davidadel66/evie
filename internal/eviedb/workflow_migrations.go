package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Workflow Run migration (Stage 14 sub-stage 12 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md: "An
// incompatible unfinished run pauses. Explicit migration creates a linked
// Successor Run under an approved new definition, carrying only validated
// state and durable effect evidence. The predecessor remains immutable";
// ADR 0025, 0030). Nothing migrates a run but the owner's request:
//
//   - workflow_run_migrations: one row per migration, recorded in the
//     transaction that records the Successor Run and supersedes its
//     predecessor: both runs, the versions, the predecessor's checkpoint
//     and the step the successor resumes at, what was carried and dropped,
//     the predecessor's Effect Intents with their receipts (referenced,
//     never performed again), what the predecessor already used of its
//     limits (time, less the time it waited for answers and at the daily
//     cap; counted attempts; tokens and cost), the schedule slot it
//     decided, and the owner's reason. Immutable.
//   - The predecessor ends in the terminal state superseded, which frees
//     its Run Key for the successor in the same transaction; its pins,
//     attempts, checkpoints, effects and answers are never rewritten. A
//     trigger refuses superseded without the migration record.

var (
	// ErrWorkflowMigrationRefused refuses a migration the store's
	// transaction cannot record: the run moved on, is held, or holds
	// something a successor cannot take over.
	ErrWorkflowMigrationRefused = errors.New("workflow run migration refused")
)

// workflowRunSuperseded is the terminal state of a migrated predecessor.
const WorkflowRunSuperseded WorkflowRunState = "superseded"

// WorkflowCarried is what a run's predecessors already used of the limits
// it continues: time counted against limits.max_duration (less the time
// they waited for the owner's answers and at the daily cap), attempts
// counted against limits.max_attempts, and AI and worker tokens and cost.
type WorkflowCarried struct {
	ElapsedMS int64 `json:"elapsed_ms"`
	Attempts  int   `json:"attempts"`
	Tokens    int64 `json:"tokens"`
	MicroUSD  int64 `json:"micro_usd"`
}

// Elapsed is the carried time.
func (c WorkflowCarried) Elapsed() time.Duration {
	return time.Duration(c.ElapsedMS) * time.Millisecond
}

// WorkflowInheritedEffect is one Effect Intent of the predecessor with how
// it ended: the successor references it and never performs it again.
type WorkflowInheritedEffect struct {
	RunID          string               `json:"run_id"`
	Seq            int64                `json:"seq"`
	Step           string               `json:"step"`
	Checkpoint     int64                `json:"checkpoint"`
	IdempotencyKey string               `json:"idempotency_key"`
	Outcome        WorkflowEffectState  `json:"outcome"`
	Source         WorkflowEffectSource `json:"source,omitempty"`
}

// WorkflowRunMigration is one recorded migration.
type WorkflowRunMigration struct {
	SuccessorID   string `json:"successor_id"`
	PredecessorID string `json:"predecessor_id"`
	FromVersion   int64  `json:"from_version"`
	ToVersion     int64  `json:"to_version"`
	// Checkpoint is the predecessor's checkpoint the successor continues
	// from, and Resume the step it resumes at.
	Checkpoint int64  `json:"checkpoint"`
	Resume     string `json:"resume"`
	// LogicalKey is the logical Run Key both runs continue (ADR 0021).
	LogicalKey string `json:"logical_key"`
	// Plan is what was carried and dropped (workflow.ResumePlan).
	Plan    json.RawMessage           `json:"plan"`
	Effects []WorkflowInheritedEffect `json:"effects"`
	Carried WorkflowCarried           `json:"carried"`
	// SlotTimezone and SlotKey are the schedule slot the predecessor (or
	// its own predecessor) decided, which the successor keeps.
	SlotTimezone string    `json:"slot_timezone,omitempty"`
	SlotKey      string    `json:"slot_key,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Actor        string    `json:"actor"`
	Surface      string    `json:"surface"`
	RecordedAt   time.Time `json:"recorded_at"`
}

// workflowRunStatesSuperseded is the run state check since sub-stage 12;
// workflowRunStatesBefore the one before it.
const (
	workflowRunStatesBefore     = "state TEXT NOT NULL CHECK(state IN ('queued','running','waiting_for_human','waiting_for_dependency','needs_reconciliation','cancelling','cancelled','failed','succeeded'))"
	workflowRunStatesSuperseded = "state TEXT NOT NULL CHECK(state IN ('queued','running','waiting_for_human','waiting_for_dependency','needs_reconciliation','cancelling','cancelled','failed','succeeded','superseded'))"
	workflowRunFinishedBefore   = "CHECK((state IN ('succeeded','failed','cancelled')) = (finished_at IS NOT NULL))"
	workflowRunFinished         = "CHECK((state IN ('succeeded','failed','cancelled','superseded')) = (finished_at IS NOT NULL))"
	// terminalWorkflowStates is the SQL list of terminal run states.
	terminalWorkflowStates = "'succeeded','failed','cancelled','superseded'"
)

func ensureWorkflowMigrationSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_run_migrations (
 successor_id TEXT PRIMARY KEY REFERENCES workflow_runs(id) DEFERRABLE INITIALLY DEFERRED,
 predecessor_id TEXT NOT NULL UNIQUE REFERENCES workflow_runs(id),
 workspace_id TEXT NOT NULL,
 workflow TEXT NOT NULL CHECK(length(workflow) BETWEEN 1 AND 64),
 from_version INTEGER NOT NULL CHECK(from_version > 0),
 to_version INTEGER NOT NULL,
 checkpoint INTEGER NOT NULL CHECK(checkpoint >= 0),
 resume_step TEXT NOT NULL CHECK(length(resume_step) BETWEEN 1 AND 64),
 logical_key TEXT NOT NULL CHECK(length(logical_key) BETWEEN 1 AND 256),
 plan TEXT NOT NULL CHECK(json_valid(plan) AND json_type(plan) = 'object'),
 effects TEXT NOT NULL CHECK(json_valid(effects) AND json_type(effects) = 'array'),
 carried_elapsed_ms INTEGER NOT NULL CHECK(carried_elapsed_ms >= 0),
 carried_attempts INTEGER NOT NULL CHECK(carried_attempts >= 0),
 carried_tokens INTEGER NOT NULL CHECK(carried_tokens >= 0),
 carried_micro_usd INTEGER NOT NULL CHECK(carried_micro_usd >= 0),
 slot_timezone TEXT,
 slot_key TEXT,
 reason TEXT NOT NULL CHECK(length(reason) <= 1000),
 actor TEXT NOT NULL CHECK(actor = 'owner'),
 surface TEXT NOT NULL CHECK(surface IN ('cli','web')),
 recorded_at TEXT NOT NULL,
 CHECK(to_version > from_version),
 CHECK(successor_id <> predecessor_id),
 CHECK((slot_timezone IS NULL) = (slot_key IS NULL)));
 CREATE TRIGGER IF NOT EXISTS workflow_run_migrations_no_update BEFORE UPDATE ON workflow_run_migrations
 BEGIN SELECT RAISE(ABORT,'workflow run migrations are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_run_migrations_no_delete BEFORE DELETE ON workflow_run_migrations
 BEGIN SELECT RAISE(ABORT,'workflow run migrations are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_runs_superseded_by_migration BEFORE UPDATE OF state ON workflow_runs
 WHEN NEW.state = 'superseded' AND NOT EXISTS(SELECT 1 FROM workflow_run_migrations m WHERE m.predecessor_id=NEW.id)
 BEGIN SELECT RAISE(ABORT,'only the owner''s recorded migration supersedes a workflow run'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow migration schema: %w", err)
	}
	return nil
}

const migrationColumns = `successor_id,predecessor_id,from_version,to_version,checkpoint,resume_step,logical_key,plan,effects,
 carried_elapsed_ms,carried_attempts,carried_tokens,carried_micro_usd,slot_timezone,slot_key,reason,actor,surface,recorded_at`

func scanWorkflowRunMigration(scanner rowScanner) (WorkflowRunMigration, error) {
	var m WorkflowRunMigration
	var plan, effects, recorded string
	var tz, key sql.NullString
	if err := scanner.Scan(&m.SuccessorID, &m.PredecessorID, &m.FromVersion, &m.ToVersion, &m.Checkpoint, &m.Resume, &m.LogicalKey, &plan, &effects,
		&m.Carried.ElapsedMS, &m.Carried.Attempts, &m.Carried.Tokens, &m.Carried.MicroUSD, &tz, &key, &m.Reason, &m.Actor, &m.Surface, &recorded); err != nil {
		return m, err
	}
	m.Plan, m.SlotTimezone, m.SlotKey = json.RawMessage(plan), tz.String, key.String
	if err := json.Unmarshal([]byte(effects), &m.Effects); err != nil {
		return m, fmt.Errorf("read inherited effects: %w", err)
	}
	var err error
	m.RecordedAt, err = parseTime(recorded)
	return m, err
}

// readMigration reads the migration whose column (successor_id or
// predecessor_id) is runID; found is false when there is none.
func readMigration(ctx context.Context, q queryRower, column, runID string) (WorkflowRunMigration, bool, error) {
	m, err := scanWorkflowRunMigration(q.QueryRowContext(ctx, `SELECT `+migrationColumns+` FROM workflow_run_migrations WHERE `+column+`=?`, runID))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return WorkflowRunMigration{}, false, nil
	case err != nil:
		return WorkflowRunMigration{}, false, err
	}
	return m, true, nil
}

// WorkflowRunMigration reads the migration that created a successor run.
func (s *Store) WorkflowRunMigration(ctx context.Context, successorID string) (WorkflowRunMigration, bool, error) {
	return readMigration(ctx, s.db, "successor_id", successorID)
}

// carriedOf is what a run inherited from its predecessors.
func carriedOf(ctx context.Context, q queryRower, runID string) (WorkflowCarried, error) {
	m, found, err := readMigration(ctx, q, "successor_id", runID)
	if err != nil || !found {
		return WorkflowCarried{}, err
	}
	return m.Carried, nil
}

// WorkflowMigration is a successor the runtime validated for a run: the
// active version it continues under with both hashes and its pinned
// Execution Composition, the predecessor's checkpoint and the step it
// resumes at, the accepted input and carried state under that version, the
// plan's record and the logical Run Key, and the owner's reason.
type WorkflowMigration struct {
	PredecessorID  string
	Version        int64
	ExecutableHash string
	AuthorityHash  string
	Composition    json.RawMessage
	Checkpoint     int64
	Resume         string
	Inputs         json.RawMessage
	State          json.RawMessage
	Plan           json.RawMessage
	LogicalKey     string
	Reason         string
	Surface        string
}

// migrationRefused is a refusal with its reason.
func migrationRefused(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrWorkflowMigrationRefused, fmt.Sprintf(format, args...))
}

// MigrateWorkflowRun records the owner's migration of an unfinished run, in
// one transaction: the predecessor must still be at the checkpoint the
// plan was made from, unheld (no live lease, no attempt or worker in
// flight), with every Effect Intent resolved and no question open, and
// neither cancelling nor finished; the version must be the definition's
// active one (run admission) and newer than the predecessor's. It records
// the Successor Run (queued, holding the predecessor's Run Key, its first
// checkpoint the carried state and resume step), the migration with what
// the predecessor used of its limits, its effects and its schedule slot,
// and supersedes the predecessor.
func (s *Store) MigrateWorkflowRun(ctx context.Context, req WorkflowMigration) (WorkflowRunMigration, error) {
	reason := strings.TrimSpace(req.Reason)
	switch {
	case req.Surface != "cli" && req.Surface != "web":
		return WorkflowRunMigration{}, ErrWorkflowRunSurface
	case len(reason) > MaxWorkflowCancelReason:
		return WorkflowRunMigration{}, fmt.Errorf("a migration reason is at most %d bytes", MaxWorkflowCancelReason)
	case !workflowJSONObject(req.Inputs) || !workflowJSONObject(req.State) || !workflowJSONObject(req.Composition) || !workflowJSONObject(req.Plan):
		return WorkflowRunMigration{}, errors.New("a migration needs the successor's input, state, composition and plan as JSON objects")
	case req.Resume == "" || req.LogicalKey == "":
		return WorkflowRunMigration{}, errors.New("a migration needs the step its successor resumes at and the logical Run Key")
	}
	successor := uuid.NewString()
	var out WorkflowRunMigration
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		clock := s.now().UTC()
		now := formatTime(clock)
		p, err := readWorkflowRun(ctx, conn, req.PredecessorID)
		if err != nil {
			return err
		}
		switch p.State {
		case WorkflowRunSucceeded, WorkflowRunFailed, WorkflowRunCancelled, WorkflowRunSuperseded:
			return fmt.Errorf("%w: run %s is %s", ErrWorkflowRunFinished, p.ID, p.State)
		case WorkflowRunCancelling:
			return migrationRefused("run %s is cancelling: the owner's cancellation finishes it", p.ID)
		case WorkflowRunWaitingForHuman:
			return migrationRefused("run %s waits for the owner's answer: answer its question or cancel it first", p.ID)
		}
		if err := requireEffectsResolved(ctx, conn, p.ID); err != nil {
			return migrationRefused("run %s has an effect whose outcome is unknown: reconcile it first (evie workflows reconcile); %v", p.ID, err)
		}
		if p.State == WorkflowRunNeedsReconciliation {
			return migrationRefused("run %s needs reconciliation: reconcile it first", p.ID)
		}
		if err := requireNoOpenQuestion(ctx, conn, p.ID); err != nil {
			return migrationRefused("run %s: %v", p.ID, err)
		}
		lease, held, err := readWorkflowRunLease(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		if held && lease.ReleasedAt == nil && lease.ExpiresAt.After(clock) {
			return migrationRefused("another Evie (%s) is advancing run %s until %s; migrate it once it pauses", lease.Holder, p.ID, formatLeaseTime(lease.ExpiresAt))
		}
		var inFlight, openWorkers int
		if err := conn.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND state='running'),
 (SELECT COUNT(*) FROM workflow_worker_calls WHERE run_id=? AND finished_at IS NULL)`, p.ID, p.ID).Scan(&inFlight, &openWorkers); err != nil {
			return err
		}
		if inFlight > 0 || openWorkers > 0 {
			return migrationRefused("run %s has an attempt or worker in flight under an expired lease; the next claim records it interrupted (a running Evie does it), then migrate", p.ID)
		}
		c, err := latestCheckpoint(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		if c.Seq != req.Checkpoint || c.Next != req.Resume {
			return fmt.Errorf("%w: run %s is at checkpoint %d (next %s), not %d (next %s)", ErrWorkflowRunConflict, p.ID, c.Seq, c.Next, req.Checkpoint, req.Resume)
		}
		a, err := authorizeWorkflowActivation(ctx, conn, p.WorkspaceID, p.Workflow)
		if err != nil {
			return err
		}
		if a.Version != req.Version || a.ExecutableHash != req.ExecutableHash || a.AuthorityHash != req.AuthorityHash {
			return ErrWorkflowChanged
		}
		if a.Version <= p.Version {
			return migrationRefused("version %d is not newer than run %s's version %d", a.Version, p.ID, p.Version)
		}
		// What the predecessor already used of the limits its successor
		// continues: time (less its waits for answers and at the daily
		// cap, an open cap pause counted until now), counted attempts, and
		// tokens and cost, plus what it inherited itself.
		inherited, err := carriedOf(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		human, err := questionWaits(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		capped, err := capWaitsUntil(ctx, conn, p.ID, clock)
		if err != nil {
			return err
		}
		elapsed := max(clock.Sub(p.CreatedAt)-human-capped, 0) + inherited.Elapsed()
		carried := WorkflowCarried{ElapsedMS: elapsed.Milliseconds(), Attempts: inherited.Attempts, Tokens: inherited.Tokens, MicroUSD: inherited.MicroUSD}
		var counted int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_node_attempts WHERE run_id=? AND kind NOT IN ('branch','map') AND state<>'interrupted'`, p.ID).Scan(&counted); err != nil {
			return err
		}
		modelTokens, modelMicro, err := modelSpend(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		workerTokens, workerMicro, err := workerSpend(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		carried.Attempts += counted
		carried.Tokens += modelTokens + workerTokens
		carried.MicroUSD += modelMicro + workerMicro
		intents, err := workflowEffectIntents(ctx, conn, p.ID)
		if err != nil {
			return err
		}
		effects := []WorkflowInheritedEffect{}
		for _, i := range intents {
			e := WorkflowInheritedEffect{RunID: i.RunID, Seq: i.Seq, Step: i.Step, Checkpoint: i.Checkpoint, IdempotencyKey: i.IdempotencyKey, Outcome: i.State()}
			if i.Receipt != nil {
				e.Source = i.Receipt.Source
			}
			effects = append(effects, e)
		}
		encodedEffects, err := json.Marshal(effects)
		if err != nil {
			return err
		}
		var slotTZ, slotKey any
		if slot, err := decidedSlot(ctx, conn, p.ID); err != nil {
			return err
		} else if slot != nil {
			slotTZ, slotKey = slot.Timezone, slot.Key
		}
		w, err := readWorkspaceRow(ctx, conn, p.WorkspaceID)
		if err != nil {
			return err
		}
		access, err := workspaceAccess(ctx, conn, p.WorkspaceID)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_migrations(workspace_id,workflow,`+migrationColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'owner',?,?)`,
			p.WorkspaceID, p.Workflow, successor, p.ID, p.Version, a.Version, c.Seq, req.Resume, req.LogicalKey, string(req.Plan), string(encodedEffects),
			carried.ElapsedMS, carried.Attempts, carried.Tokens, carried.MicroUSD, slotTZ, slotKey, reason, req.Surface, now); err != nil {
			return fmt.Errorf("record workflow migration: %w", err)
		}
		// The predecessor is superseded first, which frees its Run Key for
		// its successor; nothing else about it changes.
		detail := fmt.Sprintf("superseded by run %s: migrated to version %d by the owner via %s%s", successor, a.Version, req.Surface, reasonSuffix(reason))
		if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state='superseded',error=?,not_before=NULL,finished_at=?,updated_at=? WHERE id=?`,
			detail, now, now, p.ID); err != nil {
			return fmt.Errorf("supersede workflow run: %w", err)
		}
		var session any
		if p.SessionID != "" {
			session = string(p.SessionID)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_runs(id,workspace_id,workflow,version,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,
 composition,run_key,inputs,state,outputs,error,origin_surface,session_id,created_at,started_at,finished_at,updated_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'queued',NULL,'',?,?,?,NULL,NULL,?)`, successor, p.WorkspaceID, p.Workflow, a.Version, a.Commit, a.Path, a.ExecutableHash, a.AuthorityHash,
			w.revision, access.Revision, string(req.Composition), p.RunKey, string(req.Inputs), p.Surface, session, now, now); err != nil {
			return fmt.Errorf("record successor workflow run: %w", err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_run_checkpoints(run_id,seq,after_attempt,next_step,state,created_at) VALUES(?,0,NULL,?,?,?)`,
			successor, req.Resume, string(req.State), now); err != nil {
			return fmt.Errorf("record successor checkpoint: %w", err)
		}
		out, _, err = readMigration(ctx, conn, "successor_id", successor)
		return err
	})
	return out, err
}

// capWaitsUntil is capWaits with a pause at the cap that has not ended
// counted until until.
func capWaitsUntil(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string, until time.Time) (time.Duration, error) {
	rows, err := q.QueryContext(ctx, `SELECT kind,detail,created_at FROM workflow_run_events WHERE run_id=? AND kind IN ('paused','resumed') ORDER BY seq`, runID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total time.Duration
	var pausedAt *time.Time
	for rows.Next() {
		var kind, detail, created string
		if err := rows.Scan(&kind, &detail, &created); err != nil {
			return 0, err
		}
		at, err := parseTime(created)
		if err != nil {
			return 0, err
		}
		if pausedAt != nil && (kind == string(WorkflowEventResumed) || !strings.HasPrefix(detail, WorkflowDailyCapReason)) {
			total += max(at.Sub(*pausedAt), 0)
			pausedAt = nil
		}
		if kind == string(WorkflowEventPaused) && strings.HasPrefix(detail, WorkflowDailyCapReason) && pausedAt == nil {
			pausedAt = &at
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if pausedAt != nil {
		total += max(until.Sub(*pausedAt), 0)
	}
	return total, nil
}

// workflowRunChain is the SQL of a run and every predecessor it continues,
// for the run ID bound to its one parameter.
const workflowRunChain = `WITH RECURSIVE chain(id) AS (SELECT ? UNION SELECT m.predecessor_id FROM workflow_run_migrations m JOIN chain c ON m.successor_id=c.id)`

// WorkflowRunRecord reads one run without its history.
func (s *Store) WorkflowRunRecord(ctx context.Context, id string) (WorkflowRun, error) {
	return readWorkflowRun(ctx, s.db, id)
}
