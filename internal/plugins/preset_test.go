package plugins

import (
	"reflect"
	"strings"
	"testing"

	kernelcomposition "github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/tools"
)

func TestStandardPresetComposesOnlyItsPinnedCapabilities(t *testing.T) {
	if got := canonicalPresetVersion(standardPresetContent()); got != StandardPresetVersion {
		t.Fatalf("standard canonical version = %q, want reserved %q", got, StandardPresetVersion)
	}
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}

	composition, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	if PresetID(composition.Receipt.Preset.ID) != StandardPresetID ||
		composition.Receipt.Preset.Version != StandardPresetVersion {
		t.Fatalf("preset identity = %+v, want %s@%s", composition.Receipt.Preset, StandardPresetID, StandardPresetVersion)
	}
	if composition.Receipt.EvieVersion != EvieVersion {
		t.Fatalf("Evie version = %q, want %q", composition.Receipt.EvieVersion, EvieVersion)
	}
	wantSchemas := schemaNames(
		tools.KernelToolset().WithTools(tools.FinanceTools()).WithTools(tools.WebTools()),
	)
	if got := schemaNames(composition.Toolset); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("standard schemas = %v, want %v", got, wantSchemas)
	}

	standard := BuiltinStandardPreset()
	standard.RequiredCapabilities[0].ID = "changed.capability"
	again := BuiltinStandardPreset()
	if again.RequiredCapabilities[0].ID == "changed.capability" {
		t.Fatal("caller mutated immutable built-in standard preset")
	}
	if _, err := manager.ResolvePreset("missing"); err == nil ||
		!strings.Contains(err.Error(), `Agent Preset "missing" is not allowed`) {
		t.Fatalf("unknown explicit preset error = %v", err)
	}
}

func TestPresetResolutionFailsRequiredAndPersistsOptionalDiagnostics(t *testing.T) {
	required := fakeToolPlugin("required", "required.echo", "required_echo", "required")
	optional := fakeToolPlugin("optional", "optional.echo", "optional_echo", "optional")
	manager, err := NewManager(tools.NewToolset(nil), required, optional)
	if err != nil {
		t.Fatal(err)
	}
	preset := Preset{
		ID:      "fixture",
		Version: "sha256:3df4b83c2a4f7b40c88c1a78237f622644487aa118f47d2194d637c452432bbd",
		RequiredCapabilities: []CapabilityRequirement{{
			ID: "required.echo", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		}},
		OptionalCapabilities: []CapabilityRequirement{{
			ID: "optional.echo", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		}},
	}

	if _, err := manager.resolvePreset(preset); err == nil ||
		!strings.Contains(err.Error(), `required Capability "required.echo"`) ||
		!strings.Contains(err.Error(), `enable plugin "required"`) {
		t.Fatalf("missing required error = %v", err)
	}
	if err := manager.SetEnabled("required", true); err != nil {
		t.Fatal(err)
	}
	incompatible := preset
	incompatible.RequiredCapabilities = append([]CapabilityRequirement(nil), preset.RequiredCapabilities...)
	incompatible.RequiredCapabilities[0].Compatibility = VersionRange{Minimum: "2.0.0", MaximumExclusive: "3.0.0"}
	if _, err := manager.resolvePreset(incompatible); err == nil ||
		!strings.Contains(err.Error(), "outside required range [2.0.0,3.0.0)") {
		t.Fatalf("incompatible required Capability error = %v", err)
	}
	composition, err := manager.resolvePreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	if len(composition.Warnings) != 1 ||
		!strings.Contains(composition.Warnings[0], `optional Capability "optional.echo"`) {
		t.Fatalf("warnings = %v", composition.Warnings)
	}
	wantReceiptWarnings := []CompositionWarning{{
		Code: kernelcomposition.WarningProviderDisabled, CapabilityID: "optional.echo", ProviderID: "optional",
	}}
	if !reflect.DeepEqual(composition.Receipt.Warnings, wantReceiptWarnings) {
		t.Fatalf("receipt warnings = %v, want %v", composition.Receipt.Warnings, wantReceiptWarnings)
	}
	assertToolResult(t, composition.Toolset, "required_echo", "required")
	assertUnknownTool(t, composition.Toolset, "optional_echo")

	if err := manager.SetEnabled("optional", true); err != nil {
		t.Fatal(err)
	}
	newComposition, err := manager.resolvePreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, newComposition.Toolset, "optional_echo", "optional")
	assertUnknownTool(t, composition.Toolset, "optional_echo")
}

func TestPresetValidityIgnoresConnectionReadiness(t *testing.T) {
	manager, err := NewManager(
		tools.NewToolset(nil),
		fakeToolPlugin("account", "account.read", "account_read", "connect account"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled("account", true); err != nil {
		t.Fatal(err)
	}
	preset := fixturePreset("account.read")
	preset.Configuration = []ConfigurationReference{{
		Kind: ConfigurationConnection, ID: "c9299e6b-81db-47df-8ee7-d5d9a5f9e94f",
	}}
	composition, err := manager.resolvePreset(preset)
	if err != nil {
		t.Fatalf("connection readiness invalidated preset: %v", err)
	}
	assertToolResult(t, composition.Toolset, "account_read", "connect account")
	if !reflect.DeepEqual(composition.Receipt.Configuration, preset.Configuration) {
		t.Fatalf("missing Connection ID receipt refs = %#v, want %#v", composition.Receipt.Configuration, preset.Configuration)
	}
}

func fixturePreset(required CapabilityID) Preset {
	return Preset{
		ID:      "fixture",
		Version: "sha256:3df4b83c2a4f7b40c88c1a78237f622644487aa118f47d2194d637c452432bbd",
		RequiredCapabilities: []CapabilityRequirement{{
			ID: required, Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		}},
	}
}

func TestReceiptRejectsContentThatCouldCarryCredentials(t *testing.T) {
	preset := fixturePreset("fixture.echo")
	preset.Configuration = []ConfigurationReference{{Kind: ConfigurationConnection, ID: "sk-live-secret-token"}}
	manager, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.resolvePreset(preset); err == nil || !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("credential-shaped configuration error = %v", err)
	}
}

func TestResumeCompositionRequiresEveryExactPinnedProviderAndSchema(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	pinned, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}

	changedProvider := pinned.Receipt
	changedProvider.Providers = append([]ProviderReceipt(nil), pinned.Receipt.Providers...)
	changedProvider.Providers[0].ImplementationVersion = "1.0.1"
	if _, err := manager.ResumeComposition(changedProvider); err == nil ||
		!strings.Contains(err.Error(), "does not match the exact loaded providers") {
		t.Fatalf("changed provider resume error = %v", err)
	}

	changedSchema := pinned.Receipt
	changedSchema.Capabilities = append([]CapabilityReceipt(nil), pinned.Receipt.Capabilities...)
	changedSchema.Capabilities[0].SchemaSHA256 = strings.Repeat("0", 64)
	if _, err := manager.ResumeComposition(changedSchema); err == nil ||
		!strings.Contains(err.Error(), "does not match the exact loaded providers") {
		t.Fatalf("changed schema resume error = %v", err)
	}

	if err := manager.SetEnabled(FinancePluginID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResumeComposition(pinned.Receipt); err == nil ||
		!strings.Contains(err.Error(), `required Capability "finance.sync"`) {
		t.Fatalf("missing pinned provider resume error = %v", err)
	}
}
