package plugins_test

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
)

func buildManager(t *testing.T) *plugins.Manager {
	t.Helper()
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	supervisor, err := subagents.New(store, delegation.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store), plugins.NewSubagents(supervisor))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID, plugins.SubagentsPluginID} {
		if err = manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

func names(set tools.Toolset) []string {
	var out []string
	for _, schema := range set.Schemas() {
		out = append(out, schema.Function.Name)
	}
	return out
}

// Stage 11: the build preset is exactly the worktree-confined Kernel build
// tools (bash only where an OS sandbox works) plus the Todo capabilities
// its Task grant can use. It inherits nothing else: no memory, cron,
// finance, web, approval-gated or delegation tool.
func TestBuildPresetResolvesAndReopensOnlyFencedTools(t *testing.T) {
	manager := buildManager(t)
	selected, err := manager.ResolvePreset(plugins.BuildPresetID)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := manager.ResumeComposition(selected.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected.Receipt, reopened.Receipt) {
		t.Fatal("receipt changed on reopen")
	}
	var capabilities []string
	for _, c := range selected.Receipt.Capabilities {
		capabilities = append(capabilities, c.ID)
	}
	if selected.Receipt.Preset.ID != "build" || !slices.Equal(capabilities, delegation.BuildTaskCapabilityIDs) {
		t.Fatalf("build receipt = %+v", selected.Receipt)
	}
	kernel := []string{"read_file", "edit_file", "write_file"}
	if tools.WorkerSandboxAvailable() {
		kernel = append(kernel, "bash")
	}
	want := append(kernel, "todo_get", "todo_update", "todo_decompose", "todo_claim", "todo_release")
	for _, resolved := range []plugins.ResolvedComposition{selected, reopened} {
		if got := names(resolved.Toolset); !slices.Equal(got, want) {
			t.Fatalf("build tools = %v, want %v", got, want)
		}
		for _, name := range []string{"query_db", "edit_db", "cron_add", "todo_add", "todo_list", "memory_remember_literal", "finance_sync",
			"web_fetch", "web_search", "delegate_research", "delegate_explore", "delegate_build", "continue_research", "read_subagent_report"} {
			call := openrouter.ToolCall{ID: "forged", Type: "function"}
			call.Function.Name = name
			call.Function.Arguments = "{}"
			_, isErr, err := resolved.Toolset.ExecuteWithApprovalAuthorizedCompletion(context.Background(), call, nil, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !isErr {
				t.Fatalf("excluded %s executed", name)
			}
		}
	}
	if instructions, err := plugins.WorkerInstructions(plugins.BuildPresetID); err != nil || instructions != plugins.BuildInstructions {
		t.Fatalf("build instructions = %q, %v", instructions, err)
	}
	// Build children pin this version; changing the preset or its
	// instructions needs a historical definition.
	if got := selected.Receipt.Preset.Version; got != plugins.BuiltinBuildPreset().Version || got != "sha256:"+buildPresetDigest {
		t.Fatalf("build preset version changed: %s", got)
	}

	widened := composition.Clone(selected.Receipt)
	widened.ToolSchemas = append(widened.ToolSchemas, composition.ToolSchema{Name: "edit_db", SHA256: widened.ToolSchemas[0].SHA256})
	if _, err := manager.ResumeComposition(widened); err == nil {
		t.Fatal("widened build receipt reopened")
	}
	// The build worker cannot exist without its Task capabilities.
	if err := manager.SetEnabled(plugins.TodoPluginID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolvePreset(plugins.BuildPresetID); err == nil || !strings.Contains(err.Error(), "todo") {
		t.Fatalf("build preset resolved without Todo: %v", err)
	}
}

// Stage 11: without a sandbox, build workers are composed without bash, and
// a pinned receipt that has bash does not reopen where none is available.
func TestBuildPresetWithoutASandboxHasNoBash(t *testing.T) {
	manager := buildManager(t)
	withBash := tools.WorkerSandboxAvailable()
	var pinned composition.Receipt
	if withBash {
		selected, err := manager.ResolvePreset(plugins.BuildPresetID)
		if err != nil {
			t.Fatal(err)
		}
		pinned = selected.Receipt
	}
	t.Setenv("EVIE_WORKER_SANDBOX", "off")
	selected, err := manager.ResolvePreset(plugins.BuildPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(selected.Toolset); slices.Contains(got, "bash") || !slices.Contains(got, "write_file") {
		t.Fatalf("build tools without a sandbox = %v", got)
	}
	if _, err := manager.ResumeComposition(selected.Receipt); err != nil {
		t.Fatalf("a receipt without bash does not reopen: %v", err)
	}
	if withBash {
		if _, err := manager.ResumeComposition(pinned); err == nil || !strings.Contains(err.Error(), "sandbox") {
			t.Fatalf("a receipt with bash reopened without a sandbox: %v", err)
		}
	}
}

// Stage 11: the current Standard preset adds delegate_build after
// delegate_explore; the version before it stays resumable.
func TestStandardPresetOffersBuildDelegation(t *testing.T) {
	manager := buildManager(t)
	selected, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	got := names(selected.Toolset)
	explore, build := slices.Index(got, delegation.ExploreToolName), slices.Index(got, delegation.BuildToolName)
	if explore < 0 || build != explore+1 {
		t.Fatalf("standard tools = %v", got)
	}
	inspection, err := manager.ValidatePreset(plugins.BuildPresetID)
	if err != nil || !inspection.Valid {
		t.Fatalf("build preset inspection = %+v, %v", inspection, err)
	}
}

// buildPresetDigest pins the build preset version (Stage 11).
const buildPresetDigest = "e4622db864fb23e2d77b2e674306f9036a5b458298f63717bb08d2895ff7c655"
