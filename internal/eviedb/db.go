package eviedb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

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

CREATE TABLE IF NOT EXISTS projects (
    id             TEXT PRIMARY KEY NOT NULL,
    display_name   TEXT NOT NULL,
    canonical_root TEXT NOT NULL UNIQUE,
    archived       INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id                    TEXT PRIMARY KEY NOT NULL,
    project_id            TEXT REFERENCES projects(id),
    project_root_snapshot TEXT,
    parent_session_id     TEXT REFERENCES sessions(id),
    status                TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL,
    CHECK (
        (project_id IS NULL AND project_root_snapshot IS NULL) OR
        (project_id IS NOT NULL AND project_root_snapshot IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS sessions_project_id_idx ON sessions(project_id);
CREATE INDEX IF NOT EXISTS sessions_parent_session_id_idx ON sessions(parent_session_id);

CREATE TRIGGER IF NOT EXISTS sessions_scope_immutable
BEFORE UPDATE OF project_id, project_root_snapshot, parent_session_id ON sessions
FOR EACH ROW
WHEN NEW.project_id IS NOT OLD.project_id
    OR NEW.project_root_snapshot IS NOT OLD.project_root_snapshot
    OR NEW.parent_session_id IS NOT OLD.parent_session_id
BEGIN
    SELECT RAISE(ABORT, 'session scope is immutable');
END;

CREATE TABLE IF NOT EXISTS session_turn_leases (
    session_id       TEXT PRIMARY KEY NOT NULL REFERENCES sessions(id),
    holder_id        TEXT CHECK (holder_id IS NULL OR length(trim(holder_id)) > 0),
    fencing_token    INTEGER NOT NULL CHECK (typeof(fencing_token) = 'integer' AND fencing_token > 0),
    lease_generation INTEGER NOT NULL CHECK (typeof(lease_generation) = 'integer' AND lease_generation > 0),
    expires_at       TEXT,
    CHECK (fencing_token = lease_generation),
    CHECK (
        (holder_id IS NULL AND expires_at IS NULL) OR
        (holder_id IS NOT NULL AND expires_at IS NOT NULL)
    )
);

CREATE TABLE IF NOT EXISTS events (
    id             TEXT PRIMARY KEY NOT NULL,
    session_id     TEXT NOT NULL REFERENCES sessions(id),
    sequence       INTEGER NOT NULL CHECK (sequence > 0),
    project_id     TEXT REFERENCES projects(id),
    parent_id      TEXT,
    event_type     TEXT NOT NULL CHECK (length(trim(event_type)) > 0),
    role           TEXT CHECK (role IS NULL OR role IN ('user', 'assistant', 'tool')),
    execution_id   TEXT,
    content        TEXT NOT NULL DEFAULT '',
    payload_json   TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload_json)),
    recorded_at    TEXT NOT NULL,
    format_version INTEGER NOT NULL DEFAULT 1 CHECK (format_version > 0),
    UNIQUE (session_id, sequence),
    UNIQUE (id, session_id),
    FOREIGN KEY (parent_id, session_id) REFERENCES events(id, session_id)
);

CREATE INDEX IF NOT EXISTS events_project_id_idx ON events(project_id);
CREATE INDEX IF NOT EXISTS events_parent_id_idx ON events(parent_id);
CREATE INDEX IF NOT EXISTS events_execution_id_idx ON events(execution_id) WHERE execution_id IS NOT NULL;

CREATE TRIGGER IF NOT EXISTS events_scope_matches_session
BEFORE INSERT ON events
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1
    FROM sessions
    WHERE sessions.id = NEW.session_id
      AND sessions.project_id IS NEW.project_id
)
BEGIN
    SELECT RAISE(ABORT, 'event scope does not match session scope');
END;

CREATE TRIGGER IF NOT EXISTS events_append_only_update
BEFORE UPDATE ON events
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS events_append_only_delete
BEFORE DELETE ON events
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'events are append-only');
END;
`

const (
	dsnPragmas        = "?" + connectionPragmas
	connectionPragmas = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
)

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

func OpenDBReadOnly() (*sql.DB, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(home, ".evie", "evie.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&"+connectionPragmas)
	if err != nil {
		return nil, fmt.Errorf("open db readonly: %w", err)
	}

	return db, nil
}

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
