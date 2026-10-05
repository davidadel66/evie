package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/workflow"
)

// Stage 14 sub-stage 12: run records stay immutable to everything but the
// retention sweep, and the database itself bounds what the sweep may
// change: only inside a transaction holding the retention guard, only a
// finished run's recorded values (never an identity, state or time), only
// intermediate checkpoints, only read notices of finished runs, only
// skipped or failed slots that are not their schedule's latest and only
// schedule controls a later one replaced.

// guarded runs statements in one transaction holding the retention guard
// for purpose, and rolls it back; it returns the first error.
func guarded(t *testing.T, db *sql.DB, purpose string, statements ...string) error {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	defer conn.ExecContext(ctx, `ROLLBACK`)
	if purpose != "" {
		if _, err := conn.ExecContext(ctx, `INSERT INTO workflow_retention_guard(singleton,purpose) VALUES(1,?)`, purpose); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range statements {
		if _, err := conn.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func wantRefused(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("%s: got %v, want the record refused as immutable", what, err)
	}
}

func TestWorkflowRetentionGuardBoundsWhatTheSweepMayChange(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	done, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:done"))
	if err != nil {
		t.Fatal(err)
	}
	finishRun(t, r.s, done)
	open, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:open"))
	if err != nil {
		t.Fatal(err)
	}
	id := "'" + done.ID + "'"

	// Without the guard nothing changes, as before.
	wantRefused(t, guarded(t, r.db, "", `UPDATE workflow_runs SET inputs='{}' WHERE id=`+id), "a finished run's input")
	wantRefused(t, guarded(t, r.db, "", `UPDATE workflow_node_attempts SET output='"x"' WHERE run_id=`+id), "a finished attempt's output")
	wantRefused(t, guarded(t, r.db, "", `UPDATE workflow_run_checkpoints SET state='{}' WHERE run_id=`+id), "a checkpoint")
	wantRefused(t, guarded(t, r.db, "", `DELETE FROM workflow_notifications WHERE run_id=`+id), "a notice")

	// Under the redaction guard a finished run's values change, but never
	// its state, times or identity, and an unfinished run not at all.
	if err := guarded(t, r.db, WorkflowRetentionRedact,
		`UPDATE workflow_runs SET inputs='{"bill_id":"[redacted]"}',outputs='{"x":"[redacted]"}',error='[redacted]' WHERE id=`+id,
		`UPDATE workflow_node_attempts SET input='{}',output='"[redacted]"' WHERE run_id=`+id,
		`UPDATE workflow_run_checkpoints SET state='{"split":"[redacted]"}' WHERE run_id=`+id,
		`UPDATE workflow_notifications SET detail='[redacted]' WHERE run_id=`+id); err != nil {
		t.Fatalf("redaction under the guard: %v", err)
	}
	wantRefused(t, guarded(t, r.db, WorkflowRetentionRedact, `UPDATE workflow_runs SET state='failed',error='x' WHERE id=`+id), "a finished run's state under the guard")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionRedact, `UPDATE workflow_runs SET run_key='other' WHERE id=`+id), "a finished run's Run Key under the guard")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionRedact, `UPDATE workflow_node_attempts SET finished_at='2026-01-01T00:00:00Z' WHERE run_id=`+id), "an attempt's time under the guard")
	if err := guarded(t, r.db, WorkflowRetentionRedact, `UPDATE workflow_runs SET inputs='{}' WHERE id='`+open.ID+`'`); err == nil {
		t.Fatal("an unfinished run's input was redacted")
	}
	// The wrong purpose opens nothing.
	wantRefused(t, guarded(t, r.db, WorkflowRetentionDelete, `UPDATE workflow_node_attempts SET output='"x"' WHERE run_id=`+id), "an attempt under the deletion guard")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionRedact, `DELETE FROM workflow_node_attempts WHERE run_id=`+id), "deleting an attempt")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionDelete, `DELETE FROM workflow_runs WHERE id=`+id), "deleting a run")

	// Compaction empties only intermediate checkpoints: never the first or
	// the last (this run has only those two).
	wantRefused(t, guarded(t, r.db, WorkflowRetentionCompact, `UPDATE workflow_run_checkpoints SET state='{}',compacted_at='x' WHERE run_id=`+id+` AND seq=1`), "the final checkpoint")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionCompact, `UPDATE workflow_run_checkpoints SET state='{}',compacted_at='x' WHERE run_id=`+id+` AND seq=0`), "the first checkpoint")

	// Deletion takes only read notices.
	wantRefused(t, guarded(t, r.db, WorkflowRetentionDelete, `DELETE FROM workflow_notifications WHERE run_id=`+id), "an unread notice")
	if _, err := r.s.MarkAllWorkflowNotificationsRead(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := guarded(t, r.db, WorkflowRetentionDelete, `DELETE FROM workflow_notifications WHERE run_id=`+id); err != nil {
		t.Fatalf("a read notice of a finished run: %v", err)
	}

	// No guard survives a transaction, and every open removes one a defect
	// might have left.
	if _, err := r.db.ExecContext(ctx, `INSERT INTO workflow_retention_guard(singleton,purpose) VALUES(1,'redact')`); err != nil {
		t.Fatal(err)
	}
	r.reopen()
	var left int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workflow_retention_guard`).Scan(&left); err != nil || left != 0 {
		t.Fatalf("guards after a reopen = %d, %v", left, err)
	}
	wantRefused(t, guarded(t, r.db, "", `UPDATE workflow_runs SET inputs='{}' WHERE id=`+id), "a finished run's input after the reopen")
}

func TestWorkflowRetentionGuardKeepsTheLatestSlotAndStartedSlots(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	a := notesActivation(t, r.s)
	if _, _, err := r.s.StartWorkflowRun(ctx, scheduledStart(a, "2026-10-05", "holder-a")); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"2026-10-06T07:00", "2026-10-07T07:00"} {
		clock.Advance(time.Hour)
		due, _ := time.Parse("2006-01-02T15:04", key)
		if err := r.s.RecordWorkflowScheduleSlot(ctx, WorkflowScheduleSlot{WorkspaceID: a.WorkspaceID, Workflow: a.Name, Version: a.Version, Timezone: "America/New_York",
			Key: key, Due: due, Outcome: WorkflowSlotSkipped, Detail: "the Evie was off", DecidedBy: "holder-a"}); err != nil {
			t.Fatal(err)
		}
	}
	wantRefused(t, guarded(t, r.db, "", `DELETE FROM workflow_schedule_slots WHERE slot_key='2026-10-06T07:00'`), "a slot without the guard")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionDelete, `DELETE FROM workflow_schedule_slots WHERE outcome='started'`), "a started slot")
	wantRefused(t, guarded(t, r.db, WorkflowRetentionDelete, `DELETE FROM workflow_schedule_slots WHERE slot_key='2026-10-07T07:00'`), "the schedule's latest slot")
	if err := guarded(t, r.db, WorkflowRetentionDelete, `DELETE FROM workflow_schedule_slots WHERE slot_key='2026-10-06T07:00'`); err != nil {
		t.Fatalf("an earlier skipped slot: %v", err)
	}
	wantRefused(t, guarded(t, r.db, WorkflowRetentionRedact, `UPDATE workflow_schedule_slots SET outcome='failed' WHERE slot_key='2026-10-06T07:00'`), "a slot's outcome")
}

// A sub-stage 11 database is upgraded once: the run table gains the
// superseded state with every row kept, and the immutability triggers
// their guarded forms.
func TestWorkflowRunsTableUpgradesForMigrationAndRetention(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := rebuildWorkflowRunsWith(ctx, r.db, [][2]string{{workflowRunStatesSuperseded, workflowRunStatesBefore}, {workflowRunFinished, workflowRunFinishedBefore}}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`DROP TRIGGER workflow_runs_superseded_by_migration`, `DROP TABLE workflow_run_migrations`,
		`DROP TRIGGER workflow_runs_terminal`, `CREATE TRIGGER workflow_runs_terminal BEFORE UPDATE ON workflow_runs
 WHEN OLD.state IN ('succeeded','failed','cancelled') BEGIN SELECT RAISE(ABORT,'terminal workflow runs are immutable'); END`,
		`DROP INDEX workflow_runs_one_per_key`, `CREATE UNIQUE INDEX workflow_runs_one_per_key ON workflow_runs(workspace_id, workflow, run_key) WHERE state NOT IN ('failed','cancelled')`} {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET state='superseded' WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "CHECK") {
		t.Fatalf("the sub-stage 11 table accepted superseded: %v", err)
	}
	r.reopen()
	var definition, index, trigger string
	if err := r.db.QueryRowContext(ctx, `SELECT (SELECT sql FROM sqlite_master WHERE name='workflow_runs'),(SELECT sql FROM sqlite_master WHERE name='workflow_runs_one_per_key'),
 (SELECT sql FROM sqlite_master WHERE name='workflow_runs_terminal')`).Scan(&definition, &index, &trigger); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, workflowRunStatesSuperseded) || !strings.Contains(definition, workflowRunFinished) || !strings.Contains(index, "superseded") ||
		!strings.Contains(trigger, "workflow_retention_guard") {
		t.Fatalf("after the upgrade:\n%s\n%s\n%s", definition, index, trigger)
	}
	if got, err := r.s.WorkflowRun(ctx, run.ID); err != nil || got.Run.State != WorkflowRunQueued {
		t.Fatalf("the run after the upgrade = %+v, %v", got.Run, err)
	}
	// Only a recorded migration supersedes a run.
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET state='superseded',finished_at=updated_at WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "migration") {
		t.Fatalf("superseded without a migration: %v", err)
	}
}

func TestWorkflowRedactionRewritesARunsValuesOnceWithATombstone(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	start := runStart(a, "split-bill:secret")
	start.Inputs = json.RawMessage(`{"bill_id":"bill-for-alice","total_cents":1000,"people":3}`)
	run, _, err := r.s.StartWorkflowRun(ctx, start)
	if err != nil {
		t.Fatal(err)
	}
	lease := testLease(t, r.s, run.ID)
	at, err := r.s.BeginWorkflowAttempt(ctx, lease, WorkflowAttemptStart{RunID: run.ID, Checkpoint: 0, Step: "split", Kind: workflow.KindDeterministic, Attempt: 1,
		Input: json.RawMessage(`{"label":"bill-for-alice","total":1000}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: at.Seq, Output: json.RawMessage(`{"note":"split bill-for-alice"}`),
		State: json.RawMessage(`{"split":{"note":"split bill-for-alice"}}`), Outputs: json.RawMessage(`{"bill":"bill-for-alice","shares":[334]}`)}); err != nil {
		t.Fatal(err)
	}
	sensitivity := workflow.Sensitivity{Classification: workflow.ClassificationOrdinary, Inputs: []string{"bill_id"}, StepArgs: map[string][]string{"split": {"label"}}, Outputs: []string{"bill"}}
	req := WorkflowRetentionRun{RunID: run.ID, Sensitivity: sensitivity, SweepID: "sweep-1", Policy: "ordinary: redact sensitive inputs 30d after the run ended",
		Reason: "retention policy", Actor: "runtime", FinishedBefore: clock.Now()}
	// Inside the window nothing happens.
	early := req
	early.FinishedBefore = clock.Now().Add(-time.Hour)
	var skip *WorkflowRetentionSkip
	if _, err := r.s.RedactWorkflowRun(ctx, early); !errors.As(err, &skip) || !strings.Contains(skip.Reason, "window") {
		t.Fatalf("inside the window: %v", err)
	}
	before, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	dry := req
	dry.DryRun = true
	planned, err := r.s.RedactWorkflowRun(ctx, dry)
	if err != nil || planned == nil || !planned.DryRun || planned.Records == 0 {
		t.Fatalf("dry run = %+v, %v", planned, err)
	}
	if after, _ := r.s.WorkflowRun(ctx, run.ID); string(after.Run.Inputs) != string(before.Run.Inputs) || len(after.Retention) != 0 {
		t.Fatalf("a dry run changed the run: %s %+v", after.Run.Inputs, after.Retention)
	}
	tomb, err := r.s.RedactWorkflowRun(ctx, req)
	if err != nil || tomb == nil || tomb.ID == 0 || tomb.Action != WorkflowRetentionRedact || tomb.DataClass != WorkflowDataSensitiveInputs || tomb.RunID != run.ID ||
		tomb.Records != planned.Records || tomb.Fields["workflow_runs"] != 1 || tomb.Fields["workflow_node_attempts"] != 1 || tomb.Fields["workflow_run_checkpoints"] != 1 {
		t.Fatalf("tombstone = %+v, %v", tomb, err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw), "bill-for-alice") {
		t.Fatalf("the sensitive value survived redaction: %s", raw)
	}
	if string(d.Run.Inputs) != `{"bill_id":"[redacted]","people":3,"total_cents":1000}` || string(d.Attempts[0].Input) != `{"label":"[redacted]","total":1000}` ||
		string(d.Run.Outputs) != `{"bill":"[redacted]","shares":[334]}` || d.Run.State != WorkflowRunSucceeded || d.Run.RunKey != run.RunKey {
		t.Fatalf("redacted run = %s %s %s %s", d.Run.Inputs, d.Attempts[0].Input, d.Run.Outputs, d.Run.State)
	}
	if len(d.Retention) != 1 || d.Retention[0].ID != tomb.ID {
		t.Fatalf("the run's retention = %+v", d.Retention)
	}
	// Once is enough.
	if again, err := r.s.RedactWorkflowRun(ctx, req); err != nil || again != nil {
		t.Fatalf("a second redaction = %+v, %v", again, err)
	}
}
