package eviedb

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Opening the database checks foreign keys in the semantic_* tables only. It
// must find exactly the failures an unscoped check finds there, and none of a
// broken key elsewhere.
func TestSemanticForeignKeyCheckFindsWhatTheWholeDatabaseCheckFinds(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claim := rememberScopeClaim(t, ctx, store, session, false, 951)

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`PRAGMA foreign_keys = OFF`,
		`DROP TRIGGER semantic_claims_append_only_update`,
		`UPDATE semantic_claims SET scope_id = 'ffffffff-ffff-4fff-8fff-ffffffffffff' WHERE claim_id = '` + string(claim.Claim.ID) + `'`,
		semanticProjectionAppendOnlyTriggerSQL(),
		`CREATE TABLE unrelated_child(session_id TEXT REFERENCES sessions(id))`,
		`INSERT INTO unrelated_child VALUES ('no-such-session')`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	conn.Close()

	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	var whole, unrelated []string
	for rows.Next() {
		var failure semanticForeignKeyFailure
		if err := rows.Scan(&failure.Table, &failure.RowID, &failure.Parent, &failure.ForeignID); err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(failure.Table, "semantic_") {
			whole = append(whole, failure.canonical())
		} else {
			unrelated = append(unrelated, failure.canonical())
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(whole) == 0 || len(unrelated) == 0 {
		t.Fatalf("fixture needs broken keys inside and outside semantic tables: semantic %v, other %v", whole, unrelated)
	}

	failures, err := loadSemanticForeignKeyFailures(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var scoped []string
	for _, failure := range failures {
		scoped = append(scoped, failure.canonical())
	}
	slices.Sort(whole)
	slices.Sort(scoped)
	if !slices.Equal(scoped, whole) {
		t.Fatalf("semantic check found %v, whole-database check found %v", scoped, whole)
	}
}
