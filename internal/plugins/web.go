package plugins

import (
	"context"

	"github.com/davidadel66/evie/internal/tools"
)

const (
	WebPluginID           PluginID     = "web"
	WebFetchCapabilityID  CapabilityID = "web.fetch"
	WebSearchCapabilityID CapabilityID = "web.search"

	webImplementationVersion = "1.1.0"
	webContractVersion       = "1.0.0"
	webFetchContractVersion  = "1.1.0"
)

type Web struct{}

func NewWeb() Web { return Web{} }

func (Web) Start(context.Context) error { return nil }

func (Web) Stop(context.Context) error { return nil }

func (Web) Manifest() Manifest {
	return Manifest{
		ID:                    WebPluginID,
		ImplementationVersion: webImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: WebFetchCapabilityID, Version: webFetchContractVersion},
			{ID: WebSearchCapabilityID, Version: webContractVersion},
		},
		ResumableFrom: []ImplementationCompatibility{{ImplementationVersion: "1.0.0", Capabilities: []CapabilityCompatibility{
			{ID: WebFetchCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.WebTools()[0].Schema)},
			{ID: WebSearchCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: schemaHash(tools.WebTools()[1].Schema)},
		}}},
	}
}

func (Web) ToolCapabilities() []ToolCapability {
	webTools := tools.WebTools()
	return []ToolCapability{
		{ID: WebFetchCapabilityID, ContractVersion: webFetchContractVersion, Tool: tools.WebExcerptTool()},
		{ID: WebSearchCapabilityID, ContractVersion: webContractVersion, Tool: webTools[1]},
	}
}

func (Web) ResumableToolCapabilities(version string) []ToolCapability {
	if version != "1.0.0" {
		return nil
	}
	legacy := tools.WebTools()
	return []ToolCapability{
		{ID: WebFetchCapabilityID, ContractVersion: webContractVersion, Tool: legacy[0]},
		{ID: WebSearchCapabilityID, ContractVersion: webContractVersion, Tool: legacy[1]},
	}
}
