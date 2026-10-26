package eviedb

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// copyFinishedWorkflowRuns copies a started run into n succeeded runs, the
// history long retention leaves behind.
func copyFinishedWorkflowRuns(t *testing.T, r *runDB, template string, n int) {
	t.Helper()
	rows, err := r.s.db.Query(`SELECT name FROM pragma_table_info('workflow_runs')`)
	if err != nil {
		t.Fatal(err)
	}
	var columns, values []string
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatal(err)
		}
		value := column
		switch column {
		case "id":
			value = "'finished-'||n"
		case "run_key":
			value = "'finished-key-'||n"
		case "state":
			value = "'succeeded'"
		case "finished_at":
			value = "updated_at"
		case "outputs":
			value = "'{}'"
		}
		columns, values = append(columns, column), append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if _, err := r.s.db.Exec(fmt.Sprintf(`WITH RECURSIVE k(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM k WHERE n<?)
 INSERT INTO workflow_runs(%s) SELECT %s FROM workflow_runs, k WHERE id=?`, strings.Join(columns, ","), strings.Join(values, ",")), n, template); err != nil {
		t.Fatal(err)
	}
}

// dueRunsWithHistory starts one queued run beside finished history.
func dueRunsWithHistory(t *testing.T, r *runDB, finished int) WorkflowRun {
	t.Helper()
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(context.Background(), runStart(a, "due"))
	if err != nil {
		t.Fatal(err)
	}
	copyFinishedWorkflowRuns(t, r, run.ID, finished)
	return run
}

// The scheduler asks for due runs once a second. Finished runs must not be
// read to answer it, however many retention keeps.
func TestDueWorkflowRunsReadOnlyUnfinishedRuns(t *testing.T) {
	r, _ := openLeasedRunDB(t)
	run := dueRunsWithHistory(t, r, 2000)
	ids, err := r.s.DueWorkflowRuns(context.Background(), 10)
	if err != nil || len(ids) != 1 || ids[0] != run.ID {
		t.Fatalf("due = %v, %v", ids, err)
	}
	now := formatLeaseTime(r.clock())
	rows, err := r.s.db.Query(`EXPLAIN QUERY PLAN `+dueWorkflowRunsSQL, now, now, now, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan[0], "USING INDEX workflow_runs_due") {
		t.Fatalf("due runs are not read through workflow_runs_due:\n%s", strings.Join(plan, "\n"))
	}
}

// A database from before workflow_runs_due trades the narrower index for it on
// its next open.
func TestOpeningReplacesTheUnfinishedRunsIndex(t *testing.T) {
	r := openRunDB(t)
	if _, err := r.db.Exec(`DROP INDEX workflow_runs_due;
 CREATE INDEX workflow_runs_unfinished ON workflow_runs(state) WHERE state IN ('queued','running','waiting_for_dependency')`); err != nil {
		t.Fatal(err)
	}
	r.reopen()
	var indexes []string
	rows, err := r.db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND name IN ('workflow_runs_due','workflow_runs_unfinished') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(indexes, ",") != "workflow_runs_due" {
		t.Fatalf("indexes after upgrade = %v, want only workflow_runs_due", indexes)
	}
}
