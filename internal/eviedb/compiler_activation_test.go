package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type activationScript struct{ workerScript }

func (*activationScript) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return nil
}

type unavailableActivationScript struct{ activationScript }

func (*unavailableActivationScript) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return errors.New("metadata unavailable")
}

func TestCompilerActivationPreflightAndIdempotence(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	req := activationRequest(f, "preflight", 0)
	if _, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, nil); !errors.Is(err, ErrCompilerNotConfigured) {
		t.Fatal(err)
	}
	if _, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &workerScript{}); !errors.Is(err, ErrCompilerConfiguration) {
		t.Fatal(err)
	}
	if _, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &unavailableActivationScript{}); err == nil {
		t.Fatal("unverified configuration activated")
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activations`); got != 0 {
		t.Fatal("failed preflight activated")
	}
	a, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &unavailableActivationScript{})
	if err != nil || again.ID != a.ID {
		t.Fatalf("idempotence requires a fresh network preflight: %+v %v", again, err)
	}
}

func TestCompilerActivationReplacementRaceAndSelectorOverlap(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	other := f.second(t)
	ctx := context.Background()
	start := make(chan struct{})
	out := make(chan error, 2)
	for n, store := range []*Store{f.store, other} {
		go func(n int, store *Store) {
			<-start
			_, err := store.ActivateCompiler(ctx, f.owner, activationRequest(f, fmt.Sprintf("replace-%d", n), 1), f.generation, &activationScript{})
			out <- err
		}(n, store)
	}
	close(start)
	success, conflicts := 0, 0
	for range 2 {
		err := <-out
		if err == nil {
			success++
		} else if errors.Is(err, ErrCompilerActivationConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("success %d conflicts %d", success, conflicts)
	}
	req := activationRequest(f, "overlap", 0)
	req.Selector.SessionID = ""
	if _, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &activationScript{}); !errors.Is(err, ErrCompilerActivationConflict) {
		t.Fatalf("overlapping lineage accepted: %v", err)
	}
	req = activationRequest(f, "session-destination", 0)
	req.Selector.Destination = "session:" + string(f.owner.SessionID)
	if _, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &activationScript{}); err != nil {
		t.Fatal("distinct session destination", err)
	}
}

func TestCompilerActivationQueueFullRetainsRevisitableObligation(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	// Populate queue metadata without making 1,024 inference calls. These are
	// independent audit jobs; only the activation root is later reconciled.
	_, err := f.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1024) INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state) SELECT 'full-'||x,?,'global',?,?,10000+x,10000+x,'fixture','{}','queued' FROM n`, f.generationID, f.owner.SessionID, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	activationReconcile(t, f, 4)
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activation_roots WHERE state='selected_unmaterialized' AND reason='job_capacity'`); got != 1 {
		t.Fatalf("lost full-queue obligation: %d", got)
	}
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "I also prefer green tea."})
	activationReconcile(t, f, 2)
	if _, err := f.db.Exec(`UPDATE memory_compiler_jobs SET state='failed' WHERE job_id IN ('full-1','full-2')`); err != nil {
		t.Fatal(err)
	}
	f.store = f.second(t)
	activationReconcile(t, f, 4)
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE job_id NOT LIKE 'full-%' AND state='queued'`); got != 2 {
		t.Fatalf("not rediscovered: %d", got)
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_selections WHERE state='selected_unmaterialized'`); got != 0 {
		t.Fatalf("extension hid older queuefull prefix: %d", got)
	}
}

func TestCompilerActivationUnconfiguredAndUnavailableAreIndependentOfAppend(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Unconfigured durable evidence."})
	if _, err := f.store.ReconcileCompilerEvidence(ctx, CompilerSupervisorConfig{}); !errors.Is(err, ErrCompilerNotConfigured) {
		t.Fatal(err)
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs`); got != 0 {
		t.Fatal("unconfigured pending job")
	}
	activationStart(t, f)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Configured durable evidence."})
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	activationReconcile(t, f, 4)
	unavailable := &workerScript{run: func(context.Context, memory.CompilerRequest) (CompilerExtraction, error) {
		return CompilerExtraction{ReleaseEvidence: "not_dispatched"}, errors.New("configured endpoint unreachable")
	}}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(unavailable)); !worked || err == nil {
		t.Fatalf("unavailable endpoint result %v %v", worked, err)
	}
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Foreground still commits."})
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE state='retry_wait' AND attempts=1`); got != 1 {
		t.Fatalf("bounded unavailable retry: %d", got)
	}
}

func activationRequest(f *workerFixture, id string, revision int64) memory.CompilerActivationRequest {
	return memory.CompilerActivationRequest{RequestID: id, ExpectedRevision: revision, Selector: memory.CompilerLiveSelector{SourceScope: "global", Destination: "global", SessionID: f.owner.SessionID}}
}
func activationAppend(t *testing.T, f *workerFixture, input memory.EventInput) memory.Event {
	t.Helper()
	e, err := f.store.AppendEventWithLease(context.Background(), f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, input)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func activationStart(t *testing.T, f *workerFixture) memory.CompilerActivation {
	t.Helper()
	a, err := f.store.ActivateCompiler(context.Background(), f.owner, activationRequest(f, "activate", 0), f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func activationReconcile(t *testing.T, f *workerFixture, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := f.store.ReconcileCompilerEvidence(context.Background(), f.config(&workerScript{})); err != nil {
			t.Fatal(err)
		}
	}
}
func activationCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCompilerActivationAppendCASAndRollback(t *testing.T) {
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			f := newWorkerFixture(t)
			other := f.second(t)
			ctx := context.Background()
			start := make(chan struct{})
			var wg sync.WaitGroup
			var a memory.CompilerActivation
			var event memory.Event
			var activateErr, appendErr error
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				a, activateErr = other.ActivateCompiler(ctx, f.owner, activationRequest(f, "race", 0), f.generation, &activationScript{})
			}()
			go func() {
				defer wg.Done()
				<-start
				event, appendErr = f.store.AppendEventWithLease(ctx, f.owner.SessionID, f.lease.HolderID, f.lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
			}()
			close(start)
			wg.Wait()
			if activateErr != nil || appendErr != nil {
				t.Fatal(activateErr, appendErr)
			}
			position := activationCount(t, f.db, `SELECT commit_position FROM memory_compiler_event_positions WHERE event_id=?`, event.ID)
			want := 0
			if int64(position) > a.AfterPosition {
				want = 1
			}
			if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activation_dirty`); got != want {
				t.Fatalf("dirty=%d want=%d frontier=%d position=%d", got, want, a.AfterPosition, position)
			}
			again, err := f.store.ActivateCompiler(ctx, f.owner, activationRequest(f, "race", 0), f.generation, &activationScript{})
			if err != nil || again.AfterPosition != a.AfterPosition || again.ID != a.ID {
				t.Fatalf("idempotence: %+v %v", again, err)
			}
			before := activationCount(t, f.db, `SELECT value FROM memory_compiler_position_counter`)
			rollback := errors.New("roll back append")
			err = f.store.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
				_, err := conn.ExecContext(ctx, `INSERT INTO events(id,session_id,sequence,event_type,role,content,recorded_at,format_version) VALUES('rolled-back',?,999,'user_message','user','do not retain',datetime('now'),1)`, f.owner.SessionID)
				if err != nil {
					return err
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			if got := activationCount(t, f.db, `SELECT value FROM memory_compiler_position_counter`); got != before {
				t.Fatalf("rollback changed position: %d %d", got, before)
			}
		})
	}
}

func TestCompilerActivationPostFrontierSuffixAndRestart(t *testing.T) {
	f := newWorkerFixture(t)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	a := activationStart(t, f)
	post := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, ParentID: root.ID, Content: "I also prefer green tea."})
	activationReconcile(t, f, 1)
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs`); got != 0 {
		t.Fatal("live root materialized", got)
	}
	end := activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	f.store = f.second(t)
	activationReconcile(t, f, 5)
	var raw []byte
	if err := f.db.QueryRow(`SELECT request FROM memory_compiler_jobs`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var request memory.CompilerRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	if request.Window.FirstSequence != post.Sequence || request.Window.Selection.Cutoff != end.Sequence {
		t.Fatalf("suffix %+v frontier %+v", request.Window, a)
	}
	for _, id := range request.Window.NewEventIDs {
		if id == root.ID {
			t.Fatal("selected pre-frontier root")
		}
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_job_schedule WHERE lane='new' AND position=?`, a.AfterPosition+1); got != 1 {
		t.Fatalf("atomic new scheduling %d", got)
	}
	status, err := f.store.InspectCompilerActivations(context.Background(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if status.SelectedEvents != 2 || status.OutsideSelectionEvents != 1 {
		t.Fatalf("status %+v", status)
	}
}

func TestCompilerActivationReplacementDisableAndResume(t *testing.T) {
	f := newWorkerFixture(t)
	a := activationStart(t, f)
	ctx := context.Background()
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	activationReconcile(t, f, 4)
	req := activationRequest(f, "disable", 1)
	req.ActivationID = a.ID
	disabled, err := f.store.DisableCompilerActivation(ctx, f.owner, req)
	if err != nil {
		t.Fatal(err)
	}
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "This disabled interval is historical."})
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); err != nil || worked {
		t.Fatalf("disabled worker worked=%v err=%v", worked, err)
	}
	g2 := f.generation
	g2.Prompt += " Another generation."
	next, err := f.store.ActivateCompiler(ctx, f.owner, activationRequest(f, "reactivate", 2), g2, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	if next.AfterPosition <= *disabled.ThroughPosition {
		t.Fatal("missing disabled interval")
	}
	req = activationRequest(f, "resume-old", 3)
	req.ActivationID = a.ID
	resumed, err := f.store.ResumeCompilerActivation(ctx, f.owner, req, f.generation, &activationScript{})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.ThroughPosition == nil || resumed.WorkPaused {
		t.Fatalf("resume reopened selection %+v", resumed)
	}
	if worked, err := f.store.RunCompilerStep(ctx, f.config(&workerScript{})); err != nil || !worked {
		t.Fatalf("resumed work=%v err=%v", worked, err)
	}
	status, err := f.store.InspectCompilerActivations(ctx, f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if status.OutsideSelectionEvents != 1 {
		t.Fatalf("disabled interval selected %+v", status)
	}
}

func TestCompilerActivationAllClosureClasses(t *testing.T) {
	for _, closure := range []string{"final", "failed", "interrupted", "crashed", "command", "later_root"} {
		t.Run(closure, func(t *testing.T) {
			f := newWorkerFixture(t)
			activationStart(t, f)
			root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
			switch closure {
			case "final":
				activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
			case "failed", "interrupted":
				kind := memory.EventTurnFailed
				if closure == "interrupted" {
					kind = memory.EventTurnInterrupted
				}
				payload := memory.TurnTerminalPayload{TurnID: root.ID, Stage: "provider", Classification: "provider_error"}
				if closure == "interrupted" {
					payload.Classification = memory.ClassificationCallerCancelled
				}
				activationAppend(t, f, memory.EventInput{Type: kind, ParentID: root.ID, Content: payload.SafeContent(), Payload: compilerJSON(payload)})
			case "crashed", "command":
				if _, err := f.db.Exec(`UPDATE session_turn_leases SET expires_at='2000-01-01T00:00:00Z' WHERE session_id=?`, f.owner.SessionID); err != nil {
					t.Fatal(err)
				}
			case "later_root":
				activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Another turn."})
			}
			before := activationCount(t, f.db, `SELECT COUNT(*) FROM events`)
			activationReconcile(t, f, 6)
			if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE root_id=? AND state='queued'`, root.ID); got != 1 {
				t.Fatalf("closed root not queued: %d", got)
			}
			if got := activationCount(t, f.db, `SELECT COUNT(*) FROM events`); got != before {
				t.Fatal("synthetic terminal")
			}
		})
	}
}

func TestCompilerActivationStalledExtractionDoesNotBlockForeground(t *testing.T) {
	f := newWorkerFixture(t)
	activationStart(t, f)
	root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
	started := make(chan struct{})
	extractor := &workerScript{run: func(ctx context.Context, _ memory.CompilerRequest) (CompilerExtraction, error) {
		close(started)
		<-ctx.Done()
		return CompilerExtraction{}, ctx.Err()
	}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- f.store.RunCompilerHost(ctx, f.config(extractor)) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("extractor did not start")
	}
	foreground := time.Now()
	newRoot := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer coffee."})
	activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: newRoot.ID, Content: "Noted."})
	t.Logf("scripted foreground durable finalization=%s", time.Since(foreground))
	cancel()
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		t.Fatal("host shutdown exceeded cleanup bound")
	}
}

func TestCompilerActivationDisableResumeDoesNotReviveInFlightAuthority(t *testing.T) {
	for _, knownRelease := range []bool{false, true} {
		t.Run(fmt.Sprint(knownRelease), func(t *testing.T) {
			f := newWorkerFixture(t)
			a := activationStart(t, f)
			ctx := context.Background()
			root := activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
			activationAppend(t, f, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Noted."})
			activationReconcile(t, f, 3)
			started := make(chan struct{})
			cancelled := make(chan struct{})
			release := make(chan struct{})
			e := &workerScript{run: func(callCtx context.Context, r memory.CompilerRequest) (CompilerExtraction, error) {
				close(started)
				<-callCtx.Done()
				close(cancelled)
				<-release
				if knownRelease {
					return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}}), ReleaseEvidence: "completed"}, nil
				}
				return CompilerExtraction{}, callCtx.Err()
			}}
			done := make(chan error, 1)
			go func() { _, err := f.store.RunCompilerStep(ctx, f.config(e)); done <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("dispatch did not start")
			}
			req := activationRequest(f, "disable-inflight", 1)
			req.ActivationID = a.ID
			if _, err := f.store.DisableCompilerActivation(ctx, f.owner, req); err != nil {
				t.Fatal(err)
			}
			req = activationRequest(f, "resume-inflight", 2)
			req.ActivationID = a.ID
			if _, err := f.store.ResumeCompilerActivation(ctx, f.owner, req, f.generation, &activationScript{}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-cancelled:
			case <-time.After(time.Second):
				t.Fatal("old epoch revived after resume")
			}
			close(release)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("stale dispatch reported success")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("stale dispatch did not stop")
			}
			if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_candidate_groups`); got != 0 {
				t.Fatal("stale output published")
			}
			want := 1
			if knownRelease {
				want = 0
			}
			if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_capacity`); got != want {
				t.Fatalf("capacity=%d want=%d", got, want)
			}
			if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE attempts=1 AND state='retry_wait'`); got != 1 {
				t.Fatalf("attempt budget lost: %d", got)
			}
		})
	}
}

func TestCompilerActivationDeferredRootDoesNotHideIndependentSession(t *testing.T) {
	f := newWorkerFixture(t)
	ctx := context.Background()
	req := activationRequest(f, "lineage", 0)
	req.Selector.SessionID = ""
	if _, err := f.store.ActivateCompiler(ctx, f.owner, req, f.generation, &activationScript{}); err != nil {
		t.Fatal(err)
	}
	activationAppend(t, f, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Still live."})
	other, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, other.ID, "other-session", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.store.AppendEventWithLease(ctx, other.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Independent assertion."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendEventWithLease(ctx, other.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: r.ID, Content: "Noted."}); err != nil {
		t.Fatal(err)
	}
	activationReconcile(t, f, 8)
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_activation_roots WHERE session_id=? AND state='deferred_live'`, f.owner.SessionID); got != 1 {
		t.Fatal("live root vanished")
	}
	if got := activationCount(t, f.db, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE session_id=? AND state='queued'`, other.ID); got != 1 {
		t.Fatal("independent session starved")
	}
}
