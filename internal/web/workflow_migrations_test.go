package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Stage 14 sub-stage 12: the owner migrates a run and runs the retention
// sweep from the web; the surface is always the web.
func TestWorkflowWebMigrateAndRetention(t *testing.T) {
	ctx := context.Background()
	store, rt, w := workflowRuntimeForWeb(t)
	splitFixtures, err := os.ReadFile(filepath.Join(webTestdata, "split-bill.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	approveForWeb(t, rt, w.ID, "split-bill", splitFixtures)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	queued, err := rt.StartRun(ctx, workflowruntime.RunRequest{WorkspaceID: w.ID, Workflow: "split-bill", Detach: true, Surface: "cli",
		TextInputs: map[string]string{"bill_id": "dinner-1", "total_cents": "1000", "people": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	id := queued.Run.Run.ID
	if resp := workflowPost(t, h, "migrate", map[string]any{"run_id": id}); resp.Code != http.StatusBadRequest {
		t.Fatalf("without a version = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "migrate", map[string]any{"run_id": id, "to_version": 2}); resp.Code != http.StatusConflict ||
		!strings.Contains(resp.Body.String(), "migration_refused") {
		t.Fatalf("without a newer version = %d %s", resp.Code, resp.Body)
	}
	// A newer version: the leftover step may retry.
	dir := filepath.Join(t.TempDir(), "split-bill")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"README.md", "workflow.yaml"} {
		data, err := os.ReadFile(filepath.Join(webTestdata, "split-bill", name))
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Replace(string(data), "  - id: leftover\n    kind: deterministic\n", "  - id: leftover\n    kind: deterministic\n    retry:\n      max_attempts: 2\n", 1)
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f, err := workflow.ParseFixtures(splitFixtures)
	if err != nil {
		t.Fatal(err)
	}
	card, err := rt.Propose(ctx, workflowruntime.Proposal{WorkspaceID: w.ID, Dir: dir, Fixtures: &f, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Approve(ctx, workflowruntime.Approval{ProposalID: card.Proposal.ID, ExecutableHash: card.Proposal.ExecutableHash,
		AuthorityHash: card.Proposal.AuthorityHash, ExpectedVersion: card.ExpectedVersion, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	resp := workflowPost(t, h, "migrate", map[string]any{"run_id": id, "to_version": 2, "reason": "new rules"})
	var migrated workflowruntime.MigrationResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &migrated) != nil || migrated.Migration.Surface != "web" ||
		migrated.Successor.Run.State != eviedb.WorkflowRunSucceeded || migrated.Predecessor.State != eviedb.WorkflowRunSuperseded {
		t.Fatalf("migrate = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "migrate", map[string]any{"run_id": id, "to_version": 2}); resp.Code != http.StatusConflict ||
		!strings.Contains(resp.Body.String(), "run_finished") {
		t.Fatalf("migrating again = %d %s", resp.Code, resp.Body)
	}

	// This runtime has no retention policy; one that does sweeps.
	if resp := workflowPost(t, h, "retention", map[string]any{}); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "retention_off") {
		t.Fatalf("retention without a policy = %d %s", resp.Code, resp.Body)
	}
	policy := workflowruntime.DefaultRetention()
	zero := time.Duration(0)
	policy.Ordinary.Compact = &zero
	swept, err := workflowruntime.New(workflowruntime.Config{Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")),
		Capabilities: workflowruntime.ContractsOnly(workflow.NewCatalog()), Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}, Retention: &policy})
	if err != nil {
		t.Fatal(err)
	}
	h = web.WithWorkflows(web.NewServer(nil), swept).Handler()
	resp = workflowPost(t, h, "retention/sweep", map[string]any{"dry_run": true})
	var dry workflowruntime.SweepResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &dry) != nil || !dry.DryRun {
		t.Fatalf("dry run = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "retention/sweep", map[string]any{"dry_run": "yes"}); resp.Code != http.StatusBadRequest {
		t.Fatalf("a malformed sweep = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "retention", map[string]any{})
	var status workflowruntime.RetentionStatus
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &status) != nil || status.Policies["ordinary"] == "" {
		t.Fatalf("status = %d %s", resp.Code, resp.Body)
	}
}
