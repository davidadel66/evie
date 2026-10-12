package plugins

import (
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
)

// A chat pinned to the Standard preset before the MCP tools (7f31ed96…)
// reopens with exactly its tools; a new chat gets mcp_list_tools and
// mcp_call, and a disabled MCP plugin only warns.
func TestPreMCPStandardReceiptResumesWithoutMCPTools(t *testing.T) {
	if got := canonicalPresetVersion(preMCPStandardPreset()); got != preMCPStandardPresetVersion ||
		preMCPStandardPresetVersion != "sha256:7f31ed96c814e676d51c7f9288cb0c15bca18352a2dc271d7b3894f3f20f14dd" {
		t.Fatalf("pre-MCP canonical version = %q, want frozen %q", got, preMCPStandardPresetVersion)
	}
	if got := canonicalPresetVersion(standardPresetContent()); got != StandardPresetVersion {
		t.Fatalf("standard canonical version = %q, want %q", got, StandardPresetVersion)
	}
	store, _ := readyStore(t)
	workflows := &WorkflowLink{}
	workflows.Bind(&fakeWorkflowSessions{})
	without, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(store), NewWorkflows(workflows))
	if err != nil {
		t.Fatal(err)
	}
	with, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(store), NewWorkflows(workflows), NewMCP(&MCPLink{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, manager := range []*Manager{without, with} {
		for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID, WorkflowsPluginID} {
			if err := manager.SetEnabled(id, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := with.SetEnabled(MCPPluginID, true); err != nil {
		t.Fatal(err)
	}
	legacy, err := without.resolvePreset(preMCPStandardPreset())
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := with.ResumeComposition(legacy.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) || !reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) ||
		countSchema(resumed.Toolset, MCPCallToolName) != 0 {
		t.Fatalf("resuming changed the pre-MCP receipt or its tools: %v", schemaNames(resumed.Toolset))
	}
	current, err := with.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Receipt.Preset.Version != StandardPresetVersion || countSchema(current.Toolset, MCPListToolsToolName) != 1 || countSchema(current.Toolset, MCPCallToolName) != 1 {
		t.Fatalf("current Standard tools = %v", schemaNames(current.Toolset))
	}
	// Without the MCP plugin, Standard still resolves with a warning.
	degraded, err := without.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if countSchema(degraded.Toolset, MCPCallToolName) != 0 || len(degraded.Receipt.Warnings) == 0 {
		t.Fatalf("Standard without MCP: tools %v, warnings %v", schemaNames(degraded.Toolset), degraded.Receipt.Warnings)
	}
}
