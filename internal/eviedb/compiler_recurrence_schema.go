package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

const compilerRecurrenceSchema = `
CREATE TABLE IF NOT EXISTS memory_compiler_recurrence (
 publication_order INTEGER PRIMARY KEY,
 candidate_id TEXT NOT NULL UNIQUE REFERENCES memory_compiler_candidates(candidate_id),
 encoding_version TEXT NOT NULL CHECK(encoding_version='compiler-recurrence-v2'),
 exact_hash TEXT NOT NULL, related_hash TEXT NOT NULL,
 presentation_epoch INTEGER NOT NULL DEFAULT 0 CHECK(presentation_epoch>=0),
 exact_encoding BLOB NOT NULL CHECK(length(exact_encoding)<=262144),
 related_encoding BLOB NOT NULL CHECK(length(related_encoding)<=262144),
 primary_id TEXT REFERENCES memory_compiler_candidates(candidate_id),
 relationship TEXT NOT NULL CHECK(relationship IN ('primary','exact_original','different_support','current_effect_changed','legacy')),
 suppressed INTEGER NOT NULL CHECK(suppressed IN (0,1)),
 checked_interpretation INTEGER NOT NULL CHECK(checked_interpretation>=0),
 checked_review INTEGER NOT NULL CHECK(checked_review>=0),
 checked_state TEXT NOT NULL CHECK(checked_state IN ('unresolved','accepted','rejected'))
);
CREATE INDEX IF NOT EXISTS memory_compiler_recurrence_exact ON memory_compiler_recurrence(exact_hash,presentation_epoch DESC,publication_order) WHERE suppressed=0;
CREATE INDEX IF NOT EXISTS memory_compiler_recurrence_related ON memory_compiler_recurrence(related_hash,publication_order DESC) WHERE suppressed=0;
CREATE TRIGGER IF NOT EXISTS memory_compiler_recurrence_no_update BEFORE UPDATE ON memory_compiler_recurrence BEGIN SELECT RAISE(ABORT,'recurrence lineage is immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_compiler_recurrence_no_delete BEFORE DELETE ON memory_compiler_recurrence BEGIN SELECT RAISE(ABORT,'recurrence lineage is immutable'); END;
CREATE TABLE IF NOT EXISTS memory_compiler_recurrence_migration (singleton INTEGER PRIMARY KEY CHECK(singleton=1),last_rowid INTEGER NOT NULL);
INSERT OR IGNORE INTO memory_compiler_recurrence_migration VALUES(1,0);
CREATE INDEX IF NOT EXISTS memory_compiler_legacy_recurrence ON memory_compiler_candidates(equivalence_hash,candidate_id) WHERE equivalent_to IS NULL;
`

// Installing the projection never rewrites generation, candidate, review or
// accepted-operation bytes. Each open inspects at most 31 old candidates and
// adds at most 31 projection rows plus one cursor, independent of retained size.
func migrateCompilerRecurrence(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `SELECT c.rowid,c.candidate_id,c.envelope,j.request,g.manifest,COALESCE(c.equivalent_to,'') FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id JOIN memory_compiler_generations g ON g.generation_id=j.generation_id WHERE c.rowid>(SELECT last_rowid FROM memory_compiler_recurrence_migration WHERE singleton=1) ORDER BY c.rowid LIMIT 31`)
	if err != nil {
		return err
	}
	type oldRow struct {
		rowid                          int64
		id, primary                    string
		candidate, request, generation []byte
	}
	old := []oldRow{}
	for rows.Next() {
		var r oldRow
		if err = rows.Scan(&r.rowid, &r.id, &r.candidate, &r.request, &r.generation, &r.primary); err != nil {
			rows.Close()
			return err
		}
		old = append(old, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range old {
		var exists int
		if err = conn.QueryRowContext(ctx, `SELECT count(*) FROM memory_compiler_recurrence WHERE candidate_id=?`, r.id).Scan(&exists); err != nil {
			return err
		}
		if exists != 0 {
			continue
		}
		var c memory.MemoryCandidate
		var request memory.CompilerRequest
		var g memory.CompilerGeneration
		if json.Unmarshal(r.candidate, &c) != nil || json.Unmarshal(r.request, &request) != nil || json.Unmarshal(r.generation, &g) != nil {
			return errors.New("invalid legacy recurrence input")
		}
		exact, related, e := compilerRecurrenceCanonical(g, request, c)
		if e != nil {
			return e
		}
		record := compilerRecurrenceRecord{Exact: exact, Related: related, Relationship: "primary", State: "unresolved"}
		if r.primary != "" {
			record.Primary = r.primary
			record.Relationship = "legacy"
			record.Suppressed = true
			record.Ref, record.State, err = compilerRecurrenceRef(ctx, conn, r.primary)
			if err != nil {
				return err
			}
		}
		if err = insertCompilerRecurrence(ctx, conn, r.id, record); err != nil {
			return err
		}
	}
	if len(old) > 0 {
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_recurrence_migration SET last_rowid=? WHERE singleton=1`, old[len(old)-1].rowid)
	}
	return err
}
