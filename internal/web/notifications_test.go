package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// workspace-activity.spec.md: one notifications list for the whole app,
// in a general notice shape whose only source today is the workflow
// runtime's durable notifications of every Workspace.

type notice struct {
	ID        string     `json:"id"`
	Source    string     `json:"source"`
	Label     string     `json:"label"`
	Text      string     `json:"text"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at"`
	Target    struct {
		Kind        string `json:"kind"`
		WorkspaceID string `json:"workspace_id"`
		RunID       string `json:"run_id"`
	} `json:"target"`
}

type noticePage struct {
	Unread        int      `json:"unread"`
	Notifications []notice `json:"notifications"`
}

func notificationsPost(t *testing.T, h http.Handler, path string, body any, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/notifications"+path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:5173")
	for _, m := range mutate {
		m(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// noticeSource is a WorkflowManager whose notifications are fixed; every
// other method is left to the embedded nil interface and is never called.
type noticeSource struct {
	web.WorkflowManager
	list    []workflowruntime.Notification
	filters []eviedb.WorkflowNotificationFilter
	marks   []workflowruntime.NotificationMark
}

func (s *noticeSource) Notifications(_ context.Context, f eviedb.WorkflowNotificationFilter) ([]workflowruntime.Notification, error) {
	s.filters = append(s.filters, f)
	if f.All {
		return s.list, nil
	}
	var unread []workflowruntime.Notification
	for _, n := range s.list {
		if n.ReadAt == nil {
			unread = append(unread, n)
		}
	}
	return unread, nil
}

func (s *noticeSource) MarkNotifications(_ context.Context, m workflowruntime.NotificationMark) (int, error) {
	s.marks = append(s.marks, m)
	return len(m.IDs), nil
}

func TestNotificationsListEveryWorkspaceInTheGeneralShape(t *testing.T) {
	read := time.Date(2026, 10, 9, 9, 30, 0, 0, time.UTC)
	note := func(id int64, ws memory.WorkspaceID, workflowName, run string, kind eviedb.WorkflowNotificationKind, detail string, readAt *time.Time) workflowruntime.Notification {
		return workflowruntime.Notification{WorkflowNotification: eviedb.WorkflowNotification{ID: id, WorkspaceID: ws, Workflow: workflowName, RunID: run, Kind: kind,
			Detail: detail, CreatedAt: time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Minute), ReadAt: readAt},
			Text: "cli text (evie workflows interruptions --run " + run + ")"}
	}
	source := &noticeSource{list: []workflowruntime.Notification{
		note(9, "finance", "finance-review", "run-9", eviedb.WorkflowNotifyWaitingForHuman, "", nil),
		note(8, "code", "deps-weekly", "run-8", eviedb.WorkflowNotifyFailed, "steps.collect failed: rate limited", nil),
		note(7, "finance", "finance-daily", "", eviedb.WorkflowNotifyScheduleSkipped, "2026-10-09 09:00 (catch-up skip)", &read),
	}}
	h := web.WithWorkflows(web.NewServer(nil), source).Handler()

	resp := notificationsPost(t, h, "", map[string]any{})
	var page noticePage
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &page) != nil {
		t.Fatalf("list = %d %s", resp.Code, resp.Body)
	}
	if page.Unread != 2 || len(page.Notifications) != 3 {
		t.Fatalf("unread %d of %d notices, want 2 of 3: %s", page.Unread, len(page.Notifications), resp.Body)
	}
	for _, f := range source.filters {
		if f.WorkspaceID != "" {
			t.Fatalf("listing was narrowed to Workspace %q; every Workspace is wanted", f.WorkspaceID)
		}
	}
	waiting, failed, skipped := page.Notifications[0], page.Notifications[1], page.Notifications[2]
	if waiting.ID != "workflow:9" || waiting.Source != "workflow" || waiting.Label != "finance-review" || waiting.Text != "Waiting for your answer" ||
		waiting.Target.Kind != "workflow_run" || waiting.Target.WorkspaceID != "finance" || waiting.Target.RunID != "run-9" || waiting.ReadAt != nil {
		t.Fatalf("waiting notice = %+v", waiting)
	}
	if failed.Text != "Failed: steps.collect failed: rate limited" || failed.Target.WorkspaceID != "code" {
		t.Fatalf("failed notice = %+v", failed)
	}
	// A notice without a run opens its Workspace's runs.
	if skipped.Target.Kind != "workspace_runs" || skipped.Target.WorkspaceID != "finance" || skipped.Target.RunID != "" || skipped.ReadAt == nil ||
		skipped.Text != "Schedule skipped 2026-10-09 09:00 (catch-up skip)" {
		t.Fatalf("schedule notice = %+v", skipped)
	}
	// The web list is not the CLI: no terminal commands in the text.
	if strings.Contains(resp.Body.String(), "evie workflows") {
		t.Fatalf("notice text carries CLI hints: %s", resp.Body)
	}

	if resp := notificationsPost(t, h, "/read", map[string]any{"ids": []string{"workflow:9", "workflow:8"}, "read": true}); resp.Code != http.StatusOK ||
		!strings.Contains(resp.Body.String(), `"marked":2`) {
		t.Fatalf("read = %d %s", resp.Code, resp.Body)
	}
	if got := source.marks[0]; len(got.IDs) != 2 || got.IDs[0] != 9 || got.IDs[1] != 8 || !got.Read || got.All || got.Surface != "web" {
		t.Fatalf("mark = %+v", got)
	}
	if resp := notificationsPost(t, h, "/read", map[string]any{"all": true, "read": true}); resp.Code != http.StatusOK {
		t.Fatalf("read all = %d %s", resp.Code, resp.Body)
	}
	if got := source.marks[1]; !got.All || !got.Read || got.WorkspaceID != "" || got.Surface != "web" {
		t.Fatalf("mark all = %+v", got)
	}
	for name, body := range map[string]any{
		"another source":    map[string]any{"ids": []string{"memory:1"}, "read": true},
		"a malformed id":    map[string]any{"ids": []string{"workflow:x"}, "read": true},
		"a zero id":         map[string]any{"ids": []string{"workflow:0"}, "read": true},
		"nothing":           map[string]any{"read": true},
		"all unread":        map[string]any{"all": true, "read": false},
		"ids and all":       map[string]any{"ids": []string{"workflow:9"}, "all": true, "read": true},
		"an unknown field":  map[string]any{"ids": []string{"workflow:9"}, "read": true, "workspace_id": "finance"},
		"a list over limit": map[string]any{"limit": 201},
	} {
		path := "/read"
		if name == "a list over limit" {
			path = ""
		}
		if resp := notificationsPost(t, h, path, body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
	if len(source.marks) != 2 {
		t.Fatalf("a refused request marked notices: %+v", source.marks)
	}
	for _, path := range []string{"", "/read"} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/notifications"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %q = %d", path, rec.Code)
		}
		if resp := notificationsPost(t, h, path, map[string]any{}, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); resp.Code != http.StatusForbidden {
			t.Fatalf("cross-origin %q = %d", path, resp.Code)
		}
	}
}

func TestNotificationsReachTheRunThatFinished(t *testing.T) {
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
	started, err := rt.StartRun(ctx, workflowruntime.RunRequest{WorkspaceID: w.ID, Workflow: "scheduled-split", Surface: "cli",
		TextInputs: map[string]string{"day": "2026-10-05", "at": "07:00", "total_cents": "1000", "people": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()

	resp := notificationsPost(t, h, "", map[string]any{})
	var page noticePage
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &page) != nil || page.Unread != 1 || len(page.Notifications) != 1 {
		t.Fatalf("list = %d %s", resp.Code, resp.Body)
	}
	n := page.Notifications[0]
	if n.Label != "scheduled-split" || n.Text != "Succeeded" || n.Target.Kind != "workflow_run" || n.Target.WorkspaceID != string(w.ID) ||
		n.Target.RunID != started.Run.Run.ID {
		t.Fatalf("notice = %+v", n)
	}
	if resp := notificationsPost(t, h, "/read", map[string]any{"all": true, "read": true}); resp.Code != http.StatusOK {
		t.Fatalf("read all = %d %s", resp.Code, resp.Body)
	}
	resp = notificationsPost(t, h, "", map[string]any{})
	if json.Unmarshal(resp.Body.Bytes(), &page) != nil || page.Unread != 0 || len(page.Notifications) != 1 || page.Notifications[0].ReadAt == nil {
		t.Fatalf("after reading = %s", resp.Body)
	}
}
