package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestSessionArchiveRestorePreservesHistoryScopeAndCompositionAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	workspace, err := store.RegisterWorkspace(ctx, "Research")
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)
	session, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Keep this conversation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	archived, err := store.ArchiveSession(ctx, session.ID)
	if err != nil || archived.Status != memory.SessionClosed || archived.Title != "Keep this conversation" {
		t.Fatalf("archived=%+v err=%v", archived, err)
	}
	if _, err := store.GetActiveSession(ctx, session.ID); !errors.Is(err, ErrSessionNotActive) {
		t.Fatalf("archived session resumed: %v", err)
	}
	if _, err := store.AcquireTurnLease(ctx, session.ID, "stale-browser", time.Minute); !errors.Is(err, ErrTurnLeaseSessionInactive) {
		t.Fatalf("archived session turn started: %v", err)
	}
	if again, err := store.ArchiveSession(ctx, session.ID); err != nil || again != archived {
		t.Fatalf("archive retry=%+v err=%v", again, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = NewStore(db)
	active, err := store.ListActiveSessions(ctx)
	if err != nil || len(active) != 0 {
		t.Fatalf("active after restart=%+v err=%v", active, err)
	}
	listings, err := store.ListArchivedSessions(ctx)
	if err != nil || len(listings) != 1 || listings[0].Session != archived || !listings[0].ActivityAt.Equal(event.RecordedAt) {
		t.Fatalf("archived after restart=%+v err=%v", listings, err)
	}
	restored, err := store.RestoreSession(ctx, session.ID)
	if err != nil || restored.Status != memory.SessionActive || restored.ScopeContext() != session.ScopeContext() || restored.Title != archived.Title {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	storedReceipt, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(storedReceipt, receipt) {
		t.Fatalf("restored receipt=%+v err=%v", storedReceipt, err)
	}
	events, err := store.LoadEvents(ctx, session.ID)
	if err != nil || len(events) != 1 || !reflect.DeepEqual(events[0], event) {
		t.Fatalf("restored history=%+v err=%v", events, err)
	}
	listings, err = store.ListArchivedSessions(ctx)
	if err != nil || len(listings) != 0 {
		t.Fatalf("archived after restore=%+v err=%v", listings, err)
	}
	if _, err := store.AcquireTurnLease(ctx, session.ID, "restored-browser", time.Minute); err != nil {
		t.Fatalf("restored session turn rejected: %v", err)
	}
}

func TestSessionArchiveRejectsLeasedMissingAndDelegatedSessions(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "other-process", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(context.Context, memory.SessionID) (memory.Session, error){store.ArchiveSession, store.RestoreSession} {
		if _, err := change(ctx, session.ID); !errors.Is(err, ErrTurnLeaseHeld) {
			t.Fatalf("live lease mutation error=%v", err)
		}
		if _, err := change(ctx, "missing"); !errors.Is(err, ErrSessionNotActive) {
			t.Fatalf("missing mutation error=%v", err)
		}
	}
	if _, err := store.GetActiveSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(ctx, session.ID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveSession(ctx, child.ID); !errors.Is(err, ErrSessionDelegated) {
		t.Fatalf("delegated archive error=%v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, child.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RestoreSession(ctx, child.ID); !errors.Is(err, ErrSessionDelegated) {
		t.Fatalf("delegated restore error=%v", err)
	}
	archived, err := store.ListArchivedSessions(ctx)
	if err != nil || len(archived) != 0 {
		t.Fatalf("delegated archived listing=%+v err=%v", archived, err)
	}
}

func TestSessionArchiveRejectsUnfinishedDelegation(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	store := NewStore(db)
	parent, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(ctx, parent.ID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subagent_executions
		(id, parent_session_id, idempotency_key, request_digest, child_session_id, state, record_json)
		VALUES ('attempt', ?, 'key', 'digest', ?, 'admitted', '{}')`, parent.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"admitted", "running"} {
		if _, err := db.Exec(`UPDATE subagent_executions SET state = ? WHERE id = 'attempt'`, state); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ArchiveSession(ctx, parent.ID); !errors.Is(err, ErrTurnLeaseHeld) {
			t.Fatalf("archive parent with %s work: %v", state, err)
		}
	}
	if _, err := db.Exec(`UPDATE subagent_executions SET state = 'succeeded' WHERE id = 'attempt'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveSession(ctx, parent.ID); err != nil {
		t.Fatalf("archive finished parent: %v", err)
	}
}
