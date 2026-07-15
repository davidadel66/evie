package main

// Tests for applySyncPage, written from SPEC.md before the implementation
// exists (red -> green). Contract under test:
//
//	func applySyncPage(db *sql.DB, itemID string, added, modified []SyncTxn, removed []string, nextCursor string) error
//	func openDBAt(path string) (*sql.DB, error)

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// newTestDB opens a fresh temp database and registers cleanup.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := openDBAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openDBAt: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// insertItem satisfies the transactions.item_id foreign key.
func insertItem(t *testing.T, db *sql.DB, itemID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO items (item_id, access_token, linked_at) VALUES (?, ?, ?)`,
		itemID, "test-token", "2026-07-02T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert item %q: %v", itemID, err)
	}
}

func getCursor(t *testing.T, db *sql.DB, itemID string) string {
	t.Helper()
	var cursor string
	if err := db.QueryRow(`SELECT cursor FROM items WHERE item_id = ?`, itemID).Scan(&cursor); err != nil {
		t.Fatalf("select cursor for %q: %v", itemID, err)
	}
	return cursor
}

func countTxns(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions`).Scan(&n); err != nil {
		t.Fatalf("count transactions: %v", err)
	}
	return n
}

func txn(id string, amount float64) SyncTxn {
	return SyncTxn{
		TransactionID: id,
		AccountID:     "acc-1",
		Date:          "2026-07-01",
		Name:          "Coffee Shop",
		MerchantName:  "Coffee Shop Inc",
		Amount:        amount,
		Category:      "FOOD_AND_DRINK",
		Pending:       false,
	}
}

// Spec item 1: added transactions are inserted with correct cents conversion.
// 1.15 is the float-precision trap: 1.15*100 = 114.999..., so a naive
// int64(v*100) truncates to 114; the spec mandates math.Round(v*100) -> 115.
func TestApplySyncPage_AddedCentsConversion(t *testing.T) {
	cases := []struct {
		name      string
		dollars   float64
		wantCents int64
	}{
		{"simple", 12.34, 1234},
		{"negative penny", -0.01, -1},
		{"float precision trap", 1.15, 115},
		{"zero", 0, 0},
		{"whole dollars", 250.00, 25000},
	}

	db := newTestDB(t)
	insertItem(t, db, "item-1")

	added := make([]SyncTxn, len(cases))
	for i, c := range cases {
		added[i] = txn("txn-cents-"+c.name, c.dollars)
	}
	if err := applySyncPage(db, "item-1", added, nil, nil, "cursor-1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got int64
			err := db.QueryRow(
				`SELECT amount_cents FROM transactions WHERE transaction_id = ?`,
				"txn-cents-"+c.name,
			).Scan(&got)
			if err != nil {
				t.Fatalf("select amount_cents: %v", err)
			}
			if got != c.wantCents {
				t.Errorf("amount %v: got %d cents, want %d", c.dollars, got, c.wantCents)
			}
		})
	}
}

// Spec item 1 (cont.): inserted rows carry all fields, associated to the item.
func TestApplySyncPage_AddedInsertsFields(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")

	in := SyncTxn{
		TransactionID: "txn-fields",
		AccountID:     "acc-9",
		Date:          "2026-06-15",
		Name:          "Grocery Store",
		MerchantName:  "Groceries LLC",
		Amount:        45.67,
		Category:      "GROCERIES",
		Pending:       false,
	}
	if err := applySyncPage(db, "item-1", []SyncTxn{in}, nil, nil, "cursor-1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}

	var (
		itemID, accountID, date, name, merchant, category string
		cents                                             int64
	)
	err := db.QueryRow(
		`SELECT item_id, account_id, date, name, merchant_name, amount_cents, plaid_category
		 FROM transactions WHERE transaction_id = ?`, "txn-fields",
	).Scan(&itemID, &accountID, &date, &name, &merchant, &cents, &category)
	if err != nil {
		t.Fatalf("select row: %v", err)
	}
	if itemID != "item-1" {
		t.Errorf("item_id = %q, want %q", itemID, "item-1")
	}
	if accountID != in.AccountID {
		t.Errorf("account_id = %q, want %q", accountID, in.AccountID)
	}
	if date != in.Date {
		t.Errorf("date = %q, want %q", date, in.Date)
	}
	if name != in.Name {
		t.Errorf("name = %q, want %q", name, in.Name)
	}
	if merchant != in.MerchantName {
		t.Errorf("merchant_name = %q, want %q", merchant, in.MerchantName)
	}
	if cents != 4567 {
		t.Errorf("amount_cents = %d, want %d", cents, 4567)
	}
	if category != in.Category {
		t.Errorf("category = %q, want %q", category, in.Category)
	}
}

// Spec item 2: modified with the same transaction_id updates in place
// (UPSERT), never duplicates the row.
func TestApplySyncPage_ModifiedUpserts(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")

	orig := txn("txn-mod", 10.00)
	orig.Pending = true
	if err := applySyncPage(db, "item-1", []SyncTxn{orig}, nil, nil, "cursor-1"); err != nil {
		t.Fatalf("applySyncPage (added): %v", err)
	}

	changed := orig
	changed.Amount = 20.50
	changed.Name = "Coffee Shop (posted)"
	changed.Category = "FOOD_AND_DRINK_COFFEE"
	changed.Pending = false
	if err := applySyncPage(db, "item-1", nil, []SyncTxn{changed}, nil, "cursor-2"); err != nil {
		t.Fatalf("applySyncPage (modified): %v", err)
	}

	if n := countTxns(t, db); n != 1 {
		t.Fatalf("got %d rows after modified, want 1 (must upsert, not duplicate)", n)
	}

	var (
		name, category string
		cents          int64
		pending        int64
	)
	err := db.QueryRow(
		`SELECT name, amount_cents, plaid_category, pending FROM transactions WHERE transaction_id = ?`,
		"txn-mod",
	).Scan(&name, &cents, &category, &pending)
	if err != nil {
		t.Fatalf("select row: %v", err)
	}
	if name != changed.Name {
		t.Errorf("name = %q, want %q", name, changed.Name)
	}
	if cents != 2050 {
		t.Errorf("amount_cents = %d, want %d", cents, 2050)
	}
	if category != changed.Category {
		t.Errorf("category = %q, want %q", category, changed.Category)
	}
	if pending != 0 {
		t.Errorf("pending = %d, want 0 after posted update", pending)
	}
}

// Spec item 3: removed IDs are deleted; a nonexistent ID is not an error.
func TestApplySyncPage_Removed(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")

	added := []SyncTxn{txn("txn-keep", 1.00), txn("txn-drop", 2.00)}
	if err := applySyncPage(db, "item-1", added, nil, nil, "cursor-1"); err != nil {
		t.Fatalf("applySyncPage (added): %v", err)
	}

	if err := applySyncPage(db, "item-1", nil, nil, []string{"txn-drop", "txn-never-existed"}, "cursor-2"); err != nil {
		t.Fatalf("applySyncPage (removed) returned error, want nil even with nonexistent ID: %v", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE transaction_id = ?`, "txn-drop").Scan(&n); err != nil {
		t.Fatalf("count txn-drop: %v", err)
	}
	if n != 0 {
		t.Errorf("txn-drop still present after removal")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE transaction_id = ?`, "txn-keep").Scan(&n); err != nil {
		t.Fatalf("count txn-keep: %v", err)
	}
	if n != 1 {
		t.Errorf("txn-keep was deleted; removal must only delete listed IDs")
	}
}

// Spec item 4: the item's cursor is persisted in the same call, including for
// a page with no transaction changes.
func TestApplySyncPage_CursorPersisted(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertItem(t, db, "item-2")

	if err := applySyncPage(db, "item-1", []SyncTxn{txn("txn-c1", 5.00)}, nil, nil, "cursor-abc"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}
	if got := getCursor(t, db, "item-1"); got != "cursor-abc" {
		t.Errorf("item-1 cursor = %q, want %q", got, "cursor-abc")
	}
	if got := getCursor(t, db, "item-2"); got != "" {
		t.Errorf("item-2 cursor = %q, want %q (other items must be untouched)", got, "")
	}

	// Empty page still advances the cursor.
	if err := applySyncPage(db, "item-1", nil, nil, nil, "cursor-def"); err != nil {
		t.Fatalf("applySyncPage (empty page): %v", err)
	}
	if got := getCursor(t, db, "item-1"); got != "cursor-def" {
		t.Errorf("cursor after empty page = %q, want %q", got, "cursor-def")
	}
}

// Spec item 5: the page is applied atomically. We force a genuine mid-page
// statement failure with a trigger that aborts any DELETE on transactions,
// then send a page containing both a valid added txn and a removed ID that
// exists. The call must error, and nothing from the page may persist: the
// added txn is absent, the removed txn survives, and the cursor is unchanged
// from the last committed page.
func TestApplySyncPage_AtomicOnFailure(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")

	// Commit one good page so there is known state to preserve.
	if err := applySyncPage(db, "item-1", []SyncTxn{txn("txn-committed", 3.00)}, nil, nil, "cursor-committed"); err != nil {
		t.Fatalf("applySyncPage (good page): %v", err)
	}

	// From here on, any DELETE on transactions aborts its SQL transaction.
	if _, err := db.Exec(`CREATE TRIGGER fail_del BEFORE DELETE ON transactions
BEGIN SELECT RAISE(ABORT, 'boom'); END;`); err != nil {
		t.Fatalf("create fail_del trigger: %v", err)
	}

	// The failing page: a valid insert plus a delete the trigger aborts.
	err := applySyncPage(db, "item-1", []SyncTxn{txn("txn-lost", 4.00)}, nil, []string{"txn-committed"}, "cursor-lost")
	if err == nil {
		t.Fatal("applySyncPage returned nil, want error from aborted DELETE")
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE transaction_id = ?`, "txn-lost").Scan(&n); err != nil {
		t.Fatalf("count txn-lost: %v", err)
	}
	if n != 0 {
		t.Errorf("txn-lost persisted despite page failure; insert must roll back")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE transaction_id = ?`, "txn-committed").Scan(&n); err != nil {
		t.Fatalf("count txn-committed: %v", err)
	}
	if n != 1 {
		t.Errorf("txn-committed missing after failed page; delete must have rolled back")
	}
	if got := getCursor(t, db, "item-1"); got != "cursor-committed" {
		t.Errorf("cursor = %q after failed page, want %q (must roll back)", got, "cursor-committed")
	}
}

// Spec item 4 corollary: "cursor is persisted on the item" — an itemID with
// no row in items means the cursor UPDATE matches zero rows. Silently
// succeeding would violate the contract, so the call must return an error.
func TestApplySyncPage_MissingItemCursorErrors(t *testing.T) {
	db := newTestDB(t)
	// No item inserted; the page is txn-free so only the cursor UPDATE runs.
	err := applySyncPage(db, "no-such-item", nil, nil, nil, "cursor-x")
	if err == nil {
		t.Fatal("applySyncPage with unknown itemID returned nil, want error (cursor update matched 0 rows)")
	}
}

// Spec item 6: Pending bool round-trips through the INTEGER column
// (true -> 1, false -> 0).
func TestApplySyncPage_PendingRoundTrip(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")

	pendingTxn := txn("txn-pending", 1.00)
	pendingTxn.Pending = true
	postedTxn := txn("txn-posted", 2.00)
	postedTxn.Pending = false

	if err := applySyncPage(db, "item-1", []SyncTxn{pendingTxn, postedTxn}, nil, nil, "cursor-1"); err != nil {
		t.Fatalf("applySyncPage: %v", err)
	}

	for _, c := range []struct {
		id   string
		want int64
	}{
		{"txn-pending", 1},
		{"txn-posted", 0},
	} {
		var got int64
		if err := db.QueryRow(`SELECT pending FROM transactions WHERE transaction_id = ?`, c.id).Scan(&got); err != nil {
			t.Fatalf("select pending for %q: %v", c.id, err)
		}
		if got != c.want {
			t.Errorf("%s: pending = %d, want %d", c.id, got, c.want)
		}
	}
}
