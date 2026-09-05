package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	DatabaseRowAccessRecords = "records"
	DatabaseRowAccessTyped   = "typed"
	DatabaseRowAccessNone    = "none"

	defaultDatabasePageSize = 20
	maxDatabasePageSize     = 50
	maxDatabaseCellRunes    = 300
)

var (
	ErrDatabaseTableNotFound    = errors.New("database table not found")
	ErrDatabaseTableTypedOnly   = errors.New("database table requires a typed view")
	ErrDatabaseTableUnavailable = errors.New("database table records are unavailable")
)

// DatabaseSchema is a read-only description of Evie's physical SQLite model.
// It intentionally omits raw DDL, triggers, default expressions, and values.
type DatabaseSchema struct {
	Tables []DatabaseTable `json:"tables"`
}

type DatabaseTable struct {
	Name        string               `json:"name"`
	Kind        string               `json:"kind"`
	RowAccess   string               `json:"row_access"`
	Columns     []DatabaseColumn     `json:"columns"`
	ForeignKeys []DatabaseForeignKey `json:"foreign_keys"`
	Indexes     []DatabaseIndex      `json:"indexes"`
}

type DatabaseColumn struct {
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primary_key"`
	Hidden     bool   `json:"hidden,omitempty"`
	Redacted   bool   `json:"redacted,omitempty"`
}

type DatabaseForeignKey struct {
	ID         int    `json:"id"`
	Sequence   int    `json:"sequence"`
	FromColumn string `json:"from_column"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
	OnUpdate   string `json:"on_update"`
	OnDelete   string `json:"on_delete"`
}

type DatabaseIndex struct {
	Name    string   `json:"name"`
	Unique  bool     `json:"unique"`
	Origin  string   `json:"origin"`
	Partial bool     `json:"partial"`
	Columns []string `json:"columns"`
}

type DatabaseRowsQuery struct {
	Table    string
	PageSize int
	Offset   int
}

type DatabaseRows struct {
	Table      string           `json:"table"`
	Columns    []DatabaseColumn `json:"columns"`
	Rows       [][]DatabaseCell `json:"rows"`
	Offset     int              `json:"offset"`
	PageSize   int              `json:"page_size"`
	TotalRows  int              `json:"total_rows"`
	NextOffset *int             `json:"next_offset,omitempty"`
}

type DatabaseCell struct {
	Kind      string  `json:"kind"`
	Value     *string `json:"value"`
	Redacted  bool    `json:"redacted,omitempty"`
	Truncated bool    `json:"truncated,omitempty"`
}

// InspectDatabaseSchema exposes topology only. Record access is a server-owned
// policy on each table, not a permission selected by the caller.
func (s *Store) InspectDatabaseSchema(ctx context.Context) (DatabaseSchema, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, type
		FROM sqlite_schema
		WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%'
		ORDER BY name
	`)
	if err != nil {
		return DatabaseSchema{}, fmt.Errorf("list database tables: %w", err)
	}
	defer rows.Close()

	tables := make([]DatabaseTable, 0)
	for rows.Next() {
		var table DatabaseTable
		if err := rows.Scan(&table.Name, &table.Kind); err != nil {
			return DatabaseSchema{}, fmt.Errorf("scan database table: %w", err)
		}
		table.RowAccess = databaseRowAccess(table.Name)
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return DatabaseSchema{}, fmt.Errorf("read database tables: %w", err)
	}

	for index := range tables {
		table := &tables[index]
		table.Columns, err = s.inspectDatabaseColumns(ctx, table.Name)
		if err != nil {
			return DatabaseSchema{}, err
		}
		table.ForeignKeys, err = s.inspectDatabaseForeignKeys(ctx, table.Name)
		if err != nil {
			return DatabaseSchema{}, err
		}
		table.Indexes, err = s.inspectDatabaseIndexes(ctx, table.Name)
		if err != nil {
			return DatabaseSchema{}, err
		}
	}
	return DatabaseSchema{Tables: tables}, nil
}

func (s *Store) InspectDatabaseRows(ctx context.Context, query DatabaseRowsQuery) (DatabaseRows, error) {
	tableName := strings.TrimSpace(query.Table)
	schema, err := s.InspectDatabaseSchema(ctx)
	if err != nil {
		return DatabaseRows{}, err
	}
	var table *DatabaseTable
	for index := range schema.Tables {
		if schema.Tables[index].Name == tableName {
			table = &schema.Tables[index]
			break
		}
	}
	if table == nil {
		return DatabaseRows{}, ErrDatabaseTableNotFound
	}
	switch table.RowAccess {
	case DatabaseRowAccessTyped:
		return DatabaseRows{}, ErrDatabaseTableTypedOnly
	case DatabaseRowAccessNone:
		return DatabaseRows{}, ErrDatabaseTableUnavailable
	}

	pageSize := query.PageSize
	if pageSize == 0 {
		pageSize = defaultDatabasePageSize
	}
	if pageSize < 1 || pageSize > maxDatabasePageSize || query.Offset < 0 {
		return DatabaseRows{}, fmt.Errorf("invalid database page: size=%d offset=%d", pageSize, query.Offset)
	}

	identifier := quoteDatabaseIdentifier(table.Name)
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+identifier).Scan(&total); err != nil {
		return DatabaseRows{}, fmt.Errorf("count database rows: %w", err)
	}
	columnNames := make([]string, len(table.Columns))
	selectColumns := make([]string, len(table.Columns))
	var orderColumns []string
	for index, column := range table.Columns {
		columnNames[index] = column.Name
		selectColumns[index] = quoteDatabaseIdentifier(column.Name)
		if column.PrimaryKey {
			orderColumns = append(orderColumns, quoteDatabaseIdentifier(column.Name))
		}
	}
	if len(orderColumns) == 0 {
		orderColumns = append(orderColumns, selectColumns...)
	}
	statement := `SELECT ` + strings.Join(selectColumns, ", ") + ` FROM ` + identifier +
		` ORDER BY ` + strings.Join(orderColumns, ", ") + ` LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, statement, pageSize, query.Offset)
	if err != nil {
		return DatabaseRows{}, fmt.Errorf("read database rows: %w", err)
	}
	defer rows.Close()

	result := DatabaseRows{
		Table: table.Name, Columns: table.Columns, Offset: query.Offset,
		PageSize: pageSize, TotalRows: total, Rows: make([][]DatabaseCell, 0),
	}
	for rows.Next() {
		values := make([]any, len(columnNames))
		destinations := make([]any, len(values))
		for index := range values {
			destinations[index] = &values[index]
		}
		if err := rows.Scan(destinations...); err != nil {
			return DatabaseRows{}, fmt.Errorf("scan database row: %w", err)
		}
		cells := make([]DatabaseCell, len(values))
		for index, value := range values {
			cells[index] = databaseCell(value, table.Columns[index].Redacted)
		}
		result.Rows = append(result.Rows, cells)
	}
	if err := rows.Err(); err != nil {
		return DatabaseRows{}, fmt.Errorf("iterate database rows: %w", err)
	}
	if next := query.Offset + len(result.Rows); next < total {
		result.NextOffset = &next
	}
	return result, nil
}

func (s *Store) inspectDatabaseColumns(ctx context.Context, table string) ([]DatabaseColumn, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, type, "notnull", pk, hidden FROM pragma_table_xinfo(?) ORDER BY cid`, table)
	if err != nil {
		return nil, fmt.Errorf("inspect database columns: %w", err)
	}
	defer rows.Close()
	columns := make([]DatabaseColumn, 0)
	for rows.Next() {
		var column DatabaseColumn
		var notNull, primaryKey, hidden int
		if err := rows.Scan(&column.Name, &column.DataType, &notNull, &primaryKey, &hidden); err != nil {
			return nil, fmt.Errorf("scan database column: %w", err)
		}
		column.Nullable = notNull == 0 && primaryKey == 0
		column.PrimaryKey = primaryKey > 0
		column.Hidden = hidden != 0
		column.Redacted = databaseColumnRedacted(table, column.Name)
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read database columns: %w", err)
	}
	return columns, nil
}

func (s *Store) inspectDatabaseForeignKeys(ctx context.Context, table string) ([]DatabaseForeignKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, seq, "from", "table", COALESCE("to", ''), on_update, on_delete
		FROM pragma_foreign_key_list(?) ORDER BY id, seq
	`, table)
	if err != nil {
		return nil, fmt.Errorf("inspect database foreign keys: %w", err)
	}
	defer rows.Close()
	foreignKeys := make([]DatabaseForeignKey, 0)
	for rows.Next() {
		var foreignKey DatabaseForeignKey
		if err := rows.Scan(&foreignKey.ID, &foreignKey.Sequence, &foreignKey.FromColumn, &foreignKey.ToTable,
			&foreignKey.ToColumn, &foreignKey.OnUpdate, &foreignKey.OnDelete); err != nil {
			return nil, fmt.Errorf("scan database foreign key: %w", err)
		}
		foreignKeys = append(foreignKeys, foreignKey)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read database foreign keys: %w", err)
	}
	return foreignKeys, nil
}

func (s *Store) inspectDatabaseIndexes(ctx context.Context, table string) ([]DatabaseIndex, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, "unique", origin, partial FROM pragma_index_list(?) ORDER BY name`, table)
	if err != nil {
		return nil, fmt.Errorf("inspect database indexes: %w", err)
	}
	defer rows.Close()
	indexes := make([]DatabaseIndex, 0)
	for rows.Next() {
		var index DatabaseIndex
		var unique, partial int
		if err := rows.Scan(&index.Name, &unique, &index.Origin, &partial); err != nil {
			return nil, fmt.Errorf("scan database index: %w", err)
		}
		index.Unique = unique != 0
		index.Partial = partial != 0
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read database indexes: %w", err)
	}
	for index := range indexes {
		indexes[index].Columns = make([]string, 0)
		columnRows, err := s.db.QueryContext(ctx, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, indexes[index].Name)
		if err != nil {
			return nil, fmt.Errorf("inspect database index columns: %w", err)
		}
		for columnRows.Next() {
			var name sql.NullString
			if err := columnRows.Scan(&name); err != nil {
				columnRows.Close()
				return nil, fmt.Errorf("scan database index column: %w", err)
			}
			if name.Valid {
				indexes[index].Columns = append(indexes[index].Columns, name.String)
			}
		}
		if err := columnRows.Err(); err != nil {
			columnRows.Close()
			return nil, fmt.Errorf("read database index columns: %w", err)
		}
		if err := columnRows.Close(); err != nil {
			return nil, fmt.Errorf("read database index columns: %w", err)
		}
	}
	return indexes, nil
}

func databaseRowAccess(table string) string {
	switch table {
	case "jobs", "job_runs":
		return DatabaseRowAccessRecords
	case "events":
		return DatabaseRowAccessTyped
	case "semantic_cursor_auth":
		return DatabaseRowAccessNone
	default:
		if strings.HasPrefix(table, "semantic_") {
			return DatabaseRowAccessTyped
		}
		return DatabaseRowAccessNone
	}
}

func databaseColumnRedacted(table, column string) bool {
	if table == "events" && (column == "content" || column == "payload_json") {
		return true
	}
	lower := strings.ToLower(column)
	for _, fragment := range []string{"password", "secret", "credential", "authorization", "hmac", "key_material"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func databaseCell(value any, redacted bool) DatabaseCell {
	if redacted {
		return DatabaseCell{Kind: "redacted", Redacted: true}
	}
	if value == nil {
		return DatabaseCell{Kind: "null"}
	}
	var kind, rendered string
	switch typed := value.(type) {
	case int64:
		kind, rendered = "integer", fmt.Sprint(typed)
	case float64:
		kind, rendered = "real", fmt.Sprint(typed)
	case bool:
		kind, rendered = "boolean", fmt.Sprint(typed)
	case []byte:
		if utf8.Valid(typed) {
			kind, rendered = "text", string(typed)
		} else {
			kind, rendered = "blob", fmt.Sprintf("%d bytes", len(typed))
		}
	default:
		kind, rendered = "text", fmt.Sprint(typed)
	}
	runes := []rune(rendered)
	truncated := len(runes) > maxDatabaseCellRunes
	if truncated {
		rendered = string(runes[:maxDatabaseCellRunes]) + "…"
	}
	return DatabaseCell{Kind: kind, Value: &rendered, Truncated: truncated}
}

func quoteDatabaseIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
