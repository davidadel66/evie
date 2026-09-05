package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func historyRange(f *workerFixture, first, last memory.Event) memory.CompilerHistoryRange {
	return memory.CompilerHistoryRange{SourceScope: "global", Destination: "global", SessionID: f.owner.SessionID, FirstSequence: first.Sequence, LastSequence: last.Sequence, FirstEventID: first.ID, LastEventID: last.ID}
}
func historyRoot(t *testing.T, f *workerFixture, text string) (memory.Event, memory.Event) {
	t.Helper()
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
	end := activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	return root, end
}
func historySelect(t *testing.T, f *workerFixture, id string, ranges ...memory.CompilerHistoryRange) memory.CompilerHistoryReceipt {
	t.Helper()
	r, err := f.store.SelectCompilerHistory(context.Background(), []memory.ScopeContext{f.owner}, memory.CompilerHistoryRequest{RequestID: id, Ranges: ranges}, f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func historyReconcile(t *testing.T, f *workerFixture, n int) {
	t.Helper()
	for range n {
		if _, err := f.store.ReconcileCompilerHistory(context.Background(), f.config(&activationScript{})); err != nil {
			t.Fatal(err)
		}
	}
}
func historyProgress(t *testing.T, f *workerFixture, id string, index int) memory.CompilerHistoryProgress {
	t.Helper()
	p, err := f.store.InspectCompilerHistory(context.Background(), []memory.ScopeContext{f.owner}, id, index, 0, 64)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCompilerHistoryFrozenSelectionOverlapFailureGapAndReopen(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	a, ae := historyRoot(t, f, "I prefer tea.")
	b, be := historyRoot(t, f, "I prefer coffee.")
	receipt := historySelect(t, f, "first", historyRange(f, a, be))
	// Retained legacy rows keep their coordinates without inventing positions.
	if _, err := f.db.Exec(`DELETE FROM memory_compiler_event_positions`); err != nil {
		t.Fatal(err)
	}
	extra, _ := historyRoot(t, f, "Outside captured history.")
	again := historySelect(t, f, "first", historyRange(f, a, be))
	if string(compilerJSON(receipt)) != string(compilerJSON(again)) {
		t.Fatal("receipt changed")
	}
	historySelect(t, f, "overlap", historyRange(f, b, be))
	historyReconcile(t, f, 14)
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs`); n != 2 {
		t.Fatalf("overlap jobs %d", n)
	}
	var firstJob string
	if err := f.db.QueryRow(`SELECT job_id FROM memory_compiler_jobs WHERE first_sequence=?`, a.Sequence).Scan(&firstJob); err != nil {
		t.Fatal(err)
	}
	bad := &workerScript{run: func(_ context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
		return CompilerExtraction{ReleaseEvidence: "completed"}, ErrCompilerTerminalOutput
	}}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(bad)); !worked || err == nil {
		t.Fatal(worked, err)
	}
	f.store = f.second(t)
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); !worked || err != nil {
		t.Fatal(worked, err)
	}
	p := historyProgress(t, f, "first", 0)
	if p.ContiguousFrontier != a.Sequence-1 || len(p.Intervals) != 2 || p.Intervals[0].State != "failed" || p.Intervals[1].State != "completed_empty" || p.OutsideSelectionEvents != 2 || p.SelectedSessionEvents != 4 {
		t.Fatalf("progress %+v", p)
	}
	if p.Intervals[0].LastSequence != ae.Sequence || p.Intervals[1].LastSequence != be.Sequence || p.Intervals[1].LastSequence >= extra.Sequence {
		t.Fatal("exact ranges lost")
	}
	historySelect(t, f, "third", historyRange(f, a, be))
	historyReconcile(t, f, 12)
	var attempts int
	if err := f.db.QueryRow(`SELECT attempts FROM memory_compiler_jobs WHERE job_id=?`, firstJob).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal(attempts, err)
	}
	if _, err := f.store.ResumeCompilation(ctx, f.owner, firstJob); err == nil {
		t.Fatal("failed same generation restarted")
	}
}

func TestCompilerHistoryBoundsAndAtomicAuthorizations(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	a, end := historyRoot(t, f, "An assertion.")
	r := historyRange(f, a, end)
	cases := []memory.CompilerHistoryRequest{{RequestID: "empty"}, {RequestID: "wrong-id", Ranges: []memory.CompilerHistoryRange{r}}, {RequestID: "wide", Ranges: []memory.CompilerHistoryRange{r}}}
	cases[1].Ranges[0].LastEventID = "wrong"
	cases[2].Ranges[0].Destination = "workspace:other"
	for _, req := range cases {
		if _, err := f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.owner}, req, f.generation, &activationScript{}); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	req := memory.CompilerHistoryRequest{RequestID: "101", Ranges: make([]memory.CompilerHistoryRange, 101)}
	for i := range req.Ranges {
		req.Ranges[i] = r
	}
	if _, err := f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.owner}, req, f.generation, &activationScript{}); err == nil {
		t.Fatal("101 ranges")
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_history_requests`); n != 0 {
		t.Fatal("partial request")
	}
	// Populate source coordinate fixtures to exercise the exact 10,000-event
	// admission boundary without dispatching or inspecting their source contents.
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(3) UNION ALL SELECT x+1 FROM n WHERE x<10001) INSERT INTO events(id,session_id,sequence,event_type,role,content,recorded_at,format_version) SELECT 'history-bound-'||x,?,x,'user_message','user','assertion',strftime('%Y-%m-%dT%H:%M:%fZ','now'),1 FROM n`, f.owner.SessionID); err != nil {
		t.Fatal(err)
	}
	r.LastSequence = 10000
	r.LastEventID = "history-bound-10000"
	historySelect(t, f, "10000", r)
	r.LastSequence = 10001
	r.LastEventID = "history-bound-10001"
	if _, err := f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryRequest{RequestID: "10001", Ranges: []memory.CompilerHistoryRange{r}}, f.generation, &activationScript{}); err == nil {
		t.Fatal("10001 events")
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_history_requests`); n != 1 {
		t.Fatal("partial oversized receipt")
	}
}

func TestCompilerHistoryCancellationUnionRollbackAndABAFence(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "I prefer tea.")
	historySelect(t, f, "a", historyRange(f, root, end))
	historySelect(t, f, "b", historyRange(f, root, end))
	historyReconcile(t, f, 8)
	var job string
	if err := f.db.QueryRow(`SELECT job_id FROM memory_compiler_jobs`).Scan(&job); err != nil {
		t.Fatal(err)
	}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, job, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	change := memory.CompilerHistoryChange{RequestID: "a", OperationID: "cancel-a", ExpectedRevision: 1}
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, change); err != nil {
		t.Fatal(err)
	}
	if err := f.store.renewCompilerClaim(ctx, claim); err != nil {
		t.Fatal("overlap cancellation fenced shared work", err)
	}
	// Failure at the durable receipt commit rolls back request, fence, resources
	// and capacity together. No partial cancellation becomes observable.
	if _, err := f.db.Exec(`CREATE TRIGGER fail_history_cancel BEFORE INSERT ON memory_compiler_history_changes WHEN NEW.operation_id='cancel-b' BEGIN SELECT RAISE(ABORT,'fixture rollback'); END`); err != nil {
		t.Fatal(err)
	}
	change = memory.CompilerHistoryChange{RequestID: "b", OperationID: "cancel-b", ExpectedRevision: 1}
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, change); err == nil {
		t.Fatal("rollback trigger ignored")
	}
	if err := f.store.renewCompilerClaim(ctx, claim); err != nil {
		t.Fatal("rolled back cancellation changed fence", err)
	}
	if _, err := f.db.Exec(`DROP TRIGGER fail_history_cancel`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, change); err != nil {
		t.Fatal(err)
	}
	if err := f.store.renewCompilerClaim(ctx, claim); !errors.Is(err, ErrCompilerFence) {
		t.Fatal("cancel did not fence", err)
	}
	resume := memory.CompilerHistoryChange{RequestID: "b", OperationID: "resume-b", ExpectedRevision: 2}
	if _, err := f.store.ResumeCompilerHistory(ctx, []memory.ScopeContext{f.owner}, resume, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	historyReconcile(t, f, 2)
	if err := f.store.stageCompilerResult(ctx, f.owner, claim.JobID, string(claim.Holder), claim.Fence, claim.Request, []memory.MemoryCandidate{}); !errors.Is(err, ErrCompilerFence) {
		t.Fatal("stale result after resume", err)
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_capacity WHERE state='release_pending'`); n != 1 {
		t.Fatal("unknown release was freed")
	}
	if n := activationCount(t, f.db, `SELECT attempts FROM memory_compiler_jobs`); n != 1 {
		t.Fatal("resume reset attempts")
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_coverage`); n != 0 {
		t.Fatal("cancelled result covered")
	}
}

func TestCompilerHistoryClosureAndQueueFullObligation(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Captured assertion."})
	historySelect(t, f, "bounded", historyRange(f, root, root))
	historyReconcile(t, f, 2)
	if p := historyProgress(t, f, "bounded", 0); p.Intervals[0].State != "deferred_live" {
		t.Fatalf("unfinished %+v", p)
	}
	// Newer unselected root closes this prefix while the session still has a
	// live lease. It supplies neither support nor coverage to the old request.
	outside := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Unselected later root."})
	historyReconcile(t, f, 2)
	var requestRaw []byte
	if err := f.db.QueryRow(`SELECT request FROM memory_compiler_jobs`).Scan(&requestRaw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(requestRaw), string(outside.ID)) || strings.Contains(string(requestRaw), "Unselected later") {
		t.Fatal("outside selection entered sealed request")
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); !worked || err != nil {
		t.Fatal(worked, err)
	}
	p := historyProgress(t, f, "bounded", 0)
	if p.ContiguousFrontier != root.Sequence || p.OutsideSelectionEvents != 1 {
		t.Fatalf("cutoff %+v", p)
	}
	// The independent full-queue selection stays durable until capacity returns.
	q, qe := historyRoot(t, f, "Queue-full assertion.")
	historySelect(t, f, "full", historyRange(f, q, qe))
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1024) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'full-history-'||x,?,'global',?,?,20000+x,20000+x,'fixture','{}','queued' FROM n`, f.generationID, f.owner.SessionID, q.ID); err != nil {
		t.Fatal(err)
	}
	historyReconcile(t, f, 5)
	if p := historyProgress(t, f, "full", 0); p.Intervals[0].State != "selected_unmaterialized" {
		t.Fatalf("lost queue full %+v", p)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET state='failed' WHERE job_id='full-history-1'`); err != nil {
		t.Fatal(err)
	}
	f.store = f.second(t)
	historyReconcile(t, f, 2)
	if p := historyProgress(t, f, "full", 0); p.Intervals[0].State != "queued" {
		t.Fatalf("not rediscovered %+v", p)
	}
}

func TestCompilerHistoryConcurrentOverlapSingleOwnership(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "Asserted once.")
	other := f.second(t)
	start := make(chan struct{})
	errs := make(chan error, 2)
	for i, store := range []*Store{f.store, other} {
		go func(i int, store *Store) {
			<-start
			_, err := store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryRequest{RequestID: fmt.Sprintf("race-%d", i), Ranges: []memory.CompilerHistoryRange{historyRange(f, root, end)}}, f.generation, &activationScript{})
			errs <- err
		}(i, store)
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, store := range []*Store{f.store, other} {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			for range 8 {
				if _, err := store.ReconcileCompilerHistory(ctx, f.config(&activationScript{})); err != nil {
					errs <- err
					return
				}
			}
		}(store)
	}
	wg.Wait()
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs`); n != 1 {
		t.Fatal("duplicate interval ownership", n)
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_history_requests`); n != 2 {
		t.Fatal("original request lost", n)
	}
}

func TestCompilerHistoryNewEvidenceFairnessUsesRealSelections(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	old1, _ := historyRoot(t, f, "First selected history.")
	old2, end := historyRoot(t, f, "Second selected history.")
	historySelect(t, f, "catch-up", historyRange(f, old1, end))
	historyReconcile(t, f, 10)
	activationStart(t, f)
	for range 16 {
		historyRoot(t, f, "New asserted evidence.")
	}
	activationReconcile(t, f, 75)
	order := []memory.EventID{}
	extractor := &workerScript{run: func(_ context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
		order = append(order, r.Window.Selection.RootID)
		return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}}), ReleaseEvidence: "completed"}, nil
	}}
	for n := 1; n <= 18; n++ {
		if worked, err := f.second(t).RunCompilerStep(ctx, f.config(extractor)); !worked || err != nil {
			t.Fatalf("dispatch %d: %v %v", n, worked, err)
		}
		historical := order[n-1] == old1.ID || order[n-1] == old2.ID
		if historical != (n == 9 || n == 18) {
			t.Fatalf("history fairness dispatch %d root %s", n, order[n-1])
		}
	}
	if order[8] != old1.ID || order[17] != old2.ID {
		t.Fatal("oldest historical order changed")
	}
}

func TestCompilerHistoryReselectionDoesNotResumeCancellation(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "An assertion.")
	historySelect(t, f, "original", historyRange(f, root, end))
	historyReconcile(t, f, 4)
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "original", OperationID: "cancel", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	historySelect(t, f, "repeat", historyRange(f, root, end))
	historyReconcile(t, f, 5)
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE state='cancelled' AND attempts=0`); n != 1 {
		t.Fatal("reselection implicitly resumed", n)
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); worked || err != nil {
		t.Fatal("cancelled job dispatched", worked, err)
	}
}

func TestCompilerHistoryExclusionAndOversizeRemainDistinct(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	secret, secretEnd := historyRoot(t, f, "password=not-a-real-password-fixture")
	large, largeEnd := historyRoot(t, f, strings.Repeat("x", 32769))
	historySelect(t, f, "outcomes", historyRange(f, secret, largeEnd))
	historyReconcile(t, f, 10)
	p := historyProgress(t, f, "outcomes", 0)
	if len(p.Intervals) != 2 || p.Intervals[0].State != "excluded" || p.Intervals[0].LastSequence != secretEnd.Sequence || p.Intervals[1].State != "failed" || p.Intervals[1].Reason != "oversized_input" || p.Intervals[1].FirstSequence != large.Sequence || p.ContiguousFrontier != secretEnd.Sequence {
		t.Fatalf("outcomes %+v", p)
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); worked || err != nil {
		t.Fatal("excluded/oversized input dispatched", worked, err)
	}
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "outcomes", OperationID: "cancel-outcomes", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	p = historyProgress(t, f, "outcomes", 0)
	if p.Intervals[0].State != "excluded" || p.Intervals[1].State != "failed" {
		t.Fatal("cancellation conflated prior outcomes")
	}
}

func TestCompilerHistoryRunningCancellationReturnsPromptlyAndReleasesOnlyProvenCapacity(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(fmt.Sprint(known), func(t *testing.T) {
			f := newWorkerFixture(t)
			ctx := context.Background()
			root, end := historyRoot(t, f, "An assertion.")
			historySelect(t, f, "running", historyRange(f, root, end))
			historyReconcile(t, f, 4)
			entered := make(chan struct{})
			done := make(chan error, 1)
			extractor := &workerScript{run: func(ctx context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
				close(entered)
				<-ctx.Done()
				release := ""
				if known {
					release = "completed"
				}
				return CompilerExtraction{ReleaseEvidence: release}, ctx.Err()
			}}
			go func() { _, err := f.store.RunCompilerStep(ctx, f.config(extractor)); done <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("worker did not dispatch")
			}
			if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "running", OperationID: "cancel", ExpectedRevision: 1}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("cancelled client remained blocked")
			}
			want := 1
			if known {
				want = 0
			}
			if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_capacity`); n != want {
				t.Fatal("capacity release evidence mismatch", n, want)
			}
			if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_coverage`); n != 0 {
				t.Fatal("late cancelled result published")
			}
		})
	}
}

func TestCompilerHistoryCancelledFifthStageResumesWithoutSixthAttempt(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "An assertion.")
	historySelect(t, f, "stage", historyRange(f, root, end))
	historyReconcile(t, f, 4)
	var job string
	if err := f.db.QueryRow(`SELECT job_id FROM memory_compiler_jobs`).Scan(&job); err != nil {
		t.Fatal(err)
	}
	script := &workerScript{}
	for range 4 {
		claim, err := f.store.claimCompilerJob(ctx, f.owner, job, script)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.failCompilerAttempt(ctx, job, claim.Holder, claim.Fence, true, "invalid_or_missing_output", false); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now') WHERE job_id=?`, job); err != nil {
			t.Fatal(err)
		}
	}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, job, script)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.stageCompilerResult(ctx, f.owner, job, claim.Holder, claim.Fence, claim.Request, []memory.MemoryCandidate{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "stage", OperationID: "cancel-stage", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ResumeCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "stage", OperationID: "resume-stage", ExpectedRevision: 2}, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	f.store = f.second(t)
	historyReconcile(t, f, 2)
	if worked, err := f.store.RunCompilerStep(ctx, f.config(script)); !worked || err != nil {
		t.Fatal(worked, err)
	}
	got, err := f.store.InspectCompilation(ctx, f.owner, job)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "completed_empty" || got.Attempts != 5 || script.calls.Load() != 0 {
		t.Fatalf("saved stage %+v calls %d", got, script.calls.Load())
	}
}

func TestCompilerHistoryOverlapRetainsAnotherRequestsQueueFullPrefix(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "Earlier assertion.")
	historySelect(t, f, "original", historyRange(f, root, end))
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1024) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'overlap-full-'||x,?,'global',?,?,20000+x,20000+x,'fixture','{}','queued' FROM n`, f.generationID, f.owner.SessionID, root.ID); err != nil {
		t.Fatal(err)
	}
	historyReconcile(t, f, 3)
	late := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "Later assertion."})
	last := activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET state='failed' WHERE job_id='overlap-full-1'`); err != nil {
		t.Fatal(err)
	}
	_, manifest, err := memory.CompilerGenerationIdentity(f.generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		_, _, err := selectCompilerUnitInTransaction(ctx, conn, f.owner, memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, f.generationID, manifest, f.generation, compilerScheduling{Lane: "historical", FirstSequence: late.Sequence})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	historySelect(t, f, "overlap", historyRange(f, root, last))
	historyReconcile(t, f, 10)
	if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, memory.CompilerHistoryChange{RequestID: "original", OperationID: "cancel-prefix-owner", ExpectedRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET state='failed' WHERE job_id='overlap-full-2'`); err != nil {
		t.Fatal(err)
	}
	historyReconcile(t, f, 4)
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE first_sequence=1 AND last_sequence=2`); n != 1 {
		t.Fatal("overlap lost the earlier queue-full prefix", n)
	}
	if n := activationCount(t, f.db, `SELECT pending_roots FROM memory_compiler_history_requests WHERE request_id='overlap'`); n != 0 {
		t.Fatal("completed union ownership stayed pending", n)
	}
}

func TestCompilerHistorySelectionRefsDeduplicateRangesDestinationsAndDeliveries(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	root, end := historyRoot(t, f, "An assertion.")
	base := historyRange(f, root, end)
	session := base
	session.Destination = "session:" + string(f.owner.SessionID)
	ranges := make([]memory.CompilerHistoryRange, 100)
	for i := range ranges {
		ranges[i] = base
	}
	ranges[99] = session
	receipt := historySelect(t, f, "references", ranges...)
	historySelect(t, f, "references", ranges...)
	if receipt.SelectedEvents != 2 {
		t.Fatal("range duplicates changed source bound", receipt.SelectedEvents)
	}
	if n := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_history_selection_refs WHERE active_requests=1`); n != 4 {
		t.Fatal("per-event/destination reference count", n)
	}
	cancel := memory.CompilerHistoryChange{RequestID: "references", OperationID: "cancel-refs", ExpectedRevision: 1}
	for range 2 {
		if _, err := f.store.CancelCompilerHistory(ctx, []memory.ScopeContext{f.owner}, cancel); err != nil {
			t.Fatal(err)
		}
	}
	if n := activationCount(t, f.db, `SELECT SUM(active_requests) FROM memory_compiler_history_selection_refs`); n != 0 {
		t.Fatal("cancel did not adjust exact refs", n)
	}
	resume := memory.CompilerHistoryChange{RequestID: "references", OperationID: "resume-refs", ExpectedRevision: 2}
	for range 2 {
		if _, err := f.store.ResumeCompilerHistory(ctx, []memory.ScopeContext{f.owner}, resume, f.generation, &activationScript{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := activationCount(t, f.db, `SELECT SUM(active_requests) FROM memory_compiler_history_selection_refs`); n != 4 {
		t.Fatal("duplicate resume adjusted twice", n)
	}
}
