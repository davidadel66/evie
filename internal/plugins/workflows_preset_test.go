package plugins

import (
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
)

// A chat pinned to the Standard preset before the workflow tools (377b51c0…)
// reopens with exactly its tools; a new chat gets workflow_start and
// workflow_status, and a disabled Workflows plugin only warns.
func TestPreWorkflowsStandardReceiptResumesWithoutWorkflowTools(t *testing.T) {
	if got := canonicalPresetVersion(preWorkflowsStandardPreset()); got != preWorkflowsStandardPresetVersion ||
		preWorkflowsStandardPresetVersion != "sha256:377b51c09706fba7741ad0f6f0e9ba8281921ba6b0043b8f9985a5c31966b3c0" {
		t.Fatalf("pre-workflows canonical version = %q, want frozen %q", got, preWorkflowsStandardPresetVersion)
	}
	if got := canonicalPresetVersion(standardPresetContent()); got != StandardPresetVersion {
		t.Fatalf("standard canonical version = %q, want %q", got, StandardPresetVersion)
	}
	store, _ := readyStore(t)
	link := &WorkflowLink{}
	link.Bind(&fakeWorkflowSessions{})
	manager := todoManager(t, NewTodo(store))
	withWorkflows, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(store), NewWorkflows(link))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID, WorkflowsPluginID} {
		if err := withWorkflows.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := manager.resolvePreset(preWorkflowsStandardPreset())
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := withWorkflows.ResumeComposition(legacy.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) || !reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) ||
		countSchema(resumed.Toolset, WorkflowStartToolName) != 0 {
		t.Fatalf("resuming changed the pre-workflows receipt or its tools: %v", schemaNames(resumed.Toolset))
	}
	current, err := withWorkflows.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Receipt.Preset.Version != StandardPresetVersion || countSchema(current.Toolset, WorkflowStartToolName) != 1 || countSchema(current.Toolset, WorkflowStatusToolName) != 1 {
		t.Fatalf("current Standard tools = %v", schemaNames(current.Toolset))
	}
	// Every workflow tool in the Standard composition is one of the two:
	// none proposes, approves, cancels, answers or reconciles.
	for _, name := range schemaNames(current.Toolset) {
		if strings.HasPrefix(name, "workflow") && name != WorkflowStartToolName && name != WorkflowStatusToolName {
			t.Fatalf("unexpected workflow tool %s", name)
		}
	}
	if err := withWorkflows.SetEnabled(WorkflowsPluginID, false); err != nil {
		t.Fatal(err)
	}
	disabled, err := withWorkflows.ResolvePreset(StandardPresetID)
	if err != nil || countSchema(disabled.Toolset, WorkflowStartToolName) != 0 || len(disabled.Warnings) == 0 {
		t.Fatalf("disabled Workflows: %v, tools %v", err, schemaNames(disabled.Toolset))
	}
}
