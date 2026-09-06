package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

func TestDatabaseHTTPExposesLiveSchemaAndBoundedRedactedRows(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	if _, err := db.ExecContext(ctx, `ALTER TABLE jobs ADD COLUMN secret_token TEXT`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs (name, schedule, command, created_at, secret_token) VALUES (?, ?, ?, ?, ?)`,
		"morning-review", "0 8 * * *", "evie review", "2026-09-04T12:00:00Z", "do-not-expose"); err != nil {
		t.Fatal(err)
	}

	handler := NewContextDataServer(nil, nil, nil, nil, store, store).Handler()
	schemaRecorder := httptest.NewRecorder()
	handler.ServeHTTP(schemaRecorder, managementRequest("/api/data/database/schema", `{}`))
	if schemaRecorder.Code != http.StatusOK {
		t.Fatalf("schema status=%d body=%s", schemaRecorder.Code, schemaRecorder.Body.String())
	}
	var schema struct {
		Tables []databaseHTTPTable `json:"tables"`
	}
	if err := json.Unmarshal(schemaRecorder.Body.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	events := findDatabaseTable(t, schema.Tables, "events")
	if events.RowAccess != "typed" || !databaseColumnIsRedacted(events.Columns, "content") || !databaseColumnIsRedacted(events.Columns, "payload_json") {
		t.Fatalf("events policy=%+v", events)
	}
	if !databaseForeignKeyExists(events.ForeignKeys, "session_id", "sessions", "id") {
		t.Fatalf("events foreign keys=%+v", events.ForeignKeys)
	}
	semantic := findDatabaseTable(t, schema.Tables, "semantic_claims")
	if semantic.RowAccess != "typed" {
		t.Fatalf("semantic_claims row_access=%q", semantic.RowAccess)
	}
	cursorAuth := findDatabaseTable(t, schema.Tables, "semantic_cursor_auth")
	if cursorAuth.RowAccess != "none" || !databaseColumnIsRedacted(cursorAuth.Columns, "hmac_key") {
		t.Fatalf("semantic_cursor_auth policy=%+v", cursorAuth)
	}
	jobs := findDatabaseTable(t, schema.Tables, "jobs")
	if jobs.ForeignKeys == nil || jobs.Indexes == nil {
		t.Fatalf("schema arrays must encode as [] instead of null: %+v", jobs)
	}

	rowsRecorder := httptest.NewRecorder()
	handler.ServeHTTP(rowsRecorder, managementRequest("/api/data/database/rows", `{"table":"jobs","pageSize":20}`))
	if rowsRecorder.Code != http.StatusOK {
		t.Fatalf("rows status=%d body=%s", rowsRecorder.Code, rowsRecorder.Body.String())
	}
	if !strings.Contains(rowsRecorder.Body.String(), "morning-review") || !strings.Contains(rowsRecorder.Body.String(), `"table":"jobs"`) ||
		strings.Contains(rowsRecorder.Body.String(), "do-not-expose") || !strings.Contains(rowsRecorder.Body.String(), `"kind":"redacted"`) {
		t.Fatalf("rows did not expose the allowlisted record: %s", rowsRecorder.Body.String())
	}

	for _, body := range []string{`{"table":"jobs","pageSize":51}`, `{"table":"jobs","offset":-1}`} {
		invalidRecorder := httptest.NewRecorder()
		handler.ServeHTTP(invalidRecorder, managementRequest("/api/data/database/rows", body))
		if invalidRecorder.Code != http.StatusBadRequest || !strings.Contains(invalidRecorder.Body.String(), "invalid_database_row_query") {
			t.Fatalf("invalid query body=%s status=%d response=%s", body, invalidRecorder.Code, invalidRecorder.Body.String())
		}
	}

	protectedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(protectedRecorder, managementRequest("/api/data/database/rows", `{"table":"events"}`))
	if protectedRecorder.Code != http.StatusForbidden || !strings.Contains(protectedRecorder.Body.String(), "database_table_typed_only") {
		t.Fatalf("protected status=%d body=%s", protectedRecorder.Code, protectedRecorder.Body.String())
	}

	injectionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(injectionRecorder, managementRequest("/api/data/database/rows", `{"table":"events; DROP TABLE events"}`))
	if injectionRecorder.Code != http.StatusBadRequest || !strings.Contains(injectionRecorder.Body.String(), "database_table_not_found") {
		t.Fatalf("injection status=%d body=%s", injectionRecorder.Code, injectionRecorder.Body.String())
	}
}

type databaseHTTPColumn struct {
	Name     string `json:"name"`
	Redacted bool   `json:"redacted"`
}

type databaseHTTPForeignKey struct {
	FromColumn string `json:"from_column"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
}

type databaseHTTPTable struct {
	Name        string                   `json:"name"`
	RowAccess   string                   `json:"row_access"`
	Columns     []databaseHTTPColumn     `json:"columns"`
	ForeignKeys []databaseHTTPForeignKey `json:"foreign_keys"`
	Indexes     []struct{}               `json:"indexes"`
}

func findDatabaseTable(t *testing.T, tables []databaseHTTPTable, name string) databaseHTTPTable {
	t.Helper()
	for _, table := range tables {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("table %q not found", name)
	return databaseHTTPTable{}
}

func databaseColumnIsRedacted(columns []databaseHTTPColumn, name string) bool {
	for _, column := range columns {
		if column.Name == name {
			return column.Redacted
		}
	}
	return false
}

func databaseForeignKeyExists(foreignKeys []databaseHTTPForeignKey, fromColumn, toTable, toColumn string) bool {
	for _, foreignKey := range foreignKeys {
		if foreignKey.FromColumn == fromColumn && foreignKey.ToTable == toTable && foreignKey.ToColumn == toColumn {
			return true
		}
	}
	return false
}
