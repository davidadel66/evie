package plugins

import (
	"context"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/tools"
)

const (
	FinancePluginID               PluginID     = "finance"
	FinanceSyncCapabilityID       CapabilityID = "finance.sync"
	FinanceRulesCapabilityID      CapabilityID = "finance.rules"
	FinanceCategorizeCapabilityID CapabilityID = "finance.categorize"

	// 1.1.0 adds the workflow reads (Stage 14 sub-stage 5); its tools are
	// unchanged, so it declares it continues 1.0.0's pinned sessions.
	financeImplementationVersion = "1.1.0"
	financeContractVersion       = "1.0.0"
)

type Finance struct {
	// spending reads the local finance database for workflow reads.
	spending *finance.SpendingService
}

func NewFinance() Finance { return Finance{spending: finance.NewSpendingService()} }

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
			{ID: FinanceMonthCashFlowCapabilityID, Version: financeReadContractVersion},
			{ID: FinanceMonthTransactionsCapabilityID, Version: financeReadContractVersion},
		},
		ResumableFrom: []ImplementationCompatibility{{ImplementationVersion: "1.0.0", Capabilities: []CapabilityCompatibility{
			{ID: FinanceSyncCapabilityID, ContractVersion: financeContractVersion, SchemaSHA256: schemaHash(tools.FinanceSyncTool().Schema)},
			{ID: FinanceRulesCapabilityID, ContractVersion: financeContractVersion, SchemaSHA256: schemaHash(tools.FinanceRulesTool().Schema)},
			{ID: FinanceCategorizeCapabilityID, ContractVersion: financeContractVersion, SchemaSHA256: schemaHash(tools.FinanceCategorizeTool().Schema)},
		}}},
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
