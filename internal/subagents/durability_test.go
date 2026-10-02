package subagents_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
)

// unfinished counts attempts that still claim to be admitted or running.
func unfinished(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM subagent_executions WHERE state IN ('admitted','running')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(gateTimeout)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func outcomes(results []delegation.Result) []string {
	var out []string
	for _, r := range results {
		out = append(out, r.Status+"/"+r.Reason)
	}
	sort.Strings(out)
	return out
}

func TestQueuedChildAtDeadlineIsReportedAsDeadline(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.PerParent = 1
	p.Deadline = 150 * time.Millisecond
	f := setup(t, p)
	var calls atomic.Int32
	release := make(chan struct{})
	// The started child ignores cancellation until released, so it holds the
	// only slot past the queued sibling's deadline. It then returns only once
	// its own deadline has fired, so its outcome never races that deadline.
	configure(t, f, clientFunc(func(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		calls.Add(1)
		<-release
		<-ctx.Done()
		return openrouter.ChatResponse{}, ctx.Err()
	}))
	done := make(chan []delegation.Result, 1)
	errs := make(chan error, 1)
	go func() {
		r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "a", Objective: "a"}, {Key: "b", Objective: "b"}})
		done <- r
		errs <- err
	}()
	waitUntil(t, "queued child to reach its deadline", func() bool {
		var n int
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM subagent_executions WHERE state NOT IN ('admitted','running')`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n == 1
	})
	close(release)
	results := <-done
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	got := outcomes(results)
	if len(got) != 2 || got[0] != "failed/deadline_limit" || got[1] != "failed/queue_deadline" || calls.Load() != 1 {
		t.Fatalf("queued deadline outcomes=%v calls=%d", got, calls.Load())
	}
	if n := unfinished(t, f); n != 0 {
		t.Fatalf("%d attempts left unfinished", n)
	}
}

func TestLateStarterReceivesItsFullDeadline(t *testing.T) {
	p := delegation.DefaultPolicy()
	p.PerParent = 1
	p.Deadline = time.Second
	f := setup(t, p)
	// Each child needs most of one deadline, so the second starts late.
	configure(t, f, clientFunc(func(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		select {
		case <-ctx.Done():
			return openrouter.ChatResponse{}, ctx.Err()
		case <-time.After(600 * time.Millisecond):
		}
		return response("evidence"), nil
	}))
	results, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "first", Objective: "a"}, {Key: "second", Objective: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := outcomes(results); len(got) != 2 || got[0] != "succeeded/" || got[1] != "succeeded/" {
		t.Fatalf("late starter lost part of its deadline: %v", got)
	}
}

func TestShutdownAndParentCancellationAreReportedDistinctly(t *testing.T) {
	for action, reason := range map[string]string{"parent_cancel": "parent_cancelled", "shutdown": "shutdown"} {
		t.Run(action, func(t *testing.T) {
			p := delegation.DefaultPolicy()
			p.PerParent = 1
			f := setup(t, p)
			g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
			configure(t, f, g)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan []delegation.Result, 1)
			go func() {
				r, _ := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "running", Objective: "a"}, {Key: "queued", Objective: "b"}})
				done <- r
			}()
			select {
			case <-g.entered:
			case <-time.After(gateTimeout):
				t.Fatal("child did not start")
			}
			switch action {
			case "parent_cancel":
				cancel()
			case "shutdown":
				stopCtx, stop := context.WithTimeout(context.Background(), gateTimeout)
				defer stop()
				if err := f.supervisor.Stop(stopCtx); err != nil {
					t.Fatal(err)
				}
			}
			var results []delegation.Result
			select {
			case results = <-done:
			case <-time.After(gateTimeout):
				t.Fatal("children did not stop")
			}
			if len(results) != 2 {
				t.Fatalf("missing outcomes: %+v", results)
			}
			for _, r := range results {
				if r.Status != "cancelled" || r.Reason != reason {
					t.Fatalf("%s reported as %s/%s", action, r.Status, r.Reason)
				}
			}
			if n := unfinished(t, f); n != 0 || g.calls != 1 {
				t.Fatalf("unfinished=%d calls=%d", n, g.calls)
			}
		})
	}
}

func TestWatchdogRetriesTransientStoreErrors(t *testing.T) {
	fault := &storeFault{}
	f := setupWith(t, delegation.DefaultPolicy(), fault)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	configure(t, f, clientFunc(func(ctx context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
		entered <- struct{}{}
		select {
		case <-ctx.Done():
			return openrouter.ChatResponse{}, ctx.Err()
		case <-release:
		}
		return response("evidence after contention"), nil
	}))
	done := make(chan []delegation.Result, 1)
	errs := make(chan error, 1)
	go func() {
		r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "busy", Objective: "research"}})
		done <- r
		errs <- err
	}()
	select {
	case <-entered:
	case <-time.After(gateTimeout):
		t.Fatal("child did not start")
	}
	// While the child waits on its provider, only the watchdog reads the attempt.
	fault.arm("FROM subagent_executions WHERE id=?", busyError{}, 3)
	fault.waitForHits(t, 3)
	close(release)
	results := <-done
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "succeeded" {
		t.Fatalf("transient store error ended healthy child: %+v", results[0])
	}
}

func TestAuthorityCheckBeforeModelCallRetriesTransientStoreErrors(t *testing.T) {
	fault := &storeFault{}
	f := setupWith(t, delegation.DefaultPolicy(), fault)
	// Only read-only authority checks prove the parent lease with this query;
	// the first ones gate the child's first model call.
	fault.arm("FROM session_turn_leases l JOIN sessions s", busyError{}, 3)
	results, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "busy-start", Objective: "research"}})
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "succeeded" || fault.hitCount() != 3 || f.client.calls != 1 {
		t.Fatalf("transient store error ended healthy child: %+v hits=%d calls=%d", results[0], fault.hitCount(), f.client.calls)
	}
}

func TestRunRecoverySurvivesTransientStoreErrors(t *testing.T) {
	fault := &storeFault{}
	f := setupWith(t, delegation.DefaultPolicy(), fault)
	ctx := context.Background()
	research, err := f.manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	requests := []delegation.Assignment{{Key: "abandoned", Objective: "research"}}
	parent, err := f.invoke(ctx, f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	attempts, err := f.store.AdmitSubagents(ctx, parent, requests, research.Receipt, delegation.DefaultPolicy(), "crashed-process")
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := f.store.StartSubagent(ctx, attempts[0].ID); err != nil || !started {
		t.Fatalf("start=%t %v", started, err)
	}
	fault.arm("FROM subagent_executions WHERE state IN ('admitted','running')", busyError{}, 2)
	var mu sync.Mutex
	var reported []error
	recoveryCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		done <- f.supervisor.RunRecovery(recoveryCtx, func(err error) {
			mu.Lock()
			defer mu.Unlock()
			reported = append(reported, err)
		})
	}()
	defer func() { cancel(); <-stopped }()
	if err = f.store.ReleaseTurnLease(ctx, f.parent.Scope.SessionID, f.parent.Lease.HolderID, f.parent.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "recovery after transient errors", func() bool {
		select {
		case <-stopped:
			t.Fatalf("recovery stopped: %v", <-done)
		default:
		}
		a, err := f.store.InspectSubagent(ctx, parent, attempts[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		return a.State == "interrupted"
	})
	mu.Lock()
	defer mu.Unlock()
	if fault.hitCount() != 2 || len(reported) == 0 || !errors.As(reported[0], new(busyError)) {
		t.Fatalf("transient recovery errors were not retried and reported: hits=%d reported=%v", fault.hitCount(), reported)
	}
}

// Statements the capacity wait and settlement perform, matched by the fault
// seam: each start attempt counts running attempts, and only the terminal
// write closes the child session.
const (
	capacityCount = "FROM subagent_executions WHERE state='running'"
	terminalWrite = "UPDATE sessions SET status='closed'"
)

// D5: a queued attempt's capacity wait performs only start attempts and, at
// its end, the terminal write. Faults in either leave the attempt waiting or
// settled, never stranded in admitted.
func TestCapacityWaitStoreFailuresNeverStrandAnAdmittedAttempt(t *testing.T) {
	type fault struct {
		match string
		err   error
		count int
	}
	for _, tc := range []struct {
		name    string
		faults  []fault
		settles bool // the queued sibling settles before the running child ends
		want    []string
	}{{
		// Lock contention on every start attempt keeps the sibling waiting
		// until the running child frees the slot.
		name:   "contended_starts_keep_waiting",
		faults: []fault{{capacityCount, busyError{}, 3}},
		want:   []string{"succeeded/", "succeeded/"},
	}, {
		// A start the store refuses settles the sibling, even when its
		// terminal write meets contention first.
		name:    "refused_start_with_contended_terminal_write",
		faults:  []fault{{capacityCount, errors.New("injected start failure"), 1}, {terminalWrite, busyError{}, 1}},
		settles: true,
		want:    []string{"failed/infrastructure_failure", "succeeded/"},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			faults := &storeFault{}
			p := delegation.DefaultPolicy()
			p.PerParent = 1
			f := setupWith(t, p, faults)
			g := &gatedProvider{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
			configure(t, f, g)
			done := make(chan []delegation.Result, 1)
			errs := make(chan error, 1)
			go func() {
				r, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "running", Objective: "a"}, {Key: "queued", Objective: "b"}})
				done <- r
				errs <- err
			}()
			select {
			case <-g.entered:
			case <-time.After(gateTimeout):
				t.Fatal("child did not start")
			}
			// Only the queued sibling starts or settles while the running
			// child is held, so every fault lands in its capacity wait.
			faults.arm(tc.faults[0].match, tc.faults[0].err, tc.faults[0].count)
			for _, extra := range tc.faults[1:] {
				faults.also(extra.match, extra.err, extra.count)
			}
			waitUntil(t, "faults in the capacity wait", func() bool {
				for _, fault := range tc.faults {
					if faults.hitsOf(fault.match) < fault.count {
						return false
					}
				}
				return true
			})
			if tc.settles {
				waitUntil(t, "queued sibling to settle", func() bool { return unfinished(t, f) == 1 })
			} else {
				g.release <- struct{}{}
				select {
				case <-g.entered:
				case <-time.After(gateTimeout):
					t.Fatal("queued child never started")
				}
			}
			g.release <- struct{}{}
			results := <-done
			if err := <-errs; err != nil {
				t.Fatalf("capacity wait lost its attempt: %v (unfinished=%d)", err, unfinished(t, f))
			}
			if got := outcomes(results); fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("outcomes=%v, want %v", got, tc.want)
			}
			if n := unfinished(t, f); n != 0 {
				t.Fatalf("%d attempts left unfinished", n)
			}
		})
	}
}

// One lock-contention error on an attempt's terminal write is retried: the
// child settles with its own outcome, and repeating its key replays that
// result at once.
func TestTerminalWriteRetriesLockContention(t *testing.T) {
	faults := &storeFault{}
	f := setupWith(t, delegation.DefaultPolicy(), faults)
	faults.arm(terminalWrite, busyError{}, 1)
	requests := []delegation.Assignment{{Key: "settles", Objective: "research"}}
	first, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if faults.hitsOf(terminalWrite) != 1 || first[0].Status != "succeeded" {
		t.Fatalf("terminal write contention: hits=%d result=%+v", faults.hitsOf(terminalWrite), first[0])
	}
	if n := unfinished(t, f); n != 0 {
		t.Fatalf("%d attempts left unfinished", n)
	}
	ctx, cancel := context.WithTimeout(context.Background(), gateTimeout)
	defer cancel()
	again, err := f.delegate(t, ctx, f.parent, requests)
	if err != nil || again[0].Status != "succeeded" || !again[0].Replayed || again[0].ExecutionID != first[0].ExecutionID || f.client.calls != 1 {
		t.Fatalf("same-key retry: %+v %v calls=%d", again, err, f.client.calls)
	}
}

// An attempt whose terminal write still fails after the bounded retries is
// handed over rather than left running until its deadlines: repeating its
// key fails at once with the reason while the store refuses the write, and
// settles it with the outcome its owner decided once the store accepts it.
func TestSameKeyRetrySettlesAnUnsettledAttemptPromptly(t *testing.T) {
	faults := &storeFault{}
	f := setupWith(t, delegation.DefaultPolicy(), faults)
	subagents.SetSettleWindowForTest(f.supervisor, 100*time.Millisecond)
	faults.arm(terminalWrite, busyError{}, -1)
	requests := []delegation.Assignment{{Key: "stuck", Objective: "research"}}
	first, err := f.delegate(t, context.Background(), f.parent, requests)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].Status != delegation.StatusError || !strings.Contains(first[0].Error, `"stuck"`) || first[0].Summary != "" {
		t.Fatalf("unsettled attempt delivered: %+v", first[0])
	}
	// Each retry must answer within this bound; before the handover a retry
	// waited out two 15-minute deadlines.
	ctx, cancel := context.WithTimeout(context.Background(), gateTimeout)
	defer cancel()
	again, err := f.delegate(t, ctx, f.parent, requests)
	if err != nil || again[0].Status != delegation.StatusError || !strings.Contains(again[0].Error, "has not settled") ||
		!strings.Contains(again[0].Error, "SQLITE_BUSY") {
		t.Fatalf("retry while the write is refused: %+v %v", again, err)
	}
	faults.disarm()
	ctx, cancel = context.WithTimeout(context.Background(), gateTimeout)
	defer cancel()
	settled, err := f.delegate(t, ctx, f.parent, requests)
	if err != nil || settled[0].Status != "succeeded" || !settled[0].Replayed || settled[0].ExecutionID != first[0].ExecutionID ||
		!strings.Contains(settled[0].Summary, "Research finding") || f.client.calls != 1 {
		t.Fatalf("retry after the store recovered: %+v %v calls=%d", settled, err, f.client.calls)
	}
	if n := unfinished(t, f); n != 0 {
		t.Fatalf("%d attempts left unfinished", n)
	}
}

// Recovery settles an attempt whose terminal write failed while its parent
// turn is still live, so the attempt stops holding its running slot.
func TestRecoverySettlesAnUnsettledAttemptAndFreesItsSlot(t *testing.T) {
	faults := &storeFault{}
	p := delegation.DefaultPolicy()
	p.PerParent = 1
	f := setupWith(t, p, faults)
	subagents.SetSettleWindowForTest(f.supervisor, 100*time.Millisecond)
	faults.arm(terminalWrite, busyError{}, -1)
	first, err := f.delegate(t, context.Background(), f.parent, []delegation.Assignment{{Key: "stuck", Objective: "research"}})
	if err != nil || first[0].Status != delegation.StatusError {
		t.Fatalf("first: %+v %v", first, err)
	}
	if n := unfinished(t, f); n != 1 {
		t.Fatalf("unsettled attempts = %d", n)
	}
	faults.disarm()
	recoveryCtx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = f.supervisor.RunRecovery(recoveryCtx, nil)
	}()
	defer func() { cancel(); <-stopped }()
	waitUntil(t, "recovery to settle the attempt", func() bool { return unfinished(t, f) == 0 })
	var state string
	if err := f.db.QueryRow(`SELECT state FROM subagent_executions WHERE id=?`, first[0].ExecutionID).Scan(&state); err != nil || state != "succeeded" {
		t.Fatalf("settled state=%q %v", state, err)
	}
	// The parent's only running slot is free again.
	ctx, stop := context.WithTimeout(context.Background(), gateTimeout)
	defer stop()
	next, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "next", Objective: "research"}})
	if err != nil || next[0].Status != "succeeded" {
		t.Fatalf("next: %+v %v", next, err)
	}
}

func TestDelegationProceedsPastAnUnrecoverableRecord(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	ctx := context.Background()
	research, err := f.manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := f.store.CreateGlobalSessionWithComposition(ctx, research.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES('corrupt',?,'corrupt-key','digest',?,'admitted','{not json')`, f.parent.Scope.SessionID, orphan.ID); err != nil {
		t.Fatal(err)
	}
	results, err := f.delegate(t, ctx, f.parent, []delegation.Assignment{{Key: "healthy", Objective: "research"}})
	if err != nil {
		t.Fatalf("one bad record blocked delegation: %v", err)
	}
	if results[0].Status != "succeeded" {
		t.Fatalf("outcome: %+v", results[0])
	}
}
