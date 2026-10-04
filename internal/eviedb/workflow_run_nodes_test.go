package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/workflow"
)

// Capability reads (Stage 14 sub-stage 5): interrupted attempts counted
// apart from the Retry Policy, an attempt its own holder records
// interrupted, the per-node admission check, and audited compatible
// provider substitutions.

// notesRun starts a run of the notes-digest example, whose read and write
// steps are covered by its Standing Authority on conn-notes.
func notesRun(t *testing.T, s *Store) (WorkflowActivation, WorkflowRun) {
	t.Helper()
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	a := activate(t, s, p, 0)
	run, _, err := s.StartWorkflowRun(context.Background(), WorkflowRunStart{
		WorkspaceID: a.WorkspaceID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash,
		Composition: json.RawMessage(`{"runtime":"test"}`), RunKey: "notes-digest:k1", Inputs: json.RawMessage(`{"day":"2026-10-05"}`), Start: "read", Surface: "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	return a, run
}

func readAdmission() WorkflowNodeAdmission {
	return WorkflowNodeAdmission{Step: "read", Capability: "notes.list", Connection: "conn-notes", Resource: "notebook:inbox"}
}

// Interrupted attempts are counted apart: they number the step's attempts
// but use neither its Retry Policy nor the run's limits.max_attempts, so a
// crash never spends a retry; the interruption bound counts them instead.
func TestWorkflowRunProgressCountsInterruptedAttemptsApart(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(run.ID, 0, 1)); err != nil {
		t.Fatal(err)
	}
	clock.Advance(leaseFor + 1)
	claim, err = r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Made != 1 || p.Attempts != 0 || p.Interrupted != 1 || p.Counted != 0 || !strings.Contains(p.LastError, "interrupted") || p.InFlight != nil {
		t.Fatalf("after an interruption = %+v", p)
	}
	second, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(run.ID, 0, p.Made+1))
	if err != nil || second.Attempt != 2 {
		t.Fatalf("the attempt after an interruption = %+v, %v", second, err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, claim.Lease, WorkflowAttemptEnd{RunID: run.ID, Seq: second.Seq, Error: "transient"}); err != nil {
		t.Fatal(err)
	}
	p, err = r.s.WorkflowRunProgress(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Made != 2 || p.Attempts != 1 || p.Interrupted != 1 || p.Counted != 1 || p.LastError != "transient" {
		t.Fatalf("after a failed attempt = %+v", p)
	}
}

// A holder that stops while a read is in flight records the attempt
// interrupted itself, under its lease, before it releases the run; a stale
// holder and a finished attempt are refused.
func TestWorkflowRunHolderRecordsItsInterruptedAttempt(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:k1"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(run.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	stale := claim.Lease
	stale.Token++
	if err := r.s.CheckWorkflowRunLease(ctx, claim.Lease); err != nil {
		t.Fatalf("the holder's lease check: %v", err)
	}
	if err := r.s.CheckWorkflowRunLease(ctx, stale); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("another lease's check: %v", err)
	}
	if err := r.s.InterruptWorkflowAttempt(ctx, stale, attempt.Seq, "stopped"); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("interrupt under another lease: %v", err)
	}
	if err := r.s.InterruptWorkflowAttempt(ctx, claim.Lease, attempt.Seq, "interrupted: the runtime stopped during the read"); err != nil {
		t.Fatal(err)
	}
	if err := r.s.InterruptWorkflowAttempt(ctx, claim.Lease, attempt.Seq, "again"); !errors.Is(err, ErrWorkflowRunConflict) {
		t.Fatalf("interrupting a finished attempt: %v", err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Attempts[0]; got.State != WorkflowAttemptInterrupted || got.Error != "interrupted: the runtime stopped during the read" || got.FinishedAt == nil {
		t.Fatalf("attempt = %+v", got)
	}
	if !sameKinds(eventKinds(d.Events), WorkflowEventAttemptInterrupted) || d.Events[0].Holder != "holder-a" || d.Events[0].Attempt != attempt.Seq {
		t.Fatalf("events = %+v", d.Events)
	}
	if err := r.s.ReleaseWorkflowRunLease(ctx, claim.Lease, ""); err != nil {
		t.Fatal(err)
	}
	clock.Advance(1)
	if claim, err = r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor); err != nil || len(claim.Interrupted) != 0 || claim.TookOver != nil {
		t.Fatalf("the next claim = %+v, %v", claim, err)
	}
}

// Before a node acts through a capability, its holder asks admission in a
// fenced transaction: the run's own version must still admit it, the
// operation must be within the run's pinned Standing Authority, and the
// Workspace's current access must still cover the connection and resource.
func TestWorkflowNodeAdmissionAppliesAuthorityAndAccessNow(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	a, run := notesRun(t, r.s)
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.s.AdmitWorkflowNode(ctx, claim.Lease, readAdmission()); err != nil {
		t.Fatalf("an admitted read: %v", err)
	}
	for name, adm := range map[string]WorkflowNodeAdmission{
		"another capability": {Step: "read", Capability: "notes.delete", Connection: "conn-notes", Resource: "notebook:inbox"},
		"another connection": {Step: "read", Capability: "notes.list", Connection: "conn-other", Resource: "notebook:inbox"},
		"another resource":   {Step: "read", Capability: "notes.list", Connection: "conn-notes", Resource: "notebook:digest"},
	} {
		err := r.s.AdmitWorkflowNode(ctx, claim.Lease, adm)
		var refusal *WorkflowNodeRefusal
		if !errors.Is(err, ErrWorkflowNodeRefused) || !errors.As(err, &refusal) || !strings.Contains(refusal.Reason, "Standing Authority") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	stale := claim.Lease
	stale.Token++
	if err := r.s.AdmitWorkflowNode(ctx, stale, readAdmission()); !errors.Is(err, ErrWorkflowRunLeaseLost) {
		t.Fatalf("admission under another lease: %v", err)
	}
	// Narrowing access revokes at once, despite the run's pinned revision.
	if _, err := r.s.SetWorkspaceAccess(ctx, a.WorkspaceID, 1, notesAccess("notebook:digest")); err != nil {
		t.Fatal(err)
	}
	err = r.s.AdmitWorkflowNode(ctx, claim.Lease, readAdmission())
	if !errors.Is(err, ErrWorkflowBlocked) || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("a read after access was narrowed: %v", err)
	}
}

// An owner-approved compatible substitution is an immutable, audited
// record of the replacement provider a run continues with; a repeated
// approval returns the same record, and a finished run takes none.
func TestWorkflowRunSubstitutionsAreAuditedAndImmutable(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	_, run := notesRun(t, r.s)
	sub := WorkflowRunSubstitution{RunID: run.ID, Capability: "notes.list", Plugin: "notes", PinnedVersion: "1.1.0", ReplacementVersion: "1.2.0",
		Contract: "1.0.0", SchemaSHA256: strings.Repeat("a", 64), Evidence: "notes 1.2.0 declares it continues 1.1.0", Surface: "cli"}
	for name, bad := range map[string]WorkflowRunSubstitution{
		"same version": func() WorkflowRunSubstitution { s := sub; s.ReplacementVersion = "1.1.0"; return s }(),
		"no evidence":  func() WorkflowRunSubstitution { s := sub; s.Evidence = ""; return s }(),
		"chat surface": func() WorkflowRunSubstitution { s := sub; s.Surface = "chat"; return s }(),
		"unknown run":  func() WorkflowRunSubstitution { s := sub; s.RunID = "missing"; return s }(),
	} {
		if _, err := r.s.RecordWorkflowRunSubstitution(ctx, bad); err == nil {
			t.Fatalf("%s: recorded", name)
		}
	}
	recorded, err := r.s.RecordWorkflowRunSubstitution(ctx, sub)
	if err != nil || recorded.Seq != 1 || recorded.Approver != "owner" || recorded.ApprovedAt.IsZero() {
		t.Fatalf("recorded = %+v, %v", recorded, err)
	}
	again, err := r.s.RecordWorkflowRunSubstitution(ctx, sub)
	if err != nil || again.Seq != 1 {
		t.Fatalf("repeated approval = %+v, %v", again, err)
	}
	r.reopen()
	subs, err := r.s.WorkflowRunSubstitutions(ctx, run.ID)
	if err != nil || len(subs) != 1 || subs[0].ReplacementVersion != "1.2.0" || subs[0].SchemaSHA256 != sub.SchemaSHA256 {
		t.Fatalf("substitutions = %+v, %v", subs, err)
	}
	d, err := r.s.WorkflowRun(ctx, run.ID)
	if err != nil || len(d.Substitutions) != 1 {
		t.Fatalf("detail substitutions = %+v, %v", d.Substitutions, err)
	}
	for _, stmt := range []string{`UPDATE workflow_run_substitutions SET evidence='x'`, `DELETE FROM workflow_run_substitutions`} {
		if _, err := r.db.ExecContext(ctx, stmt); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, claim.Lease, 0, "stopped"); err != nil {
		t.Fatal(err)
	}
	late := sub
	late.ReplacementVersion = "1.3.0"
	if _, err := r.s.RecordWorkflowRunSubstitution(ctx, late); !errors.Is(err, ErrWorkflowRunFinished) {
		t.Fatalf("a substitution for a finished run: %v", err)
	}
}

var _ = workflow.KindCapability
