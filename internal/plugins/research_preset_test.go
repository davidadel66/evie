package plugins_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestResearchPresetResolvesAndReopensOnlyWeb(t *testing.T) {
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb())
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.SetEnabled(plugins.WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	selected, err := manager.ResolvePreset(plugins.ResearchPresetID)
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
	for _, resolved := range []plugins.ResolvedComposition{selected, reopened} {
		schemas := resolved.Toolset.Schemas()
		if len(schemas) != 2 || schemas[0].Function.Name != "web_search" || schemas[1].Function.Name != "web_fetch" {
			t.Fatalf("unexpected child schemas: %+v", schemas)
		}
		for _, name := range []string{"bash", "read_file", "query_db", "todo_list", "memory_search", "delegate_research"} {
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
	if err = manager.SetEnabled(plugins.WebPluginID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.ResolvePreset(plugins.ResearchPresetID); err == nil {
		t.Fatal("missing required Web accepted")
	}
	if _, err = manager.ResumeComposition(selected.Receipt); err == nil {
		t.Fatal("disabled receipt reopened")
	}
}
