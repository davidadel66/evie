package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// Cancellation and recovery (Stage 14 sub-stage 8). The owner's
// cancellation stops future nodes: a node in flight finishes (a read or an
// AI call is stopped through its context and recorded interrupted), an
// effect in flight completes or reaches reconciliation and the run stays
// cancelling until its receipt, a question is withdrawn, and the run ends
// cancelled with its completed nodes and effects recorded and its Run Key
// free. A cancelling run is finished by whichever runtime claims it next,
// across restarts. Revocation reaches a running run at its next node
// boundary, and the owner's audited rerun override reruns a succeeded
// logical execution without its effects being suppressed. Every test uses
// a real temporary SQLite file; "processes" are separate connection pools
// and runtimes over it.

func (h *runHarness) cancel(rt *Runtime, id, reason string, detach bool) eviedb.WorkflowRunDetail {
	h.t.Helper()
	d, err := rt.Cancel(context.Background(), CancelRequest{RunID: id, Reason: reason, Surface: "cli", Detach: detach})
	if err != nil {
		h.t.Fatal(err)
	}
	return d
}

// A queued run is cancelled at once and frees its Run Key; cancelling it
// again returns the same record, and a finished run is refused and left
// unchanged. Only the owner's surfaces cancel.
func TestWorkflowCancelQueuedRunIsCancelledAndFreesItsRunKey(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	queued := h.detach(w.ID, billInputs("dinner-1", 1000, 3))
	if _, err := h.rt.Cancel(ctx, CancelRequest{RunID: queued.ID, Surface: "chat"}); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("a model surface: %v", err)
	}
	if _, err := h.rt.Cancel(ctx, CancelRequest{RunID: "missing", Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowRunNotFound) {
		t.Fatalf("a missing run: %v", err)
	}
	d := h.cancel(h.rt, queued.ID, "entered twice", false)
	c := d.Cancellation
	if d.Run.State != eviedb.WorkflowRunCancelled || len(d.Attempts) != 0 || c == nil || c.Reason != "entered twice" || c.Surface != "cli" ||
		c.Actor != "owner" || c.FromState != eviedb.WorkflowRunQueued || d.Run.Error != "cancelled by the owner via cli: entered twice" {
		t.Fatalf("cancelled = %+v cancellation %+v", d.Run, c)
	}
	if text := RunText(d); !strings.Contains(text, "Cancelled by the owner via cli") || !strings.Contains(text, "entered twice") || strings.Contains(text, "Next:") {
		t.Fatalf("inspection:\n%s", text)
	}
	if again := h.cancel(h.rt, queued.ID, "", false); again.Run.State != eviedb.WorkflowRunCancelled || again.Cancellation.Reason != "entered twice" {
		t.Fatalf("cancelling again = %+v", again.Cancellation)
	}
	next := h.start(w.ID, billInputs("dinner-1", 1000, 3))
	if next.Existing || next.Run.Run.ID == queued.ID || next.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("start of the freed key = %+v", next)
	}
	if _, err := h.rt.Cancel(ctx, CancelRequest{RunID: next.Run.Run.ID, Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowRunFinished) {
		t.Fatalf("cancelling a succeeded run: %v", err)
	}
	if d := h.inspect(next.Run.Run.ID); d.Run.State != eviedb.WorkflowRunSucceeded || d.Cancellation != nil || d.Run.Error != "" {
		t.Fatalf("a succeeded run after a refused cancellation = %+v", d)
	}
}

// A deterministic node in flight when another process cancels the run
// finishes and is recorded; no further node starts.
func TestWorkflowCancelDuringADeterministicNodeStopsFutureNodes(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	a, owner := h.process(), h.process()
	var once sync.Once
	var during eviedb.WorkflowRunDetail
	a.rt.call = func(run string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "split" {
			once.Do(func() { during = h.cancel(owner.rt, run, "wrong bill", true) })
		}
		return v.CallFunction(step, input)
	}
	res, err := a.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if during.Run.State != eviedb.WorkflowRunCancelling {
		t.Fatalf("while the node ran = %+v", during.Run)
	}
	d := h.inspect(res.Run.Run.ID)
	want := []attemptSummary{{"split", 1, eviedb.WorkflowAttemptSucceeded, "check", 0}}
	if d.Run.State != eviedb.WorkflowRunCancelled || !reflect.DeepEqual(summarize(d), want) || len(d.Checkpoints) != 2 ||
		d.Cancellation.FromState != eviedb.WorkflowRunRunning || d.Run.Outputs != nil {
		t.Fatalf("run = %+v attempts %+v", d.Run, summarize(d))
	}
	checkOrdering(t, d)
}

// A read in flight is stopped through its context when another process
// cancels the run, and recorded interrupted, not failed.
func TestWorkflowCancelDuringAReadStopsItAndRecordsItInterrupted(t *testing.T) {
	entered := make(chan struct{})
	h, _, script := newCapabilityHarness(t, blockUntilCancelled(entered))
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	a := h.process()
	done := make(chan RunResult, 1)
	go func() {
		res, err := h.startSpending(a.rt, w.ID, "2026-09", 2, false)
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	if d := h.cancel(h.rt, id, "", true); d.Run.State != eviedb.WorkflowRunCancelling {
		t.Fatalf("while the read ran = %+v", d.Run)
	}
	res := <-done
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunCancelled || len(d.Attempts) != 1 || d.Attempts[0].State != eviedb.WorkflowAttemptInterrupted ||
		!strings.Contains(d.Attempts[0].Error, "cancelled") || script.count() != 1 || script.ctx(0).Err() == nil {
		t.Fatalf("run = %+v attempts %+v reads %d", d.Run, d.Attempts, script.count())
	}
}

// An AI call in flight is stopped like a stop when the run is cancelled:
// its attempt is recorded interrupted and its call counts at its
// estimate, since it may have been billed.
func TestWorkflowCancelDuringAnAICallEndsItLikeAStop(t *testing.T) {
	entered := make(chan struct{})
	var once sync.Once
	chat := newFakeChat(func(ctx context.Context, _ openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return openrouter.ChatResponse{}, ctx.Err()
	})
	h := newAIHarness(t, chat)
	w := h.approveReceiptSplit(receiptSplitDir)
	a := h.process()
	done := make(chan RunResult, 1)
	go func() {
		res, err := h.startReceipt(a.rt, w.ID, "r-1", "")
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	h.cancel(h.rt, id, "", true)
	<-done
	d := h.inspect(id)
	want := []attemptSummary{
		{"tip", 1, eviedb.WorkflowAttemptSucceeded, "read", 0},
		{"read", 1, eviedb.WorkflowAttemptInterrupted, "", 1},
	}
	if d.Run.State != eviedb.WorkflowRunCancelled || !reflect.DeepEqual(summarize(d), want) || !strings.Contains(d.Attempts[1].Error, "cancelled") ||
		len(chat.captured()) != 1 {
		t.Fatalf("run = %+v attempts %+v", d.Run, summarize(d))
	}
	calls := d.ModelCalls
	if len(calls) != 1 || calls[0].State() != eviedb.WorkflowModelCallInterrupted {
		t.Fatalf("model calls = %+v", calls)
	}
	if p := h.progress(id); p.ModelTokens != calls[0].EstimatedInputTokens+calls[0].EstimatedOutputTokens {
		t.Fatalf("spend = %d tokens; calls %+v", p.ModelTokens, calls)
	}
}

// An effect in flight when the run is cancelled is not stopped: it
// completes, its receipt and output are recorded, and the run is then
// cancelled before the step that depends on it.
func TestWorkflowCancelDuringAnEffectLetsItComplete(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	ledger := newFakeLedger(func(l *fakeLedger, ctx context.Context, req plugins.WorkflowEffectRequest) (any, error) {
		close(entered)
		<-release
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return l.apply(req), nil
	})
	h, _ := newEffectHarness(t, ledger, true, nil)
	h.scheduler.EffectTimeout = 10 * time.Second
	h.reopen()
	w := h.approvePostShare(postShareDir)
	a := h.process()
	done := make(chan RunResult, 1)
	go func() {
		res, err := h.startPost(a.rt, w.ID, "dinner-1", false)
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	h.cancel(h.rt, id, "", true)
	time.Sleep(20 * time.Millisecond)
	close(release)
	<-done
	d := h.inspect(id)
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "post", 0},
		{"post", 1, eviedb.WorkflowAttemptSucceeded, "record", 1},
	}
	if d.Run.State != eviedb.WorkflowRunCancelled || !reflect.DeepEqual(summarize(d), want) ||
		!reflect.DeepEqual(effectStates(d), []eviedb.WorkflowEffectState{eviedb.WorkflowEffectSucceeded}) || d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromProvider ||
		checkpointAfter(d, d.Attempts[1].Seq) == nil {
		t.Fatalf("run = %+v attempts %+v effects %+v", d.Run, summarize(d), d.Effects)
	}
	if performs, _ := ledger.counts(); performs != 1 || ledger.balanceOf("account:household") != 250 {
		t.Fatalf("performs %d", performs)
	}
}

// An effect in flight whose outcome becomes unknown after the run was
// cancelled keeps the run cancelling: nothing retries it or resumes the
// run, and only its receipt (here the owner's reconciliation) lets the
// cancellation finish, without running the dependent step.
func TestWorkflowCancelDuringAnEffectWhoseOutcomeBecomesUnknownWaitsForItsReceipt(t *testing.T) {
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	ledger := newFakeLedger(func(l *fakeLedger, _ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
		close(entered)
		l.apply(req)
		<-release
		return nil, errors.New("connection reset")
	})
	h, _ := newEffectHarness(t, ledger, true, nil)
	h.scheduler.EffectTimeout = 10 * time.Second
	h.reopen()
	w := h.approvePostShare(postShareDir)
	a := h.process()
	done := make(chan RunResult, 1)
	go func() {
		res, err := h.startPost(a.rt, w.ID, "dinner-1", false)
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	<-entered
	id := h.runs(w.ID)[0].ID
	h.cancel(h.rt, id, "", true)
	close(release)
	if res := <-done; res.Run.Run.State != eviedb.WorkflowRunCancelling {
		t.Fatalf("the start returned %+v", res.Run.Run)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunCancelling || !reflect.DeepEqual(effectStates(d), []eviedb.WorkflowEffectState{eviedb.WorkflowEffectUnknown}) ||
		len(d.Interruptions) != 1 || !d.Interruptions[0].Open() || !strings.Contains(d.Run.Error, "evie workflows reconcile "+id+" post") {
		t.Fatalf("run = %+v effects %+v interruptions %+v", d.Run, d.Effects, d.Interruptions)
	}
	if text := RunText(d); !strings.Contains(text, "Cancelling") {
		t.Fatalf("inspection:\n%s", text)
	}
	p := h.process()
	stop := p.rt.Start(ctx)
	for range 3 {
		h.clock.Advance(5 * time.Minute)
		time.Sleep(20 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if due, err := h.store.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("due = %v, %v", due, err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunCancelling || len(d.Attempts) != 2 {
		t.Fatalf("after polling = %+v attempts %+v", d.Run, summarize(d))
	}
	reconciled, err := h.rt.Reconcile(ctx, Reconciliation{RunID: id, Step: "post", Outcome: OutcomeHappened, Evidence: "the ledger shows entry-1",
		Output: json.RawMessage(`{"entry_id":"entry-1","balance_cents":250}`), Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Run.State != eviedb.WorkflowRunCancelled || len(reconciled.Attempts) != 2 || reconciled.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromOwner {
		t.Fatalf("reconciled = %+v attempts %+v", reconciled.Run, summarize(reconciled))
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("performs %d", performs)
	}
}

// A run waiting for the owner's answer is cancelled by withdrawing its
// question unanswered; the question can no longer be answered.
func TestWorkflowCancelWhileWaitingForAnAnswerWithdrawsTheQuestion(t *testing.T) {
	ctx := context.Background()
	chat := newFakeChat(reply(lowTotal, 120, 15))
	h := newAIHarness(t, chat)
	w := h.approveReceiptSplit(receiptSplitDir)
	res, err := h.startReceipt(h.rt, w.ID, "r-2", "")
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	d, err := h.rt.Cancel(ctx, CancelRequest{RunID: id, Reason: "the receipt was lost", Surface: "web"})
	if err != nil {
		t.Fatal(err)
	}
	q := d.Interruptions[0]
	if d.Run.State != eviedb.WorkflowRunCancelled || d.Cancellation.FromState != eviedb.WorkflowRunWaitingForHuman || q.Resolution == nil ||
		q.Resolution.Kind != eviedb.WorkflowInterruptionWithdrawn || !strings.Contains(q.Resolution.Detail, "the receipt was lost") {
		t.Fatalf("run = %+v interruptions %+v", d.Run, d.Interruptions)
	}
	if _, err := h.rt.Answer(ctx, AnswerRequest{RunID: id, Step: "confirm", Answer: json.RawMessage(`{"total_cents":1300}`), Surface: "cli"}); !errors.Is(err, ErrAnswerRefused) &&
		!errors.Is(err, eviedb.ErrWorkflowRunFinished) {
		t.Fatalf("answering a withdrawn question: %v", err)
	}
	if open, err := h.rt.Interruptions(ctx, InterruptionFilter{WorkspaceID: w.ID}); err != nil || len(open) != 0 {
		t.Fatalf("still open = %+v, %v", open, err)
	}
}

// A run that needs reconciliation is cancelled once its effect is
// reconciled, and never retried: by the owner (did not happen), or by the
// provider's automatic reconciliation in whichever runtime claims it once
// it is due.
func TestWorkflowCancelOfARunThatNeedsReconciliationWaitsForTheReceipt(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	d := h.cancel(h.rt, id, "", false)
	if d.Run.State != eviedb.WorkflowRunCancelling || d.Cancellation.FromState != eviedb.WorkflowRunNeedsReconciliation ||
		!strings.Contains(d.Run.Error, "evie workflows reconcile "+id+" post") {
		t.Fatalf("cancelling = %+v", d.Run)
	}
	done, err := h.rt.Reconcile(ctx, Reconciliation{RunID: id, Step: "post", Outcome: OutcomeDidNotHappen, Surface: "cli"})
	if err != nil || done.Run.State != eviedb.WorkflowRunCancelled || len(done.Effects) != 1 {
		t.Fatalf("reconciled = %+v, %v", done.Run, err)
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("a cancelled effect was retried: %d performs", performs)
	}

	// The provider's lookup first fails, so automatic reconciliation waits
	// for its recheck; the cancellation keeps that time and is finished by
	// the runtime that reconciles it.
	looked := newFakeLedger(timeoutAfterAcceptance)
	var mu sync.Mutex
	failed := false
	h, _ = newEffectHarness(t, looked, true, func(ctx context.Context, req plugins.WorkflowEffectRequest) (plugins.WorkflowEffectLookup, error) {
		mu.Lock()
		first := !failed
		failed = true
		mu.Unlock()
		if first {
			return plugins.WorkflowEffectLookup{}, errors.New("the ledger is busy")
		}
		return looked.lookupByKey(ctx, req)
	})
	w = h.approvePostShare(postShareDir)
	res, err = h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil || res.Run.Run.State != eviedb.WorkflowRunNeedsReconciliation || res.Run.Run.NotBefore == nil {
		t.Fatalf("start = %+v, %v", res.Run.Run, err)
	}
	id = res.Run.Run.ID
	if d := h.cancel(h.rt, id, "", false); d.Run.State != eviedb.WorkflowRunCancelling || d.Run.NotBefore == nil {
		t.Fatalf("cancelling = %+v", d.Run)
	}
	h.clock.Advance(time.Minute)
	p := h.process()
	stop := p.rt.Start(ctx)
	eventually(t, "the cancellation to finish", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunCancelled || d.Effects[0].Receipt == nil || d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromReconcile ||
		len(d.Attempts) != 2 {
		t.Fatalf("run = %+v effects %+v attempts %+v", d.Run, d.Effects, summarize(d))
	}
	if performs, lookups := looked.counts(); performs != 1 || lookups != 1 {
		t.Fatalf("performs %d lookups %d", performs, lookups)
	}
}

// A cancellation recorded while the run's holder is dead is finished by
// the next runtime that claims the run once the dead holder's lease
// expires: the attempt it left in flight is recorded interrupted, and no
// further node runs.
func TestWorkflowCancellingRunIsFinishedAfterARestart(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, _ := h.approveSplitBill(splitBillDir)
	a := h.process()
	dead := h.crashInside(a, "split")
	go func() {
		_, _ = a.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Inputs: billInputs("dinner-1", 1000, 3), Surface: "cli"})
	}()
	<-dead
	id := h.runs(w.ID)[0].ID
	if d := h.cancel(h.rt, id, "the process died", true); d.Run.State != eviedb.WorkflowRunCancelling {
		t.Fatalf("cancelling = %+v", d.Run)
	}
	h.reopen()
	if due, err := h.store.DueWorkflowRuns(ctx, 10); err != nil || len(due) != 0 {
		t.Fatalf("due while the dead holder's lease is live = %v, %v", due, err)
	}
	b := h.process()
	stop := b.rt.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	if state := h.state(id); state != eviedb.WorkflowRunCancelling {
		t.Fatalf("taken over early: %s", state)
	}
	h.clock.Advance(testLease)
	eventually(t, "the cancellation to finish", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	want := []attemptSummary{{"split", 1, eviedb.WorkflowAttemptInterrupted, "", 0}}
	if d.Run.State != eviedb.WorkflowRunCancelled || !reflect.DeepEqual(summarize(d), want) ||
		!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseTakenOver, eviedb.WorkflowEventAttemptInterrupted}) {
		t.Fatalf("run = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
}

// A deactivation reaches a running run at its next node boundary: the
// node in flight finishes, the next one does not start and the run
// pauses with the reason; the owner can then cancel the paused run, which
// frees its Run Key. A Workspace archived during a run likewise stops it
// before its AI step sends anything.
func TestWorkflowRevocationPausesARunAtItsNextNodeAndThePausedRunIsCancelled(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, a := h.approveSplitBill(splitBillDir)
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "split" {
			if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: w.ID, Name: a.Name, Version: a.Version, Surface: "cli"}); err != nil {
				return nil, err
			}
		}
		return v.CallFunction(step, input)
	}
	res := h.start(w.ID, billInputs("dinner-1", 1000, 3))
	id := res.Run.Run.ID
	d := h.inspect(id)
	want := []attemptSummary{{"split", 1, eviedb.WorkflowAttemptSucceeded, "check", 0}}
	if d.Run.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(d.Run.Error, "version 1 was deactivated") ||
		!reflect.DeepEqual(summarize(d), want) || !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused}) {
		t.Fatalf("run = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
	h.rt.call = nil
	d = h.cancel(h.rt, id, "retired", false)
	if d.Run.State != eviedb.WorkflowRunCancelled || d.Cancellation.FromState != eviedb.WorkflowRunWaitingForDependency || !reflect.DeepEqual(summarize(d), want) {
		t.Fatalf("cancelled = %+v attempts %+v", d.Run, summarize(d))
	}
	p, err := h.store.WorkflowProposal(ctx, a.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	h.approve(Card{Proposal: p, ExpectedVersion: 1})
	if again := h.start(w.ID, billInputs("dinner-1", 1000, 3)); again.Existing || again.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("start of the freed key = %+v", again)
	}

	chat := newFakeChat(reply(`{"total_cents":1200,"confidence":"high"}`, 100, 10))
	h = newAIHarness(t, chat)
	rw := h.approveReceiptSplit(receiptSplitDir)
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "tip" {
			if _, err := h.store.ArchiveWorkspace(ctx, rw.ID); err != nil {
				return nil, err
			}
		}
		return v.CallFunction(step, input)
	}
	started, err := h.startReceipt(h.rt, rw.ID, "r-1", "")
	if err != nil {
		t.Fatal(err)
	}
	d = h.inspect(started.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(d.Run.Error, "the Workspace is archived") || len(d.Attempts) != 1 ||
		len(chat.captured()) != 0 || len(d.ModelCalls) != 0 {
		t.Fatalf("run = %+v attempts %+v requests %d", d.Run, summarize(d), len(chat.captured()))
	}
}

// The owner's audited rerun override reruns a succeeded logical execution
// as a new run whose Run Key, and so every Effect Intent's idempotency
// key, includes the override: its effects are performed again rather than
// answered with the earlier result. Repeating the override returns that
// rerun; a rerun of anything but a succeeded run of the same logical
// execution, without a reason, or from a model surface is refused.
func TestWorkflowRerunOverrideRunsASucceededKeyAgainWithoutSuppressingEffects(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger()
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	first := h.inspect(res.Run.Run.ID)
	if first.Run.State != eviedb.WorkflowRunSucceeded || ledger.balanceOf("account:household") != 250 {
		t.Fatalf("first run = %+v", first.Run)
	}
	if again, err := h.startPost(h.rt, w.ID, "dinner-1", false); err != nil || !again.Existing || again.Run.Run.ID != first.Run.ID {
		t.Fatalf("a duplicate start = %+v, %v", again, err)
	}
	cancelled, err := h.startPost(h.rt, w.ID, "dinner-3", true)
	if err != nil {
		t.Fatal(err)
	}
	h.cancel(h.rt, cancelled.Run.Run.ID, "", false)
	elsewhere := h.approvePostShare(postShareDir)
	rerun := func(of, reason string, inputs map[string]json.RawMessage, surface string) (RunResult, error) {
		return h.rt.StartRun(ctx, RunRequest{WorkspaceID: w.ID, Workflow: "post-share", Inputs: inputs, RerunOf: of, Reason: reason, Surface: surface})
	}
	for name, try := range map[string]func() (RunResult, error){
		"no reason":         func() (RunResult, error) { return rerun(first.Run.ID, "", nil, "cli") },
		"a reason alone":    func() (RunResult, error) { return rerun("", "again", postInputs("dinner-1", 1000, 4), "cli") },
		"another execution": func() (RunResult, error) { return rerun(first.Run.ID, "again", postInputs("dinner-2", 1000, 4), "cli") },
		"a cancelled run":   func() (RunResult, error) { return rerun(cancelled.Run.Run.ID, "again", nil, "cli") },
		"a missing run":     func() (RunResult, error) { return rerun("missing", "again", nil, "cli") },
		"a long reason":     func() (RunResult, error) { return rerun(first.Run.ID, strings.Repeat("x", 1001), nil, "cli") },
		"a model surface":   func() (RunResult, error) { return rerun(first.Run.ID, "again", nil, "chat") },
		"another Workspace": func() (RunResult, error) {
			return h.rt.StartRun(ctx, RunRequest{WorkspaceID: elsewhere.ID, Workflow: "post-share", RerunOf: first.Run.ID, Reason: "again", Surface: "cli"})
		},
	} {
		if _, err := try(); err == nil {
			t.Fatalf("%s: the rerun was accepted", name)
		} else if name == "no reason" && !errors.Is(err, ErrRerunRefused) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("a refused rerun performed: %d", performs)
	}
	second, err := rerun(first.Run.ID, "the ledger was restored from a backup", nil, "cli")
	if err != nil {
		t.Fatal(err)
	}
	s := second.Run
	if second.Existing || s.Run.State != eviedb.WorkflowRunSucceeded || s.Run.RunKey == first.Run.RunKey || s.Override == nil ||
		s.Override.RerunOf != first.Run.ID || s.Override.LogicalKey != first.Run.RunKey || s.Override.Reason != "the ledger was restored from a backup" ||
		s.Override.Surface != "cli" || s.Override.Actor != "owner" || string(s.Run.Inputs) != string(first.Run.Inputs) {
		t.Fatalf("rerun = %+v override %+v", s.Run, s.Override)
	}
	if len(s.Effects) != 1 || s.Effects[0].IdempotencyKey == first.Effects[0].IdempotencyKey {
		t.Fatalf("the rerun's effect shares the first run's idempotency key: %+v", s.Effects)
	}
	if performs, _ := ledger.counts(); performs != 2 || ledger.balanceOf("account:household") != 500 {
		t.Fatalf("the rerun's effect was suppressed: %d performs, balance %d", performs, ledger.balanceOf("account:household"))
	}
	if text := RunText(s); !strings.Contains(text, "Rerun of "+first.Run.ID) || !strings.Contains(text, "the ledger was restored from a backup") {
		t.Fatalf("inspection:\n%s", text)
	}
	again, err := rerun(first.Run.ID, "pressed twice", nil, "web")
	if err != nil || !again.Existing || again.Run.Run.ID != s.Run.ID {
		t.Fatalf("a repeated rerun = %+v, %v", again, err)
	}
	if held, err := h.startPost(h.rt, w.ID, "dinner-1", false); err != nil || !held.Existing || held.Run.Run.ID != first.Run.ID {
		t.Fatalf("a normal start of the key = %+v, %v", held, err)
	}
	third, err := rerun(s.Run.ID, "once more", postInputs("dinner-1", 1000, 4), "web")
	if err != nil || third.Existing || third.Run.Override.RerunOf != s.Run.ID || third.Run.Override.LogicalKey != first.Run.RunKey ||
		third.Run.Override.Surface != "web" {
		t.Fatalf("a rerun of the rerun = %+v, %v", third.Run.Override, err)
	}
	if performs, _ := ledger.counts(); performs != 3 {
		t.Fatalf("performs %d", performs)
	}
}

// Recovery of an answered question: a run whose answer was recorded by a
// surface that died before the run continued (its lease neither renewed
// nor released) is recovered like any running run once that lease
// expires: the next scheduler records the answer as the step's output and
// finishes the run.
func TestWorkflowAnsweredRunRecoversAfterTheAnsweringProcessDies(t *testing.T) {
	ctx := context.Background()
	chat := newFakeChat(reply(lowTotal, 120, 15))
	h := newAIHarness(t, chat)
	w := h.approveReceiptSplit(receiptSplitDir)
	res, err := h.startReceipt(h.rt, w.ID, "r-2", "")
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	if res.Run.Run.State != eviedb.WorkflowRunWaitingForHuman || len(res.Run.Interruptions) != 1 {
		t.Fatalf("start = %+v", res.Run.Run)
	}
	claim, err := h.store.AcquireWorkflowRunLease(ctx, id, "dead-answerer", testLease)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.AnswerWorkflowQuestion(ctx, claim.Lease, eviedb.WorkflowAnswer{RunID: id, Seq: res.Run.Interruptions[0].Seq,
		Answer: json.RawMessage(`{"total_cents":1300}`), Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	p := h.process()
	stop := p.rt.Start(ctx)
	time.Sleep(20 * time.Millisecond)
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunRunning || len(d.Attempts) != 3 {
		t.Fatalf("continued under a live lease: %+v attempts %+v", d.Run, summarize(d))
	}
	h.clock.Advance(testLease)
	eventually(t, "the answered run to finish", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Attempts) != 5 || d.Attempts[3].Step != "confirm" || string(d.Attempts[3].Output) != `{"total_cents":1300}` ||
		!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseTakenOver}) {
		t.Fatalf("run = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
}
