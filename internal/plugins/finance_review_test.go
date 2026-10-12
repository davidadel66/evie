package plugins

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/tools"
)

// The review read gives the model candidates with deterministic evidence
// and no private field; the apply effect writes only the owner's accepted
// lines, once per key, and reconciles from its receipt.
func TestFinanceReviewReadAndApplyEffectRoundTrip(t *testing.T) {
	ctx := context.Background()
	seedFinanceLedger(t)
	seedDB, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	// t5's merchant shares its first significant token with a rule.
	if _, err := seedDB.Exec(`UPDATE transactions SET merchant_name = 'Oldsmobile Diner' WHERE transaction_id = 't5'`); err != nil {
		t.Fatal(err)
	}
	if _, err := seedDB.Exec(`INSERT INTO rules (merchant, category) VALUES ('Oldsmobile', 'Dining')`); err != nil {
		t.Fatal(err)
	}
	seedDB.Close()
	manager, err := NewManager(tools.NewToolset(nil), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	read, err := manager.ResolveWorkflowCapability(ctx, string(FinanceReviewCandidatesCapabilityID))
	if err != nil || read.Read == nil || read.Perform != nil {
		t.Fatalf("read provider = %+v, %v", read, err)
	}
	out, err := read.Read(ctx, WorkflowReadRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: map[string]any{"limit": int64(10)}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.(map[string]any)
	candidates := got["candidates"].([]any)
	// t3 (paycheck, no merchant) and t5 (Old) are posted and unclassified;
	// t4 is pending and fenced.
	if len(candidates) != 2 || got["remaining"] != int64(0) {
		t.Fatalf("candidates = %#v", got)
	}
	var old map[string]any
	for _, c := range candidates {
		m := c.(map[string]any)
		if m["id"] == "t5" {
			old = m
		}
		for _, hidden := range []string{"account_id", "tags", "access_token"} {
			if _, present := m[hidden]; present {
				t.Fatalf("candidate exposes %s", hidden)
			}
		}
	}
	if old == nil || old["suggested_category"] != "Dining" || old["net_cents"] != int64(-100) {
		t.Fatalf("Old candidate = %#v", old)
	}

	apply, err := manager.ResolveWorkflowCapability(ctx, string(FinanceApplyReviewCapabilityID))
	if err != nil || apply.Perform == nil || apply.Reconcile == nil {
		t.Fatalf("apply provider = %+v, %v", apply, err)
	}
	req := WorkflowEffectRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, IdempotencyKey: "run-1/apply", Input: map[string]any{
		"lines": []any{
			map[string]any{"id": "t5", "date": "2026-08-31", "name": "OLD", "merchant_name": "Oldsmobile Diner", "net_cents": int64(-100), "category": "Dining", "confidence": 0.9, "reason": "rule", "rule": true},
			map[string]any{"id": "t3", "date": "2026-09-15", "name": "PAYROLL", "merchant_name": "", "net_cents": int64(300000), "category": "Income", "confidence": 0.8, "reason": "payroll", "rule": false},
		},
		"answer":  map[string]any{"reject": []any{"t3"}},
		"run_key": "finance-review:test",
	}}
	if lookup, err := apply.Reconcile(ctx, req); err != nil || lookup.Outcome != WorkflowEffectDidNotHappen {
		t.Fatalf("reconcile before = %+v, %v", lookup, err)
	}
	result, err := apply.Perform(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"applied": float64(1), "rules_minted": float64(1), "rejected": float64(1), "skipped": float64(0), "replayed": false}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("apply = %#v", result)
	}
	again, err := apply.Perform(ctx, req)
	if err != nil || again.(map[string]any)["replayed"] != true {
		t.Fatalf("replay = %#v, %v", again, err)
	}
	if lookup, err := apply.Reconcile(ctx, req); err != nil || lookup.Outcome != WorkflowEffectHappened || !reflect.DeepEqual(lookup.Output, want) {
		t.Fatalf("reconcile after = %+v, %v", lookup, err)
	}
	// Rejected and applied rows leave the candidate set.
	out, err = read.Read(ctx, WorkflowReadRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: map[string]any{}})
	if err != nil || len(out.(map[string]any)["candidates"].([]any)) != 0 {
		t.Fatalf("candidates after apply = %#v, %v", out, err)
	}
	if _, err := apply.Perform(ctx, WorkflowEffectRequest{Connection: FinanceConnectionID, Resource: "vault", Input: req.Input, IdempotencyKey: "k"}); !errors.Is(err, ErrWorkflowEffectNotPerformed) {
		t.Fatalf("other resource: %v", err)
	}
}
