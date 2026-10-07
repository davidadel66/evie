package finance

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Name rule seed data (categorize-patterns.spec.md, reviewed with the
// owner transaction by transaction on 2026-08-07). It is checked in
// because it encodes decisions, and embedded because the seeder runs
// inside whichever binary is current (the finance CLI, the finance_rules
// chat tool, the plugin) and none of them has a repository path at
// runtime. Changing a pattern, category or priority here and re-seeding
// updates the row; removing one does not delete it, exactly as the
// merchant lookup behaves.
//
//go:embed data/name_rules.json
var nameRulesSeed []byte

// NameRule is one seeded name pattern.
type NameRule struct {
	Pattern  string `json:"pattern"`
	Category string `json:"category"`
	Priority int    `json:"priority"`
}

// SeedNameRules parses the embedded seed file.
func SeedNameRules() ([]NameRule, error) {
	var rules []NameRule
	if err := json.Unmarshal(nameRulesSeed, &rules); err != nil {
		return nil, fmt.Errorf("parse name rules seed: %w", err)
	}
	seen := make(map[string]bool, len(rules))
	for i, r := range rules {
		switch {
		case strings.TrimSpace(r.Pattern) == "":
			return nil, fmt.Errorf("name rules seed [%d]: empty pattern", i)
		case strings.TrimSpace(r.Category) == "":
			return nil, fmt.Errorf("name rules seed [%d] %q: empty category", i, r.Pattern)
		case r.Priority < 0:
			return nil, fmt.Errorf("name rules seed [%d] %q: negative priority", i, r.Pattern)
		case seen[strings.ToLower(r.Pattern)]:
			return nil, fmt.Errorf("name rules seed [%d]: pattern %q repeats", i, r.Pattern)
		}
		seen[strings.ToLower(r.Pattern)] = true
	}
	return rules, nil
}

// NameRulesSeed loads the embedded name rules into name_rules (and any
// new categories into categories) in one transaction, with upsert
// semantics on the pattern. It returns the number of rules loaded.
func NameRulesSeed(ctx context.Context, db *sql.DB) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	rules, err := SeedNameRules()
	if err != nil {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin name rules seed: %w", err)
	}
	defer tx.Rollback()
	for _, r := range rules {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO categories (name) VALUES (?)`, r.Category); err != nil {
			return 0, fmt.Errorf("insert category %q: %w", r.Category, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO name_rules (pattern, category, priority) VALUES (?, ?, ?)
			ON CONFLICT(pattern) DO UPDATE SET category = excluded.category, priority = excluded.priority`,
			r.Pattern, r.Category, r.Priority); err != nil {
			return 0, fmt.Errorf("insert name rule %q: %w", r.Pattern, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit name rules seed: %w", err)
	}
	return len(rules), nil
}
