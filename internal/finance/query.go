package finance

import (
	"database/sql"
	"fmt"
)

// Query runs one read-only SELECT and returns a generic table: column
// names plus rows of stringified values. Shape is discovered at
// runtime because callers write free-form SQL — aggregates and joins
// don't map to any fixed struct.
func Query(db *sql.DB, query string) (columns []string, rows [][]string, err error) {
	res, err := db.Query(query)
	if err != nil {
		return nil, nil, fmt.Errorf("run query: %w", err)
	}

	defer res.Close()

	columns, err = res.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("read columns: %w", err)
	}

	for res.Next() {
		vals := make([]any, len(columns))
		for i := range vals {
			vals[i] = new(sql.NullString)
		}

		if err := res.Scan(vals...); err != nil {
			return nil, nil, fmt.Errorf("scan row: %w", err)
		}

		row := make([]string, len(columns))
		for i, v := range vals {
			ns := v.(*sql.NullString)
			if ns.Valid {
				row[i] = ns.String
			} else {
				row[i] = "NULL"
			}
		}
		rows = append(rows, row)
	}

	return columns, rows, res.Err()
}
