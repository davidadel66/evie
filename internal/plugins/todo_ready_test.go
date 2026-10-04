package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
	"github.com/davidadel66/evie/internal/tools"
)

// Stage 13 (2026-10-04): todo_ready through the Todo Plugin, and sessions
// pinned before it.

// preReadyTodoPlugin is the Todo Plugin as released before Stage 13.
type preReadyTodoPlugin struct{ service task.Service }

func (preReadyTodoPlugin) Start(context.Context) error { return nil }
func (preReadyTodoPlugin) Stop(context.Context) error  { return nil }
func (preReadyTodoPlugin) Manifest() Manifest {
	return Manifest{
		ID: TodoPluginID, ImplementationVersion: "1.7.0",
		KernelCompatibility: VersionRange{Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0"},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: "1.3.0"}, {ID: TodoAddCapabilityID, Version: "1.4.0"},
			{ID: TodoGetCapabilityID, Version: "1.1.0"}, {ID: TodoUpdateCapabilityID, Version: "1.3.0"},
			{ID: TodoDecomposeCapabilityID, Version: "1.0.0"}, {ID: TodoClaimCapabilityID, Version: "1.0.0"},
			{ID: TodoReleaseCapabilityID, Version: "1.0.0"},
		},
	}
}
func (p preReadyTodoPlugin) ToolCapabilities() []ToolCapability {
	return NewTodo(p.service).ResumableToolCapabilities("1.7.0")
}

func readyStore(t *testing.T) (*eviedb.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return eviedb.NewStore(db), path
}

func todoManager(t *testing.T, todo Plugin) *Manager {
	t.Helper()
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), todo)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

// A chat pinned to the Standard preset before todo_ready (6566fc2a…, Todo
// 1.7.0) reopens with exactly its tools: no todo_ready, and add, update and
// decompose without plan fields, which it refuses as unknown. Its earlier
// update replays unchanged.
func TestPreReadyStandardReceiptResumesWithExactTodoTools(t *testing.T) {
	if got := canonicalPresetVersion(preReadyStandardPreset()); got != preReadyStandardPresetVersion {
		t.Fatalf("pre-ready canonical version = %q, want %q", got, preReadyStandardPresetVersion)
	}
	store, _ := readyStore(t)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{ActorID: "local", SessionID: string(session.ID), RunID: "frozen"})
	legacy, err := todoManager(t, preReadyTodoPlugin{service: store}).resolvePreset(preReadyStandardPreset())
	if err != nil {
		t.Fatal(err)
	}
	root := executeTodoToolContext(t, ctx, legacy.Toolset, "todo_add", `{"title":"Earlier tree","idempotency_key":"root"}`)
	var created task.Task
	if root.IsErr || json.Unmarshal([]byte(root.Content), &created) != nil {
		t.Fatalf("earlier add = %+v", root)
	}
	before := executeTodoToolContext(t, ctx, legacy.Toolset, "todo_update", `{"task_id":"`+string(created.ID)+`","expected_revision":1,"title":"Renamed","idempotency_key":"rename"}`)
	if before.IsErr {
		t.Fatalf("earlier update = %+v", before)
	}
	resumed, err := todoManager(t, NewTodo(store)).ResumeComposition(legacy.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) || !reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) {
		t.Fatal("resuming changed the pre-ready receipt or its tools")
	}
	if len(resumed.CompatibilityResolutions) != 1 || resumed.CompatibilityResolutions[0].OriginalProvider.ImplementationVersion != "1.7.0" ||
		resumed.CompatibilityResolutions[0].ReplacementImplementationVersion != "1.8.0" || countSchema(resumed.Toolset, "todo_ready") != 0 {
		t.Fatalf("pre-ready compatibility = %+v, tools %v", resumed.CompatibilityResolutions, schemaNames(resumed.Toolset))
	}
	if replayed := executeTodoToolContext(t, ctx, resumed.Toolset, "todo_update", `{"task_id":"`+string(created.ID)+`","expected_revision":1,"title":"Renamed","idempotency_key":"rename"}`); replayed.IsErr || replayed.Content != before.Content {
		t.Fatalf("replayed earlier update = %+v, want %s", replayed, before.Content)
	}
	refused := executeTodoToolContext(t, ctx, resumed.Toolset, "todo_update", `{"task_id":"`+string(created.ID)+`","expected_revision":2,"depends_on":[],"idempotency_key":"plan"}`)
	if !refused.IsErr || !strings.Contains(refused.Content, `unknown field "depends_on"`) {
		t.Fatalf("pre-ready update with depends_on = %+v", refused)
	}
	// A new chat gets the planned tools and todo_ready.
	current, err := todoManager(t, NewTodo(store)).ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if countSchema(current.Toolset, "todo_ready") != 1 || current.Receipt.Preset.Version != StandardPresetVersion {
		t.Fatalf("current Standard tools = %v", schemaNames(current.Toolset))
	}
}

// todo_ready through the Plugin: planned decomposition, then the ready read
// model sized to the configured limits and counted against the invoking
// turn.
func TestTodoReadyReadsTheTreeThroughThePlugin(t *testing.T) {
	store, _ := readyStore(t)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, "turn", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{ActorID: "local", SessionID: string(session.ID), RunID: "turn",
		LeaseHolderID: string(lease.HolderID), LeaseToken: uint64(lease.FencingToken), LeaseGeneration: uint64(lease.Generation)})
	ctx = tools.WithInvocationContext(ctx, tools.InvocationContext{Scope: memory.ScopeContext{SessionID: session.ID}, SourceEventID: "source"})
	manager := todoManager(t, NewTodo(store).WithDispatchLimits(task.DispatchLimits{MaxBatch: 8, PerParent: 3, Runtime: 4, PerTurn: 16}))
	resolved, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	root := executeTodoToolContext(t, ctx, resolved.Toolset, "todo_add", `{"title":"Feature","idempotency_key":"root"}`)
	var created task.Task
	if root.IsErr || json.Unmarshal([]byte(root.Content), &created) != nil {
		t.Fatalf("add = %+v", root)
	}
	decomposed := executeTodoToolContext(t, ctx, resolved.Toolset, "todo_decompose", `{"task_id":"`+string(created.ID)+`","expected_revision":1,"idempotency_key":"plan","children":[
		{"title":"A","paths":["pkg/a"]},{"title":"B","paths":["pkg/b"]},{"title":"C","paths":["pkg/c"],"depends_on_children":[0,1]},{"title":"D","paths":["pkg/a/extra.go"]}]}`)
	var tree task.Decomposition
	if decomposed.IsErr || json.Unmarshal([]byte(decomposed.Content), &tree) != nil || !reflect.DeepEqual(tree.Children[2].DependsOn, []task.ID{tree.Children[0].ID, tree.Children[1].ID}) {
		t.Fatalf("planned decomposition = %+v", decomposed)
	}
	ready := executeTodoToolContext(t, ctx, resolved.Toolset, "todo_ready", `{"tree_id":"`+string(created.ID)+`"}`)
	var readiness task.Readiness
	if ready.IsErr || json.Unmarshal([]byte(ready.Content), &readiness) != nil {
		t.Fatalf("todo_ready = %+v", ready)
	}
	if readiness.Capacity.BatchSize != 3 || readiness.Capacity.PerConversation != 3 || len(readiness.Batches) != 2 ||
		len(readiness.Batches[0]) != 2 || readiness.Batches[1][0].TaskID != tree.Children[3].ID || readiness.Blocked[0].TaskID != tree.Children[2].ID {
		t.Fatalf("readiness = %s", ready.Content)
	}
	for _, bad := range []string{`{}`, `{"tree_id":"` + string(created.ID) + `","max":-1}`, `{"tree_id":"x","extra":1}`} {
		if refused := executeTodoToolContext(t, ctx, resolved.Toolset, "todo_ready", bad); !refused.IsErr {
			t.Errorf("todo_ready %s = %+v", bad, refused)
		}
	}
}
