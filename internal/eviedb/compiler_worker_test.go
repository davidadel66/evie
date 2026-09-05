package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type workerFixture struct {
	db           *sql.DB
	store        *Store
	path         string
	owner        memory.ScopeContext
	lease        memory.TurnLease
	generation   memory.CompilerGeneration
	generationID string
}

func newWorkerFixture(t *testing.T) *workerFixture {
	t.Helper()
	f := &workerFixture{path: filepath.Join(t.TempDir(), "worker.db"), generation: compilerCommitGeneration()}
	var err error
	f.db, err = OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.store = NewStore(f.db)
	session, err := f.store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.owner = session.ScopeContext()
	f.lease, err = f.store.AcquireTurnLease(context.Background(), session.ID, "worker-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	f.generationID, _, err = memory.CompilerGenerationIdentity(f.generation)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *workerFixture) queue(t *testing.T, text string) memory.Compilation {
	t.Helper()
	ctx := context.Background()
	root, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
	if err != nil {
		t.Fatal(err)
	}
	end, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	if err != nil {
		t.Fatal(err)
	}
	job, err := f.store.QueueCandidateUnit(ctx, f.owner, memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}, f.generation, &workerScript{})
	if err != nil {
		t.Fatal(err)
	}
	if job.State != "queued" {
		t.Fatalf("queue state %s", job.State)
	}
	return job
}
func (f *workerFixture) second(t *testing.T) *Store {
	t.Helper()
	db, err := OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}
func (f *workerFixture) config(extractor CompilerExtractor) CompilerSupervisorConfig {
	return CompilerSupervisorConfig{Extractors: map[string]CompilerExtractor{f.generationID: extractor}}
}

type workerScript struct {
	calls atomic.Int32
	run   func(context.Context, memory.CompilerRequest) (CompilerExtraction, error)
}

func (*workerScript) ServerIdentity() string { return "scripted:137" }
func (w *workerScript) Extract(ctx context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (CompilerExtraction, error) {
	w.calls.Add(1)
	if w.run != nil {
		return w.run(ctx, r)
	}
	return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}}), ReleaseEvidence: "completed"}, nil
}

func TestCompilerWorkerRetryBudgetSurvivesStores(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "I prefer tea.")
	seen := map[string]bool{}
	var sealed string
	extractor := &workerScript{run: func(_ context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
		if r.AttemptID == "" || seen[r.AttemptID] {
			t.Error("attempt did not get unique durable identity")
		}
		seen[r.AttemptID] = true
		if sealed == "" {
			sealed = string(compilerJSON(r))
		} else if sealed != string(compilerJSON(r)) {
			t.Error("retry changed sealed request")
		}
		return CompilerExtraction{Raw: []byte(`{"request_id":`), ReleaseEvidence: "completed"}, nil
	}}
	ctx := context.Background()
	for n := 1; n <= 5; n++ {
		store := f.second(t)
		worked, err := store.RunCompilerStep(ctx, f.config(extractor))
		if !worked || err == nil {
			t.Fatalf("attempt %d worked=%v err=%v", n, worked, err)
		}
		got, err := store.InspectCompilation(ctx, f.owner, job.JobID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Attempts != n {
			t.Fatalf("attempts %d want %d", got.Attempts, n)
		}
		if n < 5 {
			var delay int
			if err := f.db.QueryRow(`SELECT retry_at-unixepoch('now') FROM memory_compiler_jobs WHERE job_id=?`, job.JobID).Scan(&delay); err != nil {
				t.Fatal(err)
			}
			want := 5 << (n - 1)
			if delay < want-1 || delay > want || got.State != "retry_wait" {
				t.Fatalf("attempt %d state %s delay %d", n, got.State, delay)
			}
			worked, err = store.RunCompilerStep(ctx, f.config(extractor))
			if worked || err != nil {
				t.Fatalf("not-due attempt dispatched: %v %v", worked, err)
			}
			if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now') WHERE job_id=?`, job.JobID); err != nil {
				t.Fatal(err)
			}
		} else if got.State != "failed" || got.Reason != "attempts_exhausted" {
			t.Fatalf("exhaustion %+v", got)
		}
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(extractor)); worked || err != nil {
		t.Fatalf("sixth attempt %v %v", worked, err)
	}
	repeated, err := f.store.QueueCandidateUnit(ctx, f.owner, job.Window.Selection, f.generation, &workerScript{})
	if err != nil || repeated.JobID != job.JobID || repeated.Attempts != 5 {
		t.Fatalf("reselection reset budget %+v %v", repeated, err)
	}
	if _, err := f.store.ResumeCompilation(ctx, f.owner, job.JobID); err == nil {
		t.Fatal("resume bypassed exhaustion")
	}
	if extractor.calls.Load() != 5 {
		t.Fatalf("calls %d", extractor.calls.Load())
	}
}

func TestCompilerWorkerCancelledFifthStageExplicitResume(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "I prefer tea.")
	ctx := context.Background()
	e := &workerScript{}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET attempts=4 WHERE job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	c, err := f.store.claimCompilerJob(ctx, f.owner, job.JobID, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.stageCompilerResult(ctx, f.owner, c.JobID, c.Holder, c.Fence, c.Request, []memory.MemoryCandidate{}); err != nil {
		t.Fatal(err)
	}
	second := f.second(t)
	cancelled, err := second.CancelCompilation(ctx, f.owner, job.JobID)
	if err != nil || cancelled.State != "cancelled" {
		t.Fatalf("cancel %+v %v", cancelled, err)
	}
	if err := f.store.publishCompilerResult(ctx, f.owner, c.JobID, c.Holder, c.Fence, c.Request); !errors.Is(err, ErrCompilerFence) {
		t.Fatalf("stale publish %v", err)
	}
	if worked, err := second.RunCompilerStep(ctx, f.config(e)); worked || err != nil {
		t.Fatalf("automatic cancelled adoption %v %v", worked, err)
	}
	resumed, err := second.ResumeCompilation(ctx, f.owner, job.JobID)
	if err != nil || resumed.State != "staged" || resumed.Attempts != 5 {
		t.Fatalf("resume %+v %v", resumed, err)
	}
	if worked, err := second.RunCompilerStep(ctx, f.config(e)); !worked || err != nil {
		t.Fatalf("adoption %v %v", worked, err)
	}
	got, err := second.InspectCompilation(ctx, f.owner, job.JobID)
	if err != nil || got.State != "completed_empty" || got.Attempts != 5 || e.calls.Load() != 0 {
		t.Fatalf("adopted stage %+v calls %d err %v", got, e.calls.Load(), err)
	}
	var groups, coverage, resources int
	f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_candidate_groups`).Scan(&groups)
	f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_coverage`).Scan(&coverage)
	f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_resources`).Scan(&resources)
	if groups != 1 || coverage != 1 || resources != 0 {
		t.Fatalf("durable counts %d %d %d", groups, coverage, resources)
	}
}

func TestCompilerWorkerInvalidFirstUnitDoesNotStarveLater(t *testing.T) {
	for _, kind := range []string{"source", "stage"} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkerFixture(t)
			first := f.queue(t, "I prefer tea.")
			later := f.queue(t, "I prefer coffee.")
			e := &workerScript{}
			ctx := context.Background()
			if kind == "source" {
				// Corrupt only the first source projection, not the later window's overlap.
				if _, err := f.db.Exec(`DROP TRIGGER memory_compiler_request_snapshot_immutable`); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET request='{}' WHERE job_id=?`, first.JobID); err != nil {
					t.Fatal(err)
				}
			} else {
				c, err := f.store.claimCompilerJob(ctx, f.owner, first.JobID, e)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.store.stageCompilerResult(ctx, f.owner, c.JobID, c.Holder, c.Fence, c.Request, []memory.MemoryCandidate{}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`DROP TRIGGER memory_compiler_stage_immutable`); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`UPDATE memory_compiler_stages SET envelope_hash='corrupt' WHERE job_id=?`, first.JobID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, first.JobID); err != nil {
					t.Fatal(err)
				}
			}
			worked, err := f.store.RunCompilerStep(ctx, f.config(e))
			if !worked || err == nil {
				t.Fatalf("invalid first unit %v %v", worked, err)
			}
			worked, err = f.store.RunCompilerStep(ctx, f.config(e))
			if !worked || err != nil {
				t.Fatalf("later unit stalled %v %v", worked, err)
			}
			got, err := f.store.InspectCompilation(ctx, f.owner, later.JobID)
			if err != nil || got.State != "completed_empty" {
				t.Fatalf("later %+v %v", got, err)
			}
			status, err := f.store.InspectCompilerStatus(ctx, f.owner, "", 64)
			if err != nil {
				t.Fatal(err)
			}
			for _, j := range status.Jobs {
				if j.JobID == first.JobID && (j.State != "failed" || (kind == "source" && j.Attempts != 0)) {
					t.Fatalf("bad first status %+v", j)
				}
			}
			raw, _ := json.Marshal(status)
			if string(raw) == "" {
				t.Fatal("missing safe status")
			}
			if e.calls.Load() != 1 {
				t.Fatalf("calls %d", e.calls.Load())
			}
		})
	}
}

func TestCompilerWorkerSupervisorShutdownIsRecoverable(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "I prefer tea.")
	entered := make(chan struct{})
	e := &workerScript{run: func(ctx context.Context, _ memory.CompilerRequest) (CompilerExtraction, error) {
		close(entered)
		<-ctx.Done()
		return CompilerExtraction{}, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- f.store.RunCompilerSupervisor(ctx, f.config(e)) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("no dispatch")
	}
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("local cancellation exceeded one second")
	}
	if time.Since(start) >= time.Second {
		t.Fatal("slow local shutdown")
	}
	got, err := f.second(t).InspectCompilation(context.Background(), f.owner, job.JobID)
	if err != nil || got.State != "retry_wait" || got.Reason != "worker_shutdown" || got.CapacityState != "release_pending" {
		t.Fatalf("shutdown state %+v %v", got, err)
	}
}

func TestCompilerWorkerResourceWaitConsumesNoAttempt(t *testing.T) {
	for _, kind := range []string{"stages", "inbox"} {
		t.Run(kind, func(t *testing.T) {
			f := newWorkerFixture(t)
			job := f.queue(t, "I prefer tea.")
			e := &workerScript{}
			// Deliberately seed capacity through SQLite to test the exact boundary without
			// thousands of model invocations. Reserved records own real sealed jobs.
			if _, err := f.db.Exec(`INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'capacity-audit',generation_id,destination,session_id,root_id,first_sequence,last_sequence+10000,window_hash,request,'failed' FROM memory_compiler_jobs WHERE job_id=?`, job.JobID); err != nil {
				t.Fatal(err)
			}
			if kind == "stages" {
				if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<128) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'reserved-'||x,j.generation_id,j.destination,j.session_id,j.root_id,j.first_sequence,j.last_sequence+x,j.window_hash,j.request,'staged' FROM n CROSS JOIN memory_compiler_jobs j WHERE j.job_id=?`, job.JobID); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`INSERT INTO memory_compiler_resources SELECT job_id,0,131072,16 FROM memory_compiler_jobs WHERE job_id LIKE 'reserved-%'`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.db.Exec(`INSERT INTO memory_compiler_candidate_groups VALUES('capacity-audit','fixture')`); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<2033) INSERT INTO memory_compiler_candidates(candidate_id,job_id,ordinal,envelope,equivalence_hash) SELECT 'inbox-'||x,'capacity-audit',x,'{}','fixture-'||x FROM n`); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.store.claimCompilerJob(context.Background(), f.owner, job.JobID, e); !errors.Is(err, ErrCompilerCapacityBlocked) {
				t.Fatalf("full %s admitted: %v", kind, err)
			}
			got, err := f.store.InspectCompilation(context.Background(), f.owner, job.JobID)
			if err != nil || got.Attempts != 0 || got.State != "queued" {
				t.Fatalf("resource wait spent attempt %+v %v", got, err)
			}
			var capacity int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_capacity`).Scan(&capacity); err != nil || capacity != 0 {
				t.Fatalf("orphan reservation %d %v", capacity, err)
			}
		})
	}
}

func TestCompilerWorkerDurableFairnessCounter(t *testing.T) {
	f := newWorkerFixture(t)
	lanes := map[memory.EventID]string{}
	for i := 0; i < 18; i++ {
		job := f.queue(t, "I prefer tea.")
		lane := "new"
		if i >= 16 {
			lane = "historical"
		}
		lanes[job.Window.Selection.RootID] = lane
		if _, err := f.db.Exec(`UPDATE memory_compiler_job_schedule SET lane=?,position=?,historical_order=? WHERE job_id=?`, lane, i, i, job.JobID); err != nil {
			t.Fatal(err)
		}
	}
	dispatched := []string{}
	e := &workerScript{run: func(_ context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
		dispatched = append(dispatched, lanes[r.Window.Selection.RootID])
		return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}}), ReleaseEvidence: "completed"}, nil
	}}
	for n := 1; n <= 18; n++ {
		if worked, err := f.second(t).RunCompilerStep(context.Background(), f.config(e)); !worked || err != nil {
			t.Fatalf("dispatch %d %v %v", n, worked, err)
		}
		want := "new"
		if n == 9 || n == 18 {
			want = "historical"
		}
		if dispatched[n-1] != want {
			t.Fatalf("dispatch %d lane %s want %s", n, dispatched[n-1], want)
		}
	}
}

func TestCompilerWorkerOldAcknowledgementCannotReleaseReplacement(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "I prefer tea.")
	ctx := context.Background()
	e := &workerScript{}
	other := f.second(t)
	first, err := f.store.claimCompilerJob(ctx, f.owner, job.JobID, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.failCompilerAttempt(ctx, first.JobID, first.Holder, first.Fence, false, "invalid_or_missing_output", false); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	stale := compilerCrashReleaseVerifier(func(r CompilerCapacityReservation) CompilerReleaseAcknowledgement {
		close(entered)
		<-release
		return CompilerReleaseAcknowledgement{Reservation: r, Kind: "request_completed"}
	})
	go func() { done <- f.store.ReconcileCompilerCapacity(ctx, stale) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("verifier not called")
	}
	verifier := compilerCrashReleaseVerifier(func(r CompilerCapacityReservation) CompilerReleaseAcknowledgement {
		return CompilerReleaseAcknowledgement{Reservation: r, Kind: "request_completed"}
	})
	if err := other.ReconcileCompilerCapacity(ctx, verifier); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now') WHERE job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	replacement, err := other.claimCompilerJob(ctx, f.owner, job.JobID, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.failCompilerAttempt(ctx, replacement.JobID, replacement.Holder, replacement.Fence, false, "invalid_or_missing_output", false); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-done:
		if !errors.Is(err, ErrCompilerFence) {
			t.Fatalf("stale acknowledgement %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("verifier did not finish")
	}
	var requestID string
	var fence int64
	if err := f.db.QueryRow(`SELECT request_id,fence FROM memory_compiler_capacity`).Scan(&requestID, &fence); err != nil || requestID != replacement.AttemptID || fence != replacement.Fence {
		t.Fatalf("replacement capacity lost %s %d %v", requestID, fence, err)
	}
}

func TestCompilerWorkerStagePublicationDoesNotNeedModelCapacity(t *testing.T) {
	f := newWorkerFixture(t)
	first := f.queue(t, "I prefer tea.")
	later := f.queue(t, "I prefer coffee.")
	ctx := context.Background()
	e := &workerScript{}
	c, err := f.store.claimCompilerJob(ctx, f.owner, first.JobID, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.stageCompilerResult(ctx, f.owner, c.JobID, c.Holder, c.Fence, c.Request, []memory.MemoryCandidate{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, first.JobID); err != nil {
		t.Fatal(err)
	}
	blocked, err := f.store.claimCompilerJob(ctx, f.owner, later.JobID, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.failCompilerAttempt(ctx, blocked.JobID, blocked.Holder, blocked.Fence, false, "invalid_or_missing_output", false); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.second(t).RunCompilerStep(ctx, f.config(e)); !worked || err != nil {
		t.Fatalf("capacity blocked saved stage %v %v", worked, err)
	}
	got, err := f.store.InspectCompilation(ctx, f.owner, first.JobID)
	if err != nil || got.State != "completed_empty" || got.Attempts != 1 || e.calls.Load() != 0 {
		t.Fatalf("saved stage %+v %v", got, err)
	}
	status, err := f.store.InspectCompilerStatus(ctx, f.owner, "", 64)
	if err != nil || status.CapacityState != "capacity_blocked" {
		t.Fatalf("publication released another job capacity %+v %v", status, err)
	}
}

func TestCompilerWorkerUnconfiguredSelectionIsNotMaterialized(t *testing.T) {
	f := newWorkerFixture(t)
	if _, err := f.store.QueueCandidateUnit(context.Background(), f.owner, memory.CompilationSelection{SessionID: f.owner.SessionID}, f.generation, nil); !errors.Is(err, ErrCompilerNotConfigured) {
		t.Fatalf("unconfigured queue %v", err)
	}
	var jobs int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("unconfigured jobs %d %v", jobs, err)
	}
}

func TestCompilerWorkerFullQueueRetainsSelectionForRediscovery(t *testing.T) {
	f := newWorkerFixture(t)
	first := f.queue(t, "I prefer tea.")
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<1023) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'queued-'||x,j.generation_id,j.destination,j.session_id,j.root_id,j.first_sequence,j.last_sequence+x,j.window_hash,j.request,'queued' FROM n CROSS JOIN memory_compiler_jobs j WHERE j.job_id=?`, first.JobID); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	root, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer coffee."})
	if err != nil {
		t.Fatal(err)
	}
	end, err := f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	if err != nil {
		t.Fatal(err)
	}
	selection := memory.CompilationSelection{SessionID: f.owner.SessionID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}
	pending, err := f.store.QueueCandidateUnit(ctx, f.owner, selection, f.generation, &workerScript{})
	if err != nil || pending.State != "selected_unmaterialized" || pending.JobID != "" {
		t.Fatalf("full queue %+v %v", pending, err)
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET state='failed' WHERE job_id='queued-1'`); err != nil {
		t.Fatal(err)
	}
	rediscovered, err := f.second(t).QueueCandidateUnit(ctx, f.owner, selection, f.generation, &workerScript{})
	if err != nil || rediscovered.SelectionID != pending.SelectionID || rediscovered.State != "queued" || rediscovered.Attempts != 0 {
		t.Fatalf("selection lost across full queue %+v %v", rediscovered, err)
	}
}

func TestCompilerWorkerCancelledStageResumeRequiresCapacity(t *testing.T) {
	f := newWorkerFixture(t)
	job := f.queue(t, "I prefer tea.")
	ctx := context.Background()
	e := &workerScript{}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, job.JobID, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.stageCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request, []memory.MemoryCandidate{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CancelCompilation(ctx, f.owner, job.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<128) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'reserved-'||x,j.generation_id,j.destination,j.session_id,j.root_id,j.first_sequence,j.last_sequence+x,j.window_hash,j.request,'staged' FROM n CROSS JOIN memory_compiler_jobs j WHERE j.job_id=?`, job.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO memory_compiler_resources SELECT job_id,0,131072,16 FROM memory_compiler_jobs WHERE job_id LIKE 'reserved-%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.second(t).ResumeCompilation(ctx, f.owner, job.JobID); !errors.Is(err, ErrCompilerCapacityBlocked) {
		t.Fatalf("resume overfilled stages %v", err)
	}
	got, err := f.store.InspectCompilation(ctx, f.owner, job.JobID)
	if err != nil || got.State != "cancelled" || got.Attempts != 1 {
		t.Fatalf("failed admission changed cancellation %+v %v", got, err)
	}
}
