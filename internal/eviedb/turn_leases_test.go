package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func setTurnLeaseTime(store *Store, now time.Time) {
	store.now = func() time.Time { return now }
}

func noOpTurnLeaseWrite(turnLeaseWriteExecutor) error { return nil }

func TestTurnLeaseLifecycleRejectsStaleTokens(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Date(2026, 8, 22, 12, 0, 0, 123456789, time.UTC)
	setTurnLeaseTime(store, now)
	first, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", 10*time.Second)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if first.SessionID != session.ID || first.HolderID != "worker-a" ||
		first.FencingToken != 1 || first.Generation != 1 ||
		!first.ExpiresAt.Equal(now.Add(10*time.Second)) {
		t.Fatalf("first lease = %+v", first)
	}
	if !first.UnexpiredAt(now) || first.UnexpiredAt(first.ExpiresAt) {
		t.Errorf("lease activity is not half-open at expiry: %+v", first)
	}

	observed, err := store.GetTurnLease(ctx, session.ID)
	if err != nil {
		t.Fatalf("get lease: %v", err)
	}
	if observed != first {
		t.Errorf("observed lease = %+v, want %+v", observed, first)
	}
	setTurnLeaseTime(store, now.Add(time.Second))
	if err := store.withTurnLeaseWrite(ctx, session.ID, first.HolderID, first.FencingToken, noOpTurnLeaseWrite); err != nil {
		t.Fatalf("authorize current lease: %v", err)
	}
	if _, err := store.AcquireTurnLease(ctx, session.ID, "worker-b", 10*time.Second); !errors.Is(err, ErrTurnLeaseHeld) {
		t.Fatalf("competing acquire error = %v, want ErrTurnLeaseHeld", err)
	}

	setTurnLeaseTime(store, now.Add(2*time.Second))
	renewed, err := store.HeartbeatTurnLease(
		ctx,
		session.ID,
		first.HolderID,
		first.FencingToken,
		10*time.Second,
	)
	if err != nil {
		t.Fatalf("heartbeat lease: %v", err)
	}
	if renewed.FencingToken != first.FencingToken || renewed.Generation != first.Generation ||
		!renewed.ExpiresAt.Equal(now.Add(12*time.Second)) {
		t.Errorf("renewed lease = %+v", renewed)
	}
	setTurnLeaseTime(store, now.Add(3*time.Second))
	preserved, err := store.HeartbeatTurnLease(
		ctx,
		session.ID,
		renewed.HolderID,
		renewed.FencingToken,
		time.Second,
	)
	if err != nil {
		t.Fatalf("heartbeat with shorter window: %v", err)
	}
	if preserved.ExpiresAt != renewed.ExpiresAt {
		t.Errorf("short heartbeat expiry = %v, want preserved %v", preserved.ExpiresAt, renewed.ExpiresAt)
	}

	staleToken := first.FencingToken + 1
	setTurnLeaseTime(store, now.Add(3*time.Second))
	if _, err := store.HeartbeatTurnLease(ctx, session.ID, first.HolderID, staleToken, 10*time.Second); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("stale heartbeat error = %v, want ErrTurnLeaseLost", err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, first.HolderID, staleToken); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("stale release error = %v, want ErrTurnLeaseLost", err)
	}
	if err := store.withTurnLeaseWrite(ctx, session.ID, first.HolderID, staleToken, noOpTurnLeaseWrite); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("stale authorization error = %v, want ErrTurnLeaseLost", err)
	}

	if err := store.ReleaseTurnLease(ctx, session.ID, renewed.HolderID, renewed.FencingToken); err != nil {
		t.Fatalf("release current lease: %v", err)
	}
	if _, err := store.GetTurnLease(ctx, session.ID); !errors.Is(err, ErrTurnLeaseNotHeld) {
		t.Fatalf("get released lease error = %v, want ErrTurnLeaseNotHeld", err)
	}
	if err := store.withTurnLeaseWrite(ctx, session.ID, renewed.HolderID, renewed.FencingToken, noOpTurnLeaseWrite); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("released authorization error = %v, want ErrTurnLeaseLost", err)
	}

	second, err := store.AcquireTurnLease(ctx, session.ID, "worker-b", 10*time.Second)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if second.FencingToken <= first.FencingToken || second.Generation <= first.Generation {
		t.Errorf("replacement lease did not advance monotonically: first=%+v second=%+v", first, second)
	}
	if second.FencingToken != memory.FencingToken(second.Generation) {
		t.Errorf("replacement lease epoch diverged: token=%d generation=%d", second.FencingToken, second.Generation)
	}
	setTurnLeaseTime(store, now.Add(4*time.Second))
	if !first.UnexpiredAt(now.Add(4 * time.Second)) {
		t.Error("stale snapshot should remain locally unexpired")
	}
	staleCallbackRan := false
	if err := store.withTurnLeaseWrite(ctx, session.ID, first.HolderID, first.FencingToken, func(turnLeaseWriteExecutor) error {
		staleCallbackRan = true
		return nil
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("stale snapshot write error = %v, want ErrTurnLeaseLost", err)
	}
	if staleCallbackRan {
		t.Error("locally unexpired stale snapshot authorized a write")
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, first.HolderID, first.FencingToken); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("replaced holder released current lease: %v", err)
	}
}

func TestTurnLeaseRejectsInvalidAndOverflowingDurations(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Date(2026, 8, 22, 12, 30, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	for _, duration := range []time.Duration{0, -time.Nanosecond} {
		if _, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", duration); err == nil {
			t.Errorf("acquire duration %v succeeded, want validation error", duration)
		}
	}

	setTurnLeaseTime(store, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC))
	if _, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", time.Nanosecond); err == nil {
		t.Error("acquire with overflowing duration succeeded")
	}

	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire valid lease: %v", err)
	}
	for _, duration := range []time.Duration{0, -time.Nanosecond} {
		if _, err := store.HeartbeatTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken, duration); err == nil {
			t.Errorf("heartbeat duration %v succeeded, want validation error", duration)
		}
	}

	setTurnLeaseTime(store, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC))
	if _, err := store.HeartbeatTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken, time.Nanosecond); err == nil {
		t.Error("heartbeat with overflowing duration succeeded")
	}

	observed, err := store.GetTurnLease(ctx, session.ID)
	if err != nil {
		t.Fatalf("get lease after invalid heartbeats: %v", err)
	}
	if observed != lease {
		t.Errorf("invalid heartbeat mutated lease: got %+v want %+v", observed, lease)
	}
}

func TestExpiredTurnLeaseReplacementIsAtomicAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	stores := make([]*Store, 3)
	for i := range stores {
		db, err := OpenDBAt(path)
		if err != nil {
			t.Fatalf("open database %d: %v", i+1, err)
		}
		t.Cleanup(func() { db.Close() })
		stores[i] = NewStore(db)
	}

	ctx := context.Background()
	session, err := stores[0].CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	now := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	setTurnLeaseTime(stores[0], now)
	expired, err := stores[0].AcquireTurnLease(ctx, session.ID, "expired-worker", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire expiring lease: %v", err)
	}

	type result struct {
		lease memory.TurnLease
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for _, store := range stores {
		setTurnLeaseTime(store, expired.ExpiresAt)
	}
	for i, holder := range []memory.LeaseHolderID{"worker-b", "worker-c"} {
		workers.Add(1)
		go func(store *Store, holder memory.LeaseHolderID) {
			defer workers.Done()
			<-start
			lease, err := store.AcquireTurnLease(ctx, session.ID, holder, 10*time.Second)
			results <- result{lease: lease, err: err}
		}(stores[i+1], holder)
	}
	close(start)
	workers.Wait()
	close(results)

	var winner memory.TurnLease
	var acquired, held int
	for result := range results {
		switch {
		case result.err == nil:
			acquired++
			winner = result.lease
		case errors.Is(result.err, ErrTurnLeaseHeld):
			held++
		default:
			t.Fatalf("competing acquire returned unexpected error: %v", result.err)
		}
	}
	if acquired != 1 || held != 1 {
		t.Fatalf("acquired=%d held=%d, want one atomic winner and one conflict", acquired, held)
	}
	if winner.FencingToken <= expired.FencingToken || winner.Generation <= expired.Generation {
		t.Errorf("winner did not advance fencing values: expired=%+v winner=%+v", expired, winner)
	}

	for i, store := range stores {
		observed, err := store.GetTurnLease(ctx, session.ID)
		if err != nil {
			t.Fatalf("store %d observe winner: %v", i+1, err)
		}
		if observed != winner {
			t.Errorf("store %d observed %+v, want winner %+v", i+1, observed, winner)
		}
	}
	if _, err := stores[0].HeartbeatTurnLease(ctx, session.ID, expired.HolderID, expired.FencingToken, time.Second); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("expired heartbeat error = %v, want ErrTurnLeaseLost", err)
	}
	if err := stores[0].ReleaseTurnLease(ctx, session.ID, expired.HolderID, expired.FencingToken); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("expired release error = %v, want ErrTurnLeaseLost", err)
	}
	if err := stores[0].withTurnLeaseWrite(ctx, session.ID, expired.HolderID, expired.FencingToken, noOpTurnLeaseWrite); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("expired authorization error = %v, want ErrTurnLeaseLost", err)
	}
}

func TestExpiredTurnLeaseCannotBeRenewedReleasedOrAuthorized(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Date(2026, 8, 22, 13, 30, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}

	setTurnLeaseTime(store, lease.ExpiresAt)
	if _, err := store.HeartbeatTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken, time.Second); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("heartbeat at expiry error = %v, want ErrTurnLeaseLost", err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("release at expiry error = %v, want ErrTurnLeaseLost", err)
	}
	if err := store.withTurnLeaseWrite(ctx, session.ID, lease.HolderID, lease.FencingToken, noOpTurnLeaseWrite); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("authorization at expiry error = %v, want ErrTurnLeaseLost", err)
	}
}

func TestTurnLeaseWriteFenceRollsBackBeforeExpiredTakeover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("open database A: %v", err)
	}
	t.Cleanup(func() { dbA.Close() })
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("open database B: %v", err)
	}
	t.Cleanup(func() { dbB.Close() })
	dbB.SetMaxOpenConns(1)
	if _, err := dbB.Exec(`PRAGMA busy_timeout=0`); err != nil {
		t.Fatalf("disable contender busy timeout: %v", err)
	}

	storeA := NewStore(dbA)
	storeB := NewStore(dbB)
	ctx := context.Background()
	if _, err := dbA.Exec(`CREATE TABLE lease_write_probe (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create write probe: %v", err)
	}
	session, err := storeA.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Date(2026, 8, 22, 13, 45, 0, 0, time.UTC)
	setTurnLeaseTime(storeA, now)
	lease, err := storeA.AcquireTurnLease(ctx, session.ID, "worker-a", 5*time.Second)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	setTurnLeaseTime(storeB, lease.ExpiresAt)

	var takeoverSucceededInsideFence bool
	err = storeA.withTurnLeaseWrite(ctx, session.ID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		if _, err := writer.execContext(ctx, `INSERT INTO lease_write_probe (value) VALUES ('stale')`); err != nil {
			return err
		}
		if _, err := storeB.AcquireTurnLease(ctx, session.ID, "worker-b", time.Minute); err == nil {
			takeoverSucceededInsideFence = true
		}
		setTurnLeaseTime(storeA, lease.ExpiresAt)
		return nil
	})
	if takeoverSucceededInsideFence {
		t.Fatal("expired takeover succeeded while the fenced write transaction was open")
	}
	if !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("write crossing expiry error = %v, want ErrTurnLeaseLost", err)
	}

	var staleWrites int
	if err := dbA.QueryRow(`SELECT COUNT(*) FROM lease_write_probe WHERE value = 'stale'`).Scan(&staleWrites); err != nil {
		t.Fatalf("count stale writes: %v", err)
	}
	if staleWrites != 0 {
		t.Fatalf("stale writes = %d, want rolled back", staleWrites)
	}

	winner, err := storeB.AcquireTurnLease(ctx, session.ID, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("acquire after fenced rollback: %v", err)
	}
	staleCallbackRan := false
	if err := storeA.withTurnLeaseWrite(ctx, session.ID, lease.HolderID, lease.FencingToken, func(turnLeaseWriteExecutor) error {
		staleCallbackRan = true
		return nil
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("stale write error = %v, want ErrTurnLeaseLost", err)
	}
	if staleCallbackRan {
		t.Fatal("stale lease write callback ran after takeover")
	}
	if err := storeB.withTurnLeaseWrite(ctx, session.ID, winner.HolderID, winner.FencingToken, func(writer turnLeaseWriteExecutor) error {
		_, err := writer.execContext(ctx, `INSERT INTO lease_write_probe (value) VALUES ('winner')`)
		return err
	}); err != nil {
		t.Fatalf("winner write: %v", err)
	}

	var winnerWrites int
	if err := dbA.QueryRow(`SELECT COUNT(*) FROM lease_write_probe WHERE value = 'winner'`).Scan(&winnerWrites); err != nil {
		t.Fatalf("count winner writes: %v", err)
	}
	if winnerWrites != 1 {
		t.Fatalf("winner writes = %d, want 1", winnerWrites)
	}
}

func TestTurnLeaseClockIsSampledUnderWriterLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("open database A: %v", err)
	}
	t.Cleanup(func() { dbA.Close() })
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("open database B: %v", err)
	}
	t.Cleanup(func() { dbB.Close() })
	dbB.SetMaxOpenConns(1)
	if _, err := dbB.Exec(`PRAGMA busy_timeout=0`); err != nil {
		t.Fatalf("disable contender busy timeout: %v", err)
	}

	store := NewStore(dbA)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Date(2026, 8, 22, 13, 50, 0, 0, time.UTC)
	clockSamples := 0
	store.now = func() time.Time {
		clockSamples++
		if _, err := dbB.ExecContext(ctx, `BEGIN IMMEDIATE`); err == nil {
			if _, rollbackErr := dbB.ExecContext(ctx, `ROLLBACK`); rollbackErr != nil {
				t.Errorf("rollback contender transaction: %v", rollbackErr)
			}
			t.Error("lease clock was sampled before SQLite write ownership")
		}
		return now
	}

	lease, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	now = now.Add(time.Second)
	if _, err := store.HeartbeatTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken, time.Minute); err != nil {
		t.Fatalf("heartbeat lease: %v", err)
	}
	if err := store.withTurnLeaseWrite(ctx, session.ID, lease.HolderID, lease.FencingToken, noOpTurnLeaseWrite); err != nil {
		t.Fatalf("fenced write: %v", err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatalf("release lease: %v", err)
	}
	if clockSamples != 5 {
		t.Fatalf("clock samples = %d, want 5", clockSamples)
	}
}

func TestTurnLeaseWriteExecutorCannotOutliveFinalFence(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	if _, err := db.Exec(`CREATE TABLE lease_early_commit_probe (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create write probe: %v", err)
	}
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	now := time.Date(2026, 8, 22, 13, 55, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	var exposedCommit bool
	var capturedWriter turnLeaseWriteExecutor
	err = store.withTurnLeaseWrite(ctx, session.ID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		capturedWriter = writer
		if _, ok := any(writer).(interface{ Commit() error }); ok {
			exposedCommit = true
		}
		if _, err := writer.execContext(ctx, `INSERT INTO lease_early_commit_probe (value) VALUES ('stale')`); err != nil {
			return err
		}
		setTurnLeaseTime(store, lease.ExpiresAt)
		return nil
	})
	if exposedCommit {
		t.Error("turn lease writer exposed Commit")
	}
	if _, err := capturedWriter.execContext(ctx, `INSERT INTO lease_early_commit_probe (value) VALUES ('late')`); !errors.Is(err, errTurnLeaseWriterClosed) {
		t.Errorf("closed writer error = %v, want errTurnLeaseWriterClosed", err)
	}
	if !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("early commit error = %v, want ErrTurnLeaseLost", err)
	}

	var staleWrites int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lease_early_commit_probe`).Scan(&staleWrites); err != nil {
		t.Fatalf("count early writes: %v", err)
	}
	if staleWrites != 0 {
		t.Fatalf("early committed writes = %d, want rolled back", staleWrites)
	}
}

func TestClosedSessionCannotAcquireRenewOrAuthorizeLease(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	now := time.Date(2026, 8, 22, 13, 58, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)

	closedBeforeAcquire, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create closed session: %v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, closedBeforeAcquire.ID); err != nil {
		t.Fatalf("close session before acquire: %v", err)
	}
	if _, err := store.AcquireTurnLease(ctx, closedBeforeAcquire.ID, "worker-a", time.Minute); !errors.Is(err, ErrTurnLeaseSessionInactive) {
		t.Fatalf("closed session acquire error = %v, want ErrTurnLeaseSessionInactive", err)
	}

	closedWhileHeld, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create active session: %v", err)
	}
	lease, err := store.AcquireTurnLease(ctx, closedWhileHeld.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire active session: %v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, closedWhileHeld.ID); err != nil {
		t.Fatalf("close held session: %v", err)
	}

	if _, err := store.HeartbeatTurnLease(ctx, closedWhileHeld.ID, lease.HolderID, lease.FencingToken, time.Minute); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("closed session heartbeat error = %v, want ErrTurnLeaseLost", err)
	}
	callbackRan := false
	if err := store.withTurnLeaseWrite(ctx, closedWhileHeld.ID, lease.HolderID, lease.FencingToken, func(turnLeaseWriteExecutor) error {
		callbackRan = true
		return nil
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Errorf("closed session write error = %v, want ErrTurnLeaseLost", err)
	}
	if callbackRan {
		t.Error("closed session write callback ran")
	}
	if err := store.ReleaseTurnLease(ctx, closedWhileHeld.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatalf("release closed session lease: %v", err)
	}
}

func TestTurnLeaseSurvivesRestartAndKeepsMonotonicFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	ctx := context.Background()
	now := time.Date(2026, 8, 22, 14, 0, 0, 0, time.UTC)

	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	setTurnLeaseTime(store, now)
	first, err := store.AcquireTurnLease(ctx, session.ID, "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	store = NewStore(db)
	setTurnLeaseTime(store, now.Add(time.Second))
	observed, err := store.GetTurnLease(ctx, session.ID)
	if err != nil {
		t.Fatalf("get lease after restart: %v", err)
	}
	if observed != first {
		t.Fatalf("lease after restart = %+v, want %+v", observed, first)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, first.HolderID, first.FencingToken); err != nil {
		t.Fatalf("release after restart: %v", err)
	}
	setTurnLeaseTime(store, now.Add(2*time.Second))
	second, err := store.AcquireTurnLease(ctx, session.ID, "worker-b", time.Minute)
	if err != nil {
		t.Fatalf("acquire after restart: %v", err)
	}
	if second.FencingToken <= first.FencingToken || second.Generation <= first.Generation {
		t.Errorf("fence reset across restart: first=%+v second=%+v", first, second)
	}
}

func TestSessionTurnLeaseSchemaConstraints(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	tests := []struct {
		name       string
		sessionID  memory.SessionID
		holder     any
		token      int64
		generation int64
		expires    any
	}{
		{name: "missing session", sessionID: "missing", holder: "worker", token: 1, generation: 1, expires: "2026-08-22T12:00:00.000000000Z"},
		{name: "empty holder", sessionID: session.ID, holder: "", token: 1, generation: 1, expires: "2026-08-22T12:00:00.000000000Z"},
		{name: "nonpositive fencing token", sessionID: session.ID, holder: "worker", token: 0, generation: 1, expires: "2026-08-22T12:00:00.000000000Z"},
		{name: "nonpositive generation", sessionID: session.ID, holder: "worker", token: 1, generation: 0, expires: "2026-08-22T12:00:00.000000000Z"},
		{name: "mismatched acquisition epoch", sessionID: session.ID, holder: "worker", token: 1, generation: 2, expires: "2026-08-22T12:00:00.000000000Z"},
		{name: "holder without expiry", sessionID: session.ID, holder: "worker", token: 1, generation: 1, expires: nil},
		{name: "expiry without holder", sessionID: session.ID, holder: nil, token: 1, generation: 1, expires: "2026-08-22T12:00:00.000000000Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.Exec(`
				INSERT INTO session_turn_leases (
					session_id, holder_id, fencing_token, lease_generation, expires_at
				) VALUES (?, ?, ?, ?, ?)
			`, tt.sessionID, tt.holder, tt.token, tt.generation, tt.expires); err == nil {
				t.Fatal("invalid lease row was accepted")
			}
		})
	}
}
