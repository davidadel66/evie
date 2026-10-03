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

// Stage 7: the explore preset is exactly the read-only explore toolset,
// whatever plugins are enabled. It inherits nothing from the parent's base
// (no edit_file, edit_db, cron or approval-gated tool) and selects no plugin
// Capability (no Memory, Todo, Finance, Web or delegation).
func TestExplorePresetResolvesAndReopensOnlyReadOnlyTools(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	selected, err := manager.ResolvePreset(plugins.ExplorePresetID)
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
	if len(selected.Receipt.Capabilities) != 0 || len(selected.Receipt.Providers) != 0 || selected.Receipt.Preset.ID != "explore" {
		t.Fatalf("explore receipt selects plugin capabilities: %+v", selected.Receipt)
	}
	for _, resolved := range []plugins.ResolvedComposition{selected, reopened} {
		var names []string
		for _, schema := range resolved.Toolset.Schemas() {
			names = append(names, schema.Function.Name)
		}
		if !slices.Equal(names, delegation.ExploreToolNames) {
			t.Fatalf("explore tools = %v, want %v", names, delegation.ExploreToolNames)
		}
		for _, name := range []string{"edit_file", "edit_db", "cron_add", "cron_remove", "todo_add", "todo_update", "memory_remember_literal",
			"finance_sync", "web_fetch", "web_search", "delegate_research", "delegate_explore", "continue_research", "read_subagent_report"} {
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
	if instructions, err := plugins.WorkerInstructions(plugins.ExplorePresetID); err != nil || instructions != plugins.ExploreInstructions {
		t.Fatalf("explore instructions = %q, %v", instructions, err)
	}
	// Explore children pin this version; changing the preset or its
	// instructions needs a historical definition, as the Standard preset has.
	if got := selected.Receipt.Preset.Version; got != "sha256:17806556859b170434eabc067ad12e2dbe8fcf0c27085945c82f3db3d7ed1677" {
		t.Fatalf("explore preset version changed: %s", got)
	}

	// A receipt that claims more than the explore preset never reopens.
	widened := composition.Clone(selected.Receipt)
	widened.ToolSchemas = append(widened.ToolSchemas, composition.ToolSchema{Name: "edit_file", SHA256: widened.ToolSchemas[0].SHA256})
	if _, err := manager.ResumeComposition(widened); err == nil || !strings.Contains(err.Error(), "edit_file") {
		t.Fatalf("widened explore receipt reopened: %v", err)
	}
	withWeb := composition.Clone(selected.Receipt)
	research, err := manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	withWeb.Capabilities, withWeb.Providers = research.Receipt.Capabilities, research.Receipt.Providers
	if _, err := manager.ResumeComposition(withWeb); err == nil {
		t.Fatal("explore receipt with Web capabilities reopened")
	}
	// The explore preset needs no plugin: it resolves with everything off.
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.SubagentsPluginID} {
		if err = manager.SetEnabled(id, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.ResolvePreset(plugins.ExplorePresetID); err != nil {
		t.Fatalf("explore preset needs a plugin: %v", err)
	}
}
