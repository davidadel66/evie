package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

// exploreAdmission is a Global parent composed with explore delegation and
// the read_file, query_db and bash tools that bound it, in a live turn whose
// committed intent is delegate_explore (or tool, when given).
func exploreAdmission(t *testing.T, parentTools []string, tool string) (*eviedb.Store, string, delegation.Parent, composition.Receipt) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	manager, err := plugins.NewManager(tools.KernelToolset())
	if err != nil {
		t.Fatal(err)
	}
	explore, err := manager.ResolvePreset(plugins.ExplorePresetID)
	if err != nil {
		t.Fatal(err)
	}
	parentReceipt := composition.Clone(explore.Receipt)
	parentReceipt.Preset = composition.PresetIdentity{ID: "test-parent", Version: explore.Receipt.Preset.Version}
	parentReceipt.ToolSchemas = nil
	for _, name := range parentTools {
		parentReceipt.ToolSchemas = append(parentReceipt.ToolSchemas, composition.ToolSchema{Name: name, SHA256: explore.Receipt.ToolSchemas[0].SHA256})
	}
	for _, id := range []string{delegation.ExploreCapabilityID, delegation.ContinueCapabilityID} {
		parentReceipt.Capabilities = append(parentReceipt.Capabilities, composition.Capability{ID: id, ProviderID: "subagents", ContractVersion: "1.0.0", SchemaSHA256: explore.Receipt.ToolSchemas[0].SHA256})
	}
	parentReceipt.Providers = []composition.Provider{{ID: "subagents", ImplementationVersion: "1.0.0"}}
	parent, err := store.CreateGlobalSessionWithComposition(ctx, parentReceipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, parent.ID, "orchestrator", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, parent.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "explore"})
	if err != nil {
		t.Fatal(err)
	}
	if tool == "" {
		tool = delegation.ExploreToolName
	}
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call", Name: tool, Arguments: `{"assignments":[{"idempotency_key":"one","objective":"read the repository"}]}`}})
	intent, err := store.AppendEventWithLease(ctx, parent.ID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: root.ID, Type: memory.EventToolIntent, ExecutionID: "invocation", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	return store, path, delegation.Parent{Scope: parent.ScopeContext(), Lease: lease, SourceEventID: root.ID, IntentEventID: intent.ID}, explore.Receipt
}

func TestExploreAdmissionPinsItsRootAndRecoversLikeResearch(t *testing.T) {
	ctx := context.Background()
	store, _, parent, receipt := exploreAdmission(t, []string{"read_file", "query_db", "bash", "edit_file"}, "")
	folder, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := []delegation.Assignment{{Key: "one", Objective: "read the repository"}}
	policy := delegation.DefaultPolicy()
	// The explore receipt cannot be admitted as research, nor research as
	// explore, and a root is required.
	if _, err := store.AdmitSubagents(ctx, parent, request, receipt, policy, "dispatch"); err == nil {
		t.Fatal("explore receipt admitted through research admission")
	}
	if _, err := store.AdmitExploreSubagents(ctx, parent, request, []string{"relative"}, receipt, policy, "dispatch"); !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("relative root admitted: %v", err)
	}
	attempts, err := store.AdmitExploreSubagents(ctx, parent, request, []string{folder}, receipt, policy, "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a := attempts[0]
	if a.Root != folder || a.Worker() != delegation.WorkerExplore || a.Assignment.Path != "" {
		t.Fatalf("admitted explore attempt: %+v", a)
	}
	again, err := store.AdmitExploreSubagents(ctx, parent, request, []string{folder}, receipt, policy, "retry")
	if err != nil || again[0].ID != a.ID {
		t.Fatalf("retry did not retain the attempt: %+v %v", again, err)
	}
	// Abandoned by its parent turn, the attempt is interrupted, never run,
	// and its result names the explore worker.
	if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if n, err := store.RecoverSubagents(ctx); err != nil || n != 1 {
		t.Fatalf("recovered %d: %v", n, err)
	}
	recovered, err := store.InspectSubagent(ctx, parent, a.ID)
	if err != nil || recovered.State != "interrupted" || recovered.Result.Worker != delegation.WorkerExplore || recovered.Root != folder {
		t.Fatalf("recovered explore attempt: %+v %v", recovered, err)
	}
	if view := recovered.Result.ParentView(); view.Worker != delegation.WorkerExplore || len(view.Sources) != 0 {
		t.Fatalf("recovered parent view: %+v", view)
	}
}

func TestExploreAdmissionStaysWithinTheParentsComposition(t *testing.T) {
	ctx := context.Background()
	folder := t.TempDir()
	request := []delegation.Assignment{{Key: "one", Objective: "read the repository"}}
	policy := delegation.DefaultPolicy()

	// A parent without bash cannot hand bash to a worker.
	store, _, parent, receipt := exploreAdmission(t, []string{"read_file", "query_db"}, "")
	if _, err := store.AdmitExploreSubagents(ctx, parent, request, []string{folder}, receipt, policy, "dispatch"); err == nil || !strings.Contains(err.Error(), "bash") {
		t.Fatalf("explore beyond the parent's tools: %v", err)
	}

	// A worker receipt with one more tool than the explore preset is refused.
	store, _, parent, receipt = exploreAdmission(t, []string{"read_file", "query_db", "bash", "edit_file"}, "")
	widened := composition.Clone(receipt)
	widened.ToolSchemas = append(widened.ToolSchemas, composition.ToolSchema{Name: "edit_file", SHA256: receipt.ToolSchemas[0].SHA256})
	if _, err := store.AdmitExploreSubagents(ctx, parent, request, []string{folder}, widened, policy, "dispatch"); err == nil || !strings.Contains(err.Error(), "read-only explore tools") {
		t.Fatalf("widened explore receipt admitted: %v", err)
	}

	// A delegate_research intent cannot admit explore children.
	store, _, parent, receipt = exploreAdmission(t, []string{"read_file", "query_db", "bash"}, delegation.ToolName)
	if _, err := store.AdmitExploreSubagents(ctx, parent, request, []string{folder}, receipt, policy, "dispatch"); err == nil {
		t.Fatal("explore admitted under a delegate_research intent")
	}
}
