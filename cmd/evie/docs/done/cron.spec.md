# cron — spec

## Purpose

evie schedules recurring shell commands that fire even when evie
isn't running: `cron_add` / `cron_list` / `cron_remove` tools backed by
sqlite (`~/.evie/evie.db`, tables `jobs` + `job_runs`), with macOS
launchd as the firing mechanism and a `evie cron-exec <id>` subcommand
as the thing launchd runs. First customer: daily finance sync +
categorize.

## The mechanism (decided by research, verified claims marked)

**launchd LaunchAgent per job**, not an in-process scheduler and not
crontab:

- Claude Code's own cron is a 1-second tick loop inside the running CLI —
  jobs fire only while a session is open. Rejected: the first customer
  must run at 9am with no terminal open.
- User crontab still works on macOS but does **not** run missed jobs on
  wake — a sleeping laptop silently skips the fire. Disqualifying for a
  daily job on a laptop.
- launchd `StartCalendarInterval` fires on schedule, **runs a missed fire
  on wake, coalescing multiple misses into one run**, and plists in
  `~/Library/LaunchAgents/` auto-load at login. Powered-off fires are
  lost (no anacron semantics) — known gap, v1 ships without catch-up.

Each job gets `~/Library/LaunchAgents/com.evie.cron.<name>.plist`
whose `ProgramArguments` is `[<evie binary>, "cron-exec", "<id>"]`.
The binary path comes from `os.Executable()` at add time and is stored
in the plist; if the binary moves, `cron_add`'s description says to
remove and re-add jobs (out of scope to detect).

**launchctl invocations** (modern, verified against 2026 macOS):

```
launchctl bootout gui/<uid>/com.evie.cron.<name>   # ignore "not found" errors
launchctl bootstrap gui/<uid> <plist path>
```

Bootout-before-bootstrap always: bootstrap on an already-loaded label
fails with `Bootstrap failed: 5: Input/output error`. **Bootout errors
are ignored unconditionally** — distinguishing "not found" from other
failures means parsing launchctl stderr, and the bootstrap that follows
surfaces anything real. `<uid>` from `os.Getuid()`. These run via
`os/exec` directly (this is evie's own plumbing, not a model tool
call).

**The db is the source of truth; plists are generated artifacts.**
`cron_list` reads the db, never parses plists or `launchctl print`.
Drift (a plist hand-deleted, the Background-Task-Management toggle
flipped in System Settings) is accepted in v1 — the description tells
the model `launchctl print gui/<uid>/<label>` via bash is the debug path.

## Decisions

- **Everything ungated** — David's call, made knowing the tension: a
  cron job is a bash command that runs when he is *not* reading, which
  is the assumption ungated-bash rests on. Consistency with the rest of
  the toolbox won; the `job_runs` table plus `cron_list` is the audit
  trail. Recorded emphatically because it is the loudest security
  decision since ungated bash itself.
- **Failure = a row, nothing else.** A non-zero exit is recorded in
  `job_runs` (the bash "result, not error" rule applied to scheduled
  runs); no notifications, no log files beyond the table, v1. The
  question "did my syncs run this week?" is a `query_db` query.
- **sqlite, no JSON ledger.** Departure from the `todo` convention,
  David's call: state and history live together, and `query_db` already
  reads registered databases — a JSON ledger would need bespoke
  plumbing while a table is instantly queryable.
- **`evie` db registered in `query_db` only — NOT `edit_db`.** A
  hand-edited `jobs` row would silently diverge from the plist it was
  generated into (schedule changed in the db, launchd still firing the
  old one). Writes go through the cron tools, which keep both in step.
  The `edit_db` unknown-db error for "evie" should say exactly that.
- **Schedule syntax: 5-field cron expressions.** The model already
  speaks it, and it translates to `StartCalendarInterval` dicts.
  Translation rules below; unsupported shapes error with instructions
  rather than approximating.
- **Jobs run through the user's login shell + shell snapshot**, same
  replay as the `bash` tool (`source <snapshot> || true`, `eval`,
  `$SHELL -l -c`), because launchd's environment is minimal (PATH is
  `/usr/bin:/bin:/usr/sbin:/sbin`, no shell profile — verified). Without
  this, the first customer (`finance sync`) can't even find its binary.
  cron-exec calls `snapshot()` synchronously — a one-shot process has no
  Warm window; the ≤10s capture cost is fine at 9am with nobody
  waiting. No `sessionCwd` involvement: cron-exec is its own process,
  jobs run in `$HOME`.
- **Env for jobs**: `evie cron-exec` runs `main.go`'s godotenv loads,
  and child processes inherit the result (`Load` → `os.Setenv`;
  `exec.Cmd` with nil Env inherits). **Only the `~/.evie/.env` load is
  load-bearing for cron** — launchd's cwd is `/`, so the cwd-relative
  loads never fire; the serve session's in-flight main.go edits may
  reshape the load list, and cron is fine under any version that keeps
  the `~/.evie/.env` line. The finance keys must live there (not only
  in the repo `.env`) for the first customer to work; live-fire will
  confirm.
- **Job names are identifiers.** `name` is required, unique,
  `[a-z0-9-]+` (it becomes the plist label and filename); `id` is the
  db's autoincrement, used by cron-exec so renames can't orphan a
  plist. Add with an existing name = error suggesting `cron_remove`
  first (no silent upsert).

## Schema (in a new `internal/eviedb` package)

Mirrors `internal/finance/db.go` exactly: one `CREATE TABLE IF NOT
EXISTS` DDL blob applied on every open, no migrations, `OpenDB()` /
`OpenDBReadOnly()` / `OpenDBAt(path)` split so tests hit a temp file,
`0o700` dir / `0o600` file, pragma via DSN query param — **with one
addition finance doesn't need: `_pragma=busy_timeout(5000)` on BOTH open
paths.** Spike-verified: modernc.org/sqlite with no busy_timeout returns
`SQLITE_BUSY` immediately when another connection holds a write lock —
and cron-exec (a launchd-spawned writer) racing the REPL's open db is
this feature's normal condition, not an edge. Without the pragma, the
run row — the entire audit trail — is lost exactly when evie is in
use; with `busy_timeout(2000)` the spike's blocked write waited 540ms
and succeeded.

```sql
CREATE TABLE IF NOT EXISTS jobs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL UNIQUE,
    schedule   TEXT NOT NULL,            -- the 5-field cron expression, verbatim
    command    TEXT NOT NULL,            -- shell command, run via login shell
    created_at TEXT NOT NULL,            -- RFC3339, local
    enabled    INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS job_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    job_id      INTEGER NOT NULL,        -- points at jobs.id logically; NO foreign key, see below
    started_at  TEXT NOT NULL,           -- RFC3339, local
    finished_at TEXT NOT NULL,
    exit_code   INTEGER NOT NULL,        -- -1 when the process could not run at all
    output      TEXT NOT NULL            -- combined stdout+stderr, capped at 64KB with a truncation note
);
```

**AMENDED during implementation:** `job_runs.job_id` originally carried
`REFERENCES jobs(id)`, and the DSN carried `_pragma=foreign_keys(1)` to
match finance. Those two clauses contradict this spec's own `cron_remove`
rule ("job_runs rows are KEPT — history outlives the job"): with the
pragma on, an enforced `REFERENCES` makes `DELETE FROM jobs` fail with
`FOREIGN KEY constraint failed (787)` for any job that has ever run —
verified against modernc.org/sqlite, and it succeeds without the pragma.
The constraint lost, because an audit trail that blocks removal of the
thing it audits is the wrong tradeoff. Both the FK and the `foreign_keys`
pragma are therefore absent; `busy_timeout` stays. An orphan `job_id` is
legal by design and pinned by `TestJobRunsSurviveJobDeletion`.

`enabled` exists for a future `cron_pause`; v1 never writes 0 and
cron-exec ignores it — one column of forward provision, no behavior.

## Interfaces

New files: `internal/eviedb/db.go`, `internal/eviedb/db_test.go`,
`internal/tools/cron.go`, `internal/tools/cron_test.go`,
`cmd/evie/cronexec.go`. Edits: `internal/tools/db.go` (four edits,
listed below), `registry.go` (three lines), `cmd/evie/main.go` (one
dispatch case + move client construction out of the cron-exec path).

### The four db.go edits (registering "evie", read side only)

1. `queryDBTool`'s Enum gains `"evie"`.
2. `queryDBTool`'s description gains the evie blurb — this exact text:

   > Databases: … "evie" — evie's own state. Schema:
   >   jobs(id, name UNIQUE, schedule '5-field cron', command, created_at RFC3339 local, enabled INTEGER (always 1 in v1))
   >   job_runs(id, job_id → jobs.id, started_at, finished_at RFC3339 local, exit_code INTEGER (-1 = did not complete: could not start, or killed at timeout), output TEXT (combined, capped 64KB))
   > "Did my jobs run?" = job_runs joined to jobs, highest job_runs.id per job is the latest run. Rows outlive their job — job_runs keeps history for removed jobs.

3. `queryDB`'s switch gains `case "evie": db, err = eviedb.OpenDBReadOnly()`,
   and the unknown-db error string now lists both databases.
4. `editDB` does NOT gain a case — instead its `default` branch special-
   cases "evie" with: `the evie db is read-only through edit_db —
   its jobs table is kept in sync with launchd by the cron tools; use
   cron_add/cron_remove instead`. The generic unknown-db error also
   updates its db list.

### Tools

```
cron_add(name, schedule, command) — validate name; parse schedule (see
    translation rules); insert the jobs row; call installJob; if
    installJob errors, DELETE the row and return the error — no
    half-registered jobs. (Plist cleanup on failure is installJob's own
    responsibility — see the seam contract.) The rollback DELETE's own
    error must be CHECKED and joined onto the install error (errors.Join),
    naming the job: if the delete also fails, the db holds a job that will
    never fire, and the model is the only one who can tell David to run
    cron_remove. Success result names the job, where the plist lives, and
    does NOT include next fire times (out of scope: computing next-fire).

cron_list() — no parameters. Reads jobs joined with each job's most
    recent run (LEFT JOIN; most recent = highest job_runs.id — NOT max
    started_at, which is local-time RFC3339 and breaks lexicographic
    ordering across DST changes). Renders one block per job: id, name,
    schedule, command, created, last run (time + exit code) or "never
    run".

cron_remove(name) — look up by name; call uninstallJob (errors
    ignored); delete the jobs row. job_runs rows are KEPT — history
    outlives the job (the whole point of the table). Unknown name =
    error listing existing job names.

Cancellation exception (approved 2026-08-23 in
    `../active/memory.decisions.md`) — after a
    cron mutation starts, cleanup may continue under one independent 10-second
    context. If that cleanup cannot establish that launchd accepted the bootout
    and the plist is absent, preserve the jobs row as the durable recovery handle
    and return parent cancellation joined with the cleanup error. `cron_list`
    continues to expose the uncertain job, and a later ordinary `cron_remove`
    retries cleanup. Ordinary, non-cancelled add/remove behavior above is
    unchanged.

### The launchctl seam (test injection point)

Package-level vars in cron.go, replaced by tests — same seam pattern as
fetchTimeout/braveSearchURL:

    var installJob = func(label, plistPath string, plist []byte) error
        // MkdirAll(dir of plistPath, 0o755) — ~/Library/LaunchAgents
        // can be absent on a fresh account; write plist 0o644;
        // launchctl bootout (all errors ignored); launchctl bootstrap.
        // On bootstrap failure: remove the plist file it wrote, return
        // the error. Owns ALL plist file I/O.

    var uninstallJob = func(label, plistPath string) error
        // launchctl bootout (all errors ignored); os.Remove the plist
        // (not-exists ignored).
```

All three ungated. Execute funcs in `internal/tools/cron.go` take the
usual raw-JSON args string.

### `evie cron-exec <id>` (cmd/evie/cronexec.go)

```
no argument or a non-numeric one -> usage line, exit 2 (the finance
    exemplar's convention)
open eviedb (rw)
load the job row; unknown id -> exit 1 with a stderr line
started := now
output, code := tools.RunScheduled(job.command, 30*time.Minute)
INSERT the job_runs row; exit 0 (cron-exec succeeding is independent of
    the job failing — launchd should not see a failed cron-exec for a
    failed JOB, only for broken plumbing: bad id, unopenable db)
```

Structured, per code review, as a thin `runCronExec(args)` that only calls
`os.Exit(cronExec(args, os.Stderr))`, with all behavior in:

    func cronExec(args []string, stderr io.Writer) int
    var openCronExecDB = eviedb.OpenDB   // db seam, as tools has openCronDB

Two reasons the exit code is returned rather than `os.Exit`-ed inline:
the discipline above is launchd-visible and untestable otherwise (this
file had no tests at all), and `os.Exit` mid-function skips the deferred
`db.Close`. `sql.ErrNoRows` is also distinguished from a db read failure —
both exit 1, but a stale plist and a locked database need different
stderr lines, since stderr is the only channel launchd gives.

### `tools.RunScheduled` — the exported runner seam

```
RunScheduled(command string, timeout time.Duration) (output []byte, exitCode int)
```

Lives in `internal/tools` (cron.go) so it can reuse the unexported
snapshot machinery. Runs the command exactly as the bash tool does —
`source <snapshot> || true` (snapshot() called synchronously; a one-shot
process has no Warm window and the ≤10s capture cost is fine with
nobody waiting), `eval '<command>'`, `$SHELL -l -c`, `Dir = $HOME`,
`CombinedOutput`, `WaitDelay` — but with NO sessionCwd read or write:
scheduled runs are their own process and must not touch REPL state.

Error and edge encoding — there is no error return; everything is
(output, exitCode), because job_runs is the only consumer:

- normal completion: the command's output and exit code, verbatim
- timeout: process killed; exitCode -1, output gets
  "[killed: timed out after 30m]" appended — note "30m", not Go's
  `Duration.String()` "30m0s"; the note is rendered through a
  `humanDuration` helper that trims the zero tail, because this string is
  stored in job_runs.output and read by people
- could not start at all (no shell, exec failure): exitCode -1, output
  is the error text
- exit_code -1 therefore means "did not complete normally" — the output
  text distinguishes which way
- the 64KB cap (keep head, append a truncation note) is applied INSIDE
  RunScheduled — one place, and cronexec.go stores what it gets
```

No API client, no session, no tool registry — `main.go`'s client
construction moves inside the REPL/serve arms so cron-exec runs without
`OPENROUTER_API_KEY`.

### Schedule translation (cron → StartCalendarInterval)

Five fields: minute hour day-of-month month day-of-week.

- Each field: `*`, a single number, a comma list **of single numbers
  only**, a range `a-b`, or a step `*/n` (steps apply to `*` only —
  `1-10/2` and list-embedded ranges like `1-5,30` are rejected with an
  error naming the supported forms). Names (jan, mon) are NOT
  supported — numbers only, error says so. Values dedupe before
  anything is counted.
- A restricted field expands to its value set; **a `*` field contributes
  nothing to the cross-product** (it is omitted from the dict — launchd
  treats a missing key as a wildcard, verified against the launchd.plist
  man page: "Missing arguments are considered to be wildcard"). The
  plist gets the cross-product of the restricted fields as an array of
  `StartCalendarInterval` dicts.
- **Cross-product capped at 100 dicts** — `* * * * *` is one empty dict
  (fires every minute, per the wildcard rule above), but
  `*/2 */2 * * *` (30×12=360) errors telling the model to simplify.
- **Restricting BOTH day-of-month and day-of-week errors.** Vixie cron
  ORs them; launchd's dict ANDs them; silently changing semantics is
  worse than refusing. The error explains it.
- Ranges validated: minute 0-59, hour 0-23, dom 1-31, month 1-12, dow
  0-6 (0=Sunday; 7 rejected with "use 0 for Sunday").
- launchd evaluates in **local time**; the description says so.

## Codebase context

- **`internal/finance/db.go`** — the db package to mirror, function for
  function (schema blob, OpenDB/OpenDBReadOnly/OpenDBAt, perms).
- **`internal/tools/bash.go:172-190`** — the snapshot replay to reuse
  for cron-exec's command execution (`eval` is load-bearing:
  `bash.decisions.md:144`). `snapshot()` and `shellQuote` are already
  in-package for `internal/tools`; cron-exec in `cmd/evie` needs a
  small exported seam — add `tools.RunScheduled(command string, timeout
  time.Duration) (output []byte, exitCode int)` to `internal/tools`
  (cron.go), which cron-exec calls. Keeps snapshot machinery unexported.
- **`internal/tools/db.go`** — the four edits are enumerated in the
  Interfaces section above. `finance.Query` at
  `internal/finance/query.go:12` is db-agnostic and is what `queryDB`
  already calls — reuse as-is, do not move it in this feature.
- **`cmd/finance/main.go:37-115`** — exemplar for an arg-carrying
  subcommand (usage(), exit 2, log.Fatal on domain errors).
- **`cmd/evie/main.go`** — dispatch switch at 46-53; note the serve
  session's in-flight work owns this file too, so keep the diff minimal:
  one case line + moving client construction into the arms that need it.
- **Conventions**: error strings are instructions the model acts on
  (`bash.decisions.md`, "a non-zero exit is a result"); tool
  descriptions are the contract; decisions docs get a "Known gaps,
  shipped deliberately" section.

## Out of scope

- **No catch-up for powered-off misses** (launchd loses them; recorded).
- **A run killed before its INSERT leaves no job_runs row** — kill -9,
  power loss, or a cron-exec crash mid-command produces no trace, since
  the row is written after completion. Accepted for v1 rather than a
  started-row-then-update scheme; goes in the decisions doc's known
  gaps.
- **No next-fire-time computation** anywhere — that's a cron-expression
  evaluator; v1 stores and translates expressions, never evaluates them.
- **No cron_pause/cron_enable** — the `enabled` column waits.
- **No editing** — change a job by remove + add.
- **No drift reconciliation** between db and launchd/plists; no reading
  `launchctl print`.
- **No scheduled agent prompts** — commands only. (Scheduling "ask
  evie to review transactions" is a future feature riding the same
  tables.)
- **No Linux/systemd** — darwin only, like the rest of the repo.
- **No log files** beyond the `job_runs.output` column.
- **`edit_db` stays closed to the evie db.**
- **No touching serve.spec.md's territory** — no HTTP, no daemon.

## Testing

Pure parts, no launchd, no real db path:

- Schedule parser/translator: each field form (`*`, value, list, range,
  step); the cross-product expansion and its cap; both-dom-and-dow
  error; field range validation; 7-for-Sunday rejection; malformed
  expressions (4 fields, 6 fields, garbage).
- Plist generation: the generator takes the binary path as a parameter —
  `plistFor(label string, id int64, binPath string, dicts []calendarDict)
  []byte` — because `os.Executable()` inside a test binary is a temp
  build path no golden file can match; `cron_add` passes
  `os.Executable()`, the test passes a fixed path. Golden-file
  comparison for a representative job (label, fixed binary path, two
  calendar dicts). Every string interpolated into the XML goes through
  an escape helper; tested with a binPath containing `&` and a space
  (the command never enters the plist — only label, path, id).
- eviedb: open a temp-path db (the `OpenDBAt` seam, as
  `sync_test.go:18` does), assert schema applies idempotently (open
  twice), insert/read a job and a run, and that deleting a job with run
  history succeeds and leaves the runs (the amended FK decision — this
  test is what stops the constraint being "helpfully" re-added).
- Tool funcs against a temp db with the installJob/uninstallJob seams
  replaced (signatures in the Interfaces section). Cases: add happy
  path (row present, install called with the right label and plist
  bytes); add with a taken name (error, no install call); add with a
  bad schedule (error, no row); install failure rolls back the row
  (plist cleanup is the seam's job, not asserted here); a rollback that
  ALSO fails reports both errors and names the orphaned job (close the
  seam's connection under cron_add to force it); list with no
  jobs / with jobs / with a never-run job; remove happy path
  (uninstall called, row gone, runs kept); remove unknown name (error
  names existing jobs).

  **The temp db must be built by `eviedb.OpenDBAt`, not a hand-copied DDL
  blob.** Code review found the copy had kept a foreign key production had
  dropped, and opened with its own pragma string that disabled enforcement
  — so "runs kept" passed for the wrong reason and would have stayed green
  if production regressed. This is why `OpenDBAt` is exported.
- cron-exec's runner via `RunScheduled`: a real command's output and
  exit code recorded (echo; exit 3), output cap at 64KB, the -1 path
  (unrunnable shell) — this hits a real shell, no mocking, same as
  bash_test.go. Plus `humanDuration`, which renders the timeout note.
- cron-exec itself (`cmd/evie/cronexec_test.go`, added after review found
  the package had no tests): exit 2 for no/extra/non-numeric args, exit 1
  and a stale-plist message for an unknown id, **exit 0 for a job that
  itself exits 7** with the 7 landing in job_runs, and the job_runs INSERT
  round-tripping output plus ordered RFC3339 timestamps. Driven through
  `cronExec(args, stderr)` with the `openCronExecDB` seam on a temp db.
- Registry: three tools present, none gated; "evie" in query_db's
  enum, absent from edit_db's, and the edit_db error for "evie"
  mentions the cron tools.

## End-to-end verification

1. `go vet ./... && go test ./internal/... ./cmd/...`
2. `go build -o ~/go/bin/evie ./cmd/evie`
3. Ask evie to schedule `echo hello from cron` two minutes from now
   (e.g. `17 14 * * *`). Confirm: `cron_list` shows it; the plist
   exists; `launchctl print gui/$(id -u)/com.evie.cron.<name>` shows
   it loaded.
4. Wait for the fire; `query_db` the evie db for the run row — exit 0,
   output "hello from cron".
5. Schedule a failing job (`exit 7`), wait, confirm the run row records
   exit 7 and cron_list shows it as the last run.
6. `cron_remove` both; confirm plists gone, `launchctl print` says no
   such service, job rows gone, run rows still present.
7. The first customer: `cron_add name=finance-daily schedule="0 9 * * *"
   command="finance sync && finance categorize"` — verify the plist,
   then `launchctl kickstart -k gui/$(id -u)/com.evie.cron.finance-daily`
   to force one run NOW, and check the run row for a real sync (this
   also proves the env/PATH story: finance binary found, Plaid keys
   loaded from ~/.evie/.env).
