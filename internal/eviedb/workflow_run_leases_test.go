package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/workflow"
)

// leaseClock is the store's clock in lease tests, so expiry and backoff are
// exact rather than raced against the wall clock.
type leaseClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *leaseClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *leaseClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func openLeasedRunDB(t *testing.T) (*runDB, *leaseClock) {
	t.Helper()
	clock := &leaseClock{now: time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)}
	r := openRunDB(t)
	r.clock = clock.Now
	r.s.UseClock(clock.Now, time.UTC)
	return r, clock
}

const leaseFor = 30 * time.Second

func splitAttempt(runID string, checkpoint int64, attempt int) WorkflowAttemptStart {
	return WorkflowAttemptStart{RunID: runID, Checkpoint: checkpoint, Step: "split", Kind: workflow.KindDeterministic, Attempt: attempt, Input: json.RawMessage(`{"total":1000}`)}
}

func eventKinds(events []WorkflowRunEvent) []WorkflowRunEventKind {
	out := []WorkflowRunEventKind{}
	for _, e := range events {
		out = append(out, e.Kind)
	}
	return out
}

func sameKinds(got []WorkflowRunEventKind, want ...WorkflowRunEventKind) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Every attempt begin, finish and run failure is fenced by the run's current
// lease: no lease, an expired lease and a lease another holder took over all
// write nothing. A takeover records the earlier holder's in-flight attempt
// as interrupted before anything else happens, and the fencing token only
// grows.
func TestWorkflowRunLeaseFencesEveryWrite(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.BeginWorkflowAttempt(ctx, WorkflowRunLease{RunID: run.ID, Holder: "holder-a", Token: 1}, splitAttempt(run.ID, 0, 1)); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("attempt without a lease: %v", err)
	}
	start := clock.Now()
	claimA, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if l := claimA.Lease; l.Token != 1 || l.Holder != "holder-a" || !l.ExpiresAt.Equal(start.Add(leaseFor)) || !l.HeartbeatAt.Equal(start) || claimA.TookOver != nil || len(claimA.Interrupted) != 0 {
		t.Fatalf("claim A = %+v", claimA)
	}
	if _, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor); !errors.Is(err, ErrWorkflowRunLeaseHeld) {
		t.Fatalf("second holder while the lease is live: %v", err)
	}
	attempt, err := r.s.BeginWorkflowAttempt(ctx, claimA.Lease, splitAttempt(run.ID, 0, 1))
	if err != nil || attempt.LeaseHolder != "holder-a" || attempt.LeaseToken != 1 {
		t.Fatalf("attempt under A = %+v, %v", attempt, err)
	}
	clock.Advance(10 * time.Second)
	renewed, err := r.s.HeartbeatWorkflowRunLease(ctx, claimA.Lease, leaseFor)
	if err != nil || !renewed.ExpiresAt.Equal(start.Add(40*time.Second)) || !renewed.HeartbeatAt.Equal(start.Add(10*time.Second)) || renewed.Token != 1 {
		t.Fatalf("heartbeat = %+v, %v", renewed, err)
	}
	if _, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor); !errors.Is(err, ErrWorkflowRunLeaseHeld) {
		t.Fatalf("takeover of a renewed lease: %v", err)
	}

	// An expired lease writes nothing, even before anyone takes over.
	clock.Advance(31 * time.Second)
	if _, err := r.s.FinishWorkflowAttempt(ctx, claimA.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: attempt.Seq, Output: json.RawMessage(`{}`), Next: "check", State: json.RawMessage(`{"split":{}}`)}); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("finish under an expired lease: %v", err)
	}
	if _, err := r.s.HeartbeatWorkflowRunLease(ctx, claimA.Lease, leaseFor); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("heartbeat of an expired lease: %v", err)
	}

	// Another process takes over: A's attempt is recorded interrupted.
	r.reopen()
	claimB, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if claimB.Lease.Token != 2 || claimB.TookOver == nil || claimB.TookOver.Holder != "holder-a" || claimB.TookOver.Token != 1 ||
		len(claimB.Interrupted) != 1 || claimB.Interrupted[0].Seq != attempt.Seq || claimB.Interrupted[0].State != WorkflowAttemptInterrupted {
		t.Fatalf("claim B = %+v", claimB)
	}
	if e := claimB.Interrupted[0].Error; !strings.Contains(e, "interrupted") || !strings.Contains(e, "holder-a") {
		t.Fatalf("interrupted attempt error = %q", e)
	}
	// Every write of the stale holder is refused.
	if _, err := r.s.FinishWorkflowAttempt(ctx, claimA.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: attempt.Seq, Error: "late"}); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("stale finish: %v", err)
	}
	if _, err := r.s.BeginWorkflowAttempt(ctx, claimA.Lease, splitAttempt(run.ID, 0, 2)); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("stale begin: %v", err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, claimA.Lease, 0, "stale"); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("stale fail: %v", err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, claimA.Lease, "stale", time.Minute); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("stale pause: %v", err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, claimA.Lease, ""); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("stale release: %v", err)
	}
	// B retries from the same checkpoint.
	retry, err := r.s.BeginWorkflowAttempt(ctx, claimB.Lease, splitAttempt(run.ID, 0, 2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, claimB.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: retry.Seq, Output: json.RawMessage(`{"share":333}`), Next: "check", State: json.RawMessage(`{"split":{"share":333}}`)}); err != nil {
		t.Fatal(err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Attempts) != 2 || d.Attempts[0].State != WorkflowAttemptInterrupted || d.Attempts[0].LeaseHolder != "holder-a" || d.Attempts[0].FinishedAt == nil ||
		d.Attempts[1].State != WorkflowAttemptSucceeded || d.Attempts[1].LeaseToken != 2 || d.Run.Checkpoint != 1 {
		t.Fatalf("attempts = %+v", d.Attempts)
	}
	if d.Lease == nil || d.Lease.Holder != "holder-b" || d.Lease.Token != 2 || d.Lease.ReleasedAt != nil {
		t.Fatalf("lease = %+v", d.Lease)
	}
	if !sameKinds(eventKinds(d.Events), WorkflowEventLeaseTakenOver, WorkflowEventAttemptInterrupted) ||
		d.Events[0].Holder != "holder-b" || d.Events[0].Token != 2 || !strings.Contains(d.Events[0].Detail, "holder-a") || d.Events[1].Attempt != attempt.Seq {
		t.Fatalf("events = %+v", d.Events)
	}
	// History and the token's order are enforced by the database.
	for _, statement := range []string{
		`UPDATE workflow_run_leases SET fencing_token=1`,
		`DELETE FROM workflow_run_leases`,
		`DELETE FROM workflow_run_events`,
		`UPDATE workflow_run_events SET detail='x'`,
		`UPDATE workflow_node_attempts SET state='running',finished_at=NULL WHERE seq=1`,
	} {
		if _, err := r.db.ExecContext(ctx, statement); err == nil {
			t.Fatalf("the database accepted %s", statement)
		}
	}

	// Release frees the lease at once: the next holder takes it without a
	// takeover, and the released lease writes nothing.
	if err := r.s.ReleaseWorkflowRunLease(ctx, claimB.Lease, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.HeartbeatWorkflowRunLease(ctx, claimB.Lease, leaseFor); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("heartbeat after release: %v", err)
	}
	claimC, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-c", leaseFor)
	if err != nil || claimC.Lease.Token != 3 || claimC.TookOver != nil || len(claimC.Interrupted) != 0 {
		t.Fatalf("claim after release = %+v, %v", claimC, err)
	}
	if d, _ := r.s.WorkflowRun(ctx, run.ID); len(d.Events) != 2 {
		t.Fatalf("a clean release recorded events: %+v", d.Events)
	}
}

// A failed attempt with retries left records a durable not-before time in
// the transaction that records the failure. No holder can claim the run
// before it, across a reopen; once it passes the run is due again.
func TestWorkflowRunRetryWaitsForItsDurableNotBefore(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(run.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, claim.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: attempt.Seq, Error: "boom", RunError: "steps.split failed", RetryAfter: time.Minute}); err == nil {
		t.Fatal("a failed run was given a retry time")
	}
	due := clock.Now().Add(time.Minute)
	failed, err := r.s.FinishWorkflowAttempt(ctx, claim.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: attempt.Seq, Error: "boom", RetryAfter: time.Minute})
	if err != nil || failed.NotBefore == nil || !failed.NotBefore.Equal(due) || failed.State != WorkflowRunRunning {
		t.Fatalf("failed attempt = %+v, %v", failed, err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, claim.Lease, ""); err != nil {
		t.Fatal(err)
	}
	r.reopen()
	if ids, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(ids) != 0 {
		t.Fatalf("due before its retry time = %v, %v", ids, err)
	}
	_, err = r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	var notDue *WorkflowRunNotDueError
	if !errors.Is(err, ErrWorkflowRunNotDue) || !errors.As(err, &notDue) || !notDue.NotBefore.Equal(due) {
		t.Fatalf("claim before the retry time: %v", err)
	}
	clock.Advance(time.Minute)
	if ids, err := r.s.DueWorkflowRuns(ctx, 10); err != nil || len(ids) != 1 || ids[0] != run.ID {
		t.Fatalf("due at its retry time = %v, %v", ids, err)
	}
	claimB, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil || claimB.TookOver != nil || len(claimB.Interrupted) != 0 {
		t.Fatalf("claim at the retry time = %+v, %v", claimB, err)
	}
	if _, err := r.s.BeginWorkflowAttempt(ctx, claimB.Lease, splitAttempt(run.ID, 0, 2)); err != nil {
		t.Fatal(err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || d.Run.NotBefore != nil {
		t.Fatalf("the retry did not clear its not-before time: %+v, %v", d.Run, err)
	}
	if !sameKinds(eventKinds(d.Events), WorkflowEventRetryScheduled) || !strings.Contains(d.Events[0].Detail, "split") {
		t.Fatalf("events = %+v", d.Events)
	}
}

// The scheduler's due list is every queued run, every run whose lease
// expired or was released, and every paused run whose recheck time came,
// oldest first; a live lease, a finished run and a pending retry are not
// due.
func TestWorkflowDueRunsAreQueuedExpiredOrRecheckedRuns(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	start := func(key string) WorkflowRun {
		run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, key))
		if err != nil {
			t.Fatal(err)
		}
		return run
	}
	claim := func(run WorkflowRun) WorkflowRunLease {
		c, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
		if err != nil {
			t.Fatal(err)
		}
		return c.Lease
	}
	queued, held, expired, paused, done := start("q"), start("h"), start("e"), start("p"), start("d")
	claim(expired)
	pausedLease := claim(paused)
	if _, err := r.s.PauseWorkflowRun(ctx, pausedLease, "waiting for a test", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, pausedLease, ""); err != nil {
		t.Fatal(err)
	}
	doneLease := claim(done)
	at, err := r.s.BeginWorkflowAttempt(ctx, doneLease, WorkflowAttemptStart{RunID: done.ID, Checkpoint: 0, Step: "split", Kind: workflow.KindDeterministic, Attempt: 1, Input: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, doneLease, WorkflowAttemptEnd{RunID: done.ID, Seq: at.Seq, Output: json.RawMessage(`1`), State: json.RawMessage(`{"split":1}`), Outputs: json.RawMessage(`{"x":1}`)}); err != nil {
		t.Fatal(err)
	}
	clock.Advance(31 * time.Second)
	claim(held)
	ids, err := r.s.DueWorkflowRuns(ctx, 10)
	if err != nil || strings.Join(ids, ",") != queued.ID+","+expired.ID {
		t.Fatalf("due = %v, %v", ids, err)
	}
	clock.Advance(29 * time.Second)
	ids, err = r.s.DueWorkflowRuns(ctx, 10)
	if err != nil || strings.Join(ids, ",") != queued.ID+","+expired.ID+","+paused.ID {
		t.Fatalf("due after the recheck time = %v, %v", ids, err)
	}
	if ids, err := r.s.DueWorkflowRuns(ctx, 1); err != nil || len(ids) != 1 || ids[0] != queued.ID {
		t.Fatalf("due with a limit = %v, %v", ids, err)
	}
}

// A pause is visible on the run (waiting_for_dependency with its reason and
// recheck time) and recorded once per reason. Resuming re-applies run
// admission in its transaction: the definition must still admit runs, and
// the run's own pinned version must be neither deactivated nor blocked.
func TestWorkflowRunPauseIsVisibleAndResumeReappliesAdmission(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	w, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := r.s.ResumeWorkflowRun(ctx, claim.Lease); err != nil || got.State != WorkflowRunQueued {
		t.Fatalf("admitting a queued run = %+v, %v", got, err)
	}
	recheck := clock.Now().Add(time.Minute)
	paused, err := r.s.PauseWorkflowRun(ctx, claim.Lease, "the pinned runtime is not compatible", time.Minute)
	if err != nil || paused.State != WorkflowRunWaitingForDependency || paused.Error != "the pinned runtime is not compatible" || paused.NotBefore == nil || !paused.NotBefore.Equal(recheck) {
		t.Fatalf("paused = %+v, %v", paused, err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, claim.Lease, "the pinned runtime is not compatible", time.Minute); err != nil {
		t.Fatal(err)
	}
	resumed, err := r.s.ResumeWorkflowRun(ctx, claim.Lease)
	if err != nil || resumed.State != WorkflowRunRunning || resumed.Error != "" || resumed.NotBefore != nil {
		t.Fatalf("resumed = %+v, %v", resumed, err)
	}
	d, _ := r.s.WorkflowRun(ctx, run.ID)
	if !sameKinds(eventKinds(d.Events), WorkflowEventPaused, WorkflowEventResumed) || d.Events[0].Detail != "the pinned runtime is not compatible" {
		t.Fatalf("events = %+v", d.Events)
	}
	// A pause cannot hide an attempt in flight.
	attempt, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(run.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, claim.Lease, "mid-node", time.Minute); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("pause with an attempt in flight: %v", err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, claim.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: attempt.Seq, Error: "boom"}); err != nil {
		t.Fatal(err)
	}

	// Deactivation pauses at resume; approving again does not revive a run
	// whose own version was deactivated.
	if _, err := r.s.DeactivateWorkflow(ctx, w.ID, a.Name, a.Version, "retired", "cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.ResumeWorkflowRun(ctx, claim.Lease); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("resume after deactivation: %v", err)
	}
	p, err := r.s.WorkflowProposal(ctx, a.ProposalID)
	if err != nil {
		t.Fatal(err)
	}
	activate(t, r.s, p, 1)
	if _, err := r.s.ResumeWorkflowRun(ctx, claim.Lease); !errors.Is(err, ErrWorkflowNotActive) || !strings.Contains(err.Error(), "version 1") {
		t.Fatalf("resume of a deactivated version after a new approval: %v", err)
	}

	// A blocked definition pauses too.
	w2, a2 := splitBillActivation(t, r.s)
	other, _, err := r.s.StartWorkflowRun(ctx, runStart(a2, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	otherClaim, err := r.s.AcquireWorkflowRunLease(ctx, other.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.ArchiveWorkspace(ctx, w2.ID); err != nil {
		t.Fatal(err)
	}
	var blocked *WorkflowBlockedError
	if _, err := r.s.ResumeWorkflowRun(ctx, otherClaim.Lease); !errors.As(err, &blocked) || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("resume in an archived Workspace: %v", err)
	}
	if _, err := r.s.ResumeWorkflowRun(ctx, WorkflowRunLease{RunID: other.ID, Holder: "holder-a", Token: 99}); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("resume under a stale lease: %v", err)
	}
}

// Releasing a lease at shutdown, while the run still has work to do, is
// recorded so inspection can tell a clean stop from a crash.
func TestWorkflowRunShutdownReleaseIsRecorded(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, claim.Lease, "the runtime stopped"); err != nil {
		t.Fatal(err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || d.Lease == nil || d.Lease.ReleasedAt == nil || !sameKinds(eventKinds(d.Events), WorkflowEventLeaseReleased) || d.Events[0].Detail != "the runtime stopped" {
		t.Fatalf("detail = %+v, %v", d, err)
	}

	// A finished run has no work to hand over: its release records nothing.
	done, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k2"))
	if err != nil {
		t.Fatal(err)
	}
	finished, err := r.s.AcquireWorkflowRunLease(ctx, done.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	at, err := r.s.BeginWorkflowAttempt(ctx, finished.Lease, splitAttempt(done.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, finished.Lease, WorkflowAttemptEnd{RunID: done.ID, Seq: at.Seq, Output: json.RawMessage(`1`), State: json.RawMessage(`{"split":1}`), Outputs: json.RawMessage(`{"x":1}`)}); err != nil {
		t.Fatal(err)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, finished.Lease, "the runtime stopped"); err != nil {
		t.Fatal(err)
	}
	if d, err := r.s.WorkflowRun(ctx, done.ID); err != nil || len(d.Events) != 0 || d.Lease.ReleasedAt == nil {
		t.Fatalf("finished run's release = %+v, %v", d, err)
	}
}

// subStage3Shape puts a database back in its sub-stage 3 shape: node
// attempts without lease columns or the interrupted state, runs without a
// not-before time, and no lease or event tables.
const subStage3Shape = `CREATE TABLE workflow_node_attempts_old (
 run_id TEXT NOT NULL REFERENCES workflow_runs(id),
 seq INTEGER NOT NULL CHECK(seq > 0),
 step_id TEXT NOT NULL CHECK(length(step_id) BETWEEN 1 AND 64),
 kind TEXT NOT NULL CHECK(kind IN ('deterministic','capability','ai','agent','map','branch','interrupt')),
 attempt INTEGER NOT NULL CHECK(attempt > 0),
 input_checkpoint INTEGER NOT NULL,
 input TEXT NOT NULL CHECK(json_valid(input)),
 state TEXT NOT NULL CHECK(state IN ('running','succeeded','failed')),
 output TEXT CHECK(output IS NULL OR json_valid(output)),
 error TEXT NOT NULL,
 next_step TEXT NOT NULL,
 note TEXT NOT NULL,
 started_at TEXT NOT NULL,
 finished_at TEXT,
 PRIMARY KEY(run_id, seq),
 FOREIGN KEY(run_id, input_checkpoint) REFERENCES workflow_run_checkpoints(run_id, seq),
 CHECK((state = 'running') = (finished_at IS NULL)),
 CHECK(state <> 'succeeded' OR output IS NOT NULL),
 CHECK(state <> 'failed' OR (error <> '' AND output IS NULL)));
 INSERT INTO workflow_node_attempts_old SELECT run_id,seq,step_id,kind,attempt,input_checkpoint,input,state,output,error,next_step,note,started_at,finished_at FROM workflow_node_attempts;
 DROP TABLE workflow_node_attempts;
 ALTER TABLE workflow_node_attempts_old RENAME TO workflow_node_attempts;
 CREATE UNIQUE INDEX workflow_node_attempts_one_in_flight ON workflow_node_attempts(run_id) WHERE state = 'running';
 CREATE UNIQUE INDEX workflow_node_attempts_numbered ON workflow_node_attempts(run_id, input_checkpoint, step_id, attempt);
 CREATE TRIGGER workflow_node_attempts_finished BEFORE UPDATE ON workflow_node_attempts
 WHEN OLD.state <> 'running'
 BEGIN SELECT RAISE(ABORT,'finished workflow node attempts are immutable'); END;
 DROP TABLE workflow_run_events;
 DROP TABLE workflow_run_leases;
 DROP TRIGGER workflow_runs_terminal;
 CREATE TRIGGER workflow_runs_terminal BEFORE UPDATE ON workflow_runs
 WHEN OLD.state IN ('succeeded','failed','cancelled')
 BEGIN SELECT RAISE(ABORT,'terminal workflow runs are immutable'); END;
 ALTER TABLE workflow_runs DROP COLUMN not_before;`

// A database written by sub-stage 3 is upgraded in place: its attempts keep
// every field, and an attempt a crash left in flight is recorded
// interrupted by the first claim, so the run recovers.
func TestWorkflowNodeAttemptsUpgradeFromTheSubStage3Schema(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(run.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.ExecContext(ctx, subStage3Shape); err != nil {
		t.Fatal(err)
	}
	r.reopen()
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil || p.InFlight == nil || p.InFlight.Seq != attempt.Seq || p.InFlight.LeaseToken != 0 || p.InFlight.LeaseHolder != "" || string(p.InFlight.Input) != `{"total":1000}` {
		t.Fatalf("upgraded progress = %+v, %v", p, err)
	}
	claimB, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil || claimB.Lease.Token != 1 || len(claimB.Interrupted) != 1 || claimB.Interrupted[0].Seq != attempt.Seq {
		t.Fatalf("claim after the upgrade = %+v, %v", claimB, err)
	}
	r.reopen()
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || len(d.Attempts) != 1 || d.Attempts[0].State != WorkflowAttemptInterrupted || d.Lease == nil || d.Lease.Holder != "holder-b" {
		t.Fatalf("after a second open = %+v, %v", d, err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_node_attempts SET note='x' WHERE seq=1`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("the upgraded table lost its triggers: %v", err)
	}
}
