package finance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// candidate is a transaction with no budget entry yet: what's needed to
// mint one — merchant for rule matching, amount for the entry itself,
// and any category the legacy column already carries.
type candidate struct {
	id          string
	merchant    string
	name        string
	amountCents int64
	category    string
	source      string
}

// ExcludedCategory is the category whose entries record that a
// transaction deliberately does not count as spending.
const ExcludedCategory = "excluded"

// matchName finds the name rule for a transaction with no merchant match:
// among the name_rules patterns that LIKE-match the raw bank string,
// case-insensitively and in SQL so the pattern semantics are exactly
// SQLite's, those at the lowest priority decide. If they all agree on a
// category that is the answer; if they disagree it is an error naming the
// transaction, the patterns and the priority. Silently choosing between
// two plausible categories for money is the failure this exists to avoid;
// two patterns that say the same thing are not a choice.
func matchName(ctx context.Context, tx *sql.Tx, c candidate) (category string, ok bool, err error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT pattern, category, priority FROM name_rules
		WHERE lower(?) LIKE lower(pattern)
		  AND priority = (SELECT MIN(priority) FROM name_rules WHERE lower(?) LIKE lower(pattern))
		ORDER BY pattern`, c.name, c.name)
	if err != nil {
		return "", false, fmt.Errorf("match name rules: %w", err)
	}
	defer rows.Close()
	var patterns []string
	var priority int64
	categories := map[string]bool{}
	for rows.Next() {
		var pattern, cat string
		if err := rows.Scan(&pattern, &cat, &priority); err != nil {
			return "", false, fmt.Errorf("scan name rule: %w", err)
		}
		patterns = append(patterns, pattern)
		categories[cat] = true
		category = cat
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("read name rules: %w", err)
	}
	switch {
	case len(patterns) == 0:
		return "", false, nil
	case len(categories) > 1:
		return "", false, fmt.Errorf("transaction %s %q matches name rules %s with different categories at the same priority %d; give one a lower priority",
			c.id, c.name, strings.Join(quoteAll(patterns), " and "), priority)
	}
	return category, true, nil
}

func quoteAll(items []string) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = fmt.Sprintf("%q", item)
	}
	return out
}

// matchTxn looks a merchant up in the rules map, case-insensitively (the
// map is keyed lowercase; the merchant is lowered here). The whole matching
// strategy lives in this one function so it can grow (prefixes, patterns)
// without touching Categorize.
func matchTxn(rules map[string]string, merchantName string) (category string, ok bool) {
	category, ok = rules[strings.ToLower(merchantName)]
	return category, ok
}

// RulesSeed loads a merchant→category JSON file into the rules table (and
// any new categories into categories), all in one transaction. Upsert
// semantics: re-seeding updates a merchant's category rather than
// duplicating or failing, so the JSON file stays the source of truth.
func RulesSeed(ctx context.Context, db *sql.DB, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	merchantLookup, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read merchant lookup: %w", err)
	}

	var lookup map[string]string

	if err := json.Unmarshal(merchantLookup, &lookup); err != nil {
		return fmt.Errorf("parse merchant lookup: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin seed: %w", err)
	}

	defer tx.Rollback()

	for merchant, category := range lookup {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO categories (name) VALUES (?)`, category,
		); err != nil {
			return fmt.Errorf("insert category %q: %w", category, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO rules (merchant, category) VALUES (?, ?)
			ON CONFLICT(merchant) DO UPDATE SET category = excluded.category`, merchant, category,
		); err != nil {
			return fmt.Errorf("insert rule %q: %w", merchant, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit seed: %w", err)
	}

	return nil
}

// Categorize mints a budget_entries row (full amount, one row per
// transaction) for every POSTED transaction that doesn't have one yet.
// Transactions whose legacy category column is already set — the old
// system's output, including human decisions — get their entry from it,
// preserving the source; bare transactions go through the rules table
// with source 'rule'. "Posted and no entry yet" is the candidate filter,
// which makes the pass idempotent (re-runs skip everything already
// entered) and makes the legacy backfill automatic. Unmatched
// transactions stay entry-less — that is the awaiting-review state. The
// legacy category column is no longer written.
func Categorize(ctx context.Context, db *sql.DB) (matched, unmatched int, err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin categorize: %w", err)
	}
	defer tx.Rollback()
	matched, unmatched, err = categorizeTx(ctx, tx)
	if err != nil {
		return matched, unmatched, err
	}
	if err := tx.Commit(); err != nil {
		return matched, unmatched, fmt.Errorf("commit categorize: %w", err)
	}
	return matched, unmatched, nil
}

// categorizeTx is Categorize inside one transaction the caller commits:
// the rules, the candidates and the entries are all read and written on
// tx, so a caller can record its own row (an effect receipt) atomically
// with the entries it minted.
func categorizeTx(ctx context.Context, tx *sql.Tx) (matched, unmatched int, err error) {
	rules := make(map[string]string)

	rows, err := tx.QueryContext(ctx, `SELECT merchant, category FROM rules`)
	if err != nil {
		return 0, 0, fmt.Errorf("load rules: %w", err)
	}

	defer rows.Close()

	for rows.Next() {
		var merchant, category string

		if err := rows.Scan(&merchant, &category); err != nil {
			return 0, 0, fmt.Errorf("scan rule: %w", err)
		}

		rules[strings.ToLower(merchant)] = category
	}

	if err := rows.Err(); err != nil {
		return 0, 0, fmt.Errorf("read rules: %w", err)
	}

	// Pending transactions are excluded: Plaid's normal lifecycle is to
	// issue a pending row, then remove it and add a posted one when it
	// settles. Categorizing the pending leg mints an entry at an amount
	// that is often wrong (gas holds, unadded tips) and that sync then has
	// to delete — so budget totals moved on their own and the delete
	// wedged the sync. The posted transaction gets categorized on the next
	// run; a day's lag beats an entry that is wrong and then vanishes.
	txns, err := tx.QueryContext(ctx, `
		SELECT t.transaction_id, COALESCE(t.merchant_name, ''), COALESCE(t.name, ''), t.amount_cents,
		       COALESCE(t.category, ''), COALESCE(t.category_source, 'rule')
		FROM transactions t
		WHERE t.pending = 0
		  AND NOT EXISTS (
			SELECT 1 FROM budget_entries e WHERE e.transaction_id = t.transaction_id
		)`)
	if err != nil {
		return 0, 0, fmt.Errorf("load transactions: %w", err)
	}

	defer txns.Close()

	var candidates []candidate
	for txns.Next() {
		var c candidate
		if err := txns.Scan(&c.id, &c.merchant, &c.name, &c.amountCents, &c.category, &c.source); err != nil {
			return 0, 0, fmt.Errorf("scan transactions: %w", err)
		}
		candidates = append(candidates, c)
	}

	if err := txns.Err(); err != nil {
		return 0, 0, fmt.Errorf("read transactions: %w", err)
	}

	txns.Close()

	for _, c := range candidates {
		category, source := c.category, c.source
		// Precedence: the legacy category column, then an exact merchant
		// rule (Plaid's own normalization, the most specific thing we
		// have), then a name rule on the raw bank string.
		if category == "" {
			var ok bool
			category, ok = matchTxn(rules, c.merchant)
			if !ok {
				category, ok, err = matchName(ctx, tx, c)
				if err != nil {
					return matched, unmatched, err
				}
			}
			if !ok {
				unmatched++
				continue
			}
			source = "rule"
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO categories (name) VALUES (?)`, category,
		); err != nil {
			return matched, unmatched, fmt.Errorf("insert category %q: %w", category, err)
		}

		inserted, err := insertCategorizedCandidate(ctx, tx, c, category, source)
		if err != nil {
			return matched, unmatched, fmt.Errorf("insert budget entry: %w", err)
		}
		if inserted {
			matched++
		}
	}

	if err := ctx.Err(); err != nil {
		return matched, unmatched, err
	}

	return matched, unmatched, nil
}

// Candidate discovery precedes the writes. Recheck its inputs and
// the entry-existence fence in the INSERT so a human edit or intervening sync
// cannot be followed by a duplicate entry or an allocation of a stale amount.
func insertCategorizedCandidate(ctx context.Context, tx *sql.Tx, c candidate, category, source string) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO budget_entries(transaction_id,category,amount_cents,source)
		SELECT t.transaction_id,?,t.amount_cents,? FROM transactions t
		WHERE t.transaction_id=? AND t.pending=0 AND t.amount_cents=?
		  AND COALESCE(t.merchant_name,'')=? AND COALESCE(t.category,'')=?
		  AND COALESCE(t.category_source,'rule')=?
		  AND NOT EXISTS(SELECT 1 FROM budget_entries e WHERE e.transaction_id=t.transaction_id)`,
		category, source, c.id, c.amountCents, c.merchant, c.category, c.source)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}
