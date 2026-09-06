package eviedb

import (
	"context"
	"database/sql"
)

func ensureCompilerHistorySchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransactionResolver(ctx, db, executeImmediateTransactionStatement, transactionResolutionContext, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, compilerHistorySchema)
		return err
	})
}

const compilerHistorySchema = `
CREATE TABLE IF NOT EXISTS memory_compiler_history_requests (
 request_id TEXT PRIMARY KEY, request_hash TEXT NOT NULL, owner_id TEXT NOT NULL,
 generation_id TEXT NOT NULL REFERENCES memory_compiler_generations,
 selection_order INTEGER NOT NULL UNIQUE, receipt BLOB NOT NULL,
 pending_ranges INTEGER NOT NULL DEFAULT 0, pending_roots INTEGER NOT NULL DEFAULT 0, range_checked INTEGER NOT NULL DEFAULT 0, root_checked INTEGER NOT NULL DEFAULT 0, resume_order INTEGER NOT NULL DEFAULT 0, revision INTEGER NOT NULL DEFAULT 1, cancelled INTEGER NOT NULL DEFAULT 0 CHECK(cancelled IN (0,1))
);
CREATE TABLE IF NOT EXISTS memory_compiler_history_schedule_counter(singleton INTEGER PRIMARY KEY CHECK(singleton=1),value INTEGER NOT NULL);
INSERT OR IGNORE INTO memory_compiler_history_schedule_counter VALUES(1,0);
CREATE INDEX IF NOT EXISTS memory_compiler_history_discovery_ready ON memory_compiler_history_requests(range_checked,selection_order,request_id) WHERE cancelled=0 AND pending_ranges>0;
CREATE INDEX IF NOT EXISTS memory_compiler_history_roots_ready ON memory_compiler_history_requests(root_checked,selection_order,request_id) WHERE cancelled=0 AND pending_roots>0;
CREATE TRIGGER IF NOT EXISTS memory_compiler_history_receipt_immutable BEFORE UPDATE OF request_id,request_hash,owner_id,generation_id,selection_order,receipt ON memory_compiler_history_requests BEGIN SELECT RAISE(ABORT,'immutable history selection receipt'); END;
CREATE TABLE IF NOT EXISTS memory_compiler_history_ranges (
 request_id TEXT NOT NULL REFERENCES memory_compiler_history_requests, ordinal INTEGER NOT NULL,
 source_scope TEXT NOT NULL, destination TEXT NOT NULL, session_id TEXT NOT NULL REFERENCES sessions,
 first_sequence INTEGER NOT NULL, last_sequence INTEGER NOT NULL,
 first_event_id TEXT NOT NULL REFERENCES events, last_event_id TEXT NOT NULL REFERENCES events,
 scanned_sequence INTEGER NOT NULL, checked_order INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(request_id,ordinal)
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_history_range_immutable BEFORE UPDATE OF request_id,ordinal,source_scope,destination,session_id,first_sequence,last_sequence,first_event_id,last_event_id ON memory_compiler_history_ranges BEGIN SELECT RAISE(ABORT,'immutable history range'); END;
CREATE INDEX IF NOT EXISTS memory_compiler_history_coordinate ON memory_compiler_history_ranges(session_id,destination,first_sequence,last_sequence);
CREATE TABLE IF NOT EXISTS memory_compiler_history_roots (
 request_id TEXT NOT NULL, range_ordinal INTEGER NOT NULL, root_id TEXT NOT NULL REFERENCES events,
 first_sequence INTEGER NOT NULL, last_sequence INTEGER NOT NULL,
 state TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '', selection_id TEXT NOT NULL DEFAULT '',
 checked_order INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(request_id,range_ordinal,root_id),
 FOREIGN KEY(request_id,range_ordinal) REFERENCES memory_compiler_history_ranges(request_id,ordinal)
);
CREATE INDEX IF NOT EXISTS memory_compiler_history_range_pending ON memory_compiler_history_ranges(request_id,checked_order,ordinal) WHERE scanned_sequence<last_sequence;
CREATE INDEX IF NOT EXISTS memory_compiler_history_range_discovered ON memory_compiler_history_ranges(request_id,checked_order,ordinal) WHERE scanned_sequence=last_sequence;
CREATE INDEX IF NOT EXISTS memory_compiler_history_root_pending ON memory_compiler_history_roots(request_id,range_ordinal,checked_order,first_sequence,root_id) WHERE state IN ('selected_unmaterialized','deferred_live');
CREATE INDEX IF NOT EXISTS memory_compiler_history_unmaterialized ON memory_compiler_selections(generation_id,destination,session_id,root_id,first_sequence,last_sequence) WHERE job_id IS NULL AND state IN ('selected_unmaterialized','deferred_live');
CREATE INDEX IF NOT EXISTS memory_compiler_history_cancelled ON memory_compiler_jobs(session_id,first_sequence,job_id) WHERE state='cancelled' AND reason='history_cancelled';
CREATE TRIGGER IF NOT EXISTS memory_compiler_history_range_discovered_count AFTER UPDATE OF scanned_sequence ON memory_compiler_history_ranges WHEN OLD.scanned_sequence<OLD.last_sequence AND NEW.scanned_sequence=NEW.last_sequence BEGIN UPDATE memory_compiler_history_requests SET pending_ranges=pending_ranges-1 WHERE request_id=NEW.request_id; END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_history_root_insert_count AFTER INSERT ON memory_compiler_history_roots WHEN NEW.state IN ('selected_unmaterialized','deferred_live') BEGIN UPDATE memory_compiler_history_requests SET pending_roots=pending_roots+1 WHERE request_id=NEW.request_id; END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_history_root_update_count AFTER UPDATE OF state ON memory_compiler_history_roots WHEN (NEW.state IN ('selected_unmaterialized','deferred_live'))<>(OLD.state IN ('selected_unmaterialized','deferred_live')) BEGIN UPDATE memory_compiler_history_requests SET pending_roots=pending_roots+(NEW.state IN ('selected_unmaterialized','deferred_live'))-(OLD.state IN ('selected_unmaterialized','deferred_live')) WHERE request_id=NEW.request_id; END;
CREATE TABLE IF NOT EXISTS memory_compiler_history_events (
 request_id TEXT NOT NULL, range_ordinal INTEGER NOT NULL, event_id TEXT NOT NULL REFERENCES events,
 sequence INTEGER NOT NULL, root_id TEXT NOT NULL REFERENCES events,
 PRIMARY KEY(request_id,range_ordinal,event_id),
 FOREIGN KEY(request_id,range_ordinal) REFERENCES memory_compiler_history_ranges(request_id,ordinal)
);
CREATE TABLE IF NOT EXISTS memory_compiler_history_jobs (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs, cancel_order INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS memory_compiler_history_changes (
 operation_id TEXT PRIMARY KEY, request_hash TEXT NOT NULL, response BLOB NOT NULL, change_order INTEGER NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS memory_compiler_history_resume_queue(request_id TEXT PRIMARY KEY REFERENCES memory_compiler_history_requests,change_order INTEGER NOT NULL,checked_order INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS memory_compiler_history_resume_ready ON memory_compiler_history_resume_queue(checked_order,change_order,request_id);
CREATE INDEX IF NOT EXISTS memory_compiler_history_cancelled_coordinate ON memory_compiler_jobs(generation_id,destination,session_id,last_sequence,first_sequence) WHERE state='cancelled' AND reason='history_cancelled';
CREATE TABLE IF NOT EXISTS memory_compiler_history_selection_refs (
 generation_id TEXT NOT NULL REFERENCES memory_compiler_generations,destination TEXT NOT NULL,session_id TEXT NOT NULL REFERENCES sessions,event_id TEXT NOT NULL REFERENCES events,
 active_requests INTEGER NOT NULL CHECK(active_requests>=0),
 PRIMARY KEY(generation_id,destination,session_id,event_id)
);
CREATE VIEW IF NOT EXISTS memory_compiler_history_paused_jobs AS
 SELECT j.job_id FROM memory_compiler_history_jobs h JOIN memory_compiler_jobs j USING(job_id)
 WHERE NOT EXISTS (
 SELECT 1 FROM json_each(j.request,'$.window.new_event_ids') member
 JOIN memory_compiler_history_selection_refs r ON r.generation_id=j.generation_id AND r.destination=j.destination AND r.session_id=j.session_id AND r.event_id=member.value
 WHERE r.active_requests>0
 );
CREATE VIEW IF NOT EXISTS memory_compiler_paused_jobs AS
 SELECT job_id FROM memory_compiler_activation_paused_jobs UNION ALL SELECT job_id FROM memory_compiler_history_paused_jobs;
CREATE VIEW IF NOT EXISTS memory_compiler_invalid_claims AS
 SELECT job_id FROM memory_compiler_activation_invalid_claims UNION ALL SELECT job_id FROM memory_compiler_history_paused_jobs;
`
