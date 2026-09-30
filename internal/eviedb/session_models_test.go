package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionModelPersistsAcrossRestartAndRejectsStaleBusyArchivedWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := NewStore(db)
	session, err := store.CreateGlobalSessionWithComposition(ctx, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	setting, err := store.SessionModel(ctx, session.ID)
	if err != nil || setting != (SessionModel{}) {
		t.Fatalf("initial=%+v err=%v", setting, err)
	}
	setting, err = store.SetSessionModel(ctx, session.ID, 0, "anthropic/claude-test")
	if err != nil || setting.Revision != 1 {
		t.Fatalf("selected=%+v err=%v", setting, err)
	}
	if _, err := store.SetSessionModel(ctx, session.ID, 0, "openai/test"); !errors.Is(err, ErrSessionModelChanged) {
		t.Fatalf("stale write=%v", err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "turn", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetSessionModel(ctx, session.ID, 1, "openai/test"); !errors.Is(err, ErrTurnLeaseHeld) {
		t.Fatalf("busy write=%v", err)
	}
	if err := store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = NewStore(db)
	got, err := store.SessionModel(ctx, session.ID)
	if err != nil || got != setting {
		t.Fatalf("reopened=%+v want=%+v err=%v", got, setting, err)
	}
	if _, err := store.ArchiveSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetSessionModel(ctx, session.ID, 1, "openai/test"); !errors.Is(err, ErrSessionNotActive) {
		t.Fatalf("archived write=%v", err)
	}
}
