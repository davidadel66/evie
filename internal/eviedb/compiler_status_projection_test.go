package eviedb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func statusFixtureEvents(t *testing.T, f *workerFixture, n int) {
	t.Helper()
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?) INSERT INTO events(id,session_id,sequence,event_type,role,content,recorded_at,format_version) SELECT 'status-event-'||x,?,x,'user_message','user','PRIVATE status fixture',strftime('%Y-%m-%dT%H:%M:%fZ','now'),1 FROM n`, n, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
}

func completeActivationStatus(t *testing.T, f *workerFixture) memory.CompilerActivationStatus {
	t.Helper()
	for range 100 {
		got, err := f.store.InspectCompilerActivations(context.Background(), f.owner)
		if err == nil {
			return got
		}
		if !errors.Is(err, ErrCompilerStatusIndexing) {
			t.Fatal(err)
		}
	}
	t.Fatal("status cursor did not finish")
	return memory.CompilerActivationStatus{}
}

func TestCompilerStatusProjectionBoundedRestartAndTransactionalRootChanges(t *testing.T) {
	f := newWorkerFixture(t)
	a := activationStart(t, f)
	statusFixtureEvents(t, f, 513)
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_activation_roots(activation_id,session_id,root_id,first_sequence,last_sequence,position,state) SELECT ?,session_id,id,sequence,sequence,sequence,CASE WHEN sequence%2=0 THEN 'failed' ELSE 'selected_unmaterialized' END FROM events WHERE session_id=? AND sequence<=100`, a.ID, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	// Reopening a retained database with the new side projections absent creates
	// empty counters. It does not reconstruct counts in startup DDL.
	if _, err := f.db.Exec(`DROP TABLE memory_compiler_status_events;DROP TABLE memory_compiler_status_roots;DROP TABLE memory_compiler_status_history_revision;DROP TABLE memory_compiler_status_event_revision`); err != nil {
		t.Fatal(err)
	}
	f.store = f.second(t)
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_status_events`); n != 0 {
		t.Fatal("startup reconstructed event counts")
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_status_roots`); n != 0 {
		t.Fatal("startup reconstructed root counts")
	}
	// No startup/first poll sweep: exactly the admitted page advances, and the
	// public error carries no partial totals that a caller could mistake for exact.
	got, err := f.store.InspectCompilerActivations(context.Background(), f.owner)
	if !errors.Is(err, ErrCompilerStatusIndexing) || got.SelectedEvents != 0 || got.PendingRoots != 0 {
		t.Fatalf("partial status %+v %v", got, err)
	}
	if n := activationCount(t, f.db, `SELECT after_sequence FROM memory_compiler_status_events`); n != 128 {
		t.Fatalf("event visits %d", n)
	}
	if n := activationCount(t, f.db, `SELECT pending+failed FROM memory_compiler_status_roots`); n != 32 {
		t.Fatalf("root visits %d", n)
	}
	// Existing indexed roots get deltas; unvisited roots are read in their latest
	// state by later pages. Rollback must roll both source and projection back.
	if _, err := f.db.Exec(`UPDATE memory_compiler_activation_roots SET state='completed_empty' WHERE first_sequence IN (1,99)`); err != nil {
		t.Fatal(err)
	}
	tx, err := f.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE memory_compiler_activation_roots SET state='failed' WHERE first_sequence=3`); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`VACUUM`); err != nil {
		t.Fatal(err)
	}
	f.store = f.second(t)
	got = completeActivationStatus(t, f)
	if got.SelectedEvents != 513 || got.OutsideSelectionEvents != 0 || got.PendingRoots != 48 || got.SourceErrors != 50 {
		t.Fatalf("exact reopened totals %+v", got)
	}
	// A ready projection reads only new append coordinates and applies root state
	// transitions in the same transaction, with no retained-root rescans.
	if _, err := f.db.Exec(`UPDATE memory_compiler_activation_roots SET state='completed_empty' WHERE state='failed'`); err != nil {
		t.Fatal(err)
	}
	got = completeActivationStatus(t, f)
	if got.PendingRoots != 48 || got.SourceErrors != 0 {
		t.Fatalf("root deltas %+v", got)
	}
	// Stable lexical cursors also account for newly discovered roots whose
	// immutable IDs sort before the already indexed prefix.
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_activation_roots(activation_id,session_id,root_id,first_sequence,last_sequence,position,state) VALUES(?,?,'status-event-101',101,101,101,'failed')`, a.ID, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	got = completeActivationStatus(t, f)
	if got.SourceErrors != 1 || got.PendingRoots != 48 {
		t.Fatalf("insert before stable cursor %+v", got)
	}
	if n := activationCount(t, f.db, `SELECT total FROM memory_compiler_status_events`); n != 513 {
		t.Fatalf("recounted total %d", n)
	}
}

func TestCompilerStatusProjectionRollbackAuthorizationAndConcurrentContinuation(t *testing.T) {
	f := newWorkerFixture(t)
	statusFixtureEvents(t, f, 1025)
	foreign := f.owner
	foreign.ProjectID = "foreign"
	if _, err := f.store.InspectCompilerActivations(context.Background(), foreign); err == nil || errors.Is(err, ErrCompilerStatusIndexing) {
		t.Fatalf("foreign indexing: %v", err)
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_status_events`); n != 0 {
		t.Fatal("unauthorized cache write")
	}
	if _, err := f.db.Exec(`CREATE TRIGGER status_fail_write BEFORE INSERT ON memory_compiler_status_events BEGIN SELECT RAISE(ABORT,'injected projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.InspectCompilerActivations(context.Background(), f.owner); err == nil || errors.Is(err, ErrCompilerStatusIndexing) {
		t.Fatalf("infrastructure failure hidden: %v", err)
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_status_events`); n != 0 {
		t.Fatal("failed transaction persisted progress")
	}
	if _, err := f.db.Exec(`DROP TRIGGER status_fail_write`); err != nil {
		t.Fatal(err)
	}
	second := f.second(t)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for _, s := range []*Store{f.store, second} {
		wg.Add(1)
		go func(s *Store) {
			defer wg.Done()
			for range 5 {
				_, err := s.InspectCompilerActivations(context.Background(), f.owner)
				if err != nil && !errors.Is(err, ErrCompilerStatusIndexing) {
					failures <- err
					return
				}
			}
		}(s)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	got := completeActivationStatus(t, f)
	if got.SelectedEvents != 0 || got.OutsideSelectionEvents != 1025 {
		t.Fatalf("concurrent duplicate/omission %+v", got)
	}
	// Current authorization is still checked after the exact totals are cached.
	if _, err := f.store.InspectCompilerActivations(context.Background(), foreign); err == nil {
		t.Fatal("cached result bypassed authorization")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.store.InspectCompilerActivations(cancelled, f.owner); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation identity %v", err)
	}
}

func TestCompilerStatusProjectionHistoryUnionChangesAndScopeIsolation(t *testing.T) {
	f := newWorkerFixture(t)
	statusFixtureEvents(t, f, 300)
	event := func(n int) memory.Event {
		return memory.Event{ID: memory.EventID(fmt.Sprintf("status-event-%d", n)), Sequence: int64(n)}
	}
	historySelect(t, f, "old", historyRange(f, event(1), event(2)))
	inspect := func() (memory.CompilerHistoryProgress, error) {
		return f.store.InspectCompilerHistory(context.Background(), []memory.ScopeContext{f.owner}, "old", 0, 0, 64)
	}
	if got, err := inspect(); !errors.Is(err, ErrCompilerStatusIndexing) || got.SelectedSessionEvents != 0 {
		t.Fatalf("initial history %+v %v", got, err)
	}
	if n := activationCount(t, f.db, `SELECT after_sequence FROM memory_compiler_status_events`); n != 128 {
		t.Fatalf("history event budget %d", n)
	}
	f.store = f.second(t)
	if _, err := inspect(); !errors.Is(err, ErrCompilerStatusIndexing) {
		t.Fatal(err)
	}
	got, err := inspect()
	if err != nil || got.SelectedSessionEvents != 2 || got.OutsideSelectionEvents != 298 {
		t.Fatalf("history totals %+v %v", got, err)
	}
	historySelect(t, f, "overlap", historyRange(f, event(2), event(4)))
	// New history selection can cover already-indexed coordinates. The exact
	// generation/session revision invalidates just that view, never misreports
	// the old totals as current; overlap and cancelled selections count once.
	if _, err = inspect(); !errors.Is(err, ErrCompilerStatusIndexing) {
		t.Fatal(err)
	}
	if n := activationCount(t, f.db, `SELECT after_sequence FROM memory_compiler_status_events`); n != 128 {
		t.Fatalf("bounded union rebuild %d", n)
	}
	if _, err = f.store.CancelCompilerHistory(context.Background(), []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "overlap", OperationID: "cancel-overlap", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = inspect(); !errors.Is(err, ErrCompilerStatusIndexing) {
		t.Fatal(err)
	}
	got, err = inspect()
	if err != nil || got.SelectedSessionEvents != 4 || got.OutsideSelectionEvents != 296 {
		t.Fatalf("ever-selected union %+v %v", got, err)
	}
	// Selection for another destination does not reset or inflate this union.
	other := historyRange(f, event(5), event(6))
	other.Destination = "session:" + string(f.owner.SessionID)
	historySelect(t, f, "session-only", other)
	got, err = inspect()
	if err != nil || got.SelectedSessionEvents != 4 {
		t.Fatalf("destination isolation %+v %v", got, err)
	}
	// A different generation remains an independent exact union too.
	f.generation.Prompt += " New generation."
	historySelect(t, f, "next-generation", historyRange(f, event(7), event(8)))
	got, err = inspect()
	if err != nil || got.SelectedSessionEvents != 4 {
		t.Fatalf("generation isolation %+v %v", got, err)
	}
}

func TestCompilerStatusProjectionSameFrontierResumedEmptySegment(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	old := activationStart(t, f)
	req := activationRequest(f, "disable-empty", 1)
	req.ActivationID = old.ID
	if _, err := f.store.DisableCompilerActivation(ctx, f.owner, req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ActivateCompiler(ctx, f.owner, activationRequest(f, "replacement", 2), f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	req = activationRequest(f, "resume-old-empty", 3)
	req.ActivationID = old.ID
	if _, err := f.store.ResumeCompilerActivation(ctx, f.owner, req, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	root, end := historyRoot(t, f, "selected by open segment")
	got := completeActivationStatus(t, f)
	if got.SelectedEvents != 2 {
		t.Fatalf("empty higher revision hid open segment %+v", got)
	}
	historySelect(t, f, "history-one", historyRange(f, root, root))
	h := historyProgress(t, f, "history-one", 0)
	if h.SelectedSessionEvents != end.Sequence {
		t.Fatalf("generation predecessor picked empty segment %+v", h)
	}
}

func TestCompilerStatusProjectionIndexedPlansWithRetainedGrowth(t *testing.T) {
	f := newWorkerFixture(t)
	a := activationStart(t, f)
	statusFixtureEvents(t, f, 3000)
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<3000) INSERT INTO memory_compiler_activations(activation_id,selector_key,source_scope,source_session,destination,generation_id,revision,after_position,through_position) SELECT 'retained-'||x,'retained-selector-'||x,'global',?,'global',?,x,0,0 FROM n`, f.owner.SessionID, f.generationID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_activation_roots(activation_id,session_id,root_id,first_sequence,last_sequence,position) SELECT ?,session_id,id,sequence,sequence,sequence FROM events WHERE session_id=?`, a.ID, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	queries := []struct {
		query, index string
		args         []any
	}{
		{compilerStatusEventPage, "events", []any{f.owner.SessionID, 128}},
		{compilerStatusRootPage, "memory_compiler_status_root_bootstrap", []any{f.owner.SessionID, a.ID, "status-event-32"}},
		{compilerStatusLivePredecessor, "memory_compiler_status_activation_interval", []any{"global", f.owner.SessionID, "global", 100}},
		{compilerStatusGenerationPredecessor, "memory_compiler_status_activation_generation_interval", []any{f.generationID, "global", f.owner.SessionID, "global", 100}},
		{`SELECT COALESCE(MAX(sequence),0) FROM events WHERE session_id=?`, "events", []any{f.owner.SessionID}},
		{`SELECT activation_id,root_id FROM memory_compiler_activation_roots WHERE session_id=? ORDER BY activation_id DESC,root_id DESC LIMIT 1`, "memory_compiler_status_root_bootstrap", []any{f.owner.SessionID}},
		{`SELECT activation_id FROM memory_compiler_activations WHERE source_scope=? AND source_session=? ORDER BY revision DESC,activation_id LIMIT 129`, "memory_compiler_status_activation_list", []any{"global", f.owner.SessionID}},
		{`SELECT root_id FROM memory_compiler_activation_roots WHERE session_id=? ORDER BY position DESC,activation_id LIMIT 129`, "memory_compiler_status_root_list", []any{f.owner.SessionID}},
	}
	for _, test := range queries {
		rows, err := f.db.Query("EXPLAIN QUERY PLAN "+test.query, test.args...)
		if err != nil {
			t.Fatal(err)
		}
		var plan strings.Builder
		for rows.Next() {
			var id, parent, unused int
			var line string
			if err := rows.Scan(&id, &parent, &unused, &line); err != nil {
				t.Fatal(err)
			}
			plan.WriteString(line)
			plan.WriteByte('\n')
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		got := plan.String()
		if !strings.Contains(got, "SEARCH") || !strings.Contains(got, test.index) || strings.Contains(got, "SCAN ") || strings.Contains(got, "TEMP B-TREE") {
			t.Fatalf("unbounded plan for %s:\n%s", test.query, got)
		}
	}
	if _, err := f.store.InspectCompilerActivations(context.Background(), f.owner); !errors.Is(err, ErrCompilerStatusIndexing) {
		t.Fatal(err)
	}
	// Actual progress proves retained growth did not turn one page into a sweep.
	if n := activationCount(t, f.db, `SELECT total FROM memory_compiler_status_events`); n != 128 {
		t.Fatalf("retained event visits %d", n)
	}
	if n := activationCount(t, f.db, `SELECT pending FROM memory_compiler_status_roots`); n != 32 {
		t.Fatalf("retained root visits %d", n)
	}
}

func TestCompilerStatusProjectionHistoryIntervalPredecessorPreservesGap(t *testing.T) {
	f := newWorkerFixture(t)
	root, end := historyRoot(t, f, "one long retained root")
	historySelect(t, f, "range", historyRange(f, root, end))
	// Build retained, disjoint ownership coordinates for the same root. The
	// second coordinate is deliberately unowned; later retained owners must not
	// hide it or multiply the result rows for the first event.
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(3) UNION ALL SELECT x+1 FROM n WHERE x<3000) INSERT INTO events(id,session_id,sequence,event_type,parent_id,role,content,recorded_at,format_version) SELECT 'status-late-'||x,?,x,'assistant_message',?,'assistant','retained context',strftime('%Y-%m-%dT%H:%M:%fZ','now'),1 FROM n`, f.owner.SessionID, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_history_roots(request_id,range_ordinal,root_id,first_sequence,last_sequence,state) VALUES('range',0,?,1,2,'selected_unmaterialized')`, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_history_events SELECT 'range',0,id,sequence,? FROM events WHERE session_id=? AND sequence<=2`, root.ID, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<3000) INSERT INTO memory_compiler_selections(selection_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,state,window) SELECT 'status-unit-'||x,?,'global',?,?,x,x,'failed','{}' FROM n WHERE x<>2`, f.generationID, f.owner.SessionID, root.ID); err != nil {
		t.Fatal(err)
	}
	query := `SELECT selection_id FROM memory_compiler_selections WHERE generation_id=? AND destination=? AND session_id=? AND root_id=? AND first_sequence<=? ORDER BY first_sequence DESC LIMIT 1`
	rows, err := f.db.Query("EXPLAIN QUERY PLAN "+query, f.generationID, "global", f.owner.SessionID, root.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	var plan string
	for rows.Next() {
		var id, parent, unused int
		var line string
		if err = rows.Scan(&id, &parent, &unused, &line); err != nil {
			t.Fatal(err)
		}
		plan += line
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "SEARCH") || !strings.Contains(plan, "memory_compiler_status_selection_interval") || strings.Contains(plan, "SCAN ") || strings.Contains(plan, "TEMP B-TREE") {
		t.Fatal(plan)
	}
	for range 24 {
		got, err := f.store.InspectCompilerHistory(context.Background(), []memory.ScopeContext{f.owner}, "range", 0, 0, 64)
		if errors.Is(err, ErrCompilerStatusIndexing) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Intervals) != 2 || got.Intervals[0].State != "failed" || got.Intervals[0].FirstSequence != 1 || got.Intervals[0].LastSequence != 1 || got.Intervals[1].State != "selected_unmaterialized" || got.Intervals[1].FirstSequence != 2 || got.Intervals[1].LastSequence != 2 || got.ContiguousFrontier != 0 || got.SelectedSessionEvents != 2 || got.OutsideSelectionEvents != 2998 {
			t.Fatalf("predecessor lost exact gap %+v", got)
		}
		return
	}
	t.Fatal("bounded history cursor did not finish")
}
