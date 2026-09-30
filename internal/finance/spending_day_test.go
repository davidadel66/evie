package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func spendingDayFixture(t *testing.T) (*SpendingService, *sql.DB) {
	t.Helper()
	s, db := spendingTestService(t)
	s.openEdit = s.openWrite
	insertItem(t, db, "item-1")
	insertSpendingTxn(t, db, "a-refund", "2026-09-20", math.MinInt64, 0)
	insertSpendingTxn(t, db, "b-split", "2026-09-20", 1000, 0)
	insertSpendingTxn(t, db, "c-unclassified", "2026-09-20", 500, 0)
	insertSpendingTxn(t, db, "d-pending", "2026-09-20", 400, 1)
	insertSpendingTxn(t, db, "other-day", "2026-09-21", 999, 0)
	insertSpendingEntry(t, db, "a-refund", "Food", math.MinInt64, "manual")
	insertSpendingEntry(t, db, "b-split", "Food", 600, "rule")
	insertSpendingEntry(t, db, "b-split", "Home", 200, "human")
	insertSpendingEntry(t, db, "d-pending", "Food", 400, "rule")
	if _, err := db.Exec(`INSERT INTO categories(name) VALUES('excluded')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE transactions SET name='Statement description',merchant_name='Merchant',account_id='private-account',plaid_category='private-plaid-category',tags='["private-transaction-tag"]',category='Legacy',category_source='agent',reviewed=1 WHERE transaction_id='c-unclassified'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE budget_entries SET id=9223372036854775807,tags='["private-entry-tag"]' WHERE transaction_id='b-split' AND category='Food'`); err != nil {
		t.Fatal(err)
	}
	return s, db
}

func inspectDayRow(t *testing.T, s *SpendingService, id string) SpendingDayTransaction {
	t.Helper()
	report, err := s.InspectSpendingDay(context.Background(), SpendingDayQuery{Date: "2026-09-20"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Transactions {
		if row.ID == id {
			return row
		}
	}
	t.Fatalf("missing day row %s", id)
	return SpendingDayTransaction{}
}

func TestSpendingDayPagesPostedRowsAndSummaryWithoutLosingCents(t *testing.T) {
	s, db := spendingDayFixture(t)
	var ids []string
	for offset := 0; offset < 3; offset++ {
		report, err := s.InspectSpendingDay(context.Background(), SpendingDayQuery{Date: "2026-09-20", Offset: offset, PageSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		if report.Total != 3 || len(report.Transactions) != 1 || report.HasMore != (offset < 2) || report.Offset != offset || report.PageSize != 1 {
			t.Fatalf("page = %+v", report)
		}
		wantSummary := SpendingDay{Date: "2026-09-20", InflowCents: "9223372036854775808", OutflowCents: "1500", NetCents: "9223372036854774308", Transactions: 3}
		if report.Summary != wantSummary || !reflect.DeepEqual(report.Categories, []string{"excluded", "Food", "Home"}) {
			t.Fatalf("day metadata = %+v", report)
		}
		ids = append(ids, report.Transactions[0].ID)
		encoded, _ := json.Marshal(report)
		for _, private := range []string{"private-account", "private-plaid-category", "private-transaction-tag", "private-entry-tag", "test-token", `"item_id"`, `"account_id"`, `"reviewed"`} {
			if strings.Contains(string(encoded), private) {
				t.Fatalf("day response leaked %s", private)
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"a-refund", "b-split", "c-unclassified"}) {
		t.Fatalf("paged IDs = %v", ids)
	}
	refund := inspectDayRow(t, s, "a-refund")
	if refund.NetCents != "9223372036854775808" || refund.Entries[0].AmountCents != "-9223372036854775808" {
		t.Fatalf("refund precision = %+v", refund)
	}
	split := inspectDayRow(t, s, "b-split")
	if len(split.Entries) != 2 || split.Entries[1].ID != "9223372036854775807" || split.Entries[0].AmountCents != "200" || split.Entries[1].AmountCents != "600" {
		t.Fatalf("split identity/amounts = %+v", split)
	}
	legacy := inspectDayRow(t, s, "c-unclassified")
	if legacy.Classification != "unclassified" || legacy.Name != "Statement description" || legacy.MerchantName != "Merchant" || legacy.LegacyCategory == nil || *legacy.LegacyCategory != "Legacy" || legacy.LegacySource == nil || *legacy.LegacySource != "agent" {
		t.Fatalf("legacy row = %+v", legacy)
	}
	empty, err := s.InspectSpendingDay(context.Background(), SpendingDayQuery{Date: "2026-09-20", Offset: math.MaxInt})
	if err != nil || empty.Total != 3 || len(empty.Transactions) != 0 || empty.HasMore {
		t.Fatalf("large offset = %+v, %v", empty, err)
	}
	if got := entryCount(t, db, "b-split"); got != 2 {
		t.Fatalf("read changed entries: %d", got)
	}
}

func TestSpendingDayValidationAndMissingStorage(t *testing.T) {
	s := NewSpendingService()
	reads := 0
	s.openRead = func(context.Context) (*sql.DB, error) { reads++; return nil, nil }
	s.openEdit = func(context.Context) (*sql.DB, error) { t.Fatal("read opened writable storage"); return nil, nil }
	for _, query := range []SpendingDayQuery{
		{}, {Date: "0000-01-01"}, {Date: "10000-01-01"}, {Date: "2026-02-29"}, {Date: "2024-02-30"},
		{Date: "2026-9-20"}, {Date: "2026-09-20Z"}, {Date: " 2026-09-20"},
		{Date: "2026-09-20", Offset: -1}, {Date: "2026-09-20", PageSize: -1}, {Date: "2026-09-20", PageSize: 101},
	} {
		if _, err := s.InspectSpendingDay(context.Background(), query); err != ErrSpendingDayQuery {
			t.Fatalf("invalid day %+v = %v", query, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.InspectSpendingDay(ctx, SpendingDayQuery{Date: "2026-09-20"}); err != context.Canceled || reads != 0 {
		t.Fatalf("cancelled day: %v, reads=%d", err, reads)
	}
	for _, date := range []string{"0001-01-01", "9999-12-31", "2024-02-29"} {
		report, err := s.InspectSpendingDay(context.Background(), SpendingDayQuery{Date: date})
		if err != nil || report.Date != date || report.PageSize != 50 || report.Total != 0 || report.Summary.NetCents != "0" || report.Transactions == nil || report.Categories == nil {
			t.Fatalf("missing database day %s = %+v, %v", date, report, err)
		}
	}
	s.openRead = func(context.Context) (*sql.DB, error) { return nil, errors.New("private SQLite path") }
	if _, err := s.InspectSpendingDay(context.Background(), SpendingDayQuery{Date: "2026-09-20"}); err != ErrSpendingUnavailable {
		t.Fatalf("raw read error: %v", err)
	}
}

func TestSpendingCategoryCreatesHumanAllocationAndPreservesRawTransaction(t *testing.T) {
	s, db := spendingDayFixture(t)
	before, err := readSpendingDaySnapshots(context.Background(), db, `WHERE transaction_id='c-unclassified'`)
	if err != nil {
		t.Fatal(err)
	}
	row := inspectDayRow(t, s, "c-unclassified")
	updated, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "excluded", Revision: row.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Classification != "classified" || len(updated.Entries) != 1 || updated.Entries[0].Category != "excluded" || updated.Entries[0].AmountCents != "500" || updated.Entries[0].Source != "human" || updated.Revision == row.Revision {
		t.Fatalf("created allocation = %+v", updated)
	}
	after, err := readSpendingDaySnapshots(context.Background(), db, `WHERE transaction_id='c-unclassified'`)
	if err != nil {
		t.Fatal(err)
	}
	after[0].Entries = nil
	if !reflect.DeepEqual(before, after) {
		t.Fatal("category edit changed raw transaction")
	}
	if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "Food", Revision: row.Revision}); err != ErrSpendingCategoryConflict {
		t.Fatalf("stale unclassified update = %v", err)
	}
	if entryCount(t, db, row.ID) != 1 {
		t.Fatal("repeat update duplicated allocation")
	}
	if got := inspectDayRow(t, s, row.ID); !reflect.DeepEqual(got, updated) {
		t.Fatalf("saved allocation did not survive reopen: %+v", got)
	}
}

func TestSpendingCategoryEditsOneSplitWithoutRepairingAmountsOrTags(t *testing.T) {
	s, db := spendingDayFixture(t)
	row := inspectDayRow(t, s, "b-split")
	entryID := row.Entries[1].ID
	updated, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, EntryID: &entryID, Category: "Home", Revision: row.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Entries) != 2 || updated.Entries[0] != row.Entries[0] || updated.Entries[1] != (SpendingDayEntry{ID: entryID, Category: "Home", AmountCents: "600", Source: "human"}) || updated.NetCents != "-1000" {
		t.Fatalf("split edit = %+v", updated)
	}
	var tags string
	if err := db.QueryRow(`SELECT tags FROM budget_entries WHERE id=?`, entryID).Scan(&tags); err != nil || tags != `["private-entry-tag"]` {
		t.Fatalf("tags changed: %s, %v", tags, err)
	}
	if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "Food", Revision: updated.Revision}); err != ErrSpendingCategoryConflict {
		t.Fatalf("split replacement accepted: %v", err)
	}
	refund := inspectDayRow(t, s, "a-refund")
	refundID := refund.Entries[0].ID
	refunded, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: refund.ID, EntryID: &refundID, Category: "Home", Revision: refund.Revision})
	if err != nil || refunded.Entries[0].AmountCents != "-9223372036854775808" {
		t.Fatalf("refund edit changed amount: %+v, %v", refunded, err)
	}
}

func TestSpendingCategoryRejectsChangedSnapshots(t *testing.T) {
	for _, statement := range []string{
		`UPDATE transactions SET amount_cents=2000 WHERE transaction_id='b-split'`,
		`UPDATE transactions SET date='2026-09-19' WHERE transaction_id='b-split'`,
		`UPDATE transactions SET pending=1 WHERE transaction_id='b-split'`,
		`UPDATE transactions SET name='New description' WHERE transaction_id='b-split'`,
		`UPDATE transactions SET merchant_name='New merchant' WHERE transaction_id='b-split'`,
		`UPDATE transactions SET tags='["new"]' WHERE transaction_id='b-split'`,
		`UPDATE transactions SET category='Changed legacy' WHERE transaction_id='b-split'`,
		`UPDATE budget_entries SET amount_cents=700 WHERE transaction_id='b-split'`,
		`UPDATE budget_entries SET category='excluded' WHERE transaction_id='b-split'`,
		`UPDATE budget_entries SET source='other' WHERE transaction_id='b-split'`,
		`UPDATE budget_entries SET tags='["new"]' WHERE transaction_id='b-split'`,
		`DELETE FROM budget_entries WHERE transaction_id='b-split'`,
	} {
		t.Run(statement, func(t *testing.T) {
			s, db := spendingDayFixture(t)
			row := inspectDayRow(t, s, "b-split")
			if _, err := db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			id := row.Entries[0].ID
			if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, EntryID: &id, Category: "excluded", Revision: row.Revision}); err != ErrSpendingCategoryConflict {
				t.Fatalf("stale edit accepted: %v", err)
			}
		})
	}
	s, db := spendingDayFixture(t)
	row := inspectDayRow(t, s, "c-unclassified")
	if _, err := db.Exec(`DELETE FROM transactions WHERE transaction_id='c-unclassified'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "Home", Revision: row.Revision}); err != ErrSpendingCategoryConflict {
		t.Fatalf("removed transaction edit accepted: %v", err)
	}
}

func TestSpendingCategoryValidatesCategoryAndEntryOwnership(t *testing.T) {
	s, db := spendingDayFixture(t)
	row := inspectDayRow(t, s, "c-unclassified")
	if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "New category", Revision: row.Revision}); err != ErrSpendingCategoryQuery {
		t.Fatalf("unknown category accepted: %v", err)
	}
	var categoryCount, ruleCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&categoryCount); err != nil || categoryCount != 3 {
		t.Fatalf("created category: %d, %v", categoryCount, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM rules`).Scan(&ruleCount); err != nil || ruleCount != 0 {
		t.Fatalf("created rule: %d, %v", ruleCount, err)
	}
	foreign := inspectDayRow(t, s, "a-refund").Entries[0].ID
	if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, EntryID: &foreign, Category: "Home", Revision: row.Revision}); err != ErrSpendingCategoryConflict {
		t.Fatalf("foreign entry accepted: %v", err)
	}
}

func TestSpendingCategoryRollbackAndConcurrentWriters(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		s, db := spendingDayFixture(t)
		row := inspectDayRow(t, s, "b-split")
		if _, err := db.Exec(`CREATE TRIGGER fail_category AFTER UPDATE ON budget_entries BEGIN SELECT RAISE(ABORT,'private trigger detail'); END`); err != nil {
			t.Fatal(err)
		}
		id := row.Entries[1].ID
		if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, EntryID: &id, Category: "Home", Revision: row.Revision}); err != ErrSpendingCategoryUnavailable {
			t.Fatalf("unsafe failure: %v", err)
		}
		if got := inspectDayRow(t, s, row.ID); !reflect.DeepEqual(got, row) {
			t.Fatalf("failed edit changed row: %+v", got)
		}
	})
	t.Run("two editors", func(t *testing.T) {
		s, db := spendingDayFixture(t)
		row := inspectDayRow(t, s, "c-unclassified")
		var wg sync.WaitGroup
		start := make(chan struct{})
		errs := make(chan error, 2)
		for _, category := range []string{"Food", "Home"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: category, Revision: row.Revision})
				errs <- err
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		successes := 0
		for err := range errs {
			if err == nil {
				successes++
			} else if err != ErrSpendingCategoryConflict && err != ErrSpendingCategoryUnavailable {
				t.Fatalf("unexpected concurrent error: %v", err)
			}
		}
		if successes != 1 || entryCount(t, db, row.ID) != 1 {
			t.Fatalf("concurrent writes: successes=%d entries=%d", successes, entryCount(t, db, row.ID))
		}
	})
}

func TestSpendingCategoryValidationCancellationAndExistingOnlyOpen(t *testing.T) {
	good := SpendingCategoryUpdate{TransactionID: "txn", Category: "Food", Revision: strings.Repeat("a", 64)}
	badID := "01"
	badEntry := good
	badEntry.EntryID = &badID
	for _, update := range []SpendingCategoryUpdate{
		{}, {TransactionID: "", Category: "Food", Revision: good.Revision},
		{TransactionID: "txn", Category: " ", Revision: good.Revision},
		{TransactionID: "txn", Category: "Food", Revision: "opaque"},
		{TransactionID: "txn", Category: "Food", Revision: strings.Repeat("A", 64)}, badEntry,
	} {
		s := NewSpendingService()
		s.openEdit = func(context.Context) (*sql.DB, error) { t.Fatal("invalid edit opened database"); return nil, nil }
		if _, err := s.UpdateSpendingCategory(context.Background(), update); err != ErrSpendingCategoryQuery {
			t.Fatalf("invalid edit %+v = %v", update, err)
		}
	}
	s := NewSpendingService()
	s.openEdit = func(context.Context) (*sql.DB, error) { t.Fatal("cancelled edit opened database"); return nil, nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.UpdateSpendingCategory(ctx, good); err != context.Canceled {
		t.Fatalf("cancelled edit = %v", err)
	}
	s.openEdit = func(context.Context) (*sql.DB, error) { return nil, errors.New("private path") }
	if _, err := s.UpdateSpendingCategory(context.Background(), good); err != ErrSpendingCategoryUnavailable {
		t.Fatalf("raw edit error: %v", err)
	}
	root := t.TempDir()
	t.Setenv("HOME", root)
	s = NewSpendingService()
	if _, err := s.UpdateSpendingCategory(context.Background(), good); err != ErrSpendingCategoryUnavailable {
		t.Fatalf("missing storage edit = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".finance")); !os.IsNotExist(err) {
		t.Fatalf("edit created finance directory: %v", err)
	}
	if err := os.Mkdir(filepath.Join(root, ".finance"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateSpendingCategory(context.Background(), good); err != ErrSpendingCategoryUnavailable {
		t.Fatalf("missing file edit = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".finance", "finance.db")); !os.IsNotExist(err) {
		t.Fatalf("edit created finance file: %v", err)
	}
}

func TestSpendingCategoryExistingOpenerPersistsWithoutApplyingSchema(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	dir := filepath.Join(root, ".finance")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := openDBAt(filepath.Join(dir, "finance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insertItem(t, db, "item-1")
	insertSpendingTxn(t, db, "c-unclassified", "2026-09-20", 123, 0)
	if _, err := db.Exec(`INSERT INTO categories(name) VALUES('Food')`); err != nil {
		t.Fatal(err)
	}
	// A finance editor should not recreate unrelated schema, which OpenDB does.
	if _, err := db.Exec(`DROP TABLE budget_limits`); err != nil {
		t.Fatal(err)
	}
	s := NewSpendingService()
	row := inspectDayRow(t, s, "c-unclassified")
	updated, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "Food", Revision: row.Revision})
	if err != nil || len(updated.Entries) != 1 || updated.Entries[0].AmountCents != "123" {
		t.Fatalf("existing database edit = %+v, %v", updated, err)
	}
	var exists bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE name='budget_limits')`).Scan(&exists); err != nil || exists {
		t.Fatalf("editor applied schema: %v, %v", exists, err)
	}
	if got := inspectDayRow(t, NewSpendingService(), row.ID); !reflect.DeepEqual(got, updated) {
		t.Fatalf("canonical reopened edit = %+v", got)
	}
}
