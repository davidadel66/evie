package finance

// Tests for Categorize and for the sync/categorize interaction that broke
// in production: budget_entries.transaction_id REFERENCES transactions, so
// a categorized transaction could not be deleted when Plaid removed it.
// Contract under test:
//
//	func Categorize(context.Background(), db *sql.DB) (matched, unmatched int, err error)
//	func applySyncPage(context.Background(), db *sql.DB, itemID string, added, modified []SyncTxn, removed []string, nextCursor string) error

import (
	"context"
	"database/sql"
	"testing"
)

// insertRule adds a merchant→category rule, plus the category it names.
func insertRule(t *testing.T, db *sql.DB, merchant, category string) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO categories (name) VALUES (?)`, category); err != nil {
		t.Fatalf("insert category %q: %v", category, err)
	}
	if _, err := db.Exec(`INSERT INTO rules (merchant, category) VALUES (?, ?)`, merchant, category); err != nil {
		t.Fatalf("insert rule %q: %v", merchant, err)
	}
}

// entryCount counts budget entries for one transaction.
func entryCount(t *testing.T, db *sql.DB, txnID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM budget_entries WHERE transaction_id = ?`, txnID,
	).Scan(&n); err != nil {
		t.Fatalf("count entries for %q: %v", txnID, err)
	}
	return n
}

// Posted transactions matching a rule get an entry for their full amount.
func TestCategorizeMatchesPostedTransactions(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")

	if err := applySyncPage(context.Background(), db, "item-1", []SyncTxn{txn("txn-posted", 3.50)}, nil, nil, "c1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}

	matched, _, err := Categorize(context.Background(), db)
	if err != nil {
		t.Fatalf("Categorize: %v", err)
	}
	if matched != 1 {
		t.Errorf("matched = %d, want 1", matched)
	}

	var cents int64
	var category string
	if err := db.QueryRow(
		`SELECT amount_cents, category FROM budget_entries WHERE transaction_id = ?`, "txn-posted",
	).Scan(&cents, &category); err != nil {
		t.Fatalf("select entry: %v", err)
	}
	if cents != 350 {
		t.Errorf("amount_cents = %d, want 350", cents)
	}
	if category != "Coffee" {
		t.Errorf("category = %q, want Coffee", category)
	}
}

// The root-cause fix: pending transactions must NOT be categorized. Plaid's
// lifecycle is to issue a pending row, then remove it and add a posted one
// when it settles — so an entry minted against the pending leg is priced at
// an amount that changes (gas holds, unadded tips) and then has to be
// deleted. Two production banks wedged their sync on exactly this.
func TestCategorizeSkipsPendingTransactions(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")

	pending := txn("txn-pending", 5.00)
	pending.Pending = true
	if err := applySyncPage(context.Background(), db, "item-1", []SyncTxn{pending}, nil, nil, "c1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}

	matched, _, err := Categorize(context.Background(), db)
	if err != nil {
		t.Fatalf("Categorize: %v", err)
	}
	if matched != 0 {
		t.Errorf("matched = %d, want 0 — a pending transaction must not be categorized", matched)
	}
	if n := entryCount(t, db, "txn-pending"); n != 0 {
		t.Errorf("%d entries minted for a pending transaction, want 0", n)
	}

	// Once it posts, the same rule categorizes it at the settled amount.
	posted := txn("txn-pending", 5.75)
	posted.Pending = false
	if err := applySyncPage(context.Background(), db, "item-1", nil, []SyncTxn{posted}, nil, "c2"); err != nil {
		t.Fatalf("applySyncPage (posted): %v", err)
	}

	if _, _, err := Categorize(context.Background(), db); err != nil {
		t.Fatalf("Categorize after posting: %v", err)
	}
	var cents int64
	if err := db.QueryRow(
		`SELECT amount_cents FROM budget_entries WHERE transaction_id = ?`, "txn-pending",
	).Scan(&cents); err != nil {
		t.Fatalf("select entry after posting: %v", err)
	}
	if cents != 575 {
		t.Errorf("amount_cents = %d, want 575 — the settled amount, not the pending hold", cents)
	}
}

// Re-running Categorize must not mint a second entry for the same
// transaction: "no entry yet" is the candidate filter.
func TestCategorizeIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")

	if err := applySyncPage(context.Background(), db, "item-1", []SyncTxn{txn("txn-1", 1.00)}, nil, nil, "c1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}
	for i := range 2 {
		if _, _, err := Categorize(context.Background(), db); err != nil {
			t.Fatalf("Categorize run %d: %v", i+1, err)
		}
	}
	if n := entryCount(t, db, "txn-1"); n != 1 {
		t.Errorf("%d entries after two Categorize runs, want 1", n)
	}
}

// The production failure, end to end: a categorized transaction that Plaid
// later removes must delete cleanly. Before the fix this returned
// "FOREIGN KEY constraint failed (787)" and — because the delete shares the
// page's transaction with the cursor UPDATE — rolled the whole page back,
// so the bank replayed the same failing page on every future sync and never
// advanced its cursor.
func TestApplySyncPageRemovesCategorizedTransaction(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")

	if err := applySyncPage(context.Background(), db, "item-1", []SyncTxn{txn("txn-doomed", 2.00)}, nil, nil, "c1"); err != nil {
		t.Fatalf("applySyncPage (added): %v", err)
	}
	if _, _, err := Categorize(context.Background(), db); err != nil {
		t.Fatalf("Categorize: %v", err)
	}
	if n := entryCount(t, db, "txn-doomed"); n != 1 {
		t.Fatalf("setup: %d entries, want 1 — the test needs a categorized transaction", n)
	}

	if err := applySyncPage(context.Background(), db, "item-1", nil, nil, []string{"txn-doomed"}, "c2"); err != nil {
		t.Fatalf("removing a categorized transaction: %v", err)
	}

	if n := entryCount(t, db, "txn-doomed"); n != 0 {
		t.Errorf("%d entries survived the transaction's removal, want 0 — they would count toward spend forever", n)
	}
	if n := countTxns(t, db); n != 0 {
		t.Errorf("%d transactions remain, want 0", n)
	}
	// The cursor is the part that wedged: it must have advanced.
	if got := getCursor(t, db, "item-1"); got != "c2" {
		t.Errorf("cursor = %q, want c2 — a failed delete rolls back the cursor and the bank replays forever", got)
	}
}

// Removing a transaction must only delete ITS entries.
func TestApplySyncPageRemovalSparesOtherEntries(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")

	added := []SyncTxn{txn("txn-drop", 1.00), txn("txn-keep", 2.00)}
	if err := applySyncPage(context.Background(), db, "item-1", added, nil, nil, "c1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}
	if _, _, err := Categorize(context.Background(), db); err != nil {
		t.Fatalf("Categorize: %v", err)
	}

	if err := applySyncPage(context.Background(), db, "item-1", nil, nil, []string{"txn-drop"}, "c2"); err != nil {
		t.Fatalf("applySyncPage (removed): %v", err)
	}

	if n := entryCount(t, db, "txn-drop"); n != 0 {
		t.Errorf("%d entries for the removed transaction, want 0", n)
	}
	if n := entryCount(t, db, "txn-keep"); n != 1 {
		t.Errorf("%d entries for the surviving transaction, want 1 — removal must not touch other entries", n)
	}
}

func TestCategorizeCandidateRechecksInterveningHumanEntry(t *testing.T) {
	s, db := spendingDayFixture(t)
	// This is the candidate captured before the owner's category save, as in
	// Categorize's discovery phase. Its later insert must recheck that fence.
	stale := candidate{id: "c-unclassified", merchant: "Merchant", amountCents: 500, category: "Legacy", source: "agent"}
	row := inspectDayRow(t, s, stale.id)
	if _, err := s.UpdateSpendingCategory(context.Background(), SpendingCategoryUpdate{TransactionID: row.ID, Category: "Home", Revision: row.Revision}); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	inserted, err := insertCategorizedCandidate(context.Background(), tx, stale, "Food", "rule")
	if err != nil || inserted {
		t.Fatalf("stale categorizer inserted after human edit: %v, %v", inserted, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	saved := inspectDayRow(t, s, row.ID)
	if len(saved.Entries) != 1 || saved.Entries[0].Category != "Home" || saved.Entries[0].Source != "human" {
		t.Fatalf("human entry changed: %+v", saved)
	}
}

func TestCategorizeCandidateRechecksInterveningSync(t *testing.T) {
	for _, changed := range []string{
		`UPDATE transactions SET amount_cents=900 WHERE transaction_id='txn'`,
		`UPDATE transactions SET pending=1 WHERE transaction_id='txn'`,
		`UPDATE transactions SET merchant_name='Other merchant' WHERE transaction_id='txn'`,
		`UPDATE transactions SET category='Home' WHERE transaction_id='txn'`,
		`UPDATE transactions SET category_source='human' WHERE transaction_id='txn'`,
		`DELETE FROM transactions WHERE transaction_id='txn'`,
	} {
		t.Run(changed, func(t *testing.T) {
			db := newTestDB(t)
			insertItem(t, db, "item-1")
			insertRule(t, db, "Merchant", "Food")
			insertSpendingTxn(t, db, "txn", "2026-09-20", 500, 0)
			if _, err := db.Exec(`UPDATE transactions SET merchant_name='Merchant' WHERE transaction_id='txn'`); err != nil {
				t.Fatal(err)
			}
			stale := candidate{id: "txn", merchant: "Merchant", amountCents: 500, source: "rule"}
			if _, err := db.Exec(changed); err != nil {
				t.Fatal(err)
			}
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			inserted, err := insertCategorizedCandidate(context.Background(), tx, stale, "Food", "rule")
			if err != nil || inserted {
				t.Fatalf("stale categorizer inserted after sync: %v, %v", inserted, err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if entryCount(t, db, "txn") != 0 {
				t.Fatal("stale category entry survived")
			}
		})
	}
}
