package tools

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/finance"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	// MaxReadRows and MaxReadBytes bound one ReadDB result.
	MaxReadRows  = 5000
	MaxReadBytes = 2 << 20
)

// ReadDBDatabases are the registered databases ReadDB serves. Transcripts
// stay with query_db, which labels their untrusted output.
var ReadDBDatabases = []string{"finance", "evie"}

// DBRows is a ReadDB result: values keep their SQLite type (integer, real,
// text, or nil for NULL); blobs become text.
type DBRows struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated,omitempty"`
}

// ReadDB runs one read-only SELECT against a registered database for a
// Workspace tab (workspace-tabs.spec.md, story 2), behind exactly query_db's
// fences (queryDB): the single-SELECT check and engine-level mode=ro for
// both; for finance no items table and a connection that cannot ATTACH; for
// evie only its listed tables, judged on the compiled program.
func ReadDB(ctx context.Context, name, query string) (DBRows, error) {
	q := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(q), "SELECT") {
		return DBRows{}, fmt.Errorf("only SELECT statements are allowed")
	}
	var open func(context.Context) (*sql.DB, error)
	switch name {
	case "finance":
		if err := validateSingleSelect(q, "finance"); err != nil {
			return DBRows{}, err
		}
		if strings.Contains(strings.ToLower(q), "items") {
			return DBRows{}, fmt.Errorf("the items table is off-limits")
		}
		open = finance.OpenDBReadOnlyContext
	case "evie":
		if err := validateEvieSelect(q); err != nil {
			return DBRows{}, err
		}
		open = eviedb.OpenDBReadOnlyContext
	default:
		return DBRows{}, fmt.Errorf("unknown db %q — tabs can read: %s", name, strings.Join(ReadDBDatabases, ", "))
	}
	db, err := open(ctx)
	if err != nil {
		return DBRows{}, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return DBRows{}, fmt.Errorf("open connection: %w", err)
	}
	defer conn.Close()
	switch name {
	case "finance":
		if _, err := sqlite.Limit(conn, sqlite3.SQLITE_LIMIT_ATTACHED, 0); err != nil {
			return DBRows{}, fmt.Errorf("fence connection: %w", err)
		}
	case "evie":
		if err := refuseUnlistedEvieReads(ctx, conn, q); err != nil {
			return DBRows{}, err
		}
	}
	return readRows(ctx, conn, q)
}

// readRows reads at most MaxReadRows rows and MaxReadBytes of values.
func readRows(ctx context.Context, conn *sql.Conn, query string) (DBRows, error) {
	result, err := conn.QueryContext(ctx, query)
	if err != nil {
		return DBRows{}, fmt.Errorf("run query: %w", err)
	}
	defer result.Close()
	columns, err := result.Columns()
	if err != nil {
		return DBRows{}, fmt.Errorf("read columns: %w", err)
	}
	out := DBRows{Columns: columns, Rows: [][]any{}}
	size := 0
	for result.Next() {
		if len(out.Rows) == MaxReadRows {
			out.Truncated = true
			break
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := result.Scan(pointers...); err != nil {
			return DBRows{}, fmt.Errorf("scan row: %w", err)
		}
		for i, value := range values {
			switch typed := value.(type) {
			case []byte:
				values[i] = string(typed)
				size += len(typed)
			case string:
				size += len(typed)
			default:
				size += 8
			}
		}
		if size > MaxReadBytes {
			out.Truncated = true
			break
		}
		out.Rows = append(out.Rows, values)
	}
	return out, result.Err()
}
