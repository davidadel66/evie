package eviedb

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func requireCompilerHostWork(t *testing.T, f *workerFixture, config CompilerSupervisorConfig, want compilerWork) {
	t.Helper()
	got, err := compilerHostWork(context.Background(), f.db, config)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("host work = %d, want %d", got, want)
	}
}

// runCompilerHostUntilStopped runs the host in the background for the rest of
// the test.
func runCompilerHostUntilStopped(t *testing.T, f *workerFixture, config CompilerSupervisorConfig) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.store.RunCompilerHost(ctx, config) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("host shutdown exceeded cleanup bound")
		}
	})
}

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(within)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not happen within %s", what, within)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

const liveRootsSQL = `SELECT COUNT(*) FROM memory_compiler_activation_roots WHERE state='deferred_live'`

func TestCompilerHostWorkClassifiesDurableWork(t *testing.T) {
	f := newWorkerFixture(t)
	config := f.config(&workerScript{})
	activationStart(t, f)
	activationReconcile(t, f, 4)
	requireCompilerHostWork(t, f, config, compilerWorkNone)

	// A new message is unscanned evidence: ready.
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Still live."})
	requireCompilerHostWork(t, f, config, compilerWorkReady)

	// Discovered but not yet selected, its root is ready only for a host that
	// can extract its generation.
	if err := withImmediateTransaction(context.Background(), f.db, func(conn *sql.Conn) error {
		var result memory.CompilerReconciliation
		return discoverCompilerEvidenceInTransaction(context.Background(), conn, &result)
	}); err != nil {
		t.Fatal(err)
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activation_roots WHERE state='selected_unmaterialized'`); got != 1 {
		t.Fatalf("discovery left %d selected roots, want 1", got)
	}
	requireCompilerHostWork(t, f, config, compilerWorkReady)
	requireCompilerHostWork(t, f, CompilerSupervisorConfig{}, compilerWorkWaiting)

	// Its turn is still in progress, so only time or the lease can move it.
	activationReconcile(t, f, 4)
	if got := activationCount(t, f.db, liveRootsSQL); got != 1 {
		t.Fatalf("live roots = %d, want 1", got)
	}
	requireCompilerHostWork(t, f, config, compilerWorkWaiting)

	// Releasing the lease closes the turn; once selected, nothing remains.
	if err := f.store.ReleaseTurnLease(context.Background(), f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	activationReconcile(t, f, 4)
	if got := activationCount(t, f.db, liveRootsSQL); got != 0 {
		t.Fatalf("live roots after release = %d, want 0", got)
	}
	requireCompilerHostWork(t, f, config, compilerWorkNone)
}

// Before, a turn in progress was re-checked, under the write lock, on every
// 25ms tick: 40 times a second.
func TestCompilerHostRechecksATurnInProgressOnceASecond(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Still live."})
	activationReconcile(t, f, 4)
	if got := activationCount(t, f.db, liveRootsSQL); got != 1 {
		t.Fatalf("live roots = %d, want 1", got)
	}
	checks := func() int {
		return activationCount(t, f.db, `SELECT COALESCE(MAX(checked_order),0) FROM memory_compiler_activation_roots`)
	}
	before := checks()
	runCompilerHostUntilStopped(t, f, f.config(&workerScript{}))
	time.Sleep(1600 * time.Millisecond)
	if rechecks := checks() - before; rechecks < 1 || rechecks > 3 {
		t.Fatalf("turn in progress was re-checked %d times in 1.6s, want about 2", rechecks)
	}
}

func TestCompilerHostPicksUpNewWorkAndFinishedTurns(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	activationReconcile(t, f, 4)
	runCompilerHostUntilStopped(t, f, f.config(&workerScript{run: func(ctx context.Context, _ memory.CompilerRequest) (CompilerExtraction, error) {
		<-ctx.Done()
		return CompilerExtraction{}, ctx.Err()
	}}))
	time.Sleep(300 * time.Millisecond) // idle, looking every 250ms

	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Still live."})
	eventually(t, 2*time.Second, "an idle host discovering a new turn", func() bool {
		return activationCount(t, f.db, liveRootsSQL) == 1
	})

	if err := f.store.ReleaseTurnLease(context.Background(), f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	eventually(t, 3*time.Second, "a released turn leaving the waiting state", func() bool {
		return activationCount(t, f.db, liveRootsSQL) == 0
	})
}

func TestCompilerHostReportsAFailingReconciliationOnce(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	activationReconcile(t, f, 4)
	var mu sync.Mutex
	var reports []error
	config := f.config(&workerScript{})
	config.ReconcileHealth = func(err error) {
		mu.Lock()
		defer mu.Unlock()
		reports = append(reports, err)
	}
	reported := func() []error {
		mu.Lock()
		defer mu.Unlock()
		return append([]error(nil), reports...)
	}
	runCompilerHostUntilStopped(t, f, config)

	if _, err := f.db.Exec(`ALTER TABLE memory_compiler_activation_dirty RENAME TO memory_compiler_activation_dirty_hidden`); err != nil {
		t.Fatal(err)
	}
	eventually(t, 2*time.Second, "a failure report", func() bool { return len(reported()) > 0 })
	time.Sleep(time.Second) // several more failing ticks
	if got := reported(); len(got) != 1 || got[0] == nil {
		t.Fatalf("a persistent failure was reported %d times: %v", len(got), got)
	}

	if _, err := f.db.Exec(`ALTER TABLE memory_compiler_activation_dirty_hidden RENAME TO memory_compiler_activation_dirty`); err != nil {
		t.Fatal(err)
	}
	eventually(t, 3*time.Second, "a recovery report", func() bool { return len(reported()) > 1 })
	time.Sleep(500 * time.Millisecond)
	if got := reported(); len(got) != 2 || got[1] != nil {
		t.Fatalf("recovery was not reported exactly once: %v", got)
	}
}
