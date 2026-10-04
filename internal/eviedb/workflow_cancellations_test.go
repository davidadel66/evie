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

// Cancellation, per-node admission and the audited rerun override (Stage
// 14 sub-stage 8). The owner's cancellation is recorded once, immutably,
// and moves an unfinished run to cancelling without its lease: no node,
// question, pause or failure starts after it, a node in flight finishes
// and is recorded, and the holder of the run's lease finishes the
// cancellation once no attempt is in flight and no Effect Intent lacks its
// receipt, withdrawing an open question in the same transaction. A
// cancelled run frees its Run Key; a terminal run is never changed.

func cancelOf(t *testing.T, s *Store, runID, reason string) WorkflowRunCancellation {
	t.Helper()
	c, err := s.RequestWorkflowRunCancellation(context.Background(), WorkflowRunCancellationRequest{RunID: runID, Reason: reason, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// sameCancellation compares two reads of one cancellation record.
func sameCancellation(x, y WorkflowRunCancellation) bool {
	return x.RunID == y.RunID && x.Reason == y.Reason && x.Actor == y.Actor && x.Surface == y.Surface && x.FromState == y.FromState && x.RequestedAt.Equal(y.RequestedAt)
}

func runState(t *testing.T, s *Store, runID string) WorkflowRun {
	t.Helper()
	d, err := s.WorkflowRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return d.Run
}

// A queued run's cancellation is recorded once with the owner's surface,
// reason and the state it interrupted; its holder finishes it cancelled,
// which frees the Run Key. Repeating it returns the same record; a
// finished run is refused and left as it was.
func TestWorkflowRunCancellationIsRecordedOnceAndFreesTheRunKey(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	for name, req := range map[string]WorkflowRunCancellationRequest{
		"a model surface": {RunID: run.ID, Surface: "chat"},
		"no surface":      {RunID: run.ID},
		"a long reason":   {RunID: run.ID, Surface: "cli", Reason: strings.Repeat("x", 1001)},
	} {
		if _, err := r.s.RequestWorkflowRunCancellation(ctx, req); err == nil {
			t.Fatalf("%s: the cancellation was accepted", name)
		}
	}
	if _, err := r.s.RequestWorkflowRunCancellation(ctx, WorkflowRunCancellationRequest{RunID: run.ID, Surface: "chat"}); !errors.Is(err, ErrWorkflowApprover) {
		t.Fatalf("a model surface: %v", err)
	}
	if _, err := r.s.RequestWorkflowRunCancellation(ctx, WorkflowRunCancellationRequest{RunID: "missing", Surface: "cli"}); !errors.Is(err, ErrWorkflowRunNotFound) {
		t.Fatalf("a missing run: %v", err)
	}
	c := cancelOf(t, r.s, run.ID, "entered twice")
	if c.RunID != run.ID || c.Reason != "entered twice" || c.Actor != "owner" || c.Surface != "cli" || c.FromState != WorkflowRunQueued || !c.RequestedAt.Equal(clock.Now()) {
		t.Fatalf("cancellation = %+v", c)
	}
	got := runState(t, r.s, run.ID)
	if got.State != WorkflowRunCancelling || got.FinishedAt != nil || !strings.Contains(got.Error, "cancelling at the owner's request") {
		t.Fatalf("cancelling run = %+v", got)
	}
	// The cancellation holds the key until it finishes.
	if again, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1")); err != nil || !existing || again.ID != run.ID {
		t.Fatalf("start while cancelling = %+v, %v, %v", again, existing, err)
	}
	r.reopen()
	if again := cancelOf(t, r.s, run.ID, "another reason"); !sameCancellation(again, c) {
		t.Fatalf("a repeated cancellation = %+v, want %+v", again, c)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	done, err := r.s.FinishWorkflowCancellation(ctx, claim.Lease)
	if err != nil || done.State != WorkflowRunCancelled || done.FinishedAt == nil || done.NotBefore != nil ||
		done.Error != "cancelled by the owner via cli: entered twice" || done.StartedAt != nil {
		t.Fatalf("cancelled = %+v, %v", done, err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || d.Cancellation == nil || !sameCancellation(*d.Cancellation, c) || len(d.Attempts) != 0 {
		t.Fatalf("detail = %+v, %v", d, err)
	}
	if again := cancelOf(t, r.s, run.ID, ""); !sameCancellation(again, c) {
		t.Fatalf("cancelling a cancelled run = %+v", again)
	}
	if _, err := r.s.FinishWorkflowCancellation(ctx, claim.Lease); !errors.Is(err, ErrWorkflowRunFinished) {
		t.Fatalf("finishing a cancelled run again: %v", err)
	}
	second, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil || existing || second.ID == run.ID {
		t.Fatalf("start after the cancellation = %+v, %v, %v", second, existing, err)
	}
	// A finished run is never changed.
	finishRun(t, r.s, second)
	if _, err := r.s.RequestWorkflowRunCancellation(ctx, WorkflowRunCancellationRequest{RunID: second.ID, Surface: "web"}); !errors.Is(err, ErrWorkflowRunFinished) {
		t.Fatalf("cancelling a succeeded run: %v", err)
	}
	if d, _ := r.s.WorkflowRun(ctx, second.ID); d.Run.State != WorkflowRunSucceeded || d.Cancellation != nil {
		t.Fatalf("succeeded run after a refused cancellation = %+v", d)
	}
	for _, stmt := range []string{
		`UPDATE workflow_run_cancellations SET reason='edited'`,
		`DELETE FROM workflow_run_cancellations`,
		`UPDATE workflow_runs SET state='running' WHERE id='` + run.ID + `'`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err == nil {
			t.Fatalf("%s was allowed", stmt)
		}
	}
}

// While a run is cancelling, a node in flight finishes and is recorded,
// but no node starts and the run is neither paused nor failed: its holder
// finishes the cancellation. A node in flight that ends the run (the
// graph's last node) ends it, since nothing was left to cancel; a failed
// attempt is not retried.
func TestWorkflowCancellingRunStartsNoFurtherNode(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	lease := testLease(t, r.s, run.ID)
	split, err := r.s.BeginWorkflowAttempt(ctx, lease, splitAttempt(run.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if c := cancelOf(t, r.s, run.ID, "wrong bill"); c.FromState != WorkflowRunRunning {
		t.Fatalf("cancellation = %+v", c)
	}
	if _, err := r.s.FinishWorkflowCancellation(ctx, lease); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("finishing with an attempt in flight: %v", err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: split.Seq, Output: json.RawMessage(`{"share":1}`), Next: "check",
		State: json.RawMessage(`{"split":{"share":1}}`)}); err != nil {
		t.Fatalf("the node in flight was not recorded: %v", err)
	}
	if got := runState(t, r.s, run.ID); got.State != WorkflowRunCancelling || got.Checkpoint != 1 {
		t.Fatalf("after the node = %+v", got)
	}
	check := WorkflowAttemptStart{RunID: run.ID, Checkpoint: 1, Step: "check", Kind: workflow.KindBranch, Attempt: 1, Input: json.RawMessage(`{}`)}
	if _, err := r.s.BeginWorkflowAttempt(ctx, lease, check); !errors.Is(err, ErrWorkflowRunCancelling) {
		t.Fatalf("a node after the cancellation: %v", err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, lease, "a dependency", time.Minute); !errors.Is(err, ErrWorkflowRunCancelling) {
		t.Fatalf("pausing a cancelling run: %v", err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, lease, 1, "a limit"); !errors.Is(err, ErrWorkflowRunCancelling) {
		t.Fatalf("failing a cancelling run: %v", err)
	}
	if got, err := r.s.ResumeWorkflowRun(ctx, lease); err != nil || got.State != WorkflowRunCancelling {
		t.Fatalf("resuming a cancelling run = %+v, %v", got, err)
	}
	done, err := r.s.FinishWorkflowCancellation(ctx, lease)
	if err != nil || done.State != WorkflowRunCancelled || done.Checkpoint != 1 {
		t.Fatalf("cancelled = %+v, %v", done, err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: split.Seq, Error: "late"}); !errors.Is(err, ErrWorkflowRunFinished) {
		t.Fatalf("a write to a cancelled run: %v", err)
	}

	// The graph's last node, in flight when the cancellation arrives, ends
	// the run: it succeeded, and the cancellation stays recorded.
	last, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k2"))
	if err != nil {
		t.Fatal(err)
	}
	lease = testLease(t, r.s, last.ID)
	at, err := r.s.BeginWorkflowAttempt(ctx, lease, splitAttempt(last.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	cancelOf(t, r.s, last.ID, "")
	if got, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: last.ID, Seq: at.Seq, Output: json.RawMessage(`1`), State: json.RawMessage(`{"split":1}`),
		Outputs: json.RawMessage(`{"x":1}`)}); err != nil || got.State != WorkflowRunSucceeded || got.Error != "" {
		t.Fatalf("the last node = %+v, %v", got, err)
	}
	if d, _ := r.s.WorkflowRun(ctx, last.ID); d.Cancellation == nil {
		t.Fatal("the cancellation was not kept")
	}

	// A failed attempt is not retried while cancelling.
	failing, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k3"))
	if err != nil {
		t.Fatal(err)
	}
	lease = testLease(t, r.s, failing.ID)
	at, err = r.s.BeginWorkflowAttempt(ctx, lease, splitAttempt(failing.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	cancelOf(t, r.s, failing.ID, "")
	if got, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: failing.ID, Seq: at.Seq, Error: "boom", RetryAfter: time.Minute}); err != nil ||
		got.State != WorkflowRunCancelling || got.NotBefore != nil {
		t.Fatalf("a failed attempt while cancelling = %+v, %v", got, err)
	}
	if d, _ := r.s.WorkflowRun(ctx, failing.ID); len(d.Events) != 0 {
		t.Fatalf("a retry was scheduled: %+v", d.Events)
	}
}

// A run waiting for the owner's answer is cancelled by withdrawing its
// question unanswered in the transaction that finishes the cancellation;
// a cancelling run asks nothing new and takes no answer.
func TestWorkflowCancellationWithdrawsAnOpenQuestion(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	run, lease := notesAtAsk(t, r.s)
	q, err := r.s.AskWorkflowQuestion(ctx, lease, askQuestion(run.ID))
	if err != nil {
		t.Fatal(err)
	}
	if c := cancelOf(t, r.s, run.ID, "not needed today"); c.FromState != WorkflowRunWaitingForHuman {
		t.Fatalf("cancellation = %+v", c)
	}
	if _, err := r.s.AnswerWorkflowQuestion(ctx, lease, WorkflowAnswer{RunID: run.ID, Seq: q.Seq, Answer: json.RawMessage(`{"expected":true}`), Surface: "cli"}); err == nil {
		t.Fatal("a cancelling run's question was answered")
	}
	done, err := r.s.FinishWorkflowCancellation(ctx, lease)
	if err != nil || done.State != WorkflowRunCancelled {
		t.Fatalf("cancelled = %+v, %v", done, err)
	}
	res := interruptionsOf(t, r.s, run.ID)[0].Resolution
	if res == nil || res.Kind != WorkflowInterruptionWithdrawn || res.Actor != "runtime" || len(res.Answer) != 0 ||
		!strings.Contains(res.Detail, "the owner cancelled the run via cli: not needed today") {
		t.Fatalf("resolution = %+v", res)
	}
	if open := listInterruptions(t, r.s, WorkflowInterruptionFilter{RunID: run.ID}); len(open) != 0 {
		t.Fatalf("still open = %+v", open)
	}

	r2, _ := openLeasedRunDB(t)
	asking, askLease := notesAtAsk(t, r2.s)
	cancelOf(t, r2.s, asking.ID, "")
	if _, err := r2.s.AskWorkflowQuestion(ctx, askLease, askQuestion(asking.ID)); !errors.Is(err, ErrWorkflowRunCancelling) {
		t.Fatalf("asking while cancelling: %v", err)
	}
}

// An effect in flight when the cancellation arrives completes or reaches
// reconciliation: the run stays cancelling, never needs_reconciliation,
// while the intent has no receipt, and is not due for a scheduler until
// its reconciliation is; the receipt (here the owner's) lets its holder
// finish the cancellation, without retrying the effect.
func TestWorkflowCancellationWaitsForAnEffectsReceipt(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	cancelOf(t, r.s, run.ID, "")
	if _, err := r.s.FinishWorkflowCancellation(ctx, lease); err == nil {
		t.Fatal("a cancellation finished with an effect in flight")
	}
	if _, err := r.s.MarkWorkflowEffectUnknown(ctx, lease, WorkflowEffectUnknownMark{RunID: run.ID, IntentSeq: intent.Seq, Reason: "steps.write: timed out",
		RunError: "steps.write: timed out; reconcile it with: evie workflows reconcile"}); err != nil {
		t.Fatal(err)
	}
	got := runState(t, r.s, run.ID)
	if got.State != WorkflowRunCancelling || !strings.Contains(got.Error, "reconcile it with") || !strings.Contains(got.Error, "cancelling") {
		t.Fatalf("an unknown effect while cancelling = %+v", got)
	}
	if _, err := r.s.FinishWorkflowCancellation(ctx, lease); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("finishing with an unknown effect: %v", err)
	}
	if _, err := r.s.WithdrawWorkflowQuestion(ctx, lease, run.ID, 1, "cancel"); !errors.Is(err, ErrWorkflowEffectUnresolved) {
		t.Fatalf("withdrawing the effect's interruption: %v", err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, lease, ""); err != nil {
		t.Fatal(err)
	}
	if due, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("due while it waits for the owner's reconciliation = %v, %v", due, err)
	}
	lease = testLease(t, r.s, run.ID)
	resolved, err := r.s.ResolveWorkflowEffect(ctx, lease, WorkflowEffectResolution{RunID: run.ID, IntentSeq: intent.Seq, Outcome: WorkflowEffectFailed,
		Error: "the owner reconciled it as not having happened", Source: WorkflowEffectFromOwner, Surface: "cli"})
	if err != nil || resolved.State != WorkflowRunCancelling || resolved.NotBefore != nil {
		t.Fatalf("reconciled while cancelling = %+v, %v", resolved, err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, lease, ""); err != nil {
		t.Fatal(err)
	}
	if due, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 1 || due[0] != run.ID {
		t.Fatalf("due once reconciled = %v, %v", due, err)
	}
	lease = testLease(t, r.s, run.ID)
	if done, err := r.s.FinishWorkflowCancellation(ctx, lease); err != nil || done.State != WorkflowRunCancelled {
		t.Fatalf("cancelled = %+v, %v", done, err)
	}
	if held, err := r.s.WorkflowEffectConflict(ctx, "another-run", []string{"conn-notes/notebook:digest"}); err != nil || held != nil {
		t.Fatalf("the key is still held: %+v, %v", held, err)
	}
}

// A run that needs reconciliation is cancelled by moving it to
// cancelling, keeping its automatic reconciliation time: it is due for
// that, a claim after a crash records an effect left in flight unknown and
// keeps it cancelling, and automatic reconciliation may be deferred.
func TestWorkflowCancellationOfARunThatNeedsReconciliation(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	start := writeEffect(run.ID, 3, 1)
	start.Reconcile = WorkflowEffectReconcileByProvider
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.MarkWorkflowEffectUnknown(ctx, lease, WorkflowEffectUnknownMark{RunID: run.ID, IntentSeq: intent.Seq, Reason: "timed out", Settle: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, lease, ""); err != nil {
		t.Fatal(err)
	}
	if c := cancelOf(t, r.s, run.ID, ""); c.FromState != WorkflowRunNeedsReconciliation {
		t.Fatalf("cancellation = %+v", c)
	}
	got := runState(t, r.s, run.ID)
	if got.State != WorkflowRunCancelling || got.NotBefore == nil || !got.NotBefore.Equal(clock.Now().Add(time.Minute)) {
		t.Fatalf("cancelling = %+v", got)
	}
	if due, _ := r.s.DueWorkflowRuns(ctx, 10); len(due) != 0 {
		t.Fatalf("due before its reconciliation time = %v", due)
	}
	clock.Advance(time.Minute)
	if due, _ := r.s.DueWorkflowRuns(ctx, 10); len(due) != 1 {
		t.Fatalf("due at its reconciliation time = %v", due)
	}
	lease = testLease(t, r.s, run.ID)
	deferred, err := r.s.DeferWorkflowReconciliation(ctx, lease, "the provider is unavailable", time.Minute)
	if err != nil || deferred.State != WorkflowRunCancelling || !strings.Contains(deferred.Error, "the provider is unavailable") {
		t.Fatalf("deferred = %+v, %v", deferred, err)
	}

	// A crash between intent and receipt while cancelling leaves it
	// cancelling, its outcome unknown.
	r2, clock2 := openLeasedRunDB(t)
	crashed, crashedLease := notesAtWrite(t, r2.s)
	if _, err := r2.s.BeginWorkflowEffect(ctx, crashedLease, writeEffect(crashed.ID, 3, 1)); err != nil {
		t.Fatal(err)
	}
	cancelOf(t, r2.s, crashed.ID, "")
	if due, _ := r2.s.DueWorkflowRuns(ctx, 10); len(due) != 0 {
		t.Fatalf("due while its holder's lease is live = %v", due)
	}
	clock2.Advance(leaseFor + time.Second)
	r2.reopen()
	if due, _ := r2.s.DueWorkflowRuns(ctx, 10); len(due) != 1 {
		t.Fatalf("due once its holder's lease expired = %v", due)
	}
	claim, err := r2.s.AcquireWorkflowRunLease(ctx, crashed.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if got := runState(t, r2.s, crashed.ID); got.State != WorkflowRunCancelling || len(claim.Interrupted) != 1 ||
		effectsOf(t, r2.s, crashed.ID)[0].State() != WorkflowEffectUnknown {
		t.Fatalf("after the claim = %+v claim %+v", got, claim)
	}
}

// Every node attempt and question applies run admission in the
// transaction that records it, so a deactivation or a block reaches a
// running run at its next node boundary, whatever the node.
func TestWorkflowNodeAttemptsApplyRunAdmission(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	w, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	lease := testLease(t, r.s, run.ID)
	if _, err := r.s.DeactivateWorkflow(ctx, w.ID, a.Name, a.Version, "retired", "cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.BeginWorkflowAttempt(ctx, lease, splitAttempt(run.ID, 0, 1)); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("an attempt after deactivation: %v", err)
	}
	if d, _ := r.s.WorkflowRun(ctx, run.ID); len(d.Attempts) != 0 || d.Run.State != WorkflowRunQueued {
		t.Fatalf("a refused attempt recorded something: %+v", d)
	}

	w2, a2 := splitBillActivation(t, r.s)
	other, _, err := r.s.StartWorkflowRun(ctx, runStart(a2, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	otherLease := testLease(t, r.s, other.ID)
	if _, err := r.s.ArchiveWorkspace(ctx, w2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.BeginWorkflowAttempt(ctx, otherLease, splitAttempt(other.ID, 0, 1)); !errors.Is(err, ErrWorkflowBlocked) {
		t.Fatalf("an attempt in an archived Workspace: %v", err)
	}

	r2, _ := openLeasedRunDB(t)
	asking, askLease := notesAtAsk(t, r2.s)
	d, err := r2.s.WorkflowRun(ctx, asking.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r2.s.DeactivateWorkflow(ctx, d.Run.WorkspaceID, d.Run.Workflow, d.Run.Version, "retired", "cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := r2.s.AskWorkflowQuestion(ctx, askLease, askQuestion(asking.ID)); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("a question after deactivation: %v", err)
	}
}

// The owner's rerun of a succeeded run is a new run of the same logical
// execution under a Run Key that includes the override, recorded with the
// run in one transaction (the run it reruns, the logical Run Key, the
// reason, the owner's surface). Repeating it returns that run; a rerun of
// anything but a succeeded run of the same logical execution is refused.
func TestWorkflowRerunOverrideIsRecordedWithItsOwnRunKey(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	_, a := splitBillActivation(t, r.s)
	first, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	rerun := func(of, logical, key, reason string) WorkflowRunStart {
		s := runStart(a, key)
		s.RerunOf, s.LogicalKey, s.Reason = of, logical, reason
		return s
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, rerun(first.ID, "split-bill:k1", "split-bill:rerun:1", "again")); !errors.Is(err, ErrWorkflowRerunRefused) {
		t.Fatalf("a rerun of a queued run: %v", err)
	}
	finishRun(t, r.s, first)
	other, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k2"))
	if err != nil {
		t.Fatal(err)
	}
	finishRun(t, r.s, other)
	for name, s := range map[string]WorkflowRunStart{
		"no reason":         rerun(first.ID, "split-bill:k1", "split-bill:rerun:1", ""),
		"a long reason":     rerun(first.ID, "split-bill:k1", "split-bill:rerun:1", strings.Repeat("x", 1001)),
		"another key":       rerun(first.ID, "split-bill:k2", "split-bill:rerun:1", "again"),
		"the same key":      rerun(first.ID, "split-bill:k1", "split-bill:k1", "again"),
		"a missing run":     rerun("missing", "split-bill:k1", "split-bill:rerun:1", "again"),
		"no logical key":    rerun(first.ID, "", "split-bill:rerun:1", "again"),
		"another execution": rerun(other.ID, "split-bill:k1", "split-bill:rerun:1", "again"),
	} {
		if _, _, err := r.s.StartWorkflowRun(ctx, s); !errors.Is(err, ErrWorkflowRerunRefused) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	second, existing, err := r.s.StartWorkflowRun(ctx, rerun(first.ID, "split-bill:k1", "split-bill:rerun:1", "the ledger was restored"))
	if err != nil || existing || second.RunKey != "split-bill:rerun:1" {
		t.Fatalf("rerun = %+v, %v, %v", second, existing, err)
	}
	r.reopen()
	d, err := r.s.WorkflowRun(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	o := d.Override
	if o == nil || o.RunID != second.ID || o.RerunOf != first.ID || o.LogicalKey != "split-bill:k1" || o.Reason != "the ledger was restored" ||
		o.Actor != "owner" || o.Surface != "cli" || o.RecordedAt.IsZero() {
		t.Fatalf("override = %+v", o)
	}
	again, existing, err := r.s.StartWorkflowRun(ctx, rerun(first.ID, "split-bill:k1", "split-bill:rerun:1", "again"))
	if err != nil || !existing || again.ID != second.ID {
		t.Fatalf("a repeated rerun = %+v, %v, %v", again, existing, err)
	}
	// A normal start of the logical key still returns the run that holds it.
	if held, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1")); err != nil || !existing || held.ID != first.ID {
		t.Fatalf("a normal start = %+v, %v, %v", held, existing, err)
	}
	// A rerun of the rerun continues the same logical execution.
	finishRun(t, r.s, second)
	third, existing, err := r.s.StartWorkflowRun(ctx, rerun(second.ID, "split-bill:k1", "split-bill:rerun:2", "once more"))
	if err != nil || existing {
		t.Fatalf("a rerun of the rerun = %+v, %v, %v", third, existing, err)
	}
	for _, stmt := range []string{`UPDATE workflow_run_overrides SET reason='edited'`, `DELETE FROM workflow_run_overrides`} {
		if _, err := r.db.ExecContext(ctx, stmt); err == nil {
			t.Fatalf("%s was allowed", stmt)
		}
	}
}
