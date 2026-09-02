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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOpenDBAtContextCanceledBeforeSetupCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancelled.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenDBAtContext(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenDBAtContext error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled setup created database: %v", err)
	}
}

func TestOpenDBAtContextCancellationAfterFileCreationLeavesMode0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cancelled-after-create.db")
	ctx, cancel := context.WithCancel(context.Background())
	original := hardenWritableDBFile
	hardenWritableDBFile = func(path string) error {
		err := original(path)
		cancel()
		return err
	}
	t.Cleanup(func() { hardenWritableDBFile = original })

	if _, err := OpenDBAtContext(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenDBAtContext error = %v, want context.Canceled", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("cancelled database mode = %04o, want 0600", got)
	}
}

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

func TestTaskResultSummaryUpgradeSerializesConcurrentLegacyOpeners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`
		CREATE TABLE tasks (id TEXT PRIMARY KEY);
		CREATE TABLE task_revisions (task_id TEXT, revision INTEGER);
	`); err != nil {
		_ = legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	dbA, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	dbB, err := sql.Open("sqlite", path+dsnPragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()

	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	hooks := taskResultSummaryUpgradeHooks{afterFastMissingCheck: func() {
		ready <- struct{}{}
		<-start
	}}
	errors := make(chan error, 2)
	var wait sync.WaitGroup
	for _, db := range []*sql.DB{dbA, dbB} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errors <- ensureTaskResultSummaryWithHooks(context.Background(), db, hooks)
		}()
	}
	<-ready
	<-ready
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent result-summary upgrade: %v", err)
		}
	}
	for _, table := range []string{"tasks", "task_revisions"} {
		present, err := tableHasColumn(context.Background(), dbA, table, "result_summary")
		if err != nil || !present {
			t.Fatalf("%s result_summary present=%v err=%v", table, present, err)
		}
	}
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

func TestOpenDBAtConfiguresEveryConnection(t *testing.T) {
	db := newTestDB(t)
	db.SetMaxOpenConns(2)

	if _, err := db.Exec(`
		CREATE TABLE pragma_test_parents (id INTEGER PRIMARY KEY);
		CREATE TABLE pragma_test_children (
			id        INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL REFERENCES pragma_test_parents(id)
		);
	`); err != nil {
		t.Fatalf("create foreign-key test tables: %v", err)
	}

	ctx := context.Background()
	connA, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire first connection: %v", err)
	}
	defer connA.Close()

	connB, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire second connection: %v", err)
	}
	defer connB.Close()

	for i, conn := range []*sql.Conn{connA, connB} {
		var journalMode string
		if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("connection %d journal mode: %v", i+1, err)
		}
		if journalMode != "wal" {
			t.Errorf("connection %d journal mode = %q, want wal", i+1, journalMode)
		}

		var foreignKeys int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatalf("connection %d foreign keys: %v", i+1, err)
		}
		if foreignKeys != 1 {
			t.Errorf("connection %d foreign_keys = %d, want 1", i+1, foreignKeys)
		}

		if _, err := conn.ExecContext(ctx,
			`INSERT INTO pragma_test_children (id, parent_id) VALUES (?, 999)`, i+1,
		); err == nil {
			t.Errorf("connection %d accepted a child with no parent", i+1)
		}
	}
}

func TestOpenDBReadOnlyConfiguresEveryConnection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	writer, err := OpenDB()
	if err != nil {
		t.Fatalf("create canonical database: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	db, err := OpenDBReadOnly()
	if err != nil {
		t.Fatalf("open canonical database read-only: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(2)

	ctx := context.Background()
	connA, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire first read-only connection: %v", err)
	}
	defer connA.Close()

	connB, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("acquire second read-only connection: %v", err)
	}
	defer connB.Close()

	for i, conn := range []*sql.Conn{connA, connB} {
		var journalMode string
		if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("connection %d journal mode: %v", i+1, err)
		}
		if journalMode != "wal" {
			t.Errorf("connection %d journal mode = %q, want wal", i+1, journalMode)
		}

		var foreignKeys int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
			t.Fatalf("connection %d foreign keys: %v", i+1, err)
		}
		if foreignKeys != 1 {
			t.Errorf("connection %d foreign_keys = %d, want 1", i+1, foreignKeys)
		}

		if _, err := conn.ExecContext(ctx,
			`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, '* * * * *', 'true', '2026-08-17T12:00:00Z')`,
			fmt.Sprintf("forbidden-%d", i+1),
		); err == nil {
			t.Errorf("connection %d accepted a write through mode=ro", i+1)
		}
	}
}

func TestProjectsSchemaAndConstraints(t *testing.T) {
	db := newTestDB(t)

	const (
		projectID = "project-1"
		root      = "/tmp/evie-project"
		createdAt = "2026-08-17T12:00:00Z"
		updatedAt = "2026-08-17T12:00:00Z"
	)

	if _, err := db.Exec(`
		INSERT INTO projects (id, display_name, canonical_root, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, projectID, "Evie", root, createdAt, updatedAt); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	var (
		gotName, gotRoot, gotCreatedAt, gotUpdatedAt string
		gotArchived                                  int
	)
	if err := db.QueryRow(`
		SELECT display_name, canonical_root, archived, created_at, updated_at
		FROM projects
		WHERE id = ?
	`, projectID).Scan(&gotName, &gotRoot, &gotArchived, &gotCreatedAt, &gotUpdatedAt); err != nil {
		t.Fatalf("read project: %v", err)
	}
	if gotName != "Evie" || gotRoot != root || gotArchived != 0 ||
		gotCreatedAt != createdAt || gotUpdatedAt != updatedAt {
		t.Errorf("project = (%q, %q, %d, %q, %q)", gotName, gotRoot, gotArchived, gotCreatedAt, gotUpdatedAt)
	}

	if _, err := db.Exec(`
		INSERT INTO projects (id, display_name, canonical_root, created_at, updated_at)
		VALUES ('project-2', 'Same root', ?, ?, ?)
	`, root, createdAt, updatedAt); err == nil {
		t.Error("duplicate canonical root was accepted")
	}

	if _, err := db.Exec(`
		INSERT INTO projects (id, display_name, canonical_root, archived, created_at, updated_at)
		VALUES ('project-3', 'Invalid archive state', '/tmp/another-project', 2, ?, ?)
	`, createdAt, updatedAt); err == nil {
		t.Error("archive state outside 0 or 1 was accepted")
	}
}

func TestSessionsSchemaAndScopeConstraints(t *testing.T) {
	db := newTestDB(t)
	const now = "2026-08-17T12:00:00Z"

	if _, err := db.Exec(`
		INSERT INTO projects (id, display_name, canonical_root, created_at, updated_at)
		VALUES ('project-1', 'Evie', '/tmp/evie-project', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO sessions (id, created_at, updated_at)
		VALUES ('global-session', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("insert global session: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO sessions (
			id, project_id, project_root_snapshot, parent_session_id, created_at, updated_at
		) VALUES ('project-session', 'project-1', '/tmp/evie-project', 'global-session', ?, ?)
	`, now, now); err != nil {
		t.Fatalf("insert project session: %v", err)
	}

	var (
		projectID    sql.NullString
		rootSnapshot sql.NullString
		status       string
	)
	if err := db.QueryRow(`
		SELECT project_id, project_root_snapshot, status
		FROM sessions
		WHERE id = 'global-session'
	`).Scan(&projectID, &rootSnapshot, &status); err != nil {
		t.Fatalf("read global session: %v", err)
	}
	if projectID.Valid || rootSnapshot.Valid || status != "active" {
		t.Errorf("global session = (project=%v, root=%v, status=%q)", projectID, rootSnapshot, status)
	}

	invalidInserts := []struct {
		name string
		sql  string
	}{
		{
			name: "project without root snapshot",
			sql:  `INSERT INTO sessions (id, project_id, created_at, updated_at) VALUES ('bad-1', 'project-1', '` + now + `', '` + now + `')`,
		},
		{
			name: "root snapshot without project",
			sql:  `INSERT INTO sessions (id, project_root_snapshot, created_at, updated_at) VALUES ('bad-2', '/tmp/x', '` + now + `', '` + now + `')`,
		},
		{
			name: "missing project",
			sql:  `INSERT INTO sessions (id, project_id, project_root_snapshot, created_at, updated_at) VALUES ('bad-3', 'missing', '/tmp/x', '` + now + `', '` + now + `')`,
		},
		{
			name: "missing parent",
			sql:  `INSERT INTO sessions (id, parent_session_id, created_at, updated_at) VALUES ('bad-4', 'missing', '` + now + `', '` + now + `')`,
		},
		{
			name: "invalid status",
			sql:  `INSERT INTO sessions (id, status, created_at, updated_at) VALUES ('bad-5', 'unknown', '` + now + `', '` + now + `')`,
		},
	}

	for _, tt := range invalidInserts {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.Exec(tt.sql); err == nil {
				t.Error("invalid session was accepted")
			}
		})
	}

	if _, err := db.Exec(`
		UPDATE sessions
		SET project_id = 'project-1', project_root_snapshot = '/tmp/evie-project'
		WHERE id = 'global-session'
	`); err == nil {
		t.Error("immutable session scope was updated")
	}

	if _, err := db.Exec(`UPDATE sessions SET status = 'closed', updated_at = ? WHERE id = 'global-session'`, now); err != nil {
		t.Fatalf("close session: %v", err)
	}
}

func TestEventsSchemaAndAppendOnlyConstraints(t *testing.T) {
	db := newTestDB(t)
	const now = "2026-08-17T12:00:00Z"

	if _, err := db.Exec(`
		INSERT INTO projects (id, display_name, canonical_root, created_at, updated_at)
		VALUES ('project-1', 'Evie', '/tmp/evie-project', ?, ?);
		INSERT INTO sessions (id, created_at, updated_at)
		VALUES ('global-session', ?, ?);
		INSERT INTO sessions (id, project_id, project_root_snapshot, created_at, updated_at)
		VALUES ('project-session', 'project-1', '/tmp/evie-project', ?, ?);
	`, now, now, now, now, now, now); err != nil {
		t.Fatalf("seed event scopes: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO events (
			id, session_id, sequence, event_type, role, content, payload_json, recorded_at
		) VALUES ('event-global-1', 'global-session', 1, 'user_message', 'user',
			'hello', '{"source":"repl"}', ?)
	`, now); err != nil {
		t.Fatalf("insert global event: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO events (
			id, session_id, sequence, project_id, event_type, role, content, recorded_at
		) VALUES ('event-project-1', 'project-session', 1, 'project-1',
			'assistant_message', 'assistant', 'hello back', ?)
	`, now); err != nil {
		t.Fatalf("insert project event: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO events (
			id, session_id, sequence, parent_id, event_type, execution_id, payload_json, recorded_at
		) VALUES ('event-global-2', 'global-session', 2, 'event-global-1',
			'tool_intent', 'execution-1', '{"tool":"time"}', ?)
	`, now); err != nil {
		t.Fatalf("insert child execution event: %v", err)
	}

	var (
		projectID, parentID, executionID sql.NullString
		content, payload, eventType      string
		sequence, formatVersion          int
	)
	if err := db.QueryRow(`
		SELECT sequence, project_id, parent_id, event_type, execution_id,
			content, payload_json, format_version
		FROM events
		WHERE id = 'event-global-2'
	`).Scan(
		&sequence, &projectID, &parentID, &eventType, &executionID,
		&content, &payload, &formatVersion,
	); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if sequence != 2 || projectID.Valid || !parentID.Valid || parentID.String != "event-global-1" ||
		eventType != "tool_intent" || !executionID.Valid || executionID.String != "execution-1" ||
		content != "" || payload != `{"tool":"time"}` || formatVersion != 1 {
		t.Errorf("event round trip mismatch: sequence=%d project=%v parent=%v type=%q execution=%v content=%q payload=%q version=%d",
			sequence, projectID, parentID, eventType, executionID, content, payload, formatVersion)
	}

	invalidInserts := []struct {
		name string
		sql  string
	}{
		{
			name: "duplicate session sequence",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, recorded_at) VALUES ('bad-1', 'global-session', 1, 'user_message', '` + now + `')`,
		},
		{
			name: "missing session",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, recorded_at) VALUES ('bad-2', 'missing', 1, 'user_message', '` + now + `')`,
		},
		{
			name: "project scope omitted",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, recorded_at) VALUES ('bad-3', 'project-session', 2, 'user_message', '` + now + `')`,
		},
		{
			name: "project scope added to global session",
			sql:  `INSERT INTO events (id, session_id, sequence, project_id, event_type, recorded_at) VALUES ('bad-4', 'global-session', 3, 'project-1', 'user_message', '` + now + `')`,
		},
		{
			name: "parent from another session",
			sql:  `INSERT INTO events (id, session_id, sequence, project_id, parent_id, event_type, recorded_at) VALUES ('bad-5', 'project-session', 2, 'project-1', 'event-global-1', 'tool_succeeded', '` + now + `')`,
		},
		{
			name: "invalid role",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, role, recorded_at) VALUES ('bad-6', 'global-session', 3, 'user_message', 'developer', '` + now + `')`,
		},
		{
			name: "invalid payload JSON",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, payload_json, recorded_at) VALUES ('bad-7', 'global-session', 3, 'user_message', '{', '` + now + `')`,
		},
		{
			name: "non-positive sequence",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, recorded_at) VALUES ('bad-8', 'global-session', 0, 'user_message', '` + now + `')`,
		},
		{
			name: "non-positive format version",
			sql:  `INSERT INTO events (id, session_id, sequence, event_type, format_version, recorded_at) VALUES ('bad-9', 'global-session', 3, 'user_message', 0, '` + now + `')`,
		},
	}

	for _, tt := range invalidInserts {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := db.Exec(tt.sql); err == nil {
				t.Error("invalid event was accepted")
			}
		})
	}

	if _, err := db.Exec(`UPDATE events SET content = 'rewritten' WHERE id = 'event-global-1'`); err == nil {
		t.Error("event update was accepted")
	}
	if _, err := db.Exec(`DELETE FROM events WHERE id = 'event-project-1'`); err == nil {
		t.Error("event deletion was accepted")
	}
}
