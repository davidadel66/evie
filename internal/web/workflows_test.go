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
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

func workflowPost(t *testing.T, h http.Handler, path string, body any, mutate ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workflows/"+path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:5173")
	for _, m := range mutate {
		m(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func workflowRuntimeForWeb(t *testing.T) (*eviedb.Store, *workflowruntime.Runtime, memory.Workspace) {
	t.Helper()
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	note := workflow.ObjectType(workflow.Field{Name: "id", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()})
	catalog := workflow.NewCatalog(
		workflow.CapabilityContract{ID: "notes.list", Contract: "1.0.0",
			Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}),
			Output: workflow.ObjectType(workflow.Field{Name: "notes", Type: workflow.ArrayType(note)})},
		workflow.CapabilityContract{ID: "notes.append", Contract: "1.0.0", Effect: true, Idempotent: true,
			Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()}),
			Output: workflow.ObjectType(workflow.Field{Name: "entry_id", Type: workflow.StringType()})},
	)
	rt, err := workflowruntime.New(workflowruntime.Config{Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")),
		Catalog: catalog, Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}})
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.RegisterWorkspace(ctx, "Notes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetWorkspaceAccess(ctx, w.ID, 0, workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{
		{ID: "conn-notes", Resources: []string{"notebook:inbox", "notebook:digest"}}}}); err != nil {
		t.Fatal(err)
	}
	return store, rt, w
}

// The web management API lists and shows workflows, presents the approval
// card and approves exactly what it showed, as the owner from the web.
func TestWorkflowWebRoutesReviewAndApprove(t *testing.T) {
	ctx := context.Background()
	_, rt, w := workflowRuntimeForWeb(t)
	f, err := workflow.ParseFixtures([]byte(`{"inputs": {"day": "2026-10-05"}, "steps": {
		"read": {"output": {"notes": [{"id": "n1", "text": "x"}]}}, "summarize": {"output": {"digest": "d"}}, "write": {"output": {"entry_id": "e"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	card, err := rt.Propose(ctx, workflowruntime.Proposal{WorkspaceID: w.ID, Dir: filepath.Join("..", "workflow", "testdata", "notes-digest"), Fixtures: &f, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()

	resp := workflowPost(t, h, "list", map[string]any{"workspace_id": w.ID})
	var listed struct {
		Workflows []eviedb.WorkflowDefinitionState `json:"workflows"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || len(listed.Workflows) != 1 || listed.Workflows[0].Pending != 1 {
		t.Fatalf("list = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "diff", map[string]any{"proposal_id": card.Proposal.ID})
	var shown workflowruntime.Card
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &shown) != nil || !shown.NeedsApproval || shown.Proposal.ExecutableHash != card.Proposal.ExecutableHash || len(shown.Diff.Authority) != 2 {
		t.Fatalf("diff = %d %s", resp.Code, resp.Body)
	}
	approve := map[string]any{"proposal_id": card.Proposal.ID, "executable_hash": card.Proposal.ExecutableHash, "authority_hash": card.Proposal.AuthorityHash, "expected_version": 0}
	wrong := map[string]any{"proposal_id": card.Proposal.ID, "executable_hash": "sha256:" + strings.Repeat("0", 64), "authority_hash": card.Proposal.AuthorityHash, "expected_version": 0}
	if resp := workflowPost(t, h, "approve", wrong); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "hash_mismatch") {
		t.Fatalf("wrong hash = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "approve", approve)
	var activation eviedb.WorkflowActivation
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &activation) != nil || activation.Version != 1 || activation.Approver.Surface != "web" || activation.Approver.Actor != "owner" {
		t.Fatalf("approve = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "approve", approve); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "workflow_changed") {
		t.Fatalf("stale approval = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "show", map[string]any{"workspace_id": w.ID, "name": "notes-digest"})
	var state eviedb.WorkflowDefinitionState
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &state) != nil || state.Status != eviedb.WorkflowActive || len(state.Versions) != 1 {
		t.Fatalf("show = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "deactivate", map[string]any{"workspace_id": w.ID, "name": "notes-digest", "version": 1, "reason": "retired"}); resp.Code != http.StatusOK {
		t.Fatalf("deactivate = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "show", map[string]any{"workspace_id": w.ID, "name": "missing"}); resp.Code != http.StatusNotFound {
		t.Fatalf("missing definition = %d %s", resp.Code, resp.Body)
	}
}

// The workflow routes are management routes: POST with JSON from a
// loopback origin only, with exact, bounded request bodies.
func TestWorkflowWebRoutesAreGuarded(t *testing.T) {
	_, rt, w := workflowRuntimeForWeb(t)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	for _, path := range []string{"list", "show", "diff", "approve", "deactivate"} {
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
		"no hashes":       {"proposal_id": "p"},
		"no authority":    {"proposal_id": "p", "executable_hash": "sha256:" + strings.Repeat("0", 64)},
		"negative":        {"proposal_id": "p", "executable_hash": "sha256:" + strings.Repeat("0", 64), "authority_hash": "sha256:" + strings.Repeat("0", 64), "expected_version": -1},
		"unknown field":   {"proposal_id": "p", "executable_hash": "x", "authority_hash": "y", "expected_version": 0, "surface": "chat"},
		"approver chosen": {"proposal_id": "p", "executable_hash": "x", "authority_hash": "y", "expected_version": 0, "approver": map[string]string{"actor": "evie"}},
	} {
		if resp := workflowPost(t, h, "approve", body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
	if resp := workflowPost(t, h, "deactivate", map[string]any{"workspace_id": w.ID, "name": "notes-digest"}); resp.Code != http.StatusBadRequest {
		t.Fatalf("deactivate without version = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "diff", map[string]any{"proposal_id": "missing"}); resp.Code != http.StatusNotFound {
		t.Fatalf("missing proposal = %d %s", resp.Code, resp.Body)
	}
	// Without a Workflow Runtime the routes do not exist.
	bare := web.NewServer(nil).Handler()
	if resp := workflowPost(t, bare, "list", map[string]any{}); resp.Code == http.StatusOK {
		t.Fatalf("list without a runtime = %d", resp.Code)
	}
}
