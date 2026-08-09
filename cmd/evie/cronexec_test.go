package main

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
)

// The spec is emphatic about cron-exec's exit codes because launchd reads
// them: a non-zero exit teaches launchd that evie itself is broken. The
// distinction it draws — plumbing failures are non-zero, a failing JOB is
// not — is invisible in the launchd UI and can only be caught here.
// These tests were added after code review found cmd/evie had none.

// newExecDB shapes a temp db with the production opener and points the
// cron-exec seam at it, returning an assertion connection.
func newExecDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evie.db")

	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatalf("test setup: open temp db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	original := openCronExecDB
	openCronExecDB = func() (*sql.DB, error) { return eviedb.OpenDBAt(path) }
	t.Cleanup(func() { openCronExecDB = original })
	return db
}

// addJob inserts a job and returns its id.
func addJob(t *testing.T, db *sql.DB, name, command string) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO jobs (name, schedule, command, created_at) VALUES (?, ?, ?, ?)`,
		name, "0 9 * * *", command, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		t.Fatalf("test setup: insert job: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("test setup: job id: %v", err)
	}
	return id
}

// Spec: "no argument or a non-numeric one -> usage line, exit 2 (the
// finance exemplar's convention)".
func TestCronExecUsageExitsTwo(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"no arguments", nil},
		{"too many arguments", []string{"1", "2"}},
		{"non-numeric", []string{"abc"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			if code := cronExec(tc.args, &stderr); code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), "usage:") {
				t.Errorf("stderr = %q, want a usage line", stderr.String())
			}
		})
	}
}

// Spec: "unknown id -> exit 1 with a stderr line". Code review also asked
// that a missing row be distinguishable from a db read failure, so the
// message must point at the stale plist rather than blaming the database.
func TestCronExecUnknownIDExitsOne(t *testing.T) {
	newExecDB(t)

	var stderr bytes.Buffer
	if code := cronExec([]string{"999"}, &stderr); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	msg := stderr.String()
	if !strings.Contains(msg, "999") {
		t.Errorf("stderr = %q, want the offending id named", msg)
	}
	if !strings.Contains(msg, "stale") {
		t.Errorf("stderr = %q, want the stale-plist diagnosis, not a db error", msg)
	}
}

// The load-bearing one: "cron-exec succeeding is independent of the job
// failing — launchd should not see a failed cron-exec for a failed JOB".
// The failure must land in job_runs instead.
func TestCronExecReturnsZeroWhenJobFails(t *testing.T) {
	db := newExecDB(t)
	id := addJob(t, db, "failing", "exit 7")

	var stderr bytes.Buffer
	if code := cronExec([]string{strconv.FormatInt(id, 10)}, &stderr); code != 0 {
		t.Errorf("exit code = %d, want 0 — a failing job is not a failing cron-exec", code)
	}

	var exitCode int
	if err := db.QueryRow(`SELECT exit_code FROM job_runs WHERE job_id = ?`, id).Scan(&exitCode); err != nil {
		t.Fatalf("select run: %v", err)
	}
	if exitCode != 7 {
		t.Errorf("recorded exit_code = %d, want 7 — the failure belongs in job_runs", exitCode)
	}
}

// The job_runs INSERT is the entire audit trail this feature exists to
// produce: output captured, exit code 0, both timestamps RFC3339 and
// ordered.
func TestCronExecRecordsRun(t *testing.T) {
	db := newExecDB(t)
	id := addJob(t, db, "greeter", "echo hello from cron")

	var stderr bytes.Buffer
	if code := cronExec([]string{strconv.FormatInt(id, 10)}, &stderr); code != 0 {
		t.Fatalf("exit code = %d (stderr: %s), want 0", code, stderr.String())
	}

	var (
		exitCode              int
		output                string
		startedAt, finishedAt string
	)
	if err := db.QueryRow(
		`SELECT exit_code, output, started_at, finished_at FROM job_runs WHERE job_id = ?`, id,
	).Scan(&exitCode, &output, &startedAt, &finishedAt); err != nil {
		t.Fatalf("select run: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exit_code = %d, want 0", exitCode)
	}
	if !strings.Contains(output, "hello from cron") {
		t.Errorf("output = %q, want the command's stdout", output)
	}

	started, err := time.Parse(time.RFC3339, startedAt)
	if err != nil {
		t.Errorf("started_at %q is not RFC3339: %v", startedAt, err)
	}
	finished, err := time.Parse(time.RFC3339, finishedAt)
	if err != nil {
		t.Errorf("finished_at %q is not RFC3339: %v", finishedAt, err)
	}
	if finished.Before(started) {
		t.Errorf("finished_at %s precedes started_at %s", finishedAt, startedAt)
	}
}
