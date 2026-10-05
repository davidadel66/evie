package workflowruntime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflow"
)

// Cancellation and the audited rerun override (Stage 14 sub-stage 8,
// 2026-10-04; ADR 0017, 0021, 0029).
//
// Cancelling a run records the owner's cancellation and moves the run to
// cancelling at once, whoever holds it: no further node starts. What is in
// flight ends by its kind. A deterministic function finishes and is
// recorded (it is pure and bounded, and takes no context); a capability
// read or an AI call is stopped through its context, at once when the
// cancellation came from this process and otherwise within one scheduler
// poll, and its attempt is recorded interrupted (an AI call counts at its
// estimate, since it may have been billed); a question is withdrawn
// unanswered; an effect is never stopped: it completes and its receipt is
// recorded, or its outcome becomes unknown and the run stays cancelling
// until its receipt (the provider's reconciliation or the owner's) is
// recorded, without any retry. The runtime that holds or next claims the
// run then finishes the cancellation: the run is cancelled, its completed
// nodes and effects stay recorded and its Run Key is free. Finishing it
// calls nothing, so it needs neither admission nor the approved
// definition: a paused run whose version was deactivated, blocked or no
// longer matches its approval can always be cancelled.
//
// A rerun override starts a new run of a logical execution that already
// succeeded (ADR 0021): only from the owner's surfaces, only of a
// succeeded run of the same Workspace, workflow and logical Run Key, and
// only with a reason, all recorded with the run. The new run's Run Key
// includes the override (the logical key and the run it reruns), so the
// database's one-run-per-key rule holds, repeating the override returns
// that rerun, and every Effect Intent's idempotency key, derived from the
// Run Key, differs from the earlier run's: an idempotent provider performs
// the rerun's effects instead of answering with the earlier result.

// ErrRerunRefused refuses a rerun override that does not name a
// succeeded run of the same logical execution with a reason; the store
// refuses with the same error.
var ErrRerunRefused = eviedb.ErrWorkflowRerunRefused

// CancelRequest is the owner cancelling a run, with an optional reason.
// Detach records the cancellation and returns at once; otherwise Cancel
// waits until the run is cancelled, or until it can only finish once an
// effect is reconciled.
type CancelRequest struct {
	RunID   string `json:"run_id"`
	Reason  string `json:"reason,omitempty"`
	Detach  bool   `json:"detach,omitempty"`
	Surface string `json:"-"`
}

// cancelRun records the owner's cancellation of an unfinished run and lets
// the runtime holding it, or the next to claim it, finish it: at once for
// a queued, paused or waiting run, after the node in flight for a running
// one, and once its effect is reconciled for a run with an effect whose
// outcome is unknown. Cancelling a cancelling or cancelled run returns it
// unchanged; a run that succeeded or failed is refused
// (ErrWorkflowRunFinished) and never changed. Only the owner's CLI and web
// surfaces cancel; no model-facing tool does.
func (r *Runtime) cancelRun(ctx context.Context, req CancelRequest) (eviedb.WorkflowRunDetail, error) {
	if !ownerSurface(req.Surface) {
		return eviedb.WorkflowRunDetail{}, eviedb.ErrWorkflowApprover
	}
	writes := context.WithoutCancel(ctx)
	if _, err := r.store.RequestWorkflowRunCancellation(writes, eviedb.WorkflowRunCancellationRequest{RunID: req.RunID, Reason: req.Reason, Surface: req.Surface}); err != nil {
		return eviedb.WorkflowRunDetail{}, err
	}
	r.signalCancel(req.RunID)
	r.wake()
	if req.Detach {
		return r.store.WorkflowRun(writes, req.RunID)
	}
	return r.await(ctx, req.RunID)
}

// cancelSignal ends a pass's cancelled context when the owner's
// cancellation of its run is seen.
type cancelSignal struct {
	ctx  context.Context
	fire context.CancelFunc
}

// listenForCancel registers the pass of a run in this process, so a
// cancellation requested here reaches its node in flight at once; the
// returned stop unregisters it.
func (r *Runtime) listenForCancel(runID string) (*cancelSignal, func()) {
	ctx, fire := context.WithCancel(context.Background())
	s := &cancelSignal{ctx: ctx, fire: fire}
	r.mu.Lock()
	r.signals[runID] = s
	r.mu.Unlock()
	return s, func() {
		r.mu.Lock()
		if r.signals[runID] == s {
			delete(r.signals, runID)
		}
		r.mu.Unlock()
		fire()
	}
}

// signalCancel reaches this process's pass of the run, if it runs one.
func (r *Runtime) signalCancel(runID string) {
	r.mu.Lock()
	s := r.signals[runID]
	r.mu.Unlock()
	if s != nil {
		s.fire()
	}
}

// cancellable is ctx ending also when the owner's cancellation of the run
// is seen: at once when it was requested in this process, otherwise when a
// check of the run's state, one each scheduler poll while the call runs,
// finds it cancelling. stop ends the watch.
func (r *Runtime) cancellable(pc *pass, ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	if pc.cancel == nil {
		return ctx, cancel
	}
	stopSignal := context.AfterFunc(pc.cancel.ctx, cancel)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(r.sched.Poll)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if state, err := r.store.WorkflowRunState(ctx, pc.lease.RunID); err == nil && state == eviedb.WorkflowRunCancelling {
				pc.cancel.fire()
				return
			}
		}
	}()
	return ctx, func() {
		cancel()
		stopSignal()
		<-done
	}
}

// cancelled reports whether the owner's cancellation reached this pass.
func (pc *pass) cancelled() bool {
	return pc.cancel != nil && pc.cancel.ctx.Err() != nil
}

// continueCancellation is a claim of a cancelling run: with no effect
// unresolved it finishes the cancellation at once; with an effect whose
// outcome is unknown it waits for that effect's receipt, reconciling it
// automatically when that is due, exactly as a run that needs
// reconciliation does.
func (r *Runtime) continueCancellation(ctx, writes context.Context, lease eviedb.WorkflowRunLease, lost context.Context, p eviedb.WorkflowRunProgress, signal *cancelSignal) (passEnd, error) {
	if p.Unresolved > 0 {
		return r.continueReconciliation(ctx, writes, lease, lost, p, signal)
	}
	if _, err := r.store.FinishWorkflowCancellation(writes, lease); err != nil {
		return 0, err
	}
	return passDone, nil
}

// rerunKey is a rerun's Run Key: the logical execution's Run Key and the
// run the override reruns, so repeating the override finds the same run
// and the rerun's Effect Intents get idempotency keys of their own.
func rerunKey(name, logical, rerunOf string) (string, error) {
	data, err := workflow.EncodeValue(map[string]any{"run_key": logical, "rerun_of": rerunOf})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return name + ":rerun:sha256:" + hex.EncodeToString(sum[:]), nil
}

// rerunBase reads the succeeded run a rerun override names and checks it
// can be rerun from this request: the same Workspace and workflow, a
// succeeded run, and a reason. It returns the run and the logical Run Key
// it belongs to.
func (r *Runtime) rerunBase(ctx context.Context, req RunRequest, workflowName string) (eviedb.WorkflowRunDetail, string, error) {
	refuse := func(format string, args ...any) (eviedb.WorkflowRunDetail, string, error) {
		return eviedb.WorkflowRunDetail{}, "", fmt.Errorf("%w: %s", ErrRerunRefused, fmt.Sprintf(format, args...))
	}
	reason := strings.TrimSpace(req.Reason)
	switch {
	case req.RerunOf == "":
		return refuse("a reason is recorded only with --rerun-of, the succeeded run the override reruns")
	case reason == "":
		return refuse("a rerun of run %s needs a reason, which is recorded with it", req.RerunOf)
	case len(reason) > eviedb.MaxWorkflowCancelReason:
		return refuse("a rerun's reason is at most %d bytes", eviedb.MaxWorkflowCancelReason)
	}
	prior, err := r.store.WorkflowRun(ctx, req.RerunOf)
	if err != nil {
		return eviedb.WorkflowRunDetail{}, "", err
	}
	switch run := prior.Run; {
	case run.WorkspaceID != req.WorkspaceID || run.Workflow != workflowName:
		return refuse("run %s is a run of %s in Workspace %s, not of %s in Workspace %s", run.ID, run.Workflow, run.WorkspaceID, workflowName, req.WorkspaceID)
	case run.State == eviedb.WorkflowRunFailed || run.State == eviedb.WorkflowRunCancelled:
		return refuse("run %s is %s, so its Run Key is free: start it again without --rerun-of", run.ID, run.State)
	case run.State != eviedb.WorkflowRunSucceeded:
		return refuse("run %s is %s: only a succeeded run's Run Key needs an override", run.ID, run.State)
	}
	// A rerun's logical key, or (sub-stage 12) a successor's.
	return prior, logicalRunKey(prior), nil
}
