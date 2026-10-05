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
// runs; sub-stage 6 the owner's reconciliation of an effect whose outcome
// is unknown; sub-stage 7 answering a run's durable interruptions and
// listing them for a Workspace, the session that started a run, or a run;
// sub-stage 8 cancelling a run and the audited rerun override.
type WorkflowManager interface {
	List(context.Context, memory.WorkspaceID) ([]eviedb.WorkflowDefinitionState, error)
	Show(context.Context, memory.WorkspaceID, string) (eviedb.WorkflowDefinitionState, error)
	Card(context.Context, string) (workflowruntime.Card, error)
	Approve(context.Context, workflowruntime.Approval) (eviedb.WorkflowActivation, error)
	Deactivate(context.Context, workflowruntime.Deactivation) (eviedb.WorkflowActivation, error)
	StartRun(context.Context, workflowruntime.RunRequest) (workflowruntime.RunResult, error)
	InspectRun(context.Context, string) (eviedb.WorkflowRunDetail, error)
	ListRuns(context.Context, workflowruntime.RunFilter) ([]eviedb.WorkflowRun, error)
	Reconcile(context.Context, workflowruntime.Reconciliation) (eviedb.WorkflowRunDetail, error)
	Answer(context.Context, workflowruntime.AnswerRequest) (eviedb.WorkflowRunDetail, error)
	Interruptions(context.Context, workflowruntime.InterruptionFilter) ([]eviedb.WorkflowInterruption, error)
	Cancel(context.Context, workflowruntime.CancelRequest) (eviedb.WorkflowRunDetail, error)
	// Sub-stage 11: schedules and the owner's pause and resume, durable
	// notifications and their read state, and the daily cap's status.
	Schedules(context.Context, memory.WorkspaceID) ([]workflowruntime.ScheduleStatus, error)
	ControlSchedule(context.Context, workflowruntime.ScheduleControl) (eviedb.WorkflowScheduleControl, error)
	Notifications(context.Context, eviedb.WorkflowNotificationFilter) ([]workflowruntime.Notification, error)
	MarkNotifications(context.Context, workflowruntime.NotificationMark) (int, error)
	Status(context.Context, memory.WorkspaceID) (workflowruntime.Status, error)
	// Sub-stage 12: the owner's migration of a run, and the retention
	// sweep and its status.
	Migrate(context.Context, workflowruntime.MigrationRequest) (workflowruntime.MigrationResult, error)
	Sweep(context.Context, workflowruntime.SweepRequest) (workflowruntime.SweepResult, error)
	RetentionStatus(context.Context) (workflowruntime.RetentionStatus, error)
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
	mux.Handle("/api/workflows/reconcile", s.managementRoute(s.handleWorkflowReconcile))
	mux.Handle("/api/workflows/answer", s.managementRoute(s.handleWorkflowAnswer))
	mux.Handle("/api/workflows/interruptions", s.managementRoute(s.handleWorkflowInterruptions))
	mux.Handle("/api/workflows/cancel", s.managementRoute(s.handleWorkflowCancel))
	mux.Handle("/api/workflows/schedules", s.managementRoute(s.handleWorkflowSchedules))
	mux.Handle("/api/workflows/schedule", s.managementRoute(s.handleWorkflowSchedule))
	mux.Handle("/api/workflows/notifications", s.managementRoute(s.handleWorkflowNotifications))
	mux.Handle("/api/workflows/notifications/read", s.managementRoute(s.handleWorkflowNotificationsRead))
	mux.Handle("/api/workflows/status", s.managementRoute(s.handleWorkflowStatus))
	mux.Handle("/api/workflows/migrate", s.managementRoute(s.handleWorkflowMigrate))
	mux.Handle("/api/workflows/retention", s.managementRoute(s.handleWorkflowRetention))
	mux.Handle("/api/workflows/retention/sweep", s.managementRoute(s.handleWorkflowRetentionSweep))
}

// handleWorkflowMigrate is the owner migrating an unfinished run to the
// definition's newer active version from the web (Stage 14 sub-stage 12),
// with an optional reason; the surface is always the web. The response
// waits until the successor finishes or stops, unless detach is set.
func (s *Server) handleWorkflowMigrate(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID     string `json:"run_id"`
		ToVersion int64  `json:"to_version"`
		Reason    string `json:"reason"`
		Detach    bool   `json:"detach"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(request.RunID) || request.ToVersion < 1 || len(request.Reason) > eviedb.MaxWorkflowCancelReason {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "migrate requires the run ID and the version to migrate to, and takes a reason of at most 1000 bytes")
		return
	}
	result, err := s.workflows.Migrate(r.Context(), workflowruntime.MigrationRequest{RunID: request.RunID, ToVersion: request.ToVersion, Reason: request.Reason,
		Detach: request.Detach, Surface: "web"})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleWorkflowRetention is the retention policies, the sweep's state
// and what is due now (sub-stage 12).
func (s *Server) handleWorkflowRetention(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	status, err := s.workflows.RetentionStatus(r.Context())
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleWorkflowRetentionSweep is the owner's retention sweep from the web
// (sub-stage 12), or a dry run of it.
func (s *Server) handleWorkflowRetentionSweep(w http.ResponseWriter, r *http.Request) {
	var request struct {
		DryRun bool `json:"dry_run"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	result, err := s.workflows.Sweep(r.Context(), workflowruntime.SweepRequest{DryRun: request.DryRun, Surface: "web"})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	case errors.Is(err, workflowruntime.ErrReconcileRefused):
		status, code, message = http.StatusConflict, "reconcile_refused", err.Error()
	case errors.Is(err, workflowruntime.ErrAnswerRefused):
		status, code, message = http.StatusConflict, "answer_refused", err.Error()
	case errors.Is(err, workflowruntime.ErrModelsUnavailable):
		status, code, message = http.StatusConflict, "models_unavailable", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowRunFinished):
		status, code, message = http.StatusConflict, "run_finished", err.Error()
	case errors.Is(err, workflowruntime.ErrRerunRefused):
		status, code, message = http.StatusConflict, "rerun_refused", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowScheduleControl):
		status, code, message = http.StatusConflict, "schedule_state", err.Error()
	case errors.Is(err, eviedb.ErrWorkflowScheduleNotFound):
		status, code, message = http.StatusConflict, "no_schedule", err.Error()
	case errors.Is(err, workflowruntime.ErrMigrationRefused), errors.Is(err, eviedb.ErrWorkflowMigrationRefused):
		status, code, message = http.StatusConflict, "migration_refused", err.Error()
	case errors.Is(err, workflowruntime.ErrRetentionBusy):
		status, code, message = http.StatusConflict, "retention_busy", err.Error()
	case errors.Is(err, workflowruntime.ErrRetentionOff):
		status, code, message = http.StatusConflict, "retention_off", err.Error()
	}
	managementJSONError(w, status, code, message)
}

// handleWorkflowSchedules lists the schedules of a Workspace's (or every
// Workspace's) current versions (Stage 14 sub-stage 11).
func (s *Server) handleWorkflowSchedules(w http.ResponseWriter, r *http.Request) {
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
	list, err := s.workflows.Schedules(r.Context(), request.WorkspaceID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Schedules []workflowruntime.ScheduleStatus `json:"schedules"`
	}{list})
}

// handleWorkflowSchedule is the owner pausing or resuming a schedule from
// the web (sub-stage 11); the surface is always the web.
func (s *Server) handleWorkflowSchedule(w http.ResponseWriter, r *http.Request) {
	var request workflowruntime.ScheduleControl
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(string(request.WorkspaceID)) || !boundedID(request.Workflow) || request.Version < 0 || len(request.Reason) > 1000 ||
		request.Action != "pause" && request.Action != "resume" {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "schedule requires the Workspace ID, the workflow name and an action of pause or resume")
		return
	}
	request.Surface = "web"
	control, err := s.workflows.ControlSchedule(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, control)
}

// handleWorkflowNotifications lists durable notifications (sub-stage 11):
// unread ones unless all is set, for a Workspace, the session that started
// a run, or one run, newest first.
func (s *Server) handleWorkflowNotifications(w http.ResponseWriter, r *http.Request) {
	var request eviedb.WorkflowNotificationFilter
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if len(request.WorkspaceID) > 128 || len(request.SessionID) > 128 || len(request.RunID) > 128 || request.Limit < 0 || request.Limit > 200 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "notifications takes an optional Workspace ID, session ID, run ID and a limit up to 200")
		return
	}
	list, err := s.workflows.Notifications(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Notifications []workflowruntime.Notification `json:"notifications"`
	}{list})
}

// handleWorkflowNotificationsRead is the owner marking notifications read
// or unread by ID, or every unread one (of a Workspace) read (sub-stage
// 11).
func (s *Server) handleWorkflowNotificationsRead(w http.ResponseWriter, r *http.Request) {
	var request workflowruntime.NotificationMark
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	valid := len(request.WorkspaceID) <= 128 && len(request.IDs) <= 200 && (request.All && request.Read && len(request.IDs) == 0 || !request.All && len(request.IDs) > 0)
	for _, id := range request.IDs {
		valid = valid && id > 0
	}
	if !valid {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "name 1 to 200 notification IDs with read true or false, or all with read true")
		return
	}
	request.Surface = "web"
	marked, err := s.workflows.MarkNotifications(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Marked int `json:"marked"`
	}{marked})
}

// handleWorkflowStatus is the daily cap with today's spend, the schedules
// and the notification counts (sub-stage 11).
func (s *Server) handleWorkflowStatus(w http.ResponseWriter, r *http.Request) {
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
	status, err := s.workflows.Status(r.Context(), request.WorkspaceID)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// maxWorkflowReconcileBytes bounds a reconciliation request: the effect's
// output (at most 64 KiB), the evidence and the envelope.
const maxWorkflowReconcileBytes = 72 << 10

// handleWorkflowReconcile is the owner reconciling, from the web, an
// effect whose outcome is unknown (Stage 14 sub-stage 6): the run, the
// step, whether the effect happened or did not happen, the evidence and,
// when it happened, its output object. The surface is always the web. The
// response waits until the run finishes or stops again, unless detach is
// set; either way the run does not stop if the browser disconnects.
func (s *Server) handleWorkflowReconcile(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID    string          `json:"run_id"`
		Step     string          `json:"step"`
		Outcome  string          `json:"outcome"`
		Evidence string          `json:"evidence"`
		Output   json.RawMessage `json:"output"`
		Detach   bool            `json:"detach"`
	}
	if !decodeCandidateRequestLimit(w, r, &request, maxWorkflowReconcileBytes) {
		return
	}
	outcomes := request.Outcome == workflowruntime.OutcomeHappened || request.Outcome == workflowruntime.OutcomeDidNotHappen
	if !boundedID(request.RunID) || !boundedID(request.Step) || !outcomes || len(request.Evidence) > workflowruntime.MaxReconcileEvidence {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request",
			"reconcile requires the run ID, the step, an outcome of happened or did-not-happen and evidence of at most 2000 bytes")
		return
	}
	reconciled, err := s.workflows.Reconcile(r.Context(), workflowruntime.Reconciliation{RunID: request.RunID, Step: request.Step, Outcome: request.Outcome,
		Evidence: request.Evidence, Output: request.Output, Detach: request.Detach, Surface: "web"})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reconciled)
}

// maxWorkflowAnswerBytes bounds an answer request: the answer (at most 64
// KiB) and its envelope.
const maxWorkflowAnswerBytes = 72 << 10

// handleWorkflowAnswer is the owner answering, from the web, the
// interruption a run waits for at a step (Stage 14 sub-stage 7): a
// question's typed answer, or an unknown effect's reconciliation. It
// names the run, the step, the answer object and, when answered from a
// session's view, that session (any session of the run's Workspace); the
// surface is always the web. The response waits until the run finishes or
// stops again, unless detach is set; either way the run does not stop if
// the browser disconnects.
func (s *Server) handleWorkflowAnswer(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID     string           `json:"run_id"`
		Step      string           `json:"step"`
		Answer    json.RawMessage  `json:"answer"`
		SessionID memory.SessionID `json:"session_id"`
		Detach    bool             `json:"detach"`
	}
	if !decodeCandidateRequestLimit(w, r, &request, maxWorkflowAnswerBytes) {
		return
	}
	var object map[string]json.RawMessage
	if !boundedID(request.RunID) || !boundedID(request.Step) || len(request.SessionID) > 128 || json.Unmarshal(request.Answer, &object) != nil || object == nil {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "answer requires the run ID, the step and an answer object")
		return
	}
	answered, err := s.workflows.Answer(r.Context(), workflowruntime.AnswerRequest{RunID: request.RunID, Step: request.Step, Answer: request.Answer,
		SessionID: request.SessionID, Detach: request.Detach, Surface: "web"})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, answered)
}

// handleWorkflowInterruptions lists what runs ask the owner (Stage 14
// sub-stage 7): by default the open interruptions, for a Workspace, the
// session that started a run (so that session can show them) or one run.
func (s *Server) handleWorkflowInterruptions(w http.ResponseWriter, r *http.Request) {
	var request workflowruntime.InterruptionFilter
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if len(request.WorkspaceID) > 128 || len(request.SessionID) > 128 || len(request.RunID) > 128 || request.Limit < 0 || request.Limit > 200 {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "interruptions takes an optional Workspace ID, session ID, run ID and a limit up to 200")
		return
	}
	found, err := s.workflows.Interruptions(r.Context(), request)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Interruptions []eviedb.WorkflowInterruption `json:"interruptions"`
	}{found})
}

// handleWorkflowCancel is the owner cancelling a run from the web (Stage
// 14 sub-stage 8), with an optional reason; the surface is always the web.
// The response waits until the run is cancelled, or until it can only
// finish once an effect is reconciled, unless detach is set; either way
// the cancellation stands if the browser disconnects.
func (s *Server) handleWorkflowCancel(w http.ResponseWriter, r *http.Request) {
	var request struct {
		RunID  string `json:"run_id"`
		Reason string `json:"reason"`
		Detach bool   `json:"detach"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if !boundedID(request.RunID) || len(request.Reason) > eviedb.MaxWorkflowCancelReason {
		managementJSONError(w, http.StatusBadRequest, "invalid_workflow_request", "cancel requires the run ID and takes a reason of at most 1000 bytes")
		return
	}
	cancelled, err := s.workflows.Cancel(r.Context(), workflowruntime.CancelRequest{RunID: request.RunID, Reason: request.Reason, Detach: request.Detach, Surface: "web"})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cancelled)
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
// stop if the browser disconnects. Since sub-stage 8 it also takes the
// owner's audited rerun override: rerun_of, a succeeded run, with a reason
// (and that run's input when inputs is omitted).
func (s *Server) handleWorkflowStart(w http.ResponseWriter, r *http.Request) {
	var request struct {
		WorkspaceID memory.WorkspaceID `json:"workspace_id"`
		Name        string             `json:"name"`
		Inputs      json.RawMessage    `json:"inputs"`
		SessionID   memory.SessionID   `json:"session_id"`
		Detach      bool               `json:"detach"`
		RerunOf     string             `json:"rerun_of"`
		Reason      string             `json:"reason"`
	}
	if !decodeCandidateRequestLimit(w, r, &request, maxWorkflowStartBytes) {
		return
	}
	run := workflowruntime.RunRequest{WorkspaceID: request.WorkspaceID, Workflow: request.Name, SessionID: request.SessionID, Detach: request.Detach, Surface: "web",
		RerunOf: request.RerunOf, Reason: request.Reason}
	valid := boundedID(string(request.WorkspaceID)) && boundedID(request.Name) && len(request.SessionID) <= 128 && len(request.RerunOf) <= 128 &&
		len(request.Reason) <= eviedb.MaxWorkflowCancelReason
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
