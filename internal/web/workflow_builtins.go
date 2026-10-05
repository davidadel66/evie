package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Built-in workflows (owner decision 2026-10-04): the workflows built into
// Evie run in every Workspace without a proposal or approval. The owner's
// web management API lists them, with what a run of each would pin in a
// Workspace, and turns one off or on for every Workspace; the surface is
// always the web. Customizing (a proposal from Evie's own files) and
// approving stay on the CLI and the existing approve route.

// handleWorkflowBuiltins lists the built-ins, and for workspace_id what a
// run of each would pin there and why it could not start.
func (s *Server) handleWorkflowBuiltins(w http.ResponseWriter, r *http.Request) {
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
	views, err := s.workflows.Builtins(r.Context(), request.WorkspaceID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Builtins []workflowruntime.BuiltinView `json:"builtins"`
	}{views})
}

// handleWorkflowBuiltinControl turns a built-in off or on for every
// Workspace, with an optional reason.
func (s *Server) handleWorkflowBuiltinControl(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
		Reason  string `json:"reason"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(request.Name) || request.Enabled == nil || len(request.Reason) > 1000 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "a built-in control requires the workflow name and enabled, and takes a reason of at most 1000 bytes")
		return
	}
	control, err := s.workflows.ControlBuiltin(r.Context(), workflowruntime.BuiltinControl{Name: strings.TrimSpace(request.Name), Enabled: *request.Enabled,
		Reason: request.Reason, Surface: "web"})
	switch {
	case errors.Is(err, eviedb.ErrWorkflowBuiltinUnchanged):
		managementJSONError(w, http.StatusConflict, "builtin_unchanged", err.Error())
		return
	case errors.Is(err, workflowruntime.ErrBuiltinUnknown):
		managementJSONError(w, http.StatusNotFound, "workflow_not_found", err.Error())
		return
	case err != nil:
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, control)
}
