package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func TestCanonicalRequestEstimatorAccountsForCompleteStreamingRequest(t *testing.T) {
	req := openrouter.ChatRequest{
		Model:    "vendor/model",
		Messages: []openrouter.Message{{Role: "system", Content: "rules"}, {Role: "user", Content: "hello"}},
		Tools: []openrouter.Tool{{Type: "function", Function: openrouter.Function{
			Name: "lookup", Parameters: openrouter.Parameter{Type: "object", Properties: map[string]openrouter.Property{
				"query": {Type: "string"},
			}},
		}}},
		Reasoning: &openrouter.ReasoningConfig{Effort: "low"},
		MaxTokens: 8192,
		Stream:    true,
	}

	got, err := (CanonicalRequestEstimator{}).Estimate(req)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.SerializedBytes != int64(len(canonical)) || got.RoughTokens != int64((len(canonical)+3)/4) {
		t.Fatalf("estimate=%+v canonical bytes=%d", got, len(canonical))
	}
	if got.RequestSHA256 != "580d4e18fdb99133efe0d115b6849b6514b8761506ec072714dcccbb92a3f896" {
		t.Fatalf("request hash=%q", got.RequestSHA256)
	}
}

func TestContextComposerUsesLegalRootTurnCutsAndProtectsActiveTurn(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 8000, 8000, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := []memory.Event{
		{ID: "old-user", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: strings.Repeat("o", 700)},
		{ID: "old-assistant", Sequence: 2, ParentID: "old-user", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "old answer", Payload: json.RawMessage(`{}`)},
		{ID: "active-user", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "current"},
	}
	composer := NewContextComposer(CanonicalRequestEstimator{})
	result, err := composer.Compose(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "active-user", TriggerEventID: "active-user", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Request.Messages) != 2 || result.Request.Messages[1].Content != "current" {
		t.Fatalf("messages=%+v, want system plus active root", result.Request.Messages)
	}
	if result.Snapshot.RetainedFirstEventID != "active-user" || result.Snapshot.RetainedFirstSequence != 3 {
		t.Fatalf("retained frontier=%+v", result.Snapshot)
	}
	if result.Snapshot.SerializedBytes > result.Snapshot.UsableInputBytes {
		t.Fatalf("admitted %d bytes above %d", result.Snapshot.SerializedBytes, result.Snapshot.UsableInputBytes)
	}
}

func TestContextComposerIncludesAcceptedSummaryBeforeRecentHistory(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 8000, 8000, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: profile,
		Summary: &ContextSummary{
			CompactionEventID: "compaction-1",
			Content:           "approved rolling summary",
		},
		Events: []memory.Event{
			{ID: "active-user", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "current"},
		},
		ActiveRootID: "active-user", TriggerEventID: "active-user", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Request.Messages) != 3 || result.Request.Messages[1].Role != "system" ||
		!strings.Contains(result.Request.Messages[1].Content, "approved rolling summary") ||
		result.Request.Messages[2].Content != "current" {
		t.Fatalf("messages=%+v", result.Request.Messages)
	}
	if result.Snapshot.ActiveCompactionEventID != "compaction-1" || result.Snapshot.SummaryMessageBytes <= 0 {
		t.Fatalf("snapshot=%+v", result.Snapshot)
	}
}

func TestContextComposerRejectsAnActiveTurnThatCannotFit(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 8000, 8000, 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: profile,
		Events: []memory.Event{{
			ID: "active-user", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser,
			Content: strings.Repeat("x", 1000),
		}},
		ActiveRootID: "active-user", TriggerEventID: "active-user", Iteration: 1,
	})
	if err == nil || !IsContextOverflow(err) {
		t.Fatalf("Compose error=%v, want context overflow", err)
	}
}

func TestDefaultContextBudgetAdmitsAtMost241664SerializedBytes(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 262144, 262144, 16384)
	if err != nil {
		t.Fatal(err)
	}
	got, err := usableInputBytes(profile.Diagnostics())
	if err != nil {
		t.Fatal(err)
	}
	if got != 241664 {
		t.Fatalf("usable input=%d, want 241664", got)
	}
}

type fixedRequestEstimator struct{ bytes int64 }

func (fixedRequestEstimator) Version() string { return "fixed-test-estimator" }
func (e fixedRequestEstimator) Estimate(openrouter.ChatRequest) (RequestEstimate, error) {
	return RequestEstimate{
		SerializedBytes: e.bytes,
		RoughTokens:     (e.bytes + 3) / 4,
		RequestSHA256:   strings.Repeat("b", 64),
	}, nil
}

func TestContextComposerExactAdmissionBoundary(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 8000, 8000, 1)
	if err != nil {
		t.Fatal(err)
	}
	const usable = int64(3903)
	for _, test := range []struct {
		name      string
		bytes     int64
		overflows bool
	}{
		{name: "exactly usable", bytes: usable},
		{name: "one byte above", bytes: usable + 1, overflows: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewContextComposer(fixedRequestEstimator{bytes: test.bytes}).Compose(ContextComposeInput{
				Profile: profile,
				Events: []memory.Event{{
					ID: "active", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "hello",
				}},
				ActiveRootID: "active", TriggerEventID: "active", Iteration: 1,
			})
			if IsContextOverflow(err) != test.overflows {
				t.Fatalf("Compose error=%v, overflow=%v", err, test.overflows)
			}
		})
	}
}

func TestInspectContextIsReadOnlyRedactedAndReportsCurrentProjection(t *testing.T) {
	history := &fakeHistory{events: []memory.Event{
		{ID: "event-1", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "secret conversation text"},
		{ID: "event-2", Sequence: 2, ParentID: "event-1", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "secret answer", Payload: json.RawMessage(`{}`)},
	}}
	session := New(nil, testContextProfile("test/model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())

	diagnostics, err := session.InspectContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if history.appendAttempts != 0 || history.snapshotCount != 0 {
		t.Fatalf("inspection appended durable events: %+v", history)
	}
	if diagnostics.Projection.SerializedBytes <= 0 || diagnostics.HeadroomBytes < 0 ||
		diagnostics.Projection.RetainedFirstEventID != "event-1" {
		t.Fatalf("diagnostics=%+v", diagnostics)
	}
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret conversation") || strings.Contains(string(encoded), "secret answer") {
		t.Fatalf("diagnostics leaked content: %s", encoded)
	}
}

func TestInspectContextMakesInvalidDurableHistoryVisible(t *testing.T) {
	history := &fakeHistory{events: []memory.Event{{
		ID: "event-1", Sequence: 1, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Content: "orphan", Payload: json.RawMessage(`{}`),
	}}}
	session := New(nil, testContextProfile("test/model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	if _, err := session.InspectContext(context.Background()); err == nil {
		t.Fatal("InspectContext accepted invalid durable history")
	}
}

func TestInspectContextRejectsMalformedToolResultGroups(t *testing.T) {
	for _, test := range []struct {
		name   string
		events []memory.Event
	}{
		{
			name: "orphan result",
			events: []memory.Event{
				{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser},
				{ID: "result", Sequence: 2, ParentID: "root", Type: memory.EventToolSucceeded, Role: memory.RoleTool,
					Payload: json.RawMessage(`{"tool_call_id":"missing"}`)},
			},
		},
		{
			name: "duplicate result",
			events: []memory.Event{
				{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser},
				{ID: "assistant", Sequence: 2, ParentID: "root", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
					Payload: json.RawMessage(`{"tool_calls":[{"id":"call-1","name":"echo","arguments":"{}"}]}`)},
				{ID: "result-1", Sequence: 3, ParentID: "assistant", Type: memory.EventToolSucceeded, Role: memory.RoleTool,
					Payload: json.RawMessage(`{"tool_call_id":"call-1"}`)},
				{ID: "result-2", Sequence: 4, ParentID: "assistant", Type: memory.EventToolFailed, Role: memory.RoleTool,
					Payload: json.RawMessage(`{"tool_call_id":"call-1"}`)},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := &fakeHistory{events: test.events}
			session := New(nil, testContextProfile("test/model"), history, memory.ScopeContext{
				OwnerID: memory.LocalOwnerID, SessionID: "test-session",
			}, newFakeTurnOwner())
			if _, err := session.InspectContext(context.Background()); err == nil {
				t.Fatal("InspectContext accepted malformed tool-result history")
			}
		})
	}
}

func TestInspectContextRejectsSnapshotWithInvalidDurableCorrelation(t *testing.T) {
	root := memory.Event{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser}
	nextRoot := memory.Event{ID: "next-root", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser}
	payload := memory.ContextSnapshotPayload{
		SchemaVersion: memory.ContextSnapshotSchemaVersion, ComposerVersion: ContextComposerVersion,
		EstimatorVersion: CanonicalRequestEstimatorVersion, Iteration: 1,
		ConfiguredModel: "test/model", CanonicalModel: "test/model", ProfileSource: "explicit_override",
		HardWindowTokens: 262144, WorkingCeilingTokens: 262144, OutputReserveTokens: 16384,
		EstimationMarginTokens: 4096, UsableInputBytes: 241664, SerializedBytes: 100,
		RoughTokenEstimate: 25, RequestSHA256: strings.Repeat("a", 64),
		RetainedFirstEventID: "assistant", RetainedFirstSequence: 2,
		RetainedLastEventID: nextRoot.ID, RetainedLastSequence: nextRoot.Sequence,
		MessageCount: 2, SystemMessageBytes: 10, HistoryMessageBytes: 10, RequestSettingsBytes: 10,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistory{events: []memory.Event{
		root,
		{ID: "assistant", Sequence: 2, ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: json.RawMessage(`{}`)},
		nextRoot,
		{ID: "snapshot", Sequence: 4, ParentID: nextRoot.ID, Type: memory.EventContextSnapshot, Payload: encoded},
	}}
	session := New(nil, testContextProfile("test/model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	if _, err := session.InspectContext(context.Background()); err == nil {
		t.Fatal("InspectContext accepted snapshot with a non-root retained frontier")
	}
}

func TestInspectContextRejectsDelayedSnapshot(t *testing.T) {
	root := memory.Event{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser}
	payload := memory.ContextSnapshotPayload{
		SchemaVersion: memory.ContextSnapshotSchemaVersion, ComposerVersion: ContextComposerVersion,
		EstimatorVersion: CanonicalRequestEstimatorVersion, Iteration: 1,
		ConfiguredModel: "test/model", CanonicalModel: "test/model", ProfileSource: "explicit_override",
		HardWindowTokens: 262144, WorkingCeilingTokens: 262144, OutputReserveTokens: 16384,
		EstimationMarginTokens: 4096, UsableInputBytes: 241664, SerializedBytes: 100,
		RoughTokenEstimate: 25, RequestSHA256: strings.Repeat("a", 64),
		RetainedFirstEventID: root.ID, RetainedFirstSequence: root.Sequence,
		RetainedLastEventID: root.ID, RetainedLastSequence: root.Sequence,
		MessageCount: 2, SystemMessageBytes: 10, HistoryMessageBytes: 10, RequestSettingsBytes: 10,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	history := &fakeHistory{events: []memory.Event{
		root,
		{ID: "assistant", Sequence: 2, ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: json.RawMessage(`{}`)},
		{ID: "snapshot", Sequence: 3, ParentID: root.ID, Type: memory.EventContextSnapshot, Payload: encoded},
	}}
	session := New(nil, testContextProfile("test/model"), history, memory.ScopeContext{
		OwnerID: memory.LocalOwnerID, SessionID: "test-session",
	}, newFakeTurnOwner())
	if _, err := session.InspectContext(context.Background()); err == nil {
		t.Fatal("InspectContext accepted a snapshot delayed past its trigger")
	}
}
