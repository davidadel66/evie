package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

const splitBillFixtures = `{"inputs": {"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}`

// runDB is a real temporary SQLite file that tests close and reopen.
type runDB struct {
	t    *testing.T
	path string
	db   *sql.DB
	s    *Store
	// clock, when set, is every reopened store's clock.
	clock func() time.Time
}

func openRunDB(t *testing.T) *runDB {
	t.Helper()
	r := &runDB{t: t, path: filepath.Join(t.TempDir(), "evie.db")}
	r.reopen()
	t.Cleanup(func() { r.db.Close() })
	return r
}

// reopen closes the database and opens the same file again, as a restart.
func (r *runDB) reopen() {
	r.t.Helper()
	if r.db != nil {
		if err := r.db.Close(); err != nil {
			r.t.Fatal(err)
		}
	}
	db, err := OpenDBAt(r.path)
	if err != nil {
		r.t.Fatal(err)
	}
	r.db, r.s = db, NewStore(db)
	if r.clock != nil {
		r.s.UseClock(r.clock, time.UTC)
	}
}

// splitBillActivation approves the split-bill example in a new Workspace.
func splitBillActivation(t *testing.T, s *Store) (memory.Workspace, WorkflowActivation) {
	t.Helper()
	w := notesWorkspace(t, s)
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "split-bill"), splitBillFixtures))
	return w, activate(t, s, p, 0)
}

func runStart(a WorkflowActivation, key string) WorkflowRunStart {
	return WorkflowRunStart{
		WorkspaceID: a.WorkspaceID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash,
		Composition: json.RawMessage(`{"runtime":"test"}`), RunKey: key, Inputs: json.RawMessage(`{"bill_id":"b"}`), Start: "split", Surface: "cli",
	}
}

// A run starts only through run admission, in the transaction that records
// it, and pins the approved version, both hashes and the Workspace revision
// and access revision it started under, with its first checkpoint.
func TestWorkflowRunStartPinsTheActivationAndCheckpointsInput(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w, a := splitBillActivation(t, r.s)
	if _, err := r.s.SetWorkspaceAccess(ctx, w.ID, 0, notesAccess("notebook:inbox")); err != nil {
		t.Fatal(err)
	}
	run, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil || existing {
		t.Fatalf("start = %+v, %v, %v", run, existing, err)
	}
	r.reopen()
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := d.Run
	if got.State != WorkflowRunQueued || got.Version != 1 || got.ExecutableHash != a.ExecutableHash || got.AuthorityHash != a.AuthorityHash ||
		got.Commit != a.Commit || got.Path != a.Path || got.WorkspaceRevision != w.CurrentRevisionID || got.AccessRevision != 1 ||
		got.RunKey != "split-bill:k1" || string(got.Inputs) != `{"bill_id":"b"}` || got.Surface != "cli" || got.Checkpoint != 0 || got.NextStep != "split" {
		t.Fatalf("run = %+v", got)
	}
	if len(d.Checkpoints) != 1 || d.Checkpoints[0].Seq != 0 || d.Checkpoints[0].Next != "split" || string(d.Checkpoints[0].State) != `{}` || len(d.Attempts) != 0 {
		t.Fatalf("detail = %+v", d)
	}

	// A version the runtime did not validate is refused: the activation
	// changed between its check and this transaction.
	stale := runStart(a, "split-bill:k2")
	stale.ExecutableHash = "sha256:" + strings.Repeat("0", 64)
	if _, _, err := r.s.StartWorkflowRun(ctx, stale); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("stale hash: %v", err)
	}
	stale = runStart(a, "split-bill:k2")
	stale.Version = 2
	if _, _, err := r.s.StartWorkflowRun(ctx, stale); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("stale version: %v", err)
	}
	for _, surface := range []string{"chat", "model", ""} {
		bad := runStart(a, "split-bill:k3")
		bad.Surface = surface
		if _, _, err := r.s.StartWorkflowRun(ctx, bad); !errors.Is(err, ErrWorkflowRunSurface) {
			t.Fatalf("surface %q: %v", surface, err)
		}
	}
}

// Admission refuses a definition that was never approved, was deactivated,
// or is blocked; nothing is recorded.
func TestWorkflowRunStartGoesThroughRunAdmission(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w, a := splitBillActivation(t, r.s)
	missing := runStart(a, "k")
	missing.Workflow = "nothing"
	if _, _, err := r.s.StartWorkflowRun(ctx, missing); !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("unknown workflow: %v", err)
	}
	if _, err := r.s.ArchiveWorkspace(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "k")); !errors.Is(err, ErrWorkflowBlocked) {
		t.Fatalf("archived Workspace: %v", err)
	}
	w2, a2 := splitBillActivation(t, r.s)
	if _, err := r.s.DeactivateWorkflow(ctx, w2.ID, a2.Name, a2.Version, "retired", "cli"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, runStart(a2, "k")); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("deactivated: %v", err)
	}
	if runs, err := r.s.ListWorkflowRuns(ctx, WorkflowRunFilter{}); err != nil || len(runs) != 0 {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
}

// The database itself refuses a second active or succeeded run for one
// Run Key; a duplicate start returns the existing run, and a failed run
// leaves its key free.
func TestWorkflowRunKeyIsUniqueWhileActiveOrSucceeded(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	_, a := splitBillActivation(t, r.s)
	first, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	again, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil || !existing || again.ID != first.ID {
		t.Fatalf("duplicate start = %+v, %v, %v", again, existing, err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO workflow_runs SELECT 'other',workspace_id,workflow,version,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,composition,run_key,inputs,state,outputs,error,origin_surface,session_id,created_at,started_at,finished_at,updated_at,not_before,source FROM workflow_runs WHERE id=?`, first.ID); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("the database accepted a second active run for one key: %v", err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, testLease(t, r.s, first.ID), 0, "stopped by test"); err != nil {
		t.Fatal(err)
	}
	second, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil || existing || second.ID == first.ID {
		t.Fatalf("start after failure = %+v, %v, %v", second, existing, err)
	}
	finishRun(t, r.s, second)
	r.reopen()
	third, existing, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil || !existing || third.ID != second.ID || third.State != WorkflowRunSucceeded {
		t.Fatalf("start after success = %+v, %v, %v", third, existing, err)
	}
	if runs, _ := r.s.ListWorkflowRuns(ctx, WorkflowRunFilter{WorkspaceID: a.WorkspaceID, Workflow: a.Name}); len(runs) != 2 || runs[0].ID != second.ID {
		t.Fatalf("runs newest first = %+v", runs)
	}
}

// testLease claims a run's lease for a test's writes.
func testLease(t *testing.T, s *Store, runID string) WorkflowRunLease {
	t.Helper()
	c, err := s.AcquireWorkflowRunLease(context.Background(), runID, "test-holder", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return c.Lease
}

// finishRun drives one attempt that ends the graph.
func finishRun(t *testing.T, s *Store, run WorkflowRun) {
	t.Helper()
	ctx := context.Background()
	lease := testLease(t, s, run.ID)
	at, err := s.BeginWorkflowAttempt(ctx, lease, WorkflowAttemptStart{RunID: run.ID, Checkpoint: 0, Step: "split", Kind: workflow.KindDeterministic, Attempt: 1, Input: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: at.Seq, Output: json.RawMessage(`1`), State: json.RawMessage(`{"split":1}`), Outputs: json.RawMessage(`{"x":1}`)}); err != nil {
		t.Fatal(err)
	}
}

// Each attempt is recorded before it runs, from the latest checkpoint; its
// output and the next checkpoint are recorded together before another
// attempt can start; and history is never rewritten.
func TestWorkflowRunAttemptsAndCheckpointsAreOrderedAndImmutable(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	lease := testLease(t, r.s, run.ID)
	begin := func(checkpoint int64, step string, attempt int) (WorkflowNodeAttempt, error) {
		return r.s.BeginWorkflowAttempt(ctx, lease, WorkflowAttemptStart{RunID: run.ID, Checkpoint: checkpoint, Step: step, Kind: workflow.KindDeterministic, Attempt: attempt, Input: json.RawMessage(`{"total":1000}`)})
	}
	if _, err := begin(1, "split", 1); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("future checkpoint: %v", err)
	}
	if _, err := begin(0, "check", 1); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("a step the checkpoint does not name: %v", err)
	}
	if _, err := begin(0, "split", 2); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("skipped attempt number: %v", err)
	}
	first, err := begin(0, "split", 1)
	if err != nil || first.Seq != 1 || first.State != WorkflowAttemptRunning || first.InputCheckpoint != 0 {
		t.Fatalf("first attempt = %+v, %v", first, err)
	}
	r.reopen()
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil || p.Run.State != WorkflowRunRunning || p.Run.StartedAt == nil || p.InFlight == nil || p.InFlight.Seq != 1 {
		t.Fatalf("progress with an attempt in flight = %+v, %v", p, err)
	}
	if _, err := begin(0, "split", 2); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("second attempt while one is in flight: %v", err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: 1, Error: "parts must be between 1 and 1000"}); err != nil {
		t.Fatal(err)
	}
	p, _ = r.s.WorkflowRunProgress(ctx, run.ID)
	if p.Checkpoint.Seq != 0 || p.Attempts != 1 || p.LastError != "parts must be between 1 and 1000" || p.Counted != 1 || p.InFlight != nil || p.Run.State != WorkflowRunRunning {
		t.Fatalf("progress after a failed attempt = %+v", p)
	}
	second, err := begin(0, "split", 2)
	if err != nil || second.Seq != 2 || second.Attempt != 2 {
		t.Fatalf("retry = %+v, %v", second, err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: 2, Output: json.RawMessage(`{"share":333}`), Next: "check", State: json.RawMessage(`{"split":{"share":333}}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: 2, Output: json.RawMessage(`{}`), Next: "check", State: json.RawMessage(`{}`)}); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("finishing an attempt twice: %v", err)
	}
	r.reopen()
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Checkpoints) != 2 || d.Checkpoints[1].Seq != 1 || d.Checkpoints[1].AfterAttempt != 2 || d.Checkpoints[1].Next != "check" || string(d.Checkpoints[1].State) != `{"split":{"share":333}}` {
		t.Fatalf("checkpoints = %+v", d.Checkpoints)
	}
	if len(d.Attempts) != 2 || d.Attempts[0].State != WorkflowAttemptFailed || d.Attempts[1].State != WorkflowAttemptSucceeded || string(d.Attempts[1].Output) != `{"share":333}` || d.Run.NextStep != "check" {
		t.Fatalf("detail = %+v", d)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_node_attempts SET output='{}' WHERE run_id=? AND seq=2`, run.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("rewriting a finished attempt: %v", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_run_checkpoints SET state='{}' WHERE run_id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("rewriting a checkpoint: %v", err)
	}
	for _, table := range []string{"workflow_runs", "workflow_node_attempts", "workflow_run_checkpoints"} {
		if _, err := r.db.ExecContext(ctx, `DELETE FROM `+table); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("deleting %s: %v", table, err)
		}
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET run_key='other' WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("rewriting a pinned field: %v", err)
	}

	// The run fails when its last attempt fails; terminal history is never
	// rewritten.
	third, err := begin(1, "check", 1)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := r.s.FinishWorkflowAttempt(ctx, lease, WorkflowAttemptEnd{RunID: run.ID, Seq: third.Seq, Error: "boom", RunError: "steps.check failed after 1 attempt: boom"})
	if err != nil || failed.State != WorkflowRunFailed || failed.FinishedAt == nil || failed.Error != "steps.check failed after 1 attempt: boom" {
		t.Fatalf("failed run = %+v, %v", failed, err)
	}
	if _, err := begin(1, "check", 2); !errors.Is(err, ErrWorkflowRunFinished) {
		t.Fatalf("attempt on a failed run: %v", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET state='running', finished_at=NULL WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("reopening a terminal run: %v", err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, lease, 1, "again"); !errors.Is(err, ErrWorkflowRunFinished) {
		t.Fatalf("failing a finished run: %v", err)
	}
}

// An originating session must belong to the run's Workspace.
func TestWorkflowRunOriginatingSessionMustBeInTheWorkspace(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w, a := splitBillActivation(t, r.s)
	global, err := r.s.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	req := runStart(a, "k1")
	req.SessionID = global.ID
	if _, _, err := r.s.StartWorkflowRun(ctx, req); !errors.Is(err, ErrWorkflowRunSession) {
		t.Fatalf("session outside the Workspace: %v", err)
	}
	req.SessionID = "missing"
	if _, _, err := r.s.StartWorkflowRun(ctx, req); !errors.Is(err, ErrWorkflowRunSession) {
		t.Fatalf("unknown session: %v", err)
	}
	session, err := r.s.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	req.SessionID = session.ID
	run, _, err := r.s.StartWorkflowRun(ctx, req)
	if err != nil || run.SessionID != session.ID {
		t.Fatalf("session start = %+v, %v", run, err)
	}
}
