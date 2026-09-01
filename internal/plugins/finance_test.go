package plugins

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type financeCapabilityAssociation struct {
	ID              CapabilityID
	ContractVersion string
	SchemaName      string
}

func TestFinanceManifestAndToolContractsAreStable(t *testing.T) {
	finance := NewFinance()
	want := Manifest{
		ID:                    FinancePluginID,
		ImplementationVersion: "1.0.0",
		KernelCompatibility: VersionRange{
			Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0",
		},
		Capabilities: []CapabilityContract{
			{ID: FinanceSyncCapabilityID, Version: "1.0.0"},
			{ID: FinanceRulesCapabilityID, Version: "1.0.0"},
			{ID: FinanceCategorizeCapabilityID, Version: "1.0.0"},
		},
	}
	if got := finance.Manifest(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Finance manifest\n got: %+v\nwant: %+v", got, want)
	}
	wantAssociations := []financeCapabilityAssociation{
		{ID: FinanceSyncCapabilityID, ContractVersion: "1.0.0", SchemaName: "finance_sync"},
		{ID: FinanceRulesCapabilityID, ContractVersion: "1.0.0", SchemaName: "finance_rules"},
		{ID: FinanceCategorizeCapabilityID, ContractVersion: "1.0.0", SchemaName: "finance_categorize"},
	}
	capabilities := finance.ToolCapabilities()
	gotAssociations := make([]financeCapabilityAssociation, len(capabilities))
	for i, capability := range capabilities {
		gotAssociations[i] = financeCapabilityAssociation{
			ID:              capability.ID,
			ContractVersion: capability.ContractVersion,
			SchemaName:      capability.Tool.Schema.Function.Name,
		}
	}
	if !reflect.DeepEqual(gotAssociations, wantAssociations) {
		t.Fatalf("Finance Capability associations\n got: %+v\nwant: %+v", gotAssociations, wantAssociations)
	}

	manager, err := NewManager(tools.NewToolset(nil), finance)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	if got, wantSchemas := toolset.Schemas(), tools.NewToolset(tools.FinanceTools()).Schemas(); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("Finance plugin schemas changed\n got: %#v\nwant: %#v", got, wantSchemas)
	}
	if got := schemaNames(toolset); !reflect.DeepEqual(got, []string{"finance_sync", "finance_rules", "finance_categorize"}) {
		t.Fatalf("Finance schema names = %v", got)
	}
}

func TestFinanceCanBeEnabledAndDisabledWithoutChangingWebOrKernel(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(WebPluginID, true); err != nil {
		t.Fatal(err)
	}

	webOnly, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"finance_sync", "finance_rules", "finance_categorize"} {
		assertUnknownTool(t, webOnly, name)
	}
	assertToolErrorContaining(t, webOnly, "web_search", `{"query":""}`, "query must not be empty")
	wantWebAndKernel := append(schemaNames(tools.KernelToolset()), "web_fetch", "web_search")
	if got := schemaNames(webOnly); !reflect.DeepEqual(got, wantWebAndKernel) {
		t.Fatalf("Web-only schema names = %v, want Kernel and Web schemas %v", got, wantWebAndKernel)
	}

	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	both, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"finance_sync", "finance_rules", "finance_categorize", "web_fetch", "web_search"} {
		if countSchema(both, name) != 1 {
			t.Fatalf("composed schema names = %v, want exactly one %s schema", schemaNames(both), name)
		}
	}

	if err := manager.SetEnabled(FinancePluginID, false); err != nil {
		t.Fatal(err)
	}
	afterDisable, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"finance_sync", "finance_rules", "finance_categorize"} {
		assertUnknownTool(t, afterDisable, name)
	}
	assertToolErrorContaining(t, afterDisable, "web_search", `{"query":""}`, "query must not be empty")
	if got := schemaNames(afterDisable); !reflect.DeepEqual(got, wantWebAndKernel) {
		t.Fatalf("post-disable schema names = %v, want Kernel and Web schemas %v", got, wantWebAndKernel)
	}

	inspection := manager.Inspect()
	if inspection.Degraded {
		t.Fatalf("inspection is degraded after a deliberate Finance disable: %+v", inspection)
	}
	wantStatuses := []PluginStatus{
		{ID: FinancePluginID, Enabled: false, State: StateDisabled},
		{ID: WebPluginID, Enabled: true, State: StateReady},
	}
	if !reflect.DeepEqual(inspection.Plugins, wantStatuses) {
		t.Fatalf("provider statuses\n got: %+v\nwant: %+v", inspection.Plugins, wantStatuses)
	}
}

type financeToolOutcome struct {
	Content string
	IsErr   bool
}

func TestFinanceManagerToolsetPreservesLegacyBehavior(t *testing.T) {
	manager, err := NewManager(tools.NewToolset(nil), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	managerToolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}

	legacy := exerciseFinanceToolset(t, tools.BuiltinToolset())
	composed := exerciseFinanceToolset(t, managerToolset)
	want := map[CapabilityID]financeToolOutcome{
		FinanceSyncCapabilityID: {
			Content: "tool call came back with error set PLAID_CLIENT_ID and PLAID_SECRET to your environment",
			IsErr:   true,
		},
		FinanceRulesCapabilityID: {
			Content: "Seeded rules from <HOME>/.finance/merchantLookup.json\n",
		},
		FinanceCategorizeCapabilityID: {
			Content: "1 categorized by rule, 0 uncategorized\n",
		},
	}
	if !reflect.DeepEqual(legacy, want) {
		t.Fatalf("legacy Finance behavior\n got: %+v\nwant: %+v", legacy, want)
	}
	if !reflect.DeepEqual(composed, legacy) {
		t.Fatalf("Manager-composed Finance behavior\n got: %+v\nlegacy: %+v", composed, legacy)
	}
}

func exerciseFinanceToolset(t *testing.T, toolset tools.Toolset) map[CapabilityID]financeToolOutcome {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PLAID_CLIENT_ID", "")
	t.Setenv("PLAID_SECRET", "")
	financeDir := filepath.Join(home, ".finance")
	if err := os.MkdirAll(financeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(financeDir, "merchantLookup.json"), []byte(`{"Coffee Shop":"Coffee"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	outcomes := make(map[CapabilityID]financeToolOutcome, 3)
	outcomes[FinanceRulesCapabilityID] = executeFinanceTool(t, toolset, "finance_rules", home)
	seedManagerFinanceTransaction(t)
	outcomes[FinanceCategorizeCapabilityID] = executeFinanceTool(t, toolset, "finance_categorize", home)
	outcomes[FinanceSyncCapabilityID] = executeFinanceTool(t, toolset, "finance_sync", home)
	return outcomes
}

func executeFinanceTool(t *testing.T, toolset tools.Toolset, name, home string) financeToolOutcome {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), financeToolCall(name, `{}`), nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("execute %q: %v", name, err)
	}
	return financeToolOutcome{
		Content: strings.ReplaceAll(message.Content, home, "<HOME>"),
		IsErr:   isErr,
	}
}

func seedManagerFinanceTransaction(t *testing.T) {
	t.Helper()
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items (item_id, access_token, linked_at) VALUES ('item-1', 'token', 'now')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO transactions
		(transaction_id, item_id, merchant_name, amount_cents)
		VALUES ('txn-1', 'item-1', 'Coffee Shop', 350)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWebAndFinanceCompositionDoesNotDependOnLoadOrder(t *testing.T) {
	tests := []struct {
		name        string
		compiled    []Plugin
		enableOrder []PluginID
	}{
		{
			name:        "Web then Finance",
			compiled:    []Plugin{NewWeb(), NewFinance()},
			enableOrder: []PluginID{WebPluginID, FinancePluginID},
		},
		{
			name:        "Finance then Web",
			compiled:    []Plugin{NewFinance(), NewWeb()},
			enableOrder: []PluginID{FinancePluginID, WebPluginID},
		},
	}
	wantNames := []string{
		"finance_sync", "finance_rules", "finance_categorize",
		"web_fetch", "web_search",
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("PLAID_CLIENT_ID", "")
			t.Setenv("PLAID_SECRET", "")
			manager, err := NewManager(tools.NewToolset(nil), tc.compiled...)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range tc.enableOrder {
				if err := manager.SetEnabled(id, true); err != nil {
					t.Fatal(err)
				}
			}
			toolset, err := manager.NewSessionToolset()
			if err != nil {
				t.Fatal(err)
			}
			if got := schemaNames(toolset); !reflect.DeepEqual(got, wantNames) {
				t.Fatalf("composed schema names = %v, want %v", got, wantNames)
			}
			assertToolErrorContaining(t, toolset, "finance_sync", `{}`, "set PLAID_CLIENT_ID and PLAID_SECRET")
			assertToolErrorContaining(t, toolset, "web_search", `{"query":""}`, "query must not be empty")
		})
	}
}

func assertToolErrorContaining(t *testing.T, toolset tools.Toolset, name, arguments, want string) {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(), financeToolCall(name, arguments), nil, nil, nil, nil,
	)
	if err != nil || !isErr || !strings.Contains(message.Content, want) {
		t.Fatalf("execute %q = (%+v, %v, %v), want tool error containing %q", name, message, isErr, err, want)
	}
}

func financeToolCall(name, arguments string) openrouter.ToolCall {
	return openrouter.ToolCall{
		ID: "call-finance", Type: "function",
		Function: openrouter.FunctionCall{Name: name, Arguments: arguments},
	}
}

func countSchema(toolset tools.Toolset, name string) int {
	count := 0
	for _, schema := range toolset.Schemas() {
		if schema.Function.Name == name {
			count++
		}
	}
	return count
}
