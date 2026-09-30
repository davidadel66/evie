package plugins

import (
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
)

func TestWebManifestAndToolContractsAreStable(t *testing.T) {
	web := NewWeb()
	manifest := web.Manifest()
	want := Manifest{
		ID:                    WebPluginID,
		ImplementationVersion: "1.1.0",
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: WebFetchCapabilityID, Version: "1.1.0"},
			{ID: WebSearchCapabilityID, Version: "1.0.0"},
		},
		ResumableFrom: []ImplementationCompatibility{{ImplementationVersion: "1.0.0", Capabilities: []CapabilityCompatibility{
			{ID: WebFetchCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.WebTools()[0].Schema)},
			{ID: WebSearchCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.WebTools()[1].Schema)},
		}}},
	}
	if !reflect.DeepEqual(manifest, want) {
		t.Fatalf("Web manifest\n got: %+v\nwant: %+v", manifest, want)
	}

	manager, err := NewManager(tools.NewToolset(nil), web)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	if got, wantSchemas := toolset.Schemas(), tools.NewToolset([]tools.Tool{tools.WebExcerptTool(), tools.WebTools()[1]}).Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("Web plugin schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}
	if got := schemaNames(toolset); !reflect.DeepEqual(got, []string{"web_fetch", "web_search"}) {
		t.Fatalf("Web schema names = %v", got)
	}
}

type legacyWeb struct{ Web }

func (legacyWeb) Manifest() Manifest {
	m := NewWeb().Manifest()
	m.ImplementationVersion = "1.0.0"
	m.Capabilities[0].Version = "1.0.0"
	m.ResumableFrom = nil
	return m
}
func (legacyWeb) ToolCapabilities() []ToolCapability {
	return NewWeb().ResumableToolCapabilities("1.0.0")
}

func TestWebExcerptUpgradePreservesFrozenResearchReceipt(t *testing.T) {
	oldManager, err := NewManager(tools.NewToolset(nil), legacyWeb{})
	if err != nil {
		t.Fatal(err)
	}
	if err := oldManager.SetEnabled(WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	old, err := oldManager.ResolvePreset(ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(tools.NewToolset(nil), NewWeb())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.ResumeComposition(old.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, old.Receipt) || !reflect.DeepEqual(resumed.Toolset.Schemas(), old.Toolset.Schemas()) {
		t.Fatal("frozen Web receipt/schema changed")
	}
	current, err := manager.ResolvePreset(ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(current.Toolset.Schemas(), old.Toolset.Schemas()) {
		t.Fatal("new research did not receive excerpt contract")
	}
}
