package tools

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

const evieFenceSecret = "private-evie-secret-7731"

// newEvieFenceDB builds a real Evie database with one job, one run, and a
// private single-column table holding a recognizable secret — the shape that
// makes `x IN <table>` an exact-match oracle. It returns a read-only opener
// for query_db and a pointer counting how often query_db opened it.
func newEvieFenceDB(t *testing.T) (func(context.Context) (*sql.DB, error), *int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatalf("open evie db: %v", err)
	}
	defer db.Close()
	for _, statement := range []string{
		`INSERT INTO jobs (id, name, schedule, command, created_at) VALUES (1, 'nightly', '0 3 * * *', 'true', 'now')`,
		`INSERT INTO job_runs (job_id, started_at, finished_at, exit_code, output) VALUES (1, 'a', 'b', 0, 'ok')`,
		`CREATE TABLE private_secrets (token TEXT)`,
		`INSERT INTO private_secrets VALUES ('` + evieFenceSecret + `')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed %q: %v", statement, err)
		}
	}
	opens := 0
	return func(ctx context.Context) (*sql.DB, error) {
		opens++
		reader, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
		if err != nil {
			return nil, err
		}
		if err := reader.PingContext(ctx); err != nil {
			reader.Close()
			return nil, err
		}
		return reader, nil
	}, &opens
}

// evieTableOracleQueries name a private table outside FROM and JOIN. SQLite
// reads `x IN <table>` as `x IN (SELECT * FROM <table>)`, so each of these is
// an exact-match oracle on a table query_db must not read.
var evieTableOracleQueries = []string{
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN private_secrets`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' NOT IN private_secrets`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' in PRIVATE_SECRETS`,
	`SELECT '` + evieFenceSecret + `' IN private_secrets FROM jobs`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN "private_secrets"`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN 'private_secrets'`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN [private_secrets]`,
	"SELECT count(*) FROM jobs WHERE '" + evieFenceSecret + "' IN `private_secrets`",
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN main.private_secrets`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN main."private_secrets"`,
	`SELECT count(*) FROM jobs WHERE '` + evieFenceSecret + `' IN/**/private_secrets`,
	"SELECT count(*) FROM jobs WHERE '" + evieFenceSecret + "' IN -- comment\n private_secrets",
	`SELECT count(*) FROM jobs WHERE id IN (SELECT 1 WHERE '` + evieFenceSecret + `' IN private_secrets)`,
	`SELECT count(*) FROM jobs WHERE 'token' IN pragma_table_info('private_secrets')`,
	`SELECT count(*) FROM jobs WHERE 'sessions' IN sqlite_schema`,
}

// evieAllowedQueries read only jobs and job_runs, including IN lists and IN
// subqueries over the allowed tables.
var evieAllowedQueries = []string{
	`SELECT name FROM jobs`,
	`SELECT count(*) FROM jobs WHERE id IN (1, 2)`,
	`SELECT count(*) FROM jobs WHERE id NOT IN (SELECT job_id FROM job_runs WHERE exit_code <> 0)`,
	`SELECT j.name, max(r.id) FROM jobs j JOIN job_runs r ON r.job_id = j.id GROUP BY j.id ORDER BY j.name`,
	`SELECT (SELECT count(*) FROM job_runs WHERE job_id = jobs.id) FROM jobs`,
	`SELECT count(*) FROM jobs, job_runs WHERE job_runs.job_id = jobs.id AND job_runs.id IN (SELECT max(id) FROM job_runs GROUP BY job_id)`,
}

// Final pass: the evie table fence only checked names after FROM and JOIN, so
// a private table named after IN reached SQLite. The lexical fence now refuses
// it before the database is opened.
func TestQueryDBEvieRejectsTablesNamedAfterIn(t *testing.T) {
	open, opens := newEvieFenceDB(t)
	for _, query := range evieTableOracleQueries {
		out, err := queryDBWithEvieReader(context.Background(), queryDBArgs(t, "evie", query), open)
		if err == nil {
			t.Errorf("query_db accepted %q: %q", query, out)
			continue
		}
		if strings.Contains(out+err.Error(), evieFenceSecret) {
			t.Errorf("query_db leaked the secret for %q", query)
		}
	}
	if *opens != 0 {
		t.Fatalf("protected queries reached the SQLite opener %d times", *opens)
	}

	for _, query := range evieAllowedQueries {
		if _, err := queryDBWithEvieReader(context.Background(), queryDBArgs(t, "evie", query), open); err != nil {
			t.Errorf("allowed evie query %q failed: %v", query, err)
		}
	}
}

// The engine-level half: whatever the lexical fence misses, SQLite's own
// compiled program for the statement names every table it opens, and only
// jobs and job_runs may appear there.
func TestEvieReadFenceJudgesTheCompiledStatement(t *testing.T) {
	open, _ := newEvieFenceDB(t)
	db, err := open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	refused := append([]string{
		`SELECT token FROM private_secrets`,
		`SELECT id FROM sessions`,
		`SELECT name FROM sqlite_schema`,
		`SELECT name FROM jobs WHERE id IN (SELECT rowid FROM sessions)`,
		`SELECT key FROM json_each('[1]')`,
	}, evieTableOracleQueries...)
	for _, query := range refused {
		err := refuseUnlistedEvieReads(context.Background(), conn, query)
		if err == nil {
			t.Errorf("engine fence allowed %q", query)
			continue
		}
		if strings.Contains(err.Error(), evieFenceSecret) {
			t.Errorf("engine fence error leaked the secret for %q", query)
		}
		// A refusal only because EXPLAIN failed must be a statement SQLite
		// cannot run at all, not a readable one the fence failed to judge.
		if strings.HasPrefix(err.Error(), "run query:") {
			if rows, runErr := conn.QueryContext(context.Background(), query); runErr == nil {
				rows.Close()
				t.Errorf("engine fence refused %q only by compile error, but SQLite runs it", query)
			}
		}
	}
	allowed := append([]string{
		`WITH latest AS (SELECT job_id, max(id) AS id FROM job_runs GROUP BY job_id) SELECT j.name FROM jobs j JOIN latest l ON l.job_id = j.id`,
		`SELECT name FROM jobs WHERE name = 'nightly' ORDER BY name`,
	}, evieAllowedQueries...)
	for _, query := range allowed {
		if err := refuseUnlistedEvieReads(context.Background(), conn, query); err != nil {
			t.Errorf("engine fence refused allowed query %q: %v", query, err)
		}
	}
}
