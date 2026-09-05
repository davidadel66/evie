package eviedb

import (
	"context"
	"database/sql"
)

func ensureCompilerActivationSchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransactionResolver(ctx, db, executeImmediateTransactionStatement, transactionResolutionContext, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, compilerActivationSchema)
		return err
	})
}

// The position trigger is downstream of the position INSERT in the original
// append trigger. It does not depend on ordering between sibling event triggers.
const compilerActivationSchema = `
CREATE TABLE IF NOT EXISTS memory_compiler_activations (
 activation_id TEXT PRIMARY KEY, selector_key TEXT NOT NULL, source_scope TEXT NOT NULL,
 source_session TEXT NOT NULL, destination TEXT NOT NULL, generation_id TEXT NOT NULL REFERENCES memory_compiler_generations,
 revision INTEGER NOT NULL, after_position INTEGER NOT NULL, through_position INTEGER,
 work_paused INTEGER NOT NULL DEFAULT 0 CHECK(work_paused IN (0,1)), work_epoch INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS memory_compiler_activation_open ON memory_compiler_activations(selector_key) WHERE through_position IS NULL;
CREATE TABLE IF NOT EXISTS memory_compiler_activation_requests (
 request_id TEXT PRIMARY KEY, request_hash TEXT NOT NULL, response BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_compiler_activation_dirty (
 activation_id TEXT NOT NULL REFERENCES memory_compiler_activations, session_id TEXT NOT NULL REFERENCES sessions,
 high_position INTEGER NOT NULL, scanned_position INTEGER NOT NULL DEFAULT 0, scan_order INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(activation_id,session_id)
);
CREATE TABLE IF NOT EXISTS memory_compiler_activation_roots (
 activation_id TEXT NOT NULL REFERENCES memory_compiler_activations, session_id TEXT NOT NULL REFERENCES sessions,
 root_id TEXT NOT NULL REFERENCES events, first_sequence INTEGER NOT NULL, last_sequence INTEGER NOT NULL,
 position INTEGER NOT NULL, state TEXT NOT NULL DEFAULT 'selected_unmaterialized', reason TEXT NOT NULL DEFAULT '',
 selection_id TEXT NOT NULL DEFAULT '', checked_order INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(activation_id,root_id)
);
CREATE TABLE IF NOT EXISTS memory_compiler_activation_jobs (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs, activation_id TEXT NOT NULL REFERENCES memory_compiler_activations
);
CREATE VIEW IF NOT EXISTS memory_compiler_activation_paused_jobs AS
 SELECT l.job_id FROM memory_compiler_activation_jobs l JOIN memory_compiler_activations a USING(activation_id) WHERE a.work_paused=1;
CREATE TABLE IF NOT EXISTS memory_compiler_activation_claims (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs, fence INTEGER NOT NULL, work_epoch INTEGER NOT NULL
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_activation_claim AFTER UPDATE OF fence,holder ON memory_compiler_jobs WHEN NEW.holder IS NOT NULL BEGIN
 INSERT INTO memory_compiler_activation_claims(job_id,fence,work_epoch)
 SELECT NEW.job_id,NEW.fence,a.work_epoch FROM memory_compiler_activation_jobs l JOIN memory_compiler_activations a USING(activation_id) WHERE l.job_id=NEW.job_id
 ON CONFLICT(job_id) DO UPDATE SET fence=excluded.fence,work_epoch=excluded.work_epoch;
END;
CREATE VIEW IF NOT EXISTS memory_compiler_activation_invalid_claims AS
 SELECT l.job_id FROM memory_compiler_activation_jobs l JOIN memory_compiler_activations a USING(activation_id)
 JOIN memory_compiler_jobs j USING(job_id) LEFT JOIN memory_compiler_activation_claims c USING(job_id)
 WHERE a.work_paused=1 OR c.job_id IS NULL OR c.fence<>j.fence OR c.work_epoch<>a.work_epoch;
CREATE TRIGGER IF NOT EXISTS memory_compiler_activation_append AFTER INSERT ON memory_compiler_event_positions BEGIN
 INSERT INTO memory_compiler_activation_dirty(activation_id,session_id,high_position)
 SELECT a.activation_id,e.session_id,NEW.commit_position
 FROM events e JOIN sessions s ON s.id=e.session_id JOIN memory_compiler_activations a
 ON a.source_scope=CASE WHEN s.workspace_id IS NOT NULL THEN 'workspace:'||s.workspace_id WHEN s.project_id IS NOT NULL THEN 'project:'||s.project_id ELSE 'global' END
 AND (a.source_session='' OR a.source_session=e.session_id)
 WHERE e.id=NEW.event_id AND a.through_position IS NULL AND NEW.commit_position>a.after_position
 ON CONFLICT(activation_id,session_id) DO UPDATE SET high_position=excluded.high_position;
END;
`
