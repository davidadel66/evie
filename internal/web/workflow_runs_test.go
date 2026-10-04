package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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

var webTestdata = filepath.Join("..", "workflow", "testdata")

func approveForWeb(t *testing.T, rt *workflowruntime.Runtime, ws memory.WorkspaceID, name string, fixtures []byte) {
	t.Helper()
	ctx := context.Background()
	f, err := workflow.ParseFixtures(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	card, err := rt.Propose(ctx, workflowruntime.Proposal{WorkspaceID: ws, Dir: filepath.Join(webTestdata, name), Fixtures: &f, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Approve(ctx, workflowruntime.Approval{ProposalID: card.Proposal.ID, ExecutableHash: card.Proposal.ExecutableHash,
		AuthorityHash: card.Proposal.AuthorityHash, ExpectedVersion: card.ExpectedVersion, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
}

// The owner starts a run from the web, lists and inspects runs; a
// duplicate start returns the run that holds the Run Key, and refusals carry
// their reason.
func TestWorkflowWebRoutesStartListAndInspectRuns(t *testing.T) {
	store, rt, w := workflowRuntimeForWeb(t)
	splitFixtures, err := os.ReadFile(filepath.Join(webTestdata, "split-bill.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	approveForWeb(t, rt, w.ID, "split-bill", splitFixtures)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	start := map[string]any{"workspace_id": w.ID, "name": "split-bill", "inputs": map[string]any{"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}
	resp := workflowPost(t, h, "start", start)
	var started workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &started) != nil || started.Existing ||
		started.Run.Run.State != eviedb.WorkflowRunSucceeded || started.Run.Run.Surface != "web" || len(started.Run.Attempts) != 3 {
		t.Fatalf("start = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "start", start)
	var again workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &again) != nil || !again.Existing || again.Run.Run.ID != started.Run.Run.ID {
		t.Fatalf("duplicate start = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "runs", map[string]any{"workspace_id": w.ID})
	var listed struct {
		Runs []eviedb.WorkflowRun `json:"runs"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || len(listed.Runs) != 1 || listed.Runs[0].ID != started.Run.Run.ID {
		t.Fatalf("runs = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "inspect", map[string]any{"run_id": started.Run.Run.ID})
	var detail eviedb.WorkflowRunDetail
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &detail) != nil || len(detail.Checkpoints) != 4 {
		t.Fatalf("inspect = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "inspect", map[string]any{"run_id": "missing"}); resp.Code != http.StatusNotFound {
		t.Fatalf("missing run = %d %s", resp.Code, resp.Body)
	}
	bad := map[string]any{"workspace_id": w.ID, "name": "split-bill", "inputs": map[string]any{"bill_id": "b", "total_cents": "ten", "people": 3}}
	if resp := workflowPost(t, h, "start", bad); resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "invalid_input") {
		t.Fatalf("invalid input = %d %s", resp.Code, resp.Body)
	}
	global, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foreign := map[string]any{"workspace_id": w.ID, "name": "split-bill", "inputs": map[string]any{"bill_id": "c", "total_cents": 1, "people": 1}, "session_id": global.ID}
	if resp := workflowPost(t, h, "start", foreign); resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "invalid_session") {
		t.Fatalf("session outside the Workspace = %d %s", resp.Code, resp.Body)
	}

	approveForWeb(t, rt, w.ID, "notes-digest", []byte(`{"inputs": {"day": "2026-10-05"}, "steps": {
		"read": {"output": {"notes": [{"id": "n1", "text": "x"}]}}, "summarize": {"output": {"digest": "d"}}, "write": {"output": {"entry_id": "e"}}}}`))
	notes := map[string]any{"workspace_id": w.ID, "name": "notes-digest", "inputs": map[string]any{"day": "2026-10-05"}}
	if resp := workflowPost(t, h, "start", notes); resp.Code != http.StatusUnprocessableEntity || !strings.Contains(resp.Body.String(), "unsupported_steps") ||
		!strings.Contains(resp.Body.String(), "not yet supported in this runtime version") {
		t.Fatalf("unsupported steps = %d %s", resp.Code, resp.Body)
	}
	if _, err := rt.Deactivate(context.Background(), workflowruntime.Deactivation{WorkspaceID: w.ID, Name: "split-bill", Version: 1, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	if resp := workflowPost(t, h, "start", start); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "not_active") {
		t.Fatalf("deactivated = %d %s", resp.Code, resp.Body)
	}
}

// A detached web start records the run and returns it at once; the
// server's scheduler executes it, and inspection shows its lease and
// recovery events.
func TestWorkflowWebDetachedStartReturnsTheRecordedRun(t *testing.T) {
	_, rt, w := workflowRuntimeForWeb(t)
	splitFixtures, err := os.ReadFile(filepath.Join(webTestdata, "split-bill.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	approveForWeb(t, rt, w.ID, "split-bill", splitFixtures)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	resp := workflowPost(t, h, "start", map[string]any{"workspace_id": w.ID, "name": "split-bill", "detach": true,
		"inputs": map[string]any{"bill_id": "dinner-1", "total_cents": 1000, "people": 3}})
	var started workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &started) != nil || started.Existing ||
		started.Run.Run.State != eviedb.WorkflowRunQueued || len(started.Run.Attempts) != 0 || started.Run.Lease != nil {
		t.Fatalf("detached start = %d %s", resp.Code, resp.Body)
	}
	stop := rt.Start(context.Background())
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp = workflowPost(t, h, "inspect", map[string]any{"run_id": started.Run.Run.ID})
		var detail eviedb.WorkflowRunDetail
		if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &detail) != nil {
			t.Fatalf("inspect = %d %s", resp.Code, resp.Body)
		}
		if detail.Run.State == eviedb.WorkflowRunSucceeded {
			if detail.Lease == nil || detail.Lease.Holder != rt.Holder() || detail.Lease.ReleasedAt == nil || detail.Events == nil {
				t.Fatalf("inspected lease = %s", resp.Body)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the scheduler did not finish the run: %s", resp.Body)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The run routes are management routes with exact, bounded bodies: the
// browser cannot choose the surface or pass command-line text.
func TestWorkflowWebRunRoutesAreGuarded(t *testing.T) {
	_, rt, w := workflowRuntimeForWeb(t)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	for _, path := range []string{"start", "runs", "inspect"} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workflows/"+path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("GET %s = %d", path, rec.Code)
		}
		if resp := workflowPost(t, h, path, map[string]any{}, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); resp.Code != http.StatusForbidden {
			t.Fatalf("cross-origin %s = %d", path, resp.Code)
		}
		if resp := workflowPost(t, h, path, map[string]any{}, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }); resp.Code != http.StatusForbidden {
			t.Fatalf("form-style %s = %d", path, resp.Code)
		}
	}
	for name, body := range map[string]map[string]any{
		"no name":       {"workspace_id": w.ID},
		"no workspace":  {"name": "split-bill"},
		"surface":       {"workspace_id": w.ID, "name": "split-bill", "surface": "chat"},
		"text inputs":   {"workspace_id": w.ID, "name": "split-bill", "text_inputs": map[string]string{"a": "b"}},
		"inputs list":   {"workspace_id": w.ID, "name": "split-bill", "inputs": []int{1}},
		"long name":     {"workspace_id": w.ID, "name": strings.Repeat("x", 200)},
		"long session":  {"workspace_id": w.ID, "name": "split-bill", "session_id": strings.Repeat("s", 200)},
		"unknown field": {"workspace_id": w.ID, "name": "split-bill", "approver": "evie"},
	} {
		if resp := workflowPost(t, h, "start", body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
	if resp := workflowPost(t, h, "inspect", map[string]any{}); resp.Code != http.StatusBadRequest {
		t.Fatalf("inspect without a run = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "runs", map[string]any{"limit": 1000}); resp.Code != http.StatusBadRequest {
		t.Fatalf("unbounded listing = %d %s", resp.Code, resp.Body)
	}
}
