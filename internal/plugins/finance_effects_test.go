package plugins

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/tools"
)

// Finance contributes the two ledger effects through the effect role:
// idempotent per key, reconcilable from the effect receipt, serialized on
// the ledger by the default Resource Conflict Keys, and callable only
// while the plugin is ready. The chat tools and reads keep their IDs.
func TestFinanceWorkflowEffectsCategorizeTheLedgerOnce(t *testing.T) {
	ctx := context.Background()
	seedFinanceLedger(t)
	seedDB, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seedDB.Exec(`INSERT INTO rules (merchant, category) VALUES ('Store', 'Groceries'), ('Old', 'Dining')`); err != nil {
		t.Fatal(err)
	}
	seedDB.Close()

	manager, err := NewManager(tools.NewToolset(nil), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []CapabilityID{FinanceSyncLedgerCapabilityID, FinanceCategorizeLedgerCapabilityID} {
		contract, ok := manager.WorkflowContract(string(id))
		if !ok || contract.Contract != "1.0.0" || !contract.Effect || !contract.Idempotent || !contract.Reconciles || contract.ConflictKeys != "resource" ||
			!reflect.DeepEqual(contract.Connections, []string{FinanceConnectionID}) || !reflect.DeepEqual(contract.Resources, []string{FinanceLedgerResource}) {
			t.Fatalf("%s contract = %+v, %v", id, contract, ok)
		}
	}
	if _, err := manager.ResolveWorkflowCapability(ctx, string(FinanceCategorizeLedgerCapabilityID)); !errors.Is(err, ErrWorkflowCapabilityUnavailable) {
		t.Fatalf("disabled plugin resolved: %v", err)
	}
	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	provider, err := manager.ResolveWorkflowCapability(ctx, string(FinanceCategorizeLedgerCapabilityID))
	if err != nil || provider.Perform == nil || provider.Reconcile == nil || provider.Read != nil {
		t.Fatalf("provider = %+v, %v", provider, err)
	}
	req := WorkflowEffectRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: map[string]any{"day": "2026-09-30"}, IdempotencyKey: "run-1/categorize"}

	// Before anything happened, reconciliation proves it did not.
	lookup, err := provider.Reconcile(ctx, req)
	if err != nil || lookup.Outcome != WorkflowEffectDidNotHappen {
		t.Fatalf("reconcile before = %+v, %v", lookup, err)
	}
	// t5 (Old, posted) matches a rule; t4 (Store) is pending and fenced;
	// t3 (paycheck) has no merchant.
	out, err := provider.Perform(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"categorized": float64(1), "uncategorized": float64(1), "replayed": false}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("categorize = %#v", out)
	}
	again, err := provider.Perform(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	replayed := map[string]any{"categorized": float64(1), "uncategorized": float64(1), "replayed": true}
	if !reflect.DeepEqual(again, replayed) {
		t.Fatalf("replay = %#v", again)
	}
	// Reconciliation answers with the output as it was recorded.
	lookup, err = provider.Reconcile(ctx, req)
	if err != nil || lookup.Outcome != WorkflowEffectHappened || !reflect.DeepEqual(lookup.Output, want) {
		t.Fatalf("reconcile after = %+v, %v", lookup, err)
	}

	// Outside the ledger, with no key, or with an undeclared input field,
	// nothing is performed and the error proves it.
	for name, bad := range map[string]WorkflowEffectRequest{
		"other resource": {Connection: FinanceConnectionID, Resource: "vault", Input: req.Input, IdempotencyKey: "k"},
		"no key":         {Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: req.Input},
		"unknown field":  {Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: map[string]any{"day": "d", "table": "rules"}, IdempotencyKey: "k"},
	} {
		if _, err := provider.Perform(ctx, bad); !errors.Is(err, ErrWorkflowEffectNotPerformed) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

// sync_ledger reports a failed bank in its output rather than failing,
// with no bank credential in the response, and replays by key.
func TestFinanceWorkflowSyncLedgerReportsBanksAndReplays(t *testing.T) {
	ctx := context.Background()
	seedFinanceLedger(t)
	t.Setenv("PLAID_CLIENT_ID", "test")
	t.Setenv("PLAID_SECRET", "test")
	calls := 0
	restore := finance.FakeSyncItemsForTest(func(itemID string) (int, int, int, error) {
		calls++
		return 0, 0, 0, errors.New("ITEM_LOGIN_REQUIRED")
	})
	defer restore()
	manager, err := NewManager(tools.NewToolset(nil), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	provider, err := manager.ResolveWorkflowCapability(ctx, string(FinanceSyncLedgerCapabilityID))
	if err != nil {
		t.Fatal(err)
	}
	req := WorkflowEffectRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: map[string]any{"day": "2026-09-30"}, IdempotencyKey: "run-1/sync"}
	out, err := provider.Perform(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	got := out.(map[string]any)
	banks := got["banks"].([]any)
	if got["failed_banks"] != float64(1) || got["replayed"] != false || len(banks) != 1 || calls != 1 {
		t.Fatalf("sync = %#v (calls %d)", got, calls)
	}
	bank := banks[0].(map[string]any)
	if bank["failed"] != true || bank["error"] != "ITEM_LOGIN_REQUIRED" || bank["label"] != "item-1" {
		t.Fatalf("bank = %#v", bank)
	}
	for _, v := range []any{bank["label"], bank["error"]} {
		if strings.Contains(v.(string), "SECRET") {
			t.Fatalf("a credential leaked into the response: %#v", bank)
		}
	}
	again, err := provider.Perform(ctx, req)
	if err != nil || again.(map[string]any)["replayed"] != true || calls != 1 {
		t.Fatalf("replay = %#v, %v (calls %d)", again, err, calls)
	}
}
