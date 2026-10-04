package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/workflow"
)

// The effect protocol's durable records (Stage 14 sub-stage 6): an Effect
// Intent is recorded with its node attempt under the fenced lease after
// per-node admission, the run's max_calls_per_run and the Resource
// Conflict Keys other runs hold; an intent without a receipt blocks every
// later node, pause, resume and failure of its run; a claim that finds one
// left in flight records its outcome unknown; reconciliation records an
// immutable receipt and lets the run continue.

// notesAtWrite starts a notes-digest run under a lease and advances it to
// its write step, the notes.append effect on conn-notes notebook:digest
// (at most one call per run).
func notesAtWrite(t *testing.T, s *Store) (WorkflowRun, WorkflowRunLease) {
	t.Helper()
	_, run := notesRun(t, s)
	claim, err := s.AcquireWorkflowRunLease(context.Background(), run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	advanceTo(t, s, claim.Lease, run.ID, []string{"read", "check", "summarize"}, []string{"check", "summarize", "write"})
	return run, claim.Lease
}

// advanceTo records one succeeded attempt of each step in turn.
func advanceTo(t *testing.T, s *Store, lease WorkflowRunLease, runID string, steps, next []string) {
	t.Helper()
	ctx := context.Background()
	state := map[string]any{}
	for i, step := range steps {
		attempt, err := s.BeginWorkflowAttempt(ctx, lease, WorkflowAttemptStart{RunID: runID, Checkpoint: int64(i), Step: step, Kind: workflow.KindDeterministic, Attempt: 1, Input: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		state[step] = map[string]any{}
		encoded, _ := json.Marshal(state)
		if _, err := s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: runID, Seq: attempt.Seq, Output: json.RawMessage(`{}`), Next: next[i], State: encoded}); err != nil {
			t.Fatal(err)
		}
	}
}

func writeEffect(runID string, checkpoint int64, attempt int, keys ...string) WorkflowEffectStart {
	if len(keys) == 0 {
		keys = []string{"conn-notes/notebook:digest"}
	}
	return WorkflowEffectStart{
		Attempt:    WorkflowAttemptStart{RunID: runID, Checkpoint: checkpoint, Step: "write", Kind: workflow.KindCapability, Attempt: attempt, Input: json.RawMessage(`{"day":"2026-10-05","text":"- x"}`)},
		Capability: "notes.append", Contract: "1.0.0", SchemaSHA256: strings.Repeat("a", 64), Plugin: "notes", Version: "1.0.0",
		Connection: "conn-notes", Resource: "notebook:digest", IdempotencyKey: "sha256:" + strings.Repeat("b", 64), Idempotent: true,
		Reconcile: WorkflowEffectReconcileByOwner, ConflictKeys: keys,
	}
}

func effectsOf(t *testing.T, s *Store, runID string) []WorkflowEffectIntent {
	t.Helper()
	effects, err := s.WorkflowEffectIntents(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return effects
}

// An intent without a receipt blocks its run: no further node attempt,
// pause, resume or failure is recorded until reconciliation records a
// receipt, which the owner's evidence makes immutable history.
func TestWorkflowEffectIntentBlocksTheRunUntilItsReceipt(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	var authority map[string]any
	if err := json.Unmarshal(intent.Authority, &authority); err != nil || authority["calls"] != float64(1) || authority["max_calls_per_run"] != float64(1) ||
		intent.State() != WorkflowEffectPending || intent.Seq != 1 || intent.Attempt != 1 || intent.Step != "write" || intent.Checkpoint != 3 {
		t.Fatalf("intent = %+v (authority %s), %v", intent, intent.Authority, err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: intent.AttemptSeq, Error: "x"}); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("finishing an effect attempt without its receipt: %v", err)
	}
	if err := r.s.InterruptWorkflowAttempt(ctx, lease, intent.AttemptSeq, "stopped"); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("interrupting an effect attempt as a safe rerun: %v", err)
	}
	if _, err := r.s.MarkWorkflowEffectUnknown(ctx, lease, WorkflowEffectUnknownMark{RunID: run.ID, IntentSeq: intent.Seq, Reason: "steps.write: the provider did not answer within 30s",
		Response: json.RawMessage(`{"partial":true}`)}); err != nil {
		t.Fatal(err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if d.Run.State != WorkflowRunNeedsReconciliation || d.Run.NotBefore != nil || !strings.Contains(d.Run.Error, "30s") || len(d.Effects) != 1 ||
		d.Effects[0].State() != WorkflowEffectUnknown || string(d.Effects[0].Unknown.Response) != `{"partial":true}` ||
		d.Attempts[len(d.Attempts)-1].State != WorkflowAttemptFailed {
		t.Fatalf("after unknown = %+v effects %+v", d.Run, d.Effects)
	}

	// Dependent nodes are blocked and nothing resumes, pauses or fails it.
	if _, err := r.s.BeginWorkflowAttempt(ctx, lease, WorkflowAttemptStart{RunID: run.ID, Checkpoint: 3, Step: "write", Kind: workflow.KindCapability, Attempt: 2, Input: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("a node attempt began while an effect is unknown")
	}
	if _, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 2)); err == nil {
		t.Fatal("an effect was sent again while its outcome is unknown")
	}
	if _, err := r.s.PauseWorkflowRun(ctx, lease, "provider away", time.Minute); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("pause while unknown: %v", err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, lease, 3, "give up"); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("fail while unknown: %v", err)
	}
	if resumed, err := r.s.ResumeWorkflowRun(ctx, lease); err != nil || resumed.State != WorkflowRunNeedsReconciliation {
		t.Fatalf("resume while unknown = %+v, %v", resumed, err)
	}
	if due, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("an owner-reconciled run is due: %v, %v", due, err)
	}
	// Its Resource Conflict Key stays held by the unknown intent.
	held, err := r.s.WorkflowEffectConflict(ctx, "another-run", []string{"conn-notes/notebook:digest", "other"})
	if err != nil || held == nil || held.RunID != run.ID || held.Key != "conn-notes/notebook:digest" || !held.Unknown {
		t.Fatalf("conflict = %+v, %v", held, err)
	}

	// The owner reconciles under a fresh claim; the receipt is immutable.
	if err := r.s.ReleaseWorkflowRunLease(ctx, lease, ""); err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "owner-cli", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.ResolveWorkflowEffect(ctx, claim.Lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectPending, Source: WorkflowEffectFromOwner, Surface: "cli"}); err == nil {
		t.Fatal("a pending outcome resolved an unknown effect")
	}
	clock.Advance(time.Second)
	resolved, err := r.s.ResolveWorkflowEffect(ctx, claim.Lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectSucceeded,
		Response: json.RawMessage(`{"entry_id":"e1"}`), Source: WorkflowEffectFromOwner, Evidence: "the digest shows entry e1", Surface: "cli"})
	if err != nil || resolved.State != WorkflowRunRunning || resolved.Error != "" || resolved.NotBefore != nil {
		t.Fatalf("resolved run = %+v, %v", resolved, err)
	}
	if _, err := r.s.ResolveWorkflowEffect(ctx, claim.Lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed,
		Source: WorkflowEffectFromOwner, Surface: "cli"}); err == nil {
		t.Fatal("a resolved effect was resolved again")
	}
	effects := effectsOf(t, r.s, run.ID)
	rc := effects[0].Receipt
	if effects[0].State() != WorkflowEffectSucceeded || rc.Source != WorkflowEffectFromOwner || rc.Actor != "owner" || rc.Surface != "cli" ||
		rc.Evidence != "the digest shows entry e1" || string(rc.Response) != `{"entry_id":"e1"}` || !rc.RecordedAt.Equal(clock.Now()) {
		t.Fatalf("receipt = %+v", rc)
	}
	if held, err := r.s.WorkflowEffectConflict(ctx, "another-run", []string{"conn-notes/notebook:digest"}); err != nil || held != nil {
		t.Fatalf("the reconciled intent still holds its key: %+v, %v", held, err)
	}
	for _, stmt := range []string{
		`UPDATE workflow_effect_intents SET idempotency_key='x'`,
		`DELETE FROM workflow_effect_intents`,
		`UPDATE workflow_effect_receipts SET outcome='failed'`,
		`DELETE FROM workflow_effect_receipts`,
		`UPDATE workflow_effect_unknowns SET reason='x'`,
		`DELETE FROM workflow_effect_conflicts`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err == nil {
			t.Fatalf("%s was allowed", stmt)
		}
	}
}

// A claim that finds an effect attempt left in flight records the attempt
// interrupted and the intent's outcome unknown in its own transaction: the
// run needs reconciliation, due at once for a provider that reconciles,
// and waiting for the owner once reconciliation is deferred to them.
func TestWorkflowEffectClaimRecordsAnIntentLeftInFlightUnknown(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	start := writeEffect(run.ID, 3, 1)
	start.Reconcile = WorkflowEffectReconcileByProvider
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, start)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(leaseFor + time.Second)
	r.reopen()
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if len(claim.Interrupted) != 1 || claim.Interrupted[0].Seq != intent.AttemptSeq {
		t.Fatalf("claim = %+v", claim)
	}
	effects := effectsOf(t, r.s, run.ID)
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effects[0].State() != WorkflowEffectUnknown || !strings.Contains(effects[0].Unknown.Reason, "holder-a") || effects[0].Unknown.LeaseHolder != "holder-b" ||
		p.Run.State != WorkflowRunNeedsReconciliation || p.Run.NotBefore == nil || !p.Run.NotBefore.Equal(clock.Now()) {
		t.Fatalf("after the claim = %+v effects %+v", p.Run, effects)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, claim.Lease, ""); err != nil {
		t.Fatal(err)
	}
	if due, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 1 || due[0] != run.ID {
		t.Fatalf("due = %v, %v", due, err)
	}
	claim, err = r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	deferred, err := r.s.DeferWorkflowReconciliation(ctx, claim.Lease, "the provider cannot tell", 0)
	if err != nil || deferred.State != WorkflowRunNeedsReconciliation || deferred.NotBefore != nil || deferred.Error != "the provider cannot tell" {
		t.Fatalf("deferred = %+v, %v", deferred, err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, claim.Lease, ""); err != nil {
		t.Fatal(err)
	}
	if due, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("due after deferring to the owner = %v, %v", due, err)
	}
}

// An Evie that predates the effect protocol records an effect attempt
// left in flight interrupted and may pause the run; the next claim still
// finds the intent without a receipt, before the pause's recheck time,
// records its outcome unknown and moves the run to needs reconciliation,
// so its Resource Conflict Keys are never held by a run that cannot be
// reconciled.
func TestWorkflowEffectClaimRecordsAnOrphanedIntentUnknown(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	// What an older Evie's claim and pause leave behind.
	later := formatLeaseTime(clock.Now().Add(time.Hour))
	for _, stmt := range []string{
		`UPDATE workflow_node_attempts SET state='interrupted',error='interrupted: holder-a stopped',finished_at='` + formatTime(clock.Now()) + `' WHERE run_id='` + run.ID + `' AND state='running'`,
		`UPDATE workflow_runs SET state='waiting_for_dependency',error='its pinned runtime is not compatible',not_before='` + later + `' WHERE id='` + run.ID + `'`,
		`UPDATE workflow_run_leases SET released_at='` + formatLeaseTime(clock.Now()) + `' WHERE run_id='` + run.ID + `'`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatalf("a claim of a run with an orphaned intent: %v", err)
	}
	effects := effectsOf(t, r.s, run.ID)
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if effects[0].Seq != intent.Seq || effects[0].State() != WorkflowEffectUnknown || !strings.Contains(effects[0].Unknown.Reason, "holder-a") ||
		p.Run.State != WorkflowRunNeedsReconciliation || !strings.Contains(p.Run.Error, "evie workflows reconcile") || p.Run.NotBefore != nil {
		t.Fatalf("after the claim = %+v effects %+v", p.Run, effects)
	}
	if _, err := r.s.ResolveWorkflowEffect(ctx, claim.Lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed,
		Source: WorkflowEffectFromOwner, Evidence: "not in the digest", Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	if held, err := r.s.WorkflowEffectConflict(ctx, "another-run", []string{"conn-notes/notebook:digest"}); err != nil || held != nil {
		t.Fatalf("the reconciled orphan still holds its key: %+v, %v", held, err)
	}
}

// A holder whose lease expired or was taken over writes no receipt, no
// unknown outcome and no reconciliation.
func TestWorkflowEffectStaleHolderWritesNothing(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(leaseFor + time.Second)
	if _, err := r.s.FinishWorkflowEffect(ctx, lease, WorkflowEffectEnd{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed, Error: "late",
		Attempt: WorkflowAttemptEnd{Error: "late"}}); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("a late receipt: %v", err)
	}
	if _, err := r.s.MarkWorkflowEffectUnknown(ctx, lease, WorkflowEffectUnknownMark{RunID: run.ID, IntentSeq: intent.Seq, Reason: "late"}); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("a late unknown outcome: %v", err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.ResolveWorkflowEffect(ctx, lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed,
		Source: WorkflowEffectFromReconcile}); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("a late reconciliation: %v", err)
	}
	if _, err := r.s.DeferWorkflowReconciliation(ctx, lease, "late", time.Minute); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("a late deferral: %v", err)
	}
	effects := effectsOf(t, r.s, run.ID)
	if effects[0].State() != WorkflowEffectUnknown || effects[0].Unknown.LeaseHolder != "holder-b" || effects[0].Receipt != nil {
		t.Fatalf("effects = %+v", effects)
	}
	if _, err := r.s.ResolveWorkflowEffect(ctx, claim.Lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed,
		Source: WorkflowEffectFromReconcile, Evidence: "not found"}); err != nil {
		t.Fatal(err)
	}
}

// max_calls_per_run counts Effect Intents of the operation in the run: once
// it is reached, another intent is refused before anything is recorded.
// Another run holding a Resource Conflict Key refuses the intent too.
func TestWorkflowEffectIntentLimitsAndConflicts(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1, "conn-notes/a", "conn-notes/b"))
	if err != nil {
		t.Fatal(err)
	}
	if held, err := r.s.WorkflowEffectConflict(ctx, "another-run", []string{"conn-notes/b"}); err != nil || held == nil || held.Unknown || held.Step != "write" || held.IntentSeq != intent.Seq {
		t.Fatalf("held = %+v, %v", held, err)
	}
	if held, err := r.s.WorkflowEffectConflict(ctx, run.ID, []string{"conn-notes/b"}); err != nil || held != nil {
		t.Fatalf("a run conflicts with itself: %+v, %v", held, err)
	}
	if _, err := r.s.FinishWorkflowEffect(ctx, lease, WorkflowEffectEnd{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed, Error: "refused",
		Attempt: WorkflowAttemptEnd{Error: "refused", RetryAfter: time.Second}}); err != nil {
		t.Fatal(err)
	}
	if held, err := r.s.WorkflowEffectConflict(ctx, "another-run", []string{"conn-notes/a"}); err != nil || held != nil {
		t.Fatalf("a receipt did not release the keys: %+v, %v", held, err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET not_before=NULL WHERE id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 2)); !errors.Is(err, ErrWorkflowEffectLimit) || !strings.Contains(err.Error(), "max_calls_per_run 1") {
		t.Fatalf("an intent past max_calls_per_run: %v", err)
	}
	if effects := effectsOf(t, r.s, run.ID); len(effects) != 1 || effects[0].State() != WorkflowEffectFailed {
		t.Fatalf("effects = %+v", effects)
	}
}
