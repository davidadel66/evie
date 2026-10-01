package eviedb_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
)

// The subagent table and record shapes written before the 2026-10-01 budget
// and result amendments.
const preBudgetSubagentTable = `CREATE TABLE subagent_executions_pre_budget (
 id TEXT PRIMARY KEY, parent_session_id TEXT NOT NULL REFERENCES sessions(id),
 idempotency_key TEXT NOT NULL, request_digest TEXT NOT NULL,
 child_session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id),
 state TEXT NOT NULL CHECK(state IN ('admitted','running','succeeded','failed','cancelled','interrupted')),
 record_json TEXT NOT NULL, UNIQUE(parent_session_id,idempotency_key)
 );
 INSERT INTO subagent_executions_pre_budget SELECT * FROM subagent_executions;
 DROP TABLE subagent_executions;
 ALTER TABLE subagent_executions_pre_budget RENAME TO subagent_executions;
 CREATE INDEX subagent_execution_state ON subagent_executions(state,parent_session_id);`

const preBudgetPolicy = `{"per_parent":2,"runtime":4,"max_batch":8,"deadline":120000000000,"model_calls":8,"assignment_bytes":8192,"request_bytes":1048576,"result_bytes":2048,"output_tokens":1024}`

// G1/G2/G3: records and schema from before the amendment still load,
// recover, and render; the migrated table accepts the new partial state.
func TestPreBudgetSubagentRecordsStillLoadAfterMigration(t *testing.T) {
	for _, state := range []string{"succeeded", "running"} {
		t.Run(state, func(t *testing.T) {
			ctx := context.Background()
			store, db, path, parent, receipt := newSubagentAdmission(t)
			attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
			if err != nil {
				t.Fatal(err)
			}
			id := attempts[0].ID
			var raw string
			if err = db.QueryRow(`SELECT record_json FROM subagent_executions WHERE id=?`, id).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var record map[string]json.RawMessage
			if err = json.Unmarshal([]byte(raw), &record); err != nil {
				t.Fatal(err)
			}
			record["policy"] = json.RawMessage(preBudgetPolicy)
			record["state"], _ = json.Marshal(state)
			started, _ := json.Marshal(time.Now().UTC().Add(-time.Minute))
			record["started_at"] = started
			oldResult := `{"execution_id":"` + id + `","child_session_id":"` + string(attempts[0].Child.ID) + `","status":"succeeded","findings":"old findings https://example.com/a","sources":["https://example.com/a"],"usage":{"total_tokens":40}}`
			if state == "succeeded" {
				record["ended_at"] = started
				record["result"] = json.RawMessage(oldResult)
			}
			encoded, _ := json.Marshal(record)
			if _, err = db.Exec(preBudgetSubagentTable); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`UPDATE subagent_executions SET state=?,record_json=? WHERE id=?`, state, string(encoded), id); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(`UPDATE subagent_executions SET state='partial' WHERE id=?`, id); err == nil {
				t.Fatal("fixture table already accepts partial")
			}
			db.Close()
			db, err = eviedb.OpenDBAt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store = eviedb.NewStore(db)
			var definition string
			if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='subagent_executions'`).Scan(&definition); err != nil || !strings.Contains(definition, "'partial'") {
				t.Fatalf("table not migrated: %s %v", definition, err)
			}
			var indexed int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='subagent_execution_state'`).Scan(&indexed); err != nil || indexed != 1 {
				t.Fatalf("state index lost: %d %v", indexed, err)
			}
			if state == "running" {
				// The earlier process's parent ownership has ended.
				if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
					t.Fatal(err)
				}
				if n, err := store.RecoverSubagents(ctx); err != nil || n != 1 {
					t.Fatalf("recovery=%d %v", n, err)
				}
			}
			loaded, err := store.InspectSubagent(ctx, parent, id)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Policy.LegacyModelCalls != 8 || loaded.Policy.LegacyOutputTokens != 1024 || loaded.Policy.ResultBytes != 2048 || loaded.Result == nil {
				t.Fatalf("pre-budget record: %+v", loaded)
			}
			if state == "succeeded" {
				rendered, _ := json.Marshal(loaded.Result)
				if string(rendered) != oldResult || loaded.Result.Findings != "old findings https://example.com/a" || loaded.Result.Sources[0].URL != "https://example.com/a" {
					t.Fatalf("old result renders as %s", rendered)
				}
			} else if loaded.State != "interrupted" || loaded.Result.Reason != "original_parent_ownership_ended" {
				t.Fatalf("old running record recovered as %+v", loaded.Result)
			}
			// A rewritten record keeps the retired limits it was pinned with.
			if err = db.QueryRow(`SELECT record_json FROM subagent_executions WHERE id=?`, id).Scan(&raw); err != nil || !strings.Contains(raw, `"model_calls":8`) || !strings.Contains(raw, `"output_tokens":1024`) {
				t.Fatalf("retired limits lost: %s %v", raw, err)
			}
			if _, err = db.Exec(`UPDATE subagent_executions SET state='partial' WHERE id=?`, id); err != nil {
				t.Fatalf("migrated table rejects partial: %v", err)
			}
		})
	}
}
