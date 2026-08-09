# cron — decisions

Companion to `cron.spec.md`: what changed while building it, what the
implementation learned that the spec couldn't know, and what shipped
knowingly broken. Verified live on 2026-08-07 (see "Live fire" at the
bottom).

## Amendments to the spec

### The foreign key lost to the audit trail

The spec's DDL had `job_runs.job_id INTEGER NOT NULL REFERENCES jobs(id)`
and a DSN carrying `_pragma=foreign_keys(1)`, both copied from finance.
Those clauses contradict the spec's own `cron_remove` rule three sections
later — "job_runs rows are KEPT — history outlives the job (the whole
point of the table)". With enforcement on, `DELETE FROM jobs` fails with
`FOREIGN KEY constraint failed (787)` for any job that has ever run, so
`cron_remove` would break on exactly the jobs worth removing.

Both were dropped: no `REFERENCES`, no `foreign_keys` pragma (there is now
no FK in the schema for it to enforce). `busy_timeout(5000)` stays — that
one is load-bearing. An orphaned `job_id` is legal by design.

The reason this is worth writing down: the constraint *looks* like
diligence, and the next person to read the schema will want to add it
back. `TestJobRunsSurviveJobDeletion` in `internal/eviedb/db_test.go` is
the tripwire — re-adding the FK turns it red immediately.

### `openDBAt` became `eviedb.OpenDBAt`

The tools tests originally hand-copied the DDL into `cron_test.go` and
opened their temp db with their own pragma string, because `openDBAt` was
unexported and unreachable from `internal/tools`.

That copy is how the FK problem above hid for as long as it did. The
copied DDL kept the foreign key after production dropped it, and the seam's
connection happened to have enforcement *off* — so "remove keeps run
history" passed for entirely the wrong reason, and would have kept passing
if production had regressed. A test fixture shaped by anything other than
the production opener can be green while production is broken.

`OpenDBAt` is now exported and both test packages use it. The DDL exists
in exactly one place.

### `cron_add`'s rollback error is checked

The spec said "if installJob errors, DELETE the row and return the error —
no half-registered jobs". The implementation did that but discarded the
DELETE's own error, which meant a failed rollback produced precisely the
half-registered job the rule exists to prevent, while reporting only the
install failure. The job would sit in `cron_list` looking scheduled and
never fire.

Now the two errors travel together via `errors.Join`, and the joined
message names the job and tells the model to retry `cron_remove <name>` —
the model is the only party who can relay that to David.

### `cronexec.go` returns its exit code instead of calling `os.Exit`

`runCronExec` is now a one-liner wrapping `cronExec(args, stderr) int`.
Two payoffs: the exit-code discipline is testable (the package had zero
tests, and this is the one file whose failure mode surfaces at 9am with
nobody watching), and the deferred `db.Close` actually runs — `os.Exit`
mid-function skips every pending defer.

`sql.ErrNoRows` is now also distinguished from a genuine read failure.
Both still exit 1, per spec, but a stale plist and a locked database are
different problems and stderr is the only channel launchd offers.

### The timeout note says "30m", not "30m0s"

`fmt.Sprintf("%s", 30*time.Minute)` renders `30m0s`. The spec and
`eviedb/db_test.go` both pin `[killed: timed out after 30m]`. A
`humanDuration` helper trims the zero tail. Small, but the string is
stored in `job_runs.output` and read by a person, and the trailing `0s`
implies a precision the timeout does not have.

The helper trims `"m0s"`→`"m"` and `"h0m"`→`"h"`, never a bare `"0s"` —
that would turn `30s` into `3`.

### Two mandated clauses were missing from `cron_add`'s description

The spec requires the description to tell the model (a) to remove and
re-add jobs if the evie binary moves, and (b) that
`launchctl print gui/$(id -u)/com.evie.cron.<name>` is the debug path.
Neither string was there. For a tool whose failures are invisible by
design, the description is the only place the model can learn how to
recover — these were the two clauses that make drift and a moved binary
fixable at all.

## Known gaps, shipped deliberately

- **Powered-off fires are lost.** launchd runs a missed fire on wake and
  coalesces multiple misses into one run, but a machine that was *off* at
  fire time gets no catch-up (no anacron semantics). Accepted in v1: the
  first customer is a daily sync where a skipped day self-heals on the
  next run, because `/transactions/sync` is cursor-based.

- **A job killed between running and recording leaves no row.** `cron-exec`
  runs the command, then INSERTs. If the process dies in that window
  (machine sleeps mid-run, SIGKILL), the job ran and the audit trail
  doesn't know. Closing this means a two-phase write — INSERT a "started"
  row, UPDATE it on completion — which doubles the write path to cover a
  window measured in milliseconds. Not worth it while `job_runs` is the
  only consumer.

- **Drift between the db and launchd is not reconciled.** The db is the
  source of truth and plists are generated artifacts; `cron_list` reads
  only the db. A hand-deleted plist, or the Background Task Management
  toggle flipped in System Settings, makes `cron_list` confidently report
  a job that will never fire. The debug path is the `launchctl print` line
  now in the tool description.

- **A moved evie binary silently breaks every job.** The plist stores the
  absolute path from `os.Executable()` at add time. Reinstalling elsewhere
  leaves every job pointing at a path that no longer exists — launchd
  fires, the exec fails, and no `job_runs` row is written because the
  process that would write it is the one that didn't start. Detection is
  out of scope; the description tells the model to re-add jobs.

- **`query_db db=evie` has no table fence**, unlike finance's `items`
  guard. `jobs.command` and `job_runs.output` are model-readable, and a
  job that echoes a secret has that secret in its output column. This is
  deliberate — `job_runs` *is* the audit surface, and fencing it would
  defeat the point — but it compounds the ungated-tools decision below and
  deserves to be on the record.

- **All three tools are ungated**, David's call, made knowing the tension:
  a cron job is a bash command that runs when he is *not* reading, which is
  the assumption ungated bash rests on. `job_runs` plus `cron_list` is the
  compensating control. Loudest security decision since ungated bash
  itself.

## Live fire (2026-08-07)

All seven of the spec's end-to-end steps ran against the real launchd:

- A job scheduled for two minutes out fired on time; `job_runs` recorded
  exit 0 and `hello from cron`.
- A job running `exit 7` recorded exit 7, and `cron_list` showed it as the
  last run.
- `cron_remove` on both: plists gone, `launchctl print` reports no such
  service, `jobs` rows gone, all three `job_runs` rows still present.
- `cron-exec`'s exit codes confirmed by hand before they had tests: 2 for
  no arg and for a non-numeric one, 1 for an unknown id, 0 for a job that
  itself failed.
- The first customer (`finance-daily`, `finance sync && finance
  categorize`) forced with `launchctl kickstart -k` ran a real Plaid sync
  and added 13 Amex transactions. **This is what proves the env story** —
  the `finance` binary was found on PATH and the Plaid keys loaded from
  `~/.evie/.env`, under launchd's minimal environment, exactly as the
  shell-snapshot replay is supposed to deliver.

One unrelated find, worth its own backlog item: `finance sync` fails for
Citibank and PNC with `FOREIGN KEY constraint failed (787)` while deleting
transactions. Pre-existing in `internal/finance`, nothing to do with cron
— cron simply gave it a stage.
