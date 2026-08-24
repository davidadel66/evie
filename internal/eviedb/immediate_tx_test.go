package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestImmediateTransactionCancellationBeforeCommitRollsBack(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.Exec(`CREATE TABLE immediate_tx_probe (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create transaction probe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	err := withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `INSERT INTO immediate_tx_probe (value) VALUES ('uncommitted')`); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("transaction error = %v, want context.Canceled", err)
	}

	var writes int
	if err := db.QueryRow(`SELECT COUNT(*) FROM immediate_tx_probe`).Scan(&writes); err != nil {
		t.Fatalf("count transaction writes: %v", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want cancellation rollback", writes)
	}
}

type transactionResolutionObservation struct {
	statement   string
	deadline    time.Time
	hasDeadline bool
}

func TestTransactionResolutionContextIgnoresCancellationButKeepsDeadline(t *testing.T) {
	deadlineCtx, stopDeadline := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	defer stopDeadline()
	wantDeadline, _ := deadlineCtx.Deadline()
	callerCtx, cancelCaller := context.WithCancel(deadlineCtx)
	cancelCaller()

	resolutionCtx, cancelResolution := transactionResolutionContext(callerCtx)
	defer cancelResolution()
	if err := resolutionCtx.Err(); err != nil {
		t.Fatalf("resolution context inherited caller cancellation: %v", err)
	}
	gotDeadline, ok := resolutionCtx.Deadline()
	if !ok || !gotDeadline.Equal(wantDeadline) {
		t.Fatalf("resolution deadline=%v present=%v, want %v", gotDeadline, ok, wantDeadline)
	}
}

func scriptStoreResolutionDeadline(
	store *Store,
) (<-chan transactionResolutionObservation, <-chan struct{}, func()) {
	observed := make(chan transactionResolutionObservation, 2)
	commitEntered := make(chan struct{})
	releaseCommit := make(chan struct{})
	store.resolveImmediateTransaction = func(
		ctx context.Context,
		_ *sql.Conn,
		statement string,
	) (sql.Result, error) {
		deadline, ok := ctx.Deadline()
		observed <- transactionResolutionObservation{
			statement: statement, deadline: deadline, hasDeadline: ok,
		}
		if statement == `COMMIT` {
			close(commitEntered)
			<-releaseCommit
		}
		return nil, context.DeadlineExceeded
	}
	return observed, commitEntered, func() { close(releaseCommit) }
}

func requireBoundedTransactionResolution(
	t *testing.T,
	observed <-chan transactionResolutionObservation,
	wantDeadline time.Time,
) {
	t.Helper()
	for _, wantStatement := range []string{`COMMIT`, `ROLLBACK`} {
		select {
		case got := <-observed:
			if got.statement != wantStatement {
				t.Fatalf("resolution statement=%q, want %q", got.statement, wantStatement)
			}
			if !got.hasDeadline || !got.deadline.Equal(wantDeadline) {
				t.Fatalf("%s deadline=%v present=%v, want %v", got.statement, got.deadline, got.hasDeadline, wantDeadline)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s resolution attempt", wantStatement)
		}
	}
}

func TestCleanupDeadlineBoundsReleaseCommitAndRollback(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	observed, commitEntered, releaseCommit := scriptStoreResolutionDeadline(store)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	wantDeadline, _ := ctx.Deadline()
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken)
	}()
	select {
	case <-commitEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("release did not reach COMMIT resolution")
	}
	releaseCommit()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("release error=%v, want deadline exceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("release transaction outlived cleanup deadline")
	}
	requireBoundedTransactionResolution(t, observed, wantDeadline)

	store.resolveImmediateTransaction = executeImmediateTransactionStatement
	if _, err := store.GetTurnLease(context.Background(), session.ID); err != nil {
		t.Fatalf("failed release must leave lease held: %v", err)
	}
}

func TestCleanupDeadlineBoundsTerminalCommitAndRollback(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := store.BindHistory(session.ID, lease.HolderID)
	root, err := history.Append(context.Background(), lease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := memory.TurnTerminalPayload{
		TurnID: root.ID, Classification: memory.ClassificationProviderError, Stage: memory.StageProvider,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	observed, commitEntered, releaseCommit := scriptStoreResolutionDeadline(store)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Hour))
	wantDeadline, _ := ctx.Deadline()
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, appendErr := history.Append(ctx, lease, memory.EventInput{
			ParentID: root.ID,
			Type:     memory.EventTurnFailed,
			Content:  payload.SafeContent(),
			Payload:  payloadJSON,
		})
		done <- appendErr
	}()
	select {
	case <-commitEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal append did not reach COMMIT resolution")
	}
	releaseCommit()

	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("terminal append error=%v, want deadline exceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal transaction outlived cleanup deadline")
	}
	requireBoundedTransactionResolution(t, observed, wantDeadline)

	store.resolveImmediateTransaction = executeImmediateTransactionStatement
	events, err := store.LoadEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID != root.ID {
		t.Fatalf("timed-out terminal transaction committed events=%+v", events)
	}
}
