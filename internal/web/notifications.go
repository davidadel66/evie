package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// The app-wide notifications list (workspace-activity.spec.md): every
// Durable Notification the owner should hear about, from every Workspace,
// in one general shape so later sources join the same list. The workflow
// runtime's notices are the only source today; the per-Workspace workflow
// notification routes stay for the CLI.

// notice is one notification in the general shape. Its ID names its
// source ("workflow:12"), and its target is what opening it shows.
type notice struct {
	ID        string       `json:"id"`
	Source    string       `json:"source"`
	Label     string       `json:"label"`
	Text      string       `json:"text"`
	CreatedAt time.Time    `json:"created_at"`
	ReadAt    *time.Time   `json:"read_at,omitempty"`
	Target    noticeTarget `json:"target"`
}

// noticeTarget is where a notice leads: a workflow run in its Workspace,
// or a Workspace's runs for a notice with no run (a skipped schedule slot).
type noticeTarget struct {
	Kind        string `json:"kind"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
}

const workflowNoticePrefix = "workflow:"

func (s *Server) registerNotificationRoutes(mux *http.ServeMux) {
	if s.workflows == nil {
		return
	}
	mux.Handle("/api/notifications", s.managementRoute(s.handleNotifications))
	mux.Handle("/api/notifications/read", s.managementRoute(s.handleNotificationsRead))
}

// handleNotifications lists the newest notices, read or not, with how many
// are unread (counted up to the listing limit of 200).
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Limit int `json:"limit,omitempty"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	if request.Limit < 0 || request.Limit > 200 {
		managementJSONError(w, http.StatusBadRequest, "invalid_notification_request", "notifications takes an optional limit up to 200")
		return
	}
	list, err := s.workflows.Notifications(r.Context(), eviedb.WorkflowNotificationFilter{All: true, Limit: request.Limit})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	unread, err := s.workflows.Notifications(r.Context(), eviedb.WorkflowNotificationFilter{Limit: 200})
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	out := make([]notice, len(list))
	for i, n := range list {
		out[i] = workflowNotice(n)
	}
	writeJSON(w, http.StatusOK, struct {
		Unread        int      `json:"unread"`
		Notifications []notice `json:"notifications"`
	}{len(unread), out})
}

// handleNotificationsRead marks the named notices read or unread, or every
// unread notice read.
func (s *Server) handleNotificationsRead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		IDs  []string `json:"ids,omitempty"`
		Read bool     `json:"read"`
		All  bool     `json:"all,omitempty"`
	}
	if !decodeCandidateRequest(w, r, &request) {
		return
	}
	mark := workflowruntime.NotificationMark{Read: request.Read, All: request.All, Surface: "web"}
	valid := len(request.IDs) <= 200 && (request.All && request.Read && len(request.IDs) == 0 || !request.All && len(request.IDs) > 0)
	for _, id := range request.IDs {
		n, err := strconv.ParseInt(strings.TrimPrefix(id, workflowNoticePrefix), 10, 64)
		valid = valid && strings.HasPrefix(id, workflowNoticePrefix) && err == nil && n > 0
		mark.IDs = append(mark.IDs, n)
	}
	if !valid {
		managementJSONError(w, http.StatusBadRequest, "invalid_notification_request", "name 1 to 200 notification IDs with read true or false, or all with read true")
		return
	}
	marked, err := s.workflows.MarkNotifications(r.Context(), mark)
	if err != nil {
		writeWorkflowError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Marked int `json:"marked"`
	}{marked})
}

// workflowNotice is a workflow notice in the general shape. Its label is
// the workflow; its text says what happened without the CLI's commands.
// The detail was already scrubbed of sensitive run inputs by the runtime.
func workflowNotice(n workflowruntime.Notification) notice {
	target := noticeTarget{Kind: "workspace_runs", WorkspaceID: string(n.WorkspaceID)}
	if n.RunID != "" {
		target = noticeTarget{Kind: "workflow_run", WorkspaceID: string(n.WorkspaceID), RunID: n.RunID}
	}
	return notice{ID: workflowNoticePrefix + strconv.FormatInt(n.ID, 10), Source: "workflow", Label: n.Workflow,
		Text: workflowNoticeText(n.WorkflowNotification), CreatedAt: n.CreatedAt, ReadAt: n.ReadAt, Target: target}
}

func workflowNoticeText(n eviedb.WorkflowNotification) string {
	with := func(text string) string {
		if n.Detail == "" {
			return text
		}
		return text + ": " + n.Detail
	}
	switch n.Kind {
	case eviedb.WorkflowNotifySucceeded:
		return "Succeeded"
	case eviedb.WorkflowNotifyFailed:
		return with("Failed")
	case eviedb.WorkflowNotifyCancelled:
		return "Cancelled"
	case eviedb.WorkflowNotifyWaitingForHuman:
		return "Waiting for your answer"
	case eviedb.WorkflowNotifyNeedsReconciliation:
		return with("Needs you to settle an effect")
	case eviedb.WorkflowNotifyPaused:
		return with("Paused")
	case eviedb.WorkflowNotifyCapReached:
		return "Paused at the daily workflow cap"
	case eviedb.WorkflowNotifyScheduleSkipped:
		return strings.TrimSpace("Schedule skipped " + n.Detail)
	case eviedb.WorkflowNotifyScheduleFailed:
		return strings.TrimSpace("Schedule could not start " + n.Detail)
	}
	return with(string(n.Kind))
}
