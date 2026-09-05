package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const candidateReviewSchema = `
CREATE INDEX IF NOT EXISTS memory_review_jobs_destination ON memory_compiler_jobs(destination,job_id);
CREATE INDEX IF NOT EXISTS memory_review_actionable_candidates ON memory_compiler_candidates(job_id,candidate_id) WHERE review_state='unresolved' AND equivalent_to IS NULL;
CREATE TABLE IF NOT EXISTS memory_review_authorization (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), revision INTEGER NOT NULL,
 authentication_key BLOB NOT NULL CHECK(length(authentication_key)=32), source_policy TEXT NOT NULL
);
INSERT OR IGNORE INTO memory_review_authorization VALUES(1,1,randomblob(32),'owner-assertions-v1');
CREATE TABLE IF NOT EXISTS memory_review_previews (
 preview_id TEXT PRIMARY KEY, scope_key TEXT NOT NULL, preview_sha256 TEXT NOT NULL, envelope BLOB NOT NULL CHECK(length(envelope)<=262144)
);
CREATE TABLE IF NOT EXISTS memory_review_deliveries (
 owner_id TEXT NOT NULL, delivery_key TEXT NOT NULL, scope_key TEXT NOT NULL,
 request_hash TEXT NOT NULL, result BLOB NOT NULL, PRIMARY KEY(owner_id,delivery_key)
);
CREATE TABLE IF NOT EXISTS memory_review_audits (
 audit_id TEXT PRIMARY KEY, scope_key TEXT NOT NULL, preview_id TEXT NOT NULL REFERENCES memory_review_previews(preview_id),
 action TEXT NOT NULL CHECK(action IN ('accept','reject')), envelope BLOB NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_review_resolutions (
 candidate_id TEXT PRIMARY KEY REFERENCES memory_compiler_candidates(candidate_id),
 owner_id TEXT NOT NULL, delivery_key TEXT NOT NULL,
 audit_id TEXT NOT NULL REFERENCES memory_review_audits(audit_id)
);
CREATE TABLE IF NOT EXISTS memory_review_inbox_revisions (
 scope_key TEXT PRIMARY KEY, revision INTEGER NOT NULL
);
CREATE TRIGGER IF NOT EXISTS memory_review_candidate_insert AFTER INSERT ON memory_compiler_candidates BEGIN
 INSERT INTO memory_review_inbox_revisions(scope_key,revision) SELECT destination,1 FROM memory_compiler_jobs WHERE job_id=NEW.job_id
 ON CONFLICT(scope_key) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER IF NOT EXISTS memory_review_candidate_change AFTER UPDATE OF review_revision,equivalent_to ON memory_compiler_candidates BEGIN
 INSERT INTO memory_review_inbox_revisions(scope_key,revision) SELECT destination,1 FROM memory_compiler_jobs WHERE job_id=NEW.job_id
 ON CONFLICT(scope_key) DO UPDATE SET revision=revision+1;
END;
CREATE TRIGGER IF NOT EXISTS memory_review_resolutions_no_update BEFORE UPDATE ON memory_review_resolutions BEGIN SELECT RAISE(ABORT,'review resolutions are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_resolutions_no_delete BEFORE DELETE ON memory_review_resolutions BEGIN SELECT RAISE(ABORT,'review resolutions are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_previews_no_update BEFORE UPDATE ON memory_review_previews BEGIN SELECT RAISE(ABORT,'review previews are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_previews_no_delete BEFORE DELETE ON memory_review_previews BEGIN SELECT RAISE(ABORT,'review previews are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_audits_no_update BEFORE UPDATE ON memory_review_audits BEGIN SELECT RAISE(ABORT,'review audits are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_audits_no_delete BEFORE DELETE ON memory_review_audits BEGIN SELECT RAISE(ABORT,'review audits are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_deliveries_no_update BEFORE UPDATE ON memory_review_deliveries BEGIN SELECT RAISE(ABORT,'review results are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_deliveries_no_delete BEFORE DELETE ON memory_review_deliveries BEGIN SELECT RAISE(ABORT,'review results are immutable'); END;
`

func ensureCandidateReviewSchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, candidateReviewSchema+reviewIdentitySchema+reviewTemporalSchema+reviewEditSchema+reviewBatchSchema)
		return err
	})
}

func ensureSemanticOperationSchemaV6(ctx context.Context, db *sql.DB) error {
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='semantic_operations'`).Scan(&definition); err != nil {
		return err
	}
	if strings.Contains(definition, "schema_version IN (1, 2, 3, 4, 5, 6)") {
		return nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys=ON`)
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()
	migration := strings.ReplaceAll(semanticOperationsV5Migration, "semantic_operations_v5", "semantic_operations_v6")
	migration = strings.ReplaceAll(migration, "schema_version IN (1, 2, 3, 4, 5)", "schema_version IN (1, 2, 3, 4, 5, 6)")
	if _, err = conn.ExecContext(ctx, migration); err != nil {
		return fmt.Errorf("migrate semantic operation v6: %w", err)
	}
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	invalid := rows.Next()
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return scanErr
	}
	if invalid {
		return errors.New("semantic v6 migration foreign key violation")
	}
	if _, err = conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}
