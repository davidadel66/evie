// Package finance owns the personal-finance domain: linking banks through
// Plaid, syncing transactions into a local SQLite database
// (~/.finance/finance.db), and rule-based categorization. Functions return
// data and errors rather than printing, so the finance CLI and the agent
// can both drive the same logic and render results their own way (the
// interactive Link flow is the current exception). Plaid access tokens
// live in the database, which is why it is created 0600.
package finance

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// schema is the full DDL, applied on every open. Every statement is
// CREATE IF NOT EXISTS so reopening an existing database is a no-op —
// there is no separate migration step.
const schema = `
CREATE TABLE IF NOT EXISTS items (
	item_id      TEXT PRIMARY KEY,
	access_token TEXT NOT NULL,
	cursor       TEXT NOT NULL DEFAULT '',
	institution  TEXT,
	linked_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS transactions (
	transaction_id  TEXT PRIMARY KEY,
	item_id         TEXT NOT NULL,
	account_id      TEXT,
	date            TEXT,
	name            TEXT,
	merchant_name   TEXT,
	amount_cents    INTEGER NOT NULL,
	plaid_category  TEXT,
	category        TEXT,
	category_source TEXT,
	reviewed        INTEGER NOT NULL DEFAULT 0,
	pending         INTEGER NOT NULL DEFAULT 0,
	tags            TEXT NOT NULL DEFAULT '[]',
	FOREIGN KEY (item_id) REFERENCES items(item_id)
);

CREATE TABLE IF NOT EXISTS categories (
	name TEXT PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS rules (
	id       INTEGER PRIMARY KEY,
	merchant TEXT NOT NULL UNIQUE,
	category TEXT NOT NULL REFERENCES categories(name)
);

CREATE TABLE IF NOT EXISTS budget_entries (
	id             INTEGER PRIMARY KEY,
	transaction_id TEXT NOT NULL REFERENCES transactions(transaction_id),
	category       TEXT NOT NULL REFERENCES categories(name),
	amount_cents   INTEGER NOT NULL,
	source         TEXT NOT NULL DEFAULT 'rule',
	tags           TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS budget_limits (
	category    TEXT NOT NULL REFERENCES categories(name),
	month       TEXT,
	limit_cents INTEGER NOT NULL,
	UNIQUE(category, month)
);
`

// OpenDB opens (creating if needed) the canonical database at
// ~/.finance/finance.db, ensuring the directory exists first. All
// callers — CLI commands and agent tools alike — go through here so
// there is exactly one database location.
func OpenDB() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".finance")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make dir: %w", err)
	}
	return openDBAt(filepath.Join(dir, "finance.db"))
}

func OpenDBReadOnly() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(home, ".finance", "finance.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open db readonly error: %w", err)
	}

	return db, nil
}

// openDBAt opens a database at an explicit path, applies the schema, and
// locks the file down to 0600 (access tokens live in it). Foreign keys are
// enabled via the _pragma query parameter rather than an Exec because the
// pragma applies per connection — this way every pooled connection gets
// it, not just the first. Split from OpenDB so tests can use a temp path.
func openDBAt(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("secure db: %w", err)
	}
	return db, nil
}
