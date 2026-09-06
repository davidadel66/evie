package eviedb

import (
	"context"
	"database/sql"
)

// Selecting compilation makes its exact destination navigable before an
// extractor produces any candidates. Registry visibility still passes the
// current owner scope checks in ListLocalOwnerCandidateScopes.
const compilerDiagnosticNavigationSchema = `
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_activation_navigation AFTER INSERT ON memory_compiler_activations BEGIN
 INSERT OR IGNORE INTO memory_review_inbox_revisions(scope_key,revision) VALUES(NEW.destination,0);
END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_diagnostic_history_navigation AFTER INSERT ON memory_compiler_history_ranges BEGIN
 INSERT OR IGNORE INTO memory_review_inbox_revisions(scope_key,revision) VALUES(NEW.destination,0);
END;
CREATE TABLE IF NOT EXISTS memory_compiler_diagnostic_navigation (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1),cohort INTEGER NOT NULL DEFAULT 0,
 last_id TEXT NOT NULL DEFAULT '',last_ordinal INTEGER NOT NULL DEFAULT -1,
 activation_cutoff TEXT NOT NULL,history_cutoff TEXT NOT NULL,job_cutoff TEXT NOT NULL
);
INSERT OR IGNORE INTO memory_compiler_diagnostic_navigation(singleton,cohort,activation_cutoff,history_cutoff,job_cutoff)
 SELECT 1,CASE WHEN a<>'' THEN 0 WHEN h<>'' THEN 1 WHEN j<>'' THEN 2 ELSE 3 END,a,h,j FROM (
 SELECT COALESCE((SELECT MAX(activation_id) FROM memory_compiler_activations),'') a,COALESCE((SELECT MAX(request_id) FROM memory_compiler_history_ranges),'') h,COALESCE((SELECT MAX(job_id) FROM memory_compiler_jobs),'') j);
`

// Each separately committed pass visits at most 15 legacy selection records and
// changes at most 16 rows. No per-scope DISTINCT scan hides retained job work.
func reconcileCompilerScopeNavigation(ctx context.Context, tx *sql.Conn) (bool, error) {
	var cohort int
	var last, activation, history, job string
	var ordinal int64
	if err := tx.QueryRowContext(ctx, `SELECT cohort,last_id,last_ordinal,activation_cutoff,history_cutoff,job_cutoff FROM memory_compiler_diagnostic_navigation WHERE singleton=1`).Scan(&cohort, &last, &ordinal, &activation, &history, &job); err != nil {
		return false, err
	}
	if cohort >= 3 {
		return false, nil
	}
	var query string
	var args []any
	switch cohort {
	case 0:
		query = `SELECT activation_id,0,destination FROM memory_compiler_activations WHERE activation_id>? AND activation_id<=? ORDER BY activation_id LIMIT 15`
		args = []any{last, activation}
	case 1:
		query = `SELECT request_id,ordinal,destination FROM memory_compiler_history_ranges WHERE request_id>=? AND request_id<=? AND (request_id>? OR ordinal>?) ORDER BY request_id,ordinal LIMIT 15`
		args = []any{last, history, last, ordinal}
	case 2:
		query = `SELECT job_id,0,destination FROM memory_compiler_jobs WHERE job_id>? AND job_id<=? ORDER BY job_id LIMIT 15`
		args = []any{last, job}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	type entry struct {
		id, scope string
		ordinal   int64
	}
	entries := []entry{}
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.ordinal, &e.scope); err != nil {
			rows.Close()
			return false, err
		}
		entries = append(entries, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO memory_review_inbox_revisions(scope_key,revision) VALUES(?,0)`, e.scope); err != nil {
			return false, err
		}
		last, ordinal = e.id, e.ordinal
	}
	if len(entries) < 15 {
		cohort++
		for cohort < 3 && []string{activation, history, job}[cohort] == "" {
			cohort++
		}
		last = ""
		ordinal = -1
	}
	_, err = tx.ExecContext(ctx, `UPDATE memory_compiler_diagnostic_navigation SET cohort=?,last_id=?,last_ordinal=? WHERE singleton=1`, cohort, last, ordinal)
	return cohort < 3, err
}
