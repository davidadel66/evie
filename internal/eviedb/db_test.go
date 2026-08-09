package eviedb

// Tests for the eviedb package, written from
// cmd/evie/docs/active/cron.spec.md before the implementation exists
// (red -> green). Contract under test — mirrors internal/finance/db.go
// function for function, plus busy_timeout on both open paths:
//
//	func OpenDB() (*sql.DB, error)
//	func OpenDBReadOnly() (*sql.DB, error)
//	func OpenDBAt(path string) (*sql.DB, error)
//
// OpenDB/OpenDBReadOnly point at the real ~/.evie/evie.db, so the
// tests never call them — the signatures are pinned at compile time
// below and all behavior goes through the OpenDBAt temp-path seam, the
// same way finance/sync_test.go does.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Pin the exported signatures without touching the home directory.
var (
	_ func() (*sql.DB, error) = OpenDB
	_ func() (*sql.DB, error) = OpenDBReadOnly
)

// newTestDB opens a fresh temp database and registers cleanup.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatalf("OpenDBAt: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// insertJob adds a minimal jobs row and returns its id.
func insertJob(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, ?, ?, ?)`,
		name, "0 9 * * *", "echo hi", "2026-08-05T09:00:00-04:00",
	)
	if err != nil {
		t.Fatalf("insert job %q: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last insert id: %v", err)
	}
	return id
}

// Spec: one CREATE IF NOT EXISTS DDL blob applied on every open — both
// tables must exist and accept rows immediately after the first open.
func TestOpenDBAtCreatesSchema(t *testing.T) {
	db := newTestDB(t)

	jobID := insertJob(t, db, "schema-check")

	if _, err := db.Exec(
		`INSERT INTO job_runs (job_id, started_at, finished_at, exit_code, output)
		 VALUES (?, ?, ?, ?, ?)`,
		jobID, "2026-08-05T09:00:00-04:00", "2026-08-05T09:00:05-04:00", 0, "hello",
	); err != nil {
		t.Fatalf("insert job_runs row: %v", err)
	}
}

// Spec: `enabled INTEGER NOT NULL DEFAULT 1` — v1 never writes it, so
// the default is what every row gets.
func TestJobsEnabledDefaultsToOne(t *testing.T) {
	db := newTestDB(t)
	id := insertJob(t, db, "enabled-default")

	var enabled int64
	if err := db.QueryRow(`SELECT enabled FROM jobs WHERE id = ?`, id).Scan(&enabled); err != nil {
		t.Fatalf("select enabled: %v", err)
	}
	if enabled != 1 {
		t.Errorf("enabled = %d, want 1 by default", enabled)
	}
}

// Spec: no migrations — reopening an existing database is a no-op and
// the data survives.
func TestOpenDBAtIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")

	db1, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("first OpenDBAt: %v", err)
	}
	if _, err := db1.Exec(
		`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, ?, ?, ?)`,
		"survivor", "* * * * *", "true", "2026-08-05T09:00:00-04:00",
	); err != nil {
		t.Fatalf("insert into first open: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close first open: %v", err)
	}

	db2, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("second OpenDBAt on an existing db: %v", err)
	}
	t.Cleanup(func() { db2.Close() })

	var n int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM jobs WHERE name = ?`, "survivor").Scan(&n); err != nil {
		t.Fatalf("count after reopen: %v", err)
	}
	if n != 1 {
		t.Errorf("got %d survivor rows after reopen, want 1", n)
	}
}

// Spec: 0o600 file, mirroring finance (state should not be world-readable).
func TestOpenDBAtFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("OpenDBAt: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat db file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("db file mode = %o, want 600", got)
	}
}

// A job and a run round-trip through the schema with every column intact,
// including the -1 "did not complete" exit code.
func TestJobAndRunRoundTrip(t *testing.T) {
	db := newTestDB(t)

	if _, err := db.Exec(
		`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, ?, ?, ?)`,
		"finance-daily", "0 9 * * *", "finance sync && finance categorize", "2026-08-05T09:00:00-04:00",
	); err != nil {
		t.Fatalf("insert job: %v", err)
	}

	var (
		id                           int64
		schedule, command, createdAt string
	)
	if err := db.QueryRow(
		`SELECT id, schedule, command, created_at FROM jobs WHERE name = ?`, "finance-daily",
	).Scan(&id, &schedule, &command, &createdAt); err != nil {
		t.Fatalf("select job: %v", err)
	}
	if schedule != "0 9 * * *" {
		t.Errorf("schedule = %q, want it stored verbatim", schedule)
	}
	if command != "finance sync && finance categorize" {
		t.Errorf("command = %q round-tripped wrong", command)
	}
	if _, err := time.Parse(time.RFC3339, createdAt); err != nil {
		t.Errorf("created_at %q is not RFC3339: %v", createdAt, err)
	}

	if _, err := db.Exec(
		`INSERT INTO job_runs (job_id, started_at, finished_at, exit_code, output)
		 VALUES (?, ?, ?, ?, ?)`,
		id, "2026-08-05T09:00:00-04:00", "2026-08-05T09:30:00-04:00", -1, "[killed: timed out after 30m]",
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	var exitCode int64
	var output string
	if err := db.QueryRow(
		`SELECT exit_code, output FROM job_runs WHERE job_id = ?`, id,
	).Scan(&exitCode, &output); err != nil {
		t.Fatalf("select run: %v", err)
	}
	if exitCode != -1 {
		t.Errorf("exit_code = %d, want -1 to round-trip", exitCode)
	}
	if output != "[killed: timed out after 30m]" {
		t.Errorf("output = %q round-tripped wrong", output)
	}
}

// Spec: job_runs.job_id REFERENCES jobs(id) and the pragma DSN mirrors
// finance's foreign_keys(1) — a run for a job that never existed must be
// refused at insert time.
// AMENDED after code review: this test originally asserted the opposite —
// that job_runs.job_id was FK-enforced — faithfully pinning a spec bug.
// An enforced REFERENCES makes `DELETE FROM jobs` fail for any job that
// ever ran, which contradicts the spec's own "job_runs rows are KEPT;
// history outlives the job". The constraint lost: run history must
// survive its job, so an orphan job_id is legal by design.
func TestJobRunsSurviveJobDeletion(t *testing.T) {
	db := newTestDB(t)

	res, err := db.Exec(
		`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, ?, ?, ?)`,
		"doomed", "0 9 * * *", "true", "2026-08-05T09:00:00-04:00",
	)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	jobID, _ := res.LastInsertId()

	if _, err := db.Exec(
		`INSERT INTO job_runs (job_id, started_at, finished_at, exit_code, output)
		 VALUES (?, ?, ?, ?, ?)`,
		jobID, "2026-08-05T09:00:00-04:00", "2026-08-05T09:00:01-04:00", 0, "ran once",
	); err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM jobs WHERE id = ?`, jobID); err != nil {
		t.Fatalf("deleting a job with run history failed: %v — history must not block removal", err)
	}

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM job_runs WHERE job_id = ?`, jobID).Scan(&n); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if n != 1 {
		t.Errorf("run rows after job deletion = %d, want 1 — history outlives the job", n)
	}
}

// Spec: _pragma=busy_timeout(5000) — the whole point is that cron-exec's
// write while the REPL holds the lock WAITS and lands rather than failing
// immediately with SQLITE_BUSY. Same shape as the spec's spike: one
// connection takes the write lock, a second connection's insert must
// block until the first commits and then succeed.
func TestBusyTimeoutWriterWaitsForLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")

	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("OpenDBAt (locker): %v", err)
	}
	t.Cleanup(func() { dbA.Close() })
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatalf("OpenDBAt (writer): %v", err)
	}
	t.Cleanup(func() { dbB.Close() })

	ctx := context.Background()
	conn, err := dbA.Conn(ctx)
	if err != nil {
		t.Fatalf("dedicated connection: %v", err)
	}
	// BEGIN IMMEDIATE takes the write lock right now, on this one
	// connection — database/sql's pooling would otherwise scatter the
	// statements across connections.
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("BEGIN IMMEDIATE: %v", err)
	}

	release := make(chan struct{})
	go func() {
		time.Sleep(400 * time.Millisecond)
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			t.Errorf("COMMIT on locker: %v", err)
		}
		conn.Close()
		close(release)
	}()

	start := time.Now()
	_, err = dbB.Exec(
		`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, ?, ?, ?)`,
		"under-contention", "* * * * *", "true", "2026-08-05T09:00:00-04:00",
	)
	elapsed := time.Since(start)
	<-release

	if err != nil {
		t.Fatalf("write while another connection held the lock failed after %s: %v — busy_timeout pragma missing?", elapsed, err)
	}
	// Without the pragma the failure is instant; succeeding here after a
	// real wait is what proves the write actually blocked on the lock.
	if elapsed < 200*time.Millisecond {
		t.Errorf("blocked write returned in %s, want it to have waited for the lock (~400ms)", elapsed)
	}
}
