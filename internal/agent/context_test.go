package agent

import (
	"context"
	"encoding/json"
	"fmt"
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
	profile, err := openrouter.NewExplicitContextProfile("test/model", 10000, 10000, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := []memory.Event{
		{ID: "old-user", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: strings.Repeat("o", 2500)},
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
	profile, err := openrouter.NewExplicitContextProfile("test/model", 10000, 10000, 1)
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

func TestContextComposerPlacesWorkingContextBeforeSummaryAndHistory(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 10000, 10000, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile:        profile,
		Summary:        &ContextSummary{CompactionEventID: "compaction-1", Content: "prior summary"},
		WorkingContext: "# Task Focus\n- id=task-1 title=\"ship\"",
		Events:         []memory.Event{{ID: "active-user", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "continue"}},
		ActiveRootID:   "active-user", TriggerEventID: "active-user", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	messages := result.Request.Messages
	if len(messages) != 4 || messages[0].Content != systemPrompt || messages[1].Content != "prior summary" ||
		messages[2].Role != "user" || messages[2].Content != "# Task Focus\n- id=task-1 title=\"ship\"" || messages[3].Content != "continue" {
		t.Fatalf("messages = %+v", messages)
	}
	if result.Snapshot.SystemMessageBytes == 0 || result.Snapshot.HistoryMessageBytes == 0 || result.Snapshot.SummaryMessageBytes == 0 {
		t.Fatalf("snapshot does not account for working context and summary: %+v", result.Snapshot)
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

func TestContextComposerReducesLargestResultsFirstWithinGroupBudget(t *testing.T) {
	profile := testContextProfile("test/model")
	events := toolGroupEvents(t, "root", []string{
		strings.Repeat("a", 90*1024),
		strings.Repeat("b", 70*1024),
		strings.Repeat("c", 10*1024),
	})
	result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "root",
		TriggerEventID: "result-3", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var projected []openrouter.Message
	for _, message := range result.Request.Messages {
		if message.Role == "tool" {
			projected = append(projected, message)
		}
	}
	if len(projected) != 3 {
		t.Fatalf("tool messages=%d", len(projected))
	}
	if got := len(projected[0].Content) + len(projected[1].Content) + len(projected[2].Content); got != toolResultGroupBytes {
		t.Fatalf("group bytes=%d, want %d", got, toolResultGroupBytes)
	}
	if len(projected[0].Content) != len(projected[1].Content) || len(projected[2].Content) != 10*1024 {
		t.Fatalf("largest-first allocations=%d,%d,%d", len(projected[0].Content), len(projected[1].Content), len(projected[2].Content))
	}
	if len(result.Snapshot.Placeholders) != 2 {
		t.Fatalf("placeholders=%+v", result.Snapshot.Placeholders)
	}
}

func TestContextComposerRetainsThreeNewestGroupsAndProjectsOlderResultsOldestFirst(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 120000, 120000, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := []memory.Event{{ID: "root", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run"}}
	sequence := int64(2)
	for group := 1; group <= 5; group++ {
		assistantID := memory.EventID(fmt.Sprintf("assistant-%d", group))
		callID := fmt.Sprintf("call-%d", group)
		resultID := memory.EventID(fmt.Sprintf("result-%d", group))
		events = append(events,
			memory.Event{ID: assistantID, Sequence: sequence, ParentID: "root", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
				Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{{ID: callID, Name: "large", Arguments: `{}`}}})},
			memory.Event{ID: resultID, Sequence: sequence + 1, ParentID: assistantID, Type: memory.EventToolSucceeded, Role: memory.RoleTool,
				Content: strings.Repeat(string(rune('a'+group-1)), 16*1024), Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: callID})},
		)
		sequence += 2
	}
	result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "root", TriggerEventID: "result-5", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Snapshot.Placeholders) == 0 || result.Snapshot.Placeholders[0].EventID != "result-1" {
		t.Fatalf("oldest result was not projected first: %+v", result.Snapshot.Placeholders)
	}
	for _, message := range result.Request.Messages {
		if message.Role != "tool" {
			continue
		}
		n := strings.TrimPrefix(message.ToolCallID, "call-")
		if n == "3" || n == "4" || n == "5" {
			if len(message.Content) != 16*1024 {
				t.Fatalf("recent group %s bytes=%d", n, len(message.Content))
			}
		}
	}
}

func TestContextComposerNewestCompleteGroupWithNoLegalFitOverflowsWhole(t *testing.T) {
	profile, err := openrouter.NewExplicitContextProfile("test/model", 100000, 100000, 1)
	if err != nil {
		t.Fatal(err)
	}
	events := toolGroupEvents(t, "root", []string{strings.Repeat("x", 100*1024), strings.Repeat("y", 100*1024)})
	_, err = NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "root", TriggerEventID: "result-2", Iteration: 1,
	})
	if !IsContextOverflow(err) {
		t.Fatalf("Compose error=%v, want context overflow", err)
	}
}

func TestContextComposerPressureNeverExpandsTighterGroupProjection(t *testing.T) {
	results := make([]string, 500)
	for i := range results {
		results[i] = strings.Repeat("x", 10*1024)
	}
	events := toolGroupEvents(t, "root", results)
	sequence := int64(len(events) + 1)
	for group := 1; group <= retainedCompleteToolResultGroups; group++ {
		assistantID := memory.EventID(fmt.Sprintf("recent-assistant-%d", group))
		callID := fmt.Sprintf("recent-call-%d", group)
		resultID := memory.EventID(fmt.Sprintf("recent-result-%d", group))
		events = append(events,
			memory.Event{ID: assistantID, Sequence: sequence, ParentID: "root", Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
				Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{{ID: callID, Name: "small", Arguments: `{}`}}})},
			memory.Event{ID: resultID, Sequence: sequence + 1, ParentID: assistantID, Type: memory.EventToolSucceeded, Role: memory.RoleTool,
				Content: "ok", Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: callID})},
		)
		sequence += 2
	}
	groupOnly, err := applyToolResultGroupLimits(events)
	if err != nil {
		t.Fatal(err)
	}
	if len(projectOldToolResult(events[2])) <= len(groupOnly[2].Content) {
		t.Fatal("fixture does not make the pressure projection larger")
	}
	profile, err := openrouter.NewExplicitContextProfile("test/model", 304097, 304097, 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: profile, Events: events, ActiveRootID: "root",
		TriggerEventID: "recent-result-3", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.SerializedBytes <= percentageFloor(result.Snapshot.UsableInputBytes, 60) {
		t.Fatal("fixture did not remain above the pressure target after non-reducing candidates were skipped")
	}
	if got := result.Request.Messages[3].Content; got != groupOnly[2].Content {
		t.Fatalf("pressure projection expanded group result from %d to %d bytes", len(groupOnly[2].Content), len(got))
	}
}

func TestContextComposerDropsOldTurnWhoseGroupMetadataCannotFit(t *testing.T) {
	results := make([]string, 1000)
	for i := range results {
		results[i] = strings.Repeat("x", 10*1024)
	}
	events := toolGroupEvents(t, "old-root", results)
	events = append(events, memory.Event{
		ID: "active-root", Sequence: int64(len(events) + 1), Type: memory.EventUserMessage,
		Role: memory.RoleUser, Content: "continue",
	})
	result, err := NewContextComposer(CanonicalRequestEstimator{}).Compose(ContextComposeInput{
		Profile: testContextProfile("test/model"), Events: events, ActiveRootID: "active-root",
		TriggerEventID: "active-root", Iteration: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Request.Messages) != 2 || result.Request.Messages[1].Content != "continue" ||
		result.Snapshot.RetainedFirstEventID != "active-root" {
		t.Fatalf("projection=%+v snapshot=%+v", result.Request.Messages, result.Snapshot)
	}
}

func toolGroupEvents(t *testing.T, rootID memory.EventID, results []string) []memory.Event {
	t.Helper()
	calls := make([]memory.ToolCall, len(results))
	for i := range results {
		calls[i] = memory.ToolCall{ID: fmt.Sprintf("call-%d", i+1), Name: "large", Arguments: `{}`}
	}
	events := []memory.Event{
		{ID: rootID, Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "run"},
		{ID: "assistant", Sequence: 2, ParentID: rootID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
			Payload: historyPayload(t, memory.AssistantMessagePayload{ToolCalls: calls})},
	}
	for i, content := range results {
		events = append(events, memory.Event{
			ID: memory.EventID(fmt.Sprintf("result-%d", i+1)), Sequence: int64(i + 3), ParentID: "assistant",
			Type: memory.EventToolSucceeded, Role: memory.RoleTool, Content: content,
			Payload: historyPayload(t, memory.ToolResultPayload{ToolCallID: calls[i].ID}),
		})
	}
	return events
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

func TestDurableContextRejectsAutomaticSnapshotWithMismatchedCompactionFrontier(t *testing.T) {
	turn := completedCompactionTurn("old", 1, "old", "answer")
	active := memory.Event{ID: "active", Sequence: 3, Type: memory.EventUserMessage, Role: memory.RoleUser}
	compacted := contextCompactionEvent(
		t, "compacted", 4, 1, "", turn[0], turn[1], active,
		validCompactionSummary(), "test/model", CompactionPromptVersion,
	)
	var compactionPayload memory.ContextCompactedPayload
	if err := json.Unmarshal(compacted.Payload, &compactionPayload); err != nil {
		t.Fatal(err)
	}
	compactionPayload.Trigger = memory.ContextCompactionAutomatic
	encodedCompaction, err := json.Marshal(compactionPayload)
	if err != nil {
		t.Fatal(err)
	}
	compacted.Payload = encodedCompaction
	snapshotPayload := memory.ContextSnapshotPayload{
		SchemaVersion: memory.ContextSnapshotSchemaVersion, ComposerVersion: ContextComposerVersion,
		EstimatorVersion: CanonicalRequestEstimatorVersion, Iteration: 1,
		ConfiguredModel: "test/model", CanonicalModel: "test/model", ProfileSource: "explicit_override",
		HardWindowTokens: 262144, WorkingCeilingTokens: 262144, OutputReserveTokens: 16384,
		EstimationMarginTokens: 4096, UsableInputBytes: 241664, SerializedBytes: 100,
		RoughTokenEstimate: 25, RequestSHA256: strings.Repeat("a", 64),
		RetainedFirstEventID: turn[0].ID, RetainedFirstSequence: turn[0].Sequence,
		RetainedLastEventID: active.ID, RetainedLastSequence: active.Sequence,
		ActiveCompactionEventID: compacted.ID,
		MessageCount:            3, SystemMessageBytes: 10, SummaryMessageBytes: 10,
		HistoryMessageBytes: 10, RequestSettingsBytes: 10,
	}
	encodedSnapshot, err := json.Marshal(snapshotPayload)
	if err != nil {
		t.Fatal(err)
	}
	events := append(append([]memory.Event{}, turn...), active, compacted, memory.Event{
		ID: "snapshot", Sequence: 5, ParentID: active.ID, Type: memory.EventContextSnapshot, Payload: encodedSnapshot,
	})
	if err := validateDurableContextHistory(events); err == nil {
		t.Fatal("durable context accepted an automatic snapshot frontier different from its compaction")
	}
}
