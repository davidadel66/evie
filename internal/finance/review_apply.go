package finance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ApplyReview (finance-review.spec.md): the one write of the review
// workflow, performed only after the owner answered. Each line not
// rejected gets one full-amount budget entry with source 'agent', inserted
// through the same rechecks Categorize uses (posted, same amount, no entry
// yet); a line that no longer qualifies is counted as skipped, never
// forced. A line with rule, not listed under no_rule, mints the exact
// merchant rule with ON CONFLICT DO NOTHING: an existing rule is the
// owner's earlier decision and is never overwritten here. Every line gets
// a review_decisions row so a rejected transaction is not proposed again.
// The effect receipt is recorded in the same transaction as the writes,
// so a replay by key returns the recorded output and writes nothing.

// ReviewLine is one gated proposal the owner reviewed.
type ReviewLine struct {
	ID           string  `json:"id"`
	Date         string  `json:"date"`
	Name         string  `json:"name"`
	MerchantName string  `json:"merchant_name"`
	NetCents     int64   `json:"net_cents"`
	Category     string  `json:"category"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
	Rule         bool    `json:"rule"`
}

// ReviewChange is the owner's category override for one line.
type ReviewChange struct {
	ID       string `json:"id"`
	Category string `json:"category"`
}

// ReviewAnswer is the owner's answer: accept everything except the listed
// exceptions.
type ReviewAnswer struct {
	Reject []string       `json:"reject"`
	Change []ReviewChange `json:"change"`
	NoRule []string       `json:"no_rule"`
}

// ApplyReviewInput is the effect's input.
type ApplyReviewInput struct {
	Lines  []ReviewLine `json:"lines"`
	Answer ReviewAnswer `json:"answer"`
	RunKey string       `json:"run_key"`
}

// ApplyReviewOutput is the effect's response.
type ApplyReviewOutput struct {
	Applied     int  `json:"applied"`
	RulesMinted int  `json:"rules_minted"`
	Rejected    int  `json:"rejected"`
	Skipped     int  `json:"skipped"`
	Replayed    bool `json:"replayed"`
}

// EffectApplyReview names the receipt rows ApplyReview records.
const EffectApplyReview = "apply_review"

// ReviewSource is the budget_entries.source of an entry the owner approved
// from a review batch.
const ReviewSource = "agent"

// ApplyReview applies one answered batch once under key.
func ApplyReview(ctx context.Context, db *sql.DB, key string, in ApplyReviewInput) (ApplyReviewOutput, error) {
	var out ApplyReviewOutput
	if key == "" {
		return out, ErrEffectKeyRequired
	}
	if len(in.Lines) > ReviewMaxCandidates {
		return out, fmt.Errorf("a review batch holds at most %d lines", ReviewMaxCandidates)
	}
	rejected := map[string]bool{}
	for _, id := range in.Answer.Reject {
		rejected[id] = true
	}
	noRule := map[string]bool{}
	for _, id := range in.Answer.NoRule {
		noRule[id] = true
	}
	changed := map[string]string{}
	for _, c := range in.Answer.Change {
		if strings.TrimSpace(c.Category) == "" {
			return out, fmt.Errorf("change for %s names no category", c.ID)
		}
		changed[c.ID] = c.Category
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return out, fmt.Errorf("begin apply review: %w", err)
	}
	defer tx.Rollback()
	if found, err := lookupReceipt(ctx, tx, key, &out); err != nil || found {
		out.Replayed = found
		return out, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	seen := map[string]bool{}
	for _, line := range in.Lines {
		if strings.TrimSpace(line.ID) == "" || seen[line.ID] {
			return out, fmt.Errorf("review line %q is empty or repeated", line.ID)
		}
		seen[line.ID] = true
		decision, category := ReviewAccepted, line.Category
		if override, ok := changed[line.ID]; ok {
			decision, category = ReviewChanged, override
		}
		if rejected[line.ID] {
			decision = ReviewRejected
		}
		if decision != ReviewRejected {
			if strings.TrimSpace(category) == "" {
				return out, fmt.Errorf("review line %s has no category", line.ID)
			}
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO categories (name) VALUES (?)`, category); err != nil {
				return out, fmt.Errorf("insert category %q: %w", category, err)
			}
			// The same fence Categorize uses: posted, the amount the owner
			// saw, and still no entry. A transaction that changed under the
			// review is skipped and left for the next batch.
			res, err := tx.ExecContext(ctx, `
				INSERT INTO budget_entries (transaction_id, category, amount_cents, source)
				SELECT t.transaction_id, ?, t.amount_cents, ? FROM transactions t
				WHERE t.transaction_id = ? AND t.pending = 0 AND t.amount_cents = ?
				  AND NOT EXISTS (SELECT 1 FROM budget_entries e WHERE e.transaction_id = t.transaction_id)`,
				category, ReviewSource, line.ID, -line.NetCents)
			if err != nil {
				return out, fmt.Errorf("insert entry for %s: %w", line.ID, err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return out, err
			}
			if n == 0 {
				out.Skipped++
				continue
			}
			out.Applied++
			if line.Rule && !noRule[line.ID] && strings.TrimSpace(line.MerchantName) != "" {
				res, err := tx.ExecContext(ctx, `INSERT INTO rules (merchant, category) VALUES (?, ?) ON CONFLICT(merchant) DO NOTHING`, line.MerchantName, category)
				if err != nil {
					return out, fmt.Errorf("insert rule for %q: %w", line.MerchantName, err)
				}
				if n, _ := res.RowsAffected(); n == 1 {
					out.RulesMinted++
				}
			}
		} else {
			out.Rejected++
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO review_decisions (transaction_id, decision, category, reason, run_key, decided_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(transaction_id) DO UPDATE SET decision = excluded.decision, category = excluded.category,
				reason = excluded.reason, run_key = excluded.run_key, decided_at = excluded.decided_at`,
			line.ID, decision, category, line.Reason, in.RunKey, now); err != nil {
			return out, fmt.Errorf("record decision for %s: %w", line.ID, err)
		}
	}
	if err := recordReceipt(ctx, tx, key, EffectApplyReview, out); err != nil {
		return out, err
	}
	if err := tx.Commit(); err != nil {
		return out, fmt.Errorf("commit apply review: %w", err)
	}
	return out, nil
}
