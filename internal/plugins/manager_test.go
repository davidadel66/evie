package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type fakePlugin struct {
	manifest     Manifest
	capabilities []ToolCapability
}

func (p fakePlugin) Manifest() Manifest { return p.manifest }

func (p fakePlugin) Start(context.Context) error { return nil }

func (p fakePlugin) Stop(context.Context) error { return nil }

func (p fakePlugin) ToolCapabilities() []ToolCapability { return p.capabilities }

func fakeToolPlugin(pluginID, capabilityID, toolName, result string) fakePlugin {
	const version = "1.0.0"
	return fakePlugin{
		manifest: Manifest{
			ID:                    PluginID(pluginID),
			ImplementationVersion: version,
			KernelCompatibility: VersionRange{
				Minimum: "1.0.0", MaximumExclusive: "2.0.0",
			},
			Capabilities: []CapabilityContract{{ID: CapabilityID(capabilityID), Version: version}},
		},
		capabilities: []ToolCapability{{
			ID:              CapabilityID(capabilityID),
			ContractVersion: version,
			Tool: tools.Tool{
				Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{
					Name: toolName, Parameters: openrouter.Parameter{Type: "object"},
				}},
				Execute: func(context.Context, string) (string, error) { return result, nil },
			},
		}},
	}
}

func TestManagerComposesEnabledPluginsIntoNewSessionToolsets(t *testing.T) {
	plugin := fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "enabled result")
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}

	disabled, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	if len(disabled.Schemas()) != 0 {
		t.Fatalf("disabled schemas = %+v, want none", disabled.Schemas())
	}
	assertUnknownTool(t, disabled, "fixture_echo")

	if err := manager.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}
	enabled, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaNames(enabled); strings.Join(got, ",") != "fixture_echo" {
		t.Fatalf("enabled schemas = %v, want fixture_echo", got)
	}
	assertToolResult(t, enabled, "fixture_echo", "enabled result")

	if err := manager.SetEnabled("fixture", false); err != nil {
		t.Fatal(err)
	}
	afterDisable, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownTool(t, afterDisable, "fixture_echo")
	assertToolResult(t, enabled, "fixture_echo", "enabled result")
}

func TestManagerRejectsDuplicatePluginAndCapabilityIDs(t *testing.T) {
	tests := []struct {
		name    string
		plugins []Plugin
		want    string
	}{
		{
			name: "plugin ID",
			plugins: []Plugin{
				fakeToolPlugin("same", "same.first", "first", "first"),
				fakeToolPlugin("same", "same.second", "second", "second"),
			},
			want: `duplicate plugin ID "same"`,
		},
		{
			name: "Capability ID",
			plugins: []Plugin{
				fakeToolPlugin("same", "same.duplicate", "first", "first"),
				fakeToolPlugin("other", "same.duplicate", "second", "second"),
			},
			want: `duplicate Capability ID "same.duplicate"`,
		},
	}

	for _, tc := range tests {
		for _, order := range []string{"forward", "reverse"} {
			t.Run(tc.name+"/"+order, func(t *testing.T) {
				plugins := append([]Plugin(nil), tc.plugins...)
				if order == "reverse" {
					plugins[0], plugins[1] = plugins[1], plugins[0]
				}
				_, err := NewManager(tools.NewToolset(nil), plugins...)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("NewManager error = %v, want containing %q", err, tc.want)
				}
			})
		}
	}
}

func TestManagerChecksKernelCompatibilityRange(t *testing.T) {
	tests := []struct {
		name    string
		range_  VersionRange
		wantErr string
	}{
		{
			name:   "current version equals inclusive minimum",
			range_: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		},
		{
			name:   "current version is below exclusive maximum",
			range_: VersionRange{Minimum: "0.9.9", MaximumExclusive: "1.0.1"},
		},
		{
			name:    "current version below minimum",
			range_:  VersionRange{Minimum: "1.0.1", MaximumExclusive: "2.0.0"},
			wantErr: `plugin "fixture" is incompatible with Kernel API 1.0.0`,
		},
		{
			name:    "current version equals exclusive maximum",
			range_:  VersionRange{Minimum: "0.9.0", MaximumExclusive: "1.0.0"},
			wantErr: `plugin "fixture" is incompatible with Kernel API 1.0.0`,
		},
		{
			name:    "current version above maximum",
			range_:  VersionRange{Minimum: "0.8.0", MaximumExclusive: "0.9.0"},
			wantErr: `plugin "fixture" is incompatible with Kernel API 1.0.0`,
		},
		{
			name:    "empty range",
			range_:  VersionRange{Minimum: "1.0.0", MaximumExclusive: "1.0.0"},
			wantErr: `plugin "fixture" has invalid Kernel compatibility range`,
		},
		{
			name:    "invalid version grammar",
			range_:  VersionRange{Minimum: "v1.0", MaximumExclusive: "2.0.0"},
			wantErr: `plugin "fixture" has invalid Kernel compatibility minimum "v1.0"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plugin := fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "result")
			plugin.manifest.KernelCompatibility = tc.range_
			manager, err := NewManager(tools.NewToolset(nil), plugin)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("NewManager error = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewManager: %v", err)
			}
			if err := manager.SetEnabled("fixture", true); err != nil {
				t.Fatal(err)
			}
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			assertToolResult(t, toolset, "fixture_echo", "result")
		})
	}
}

func TestManagerRejectsCapabilityOutsidePluginNamespace(t *testing.T) {
	plugin := fakeToolPlugin("fixture", "other.echo", "fixture_echo", "result")
	_, err := NewManager(tools.NewToolset(nil), plugin)
	if err == nil || !strings.Contains(err.Error(), `Capability ID "other.echo" is not namespaced by plugin "fixture"`) {
		t.Fatalf("NewManager error = %v", err)
	}
}

func TestManagerRejectsMalformedCapabilityContractVersionBeforeStart(t *testing.T) {
	plugin := fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "result")
	plugin.manifest.Capabilities[0].Version = "banana"
	plugin.capabilities[0].ContractVersion = "banana"

	_, err := NewManager(tools.NewToolset(nil), plugin)
	if err == nil || !strings.Contains(err.Error(), `Capability "fixture.echo" has invalid contract version "banana"`) {
		t.Fatalf("NewManager error = %v, want malformed Capability Contract rejection", err)
	}
}

func TestManagerRejectsNonCanonicalPluginIDs(t *testing.T) {
	for _, id := range []string{"fixture.plugin", "Fixture", " fixture", "fixture "} {
		t.Run(id, func(t *testing.T) {
			plugin := fakeToolPlugin(id, id+".echo", "fixture_echo", "result")
			_, err := NewManager(tools.NewToolset(nil), plugin)
			if err == nil || !strings.Contains(err.Error(), "has invalid ID") {
				t.Fatalf("NewManager error = %v, want canonical plugin ID rejection", err)
			}
		})
	}
}

func TestManagerRejectsMalformedCapabilityIDs(t *testing.T) {
	for _, id := range []string{"fixture.Echo", "fixture. echo", "fixture.", "fixture..echo", "fixture.echo."} {
		t.Run(id, func(t *testing.T) {
			plugin := fakeToolPlugin("fixture", id, "fixture_echo", "result")
			_, err := NewManager(tools.NewToolset(nil), plugin)
			if err == nil || !strings.Contains(err.Error(), "namespaced by plugin") {
				t.Fatalf("NewManager error = %v, want canonical Capability ID rejection", err)
			}
		})
	}
}

func TestManagerAcceptsCanonicalNamespacedIdentityGrammar(t *testing.T) {
	plugin := fakeToolPlugin(
		"fixture-plugin_2",
		"fixture-plugin_2.echo-detail_v2.extended",
		"fixture_echo",
		"result",
	)
	if _, err := NewManager(tools.NewToolset(nil), plugin); err != nil {
		t.Fatalf("NewManager rejected canonical identities: %v", err)
	}
}

func assertToolResult(t *testing.T, toolset tools.Toolset, name, want string) {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), toolCall(name), nil, nil, nil, nil,
	)
	if err != nil || isErr || message.Content != want {
		t.Fatalf("execute %q = (%+v, %v, %v), want %q", name, message, isErr, err, want)
	}
}

func assertUnknownTool(t *testing.T, toolset tools.Toolset, name string) {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), toolCall(name), nil, nil, nil, nil,
	)
	if err != nil || !isErr || !strings.Contains(strings.ToLower(message.Content), "unknown tool") {
		t.Fatalf("execute absent %q = (%+v, %v, %v), want unknown tool", name, message, isErr, err)
	}
}

func schemaNames(toolset tools.Toolset) []string {
	schemas := toolset.Schemas()
	names := make([]string, len(schemas))
	for i, schema := range schemas {
		names[i] = schema.Function.Name
	}
	return names
}

func toolCall(name string) openrouter.ToolCall {
	return openrouter.ToolCall{
		ID: "call-1", Type: "function",
		Function: openrouter.FunctionCall{Name: name, Arguments: `{}`},
	}
}
