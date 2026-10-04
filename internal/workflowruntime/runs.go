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
// attempts against a step's Retry Policy.
const RuntimeVersion = "evie.workflow.runtime/2"

// compatibleRuntimes are the pinned runtimes this runtime declares it can
// continue (ADR 0025). Runs pinned to evie.workflow.runtime/1 (sub-stages 3
// and 4) hold only deterministic and branch steps, whose semantics are
// unchanged; the one difference, that an interruption no longer spends one
// of a step's attempts, only lets a pure node run again from the same
// checkpoint and never changes a recorded output. A run pinned to any other
// runtime pauses visibly.
var compatibleRuntimes = []string{"evie.workflow.runtime/1", RuntimeVersion}

// executableKinds are the step kinds this runtime version executes;
// capability steps only when they read (effects are sub-stage 6's).
var executableKinds = []string{workflow.KindBranch, workflow.KindCapability, workflow.KindDeterministic}

// capabilityEffect names capability effect steps among unsupported kinds.
const capabilityEffect = "capability effect"

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
	return fmt.Sprintf("workflow %s uses %s steps, not yet supported in this runtime version (it runs deterministic, branch and capability read steps); no run was started",
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
	if !ownerSurface(req.Surface) {
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
	access, err := r.store.WorkspaceAccess(ctx, req.WorkspaceID)
	if err != nil {
		return preparedRun{}, err
	}
	v, err := r.approvedDefinition(ctx, a, access.Access)
	if err != nil {
		return preparedRun{}, err
	}
	kinds := v.StepKinds()
	var unsupported []string
	for _, kind := range kinds {
		if !slices.Contains(executableKinds, kind) {
			unsupported = append(unsupported, kind)
		}
	}
	for _, s := range v.CapabilitySteps() {
		if s.Effect && !slices.Contains(unsupported, capabilityEffect) {
			unsupported = append(unsupported, capabilityEffect)
		}
	}
	if len(unsupported) > 0 {
		slices.Sort(unsupported)
		return preparedRun{}, &UnsupportedStepsError{Workflow: a.Name, Kinds: unsupported}
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
	pinned, err := json.Marshal(composition{Runtime: RuntimeVersion, StepKinds: kinds, Functions: v.Functions(), Providers: providers})
	if err != nil {
		return preparedRun{}, err
	}
	return preparedRun{v: v, start: eviedb.WorkflowRunStart{
		WorkspaceID: req.WorkspaceID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash,
		Composition: pinned, RunKey: v.RunKey(inputs), Inputs: encoded, Start: v.Start(), Surface: req.Surface, SessionID: req.SessionID,
	}}, nil
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
		if err != nil || settled(d.Run.State) {
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

// settled reports a run state await returns on: finished, paused, or
// waiting for something no scheduler resumes.
func settled(state eviedb.WorkflowRunState) bool {
	return state.Terminal() || state != eviedb.WorkflowRunQueued && state != eviedb.WorkflowRunRunning
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
	lost, markLost := context.WithCancelCause(context.Background())
	defer markLost(nil)
	beat, stopBeat := context.WithCancel(context.Background())
	beating := make(chan struct{})
	go func() {
		defer close(beating)
		r.heartbeat(beat, lease, markLost)
	}()
	end, err := r.continueRun(ctx, writes, lease, lost)
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
func (r *Runtime) continueRun(ctx, writes context.Context, lease eviedb.WorkflowRunLease, lost context.Context) (passEnd, error) {
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
	pinned := eviedb.WorkflowActivation{WorkspaceID: run.WorkspaceID, Name: run.Workflow, Version: run.Version, Commit: run.Commit, Path: run.Path,
		ExecutableHash: run.ExecutableHash, AuthorityHash: run.AuthorityHash}
	v, err := r.approvedDefinition(writes, pinned, access.Access)
	if errors.Is(err, ErrDefinitionMismatch) {
		return pause(err.Error())
	}
	if err != nil {
		return 0, err
	}
	// A run paused for an unavailable provider stays paused, rather than
	// resuming only to pause again before the read, until it is ready.
	if next, ok := v.Step(p.Checkpoint.Next); ok && next.Kind == workflow.KindCapability && p.InFlight == nil && !run.State.Terminal() {
		if pin, ok := comp.pin(next.Capability); ok {
			_, reason, err := r.checkPinned(writes, run.ID, pin, true)
			if err != nil {
				return 0, err
			}
			if reason != "" {
				return pause(fmt.Sprintf("steps.%s: %s", next.ID, reason))
			}
		}
	}
	if _, err := r.store.ResumeWorkflowRun(writes, lease); err != nil {
		if admissionRefused(err) {
			return pause(err.Error())
		}
		return 0, err
	}
	return r.advance(&pass{stop: ctx, writes: writes, lost: lost, lease: lease, v: v, comp: comp})
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
		case p.InFlight != nil:
			return 0, fmt.Errorf("%w: steps.%s attempt %d is in flight under lease %d", eviedb.ErrWorkflowRunConflict, p.InFlight.Step, p.InFlight.Attempt, p.InFlight.LeaseToken)
		case p.Run.NotBefore != nil && p.Run.NotBefore.After(r.now()):
			return passWaiting, nil
		case pc.stop.Err() != nil:
			// Stopping: only a run with work left is released as such.
			return passStopped, nil
		}
		if err := r.step(pc, p); err != nil {
			if errors.Is(err, eviedb.ErrWorkflowRunNotDue) {
				return passWaiting, nil
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
	if p.Interrupted >= r.sched.Interruptions {
		return fail(fmt.Sprintf("steps.%s was interrupted %s at checkpoint %d (each time its runtime crashed, stopped or lost its lease before recording the outcome); the run fails instead of running it again: %s",
			step.ID, times(p.Interrupted), p.Checkpoint.Seq, p.LastError))
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
		return r.read(pc, p, step, inputs, state, fail, pause)
	}
	return fail(fmt.Sprintf("steps.%s is a %s step, which this runtime version cannot run", step.ID, step.Kind))
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
	case r.now().Sub(p.Run.CreatedAt) > limits.Duration:
		return fmt.Sprintf("steps.%s: the run exceeded limits.max_duration %s", step.ID, limits.Duration)
	}
	return ""
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
	next := make(map[string]any, len(state)+1)
	for k, v := range state {
		next[k] = v
	}
	next[step.ID] = out
	nextState, err := workflow.EncodeValue(next)
	if err != nil {
		return err
	}
	end := eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: seq, Output: output, Next: step.Next, State: nextState}
	if step.Next == "" {
		if end.Outputs, err = workflow.EncodeValue(pc.v.Outputs(inputs, next)); err != nil {
			return err
		}
	}
	_, err = r.store.FinishWorkflowAttempt(pc.writes, pc.lease, end)
	return err
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
	if !run.State.Terminal() && run.NextStep != "" {
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
	line("Pinned: commit %s, executable %s, authority %s, Workspace revision %s, access revision %d",
		short(run.Commit), run.ExecutableHash, run.AuthorityHash, run.WorkspaceRevision, run.AccessRevision)
	var pinned composition
	if json.Unmarshal(run.Composition, &pinned) == nil && len(pinned.Providers) > 0 {
		line("Providers: %s", providersLine(pinned))
	}
	for _, s := range d.Substitutions {
		line("Substituted: %s %s %s -> %s (contract %s, same schema), approved by %s via %s at %s",
			s.Capability, s.Plugin, s.PinnedVersion, s.ReplacementVersion, s.Contract, s.Approver, s.Surface, s.ApprovedAt.Format(timeLayout))
	}
	started := fmt.Sprintf("Started via %s at %s", run.Surface, run.CreatedAt.Format(timeLayout))
	if run.SessionID != "" {
		started += " from session " + string(run.SessionID)
	}
	if run.FinishedAt != nil {
		started += "; finished at " + run.FinishedAt.Format(timeLayout)
	}
	line("%s", started)
	line("Inputs: %s", valuesLine(run.Inputs))
	switch {
	case run.State == eviedb.WorkflowRunWaitingForDependency:
		paused := "Paused: " + run.Error
		if run.NotBefore != nil {
			paused += fmt.Sprintf(" (checked again after %s)", run.NotBefore.Format(timeLayout))
		}
		line("%s", paused)
	case run.Error != "":
		line("Error: %s", run.Error)
	}
	if run.NotBefore != nil && run.State != eviedb.WorkflowRunWaitingForDependency && !run.State.Terminal() {
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
	if !run.State.Terminal() {
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
