package finance

import (
	"context"
	"testing"
)

// The embedded seed is the spec's reviewed table: it parses, loads with
// upsert semantics, creates its categories, and categorizes the bank
// strings it was written for.
func TestNameRulesSeedLoadsTheReviewedPatterns(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	rules, err := SeedNameRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 20 {
		t.Fatalf("seed has %d rules, want 20", len(rules))
	}
	n, err := NameRulesSeed(ctx, db)
	if err != nil || n != 20 {
		t.Fatalf("seed = %d, %v", n, err)
	}
	// Re-seeding is an upsert: the count is unchanged and an edited row
	// takes the new category and priority.
	if _, err := db.Exec(`UPDATE name_rules SET category = 'Loans', priority = 1 WHERE pattern = '%citi card%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := NameRulesSeed(ctx, db); err != nil {
		t.Fatal(err)
	}
	var count int
	var category string
	var priority int
	if err := db.QueryRow(`SELECT COUNT(*) FROM name_rules`).Scan(&count); err != nil || count != 20 {
		t.Fatalf("after re-seed: %d rules, %v", count, err)
	}
	if err := db.QueryRow(`SELECT category, priority FROM name_rules WHERE pattern = '%citi card%'`).Scan(&category, &priority); err != nil || category != ExcludedCategory || priority != 50 {
		t.Fatalf("re-seed did not restore the row: %s %d %v", category, priority, err)
	}
	for _, c := range []string{"Loans", "Car Lease", "Mortgage", "Bank Fees", "Income", ExcludedCategory} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM categories WHERE name = ?`, c).Scan(&n); err != nil || n != 1 {
			t.Fatalf("category %s seeded = %d, %v", c, n, err)
		}
	}
	syncPage(t, db, "c1",
		bankTxn("card", "*6229 PAYMENT CITI CARD ONLINE ACH WEB", 1000),
		bankTxn("thanks", "ONLINE PAYMENT, THANK YOU", -1000),
		bankTxn("lease", "KMFUSA LEASE PYMT", 384.73),
		bankTxn("mortgage", "MR COOPER MTG PYT", 1951.40),
		bankTxn("payroll", "ACME CORP PAYROLL", -3000),
		bankTxn("zelle", "ZEL TO LISA WATTERS", 40),
		bankTxn("interest", "INTEREST CHARGE ON PURCHASES", 12.34),
		bankTxn("mystery", "SOMETHING ELSE", 5),
	)
	matched, unmatched, err := Categorize(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if matched != 7 || unmatched != 1 {
		t.Fatalf("matched, unmatched = %d, %d", matched, unmatched)
	}
	for id, want := range map[string]string{"card": ExcludedCategory, "thanks": ExcludedCategory, "lease": "Car Lease", "mortgage": "Mortgage",
		"payroll": "Income", "zelle": ExcludedCategory, "interest": "Bank Fees"} {
		if got := entryCategory(t, db, id); got != want {
			t.Fatalf("%s = %q, want %q", id, got, want)
		}
	}
}
