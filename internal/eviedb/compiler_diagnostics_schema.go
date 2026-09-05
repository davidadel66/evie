package eviedb

import (
	"context"
	"database/sql"
)

// Side projections never rewrite compiler requests, candidates, or accepted
// operations. Legacy reconciliation visits at most 15 jobs per transaction;
// missing historical wall-clock observations remain NULL.
func ensureCompilerDiagnosticsSchema(ctx context.Context, db *sql.DB) error {
	if err := withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, compilerDiagnosticsSchema+compilerDiagnosticNavigationSchema)
		return err
	}); err != nil {
		return err
	}
	return ensureCompilerStatusProjectionSchema(ctx, db)
}

const compilerDiagnosticsSchema = `
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_selection_page ON memory_compiler_selections(destination,session_id,selection_id);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_root_page ON memory_compiler_activation_roots(session_id,activation_id,root_id);
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_sessions (
 destination TEXT NOT NULL, session_id TEXT NOT NULL REFERENCES sessions(id),
 revision INTEGER NOT NULL DEFAULT 0, counts TEXT NOT NULL DEFAULT '{}',
 PRIMARY KEY(destination,session_id)
);
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_jobs (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs(job_id),
 destination TEXT NOT NULL, session_id TEXT NOT NULL, state TEXT NOT NULL,
 attempts INTEGER NOT NULL, cancellations INTEGER NOT NULL DEFAULT 0,
 queued_at INTEGER, ready_at INTEGER, published_at INTEGER, publication_ns INTEGER,
 unresolved INTEGER NOT NULL DEFAULT 0, accepted INTEGER NOT NULL DEFAULT 0,
 rejected INTEGER NOT NULL DEFAULT 0, suppressed INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_attempts (
 job_id TEXT NOT NULL REFERENCES memory_compiler_jobs(job_id), attempt INTEGER NOT NULL, fence INTEGER NOT NULL,
 claimed_at INTEGER NOT NULL, queue_wait_ns INTEGER,
 inference_ns INTEGER, validation_ns INTEGER, database_ns INTEGER,
 outcome TEXT NOT NULL DEFAULT 'incomplete', PRIMARY KEY(job_id,attempt)
);
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_decisions (
 candidate_id TEXT PRIMARY KEY REFERENCES memory_compiler_candidates(candidate_id), decided_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_foreground (
 root_id TEXT PRIMARY KEY REFERENCES events(id), session_id TEXT NOT NULL REFERENCES sessions(id),
 started_at INTEGER NOT NULL, terminal_at INTEGER, terminal_ns INTEGER,
 finalized_at INTEGER, finalization_ns INTEGER, outcome TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_foreground_page ON memory_compiler_diagnostic_foreground(session_id,root_id);
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_reconcile (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), last_job TEXT NOT NULL, cutoff TEXT NOT NULL,
 complete INTEGER NOT NULL CHECK(complete IN(0,1))
);
INSERT OR IGNORE INTO memory_compiler_diagnostic_reconcile SELECT 1,'',COALESCE(MAX(job_id),''),CASE WHEN MAX(job_id) IS NULL THEN 1 ELSE 0 END FROM memory_compiler_jobs;
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_legacy_job_page ON memory_compiler_jobs(session_id,job_id);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_job_page ON memory_compiler_jobs(destination,session_id,job_id);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_candidate_page ON memory_compiler_candidates(job_id,candidate_id);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_activation_page ON memory_compiler_activations(destination,source_scope,source_session,activation_id);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_activation_nonempty_interval ON memory_compiler_activations(generation_id,destination,source_scope,source_session,after_position DESC) WHERE through_position IS NULL OR through_position>after_position;
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_history_page ON memory_compiler_history_ranges(destination,session_id,request_id,ordinal);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_global_session ON sessions(id) WHERE workspace_id IS NULL AND project_id IS NULL;
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_workspace_session ON sessions(workspace_id,id);
CREATE INDEX IF NOT EXISTS memory_compiler_diagnostic_project_session ON sessions(project_id,id) WHERE workspace_id IS NULL;
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_count_insert AFTER INSERT ON memory_compiler_diagnostic_jobs BEGIN
 INSERT OR IGNORE INTO memory_review_inbox_revisions(scope_key,revision) VALUES(NEW.destination,0);
 INSERT INTO memory_compiler_diagnostic_sessions(destination,session_id,revision) VALUES(NEW.destination,NEW.session_id,0) ON CONFLICT DO NOTHING;
 UPDATE memory_compiler_diagnostic_sessions SET revision=revision+1,counts=json_set(counts,'$.jobs_queued',COALESCE(json_extract(counts,'$.jobs_queued'),0)+(NEW.state='queued'),'$.jobs_running',COALESCE(json_extract(counts,'$.jobs_running'),0)+(NEW.state='running'),'$.jobs_retry_wait',COALESCE(json_extract(counts,'$.jobs_retry_wait'),0)+(NEW.state='retry_wait'),'$.jobs_staged',COALESCE(json_extract(counts,'$.jobs_staged'),0)+(NEW.state='staged'),'$.jobs_cancelled',COALESCE(json_extract(counts,'$.jobs_cancelled'),0)+(NEW.state='cancelled'),'$.jobs_failed',COALESCE(json_extract(counts,'$.jobs_failed'),0)+(NEW.state='failed'),'$.jobs_completed_candidates',COALESCE(json_extract(counts,'$.jobs_completed_candidates'),0)+(NEW.state='completed_candidates'),'$.jobs_completed_empty',COALESCE(json_extract(counts,'$.jobs_completed_empty'),0)+(NEW.state='completed_empty'),'$.jobs_excluded',COALESCE(json_extract(counts,'$.jobs_excluded'),0)+(NEW.state='excluded'),'$.attempts',COALESCE(json_extract(counts,'$.attempts'),0)+NEW.attempts,'$.cancellations',COALESCE(json_extract(counts,'$.cancellations'),0)+NEW.cancellations,'$.candidates_unresolved',COALESCE(json_extract(counts,'$.candidates_unresolved'),0)+NEW.unresolved,'$.candidates_accepted',COALESCE(json_extract(counts,'$.candidates_accepted'),0)+NEW.accepted,'$.candidates_rejected',COALESCE(json_extract(counts,'$.candidates_rejected'),0)+NEW.rejected,'$.candidates_suppressed',COALESCE(json_extract(counts,'$.candidates_suppressed'),0)+NEW.suppressed) WHERE destination=NEW.destination AND session_id=NEW.session_id;
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_count_update AFTER UPDATE ON memory_compiler_diagnostic_jobs BEGIN
 UPDATE memory_compiler_diagnostic_sessions SET revision=revision+1,counts=json_set(counts,'$.jobs_queued',COALESCE(json_extract(counts,'$.jobs_queued'),0)+(NEW.state='queued')-(OLD.state='queued'),'$.jobs_running',COALESCE(json_extract(counts,'$.jobs_running'),0)+(NEW.state='running')-(OLD.state='running'),'$.jobs_retry_wait',COALESCE(json_extract(counts,'$.jobs_retry_wait'),0)+(NEW.state='retry_wait')-(OLD.state='retry_wait'),'$.jobs_staged',COALESCE(json_extract(counts,'$.jobs_staged'),0)+(NEW.state='staged')-(OLD.state='staged'),'$.jobs_cancelled',COALESCE(json_extract(counts,'$.jobs_cancelled'),0)+(NEW.state='cancelled')-(OLD.state='cancelled'),'$.jobs_failed',COALESCE(json_extract(counts,'$.jobs_failed'),0)+(NEW.state='failed')-(OLD.state='failed'),'$.jobs_completed_candidates',COALESCE(json_extract(counts,'$.jobs_completed_candidates'),0)+(NEW.state='completed_candidates')-(OLD.state='completed_candidates'),'$.jobs_completed_empty',COALESCE(json_extract(counts,'$.jobs_completed_empty'),0)+(NEW.state='completed_empty')-(OLD.state='completed_empty'),'$.jobs_excluded',COALESCE(json_extract(counts,'$.jobs_excluded'),0)+(NEW.state='excluded')-(OLD.state='excluded'),'$.attempts',COALESCE(json_extract(counts,'$.attempts'),0)+NEW.attempts-OLD.attempts,'$.cancellations',COALESCE(json_extract(counts,'$.cancellations'),0)+NEW.cancellations-OLD.cancellations,'$.candidates_unresolved',COALESCE(json_extract(counts,'$.candidates_unresolved'),0)+NEW.unresolved-OLD.unresolved,'$.candidates_accepted',COALESCE(json_extract(counts,'$.candidates_accepted'),0)+NEW.accepted-OLD.accepted,'$.candidates_rejected',COALESCE(json_extract(counts,'$.candidates_rejected'),0)+NEW.rejected-OLD.rejected,'$.candidates_suppressed',COALESCE(json_extract(counts,'$.candidates_suppressed'),0)+NEW.suppressed-OLD.suppressed) WHERE destination=NEW.destination AND session_id=NEW.session_id;
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_job_insert AFTER INSERT ON memory_compiler_jobs BEGIN
 INSERT INTO memory_compiler_diagnostic_jobs(job_id,destination,session_id,state,attempts,queued_at,ready_at)
 VALUES(NEW.job_id,NEW.destination,NEW.session_id,NEW.state,NEW.attempts,unixepoch('subsec')*1000,unixepoch('subsec')*1000);
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_job_change AFTER UPDATE OF state,attempts ON memory_compiler_jobs WHEN NEW.state<>OLD.state OR NEW.attempts<>OLD.attempts BEGIN
 INSERT OR IGNORE INTO memory_compiler_diagnostic_jobs(job_id,destination,session_id,state,attempts)
 VALUES(OLD.job_id,OLD.destination,OLD.session_id,OLD.state,OLD.attempts);
 UPDATE memory_compiler_diagnostic_jobs SET state=NEW.state,attempts=NEW.attempts,
 cancellations=cancellations+(NEW.state='cancelled' AND OLD.state<>'cancelled'),
 ready_at=CASE WHEN NEW.state='retry_wait' THEN NEW.retry_at*1000 ELSE ready_at END,
  unresolved=(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=NEW.job_id AND review_state='unresolved' AND equivalent_to IS NULL),
 accepted=(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=NEW.job_id AND review_state='accepted'),
 rejected=(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=NEW.job_id AND review_state='rejected'),
 suppressed=(SELECT COUNT(*) FROM memory_compiler_candidates WHERE job_id=NEW.job_id AND equivalent_to IS NOT NULL)
 WHERE job_id=NEW.job_id;
 INSERT INTO memory_compiler_diagnostic_attempts(job_id,attempt,fence,claimed_at,queue_wait_ns)
 SELECT NEW.job_id,NEW.attempts,NEW.fence,unixepoch('subsec')*1000,CASE WHEN CAST(unixepoch('subsec')*1000 AS INTEGER)>=ready_at THEN (CAST(unixepoch('subsec')*1000 AS INTEGER)-ready_at)*1000000 END
 FROM memory_compiler_diagnostic_jobs WHERE job_id=NEW.job_id AND NEW.attempts>OLD.attempts;
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_review_change AFTER UPDATE OF review_state,equivalent_to ON memory_compiler_candidates WHEN NEW.review_state<>OLD.review_state OR NEW.equivalent_to IS NOT OLD.equivalent_to BEGIN
 UPDATE memory_compiler_diagnostic_jobs SET
 unresolved=unresolved+(NEW.review_state='unresolved' AND NEW.equivalent_to IS NULL)-(OLD.review_state='unresolved' AND OLD.equivalent_to IS NULL),
 accepted=accepted+(NEW.review_state='accepted')-(OLD.review_state='accepted'),
 rejected=rejected+(NEW.review_state='rejected')-(OLD.review_state='rejected'),
 suppressed=suppressed+(NEW.equivalent_to IS NOT NULL)-(OLD.equivalent_to IS NOT NULL) WHERE job_id=NEW.job_id;
 INSERT OR IGNORE INTO memory_compiler_diagnostic_decisions SELECT NEW.candidate_id,unixepoch('subsec')*1000 WHERE NEW.review_state IN('accepted','rejected');
END;
`
