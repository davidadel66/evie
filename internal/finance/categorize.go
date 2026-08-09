package finance

import (
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
	amountCents int64
	category    string
	source      string
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
func RulesSeed(db *sql.DB, path string) error {
	merchantLookup, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read merchant lookup: %w", err)
	}

	var lookup map[string]string

	if err := json.Unmarshal(merchantLookup, &lookup); err != nil {
		return fmt.Errorf("parse merchant lookup: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin seed: %w", err)
	}

	defer tx.Rollback()

	for merchant, category := range lookup {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO categories (name) VALUES (?)`, category,
		); err != nil {
			return fmt.Errorf("insert category %q: %w", category, err)
		}

		if _, err := tx.Exec(
			`INSERT INTO rules (merchant, category) VALUES (?, ?)
			ON CONFLICT(merchant) DO UPDATE SET category = excluded.category`, merchant, category,
		); err != nil {
			return fmt.Errorf("insert rule %q: %w", merchant, err)
		}
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
func Categorize(db *sql.DB) (matched, unmatched int, err error) {
	rules := make(map[string]string)

	rows, err := db.Query(`SELECT merchant, category FROM rules`)
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
	txns, err := db.Query(`
		SELECT t.transaction_id, COALESCE(t.merchant_name, ''), t.amount_cents,
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
		if err := txns.Scan(&c.id, &c.merchant, &c.amountCents, &c.category, &c.source); err != nil {
			return 0, 0, fmt.Errorf("scan transactions: %w", err)
		}
		candidates = append(candidates, c)
	}

	if err := txns.Err(); err != nil {
		return 0, 0, fmt.Errorf("read transactions: %w", err)
	}

	txns.Close()

	tx, err := db.Begin()
	if err != nil {
		return 0, 0, fmt.Errorf("begin categorize: %w", err)
	}

	defer tx.Rollback()

	for _, c := range candidates {
		category, source := c.category, c.source
		if category == "" {
			var ok bool
			category, ok = matchTxn(rules, c.merchant)
			if !ok {
				unmatched++
				continue
			}
			source = "rule"
		}

		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO categories (name) VALUES (?)`, category,
		); err != nil {
			return matched, unmatched, fmt.Errorf("insert category %q: %w", category, err)
		}

		if _, err := tx.Exec(`
			INSERT INTO budget_entries (transaction_id, category, amount_cents, source)
			VALUES (?, ?, ?, ?)
			`, c.id, category, c.amountCents, source); err != nil {
			return matched, unmatched, fmt.Errorf("insert budget entry: %w", err)
		}
		matched++
	}

	if err := tx.Commit(); err != nil {
		return matched, unmatched, fmt.Errorf("commit categorize: %w", err)
	}

	return matched, unmatched, nil
}
