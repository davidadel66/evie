package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Frozen from the Stage 3 schema at 9ef0771. The fixture is populated through
// Stage 3 prepare/apply APIs before any Stage 4 startup upgrade is invoked.
const stage3CorrectionTable = `
CREATE TABLE IF NOT EXISTS semantic_claim_corrections (
    operation_id          TEXT PRIMARY KEY NOT NULL REFERENCES semantic_operations(operation_id),
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
    CHECK ((mode = 'error' AND effective_time IS NULL)
        OR (mode = 'changed' AND effective_time IS NOT NULL)),
    CHECK (old_valid_from IS NULL OR old_valid_to IS NULL OR old_valid_from < old_valid_to),
    CHECK (old_effective_from IS NULL OR old_effective_to IS NULL OR old_effective_from < old_effective_to),
    CHECK (replacement_from IS NULL OR replacement_to IS NULL OR replacement_from < replacement_to)
);
`

func newStage3CorrectionMigrationFixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "stage3.db")
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	legacySchema := strings.Replace(semanticSchema, semanticClaimCorrectionsTable, stage3CorrectionTable, 1)
	if _, err := db.Exec(schema + legacySchema); err != nil {
		t.Fatal(err)
	}
	if err := ensureWorkspaceScope(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ensureSemanticCursorAuth(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := ensureSemanticObjectScopeColumns(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 2, 12, 0, 0, 123, time.UTC)
	setTurnLeaseTime(store, at)
	lease, err := store.AcquireTurnLease(ctx, session.ID, "stage3-correction-migration", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	from, to := at.AddDate(-6, 0, 0), at.AddDate(4, 0, 0)
	old := prepareLiteralForCorrection(t, ctx, store, session, lease,
		"idem:v1:77000000-0000-4000-8000-000000000801", "My timezone was Detroit", "Detroit", memory.ValidTime{From: &from, To: &to})
	prior, err := store.ApplyRememberLiteral(ctx, lease, old)
	if err != nil {
		t.Fatal(err)
	}
	for i, mode := range []memory.CorrectionMode{memory.CorrectionChanged, memory.CorrectionError} {
		at = at.Add(time.Minute)
		setTurnLeaseTime(store, at)
		value := []string{"Chicago", "New York"}[i]
		source := appendLifecycleEvent(t, ctx, store, session, lease, "Correction: "+value+" — retained original evidence")
		request := memory.CorrectClaimRequest{
			IdempotencyKey: fmt.Sprintf("idem:v1:77000000-0000-4000-8000-%012d", 802+i), SourceEventID: source.ID,
			OldClaimID: prior.ClaimID, Mode: mode,
			Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID,
				Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}}, Polarity: memory.PolarityAffirmed},
		}
		if mode == memory.CorrectionChanged {
			effective := from.AddDate(2, 0, 0)
			request.EffectiveTime = &effective
		}
		prepared, err := store.PrepareCorrectClaim(ctx, session.ScopeContext(), request)
		if err != nil {
			t.Fatal(err)
		}
		result, err := store.ApplyCorrectClaim(ctx, lease, prepared)
		if err != nil {
			t.Fatal(err)
		}
		prior.ClaimID = result.ReplacementClaimID
	}
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	var currentVersions int
	if err := db.QueryRow(`SELECT count(*) FROM semantic_operations WHERE schema_version>5`).Scan(&currentVersions); err != nil || currentVersions != 0 {
		t.Fatalf("fixture has non-Stage-3 operations: %d, %v", currentVersions, err)
	}
	assertCorrectionMigrationPrimaryKey(t, db, []string{"operation_id"})
	return db, path
}

func assertCorrectionMigrationPrimaryKey(t *testing.T, db *sql.DB, want []string) {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM pragma_table_info('semantic_claim_corrections') WHERE pk>0 ORDER BY pk`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("correction primary key = %v; want %v", got, want)
	}
}

// Include SQLite storage class and bytes for every cell. This covers canonical
// envelopes, original source bytes, correction times, and lifecycle projections
// without hiding changes through JSON decoding or semantic comparisons.
func correctionMigrationBytes(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	tables := append(semanticProjectionTableNames(), "semantic_operations", "semantic_operation_scopes", "events")
	result := make(map[string][]string)
	for _, table := range tables {
		rows, err := db.Query(`SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
		if err != nil {
			t.Fatal(err)
		}
		var cells []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatal(err)
			}
			cells = append(cells, `typeof("`+name+`"),hex("`+name+`")`)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		rows, err = db.Query(`SELECT ` + strings.Join(cells, ",") + ` FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]string, len(cells)*2)
			destinations := make([]any, len(values))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err := rows.Scan(destinations...); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			result[table] = append(result[table], string(encoded))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		sort.Strings(result[table])
	}
	return result
}

func TestCorrectionSchemaStage3UpgradePreservesBytesAndReplay(t *testing.T) {
	db, path := newStage3CorrectionMigrationFixture(t)
	before := correctionMigrationBytes(t, db)
	verification, err := NewStore(db).VerifySemanticProjection(context.Background())
	if err != nil || !verification.Valid {
		t.Fatalf("old schema replay: %+v, %v", verification, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	assertCorrectionMigrationPrimaryKey(t, upgraded, []string{"operation_id", "old_claim_id"})
	if got := correctionMigrationBytes(t, upgraded); !reflect.DeepEqual(got, before) {
		t.Fatal("Stage 3 upgrade changed accepted/source/projection bytes")
	}
	verification, err = NewStore(upgraded).VerifySemanticProjection(context.Background())
	if err != nil || !verification.Valid {
		t.Fatalf("upgraded replay: %+v, %v", verification, err)
	}
	rebuilt, err := NewStore(upgraded).OwnerRebuildSemanticProjection(context.Background(), "correction-migration-rebuild")
	if err != nil {
		t.Fatalf("upgraded shadow rebuild: %+v, %v", rebuilt, err)
	}
	if got := correctionMigrationBytes(t, upgraded); !reflect.DeepEqual(got, before) {
		t.Fatal("shadow rebuild changed accepted/source/projection bytes")
	}
	if err := runCorrectionSchemaMigration(context.Background(), upgraded); err != nil {
		t.Fatal(err)
	}
}

func TestCorrectionSchemaMigrationRollbackAndCommitResolution(t *testing.T) {
	for _, failure := range []string{"operation_failure", "cancelled", "commit_absent", "commit_durable_response_lost"} {
		t.Run(failure, func(t *testing.T) {
			db, path := newStage3CorrectionMigrationFixture(t)
			before := correctionMigrationBytes(t, db)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			injected := errors.New("injected migration interruption")
			resolve := executeImmediateTransactionStatement
			if strings.HasPrefix(failure, "commit_") {
				resolve = func(ctx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
					if statement == "COMMIT" {
						if failure == "commit_durable_response_lost" {
							if _, err := conn.ExecContext(ctx, statement); err != nil {
								return nil, err
							}
						}
						return nil, injected
					}
					return conn.ExecContext(ctx, statement)
				}
			}
			err := withImmediateTransactionResolver(ctx, db, resolve, transactionResolutionContext, func(conn *sql.Conn) error {
				if err := migrateSemanticCorrectionSchema(ctx, conn); err != nil {
					return err
				}
				if failure == "operation_failure" {
					return injected
				}
				if failure == "cancelled" {
					cancel()
				}
				return nil
			})
			if err == nil {
				t.Fatal("interrupted migration reported success")
			}
			wantPK := []string{"operation_id"}
			if failure == "commit_durable_response_lost" {
				wantPK = append(wantPK, "old_claim_id")
			}
			assertCorrectionMigrationPrimaryKey(t, db, wantPK)
			if got := correctionMigrationBytes(t, db); !reflect.DeepEqual(got, before) {
				t.Fatal("interruption changed historical bytes")
			}
			var temporary int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='semantic_claim_corrections_compound'`).Scan(&temporary); err != nil || temporary != 0 {
				t.Fatalf("migration temporary table leaked: %d %v", temporary, err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenDBAt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			assertCorrectionMigrationPrimaryKey(t, reopened, []string{"operation_id", "old_claim_id"})
			if got := correctionMigrationBytes(t, reopened); !reflect.DeepEqual(got, before) {
				t.Fatal("restart changed historical bytes")
			}
		})
	}
}

func TestCorrectionSchemaConcurrentAndRepeatedStartup(t *testing.T) {
	db, path := newStage3CorrectionMigrationFixture(t)
	before := correctionMigrationBytes(t, db)
	// Keep the operation-version upgrade out of this test's independent migration
	// boundary; corrections remain genuinely old when all startup calls race.
	if err := ensureSemanticOperationSchemaV6(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	const processes = 4
	start := make(chan struct{})
	failures := make(chan error, processes)
	var workers sync.WaitGroup
	for range processes {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			opened, err := OpenDBAt(path)
			if err == nil {
				err = opened.Close()
			}
			failures <- err
		}()
	}
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		reopened, err := OpenDBAt(path)
		if err != nil {
			t.Fatal(err)
		}
		assertCorrectionMigrationPrimaryKey(t, reopened, []string{"operation_id", "old_claim_id"})
		if got := correctionMigrationBytes(t, reopened); !reflect.DeepEqual(got, before) {
			t.Fatal("concurrent startup changed historical bytes")
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func correctionSchemaRelationalDB(t *testing.T, definition string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "constraints.db")+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// These are FK parents only. This test proves relational constraints;
	// canonical accepted operations and replay are covered separately.
	_, err = db.Exec(`
CREATE TABLE semantic_operations(operation_id TEXT PRIMARY KEY);
CREATE TABLE semantic_scopes(scope_id TEXT PRIMARY KEY);
CREATE TABLE semantic_claims(claim_id TEXT PRIMARY KEY);
INSERT INTO semantic_operations VALUES('operation-a'),('operation-b');
INSERT INTO semantic_scopes VALUES('scope');
INSERT INTO semantic_claims VALUES('old-a'),('new-a'),('old-b'),('new-b'),('old-c'),('new-c');
` + definition + semanticClaimCorrectionsAuxiliary)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

const correctionSchemaTestInsert = `INSERT INTO semantic_claim_corrections
(operation_id,scope_id,old_claim_id,replacement_claim_id,mode,effective_time,
old_valid_from,old_valid_to,old_effective_from,old_effective_to,replacement_from,
replacement_to,scope_revision,transaction_time) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`

func correctionSchemaTestRow(old, replacement string) []any {
	return []any{"operation-a", "scope", old, replacement, "error", nil, nil, nil, nil, nil, nil, nil, 1, "2026-09-02T12:00:00.000000123Z"}
}

func TestCorrectionSchemaCompoundRelationalConstraints(t *testing.T) {
	for _, shape := range []string{"fresh", "upgraded"} {
		t.Run(shape, func(t *testing.T) {
			definition := semanticClaimCorrectionsTable
			if shape == "upgraded" {
				definition = stage3CorrectionTable
			}
			db := correctionSchemaRelationalDB(t, definition)
			if err := runCorrectionSchemaMigration(context.Background(), db); err != nil {
				t.Fatal(err)
			}
			for _, row := range [][]any{correctionSchemaTestRow("old-a", "new-a"), correctionSchemaTestRow("old-b", "new-b")} {
				if _, err := db.Exec(correctionSchemaTestInsert, row...); err != nil {
					t.Fatalf("two distinct corrections in one operation: %v", err)
				}
			}
			cases := []struct {
				name   string
				mutate func([]any)
			}{
				{"duplicate_old", func(row []any) { row[0], row[2] = "operation-b", "old-a" }},
				{"duplicate_replacement", func(row []any) { row[0], row[3] = "operation-b", "new-a" }},
				{"missing_operation", func(row []any) { row[0] = "missing" }},
				{"missing_scope", func(row []any) { row[1] = "missing" }},
				{"missing_old", func(row []any) { row[2] = "missing" }},
				{"missing_replacement", func(row []any) { row[3] = "missing" }},
				{"null_operation", func(row []any) { row[0] = nil }},
				{"null_old", func(row []any) { row[2] = nil }},
				{"null_replacement", func(row []any) { row[3] = nil }},
				{"mode", func(row []any) { row[4] = "unknown" }},
				{"error_effective_time", func(row []any) { row[5] = "2026" }},
				{"changed_missing_time", func(row []any) { row[4] = "changed" }},
				{"old_valid_interval", func(row []any) { row[6], row[7] = "2026", "2025" }},
				{"old_effective_interval", func(row []any) { row[8], row[9] = "2026", "2026" }},
				{"replacement_interval", func(row []any) { row[10], row[11] = "2026", "2025" }},
				{"nonpositive_revision", func(row []any) { row[12] = 0 }},
				{"null_transaction_time", func(row []any) { row[13] = nil }},
			}
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					row := correctionSchemaTestRow("old-c", "new-c")
					test.mutate(row)
					if _, err := db.Exec(correctionSchemaTestInsert, row...); err == nil {
						t.Fatal("invalid correction accepted")
					}
				})
			}
			for _, statement := range []string{
				`UPDATE semantic_claim_corrections SET scope_revision=2`,
				`DELETE FROM semantic_claim_corrections`,
			} {
				if _, err := db.Exec(statement); err == nil || !strings.Contains(err.Error(), "append-only") {
					t.Fatalf("append-only constraint: %v", err)
				}
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM semantic_claim_corrections WHERE operation_id='operation-a'`).Scan(&count); err != nil || count != 2 {
				t.Fatalf("compound rows = %d, %v", count, err)
			}
			var indexColumns string
			if err := db.QueryRow(`SELECT group_concat(name,',') FROM (SELECT name FROM pragma_index_info('semantic_claim_corrections_scope_idx') ORDER BY seqno)`).Scan(&indexColumns); err != nil || indexColumns != "scope_id,transaction_time,scope_revision" {
				t.Fatalf("scope index = %q, %v", indexColumns, err)
			}
			var foreignKeys int
			if err := db.QueryRow(`SELECT foreign_keys FROM pragma_foreign_keys`).Scan(&foreignKeys); err != nil || foreignKeys != 1 {
				t.Fatalf("connection foreign keys = %d, %v", foreignKeys, err)
			}
		})
	}
}

func TestCorrectionSchemaRejectsIncompatibleUpgrade(t *testing.T) {
	for _, test := range []struct {
		name       string
		definition string
		alter      string
	}{
		{"unknown_column", stage3CorrectionTable, `ALTER TABLE semantic_claim_corrections ADD COLUMN future_data TEXT`},
		{"missing_unique", strings.Replace(stage3CorrectionTable, "old_claim_id          TEXT NOT NULL UNIQUE", "old_claim_id          TEXT NOT NULL", 1), ""},
		{"missing_check", strings.Replace(stage3CorrectionTable, "CHECK (scope_revision > 0)", "CHECK (scope_revision >= 0)", 1), ""},
		{"missing_foreign_key", strings.Replace(stage3CorrectionTable, "REFERENCES semantic_operations(operation_id)", "", 1), ""},
		{"missing_trigger", stage3CorrectionTable, `DROP TRIGGER semantic_claim_corrections_append_only_update`},
		{"changed_trigger", stage3CorrectionTable, `DROP TRIGGER semantic_claim_corrections_append_only_delete; CREATE TRIGGER semantic_claim_corrections_append_only_delete BEFORE DELETE ON semantic_claim_corrections BEGIN SELECT 1; END`},
		{"additional_index", stage3CorrectionTable, `CREATE INDEX custom_correction_index ON semantic_claim_corrections(mode)`},
		{"inbound_reference", stage3CorrectionTable, `CREATE TABLE future_correction_use(operation_id REFERENCES semantic_claim_corrections(operation_id))`},
		{"migration_name_collision", stage3CorrectionTable, `CREATE TABLE semantic_claim_corrections_compound(retain TEXT); INSERT INTO semantic_claim_corrections_compound VALUES('retain')`},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := correctionSchemaRelationalDB(t, test.definition)
			if _, err := db.Exec(correctionSchemaTestInsert, correctionSchemaTestRow("old-a", "new-a")...); err != nil {
				t.Fatal(err)
			}
			if test.alter != "" {
				if _, err := db.Exec(test.alter); err != nil {
					t.Fatal(err)
				}
			}
			var before string
			if err := db.QueryRow(`SELECT group_concat(sql,';') FROM (SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY name)`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err := ensureSemanticSchema(context.Background(), db); err == nil {
				t.Fatal("incompatible correction schema accepted")
			}
			var after string
			if err := db.QueryRow(`SELECT group_concat(sql,';') FROM (SELECT sql FROM sqlite_master WHERE sql IS NOT NULL ORDER BY name)`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatal("failed migration changed schema")
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM semantic_claim_corrections WHERE operation_id='operation-a' AND old_claim_id='old-a' AND replacement_claim_id='new-a'`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("failed migration lost correction: %d, %v", count, err)
			}
		})
	}
}

func TestCorrectionSchemaCopyFailurePreservesLegacyRows(t *testing.T) {
	db := correctionSchemaRelationalDB(t, stage3CorrectionTable)
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	row := correctionSchemaTestRow("old-a", "new-a")
	row[0] = "missing-operation"
	if _, err := conn.ExecContext(context.Background(), correctionSchemaTestInsert, row...); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := runCorrectionSchemaMigration(context.Background(), db); err == nil {
		t.Fatal("invalid foreign-key row migrated")
	}
	assertCorrectionMigrationPrimaryKey(t, db, []string{"operation_id"})
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM semantic_claim_corrections WHERE operation_id='missing-operation'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("copy failure lost old row: %d, %v", count, err)
	}
	if err := withImmediateTransaction(context.Background(), db, func(conn *sql.Conn) error {
		return validateSemanticCorrectionAuxiliary(context.Background(), conn)
	}); err != nil {
		t.Fatalf("copy failure lost original constraints: %v", err)
	}
}

func runCorrectionSchemaMigration(ctx context.Context, db *sql.DB) error {
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		return migrateSemanticCorrectionSchema(ctx, conn)
	})
}

func TestCorrectionSchemaConcurrentFreshBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	const processes = 6
	start := make(chan struct{})
	failures := make(chan error, processes)
	var workers sync.WaitGroup
	// Initialize each connection's WAL pragmas before racing the schema boundary.
	// Concurrent journal-mode activation is separate from the table migration.
	for range processes {
		db, err := sql.Open("sqlite", path+dsnPragmas)
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = db.Close() })
		if err := db.Ping(); err != nil {
			t.Fatal(err)
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			failures <- ensureSemanticBaseSchema(context.Background(), db)
		}()
	}
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertCorrectionMigrationPrimaryKey(t, db, []string{"operation_id", "old_claim_id"})
	if err := runCorrectionSchemaMigration(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}
