package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

// finishedChild runs one admitted child to an accepted report through the
// store, leaving its child lease held, as after a crash right after the
// report was accepted.
func finishedChild(t *testing.T, store *eviedb.Store, parent delegation.Parent, receipt composition.Receipt, report string) (delegation.Attempt, memory.TurnLease) {
	t.Helper()
	ctx := context.Background()
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	lease, err := store.AcquireTurnLease(ctx, a.Child.ID, "child-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "assignment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: report}); err != nil {
		t.Fatal(err)
	}
	if a, err = store.FinishSubagent(ctx, a.ID, "succeeded", ""); err != nil || a.State != "succeeded" {
		t.Fatalf("finish: %+v %v", a, err)
	}
	return a, lease
}

// continueTurn starts a new parent turn and commits a continue_research
// intent in it.
func continueTurn(t *testing.T, store *eviedb.Store, p delegation.Parent, executionID, message string) delegation.Parent {
	t.Helper()
	ctx := context.Background()
	root, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "continue"})
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(delegation.Continuation{ExecutionID: executionID, Message: message})
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call-" + uuid.NewString(), Name: delegation.ContinueToolName, Arguments: string(args)}})
	intent, err := store.AppendEventWithLease(ctx, p.Scope.SessionID, p.Lease.HolderID, p.Lease.FencingToken, memory.EventInput{ParentID: root.ID, Type: memory.EventToolIntent, ExecutionID: memory.ExecutionID(uuid.NewString()), Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	p.SourceEventID, p.IntentEventID = root.ID, intent.ID
	return p
}

func eventCount(t *testing.T, store *eviedb.Store, session memory.SessionID) int {
	t.Helper()
	events, err := store.LoadEvents(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	return len(events)
}

// G10: a crash during a continuation recovers exactly like a fresh attempt.
// Recovery reads only the continuation's own turn: an earlier report never
// counts as its answer, while a report it accepted before the crash does.
// The child's earlier attempt is untouched and the child can be continued
// again from its latest report.
func TestRecoveryAfterCrashMidContinuation(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprintf("accepted=%t", accepted), func(t *testing.T) {
			ctx := context.Background()
			store, db, path, parent, receipt := newSubagentAdmission(t)
			original, staleLease := finishedChild(t, store, parent, receipt, "## Summary\nfirst report https://example.com/a")
			p := continueTurn(t, store, parent, original.ID, "go on")
			c, err := store.AdmitSubagentContinuation(ctx, p, original.ID, "go on", delegation.DefaultPolicy(), "continuation")
			if err != nil {
				t.Fatal(err)
			}
			if c.State != "admitted" || c.ID == original.ID || c.Child.ID != original.Child.ID || c.Continues == nil || c.Continues.ExecutionID != original.ID {
				t.Fatalf("continuation attempt: %+v", c)
			}
			c, started, err := store.StartSubagent(ctx, c.ID)
			if err != nil || !started {
				t.Fatalf("start continuation: %t %v", started, err)
			}
			child, err := store.GetSession(ctx, c.Child.ID)
			if err != nil || child.Status != memory.SessionActive {
				t.Fatalf("child not reopened: %+v %v", child, err)
			}
			// The earlier attempt's lease is fenced out; the continuation
			// acquires its own.
			lease, err := store.AcquireTurnLease(ctx, c.Child.ID, "child-holder", time.Minute)
			if err != nil {
				t.Fatalf("continuation cannot own its child: %v", err)
			}
			if _, err = store.AppendEventWithLease(ctx, c.Child.ID, staleLease.HolderID, staleLease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "stale"}); err == nil {
				t.Fatal("earlier child lease wrote into the continuation")
			}
			root, err := store.AppendEventWithLease(ctx, c.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go on"})
			if err != nil {
				t.Fatal(err)
			}
			if accepted {
				if _, err = store.AppendEventWithLease(ctx, c.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "## Summary\nsecond report"}); err != nil {
					t.Fatal(err)
				}
			}
			before := eventCount(t, store, c.Child.ID)
			db.Close()
			db, err = eviedb.OpenDBAt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store = eviedb.NewStore(db)
			if n, err := store.RecoverSubagents(ctx); err != nil || n != 0 {
				t.Fatalf("recovered live continuation: %d %v", n, err)
			}
			if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			if n, err := store.RecoverSubagents(ctx); err != nil || n != 1 {
				t.Fatalf("recovery=%d %v", n, err)
			}
			retained, err := store.InspectSubagent(ctx, p, c.ID)
			if err != nil || retained.Result == nil || retained.Result.ContinuesExecutionID != original.ID {
				t.Fatalf("recovered continuation: %+v %v", retained, err)
			}
			if accepted && (retained.State != "succeeded" || retained.Result.Summary != "second report") {
				t.Fatalf("accepted continuation report lost: %+v", retained.Result)
			}
			if !accepted && (retained.State != "interrupted" || retained.Result.Reason != "original_parent_ownership_ended" || retained.Result.Summary != "" || retained.FinalEventID != "") {
				t.Fatalf("earlier report counted as the continuation's: %+v", retained.Result)
			}
			if after := eventCount(t, store, c.Child.ID); after != before {
				t.Fatalf("recovery fabricated history: %d -> %d", before, after)
			}
			kept, err := store.InspectSubagent(ctx, p, original.ID)
			if err != nil || kept.State != "succeeded" || kept.Result.Summary != "first report https://example.com/a" {
				t.Fatalf("earlier attempt changed: %+v %v", kept, err)
			}
			if child, err = store.GetSession(ctx, c.Child.ID); err != nil || child.Status != memory.SessionClosed {
				t.Fatalf("child left open: %+v %v", child, err)
			}
			// A new parent turn can continue from the child's latest report.
			parent.Lease, err = store.AcquireTurnLease(ctx, parent.Scope.SessionID, "orchestrator-2", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			latest, superseded := original.ID, c.ID
			if accepted {
				latest, superseded = c.ID, original.ID
			}
			again := continueTurn(t, store, parent, superseded, "again")
			if _, err = store.AdmitSubagentContinuation(ctx, again, superseded, "again", delegation.DefaultPolicy(), "again"); !errors.Is(err, delegation.ErrNotResumable) {
				t.Fatalf("continued %s, not the latest report: %v", superseded, err)
			}
			again = continueTurn(t, store, parent, latest, "again")
			if next, err := store.AdmitSubagentContinuation(ctx, again, latest, "again", delegation.DefaultPolicy(), "again"); err != nil || next.Continues.ExecutionID != latest {
				t.Fatalf("latest report not continuable: %+v %v", next, err)
			}
		})
	}
}

// G10: a parent composed without continue_research cannot authorize one,
// and a delegation intent cannot be reused as a continuation.
func TestContinuationRequiresTheContinueCapabilityAndIntent(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmissionWith(t, delegation.CapabilityID)
	original, _ := finishedChild(t, store, parent, receipt, "## Summary\nreport")
	p := continueTurn(t, store, parent, original.ID, "go on")
	if _, err := store.AdmitSubagentContinuation(ctx, p, original.ID, "go on", delegation.DefaultPolicy(), "continuation"); err == nil || !strings.Contains(err.Error(), "continu") {
		t.Fatalf("parent without the capability continued: %v", err)
	}
	store, _, _, parent, receipt = newSubagentAdmission(t)
	original, _ = finishedChild(t, store, parent, receipt, "## Summary\nreport")
	if _, err := store.AdmitSubagentContinuation(ctx, parent, original.ID, "go on", delegation.DefaultPolicy(), "continuation"); err == nil {
		t.Fatal("delegation intent authorized a continuation")
	}
}

// The subagent table as Stage 8 (2026-10-01) created it: one attempt per child.
const stage8SubagentTable = `CREATE TABLE subagent_executions_stage8 (
 id TEXT PRIMARY KEY, parent_session_id TEXT NOT NULL REFERENCES sessions(id),
 idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
 child_session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id),
 state TEXT NOT NULL CHECK(state IN ('admitted','running','succeeded','partial','failed','cancelled','interrupted')),
 record_json TEXT NOT NULL, UNIQUE(parent_session_id,idempotency_key)
 );
 INSERT INTO subagent_executions_stage8(rowid,id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json)
 SELECT rowid,id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json FROM subagent_executions;
 DROP TABLE subagent_executions;
 ALTER TABLE subagent_executions_stage8 RENAME TO subagent_executions;
 CREATE INDEX subagent_execution_state ON subagent_executions(state,parent_session_id);`

// G10: startup lets one child session hold several attempts. A table from
// before continuation is rebuilt with every row unchanged, and an attempt
// recorded before the budget amendment (legacy policy and result) is
// continued under the current policy.
func TestEarlierSubagentTableMigratesAndItsAttemptsContinue(t *testing.T) {
	ctx := context.Background()
	store, db, path, parent, receipt := newSubagentAdmission(t)
	original, _ := finishedChild(t, store, parent, receipt, "old findings https://example.com/a")
	var raw string
	if err := db.QueryRow(`SELECT record_json FROM subagent_executions WHERE id=?`, original.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		t.Fatal(err)
	}
	record["policy"] = json.RawMessage(preBudgetPolicy)
	record["result"] = json.RawMessage(`{"execution_id":"` + original.ID + `","child_session_id":"` + string(original.Child.ID) + `","status":"succeeded","findings":"old findings https://example.com/a","sources":["https://example.com/a"],"usage":{"total_tokens":40}}`)
	encoded, _ := json.Marshal(record)
	if _, err := db.Exec(stage8SubagentTable); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE subagent_executions SET record_json=? WHERE id=?`, string(encoded), original.ID); err != nil {
		t.Fatal(err)
	}
	var rowid int64
	if err := db.QueryRow(`SELECT rowid,record_json FROM subagent_executions WHERE id=?`, original.ID).Scan(&rowid, &raw); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	var definition string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='subagent_executions'`).Scan(&definition); err != nil ||
		strings.Contains(definition, "UNIQUE REFERENCES") || !strings.Contains(definition, "'partial'") {
		t.Fatalf("table not migrated: %s %v", definition, err)
	}
	for _, index := range []string{"subagent_execution_state", "subagent_execution_child", "subagent_execution_live_child"} {
		var n int
		if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, index).Scan(&n); err != nil || n != 1 {
			t.Fatalf("index %s: %d %v", index, n, err)
		}
	}
	var migratedRowid int64
	var migrated string
	if err = db.QueryRow(`SELECT rowid,record_json FROM subagent_executions WHERE id=?`, original.ID).Scan(&migratedRowid, &migrated); err != nil || migratedRowid != rowid || migrated != raw {
		t.Fatalf("row changed by migration: %d/%d %v", migratedRowid, rowid, err)
	}
	p := continueTurn(t, store, parent, original.ID, "go on")
	c, err := store.AdmitSubagentContinuation(ctx, p, original.ID, "go on", delegation.DefaultPolicy(), "continuation")
	if err != nil {
		t.Fatalf("pre-amendment attempt cannot be continued: %v", err)
	}
	if c.Policy != delegation.DefaultPolicy() || c.Child.ID != original.Child.ID {
		t.Fatalf("continuation policy/child: %+v", c)
	}
	// One child session holds at most one unfinished attempt.
	if _, err = db.Exec(`INSERT INTO subagent_executions(id,parent_session_id,idempotency_key,request_digest,child_session_id,state,record_json) VALUES(?,?,?,?,?,'running','{}')`,
		uuid.NewString(), parent.Scope.SessionID, "duplicate", "digest", original.Child.ID); err == nil {
		t.Fatal("second unfinished attempt admitted for one child")
	}
	reopened, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatalf("migrated table does not reopen: %v", err)
	}
	reopened.Close()
}
