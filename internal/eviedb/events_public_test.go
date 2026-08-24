package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestBoundTerminalMutationCanonicalizesAndProvesTurnAncestryAcrossStores(t *testing.T) {
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
	lease, err := storeA.AcquireTurnLease(ctx, session.ID, "holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	history := storeB.BindHistory(session.ID, lease.HolderID)
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := history.Append(ctx, lease, input)
		if err != nil {
			t.Fatalf("append fixture: %v", err)
		}
		return event
	}
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "root"})

	otherSession, err := storeA.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	otherLease, err := storeA.AcquireTurnLease(ctx, otherSession.ID, "other-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	otherHistory := storeA.BindHistory(otherSession.ID, otherLease.HolderID)
	crossSessionRoot, err := otherHistory.Append(ctx, otherLease, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "cross session",
	})
	if err != nil {
		t.Fatal(err)
	}

	terminalPayload := func(turnID memory.EventID) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(
			`{"turn_id":%q,"classification":"provider_error","stage":"provider"}`,
			turnID,
		))
	}
	base := memory.TurnTerminalPayload{
		TurnID: root.ID, Classification: memory.ClassificationProviderError, Stage: memory.StageProvider,
	}
	invalid := []struct {
		name     string
		parentID memory.EventID
		payload  json.RawMessage
	}{
		{name: "missing turn ID", parentID: root.ID, payload: terminalPayload("")},
		{name: "nonexistent turn ID", parentID: root.ID, payload: terminalPayload("missing")},
		{name: "cross-session turn ID", parentID: root.ID, payload: terminalPayload(crossSessionRoot.ID)},
		{name: "sensitive turn ID", parentID: root.ID, payload: terminalPayload("https://provider.invalid/secret?token=value")},
		{name: "missing parent", payload: terminalPayload(root.ID)},
		{name: "nonexistent parent", parentID: "missing-parent", payload: terminalPayload(root.ID)},
		{name: "cross-session parent", parentID: crossSessionRoot.ID, payload: terminalPayload(root.ID)},
		{name: "sensitive parent", parentID: "https://provider.invalid/secret?token=value", payload: terminalPayload(root.ID)},
		{
			name:     "duplicate allowlisted key",
			parentID: root.ID,
			payload: json.RawMessage(fmt.Sprintf(
				`{"turn_id":%q,"turn_id":%q,"classification":"provider_error","stage":"provider"}`,
				root.ID,
				root.ID,
			)),
		},
		{
			name:     "unknown terminal field",
			parentID: root.ID,
			payload: json.RawMessage(fmt.Sprintf(
				`{"turn_id":%q,"classification":"provider_error","stage":"provider","detail":"secret"}`,
				root.ID,
			)),
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if event, err := history.Append(ctx, lease, memory.EventInput{
				ParentID: test.parentID,
				Type:     memory.EventTurnFailed,
				Content:  base.SafeContent(),
				Payload:  test.payload,
			}); err == nil {
				t.Fatalf("invalid terminal appended: %+v", event)
			}
		})
	}

	rootTerminal, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: root.ID,
		Type:     memory.EventTurnFailed,
		Content:  base.SafeContent(),
		Payload: json.RawMessage(fmt.Sprintf(
			`{ "stage" : "provider", "turn_id" : %q, "classification" : "provider_error" }`,
			root.ID,
		)),
	})
	if err != nil {
		t.Fatalf("append root terminal: %v", err)
	}
	canonical, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	if string(rootTerminal.Payload) != string(canonical) {
		t.Fatalf("root terminal payload=%s, want canonical %s", rootTerminal.Payload, canonical)
	}
	publicTerminal, err := storeA.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		ParentID: root.ID,
		Type:     memory.EventTurnFailed,
		Content:  base.SafeContent(),
		Payload: json.RawMessage(fmt.Sprintf(
			`{"stage":"provider","classification":"provider_error","turn_id":%q}`,
			root.ID,
		)),
	})
	if err != nil {
		t.Fatalf("append canonical public terminal: %v", err)
	}
	if string(publicTerminal.Payload) != string(canonical) {
		t.Fatalf("public terminal payload=%s, want canonical %s", publicTerminal.Payload, canonical)
	}

	assistant := appendEvent(memory.EventInput{
		ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Payload: json.RawMessage(`{"tool_calls":[{"id":"call-1","name":"echo","arguments":"{}"}]}`),
	})
	intent := appendEvent(memory.EventInput{
		ParentID: assistant.ID, Type: memory.EventToolIntent, ExecutionID: "execution-1",
		Payload: json.RawMessage(`{"call":{"id":"call-1","name":"echo","arguments":"{}"}}`),
	})
	outcome := appendEvent(memory.EventInput{
		ParentID: intent.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool,
		ExecutionID: "execution-1", Payload: json.RawMessage(`{"tool_call_id":"call-1","is_error":false}`),
	})
	for _, test := range []struct {
		name     string
		parentID memory.EventID
	}{
		{name: "wrong parent type", parentID: assistant.ID},
		{name: "superseded provider trigger", parentID: root.ID},
	} {
		t.Run(test.name, func(t *testing.T) {
			if event, err := history.Append(ctx, lease, memory.EventInput{
				ParentID: test.parentID,
				Type:     memory.EventTurnFailed,
				Content:  base.SafeContent(),
				Payload:  terminalPayload(root.ID),
			}); err == nil {
				t.Fatalf("invalid terminal appended: %+v", event)
			}
		})
	}
	secondCycle, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: outcome.ID,
		Type:     memory.EventTurnFailed,
		Content:  base.SafeContent(),
		Payload:  terminalPayload(root.ID),
	})
	if err != nil {
		t.Fatalf("append second-cycle terminal: %v", err)
	}
	if secondCycle.ParentID != outcome.ID {
		t.Fatalf("second-cycle terminal=%+v", secondCycle)
	}

	incompleteAssistant := appendEvent(memory.EventInput{
		ParentID: outcome.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Payload: json.RawMessage(`{"tool_calls":[{"id":"partial-1","name":"echo","arguments":"{}"},{"id":"partial-2","name":"echo","arguments":"{}"}]}`),
	})
	incompleteIntent := appendEvent(memory.EventInput{
		ParentID: incompleteAssistant.ID, Type: memory.EventToolIntent, ExecutionID: "execution-partial-1",
		Payload: json.RawMessage(`{"call":{"id":"partial-1","name":"echo","arguments":"{}"}}`),
	})
	partialOutcome := appendEvent(memory.EventInput{
		ParentID: incompleteIntent.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool,
		ExecutionID: "execution-partial-1", Payload: json.RawMessage(`{"tool_call_id":"partial-1","is_error":false}`),
	})
	appendEvent(memory.EventInput{
		ParentID: incompleteAssistant.ID, Type: memory.EventToolIntent, ExecutionID: "execution-partial-2",
		Payload: json.RawMessage(`{"call":{"id":"partial-2","name":"echo","arguments":"{}"}}`),
	})
	if event, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: partialOutcome.ID,
		Type:     memory.EventTurnFailed,
		Content:  base.SafeContent(),
		Payload:  terminalPayload(root.ID),
	}); err == nil {
		t.Fatalf("incomplete group outcome accepted as provider trigger: %+v", event)
	}
	if _, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: outcome.ID,
		Type:     memory.EventTurnFailed,
		Content:  base.SafeContent(),
		Payload:  terminalPayload(root.ID),
	}); err != nil {
		t.Fatalf("append terminal against trigger preceding incomplete group: %v", err)
	}

	otherRoot := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "other turn"})
	if event, err := history.Append(ctx, lease, memory.EventInput{
		ParentID: otherRoot.ID,
		Type:     memory.EventTurnFailed,
		Content:  base.SafeContent(),
		Payload:  terminalPayload(root.ID),
	}); err == nil {
		t.Fatalf("cross-turn terminal appended: %+v", event)
	}
}
