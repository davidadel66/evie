package eviedb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestListActiveSessionsUsesGreatestSequenceActivityAndDeterministicFallbacks(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	insertSession := func(id, status, created string) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO sessions (id, status, created_at, updated_at) VALUES (?, ?, ?, ?)
		`, id, status, created, created); err != nil {
			t.Fatal(err)
		}
	}
	insertSession("empty-a", "active", "2026-08-24T12:00:00Z")
	insertSession("empty-b", "active", "2026-08-24T12:00:00.000000000Z")
	insertSession("sequence", "active", "2026-08-24T08:00:00Z")
	insertSession("fraction", "active", "2026-08-24T08:00:00Z")
	insertSession("closed", "closed", "2026-08-25T12:00:00Z")

	insertEvent := func(id, session string, sequence int, recorded string) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO events (id, session_id, sequence, event_type, role, content, recorded_at)
			VALUES (?, ?, ?, 'user_message', 'user', ?, ?)
		`, id, session, sequence, id, recorded); err != nil {
			t.Fatal(err)
		}
	}
	// The greatest sequence is authoritative even when an earlier event has a
	// lexically and chronologically later recorded_at value.
	insertEvent("sequence-1", "sequence", 1, "2026-08-24T23:00:00Z")
	insertEvent("sequence-2", "sequence", 2, "2026-08-24T12:00:00.1Z")
	insertEvent("fraction-1", "fraction", 1, "2026-08-24T12:00:00.09Z")
	insertEvent("closed-1", "closed", 1, "2026-08-25T12:01:00Z")

	listings, err := store.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []memory.SessionID{"sequence", "fraction", "empty-a", "empty-b"}
	if len(listings) != len(want) {
		t.Fatalf("listings=%+v, want IDs %v", listings, want)
	}
	for i := range want {
		if listings[i].ID != want[i] {
			t.Fatalf("listing[%d]=%q activity=%s, want %q", i, listings[i].ID, listings[i].ActivityAt, want[i])
		}
	}
	if listings[0].ActivityAt != time.Date(2026, 8, 24, 12, 0, 0, 100_000_000, time.UTC) {
		t.Fatalf("greatest-sequence activity=%s", listings[0].ActivityAt)
	}
}

func TestActiveSessionBoundaryExcludesClosedAndReturnsStoredScopeAndTitle(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	root := t.TempDir()
	project, err := store.RegisterProject(context.Background(), "Project", root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateProjectSession(context.Background(), project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET title = 'Stored title' WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetActiveSession(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Title != "Stored title" || active.ScopeContext() != session.ScopeContext() {
		t.Fatalf("active session=%+v, original scope=%+v", active, session.ScopeContext())
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetActiveSession(context.Background(), session.ID); !errors.Is(err, ErrSessionNotActive) {
		t.Fatalf("closed GetActiveSession error=%v", err)
	}
	listings, err := store.ListActiveSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listings) != 0 {
		t.Fatalf("closed sessions leaked from storage boundary: %+v", listings)
	}
}
