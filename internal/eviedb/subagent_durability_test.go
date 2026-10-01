package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// appendToolRound appends one model round under the parent's lease, linked the
// way the agent loop links it: assistant -> tool intent -> tool outcome.
func appendToolRound(t *testing.T, store *eviedb.Store, p delegation.Parent, prior memory.EventID, call memory.ToolCall) (memory.EventID, memory.EventID) {
	t.Helper()
	ctx := context.Background()
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	assistant := appendEvent(memory.EventInput{ParentID: prior, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "working"})
	execution := memory.ExecutionID("execution-" + call.ID)
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: call})
	intent := appendEvent(memory.EventInput{ParentID: assistant.ID, Type: memory.EventToolIntent, ExecutionID: execution, Payload: payload})
	if call.Name == delegation.ToolName {
		return intent.ID, ""
	}
	result, _ := json.Marshal(memory.ToolResultPayload{ToolCallID: call.ID})
	outcome := appendEvent(memory.EventInput{ParentID: intent.ID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: execution, Content: "result", Payload: result})
	return intent.ID, outcome.ID
}

func TestSubagentAuthorityHoldsLateInALongParentTurn(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	prior := parent.SourceEventID
	// Each round adds three parent links; 110 rounds put the intent far past
	// any bounded ancestry walk.
	for i := 0; i < 110; i++ {
		_, prior = appendToolRound(t, store, parent, prior, memory.ToolCall{ID: fmt.Sprint("search-", i), Name: "web_search", Arguments: `{}`})
	}
	requests := []delegation.Assignment{{Key: "late", Objective: "find evidence"}}
	args, _ := json.Marshal(struct {
		Assignments []delegation.Assignment `json:"assignments"`
	}{requests})
	late := parent
	late.IntentEventID, _ = appendToolRound(t, store, parent, prior, memory.ToolCall{ID: "late", Name: delegation.ToolName, Arguments: string(args)})
	attempts, err := store.AdmitSubagents(ctx, late, requests, receipt, delegation.DefaultPolicy(), "late-dispatch")
	if err != nil {
		t.Fatalf("delegation refused late in the parent turn: %v", err)
	}
	if _, started, err := store.StartSubagent(ctx, attempts[0].ID); err != nil || !started {
		t.Fatalf("start=%t %v", started, err)
	}
	if err = store.AuthorizeSubagent(ctx, attempts[0].ID); err != nil {
		t.Fatalf("running child lost authority late in the parent turn: %v", err)
	}
}

func TestSubagentAuthorityStillRequiresTheCurrentTurnAndAnOutstandingIntent(t *testing.T) {
	ctx := context.Background()
	requests := []delegation.Assignment{{Key: "one", Objective: "find evidence"}}
	for name, change := range map[string]func(*testing.T, *eviedb.Store, *delegation.Parent){
		"completed intent": func(t *testing.T, store *eviedb.Store, p *delegation.Parent) {
			result, _ := json.Marshal(memory.ToolResultPayload{ToolCallID: "call"})
			if _, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{ParentID: p.IntentEventID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: "invocation", Content: "done", Payload: result}); err != nil {
				t.Fatal(err)
			}
		},
		"intent before a later turn": func(t *testing.T, store *eviedb.Store, p *delegation.Parent) {
			if _, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "next turn"}); err != nil {
				t.Fatal(err)
			}
		},
		"intent attributed to a later turn": func(t *testing.T, store *eviedb.Store, p *delegation.Parent) {
			next, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "next turn"})
			if err != nil {
				t.Fatal(err)
			}
			p.SourceEventID = next.ID
		},
		"source is not a turn root": func(t *testing.T, store *eviedb.Store, p *delegation.Parent) {
			assistant, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{ParentID: p.SourceEventID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "working"})
			if err != nil {
				t.Fatal(err)
			}
			p.SourceEventID = assistant.ID
		},
	} {
		t.Run(name, func(t *testing.T) {
			store, _, _, parent, receipt := newSubagentAdmission(t)
			change(t, store, &parent)
			if _, err := store.AdmitSubagents(ctx, parent, requests, receipt, delegation.DefaultPolicy(), "dispatch"); !errors.Is(err, delegation.ErrAuthority) {
				t.Fatalf("admission without current turn lineage: %v", err)
			}
		})
	}
}

func TestRecoveryInterruptsAdmittedAttemptThatNeverStartedAfterReopen(t *testing.T) {
	ctx := context.Background()
	store, db, path, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "crashed-before-start")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	if n, err := store.RecoverSubagents(ctx); err != nil || n != 0 {
		t.Fatalf("recovered work whose parent still owns it: %d %v", n, err)
	}
	if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if n, err := store.RecoverSubagents(ctx); err != nil || n != 1 {
		t.Fatalf("recovery=%d %v", n, err)
	}
	retained, err := store.InspectSubagent(ctx, parent, attempts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.State != "interrupted" || retained.StartedAt != nil || retained.Result == nil || retained.Result.Reason != "original_parent_ownership_ended" {
		t.Fatalf("never-started attempt: %+v", retained)
	}
	child, err := store.GetSession(ctx, attempts[0].Child.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadEvents(ctx, child.ID)
	if err != nil || len(events) != 0 || child.Status != memory.SessionClosed {
		t.Fatalf("recovery left a resumable child or fabricated history: %s %d %v", child.Status, len(events), err)
	}
}

func TestRecoveryIsolatesAnUnreadableRecord(t *testing.T) {
	ctx := context.Background()
	store, db, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES('corrupt-record',?,'corrupt','digest',?,'running','{not json')`, parent.Scope.SessionID, orphan.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	n, err := store.RecoverSubagents(ctx)
	if n != 1 || err == nil || !strings.Contains(err.Error(), "corrupt-record") {
		t.Fatalf("recovery=%d err=%v", n, err)
	}
	retained, err := store.InspectSubagent(ctx, parent, attempts[0].ID)
	if err != nil || retained.State != "interrupted" {
		t.Fatalf("healthy attempt blocked by a bad record: %+v %v", retained, err)
	}
	var state string
	if err = db.QueryRow(`SELECT state FROM subagent_executions WHERE id='corrupt-record'`).Scan(&state); err != nil || state != "running" {
		t.Fatalf("unreadable record was rewritten: %q %v", state, err)
	}
}
