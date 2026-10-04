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

// seedFinanceLedger creates an isolated finance database for September
// 2026: two classified purchases, an unclassified paycheck, a pending
// purchase, and one August purchase outside the month.
func seedFinanceLedger(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatalf("open finance db: %v", err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`INSERT INTO items (item_id, access_token, linked_at) VALUES ('item-1', 'access-token-SECRET', 'now')`,
		`INSERT INTO categories (name) VALUES ('Groceries'), ('Dining')`,
		`INSERT INTO transactions (transaction_id, item_id, date, name, merchant_name, amount_cents, pending) VALUES
			('t1', 'item-1', '2026-09-03', 'MARKET 12', 'Market', 4520, 0),
			('t2', 'item-1', '2026-09-10', 'CAFE', 'Cafe', 1250, 0),
			('t3', 'item-1', '2026-09-15', 'PAYROLL', '', -300000, 0),
			('t4', 'item-1', '2026-09-20', 'STORE', 'Store', 999, 1),
			('t5', 'item-1', '2026-08-31', 'OLD', 'Old', 100, 0)`,
		`INSERT INTO budget_entries (transaction_id, category, amount_cents, source) VALUES
			('t1', 'Groceries', 4520, 'rule'), ('t2', 'Dining', 1250, 'manual')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// Finance contributes two fixed, parameterized reads of the local finance
// database to workflows, on the finance-local connection's ledger
// resource. They read through the same read-only Spending queries as the
// Spending view, return whole cents as integers and never include bank
// credentials.
func TestFinanceWorkflowReadsTheLocalLedger(t *testing.T) {
	ctx := context.Background()
	seedFinanceLedger(t)
	manager, err := NewManager(tools.NewToolset(nil), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []CapabilityID{FinanceMonthCashFlowCapabilityID, FinanceMonthTransactionsCapabilityID} {
		contract, ok := manager.WorkflowContract(string(id))
		if !ok || contract.Contract != "1.0.0" || contract.Effect ||
			!reflect.DeepEqual(contract.Connections, []string{FinanceConnectionID}) || !reflect.DeepEqual(contract.Resources, []string{FinanceLedgerResource}) {
			t.Fatalf("%s contract = %+v, %v", id, contract, ok)
		}
	}
	if err := manager.SetEnabled(FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	read := func(id CapabilityID, input map[string]any) (any, error) {
		t.Helper()
		provider, err := manager.ResolveWorkflowCapability(ctx, string(id))
		if err != nil {
			t.Fatal(err)
		}
		return provider.Read(ctx, WorkflowReadRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: input})
	}
	flow, err := read(FinanceMonthCashFlowCapabilityID, map[string]any{"month": "2026-09"})
	if err != nil {
		t.Fatal(err)
	}
	wantFlow := map[string]any{
		"month": "2026-09", "inflow_cents": int64(300000), "outflow_cents": int64(5770), "net_cents": int64(294230),
		"transactions": int64(3), "pending_transactions": int64(1),
		"categories": []any{
			map[string]any{"kind": "unclassified", "category": "", "inflow_cents": int64(300000), "outflow_cents": int64(0), "net_cents": int64(300000), "transactions": int64(1)},
			map[string]any{"kind": "category", "category": "Groceries", "inflow_cents": int64(0), "outflow_cents": int64(4520), "net_cents": int64(-4520), "transactions": int64(1)},
			map[string]any{"kind": "category", "category": "Dining", "inflow_cents": int64(0), "outflow_cents": int64(1250), "net_cents": int64(-1250), "transactions": int64(1)},
		},
	}
	if !reflect.DeepEqual(flow, wantFlow) {
		t.Fatalf("cash flow = %#v", flow)
	}
	page, err := read(FinanceMonthTransactionsCapabilityID, map[string]any{"month": "2026-09", "page_size": int64(2)})
	if err != nil {
		t.Fatal(err)
	}
	wantPage := map[string]any{
		"month": "2026-09", "status": "all", "offset": int64(0), "page_size": int64(2), "total": int64(4), "has_more": true,
		"counts": map[string]any{"all": int64(4), "unclassified": int64(1), "classified": int64(2), "pending": int64(1)},
		"transactions": []any{
			map[string]any{"id": "t4", "date": "2026-09-20", "name": "STORE", "merchant_name": "Store", "net_cents": int64(-999), "classification": "pending", "entries": []any{}},
			map[string]any{"id": "t3", "date": "2026-09-15", "name": "PAYROLL", "merchant_name": "", "net_cents": int64(300000), "classification": "unclassified", "entries": []any{}},
		},
	}
	if !reflect.DeepEqual(page, wantPage) {
		t.Fatalf("transactions = %#v", page)
	}
	classified, err := read(FinanceMonthTransactionsCapabilityID, map[string]any{"month": "2026-09", "status": "classified"})
	if err != nil {
		t.Fatal(err)
	}
	got := classified.(map[string]any)["transactions"].([]any)
	if len(got) != 2 || !reflect.DeepEqual(got[1].(map[string]any)["entries"], []any{map[string]any{"category": "Groceries", "amount_cents": int64(4520), "source": "rule"}}) {
		t.Fatalf("classified transactions = %#v", got)
	}
	for name, input := range map[string]map[string]any{
		"bad month":     {"month": "September"},
		"bad status":    {"month": "2026-09", "status": "reviewed"},
		"page too big":  {"month": "2026-09", "page_size": int64(101)},
		"no month":      {},
		"unknown field": {"month": "2026-09", "query": "SELECT * FROM items"},
	} {
		if _, err := read(FinanceMonthTransactionsCapabilityID, input); err == nil {
			t.Fatalf("%s: accepted %v", name, input)
		}
	}
	if _, err := read(FinanceMonthCashFlowCapabilityID, map[string]any{"month": "2026-9"}); err == nil {
		t.Fatal("a malformed month was accepted")
	}
	provider, err := manager.ResolveWorkflowCapability(ctx, string(FinanceMonthCashFlowCapabilityID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Read(ctx, WorkflowReadRequest{Connection: "elsewhere", Resource: FinanceLedgerResource, Input: map[string]any{"month": "2026-09"}}); err == nil ||
		!strings.Contains(err.Error(), FinanceConnectionID) {
		t.Fatalf("a read on another connection: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := provider.Read(cancelled, WorkflowReadRequest{Connection: FinanceConnectionID, Resource: FinanceLedgerResource, Input: map[string]any{"month": "2026-09"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("a cancelled read: %v", err)
	}
}
