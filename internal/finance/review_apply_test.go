package finance

import (
	"context"
	"testing"
)

// ApplyReview writes only what the owner accepted, through Categorize's
// fence, mints exact rules without overwriting existing ones, records a
// decision for every line, and replays by key without writing again.
func TestApplyReviewWritesAcceptedLinesOnce(t *testing.T) {
	ctx := context.Background()
	db := newTestDB(t)
	insertItem(t, db, "item-1")
	insertRule(t, db, "Roca", "Dining")
	for _, row := range []struct {
		id, merchant string
		cents        int64
	}{{"notion", "Notion Labs Inc.", 2400}, {"roca", "Roca", 10800}, {"watch", "Watch It", 249}, {"transfer", "", 38700}, {"changed", "Mystery", 500}} {
		tx := txn(row.id, float64(row.cents)/100)
		tx.MerchantName = row.merchant
		syncPage(t, db, "c-"+row.id, tx)
	}
	lines := []ReviewLine{
		{ID: "notion", MerchantName: "Notion Labs Inc.", NetCents: -2400, Category: "Subscriptions", Rule: true, Reason: "rule for Notion"},
		{ID: "roca", MerchantName: "Roca", NetCents: -10800, Category: "Coffee", Rule: true},           // existing Dining rule must survive
		{ID: "watch", MerchantName: "Watch It", NetCents: -249, Category: "Entertainment", Rule: true}, // rejected
		{ID: "transfer", MerchantName: "", NetCents: -38700, Category: ExcludedCategory, Rule: false},
		{ID: "changed", MerchantName: "Mystery", NetCents: -500, Category: "Shopping", Rule: true}, // changed to Dining, no rule
	}
	answer := ReviewAnswer{Reject: []string{"watch"}, Change: []ReviewChange{{ID: "changed", Category: "Dining"}}, NoRule: []string{"changed"}}
	out, err := ApplyReview(ctx, db, "k1", ApplyReviewInput{Lines: lines, Answer: answer, RunKey: "finance-review:x"})
	if err != nil {
		t.Fatal(err)
	}
	if out != (ApplyReviewOutput{Applied: 4, RulesMinted: 1, Rejected: 1, Skipped: 0}) {
		t.Fatalf("output = %+v", out)
	}
	for id, want := range map[string]string{"notion": "Subscriptions", "roca": "Coffee", "transfer": ExcludedCategory, "changed": "Dining"} {
		var category, source string
		if err := db.QueryRow(`SELECT category, source FROM budget_entries WHERE transaction_id = ?`, id).Scan(&category, &source); err != nil || category != want || source != ReviewSource {
			t.Fatalf("%s entry = %s/%s, %v", id, category, source, err)
		}
	}
	if n := entryCount(t, db, "watch"); n != 0 {
		t.Fatalf("rejected line minted %d entries", n)
	}
	var rocaRule, notionRule string
	if err := db.QueryRow(`SELECT category FROM rules WHERE merchant = 'Roca'`).Scan(&rocaRule); err != nil || rocaRule != "Dining" {
		t.Fatalf("existing rule overwritten: %s %v", rocaRule, err)
	}
	if err := db.QueryRow(`SELECT category FROM rules WHERE merchant = 'Notion Labs Inc.'`).Scan(&notionRule); err != nil || notionRule != "Subscriptions" {
		t.Fatalf("notion rule = %s %v", notionRule, err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM rules WHERE merchant = 'Mystery'`).Scan(&n); err != nil || n != 0 {
		t.Fatal("no_rule line minted a rule")
	}
	decisions := map[string]string{}
	rows, err := db.Query(`SELECT transaction_id, decision FROM review_decisions`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, d string
		if err := rows.Scan(&id, &d); err != nil {
			t.Fatal(err)
		}
		decisions[id] = d
	}
	rows.Close()
	if len(decisions) != 5 || decisions["watch"] != ReviewRejected || decisions["changed"] != ReviewChanged || decisions["notion"] != ReviewAccepted {
		t.Fatalf("decisions = %v", decisions)
	}
	// The rejected row is no longer a candidate.
	report, err := ReviewCandidates(ctx, db, 40)
	if err != nil || len(report.Candidates) != 0 {
		t.Fatalf("candidates after review = %d, %v", len(report.Candidates), err)
	}
	// Replay by key writes nothing more.
	again, err := ApplyReview(ctx, db, "k1", ApplyReviewInput{Lines: lines, Answer: answer})
	if err != nil || !again.Replayed || again.Applied != 4 {
		t.Fatalf("replay = %+v, %v", again, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM budget_entries`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("entries after replay = %d", n)
	}
	// A line whose transaction changed under the review is skipped, not forced.
	syncPage(t, db, "c-late", txn("late", 9.99))
	if _, err := db.Exec(`UPDATE transactions SET amount_cents = 1099 WHERE transaction_id = 'late'`); err != nil {
		t.Fatal(err)
	}
	late, err := ApplyReview(ctx, db, "k2", ApplyReviewInput{Lines: []ReviewLine{{ID: "late", MerchantName: "Coffee Shop Inc", NetCents: -999, Category: "Coffee"}}})
	if err != nil || late.Skipped != 1 || late.Applied != 0 || entryCount(t, db, "late") != 0 {
		t.Fatalf("stale line = %+v, %v", late, err)
	}
}
