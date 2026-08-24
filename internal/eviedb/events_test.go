package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func TestEventStoreAppendsBoundEventsAndLoadsInOrder(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create global session: %v", err)
	}

	first, err := store.AppendEvent(ctx, session.ID, memory.EventInput{
		Type:    memory.EventUserMessage,
		Role:    memory.RoleUser,
		Content: "hello",
	})
	if err != nil {
		t.Fatalf("append first event: %v", err)
	}
	if first.ID == "" || first.SessionID != session.ID || first.Sequence != 1 ||
		first.ProjectID != "" || first.Type != memory.EventUserMessage ||
		first.Role != memory.RoleUser || first.Content != "hello" ||
		string(first.Payload) != `{}` || first.FormatVersion != 1 ||
		first.RecordedAt.IsZero() || first.RecordedAt.Location() != time.UTC {
		t.Errorf("first event = %+v, payload = %q", first, first.Payload)
	}

	inputPayload := json.RawMessage(`{"tool":"time"}`)
	second, err := store.AppendEvent(ctx, session.ID, memory.EventInput{
		ParentID:    first.ID,
		Type:        memory.EventToolIntent,
		ExecutionID: memory.ExecutionID("execution-1"),
		Payload:     inputPayload,
	})
	if err != nil {
		t.Fatalf("append second event: %v", err)
	}
	if second.Sequence != 2 || second.ParentID != first.ID ||
		second.ExecutionID != "execution-1" || string(second.Payload) != string(inputPayload) {
		t.Errorf("second event = %+v, payload = %q", second, second.Payload)
	}

	inputPayload[0] = '['
	if string(second.Payload) != `{"tool":"time"}` {
		t.Errorf("returned event payload aliases caller input: %q", second.Payload)
	}

	events, err := store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatalf("load events: %v", err)
	}
	if len(events) != 2 || events[0].ID != first.ID || events[1].ID != second.ID ||
		events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("loaded events = %+v", events)
	}
	if string(events[1].Payload) != `{"tool":"time"}` {
		t.Errorf("stored payload changed with caller slice: %q", events[1].Payload)
	}

	project, err := store.RegisterProject(ctx, "Evie", t.TempDir())
	if err != nil {
		t.Fatalf("register project: %v", err)
	}
	projectSession, err := store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatalf("create project session: %v", err)
	}
	projectEvent, err := store.AppendEvent(ctx, projectSession.ID, memory.EventInput{
		Type:    memory.EventAssistantMessage,
		Role:    memory.RoleAssistant,
		Content: "project-bound",
	})
	if err != nil {
		t.Fatalf("append project event: %v", err)
	}
	if projectEvent.ProjectID != project.ID {
		t.Errorf("project event scope = %q, want %q", projectEvent.ProjectID, project.ID)
	}
}

func TestEventStoreRejectsInvalidOrUnavailableAppends(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	otherSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create other session: %v", err)
	}
	otherEvent, err := store.AppendEvent(ctx, otherSession.ID, memory.EventInput{
		Type: memory.EventUserMessage,
		Role: memory.RoleUser,
	})
	if err != nil {
		t.Fatalf("append other-session event: %v", err)
	}

	tests := []struct {
		name      string
		sessionID memory.SessionID
		input     memory.EventInput
	}{
		{
			name:      "missing session",
			sessionID: memory.SessionID("missing"),
			input:     memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser},
		},
		{
			name:      "empty event type",
			sessionID: session.ID,
			input:     memory.EventInput{Role: memory.RoleUser},
		},
		{
			name:      "invalid role",
			sessionID: session.ID,
			input:     memory.EventInput{Type: memory.EventUserMessage, Role: memory.EventRole("developer")},
		},
		{
			name:      "invalid JSON payload",
			sessionID: session.ID,
			input:     memory.EventInput{Type: memory.EventUserMessage, Payload: json.RawMessage(`{`)},
		},
		{
			name:      "parent from another session",
			sessionID: session.ID,
			input: memory.EventInput{
				ParentID: otherEvent.ID,
				Type:     memory.EventToolSucceeded,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if event, err := store.AppendEvent(ctx, tt.sessionID, tt.input); err == nil {
				t.Fatalf("invalid append returned event %+v", event)
			}
		})
	}

	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, session.ID); err != nil {
		t.Fatalf("close session fixture: %v", err)
	}
	if event, err := store.AppendEvent(ctx, session.ID, memory.EventInput{
		Type: memory.EventUserMessage,
		Role: memory.RoleUser,
	}); err == nil {
		t.Fatalf("append to closed session returned event %+v", event)
	}
}

func TestFencedEventAppendRejectsStaleLeaseAndRollsBackAtFinalFence(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	setTurnLeaseTime(store, now)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "holder", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken+1, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "stale",
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("stale append error = %v, want ErrTurnLeaseLost", err)
	}

	samples := 0
	store.now = func() time.Time {
		samples++
		if samples == 1 {
			return now
		}
		return lease.ExpiresAt
	}
	if _, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "crossed expiry",
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("final-fence append error = %v, want ErrTurnLeaseLost", err)
	}
	events, err := store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("rolled-back fenced appends = %+v", events)
	}
}

func TestTerminalEventStorageRejectsUnsafeOrOpenPayloads(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	base := memory.TurnTerminalPayload{
		TurnID: "root", Classification: memory.ClassificationProviderError, Stage: memory.StageProvider,
	}
	valid, _ := json.Marshal(base)
	tests := []struct {
		name    string
		content string
		payload json.RawMessage
	}{
		{name: "raw error content", content: "provider URL and body", payload: valid},
		{name: "unknown payload field", content: base.SafeContent(), payload: json.RawMessage(`{"turn_id":"root","classification":"provider_error","stage":"provider","raw_body":"secret"}`)},
		{name: "unknown stage", content: base.SafeContent(), payload: json.RawMessage(`{"turn_id":"root","classification":"provider_error","stage":"future"}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
				ParentID: "root", Type: memory.EventTurnFailed, Content: tt.content, Payload: tt.payload,
			}); err == nil {
				t.Fatalf("unsafe terminal event appended: %+v", event)
			}
		})
	}
}
