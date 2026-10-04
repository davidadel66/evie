package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflow"
)

// Agent steps (Stage 14 sub-stage 9, 2026-10-04; workflow-runtime.spec.md
// amendment, ADR 0010, 0016, 0019, 0025, 0027).
//
// An agent step runs one existing worker preset (research, explore or
// build) through the Kernel's subagent supervisor with the run's fenced
// lease and the node attempt in place of a parent turn. Its assignment is
// the step's reviewed objective prompt with its declared input as framed
// assignment data; its model is the step's pinned Model Policy; its budget
// is the step's, no larger than the operator's subagent policy and the
// run's limits. Each attempt is recorded with its worker call (worker,
// policy, budget, price) under the lease before the worker is admitted,
// and its outcome, usage and cost with the attempt's end. The worker's
// result is the step's output (status, summary, report, fetched sources,
// limitations, tokens); when the step declares an output schema, the
// typed result is the report's last "## Result" json block, validated like
// AI output. succeeded and partial (a wrap-up report) succeed; a worker
// that ended failed, cancelled or interrupted, and a typed result missing
// or outside the schema, fail the attempt, which retries as a fresh worker
// under a fresh budget within the step's Retry Policy.
//
// An attempt cut short by a crash, a stop or a lost lease is recorded
// interrupted and runs again from the same checkpoint within the per-node
// interruption bound. What runs again depends on what the interrupted
// worker left: a worker that finished with a report is not run again (the
// new attempt takes its result); an interrupted build worker is continued
// in its kept worktree and branch, on its own child session, under the new
// attempt's authority; any other worker is started afresh, which is safe
// for research and explore. The owner's cancellation and a lost lease stop
// a worker in flight through its context, and the worker's own authority
// ends independently once a durable check sees the run cancelling, its
// lease or node attempt gone, or a revocation.

// Agents runs agent steps' workers: the Kernel subagent supervisor in
// production.
type Agents interface {
	// RunWorker admits, runs and settles one run-owned worker attempt and
	// returns its result and full report. An admission refusal is an error
	// and nothing ran.
	RunWorker(ctx context.Context, req delegation.RunAssignment) (delegation.RunOutcome, error)
	// RunWorkerAvailable reports whether a worker of the preset could be
	// admitted now (delegation.ErrUnavailable otherwise).
	RunWorkerAvailable(ctx context.Context, worker string) error
	// RunWorkerPin is the digest of the preset's composition now, which a
	// run pins at start.
	RunWorkerPin(ctx context.Context, worker string) (string, error)
	// RunCapacity is how many workers one run may run at once.
	RunCapacity() int
}

var (
	// ErrAgentsUnavailable refuses a start of a workflow with agent steps
	// while this Evie cannot run workers; nothing is recorded.
	ErrAgentsUnavailable = errors.New("workflow agent steps are unavailable")
)

// AgentsUnavailableError names the workflow and why its agent steps
// cannot run here.
type AgentsUnavailableError struct {
	Workflow string
	Reason   string
}

func (e *AgentsUnavailableError) Error() string {
	return fmt.Sprintf("workflow %s runs agent steps, which are unavailable: %s; no run was started", e.Workflow, e.Reason)
}

func (e *AgentsUnavailableError) Unwrap() error { return ErrAgentsUnavailable }

// pinnedWorker is a worker preset a run's agent steps run, with the digest
// of its composition when the run started (ADR 0016).
type pinnedWorker struct {
	Worker string `json:"worker"`
	Digest string `json:"digest"`
}

// hasAgentSteps reports whether the definition runs workers.
func hasAgentSteps(v *workflow.Validated) bool {
	return slices.Contains(v.StepKinds(), workflow.KindAgent)
}

// agentsPause is why a run waits before an agent step in an Evie that
// cannot run its worker.
func (r *Runtime) agentsPause(step, reason string) string {
	return fmt.Sprintf("steps.%s: %s; the run continues by itself once an Evie that can run its workers claims it", step, reason)
}

// pinWorkers resolves each worker preset the definition runs and pins its
// composition's digest.
func (r *Runtime) pinWorkers(ctx context.Context, v *workflow.Validated) ([]pinnedWorker, error) {
	if !hasAgentSteps(v) {
		return nil, nil
	}
	if r.agents == nil {
		return nil, &AgentsUnavailableError{Workflow: v.Name(), Reason: r.agentsOff}
	}
	var out []pinnedWorker
	for _, worker := range workflow.RequirementsOf(v.Report()).Workers {
		// Like an unavailable capability provider (sub-stage 5), a worker
		// that cannot run now refuses the start, recording nothing.
		if err := r.agents.RunWorkerAvailable(ctx, worker); err != nil {
			return nil, &AgentsUnavailableError{Workflow: v.Name(), Reason: fmt.Sprintf("its %s workers cannot run now: %v", worker, err)}
		}
		digest, err := r.agents.RunWorkerPin(ctx, worker)
		if err != nil {
			return nil, &AgentsUnavailableError{Workflow: v.Name(), Reason: fmt.Sprintf("the %s worker cannot be composed: %v", worker, err)}
		}
		out = append(out, pinnedWorker{Worker: worker, Digest: digest})
	}
	return out, nil
}

// pinnedDigest is the digest the run pinned for worker ("" when the run
// pinned none).
func (c composition) pinnedDigest(worker string) string {
	for _, w := range c.Workers {
		if w.Worker == worker {
			return w.Digest
		}
	}
	return ""
}

// workerPlan is what an agent attempt does: take the finished result of a
// worker an interrupted attempt left (adopt), continue an interrupted
// build worker in its kept worktree (continueFrom), or start a fresh one.
type workerPlan struct {
	adopt        *delegation.Attempt
	adoptReport  string
	continueFrom string
}

// planWorker reads what the step's (or the item's) latest attempt at this
// checkpoint left. Only an interrupted attempt's worker is taken over: a
// failed attempt's retry is a fresh worker under a fresh budget.
func (r *Runtime) planWorker(pc *pass, runID string, checkpoint int64, step string, item *int, worker string) (workerPlan, error) {
	calls, err := r.store.WorkflowStepWorkerCalls(pc.writes, runID, checkpoint, step, item)
	if err != nil || len(calls) == 0 {
		return workerPlan{}, err
	}
	latest := calls[0]
	if latest.AttemptState != eviedb.WorkflowAttemptInterrupted {
		return workerPlan{}, nil
	}
	id := latest.ExecutionID
	if latest.AdoptedFrom != "" {
		id = latest.AdoptedFrom
	}
	if id == "" {
		return workerPlan{}, nil
	}
	a, report, err := r.store.RunWorkerExecution(pc.writes, runID, id)
	if err != nil {
		return workerPlan{}, err
	}
	switch {
	case (a.State == "succeeded" || a.State == delegation.StatePartial) && a.FinalEventID != "" && a.Result != nil:
		return workerPlan{adopt: &a, adoptReport: report}, nil
	case worker == delegation.WorkerBuild && (a.State == "interrupted" || a.State == "cancelled") && a.StartedAt != nil && a.Worktree != nil && a.Worktree.RemovedAt == nil:
		return workerPlan{continueFrom: a.ID}, nil
	}
	return workerPlan{}, nil
}

// workerCallStart is an agent attempt's worker call.
func (r *Runtime) workerCallStart(ctx context.Context, spec workflow.AgentPlan, plan workerPlan) eviedb.WorkflowWorkerCallStart {
	pricing := fallbackPricing
	if r.models != nil {
		pricing = r.models.Pricing(ctx, spec.Policy.Model)
	}
	w := eviedb.WorkflowWorkerCallStart{Worker: spec.Worker, Policy: spec.Policy.Name, PolicyVersion: spec.Policy.Version, Provider: spec.Policy.Provider,
		Model: spec.Policy.Model, ObjectiveSHA256: spec.ObjectiveSHA256, SchemaSHA256: spec.SchemaSHA256, Budget: spec.Budget, BudgetTokens: spec.Tokens,
		InputPerMillion: pricing.InputPerMillion, OutputPerMillion: pricing.OutputPerMillion, PricingSource: pricing.Source,
		EstimatedMicroUSD: pricing.MicroUSD(0, spec.Tokens), Continues: plan.continueFrom, Cap: r.cap}
	if plan.adopt != nil {
		w.AdoptedFrom = plan.adopt.ID
	}
	return w
}

// agentOutcome is how one agent attempt's worker ended, as the runtime
// records it: the attempt's output (value and its canonical JSON, with a
// note) or failure, or its interruption (with a pause reason when the
// worker could not be admitted for a reason that clears by itself), the
// worker call's end and the tokens it counts, or a lost lease.
type agentOutcome struct {
	value       any
	output      []byte
	note        string
	failure     error
	interrupted string
	pause       string
	end         eviedb.WorkflowWorkerCallEnd
	counted     int64
	lost        bool
}

// runAgent runs (or adopts) one agent attempt's worker. ctx ends the
// worker as its parent's cancellation does: a map ends it for a failed
// item; the run's lease, the owner's cancellation and the runtime
// stopping end it through the pass.
func (r *Runtime) runAgent(pc *pass, ctx context.Context, run eviedb.WorkflowRun, a eviedb.WorkflowNodeAttempt, spec workflow.AgentPlan, input map[string]any, plan workerPlan) agentOutcome {
	if plan.adopt != nil {
		prior := plan.adopt
		end := eviedb.WorkerCallEndOf(*prior)
		end.Reason = "took the result of worker execution " + prior.ID + ", which finished before an interrupted attempt recorded it"
		o := agentOutcome{end: end}
		adopted := *prior.Result
		if adopted.ExecutionID == "" {
			adopted.ExecutionID = prior.ID
		}
		o.value, o.output, o.note, o.failure = r.agentResult(pc, spec, adopted, plan.adoptReport, tokensOf(*prior, spec))
		o.note = joinNotes("adopted worker execution "+prior.ID+" (it finished before the runtime recorded it; no worker ran again)", o.note)
		return o
	}
	data, err := workflow.EncodeValue(input)
	if err != nil {
		return agentOutcome{failure: err, end: notAdmitted(err)}
	}
	taskID, _ := input["task_id"].(string)
	owner := delegation.RunOwner{RunID: run.ID, WorkspaceID: run.WorkspaceID, Workflow: run.Workflow, Step: a.Step, Item: a.Item, AttemptSeq: a.Seq,
		Holder: pc.lease.Holder, Token: pc.lease.Token, Policy: spec.Policy.Name, PolicyVersion: spec.Policy.Version, Model: spec.Policy.Model,
		Temperature: spec.Policy.Temperature, MaxOutputTokens: spec.Policy.MaxOutputTokens, ResultSchema: string(spec.Schema)}
	req := delegation.RunAssignment{Owner: owner, Worker: spec.Worker, Objective: string(spec.Objective), Data: string(data), TaskID: taskID,
		Deadline: spec.Budget, TokenBudget: spec.Tokens, ContinueFrom: plan.continueFrom, Preset: pc.comp.pinnedDigest(spec.Worker)}
	// The lease is checked immediately before the worker is admitted: a
	// holder that lost it starts nothing (admission checks it again).
	if pc.lost.Err() != nil {
		return agentOutcome{lost: true}
	}
	if err := r.store.CheckWorkflowRunLease(pc.writes, pc.lease); err != nil {
		return agentOutcome{lost: true}
	}
	workerCtx, cancel := context.WithCancel(ctx)
	stopLost := context.AfterFunc(pc.lost, cancel)
	out, err := r.agents.RunWorker(workerCtx, req)
	stopLost()
	cancel()
	if pc.lost.Err() != nil {
		return agentOutcome{lost: true}
	}
	label := "the worker"
	if out.Result.ExecutionID != "" {
		label = "worker execution " + out.Result.ExecutionID
	}
	if err != nil && out.Result.ExecutionID == "" {
		// Nothing ran: the worker was not admitted.
		o := agentOutcome{end: notAdmitted(err)}
		switch {
		case pc.cancelled():
			o.interrupted = "interrupted: the owner cancelled the run before its worker was admitted"
		case pc.stop.Err() != nil:
			o.interrupted = "interrupted: the runtime stopped before its worker was admitted; the step runs again from the same checkpoint"
		case errors.Is(err, delegation.ErrUnavailable) || errors.Is(err, delegation.ErrWorkerChanged) || errors.Is(err, delegation.ErrAuthority):
			o.interrupted = "interrupted: the worker was not admitted: " + clip(err.Error())
			o.pause = fmt.Sprintf("steps.%s: its %s worker was not admitted: %s", a.Step, spec.Worker, clip(err.Error()))
		default:
			o.failure = fmt.Errorf("the worker was not admitted: %s", clip(err.Error()))
		}
		return o
	}
	result := out.Result
	end := eviedb.WorkerCallEndOf(delegation.Attempt{State: result.Status, Result: &result})
	o := agentOutcome{end: end}
	ended := result.Status != "succeeded" && result.Status != delegation.StatePartial
	switch {
	case err != nil:
		o.failure = fmt.Errorf("%s could not be settled: %s", label, clip(err.Error()))
		return o
	case ended && pc.cancelled():
		o.interrupted = fmt.Sprintf("interrupted: the owner cancelled the run while the worker ran; %s ended %s%s", label, result.Status, reasonText(result.Reason))
		return o
	case ended && pc.stop.Err() != nil:
		o.interrupted = fmt.Sprintf("interrupted: the runtime stopped while the worker ran; %s ended %s%s and the step runs again from the same checkpoint", label, result.Status, reasonText(result.Reason))
		return o
	case ended && ctx.Err() != nil:
		o.interrupted = fmt.Sprintf("interrupted: %s; %s ended %s%s", context.Cause(ctx), label, result.Status, reasonText(result.Reason))
		return o
	}
	o.value, o.output, o.note, o.failure = r.agentResult(pc, spec, result, out.Report, tokensOf(delegation.Attempt{State: result.Status, Result: &result}, spec))
	if ended && o.failure != nil {
		o.failure = fmt.Errorf("%s ended %s%s", label, result.Status, reasonText(result.Reason))
	}
	return o
}

// tokensOf is what a worker's attempt counts, as the store records it.
func tokensOf(a delegation.Attempt, spec workflow.AgentPlan) int64 {
	counted, _ := eviedb.WorkerCallCounted(eviedb.WorkerCallEndOf(a), spec.Tokens, false)
	return counted
}

// notAdmitted is the worker call's end when no worker ran.
func notAdmitted(err error) eviedb.WorkflowWorkerCallEnd {
	return eviedb.WorkflowWorkerCallEnd{Status: "not_admitted", Reason: clip(err.Error())}
}

// agentResult turns a worker's result and full report into the step's
// output: the sources it fetched (each marked cited or not), and the typed
// result when the step declares a schema.
func (r *Runtime) agentResult(pc *pass, spec workflow.AgentPlan, result delegation.Result, report string, tokens int64) (any, []byte, string, error) {
	// Since sub-stage 10 the output names the execution whose report the
	// report store keeps instead of carrying it; a run pinned to /5 keeps
	// the shape it started with.
	wr := workflow.WorkerResult{Status: result.Status, Summary: result.Summary, Report: report, ExecutionID: result.ExecutionID,
		FullReport: pc.comp.Runtime == fullReportRuntime, Limitations: result.Limitations, Tokens: tokens}
	for _, src := range result.Sources {
		if src.Fetched {
			wr.Sources = append(wr.Sources, workflow.WorkerSource{URL: src.URL, Cited: src.Cited})
		}
	}
	value, note, failure := pc.v.AgentOutput(spec.ID, wr)
	if failure != nil {
		return nil, nil, note, failure
	}
	output, err := workflow.EncodeValue(value)
	if err != nil {
		return nil, nil, note, fmt.Errorf("its output cannot be recorded: %v", err)
	}
	return value, output, note, nil
}

func joinNotes(notes ...string) string {
	var kept []string
	for _, n := range notes {
		if n != "" {
			kept = append(kept, n)
		}
	}
	return strings.Join(kept, "; ")
}

// agentStep makes one attempt of an agent step within its Retry Policy and
// the run's limits.
func (r *Runtime) agentStep(pc *pass, p eviedb.WorkflowRunProgress, step workflow.StepPlan, inputs, state map[string]any, fail, pause func(string) error) error {
	if reason := r.limitReached(pc.v, p, step); reason != "" {
		return fail(reason)
	}
	spec, ok := pc.v.AgentStep(step.ID)
	if !ok {
		return fail(fmt.Sprintf("steps.%s is not an agent step this runtime can run", step.ID))
	}
	if r.agents == nil {
		return pause(r.agentsPause(step.ID, r.agentsOff))
	}
	input, err := pc.v.StepInput(step.ID, inputs, state)
	if err != nil {
		return fail(err.Error())
	}
	if spec.Worker == workflow.WorkerBuild {
		if id, _ := input["task_id"].(string); strings.TrimSpace(id) == "" {
			return fail(fmt.Sprintf("steps.%s: a build worker needs its task_id input to name a Task", step.ID))
		}
	}
	plan, err := r.planWorker(pc, p.Run.ID, p.Checkpoint.Seq, step.ID, nil, spec.Worker)
	if err != nil {
		return err
	}
	if plan.adopt == nil {
		if err := r.agents.RunWorkerAvailable(pc.stop, spec.Worker); err != nil {
			return pause(r.agentsPause(step.ID, fmt.Sprintf("its %s worker cannot run now: %v", spec.Worker, err)))
		}
		limit := pc.v.Limits().Tokens
		if p.SpentTokens()+spec.Tokens > limit {
			return fail(fmt.Sprintf("steps.%s: its next attempt could use up to %s tokens (its budget), and the run has used %s of limits.max_tokens %s; no worker was started",
				step.ID, grouped(spec.Tokens), grouped(p.SpentTokens()), grouped(limit)))
		}
	}
	encoded, err := workflow.EncodeValue(input)
	if err != nil {
		return err
	}
	start := r.workerCallStart(pc.stop, spec, plan)
	start.Attempt = eviedb.WorkflowAttemptStart{RunID: p.Run.ID, Checkpoint: p.Checkpoint.Seq, Step: step.ID, Kind: step.Kind, Attempt: p.Made + 1, Input: encoded}
	a, err := r.store.BeginWorkflowWorkerCall(pc.writes, pc.lease, start)
	if err != nil {
		return err
	}
	calls, stopWatch := r.cancellable(pc, pc.stop)
	o := r.runAgent(pc, calls, p.Run, a, spec, input, plan)
	stopWatch()
	if o.lost {
		return fmt.Errorf("%w: lost while steps.%s's worker ran", eviedb.ErrWorkflowRunLeaseLost, step.ID)
	}
	end := o.end
	end.RunID, end.AttemptSeq = p.Run.ID, a.Seq
	if o.interrupted != "" {
		if err := r.store.InterruptWorkflowWorkerCall(pc.writes, pc.lease, end, o.interrupted); err != nil {
			return err
		}
		if o.pause != "" {
			_, err := r.store.PauseWorkflowRun(pc.writes, pc.lease, o.pause, r.sched.Recheck)
			return err
		}
		return nil
	}
	counted, _ := eviedb.WorkerCallCounted(end, spec.Tokens, plan.adopt != nil)
	limit := pc.v.Limits().Tokens
	switch used := p.SpentTokens() + counted; {
	case used > limit:
		end.Attempt = eviedb.WorkflowAttemptEnd{Error: fmt.Sprintf("the run exceeded limits.max_tokens %s", grouped(limit)), Note: o.note,
			RunError: fmt.Sprintf("steps.%s: the run exceeded limits.max_tokens %s: its AI steps and workers used %s tokens", step.ID, grouped(limit), grouped(used))}
	case o.failure != nil:
		end.Attempt = eviedb.WorkflowAttemptEnd{Error: o.failure.Error(), Note: o.note}
		if p.Attempts+1 >= step.Attempts {
			end.Attempt.RunError = fmt.Sprintf("steps.%s failed after %s: %s", step.ID, attempts(step.Attempts), o.failure.Error())
		} else {
			end.Attempt.RetryAfter = step.Backoff
		}
	default:
		if end.Attempt, err = successEnd(pc, p, step, a.Seq, inputs, state, o.value, o.output); err != nil {
			return err
		}
		end.Attempt.Note = o.note
	}
	_, err = r.store.FinishWorkflowWorkerCall(pc.writes, pc.lease, end)
	return err
}

// workersText renders a run's worker calls: each agent attempt's worker,
// the hidden child session and execution it ran as, its outcome and
// spend.
func workersText(b *strings.Builder, calls []eviedb.WorkflowWorkerCall) {
	if len(calls) == 0 {
		return
	}
	line := func(format string, args ...any) { fmt.Fprintf(b, format+"\n", args...) }
	line("Workers")
	var tokens, micro int64
	for _, c := range calls {
		tokens, micro = tokens+c.Tokens(), micro+c.Cost()
		label := "steps." + c.Step
		if c.Item != nil {
			label += fmt.Sprintf(" item %d", *c.Item)
		}
		text := fmt.Sprintf("  %s attempt #%d: %s worker, %s v%d %s, budget %s / %s tokens", label, c.AttemptSeq, c.Worker, c.Policy, c.PolicyVersion, c.Model,
			formatDuration(c.BudgetMS), grouped(c.BudgetTokens))
		if c.ExecutionID != "" {
			text += fmt.Sprintf("; execution %s, child session %s", c.ExecutionID, c.ChildSessionID)
		}
		if c.AdoptedFrom != "" {
			text += "; took the result of execution " + c.AdoptedFrom
		}
		if c.Continues != "" {
			text += "; continued interrupted execution " + c.Continues + " in its worktree"
		}
		line("%s", text)
		switch {
		case c.FinishedAt == nil:
			line("     in flight: counted at its budget of %s tokens, %s", grouped(c.BudgetTokens), dollars(c.EstimatedMicroUSD))
		default:
			usage := "reported"
			if !c.UsageComplete {
				usage = "incomplete, counted at least at its budget"
			}
			outcome := c.Status + reasonText(c.Reason)
			line("     %s; %s in / %s out tokens (%s), counted %s, %s (%s prices)", outcome, grouped(c.InputTokens), grouped(c.OutputTokens), usage,
				grouped(c.CountedTokens), dollars(c.MicroUSD), c.PricingSource)
		}
	}
	line("Worker spend: %s tokens, %s", grouped(tokens), dollars(micro))
}

// formatDuration renders a budget in milliseconds briefly.
func formatDuration(ms int64) string {
	switch {
	case ms%60000 == 0:
		return fmt.Sprintf("%dm", ms/60000)
	case ms%1000 == 0:
		return fmt.Sprintf("%ds", ms/1000)
	}
	return fmt.Sprintf("%dms", ms)
}
