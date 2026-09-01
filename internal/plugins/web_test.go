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
		ImplementationVersion: "1.0.0",
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: WebFetchCapabilityID, Version: "1.0.0"},
			{ID: WebSearchCapabilityID, Version: "1.0.0"},
		},
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
	if got, wantSchemas := toolset.Schemas(), tools.NewToolset(tools.WebTools()).Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("Web plugin schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}
	if got := schemaNames(toolset); !reflect.DeepEqual(got, []string{"web_fetch", "web_search"}) {
		t.Fatalf("Web schema names = %v", got)
	}
}
