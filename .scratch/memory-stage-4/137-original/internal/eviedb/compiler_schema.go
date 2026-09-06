package eviedb

import (
	"context"
	"database/sql"
)

// The trigger runs in the event append transaction, including when compilation
// is unconfigured. Existing events remain explicitly legacy in the view; the
// migration assigns no invented cross-session order to historical events.
const compilerSchema = `
CREATE TABLE IF NOT EXISTS memory_compiler_position_counter (
 singleton INTEGER PRIMARY KEY CHECK(singleton = 1), value INTEGER NOT NULL CHECK(value >= 0)
);
INSERT OR IGNORE INTO memory_compiler_position_counter VALUES(1,0);
CREATE TABLE IF NOT EXISTS memory_compiler_event_positions (
 event_id TEXT PRIMARY KEY REFERENCES events(id), commit_position INTEGER NOT NULL UNIQUE
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_append_position AFTER INSERT ON events BEGIN
 UPDATE memory_compiler_position_counter SET value = value + 1 WHERE singleton = 1;
 INSERT INTO memory_compiler_event_positions VALUES(NEW.id,(SELECT value FROM memory_compiler_position_counter WHERE singleton=1));
END;
CREATE VIEW IF NOT EXISTS memory_compiler_event_coordinates AS
 SELECT e.id AS event_id,e.session_id,e.sequence,p.commit_position,
 CASE WHEN p.event_id IS NULL THEN 'legacy' ELSE 'positioned' END AS cohort
 FROM events e LEFT JOIN memory_compiler_event_positions p ON p.event_id=e.id;
CREATE TABLE IF NOT EXISTS memory_compiler_generations (
 generation_id TEXT PRIMARY KEY, manifest BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_compiler_selections (
 selection_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES memory_compiler_generations(generation_id),
 destination TEXT NOT NULL, session_id TEXT NOT NULL REFERENCES sessions(id), root_id TEXT NOT NULL REFERENCES events(id),
 first_sequence INTEGER NOT NULL, last_sequence INTEGER NOT NULL, state TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
 job_id TEXT, window BLOB NOT NULL, UNIQUE(generation_id,destination,session_id,root_id,last_sequence)
);
CREATE TABLE IF NOT EXISTS memory_compiler_jobs (
 job_id TEXT PRIMARY KEY, generation_id TEXT NOT NULL REFERENCES memory_compiler_generations(generation_id),
 destination TEXT NOT NULL, session_id TEXT NOT NULL REFERENCES sessions(id), root_id TEXT NOT NULL REFERENCES events(id),
 first_sequence INTEGER NOT NULL, last_sequence INTEGER NOT NULL, window_hash TEXT NOT NULL,
 request BLOB NOT NULL, state TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '',
 attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts BETWEEN 0 AND 5), fence INTEGER NOT NULL DEFAULT 0,
 holder TEXT, lease_until INTEGER, retry_at INTEGER,
 UNIQUE(generation_id,destination,session_id,root_id,last_sequence)
);
CREATE TABLE IF NOT EXISTS memory_compiler_capacity (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), request_id TEXT NOT NULL UNIQUE,
 job_id TEXT NOT NULL REFERENCES memory_compiler_jobs(job_id), fence INTEGER NOT NULL,
 holder TEXT NOT NULL, server_identity TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('reserved','release_pending'))
);
CREATE TABLE IF NOT EXISTS memory_compiler_stages (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs(job_id), fence INTEGER NOT NULL,
 envelope BLOB NOT NULL CHECK(length(envelope)<=131072), envelope_hash TEXT NOT NULL, consumed INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS memory_compiler_candidate_groups (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs(job_id), envelope_hash TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_compiler_candidates (
 candidate_id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES memory_compiler_candidate_groups(job_id),
 ordinal INTEGER NOT NULL, envelope BLOB NOT NULL, equivalence_hash TEXT NOT NULL,
 review_state TEXT NOT NULL DEFAULT 'unresolved', review_revision INTEGER NOT NULL DEFAULT 0,
 equivalent_to TEXT REFERENCES memory_compiler_candidates(candidate_id), UNIQUE(job_id,ordinal)
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_candidate_immutable BEFORE UPDATE OF envelope,job_id,ordinal,equivalence_hash ON memory_compiler_candidates BEGIN SELECT RAISE(ABORT,'immutable compiler candidate'); END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_generation_immutable BEFORE UPDATE ON memory_compiler_generations BEGIN SELECT RAISE(ABORT,'immutable compiler generation'); END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_request_immutable BEFORE UPDATE OF generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request ON memory_compiler_jobs BEGIN SELECT RAISE(ABORT,'immutable compiler request'); END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_stage_immutable BEFORE UPDATE OF envelope,envelope_hash ON memory_compiler_stages BEGIN SELECT RAISE(ABORT,'immutable compiler stage'); END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_position_immutable BEFORE UPDATE ON memory_compiler_event_positions BEGIN SELECT RAISE(ABORT,'immutable compiler event position'); END;
CREATE INDEX IF NOT EXISTS memory_compiler_candidate_equivalence ON memory_compiler_candidates(equivalence_hash);
CREATE TABLE IF NOT EXISTS memory_compiler_coverage (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs(job_id), outcome TEXT NOT NULL CHECK(outcome IN ('completed_candidates','completed_empty','excluded')),
 event_ids BLOB NOT NULL
);
`

func ensureCompilerSchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransactionResolver(ctx, db, executeImmediateTransactionStatement, transactionResolutionContext, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, compilerSchema)
		return err
	})
}
