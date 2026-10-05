package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// Session starts (Stage 14 sub-stage 10, 2026-10-04; workflow-runtime.spec.md
// user story 18 and "Manual/session start is the first delivery slice").
//
// The Workflows plugin's workflow_start and workflow_status reach the
// runtime through the three methods below, which implement
// plugins.WorkflowSessions. A chat starts only a run of its own Workspace's
// ACTIVE workflow: the Workspace and session come from the invoking turn,
// and StartRun applies every gate an owner's start has (the active
// version's approved commit re-read and re-hashed, the step kinds this
// runtime runs, EVIE_WORKFLOW_REMOTE_MODELS for AI steps and workers, the
// workers' availability, the declared inputs and the Run Key), then the
// store proves the chat's live turn and committed workflow_start call in
// the transaction that records the run with origin "chat". The run is
// detached: the scheduler of `evie serve` or the REPL advances it, and the
// chat reads it with workflow_status. A chat cannot propose, approve,
// deactivate, cancel, answer, reconcile, substitute or rerun: those
// methods accept only the owner's CLI and web surfaces.

var _ plugins.WorkflowSessions = (*Runtime)(nil)

// errNotInWorkspace refuses a chat outside a Workspace: workflows belong to
// Workspaces.
var errNotInWorkspace = errors.New("workflows belong to Workspaces, and this conversation is not in one, so it cannot start or read workflow runs")

// maxChatError bounds the run error a chat sees.
const maxChatError = 2000

// StartFromChat starts a run of the chat's Workspace's active workflow.
func (r *Runtime) StartFromChat(ctx context.Context, req plugins.WorkflowChatStart) (plugins.WorkflowChatRun, error) {
	if req.Scope.WorkspaceID == "" {
		return plugins.WorkflowChatRun{}, errNotInWorkspace
	}
	if strings.TrimSpace(req.Workflow) == "" {
		return plugins.WorkflowChatRun{}, errors.New("name a workflow to start")
	}
	res, err := r.startRun(ctx, RunRequest{WorkspaceID: req.Scope.WorkspaceID, Workflow: strings.TrimSpace(req.Workflow), Inputs: req.Inputs,
		SessionID: req.Scope.SessionID, Surface: "chat",
		Chat: &eviedb.WorkflowChatOrigin{Lease: req.Lease, SourceEventID: req.SourceEventID, IntentEventID: req.IntentEventID}})
	if err != nil {
		return plugins.WorkflowChatRun{}, r.chatStartRefusal(ctx, req.Scope.WorkspaceID, strings.TrimSpace(req.Workflow), err)
	}
	run, err := r.chatRun(ctx, res.Run, false)
	run.Existing = res.Existing
	return run, err
}

// StartNeedsError is a chat start refused for settings only David can
// change (2026-10-04): every one the workflow needs in this Workspace, so
// the model tells him all of them at once instead of one per attempt.
type StartNeedsError struct {
	Workflow string
	Needs    []string
	Cause    error
}

func (e *StartNeedsError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s cannot start in this Workspace yet; nothing was started. Tell David exactly what to change, then start it again:", e.Workflow)
	for i, need := range e.Needs {
		fmt.Fprintf(&b, " (%d) %s;", i+1, need)
	}
	return strings.TrimSuffix(b.String(), ";") + "."
}

func (e *StartNeedsError) Unwrap() error { return e.Cause }

// chatStartRefusal turns a start refused for David's settings (remote
// models, the Subagents plugin, the Workspace's worker allowance, a
// disabled built-in) into one StartNeedsError listing every setting the
// workflow needs here; other errors are returned as they are.
func (r *Runtime) chatStartRefusal(ctx context.Context, ws memory.WorkspaceID, name string, cause error) error {
	var models *ModelsUnavailableError
	var agents *AgentsUnavailableError
	var blocked *eviedb.WorkflowBlockedError
	if !errors.As(cause, &models) && !errors.As(cause, &agents) && !errors.As(cause, &blocked) && !errors.Is(cause, eviedb.ErrWorkflowBuiltinDisabled) {
		return cause
	}
	needs, err := r.startNeeds(ctx, ws, name)
	if err != nil || len(needs) == 0 {
		return cause
	}
	return &StartNeedsError{Workflow: name, Needs: needs, Cause: cause}
}

// startNeeds lists the owner's settings a start of name in the Workspace
// lacks now.
func (r *Runtime) startNeeds(ctx context.Context, ws memory.WorkspaceID, name string) ([]string, error) {
	var needs []string
	res, err := r.store.ResolveWorkflow(ctx, ws, name)
	var blocked *eviedb.WorkflowBlockedError
	if err != nil && !errors.As(err, &blocked) {
		return nil, err
	}
	var v *workflow.Validated
	switch {
	case res.Source == eviedb.WorkflowSourceBuiltin && r.isBuiltin(name):
		if !res.BuiltinEnabled {
			needs = append(needs, fmt.Sprintf("enable the built-in %s (evie workflows builtin enable %s, or its switch on the Workspace page)", name, name))
		}
		if v, err = r.builtin(name); err != nil {
			return nil, err
		}
	case res.Activation != nil:
		access, err := r.store.WorkspaceAccess(ctx, ws)
		if err != nil {
			return nil, err
		}
		if v, err = r.approvedDefinition(ctx, *res.Activation, access.Access); err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	if hasAISteps(v) && r.models == nil {
		needs = append(needs, r.modelsOff)
	}
	workers := workflow.RequirementsOf(v.Report()).Workers
	if hasAgentSteps(v) {
		if r.agents == nil {
			needs = append(needs, r.agentsOff)
		} else {
			for _, worker := range workers {
				if err := r.agents.RunWorkerAvailable(ctx, worker); err != nil {
					needs = append(needs, fmt.Sprintf("turn on the Subagents plugin (evie plugins enable subagents) so %s workers can run: %v", worker, err))
					break
				}
			}
		}
	}
	switch {
	case blocked != nil:
		needs = append(needs, blocked.Reasons...)
	case res.Source == eviedb.WorkflowSourceBuiltin:
		reasons, err := r.store.WorkspaceWorkerBlocks(ctx, ws, workers)
		if err != nil {
			return nil, err
		}
		needs = append(needs, reasons...)
	}
	return needs, nil
}

// ChatRunStatus reads one run of the chat's Workspace, with its outputs
// once it succeeded. A run of another Workspace is not found.
func (r *Runtime) ChatRunStatus(ctx context.Context, scope memory.ScopeContext, runID string) (plugins.WorkflowChatRun, error) {
	if scope.WorkspaceID == "" {
		return plugins.WorkflowChatRun{}, errNotInWorkspace
	}
	d, err := r.store.WorkflowRun(ctx, runID)
	if err != nil {
		return plugins.WorkflowChatRun{}, err
	}
	if d.Run.WorkspaceID != scope.WorkspaceID {
		return plugins.WorkflowChatRun{}, fmt.Errorf("%w: %s in this Workspace", eviedb.ErrWorkflowRunNotFound, runID)
	}
	return r.chatRun(ctx, d, true)
}

// ChatWorkflows lists the workflows the chat's Workspace can start, each
// by what a new run of its name would pin there (the Workspace's custom
// version, the Global one or the built-in, 2026-10-04), with the inputs
// it declares; a built-in or Global one the Workspace cannot run yet is
// listed with what David must enable. It also lists the latest runs the
// chat started.
func (r *Runtime) ChatWorkflows(ctx context.Context, scope memory.ScopeContext) (plugins.WorkflowChatCatalog, error) {
	if scope.WorkspaceID == "" {
		return plugins.WorkflowChatCatalog{}, errNotInWorkspace
	}
	defs, err := r.store.ListWorkflowDefinitions(ctx, scope.WorkspaceID)
	if err != nil {
		return plugins.WorkflowChatCatalog{}, err
	}
	globals, err := r.store.ListGlobalWorkflowDefinitions(ctx)
	if err != nil {
		return plugins.WorkflowChatCatalog{}, err
	}
	names := []string{}
	for _, d := range defs {
		names = append(names, d.Name)
	}
	for _, d := range globals {
		names = append(names, d.Name)
	}
	for _, b := range r.builtinSet.list() {
		names = append(names, b.Name)
	}
	slices.Sort(names)
	out := plugins.WorkflowChatCatalog{Workflows: []plugins.WorkflowChatDefinition{}}
	for _, name := range slices.Compact(names) {
		def, ok, err := r.chatDefinition(ctx, scope.WorkspaceID, name)
		if err != nil {
			return out, err
		}
		if ok {
			out.Workflows = append(out.Workflows, def)
		}
	}
	runs, err := r.store.ListWorkflowRuns(ctx, eviedb.WorkflowRunFilter{WorkspaceID: scope.WorkspaceID, Limit: 200})
	if err != nil {
		return out, err
	}
	for _, run := range runs {
		if run.SessionID != scope.SessionID || len(out.Runs) == 10 {
			continue
		}
		view, err := r.chatRun(ctx, eviedb.WorkflowRunDetail{Run: run}, false)
		if err != nil {
			return out, err
		}
		out.Runs = append(out.Runs, view)
	}
	// Sub-stage 11: the unread notices of the runs this chat started; the
	// owner's read state is never changed by a chat.
	if scope.SessionID != "" {
		notes, err := r.store.ListWorkflowNotifications(ctx, eviedb.WorkflowNotificationFilter{WorkspaceID: scope.WorkspaceID, SessionID: scope.SessionID, Limit: 10})
		if err != nil {
			return out, err
		}
		for _, n := range notes {
			out.Notifications = append(out.Notifications, plugins.WorkflowChatNotification{RunID: n.RunID, Workflow: n.Workflow, Kind: string(n.Kind), At: n.CreatedAt})
		}
	}
	return out, nil
}

// chatDefinition is one name as a chat may start it in the Workspace, or
// false when nothing of that name can start there: a Workspace version
// that is not active, a disabled built-in, or neither. A Global or
// built-in version the Workspace's allowance does not cover is listed with
// what to enable.
func (r *Runtime) chatDefinition(ctx context.Context, ws memory.WorkspaceID, name string) (plugins.WorkflowChatDefinition, bool, error) {
	def := plugins.WorkflowChatDefinition{Name: name, Inputs: []string{}}
	res, err := r.store.ResolveWorkflow(ctx, ws, name)
	var blocked *eviedb.WorkflowBlockedError
	switch {
	case errors.As(err, &blocked) && res.Source == eviedb.WorkflowSourceGlobal:
		def.Unavailable = strings.Join(blocked.Reasons, "; ")
	case err != nil && (errors.As(err, &blocked) || admissionRefused(err)):
		return def, false, nil
	case err != nil:
		return def, false, err
	}
	def.Source = string(res.Source)
	var report workflow.Report
	switch res.Source {
	case eviedb.WorkflowSourceWorkspace:
		p, err := r.store.WorkflowProposal(ctx, res.Activation.ProposalID)
		if err != nil {
			return def, false, err
		}
		report, def.Version = p.Report, res.Activation.Version
	case eviedb.WorkflowSourceGlobal:
		p, err := r.store.GlobalWorkflowProposal(ctx, res.Activation.ProposalID)
		if err != nil {
			return def, false, err
		}
		report, def.Version = p.Report, res.Activation.Version
	default:
		if !res.BuiltinEnabled || !r.isBuiltin(name) {
			return def, false, nil
		}
		v, err := r.builtin(name)
		if err != nil {
			def.Unavailable = err.Error()
			return def, true, nil
		}
		report = v.Report()
		if state, err := r.store.WorkflowBuiltinState(ctx, name); err != nil {
			return def, false, err
		} else if i := slices.IndexFunc(state.Versions, func(b eviedb.WorkflowBuiltinVersion) bool { return b.ExecutableHash == v.Hash() }); i >= 0 {
			def.Version = state.Versions[i].Version
		}
		reasons, err := r.store.WorkspaceWorkerBlocks(ctx, ws, workflow.RequirementsOf(report).Workers)
		if err != nil {
			return def, false, err
		}
		def.Unavailable = strings.Join(reasons, "; ")
	}
	def.Description, def.MaxTokens = report.Description, report.Limits.MaxTokens
	for _, in := range report.Inputs {
		def.Inputs = append(def.Inputs, in.Name+": "+in.Summary)
	}
	return def, true, nil
}

// WorkerReport is one run-owned worker's full report, which since
// sub-stage 10 no step output carries: the report store keeps it by
// execution.
type WorkerReport struct {
	RunID       string `json:"run_id"`
	ExecutionID string `json:"execution_id"`
	Step        string `json:"step"`
	Item        *int   `json:"item,omitempty"`
	Worker      string `json:"worker"`
	Status      string `json:"status"`
	Report      string `json:"report"`
}

// WorkerReport reads the full report of a worker execution of the run, for
// the owner's `evie workflows report`.
func (r *Runtime) WorkerReport(ctx context.Context, runID, executionID string) (WorkerReport, error) {
	a, report, err := r.store.RunWorkerExecution(ctx, runID, executionID)
	if err != nil {
		return WorkerReport{}, err
	}
	out := WorkerReport{RunID: runID, ExecutionID: a.ID, Worker: a.Worker(), Status: a.State, Report: report}
	if a.Run != nil {
		out.Step, out.Item = a.Run.Step, a.Run.Item
	}
	return out, nil
}

// chatRun is a run as a chat sees it: its state and pins, each step's
// progress (a map's items counted), what it waits for, its build branches,
// its spend against limits.max_tokens and, when asked, its outputs.
func (r *Runtime) chatRun(ctx context.Context, d eviedb.WorkflowRunDetail, outputs bool) (plugins.WorkflowChatRun, error) {
	run := d.Run
	// A run's sensitive inputs never reach a chat (sub-stage 12).
	scrub := r.inputScrubber(ctx, run)
	view := plugins.WorkflowChatRun{RunID: run.ID, Workflow: run.Workflow, Version: run.Version, State: string(run.State), StartedFrom: run.Surface,
		SessionID: string(run.SessionID), CreatedAt: run.CreatedAt, FinishedAt: run.FinishedAt, Error: clipText(scrub.Text(run.Error), maxChatError)}
	if !run.State.Terminal() {
		view.NextStep = run.NextStep
	}
	if report, err := r.pinnedReport(ctx, run); err == nil {
		view.MaxTokens = report.Limits.MaxTokens
	} else {
		return view, err
	}
	spend, err := r.store.WorkflowRunSpend(ctx, run.ID)
	if err != nil {
		return view, err
	}
	view.Spend = plugins.WorkflowChatSpend{Tokens: spend.Tokens, MicroUSD: spend.MicroUSD}
	view.Steps = chatSteps(d)
	for _, i := range d.Interruptions {
		if i.Resolution != nil {
			continue
		}
		switch i.Kind {
		case eviedb.WorkflowInterruptionQuestion:
			view.WaitingFor = fmt.Sprintf("David's answer at steps.%s: %s (he answers with evie workflows answer or on the Workspace page)", i.Step, i.Message)
		default:
			view.WaitingFor = fmt.Sprintf("David's reconciliation of the effect at steps.%s, whose outcome is unknown (evie workflows reconcile)", i.Step)
		}
	}
	for _, w := range d.Workers {
		if w.Worker == workflow.WorkerBuild && w.ExecutionID != "" {
			view.Builds = append(view.Builds, plugins.WorkflowChatBuild{Step: w.Step, Item: w.Item, ExecutionID: w.ExecutionID, Status: w.Status})
		}
	}
	if outputs && run.State == eviedb.WorkflowRunSucceeded {
		view.OutputsJSON = string(scrub.Outputs(run.Outputs))
	}
	return view, nil
}

// chatSteps is each step's latest state in the order the run reached it;
// a map step counts its items at its latest checkpoint.
func chatSteps(d eviedb.WorkflowRunDetail) []plugins.WorkflowChatStep {
	var order []string
	latest := map[string]eviedb.WorkflowNodeAttempt{}
	for _, a := range d.Attempts {
		if a.Item != nil {
			continue
		}
		if _, seen := latest[a.Step]; !seen {
			order = append(order, a.Step)
		}
		latest[a.Step] = a
	}
	maps := map[string]MapView{}
	for _, m := range MapViews(d) {
		maps[m.Step] = m
	}
	out := []plugins.WorkflowChatStep{}
	for _, step := range order {
		a := latest[step]
		s := plugins.WorkflowChatStep{Step: step, Kind: a.Kind, State: string(a.State)}
		if m, ok := maps[step]; ok {
			s.Items = len(m.Items)
			for _, it := range m.Items {
				switch it.State {
				case string(eviedb.WorkflowAttemptSucceeded):
					s.Succeeded++
				case string(eviedb.WorkflowAttemptFailed):
					s.Failed++
				case string(eviedb.WorkflowAttemptRunning):
					s.Running++
				}
			}
		}
		out = append(out, s)
	}
	return out
}

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
