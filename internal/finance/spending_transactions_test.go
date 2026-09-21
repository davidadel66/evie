package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func insertSpendingEntry(t *testing.T, db *sql.DB, transactionID, category string, cents int64, source string) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO categories(name) VALUES(?)`, category); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO budget_entries(transaction_id,category,amount_cents,source) VALUES(?,?,?,?)`, transactionID, category, cents, source); err != nil {
		t.Fatal(err)
	}
}

func spendingClassificationFixture(t *testing.T) (*SpendingService, *sql.DB) {
	t.Helper()
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	insertSpendingTxn(t, db, "unclassified", "2026-09-30", -2500, 0)
	insertSpendingTxn(t, db, "legacy", "2026-09-29", 1000, 0)
	insertSpendingTxn(t, db, "split", "2026-09-28", 1000, 0)
	insertSpendingTxn(t, db, "manual", "2026-09-28", -150, 0)
	insertSpendingTxn(t, db, "pending-entry", "2026-09-27", 200, 1)
	insertSpendingTxn(t, db, "pending-empty", "2026-09-26", 300, 1)
	insertSpendingTxn(t, db, "incomplete-split", "2026-09-25", 800, 0)
	insertSpendingEntry(t, db, "split", "Food", 600, "rule")
	insertSpendingEntry(t, db, "split", "Home", 400, "human")
	insertSpendingEntry(t, db, "manual", "Food", -150, "manual")
	insertSpendingEntry(t, db, "pending-entry", "Food", 200, "human")
	insertSpendingEntry(t, db, "incomplete-split", "Home", 500, "legacy-source")
	if _, err := db.Exec(`UPDATE transactions SET category='Old category',category_source='agent',reviewed=1 WHERE transaction_id='legacy'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE transactions SET name='Statement descriptor',merchant_name='Merchant',plaid_category='FOOD_AND_DRINK',account_id='private-account' WHERE transaction_id='unclassified'`); err != nil {
		t.Fatal(err)
	}
	for i, date := range []any{"2026-08-31", "2026-10-01", "2026-09-31", "2026-09-00", "2026-09-1a", "2026-9-02", "2026-09-01T00:00:00Z", nil, ""} {
		insertSpendingTxn(t, db, "excluded-"+string(rune('a'+i)), date, 1, 0)
	}
	return s, db
}

func TestSpendingTransactionsDerivesSavedClassificationAndPreservesSplits(t *testing.T) {
	s, db := spendingClassificationFixture(t)
	report, err := s.InspectSpendingTransactions(context.Background(), SpendingTransactionsQuery{Month: "2026-09"})
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := SpendingClassificationCounts{All: 7, Unclassified: 2, Classified: 3, Pending: 2}
	if report.Counts != wantCounts || report.Total != 7 || report.Status != "all" || report.Offset != 0 || report.PageSize != 50 || report.HasMore {
		t.Fatalf("report metadata = %+v", report)
	}
	wantIDs := []string{"unclassified", "legacy", "manual", "split", "pending-entry", "pending-empty", "incomplete-split"}
	var ids []string
	byID := map[string]SpendingTransaction{}
	for _, transaction := range report.Transactions {
		ids = append(ids, transaction.ID)
		byID[transaction.ID] = transaction
	}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("ordered transaction IDs = %v, want %v", ids, wantIDs)
	}
	unclassified := byID["unclassified"]
	if unclassified.Classification != "unclassified" || unclassified.NetCents != "2500" || unclassified.Name != "Statement descriptor" || unclassified.MerchantName != "Merchant" || len(unclassified.Entries) != 0 || unclassified.LegacyCategory != nil || unclassified.LegacySource != nil {
		t.Fatalf("unclassified = %+v", unclassified)
	}
	legacy := byID["legacy"]
	if legacy.Classification != "unclassified" || len(legacy.Entries) != 0 || legacy.LegacyCategory == nil || *legacy.LegacyCategory != "Old category" || legacy.LegacySource == nil || *legacy.LegacySource != "agent" {
		t.Fatalf("legacy fields invented canonical classification/proposal: %+v", legacy)
	}
	split := byID["split"]
	wantEntries := []SpendingEntry{{Category: "Food", AmountCents: "600", Source: "rule"}, {Category: "Home", AmountCents: "400", Source: "human"}}
	if split.Classification != "classified" || split.NetCents != "-1000" || !reflect.DeepEqual(split.Entries, wantEntries) {
		t.Fatalf("split entries = %+v", split)
	}
	manual := byID["manual"]
	if manual.NetCents != "150" || len(manual.Entries) != 1 || manual.Entries[0].AmountCents != "-150" || manual.Entries[0].Source != "manual" {
		t.Fatalf("saved source or entry sign changed: %+v", manual)
	}
	if pending := byID["pending-entry"]; pending.Classification != "pending" || len(pending.Entries) != 1 {
		t.Fatalf("pending did not take precedence or lost saved entries: %+v", pending)
	}
	if incomplete := byID["incomplete-split"]; incomplete.Classification != "classified" || len(incomplete.Entries) != 1 || incomplete.Entries[0].Source != "legacy-source" {
		t.Fatalf("classification diverged from Categorize's existence fence: %+v", incomplete)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-account", "test-token", `"item_id"`, `"account_id"`, `"plaid_category"`, `"reviewed"`} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("response leaked excluded field %q", secret)
		}
	}
	var entries int
	if err := db.QueryRow(`SELECT COUNT(*) FROM budget_entries`).Scan(&entries); err != nil || entries != 5 {
		t.Fatalf("reading changed classifications: %d, %v", entries, err)
	}
}

func TestSpendingTransactionsFiltersAndPagesTransactionsWithoutDuplicatingSplits(t *testing.T) {
	s, _ := spendingClassificationFixture(t)
	for _, test := range []struct {
		status string
		total  int
		ids    []string
	}{
		{"all", 7, []string{"unclassified", "legacy", "manual", "split", "pending-entry", "pending-empty", "incomplete-split"}},
		{"unclassified", 2, []string{"unclassified", "legacy"}},
		{"classified", 3, []string{"manual", "split", "incomplete-split"}},
		{"pending", 2, []string{"pending-entry", "pending-empty"}},
	} {
		t.Run(test.status, func(t *testing.T) {
			var got []string
			for offset := 0; offset < test.total; offset += 2 {
				report, err := s.InspectSpendingTransactions(context.Background(), SpendingTransactionsQuery{Month: "2026-09", Status: test.status, Offset: offset, PageSize: 2})
				if err != nil || report.Total != test.total || report.Counts.All != 7 || len(report.Transactions) > 2 || report.Offset != offset || report.PageSize != 2 || report.HasMore != (offset+2 < test.total) {
					t.Fatalf("page %d = %+v, %v", offset, report, err)
				}
				for _, transaction := range report.Transactions {
					got = append(got, transaction.ID)
				}
			}
			if !reflect.DeepEqual(got, test.ids) {
				t.Fatalf("paged IDs = %v, want %v", got, test.ids)
			}
			empty, err := s.InspectSpendingTransactions(context.Background(), SpendingTransactionsQuery{Month: "2026-09", Status: test.status, Offset: math.MaxInt, PageSize: 2})
			if err != nil || empty.Total != test.total || len(empty.Transactions) != 0 || empty.HasMore {
				t.Fatalf("out-of-range page = %+v, %v", empty, err)
			}
		})
	}
}

func TestSpendingTransactionsCalendarBoundariesAndExactCents(t *testing.T) {
	s, db := spendingTestService(t)
	insertItem(t, db, "item-1")
	for _, test := range []struct{ id, date string }{
		{"first-year", "0001-01-01"}, {"last-year", "9999-12-31"},
		{"valid-leap", "2024-02-29"}, {"valid-february", "2026-02-28"},
		{"invalid-leap", "2026-02-29"}, {"invalid-february", "2024-02-30"},
	} {
		insertSpendingTxn(t, db, test.id, test.date, math.MinInt64, 0)
	}
	insertSpendingEntry(t, db, "valid-leap", "Large refund", math.MinInt64, "human")
	for _, test := range []struct{ month, id string }{
		{"0001-01", "first-year"}, {"9999-12", "last-year"}, {"2024-02", "valid-leap"}, {"2026-02", "valid-february"},
	} {
		report, err := s.InspectSpendingTransactions(context.Background(), SpendingTransactionsQuery{Month: test.month})
		if err != nil || report.Total != 1 || len(report.Transactions) != 1 || report.Transactions[0].ID != test.id || report.Transactions[0].NetCents != "9223372036854775808" {
			t.Fatalf("month %s = %+v, %v", test.month, report, err)
		}
		if test.id == "valid-leap" && (len(report.Transactions[0].Entries) != 1 || report.Transactions[0].Entries[0].AmountCents != "-9223372036854775808") {
			t.Fatalf("entry cents lost precision: %+v", report.Transactions[0])
		}
	}
}

func TestSpendingTransactionsValidatesBeforeReadingAndMissingDatabaseStaysEmpty(t *testing.T) {
	s := NewSpendingService()
	reads := 0
	s.openRead = func(context.Context) (*sql.DB, error) { reads++; return nil, nil }
	s.openWrite = func(context.Context) (*sql.DB, error) {
		t.Fatal("read attempted to create a database")
		return nil, nil
	}
	for _, query := range []SpendingTransactionsQuery{
		{}, {Month: "2026-9"}, {Month: "2026-00"}, {Month: "2026-13"}, {Month: "0000-01"}, {Month: "10000-01"},
		{Month: "2026-09-01"}, {Month: " 2026-09"}, {Month: "2026-09", Status: "proposed"},
		{Month: "2026-09", Offset: -1}, {Month: "2026-09", PageSize: -1}, {Month: "2026-09", PageSize: 101},
	} {
		if _, err := s.InspectSpendingTransactions(context.Background(), query); !errors.Is(err, ErrSpendingTransactionsQuery) {
			t.Fatalf("invalid query %+v = %v", query, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.InspectSpendingTransactions(ctx, SpendingTransactionsQuery{Month: "2026-09"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled query = %v", err)
	}
	if reads != 0 {
		t.Fatal("invalid or cancelled request opened database")
	}
	report, err := s.InspectSpendingTransactions(context.Background(), SpendingTransactionsQuery{Month: "2026-09"})
	if err != nil || report.Total != 0 || report.HasMore || report.Status != "all" || report.PageSize != 50 || report.Counts != (SpendingClassificationCounts{}) {
		t.Fatalf("missing database = %+v, %v", report, err)
	}
	encoded, err := json.Marshal(report)
	if err != nil || !strings.Contains(string(encoded), `"transactions":[]`) {
		t.Fatalf("empty transactions JSON = %s, %v", encoded, err)
	}
	s.openRead = func(context.Context) (*sql.DB, error) { return nil, errors.New("private path or provider payload") }
	if _, err := s.InspectSpendingTransactions(context.Background(), SpendingTransactionsQuery{Month: "2026-09"}); err != ErrSpendingUnavailable {
		t.Fatalf("raw database error exposed: %v", err)
	}
}
