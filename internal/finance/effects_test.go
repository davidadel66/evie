package finance

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/plaid/plaid-go/v43/plaid"
)

func fakeSyncItems(t *testing.T, db *sql.DB, outcomes map[string]error) *int {
	t.Helper()
	t.Setenv("PLAID_CLIENT_ID", "test")
	t.Setenv("PLAID_SECRET", "test")
	original := runSyncItem
	t.Cleanup(func() { runSyncItem = original })
	calls := 0
	runSyncItem = func(_ context.Context, _ *plaid.APIClient, _ *sql.DB, item syncItem) (int, int, int, error) {
		calls++
		if err := outcomes[item.itemID]; err != nil {
			return 0, 0, 0, err
		}
		return 2, 1, 0, nil
	}
	return &calls
}

func insertBank(t *testing.T, db *sql.DB, itemID, institution string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO items (item_id, access_token, cursor, institution, linked_at) VALUES (?, 'token', '', ?, 'now')`, itemID, institution); err != nil {
		t.Fatal(err)
	}
}

// The sync_ledger effect records one receipt under its idempotency key: a
// replay returns the recorded output, marked Replayed, without calling
// Plaid again. A failed bank is reported in the output, not as an error,
// so the workflow decides what a failed bank means.
func TestSyncLedgerRecordsAReceiptAndReplaysIt(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	insertBank(t, db, "item-1", "Bank One")
	insertBank(t, db, "item-2", "Bank Two")
	calls := fakeSyncItems(t, db, map[string]error{"item-2": errors.New("ITEM_LOGIN_REQUIRED")})

	first, err := SyncLedger(ctx, db, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Added != 2 || first.Modified != 1 || first.FailedBanks != 1 || len(first.Banks) != 2 ||
		first.Banks[0].Label != "Bank One" || first.Banks[0].Failed || !first.Banks[1].Failed || first.Banks[1].Error != "ITEM_LOGIN_REQUIRED" {
		t.Fatalf("first sync = %+v", first)
	}
	if *calls != 2 {
		t.Fatalf("bank calls = %d, want 2", *calls)
	}
	again, err := SyncLedger(ctx, db, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.Added != 2 || again.FailedBanks != 1 || len(again.Banks) != 2 {
		t.Fatalf("replay = %+v", again)
	}
	if *calls != 2 {
		t.Fatalf("a replay called Plaid: bank calls = %d", *calls)
	}
	var found SyncLedgerOutput
	if ok, err := LookupEffect(ctx, db, "key-1", &found); err != nil || !ok || found.Added != 2 {
		t.Fatalf("lookup = %+v, %v, %v", found, ok, err)
	}
	if ok, err := LookupEffect(ctx, db, "key-2", &found); err != nil || ok {
		t.Fatalf("unknown key lookup = %v, %v", ok, err)
	}
	if _, err := SyncLedger(ctx, db, ""); !errors.Is(err, ErrEffectKeyRequired) {
		t.Fatalf("empty key error = %v", err)
	}
	if *calls != 2 {
		t.Fatal("an empty key performed a sync")
	}
}

// The categorize_ledger effect mints entries and its receipt in one
// transaction, and a replay mints nothing more.
func TestCategorizeLedgerRecordsAReceiptWithItsEntries(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")
	unknown := txn("t-unknown", 9.99)
	unknown.MerchantName = "Nobody Knows"
	if err := applySyncPage(ctx, db, "item-1", []SyncTxn{txn("t-1", 4.50), unknown}, nil, nil, "c1"); err != nil {
		t.Fatal(err)
	}
	first, err := CategorizeLedger(ctx, db, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Replayed || first.Categorized != 1 || first.Uncategorized != 1 || entryCount(t, db, "t-1") != 1 {
		t.Fatalf("first categorize = %+v", first)
	}
	// A new matching transaction arrives; the same key must not touch it.
	if err := applySyncPage(ctx, db, "item-1", []SyncTxn{txn("t-2", 3.25)}, nil, nil, "c2"); err != nil {
		t.Fatal(err)
	}
	again, err := CategorizeLedger(ctx, db, "key-1")
	if err != nil {
		t.Fatal(err)
	}
	if !again.Replayed || again.Categorized != 1 || entryCount(t, db, "t-2") != 0 {
		t.Fatalf("replay = %+v, t-2 entries = %d", again, entryCount(t, db, "t-2"))
	}
	fresh, err := CategorizeLedger(ctx, db, "key-2")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Replayed || fresh.Categorized != 1 || fresh.Uncategorized != 1 || entryCount(t, db, "t-2") != 1 {
		t.Fatalf("new key = %+v", fresh)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM effect_receipts WHERE effect = ?`, EffectCategorizeLedger).Scan(&n); err != nil || n != 2 {
		t.Fatalf("receipts = %d, %v", n, err)
	}
}
