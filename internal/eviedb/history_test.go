package eviedb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestSessionHistoryBindsAppendsAndReadsToOneSession(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	firstSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create first session: %v", err)
	}
	secondSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}

	history := store.BindHistory(firstSession.ID, "worker")
	lease, err := store.AcquireTurnLease(ctx, firstSession.ID, "worker", time.Minute)
	if err != nil {
		t.Fatalf("acquire first-session lease: %v", err)
	}
	firstEvent, err := history.Append(ctx, lease, memory.EventInput{
		Type:    memory.EventUserMessage,
		Role:    memory.RoleUser,
		Content: "first session",
	})
	if err != nil {
		t.Fatalf("append through bound history: %v", err)
	}
	if firstEvent.SessionID != firstSession.ID {
		t.Errorf("bound append session = %q, want %q", firstEvent.SessionID, firstSession.ID)
	}

	if _, err := store.appendEventForTest(ctx, secondSession.ID, memory.EventInput{
		Type:    memory.EventUserMessage,
		Role:    memory.RoleUser,
		Content: "second session",
	}); err != nil {
		t.Fatalf("append second-session event: %v", err)
	}

	events, err := history.Events(ctx)
	if err != nil {
		t.Fatalf("read bound history: %v", err)
	}
	if len(events) != 1 || events[0].SessionID != firstSession.ID ||
		events[0].Content != "first session" {
		t.Errorf("bound events = %+v", events)
	}
}

func TestBoundHistoryAndOwnerRejectMismatchedLeaseIdentity(t *testing.T) {
	store := NewStore(newTestDB(t))
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const holder = memory.LeaseHolderID("bound-holder")
	lease, err := store.AcquireTurnLease(ctx, session.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := store.BindHistory(session.ID, holder)
	owner := store.BindTurnOwner(session.ID, holder)

	variants := []struct {
		name  string
		lease memory.TurnLease
	}{
		{name: "session", lease: func() memory.TurnLease { l := lease; l.SessionID = "other-session"; return l }()},
		{name: "holder", lease: func() memory.TurnLease { l := lease; l.HolderID = "other-holder"; return l }()},
		{name: "generation", lease: func() memory.TurnLease { l := lease; l.Generation++; return l }()},
		{name: "token", lease: func() memory.TurnLease { l := lease; l.FencingToken++; l.Generation++; return l }()},
	}
	for _, tt := range variants {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := history.Append(ctx, tt.lease, memory.EventInput{
				Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "must not persist",
			}); !errors.Is(err, ErrTurnLeaseLost) {
				t.Fatalf("Append error=%v, want ErrTurnLeaseLost", err)
			}
			if _, err := owner.Heartbeat(ctx, tt.lease, time.Minute); !errors.Is(err, ErrTurnLeaseLost) {
				t.Fatalf("Heartbeat error=%v, want ErrTurnLeaseLost", err)
			}
			if err := owner.Authorize(ctx, tt.lease); !errors.Is(err, ErrTurnLeaseLost) {
				t.Fatalf("Authorize error=%v, want ErrTurnLeaseLost", err)
			}
			if err := owner.Release(ctx, tt.lease); !errors.Is(err, ErrTurnLeaseLost) {
				t.Fatalf("Release error=%v, want ErrTurnLeaseLost", err)
			}
		})
	}
	events, err := store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("mismatched identities persisted events: %+v", events)
	}
	if err := owner.Authorize(ctx, lease); err != nil {
		t.Fatalf("mismatched release disturbed current lease: %v", err)
	}
}

func TestSessionHistoryCannotAppendToMissingSession(t *testing.T) {
	db := newTestDB(t)
	history := NewStore(db).BindHistory(memory.SessionID("missing"), "worker")

	if event, err := history.Append(context.Background(), memory.TurnLease{
		SessionID: "missing", HolderID: "worker", FencingToken: 1, Generation: 1,
	}, memory.EventInput{
		Type: memory.EventUserMessage,
		Role: memory.RoleUser,
	}); err == nil {
		t.Fatalf("missing-session history appended %+v", event)
	}
}
