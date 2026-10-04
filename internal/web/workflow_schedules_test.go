package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Stage 14 sub-stage 11: the owner's web management API lists schedules,
// pauses and resumes them, lists durable notifications and marks them
// read, and shows the daily cap; all are management routes.
func TestWorkflowWebSchedulesNotificationsAndStatus(t *testing.T) {
	ctx := context.Background()
	_, rt, w := workflowRuntimeForWeb(t)
	f, err := workflow.ParseFixtures([]byte(`{"inputs": {"day": "2026-10-05", "at": "07:00", "total_cents": 1000, "people": 3}}`))
	if err != nil {
		t.Fatal(err)
	}
	card, err := rt.Propose(ctx, workflowruntime.Proposal{WorkspaceID: w.ID, Dir: filepath.Join("..", "workflow", "testdata", "scheduled-split"), Fixtures: &f, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Approve(ctx, workflowruntime.Approval{ProposalID: card.Proposal.ID, ExecutableHash: card.Proposal.ExecutableHash, AuthorityHash: card.Proposal.AuthorityHash, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()

	resp := workflowPost(t, h, "schedules", map[string]any{"workspace_id": w.ID})
	var listed struct {
		Schedules []workflowruntime.ScheduleStatus `json:"schedules"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || len(listed.Schedules) != 1 || listed.Schedules[0].State != "active" ||
		listed.Schedules[0].Next == nil {
		t.Fatalf("schedules = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "schedule", map[string]any{"workspace_id": w.ID, "name": "scheduled-split", "action": "pause", "reason": "away"})
	var control eviedb.WorkflowScheduleControl
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &control) != nil || control.Surface != "web" || control.Action != "pause" {
		t.Fatalf("pause = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "schedule", map[string]any{"workspace_id": w.ID, "name": "scheduled-split", "action": "pause"}); resp.Code != http.StatusConflict ||
		!strings.Contains(resp.Body.String(), "schedule_state") {
		t.Fatalf("pausing twice = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "schedule", map[string]any{"workspace_id": w.ID, "name": "scheduled-split", "action": "stop"}); resp.Code != http.StatusBadRequest {
		t.Fatalf("unknown action = %d %s", resp.Code, resp.Body)
	}
	if _, err := rt.StartRun(ctx, workflowruntime.RunRequest{WorkspaceID: w.ID, Workflow: "scheduled-split", Surface: "cli",
		TextInputs: map[string]string{"day": "2026-10-05", "at": "07:00", "total_cents": "1000", "people": "3"}}); err != nil {
		t.Fatal(err)
	}
	resp = workflowPost(t, h, "notifications", map[string]any{"workspace_id": w.ID})
	var notes struct {
		Notifications []workflowruntime.Notification `json:"notifications"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &notes) != nil || len(notes.Notifications) != 1 || notes.Notifications[0].Kind != eviedb.WorkflowNotifySucceeded ||
		!strings.Contains(notes.Notifications[0].Text, "succeeded") {
		t.Fatalf("notifications = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "notifications/read", map[string]any{"ids": []int64{notes.Notifications[0].ID}, "read": true})
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"marked":1`) {
		t.Fatalf("read = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "status", map[string]any{"workspace_id": w.ID})
	var status workflowruntime.Status
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &status) != nil || status.Notifications.Total != 1 || status.Notifications.Unread != 0 ||
		len(status.Schedules) != 1 || status.Schedules[0].State != "paused" {
		t.Fatalf("status = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "notifications/read", map[string]any{"read": true}); resp.Code != http.StatusBadRequest {
		t.Fatalf("marking nothing = %d %s", resp.Code, resp.Body)
	}
	for _, path := range []string{"schedules", "schedule", "notifications", "notifications/read", "status"} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workflows/"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		if resp := workflowPost(t, h, path, map[string]any{}, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); resp.Code != http.StatusForbidden {
			t.Fatalf("cross-origin %s = %d", path, resp.Code)
		}
	}
}
