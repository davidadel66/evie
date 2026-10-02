package subagents_test

import (
	"context"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/subagents"
)

// promptly bounds a hand-off that must not wait out retries or a contended
// write: generous for race builds, far below the 3-second write timeout and
// the 5-second settle window it replaces.
const promptly = time.Second

// mainStopBudget is the shutdown budget cmd/evie gives Supervisor.Stop.
const mainStopBudget = 5 * time.Second

// delegateInBackground starts one assignment and reports when it returns.
func delegateInBackground(t *testing.T, f *fixture, ctx context.Context) <-chan time.Time {
	t.Helper()
	done := make(chan time.Time, 1)
	go func() {
		_, _ = f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "k", Objective: "research"}})
		done <- time.Now()
	}()
	return done
}

func awaitReturn(t *testing.T, done <-chan time.Time) time.Time {
	t.Helper()
	select {
	case at := <-done:
		return at
	case <-time.After(3 * mainStopBudget):
		t.Fatal("Delegate did not return")
		return time.Time{}
	}
}

func stopWithMainBudget(t *testing.T, f *fixture) (time.Duration, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), mainStopBudget)
	defer cancel()
	start := time.Now()
	err := f.supervisor.Stop(ctx)
	return time.Since(start), err
}

// Confirmation review (C): the bounded terminal-write retries ignored
// shutdown, so Stop during a contended write took 4.6 s (formerly 1.5 ms),
// and with the write lock held Stop spent its whole 5 s budget while Delegate
// returned after 6 s. On shutdown an owner makes at most the one bounded
// write its outcome needs, stops retrying at once, and hands the outcome to
// recovery.
func TestStopDoesNotWaitOutTerminalWriteRetries(t *testing.T) {
	for _, tc := range []struct {
		name string
		// fault is armed after the child starts; beforeOutcome arms it before
		// the child finishes, so Stop lands during the owner's retries.
		fault         error
		beforeOutcome bool
		stopWithin    time.Duration
	}{
		// The outcome is decided by shutdown: one write, then the hand-off.
		{name: "shutdown outcome, write refused", fault: busyError{}, stopWithin: promptly},
		// One write bounded by its 3-second timeout, never a second.
		{name: "shutdown outcome, write lock held", fault: slowBusy{10 * time.Second}, stopWithin: 4 * time.Second},
		// The owner is between retries when Stop arrives.
		{name: "Stop during retries", fault: busyError{}, beforeOutcome: true, stopWithin: promptly},
		// The owner's write is waiting on the lock when Stop arrives.
		{name: "Stop during a held-lock write", fault: slowBusy{10 * time.Second}, beforeOutcome: true, stopWithin: promptly},
	} {
		t.Run(tc.name, func(t *testing.T) {
			faults := &storeFault{}
			f := setupWith(t, delegation.DefaultPolicy(), faults)
			g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
			configure(t, f, g)
			done := delegateInBackground(t, f, context.Background())
			select {
			case <-g.entered:
			case <-time.After(gateTimeout):
				t.Fatal("child did not start")
			}
			faults.arm(terminalWrite, tc.fault, -1)
			if tc.beforeOutcome {
				g.release <- struct{}{}
				hits := 2 // retrying
				if _, slow := tc.fault.(slowBusy); slow {
					hits = 1 // waiting on the lock
				}
				faults.waitForHits(t, hits)
			}
			start := time.Now()
			took, err := stopWithMainBudget(t, f)
			returned := awaitReturn(t, done).Sub(start)
			t.Logf("Stop took %v; Delegate returned after %v", took, returned)
			if err != nil {
				t.Fatalf("Stop spent its %v budget: %v after %v", mainStopBudget, err, took)
			}
			if took > tc.stopWithin || returned > tc.stopWithin {
				t.Fatalf("Stop took %v and Delegate returned after %v; want both within %v", took, returned, tc.stopWithin)
			}
			if n := subagents.PendingForTest(f.supervisor); n != 1 {
				t.Fatalf("unrecorded outcomes handed to recovery = %d, want 1", n)
			}

			// Recovery records the handed-over outcome once the store accepts it.
			faults.disarm()
			recoveryCtx, cancel := context.WithCancel(context.Background())
			stopped := make(chan struct{})
			go func() { defer close(stopped); _ = f.supervisor.RunRecovery(recoveryCtx, nil) }()
			defer func() { cancel(); <-stopped }()
			waitUntil(t, "recovery to record the outcome", func() bool { return unfinished(t, f) == 0 })
			want := "cancelled"
			if tc.beforeOutcome {
				want = "succeeded"
			}
			var state string
			if err := f.db.QueryRow(`SELECT state FROM subagent_executions`).Scan(&state); err != nil || state != want {
				t.Fatalf("recorded state %q (%v), want the owner's outcome %q", state, err, want)
			}
		})
	}
}

// A cancelled parent call no longer waits out the 5-second settle window
// either: after the one write its outcome needs, an owner whose caller is gone
// hands the outcome to recovery.
func TestParentCancellationHandsAnUnrecordedOutcomeToRecoveryAtOnce(t *testing.T) {
	faults := &storeFault{}
	f := setupWith(t, delegation.DefaultPolicy(), faults)
	g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
	configure(t, f, g)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := delegateInBackground(t, f, ctx)
	select {
	case <-g.entered:
	case <-time.After(gateTimeout):
		t.Fatal("child did not start")
	}
	faults.arm(terminalWrite, busyError{}, -1)
	start := time.Now()
	cancel()
	returned := awaitReturn(t, done).Sub(start)
	t.Logf("Delegate returned %v after its parent was cancelled", returned)
	if returned > promptly {
		t.Fatalf("Delegate returned %v after cancellation, want within %v", returned, promptly)
	}
	if n := subagents.PendingForTest(f.supervisor); n != 1 {
		t.Fatalf("unrecorded outcomes handed to recovery = %d, want 1", n)
	}
}

// Confirmation review (D): each recovery pass recorded pending outcomes one
// context-free 3-second write at a time, so RunRecovery returned 8.9 s after
// cancellation with three pending outcomes, and shutdown waits for it.
func TestRunRecoveryStopsPromptlyWhenCancelled(t *testing.T) {
	faults := &storeFault{}
	p := delegation.DefaultPolicy()
	p.PerParent = 3
	f := setupWith(t, p, faults)
	subagents.SetSettleWindowForTest(f.supervisor, 100*time.Millisecond)
	faults.arm(terminalWrite, busyError{}, -1)
	results, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{
		{Key: "a", Objective: "research"}, {Key: "b", Objective: "research"}, {Key: "c", Objective: "research"}})
	if err != nil {
		t.Fatal(err)
	}
	if n := subagents.PendingForTest(f.supervisor); n != 3 || len(results) != 3 {
		t.Fatalf("pending outcomes = %d for %d results, want 3", n, len(results))
	}
	faults.arm(terminalWrite, slowBusy{time.Minute}, -1)
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() { defer close(stopped); _ = f.supervisor.RunRecovery(ctx, nil) }()
	faults.waitForHits(t, 1) // a pass is waiting on the write lock
	start := time.Now()
	cancel()
	select {
	case <-stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("RunRecovery did not return after cancellation")
	}
	took := time.Since(start)
	t.Logf("RunRecovery returned %v after cancellation", took)
	if took > promptly {
		t.Fatalf("RunRecovery returned %v after cancellation, want within %v", took, promptly)
	}
	if n := subagents.PendingForTest(f.supervisor); n != 3 {
		t.Fatalf("cancellation dropped pending outcomes: %d left, want 3", n)
	}
}
