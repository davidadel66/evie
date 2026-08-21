package eviedb

import (
	"context"
	"testing"

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

	history := store.BindHistory(firstSession.ID)
	firstEvent, err := history.Append(ctx, memory.EventInput{
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

	if _, err := store.AppendEvent(ctx, secondSession.ID, memory.EventInput{
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

func TestSessionHistoryCannotAppendToMissingSession(t *testing.T) {
	db := newTestDB(t)
	history := NewStore(db).BindHistory(memory.SessionID("missing"))

	if event, err := history.Append(context.Background(), memory.EventInput{
		Type: memory.EventUserMessage,
		Role: memory.RoleUser,
	}); err == nil {
		t.Fatalf("missing-session history appended %+v", event)
	}
}
