package plugins

import (
	"context"

	"github.com/davidadel66/evie/internal/tools"
)

const (
	FinancePluginID               PluginID     = "finance"
	FinanceSyncCapabilityID       CapabilityID = "finance.sync"
	FinanceRulesCapabilityID      CapabilityID = "finance.rules"
	FinanceCategorizeCapabilityID CapabilityID = "finance.categorize"

	financeImplementationVersion = "1.0.0"
	financeContractVersion       = "1.0.0"
)

type Finance struct{}

func NewFinance() Finance { return Finance{} }

func (Finance) Start(context.Context) error { return nil }

func (Finance) Stop(context.Context) error { return nil }

func (Finance) Manifest() Manifest {
	return Manifest{
		ID:                    FinancePluginID,
		ImplementationVersion: financeImplementationVersion,
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: FinanceSyncCapabilityID, Version: financeContractVersion},
			{ID: FinanceRulesCapabilityID, Version: financeContractVersion},
			{ID: FinanceCategorizeCapabilityID, Version: financeContractVersion},
		},
	}
}

func (Finance) ToolCapabilities() []ToolCapability {
	return []ToolCapability{
		{
			ID: FinanceSyncCapabilityID, ContractVersion: financeContractVersion,
			Tool: tools.FinanceSyncTool(),
		},
		{
			ID: FinanceRulesCapabilityID, ContractVersion: financeContractVersion,
			Tool: tools.FinanceRulesTool(),
		},
		{
			ID: FinanceCategorizeCapabilityID, ContractVersion: financeContractVersion,
			Tool: tools.FinanceCategorizeTool(),
		},
	}
}
