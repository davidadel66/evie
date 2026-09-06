package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
    scope       TEXT NOT NULL CHECK (
        typeof(scope) = 'text' AND (
            scope = 'global' OR scope GLOB 'workspace:*' OR scope GLOB 'project:*'
        )
    ),
    title       TEXT NOT NULL CHECK (typeof(title) = 'text' AND length(trim(title)) > 0),
    description TEXT CHECK (description IS NULL OR typeof(description) = 'text'),
    priority    INTEGER CHECK (priority IS NULL OR (typeof(priority) = 'integer' AND priority BETWEEN 1 AND 5)),
    due_date    TEXT CHECK (due_date IS NULL OR typeof(due_date) = 'text'),
    result_summary TEXT CHECK (result_summary IS NULL OR typeof(result_summary) = 'text'),
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

CREATE TABLE IF NOT EXISTS task_hierarchy (
    task_id       TEXT PRIMARY KEY NOT NULL REFERENCES tasks(id),
    parent_id     TEXT REFERENCES tasks(id),
    root_id       TEXT NOT NULL REFERENCES tasks(id),
    sibling_order INTEGER NOT NULL CHECK (typeof(sibling_order) = 'integer' AND sibling_order >= 0),
    CHECK (
        (parent_id IS NULL AND root_id = task_id AND sibling_order = 0) OR
        (parent_id IS NOT NULL AND root_id <> task_id AND sibling_order > 0)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS task_hierarchy_parent_order_idx
ON task_hierarchy(parent_id, sibling_order)
WHERE parent_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS task_hierarchy_root_idx
ON task_hierarchy(root_id, parent_id, sibling_order, task_id);

CREATE TRIGGER IF NOT EXISTS task_hierarchy_validate_child
BEFORE INSERT ON task_hierarchy
FOR EACH ROW
WHEN NEW.parent_id IS NOT NULL AND NOT EXISTS (
    SELECT 1
    FROM task_hierarchy parent_h
    JOIN tasks parent_t ON parent_t.id = parent_h.task_id
    JOIN tasks child_t ON child_t.id = NEW.task_id
    WHERE parent_h.task_id = NEW.parent_id
      AND parent_h.root_id = NEW.root_id
      AND parent_t.scope = child_t.scope
)
BEGIN
    SELECT RAISE(ABORT, 'task hierarchy parent, root, and scope must agree');
END;

CREATE TRIGGER IF NOT EXISTS task_hierarchy_append_only_update
BEFORE UPDATE ON task_hierarchy
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task hierarchy is append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_hierarchy_append_only_delete
BEFORE DELETE ON task_hierarchy
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task hierarchy is append-only');
END;

INSERT OR IGNORE INTO task_hierarchy (task_id, parent_id, root_id, sibling_order)
SELECT id, NULL, id, 0 FROM tasks;

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

CREATE TABLE IF NOT EXISTS task_revisions (
    task_id     TEXT NOT NULL REFERENCES tasks(id),
    revision    INTEGER NOT NULL CHECK (typeof(revision) = 'integer' AND revision > 0),
    scope       TEXT NOT NULL CHECK (scope = 'global' OR scope GLOB 'workspace:*' OR scope GLOB 'project:*'),
    title       TEXT NOT NULL CHECK (typeof(title) = 'text' AND length(trim(title)) > 0),
    description TEXT CHECK (description IS NULL OR typeof(description) = 'text'),
    priority    INTEGER CHECK (priority IS NULL OR (typeof(priority) = 'integer' AND priority BETWEEN 1 AND 5)),
    due_date    TEXT CHECK (due_date IS NULL OR typeof(due_date) = 'text'),
    result_summary TEXT CHECK (result_summary IS NULL OR typeof(result_summary) = 'text'),
    status      TEXT NOT NULL CHECK (status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
    created_at  TEXT NOT NULL CHECK (typeof(created_at) = 'text'),
    updated_at  TEXT NOT NULL CHECK (typeof(updated_at) = 'text'),
    PRIMARY KEY (task_id, revision)
);

CREATE TRIGGER IF NOT EXISTS task_revisions_append_only_update
BEFORE UPDATE ON task_revisions
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task revisions are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_revisions_append_only_delete
BEFORE DELETE ON task_revisions
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task revisions are append-only');
END;

INSERT OR IGNORE INTO task_revisions (
    task_id, revision, scope, title, description, priority, due_date,
    status, created_at, updated_at
)
SELECT id, revision, scope, title, description, priority, due_date,
       status, created_at, updated_at
FROM tasks;

CREATE TABLE IF NOT EXISTS task_mutation_results (
    actor_id            TEXT NOT NULL CHECK (typeof(actor_id) = 'text' AND length(trim(actor_id)) > 0),
    session_id          TEXT NOT NULL CHECK (typeof(session_id) = 'text' AND length(trim(session_id)) > 0),
    run_id              TEXT NOT NULL CHECK (typeof(run_id) = 'text' AND length(trim(run_id)) > 0),
    identity_sha256     TEXT NOT NULL CHECK (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*'),
    request_sha256      TEXT NOT NULL CHECK (length(request_sha256) = 64 AND request_sha256 NOT GLOB '*[^0-9a-f]*'),
    operation           TEXT NOT NULL CHECK (operation IN ('create', 'update')),
    task_id             TEXT,
    event_id            TEXT UNIQUE REFERENCES task_events(id),
    outcome_code        TEXT NOT NULL CHECK (outcome_code IN ('accepted', 'invalid_input', 'not_found', 'revision_conflict', 'invalid_transition')),
    diagnostic_field    TEXT,
    previous_revision   INTEGER CHECK (previous_revision IS NULL OR (typeof(previous_revision) = 'integer' AND previous_revision >= 0)),
    resulting_revision  INTEGER CHECK (resulting_revision IS NULL OR (typeof(resulting_revision) = 'integer' AND resulting_revision >= 0)),
    from_status         TEXT CHECK (from_status IS NULL OR from_status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
    to_status           TEXT CHECK (to_status IS NULL OR to_status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
    recorded_at         TEXT NOT NULL CHECK (typeof(recorded_at) = 'text'),
    PRIMARY KEY (actor_id, session_id, identity_sha256),
    CHECK ((outcome_code = 'accepted' AND task_id IS NOT NULL AND event_id IS NOT NULL AND resulting_revision > 0) OR outcome_code <> 'accepted')
);

CREATE TRIGGER IF NOT EXISTS task_mutation_results_append_only_update
BEFORE UPDATE ON task_mutation_results
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task mutation results are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_mutation_results_append_only_delete
BEFORE DELETE ON task_mutation_results
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task mutation results are append-only');
END;

CREATE TABLE IF NOT EXISTS task_idempotency_conflicts (
    id                      TEXT PRIMARY KEY NOT NULL CHECK (length(trim(id)) > 0),
    actor_id                TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id              TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    identity_sha256         TEXT NOT NULL CHECK (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*'),
    original_request_sha256 TEXT NOT NULL CHECK (length(original_request_sha256) = 64 AND original_request_sha256 NOT GLOB '*[^0-9a-f]*'),
    attempted_request_sha256 TEXT NOT NULL CHECK (length(attempted_request_sha256) = 64 AND attempted_request_sha256 NOT GLOB '*[^0-9a-f]*'),
    operation               TEXT NOT NULL CHECK (operation IN ('create', 'update')),
    task_id                 TEXT,
    recorded_at             TEXT NOT NULL CHECK (typeof(recorded_at) = 'text')
);

CREATE TRIGGER IF NOT EXISTS task_idempotency_conflicts_append_only_update
BEFORE UPDATE ON task_idempotency_conflicts
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task idempotency conflicts are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_idempotency_conflicts_append_only_delete
BEFORE DELETE ON task_idempotency_conflicts
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task idempotency conflicts are append-only');
END;

CREATE TABLE IF NOT EXISTS task_decomposition_conflicts (
    id                       TEXT PRIMARY KEY NOT NULL CHECK (length(trim(id)) > 0),
    actor_id                 TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id               TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    identity_sha256          TEXT NOT NULL CHECK (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*'),
    original_request_sha256  TEXT NOT NULL CHECK (length(original_request_sha256) = 64 AND original_request_sha256 NOT GLOB '*[^0-9a-f]*'),
    attempted_request_sha256 TEXT NOT NULL CHECK (length(attempted_request_sha256) = 64 AND attempted_request_sha256 NOT GLOB '*[^0-9a-f]*'),
    parent_id                TEXT,
    recorded_at              TEXT NOT NULL CHECK (typeof(recorded_at) = 'text')
);

CREATE TRIGGER IF NOT EXISTS task_decomposition_conflicts_append_only_update
BEFORE UPDATE ON task_decomposition_conflicts
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task decomposition conflicts are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_decomposition_conflicts_append_only_delete
BEFORE DELETE ON task_decomposition_conflicts
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task decomposition conflicts are append-only');
END;

CREATE TABLE IF NOT EXISTS task_idempotency_claims (
    actor_id        TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id      TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    identity_sha256 TEXT NOT NULL CHECK (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*'),
    request_sha256  TEXT NOT NULL CHECK (length(request_sha256) = 64 AND request_sha256 NOT GLOB '*[^0-9a-f]*'),
    operation       TEXT NOT NULL CHECK (operation IN ('create', 'update', 'decompose')),
    recorded_at     TEXT NOT NULL CHECK (typeof(recorded_at) = 'text'),
    PRIMARY KEY (actor_id, session_id, identity_sha256)
);

CREATE TRIGGER IF NOT EXISTS task_idempotency_claims_append_only_update
BEFORE UPDATE ON task_idempotency_claims
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task idempotency claims are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_idempotency_claims_append_only_delete
BEFORE DELETE ON task_idempotency_claims
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task idempotency claims are append-only');
END;

INSERT OR IGNORE INTO task_idempotency_claims (
    actor_id, session_id, identity_sha256, request_sha256, operation, recorded_at
)
SELECT actor_id, session_id, identity_sha256, request_sha256, operation, recorded_at
FROM task_mutation_results;

CREATE TABLE IF NOT EXISTS task_hierarchy_events (
    id                 TEXT PRIMARY KEY NOT NULL CHECK (length(trim(id)) > 0),
    task_id            TEXT NOT NULL REFERENCES tasks(id),
    sequence           INTEGER NOT NULL CHECK (typeof(sequence) = 'integer' AND sequence > 0),
    actor_id           TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id         TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    run_id             TEXT NOT NULL CHECK (length(trim(run_id)) > 0),
    recorded_at        TEXT NOT NULL CHECK (typeof(recorded_at) = 'text'),
    previous_revision  INTEGER NOT NULL CHECK (typeof(previous_revision) = 'integer' AND previous_revision > 0),
    resulting_revision INTEGER NOT NULL CHECK (typeof(resulting_revision) = 'integer' AND resulting_revision >= previous_revision),
    operation           TEXT NOT NULL CHECK (operation IN ('create', 'decompose')),
    outcome            TEXT NOT NULL CHECK (outcome IN ('accepted', 'rejected')),
    diagnostic_code    TEXT CHECK (diagnostic_code IS NULL OR diagnostic_code IN ('invalid_input', 'invalid_transition', 'revision_conflict')),
    UNIQUE (task_id, sequence),
    CHECK (
        (outcome = 'accepted' AND diagnostic_code IS NULL AND resulting_revision = previous_revision + 1) OR
        (outcome = 'rejected' AND diagnostic_code IS NOT NULL AND resulting_revision = previous_revision)
    )
);

CREATE TRIGGER IF NOT EXISTS task_hierarchy_events_append_only_update
BEFORE UPDATE ON task_hierarchy_events
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task hierarchy events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_hierarchy_events_append_only_delete
BEFORE DELETE ON task_hierarchy_events
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task hierarchy events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_hierarchy_events_unique_sequence
BEFORE INSERT ON task_hierarchy_events
FOR EACH ROW
WHEN EXISTS (
    SELECT 1 FROM task_events
    WHERE task_id = NEW.task_id AND sequence = NEW.sequence
)
BEGIN
    SELECT RAISE(ABORT, 'Task event sequence already exists');
END;

CREATE TRIGGER IF NOT EXISTS task_events_unique_hierarchy_sequence
BEFORE INSERT ON task_events
FOR EACH ROW
WHEN EXISTS (
    SELECT 1 FROM task_hierarchy_events
    WHERE task_id = NEW.task_id AND sequence = NEW.sequence
)
BEGIN
    SELECT RAISE(ABORT, 'Task event sequence already exists');
END;

CREATE TABLE IF NOT EXISTS task_event_idempotency (
    event_id        TEXT PRIMARY KEY NOT NULL,
    identity_sha256 TEXT NOT NULL CHECK (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*')
);

CREATE TRIGGER IF NOT EXISTS task_event_idempotency_requires_event
BEFORE INSERT ON task_event_idempotency
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM task_events WHERE id = NEW.event_id)
 AND NOT EXISTS (SELECT 1 FROM task_hierarchy_events WHERE id = NEW.event_id)
BEGIN
    SELECT RAISE(ABORT, 'Task event idempotency requires an event');
END;

CREATE TRIGGER IF NOT EXISTS task_event_idempotency_append_only_update
BEFORE UPDATE ON task_event_idempotency
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task event idempotency is append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_event_idempotency_append_only_delete
BEFORE DELETE ON task_event_idempotency
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task event idempotency is append-only');
END;

INSERT OR IGNORE INTO task_event_idempotency (event_id, identity_sha256)
SELECT event_id, identity_sha256 FROM task_mutation_results WHERE event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS task_decomposition_results (
    actor_id            TEXT NOT NULL,
    session_id          TEXT NOT NULL,
    identity_sha256     TEXT NOT NULL,
    parent_id           TEXT NOT NULL,
    event_id            TEXT UNIQUE REFERENCES task_hierarchy_events(id),
    outcome_code        TEXT NOT NULL CHECK (outcome_code IN ('accepted', 'invalid_input', 'not_found', 'revision_conflict', 'invalid_transition')),
    diagnostic_field    TEXT,
    previous_revision   INTEGER CHECK (previous_revision IS NULL OR previous_revision > 0),
    resulting_revision  INTEGER CHECK (resulting_revision IS NULL OR resulting_revision >= previous_revision),
    from_status         TEXT CHECK (from_status IS NULL OR from_status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
    to_status           TEXT CHECK (to_status IS NULL OR to_status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
    PRIMARY KEY (actor_id, session_id, identity_sha256),
    FOREIGN KEY (actor_id, session_id, identity_sha256)
        REFERENCES task_idempotency_claims(actor_id, session_id, identity_sha256),
    CHECK (
        (outcome_code = 'accepted' AND event_id IS NOT NULL AND resulting_revision = previous_revision + 1) OR
        outcome_code <> 'accepted'
    )
);

CREATE TRIGGER IF NOT EXISTS task_decomposition_results_append_only_update
BEFORE UPDATE ON task_decomposition_results
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task decomposition results are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_decomposition_results_append_only_delete
BEFORE DELETE ON task_decomposition_results
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task decomposition results are append-only');
END;

CREATE TABLE IF NOT EXISTS task_decomposition_children (
    actor_id        TEXT NOT NULL,
    session_id      TEXT NOT NULL,
    identity_sha256 TEXT NOT NULL,
    child_order     INTEGER NOT NULL CHECK (child_order >= 0),
    child_task_id   TEXT NOT NULL,
    child_revision  INTEGER NOT NULL CHECK (child_revision > 0),
    PRIMARY KEY (actor_id, session_id, identity_sha256, child_order),
    FOREIGN KEY (actor_id, session_id, identity_sha256)
        REFERENCES task_decomposition_results(actor_id, session_id, identity_sha256),
    FOREIGN KEY (child_task_id, child_revision)
        REFERENCES task_revisions(task_id, revision)
);

CREATE TRIGGER IF NOT EXISTS task_decomposition_children_append_only_update
BEFORE UPDATE ON task_decomposition_children
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task decomposition children are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_decomposition_children_append_only_delete
BEFORE DELETE ON task_decomposition_children
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'task decomposition children are append-only');
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

CREATE TABLE IF NOT EXISTS session_task_focus (
    session_id  TEXT PRIMARY KEY NOT NULL REFERENCES sessions(id),
    task_id     TEXT NOT NULL REFERENCES tasks(id),
    selected_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_access_grants (
    id                    TEXT PRIMARY KEY NOT NULL CHECK (length(trim(id)) > 0),
    grantee_session_id    TEXT NOT NULL REFERENCES sessions(id),
    root_task_id          TEXT NOT NULL REFERENCES tasks(id),
    access_level          TEXT NOT NULL CHECK (access_level IN ('read', 'contribute', 'manage')),
    issuer_actor_id       TEXT NOT NULL CHECK (length(trim(issuer_actor_id)) > 0),
    issuer_session_id     TEXT NOT NULL REFERENCES sessions(id),
    issuer_run_id         TEXT NOT NULL CHECK (length(trim(issuer_run_id)) > 0),
    issued_at             TEXT NOT NULL,
    ended_at              TEXT,
    end_reason            TEXT,
	ended_by_actor_id      TEXT,
	ended_by_session_id    TEXT,
	ended_by_run_id        TEXT,
	CHECK ((ended_at IS NULL AND end_reason IS NULL AND ended_by_actor_id IS NULL AND
	        ended_by_session_id IS NULL AND ended_by_run_id IS NULL) OR
	       (ended_at IS NOT NULL AND length(trim(end_reason)) > 0 AND
	        length(trim(ended_by_actor_id)) > 0 AND length(trim(ended_by_session_id)) > 0 AND
	        length(trim(ended_by_run_id)) > 0))
);

CREATE UNIQUE INDEX IF NOT EXISTS task_access_grants_one_active_execution
ON task_access_grants(grantee_session_id) WHERE ended_at IS NULL;

CREATE INDEX IF NOT EXISTS task_access_grants_root_idx
ON task_access_grants(root_task_id, grantee_session_id);

CREATE TRIGGER IF NOT EXISTS task_access_grants_immutable_identity
BEFORE UPDATE OF id, grantee_session_id, root_task_id, access_level,
                 issuer_actor_id, issuer_session_id, issuer_run_id, issued_at
ON task_access_grants
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'Task Access Grant identity is immutable');
END;

CREATE TRIGGER IF NOT EXISTS task_access_grants_no_hard_delete
BEFORE DELETE ON task_access_grants
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'Task Access Grants are retained');
END;

CREATE TRIGGER IF NOT EXISTS task_access_grants_end_once
BEFORE UPDATE OF ended_at, end_reason, ended_by_actor_id, ended_by_session_id, ended_by_run_id ON task_access_grants
FOR EACH ROW
WHEN OLD.ended_at IS NOT NULL
  OR NEW.ended_at IS NULL
  OR NEW.end_reason IS NULL
  OR length(trim(NEW.end_reason)) = 0
	OR NEW.ended_by_actor_id IS NULL OR length(trim(NEW.ended_by_actor_id)) = 0
	OR NEW.ended_by_session_id IS NULL OR length(trim(NEW.ended_by_session_id)) = 0
	OR NEW.ended_by_run_id IS NULL OR length(trim(NEW.ended_by_run_id)) = 0
BEGIN
    SELECT RAISE(ABORT, 'Task Access Grant termination is append-only');
END;

CREATE TRIGGER IF NOT EXISTS sessions_end_task_access_grants
AFTER UPDATE OF status ON sessions
FOR EACH ROW
WHEN OLD.status = 'active' AND NEW.status = 'closed'
BEGIN
    UPDATE task_access_grants
	SET ended_at = CASE WHEN NEW.updated_at > issued_at THEN NEW.updated_at ELSE issued_at END,
	    end_reason = 'session_closed',
	    ended_by_actor_id = 'kernel', ended_by_session_id = NEW.id, ended_by_run_id = 'session-close'
    WHERE (grantee_session_id = NEW.id OR issuer_session_id = NEW.id) AND ended_at IS NULL;
    DELETE FROM session_task_focus
    WHERE session_id = NEW.id
       OR session_id IN (SELECT grantee_session_id FROM task_access_grants WHERE issuer_session_id = NEW.id);
END;

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

CREATE TABLE IF NOT EXISTS task_claims (
    task_id          TEXT PRIMARY KEY NOT NULL REFERENCES tasks(id),
    claim_id         TEXT NOT NULL UNIQUE CHECK (length(trim(claim_id)) > 0),
    actor_id         TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id       TEXT NOT NULL REFERENCES sessions(id),
    lease_holder_id  TEXT NOT NULL CHECK (length(trim(lease_holder_id)) > 0),
    lease_token      INTEGER NOT NULL CHECK (lease_token > 0),
    lease_generation INTEGER NOT NULL CHECK (lease_generation > 0),
    acquired_run_id  TEXT NOT NULL CHECK (length(trim(acquired_run_id)) > 0),
    claimed_at       TEXT NOT NULL CHECK (typeof(claimed_at) = 'text'),
    CHECK (lease_token = lease_generation)
);

CREATE INDEX IF NOT EXISTS task_claims_execution_idx
ON task_claims(session_id, lease_holder_id, lease_token, lease_generation);

CREATE TRIGGER IF NOT EXISTS task_claims_validate_execution
BEFORE INSERT ON task_claims
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1
    FROM sessions s
    JOIN session_turn_leases l ON l.session_id = s.id
    WHERE s.id = NEW.session_id
      AND s.status = 'active'
      AND l.holder_id = NEW.lease_holder_id
      AND l.fencing_token = NEW.lease_token
      AND l.lease_generation = NEW.lease_generation
      AND l.expires_at > NEW.claimed_at
)
BEGIN
    SELECT RAISE(ABORT, 'Task claim execution is not active');
END;

CREATE TABLE IF NOT EXISTS task_claim_events (
    id                  TEXT PRIMARY KEY NOT NULL CHECK (length(trim(id)) > 0),
    task_id             TEXT NOT NULL REFERENCES tasks(id),
    sequence            INTEGER NOT NULL CHECK (sequence > 0),
    operation           TEXT NOT NULL CHECK (operation IN ('update', 'claim', 'release')),
    actor_id            TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id          TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    run_id              TEXT NOT NULL CHECK (length(trim(run_id)) > 0),
    recorded_at         TEXT NOT NULL CHECK (typeof(recorded_at) = 'text'),
    previous_revision   INTEGER NOT NULL CHECK (previous_revision > 0),
    resulting_revision  INTEGER NOT NULL CHECK (resulting_revision = previous_revision),
    outcome             TEXT NOT NULL CHECK (outcome IN ('accepted', 'rejected')),
    diagnostic_code     TEXT CHECK (diagnostic_code IS NULL OR diagnostic_code IN (
        'invalid_input', 'invalid_transition', 'claim_held', 'claim_required',
        'claim_not_owned', 'execution_inactive'
    )),
    identity_sha256     TEXT CHECK (
        identity_sha256 IS NULL OR
        (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*')
    ),
    claim_id            TEXT,
    claim_reason        TEXT,
	management_override INTEGER NOT NULL DEFAULT 0 CHECK (management_override IN (0, 1)),
	management_reason   TEXT,
    UNIQUE (task_id, sequence),
    CHECK (
        (outcome = 'accepted' AND diagnostic_code IS NULL) OR
        (outcome = 'rejected' AND diagnostic_code IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS task_claim_events_task_sequence_idx
ON task_claim_events(task_id, sequence);

CREATE TRIGGER IF NOT EXISTS task_claim_events_append_only_update
BEFORE UPDATE ON task_claim_events
BEGIN
    SELECT RAISE(ABORT, 'task claim events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_claim_events_append_only_delete
BEFORE DELETE ON task_claim_events
BEGIN
    SELECT RAISE(ABORT, 'task claim events are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_claim_events_unique_sequence
BEFORE INSERT ON task_claim_events
FOR EACH ROW
WHEN EXISTS (SELECT 1 FROM task_events WHERE task_id = NEW.task_id AND sequence = NEW.sequence)
  OR EXISTS (SELECT 1 FROM task_hierarchy_events WHERE task_id = NEW.task_id AND sequence = NEW.sequence)
BEGIN
    SELECT RAISE(ABORT, 'Task event sequence already exists');
END;

CREATE TRIGGER IF NOT EXISTS task_events_unique_claim_sequence
BEFORE INSERT ON task_events
FOR EACH ROW
WHEN EXISTS (SELECT 1 FROM task_claim_events WHERE task_id = NEW.task_id AND sequence = NEW.sequence)
BEGIN
    SELECT RAISE(ABORT, 'Task event sequence already exists');
END;

CREATE TRIGGER IF NOT EXISTS task_hierarchy_events_unique_claim_sequence
BEFORE INSERT ON task_hierarchy_events
FOR EACH ROW
WHEN EXISTS (SELECT 1 FROM task_claim_events WHERE task_id = NEW.task_id AND sequence = NEW.sequence)
BEGIN
    SELECT RAISE(ABORT, 'Task event sequence already exists');
END;

CREATE TABLE IF NOT EXISTS task_event_grants (
    event_id TEXT PRIMARY KEY NOT NULL,
    grant_id TEXT NOT NULL REFERENCES task_access_grants(id)
);

CREATE TRIGGER IF NOT EXISTS task_event_grants_requires_event
BEFORE INSERT ON task_event_grants
FOR EACH ROW
WHEN NOT EXISTS (SELECT 1 FROM task_events WHERE id = NEW.event_id)
 AND NOT EXISTS (SELECT 1 FROM task_hierarchy_events WHERE id = NEW.event_id)
 AND NOT EXISTS (SELECT 1 FROM task_claim_events WHERE id = NEW.event_id)
BEGIN
    SELECT RAISE(ABORT, 'Task event grant link requires an event');
END;

CREATE TRIGGER IF NOT EXISTS task_event_grants_append_only_update
BEFORE UPDATE ON task_event_grants
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'Task event grant links are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_event_grants_append_only_delete
BEFORE DELETE ON task_event_grants
FOR EACH ROW
BEGIN
    SELECT RAISE(ABORT, 'Task event grant links are append-only');
END;

CREATE TABLE IF NOT EXISTS task_coordination_results (
    actor_id         TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id       TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    run_id           TEXT NOT NULL CHECK (length(trim(run_id)) > 0),
    identity_sha256  TEXT NOT NULL CHECK (length(identity_sha256) = 64 AND identity_sha256 NOT GLOB '*[^0-9a-f]*'),
    request_sha256   TEXT NOT NULL CHECK (length(request_sha256) = 64 AND request_sha256 NOT GLOB '*[^0-9a-f]*'),
    operation        TEXT NOT NULL CHECK (operation IN ('claim', 'release')),
    task_id          TEXT NOT NULL,
    event_id         TEXT UNIQUE REFERENCES task_claim_events(id),
    outcome_code     TEXT NOT NULL CHECK (outcome_code IN (
        'accepted', 'not_found', 'invalid_transition', 'claim_held',
        'claim_required', 'claim_not_owned', 'execution_inactive'
    )),
    claim_id         TEXT,
    claimed_at       TEXT,
    released_at      TEXT,
	release_reason   TEXT,
	from_status      TEXT CHECK (from_status IS NULL OR from_status IN ('open', 'in_progress', 'blocked', 'completed', 'cancelled')),
	recorded_at      TEXT NOT NULL,
    PRIMARY KEY (actor_id, session_id, identity_sha256)
);

CREATE TRIGGER IF NOT EXISTS task_coordination_results_append_only_update
BEFORE UPDATE ON task_coordination_results
BEGIN
    SELECT RAISE(ABORT, 'task coordination results are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_coordination_results_append_only_delete
BEFORE DELETE ON task_coordination_results
BEGIN
    SELECT RAISE(ABORT, 'task coordination results are append-only');
END;

CREATE TABLE IF NOT EXISTS task_coordination_conflicts (
    id                       TEXT PRIMARY KEY NOT NULL CHECK (length(trim(id)) > 0),
    actor_id                 TEXT NOT NULL CHECK (length(trim(actor_id)) > 0),
    session_id               TEXT NOT NULL CHECK (length(trim(session_id)) > 0),
    identity_sha256          TEXT NOT NULL CHECK (length(identity_sha256) = 64),
    original_request_sha256  TEXT NOT NULL CHECK (length(original_request_sha256) = 64),
    attempted_request_sha256 TEXT NOT NULL CHECK (length(attempted_request_sha256) = 64),
    operation                TEXT NOT NULL CHECK (operation IN ('create', 'update', 'decompose', 'claim', 'release')),
    task_id                  TEXT,
    recorded_at              TEXT NOT NULL
);

CREATE TRIGGER IF NOT EXISTS task_coordination_conflicts_append_only_update
BEFORE UPDATE ON task_coordination_conflicts
BEGIN
    SELECT RAISE(ABORT, 'task coordination conflicts are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_coordination_conflicts_append_only_delete
BEFORE DELETE ON task_coordination_conflicts
BEGIN
    SELECT RAISE(ABORT, 'task coordination conflicts are append-only');
END;

CREATE TABLE IF NOT EXISTS task_legacy_todo_imports (
    migration_id  TEXT PRIMARY KEY NOT NULL CHECK (length(trim(migration_id)) > 0),
    source_list   TEXT NOT NULL CHECK (length(trim(source_list)) > 0),
    source_sha256 TEXT NOT NULL CHECK (length(source_sha256) = 64 AND source_sha256 NOT GLOB '*[^0-9a-f]*'),
    item_count    INTEGER NOT NULL CHECK (typeof(item_count) = 'integer' AND item_count >= 0),
    applied_at    TEXT NOT NULL CHECK (typeof(applied_at) = 'text')
);

CREATE TRIGGER IF NOT EXISTS task_legacy_todo_imports_append_only_update
BEFORE UPDATE ON task_legacy_todo_imports
BEGIN
    SELECT RAISE(ABORT, 'legacy Todo imports are append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_legacy_todo_imports_append_only_delete
BEFORE DELETE ON task_legacy_todo_imports
BEGIN
    SELECT RAISE(ABORT, 'legacy Todo imports are append-only');
END;

CREATE TABLE IF NOT EXISTS task_legacy_todo_provenance (
    migration_id TEXT NOT NULL REFERENCES task_legacy_todo_imports(migration_id),
    source_list  TEXT NOT NULL CHECK (length(trim(source_list)) > 0),
    legacy_id    INTEGER NOT NULL CHECK (typeof(legacy_id) = 'integer' AND legacy_id >= 0),
    source_index INTEGER NOT NULL CHECK (typeof(source_index) = 'integer' AND source_index >= 0),
    task_id      TEXT NOT NULL UNIQUE REFERENCES tasks(id),
    PRIMARY KEY (migration_id, source_list, legacy_id),
    UNIQUE (migration_id, source_index)
);

CREATE TRIGGER IF NOT EXISTS task_legacy_todo_provenance_append_only_update
BEFORE UPDATE ON task_legacy_todo_provenance
BEGIN
    SELECT RAISE(ABORT, 'legacy Todo provenance is append-only');
END;

CREATE TRIGGER IF NOT EXISTS task_legacy_todo_provenance_append_only_delete
BEFORE DELETE ON task_legacy_todo_provenance
BEGIN
    SELECT RAISE(ABORT, 'legacy Todo provenance is append-only');
END;

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

DROP TRIGGER IF EXISTS session_task_focus_authorized_insert;
DROP TRIGGER IF EXISTS session_task_focus_authorized_update;

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

CREATE TRIGGER IF NOT EXISTS tasks_scope_identity_insert
BEFORE INSERT ON tasks
FOR EACH ROW
WHEN (NEW.scope GLOB 'workspace:*' AND NOT EXISTS (
        SELECT 1 FROM workspaces WHERE id = substr(NEW.scope, length('workspace:') + 1)
    ))
  OR (NEW.scope GLOB 'project:*' AND NOT EXISTS (
        SELECT 1 FROM projects WHERE id = substr(NEW.scope, length('project:') + 1)
    ))
BEGIN
    SELECT RAISE(ABORT, 'Task Context Scope identity is missing');
END;

CREATE TRIGGER IF NOT EXISTS task_hierarchy_validate_child
BEFORE INSERT ON task_hierarchy
FOR EACH ROW
WHEN NEW.parent_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM task_hierarchy parent_h
    JOIN tasks parent_t ON parent_t.id = parent_h.task_id
    JOIN tasks child_t ON child_t.id = NEW.task_id
    WHERE parent_h.task_id = NEW.parent_id
      AND parent_h.root_id = NEW.root_id
      AND parent_t.scope = child_t.scope
)
BEGIN
    SELECT RAISE(ABORT, 'task hierarchy parent, root, and scope must agree');
END;

CREATE TRIGGER IF NOT EXISTS tasks_scope_immutable
BEFORE UPDATE OF scope ON tasks
FOR EACH ROW
WHEN NEW.scope <> OLD.scope
BEGIN
    SELECT RAISE(ABORT, 'Task Context Scope is immutable');
END;

CREATE TRIGGER IF NOT EXISTS session_task_focus_authorized_insert
BEFORE INSERT ON session_task_focus
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1
    FROM sessions s JOIN tasks t ON t.id = NEW.task_id
    WHERE s.id = NEW.session_id AND s.status = 'active'
      AND (t.scope = 'global'
        OR t.scope = 'workspace:' || s.workspace_id
        OR t.scope = 'project:' || s.project_id)
      AND (s.parent_session_id IS NULL OR EXISTS (
        SELECT 1 FROM task_access_grants g
        WHERE g.grantee_session_id = s.id AND g.ended_at IS NULL
          AND NEW.task_id IN (
            WITH RECURSIVE granted(task_id) AS (
              SELECT g.root_task_id
              UNION ALL
              SELECT child.task_id FROM task_hierarchy child JOIN granted ON child.parent_id = granted.task_id
            )
            SELECT task_id FROM granted
          )
      ))
)
BEGIN
    SELECT RAISE(ABORT, 'Task Focus is outside the session Context Scope');
END;

CREATE TRIGGER IF NOT EXISTS session_task_focus_authorized_update
BEFORE UPDATE OF task_id ON session_task_focus
FOR EACH ROW
WHEN NOT EXISTS (
    SELECT 1
    FROM sessions s JOIN tasks t ON t.id = NEW.task_id
    WHERE s.id = NEW.session_id AND s.status = 'active'
      AND (t.scope = 'global'
        OR t.scope = 'workspace:' || s.workspace_id
        OR t.scope = 'project:' || s.project_id)
      AND (s.parent_session_id IS NULL OR EXISTS (
        SELECT 1 FROM task_access_grants g
        WHERE g.grantee_session_id = s.id AND g.ended_at IS NULL
          AND NEW.task_id IN (
            WITH RECURSIVE granted(task_id) AS (
              SELECT g.root_task_id
              UNION ALL
              SELECT child.task_id FROM task_hierarchy child JOIN granted ON child.parent_id = granted.task_id
            )
            SELECT task_id FROM granted
          )
      ))
)
BEGIN
    SELECT RAISE(ABORT, 'Task Focus is outside the session Context Scope');
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
	if err := ensureCompilerSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create memory compiler schema: %w", err)
	}
	if err := ensureCompilerActivationSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create memory compiler activation schema: %w", err)
	}
	if err := ensureCompilerHistorySchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create memory compiler history schema: %w", err)
	}
	if err := ensureCandidateReviewSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create candidate review schema: %w", err)
	}
	if err := ensureCandidateReviewNavigationSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create candidate review navigation schema: %w", err)
	}
	if err := checkSemanticProjectionStartup(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("check Semantic Memory projection: %w", err)
	}
	if err := ensurePluginConfigurationRevision(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade plugin enabled configuration: %w", err)
	}
	if err := ensureTaskResultSummary(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade Task result summary: %w", err)
	}
	if err := ensureTaskContextScopes(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("upgrade Task Context Scopes: %w", err)
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

func ensureTaskContextScopes(ctx context.Context, db *sql.DB) error {
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'tasks'`).Scan(&definition); err != nil {
		return err
	}
	legacyGlobalOnly := strings.Contains(definition, "scope = 'global'") || strings.Contains(definition, "scope='global'")
	if !legacyGlobalOnly || strings.Contains(definition, "workspace:*") {
		return nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	upgrade := `
		DROP TRIGGER IF EXISTS tasks_no_hard_delete;
		DROP TRIGGER IF EXISTS task_hierarchy_validate_child;
		DROP TRIGGER IF EXISTS session_task_focus_authorized_insert;
		DROP TRIGGER IF EXISTS session_task_focus_authorized_update;
		DROP TRIGGER IF EXISTS task_revisions_append_only_update;
		DROP TRIGGER IF EXISTS task_revisions_append_only_delete;
		CREATE TABLE tasks_scope_upgrade (
			id TEXT PRIMARY KEY NOT NULL CHECK (typeof(id) = 'text' AND length(trim(id)) > 0),
			scope TEXT NOT NULL CHECK (typeof(scope) = 'text' AND (scope = 'global' OR scope GLOB 'workspace:*' OR scope GLOB 'project:*')),
			title TEXT NOT NULL CHECK (typeof(title) = 'text' AND length(trim(title)) > 0),
			description TEXT CHECK (description IS NULL OR typeof(description) = 'text'),
			priority INTEGER CHECK (priority IS NULL OR (typeof(priority) = 'integer' AND priority BETWEEN 1 AND 5)),
			due_date TEXT CHECK (due_date IS NULL OR typeof(due_date) = 'text'),
			result_summary TEXT CHECK (result_summary IS NULL OR typeof(result_summary) = 'text'),
			status TEXT NOT NULL CHECK (typeof(status) = 'text' AND status IN ('open','in_progress','blocked','completed','cancelled')),
			revision INTEGER NOT NULL CHECK (typeof(revision) = 'integer' AND revision > 0),
			created_at TEXT NOT NULL CHECK (typeof(created_at) = 'text'), updated_at TEXT NOT NULL CHECK (typeof(updated_at) = 'text')
		);
		INSERT INTO tasks_scope_upgrade SELECT id, scope, title, description, priority, due_date, result_summary, status, revision, created_at, updated_at FROM tasks;
		DROP TABLE tasks;
		ALTER TABLE tasks_scope_upgrade RENAME TO tasks;
		CREATE INDEX tasks_scope_status_created_idx ON tasks(scope, status, created_at, id);
		CREATE TRIGGER tasks_no_hard_delete BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'tasks cannot be hard deleted'); END;
		CREATE TABLE task_revisions_scope_upgrade (
			task_id TEXT NOT NULL REFERENCES tasks(id), revision INTEGER NOT NULL CHECK (typeof(revision) = 'integer' AND revision > 0),
			scope TEXT NOT NULL CHECK (scope = 'global' OR scope GLOB 'workspace:*' OR scope GLOB 'project:*'),
			title TEXT NOT NULL CHECK (typeof(title) = 'text' AND length(trim(title)) > 0),
			description TEXT CHECK (description IS NULL OR typeof(description) = 'text'),
			priority INTEGER CHECK (priority IS NULL OR (typeof(priority) = 'integer' AND priority BETWEEN 1 AND 5)),
			due_date TEXT CHECK (due_date IS NULL OR typeof(due_date) = 'text'),
			result_summary TEXT CHECK (result_summary IS NULL OR typeof(result_summary) = 'text'),
			status TEXT NOT NULL CHECK (status IN ('open','in_progress','blocked','completed','cancelled')),
			created_at TEXT NOT NULL CHECK (typeof(created_at) = 'text'), updated_at TEXT NOT NULL CHECK (typeof(updated_at) = 'text'), PRIMARY KEY(task_id, revision)
		);
		INSERT INTO task_revisions_scope_upgrade SELECT task_id, revision, scope, title, description, priority, due_date, result_summary, status, created_at, updated_at FROM task_revisions;
		DROP TABLE task_revisions;
		ALTER TABLE task_revisions_scope_upgrade RENAME TO task_revisions;
		CREATE TRIGGER task_revisions_append_only_update BEFORE UPDATE ON task_revisions BEGIN SELECT RAISE(ABORT, 'task revisions are append-only'); END;
		CREATE TRIGGER task_revisions_append_only_delete BEFORE DELETE ON task_revisions BEGIN SELECT RAISE(ABORT, 'task revisions are append-only'); END;
	`
	if _, err := conn.ExecContext(ctx, upgrade); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		return err
	}
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	if rows.Next() {
		_ = rows.Close()
		return errors.New("Task Context Scope upgrade violates foreign keys")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, workspaceScopeSchema)
	return err
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

func ensureTaskResultSummary(ctx context.Context, db *sql.DB) error {
	return ensureTaskResultSummaryWithHooks(ctx, db, taskResultSummaryUpgradeHooks{})
}

type taskResultSummaryUpgradeHooks struct {
	afterFastMissingCheck func()
}

func ensureTaskResultSummaryWithHooks(
	ctx context.Context,
	db *sql.DB,
	hooks taskResultSummaryUpgradeHooks,
) error {
	missing := false
	for _, table := range []string{"tasks", "task_revisions"} {
		present, err := tableHasColumn(ctx, db, table, "result_summary")
		if err != nil {
			return err
		}
		missing = missing || !present
	}
	if !missing {
		return nil
	}
	if hooks.afterFastMissingCheck != nil {
		hooks.afterFastMissingCheck()
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		for _, table := range []string{"tasks", "task_revisions"} {
			present, err := tableHasColumn(ctx, conn, table, "result_summary")
			if err != nil {
				return err
			}
			if present {
				continue
			}
			if _, err := conn.ExecContext(ctx, `ALTER TABLE `+table+`
				ADD COLUMN result_summary TEXT CHECK (result_summary IS NULL OR typeof(result_summary) = 'text')`); err != nil {
				return fmt.Errorf("add %s.result_summary: %w", table, err)
			}
		}
		return nil
	})
}

func tableHasColumn(ctx context.Context, queryer rowsQueryer, table, column string) (bool, error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("scan %s schema: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read %s schema: %w", table, err)
	}
	return false, nil
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
