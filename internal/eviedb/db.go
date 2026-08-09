// Package eviedb owns evie's own state database at ~/.evie/evie.db —
// the first customer is the cron tables (jobs + job_runs), with future
// evie-internal state expected to land here rather than growing new
// files. Mirrors internal/finance/db.go: one CREATE IF NOT EXISTS blob
// applied on every open, no migrations.
package eviedb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// schema is applied in full on every open; every statement must stay
// idempotent (CREATE TABLE IF NOT EXISTS), which is this package's whole
// migration story, same as finance.
//
// jobs is the cron ledger — the source of truth that launchd plists are
// generated FROM, never reconciled back to. job_runs is append-only
// history whose rows deliberately outlive their job: job_id points at
// jobs.id logically but carries NO foreign-key constraint, because the
// audit trail surviving cron_remove is the point of the table, and an
// enforced REFERENCES would make deleting any job that ever ran fail
// (caught by code review — the constraint and the design contradicted
// each other, and the constraint lost). enabled is forward provision
// for a future cron_pause: v1 always writes 1 and never reads it.
const schema = `
CREATE TABLE IF NOT EXISTS jobs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    schedule   TEXT NOT NULL,
    command    TEXT NOT NULL,
    created_at TEXT NOT NULL,
    enabled    INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS job_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id      INTEGER NOT NULL,
    started_at  TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    exit_code   INTEGER NOT NULL,
    output      TEXT NOT NULL
);
`

// dsnPragmas ride the DSN rather than an Exec so every pooled connection
// gets them, not just the first. busy_timeout is the one finance doesn't
// carry, and here it is load-bearing: a launchd-spawned cron-exec writes
// this db while the REPL may hold it open, and modernc.org/sqlite with
// no busy_timeout returns SQLITE_BUSY immediately instead of waiting —
// losing the run row exactly when evie is in use.
//
// No foreign_keys pragma: nothing in this schema declares a foreign key
// (see job_runs above), so enabling enforcement would be provision for a
// constraint the design deliberately refuses.
const dsnPragmas = "?_pragma=busy_timeout(5000)"

// OpenDB opens (creating if needed) the canonical database at
// ~/.evie/evie.db, ensuring the directory exists first. All callers —
// cron tools and the cron-exec subcommand alike — go through here so
// there is exactly one database location.
func OpenDB() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".evie")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make dir: %w", err)
	}
	return OpenDBAt(filepath.Join(dir, "evie.db"))
}

// OpenDBReadOnly opens the canonical database engine-level read-only,
// for query_db. No schema exec — reading must not create anything.
func OpenDBReadOnly() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(home, ".evie", "evie.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open db readonly: %w", err)
	}

	return db, nil
}

// OpenDBAt opens a database at an explicit path, applies the schema, and
// locks the file down to 0600. Split from OpenDB so tests can use a temp
// path — exported because internal/tools' cron tests need a db shaped by
// this exact schema and these exact pragmas. Hand-copying the DDL into a
// test file let the real schema drift from the tested one (a dropped
// foreign key stayed green for exactly that reason); there is now one
// definition and every caller, test or not, goes through it.
func OpenDBAt(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+dsnPragmas)
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
