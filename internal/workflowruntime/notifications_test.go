package workflowruntime

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

// Durable notifications (Stage 14 sub-stage 11, ADR 0046): the owner reads
// them on the CLI, the web API and the Workspace home, marks them read,
// and a chat that started a run sees that run's unread notices in its
// workflow_status listing (the run's ID, workflow and what happened; the
// run's own status says the rest).

func TestWorkflowNotificationsReachTheChatThatStartedTheRun(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace()
	h.approve(h.propose(w.ID, filepath.Join(testdata, "split-bill"), fixtures(t, `{"inputs": {"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}`)))
	call := chatCall(t, h.store, w)
	call.Workflow = "split-bill"
	call.Inputs = map[string]json.RawMessage{"bill_id": json.RawMessage(`"dinner-2"`), "total_cents": json.RawMessage(`1000`), "people": json.RawMessage(`4`)}
	started, err := h.rt.StartFromChat(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := h.rt.ChatWorkflows(ctx, call.Scope)
	if err != nil || len(catalog.Notifications) != 0 {
		t.Fatalf("notices before the run finished = %+v, %v", catalog.Notifications, err)
	}
	if _, err := h.rt.execute(ctx, started.RunID); err != nil {
		t.Fatal(err)
	}
	catalog, err = h.rt.ChatWorkflows(ctx, call.Scope)
	if err != nil || len(catalog.Notifications) != 1 {
		t.Fatalf("notices = %+v, %v", catalog.Notifications, err)
	}
	n := catalog.Notifications[0]
	if n.RunID != started.RunID || n.Workflow != "split-bill" || n.Kind != "succeeded" || n.At.IsZero() {
		t.Fatalf("notice = %+v", n)
	}
	// The owner's surfaces list it with its text and mark it read; the
	// chat then has nothing unread.
	notes, err := h.rt.Notifications(ctx, eviedb.WorkflowNotificationFilter{SessionID: call.Scope.SessionID})
	if err != nil || len(notes) != 1 || notes[0].Text != "split-bill run "+started.RunID+" succeeded (started from a chat)" {
		t.Fatalf("notifications = %+v, %v", notes, err)
	}
	if _, err := h.rt.MarkNotifications(ctx, NotificationMark{IDs: []int64{notes[0].ID}, Read: true, Surface: "chat"}); err == nil {
		t.Fatal("a chat marked a notification read")
	}
	marked, err := h.rt.MarkNotifications(ctx, NotificationMark{IDs: []int64{notes[0].ID}, Read: true, Surface: "web"})
	if err != nil || marked != 1 {
		t.Fatalf("marked = %d, %v", marked, err)
	}
	if catalog, err = h.rt.ChatWorkflows(ctx, call.Scope); err != nil || len(catalog.Notifications) != 0 {
		t.Fatalf("notices after reading = %+v, %v", catalog.Notifications, err)
	}
	status, err := h.rt.Status(ctx, w.ID)
	if err != nil || status.Notifications.Total != 1 || status.Notifications.Unread != 0 || status.Caps.Enforced {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if text := status.Text(); !strings.Contains(text, "Notifications: 0 unread of 1") || !strings.Contains(text, "Daily cap: not enforced") {
		t.Fatalf("status text:\n%s", text)
	}
}

// Every kind has its text, and a waiting or failing run says what to do.
func TestWorkflowNotificationTexts(t *testing.T) {
	cases := map[eviedb.WorkflowNotificationKind]string{
		eviedb.WorkflowNotifySucceeded:           "deep-research run r1 succeeded",
		eviedb.WorkflowNotifyFailed:              "deep-research run r1 failed: boom",
		eviedb.WorkflowNotifyCancelled:           "deep-research run r1 was cancelled",
		eviedb.WorkflowNotifyWaitingForHuman:     "deep-research run r1 is waiting for your answer (evie workflows interruptions --run r1)",
		eviedb.WorkflowNotifyNeedsReconciliation: "deep-research run r1 needs reconciliation: boom (evie workflows interruptions --run r1)",
		eviedb.WorkflowNotifyPaused:              "deep-research run r1 paused: boom",
		eviedb.WorkflowNotifyCapReached:          "deep-research run r1 paused at the daily workflow cap: boom",
		eviedb.WorkflowNotifyScheduleSkipped:     "deep-research schedule skipped boom",
		eviedb.WorkflowNotifyScheduleFailed:      "deep-research schedule could not start boom",
	}
	for kind, want := range cases {
		n := eviedb.WorkflowNotification{Workflow: "deep-research", RunID: "r1", Kind: kind, Detail: "boom", Surface: "cli"}
		if kind == eviedb.WorkflowNotifyScheduleSkipped || kind == eviedb.WorkflowNotifyScheduleFailed {
			n.RunID = ""
		}
		if got := NotificationText(n); got != want {
			t.Errorf("%s: %q, want %q", kind, got, want)
		}
	}
}
