package eviedb_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestPublicEventMutationRequiresCurrentHolderAndTokenAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	storeA, storeB := eviedb.NewStore(dbA), eviedb.NewStore(dbB)
	ctx := context.Background()
	session, err := storeA.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := storeA.AcquireTurnLease(ctx, session.ID, "holder-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	_, err = storeB.AppendEventWithLease(ctx, session.ID, "holder-b", lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "unauthorized",
	})
	if !errors.Is(err, eviedb.ErrTurnLeaseLost) {
		t.Fatalf("AppendEventWithLease error=%v, want ErrTurnLeaseLost", err)
	}
	events, err := storeA.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("public mutation accepted without current identity: %+v", events)
	}
}
