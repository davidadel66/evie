package workflowruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// Durable notifications (Stage 14 sub-stage 11, 2026-10-04; ADR 0046). The
// store records a notice in the transaction of every run transition the
// owner should hear about, and of every schedule slot skipped or refused
// (eviedb/workflow_notifications.go). The owner lists them with `evie
// workflows notifications`, the web API and the Workspace home, and marks
// them read; nothing else changes them. A chat that started a run sees
// that run's unread notices (their kind, never their detail) in its
// workflow_status listing, through ChatWorkflows. No notice leaves Evie:
// external channels that mirror them are a later plugin's.

// Notification is a notice with its text.
type Notification struct {
	eviedb.WorkflowNotification
	Text string `json:"text"`
}

// NotificationMark is the owner marking notices read or unread: the
// given IDs, or (All with Read) every unread notice of a Workspace or of
// every Workspace.
type NotificationMark struct {
	IDs         []int64            `json:"ids,omitempty"`
	Read        bool               `json:"read"`
	All         bool               `json:"all,omitempty"`
	WorkspaceID memory.WorkspaceID `json:"workspace_id,omitempty"`
	Surface     string             `json:"-"`
}

// NotificationText is a notice in one line.
func NotificationText(n eviedb.WorkflowNotification) string {
	if n.ChangeID != "" {
		// A Change's notice (changes.spec.md story 5c) says it all.
		return n.Detail
	}
	run := fmt.Sprintf("%s run %s", n.Workflow, n.RunID)
	origin := ""
	switch n.Surface {
	case "chat":
		origin = " (started from a chat)"
	case "schedule":
		origin = " (started by its schedule)"
	}
	interruptions := fmt.Sprintf(" (evie workflows interruptions --run %s)", n.RunID)
	switch n.Kind {
	case eviedb.WorkflowNotifySucceeded:
		return run + " succeeded" + origin
	case eviedb.WorkflowNotifyFailed:
		return run + " failed" + reasonText(n.Detail) + origin
	case eviedb.WorkflowNotifyCancelled:
		return run + " was cancelled" + origin
	case eviedb.WorkflowNotifyWaitingForHuman:
		return run + " is waiting for your answer" + interruptions + origin
	case eviedb.WorkflowNotifyNeedsReconciliation:
		return run + " needs reconciliation" + reasonText(n.Detail) + interruptions + origin
	case eviedb.WorkflowNotifyPaused:
		return run + " paused" + reasonText(n.Detail) + origin
	case eviedb.WorkflowNotifyCapReached:
		return run + " paused at the daily workflow cap" + reasonText(n.Detail) + origin
	case eviedb.WorkflowNotifyScheduleSkipped:
		return n.Workflow + " schedule skipped " + n.Detail
	case eviedb.WorkflowNotifyScheduleFailed:
		return n.Workflow + " schedule could not start " + n.Detail
	}
	return fmt.Sprintf("%s: %s%s", run, n.Kind, reasonText(n.Detail))
}

// Notifications lists notices, newest first, with their text.
func (r *Runtime) Notifications(ctx context.Context, f eviedb.WorkflowNotificationFilter) ([]Notification, error) {
	list, err := r.store.ListWorkflowNotifications(ctx, f)
	if err != nil {
		return nil, err
	}
	out := make([]Notification, len(list))
	runs := map[string]eviedb.WorkflowRun{}
	for i, n := range list {
		// A run's sensitive inputs never reach a notice's detail,
		// whatever the policy (sub-stage 12): the detail is the run's
		// error at the transition, which may quote them.
		if n.RunID != "" && n.Detail != "" {
			run, ok := runs[n.RunID]
			if !ok {
				if run, err = r.store.WorkflowRunRecord(ctx, n.RunID); err != nil {
					return nil, err
				}
				runs[n.RunID] = run
			}
			n.Detail = r.inputScrubber(ctx, run).Text(n.Detail)
		}
		out[i] = Notification{WorkflowNotification: n, Text: NotificationText(n)}
	}
	return out, nil
}

// MarkNotifications records the owner's read state; only the owner's CLI
// and web surfaces may. It says how many notices it marked.
func (r *Runtime) MarkNotifications(ctx context.Context, m NotificationMark) (int, error) {
	if !ownerSurface(m.Surface) {
		return 0, eviedb.ErrWorkflowApprover
	}
	switch {
	case m.All && m.Read && len(m.IDs) == 0:
		return r.store.MarkAllWorkflowNotificationsRead(ctx, m.WorkspaceID)
	case m.All:
		return 0, errors.New("mark all notifications read, or name the ones to mark")
	}
	return r.store.MarkWorkflowNotifications(ctx, m.IDs, m.Read)
}

// Status is the workflow runtime's state for the owner: the daily cap, the
// schedules and the notices.
type Status struct {
	Caps          CapStatus                         `json:"caps"`
	Schedules     []ScheduleStatus                  `json:"schedules"`
	Notifications eviedb.WorkflowNotificationCounts `json:"notifications"`
}

// Status reads the cap, the schedules and the notice counts of a
// Workspace, or of every Workspace when ws is "".
func (r *Runtime) Status(ctx context.Context, ws memory.WorkspaceID) (Status, error) {
	var out Status
	var err error
	if out.Caps, err = r.Caps(ctx); err != nil {
		return out, err
	}
	if out.Schedules, err = r.Schedules(ctx, ws); err != nil {
		return out, err
	}
	out.Notifications, err = r.store.WorkflowNotificationCounts(ctx, ws)
	return out, err
}

// Text renders the status for the terminal.
func (s Status) Text() string {
	var b strings.Builder
	b.WriteString(s.Caps.Text())
	fmt.Fprintf(&b, "Notifications: %d unread of %d (evie workflows notifications)\n", s.Notifications.Unread, s.Notifications.Total)
	if len(s.Schedules) == 0 {
		b.WriteString("Schedules: none\n")
	} else {
		b.WriteString("Schedules\n")
		for _, line := range strings.Split(strings.TrimRight(SchedulesText(s.Schedules), "\n"), "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
	return b.String()
}

// NotificationsText renders notices for the terminal.
func NotificationsText(list []Notification) string {
	if len(list) == 0 {
		return "No workflow notifications.\n"
	}
	var b strings.Builder
	for _, n := range list {
		read := "unread"
		if n.ReadAt != nil {
			read = "read"
		}
		fmt.Fprintf(&b, "#%d  %s  %s  %s  %s\n", n.ID, n.CreatedAt.Format(timeLayout), n.WorkspaceID, read, n.Text)
	}
	return b.String()
}
