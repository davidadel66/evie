package eviedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func validContextSnapshotPayload(first, last memory.Event) memory.ContextSnapshotPayload {
	return memory.ContextSnapshotPayload{
		SchemaVersion: memory.ContextSnapshotSchemaVersion, ComposerVersion: "context-composer-v1",
		EstimatorVersion: "canonical-json-bytes-v1", Iteration: 1,
		ConfiguredModel: "test/model", CanonicalModel: "test/model", ProfileSource: "explicit_override",
		HardWindowTokens: 262144, WorkingCeilingTokens: 262144, OutputReserveTokens: 16384,
		EstimationMarginTokens: 4096, UsableInputBytes: 241664, SerializedBytes: 1000,
		RoughTokenEstimate: 250, RequestSHA256: strings.Repeat("a", 64),
		RetainedFirstEventID: first.ID, RetainedFirstSequence: first.Sequence,
		RetainedLastEventID: last.ID, RetainedLastSequence: last.Sequence,
		MessageCount: 2, SystemMessageBytes: 100, HistoryMessageBytes: 100,
		RequestSettingsBytes: 50,
	}
}

func validContextCompactionSummary() string {
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summary, "## %s\nkept\n\n", heading)
	}
	return summary.String()
}

func TestContextSnapshotAppendValidatesContentFreeManifestAndFrontier(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := validContextSnapshotPayload(root, root)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: root.ID, Type: memory.EventContextSnapshot, Payload: encoded,
	})
	if err != nil {
		t.Fatalf("append valid snapshot: %v", err)
	}
	if snapshot.Content != "" || snapshot.Role != "" || snapshot.ExecutionID != "" || snapshot.ParentID != root.ID {
		t.Fatalf("snapshot envelope=%+v", snapshot)
	}
	if _, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: root.ID, Type: memory.EventContextSnapshot, Payload: encoded,
	}); err == nil {
		t.Fatal("snapshot delayed past its trigger was accepted")
	}

	invalid := []memory.EventInput{
		{ParentID: root.ID, Type: memory.EventContextSnapshot, Role: memory.RoleAssistant, Payload: encoded},
		{ParentID: root.ID, Type: memory.EventContextSnapshot, Content: "prompt content", Payload: encoded},
		{Type: memory.EventContextSnapshot, Payload: encoded},
		{ParentID: root.ID, Type: memory.EventContextSnapshot, Payload: json.RawMessage(`{"schema_version":1,"secret":"value"}`)},
	}
	for _, input := range invalid {
		if got, err := store.appendEventForTest(ctx, session.ID, input); err == nil {
			t.Fatalf("invalid snapshot appended: %+v", got)
		}
	}

	assistant, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	nextRoot, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "next",
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidFrontier := validContextSnapshotPayload(assistant, nextRoot)
	invalidFrontierJSON, err := json.Marshal(invalidFrontier)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: nextRoot.ID, Type: memory.EventContextSnapshot, Payload: invalidFrontierJSON,
	}); err == nil {
		t.Fatal("snapshot with a non-root retained frontier was accepted")
	}
}

func TestContextSnapshotAppendCorrelatesPlaceholderWithDurableToolResult(t *testing.T) {
	store := NewStore(newTestDB(t))
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run",
	})
	if err != nil {
		t.Fatal(err)
	}
	assistantPayload, err := json.Marshal(memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{{
		ID: "call-1", Name: "large", Arguments: `{}`,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	assistant, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: assistantPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	resultContent := strings.Repeat("result", 1000)
	resultPayload, err := json.Marshal(memory.ToolResultPayload{ToolCallID: "call-1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: assistant.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool,
		Content: resultContent, Payload: resultPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(resultContent))
	payload := validContextSnapshotPayload(root, result)
	payload.Placeholders = []memory.ContextPlaceholderManifest{{
		EventID: result.ID, OriginalBytes: int64(len(resultContent)), ProjectedBytes: 1200,
		SHA256: fmt.Sprintf("%x", digest),
	}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		ParentID: result.ID, Type: memory.EventContextSnapshot, Payload: encoded,
	}); err != nil {
		t.Fatalf("append correlated placeholder snapshot: %v", err)
	}

	other, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	otherRoot, err := store.appendEventForTest(ctx, other.ID, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run",
	})
	if err != nil {
		t.Fatal(err)
	}
	bad := validContextSnapshotPayload(otherRoot, otherRoot)
	bad.Placeholders = []memory.ContextPlaceholderManifest{{
		EventID: result.ID, OriginalBytes: int64(len(resultContent)), ProjectedBytes: 1200,
		SHA256: fmt.Sprintf("%x", digest),
	}}
	badJSON, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendEventForTest(ctx, other.ID, memory.EventInput{
		ParentID: otherRoot.ID, Type: memory.EventContextSnapshot, Payload: badJSON,
	}); err == nil {
		t.Fatal("snapshot accepted placeholder from another session")
	}
}

func TestContextCompactedAppendValidatesSummaryAndWholeTurnFrontier(t *testing.T) {
	store := NewStore(newTestDB(t))
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appendTurn := func(label string) (memory.Event, memory.Event) {
		t.Helper()
		root, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
			Type: memory.EventUserMessage, Role: memory.RoleUser, Content: label,
		})
		if err != nil {
			t.Fatal(err)
		}
		assistant, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
			ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Content: "answer " + label, Payload: json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		return root, assistant
	}
	firstRoot, firstAssistant := appendTurn("one")
	retainedRoot, _ := appendTurn("two")
	appendTurn("three")

	summary := validContextCompactionSummary()
	digest := sha256.Sum256([]byte(summary))
	payload := memory.ContextCompactedPayload{
		SchemaVersion: memory.ContextCompactedSchemaVersion, Generation: 1,
		Trigger:             memory.ContextCompactionManual,
		CoveredFirstEventID: firstRoot.ID, CoveredFirstSequence: firstRoot.Sequence,
		CoveredLastEventID: firstAssistant.ID, CoveredLastSequence: firstAssistant.Sequence,
		FirstRetainedEventID: retainedRoot.ID, CanonicalModel: "vendor/model",
		PromptVersion: "compaction-v1", SummaryBytes: int64(len(summary)), SummarySHA256: fmt.Sprintf("%x", digest),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	const malformedSummary = "prefix ## Goal / criteria / constraints\nmissing exact sections"
	malformedDigest := sha256.Sum256([]byte(malformedSummary))
	malformedPayload := payload
	malformedPayload.SummaryBytes = int64(len(malformedSummary))
	malformedPayload.SummarySHA256 = fmt.Sprintf("%x", malformedDigest)
	malformedJSON, err := json.Marshal(malformedPayload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		Type: memory.EventContextCompacted, Content: malformedSummary, Payload: malformedJSON,
	}); err == nil {
		t.Fatal("sectionless compaction summary was accepted")
	}
	compacted, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		Type: memory.EventContextCompacted, Content: summary, Payload: encoded,
	})
	if err != nil {
		t.Fatalf("append valid compaction: %v", err)
	}
	if compacted.ParentID != "" || compacted.Role != "" || compacted.ExecutionID != "" || compacted.Content != summary {
		t.Fatalf("compaction envelope=%+v", compacted)
	}

	if _, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
		Type: memory.EventContextCompacted, Content: summary, Payload: encoded,
	}); err == nil {
		t.Fatal("second generation was accepted before chain support")
	}

	other, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bad := payload
	bad.FirstRetainedEventID = firstRoot.ID
	badJSON, err := json.Marshal(bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.appendEventForTest(ctx, other.ID, memory.EventInput{
		Type: memory.EventContextCompacted, Content: summary, Payload: badJSON,
	}); err == nil {
		t.Fatal("compaction accepted source events from another session")
	}
}

type testEventExecutor struct{ db *sql.DB }

func (e testEventExecutor) queryRowContext(ctx context.Context, query string, args ...any) rowScanner {
	return e.db.QueryRowContext(ctx, query, args...)
}

func (s *Store) appendEventForTest(
	ctx context.Context,
	sessionID memory.SessionID,
	input memory.EventInput,
) (memory.Event, error) {
	return s.appendEvent(ctx, testEventExecutor{db: s.db}, sessionID, input)
}

func TestEventStoreAppendsBoundEventsAndLoadsInOrder(t *testing.T) {
	db := newTestDB(t)
	store := NewStore(db)
	ctx := context.Background()

	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatalf("create global session: %v", err)
	}

	first, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
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
	second, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
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
	projectEvent, err := store.appendEventForTest(ctx, projectSession.ID, memory.EventInput{
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
	otherEvent, err := store.appendEventForTest(ctx, otherSession.ID, memory.EventInput{
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
			if event, err := store.appendEventForTest(ctx, tt.sessionID, tt.input); err == nil {
				t.Fatalf("invalid append returned event %+v", event)
			}
		})
	}

	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, session.ID); err != nil {
		t.Fatalf("close session fixture: %v", err)
	}
	if event, err := store.appendEventForTest(ctx, session.ID, memory.EventInput{
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
	usageTotal := int64(11)
	stalePayload, err := json.Marshal(memory.AssistantMessagePayload{
		Usage: &memory.TokenUsage{TotalTokens: &usageTotal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken+1, memory.EventInput{
		Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "stale", Payload: stalePayload,
	}); !errors.Is(err, ErrTurnLeaseLost) {
		t.Fatalf("stale assistant usage append error = %v, want ErrTurnLeaseLost", err)
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

func TestAssistantUsagePayloadSurvivesDatabaseCloseAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "holder", time.Minute)
	if err != nil {
		db.Close()
		t.Fatal(err)
	}

	zero, total := int64(0), int64(15)
	partialPayload, err := json.Marshal(memory.AssistantMessagePayload{Usage: &memory.TokenUsage{
		CachedInputTokens: &zero,
		TotalTokens:       &total,
	}})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	absentPayload, err := json.Marshal(memory.AssistantMessagePayload{})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	for _, input := range []memory.EventInput{
		{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "partial", Payload: partialPayload},
		{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "absent", Payload: absentPayload},
	} {
		if _, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input); err != nil {
			db.Close()
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	events, err := NewStore(reopened).LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events after reopen=%+v", events)
	}
	var partial memory.AssistantMessagePayload
	if err := json.Unmarshal(events[0].Payload, &partial); err != nil {
		t.Fatal(err)
	}
	if partial.Usage == nil || partial.Usage.CachedInputTokens == nil || *partial.Usage.CachedInputTokens != 0 ||
		partial.Usage.TotalTokens == nil || *partial.Usage.TotalTokens != 15 ||
		partial.Usage.InputTokens != nil {
		t.Fatalf("partial usage after reopen=%+v", partial.Usage)
	}
	var absent memory.AssistantMessagePayload
	if err := json.Unmarshal(events[1].Payload, &absent); err != nil {
		t.Fatal(err)
	}
	if absent.Usage != nil || string(events[1].Payload) != `{}` {
		t.Fatalf("absent usage after reopen payload=%s decoded=%+v", events[1].Payload, absent)
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
