package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Workflow retention (Stage 14 sub-stage 12 of
// docs/memory-architecture.md, 2026-10-04; ADR 0045: "Evie retains a
// complete local audit trail by default ... Workflow Definitions classify
// sensitive fields for redaction, and each Workspace may define retention
// and export rules. Deletion is an explicit, auditable operation rather
// than an incidental consequence of ordinary cleanup"). Every record of a
// run stays immutable to everything but the retention sweep, and the sweep
// changes it only inside a transaction that opens the retention guard and
// records a tombstone:
//
//   - workflow_retention_guard: a row that exists only inside a retention
//     transaction (it is removed before the transaction commits, and at
//     every startup in case of a defect), naming its purpose: redact,
//     compact or delete. The immutability triggers of run records admit an
//     update or deletion only while it is open and only of what that
//     purpose may change: redaction rewrites a finished run's recorded
//     values and texts, never an identity, state, time, receipt outcome or
//     authority evaluation; compaction empties the state of a finished
//     run's intermediate checkpoints (never its first or last); deletion
//     removes read notices of finished runs, schedule slots that were
//     skipped or failed and are not their schedule's latest, and schedule
//     controls a later one replaced. Runs, attempts, checkpoints, Effect
//     Intents, receipts, unknown outcomes, interruptions, answers, model
//     and worker calls, cancellations, overrides, migrations and started
//     slots are never deleted.
//   - workflow_retention_tombstones: what each retention action removed,
//     when, why (the policy and reason), by whom (the periodic sweep or the
//     owner's command) and from which Workspace, workflow and run, with how
//     many records and bytes. Immutable.
//   - workflow_retention_state: the sweep's single cross-process runner
//     lease and its latest summary.

// Retention actions and the classes of data they remove.
const (
	WorkflowRetentionRedact  = "redact"
	WorkflowRetentionCompact = "compact"
	WorkflowRetentionDelete  = "delete"

	WorkflowDataSensitiveInputs  = "sensitive_inputs"
	WorkflowDataRunValues        = "run_values"
	WorkflowDataCheckpointStates = "checkpoint_states"
	WorkflowDataNotifications    = "notifications"
	WorkflowDataScheduleSlots    = "schedule_slots"
	WorkflowDataScheduleControls = "schedule_controls"
)

// errRetentionDryRun rolls back a dry run's transaction.
var errRetentionDryRun = errors.New("retention dry run")

// WorkflowRetentionTombstone is one recorded retention action.
type WorkflowRetentionTombstone struct {
	ID             int64      `json:"id,omitempty"`
	SweepID        string     `json:"sweep_id"`
	Action         string     `json:"action"`
	DataClass      string     `json:"data_class"`
	Classification string     `json:"classification"`
	WorkspaceID    string     `json:"workspace_id"`
	Workflow       string     `json:"workflow"`
	RunID          string     `json:"run_id,omitempty"`
	Records        int        `json:"records"`
	Bytes          int64      `json:"bytes"`
	OldestAt       *time.Time `json:"oldest_at,omitempty"`
	NewestAt       *time.Time `json:"newest_at,omitempty"`
	Policy         string     `json:"policy"`
	Reason         string     `json:"reason"`
	Actor          string     `json:"actor"`
	Surface        string     `json:"surface,omitempty"`
	RecordedAt     time.Time  `json:"recorded_at"`
	// Fields counts the rewritten or removed records by table; DryRun
	// marks a tombstone a dry run reported and did not record.
	Fields map[string]int `json:"fields,omitempty"`
	DryRun bool           `json:"dry_run,omitempty"`
}

// guardCondition is the open retention guard of one purpose.
func guardCondition(purpose string) string {
	return `EXISTS(SELECT 1 FROM workflow_retention_guard WHERE purpose='` + purpose + `')`
}

// unchanged is every column's NEW value equal to its OLD one.
func unchanged(columns ...string) string {
	parts := make([]string, len(columns))
	for i, c := range columns {
		parts[i] = "NEW." + c + " IS OLD." + c
	}
	return strings.Join(parts, " AND ")
}

// except is columns without the given ones.
func except(columns []string, out ...string) []string {
	var kept []string
	for _, c := range columns {
		if !slices.Contains(out, c) {
			kept = append(kept, c)
		}
	}
	return kept
}

const runFinished = `EXISTS(SELECT 1 FROM workflow_runs r WHERE r.id=OLD.run_id AND r.state IN (` + terminalWorkflowStates + `))`

var (
	runColumnsAll          = []string{"id", "workspace_id", "workflow", "version", "commit_id", "tree_path", "executable_hash", "authority_hash", "workspace_revision_id", "access_revision", "composition", "run_key", "inputs", "state", "outputs", "error", "origin_surface", "session_id", "created_at", "started_at", "finished_at", "updated_at", "not_before"}
	runPinnedColumns       = []string{"id", "workspace_id", "workflow", "version", "commit_id", "tree_path", "executable_hash", "authority_hash", "workspace_revision_id", "access_revision", "composition", "run_key", "inputs", "origin_surface", "session_id", "created_at"}
	attemptColumnsAll      = []string{"run_id", "seq", "step_id", "kind", "attempt", "input_checkpoint", "input", "state", "output", "error", "next_step", "note", "started_at", "finished_at", "lease_holder", "lease_token", "item_index", "map_attempt"}
	attemptIdentity        = []string{"run_id", "seq", "step_id", "kind", "attempt", "input_checkpoint", "input", "started_at", "lease_holder", "lease_token", "item_index", "map_attempt"}
	eventColumns           = []string{"run_id", "seq", "kind", "holder_id", "fencing_token", "attempt_seq", "detail", "created_at"}
	intentColumns          = []string{"run_id", "seq", "attempt_seq", "attempt", "workspace_id", "step_id", "checkpoint", "capability", "contract", "schema_sha256", "plugin", "plugin_version", "connection", "resource", "idempotency_key", "idempotent", "reconcile", "conflict_keys", "input", "authority", "lease_holder", "lease_token", "created_at"}
	unknownColumns         = []string{"run_id", "intent_seq", "reason", "response", "lease_holder", "lease_token", "recorded_at"}
	receiptColumns         = []string{"run_id", "intent_seq", "outcome", "response", "error", "source", "evidence", "actor", "surface", "lease_holder", "lease_token", "recorded_at"}
	interruptionColumnsAll = []string{"run_id", "seq", "kind", "step_id", "checkpoint", "intent_seq", "message", "request", "response_schema", "session_id", "lease_holder", "lease_token", "opened_at"}
	resolutionColumns      = []string{"run_id", "interruption_seq", "resolution", "answer", "actor", "surface", "session_id", "detail", "lease_holder", "lease_token", "recorded_at"}
	modelCallColumnsAll    = []string{"run_id", "attempt_seq", "step_id", "policy", "policy_version", "provider", "model", "parameters", "prompt_sha256", "schema_sha256", "request_sha256", "estimated_input_tokens", "estimated_output_tokens", "estimated_micro_usd", "lease_holder", "lease_token", "started_at", "finished_at", "requests", "input_tokens", "output_tokens", "usage_reported", "micro_usd", "pricing_source", "response", "error"}
	workerCallColumnsAll   = []string{"run_id", "attempt_seq", "step_id", "item_index", "worker", "policy", "policy_version", "provider", "model", "objective_sha256", "schema_sha256", "budget_ms", "budget_tokens", "input_per_million", "output_per_million", "pricing_source", "estimated_micro_usd", "adopted_from", "continues", "execution_id", "child_session_id", "lease_holder", "lease_token", "started_at", "finished_at", "status", "reason", "input_tokens", "output_tokens", "usage_complete", "counted_tokens", "micro_usd"}
	notificationContent    = []string{"id", "dedup_key", "workspace_id", "workflow", "run_id", "kind", "run_state", "detail", "origin_surface", "session_id", "created_at"}
	slotColumnsAll         = []string{"workspace_id", "workflow", "version", "timezone", "slot_key", "due_at", "shifted", "outcome", "run_id", "detail", "decided_by", "decided_at"}
)

// redactable is the guarded form of a run-scoped table's no-update
// trigger: under the redaction guard, a finished run's record may change
// only in the given columns.
func redactable(trigger, table, message string, all []string, columns ...string) string {
	return fmt.Sprintf(`CREATE TRIGGER %s BEFORE UPDATE ON %s
 WHEN NOT (%s AND %s AND %s)
 BEGIN SELECT RAISE(ABORT,'%s'); END`, trigger, table, guardCondition(WorkflowRetentionRedact), runFinished, unchanged(except(all, columns...)...), message)
}

// workflowRetentionTriggers are the guarded immutability triggers (and the
// superseded state's) that replace earlier definitions of the same names.
// Each is replaced when its stored definition lacks its marker.
func workflowRetentionTriggers() []struct{ name, kind, marker, sql string } {
	type entry = struct{ name, kind, marker, sql string }
	const guard = "workflow_retention_guard"
	return []entry{
		{"workflow_runs_terminal", "trigger", guard, `CREATE TRIGGER workflow_runs_terminal BEFORE UPDATE ON workflow_runs
 WHEN OLD.state IN (` + terminalWorkflowStates + `) AND NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND ` + unchanged(except(runColumnsAll, "inputs", "outputs", "error")...) + `)
 BEGIN SELECT RAISE(ABORT,'terminal workflow runs are immutable'); END`},
		{"workflow_runs_pinned", "trigger", guard, `CREATE TRIGGER workflow_runs_pinned BEFORE UPDATE OF ` + strings.Join(runPinnedColumns, ",") + ` ON workflow_runs
 WHEN NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND OLD.state IN (` + terminalWorkflowStates + `) AND ` + unchanged(except(runPinnedColumns, "inputs")...) + `)
 BEGIN SELECT RAISE(ABORT,'a workflow run''s pinned fields are immutable'); END`},
		{"workflow_runs_finish_after_workers", "trigger", "superseded", `CREATE TRIGGER workflow_runs_finish_after_workers BEFORE UPDATE OF state ON workflow_runs
 WHEN NEW.state IN (` + terminalWorkflowStates + `) AND EXISTS(SELECT 1 FROM workflow_worker_calls c WHERE c.run_id=NEW.id AND c.finished_at IS NULL)
 BEGIN SELECT RAISE(ABORT,'a workflow run finishes only after every worker it started ended and was recorded'); END`},
		{"workflow_runs_one_per_key", "index", "superseded", `CREATE UNIQUE INDEX workflow_runs_one_per_key ON workflow_runs(workspace_id, workflow, run_key)
 WHERE state NOT IN ('failed','cancelled','superseded')`},
		{"workflow_run_checkpoints_no_update", "trigger", guard, `CREATE TRIGGER workflow_run_checkpoints_no_update BEFORE UPDATE ON workflow_run_checkpoints
 WHEN NOT (` + unchanged("run_id", "seq", "after_attempt", "next_step", "created_at") + ` AND ` + runFinished + ` AND (
 ` + guardCondition(WorkflowRetentionRedact) + ` AND NEW.compacted_at IS OLD.compacted_at
 OR ` + guardCondition(WorkflowRetentionCompact) + ` AND OLD.compacted_at IS NULL AND NEW.compacted_at IS NOT NULL AND NEW.state = '{}' AND OLD.seq > 0
 AND OLD.seq < (SELECT MAX(k.seq) FROM workflow_run_checkpoints k WHERE k.run_id=OLD.run_id)))
 BEGIN SELECT RAISE(ABORT,'workflow checkpoints are immutable'); END`},
		{"workflow_node_attempts_finished", "trigger", guard, `CREATE TRIGGER workflow_node_attempts_finished BEFORE UPDATE ON workflow_node_attempts
 WHEN OLD.state <> 'running' AND NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND ` + runFinished + ` AND ` + unchanged(except(attemptColumnsAll, "input", "output", "error", "note")...) + `)
 BEGIN SELECT RAISE(ABORT,'finished workflow node attempts are immutable'); END`},
		{"workflow_node_attempts_identity", "trigger", guard, `CREATE TRIGGER workflow_node_attempts_identity BEFORE UPDATE OF ` + strings.Join(attemptIdentity, ",") + ` ON workflow_node_attempts
 WHEN NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND ` + runFinished + ` AND ` + unchanged(except(attemptIdentity, "input")...) + `)
 BEGIN SELECT RAISE(ABORT,'a workflow node attempt''s identity is immutable'); END`},
		{"workflow_run_events_no_update", "trigger", guard, redactable("workflow_run_events_no_update", "workflow_run_events", "workflow run events are immutable", eventColumns, "detail")},
		{"workflow_effect_intents_no_update", "trigger", guard, redactable("workflow_effect_intents_no_update", "workflow_effect_intents", "workflow Effect Intents are immutable", intentColumns, "input")},
		{"workflow_effect_unknowns_no_update", "trigger", guard, redactable("workflow_effect_unknowns_no_update", "workflow_effect_unknowns", "unknown workflow effect outcomes are immutable", unknownColumns, "reason", "response")},
		{"workflow_effect_receipts_no_update", "trigger", guard, redactable("workflow_effect_receipts_no_update", "workflow_effect_receipts", "workflow Effect Receipts are immutable", receiptColumns, "response", "error", "evidence")},
		{"workflow_interruptions_no_update", "trigger", guard, redactable("workflow_interruptions_no_update", "workflow_interruptions", "workflow interruptions are immutable", interruptionColumnsAll, "message", "request")},
		{"workflow_interruption_resolutions_no_update", "trigger", guard, redactable("workflow_interruption_resolutions_no_update", "workflow_interruption_resolutions", "workflow interruption resolutions are immutable", resolutionColumns, "answer", "detail")},
		{"workflow_model_calls_finished", "trigger", guard, `CREATE TRIGGER workflow_model_calls_finished BEFORE UPDATE ON workflow_model_calls
 WHEN OLD.finished_at IS NOT NULL AND NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND ` + runFinished + ` AND ` + unchanged(except(modelCallColumnsAll, "response", "error")...) + `)
 BEGIN SELECT RAISE(ABORT,'finished workflow model calls are immutable'); END`},
		{"workflow_worker_calls_finished", "trigger", guard, `CREATE TRIGGER workflow_worker_calls_finished BEFORE UPDATE ON workflow_worker_calls
 WHEN OLD.finished_at IS NOT NULL AND NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND ` + runFinished + ` AND ` + unchanged(except(workerCallColumnsAll, "reason")...) + `)
 BEGIN SELECT RAISE(ABORT,'finished workflow worker calls are immutable'); END`},
		{"workflow_notifications_content", "trigger", guard, `CREATE TRIGGER workflow_notifications_content BEFORE UPDATE OF ` + strings.Join(notificationContent, ",") + ` ON workflow_notifications
 WHEN NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND (OLD.run_id IS NULL OR ` + runFinished + `) AND ` + unchanged(except(notificationContent, "detail")...) + `)
 BEGIN SELECT RAISE(ABORT,'workflow notifications are immutable; only their read state changes'); END`},
		{"workflow_notifications_no_delete", "trigger", guard, `CREATE TRIGGER workflow_notifications_no_delete BEFORE DELETE ON workflow_notifications
 WHEN NOT (` + guardCondition(WorkflowRetentionDelete) + ` AND OLD.read_at IS NOT NULL AND (OLD.run_id IS NULL OR ` + runFinished + `))
 BEGIN SELECT RAISE(ABORT,'workflow notifications are immutable'); END`},
		{"workflow_schedule_slots_no_update", "trigger", guard, `CREATE TRIGGER workflow_schedule_slots_no_update BEFORE UPDATE ON workflow_schedule_slots
 WHEN NOT (` + guardCondition(WorkflowRetentionRedact) + ` AND ` + unchanged(except(slotColumnsAll, "detail")...) + `)
 BEGIN SELECT RAISE(ABORT,'workflow schedule slots are immutable'); END`},
		{"workflow_schedule_slots_no_delete", "trigger", guard, `CREATE TRIGGER workflow_schedule_slots_no_delete BEFORE DELETE ON workflow_schedule_slots
 WHEN NOT (` + guardCondition(WorkflowRetentionDelete) + ` AND OLD.outcome IN ('skipped','failed') AND EXISTS(SELECT 1 FROM workflow_schedule_slots s
 WHERE s.workspace_id=OLD.workspace_id AND s.workflow=OLD.workflow AND s.timezone=OLD.timezone AND (s.due_at > OLD.due_at OR s.due_at = OLD.due_at AND s.slot_key > OLD.slot_key)))
 BEGIN SELECT RAISE(ABORT,'workflow schedule slots are immutable'); END`},
		{"workflow_schedule_controls_no_delete", "trigger", guard, `CREATE TRIGGER workflow_schedule_controls_no_delete BEFORE DELETE ON workflow_schedule_controls
 WHEN NOT (` + guardCondition(WorkflowRetentionDelete) + ` AND EXISTS(SELECT 1 FROM workflow_schedule_controls c
 WHERE c.workspace_id=OLD.workspace_id AND c.workflow=OLD.workflow AND c.version=OLD.version AND c.seq > OLD.seq))
 BEGIN SELECT RAISE(ABORT,'workflow schedule controls are immutable'); END`},
	}
}

func ensureWorkflowRetentionSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS workflow_retention_guard (
 singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
 purpose TEXT NOT NULL CHECK(purpose IN ('redact','compact','delete')));
 CREATE TABLE IF NOT EXISTS workflow_retention_tombstones (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 sweep_id TEXT NOT NULL CHECK(length(sweep_id) BETWEEN 1 AND 128),
 action TEXT NOT NULL CHECK(action IN ('redact','compact','delete')),
 data_class TEXT NOT NULL CHECK(data_class IN ('sensitive_inputs','run_values','checkpoint_states','notifications','schedule_slots','schedule_controls')),
 classification TEXT NOT NULL CHECK(classification IN ('ordinary','sensitive')),
 workspace_id TEXT NOT NULL,
 workflow TEXT NOT NULL,
 run_id TEXT REFERENCES workflow_runs(id),
 records INTEGER NOT NULL CHECK(records > 0),
 bytes INTEGER NOT NULL CHECK(bytes >= 0),
 fields TEXT NOT NULL CHECK(json_valid(fields) AND json_type(fields) = 'object'),
 oldest_at TEXT,
 newest_at TEXT,
 policy TEXT NOT NULL CHECK(length(policy) BETWEEN 1 AND 1000),
 reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 1000),
 actor TEXT NOT NULL CHECK(actor IN ('runtime','owner')),
 surface TEXT NOT NULL CHECK(surface IN ('','cli','web')),
 recorded_at TEXT NOT NULL,
 CHECK((actor = 'owner') = (surface <> '')));
 CREATE INDEX IF NOT EXISTS workflow_retention_tombstones_by_run ON workflow_retention_tombstones(run_id, action) WHERE run_id IS NOT NULL;
 CREATE TRIGGER IF NOT EXISTS workflow_retention_tombstones_no_update BEFORE UPDATE ON workflow_retention_tombstones
 BEGIN SELECT RAISE(ABORT,'workflow retention tombstones are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS workflow_retention_tombstones_no_delete BEFORE DELETE ON workflow_retention_tombstones
 BEGIN SELECT RAISE(ABORT,'workflow retention tombstones are immutable'); END;
 CREATE TABLE IF NOT EXISTS workflow_retention_state (
 singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
 runner_holder TEXT,
 runner_expires_at TEXT,
 last_sweep_at TEXT,
 last_summary TEXT CHECK(last_summary IS NULL OR json_valid(last_summary)));
 INSERT OR IGNORE INTO workflow_retention_state(singleton) VALUES(1);`); err != nil {
		return fmt.Errorf("create workflow retention schema: %w", err)
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		// The guard exists only inside a retention transaction; one left by
		// a defect would open every run record to change.
		if _, err := conn.ExecContext(ctx, `DELETE FROM workflow_retention_guard`); err != nil {
			return err
		}
		var compacted bool
		if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pragma_table_info('workflow_run_checkpoints') WHERE name='compacted_at')`).Scan(&compacted); err != nil {
			return err
		}
		if !compacted {
			if _, err := conn.ExecContext(ctx, `ALTER TABLE workflow_run_checkpoints ADD COLUMN compacted_at TEXT`); err != nil {
				return fmt.Errorf("add the workflow checkpoint compaction time: %w", err)
			}
		}
		for _, t := range workflowRetentionTriggers() {
			var current string
			err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type=? AND name=?`, t.kind, t.name).Scan(&current)
			switch {
			case err == nil && strings.Contains(current, t.marker):
				continue
			case err == nil:
				if _, err := conn.ExecContext(ctx, `DROP `+strings.ToUpper(t.kind)+` `+t.name); err != nil {
					return fmt.Errorf("replace %s %s: %w", t.kind, t.name, err)
				}
			case !errors.Is(err, sql.ErrNoRows):
				return err
			}
			if _, err := conn.ExecContext(ctx, t.sql); err != nil {
				return fmt.Errorf("create %s %s: %w", t.kind, t.name, err)
			}
		}
		return nil
	})
}
