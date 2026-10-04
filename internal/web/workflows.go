package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// WorkflowManager is the Workflow Runtime as the owner's web management API
// uses it (Stage 14 sub-stage 2): list and show definitions, present a
// proposal's approval card, approve exactly what it showed, and deactivate
// a version. Proposals come from the CLI; these routes never read a path
// the browser names. Sub-stage 3 adds starting, listing and inspecting
// runs.
type WorkflowManager interface {
	List(context.Context, memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error)
	Show(context.Context, memory.WorkspaceID, string) (eviedb.WorkflowDefinitionState, error)
	Card(context.Context, string) (workflowruntime.Card, error)
	Approve(context.Context, workflowruntime.Approval) (eviedb.WorkflowActivation, error)
	Deactivate(context.Context, workflowruntime.Deactivation) (eviedb.WorkflowActivation, error)
	StartRun(context.Context, workflowruntime.RunRequest) (workflowruntime.RunResult, error)
	InspectRun(context.Context, string) (eviedb.WorkflowRunDetail, error)
	ListRuns(context.Context, workflowruntime.RunFilter) ([]eviedb.WorkflowRun, error)
}

// WithWorkflows adds the workflow management routes.
func WithWorkflows(server *Server, workflows WorkflowManager) *Server {
	server.workflows = workflows
	return server
}

func (s *Server) registerWorkflowRoutes(mux *http.ServeMux) {
	if s.workflows == nil {
		return
	}
	mux.Handle("/api/workflows/list", s.managementRoute(s.handleWorkflowList))
	mux.Handle("/api/workflows/show", s.managementRoute(s.handleWorkflowShow))
	mux.Handle("/api/workflows/diff", s.managementRoute(s.handleWorkflowDiff))
	mux.Handle("/api/workflows/approve", s.managementRoute(s.handleWorkflowApprove))
	mux.Handle("/api/workflows/deactivate", s.managementRoute(s.handleWorkflowDeactivate))
	mux.Handle("/api/workflows/start", s.managementRoute(s.handleWorkflowStart))
	mux.Handle("/api/workflows/runs", s.managementRoute(s.handleWorkflowRuns))
	mux.Handle("/api/workflows/inspect", s.managementRoute(s.handleWorkflowInspect))
}

func boundedID(id string) bool { return id != "" && len(id) <= 128 }

func (s *Server) handleWorkflowList(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspace_id"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if len(request.WorkspaceID) > 128 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "workspace_id is too long")
		return
	}
	defs, err := s.workflows.List(r.Context(), request.WorkspaceID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Workflows []eviedb.WorkflowDefinitionState `json:"workflows"`
	}{defs})
}

func (s *Server) handleWorkflowShow(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		Name        string             `json:"name"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(string(request.WorkspaceID)) || !boundedID(request.Name) {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "show requires the Workspace ID and the workflow name")
		return
	}
	d, err := s.workflows.Show(r.Context(), request.WorkspaceID, request.Name)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleWorkflowDiff(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ProposalID string `json:"proposal_id"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(request.ProposalID) {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "diff requires the proposal ID")
		return
	}
	card, err := s.workflows.Card(r.Context(), request.ProposalID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, card)
}

// handleWorkflowApprove is the owner's web approval: it names the exact
// proposal, executable hash, authority hash and reviewed version from the
// card. The approver is always the owner on the web surface; the request
// cannot choose another.
func (s *Server) handleWorkflowApprove(w http.ResponseWriter, r *http.Request) {
	var request workflowruntime.Approval
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(request.ProposalID) || !boundedID(request.ExecutableHash) || !boundedID(request.AuthorityHash) || request.ExpectedVersion < 0 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "approval requires the proposal ID, its exact executable and authority hashes, and the version the review compared with")
		return
	}
	request.Surface, request.Detail = "web", "POST /api/workflows/approve"
	activation, err := s.workflows.Approve(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, activation)
}

func (s *Server) handleWorkflowDeactivate(w http.ResponseWriter, r *http.Request) {
	var request workflowruntime.Deactivation
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(string(request.WorkspaceID)) || !boundedID(request.Name) || request.Version < 1 || len(request.Reason) > 1000 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "deactivation requires the Workspace ID, the workflow name and its current version")
		return
	}
	request.Surface = "web"
	activation, err := s.workflows.Deactivate(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, activation)
}

func writeWorkflowError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusServiceUnavailable, "workflow_retryable", "the workflow request could not be completed; refresh and try again"
	var invalid *workflow.InvalidError
	switch {
	case errors.As(err, &invalid):
		status, code, message = http.StatusUnprocessableEntity, "invalid_definition", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowProposalNotFound), errors.Is(err, eviedb.ErrWorkflowNotFound), errors.Is(err, eviedb.ErrWorkspaceNotFound):
		status, code, message = http.StatusNotFound, "workflow_not_found", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowHashMismatch):
		status, code, message = http.StatusConflict, "hash_mismatch", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowChanged):
		status, code, message = http.StatusConflict, "workflow_changed", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowSimulationRequired):
		status, code, message = http.StatusConflict, "simulation_required", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowAlreadyActive):
		status, code, message = http.StatusConflict, "already_active", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowRefused):
		status, code, message = http.StatusConflict, "approval_refused", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowNotActive):
		status, code, message = http.StatusConflict, "not_active", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowApprover), errors.Is(err, eviedb.ErrWorkflowRunSurface):
		status, code, message = http.StatusForbidden, "owner_only", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowBlocked):
		status, code, message = http.StatusConflict, "blocked", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowRunNotFound):
		status, code, message = http.StatusNotFound, "run_not_found", err.Error()
	case errors.Is(err, workflowruntime.ErrRunInput):
		status, code, message = http.StatusUnprocessableEntity, "invalid_input", err.Error()
	case errors.Is(err, workflowruntime.ErrUnsupportedSteps):
		status, code, message = http.StatusUnprocessableEntity, "unsupported_steps", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowRunSession):
		status, code, message = http.StatusUnprocessableEntity, "invalid_session", err.Error()
	case errors.Is(err, workflowruntime.ErrDefinitionMismatch):
		status, code, message = http.StatusConflict, "definition_mismatch", err.Error()
	case errors.Is(err, workflowruntime.ErrCapabilityUnavailable):
		status, code, message = http.StatusConflict, "capability_unavailable", err.Error()
	}
	managementJSONError(w, status, code, message)
}

// maxWorkflowStartBytes bounds a start request: the run input document (at
// most 64 KiB) and its envelope.
const maxWorkflowStartBytes = 72 << 10

// handleWorkflowStart is the owner starting a run of a Workspace's active
// workflow from the web (Stage 14 sub-stage 3). It names the Workspace, the
// workflow, JSON inputs and optionally its originating session; the surface
// is always the web. The response waits until the run finishes or pauses,
// unless detach is set (sub-stage 4): then it returns the recorded run at
// once and the server's scheduler executes it. Either way the run does not
// stop if the browser disconnects.
func (s *Server) handleWorkflowStart(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		Name        string             `json:"name"`
		Inputs      json.RawMessage    `json:"inputs"`
		SessionID   memory.SessionID   `json:"session_id"`
		Detach      bool               `json:"detach"`
	}
	if !decodeCandidateRequestLimit(w, r, &request, maxWorkflowStartBytes) {
		return
	}
	run := workflowruntime.RunRequest{WorkspaceID: request.WorkspaceID, Workflow: request.Name, SessionID: request.SessionID, Detach: request.Detach, Surface: "web"}
	valid := boundedID(string(request.WorkspaceID)) && boundedID(request.Name) && len(request.SessionID) <= 128
	if valid && len(request.Inputs) > 0 {
		var err error
		run.Inputs, err = workflow.ParseInputs(request.Inputs)
		valid = err == nil
	}
	if !valid {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "start requires the Workspace ID, the workflow name and an inputs object")
		return
	}
	result, err := s.workflows.StartRun(r.Context(), run)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	var request workflowruntime.RunFilter
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if len(request.WorkspaceID) > 128 || len(request.Workflow) > 128 || request.Limit < 0 || request.Limit > 200 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "runs takes an optional Workspace ID, workflow name and a limit up to 200")
		return
	}
	runs, err := s.workflows.ListRuns(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Runs []eviedb.WorkflowRun `json:"runs"`
	}{runs})
}

func (s *Server) handleWorkflowInspect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID string `json:"run_id"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(request.RunID) {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "inspect requires the run ID")
		return
	}
	d, err := s.workflows.InspectRun(r.Context(), request.RunID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
