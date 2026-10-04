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

// Manual deterministic runs (Stage 14 sub-stage 3, 2026-10-04).
//
// A run starts only from the definition's active approval: the runtime reads
// the approved commit through procedural Git, re-validates and re-hashes it
// against the approval, refuses step kinds this runtime cannot execute yet,
// accepts the input against the declared inputs and derives the Run Key;
// then one SQLite transaction applies run admission
// (authorizeWorkflowActivation), returns the run already holding the key,
// or records the run with its pins and first checkpoint. The run then
// executes in the caller's process, foreground, node by node: each attempt
// is recorded before it runs and its output is recorded with the next
// checkpoint before another node starts. Every decision is read back from
// SQLite, so the loop continues from the last checkpoint; fenced leases,
// the scheduler and restart recovery arrive in sub-stage 4.

// RuntimeVersion names this runtime's execution semantics in each run's
// Execution Composition (ADR 0016, 0025).
const RuntimeVersion = "evie.workflow.runtime/1"

// executableKinds are the step kinds this runtime version executes.
var executableKinds = []string{workflow.KindBranch, workflow.KindDeterministic}

var (
	// ErrUnsupportedSteps refuses a definition with steps this runtime
	// version cannot execute; nothing is recorded or run.
	ErrUnsupportedSteps = errors.New("workflow steps not yet supported in this runtime version")
	// ErrDefinitionMismatch refuses a run whose approved commit no longer
	// validates or hashes to its approval.
	ErrDefinitionMismatch = errors.New("the approved workflow definition no longer matches its approval")
	// ErrRunInput refuses input that does not match the declared inputs.
	ErrRunInput = errors.New("invalid workflow run input")
	// ErrRunInterrupted is a run whose last attempt never finished: a crash
	// interrupted it, and only restart recovery (sub-stage 4) continues it.
	ErrRunInterrupted = errors.New("the workflow run was interrupted inside a node; restart recovery arrives in sub-stage 4")
)

// UnsupportedStepsError names the step kinds a definition uses that this
// runtime version cannot execute.
type UnsupportedStepsError struct {
	Workflow string
	Kinds    []string
}

func (e *UnsupportedStepsError) Error() string {
	return fmt.Sprintf("workflow %s uses %s steps, not yet supported in this runtime version (it runs deterministic and branch steps); no run was started",
		e.Workflow, strings.Join(e.Kinds, ", "))
}

func (e *UnsupportedStepsError) Unwrap() error { return ErrUnsupportedSteps }

// RunRequest is the owner starting a run of a Workspace's active workflow.
// Inputs are JSON values; TextInputs (from a command line) are converted to
// each input's declared type. SessionID optionally names the originating
// session, which must belong to the Workspace.
type RunRequest struct {
	WorkspaceID memory.WorkspaceID         `json:"workspace_id"`
	Workflow    string                     `json:"name"`
	Inputs      map[string]json.RawMessage `json:"inputs,omitempty"`
	TextInputs  map[string]string          `json:"-"`
	SessionID   memory.SessionID           `json:"session_id,omitempty"`
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

// composition is a run's Execution Composition in this sub-stage: the
// runtime version and what it executes. Capability providers join it in
// sub-stage 5.
type composition struct {
	Runtime   string   `json:"runtime"`
	StepKinds []string `json:"step_kinds"`
	Functions []string `json:"functions"`
}

type preparedRun struct {
	v     *workflow.Validated
	start eviedb.WorkflowRunStart
}

// StartRun starts a run of the Workspace's active workflow and executes it
// in this process until it ends, or returns the run that already holds its
// Run Key without executing anything. Only the owner's CLI and web surfaces
// start runs. The run does not belong to the caller's connection: a caller
// that disconnects does not stop it.
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
		detached := context.WithoutCancel(ctx)
		if !existing {
			if err := r.advance(detached, p.v, run.ID); err != nil {
				return RunResult{}, fmt.Errorf("workflow run %s: %w", run.ID, err)
			}
		}
		detail, err := r.store.WorkflowRun(detached, run.ID)
		return RunResult{Run: detail, Existing: existing}, err
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
	if len(unsupported) > 0 {
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
	pinned, err := json.Marshal(composition{Runtime: RuntimeVersion, StepKinds: kinds, Functions: v.Functions()})
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

// advance executes a run from its latest durable checkpoint until it ends.
// Each step reads the run's progress back from SQLite, so nothing about the
// run lives only in memory between nodes.
func (r *Runtime) advance(ctx context.Context, v *workflow.Validated, runID string) error {
	for {
		p, err := r.store.WorkflowRunProgress(ctx, runID)
		if err != nil {
			return err
		}
		if p.Run.State.Terminal() {
			return nil
		}
		if p.InFlight != nil {
			return fmt.Errorf("%w (steps.%s attempt %d)", ErrRunInterrupted, p.InFlight.Step, p.InFlight.Attempt)
		}
		if err := r.step(ctx, v, p); err != nil {
			return err
		}
	}
}

// step takes the run's next step from its latest checkpoint: one branch
// decision or one deterministic attempt, or the run's failure.
func (r *Runtime) step(ctx context.Context, v *workflow.Validated, p eviedb.WorkflowRunProgress) error {
	fail := func(reason string) error {
		_, err := r.store.FailWorkflowRun(ctx, p.Run.ID, p.Checkpoint.Seq, reason)
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
	step, ok := v.Step(p.Checkpoint.Next)
	if !ok {
		return fail(fmt.Sprintf("checkpoint %d names step %q, which the definition does not have", p.Checkpoint.Seq, p.Checkpoint.Next))
	}
	switch step.Kind {
	case workflow.KindBranch:
		d, err := v.Branch(step.ID, inputs, state)
		if err != nil {
			return fail(err.Error())
		}
		read, err := workflow.EncodeValue(d.Read)
		if err != nil {
			return err
		}
		a, err := r.store.BeginWorkflowAttempt(ctx, eviedb.WorkflowAttemptStart{
			RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Attempts + 1, Input: read,
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
		_, err = r.store.FinishWorkflowAttempt(ctx, eviedb.WorkflowAttemptEnd{
			RunID: p.Run.ID, Seq: a.Seq, Output: decision, Note: d.Note, Next: d.Next, State: p.Checkpoint.State,
		})
		return err
	case workflow.KindDeterministic:
		return r.deterministic(ctx, v, p, step, inputs, state, fail)
	}
	return fail(fmt.Sprintf("steps.%s is a %s step, which this runtime version cannot run", step.ID, step.Kind))
}

// deterministic makes one attempt of a deterministic step within its Retry
// Policy and the run's limits. A failed attempt that leaves attempts waits
// the step's backoff; the next pass retries from the same checkpoint.
func (r *Runtime) deterministic(ctx context.Context, v *workflow.Validated, p eviedb.WorkflowRunProgress, step workflow.StepPlan,
	inputs, state map[string]any, fail func(string) error) error {
	limits := v.Limits()
	switch {
	case p.Attempts >= step.Attempts:
		return fail(fmt.Sprintf("steps.%s failed after %s: %s", step.ID, attempts(step.Attempts), p.LastError))
	case p.Counted >= limits.Attempts:
		return fail(fmt.Sprintf("steps.%s: the run reached limits.max_attempts %d", step.ID, limits.Attempts))
	case r.now().Sub(p.Run.CreatedAt) > limits.Duration:
		return fail(fmt.Sprintf("steps.%s: the run exceeded limits.max_duration %s", step.ID, limits.Duration))
	}
	input, err := v.StepInput(step.ID, inputs, state)
	if err != nil {
		return fail(err.Error())
	}
	encoded, err := workflow.EncodeValue(input)
	if err != nil {
		return err
	}
	a, err := r.store.BeginWorkflowAttempt(ctx, eviedb.WorkflowAttemptStart{
		RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Attempts + 1, Input: encoded,
	})
	if err != nil {
		return err
	}
	call := r.call
	if call == nil {
		call = (*workflow.Validated).CallFunction
	}
	out, callErr := call(v, step.ID, input)
	var output []byte
	if callErr == nil {
		if output, err = workflow.EncodeValue(out); err != nil {
			callErr = fmt.Errorf("its output cannot be recorded: %v", err)
		}
	}
	if callErr != nil {
		end := eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: a.Seq, Error: callErr.Error()}
		if end.Error == "" {
			end.Error = "the function failed without a message"
		}
		if a.Attempt >= step.Attempts {
			end.RunError = fmt.Sprintf("steps.%s failed after %s: %s", step.ID, attempts(step.Attempts), end.Error)
		}
		if _, err := r.store.FinishWorkflowAttempt(ctx, end); err != nil {
			return err
		}
		if end.RunError == "" && step.Backoff > 0 {
			return r.sleep(ctx, step.Backoff)
		}
		return nil
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
	end := eviedb.WorkflowAttemptEnd{RunID: p.Run.ID, Seq: a.Seq, Output: output, Next: step.Next, State: nextState}
	if step.Next == "" {
		if end.Outputs, err = workflow.EncodeValue(v.Outputs(inputs, next)); err != nil {
			return err
		}
	}
	_, err = r.store.FinishWorkflowAttempt(ctx, end)
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

// InspectRun reads one run with every attempt and checkpoint.
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

// RunText renders a run, its attempts and outputs for the terminal.
func RunText(d eviedb.WorkflowRunDetail) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	run := d.Run
	line("Run %s: %s version %d in Workspace %s, %s", run.ID, run.Workflow, run.Version, run.WorkspaceID, run.State)
	line("Run key: %s", run.RunKey)
	line("Pinned: commit %s, executable %s, authority %s, Workspace revision %s, access revision %d",
		short(run.Commit), run.ExecutableHash, run.AuthorityHash, run.WorkspaceRevision, run.AccessRevision)
	started := fmt.Sprintf("Started via %s at %s", run.Surface, run.CreatedAt.Format(timeLayout))
	if run.SessionID != "" {
		started += " from session " + string(run.SessionID)
	}
	if run.FinishedAt != nil {
		started += "; finished at " + run.FinishedAt.Format(timeLayout)
	}
	line("%s", started)
	line("Inputs: %s", valuesLine(run.Inputs))
	if run.Error != "" {
		line("Error: %s", run.Error)
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
	return b.String()
}
