package plugins

import "github.com/davidadel66/evie/internal/tools"

const (
	WebPluginID           PluginID     = "web"
	WebFetchCapabilityID  CapabilityID = "web.fetch"
	WebSearchCapabilityID CapabilityID = "web.search"

	webImplementationVersion = "1.0.0"
	webContractVersion       = "1.0.0"
)

type Web struct{}

func NewWeb() Web { return Web{} }

func (Web) Manifest() Manifest {
	return Manifest{
		ID:                    WebPluginID,
		ImplementationVersion: webImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: WebFetchCapabilityID, Version: webContractVersion},
			{ID: WebSearchCapabilityID, Version: webContractVersion},
		},
	}
}

func (Web) ToolCapabilities() []ToolCapability {
	webTools := tools.WebTools()
	return []ToolCapability{
		{ID: WebFetchCapabilityID, ContractVersion: webContractVersion, Tool: webTools[0]},
		{ID: WebSearchCapabilityID, ContractVersion: webContractVersion, Tool: webTools[1]},
	}
}
