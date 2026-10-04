package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Manual deterministic runs (Stage 14 sub-stage 3, 2026-10-04) with
// durable restart and leases (sub-stage 4).
//
// A run starts only from the definition's active approval: the runtime reads
// the approved commit through procedural Git, re-validates and re-hashes it
// against the approval, refuses step kinds this runtime cannot execute yet,
// accepts the input against the declared inputs and derives the Run Key;
// then one SQLite transaction applies run admission
// (authorizeWorkflowActivation), returns the run already holding the key,
// or records the run with its pins and first checkpoint.
//
// A run is then advanced in passes. A pass claims the run's fenced lease
// (the store records any attempt an earlier holder left in flight as
// interrupted), checks that the run may continue (its pinned runtime is one
// this runtime declares compatible, run admission still holds for it, and
// its pinned commit still hashes to its approval; otherwise the run pauses
// visibly), and executes node after node: each attempt is recorded before
// it runs and its output with the next checkpoint before another node
// starts, every write fenced by the lease, which a heartbeat renews. A pass
// ends when the run finishes, pauses, waits for a retry's durable
// not-before time, loses its lease, or the runtime stops; then the lease is
// released. Every decision is read back from SQLite, so any process can
// continue a run from its last checkpoint.

// RuntimeVersion names this runtime's execution semantics in each run's
// Execution Composition (ADR 0016, 0025). Version 2 (sub-stage 5) adds
// capability reads through pinned providers and stops counting interrupted
// attempts against a step's Retry Policy; version 3 (sub-stage 6) adds
// capability effects through the effect protocol, Outcome Unknown and
// reconciliation; version 4 (sub-stage 7) adds AI steps through pinned
// Model Policies and durable interruptions, and presents unknown effects
// as interruptions; version 5 (sub-stage 9) adds agent steps that run the
// existing workers under the run's lease, with their presets pinned, and
// bounded map fan-out with per-item attempts; version 6 (sub-stage 10)
// records an agent step's output without the worker's full report (its
// summary, sources, limitations, tokens and typed result, with the
// execution whose report the report store keeps), so checkpoints stay
// bounded. An Evie without a version's semantics pauses such a run as
// incompatible instead of running it.
const RuntimeVersion = "evie.workflow.runtime/6"

// fullReportRuntime is the last runtime whose agent outputs carry the
// worker's full report; its runs keep that shape to the end.
const fullReportRuntime = "evie.workflow.runtime/5"

// compatibleRuntimes are the pinned runtimes this runtime declares it can
// continue (ADR 0025). Runs pinned to evie.workflow.runtime/1 (sub-stages 3
// and 4) hold only deterministic and branch steps, whose semantics are
// unchanged; the one difference, that an interruption no longer spends one
// of a step's attempts, only lets a pure node run again from the same
// checkpoint and never changes a recorded output. Runs pinned to
// evie.workflow.runtime/2 (sub-stage 5) were refused any effect at start,
// so they hold only those steps and capability reads, whose semantics are
// unchanged. Runs pinned to evie.workflow.runtime/3 (sub-stage 6) were
// refused AI steps and interruptions at start; their effects keep the same
// protocol, and an unknown outcome is now also presented to the owner as
// an interruption whose answer is the same reconciliation record. Runs
// pinned to evie.workflow.runtime/4 (sub-stage 7) were refused agent and
// map steps at start, so they hold none; the only change they see is that
// a map step's own attempt and item attempts are counted per item, which
// none of them has. Runs pinned to evie.workflow.runtime/5 (sub-stage 9)
// keep recording each agent output with the worker's full report, as they
// did when they started (sub-stage 10). A run pinned to any other runtime
// pauses visibly.
var compatibleRuntimes = []string{"evie.workflow.runtime/1", "evie.workflow.runtime/2", "evie.workflow.runtime/3", "evie.workflow.runtime/4", fullReportRuntime, RuntimeVersion}

// executableKinds are the step kinds this runtime version executes:
// capability reads and, since sub-stage 6, capability effects; since
// sub-stage 7, AI steps and interruptions; since sub-stage 9, agent and
// map steps.
var executableKinds = []string{workflow.KindAgent, workflow.KindAI, workflow.KindBranch, workflow.KindCapability, workflow.KindDeterministic, workflow.KindInterrupt, workflow.KindMap}

var (
	// ErrUnsupportedSteps refuses a definition with steps this runtime
	// version cannot execute; nothing is recorded or run.
	ErrUnsupportedSteps = errors.New("workflow steps not yet supported in this runtime version")
	// ErrDefinitionMismatch refuses a run whose approved commit no longer
	// validates or hashes to its approval.
	ErrDefinitionMismatch = errors.New("the approved workflow definition no longer matches its approval")
	// ErrRunInput refuses input that does not match the declared inputs.
	ErrRunInput = errors.New("invalid workflow run input")
)

// UnsupportedStepsError names the step kinds a definition uses that this
// runtime version cannot execute.
type UnsupportedStepsError struct {
	Workflow string
	Kinds    []string
}

func (e *UnsupportedStepsError) Error() string {
	return fmt.Sprintf("workflow %s uses %s steps, not yet supported in this runtime version (it runs deterministic, branch, capability, AI and interruption steps); no run was started",
		e.Workflow, strings.Join(e.Kinds, ", "))
}

func (e *UnsupportedStepsError) Unwrap() error { return ErrUnsupportedSteps }

// RunRequest is the owner starting a run of a Workspace's active workflow.
// Inputs are JSON values; TextInputs (from a command line) are converted to
// each input's declared type. SessionID optionally names the originating
// session, which must belong to the Workspace. Detach records the run and
// returns at once, leaving it to a scheduler (a background run); otherwise
// the start waits until the run finishes or pauses.
type RunRequest struct {
	WorkspaceID memory.WorkspaceID         `json:"workspace_id"`
	Workflow    string                     `json:"name"`
	Inputs      map[string]json.RawMessage `json:"inputs,omitempty"`
	TextInputs  map[string]string          `json:"-"`
	SessionID   memory.SessionID           `json:"session_id,omitempty"`
	Detach      bool                       `json:"detach,omitempty"`
	Surface     string                     `json:"-"`
	// RerunOf and Reason are the owner's audited rerun override (sub-stage
	// 8, ADR 0021): the succeeded run whose logical execution runs again,
	// and why. Without inputs, the rerun takes that run's accepted input.
	RerunOf string `json:"rerun_of,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Chat is the chat turn starting a run with surface "chat" (sub-stage
	// 10, StartFromChat); such a start is always detached and never a
	// rerun.
	Chat *eviedb.WorkflowChatOrigin `json:"-"`
	// Schedule is the slot a start with surface "schedule" decides
	// (sub-stage 11, the schedule ticker): its inputs come from the
	// approved schedule and the slot, never from the request, and such a
	// start is always detached and never a rerun.
	Schedule *ScheduledStart `json:"-"`
}

// RunResult is a started run, or the run that already holds its Run Key
// (Existing; nothing new was started).
type RunResult struct {
	Run      eviedb.WorkflowRunDetail `json:"run"`
	Existing bool                     `json:"existing"`
}

// RunFilter narrows ListRuns.
type RunFilter struct {
	WorkspaceID memory.WorkspaceID `json:"workspace_id,omitempty"`
	Workflow    string             `json:"name,omitempty"`
	Limit       int                `json:"limit,omitempty"`
}

// composition is a run's Execution Composition: the runtime version, what
// it executes, and (sub-stage 5) the provider pinned for each capability it
// reads. Runs recorded before sub-stage 5 have no providers.
type composition struct {
	Runtime   string           `json:"runtime"`
	StepKinds []string         `json:"step_kinds"`
	Functions []string         `json:"functions"`
	Providers []pinnedProvider `json:"providers,omitempty"`
	// Models are the Model Policies its AI steps use (sub-stage 7), each
	// with its exact provider, model and parameters (ADR 0019); since
	// sub-stage 9 also those of its agent steps.
	Models []workflow.PolicyReport `json:"model_policies,omitempty"`
	// Workers are the worker presets its agent steps run, each with the
	// digest of its composition at start (sub-stage 9, ADR 0016): every
	// worker admission requires the preset to compose to it still.
	Workers []pinnedWorker `json:"workers,omitempty"`
}

type preparedRun struct {
	v     *workflow.Validated
	start eviedb.WorkflowRunStart
}

// StartRun starts a run of the Workspace's active workflow, or returns the
// run that already holds its Run Key without executing anything. Only the
// owner's CLI and web surfaces start runs. A detached start returns the
// recorded run at once; otherwise StartRun waits until the run finishes or
// pauses (await). The run does not belong to the caller: a caller that
// stops waiting or disconnects does not stop it.
func (r *Runtime) StartRun(ctx context.Context, req RunRequest) (RunResult, error) {
	switch {
	case ownerSurface(req.Surface) && req.Chat == nil && req.Schedule == nil:
	case req.Surface == "chat" && req.Chat != nil && req.Schedule == nil && req.RerunOf == "" && strings.TrimSpace(req.Reason) == "":
		// A chat starts a background run and goes on with its turn.
		req.Detach = true
	case req.Surface == "schedule" && req.Schedule != nil && req.Chat == nil && req.SessionID == "" && req.RerunOf == "" && strings.TrimSpace(req.Reason) == "":
		// A schedule starts a background run (sub-stage 11).
		req.Detach = true
	default:
		return RunResult{}, eviedb.ErrWorkflowRunSurface
	}
	err := eviedb.ErrWorkflowChanged
	// A version approved between validation and the run's transaction is
	// validated again; it is never started unvalidated.
	for range 3 {
		var p preparedRun
		if p, err = r.prepareRun(ctx, req); err != nil {
			return RunResult{}, err
		}
		run, existing, startErr := r.store.StartWorkflowRun(ctx, p.start)
		if errors.Is(startErr, eviedb.ErrWorkflowChanged) {
			err = startErr
			continue
		}
		if startErr != nil {
			return RunResult{}, startErr
		}
		if existing || req.Detach {
			if !existing {
				r.wake()
			}
			detail, err := r.store.WorkflowRun(context.WithoutCancel(ctx), run.ID)
			return RunResult{Run: detail, Existing: existing}, err
		}
		detail, err := r.await(ctx, run.ID)
		return RunResult{Run: detail}, err
	}
	return RunResult{}, err
}

// prepareRun does everything a start needs before its transaction: the
// active version, its approved commit re-validated and re-hashed, the step
// kinds checked, the input accepted and the Run Key derived.
func (r *Runtime) prepareRun(ctx context.Context, req RunRequest) (preparedRun, error) {
	a, err := r.store.ActiveWorkflow(ctx, req.WorkspaceID, req.Workflow)
	if err != nil {
		return preparedRun{}, err
	}
	if req.Schedule != nil && (a.Version != req.Schedule.Version || a.Schedule == nil || a.Schedule.Timezone != req.Schedule.Timezone) {
		return preparedRun{}, fmt.Errorf("%w: version %d is active, the slot is version %d's", errScheduleChanged, a.Version, req.Schedule.Version)
	}
	access, err := r.store.WorkspaceAccess(ctx, req.WorkspaceID)
	if err != nil {
		return preparedRun{}, err
	}
	v, err := r.approvedDefinition(ctx, a, access.Access)
	if err != nil {
		return preparedRun{}, err
	}
	if req.Schedule != nil {
		// A scheduled run's inputs are the approved schedule's, with the
		// slot's nominal local date and time (sub-stage 11).
		if req.Inputs, err = v.ScheduleInputs(req.Schedule.Slot); err != nil {
			return preparedRun{}, fmt.Errorf("%w: %v", ErrRunInput, err)
		}
		req.TextInputs = nil
	}
	kinds := v.StepKinds()
	var unsupported []string
	for _, kind := range kinds {
		if !slices.Contains(executableKinds, kind) {
			unsupported = append(unsupported, kind)
		}
	}
	if len(unsupported) > 0 {
		slices.Sort(unsupported)
		return preparedRun{}, &UnsupportedStepsError{Workflow: a.Name, Kinds: unsupported}
	}
	if r.models == nil && hasAISteps(v) {
		return preparedRun{}, &ModelsUnavailableError{Workflow: a.Name, Reason: r.modelsOff}
	}
	// Agent steps need a worker supervisor, and each worker preset they run
	// is pinned now (sub-stage 9).
	workers, err := r.pinWorkers(ctx, v)
	if err != nil {
		return preparedRun{}, err
	}
	var prior eviedb.WorkflowRunDetail
	var logical string
	if req.RerunOf != "" || strings.TrimSpace(req.Reason) != "" {
		if prior, logical, err = r.rerunBase(ctx, req, a.Name); err != nil {
			return preparedRun{}, err
		}
		if len(req.Inputs) == 0 && len(req.TextInputs) == 0 {
			if req.Inputs, err = workflow.ParseInputs(prior.Run.Inputs); err != nil {
				return preparedRun{}, fmt.Errorf("%w: run %s's input cannot be read: %v", ErrRerunRefused, prior.Run.ID, err)
			}
		}
	}
	inputs, err := v.AcceptInputs(req.Inputs, req.TextInputs)
	if err != nil {
		return preparedRun{}, fmt.Errorf("%w: %v", ErrRunInput, err)
	}
	encoded, err := workflow.EncodeValue(inputs)
	if err != nil {
		return preparedRun{}, err
	}
	providers, err := r.pinProviders(ctx, v)
	if err != nil {
		return preparedRun{}, err
	}
	pinned, err := json.Marshal(composition{Runtime: r.pin, StepKinds: kinds, Functions: v.Functions(), Providers: providers, Models: v.ModelPolicies(), Workers: workers})
	if err != nil {
		return preparedRun{}, err
	}
	start := eviedb.WorkflowRunStart{
		WorkspaceID: req.WorkspaceID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash,
		Composition: pinned, RunKey: v.RunKey(inputs), Inputs: encoded, Start: v.Start(), Surface: req.Surface, SessionID: req.SessionID, Chat: req.Chat,
	}
	if s := req.Schedule; s != nil {
		start.Slot = &eviedb.WorkflowScheduleSlotStart{Timezone: s.Timezone, Key: s.Slot.Key, Due: s.Slot.Due, Shifted: s.Slot.Shifted, Holder: r.holder}
	}
	if req.RerunOf != "" {
		if start.RunKey != logical {
			return preparedRun{}, fmt.Errorf("%w: run %s ran the logical execution %s, and these inputs name %s", ErrRerunRefused, prior.Run.ID, logical, start.RunKey)
		}
		if start.RunKey, err = rerunKey(a.Name, logical, prior.Run.ID); err != nil {
			return preparedRun{}, err
		}
		start.RerunOf, start.LogicalKey, start.Reason = prior.Run.ID, logical, strings.TrimSpace(req.Reason)
	}
	return preparedRun{v: v, start: start}, nil
}

// approvedDefinition reads the activation's exact commit through procedural
// Git and requires it to validate and hash to what was approved.
func (r *Runtime) approvedDefinition(ctx context.Context, a eviedb.WorkflowActivation, access workflow.WorkspaceAccess) (*workflow.Validated, error) {
	v, err := workflow.Validate(r.repo.Source(ctx, a.Commit, a.Path), r.environment(access))
	if err != nil {
		return nil, fmt.Errorf("%w: version %d at commit %s no longer validates; approve the definition again: %v", ErrDefinitionMismatch, a.Version, short(a.Commit), err)
	}
	if v.Hash() != a.ExecutableHash {
		return nil, fmt.Errorf("%w: version %d at commit %s hashes to %s, not the approved %s; approve the definition again",
			ErrDefinitionMismatch, a.Version, short(a.Commit), v.Hash(), a.ExecutableHash)
	}
	if authority := workflow.RequirementsOf(v.Report()).AuthorityHash; authority != a.AuthorityHash {
		return nil, fmt.Errorf("%w: version %d requests authority %s, not the approved %s; approve the definition again",
			ErrDefinitionMismatch, a.Version, authority, a.AuthorityHash)
	}
	return v, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func attempts(n int) string {
	if n == 1 {
		return "1 attempt"
	}
	return fmt.Sprintf("%d attempts", n)
}

// await waits until the run finishes or pauses and returns it. While this
// process's scheduler runs, the scheduler executes the run and await only
// watches; otherwise await executes it here, detached from the caller's
// cancellation so a caller that goes away never stops it between nodes.
// Another process may hold the run meanwhile; await then watches until that
// process finishes it or its lease expires. The caller's cancellation ends
// the wait, never the run.
func (r *Runtime) await(ctx context.Context, id string) (eviedb.WorkflowRunDetail, error) {
	reads := context.WithoutCancel(ctx)
	for {
		changed := r.changes()
		wait := r.sched.Poll
		inline := !r.schedulerRunning()
		if inline {
			end, err := r.execute(reads, id)
			if err != nil {
				d, _ := r.store.WorkflowRun(reads, id)
				return d, fmt.Errorf("workflow run %s: %w", id, err)
			}
			if end == passWaiting {
				wait = 0
			}
		} else {
			r.wake()
			if err := r.failure(id); err != nil {
				d, _ := r.store.WorkflowRun(reads, id)
				return d, fmt.Errorf("workflow run %s: %w", id, err)
			}
		}
		d, err := r.store.WorkflowRun(reads, id)
		if err != nil || settled(d) {
			return d, err
		}
		if wait == 0 {
			// Waiting for a retry: until its not-before time.
			wait = r.sched.Poll
			if d.Run.NotBefore != nil {
				wait = max(d.Run.NotBefore.Sub(r.now()), time.Millisecond)
			}
		}
		if inline {
			if err := r.sleep(ctx, wait); err != nil {
				return d, err
			}
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return d, ctx.Err()
		case <-changed:
		case <-timer.C:
		}
		timer.Stop()
	}
}

// settled reports a run await returns on: finished, paused, waiting for
// something no scheduler resumes, or (sub-stage 8) cancelling while an
// effect's outcome is unknown, which only its reconciliation ends. A run
// cancelling otherwise is not settled: the node in flight finishes and the
// cancellation with it.
func settled(d eviedb.WorkflowRunDetail) bool {
	switch state := d.Run.State; {
	case state.Terminal():
		return true
	case state == eviedb.WorkflowRunCancelling:
		_, unknown := unknownIntent(d.Effects)
		return unknown
	}
	return d.Run.State != eviedb.WorkflowRunQueued && d.Run.State != eviedb.WorkflowRunRunning
}

// passEnd is why an execution pass stopped.
type passEnd int

const (
	passDone    passEnd = iota // the run is finished
	passPaused                 // the run waits for a dependency
	passWaiting                // the run waits for its retry time
	passHeld                   // another runtime holds the run
	passLost                   // the lease was lost during the pass
	passStopped                // this runtime is stopping
	passParked                 // the run waits for something no scheduler resumes
)

// releasedAtStop is recorded when a runtime stops while its run still has
// work to do.
const releasedAtStop = "the runtime stopped; the run continues from its latest checkpoint in the next process that claims it"

// execute is one execution pass of a run by this runtime: claim its lease
// (recording what an earlier holder left in flight as interrupted), check
// that the run may continue, advance it under the lease, which a heartbeat
// renews, and release the lease. ctx ending stops the pass between nodes;
// a node in flight finishes and is recorded first.
func (r *Runtime) execute(ctx context.Context, runID string) (passEnd, error) {
	writes := context.WithoutCancel(ctx)
	claim, err := r.store.AcquireWorkflowRunLease(writes, runID, r.holder, r.sched.Lease)
	switch {
	case errors.Is(err, eviedb.ErrWorkflowRunLeaseHeld):
		return passHeld, nil
	case errors.Is(err, eviedb.ErrWorkflowRunNotDue):
		return passWaiting, nil
	case errors.Is(err, eviedb.ErrWorkflowRunFinished):
		return passDone, nil
	case errors.Is(err, eviedb.ErrWorkflowRunNotSchedulable):
		return passParked, nil
	case err != nil:
		return 0, err
	}
	lease := claim.Lease
	signal, stopListening := r.listenForCancel(runID)
	defer stopListening()
	lost, markLost := context.WithCancelCause(context.Background())
	defer markLost(nil)
	beat, stopBeat := context.WithCancel(context.Background())
	beating := make(chan struct{})
	go func() {
		defer close(beating)
		r.heartbeat(beat, lease, markLost)
	}()
	end, err := r.continueRun(ctx, writes, lease, lost, signal)
	if errors.Is(err, eviedb.ErrWorkflowRunCancelling) {
		// The owner's cancellation arrived while the run was checked: the
		// run is cancelling now, which this claim finishes.
		end, err = r.continueRun(ctx, writes, lease, lost, signal)
	}
	stopBeat()
	<-beating
	if errors.Is(err, eviedb.ErrWorkflowRunLeaseLost) || lost.Err() != nil && end != passDone {
		end, err = passLost, nil
	}
	if end != passLost {
		reason := ""
		if end == passStopped {
			reason = releasedAtStop
		}
		if releaseErr := r.store.ReleaseWorkflowRunLease(writes, lease, reason); releaseErr != nil && !errors.Is(releaseErr, eviedb.ErrWorkflowRunLeaseLost) {
			err = errors.Join(err, fmt.Errorf("release workflow run lease: %w", releaseErr))
		}
	}
	return end, err
}

// heartbeat renews the lease every third of its duration until stop ends.
// A lease the store reports lost, or one past its expiry with renewals
// failing, ends the pass through markLost.
func (r *Runtime) heartbeat(stop context.Context, lease eviedb.WorkflowRunLease, markLost context.CancelCauseFunc) {
	ticker := time.NewTicker(max(r.sched.Lease/3, time.Millisecond))
	defer ticker.Stop()
	expires := lease.ExpiresAt
	for {
		select {
		case <-stop.Done():
			return
		case <-ticker.C:
		}
		renewed, err := r.store.HeartbeatWorkflowRunLease(stop, lease, r.sched.Lease)
		switch {
		case err == nil:
			expires = renewed.ExpiresAt
		case stop.Err() != nil:
			return
		case errors.Is(err, eviedb.ErrWorkflowRunLeaseLost) || !r.now().Before(expires):
			markLost(fmt.Errorf("%w: %v", eviedb.ErrWorkflowRunLeaseLost, err))
			return
		}
	}
}

// admissionRefused reports the run admission refusals that pause a run.
func admissionRefused(err error) bool {
	return errors.Is(err, eviedb.ErrWorkflowNotActive) || errors.Is(err, eviedb.ErrWorkflowBlocked) || errors.Is(err, eviedb.ErrWorkflowNotFound)
}

// pass is one execution pass's context: the runtime's (ending when it
// stops), the durable writes' (never cancelled mid-transaction), the
// lease's (ending when it is lost), the lease, the run's approved
// definition and its pinned Execution Composition.
type pass struct {
	stop   context.Context
	writes context.Context
	lost   context.Context
	lease  eviedb.WorkflowRunLease
	v      *workflow.Validated
	comp   composition
	// cancel ends when the owner's cancellation of the run reaches this
	// pass (sub-stage 8); nil outside a pass.
	cancel *cancelSignal
	// settling is set when this pass found an effect's outcome unknown and
	// automatic reconciliation waits its settle delay (sub-stage 6).
	settling bool
}

// continueRun checks, under the lease, that the run may continue, pausing
// it visibly when it may not, and then advances it. The checks run at
// every claim, so they hold whenever a run resumes after a restart, a
// retry's backoff or a pause: its pinned runtime must be one this runtime
// declares compatible (ADR 0025), its pinned providers must still be the
// ones it pinned or audited substitutes (sub-stage 5), run admission must
// still hold for it (ADR 0033: revocations apply despite the pin), its
// pinned commit must still validate and hash to its approval, and when its
// next step reads a capability, that provider must be available now.
func (r *Runtime) continueRun(ctx, writes context.Context, lease eviedb.WorkflowRunLease, lost context.Context, signal *cancelSignal) (passEnd, error) {
	pause := func(reason string) (passEnd, error) {
		if _, err := r.store.PauseWorkflowRun(writes, lease, reason, r.sched.Recheck); err != nil {
			return 0, err
		}
		return passPaused, nil
	}
	p, err := r.store.WorkflowRunProgress(writes, lease.RunID)
	if err != nil {
		return 0, err
	}
	run := p.Run
	switch run.State {
	case eviedb.WorkflowRunNeedsReconciliation:
		// Never paused or resumed: only a receipt lets it continue.
		return r.continueReconciliation(ctx, writes, lease, lost, p, signal)
	case eviedb.WorkflowRunCancelling:
		// The owner cancelled it: finishing calls nothing, so it needs
		// neither admission nor the approved definition (sub-stage 8).
		return r.continueCancellation(ctx, writes, lease, lost, p, signal)
	case eviedb.WorkflowRunWaitingForHuman:
		// Only the owner's answer lets it continue (sub-stage 7).
		return passParked, nil
	}
	comp, reason := r.incompatible(run.Composition)
	if reason != "" {
		return pause(reason)
	}
	if reason, err := r.providersCompatible(writes, run.ID, comp); err != nil || reason != "" {
		if err != nil {
			return 0, err
		}
		return pause(reason)
	}
	if err := r.store.AdmitWorkflowRun(writes, lease); err != nil {
		if admissionRefused(err) {
			return pause(err.Error())
		}
		return 0, err
	}
	access, err := r.store.WorkspaceAccess(writes, run.WorkspaceID)
	if err != nil {
		return 0, err
	}
	v, err := r.approvedDefinition(writes, pinnedActivation(run), access.Access)
	if errors.Is(err, ErrDefinitionMismatch) {
		return pause(err.Error())
	}
	if err != nil {
		return 0, err
	}
	// A run paused before an AI step in an Evie whose AI steps are off
	// stays paused, rather than resuming only to pause again, until an Evie
	// that allows them claims it (sub-stage 7).
	if next, ok := v.Step(p.Checkpoint.Next); ok && p.InFlight == nil {
		kind := next.Kind
		if m, isMap := v.MapStep(next.ID); isMap {
			kind = m.Item.Kind
		}
		switch {
		case kind == workflow.KindAI && r.models == nil:
			return pause(r.modelsPause(next.ID))
		case kind == workflow.KindAgent && r.agents == nil:
			// Likewise a run before an agent step, or a map of agent items,
			// in an Evie that cannot run workers (sub-stage 9).
			return pause(r.agentsPause(next.ID, r.agentsOff))
		}
	}
	// A run paused for an unavailable provider stays paused, rather than
	// resuming only to pause again before the read, until it is ready; one
	// paused before an effect whose Resource Conflict Key another run
	// holds stays paused until the key is free.
	if next, ok := v.Step(p.Checkpoint.Next); ok && next.Kind == workflow.KindCapability && p.InFlight == nil && !run.State.Terminal() {
		if pin, ok := comp.pin(next.Capability); ok {
			provider, reason, err := r.checkPinned(writes, run.ID, pin, true)
			if err != nil {
				return 0, err
			}
			if reason != "" {
				return pause(fmt.Sprintf("steps.%s: %s", next.ID, reason))
			}
			if next.Effect && run.State == eviedb.WorkflowRunWaitingForDependency {
				held, err := r.pendingConflict(writes, p, v, next, provider)
				if err != nil {
					return 0, err
				}
				if held != nil {
					if err := r.waitForConflict(writes, lease, next, held); err != nil {
						return 0, err
					}
					return passPaused, nil
				}
			}
		}
	}
	if _, err := r.store.ResumeWorkflowRun(writes, lease); err != nil {
		if admissionRefused(err) {
			return pause(err.Error())
		}
		return 0, err
	}
	return r.advance(&pass{stop: ctx, writes: writes, lost: lost, lease: lease, v: v, comp: comp, cancel: signal})
}

// pinnedActivation is the approved version a run is pinned to, as
// approvedDefinition reads it.
func pinnedActivation(run eviedb.WorkflowRun) eviedb.WorkflowActivation {
	return eviedb.WorkflowActivation{WorkspaceID: run.WorkspaceID, Name: run.Workflow, Version: run.Version, Commit: run.Commit, Path: run.Path,
		ExecutableHash: run.ExecutableHash, AuthorityHash: run.AuthorityHash}
}

// incompatible reads the run's pinned Execution Composition and says why
// this runtime cannot continue it, or "" when it can. Nothing is migrated:
// the run pauses until a compatible runtime claims it (ADR 0025).
func (r *Runtime) incompatible(pinned json.RawMessage) (composition, string) {
	var c composition
	if err := json.Unmarshal(pinned, &c); err != nil {
		return c, fmt.Sprintf("its pinned Execution Composition cannot be read (%v); this runtime does not continue it", err)
	}
	if !slices.Contains(r.compatible, c.Runtime) {
		return c, fmt.Sprintf("its pinned runtime %s is not one this Evie declares compatible (%s); continue it with an Evie that does, or migrate it to a new run once migration ships",
			c.Runtime, strings.Join(r.compatible, ", "))
	}
	var unknown []string
	for _, kind := range c.StepKinds {
		if !slices.Contains(executableKinds, kind) {
			unknown = append(unknown, kind)
		}
	}
	if len(unknown) > 0 {
		return c, fmt.Sprintf("its pinned step kinds %s are not ones this runtime executes", strings.Join(unknown, ", "))
	}
	return c, ""
}

// advance executes the run from its latest durable checkpoint until it ends,
// pauses or waits for a retry, the lease is lost, or the runtime stops.
// Each step reads the run's progress back from SQLite, so nothing about the
// run lives only in memory between nodes; every write is fenced by the
// lease.
func (r *Runtime) advance(pc *pass) (passEnd, error) {
	for {
		if pc.lost.Err() != nil {
			return passLost, nil
		}
		p, err := r.store.WorkflowRunProgress(pc.writes, pc.lease.RunID)
		if err != nil {
			return 0, err
		}
		switch {
		case p.Run.State.Terminal():
			return passDone, nil
		case p.Run.State == eviedb.WorkflowRunWaitingForDependency:
			return passPaused, nil
		case p.Run.State == eviedb.WorkflowRunWaitingForHuman:
			// The owner's answer, recorded under another claim, lets it
			// continue; nothing here resumes it (ADR 0029).
			return passParked, nil
		case p.Run.State == eviedb.WorkflowRunNeedsReconciliation, p.Run.State == eviedb.WorkflowRunCancelling && p.Unresolved > 0:
			// An effect's outcome is unknown (in a cancelling run too,
			// whose cancellation finishes once it is reconciled):
			// reconcile it automatically when that is due, before anything
			// else; otherwise the run waits (for its recheck, or for the
			// owner). When this pass has just found it unknown, it waits
			// out the settle delay under its lease first, so a request
			// still landing is not looked up too early.
			settling := pc.settling
			pc.settling = false
			if settling && p.Run.NotBefore != nil && p.Run.NotBefore.After(r.now()) && pc.stop.Err() == nil {
				if err := r.sleep(pc.stop, p.Run.NotBefore.Sub(r.now())); err != nil {
					return passParked, nil
				}
				continue
			}
			if p.Run.NotBefore == nil || p.Run.NotBefore.After(r.now()) || pc.stop.Err() != nil {
				return passParked, nil
			}
			resolved, err := r.reconcileAutomatically(pc, p)
			if err != nil {
				return 0, err
			}
			if !resolved {
				return passParked, nil
			}
			continue
		case p.InFlight != nil:
			return 0, fmt.Errorf("%w: steps.%s attempt %d is in flight under lease %d", eviedb.ErrWorkflowRunConflict, p.InFlight.Step, p.InFlight.Attempt, p.InFlight.LeaseToken)
		case p.Run.State == eviedb.WorkflowRunCancelling:
			// The owner cancelled it: no further node starts (sub-stage 8).
			if _, err := r.store.FinishWorkflowCancellation(pc.writes, pc.lease); err != nil {
				return 0, err
			}
			return passDone, nil
		case p.Run.NotBefore != nil && p.Run.NotBefore.After(r.now()):
			return passWaiting, nil
		case pc.stop.Err() != nil:
			// Stopping: only a run with work left is released as such.
			return passStopped, nil
		}
		if err := r.step(pc, p); err != nil {
			var capped *eviedb.WorkflowDailyCapError
			switch {
			case errors.As(err, &capped):
				// Today's cap refused the AI call or worker start before
				// anything was recorded or sent: the run pauses visibly
				// until the next UTC day (sub-stage 11).
				if _, err := r.store.PauseWorkflowRun(pc.writes, pc.lease, capped.Error(), r.untilReset(capped)); err != nil {
					if errors.Is(err, eviedb.ErrWorkflowRunCancelling) {
						continue
					}
					return 0, err
				}
				return passPaused, nil
			case errors.Is(err, eviedb.ErrWorkflowRunNotDue):
				return passWaiting, nil
			case errors.Is(err, eviedb.ErrWorkflowRunCancelling):
				// Cancelled before the node started: nothing was recorded,
				// and the next round finishes the cancellation.
				continue
			case admissionRefused(err):
				// Deactivated or blocked since the claim: the node did not
				// start, and the run pauses at this boundary (sub-stage 8,
				// ADR 0033).
				if _, err := r.store.PauseWorkflowRun(pc.writes, pc.lease, err.Error(), r.sched.Recheck); err != nil {
					if errors.Is(err, eviedb.ErrWorkflowRunCancelling) {
						continue
					}
					return 0, err
				}
				return passPaused, nil
			}
			return 0, err
		}
	}
}

// step takes the run's next step from its latest checkpoint: one branch
// decision, one deterministic attempt or one capability read, a pause
// before a read that cannot run now, or the run's failure. A node
// interrupted as many times as the interruption bound at this checkpoint
// fails the run instead of running again.
func (r *Runtime) step(pc *pass, p eviedb.WorkflowRunProgress) error {
	fail := func(reason string) error {
		_, err := r.store.FailWorkflowRun(pc.writes, pc.lease, p.Checkpoint.Seq, reason)
		return err
	}
	pause := func(reason string) error {
		_, err := r.store.PauseWorkflowRun(pc.writes, pc.lease, reason, r.sched.Recheck)
		return err
	}
	inputs, err := decodeObject(p.Run.Inputs)
	if err != nil {
		return fail(fmt.Sprintf("the run's recorded input cannot be read: %v", err))
	}
	state, err := decodeObject(p.Checkpoint.State)
	if err != nil {
		return fail(fmt.Sprintf("checkpoint %d cannot be read: %v", p.Checkpoint.Seq, err))
	}
	step, ok := pc.v.Step(p.Checkpoint.Next)
	if !ok {
		return fail(fmt.Sprintf("checkpoint %d names step %q, which the definition does not have", p.Checkpoint.Seq, p.Checkpoint.Next))
	}
	// An effect's attempts that sent an Effect Intent are never safe
	// interruptions: an interrupted one left its outcome unknown and was
	// reconciled, so only the others count toward the bound.
	interrupted := p.Interrupted
	var intents []eviedb.WorkflowEffectIntent
	if step.Kind == workflow.KindCapability && step.Effect {
		if intents, err = r.stepIntents(pc.writes, p.Run.ID, p.Checkpoint.Seq, step.ID); err != nil {
			return err
		}
		for _, i := range intents {
			if i.AttemptState == eviedb.WorkflowAttemptInterrupted {
				interrupted--
			}
		}
	}
	// A map's own attempt does no work of its own: its items are bounded
	// one by one instead (sub-stage 9), so it is not.
	if interrupted >= r.sched.Interruptions && step.Kind != workflow.KindMap {
		return fail(fmt.Sprintf("steps.%s was interrupted %s at checkpoint %d (each time its runtime crashed, stopped or lost its lease before recording the outcome); the run fails instead of running it again: %s",
			step.ID, times(interrupted), p.Checkpoint.Seq, p.LastError))
	}
	switch step.Kind {
	case workflow.KindBranch:
		// A branch decision is pure: one an interruption cut short is
		// simply decided again.
		d, err := pc.v.Branch(step.ID, inputs, state)
		if err != nil {
			return fail(err.Error())
		}
		read, err := workflow.EncodeValue(d.Read)
		if err != nil {
			return err
		}
		a, err := r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{
			RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: read,
		})
		if err != nil {
			return err
		}
		decision, err := json.Marshal(struct {
			Next string `json:"next"`
			Case int    `json:"case"`
		}{d.Next, d.Case})
		if err != nil {
			return err
		}
		_, err = r.store.FinishWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptEnd{
			RunID: p.Run.ID, Seq: a.Seq, Output: decision, Note: d.Note, Next: d.Next, State: p.Checkpoint.State,
		})
		return err
	case workflow.KindDeterministic:
		return r.deterministic(pc, p, step, inputs, state, fail)
	case workflow.KindCapability:
		if step.Effect {
			return r.effect(pc, p, step, inputs, state, intents, fail, pause)
		}
		return r.read(pc, p, step, inputs, state, fail, pause)
	case workflow.KindAI:
		return r.ai(pc, p, step, inputs, state, fail, pause)
	case workflow.KindInterrupt:
		return r.interrupt(pc, p, step, inputs, state, fail)
	case workflow.KindAgent:
		return r.agentStep(pc, p, step, inputs, state, fail, pause)
	case workflow.KindMap:
		return r.mapStep(pc, p, step, inputs, state, fail, pause)
	}
	return fail(fmt.Sprintf("steps.%s is a %s step, which this runtime version cannot run", step.ID, step.Kind))
}

// reasonText renders an optional reason after a colon.
func reasonText(reason string) string {
	if reason == "" {
		return ""
	}
	return ": " + reason
}

func times(n int) string {
	if n == 1 {
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

// limitReached says why the step's next attempt may not start: its Retry
// Policy is spent, or the run reached limits.max_attempts or
// limits.max_duration. Interrupted attempts count against neither.
func (r *Runtime) limitReached(v *workflow.Validated, p eviedb.WorkflowRunProgress, step workflow.StepPlan) string {
	limits := v.Limits()
	switch {
	case p.Attempts >= step.Attempts:
		return fmt.Sprintf("steps.%s failed after %s: %s", step.ID, attempts(step.Attempts), p.LastError)
	case p.Counted >= limits.Attempts:
		return fmt.Sprintf("steps.%s: the run reached limits.max_attempts %d", step.ID, limits.Attempts)
	case r.elapsed(p) > limits.Duration:
		return fmt.Sprintf("steps.%s: the run exceeded limits.max_duration %s", step.ID, limits.Duration)
	}
	return ""
}

// elapsed is the run's time counted against limits.max_duration: since
// its creation, less the time it waited for the owner's answers to its
// questions (sub-stage 7) and the time it was paused at the operator's
// daily cap (sub-stage 11). Time waiting for an effect's reconciliation
// still counts, as sub-stage 6 decided.
func (r *Runtime) elapsed(p eviedb.WorkflowRunProgress) time.Duration {
	return r.now().Sub(p.Run.CreatedAt) - p.HumanWait - p.CapWait
}

// deterministic makes one attempt of a deterministic step within its Retry
// Policy and the run's limits. A failed attempt that leaves attempts
// records the step's backoff as the run's durable not-before time; the
// retry starts from the same checkpoint with the same input once it passes,
// in whichever process claims the run.
func (r *Runtime) deterministic(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, fail func(string) error) error {
	if reason := r.limitReached(pc.v, p, step); reason != "" {
		return fail(reason)
	}
	input, err := pc.v.StepInput(step.ID, inputs, state)
	if err != nil {
		return fail(err.Error())
	}
	encoded, err := workflow.EncodeValue(input)
	if err != nil {
		return err
	}
	a, err := r.store.BeginWorkflowAttempt(pc.writes, pc.lease, eviedb.WorkflowAttemptStart{
		RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: encoded,
	})
	if err != nil {
		return err
	}
	var out any
	var callErr error
	if r.call != nil {
		out, callErr = r.call(p.Run.ID, pc.v, step.ID, input)
	} else {
		out, callErr = pc.v.CallFunction(step.ID, input)
	}
	var output []byte
	if callErr == nil {
		if output, err = workflow.EncodeValue(out); err != nil {
			callErr = fmt.Errorf("its output cannot be recorded: %v", err)
		}
	}
	if callErr != nil && callErr.Error() == "" {
		callErr = errors.New("the function failed without a message")
	}
	return r.settle(pc, p, step, a.Seq, inputs, state, out, output, callErr)
}

// settle records how an attempt ended: its output with the next checkpoint
// (and the run's outputs when the graph ends), or its failure, retried
// after the step's backoff, a durable not-before time, while its Retry
// Policy has attempts left and failing the run otherwise.
func (r *Runtime) settle(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, seq int64, inputs, state map[string]any, out any, output []byte, failure error) error {
	if failure != nil {
		end := eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: seq, Error: failure.Error()}
		if p.Attempts+1 >= step.Attempts {
			end.RunError = fmt.Sprintf("steps.%s failed after %s: %s", step.ID, attempts(step.Attempts), end.Error)
		} else {
			end.RetryAfter = step.Backoff
		}
		_, err := r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
		return err
	}
	end, err := successEnd(pc, p, step, seq, inputs, state, out, output)
	if err != nil {
		return err
	}
	_, err = r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
	return err
}

// successEnd is a succeeded attempt's end: its output with the next
// checkpoint, holding every step output so far, and the run's outputs when
// the graph ends.
func successEnd(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, seq int64, inputs, state map[string]any, out any, output []byte) (eviedb.WorkflowAttemptEnd, error) {
	next := make(map[string]any, len(state)+1)
	for k, v := range state {
		next[k] = v
	}
	next[step.ID] = out
	nextState, err := workflow.EncodeValue(next)
	if err != nil {
		return eviedb.WorkflowAttemptEnd{}, err
	}
	end := eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: seq, Output: output, Next: step.Next, State: nextState}
	if step.Next == "" {
		if end.Outputs, err = workflow.EncodeValue(pc.v.Outputs(inputs, next)); err != nil {
			return eviedb.WorkflowAttemptEnd{}, err
		}
	}
	return end, nil
}

func decodeObject(data json.RawMessage) (map[string]any, error) {
	v, err := workflow.DecodeValue(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("not a JSON object")
	}
	return m, nil
}

// InspectRun reads one run with every attempt and checkpoint, its lease and
// its recovery events.
func (r *Runtime) InspectRun(ctx context.Context, id string) (eviedb.WorkflowRunDetail, error) {
	return r.store.WorkflowRun(ctx, id)
}

// ListRuns lists runs, newest first.
func (r *Runtime) ListRuns(ctx context.Context, f RunFilter) ([]eviedb.WorkflowRun, error) {
	return r.store.ListWorkflowRuns(ctx, eviedb.WorkflowRunFilter{WorkspaceID: f.WorkspaceID, Workflow: f.Workflow, Limit: f.Limit})
}

// valuesLine renders a JSON object's fields as name=value, sorted.
func valuesLine(data json.RawMessage) string {
	m, err := decodeObject(data)
	if err != nil {
		return string(data)
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	slices.Sort(names)
	parts := make([]string, len(names))
	for i, name := range names {
		value, _ := workflow.EncodeValue(m[name])
		parts[i] = name + "=" + string(value)
	}
	return strings.Join(parts, ", ")
}

const timeLayout = "2006-01-02 15:04:05Z07:00"

// RunLine is one run in a listing.
func RunLine(run eviedb.WorkflowRun) string {
	line := fmt.Sprintf("%s  %s  %s v%d  %s  %s", run.ID, run.WorkspaceID, run.Workflow, run.Version, run.State, run.CreatedAt.Format(timeLayout))
	if !run.State.Terminal() && run.State != eviedb.WorkflowRunCancelling && run.NextStep != "" {
		line += "  next " + run.NextStep
	}
	if run.Error != "" {
		line += "  (" + run.Error + ")"
	}
	return line
}

// RunText renders a run, its attempts and outputs, its lease and its
// recovery events for the terminal.
func RunText(d eviedb.WorkflowRunDetail) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	run := d.Run
	line("Run %s: %s version %d in Workspace %s, %s", run.ID, run.Workflow, run.Version, run.WorkspaceID, run.State)
	line("Run key: %s", run.RunKey)
	if o := d.Override; o != nil {
		line("Rerun of %s by the %s via %s at %s (logical Run Key %s): %s", o.RerunOf, o.Actor, o.Surface, o.RecordedAt.Format(timeLayout), o.LogicalKey, o.Reason)
	}
	line("Pinned: commit %s, executable %s, authority %s, Workspace revision %s, access revision %d",
		short(run.Commit), run.ExecutableHash, run.AuthorityHash, run.WorkspaceRevision, run.AccessRevision)
	var pinned composition
	if json.Unmarshal(run.Composition, &pinned) == nil && len(pinned.Providers) > 0 {
		line("Providers: %s", providersLine(pinned))
	}
	for _, m := range pinned.Models {
		temperature := "provider default"
		if m.Temperature != nil {
			temperature = fmt.Sprint(*m.Temperature)
		}
		line("Model Policy: %s v%d, %s %s, temperature %s, at most %s output tokens", m.Name, m.Version, m.Provider, m.Model, temperature, grouped(m.MaxOutputTokens))
	}
	for _, s := range d.Substitutions {
		line("Substituted: %s %s %s -> %s (contract %s, same schema), approved by %s via %s at %s",
			s.Capability, s.Plugin, s.PinnedVersion, s.ReplacementVersion, s.Contract, s.Approver, s.Surface, s.ApprovedAt.Format(timeLayout))
	}
	started := fmt.Sprintf("Started via %s at %s", run.Surface, run.CreatedAt.Format(timeLayout))
	switch {
	case run.Surface == "chat":
		// Sub-stage 10: Evie started it with workflow_start in the owner's chat.
		started = fmt.Sprintf("Started by Evie in the owner's chat (workflow_start) at %s", run.CreatedAt.Format(timeLayout))
	case run.Surface == "schedule" && d.Slot != nil:
		// Sub-stage 11: its schedule started it for one slot.
		shifted := ""
		if d.Slot.Shifted {
			shifted = ", run when the clocks changed because that time did not exist that day"
		}
		started = fmt.Sprintf("Started by its schedule for %s %s (due %s%s) at %s", d.Slot.Key, d.Slot.Timezone, d.Slot.Due.Format(timeLayout), shifted,
			run.CreatedAt.Format(timeLayout))
	}
	if run.SessionID != "" {
		started += " from session " + string(run.SessionID)
	}
	if run.FinishedAt != nil {
		started += "; finished at " + run.FinishedAt.Format(timeLayout)
	}
	line("%s", started)
	line("Inputs: %s", valuesLine(run.Inputs))
	if c := d.Cancellation; c != nil {
		label := "Cancellation requested"
		if run.State == eviedb.WorkflowRunCancelled {
			label = "Cancelled"
		}
		line("%s by the %s via %s at %s, while the run was %s%s", label, c.Actor, c.Surface, c.RequestedAt.Format(timeLayout), c.FromState, reasonText(c.Reason))
	}
	switch {
	case run.State == eviedb.WorkflowRunCancelled:
	case run.State == eviedb.WorkflowRunCancelling:
		line("Cancelling: %s", run.Error)
	case run.State == eviedb.WorkflowRunWaitingForHuman:
		line("Waiting for your answer: no Evie continues it until you answer (see Interruptions)")
	case run.State == eviedb.WorkflowRunWaitingForDependency:
		paused := "Paused: " + run.Error
		if run.NotBefore != nil {
			paused += fmt.Sprintf(" (checked again after %s)", run.NotBefore.Format(timeLayout))
		}
		line("%s", paused)
	case run.State == eviedb.WorkflowRunNeedsReconciliation:
		needs := "Needs reconciliation: " + run.Error
		if run.NotBefore != nil {
			needs += fmt.Sprintf(" (automatic reconciliation after %s)", run.NotBefore.Format(timeLayout))
		}
		line("%s", needs)
	case run.Error != "":
		line("Error: %s", run.Error)
	}
	if run.NotBefore != nil && run.State != eviedb.WorkflowRunWaitingForDependency && run.State != eviedb.WorkflowRunNeedsReconciliation &&
		run.State != eviedb.WorkflowRunCancelling && !run.State.Terminal() {
		line("Retry: not before %s", run.NotBefore.Format(timeLayout))
	}
	if l := d.Lease; l != nil {
		if l.ReleasedAt != nil {
			line("Lease: released by %s (lease %d) at %s; last heartbeat %s", l.Holder, l.Token, l.ReleasedAt.Format(timeLayout), l.HeartbeatAt.Format(timeLayout))
		} else {
			line("Lease: held by %s (lease %d) until %s; last heartbeat %s", l.Holder, l.Token, l.ExpiresAt.Format(timeLayout), l.HeartbeatAt.Format(timeLayout))
		}
	} else if !run.State.Terminal() {
		line("Lease: not claimed yet; a running Evie (evie serve or the REPL) executes it")
	}
	if len(d.Attempts) > 0 {
		line("Nodes")
	}
	for _, a := range d.Attempts {
		text := fmt.Sprintf("  #%d %s attempt %d: %s", a.Seq, a.Step, a.Attempt, a.State)
		if a.Item != nil {
			text = fmt.Sprintf("  #%d %s item %d attempt %d: %s", a.Seq, a.Step, *a.Item, a.Attempt, a.State)
		}
		if a.Next != "" {
			text += " -> " + a.Next
		}
		text += fmt.Sprintf(" (from checkpoint %d)", a.InputCheckpoint)
		for _, detail := range []string{a.Error, a.Note} {
			if detail != "" {
				text += " - " + detail
			}
		}
		if a.State == eviedb.WorkflowAttemptRunning {
			text += " - in flight"
		}
		line("%s", text)
		line("     input %s", a.Input)
		if a.Kind != workflow.KindBranch && len(a.Output) > 0 {
			line("     output %s", a.Output)
		}
	}
	effectsText(&b, d.Effects)
	interruptionsText(&b, d.Interruptions)
	modelCallsText(&b, d.ModelCalls)
	mapsText(&b, d)
	workersText(&b, d.Workers)
	if len(d.Workers) > 0 && len(d.ModelCalls) > 0 {
		var tokens, micro int64
		for _, c := range d.ModelCalls {
			tokens, micro = tokens+c.Tokens(), micro+c.Cost()
		}
		for _, c := range d.Workers {
			tokens, micro = tokens+c.Tokens(), micro+c.Cost()
		}
		line("Run spend: %s tokens, %s (AI steps and workers)", grouped(tokens), dollars(micro))
	}
	switch {
	case run.State == eviedb.WorkflowRunCancelling:
		line("No further node starts: the cancellation finishes from checkpoint %d", run.Checkpoint)
	case !run.State.Terminal():
		line("Next: %s from checkpoint %d", run.NextStep, run.Checkpoint)
	}
	if len(run.Outputs) > 0 {
		m, err := decodeObject(run.Outputs)
		if err == nil && len(m) > 0 {
			line("Outputs")
			names := make([]string, 0, len(m))
			for name := range m {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				value, _ := workflow.EncodeValue(m[name])
				line("  %s: %s", name, value)
			}
		}
	}
	if len(d.Events) > 0 {
		line("Recovery")
		for _, e := range d.Events {
			line("  %s %s by %s (lease %d): %s", e.At.Format(timeLayout), e.Kind, e.Holder, e.Token, e.Detail)
		}
	}
	return b.String()
}
