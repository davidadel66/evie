package web

import (
	"context"
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
// the browser names.
type WorkflowManager interface {
	List(context.Context, memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error)
	Show(context.Context, memory.WorkspaceID, string) (eviedb.WorkflowDefinitionState, error)
	Card(context.Context, string) (workflowruntime.Card, error)
	Approve(context.Context, workflowruntime.Approval) (eviedb.WorkflowActivation, error)
	Deactivate(context.Context, workflowruntime.Deactivation) (eviedb.WorkflowActivation, error)
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
	case errors.Is(err, eviedb.ErrWorkflowApprover):
		status, code, message = http.StatusForbidden, "owner_only", err.Error()
	}
	managementJSONError(w, status, code, message)
}
