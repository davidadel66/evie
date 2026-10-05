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
		return plugins.WorkflowChatRun{}, err
	}
	run, err := r.chatRun(ctx, res.Run, false)
	run.Existing = res.Existing
	return run, err
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

// ChatWorkflows lists the chat's Workspace's active workflows, with the
// inputs each declares, and the latest runs the chat started.
func (r *Runtime) ChatWorkflows(ctx context.Context, scope memory.ScopeContext) (plugins.WorkflowChatCatalog, error) {
	if scope.WorkspaceID == "" {
		return plugins.WorkflowChatCatalog{}, errNotInWorkspace
	}
	defs, err := r.store.ListWorkflowDefinitions(ctx, scope.WorkspaceID)
	if err != nil {
		return plugins.WorkflowChatCatalog{}, err
	}
	out := plugins.WorkflowChatCatalog{Workflows: []plugins.WorkflowChatDefinition{}}
	for _, d := range defs {
		if d.Status != eviedb.WorkflowActive || d.Current == nil {
			continue
		}
		p, err := r.store.WorkflowProposal(ctx, d.Current.ProposalID)
		if err != nil {
			return out, err
		}
		def := plugins.WorkflowChatDefinition{Name: d.Name, Version: d.Current.Version, Description: p.Report.Description, Inputs: []string{}, MaxTokens: p.Report.Limits.MaxTokens}
		for _, in := range p.Report.Inputs {
			def.Inputs = append(def.Inputs, in.Name+": "+in.Summary)
		}
		out.Workflows = append(out.Workflows, def)
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
	if report, err := r.runReport(ctx, run); err == nil {
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

// runReport is the review report of the version a run pinned.
func (r *Runtime) runReport(ctx context.Context, run eviedb.WorkflowRun) (workflow.Report, error) {
	state, err := r.store.WorkflowDefinition(ctx, run.WorkspaceID, run.Workflow)
	if err != nil {
		return workflow.Report{}, err
	}
	i := slices.IndexFunc(state.Versions, func(a eviedb.WorkflowActivation) bool { return a.Version == run.Version })
	if i < 0 {
		return workflow.Report{}, fmt.Errorf("version %d of %s is not recorded", run.Version, run.Workflow)
	}
	p, err := r.store.WorkflowProposal(ctx, state.Versions[i].ProposalID)
	if err != nil {
		return workflow.Report{}, err
	}
	return p.Report, nil
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
