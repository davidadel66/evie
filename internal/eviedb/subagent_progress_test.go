package eviedb_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

func TestSubagentsOfCallFollowsAChildFromQueuedToItsLatestAction(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	list, err := store.SubagentsOfCall(ctx, parent.Scope.SessionID, parent.SourceEventID, "call")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ExecutionID != attempts[0].ID || list[0].State != "admitted" || list[0].Worker != "research" ||
		list[0].Objective != "find evidence" || list[0].Actions != 0 || list[0].Latest != nil || list[0].DeadlineMS <= 0 {
		t.Fatalf("queued child = %+v", list)
	}

	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	lease, err := store.AcquireTurnLease(ctx, a.Child.ID, "child-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendChild := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	root := appendChild(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "assignment"})
	for i, query := range []string{"first query", "scaling laws 2026"} {
		payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "search", Name: "web_search", Arguments: `{"query":"` + query + `"}`}})
		execution := memory.ExecutionID("search-" + string(rune('a'+i)))
		intent := appendChild(memory.EventInput{ParentID: root.ID, Type: memory.EventToolIntent, ExecutionID: execution, Payload: payload})
		if i == 0 {
			appendChild(memory.EventInput{ParentID: intent.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: execution, Content: "results", Payload: json.RawMessage(`{"tool_call_id":"search","is_error":false}`)})
		}
	}
	list, err = store.SubagentsOfCall(ctx, parent.Scope.SessionID, parent.SourceEventID, "call")
	if err != nil {
		t.Fatal(err)
	}
	if got := list[0]; got.State != "running" || got.StartedAt == nil || got.Actions != 2 || got.Latest == nil ||
		got.Latest.Name != "web_search" || got.Latest.Args != `{"query":"scaling laws 2026"}` || got.Latest.Done {
		t.Fatalf("running child = %+v latest %+v", got, got.Latest)
	}

	progress, events, err := store.SubagentEvents(ctx, parent.Scope.SessionID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if progress.ExecutionID != a.ID || len(events) != 4 || events[0].Type != memory.EventUserMessage || events[0].SessionID != a.Child.ID {
		t.Fatalf("child events = %+v %+v", progress, events)
	}
}

func TestSubagentsOfCallStaysWithinTheParentSession(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	// Another top-level session whose own call has the same ID and key.
	other, err := store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, other.ID, "other-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root := appendParentCall(t, store, other.ID, lease, "", "call", delegation.ToolName, `{"assignments":[{"idempotency_key":"one","objective":"find evidence"}]}`)
	if list, err := store.SubagentsOfCall(ctx, other.ID, root, "call"); err != nil || len(list) != 0 {
		t.Fatalf("another session listed %+v %v", list, err)
	}
	if _, _, err := store.SubagentEvents(ctx, other.ID, attempts[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("another session read a child: %v", err)
	}
	if list, err := store.SubagentsOfCall(ctx, parent.Scope.SessionID, parent.SourceEventID, "other-call"); err != nil || len(list) != 0 {
		t.Fatalf("unknown call listed %+v %v", list, err)
	}
}

func TestSubagentsOfCallTellsCallsApartByTurnAndAssignment(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	first, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	session := parent.Scope.SessionID
	// Provider call IDs repeat across turns: a later web search reuses it...
	searchTurn := appendParentCall(t, store, session, parent.Lease, "", "call", "web_search", `{"query":"x"}`)
	// ...and so does a later delegation of different work.
	laterTurn := appendParentCall(t, store, session, parent.Lease, "", "call", delegation.ToolName, `{"assignments":[{"idempotency_key":"two","objective":"other work"}]}`)
	later := parent
	later.SourceEventID, later.IntentEventID = laterTurn, latestIntent(t, store, session)
	second, err := store.AdmitSubagents(ctx, later, []delegation.Assignment{{Key: "two", Objective: "other work"}}, receipt, delegation.DefaultPolicy(), "dispatch-2")
	if err != nil {
		t.Fatal(err)
	}
	// A retry with the same key and the same work resolves to the first child;
	// one that reuses the key for different work was refused and started none.
	replayTurn := appendParentCall(t, store, session, parent.Lease, "", "retry", delegation.ToolName, `{"assignments":[{"idempotency_key":"one","objective":"find evidence"}]}`)
	refusedTurn := appendParentCall(t, store, session, parent.Lease, "", "refused", delegation.ToolName, `{"assignments":[{"idempotency_key":"one","objective":"changed"}]}`)
	for _, tc := range []struct {
		name       string
		turn       memory.EventID
		call, want string
	}{
		{"first call", parent.SourceEventID, "call", first[0].ID},
		{"web search reusing the ID", searchTurn, "call", ""},
		{"later delegation reusing the ID", laterTurn, "call", second[0].ID},
		{"replayed key", replayTurn, "retry", first[0].ID},
		{"refused key", refusedTurn, "refused", ""},
	} {
		list, err := store.SubagentsOfCall(ctx, session, tc.turn, tc.call)
		if err != nil {
			t.Fatal(err)
		}
		if (tc.want == "" && len(list) != 0) || (tc.want != "" && (len(list) != 1 || list[0].ExecutionID != tc.want)) {
			t.Fatalf("%s listed %+v", tc.name, list)
		}
	}
}

func TestSubagentProgressKeepsAContinuationOutOfTheOriginalAttempt(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	original, _ := finishedChild(t, store, parent, receipt, "## Summary\nfirst report")
	p := continueTurn(t, store, parent, original.ID, "go on")
	c, err := store.AdmitSubagentContinuation(ctx, p, original.ID, "go on", delegation.DefaultPolicy(), "continuation")
	if err != nil {
		t.Fatal(err)
	}
	if c, _, err = store.StartSubagent(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, c.Child.ID, "child-holder-2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendParentCall(t, store, c.Child.ID, lease, "go on", "search", "web_search", `{"query":"continuation query"}`)

	list, err := store.SubagentsOfCall(ctx, parent.Scope.SessionID, parent.SourceEventID, "call")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ExecutionID != original.ID || list[0].Actions != 0 || list[0].Latest != nil {
		t.Fatalf("original attempt took the continuation's work: %+v", list)
	}
	_, events, err := store.SubagentEvents(ctx, parent.Scope.SessionID, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Content != "## Summary\nfirst report" {
		t.Fatalf("original trace = %+v", events)
	}

	continued, err := store.SubagentsOfCall(ctx, parent.Scope.SessionID, p.SourceEventID, callIDOf(t, store, parent.Scope.SessionID, p.IntentEventID))
	if err != nil {
		t.Fatal(err)
	}
	if len(continued) != 1 || continued[0].ExecutionID != c.ID || continued[0].Continues != original.ID || continued[0].Actions != 1 || continued[0].Latest == nil || continued[0].Latest.Done {
		t.Fatalf("continuation = %+v", continued)
	}
	if _, events, err = store.SubagentEvents(ctx, parent.Scope.SessionID, c.ID); err != nil || len(events) != 2 || events[0].Content != "go on" {
		t.Fatalf("continuation trace = %+v %v", events, err)
	}
}

// appendParentCall starts a turn in session (with text, or "research") and
// commits one tool intent in it, returning the turn's root event.
func appendParentCall(t *testing.T, store *eviedb.Store, session memory.SessionID, lease memory.TurnLease, text, callID, name, args string) memory.EventID {
	t.Helper()
	ctx := context.Background()
	if text == "" {
		text = "research"
	}
	root, err := store.AppendEventWithLease(ctx, session, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: callID, Name: name, Arguments: args}})
	if _, err = store.AppendEventWithLease(ctx, session, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: root.ID, Type: memory.EventToolIntent, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload}); err != nil {
		t.Fatal(err)
	}
	return root.ID
}

func latestIntent(t *testing.T, store *eviedb.Store, session memory.SessionID) memory.EventID {
	t.Helper()
	events, err := store.LoadEvents(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Type == memory.EventToolIntent {
			return events[i].ID
		}
	}
	t.Fatal("no tool intent")
	return ""
}

func callIDOf(t *testing.T, store *eviedb.Store, session memory.SessionID, intent memory.EventID) string {
	t.Helper()
	events, err := store.LoadEvents(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.ID == intent {
			var p memory.ToolIntentPayload
			if err := json.Unmarshal(event.Payload, &p); err != nil {
				t.Fatal(err)
			}
			return p.Call.ID
		}
	}
	t.Fatalf("no intent %s", intent)
	return ""
}

func TestBoundToolArgumentsKeepsJSONAndItsSubject(t *testing.T) {
	args := `{"path":"internal/web/subagents.go","content":"` + strings.Repeat("x", 9000) + `"}`
	bounded := eviedb.BoundToolArguments(args, 1024)
	var fields struct {
		Path string `json:"path"`
	}
	if len(bounded) > 1024 || json.Unmarshal([]byte(bounded), &fields) != nil || fields.Path != "internal/web/subagents.go" {
		t.Fatalf("bounded = %d bytes %q", len(bounded), bounded[:80])
	}
	if eviedb.BoundToolArguments(`{"query":"short"}`, 1024) != `{"query":"short"}` {
		t.Fatal("short arguments changed")
	}
}
