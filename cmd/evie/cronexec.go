package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/tools"
)

// cronExecTimeout bounds one scheduled run. A scheduled job is not an
// interactive call — a build or a full finance sync is welcome to take
// minutes — but a hung job must not accumulate forever under launchd.
const cronExecTimeout = 30 * time.Minute

// openCronExecDB is the db seam for cron-exec, a var so tests can point
// it at a temp file — same pattern as internal/tools' openCronDB.
var openCronExecDB = eviedb.OpenDB

// runCronExec is the launchd entry point: it owns the process exit, and
// nothing else. All behavior lives in cronExec so the exit-code
// discipline can be tested without spawning a binary.
func runCronExec(args []string) {
	os.Exit(cronExec(args, os.Stderr))
}

// cronExec is the launchd side of the cron feature: load one job by id,
// run its command, record the run. It returns 0 even when the JOB fails —
// a non-zero exit here would make launchd treat evie itself as broken,
// when the failure belongs in job_runs for David and the model to read.
// Non-zero is reserved for broken plumbing: bad arguments (2, the finance
// CLI convention) or an unusable job/db (1).
//
// Returning the code instead of calling os.Exit is what makes this
// testable, and it also means the deferred db.Close actually runs —
// os.Exit mid-function skips every pending defer.
func cronExec(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: evie cron-exec <job-id>")
		return 2
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(stderr, "usage: evie cron-exec <job-id> — %q is not a job id\n", args[0])
		return 2
	}

	db, err := openCronExecDB()
	if err != nil {
		fmt.Fprintf(stderr, "cron-exec: open evie db: %v\n", err)
		return 1
	}
	defer db.Close()

	// Both cases return 1, but stderr is the only diagnostic channel
	// launchd gives us: a stale plist pointing at a deleted job and a
	// locked or corrupt database need visibly different messages.
	var command string
	switch err := db.QueryRow(`SELECT command FROM jobs WHERE id = ?`, id).Scan(&command); {
	case errors.Is(err, sql.ErrNoRows):
		fmt.Fprintf(stderr, "cron-exec: no job with id %d — its plist is stale; cron_remove and cron_add it\n", id)
		return 1
	case err != nil:
		fmt.Fprintf(stderr, "cron-exec: read job %d: %v\n", id, err)
		return 1
	}

	started := time.Now()
	output, code := tools.RunScheduled(command, cronExecTimeout)
	finished := time.Now()

	if _, err := db.Exec(
		`INSERT INTO job_runs (job_id, started_at, finished_at, exit_code, output) VALUES (?, ?, ?, ?, ?)`,
		id, started.Format(time.RFC3339), finished.Format(time.RFC3339), code, string(output),
	); err != nil {
		fmt.Fprintf(stderr, "cron-exec: record run: %v\n", err)
		return 1
	}
	return 0
}
