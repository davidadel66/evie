package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite" // the pure-Go driver Evie's databases use
)

// databaseSource is one database and the way to create it: Open runs the
// database's real open-and-migrate entry point inside dir, a fresh
// temporary directory, and returns the migrated database.
type databaseSource struct {
	ID      string
	Package string // the package that owns the database
	Open    func(dir string) (*sql.DB, error)
}

// extractDatabases creates every database on a throwaway file and reads its
// schema, sorted by id. Each source runs with HOME pointed at its own
// temporary directory, so no entry point can reach the owner's databases,
// and a database opened outside that directory fails extraction.
func extractDatabases(sources []databaseSource, isPackage map[string]bool) ([]Database, error) {
	databases := []Database{}
	for _, src := range sources {
		if !isPackage[src.Package] {
			return nil, fmt.Errorf("database %s: its owner %s is not a package", src.ID, src.Package)
		}
		db, err := readDatabase(src)
		if err != nil {
			return nil, fmt.Errorf("database %s: %w", src.ID, err)
		}
		databases = append(databases, db)
	}
	sort.Slice(databases, func(i, j int) bool { return databases[i].ID < databases[j].ID })
	return databases, nil
}

func readDatabase(src databaseSource) (Database, error) {
	dir, err := os.MkdirTemp("", "explorer-"+src.ID+"-")
	if err != nil {
		return Database{}, err
	}
	defer os.RemoveAll(dir)
	db, err := openWithHome(src, dir)
	if err != nil {
		return Database{}, err
	}
	defer db.Close()
	if err := insideDir(db, dir); err != nil {
		return Database{}, err
	}
	tables, err := readTables(db)
	if err != nil {
		return Database{}, err
	}
	return Database{ID: src.ID, Package: src.Package, Tables: tables}, nil
}

// openWithHome runs the source's entry point with HOME set to dir, and puts
// HOME back afterwards.
func openWithHome(src databaseSource, dir string) (*sql.DB, error) {
	home, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("HOME", dir); err != nil {
		return nil, err
	}
	defer func() {
		if hadHome {
			os.Setenv("HOME", home)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	return src.Open(dir)
}

// insideDir checks that the database's main file lies inside dir.
func insideDir(db *sql.DB, dir string) error {
	rows, err := db.Query(`PRAGMA database_list`)
	if err != nil {
		return err
	}
	defer rows.Close()
	file := ""
	for rows.Next() {
		var seq int
		var name, path string
		if err := rows.Scan(&seq, &name, &path); err != nil {
			return err
		}
		if name == "main" {
			file = path
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	realFile, err := filepath.EvalSymlinks(file)
	if file == "" || err != nil || !strings.HasPrefix(realFile, realDir+string(filepath.Separator)) {
		return fmt.Errorf("the entry point opened %q, which is not a file inside its temporary directory", file)
	}
	return nil
}

// readTables reads every table of the main schema, excluding SQLite's
// internal tables, the shadow tables of virtual tables, and views.
func readTables(db *sql.DB) ([]Table, error) {
	rows, err := db.Query(`SELECT name, type FROM pragma_table_list WHERE schema = 'main'`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if (kind == "table" || kind == "virtual") && !strings.HasPrefix(name, "sqlite_") {
			names = append(names, name)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(names)
	tables := make([]Table, 0, len(names))
	for _, name := range names {
		columns, err := readColumns(db, name)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", name, err)
		}
		keys, err := readForeignKeys(db, name)
		if err != nil {
			return nil, fmt.Errorf("table %s: %w", name, err)
		}
		tables = append(tables, Table{Name: name, Columns: columns, ForeignKeys: keys})
	}
	return tables, nil
}

// readColumns returns a table's columns in declared order.
func readColumns(db *sql.DB, table string) ([]Column, error) {
	rows, err := db.Query(`SELECT name, type, "notnull", pk FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := []Column{}
	for rows.Next() {
		var c Column
		if err := rows.Scan(&c.Name, &c.Type, &c.NotNull, &c.PrimaryKey); err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	return columns, rows.Err()
}

// readForeignKeys returns a table's foreign keys, sorted. A key that names
// no parent columns references the parent's primary key.
func readForeignKeys(db *sql.DB, table string) ([]ForeignKey, error) {
	rows, err := db.Query(`SELECT id, "table", "from", "to" FROM pragma_foreign_key_list(?) ORDER BY id, seq`, table)
	if err != nil {
		return nil, err
	}
	byID := map[int]*ForeignKey{}
	var ids []int
	for rows.Next() {
		var id int
		var parent, from string
		var to sql.NullString
		if err := rows.Scan(&id, &parent, &from, &to); err != nil {
			rows.Close()
			return nil, err
		}
		key, ok := byID[id]
		if !ok {
			key = &ForeignKey{Table: parent, Columns: []string{}, References: []string{}}
			byID[id] = key
			ids = append(ids, id)
		}
		key.Columns = append(key.Columns, from)
		if to.Valid {
			key.References = append(key.References, to.String)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := []ForeignKey{}
	for _, id := range ids {
		key := byID[id]
		if len(key.References) == 0 {
			pk, err := primaryKey(db, key.Table)
			if err != nil {
				return nil, err
			}
			key.References = pk
		}
		keys = append(keys, *key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.Table != b.Table {
			return a.Table < b.Table
		}
		if ac, bc := strings.Join(a.Columns, ","), strings.Join(b.Columns, ","); ac != bc {
			return ac < bc
		}
		return strings.Join(a.References, ",") < strings.Join(b.References, ",")
	})
	return keys, nil
}

// primaryKey returns a table's primary key columns in key order.
func primaryKey(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}
