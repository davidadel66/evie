package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type compilerCrashExtractor struct{ calls int }

func (*compilerCrashExtractor) ServerIdentity() string { return "scripted:worker-crash" }
func (e *compilerCrashExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, request memory.CompilerRequest) (CompilerExtraction, error) {
	e.calls++
	return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: request.ID, Candidates: []memory.ExtractorCandidate{}}), ReleaseEvidence: "completed"}, nil
}

type compilerCrashFixture struct {
	path        string
	db          *sql.DB
	store       *Store
	owner       memory.ScopeContext
	selection   memory.CompilationSelection
	compilation memory.Compilation
}

func newCompilerCrashFixture(t *testing.T) *compilerCrashFixture {
	t.Helper()
	f := &compilerCrashFixture{path: filepath.Join(t.TempDir(), "worker.db")}
	f.reopen(t)
	t.Cleanup(func() { f.db.Close() })
	ctx := context.Background()
	session, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.owner = session.ScopeContext()
	lease, err := f.store.AcquireTurnLease(ctx, session.ID, "compiler-crash-source", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := f.store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	if err != nil {
		t.Fatal(err)
	}
	last, err := f.store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	f.selection = memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}
	f.compilation, err = f.store.QueueCandidateUnit(ctx, f.owner, f.selection, compilerCommitGeneration(), &compilerCrashExtractor{})
	if err != nil || f.compilation.State != "queued" {
		t.Fatalf("queue state=%s err=%v", f.compilation.State, err)
	}
	return f
}

func (f *compilerCrashFixture) reopen(t *testing.T) {
	t.Helper()
	if f.db != nil {
		if err := f.db.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	f.db, err = OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = NewStore(f.db)
}

func (f *compilerCrashFixture) expire(t *testing.T) {
	t.Helper()
	// Move the durable lease relative to SQLite's clock, without sleeping or
	// changing a process-local clock that another Store would not observe.
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')-1 WHERE job_id=?`, f.compilation.JobID); err != nil {
		t.Fatal(err)
	}
}

type compilerCrashState struct {
	state                                         string
	attempts                                      int
	capacity                                      string
	stages, consumed, groups, coverage, resources int
}

func (f *compilerCrashFixture) assertState(t *testing.T, want compilerCrashState) {
	t.Helper()
	inspection, err := f.store.InspectCompilation(context.Background(), f.owner, f.compilation.SelectionID)
	if err != nil {
		t.Fatal(err)
	}
	got := compilerCrashState{state: inspection.State, attempts: inspection.Attempts, capacity: inspection.CapacityState}
	if err := f.db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(consumed),0) FROM memory_compiler_stages`).Scan(&got.stages, &got.consumed); err != nil {
		t.Fatal(err)
	}
	for table, target := range map[string]*int{
		"memory_compiler_candidate_groups": &got.groups,
		"memory_compiler_coverage":         &got.coverage,
		"memory_compiler_resources":        &got.resources,
	} {
		if err := f.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if got != want {
		t.Fatalf("durable state=%+v want=%+v", got, want)
	}
	var candidates, events int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_candidates`).Scan(&candidates); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id=?`, f.owner.SessionID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if candidates != 0 || len(inspection.Candidates) != 0 || events != 2 {
		t.Fatalf("candidates=%d reviewable=%d retained events=%d", candidates, len(inspection.Candidates), events)
	}
	if got.coverage != 0 {
		var outcome string
		var eventIDs []byte
		if err := f.db.QueryRow(`SELECT outcome,event_ids FROM memory_compiler_coverage WHERE job_id=?`, f.compilation.JobID).Scan(&outcome, &eventIDs); err != nil {
			t.Fatal(err)
		}
		if outcome != "completed_empty" || string(eventIDs) != string(compilerJSON(f.compilation.Window.NewEventIDs)) {
			t.Fatalf("coverage outcome=%s events=%s", outcome, eventIDs)
		}
	}
}

// Each subprocess exits without running defer or database cleanup. The parent
// opens the same SQLite file, so in-memory mocks cannot manufacture durability.
func TestCompilerWorkerCrashRecovery(t *testing.T) {
	for _, phase := range []string{"before_claim_commit", "after_claim", "after_response", "after_stage", "after_publish"} {
		t.Run(phase, func(t *testing.T) {
			f := newCompilerCrashFixture(t)
			if err := f.db.Close(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCompilerWorkerCrashChild$")
			child.Env = append(os.Environ(), "EVIE_COMPILER_CRASH_PHASE="+phase, "EVIE_COMPILER_CRASH_DB="+f.path, "EVIE_COMPILER_CRASH_JOB="+f.compilation.JobID, "EVIE_COMPILER_CRASH_SESSION="+string(f.owner.SessionID))
			output, err := child.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 86 {
				t.Fatalf("child phase=%s err=%v output=%s", phase, err, output)
			}
			f.reopen(t)
			extractor := &compilerCrashExtractor{}
			config := CompilerSupervisorConfig{Extractors: map[string]CompilerExtractor{f.compilation.GenerationID: extractor}}
			switch phase {
			case "before_claim_commit":
				f.assertState(t, compilerCrashState{state: "queued"})
				if err := f.store.RecoverCompilerWork(ctx); err != nil {
					t.Fatal(err)
				}
				f.assertState(t, compilerCrashState{state: "queued"})
				return
			case "after_claim", "after_response":
				f.assertState(t, compilerCrashState{state: "running", attempts: 1, capacity: "reserved", resources: 1})
				f.expire(t)
				if err := f.store.RecoverCompilerWork(ctx); err != nil {
					t.Fatal(err)
				}
				want := compilerCrashState{state: "retry_wait", attempts: 1, capacity: "release_pending"}
				f.assertState(t, want)
				// Even a due retry after another restart cannot infer release from
				// the dead client's absence or from the expired lease.
				if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now')-1 WHERE job_id=?`, f.compilation.JobID); err != nil {
					t.Fatal(err)
				}
				f.reopen(t)
				worked, err := f.store.RunCompilerStep(ctx, config)
				if worked || !errors.Is(err, ErrCompilerCapacityBlocked) || extractor.calls != 0 {
					t.Fatalf("uncertain retry worked=%v calls=%d err=%v", worked, extractor.calls, err)
				}
				f.assertState(t, want)
				return
			case "after_stage":
				f.assertState(t, compilerCrashState{state: "staged", attempts: 1, stages: 1, resources: 1})
				f.expire(t)
				if err := f.store.RecoverCompilerWork(ctx); err != nil {
					t.Fatal(err)
				}
				worked, err := f.store.RunCompilerStep(ctx, config)
				if !worked || err != nil || extractor.calls != 0 {
					t.Fatalf("stage adoption worked=%v calls=%d err=%v", worked, extractor.calls, err)
				}
			}
			want := compilerCrashState{state: "completed_empty", attempts: 1, stages: 1, consumed: 1, groups: 1, coverage: 1}
			f.assertState(t, want)
			worked, err := f.store.RunCompilerStep(ctx, config)
			if worked || err != nil || extractor.calls != 0 {
				t.Fatalf("completed step worked=%v calls=%d err=%v", worked, extractor.calls, err)
			}
			receipt, err := f.store.CompileCandidateUnit(ctx, f.owner, f.selection, compilerCommitGeneration(), extractor)
			if err != nil || receipt.JobID != f.compilation.JobID || receipt.SelectionID != f.compilation.SelectionID || receipt.Attempts != 1 || receipt.State != "completed_empty" || extractor.calls != 0 {
				t.Fatalf("repeated receipt=%+v calls=%d err=%v", receipt, extractor.calls, err)
			}
			f.assertState(t, want)
		})
	}
}

func TestCompilerWorkerCrashChild(t *testing.T) {
	phase := os.Getenv("EVIE_COMPILER_CRASH_PHASE")
	if phase == "" {
		return
	}
	ctx := context.Background()
	db, err := OpenDBAt(os.Getenv("EVIE_COMPILER_CRASH_DB"))
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	owner := memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: memory.SessionID(os.Getenv("EVIE_COMPILER_CRASH_SESSION"))}
	if phase == "before_claim_commit" {
		store.resolveImmediateTransaction = func(ctx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
			if statement == "COMMIT" {
				os.Exit(86)
			}
			return executeImmediateTransactionStatement(ctx, conn, statement)
		}
	}
	extractor := &compilerCrashExtractor{}
	claim, err := store.claimCompilerJob(ctx, owner, os.Getenv("EVIE_COMPILER_CRASH_JOB"), extractor)
	if err != nil {
		t.Fatal(err)
	}
	if phase == "after_claim" {
		os.Exit(86)
	}
	response, err := extractor.Extract(ctx, claim.Generation, claim.Request)
	if err != nil {
		t.Fatal(err)
	}
	if phase == "after_response" {
		os.Exit(86)
	}
	candidates, err := validateCompilerOutput(claim.Request, response.Raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.stageCompilerResult(ctx, owner, claim.JobID, claim.Holder, claim.Fence, claim.Request, candidates); err != nil {
		t.Fatal(err)
	}
	if phase == "after_stage" {
		os.Exit(86)
	}
	if phase != "after_publish" {
		t.Fatalf("unknown crash phase %q", phase)
	}
	if err := store.publishCompilerResult(ctx, owner, claim.JobID, claim.Holder, claim.Fence, claim.Request); err != nil {
		t.Fatal(err)
	}
	os.Exit(86)
}

type compilerCrashReleaseVerifier func(CompilerCapacityReservation) CompilerReleaseAcknowledgement

func (v compilerCrashReleaseVerifier) VerifyCompilerRelease(_ context.Context, reservation CompilerCapacityReservation) (CompilerReleaseAcknowledgement, error) {
	return v(reservation), nil
}

func TestCompilerWorkerStaleStoreCannotPublishOrRelease(t *testing.T) {
	f := newCompilerCrashFixture(t)
	ctx := context.Background()
	otherDB, err := OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDB.Close()
	other := NewStore(otherDB)
	extractor := &compilerCrashExtractor{}
	claim, err := f.store.claimCompilerJob(ctx, f.owner, f.compilation.JobID, extractor)
	if err != nil {
		t.Fatal(err)
	}
	f.expire(t)
	if err := other.RecoverCompilerWork(ctx); err != nil {
		t.Fatal(err)
	}
	for name, operation := range map[string]func() error{
		"renew": func() error { return f.store.renewCompilerClaim(ctx, claim) },
		"stage": func() error {
			return f.store.stageCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request, []memory.MemoryCandidate{})
		},
		"publish": func() error {
			return f.store.publishCompilerResult(ctx, f.owner, claim.JobID, claim.Holder, claim.Fence, claim.Request)
		},
		"release_after_failure": func() error {
			return f.store.failCompilerAttempt(ctx, claim.JobID, claim.Holder, claim.Fence, true, "late_response", false)
		},
	} {
		if err := operation(); !errors.Is(err, ErrCompilerFence) {
			t.Fatalf("stale %s err=%v", name, err)
		}
	}
	want := compilerCrashState{state: "retry_wait", attempts: 1, capacity: "release_pending"}
	f.assertState(t, want)
	for _, kind := range []string{"no_verifier", "healthy_socket", "wrong_fence", "wrong_holder", "wrong_server", "wrong_request"} {
		t.Run(kind, func(t *testing.T) {
			var verifier CompilerReleaseVerifier
			if kind != "no_verifier" {
				verifier = compilerCrashReleaseVerifier(func(r CompilerCapacityReservation) CompilerReleaseAcknowledgement {
					ack := CompilerReleaseAcknowledgement{Reservation: r, Kind: "request_completed"}
					switch kind {
					case "healthy_socket":
						ack.Kind = "healthy_socket"
					case "wrong_fence":
						ack.Reservation.Fence++
					case "wrong_holder":
						ack.Reservation.Holder = "another-holder"
					case "wrong_server":
						ack.Reservation.ServerIdentity = "another-server"
					case "wrong_request":
						ack.Reservation.RequestID = "another-request"
					}
					return ack
				})
			}
			if err := other.ReconcileCompilerCapacity(ctx, verifier); !errors.Is(err, ErrCompilerCapacityBlocked) {
				t.Fatalf("uncertain release err=%v", err)
			}
			f.assertState(t, want)
		})
	}
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET retry_at=unixepoch('now')-1 WHERE job_id=?`, claim.JobID); err != nil {
		t.Fatal(err)
	}
	if _, err := other.claimCompilerJob(ctx, f.owner, claim.JobID, extractor); !errors.Is(err, ErrCompilerCapacityBlocked) {
		t.Fatalf("replacement with uncertain release err=%v", err)
	}
	verifier := compilerCrashReleaseVerifier(func(r CompilerCapacityReservation) CompilerReleaseAcknowledgement {
		return CompilerReleaseAcknowledgement{Reservation: r, Kind: "request_completed"}
	})
	worked, err := other.RunCompilerStep(ctx, CompilerSupervisorConfig{Extractors: map[string]CompilerExtractor{f.compilation.GenerationID: extractor}, ReleaseVerifier: verifier})
	if !worked || err != nil || extractor.calls != 1 {
		t.Fatalf("verified retry worked=%v calls=%d err=%v", worked, extractor.calls, err)
	}
	f.assertState(t, compilerCrashState{state: "completed_empty", attempts: 2, stages: 1, consumed: 1, groups: 1, coverage: 1})
	var receipts int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_release_receipts WHERE request_id=? AND job_id=? AND fence=? AND holder=? AND server_identity=? AND kind='request_completed'`, claim.AttemptID, claim.JobID, claim.Fence, claim.Holder, extractor.ServerIdentity()).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("request-specific release receipts=%d err=%v", receipts, err)
	}
}
