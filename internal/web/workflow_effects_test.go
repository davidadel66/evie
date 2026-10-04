package web_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// unansweredLedger provides ledger.post, whose calls never answer in time:
// every outcome is unknown and only the owner can reconcile it.
type unansweredLedger struct{ contract workflow.CapabilityContract }

func (l unansweredLedger) WorkflowContract(id string) (workflow.CapabilityContract, bool) {
	return l.contract, id == l.contract.ID
}

func (l unansweredLedger) ResolveWorkflowCapability(_ context.Context, id string) (plugins.WorkflowProvider, error) {
	if id != l.contract.ID {
		return plugins.WorkflowProvider{}, fmt.Errorf("%w: %s", plugins.ErrWorkflowCapabilityUnknown, id)
	}
	return plugins.WorkflowProvider{Contract: l.contract, SchemaSHA256: l.contract.SchemaSHA256(), Plugin: "ledger", Version: "1.0.0",
		Perform: func(ctx context.Context, _ plugins.WorkflowEffectRequest) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}}, nil
}

// The owner reconciles an effect whose outcome is unknown from the web
// (Stage 14 sub-stage 6): the route names the run, the step, the outcome,
// the evidence and, for an effect that happened, its output; the surface
// is always the web. Malformed requests, unknown runs and reconciliations
// the run does not need are refused with their reason.
func TestWorkflowWebReconcilesAnUnknownEffect(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	contract := workflow.CapabilityContract{ID: "ledger.post", Contract: "1.0.0", Effect: true, Idempotent: true, ConflictKeys: "resource",
		Input:       workflow.ObjectType(workflow.Field{Name: "bill_id", Type: workflow.StringType()}, workflow.Field{Name: "amount_cents", Type: workflow.IntegerType()}),
		Output:      workflow.ObjectType(workflow.Field{Name: "entry_id", Type: workflow.StringType()}, workflow.Field{Name: "balance_cents", Type: workflow.IntegerType()}),
		Connections: []string{"conn-ledger"}, Resources: []string{"account:household"}}
	rt, err := workflowruntime.New(workflowruntime.Config{Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")),
		Capabilities: unansweredLedger{contract}, Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000},
		Scheduler: workflowruntime.SchedulerConfig{EffectTimeout: 20 * time.Millisecond}})
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.RegisterWorkspace(ctx, "Household")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetWorkspaceAccess(ctx, w.ID, 0, workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{
		{ID: "conn-ledger", Resources: []string{"account:household"}}}}); err != nil {
		t.Fatal(err)
	}
	fixtures, err := os.ReadFile(filepath.Join(webTestdata, "post-share.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	approveForWeb(t, rt, w.ID, "post-share", fixtures)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	resp := workflowPost(t, h, "start", map[string]any{"workspace_id": w.ID, "name": "post-share", "inputs": map[string]any{"bill_id": "dinner-1", "total_cents": 1000, "people": 4}})
	var started workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &started) != nil || started.Run.Run.State != eviedb.WorkflowRunNeedsReconciliation ||
		len(started.Run.Effects) != 1 || started.Run.Effects[0].State() != eviedb.WorkflowEffectUnknown {
		t.Fatalf("start = %d %s", resp.Code, resp.Body)
	}
	id := started.Run.Run.ID
	for name, body := range map[string]map[string]any{
		"no step":        {"run_id": id, "outcome": "happened"},
		"no outcome":     {"run_id": id, "step": "post"},
		"surface chosen": {"run_id": id, "step": "post", "outcome": "happened", "surface": "chat"},
		"long evidence":  {"run_id": id, "step": "post", "outcome": "did-not-happen", "evidence": strings.Repeat("x", 2001)},
	} {
		if resp := workflowPost(t, h, "reconcile", body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
	if resp := workflowPost(t, h, "reconcile", map[string]any{"run_id": "missing", "step": "post", "outcome": "happened"}); resp.Code != http.StatusNotFound {
		t.Fatalf("missing run = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "reconcile", map[string]any{"run_id": id, "step": "split", "outcome": "happened"}); resp.Code != http.StatusConflict ||
		!strings.Contains(resp.Body.String(), "reconcile_refused") {
		t.Fatalf("a step without an unknown effect = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "reconcile", map[string]any{"run_id": id, "step": "post", "outcome": "happened", "evidence": "the statement shows entry-1",
		"output": map[string]any{"entry_id": "entry-1", "balance_cents": 250}})
	var detail eviedb.WorkflowRunDetail
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &detail) != nil || detail.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("reconcile = %d %s", resp.Code, resp.Body)
	}
	r := detail.Effects[0].Receipt
	if r == nil || r.Source != eviedb.WorkflowEffectFromOwner || r.Surface != "web" || r.Evidence != "the statement shows entry-1" {
		t.Fatalf("receipt = %+v", r)
	}
	// The reconciliation is the answer to the effect's interruption
	// (sub-stage 7), recorded with the receipt.
	if len(detail.Interruptions) != 1 || detail.Interruptions[0].Resolution == nil || detail.Interruptions[0].Resolution.Surface != "web" ||
		string(detail.Interruptions[0].Resolution.Answer) != `{"evidence":"the statement shows entry-1","outcome":"happened","output":{"balance_cents":250,"entry_id":"entry-1"}}` {
		t.Fatalf("interruptions = %+v", detail.Interruptions)
	}
}
