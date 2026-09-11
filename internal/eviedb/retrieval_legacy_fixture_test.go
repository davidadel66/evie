package eviedb

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Current public write APIs maintain derived retrieval projections. Historical
// migration fixtures must remove those later objects before reconstructing or
// reopening an older schema, without changing any canonical operation or event.
func removeRetrievalSchemaFromLegacyFixture(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	before := correctionMigrationBytes(t, db)
	rows, err := db.QueryContext(ctx, `SELECT type,name FROM sqlite_schema WHERE (name GLOB 'memory_retrieval_*' OR name GLOB 'memory_dense_*') AND type IN ('trigger','table') ORDER BY CASE type WHEN 'trigger' THEN 0 ELSE 1 END,name`)
	if err != nil {
		t.Fatal(err)
	}
	type object struct{ kind, name string }
	var objects []object
	for rows.Next() {
		var item object
		if err = rows.Scan(&item.kind, &item.name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		objects = append(objects, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range objects {
		// Names come from SQLite's schema, but still quote identifiers exactly. FTS
		// removal also removes its shadow tables; IF EXISTS makes those rows harmless.
		statement := fmt.Sprintf(`DROP %s IF EXISTS "%s"`, strings.ToUpper(item.kind), strings.ReplaceAll(item.name, `"`, `""`))
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	var remaining int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_schema WHERE name GLOB 'memory_retrieval_*' OR name GLOB 'memory_dense_*'`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("historical fixture retains %d later retrieval objects: %v", remaining, err)
	}
	if after := correctionMigrationBytes(t, db); !reflect.DeepEqual(before, after) {
		t.Fatal("removing derived retrieval schema changed canonical memory or original conversation bytes")
	}
}
