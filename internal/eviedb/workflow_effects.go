package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// The effect protocol's durable records (Stage 14 sub-stage 6 of
// docs/memory-architecture.md, 2026-10-04; workflow-runtime.spec.md, ADR
// 0008, 0009, 0017, 0027, 0029, 0043). SQLite and an external provider
// cannot commit atomically, so every effect leaves evidence of where it
// stopped:
//
//   - workflow_effect_intents: one row per provider call, recorded with
//     its node attempt in one fenced transaction after per-node admission,
//     before the call: the step and checkpoint, the provider pinned for it
//     (Capability ID, contract, schema hash, plugin and version), the
//     connection and resource, the stable idempotency key every attempt of
//     the node at that checkpoint shares, whether the provider is
//     idempotent and who reconciles it, its Resource Conflict Keys, its
//     input and the authority evaluation that admitted it (the operation of
//     the run's own Standing Authority, this call's number against
//     max_calls_per_run, the access revision). No intent means the effect
//     did not start.
//   - workflow_effect_unknowns: an intent whose outcome became unknown
//     (the call timed out after it may have been accepted, answered outside
//     its contract, panicked, or its runtime stopped before the receipt),
//     with the reason and any response, recorded once.
//   - workflow_effect_receipts: how an intent ended, recorded once: the
//     provider's answer with the attempt's outcome and the next checkpoint
//     in one transaction, or a reconciliation by the provider or the owner
//     with its evidence. succeeded means the effect happened (its response
//     is the step's output); failed means it is proven not to have
//     happened.
//   - workflow_effect_conflicts: the Resource Conflict Keys an intent
//     holds until its receipt. A partial unique index lets one unresolved
//     intent hold a key at a time, across every run and Workspace, so
//     effects sharing a key are serialized.
//
// Intents, unknowns and receipts are immutable; a conflict hold is only
// ever released. While an intent of a run has no receipt the run is
// blocked: no node attempt begins, and it is never paused, resumed or
// failed; it needs reconciliation (needs_reconciliation), which only a
// receipt ends.

var (
	// ErrWorkflowEffectUnresolved refuses what an Effect Intent without a
	// receipt blocks.
	ErrWorkflowEffectUnresolved = errors.New("an Effect Intent of the workflow run has no receipt: its outcome must be reconciled first")
	// ErrWorkflowEffectLimit refuses an intent past its operation's
	// max_calls_per_run.
	ErrWorkflowEffectLimit = errors.New("the workflow run reached the operation's max_calls_per_run")
	// ErrWorkflowEffectConflict refuses an intent whose Resource Conflict
	// Key another run's unresolved intent holds.
	ErrWorkflowEffectConflict = errors.New("a Resource Conflict Key of the effect is held")
	// ErrWorkflowEffectNotUnknown refuses reconciling an intent whose
	// outcome is not unknown.
	ErrWorkflowEffectNotUnknown = errors.New("the Effect Intent's outcome is not unknown")
)

// WorkflowEffectState is an Effect Intent's state, derived from its
// records; a receipt's outcome is succeeded or failed.
type WorkflowEffectState string

const (
	// WorkflowEffectPending: recorded, the call in flight.
	WorkflowEffectPending WorkflowEffectState = "pending"
	// WorkflowEffectUnknown: Outcome Unknown, awaiting reconciliation.
	WorkflowEffectUnknown WorkflowEffectState = "unknown"
	// WorkflowEffectSucceeded: the effect happened.
	WorkflowEffectSucceeded WorkflowEffectState = "succeeded"
	// WorkflowEffectFailed: the effect is proven not to have happened.
	WorkflowEffectFailed WorkflowEffectState = "failed"
)

// WorkflowEffectReconcileMode is who reconciles an intent whose outcome is
// unknown.
type WorkflowEffectReconcileMode string

const (
	WorkflowEffectReconcileByProvider WorkflowEffectReconcileMode = "provider"
	WorkflowEffectReconcileByOwner    WorkflowEffectReconcileMode = "owner"
)

// WorkflowEffectSource is where a receipt's outcome came from.
type WorkflowEffectSource string

const (
	// WorkflowEffectFromProvider: the call's own answer.
	WorkflowEffectFromProvider WorkflowEffectSource = "provider"
	// WorkflowEffectFromReconcile: the provider's reconciliation (lookup
	// or idempotent replay by key).
	WorkflowEffectFromReconcile WorkflowEffectSource = "reconcile"
	// WorkflowEffectFromOwner: the owner's reconciliation with evidence.
	WorkflowEffectFromOwner WorkflowEffectSource = "owner"
)

// WorkflowEffectIntent is one Effect Intent with what became of it.
type WorkflowEffectIntent struct {
	RunID string `json:"run_id"`
	Seq   int64  `json:"seq"`
	// AttemptSeq is the node attempt that sent it, Attempt its number and
	// AttemptState its state.
	AttemptSeq     int64                       `json:"attempt_seq"`
	Attempt        int                         `json:"attempt"`
	AttemptState   WorkflowAttemptState        `json:"attempt_state"`
	WorkspaceID    memory.WorkspaceID          `json:"workspace_id"`
	Step           string                      `json:"step"`
	Checkpoint     int64                       `json:"checkpoint"`
	Capability     string                      `json:"capability"`
	Contract       string                      `json:"contract"`
	SchemaSHA256   string                      `json:"schema_sha256"`
	Plugin         string                      `json:"plugin"`
	Version        string                      `json:"version"`
	Connection     string                      `json:"connection"`
	Resource       string                      `json:"resource"`
	IdempotencyKey string                      `json:"idempotency_key"`
	Idempotent     bool                        `json:"idempotent"`
	Reconcile      WorkflowEffectReconcileMode `json:"reconcile"`
	ConflictKeys   []string                    `json:"conflict_keys"`
	Input          json.RawMessage             `json:"input"`
	// Authority is the evaluation that admitted it.
	Authority   json.RawMessage               `json:"authority"`
	LeaseHolder string                        `json:"lease_holder"`
	LeaseToken  int64                         `json:"lease_token"`
	CreatedAt   time.Time                     `json:"created_at"`
	Unknown     *WorkflowEffectUnknownOutcome `json:"unknown,omitempty"`
	Receipt     *WorkflowEffectReceipt        `json:"receipt,omitempty"`
}

// State derives the intent's state from its records.
func (i WorkflowEffectIntent) State() WorkflowEffectState {
	switch {
	case i.Receipt != nil:
		return i.Receipt.Outcome
	case i.Unknown != nil:
		return WorkflowEffectUnknown
	}
	return WorkflowEffectPending
}

// WorkflowEffectUnknownOutcome is why an intent's outcome became unknown.
type WorkflowEffectUnknownOutcome struct {
	Reason string `json:"reason"`
	// Response is what the provider answered, when it answered outside
	// its contract.
	Response    json.RawMessage `json:"response,omitempty"`
	LeaseHolder string          `json:"lease_holder"`
	LeaseToken  int64           `json:"lease_token"`
	RecordedAt  time.Time       `json:"recorded_at"`
}

// WorkflowEffectReceipt is how an intent ended.
type WorkflowEffectReceipt struct {
	Outcome  WorkflowEffectState  `json:"outcome"`
	Response json.RawMessage      `json:"response,omitempty"`
	Error    string               `json:"error,omitempty"`
	Source   WorkflowEffectSource `json:"source"`
	Evidence string               `json:"evidence,omitempty"`
	// Actor is "runtime" or "owner", and Surface the owner's surface.
	Actor       string    `json:"actor"`
	Surface     string    `json:"surface,omitempty"`
	LeaseHolder string    `json:"lease_holder"`
	LeaseToken  int64     `json:"lease_token"`
	RecordedAt  time.Time `json:"recorded_at"`
}

// WorkflowEffectConflictError names the unresolved intent holding a
// Resource Conflict Key; Unknown when its outcome is unknown.
type WorkflowEffectConflictError struct {
	Key       string
	RunID     string
	Step      string
	IntentSeq int64
	Unknown   bool
}

func (e *WorkflowEffectConflictError) Error() string {
	held := "in flight"
	if e.Unknown {
		held = "waiting for reconciliation"
	}
	return fmt.Sprintf("Resource Conflict Key %s is held by run %s steps.%s (Effect Intent #%d, %s)", e.Key, e.RunID, e.Step, e.IntentSeq, held)
}

func (e *WorkflowEffectConflictError) Unwrap() error { return ErrWorkflowEffectConflict }

// WorkflowEffectLimitError is an Effect Intent refused because the run
// already sent as many intents of the operation as its max_calls_per_run
// allows.
type WorkflowEffectLimitError struct {
	Step       string
	Capability string
	Connection string
	Calls      int
	Max        int
}

func (e *WorkflowEffectLimitError) Error() string {
	return fmt.Sprintf("steps.%s: the Standing Authority's max_calls_per_run %d for %s on %s is reached (Effect Intents sent in this run: %d); the effect is refused before the provider is called",
		e.Step, e.Max, e.Capability, e.Connection, e.Calls)
}

func (e *WorkflowEffectLimitError) Unwrap() error { return ErrWorkflowEffectLimit }

// maxEffectText bounds reasons, errors and evidence.
const maxEffectText = 4000

func ensureWorkflowEffectSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_effect_intents (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq > 0),
 attempt_seq INTEGER NOT NULL CHECK(attempt_seq > 0),
 attempt INTEGER NOT NULL CHECK(attempt > 0),
 workspace_id TEXT NOT NULL,
 step_id TEXT NOT NULL CHECK(length(step_id) BETWEEN 1 AND 64),
 checkpoint INTEGER NOT NULL CHECK(checkpoint >= 0),
 capability TEXT NOT NULL CHECK(length(capability) BETWEEN 1 AND 128),
 contract TEXT NOT NULL CHECK(length(contract) BETWEEN 1 AND 64),
 schema_sha256 TEXT NOT NULL CHECK(length(schema_sha256) = 64),
 plugin TEXT NOT NULL CHECK(length(plugin) BETWEEN 1 AND 128),
 plugin_version TEXT NOT NULL CHECK(length(plugin_version) BETWEEN 1 AND 64),
 connection TEXT NOT NULL CHECK(length(connection) BETWEEN 1 AND 256),
 resource TEXT NOT NULL CHECK(length(resource) BETWEEN 1 AND 256),
 idempotency_key TEXT NOT NULL CHECK(length(idempotency_key) BETWEEN 1 AND 128),
 idempotent INTEGER NOT NULL CHECK(idempotent IN (0,1)),
 reconcile TEXT NOT NULL CHECK(reconcile IN ('provider','owner')),
 conflict_keys TEXT NOT NULL CHECK(json_valid(conflict_keys) AND json_type(conflict_keys) = 'array' AND json_array_length(conflict_keys) > 0),
 input TEXT NOT NULL CHECK(json_valid(input) AND json_type(input) = 'object'),
 authority TEXT NOT NULL CHECK(json_valid(authority) AND json_type(authority) = 'object'),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 created_at TEXT NOT NULL,
 PRIMARY KEY(run_id, seq),
 UNIQUE(run_id, attempt_seq));
 CREATE INDEX IF NOT EXISTS workflow_effect_intents_by_step ON workflow_effect_intents(run_id, checkpoint, step_id);
 CREATE TRIGGER IF NOT EXISTS workflow_effect_intents_no_update BEFORE UPDATE ON workflow_effect_intents
 BEGIN SELECT RAISE(ABORT,'workflow Effect Intents are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_effect_intents_no_delete BEFORE DELETE ON workflow_effect_intents
 BEGIN SELECT RAISE(ABORT,'workflow Effect Intents are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_effect_unknowns (
 run_id TEXT NOT NULL,
 intent_seq INTEGER NOT NULL,
 reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 4000),
 response TEXT CHECK(response IS NULL OR json_valid(response)),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(run_id, intent_seq),
 FOREIGN KEY(run_id, intent_seq) REFERENCES workflow_effect_intents(run_id, seq));
 CREATE TRIGGER IF NOT EXISTS workflow_effect_unknowns_no_update BEFORE UPDATE ON workflow_effect_unknowns
 BEGIN SELECT RAISE(ABORT,'unknown workflow effect outcomes are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_effect_unknowns_no_delete BEFORE DELETE ON workflow_effect_unknowns
 BEGIN SELECT RAISE(ABORT,'unknown workflow effect outcomes are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_effect_receipts (
 run_id TEXT NOT NULL,
 intent_seq INTEGER NOT NULL,
 outcome TEXT NOT NULL CHECK(outcome IN ('succeeded','failed')),
 response TEXT CHECK(response IS NULL OR json_valid(response)),
 error TEXT NOT NULL CHECK(length(error) <= 4000),
 source TEXT NOT NULL CHECK(source IN ('provider','reconcile','owner')),
 evidence TEXT NOT NULL CHECK(length(evidence) <= 4000),
 actor TEXT NOT NULL CHECK(actor IN ('runtime','owner')),
 surface TEXT NOT NULL CHECK(surface IN ('','cli','web')),
 lease_holder TEXT NOT NULL CHECK(length(lease_holder) BETWEEN 1 AND 128),
 lease_token INTEGER NOT NULL CHECK(lease_token > 0),
 recorded_at TEXT NOT NULL,
 PRIMARY KEY(run_id, intent_seq),
 FOREIGN KEY(run_id, intent_seq) REFERENCES workflow_effect_intents(run_id, seq),
 CHECK(outcome <> 'succeeded' OR response IS NOT NULL),
 CHECK((actor = 'owner') = (source = 'owner')),
 CHECK((source = 'owner') = (surface <> '')));
 CREATE TRIGGER IF NOT EXISTS workflow_effect_receipts_no_update BEFORE UPDATE ON workflow_effect_receipts
 BEGIN SELECT RAISE(ABORT,'workflow Effect Receipts are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_effect_receipts_no_delete BEFORE DELETE ON workflow_effect_receipts
 BEGIN SELECT RAISE(ABORT,'workflow Effect Receipts are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_effect_conflicts (
 conflict_key TEXT NOT NULL CHECK(length(conflict_key) BETWEEN 1 AND 512),
 run_id TEXT NOT NULL,
 intent_seq INTEGER NOT NULL,
 acquired_at TEXT NOT NULL,
 released_at TEXT,
 PRIMARY KEY(run_id, intent_seq, conflict_key),
 FOREIGN KEY(run_id, intent_seq) REFERENCES workflow_effect_intents(run_id, seq));
 CREATE UNIQUE INDEX IF NOT EXISTS workflow_effect_conflicts_held ON workflow_effect_conflicts(conflict_key) WHERE released_at IS NULL;
 CREATE TRIGGER IF NOT EXISTS workflow_effect_conflicts_release_only BEFORE UPDATE ON workflow_effect_conflicts
 WHEN OLD.released_at IS NOT NULL OR NEW.released_at IS NULL OR NEW.conflict_key <> OLD.conflict_key OR NEW.run_id <> OLD.run_id
 OR NEW.intent_seq <> OLD.intent_seq OR NEW.acquired_at <> OLD.acquired_at
 BEGIN SELECT RAISE(ABORT,'a workflow Resource Conflict Key hold is only released'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_effect_conflicts_no_delete BEFORE DELETE ON workflow_effect_conflicts
 BEGIN SELECT RAISE(ABORT,'workflow Resource Conflict Key holds are kept'); END;`)
	if err != nil {
		return fmt.Errorf("create workflow effect schema: %w", err)
	}
	return nil
}

const effectColumns = `i.run_id,i.seq,i.attempt_seq,i.attempt,a.state,i.workspace_id,i.step_id,i.checkpoint,i.capability,i.contract,i.schema_sha256,i.plugin,i.plugin_version,
 i.connection,i.resource,i.idempotency_key,i.idempotent,i.reconcile,i.conflict_keys,i.input,i.authority,i.lease_holder,i.lease_token,i.created_at,
 u.reason,u.response,u.lease_holder,u.lease_token,u.recorded_at,
 r.outcome,r.response,r.error,r.source,r.evidence,r.actor,r.surface,r.lease_holder,r.lease_token,r.recorded_at`

const effectFrom = ` FROM workflow_effect_intents i JOIN workflow_node_attempts a ON a.run_id=i.run_id AND a.seq=i.attempt_seq
 LEFT JOIN workflow_effect_unknowns u ON u.run_id=i.run_id AND u.intent_seq=i.seq
 LEFT JOIN workflow_effect_receipts r ON r.run_id=i.run_id AND r.intent_seq=i.seq`

func scanWorkflowEffectIntent(scanner rowScanner) (WorkflowEffectIntent, error) {
	var i WorkflowEffectIntent
	var keys, input, authority, created string
	var uReason, uResponse, uHolder, uRecorded sql.NullString
	var uToken sql.NullInt64
	var rOutcome, rResponse, rError, rSource, rEvidence, rActor, rSurface, rHolder, rRecorded sql.NullString
	var rToken sql.NullInt64
	if err := scanner.Scan(&i.RunID, &i.Seq, &i.AttemptSeq, &i.Attempt, &i.AttemptState, &i.WorkspaceID, &i.Step, &i.Checkpoint, &i.Capability, &i.Contract,
		&i.SchemaSHA256, &i.Plugin, &i.Version, &i.Connection, &i.Resource, &i.IdempotencyKey, &i.Idempotent, &i.Reconcile, &keys, &input, &authority,
		&i.LeaseHolder, &i.LeaseToken, &created,
		&uReason, &uResponse, &uHolder, &uToken, &uRecorded,
		&rOutcome, &rResponse, &rError, &rSource, &rEvidence, &rActor, &rSurface, &rHolder, &rToken, &rRecorded); err != nil {
		return i, err
	}
	if err := json.Unmarshal([]byte(keys), &i.ConflictKeys); err != nil {
		return i, fmt.Errorf("read Effect Intent conflict keys: %w", err)
	}
	i.Input, i.Authority = json.RawMessage(input), json.RawMessage(authority)
	var err error
	if i.CreatedAt, err = parseTime(created); err != nil {
		return i, err
	}
	if uReason.Valid {
		u := &WorkflowEffectUnknownOutcome{Reason: uReason.String, LeaseHolder: uHolder.String, LeaseToken: uToken.Int64}
		if uResponse.Valid {
			u.Response = json.RawMessage(uResponse.String)
		}
		if u.RecordedAt, err = parseTime(uRecorded.String); err != nil {
			return i, err
		}
		i.Unknown = u
	}
	if rOutcome.Valid {
		r := &WorkflowEffectReceipt{Outcome: WorkflowEffectState(rOutcome.String), Error: rError.String, Source: WorkflowEffectSource(rSource.String),
			Evidence: rEvidence.String, Actor: rActor.String, Surface: rSurface.String, LeaseHolder: rHolder.String, LeaseToken: rToken.Int64}
		if rResponse.Valid {
			r.Response = json.RawMessage(rResponse.String)
		}
		if r.RecordedAt, err = parseTime(rRecorded.String); err != nil {
			return i, err
		}
		i.Receipt = r
	}
	return i, nil
}

func workflowEffectIntents(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string) ([]WorkflowEffectIntent, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+effectColumns+effectFrom+` WHERE i.run_id=? ORDER BY i.seq`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowEffectIntent{}
	for rows.Next() {
		i, err := scanWorkflowEffectIntent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func readWorkflowEffectIntent(ctx context.Context, q queryRower, runID string, seq int64) (WorkflowEffectIntent, error) {
	i, err := scanWorkflowEffectIntent(q.QueryRowContext(ctx, `SELECT `+effectColumns+effectFrom+` WHERE i.run_id=? AND i.seq=?`, runID, seq))
	if errors.Is(err, sql.ErrNoRows) {
		return i, fmt.Errorf("%w: run %s has no Effect Intent #%d", ErrWorkflowRunConflict, runID, seq)
	}
	return i, err
}

// WorkflowEffectIntents lists a run's Effect Intents in order, each with
// its unknown outcome and receipt.
func (s *Store) WorkflowEffectIntents(ctx context.Context, runID string) ([]WorkflowEffectIntent, error) {
	return workflowEffectIntents(ctx, s.db, runID)
}

// requireEffectsResolved refuses while an Effect Intent of the run has no
// receipt.
func requireEffectsResolved(ctx context.Context, q queryRower, runID string) error {
	var seq int64
	var step string
	err := q.QueryRowContext(ctx, `SELECT i.seq,i.step_id FROM workflow_effect_intents i WHERE i.run_id=?
 AND NOT EXISTS(SELECT 1 FROM workflow_effect_receipts r WHERE r.run_id=i.run_id AND r.intent_seq=i.seq) ORDER BY i.seq LIMIT 1`, runID).Scan(&seq, &step)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	}
	return fmt.Errorf("%w (steps.%s, Effect Intent #%d)", ErrWorkflowEffectUnresolved, step, seq)
}

// requireAttemptEffectResolved refuses while the attempt's Effect Intent,
// if it sent one, has no receipt.
func requireAttemptEffectResolved(ctx context.Context, q queryRower, runID string, attemptSeq int64) error {
	var seq int64
	err := q.QueryRowContext(ctx, `SELECT i.seq FROM workflow_effect_intents i WHERE i.run_id=? AND i.attempt_seq=?
 AND NOT EXISTS(SELECT 1 FROM workflow_effect_receipts r WHERE r.run_id=i.run_id AND r.intent_seq=i.seq)`, runID, attemptSeq).Scan(&seq)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil
	case err != nil:
		return err
	}
	return fmt.Errorf("%w (Effect Intent #%d of attempt %d)", ErrWorkflowEffectUnresolved, seq, attemptSeq)
}

// heldConflict finds an unresolved intent of another run holding one of
// keys.
func heldConflict(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, runID string, keys []string) (*WorkflowEffectConflictError, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	args := []any{runID}
	for _, k := range keys {
		args = append(args, k)
	}
	rows, err := q.QueryContext(ctx, `SELECT c.conflict_key,c.run_id,c.intent_seq,i.step_id,
 EXISTS(SELECT 1 FROM workflow_effect_unknowns u WHERE u.run_id=c.run_id AND u.intent_seq=c.intent_seq)
 FROM workflow_effect_conflicts c JOIN workflow_effect_intents i ON i.run_id=c.run_id AND i.seq=c.intent_seq
 WHERE c.released_at IS NULL AND c.run_id<>? AND c.conflict_key IN (`+placeholders+`) ORDER BY c.conflict_key LIMIT 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	held := &WorkflowEffectConflictError{}
	if err := rows.Scan(&held.Key, &held.RunID, &held.IntentSeq, &held.Step, &held.Unknown); err != nil {
		return nil, err
	}
	return held, rows.Err()
}

// WorkflowEffectConflict reports the unresolved Effect Intent of another
// run that holds one of keys, or nil, changing nothing.
func (s *Store) WorkflowEffectConflict(ctx context.Context, runID string, keys []string) (*WorkflowEffectConflictError, error) {
	return heldConflict(ctx, s.db, runID, keys)
}

// WorkflowEffectStart is an effect about to be sent: its node attempt and
// the Effect Intent the runtime derived for it.
type WorkflowEffectStart struct {
	Attempt        WorkflowAttemptStart
	Capability     string
	Contract       string
	SchemaSHA256   string
	Plugin         string
	Version        string
	Connection     string
	Resource       string
	IdempotencyKey string
	Idempotent     bool
	Reconcile      WorkflowEffectReconcileMode
	ConflictKeys   []string
}

func (e WorkflowEffectStart) valid() error {
	switch {
	case e.Capability == "" || e.Contract == "" || len(e.SchemaSHA256) != 64 || e.Plugin == "" || e.Version == "" || e.Connection == "" || e.Resource == "":
		return errors.New("an Effect Intent names its capability, contract, schema, provider, connection and resource")
	case e.IdempotencyKey == "" || len(e.IdempotencyKey) > 128:
		return errors.New("an Effect Intent needs an idempotency key of 1 to 128 bytes")
	case e.Reconcile != WorkflowEffectReconcileByProvider && e.Reconcile != WorkflowEffectReconcileByOwner:
		return errors.New("an Effect Intent names who reconciles it")
	case len(e.ConflictKeys) == 0 || !workflowJSONObject(e.Attempt.Input):
		return errors.New("an Effect Intent needs its Resource Conflict Keys and an input object")
	}
	for _, k := range e.ConflictKeys {
		if k == "" || len(k) > 512 {
			return errors.New("a Resource Conflict Key is 1 to 512 bytes")
		}
	}
	return nil
}

// BeginWorkflowEffect durably records an effect's node attempt and its
// Effect Intent, in one transaction under the run's current lease, before
// the provider is called. In that transaction it applies per-node
// admission (the run's admission, its own approved Standing Authority and
// the Workspace's current access; a refusal is ErrWorkflowNodeRefused,
// ErrWorkflowNotActive or ErrWorkflowBlocked), counts the run's intents of
// the operation against its max_calls_per_run (ErrWorkflowEffectLimit),
// and takes the intent's Resource Conflict Keys, refused while another
// run's unresolved intent holds one (ErrWorkflowEffectConflict). Nothing is
// recorded when it is refused.
func (s *Store) BeginWorkflowEffect(ctx context.Context, lease WorkflowRunLease, e WorkflowEffectStart) (WorkflowEffectIntent, error) {
	if err := validAttemptStart(lease, e.Attempt); err != nil {
		return WorkflowEffectIntent{}, err
	}
	if err := e.valid(); err != nil {
		return WorkflowEffectIntent{}, err
	}
	keys := slices.Clone(e.ConflictKeys)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	var seq int64
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, e.Attempt.RunID)
		if err != nil {
			return err
		}
		admitted, err := admitWorkflowNode(ctx, conn, run, WorkflowNodeAdmission{Step: e.Attempt.Step, Capability: e.Capability, Connection: e.Connection, Resource: e.Resource})
		if err != nil {
			return err
		}
		// A Successor Run counts its predecessors' intents too (sub-stage
		// 12): max_calls_per_run bounds one logical execution.
		var calls int
		if err := conn.QueryRowContext(ctx, workflowRunChain+` SELECT COUNT(*) FROM workflow_effect_intents WHERE run_id IN (SELECT id FROM chain) AND capability=? AND connection=?`,
			run.ID, e.Capability, e.Connection).Scan(&calls); err != nil {
			return err
		}
		op := admitted.operation
		if calls >= op.MaxCallsPerRun {
			return &WorkflowEffectLimitError{Step: e.Attempt.Step, Capability: e.Capability, Connection: e.Connection, Calls: calls, Max: op.MaxCallsPerRun}
		}
		if held, err := heldConflict(ctx, conn, run.ID, keys); err != nil || held != nil {
			if err != nil {
				return err
			}
			return held
		}
		attempt, err := beginWorkflowAttempt(ctx, conn, lease, e.Attempt, now)
		if err != nil {
			return err
		}
		authority, err := json.Marshal(map[string]any{
			"version": run.Version, "authority_hash": run.AuthorityHash, "capability": op.Capability, "connection": op.Connection,
			"resource": e.Resource, "resources": op.Resources, "max_calls_per_run": op.MaxCallsPerRun, "calls": calls + 1,
			"access_revision": admitted.access,
		})
		if err != nil {
			return err
		}
		encodedKeys, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		if err := conn.QueryRowContext(ctx, `INSERT INTO workflow_effect_intents(run_id,seq,attempt_seq,attempt,workspace_id,step_id,checkpoint,capability,contract,schema_sha256,
 plugin,plugin_version,connection,resource,idempotency_key,idempotent,reconcile,conflict_keys,input,authority,lease_holder,lease_token,created_at)
 VALUES(?,(SELECT COALESCE(MAX(seq),0)+1 FROM workflow_effect_intents WHERE run_id=?),?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) RETURNING seq`,
			run.ID, run.ID, attempt.Seq, attempt.Attempt, run.WorkspaceID, attempt.Step, attempt.InputCheckpoint, e.Capability, e.Contract, e.SchemaSHA256,
			e.Plugin, e.Version, e.Connection, e.Resource, e.IdempotencyKey, e.Idempotent, string(e.Reconcile), string(encodedKeys), string(e.Attempt.Input),
			string(authority), lease.Holder, lease.Token, formatTime(now)).Scan(&seq); err != nil {
			return fmt.Errorf("record Effect Intent: %w", err)
		}
		for _, key := range keys {
			if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_effect_conflicts(conflict_key,run_id,intent_seq,acquired_at,released_at) VALUES(?,?,?,?,NULL)`,
				key, run.ID, seq, formatTime(now)); err != nil {
				return fmt.Errorf("hold Resource Conflict Key %s: %w", key, err)
			}
		}
		return nil
	})
	if err != nil {
		return WorkflowEffectIntent{}, err
	}
	return readWorkflowEffectIntent(ctx, s.db, e.Attempt.RunID, seq)
}

// releaseConflicts releases the Resource Conflict Keys an intent holds.
func releaseConflicts(ctx context.Context, conn *sql.Conn, runID string, seq int64, now time.Time) error {
	_, err := conn.ExecContext(ctx, `UPDATE workflow_effect_conflicts SET released_at=? WHERE run_id=? AND intent_seq=? AND released_at IS NULL`,
		formatTime(now), runID, seq)
	return err
}

func validEffectText(text string) error {
	if len(text) > maxEffectText {
		return fmt.Errorf("effect reasons, errors and evidence are at most %d bytes", maxEffectText)
	}
	return nil
}

// insertReceipt records an intent's receipt and releases its keys.
func insertReceipt(ctx context.Context, conn *sql.Conn, runID string, seq int64, r WorkflowEffectReceipt, now time.Time) error {
	var response any
	if len(r.Response) > 0 {
		response = string(r.Response)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_effect_receipts(run_id,intent_seq,outcome,response,error,source,evidence,actor,surface,lease_holder,lease_token,recorded_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, runID, seq, string(r.Outcome), response, r.Error, string(r.Source), r.Evidence, r.Actor, r.Surface, r.LeaseHolder, r.LeaseToken,
		formatTime(now)); err != nil {
		return fmt.Errorf("record Effect Receipt: %w", err)
	}
	return releaseConflicts(ctx, conn, runID, seq, now)
}

// WorkflowEffectEnd is the provider's answer to an intent's call: its
// receipt (Outcome succeeded with the response, or failed with the error
// proving nothing happened) and how the attempt ends (Attempt; its RunID
// and Seq are the intent's).
type WorkflowEffectEnd struct {
	RunID     string
	IntentSeq int64
	Outcome   WorkflowEffectState
	Response  json.RawMessage
	Error     string
	Attempt   WorkflowAttemptEnd
}

// FinishWorkflowEffect records the provider's answer in one transaction
// under the run's current lease: the Effect Receipt, the attempt's
// normalized output with the next checkpoint (or its failure, retry time
// or the run's failure), and the release of its Resource Conflict Keys.
func (s *Store) FinishWorkflowEffect(ctx context.Context, lease WorkflowRunLease, e WorkflowEffectEnd) (WorkflowRun, error) {
	switch {
	case e.Outcome == WorkflowEffectSucceeded && (len(e.Response) == 0 || !json.Valid(e.Response) || e.Attempt.Error != ""):
		return WorkflowRun{}, errors.New("a succeeded effect needs its response and a succeeded attempt")
	case e.Outcome == WorkflowEffectFailed && (e.Error == "" || e.Attempt.Error == ""):
		return WorkflowRun{}, errors.New("a failed effect needs the error proving it did not happen and a failed attempt")
	case e.Outcome != WorkflowEffectSucceeded && e.Outcome != WorkflowEffectFailed:
		return WorkflowRun{}, errors.New("a provider's answer is succeeded or failed; an unknown outcome is marked unknown")
	}
	if err := validEffectText(e.Error); err != nil {
		return WorkflowRun{}, err
	}
	e.Attempt.RunID = e.RunID
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		i, err := readWorkflowEffectIntent(ctx, conn, e.RunID, e.IntentSeq)
		if err != nil {
			return err
		}
		if i.State() != WorkflowEffectPending || i.AttemptState != WorkflowAttemptRunning || i.LeaseToken != lease.Token {
			return fmt.Errorf("%w: Effect Intent #%d is %s", ErrWorkflowRunConflict, i.Seq, i.State())
		}
		e.Attempt.Seq = i.AttemptSeq
		if err := validAttemptEnd(lease, e.Attempt); err != nil {
			return err
		}
		if err := insertReceipt(ctx, conn, e.RunID, i.Seq, WorkflowEffectReceipt{Outcome: e.Outcome, Response: e.Response, Error: e.Error,
			Source: WorkflowEffectFromProvider, Actor: "runtime", LeaseHolder: lease.Holder, LeaseToken: lease.Token}, now); err != nil {
			return err
		}
		return finishWorkflowAttempt(ctx, conn, lease, e.Attempt, now)
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, e.RunID)
}

// WorkflowEffectUnknownMark is an intent whose call ended without a
// provable outcome: why (Reason), what the provider answered if anything
// (Response), the run's error while it needs reconciliation (RunError;
// Reason when empty), and how long automatic reconciliation waits first
// (Settle), so a request that is still landing is not looked up too early.
type WorkflowEffectUnknownMark struct {
	RunID     string
	IntentSeq int64
	Reason    string
	Response  json.RawMessage
	RunError  string
	Settle    time.Duration
}

// markUnknown records an intent's outcome unknown and moves its run to
// needs_reconciliation: due for automatic reconciliation after settle when
// the provider reconciles, otherwise waiting for the owner. In the same
// transaction the unknown outcome is presented to the owner as an
// effect_reconciliation interruption (sub-stage 7), which only the
// intent's receipt closes.
func markUnknown(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, i WorkflowEffectIntent, reason string, response json.RawMessage, runError string, now time.Time, settle time.Duration) error {
	run, err := readWorkflowRun(ctx, conn, i.RunID)
	if err != nil {
		return err
	}
	row, err := effectInterruption(i, reason)
	if err != nil {
		return err
	}
	if _, err := openInterruption(ctx, conn, lease, row, now); err != nil {
		return err
	}
	var resp any
	if len(response) > 0 {
		resp = string(response)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_effect_unknowns(run_id,intent_seq,reason,response,lease_holder,lease_token,recorded_at) VALUES(?,?,?,?,?,?,?)`,
		i.RunID, i.Seq, reason, resp, lease.Holder, lease.Token, formatTime(now)); err != nil {
		return fmt.Errorf("record unknown effect outcome: %w", err)
	}
	var notBefore any
	if i.Reconcile == WorkflowEffectReconcileByProvider {
		notBefore = formatLeaseTime(now.Add(settle))
	}
	// A run the owner is cancelling stays cancelling: it finishes once the
	// effect's receipt is recorded, never by retrying it (sub-stage 8).
	state := WorkflowRunNeedsReconciliation
	if run.State == WorkflowRunCancelling {
		state, runError = WorkflowRunCancelling, clipText(cancellingUnknownError(runError), maxEffectText)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE workflow_runs SET state=?,error=?,not_before=?,updated_at=? WHERE id=?`,
		string(state), runError, notBefore, formatTime(now), i.RunID); err != nil {
		return fmt.Errorf("record workflow run needing reconciliation: %w", err)
	}
	return nil
}

// ReconcileHint is how the owner reconciles an unknown effect.
func ReconcileHint(runID, step string) string {
	return fmt.Sprintf("reconcile it with: evie workflows reconcile %s %s --outcome happened|did-not-happen --evidence <text>", runID, step)
}

// MarkWorkflowEffectUnknown records, under the run's current lease, that
// an intent's call ended without a provable outcome: its attempt fails
// with the reason, the outcome is recorded unknown, and the run needs
// reconciliation (due for automatic reconciliation at once when its
// provider reconciles). Its Resource Conflict Keys stay held.
func (s *Store) MarkWorkflowEffectUnknown(ctx context.Context, lease WorkflowRunLease, m WorkflowEffectUnknownMark) (WorkflowRun, error) {
	if strings.TrimSpace(m.Reason) == "" || len(m.Response) > 0 && !json.Valid(m.Response) || m.Settle < 0 {
		return WorkflowRun{}, errors.New("an unknown effect outcome needs its reason and any response as JSON")
	}
	if m.RunError == "" {
		m.RunError = m.Reason
	}
	if err := errors.Join(validEffectText(m.Reason), validEffectText(m.RunError)); err != nil {
		return WorkflowRun{}, err
	}
	if m.RunID != lease.RunID {
		return WorkflowRun{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		i, err := readWorkflowEffectIntent(ctx, conn, m.RunID, m.IntentSeq)
		if err != nil {
			return err
		}
		if i.State() != WorkflowEffectPending || i.AttemptState != WorkflowAttemptRunning || i.LeaseToken != lease.Token {
			return fmt.Errorf("%w: Effect Intent #%d is %s", ErrWorkflowRunConflict, i.Seq, i.State())
		}
		if err := finishWorkflowAttempt(ctx, conn, lease, WorkflowAttemptEnd{RunID: m.RunID, Seq: i.AttemptSeq, Error: "outcome unknown: " + m.Reason}, now); err != nil {
			return err
		}
		return markUnknown(ctx, conn, lease, i, m.Reason, m.Response, m.RunError, now, m.Settle)
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, m.RunID)
}

// markOrphanedEffectsUnknown records, inside a claim, the outcome of every
// Effect Intent of the run whose attempt ended without a receipt or an
// unknown outcome as unknown: its runtime stopped after the intent and
// before the receipt (the claim has just recorded that attempt
// interrupted, or an Evie that predates the effect protocol did). The run
// then needs reconciliation, whatever it was waiting for.
func markOrphanedEffectsUnknown(ctx context.Context, conn *sql.Conn, lease WorkflowRunLease, now time.Time) error {
	rows, err := conn.QueryContext(ctx, `SELECT i.seq FROM workflow_effect_intents i JOIN workflow_node_attempts a ON a.run_id=i.run_id AND a.seq=i.attempt_seq
 WHERE i.run_id=? AND a.state<>'running'
 AND NOT EXISTS(SELECT 1 FROM workflow_effect_receipts r WHERE r.run_id=i.run_id AND r.intent_seq=i.seq)
 AND NOT EXISTS(SELECT 1 FROM workflow_effect_unknowns u WHERE u.run_id=i.run_id AND u.intent_seq=i.seq) ORDER BY i.seq`, lease.RunID)
	if err != nil {
		return err
	}
	var orphans []int64
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			rows.Close()
			return err
		}
		orphans = append(orphans, seq)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, seq := range orphans {
		i, err := readWorkflowEffectIntent(ctx, conn, lease.RunID, seq)
		if err != nil {
			return err
		}
		reason := fmt.Sprintf("steps.%s: holder %s (lease %d) stopped after recording Effect Intent #%d (%s on %s %s, idempotency key %s) and before its receipt, so whether the effect happened is unknown",
			i.Step, i.LeaseHolder, i.LeaseToken, i.Seq, i.Capability, i.Connection, i.Resource, i.IdempotencyKey)
		runError := reason + "; it is never retried by policy: Evie reconciles it with the provider before the run continues"
		if i.Reconcile == WorkflowEffectReconcileByOwner {
			runError = reason + "; the provider cannot reconcile it, so " + ReconcileHint(i.RunID, i.Step)
		}
		// Its holder stopped at least a lease ago: reconciliation may ask
		// at once.
		if err := markUnknown(ctx, conn, lease, i, reason, nil, runError, now, 0); err != nil {
			return err
		}
	}
	return nil
}

// WorkflowEffectResolution reconciles an intent whose outcome is unknown:
// Outcome succeeded (it happened, Response its output) or failed (it did
// not), from the provider's reconciliation or the owner (Source), with the
// evidence and, for the owner, the surface and the session the owner
// answered from, if any. Answer is the owner's answer document as given
// (sub-stage 7); when empty it is derived from the outcome, evidence and
// output.
type WorkflowEffectResolution struct {
	RunID     string
	IntentSeq int64
	Outcome   WorkflowEffectState
	Response  json.RawMessage
	Error     string
	Source    WorkflowEffectSource
	Evidence  string
	Surface   string
	SessionID memory.SessionID
	Answer    json.RawMessage
}

// ResolveWorkflowEffect records, under the run's current lease, the
// immutable receipt that reconciles an unknown intent, releases its
// Resource Conflict Keys and lets the run continue: it is running again
// from its latest checkpoint, where the runtime continues from the receipt
// (the step's output when it happened; a retry within the Retry Policy, or
// the run's failure, when it did not); a run the owner is cancelling stays
// cancelling, and its holder then finishes the cancellation (sub-stage 8).
// In the same transaction it closes the intent's effect_reconciliation
// interruption (sub-stage 7): answered, with the owner's answer, surface
// and session, or reconciled by the provider, so the reconciliation and
// the receipt are one record.
func (s *Store) ResolveWorkflowEffect(ctx context.Context, lease WorkflowRunLease, r WorkflowEffectResolution) (WorkflowRun, error) {
	if r.Source != WorkflowEffectFromOwner && r.SessionID != "" {
		return WorkflowRun{}, errors.New("only the owner answers from a session")
	}
	if len(r.Answer) > 0 && !validInterruptionJSON(r.Answer) {
		return WorkflowRun{}, errors.New("a reconciliation's answer is one JSON object")
	}
	receipt := WorkflowEffectReceipt{Outcome: r.Outcome, Response: r.Response, Error: r.Error, Source: r.Source, Evidence: r.Evidence,
		Actor: "runtime", LeaseHolder: lease.Holder, LeaseToken: lease.Token}
	switch {
	case r.Outcome != WorkflowEffectSucceeded && r.Outcome != WorkflowEffectFailed:
		return WorkflowRun{}, errors.New("a reconciliation finds that the effect happened (succeeded) or did not (failed)")
	case r.Outcome == WorkflowEffectSucceeded && (len(r.Response) == 0 || !json.Valid(r.Response)):
		return WorkflowRun{}, errors.New("an effect reconciled as happened needs its output")
	case r.Outcome == WorkflowEffectFailed && len(r.Response) > 0:
		return WorkflowRun{}, errors.New("an effect that did not happen has no output")
	case r.Source == WorkflowEffectFromOwner:
		if r.Surface != "cli" && r.Surface != "web" {
			return WorkflowRun{}, ErrWorkflowApprover
		}
		receipt.Actor, receipt.Surface = "owner", r.Surface
	case r.Source != WorkflowEffectFromReconcile:
		return WorkflowRun{}, errors.New("a reconciliation comes from the provider's reconciliation or the owner")
	}
	if err := errors.Join(validEffectText(r.Error), validEffectText(r.Evidence)); err != nil {
		return WorkflowRun{}, err
	}
	if r.RunID != lease.RunID {
		return WorkflowRun{}, fmt.Errorf("%w: the lease is for run %s", ErrWorkflowRunLeaseLost, lease.RunID)
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, r.RunID)
		if err != nil {
			return err
		}
		i, err := readWorkflowEffectIntent(ctx, conn, r.RunID, r.IntentSeq)
		if err != nil {
			return err
		}
		if i.State() != WorkflowEffectUnknown || run.State != WorkflowRunNeedsReconciliation && run.State != WorkflowRunCancelling {
			return fmt.Errorf("%w: Effect Intent #%d is %s and the run is %s", ErrWorkflowEffectNotUnknown, i.Seq, i.State(), run.State)
		}
		if err := requireWorkspaceSession(ctx, conn, run, r.SessionID); err != nil {
			return err
		}
		interruption, err := effectInterruptionSeq(ctx, conn, lease, i, now)
		if err != nil {
			return err
		}
		answer := r.Answer
		if len(answer) == 0 {
			if answer, err = effectAnswer(r.Outcome, r.Evidence, r.Response); err != nil {
				return err
			}
		}
		closed := resolutionRow{kind: WorkflowInterruptionReconciled, answer: answer, actor: "runtime"}
		if r.Source == WorkflowEffectFromOwner {
			closed = resolutionRow{kind: WorkflowInterruptionAnswered, answer: answer, actor: "owner", surface: r.Surface, session: r.SessionID}
		}
		if err := resolveInterruption(ctx, conn, lease, interruption, closed, now); err != nil {
			return err
		}
		if err := insertReceipt(ctx, conn, r.RunID, i.Seq, receipt, now); err != nil {
			return err
		}
		// The run continues only once none of its intents is unresolved.
		switch err := requireEffectsResolved(ctx, conn, r.RunID); {
		case errors.Is(err, ErrWorkflowEffectUnresolved):
			return nil
		case err != nil:
			return err
		}
		if run.State == WorkflowRunCancelling {
			c, _, err := readWorkflowRunCancellation(ctx, conn, r.RunID)
			if err != nil {
				return err
			}
			_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET error=?,not_before=NULL,updated_at=? WHERE id=?`, cancellingError(c), formatTime(now), r.RunID)
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET state='running',error='',not_before=NULL,updated_at=? WHERE id=?`, formatTime(now), r.RunID)
		return err
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, r.RunID)
}

// DeferWorkflowReconciliation records, under the run's current lease, why
// a run that needs reconciliation is not reconciled now (reason, its
// error) and when automatic reconciliation is tried again: after recheck,
// or never (recheck 0) when only the owner can reconcile it.
func (s *Store) DeferWorkflowReconciliation(ctx context.Context, lease WorkflowRunLease, reason string, recheck time.Duration) (WorkflowRun, error) {
	if strings.TrimSpace(reason) == "" || recheck < 0 {
		return WorkflowRun{}, errors.New("a deferred reconciliation needs its reason")
	}
	if err := validEffectText(reason); err != nil {
		return WorkflowRun{}, err
	}
	err := s.withWorkflowRunLease(ctx, lease, func(conn *sql.Conn, now time.Time) error {
		run, err := runRunnable(ctx, conn, lease.RunID)
		if err != nil {
			return err
		}
		if run.State != WorkflowRunNeedsReconciliation && run.State != WorkflowRunCancelling {
			return fmt.Errorf("%w: the run is %s", ErrWorkflowRunConflict, run.State)
		}
		if run.State == WorkflowRunCancelling {
			if err := requireEffectsResolved(ctx, conn, run.ID); !errors.Is(err, ErrWorkflowEffectUnresolved) {
				return fmt.Errorf("%w: the cancelling run has no effect to reconcile", ErrWorkflowRunConflict)
			}
			reason = clipText(cancellingUnknownError(reason), maxEffectText)
		}
		var notBefore any
		if recheck > 0 {
			notBefore = formatLeaseTime(now.Add(recheck))
		}
		_, err = conn.ExecContext(ctx, `UPDATE workflow_runs SET error=?,not_before=?,updated_at=? WHERE id=?`, reason, notBefore, formatTime(now), lease.RunID)
		return err
	})
	if err != nil {
		return WorkflowRun{}, err
	}
	return readWorkflowRun(ctx, s.db, lease.RunID)
}
