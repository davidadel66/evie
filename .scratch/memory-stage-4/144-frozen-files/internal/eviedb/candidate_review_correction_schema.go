package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const semanticClaimCorrectionsTable = `
CREATE TABLE IF NOT EXISTS semantic_claim_corrections (
    operation_id          TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    scope_id              TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    old_claim_id          TEXT NOT NULL UNIQUE REFERENCES semantic_claims(claim_id),
    replacement_claim_id  TEXT NOT NULL UNIQUE REFERENCES semantic_claims(claim_id),
    mode                  TEXT NOT NULL CHECK (mode IN ('error', 'changed')),
    effective_time        TEXT,
    old_valid_from        TEXT,
    old_valid_to          TEXT,
    old_effective_from    TEXT,
    old_effective_to      TEXT,
    replacement_from      TEXT,
    replacement_to        TEXT,
    scope_revision        INTEGER NOT NULL CHECK (scope_revision > 0),
    transaction_time      TEXT NOT NULL,
    PRIMARY KEY (operation_id, old_claim_id),
    CHECK ((mode = 'error' AND effective_time IS NULL)
        OR (mode = 'changed' AND effective_time IS NOT NULL)),
    CHECK (old_valid_from IS NULL OR old_valid_to IS NULL OR old_valid_from < old_valid_to),
    CHECK (old_effective_from IS NULL OR old_effective_to IS NULL OR old_effective_from < old_effective_to),
    CHECK (replacement_from IS NULL OR replacement_to IS NULL OR replacement_from < replacement_to)
);
`

const semanticClaimCorrectionsAuxiliary = `
CREATE INDEX IF NOT EXISTS semantic_claim_corrections_scope_idx ON semantic_claim_corrections(scope_id, transaction_time, scope_revision);
CREATE TRIGGER IF NOT EXISTS semantic_claim_corrections_append_only_update BEFORE UPDATE ON semantic_claim_corrections BEGIN SELECT RAISE(ABORT, 'semantic claim corrections are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_claim_corrections_append_only_delete BEFORE DELETE ON semantic_claim_corrections BEGIN SELECT RAISE(ABORT, 'semantic claim corrections are append-only'); END;
`

// Correction rows are projection records. A compound accepted operation can
// correct several distinct Claims, while each old and replacement Claim still
// participates in at most one correction. Detect the Stage 3 shape under the
// write lock so competing startup processes cannot migrate a stale schema.
func ensureSemanticBaseSchema(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		if err := migrateSemanticCorrectionSchema(ctx, conn); err != nil {
			return err
		}
		// Bootstrap the base tables in the same transaction. A concurrent fresh
		// startup must never inspect a table before its index/triggers exist.
		_, err := conn.ExecContext(ctx, semanticSchema)
		return err
	})
}

func migrateSemanticCorrectionSchema(ctx context.Context, conn *sql.Conn) error {
	var definition string
	err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='semantic_claim_corrections'`).Scan(&definition)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect semantic correction schema: %w", err)
	}
	current := correctionSchemaSQL(semanticClaimCorrectionsTable)
	legacy := strings.Replace(current, "operation_id TEXT NOT NULL", "operation_id TEXT PRIMARY KEY NOT NULL", 1)
	legacy = strings.Replace(legacy, " PRIMARY KEY (operation_id, old_claim_id),", "", 1)
	actual := correctionSchemaSQL(definition)
	if actual != current && actual != legacy {
		return errors.New("unsupported semantic correction table schema")
	}
	if err := validateSemanticCorrectionAuxiliary(ctx, conn); err != nil {
		return err
	}
	if actual == current {
		return nil
	}
	// No production table references corrections. Refuse an unknown inbound
	// relation rather than risking a cascade or retargeting it during replacement.
	var references int
	if err := conn.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master AS m JOIN pragma_foreign_key_list(m.name) AS f WHERE m.type='table' AND f."table"='semantic_claim_corrections'`).Scan(&references); err != nil {
		return err
	}
	if references != 0 {
		return errors.New("unsupported semantic correction foreign reference")
	}
	replacement := strings.Replace(semanticClaimCorrectionsTable, "IF NOT EXISTS semantic_claim_corrections", "semantic_claim_corrections_compound", 1)
	if _, err := conn.ExecContext(ctx, replacement); err != nil {
		return fmt.Errorf("create compound correction table: %w", err)
	}
	// Copy every column without decoding historical values or accepted envelopes.
	// Preserve rowids too, since projection diagnostics use them to locate damage.
	if _, err := conn.ExecContext(ctx, `
 INSERT INTO semantic_claim_corrections_compound
 (rowid, operation_id, scope_id, old_claim_id, replacement_claim_id, mode, effective_time,
 old_valid_from, old_valid_to, old_effective_from, old_effective_to, replacement_from,
 replacement_to, scope_revision, transaction_time)
 SELECT rowid, operation_id, scope_id, old_claim_id, replacement_claim_id, mode, effective_time,
 old_valid_from, old_valid_to, old_effective_from, old_effective_to, replacement_from,
 replacement_to, scope_revision, transaction_time FROM semantic_claim_corrections;
 DROP TABLE semantic_claim_corrections;
 ALTER TABLE semantic_claim_corrections_compound RENAME TO semantic_claim_corrections;
 `+semanticClaimCorrectionsAuxiliary); err != nil {
		return fmt.Errorf("migrate compound corrections: %w", err)
	}
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check(semantic_claim_corrections)`)
	if err != nil {
		return err
	}
	invalid := rows.Next()
	scanErr := rows.Err()
	closeErr := rows.Close()
	if scanErr != nil {
		return scanErr
	}
	if closeErr != nil {
		return closeErr
	}
	if invalid {
		return errors.New("compound correction migration foreign key violation")
	}
	return nil
}

// This migration supports only the shipped table shapes and auxiliary objects.
// Failing closed also prevents silently dropping a future/custom index, trigger,
// column, check, or foreign key while upgrading a populated database.
func validateSemanticCorrectionAuxiliary(ctx context.Context, conn *sql.Conn) error {
	expected := make(map[string]string)
	for _, statement := range strings.Split(strings.TrimSpace(semanticClaimCorrectionsAuxiliary), "\n") {
		fields := strings.Fields(statement)
		expected[fields[5]] = correctionSchemaSQL(statement)
	}
	rows, err := conn.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE tbl_name='semantic_claim_corrections' AND type IN ('index','trigger') AND sql IS NOT NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, statement string
		if err := rows.Scan(&name, &statement); err != nil {
			return err
		}
		if expected[name] != correctionSchemaSQL(statement) {
			return errors.New("unsupported semantic correction index or trigger")
		}
		delete(expected, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(expected) != 0 {
		return errors.New("incomplete semantic correction indexes or triggers")
	}
	return nil
}

func correctionSchemaSQL(statement string) string {
	statement = strings.Replace(statement, "IF NOT EXISTS ", "", 1)
	statement = strings.ReplaceAll(statement, `"semantic_claim_corrections"`, "semantic_claim_corrections")
	return strings.TrimSuffix(strings.Join(strings.Fields(statement), " "), ";")
}
