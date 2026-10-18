package eviedb

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// A turn reads only the events committed since its last request; together
// with what it holds, they must be exactly the session's full history.
func TestLoadEventsAfterListsOnlyLaterEventsOfTheSession(t *testing.T) {
	ctx := context.Background()
	store := NewStore(newTestDB(t))
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		for _, id := range []memory.SessionID{session.ID, other.ID} {
			if _, err := store.appendEvent(ctx, txEventExecutor{tx}, id, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: fmt.Sprintf("message %d", i)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	full, err := store.LoadEvents(ctx, session.ID)
	if err != nil || len(full) != 6 {
		t.Fatalf("full history = %d events, %v", len(full), err)
	}
	for held := 0; held <= len(full); held++ {
		var through int64
		if held > 0 {
			through = full[held-1].Sequence
		}
		newer, err := store.LoadEventsAfter(ctx, session.ID, through)
		if err != nil {
			t.Fatal(err)
		}
		if got := append(append([]memory.Event(nil), full[:held]...), newer...); !reflect.DeepEqual(got, full) {
			t.Fatalf("holding %d events, the later ones do not complete the history: %+v", held, newer)
		}
	}
}
