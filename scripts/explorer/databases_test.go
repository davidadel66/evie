package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureSchema stands in for a real database's migrations in the fixture
// module. It has a composite primary key declared out of column order, a
// foreign key to an implied primary key, AUTOINCREMENT (which adds the
// internal sqlite_sequence table), a full-text table (which adds shadow
// tables), and a view.
const fixtureSchema = `
CREATE TABLE owners (id INTEGER PRIMARY KEY, email TEXT NOT NULL UNIQUE);
CREATE TABLE widgets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	owner_id INTEGER REFERENCES owners
);
CREATE TABLE widget_tags (
	widget_id INTEGER NOT NULL REFERENCES widgets(id),
	tag TEXT NOT NULL,
	PRIMARY KEY (tag, widget_id)
);
CREATE VIRTUAL TABLE widget_search USING fts5(name);
CREATE VIEW widget_names AS SELECT name FROM widgets;
`

// fixtureDatabases are the fixture module's databases.
var fixtureDatabases = []databaseSource{{
	ID:      "fixture",
	Package: "internal/beta",
	Open: func(dir string) (*sql.DB, error) {
		db, err := sql.Open("sqlite", filepath.Join(dir, "fixture.db"))
		if err != nil {
			return nil, err
		}
		if _, err := db.Exec(fixtureSchema); err != nil {
			db.Close()
			return nil, err
		}
		return db, nil
	},
}}

func TestExtractDatabaseTablesFromTheMigratedSchema(t *testing.T) {
	root := newFixture(t)
	facts := decodeFacts(t, extractFixture(t, root))

	// Tables are sorted by name, columns keep their declared order, the
	// primary key position follows the key's own order, an implied foreign
	// key target is the parent's primary key, and SQLite-internal tables,
	// full-text shadow tables, and views are left out.
	want := `[{"id":"fixture","package":"internal/beta","tables":[` +
		`{"columns":[{"name":"id","notNull":false,"primaryKey":1,"type":"INTEGER"},{"name":"email","notNull":true,"primaryKey":0,"type":"TEXT"}],"foreignKeys":[],"name":"owners"},` +
		`{"columns":[{"name":"name","notNull":false,"primaryKey":0,"type":""}],"foreignKeys":[],"name":"widget_search"},` +
		`{"columns":[{"name":"widget_id","notNull":true,"primaryKey":2,"type":"INTEGER"},{"name":"tag","notNull":true,"primaryKey":1,"type":"TEXT"}],"foreignKeys":[{"columns":["widget_id"],"references":["id"],"table":"widgets"}],"name":"widget_tags"},` +
		`{"columns":[{"name":"id","notNull":false,"primaryKey":1,"type":"INTEGER"},{"name":"name","notNull":true,"primaryKey":0,"type":"TEXT"},{"name":"owner_id","notNull":false,"primaryKey":0,"type":"INTEGER"}],"foreignKeys":[{"columns":["owner_id"],"references":["id"],"table":"owners"}],"name":"widgets"}` +
		`]}]`
	if got := asJSON(t, facts["databases"]); got != want {
		t.Fatalf("databases =\n%s\nwant\n%s", got, want)
	}
}

// realDatabases extracts Evie's databases from the real repository.
func realDatabases(t *testing.T) map[string]Database {
	t.Helper()
	facts, err := extractFacts(realRoot, evieDatabases)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	byID := map[string]Database{}
	for _, db := range facts.Databases {
		byID[db.ID] = db
	}
	return byID
}

func table(t *testing.T, db Database, name string) (Table, bool) {
	t.Helper()
	for _, tb := range db.Tables {
		if tb.Name == name {
			return tb, true
		}
	}
	return Table{}, false
}

func TestExtractEvieDatabasesFromTheirRealMigrations(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbs := realDatabases(t)

	owners := map[string]string{}
	for id, db := range dbs {
		owners[id] = db.Package
	}
	if want := map[string]string{"evie": "internal/eviedb", "finance": "internal/finance", "youtube": "internal/youtube"}; !reflect.DeepEqual(owners, want) {
		t.Fatalf("databases = %v, want %v", owners, want)
	}
	for id, names := range map[string][]string{
		"evie":    {"workspaces", "workspace_tab_problems", "memory_session_episode_fts"},
		"finance": {"items", "transactions", "rules"},
		"youtube": {"channels", "videos", "transcripts"},
	} {
		for _, name := range names {
			if _, ok := table(t, dbs[id], name); !ok {
				t.Errorf("database %s has no table %s", id, name)
			}
		}
	}
	// AUTOINCREMENT's sqlite_sequence and the full-text index's shadow
	// tables are SQLite's, not Evie's.
	for _, name := range []string{"sqlite_sequence", "memory_session_episode_fts_data", "memory_session_episode_fts_config"} {
		if _, ok := table(t, dbs["evie"], name); ok {
			t.Errorf("database evie lists %s", name)
		}
	}
	problems, _ := table(t, dbs["evie"], "workspace_tab_problems")
	want := ForeignKey{Columns: []string{"workspace_id"}, Table: "workspaces", References: []string{"id"}}
	if !reflect.DeepEqual(problems.ForeignKeys, []ForeignKey{want}) {
		t.Errorf("workspace_tab_problems foreign keys = %+v, want %+v", problems.ForeignKeys, want)
	}
}

// snapshot records every file under dir with its content, mode, and
// modification time.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		entry := fmt.Sprintf("%v %v %d", info.Mode(), info.ModTime().UnixNano(), info.Size())
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry += " " + string(data)
		}
		files[path] = entry
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestExtractNeverOpensTheOwnersDatabases(t *testing.T) {
	// The owner's databases live under HOME. Each is a file that is not a
	// database, so opening and migrating it would fail extraction, and any
	// write, journal, or new file would change the snapshot.
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, owned := range []string{".evie/evie.db", ".finance/finance.db", ".evie/transcripts/transcripts.db"} {
		writeFile(t, home, owned, "the owner's data, not a database\n")
	}
	before := snapshot(t, home)

	if _, err := extractFacts(realRoot, evieDatabases); err != nil {
		t.Fatalf("extract: %v", err)
	}

	if after := snapshot(t, home); !reflect.DeepEqual(after, before) {
		t.Errorf("extraction touched the owner's home:\nbefore %v\nafter  %v", before, after)
	}
	if got := os.Getenv("HOME"); got != home {
		t.Errorf("HOME = %q after extraction, want it restored to %q", got, home)
	}
}

func TestExtractRefusesADatabaseOpenedOutsideItsTemporaryDirectory(t *testing.T) {
	root := newFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.db")
	stray := []databaseSource{{ID: "stray", Package: "internal/beta", Open: func(string) (*sql.DB, error) {
		db, err := sql.Open("sqlite", elsewhere)
		if err == nil {
			_, err = db.Exec(`CREATE TABLE t (x)`)
		}
		return db, err
	}}}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"extract"}, root, stray, &stdout, &stderr); code != 2 {
		t.Fatalf("extract exit %d, want 2:\n%s", code, stderr.String())
	}
	for _, want := range []string{"database stray", elsewhere, "not a file inside its temporary directory"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr lacks %q:\n%s", want, stderr.String())
		}
	}
}
