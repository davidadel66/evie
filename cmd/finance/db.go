package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

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
`

func openDB() (*sql.DB, error) {
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

func openDBAt(path string) (*sql.DB, error) {
	// _pragma applies per connection, so the FK constraint is enforced
	// on every pooled connection, not just the first.
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil { // secrets live here
		db.Close()
		return nil, fmt.Errorf("secure db: %w", err)
	}
	return db, nil
}
