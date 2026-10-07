package finance

import (
	"context"
	"testing"
)

// Overview counting omits excluded allocations (spending.spec.md, amended
// 2026-10-06, transfers): a fully excluded transaction appears in no
// Overview count, a split's other allocations still count, and a split
// that does not reconcile still counts in full as Needs review.
func TestSpendingOverviewOmitsExcludedAllocations(t *testing.T) {
	ctx := context.Background()
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	for _, row := range []struct {
		id, date string
		cents    int64
	}{
		{"transfer", "2026-09-10", 50000},
		{"paycheck", "2026-09-10", -300000},
		{"partial", "2026-09-10", 1000},
		{"unreconciled", "2026-09-11", 1000},
		{"groceries", "2026-09-11", 4520},
	} {
		insertSpendingTxn(t, db, row.id, row.date, row.cents, 0)
	}
	for _, entry := range []struct {
		id, category string
		cents        int64
	}{
		{"transfer", "excluded", 50000}, {"paycheck", "excluded", -300000},
		{"partial", "Food", 600}, {"partial", "excluded", 400},
		{"unreconciled", "Food", 500}, {"unreconciled", "excluded", 300},
		{"groceries", "Food", 4520},
	} {
		insertSpendingEntry(t, db, entry.id, entry.category, entry.cents, "rule")
	}

	flow, err := s.InspectSpendingCashFlow(ctx, SpendingCashFlowQuery{Month: "2026-09", HistoryEnd: "2026-09", AsOfDate: "2026-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	// transfer and paycheck vanish; partial counts 600; unreconciled counts
	// its full 1000 under Needs review; groceries 4520.
	want := SpendingCashFlowTotals{InflowCents: "0", OutflowCents: "6120", NetCents: "-6120", Transactions: 3}
	if flow.Selected != want {
		t.Fatalf("selected = %+v, want %+v", flow.Selected, want)
	}
	categories := cashFlowCategoryMap(flow)
	if _, present := categories["category/excluded"]; present {
		t.Fatal("an excluded category bar was shown")
	}
	if food := categories["category/Food"]; food.OutflowCents != "5120" || food.Transactions != 2 {
		t.Fatalf("Food = %+v", food)
	}
	if review := categories["unreconciled"]; review.OutflowCents != "1000" || review.Transactions != 1 {
		t.Fatalf("unreconciled = %+v", review)
	}
	assertCashFlowReconciles(t, flow)

	calendar, err := s.InspectSpending(ctx, 2026)
	if err != nil {
		t.Fatal(err)
	}
	wantDays := []SpendingDay{
		{Date: "2026-09-10", InflowCents: "0", OutflowCents: "600", NetCents: "-600", Transactions: 1},
		{Date: "2026-09-11", InflowCents: "0", OutflowCents: "5520", NetCents: "-5520", Transactions: 2},
	}
	if len(calendar.Days) != 2 || calendar.Days[0] != wantDays[0] || calendar.Days[1] != wantDays[1] {
		t.Fatalf("days = %+v, want %+v", calendar.Days, wantDays)
	}

	// The day dialog's summary matches the cell, but every row is listed so
	// a wrong exclusion can be fixed in place.
	day, err := s.InspectSpendingDay(ctx, SpendingDayQuery{Date: "2026-09-10"})
	if err != nil {
		t.Fatal(err)
	}
	if day.Summary != wantDays[0] || day.Total != 3 || len(day.Transactions) != 3 {
		t.Fatalf("day summary = %+v total %d rows %d", day.Summary, day.Total, len(day.Transactions))
	}
}

// The Transactions view reports a fully excluded transaction under its own
// status, leaves it out of All, keeps the five counts disjoint, and still
// treats a split with one excluded allocation as Classified.
func TestSpendingTransactionsReportExcludedSeparately(t *testing.T) {
	ctx := context.Background()
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	for _, row := range []struct {
		id, date string
		cents    int64
		pending  int
	}{
		{"transfer", "2026-09-10", 50000, 0},
		{"partial", "2026-09-11", 1000, 0},
		{"groceries", "2026-09-12", 4520, 0},
		{"mystery", "2026-09-13", 700, 0},
		{"hold", "2026-09-14", 300, 1},
	} {
		insertSpendingTxn(t, db, row.id, row.date, row.cents, row.pending)
	}
	for _, entry := range []struct {
		id, category string
		cents        int64
	}{
		{"transfer", "excluded", 50000}, {"partial", "Food", 600}, {"partial", "excluded", 400}, {"groceries", "Food", 4520},
	} {
		insertSpendingEntry(t, db, entry.id, entry.category, entry.cents, "rule")
	}
	all, err := s.InspectSpendingTransactions(ctx, SpendingTransactionsQuery{Month: "2026-09"})
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := SpendingClassificationCounts{All: 4, Unclassified: 1, Classified: 2, Pending: 1, Excluded: 1}
	if all.Counts != wantCounts || all.Total != 4 || len(all.Transactions) != 4 {
		t.Fatalf("all = %+v (%d rows)", all.Counts, len(all.Transactions))
	}
	for _, row := range all.Transactions {
		if row.ID == "transfer" || row.Classification == "excluded" {
			t.Fatalf("All listed an excluded row: %+v", row)
		}
		if row.ID == "partial" && row.Classification != "classified" {
			t.Fatalf("partial split = %q, want classified", row.Classification)
		}
	}
	excluded, err := s.InspectSpendingTransactions(ctx, SpendingTransactionsQuery{Month: "2026-09", Status: "excluded"})
	if err != nil {
		t.Fatal(err)
	}
	if excluded.Total != 1 || len(excluded.Transactions) != 1 || excluded.Transactions[0].ID != "transfer" || excluded.Transactions[0].Classification != "excluded" {
		t.Fatalf("excluded = %+v", excluded)
	}
	day, err := s.InspectSpendingDay(ctx, SpendingDayQuery{Date: "2026-09-10"})
	if err != nil || len(day.Transactions) != 1 || day.Transactions[0].Classification != "excluded" || day.Summary.Transactions != 0 {
		t.Fatalf("day = %+v, %v", day, err)
	}
}

// Both legs of a credit-card payment are a transfer between the owner's
// own accounts: the bank-side debit and the card-side credit carry the
// Credit Card Payments category, and the Overview omits them like excluded
// (spending.spec.md, amended 2026-10-07) while the category stays on the
// rows and the Transactions view reports them under Excluded.
func TestSpendingOverviewOmitsCreditCardPayments(t *testing.T) {
	ctx := context.Background()
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	insertSpendingTxn(t, db, "card-credit", "2026-09-04", -200000, 0)
	insertSpendingTxn(t, db, "bank-debit", "2026-09-09", 200000, 0)
	insertSpendingTxn(t, db, "groceries", "2026-09-09", 4520, 0)
	insertSpendingEntry(t, db, "card-credit", CreditCardPaymentsCategory, -200000, "rule")
	insertSpendingEntry(t, db, "bank-debit", CreditCardPaymentsCategory, 200000, "rule")
	insertSpendingEntry(t, db, "groceries", "Food", 4520, "rule")

	flow, err := s.InspectSpendingCashFlow(ctx, SpendingCashFlowQuery{Month: "2026-09", HistoryEnd: "2026-09", AsOfDate: "2026-09-30"})
	if err != nil {
		t.Fatal(err)
	}
	want := SpendingCashFlowTotals{InflowCents: "0", OutflowCents: "4520", NetCents: "-4520", Transactions: 1}
	if flow.Selected != want {
		t.Fatalf("selected = %+v, want %+v", flow.Selected, want)
	}
	if _, present := cashFlowCategoryMap(flow)["category/"+CreditCardPaymentsCategory]; present {
		t.Fatal("a Credit Card Payments bar was shown")
	}
	page, err := s.InspectSpendingTransactions(ctx, SpendingTransactionsQuery{Month: "2026-09"})
	if err != nil {
		t.Fatal(err)
	}
	if page.Counts != (SpendingClassificationCounts{All: 1, Classified: 1, Excluded: 2}) {
		t.Fatalf("counts = %+v", page.Counts)
	}
	excluded, err := s.InspectSpendingTransactions(ctx, SpendingTransactionsQuery{Month: "2026-09", Status: "excluded"})
	if err != nil || len(excluded.Transactions) != 2 || excluded.Transactions[0].Entries[0].Category != CreditCardPaymentsCategory {
		t.Fatalf("excluded = %+v, %v", excluded, err)
	}
	day, err := s.InspectSpendingDay(ctx, SpendingDayQuery{Date: "2026-09-09"})
	if err != nil || day.Summary.Transactions != 1 || day.Summary.OutflowCents != "4520" || len(day.Transactions) != 2 {
		t.Fatalf("day = %+v, %v", day, err)
	}
	for _, row := range day.Transactions {
		if row.ID == "bank-debit" && row.Classification != "excluded" {
			t.Fatalf("bank debit = %q", row.Classification)
		}
	}
}
