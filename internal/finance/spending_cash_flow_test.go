package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"strings"
	"testing"
)

func spendingCashFlowFixture(t *testing.T) (*SpendingService, *sql.DB) {
	t.Helper()
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	for _, row := range []struct {
		id, date string
		cents    int64
		pending  int
	}{
		{"income", "2026-09-01", -10000, 0},
		{"purchase", "2026-09-02", 1000, 0},
		{"refund", "2026-09-03", -200, 0},
		{"split", "2026-09-04", 1000, 0},
		{"mismatch", "2026-09-05", 1000, 0},
		{"mixed-sign", "2026-09-06", 1000, 0},
		{"legacy", "2026-09-07", -500, 0},
		{"excluded", "2026-09-08", 300, 0},
		{"wrong-sign", "2026-09-09", -400, 0},
		{"zero-mixed", "2026-09-10", 0, 0},
		{"zero-unclassified", "2026-09-11", 0, 0},
		{"repeat-category", "2026-09-20", 500, 0},
		{"pending", "2026-09-19", 600, 1},
		{"future", "2026-09-21", 999999, 0},
		{"future-pending", "2026-09-21", 800, 1},
		{"future-month", "2026-10-01", -99999, 0},
		{"previous-month", "2026-08-01", -100, 0},
		{"history-start", "2025-11-01", 500, 0},
		{"before-history", "2025-10-31", 9999, 0},
	} {
		insertSpendingTxn(t, db, row.id, row.date, row.cents, row.pending)
	}
	for _, entry := range []struct {
		id, category string
		cents        int64
	}{
		{"purchase", "Food", 1000}, {"refund", "Food", -200},
		{"split", "Food", 600}, {"split", "Home", 400},
		{"mismatch", "Food", 700},
		{"mixed-sign", "Food", 1200}, {"mixed-sign", "Adjustment", -200},
		{"excluded", "excluded", 300}, {"wrong-sign", "Food", 400},
		{"zero-mixed", "Food", 50}, {"zero-mixed", "Adjustment", -50},
		{"repeat-category", "Food", 200}, {"repeat-category", "Food", 300},
		{"pending", "Food", 600}, {"future", "Food", 999999},
	} {
		insertSpendingEntry(t, db, entry.id, entry.category, entry.cents, "human")
	}
	if _, err := db.Exec(`UPDATE transactions SET category='Legacy category',category_source='agent',reviewed=1,plaid_category='INCOME' WHERE transaction_id='legacy'`); err != nil {
		t.Fatal(err)
	}
	insertSpendingTxn(t, db, "undated", nil, 2000, 0)
	insertSpendingTxn(t, db, "blank", "", 2000, 0)
	insertSpendingTxn(t, db, "invalid", "2026-09-31", 2000, 0)
	return s, db
}

func cashFlowCategoryMap(report SpendingCashFlowReport) map[string]SpendingCashFlowCategory {
	result := map[string]SpendingCashFlowCategory{}
	for _, category := range report.Categories {
		key := category.Kind
		if category.Category != nil {
			key += "/" + *category.Category
		}
		result[key] = category
	}
	return result
}

func assertCashFlowReconciles(t *testing.T, report SpendingCashFlowReport) {
	t.Helper()
	var inflow, outflow, net big.Int
	for _, category := range report.Categories {
		add := func(total *big.Int, value string) {
			amount, ok := new(big.Int).SetString(value, 10)
			if !ok {
				t.Fatalf("invalid cents %q", value)
			}
			total.Add(total, amount)
		}
		add(&inflow, category.InflowCents)
		add(&outflow, category.OutflowCents)
		add(&net, category.NetCents)
	}
	if inflow.String() != report.Selected.InflowCents || outflow.String() != report.Selected.OutflowCents || net.String() != report.Selected.NetCents {
		t.Fatalf("categories do not reconcile: %s in, %s out, %s net; raw %+v", &inflow, &outflow, &net, report.Selected)
	}
}

func TestSpendingCashFlowReconcilesRawFlowsAndPreservesAllocationProblems(t *testing.T) {
	s, db := spendingCashFlowFixture(t)
	query := SpendingCashFlowQuery{Month: "2026-09", HistoryEnd: "2026-10", AsOfDate: "2026-09-20"}
	report, err := s.InspectSpendingCashFlow(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	want := SpendingCashFlowTotals{InflowCents: "11100", OutflowCents: "4800", NetCents: "6300", Transactions: 12}
	if report.Selected != want || report.Month != query.Month || report.HistoryEnd != query.HistoryEnd || report.AsOfDate != query.AsOfDate || report.PendingTransactions != 1 || report.UndatedTransactions != 3 {
		t.Fatalf("selected report = %+v", report)
	}
	categories := cashFlowCategoryMap(report)
	wantCategories := map[string]SpendingCashFlowTotals{
		"category/Food":     {InflowCents: "200", OutflowCents: "2100", NetCents: "-1900", Transactions: 4},
		"category/Home":     {InflowCents: "0", OutflowCents: "400", NetCents: "-400", Transactions: 1},
		"category/excluded": {InflowCents: "0", OutflowCents: "300", NetCents: "-300", Transactions: 1},
		"unclassified":      {InflowCents: "10500", OutflowCents: "0", NetCents: "10500", Transactions: 3},
		"unreconciled":      {InflowCents: "400", OutflowCents: "2000", NetCents: "-1600", Transactions: 4},
	}
	if len(categories) != len(wantCategories) {
		t.Fatalf("category buckets = %+v", categories)
	}
	for key, want := range wantCategories {
		if got, exists := categories[key]; !exists || got.SpendingCashFlowTotals != want {
			t.Fatalf("%s = %+v, want %+v", key, got, want)
		}
	}
	assertCashFlowReconciles(t, report)
	if len(report.History) != 12 || report.History[0].Month != "2025-11" || report.History[11].Month != "2026-10" || report.History[10].SpendingCashFlowTotals != want {
		t.Fatalf("history bounds/selected month = %+v", report.History)
	}
	if point := report.History[0]; point.OutflowCents != "500" || point.Transactions != 1 {
		t.Fatalf("first history month = %+v", point)
	}
	if point := report.History[9]; point.InflowCents != "100" || point.Transactions != 1 {
		t.Fatalf("previous month = %+v", point)
	}
	if point := report.History[11]; point.InflowCents != "0" || point.OutflowCents != "0" || point.Transactions != 0 {
		t.Fatalf("future history month leaked future data: %+v", point)
	}
	var savedEntries int
	if err := db.QueryRow(`SELECT COUNT(*) FROM budget_entries`).Scan(&savedEntries); err != nil || savedEntries != 15 {
		t.Fatalf("read changed saved allocations: %d, %v", savedEntries, err)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, hidden := range []string{"Legacy category", `"id"`, "test-token", "INCOME", "Adjustment"} {
		if strings.Contains(string(encoded), hidden) {
			t.Fatalf("cash-flow output exposed or trusted excluded field %q", hidden)
		}
	}
}

func TestSpendingCashFlowAsOfDateAndSelectionKeepHistoryWindowStable(t *testing.T) {
	s, _ := spendingCashFlowFixture(t)
	query := SpendingCashFlowQuery{Month: "2026-08", HistoryEnd: "2026-10", AsOfDate: "2026-09-20"}
	august, err := s.InspectSpendingCashFlow(context.Background(), query)
	if err != nil || august.Selected.InflowCents != "100" || len(august.History) != 12 || august.History[0].Month != "2025-11" {
		t.Fatalf("selected history month = %+v, %v", august, err)
	}
	query.Month = "2026-10"
	future, err := s.InspectSpendingCashFlow(context.Background(), query)
	if err != nil || future.Selected.Transactions != 0 || future.Selected.NetCents != "0" || len(future.Categories) != 0 || future.PendingTransactions != 0 {
		t.Fatalf("future selection = %+v, %v", future, err)
	}
	if !reflect.DeepEqual(august.History, future.History) {
		t.Fatal("changing selection moved or changed history")
	}
	query.Month = "2026-09"
	query.AsOfDate = "2026-09-21"
	later, err := s.InspectSpendingCashFlow(context.Background(), query)
	if err != nil || later.Selected.OutflowCents != "1004799" || later.Selected.Transactions != 13 || later.PendingTransactions != 2 {
		t.Fatalf("later cutoff did not include that day's rows: %+v, %v", later, err)
	}
	assertCashFlowReconciles(t, later)
}

func TestSpendingCashFlowSyntheticBucketNamesCannotCollideWithSavedCategories(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	for _, id := range []string{"named-unclassified", "named-unreconciled", "unclassified", "unreconciled", "blank-category", "zero-valid"} {
		insertSpendingTxn(t, db, id, "2026-09-01", 100, 0)
	}
	insertSpendingEntry(t, db, "named-unclassified", "Unclassified", 100, "human")
	insertSpendingEntry(t, db, "named-unreconciled", "Unreconciled", 100, "human")
	insertSpendingEntry(t, db, "unreconciled", "Other", 50, "human")
	insertSpendingEntry(t, db, "blank-category", " ", 100, "human")
	insertSpendingEntry(t, db, "zero-valid", "Zero part", 0, "human")
	insertSpendingEntry(t, db, "zero-valid", "Nonzero part", 100, "human")
	report, err := s.InspectSpendingCashFlow(context.Background(), SpendingCashFlowQuery{Month: "2026-09", HistoryEnd: "2026-09", AsOfDate: "2026-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	categories := cashFlowCategoryMap(report)
	for key, cents := range map[string]string{"category/Unclassified": "100", "category/Unreconciled": "100", "unclassified": "100", "unreconciled": "200", "category/Zero part": "0", "category/Nonzero part": "100"} {
		if got, exists := categories[key]; !exists || got.OutflowCents != cents {
			t.Fatalf("%s = %+v, want outflow %s", key, got, cents)
		}
	}
	if categories["unclassified"].Category != nil || categories["unreconciled"].Category != nil {
		t.Fatal("synthetic buckets received saved category labels")
	}
	assertCashFlowReconciles(t, report)
	// Equal-sized saved categories sort deterministically by their exact name.
	var named []string
	for _, category := range report.Categories {
		if category.Kind == "category" && category.OutflowCents == "100" {
			named = append(named, *category.Category)
		}
	}
	if !reflect.DeepEqual(named, []string{"Nonzero part", "Unclassified", "Unreconciled"}) {
		t.Fatalf("tied category ordering = %v", named)
	}
}

func TestSpendingCashFlowExactCentsAndCalendarBounds(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	for _, id := range []string{"large-inflow-one", "large-inflow-two"} {
		insertSpendingTxn(t, db, id, "2024-02-29", math.MinInt64, 0)
		insertSpendingEntry(t, db, id, "Large", math.MinInt64, "human")
	}
	insertSpendingTxn(t, db, "large-outflow", "2024-02-29", math.MaxInt64, 0)
	insertSpendingEntry(t, db, "large-outflow", "Large", math.MaxInt64, "human")
	insertSpendingTxn(t, db, "bad-leap-day", "2026-02-29", 99, 0)
	insertSpendingTxn(t, db, "first-year", "0001-01-01", 1, 0)
	insertSpendingTxn(t, db, "last-year", "9999-12-31", 1, 0)
	report, err := s.InspectSpendingCashFlow(context.Background(), SpendingCashFlowQuery{Month: "2024-02", HistoryEnd: "2024-12", AsOfDate: "2024-02-29"})
	if err != nil || report.Selected.InflowCents != "18446744073709551616" || report.Selected.OutflowCents != "9223372036854775807" || report.Selected.NetCents != "9223372036854775809" || report.Selected.Transactions != 3 || report.UndatedTransactions != 1 {
		t.Fatalf("large totals = %+v, %v", report, err)
	}
	assertCashFlowReconciles(t, report)
	first, err := s.InspectSpendingCashFlow(context.Background(), SpendingCashFlowQuery{Month: "0001-01", HistoryEnd: "0001-03", AsOfDate: "0001-03-31"})
	if err != nil || len(first.History) != 3 || first.History[0].Month != "0001-01" || first.Selected.OutflowCents != "1" {
		t.Fatalf("clamped year one = %+v, %v", first, err)
	}
	last, err := s.InspectSpendingCashFlow(context.Background(), SpendingCashFlowQuery{Month: "9999-12", HistoryEnd: "9999-12", AsOfDate: "9999-12-31"})
	if err != nil || len(last.History) != 12 || last.History[11].Month != "9999-12" || last.Selected.OutflowCents != "1" {
		t.Fatalf("last supported year = %+v, %v", last, err)
	}
}

func TestSpendingCashFlowValidatesBeforeReadingAndNeverCreatesStorage(t *testing.T) {
	s := NewSpendingService()
	reads := 0
	s.openRead = func(context.Context) (*sql.DB, error) { reads++; return nil, nil }
	s.openWrite = func(context.Context) (*sql.DB, error) { t.Fatal("read created storage"); return nil, nil }
	valid := SpendingCashFlowQuery{Month: "2026-09", HistoryEnd: "2026-09", AsOfDate: "2026-09-20"}
	for _, query := range []SpendingCashFlowQuery{
		{}, {Month: "2026-9", HistoryEnd: valid.HistoryEnd, AsOfDate: valid.AsOfDate},
		{Month: "2026-09", HistoryEnd: "2026-13", AsOfDate: valid.AsOfDate},
		{Month: "0000-01", HistoryEnd: "0001-01", AsOfDate: valid.AsOfDate},
		{Month: "10000-01", HistoryEnd: "10000-01", AsOfDate: valid.AsOfDate},
		{Month: "2025-09", HistoryEnd: valid.HistoryEnd, AsOfDate: valid.AsOfDate},
		{Month: "2026-10", HistoryEnd: valid.HistoryEnd, AsOfDate: valid.AsOfDate},
		{Month: valid.Month, HistoryEnd: valid.HistoryEnd, AsOfDate: "2026-02-29"},
		{Month: valid.Month, HistoryEnd: valid.HistoryEnd, AsOfDate: "2026-9-01"},
		{Month: valid.Month, HistoryEnd: valid.HistoryEnd, AsOfDate: "0000-01-01"},
		{Month: valid.Month, HistoryEnd: valid.HistoryEnd, AsOfDate: "2026-09-20T12:00:00Z"},
	} {
		if _, err := s.InspectSpendingCashFlow(context.Background(), query); !errors.Is(err, ErrSpendingCashFlowQuery) {
			t.Fatalf("invalid query %+v = %v", query, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.InspectSpendingCashFlow(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query = %v", err)
	}
	if reads != 0 {
		t.Fatal("invalid or cancelled query read storage")
	}
	report, err := s.InspectSpendingCashFlow(context.Background(), valid)
	if err != nil || len(report.History) != 12 || report.Selected.NetCents != "0" || report.Selected.Transactions != 0 || len(report.Categories) != 0 {
		t.Fatalf("missing storage = %+v, %v", report, err)
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"categories":[]`) {
		t.Fatalf("empty categories JSON = %s, %v", encoded, err)
	}
	s.openRead = func(context.Context) (*sql.DB, error) { return nil, errors.New("private database details") }
	if _, err := s.InspectSpendingCashFlow(context.Background(), valid); err != ErrSpendingUnavailable {
		t.Fatalf("raw database error exposed: %v", err)
	}
}
