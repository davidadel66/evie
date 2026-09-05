package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

type countedHistoryDiscovery struct {
	*sql.Conn
	reads int
	t     *testing.T
}

func (q *countedHistoryDiscovery) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if strings.HasPrefix(query, "WITH RECURSIVE ancestry") {
		end := strings.LastIndex(query, ") SELECT ")
		if end < 0 {
			q.t.Fatal("ancestry counter no longer matches query")
		}
		var depth int
		if err := q.Conn.QueryRowContext(ctx, query[:end+1]+" SELECT COUNT(*) FROM ancestry", args...).Scan(&depth); err != nil {
			q.t.Fatal(err)
		}
		q.reads += depth - 1
	} else if strings.Contains(query, "payload_json") {
		q.reads++
	}
	return q.Conn.QueryRowContext(ctx, query, args...)
}

func TestCompilerHistoryDiscoveryHonors128EventAnd64MutationBounds(t *testing.T) {
	for _, depth := range []int{128, 129} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			f := newWorkerFixture(t)
			ctx := context.Background()
			var previous memory.Event
			for range depth {
				previous = activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: previous.ID, Content: "A bounded assertion."})
			}
			historySelect(t, f, "deep", historyRange(f, previous, previous))
			if err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
				var before, after int
				if err := conn.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&before); err != nil {
					return err
				}
				counted := &countedHistoryDiscovery{Conn: conn, t: t}
				var result memory.CompilerReconciliation
				if err := discoverCompilerHistory(ctx, counted, &result); err != nil {
					return err
				}
				if err := conn.QueryRowContext(ctx, `SELECT total_changes()`).Scan(&after); err != nil {
					return err
				}
				if counted.reads != 128 || after-before > 64 {
					t.Fatalf("source reads %d ledger mutations %d", counted.reads, after-before)
				}
				var state, reason string
				if err := conn.QueryRowContext(ctx, `SELECT state,reason FROM memory_compiler_history_roots`).Scan(&state, &reason); err != nil {
					return err
				}
				if depth == 128 && state != "selected_unmaterialized" {
					t.Fatal("valid boundary lost", state, reason)
				}
				if depth == 129 && (state != "failed" || reason != "source_inspection_limit") {
					t.Fatal("over-bound ancestry disappeared", state, reason)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCompilerHistoryDiscoveryUsesPendingIndexesAcrossRetainedHistory(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "An assertion.")
	historySelect(t, f, "pending", historyRange(f, root, end))
	// These retained request metadata fixtures represent completed and cancelled
	// histories. The ready indexes must exclude them instead of scanning them on
	// every background tick; original receipt/source data is never deleted.
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000) INSERT INTO memory_compiler_history_requests(request_id,request_hash,owner_id,generation_id,selection_order,receipt,pending_ranges,pending_roots,cancelled) SELECT 'retained-'||x,'fixture','local',generation_id,selection_order+x,receipt,CASE WHEN x>1000 THEN 1 ELSE 0 END,CASE WHEN x>1000 THEN 1 ELSE 0 END,CASE WHEN x>1000 THEN 1 ELSE 0 END FROM n CROSS JOIN memory_compiler_history_requests WHERE request_id='pending'`); err != nil {
		t.Fatal(err)
	}
	plans := []struct {
		query, index string
		args         []any
	}{
		{historyNextDiscoveryRequest, "memory_compiler_history_discovery_ready", nil},
		{historyNextRootRequest, "memory_compiler_history_roots_ready", nil},
		{historyNextDiscoveryRange, "memory_compiler_history_range_pending", []any{"pending"}},
		{historyNextRootRange, "memory_compiler_history_range_discovered", []any{"pending"}},
	}
	for _, test := range plans {
		rows, err := f.db.Query(`EXPLAIN QUERY PLAN `+test.query, test.args...)
		if err != nil {
			t.Fatal(err)
		}
		var details strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			details.WriteString(detail)
			details.WriteByte('\n')
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(details.String(), test.index) || strings.Contains(details.String(), "TEMP B-TREE") {
			t.Fatalf("not an indexed bounded choice: %s", details.String())
		}
	}
	historyReconcile(t, f, 4)
	var pending int
	if err := f.db.QueryRow(`SELECT pending_ranges+pending_roots FROM memory_compiler_history_requests WHERE request_id='pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatal("completed selection stayed in ready indexes", pending)
	}
	var chosen string
	if err := f.db.QueryRowContext(ctx, historyNextDiscoveryRequest).Scan(&chosen); err != sql.ErrNoRows {
		t.Fatal("retained cancelled/completed work remained ready", chosen, err)
	}
}

func TestCompilerHistoryWorkerGateProbesOneJobAndBoundedMemberReferences(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "An assertion.")
	historySelect(t, f, "gate", historyRange(f, root, end))
	historyReconcile(t, f, 4)
	var job string
	if err := f.db.QueryRow(`SELECT job_id FROM memory_compiler_jobs`).Scan(&job); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<2000) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'retained-gate-'||x,generation_id,destination,session_id,root_id,10000+x,10000+x,window_hash,request,'completed_empty' FROM n CROSS JOIN memory_compiler_jobs WHERE job_id=?`, job); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_history_jobs(job_id) SELECT job_id FROM memory_compiler_jobs WHERE job_id LIKE 'retained-gate-%'`); err != nil {
		t.Fatal(err)
	}
	for _, view := range []string{"memory_compiler_paused_jobs", "memory_compiler_invalid_claims"} {
		query := `SELECT job_id FROM memory_compiler_jobs j WHERE j.job_id=? AND NOT EXISTS(SELECT 1 FROM ` + view + ` p WHERE p.job_id=j.job_id)`
		rows, err := f.db.Query(`EXPLAIN QUERY PLAN `+query, job)
		if err != nil {
			t.Fatal(err)
		}
		var details strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			details.WriteString(detail)
			details.WriteByte('\n')
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		text := details.String()
		t.Log(text)
		if !strings.Contains(text, "memory_compiler_history_selection_refs") || strings.Contains(text, "SCAN h") || strings.Contains(text, "SCAN r") || strings.Contains(text, "MATERIALIZE p") {
			t.Fatalf("worker gate scans retained history: %s", text)
		}
		var got string
		if err := f.db.QueryRowContext(ctx, query, job).Scan(&got); err != nil || got != job {
			t.Fatal("indexed ready check", got, err)
		}
	}
}
