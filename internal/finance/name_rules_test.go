package finance

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func insertNameRule(t *testing.T, db *sql.DB, pattern, category string, priority int) {
	t.Helper()
	if _, err := db.Exec(`INSERT OR IGNORE INTO categories (name) VALUES (?)`, category); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO name_rules (pattern, category, priority) VALUES (?, ?, ?)`, pattern, category, priority); err != nil {
		t.Fatalf("insert name rule %q: %v", pattern, err)
	}
}

// bankTxn is a machine-generated bank string with no merchant name.
func bankTxn(id, name string, amount float64) SyncTxn {
	tx := txn(id, amount)
	tx.Name, tx.MerchantName = name, ""
	return tx
}

func entryCategory(t *testing.T, db *sql.DB, txnID string) string {
	t.Helper()
	var category string
	if err := db.QueryRow(`SELECT category FROM budget_entries WHERE transaction_id = ?`, txnID).Scan(&category); err != nil {
		t.Fatalf("entry for %s: %v", txnID, err)
	}
	return category
}

func syncPage(t *testing.T, db *sql.DB, cursor string, txns ...SyncTxn) {
	t.Helper()
	if err := applySyncPage(context.Background(), db, "item-1", txns, nil, nil, cursor); err != nil {
		t.Fatal(err)
	}
}

// A name rule categorizes a transaction with an empty merchant name,
// case-insensitively, and excluded is a category like any other: the
// transaction leaves the awaiting-review set with a recorded decision.
func TestNameRulesMatchBankStringsAndExcludedIsACategory(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertNameRule(t, db, "%citi card%", ExcludedCategory, 50)
	insertNameRule(t, db, "%mr cooper%", "Mortgage", 10)
	syncPage(t, db, "c1",
		bankTxn("card", "*6229 PAYMENT CITI CARD ONLINE ACH WEB", 1000),
		bankTxn("mortgage", "MR COOPER MTG PYT", 1951.40),
		bankTxn("unknown", "SOMETHING ELSE", 5),
	)
	matched, unmatched, err := Categorize(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 2 || unmatched != 1 {
		t.Fatalf("matched, unmatched = %d, %d", matched, unmatched)
	}
	if got := entryCategory(t, db, "card"); got != ExcludedCategory {
		t.Fatalf("card payment category = %q", got)
	}
	if got := entryCategory(t, db, "mortgage"); got != "Mortgage" {
		t.Fatalf("mortgage category = %q", got)
	}
	var awaiting int
	if err := db.QueryRow(`SELECT COUNT(*) FROM transactions t WHERE t.pending = 0 AND NOT EXISTS (SELECT 1 FROM budget_entries e WHERE e.transaction_id = t.transaction_id)`).Scan(&awaiting); err != nil || awaiting != 1 {
		t.Fatalf("awaiting review = %d, %v", awaiting, err)
	}
	// Idempotent with name rules present.
	if again, _, err := Categorize(context.Background(), db); err != nil || again != 0 {
		t.Fatalf("second run = %d, %v", again, err)
	}
}

// Precedence: the legacy category column beats a merchant rule, which
// beats a name rule, even when all three would match.
func TestNameRulesLoseToMerchantRulesAndLegacyCategory(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Coffee Shop Inc", "Coffee")
	insertNameRule(t, db, "%coffee%", "Dining", 10)
	legacy := txn("legacy", 2)
	legacy.Name = "COFFEE SHOP"
	syncPage(t, db, "c1", txn("merchant", 3.50), legacy)
	if _, err := db.Exec(`INSERT OR IGNORE INTO categories (name) VALUES ('Legacy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE transactions SET category = 'Legacy', category_source = 'manual' WHERE transaction_id = 'legacy'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Categorize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := entryCategory(t, db, "merchant"); got != "Coffee" {
		t.Fatalf("merchant rule lost to name rule: %q", got)
	}
	if got := entryCategory(t, db, "legacy"); got != "Legacy" {
		t.Fatalf("legacy category lost: %q", got)
	}
}

// The lowest priority wins; an equal-priority tie fails the run naming
// the transaction, both patterns and the priority, and mints nothing.
func TestNameRulesPriorityOrdersAndTiesAreAnError(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertNameRule(t, db, "%student ln%", "Loans", 10)
	insertNameRule(t, db, "%payment%", ExcludedCategory, 50)
	syncPage(t, db, "c1", bankTxn("loan", "STUDENT LN PAYMENT", 300))
	if _, _, err := Categorize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := entryCategory(t, db, "loan"); got != "Loans" {
		t.Fatalf("priority 10 lost to 50: %q", got)
	}

	insertNameRule(t, db, "%online transfer%", ExcludedCategory, 60)
	insertNameRule(t, db, "%transfer to%", "Housing", 60)
	syncPage(t, db, "c2", bankTxn("tie", "ONLINE TRANSFER TO SAVINGS", 100))
	_, _, err := Categorize(context.Background(), db)
	if err == nil {
		t.Fatal("an equal-priority tie was decided silently")
	}
	for _, want := range []string{"tie", `"%online transfer%"`, `"%transfer to%"`, "priority 60"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("tie error %q lacks %q", err, want)
		}
	}
	if n := entryCount(t, db, "tie"); n != 0 {
		t.Fatalf("a tie minted %d entries", n)
	}

	// Two patterns at one priority that agree are not a choice.
	if _, err := db.Exec(`DELETE FROM name_rules WHERE pattern = '%transfer to%'`); err != nil {
		t.Fatal(err)
	}
	insertNameRule(t, db, "%to savings%", ExcludedCategory, 60)
	if _, _, err := Categorize(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := entryCategory(t, db, "tie"); got != ExcludedCategory {
		t.Fatalf("agreeing patterns = %q", got)
	}
}

// Patterns are plain SQL LIKE: zel % is prefix-anchored on purpose and
// does not match a merchant whose name contains "zel"; the pending fence
// holds even when a name rule matches.
func TestNameRulesArePrefixAwareAndRespectThePendingFence(t *testing.T) {
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertNameRule(t, db, "zel %", ExcludedCategory, 60)
	hazel := txn("hazel", 4)
	hazel.Name, hazel.MerchantName = "HAZEL CAFE", ""
	pending := bankTxn("pending", "ZEL TO LISA WATTERS", 20)
	pending.Pending = true
	syncPage(t, db, "c1", bankTxn("zelle", "ZEL TO LISA WATTERS", 40), hazel, pending)
	matched, unmatched, err := Categorize(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 1 || unmatched != 1 || entryCount(t, db, "hazel") != 0 || entryCount(t, db, "pending") != 0 {
		t.Fatalf("matched, unmatched = %d, %d; hazel %d, pending %d", matched, unmatched, entryCount(t, db, "hazel"), entryCount(t, db, "pending"))
	}
	if got := entryCategory(t, db, "zelle"); got != ExcludedCategory {
		t.Fatalf("zelle category = %q", got)
	}
}

// excluded exists in every database the schema opened, and a name rule
// whose category is absent is refused by the foreign key rather than
// minting an entry in an unknown category.
func TestExcludedIsSeededAndNameRulesNeedARealCategory(t *testing.T) {
	db := newTestDB(t)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM categories WHERE name = ?`, ExcludedCategory).Scan(&n); err != nil || n != 1 {
		t.Fatalf("excluded seeded = %d, %v", n, err)
	}
	if _, err := db.Exec(`INSERT INTO name_rules (pattern, category, priority) VALUES ('%x%', 'No Such Category', 1)`); err == nil {
		t.Fatal("a name rule with an unknown category was accepted")
	}
}
