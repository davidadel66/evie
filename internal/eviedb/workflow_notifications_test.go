package eviedb

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// Durable notifications (Stage 14 sub-stage 11, ADR 0046): every run
// transition the owner should hear about (succeeded, failed, cancelled,
// waiting for an answer, needing reconciliation, paused, paused at the
// daily cap) creates one notification in the transaction that makes it,
// deduplicated by run and transition, with the run's origin and session.
// Only the read state ever changes.

func notificationsOf(t *testing.T, s *Store, f WorkflowNotificationFilter) []WorkflowNotification {
	t.Helper()
	out, err := s.ListWorkflowNotifications(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func notificationKinds(notes []WorkflowNotification) string {
	kinds := make([]string, len(notes))
	for i, n := range notes {
		kinds[len(notes)-1-i] = string(n.Kind)
	}
	return strings.Join(kinds, ",")
}

func TestWorkflowRunTransitionsCreateDeduplicatedNotifications(t *testing.T) {
	ctx := context.Background()
	r, clock := openLeasedRunDB(t)
	_, run := notesRun(t, r.s)
	claim, err := r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	lease := claim.Lease
	// Queued and running notify nothing.
	if n := notificationsOf(t, r.s, WorkflowNotificationFilter{RunID: run.ID}); len(n) != 0 {
		t.Fatalf("notifications before any transition = %+v", n)
	}
	// A pause notifies once per reason at a checkpoint, however often the
	// pause is rechecked or flaps.
	if _, err := r.s.PauseWorkflowRun(ctx, lease, "steps.read: the notes provider is unavailable", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, lease, "steps.read: the notes provider is unavailable", time.Minute); err != nil {
		t.Fatal(err)
	}
	clock.Advance(5 * time.Second)
	if _, err := r.s.ResumeWorkflowRun(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, lease, "steps.read: the notes provider is unavailable", time.Minute); err != nil {
		t.Fatal(err)
	}
	// A pause at the daily cap is its own kind.
	clock.Advance(5 * time.Second)
	if _, err := r.s.ResumeWorkflowRun(ctx, lease); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.PauseWorkflowRun(ctx, lease, WorkflowDailyCapReason+" for 2026-10-04 (UTC): the run continues after 2026-10-05 00:00:00Z", 15*time.Hour); err != nil {
		t.Fatal(err)
	}
	clock.Advance(16 * time.Hour)
	claim, err = r.s.AcquireWorkflowRunLease(ctx, run.ID, "holder-b", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	lease = claim.Lease
	if _, err := r.s.ResumeWorkflowRun(ctx, lease); err != nil {
		t.Fatal(err)
	}
	advanceTo(t, r.s, lease, run.ID, []string{"read", "check"}, []string{"check", "ask"})
	// A question notifies that the run waits for the owner.
	if _, err := r.s.AskWorkflowQuestion(ctx, lease, askQuestion(run.ID)); err != nil {
		t.Fatal(err)
	}
	notes := notificationsOf(t, r.s, WorkflowNotificationFilter{RunID: run.ID})
	if got := notificationKinds(notes); got != "paused,cap_reached,waiting_for_human" {
		t.Fatalf("kinds = %s (%+v)", got, notes)
	}
	n := notes[0]
	if n.WorkspaceID != run.WorkspaceID || n.Workflow != "notes-digest" || n.RunID != run.ID || n.RunState != string(WorkflowRunWaitingForHuman) || n.Surface != "cli" || n.ReadAt != nil {
		t.Fatalf("notification = %+v", n)
	}
	if notes[1].Detail == "" || !strings.HasPrefix(notes[1].Detail, WorkflowDailyCapReason) || notes[2].Detail != "steps.read: the notes provider is unavailable" {
		t.Fatalf("details = %q, %q", notes[1].Detail, notes[2].Detail)
	}
	// Read state is the only thing that changes.
	marked, err := r.s.MarkWorkflowNotifications(ctx, []int64{notes[2].ID, notes[1].ID}, true)
	if err != nil || marked != 2 {
		t.Fatalf("marked = %d, %v", marked, err)
	}
	if unread := notificationsOf(t, r.s, WorkflowNotificationFilter{RunID: run.ID}); len(unread) != 1 || unread[0].ID != n.ID {
		t.Fatalf("unread = %+v", unread)
	}
	all := notificationsOf(t, r.s, WorkflowNotificationFilter{RunID: run.ID, All: true})
	if len(all) != 3 || all[2].ReadAt == nil {
		t.Fatalf("all = %+v", all)
	}
	if marked, err := r.s.MarkWorkflowNotifications(ctx, []int64{notes[1].ID}, false); err != nil || marked != 1 {
		t.Fatalf("mark unread = %d, %v", marked, err)
	}
	if counts, err := r.s.WorkflowNotificationCounts(ctx, run.WorkspaceID); err != nil || counts.Unread != 2 || counts.Total != 3 {
		t.Fatalf("counts = %+v, %v", counts, err)
	}
	if marked, err := r.s.MarkAllWorkflowNotificationsRead(ctx, run.WorkspaceID); err != nil || marked != 2 {
		t.Fatalf("mark all = %d, %v", marked, err)
	}
	for _, statement := range []string{`UPDATE workflow_notifications SET detail='x'`, `DELETE FROM workflow_notifications`} {
		if _, err := r.db.ExecContext(ctx, statement); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s: %v", statement, err)
		}
	}
}

// Terminal transitions notify once each: succeeded, failed and cancelled;
// a run from a chat carries its session so that chat can show it.
func TestWorkflowTerminalTransitionsNotifyWithTheirOrigin(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	w, a := splitBillActivation(t, r.s)
	session, origin := chatTurn(t, r.s, w, workflowsReceipt(WorkflowStartCapabilityID, WorkflowStatusCapabilityID), WorkflowStartToolName)
	fromChat, _, err := r.s.StartWorkflowRun(ctx, chatStart(a, "split-bill:chat", session, origin))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := r.s.AcquireWorkflowRunLease(ctx, fromChat.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := r.s.BeginWorkflowAttempt(ctx, claim.Lease, splitAttempt(fromChat.ID, 0, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowAttempt(ctx, claim.Lease, WorkflowAttemptEnd{RunID: fromChat.ID, Seq: attempt.Seq, Output: json.RawMessage(`{}`), Next: "",
		State: json.RawMessage(`{"split":{}}`), Outputs: json.RawMessage(`{"shares":[]}`)}); err != nil {
		t.Fatal(err)
	}
	failing, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:fails"))
	if err != nil {
		t.Fatal(err)
	}
	claim, err = r.s.AcquireWorkflowRunLease(ctx, failing.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FailWorkflowRun(ctx, claim.Lease, 0, "steps.split failed after 3 attempts: boom"); err != nil {
		t.Fatal(err)
	}
	cancelled, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:cancelled"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.RequestWorkflowRunCancellation(ctx, WorkflowRunCancellationRequest{RunID: cancelled.ID, Reason: "not needed", Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	claim, err = r.s.AcquireWorkflowRunLease(ctx, cancelled.ID, "holder-a", leaseFor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.FinishWorkflowCancellation(ctx, claim.Lease); err != nil {
		t.Fatal(err)
	}
	bySession := notificationsOf(t, r.s, WorkflowNotificationFilter{SessionID: session.ID})
	if len(bySession) != 1 || bySession[0].Kind != WorkflowNotifySucceeded || bySession[0].RunID != fromChat.ID || bySession[0].Surface != "chat" || bySession[0].SessionID != session.ID {
		t.Fatalf("the chat's notifications = %+v", bySession)
	}
	all := notificationsOf(t, r.s, WorkflowNotificationFilter{WorkspaceID: w.ID})
	if got := notificationKinds(all); got != "succeeded,failed,cancelled" {
		t.Fatalf("kinds = %s", got)
	}
	if all[1].Detail != "steps.split failed after 3 attempts: boom" {
		t.Fatalf("failure detail = %q", all[1].Detail)
	}
}

// An effect whose outcome becomes unknown notifies that the run needs
// reconciliation, once, with why.
func TestWorkflowUnknownEffectNotifiesNeedsReconciliation(t *testing.T) {
	ctx := context.Background()
	r, _ := openLeasedRunDB(t)
	run, lease := notesAtWrite(t, r.s)
	intent, err := r.s.BeginWorkflowEffect(ctx, lease, writeEffect(run.ID, 3, 1))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.MarkWorkflowEffectUnknown(ctx, lease, WorkflowEffectUnknownMark{RunID: run.ID, IntentSeq: intent.Seq,
		Reason: "steps.write: the provider did not answer within 30s"}); err != nil {
		t.Fatal(err)
	}
	notes := notificationsOf(t, r.s, WorkflowNotificationFilter{RunID: run.ID})
	if len(notes) != 1 || notes[0].Kind != WorkflowNotifyNeedsReconciliation || !strings.Contains(notes[0].Detail, "30s") {
		t.Fatalf("notifications = %+v", notes)
	}
}
