package finance

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type candidate struct {
	id       string
	merchant string
}

func matchTxn(rules map[string]string, merchantName string) (category string, ok bool) {
	category, ok = rules[strings.ToLower(merchantName)]
	return category, ok
}

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

	txns, err := db.Query(`
		SELECT transaction_id, COALESCE(merchant_name, '') FROM transactions
		WHERE reviewed = 0 AND (category_source IS NULL OR category_source = 'rule')`)
	if err != nil {
		return 0, 0, fmt.Errorf("load transactions: %w", err)
	}

	defer txns.Close()

	var candidates []candidate
	for txns.Next() {
		var c candidate
		if err := txns.Scan(&c.id, &c.merchant); err != nil {
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
		category, ok := matchTxn(rules, c.merchant)
		if !ok {
			unmatched++
			continue
		}

		if _, err := tx.Exec(`
			UPDATE transactions SET category = ?, category_source = 'rule'
			WHERE transaction_id = ?
			`, category, c.id); err != nil {
			return matched, unmatched, fmt.Errorf("update transactions: %w", err)
		}
		matched++
	}

	if err := tx.Commit(); err != nil {
		return matched, unmatched, fmt.Errorf("commit categorize: %w", err)
	}

	return matched, unmatched, nil
}
