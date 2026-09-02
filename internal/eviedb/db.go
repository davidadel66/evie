package eviedb

import (
	"context"
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

CREATE TABLE IF NOT EXISTS plugin_enabled_configuration (
    plugin_id  TEXT PRIMARY KEY NOT NULL CHECK (length(trim(plugin_id)) > 0),
    enabled    INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    revision   INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
    id          TEXT PRIMARY KEY NOT NULL CHECK (typeof(id) = 'text' AND length(trim(id)) > 0),
    scope       TEXT NOT NULL CHECK (typeof(scope) = 'text' AND scope = 'global'),
    title       TEXT NOT NULL CHECK (typeof(title) = 'text' AND length(trim(title)) > 0),
    description TEXT CHECK (description IS NULL OR typeof(description) = 'text'),
    priority    INTEGER CHECK (priority IS NULL OR (typeof(priority) = 'integer' AND priority BETWEEN 1 AND 5)),
    due_date    TEXT CHECK (due_date IS NULL OR typeof(due_date) = 'text'),
    status      TEXT NOT NULL CHECK (typeof(status) = 'text' AND status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
    revision    INTEGER NOT NULL CHECK (typeof(revision) = 'integer' AND revision > 0),
    created_at  TEXT NOT NULL CHECK (typeof(created_at) = 'text'),
    updated_at  TEXT NOT NULL CHECK (typeof(updated_at) = 'text')
);

CREATE INDEX IF NOT EXISTS tasks_scope_status_created_idx
ON tasks(scope, status, created_at, id);

CREATE TRIGGER IF NOT EXISTS tasks_no_hard_delete
BEFORE DELETE ON tasks
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'tasks cannot be hard deleted');
END;

CREATE TABLE IF NOT EXISTS task_events (
    id                 TEXT PRIMARY KEY NOT NULL CHECK (typeof(id) = 'text' AND length(trim(id)) > 0),
    task_id            TEXT NOT NULL REFERENCES tasks(id),
    sequence           INTEGER NOT NULL CHECK (typeof(sequence) = 'integer' AND sequence > 0),
    operation          TEXT NOT NULL CHECK (operation IN ('create', 'update')),
    actor_id           TEXT NOT NULL CHECK (typeof(actor_id) = 'text' AND length(trim(actor_id)) > 0),
    session_id         TEXT NOT NULL CHECK (typeof(session_id) = 'text' AND length(trim(session_id)) > 0),
    run_id             TEXT NOT NULL CHECK (typeof(run_id) = 'text' AND length(trim(run_id)) > 0),
    recorded_at        TEXT NOT NULL CHECK (typeof(recorded_at) = 'text'),
    previous_revision  INTEGER NOT NULL CHECK (typeof(previous_revision) = 'integer' AND previous_revision >= 0),
    resulting_revision INTEGER NOT NULL CHECK (typeof(resulting_revision) = 'integer' AND resulting_revision > 0),
    outcome            TEXT NOT NULL CHECK (outcome IN ('accepted', 'rejected')),
    diagnostic_code    TEXT CHECK (
        diagnostic_code IS NULL OR diagnostic_code IN ('invalid_input', 'invalid_transition', 'revision_conflict')
    ),
    UNIQUE (task_id, sequence),
    CHECK (
        (outcome = 'accepted' AND diagnostic_code IS NULL AND resulting_revision = previous_revision + 1) OR
        (outcome = 'rejected' AND diagnostic_code IS NOT NULL AND resulting_revision = previous_revision)
    )
);

CREATE INDEX IF NOT EXISTS task_events_task_sequence_idx
ON task_events(task_id, sequence);

CREATE TRIGGER IF NOT EXISTS task_events_append_only_update
BEFORE UPDATE ON task_events
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_events_append_only_delete
BEFORE DELETE ON task_events
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task events are append-only');
END;

CREATE TABLE IF NOT EXISTS projects (
    id             TEXT PRIMARY KEY NOT NULL,
    display_name   TEXT NOT NULL,
    canonical_root TEXT NOT NULL UNIQUE,
    archived       INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS workspaces (
    id                  TEXT PRIMARY KEY NOT NULL,
    display_name        TEXT NOT NULL,
    lifecycle_state     TEXT NOT NULL CHECK (lifecycle_state IN ('active', 'archived')),
    current_revision_id TEXT NOT NULL CHECK (length(trim(current_revision_id)) > 0),
    created_at          TEXT NOT NULL,
    updated_at          TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id                    TEXT PRIMARY KEY NOT NULL,
    project_id            TEXT REFERENCES projects(id),
    project_root_snapshot TEXT,
    parent_session_id     TEXT REFERENCES sessions(id),
    title                 TEXT,
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

CREATE TABLE IF NOT EXISTS session_composition_receipts (
    session_id   TEXT PRIMARY KEY NOT NULL REFERENCES sessions(id),
    receipt_json TEXT NOT NULL CHECK (json_valid(receipt_json) AND json_type(receipt_json) = 'object'),
    recorded_at  TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS session_composition_receipts_immutable_update
BEFORE UPDATE ON session_composition_receipts
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'composition receipts are immutable');
END;

CREATE TRIGGER IF NOT EXISTS session_composition_receipts_immutable_delete
BEFORE DELETE ON session_composition_receipts
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'composition receipts are immutable');
END;

CREATE TABLE IF NOT EXISTS session_compatibility_resolutions (
    session_id      TEXT NOT NULL REFERENCES session_composition_receipts(session_id),
    resolution_key  TEXT NOT NULL CHECK (length(resolution_key) = 64),
    resolution_json TEXT NOT NULL CHECK (json_valid(resolution_json) AND json_type(resolution_json) = 'object'),
    resolved_at     TEXT NOT NULL,
    PRIMARY KEY (session_id, resolution_key)
);

CREATE INDEX IF NOT EXISTS session_compatibility_resolutions_time_idx
ON session_compatibility_resolutions(session_id, resolved_at, resolution_key);

CREATE TRIGGER IF NOT EXISTS session_compatibility_resolutions_append_only_update
BEFORE UPDATE ON session_compatibility_resolutions
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'compatibility resolutions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS session_compatibility_resolutions_append_only_delete
BEFORE DELETE ON session_compatibility_resolutions
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'compatibility resolutions are append-only');
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

const workspaceScopeSchema = `
CREATE INDEX IF NOT EXISTS sessions_workspace_id_idx ON sessions(workspace_id);
CREATE INDEX IF NOT EXISTS events_workspace_id_idx ON events(workspace_id);

CREATE TRIGGER IF NOT EXISTS sessions_context_scope_valid_insert
BEFORE INSERT ON sessions
FOR EACH ROW
WHEN ((NEW.workspace_id IS NULL) <> (NEW.workspace_revision_snapshot IS NULL))
  OR (NEW.workspace_id IS NOT NULL AND NEW.project_id IS NOT NULL)
BEGIN
    SELECT RAISE(ABORT, 'session Context Scope must be exactly one Workspace, project, or neither');
END;

CREATE TRIGGER IF NOT EXISTS sessions_context_scope_valid_update
BEFORE UPDATE OF workspace_id, workspace_revision_snapshot, project_id, project_root_snapshot ON sessions
FOR EACH ROW
WHEN ((NEW.workspace_id IS NULL) <> (NEW.workspace_revision_snapshot IS NULL))
  OR (NEW.workspace_id IS NOT NULL AND NEW.project_id IS NOT NULL)
BEGIN
    SELECT RAISE(ABORT, 'session Context Scope must be exactly one Workspace, project, or neither');
END;

CREATE TRIGGER IF NOT EXISTS sessions_workspace_scope_immutable
BEFORE UPDATE OF workspace_id, workspace_revision_snapshot ON sessions
FOR EACH ROW
WHEN NEW.workspace_id IS NOT OLD.workspace_id
  OR NEW.workspace_revision_snapshot IS NOT OLD.workspace_revision_snapshot
BEGIN
    SELECT RAISE(ABORT, 'session scope is immutable');
END;

CREATE TRIGGER IF NOT EXISTS events_workspace_scope_matches_session
BEFORE INSERT ON events
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1 FROM sessions
    WHERE sessions.id = NEW.session_id
      AND sessions.workspace_id IS NEW.workspace_id
)
BEGIN
    SELECT RAISE(ABORT, 'event scope does not match session scope');
END;
`

const (
	dsnPragmas        = "?" + connectionPragmas
	connectionPragmas = "_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
)

func OpenDB() (*sql.DB, error) {
	return OpenDBContext(context.Background())
}

func OpenDBContext(ctx context.Context) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".evie")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make dir: %w", err)
	}
	return OpenDBAtContext(ctx, filepath.Join(dir, "evie.db"))
}

func OpenDBReadOnly() (*sql.DB, error) {
	return OpenDBReadOnlyContext(context.Background())
}

func OpenDBReadOnlyContext(ctx context.Context) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(home, ".evie", "evie.db")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&"+connectionPragmas)
	if err != nil {
		return nil, fmt.Errorf("open db readonly: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open db readonly: %w", err)
	}
	return db, nil
}

func OpenDBAt(path string) (*sql.DB, error) {
	return OpenDBAtContext(context.Background(), path)
}

type openDBAtHooks struct {
	afterSchema         func()
	sessionTitleUpgrade sessionTitleUpgradeHooks
}

var hardenWritableDBFile = func(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func OpenDBAtContext(ctx context.Context, path string) (*sql.DB, error) {
	return openDBAtContextWithHooks(ctx, path, openDBAtHooks{})
}

func openDBAtContextWithHooks(ctx context.Context, path string, hooks openDBAtHooks) (*sql.DB, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := hardenWritableDBFile(path); err != nil {
		return nil, fmt.Errorf("secure db: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if err := ensureWorkspaceScope(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade Workspace Context Scope: %w", err)
	}
	if err := ensureSemanticSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create Semantic Memory schema: %w", err)
	}
	if err := checkSemanticProjectionStartup(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("check Semantic Memory projection: %w", err)
	}
	if err := ensurePluginConfigurationRevision(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade plugin enabled configuration: %w", err)
	}
	if hooks.afterSchema != nil {
		hooks.afterSchema()
	}
	if err := ctx.Err(); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSessionTitlesWithHooks(ctx, db, hooks.sessionTitleUpgrade); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade session titles: %w", err)
	}
	if err := ctx.Err(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func ensureWorkspaceScope(ctx context.Context, db *sql.DB) error {
	columns := []struct {
		table, name, definition string
	}{
		{"sessions", "workspace_id", "TEXT REFERENCES workspaces(id)"},
		{"sessions", "workspace_revision_snapshot", "TEXT"},
		{"events", "workspace_id", "TEXT REFERENCES workspaces(id)"},
	}
	for _, column := range columns {
		present, err := tableHasColumn(ctx, db, column.table, column.name)
		if err != nil {
			return err
		}
		if present {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN %s %s", column.table, column.name, column.definition,
		)); err != nil {
			present, checkErr := tableHasColumn(ctx, db, column.table, column.name)
			if checkErr != nil || !present {
				return err
			}
		}
	}
	_, err := db.ExecContext(ctx, workspaceScopeSchema)
	return err
}

func tableHasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, fmt.Sprintf(
		"SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name = ?", table,
	), column).Scan(&count)
	return count == 1, err
}

func ensurePluginConfigurationRevision(ctx context.Context, db *sql.DB) error {
	hasRevision := func() (bool, error) {
		var count int
		err := db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pragma_table_info('plugin_enabled_configuration') WHERE name = 'revision'
		`).Scan(&count)
		return count == 1, err
	}
	present, err := hasRevision()
	if err != nil || present {
		return err
	}
	if _, err := db.ExecContext(ctx, `
		ALTER TABLE plugin_enabled_configuration
		ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
	`); err != nil {
		present, checkErr := hasRevision()
		if checkErr == nil && present {
			return nil
		}
		return err
	}
	return nil
}
