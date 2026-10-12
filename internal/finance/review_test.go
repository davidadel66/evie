package finance

import (
	"context"
	"strings"
	"testing"
)

// Candidates are the unclassified posted rows without a recorded decision,
// recurring merchants first; each carries deterministic evidence (a
// matching rule after normalization, categorized merchants sharing the
// first significant token, bank strings sharing their first two tokens)
// and a suggestion only when every piece of evidence agrees and is a rule
// or at least two rows. Nothing private leaves the read.
func TestReviewCandidatesGatherEvidenceDeterministically(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Notion", "Subscriptions")
	insertRule(t, db, "Kazoo Parking Kiosks", "Parking")
	for _, row := range []struct {
		id, date, name, merchant string
		cents                    int64
	}{
		{"known-1", "2026-08-01", "GOOGLE *ONE", "Google", 299},
		{"known-2", "2026-08-02", "GOOGLE *STORAGE", "Google", 299},
		{"known-3", "2026-08-03", "CONSUMERS", "Consumers Energy", 20000},
		{"known-4", "2026-08-04", "*8384 TRANSFER CONSUMERS CU ACH WEB", "", 50000},
		{"c-notion-a", "2026-09-30", "NOTION LABS", "Notion Labs Inc.", 2400},
		{"c-notion-b", "2026-08-30", "NOTION LABS", "Notion Labs Inc.", 2400},
		{"c-google", "2026-09-29", "GOOGLE *CLOUD", "Google Cloud", 3750},
		{"c-kazoo", "2026-09-28", "KAZOO PARKING", "Kazoo Parking", 150},
		{"c-transfer", "2026-09-27", "*8384 TRANSFER CONSUMERS CU ACH WEB", "", 38700},
		{"c-mystery", "2026-09-26", "ZZZ", "Mystery Shop", 500},
		{"c-decided", "2026-09-25", "DECIDED", "Decided Shop", 500},
		{"c-pending", "2026-09-24", "PENDING", "Pending Shop", 500},
	} {
		if _, err := db.Exec(`INSERT INTO transactions (transaction_id, item_id, date, name, merchant_name, amount_cents, pending, account_id, tags)
			VALUES (?, 'item-1', ?, ?, ?, ?, ?, 'private-account', '["private-tag"]')`, row.id, row.date, row.name, row.merchant, row.cents, row.id == "c-pending"); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []struct{ id, category, source string }{
		{"known-1", "Subscriptions", "rule"}, {"known-2", "Subscriptions", "human"}, {"known-3", "Utilities", "rule"}, {"known-4", ExcludedCategory, "human"},
	} {
		if _, err := db.Exec(`INSERT OR IGNORE INTO categories (name) VALUES (?)`, e.category); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO budget_entries (transaction_id, category, amount_cents, source) SELECT transaction_id, ?, amount_cents, ? FROM transactions WHERE transaction_id = ?`, e.category, e.source, e.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO review_decisions (transaction_id, decision, run_key, decided_at) VALUES ('c-decided', 'rejected', 'k', 'now')`); err != nil {
		t.Fatal(err)
	}

	report, err := ReviewCandidates(ctx, db, 4)
	if err != nil {
		t.Fatal(err)
	}
	if report.Remaining != 2 || len(report.Candidates) != 4 {
		t.Fatalf("batch = %d candidates, %d remaining", len(report.Candidates), report.Remaining)
	}
	// Recurring Notion first (both rows), then newest first.
	ids := []string{}
	for _, c := range report.Candidates {
		ids = append(ids, c.ID)
	}
	if strings.Join(ids, ",") != "c-notion-a,c-notion-b,c-google,c-kazoo" {
		t.Fatalf("order = %v", ids)
	}
	by := map[string]ReviewCandidate{}
	for _, c := range report.Candidates {
		by[c.ID] = c
	}
	notion := by["c-notion-a"]
	if notion.SuggestedCategory != "Subscriptions" || len(notion.Similar) != 1 || notion.Similar[0].Source != "rule" || notion.Similar[0].Label != "Notion" ||
		!strings.Contains(notion.SuggestionReason, "rule") || notion.NetCents != "-2400" {
		t.Fatalf("notion = %+v", notion)
	}
	google := by["c-google"]
	if google.SuggestedCategory != "Subscriptions" || len(google.Similar) != 2 || google.Similar[0].Count+google.Similar[1].Count != 2 {
		t.Fatalf("google = %+v", google)
	}
	kazoo := by["c-kazoo"]
	if kazoo.SuggestedCategory != "Parking" || len(kazoo.Similar) != 1 || kazoo.Similar[0].Label != "Kazoo Parking Kiosks" {
		// The longer rule name shares its first significant token.
		t.Fatalf("kazoo = %+v", kazoo)
	}
	full, err := ReviewCandidates(ctx, db, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range full.Candidates {
		switch c.ID {
		case "c-transfer":
			if c.SuggestedCategory != "" || len(c.Similar) != 1 || c.Similar[0].Category != ExcludedCategory || c.Similar[0].Count != 1 {
				t.Fatalf("transfer evidence = %+v", c)
			}
		case "c-mystery":
			if len(c.Similar) != 0 || c.SuggestedCategory != "" {
				t.Fatalf("mystery = %+v", c)
			}
		case "c-decided", "c-pending", "known-1":
			t.Fatalf("%s must not be a candidate", c.ID)
		}
	}
	if len(full.Candidates) != 6 || full.Remaining != 0 {
		t.Fatalf("full batch = %d, remaining %d", len(full.Candidates), full.Remaining)
	}
	if strings.Join(report.Categories, ",") != "excluded,Parking,Subscriptions,Utilities" {
		t.Fatalf("categories = %v", report.Categories)
	}
}
