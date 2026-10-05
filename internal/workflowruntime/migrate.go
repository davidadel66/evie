package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflow"
)

// Migration (Stage 14 sub-stage 12, 2026-10-04; workflow-runtime.spec.md:
// "An incompatible unfinished run pauses. Explicit migration creates a
// linked Successor Run under an approved new definition, carrying only
// validated state and durable effect evidence. The predecessor remains
// immutable"; ADR 0025 and 0030). Nothing migrates a run by itself: the
// owner asks, from the CLI or the web, for one unfinished run to continue
// under the definition's active version, which must be newer than the
// run's own.
//
// The runtime reads that version's approved commit, checks it can run it
// (its step kinds, models and workers, its pinned providers), and plans the
// resume (workflow.PlanResume): the successor resumes at the step the
// predecessor's latest checkpoint names, with the predecessor's input
// accepted under the new version (which must derive the same logical Run
// Key) and the recorded step outputs the new version's reads accept; any
// difference refuses the migration with every reason. The store then
// records, in one transaction that re-checks the predecessor (unheld,
// nothing in flight, every effect resolved, no question open, still at that
// checkpoint) and run admission for the new version, the Successor Run
// holding the predecessor's Run Key, the migration record and the
// predecessor's supersession.
//
// The successor continues the same logical execution: its Effect Intents
// derive the same idempotency keys, it counts its predecessors' intents
// against max_calls_per_run, an effect step it resumes at whose receipt
// says it happened gives its output from that receipt without calling the
// provider, and its limits count what its predecessors used (time less the
// waits for answers and at the daily cap, attempts, tokens and cost).

var (
	// ErrMigrationRefused refuses a migration: no newer active version, a
	// run that is held, cancelling, waiting for an answer or needing
	// reconciliation, or state the new version's reads do not accept.
	ErrMigrationRefused = errors.New("workflow migration refused")
)

// MigrationRefusedError is a refused migration with its reasons: the
// problems of the run's state under the new version, when that is why.
type MigrationRefusedError struct {
	RunID    string             `json:"run_id"`
	Reason   string             `json:"reason"`
	Problems []workflow.Problem `json:"problems,omitempty"`
}

func (e *MigrationRefusedError) Error() string {
	text := fmt.Sprintf("%s: run %s: %s", ErrMigrationRefused, e.RunID, e.Reason)
	for _, p := range e.Problems {
		text += "\n  " + p.Path + ": " + p.Message
	}
	return text
}

func (e *MigrationRefusedError) Unwrap() error { return ErrMigrationRefused }

// MigrationRequest is the owner migrating an unfinished run to the
// definition's active version, ToVersion, with a reason. Detach records
// the successor and returns at once; otherwise Migrate waits until it
// finishes or pauses, as a start does.
type MigrationRequest struct {
	RunID     string `json:"run_id"`
	ToVersion int64  `json:"to_version"`
	Reason    string `json:"reason,omitempty"`
	Detach    bool   `json:"detach,omitempty"`
	Surface   string `json:"-"`
}

// MigrationResult is the recorded migration, the superseded predecessor
// and its successor.
type MigrationResult struct {
	Migration   eviedb.WorkflowRunMigration `json:"migration"`
	Predecessor eviedb.WorkflowRun          `json:"predecessor"`
	Successor   eviedb.WorkflowRunDetail    `json:"successor"`
}

// Migrate migrates an unfinished run to a newer approved version (owner
// surfaces only).
func (r *Runtime) Migrate(ctx context.Context, req MigrationRequest) (MigrationResult, error) {
	if !ownerSurface(req.Surface) {
		return MigrationResult{}, eviedb.ErrWorkflowRunSurface
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > eviedb.MaxWorkflowCancelReason {
		return MigrationResult{}, fmt.Errorf("a migration reason is at most %d bytes", eviedb.MaxWorkflowCancelReason)
	}
	refuse := func(format string, args ...any) error {
		return &MigrationRefusedError{RunID: req.RunID, Reason: fmt.Sprintf(format, args...)}
	}
	writes := context.WithoutCancel(ctx)
	// A claim settles what a dead holder left in flight (an attempt
	// interrupted, a worker settled, an effect's outcome unknown); the
	// lease is given up at once, so the store's transaction below finds
	// the run unheld.
	claim, err := r.store.AcquireWorkflowRunLease(writes, req.RunID, r.holder, r.sched.Lease)
	switch {
	case err == nil:
		if err := r.store.ReleaseWorkflowRunLease(writes, claim.Lease, ""); err != nil && !errors.Is(err, eviedb.ErrWorkflowRunLeaseLost) {
			return MigrationResult{}, err
		}
	case errors.Is(err, eviedb.ErrWorkflowRunLeaseHeld):
		return MigrationResult{}, refuse("another Evie is advancing it now (%v); migrate it once it pauses", err)
	case errors.Is(err, eviedb.ErrWorkflowRunNotDue), errors.Is(err, eviedb.ErrWorkflowRunNotSchedulable), errors.Is(err, eviedb.ErrWorkflowRunFinished):
	default:
		return MigrationResult{}, err
	}
	d, err := r.store.WorkflowRun(writes, req.RunID)
	if err != nil {
		return MigrationResult{}, err
	}
	run := d.Run
	switch {
	case run.State.Terminal():
		return MigrationResult{}, fmt.Errorf("%w: run %s is %s", eviedb.ErrWorkflowRunFinished, run.ID, run.State)
	case run.State == eviedb.WorkflowRunCancelling:
		return MigrationResult{}, refuse("it is cancelling: the owner's cancellation finishes it")
	case run.State == eviedb.WorkflowRunWaitingForHuman:
		return MigrationResult{}, refuse("it waits for your answer: answer its question (evie workflows answer) or cancel it first")
	}
	if i, unknown := unknownIntent(d.Effects); unknown {
		return MigrationResult{}, refuse("an effect's outcome is unknown (steps.%s, Effect Intent #%d); reconcile it first with evie workflows reconcile %s %s",
			i.Step, i.Seq, run.ID, i.Step)
	}
	if run.State == eviedb.WorkflowRunNeedsReconciliation {
		return MigrationResult{}, refuse("it needs reconciliation: an effect's outcome is unknown; reconcile it first (evie workflows reconcile)")
	}
	a, err := r.store.ActiveWorkflow(writes, run.WorkspaceID, run.Workflow)
	if err != nil {
		if admissionRefused(err) {
			return MigrationResult{}, refuse("%v", err)
		}
		return MigrationResult{}, err
	}
	switch {
	case req.ToVersion != a.Version:
		return MigrationResult{}, refuse("version %d is not the definition's active version: version %d is active", req.ToVersion, a.Version)
	case a.Version <= run.Version:
		return MigrationResult{}, refuse("version %d is not newer than the run's version %d; a migration continues a run under a newer approved version", a.Version, run.Version)
	}
	access, err := r.store.WorkspaceAccess(writes, run.WorkspaceID)
	if err != nil {
		return MigrationResult{}, err
	}
	v, err := r.approvedDefinition(writes, a, access.Access)
	if err != nil {
		return MigrationResult{}, err
	}
	var unsupported []string
	for _, kind := range v.StepKinds() {
		if !slices.Contains(executableKinds, kind) {
			unsupported = append(unsupported, kind)
		}
	}
	if len(unsupported) > 0 {
		return MigrationResult{}, &UnsupportedStepsError{Workflow: a.Name, Kinds: unsupported}
	}
	if r.models == nil && hasAISteps(v) {
		return MigrationResult{}, &ModelsUnavailableError{Workflow: a.Name, Reason: r.modelsOff}
	}
	workers, err := r.pinWorkers(writes, v)
	if err != nil {
		return MigrationResult{}, err
	}
	providers, err := r.pinProviders(writes, v)
	if err != nil {
		return MigrationResult{}, err
	}
	p, err := r.store.WorkflowRunProgress(writes, run.ID)
	if err != nil {
		return MigrationResult{}, err
	}
	if p.InFlight != nil {
		return MigrationResult{}, refuse("steps.%s attempt %d is in flight", p.InFlight.Step, p.InFlight.Attempt)
	}
	inputs, err := decodeObject(run.Inputs)
	if err != nil {
		return MigrationResult{}, refuse("its recorded input cannot be read: %v", err)
	}
	state, err := decodeObject(p.Checkpoint.State)
	if err != nil {
		return MigrationResult{}, refuse("checkpoint %d cannot be read: %v", p.Checkpoint.Seq, err)
	}
	logical := logicalRunKey(d)
	plan, err := v.PlanResume(p.Checkpoint.Next, inputs, state, logical)
	var incompatible *workflow.ResumeError
	if errors.As(err, &incompatible) {
		return MigrationResult{}, &MigrationRefusedError{RunID: run.ID, Problems: incompatible.Problems,
			Reason: fmt.Sprintf("its state at checkpoint %d does not satisfy version %d's reads up to steps.%s", p.Checkpoint.Seq, a.Version, p.Checkpoint.Next)}
	}
	if err != nil {
		return MigrationResult{}, err
	}
	encodedInputs, err := workflow.EncodeValue(plan.Inputs)
	if err != nil {
		return MigrationResult{}, err
	}
	encodedState, err := workflow.EncodeValue(plan.State)
	if err != nil {
		return MigrationResult{}, err
	}
	encodedPlan, err := json.Marshal(plan)
	if err != nil {
		return MigrationResult{}, err
	}
	pinned, err := json.Marshal(composition{Runtime: r.pin, StepKinds: v.StepKinds(), Functions: v.Functions(), Providers: providers, Models: v.ModelPolicies(), Workers: workers})
	if err != nil {
		return MigrationResult{}, err
	}
	m, err := r.store.MigrateWorkflowRun(writes, eviedb.WorkflowMigration{
		PredecessorID: run.ID, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash, Composition: pinned,
		Checkpoint: p.Checkpoint.Seq, Resume: plan.Resume, Inputs: encodedInputs, State: encodedState, Plan: encodedPlan, LogicalKey: logical,
		Reason: reason, Surface: req.Surface,
	})
	if errors.Is(err, eviedb.ErrWorkflowMigrationRefused) {
		return MigrationResult{}, refuse("%v", strings.TrimPrefix(err.Error(), eviedb.ErrWorkflowMigrationRefused.Error()+": "))
	}
	if err != nil {
		return MigrationResult{}, err
	}
	out := MigrationResult{Migration: m}
	pred, err := r.store.WorkflowRunRecord(writes, run.ID)
	if err != nil {
		return out, err
	}
	out.Predecessor = r.presentRun(writes, pred)
	r.wake()
	if req.Detach {
		out.Successor, err = r.store.WorkflowRun(writes, m.SuccessorID)
	} else {
		out.Successor, err = r.await(ctx, m.SuccessorID)
	}
	out.Successor = r.present(writes, out.Successor)
	return out, err
}

// logicalRunKey is the logical execution a run continues (ADR 0021): a
// rerun's logical key, a successor's recorded one, or its own Run Key.
func logicalRunKey(d eviedb.WorkflowRunDetail) string {
	switch {
	case d.Predecessor != nil:
		return d.Predecessor.LogicalKey
	case d.Override != nil:
		return d.Override.LogicalKey
	}
	return d.Run.RunKey
}

// inheritedIntents are the Effect Intents a successor's resume step sent
// in its predecessor (and its predecessor's), at the checkpoint the
// successor continues, oldest first: an effect step a successor resumes at
// counts them as its own earlier attempts.
func (r *Runtime) inheritedIntents(ctx context.Context, runID string, step string) ([]eviedb.WorkflowEffectIntent, error) {
	m, found, err := r.store.WorkflowRunMigration(ctx, runID)
	if err != nil || !found || m.Resume != step {
		return nil, err
	}
	return r.stepIntents(ctx, m.PredecessorID, m.Checkpoint, step)
}

// migrationText renders a run's links to its predecessor and successor.
func migrationText(b *strings.Builder, d eviedb.WorkflowRunDetail) {
	line := func(format string, args ...any) { fmt.Fprintf(b, format+"\n", args...) }
	if m := d.Predecessor; m != nil {
		line("Successor of run %s (version %d), migrated to version %d by the %s via %s at %s%s; resumed at steps.%s from its checkpoint %d",
			m.PredecessorID, m.FromVersion, m.ToVersion, m.Actor, m.Surface, m.RecordedAt.Format(timeLayout), reasonText(m.Reason), m.Resume, m.Checkpoint)
		line("  Carried from the predecessor: %s of limits.max_duration, %d attempts, %s tokens, %s", m.Carried.Elapsed(), m.Carried.Attempts,
			grouped(m.Carried.Tokens), dollars(m.Carried.MicroUSD))
		var plan workflow.ResumePlan
		if json.Unmarshal(m.Plan, &plan) == nil {
			if len(plan.Carried) > 0 {
				line("  Carried steps: %s", strings.Join(plan.Carried, ", "))
			}
			for _, drop := range plan.Dropped {
				line("  Not carried: %s (%s)", drop.Name, drop.Reason)
			}
		}
		for _, e := range m.Effects {
			line("  Inherited Effect Intent #%d of run %s (steps.%s): %s, idempotency key %s; never performed again", e.Seq, e.RunID, e.Step, e.Outcome, e.IdempotencyKey)
		}
	}
	if m := d.Successor; m != nil {
		line("Superseded by run %s: migrated to version %d by the %s via %s at %s%s", m.SuccessorID, m.ToVersion, m.Actor, m.Surface,
			m.RecordedAt.Format(timeLayout), reasonText(m.Reason))
	}
}
