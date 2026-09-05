# Compiler health and measurements (#148)

`memory-health` and the web Memory → Compiler health tab use the same typed
owner Kernel projections. They do not select work, activate a generation, infer
an identity, accept a candidate, configure a model, or expose a SQL/file browser.
The existing generic SQL/file containment and privileged local-shell limitations
remain unchanged.

## Inspecting progress

List source session IDs in one exact destination, including closed sessions:

```sh
evie memory-health --scope global
```

Use the selected source session and one view. Scope is `global`, `workspace:ID`,
`project:ID`, or `session:ID`; a destination does not authorize another source
lineage. No conversation title, source text, candidate prose, model reasoning,
prompt/schema, endpoint, server identifier, or continuation state is returned.

```sh
evie memory-health --scope global --session SESSION_ID --view jobs
evie memory-health --scope global --session SESSION_ID --view candidates
evie memory-health --scope global --session SESSION_ID --view activations
evie memory-health --scope global --session SESSION_ID --view live_roots
evie memory-health --scope global --session SESSION_ID --view history
evie memory-health --scope global --session SESSION_ID --view selections
evie memory-health --scope global --session SESSION_ID --view selection --generation GENERATION_SHA256
evie memory-health --scope global --session SESSION_ID --view foreground
```

`--limit` is 1–32. Supply the returned `next_cursor` as `--cursor` to advance;
start without it to refresh. Cursors authenticate the current owner scope,
authorization revision, source session, view and generation. A page is one
transaction snapshot with its own `as_of_unix_ms`; a cursor does not freeze later
pages. `revision` is the scoped job/review counter revision. Activation and
history objects carry their own revisions, so they can change independently.

The job view distinguishes queued, running, retry-wait, staged, cancelled,
failed, completed-empty, completed-candidates and excluded. An unavailable
configured extractor leaves a retry/gap and a safe recovery description. A later
job completing does not complete that gap. A saved stage can publish without
another inference. `capacity_blocked` means release is still unknown, including
when a local request timed out; this surface has no force-release operation.

The selections view also includes units which could not become jobs. Live roots
show pending extensions, deferred live turns and source/configuration failures.
History ranges report immutable selection bounds and the discovery cursor;
`scanned_sequence` is not a completion frontier. Activations show selected
frontiers, pause state and generation IDs. A generation ID identifies durable
policy, not proof that a runtime is configured, reachable or semantically good.

Per-job `selected_new_events` and `completed_new_events` count that unit's
explicit new-event/coverage manifests. These are newly covered source-unit
members, including assistant/control events, not a count of factual support.
Its first/last coordinates bound those members; unrelated interleaved events and
previously covered overlap are not silently counted as new members. Do not sum
overlapping units/generations to invent global coverage.
The selection view separately pages exact event membership for one generation:
selected live, selected history, or outside selection. Cancellation retains the
original historical selection; it does not turn selected evidence into outside
selection. No maximum/later-success cursor is presented as a contiguous completed
frontier.

## Bounds and persistence

Normal health polling performs no full-session, retained-job, retained-candidate
or graph aggregate scan. The counter row is updated in the same transactions as
job/review changes. A partial batch rolls back that failed group's counters and
decision timestamp with its savepoint; successful independent groups count once.
Exact delivery retries and reopen do not count another decision. Candidate
presentation counts exclude suppressed recurrences from unresolved backlog.
Older installations reconcile at most 15 jobs per separate transaction (at most
61 ledger mutations including counter triggers and cursor); historical timestamps
remain unavailable. `indexing: true` means totals are partial until that pass
finishes. No retained source, candidate or accepted operation is rewritten. Selecting
activation/history or queueing a job makes its destination navigable before any
candidate exists. Older selection scopes reconcile in separate 15-record pages
without a retained-history DISTINCT scan.

Each view seeks indexed coordinates. Jobs inspect at most 32 rows, at most five
attempt records per job and bounded manifests. Candidate pages visit at most two
jobs and 32 candidates; a job with no candidates can produce an empty page with a
next cursor. Live-root pages also advance across bounded rows belonging to another
destination without disclosing them. Activation pages merge two independently
bounded exact-selector index seeks. Selection membership reads at most 32 event
identifiers, one exact history-reference lookup and two activation-interval
predecessor seeks per event. Results have a 128 KiB serialized ceiling; an oversized
response fails rather than truncating content. No evidence is read by these views.

Publication keeps the 16-candidate atomic group. Diagnostic state refresh and its
counter add two actual ledger writes to #147's maximum 53, for 55, within the
64-mutation contract. Measurement persistence runs in separate bounded
transactions. Reservations remain the existing shared 128-stage/16 MiB and
2,048-presentation limits; capacity status never identifies another scope's work.

Legacy `memory-backfill status` and activation inspection also use bounded
incremental projections. Large existing sessions can return
`ErrCompilerStatusIndexing`; repeat the same status request to continue the
persisted count projection. Incomplete totals are unavailable, never capped or
presented as complete. Current counts remain separate from coverage frontiers.
The compatibility projection is installed at startup without computing totals.
Initial SQLite CREATE INDEX operations build their indexes over retained metadata
once; subsequent status work follows bounded indexed pages. Include first-index
installation time separately when measuring an upgraded database.

## What the timings mean

All durations are nonnegative nanoseconds; wall-clock timestamps are Unix
milliseconds. SQL admission timestamps have millisecond resolution. A missing
observation is null/absent, never a zero or a successful quality observation.

- Queue wait uses the job queue/retry-due transaction clock and the attempt claim
  transaction clock, at millisecond resolution. It excludes the configured retry
  delay. Missing or backward wall-clock order remains unavailable.
- Inference measures the actual extractor call, including its transport lifetime.
  Validation measures deterministic decoding/proposal validation after extraction.
- Database completion measures staging/source validation and publication through
  completion of their transaction calls. Publication has its own duration and a
  wall-clock timestamp sampled after its COMMIT returns. Saved-stage adoption
  records publication without inventing a second inference.
- A claimed attempt that loses its process keeps incomplete/null elapsed timings.
  Fenced observations are marked stale, and cancellation is distinct from a failed
  or completed attempt. Operational attempts/cancellations are not model quality
  labels. Sum only observed phase timings when reporting stale wasted work, and
  disclose missing observations.
- Candidate freshness is publication time minus an actual observed foreground
  terminal commit for the same source root/session. It is null for unobserved
  historical closure, absent terminal measurements, or backward wall-clock order.
- Foreground terminal commit measures the actual final no-tool assistant Append,
  or the failed/interrupted terminal Append. Intermediate tool-request assistant
  messages do not end the turn. Response finalization measures Send start through
  successful host output handling and flushing (SSE turn_done or REPL output).
  An observed write, short-write or flush failure leaves both finalization fields
  unavailable while preserving the terminal commit and its outcome. This is a
  host I/O boundary, not confirmation that a remote client read the response. The
  two timings are separate; neither is first-token latency. Diagnostic persistence
  begins after the finalization clock sample and has a 500 ms timeout. A recording
  failure emits a fixed content-free host diagnostic and leaves measurement
  unavailable; it cannot change an already committed response into failure.
- Inbox age uses the observed publication timestamp and current snapshot time.
  Decision timestamps are transaction audit clocks. Edited status and exact
  interpretation/review revisions preserve lineage. None measures active human
  review seconds. The pilot must separately record David's start/stop/focus
  intervals and accepted/edit/reject/defer actions; agent test clicks are not human
  review observations. Approval rate is not accuracy.

## Repeatable resource observation

Declare before a run: machine/OS, Evie revision, model artifact/runtime/config
identity, database fixture, event/graph/scope counts, foreground workload, source
arrival schedule, generation, historical selection and candidate baseline. Keep
separate no-compiler, ongoing-new and historical-catch-up runs with the same
foreground requests. Record model-server and Evie PIDs separately; inspect
numeric resource columns without dumping command arguments or environments:

```sh
ps -p EVIE_PID,MODEL_SERVER_PID -o pid,ppid,pcpu,rss,vsz
stat -f '%z' /exact/pilot/database.db
stat -f '%z' /exact/pilot/database.db-wal
```

RSS/VSZ from macOS `ps` are KiB; CPU is a sampled process percentage. Record sample
intervals and missing samples. On Linux, use `stat -c '%s'` for file bytes. Model
work can outlive its client, so continue sampling the contracted server until its
release disposition is known. Do not assume an HTTP cancellation means idle.
Do not include full process arguments, environment, logs, database rows or source
text in exported diagnostics.

Measure DB/WAL bytes before/during/after the same workload; avoid checkpoints,
vacuum, purge or compaction during a comparison unless explicitly part of the
experiment. Fixture construction records exact event, graph, candidate and scope
counts. An evaluator can perform deliberate offline read-only counts before/after
a run, outside measured request/status paths; such scans are not normal health
polling. Report candidate growth, raw retained history and accepted graph growth
separately. No total retained database-size bound or automatic deletion is claimed.

Use queue/publication observations to compare eligible source-unit arrival with
compile service capacity, and candidate arrival with independently measured human
review capacity. Measure catch-up backlog over the fixed schedule; a falling queue
while the unresolved inbox grows is not sustainable human review. Report paired
foreground distributions, candidate freshness, missing timing coverage, process
resources and DB/WAL growth with the exact workload and fixture sizes. These
operational measurements do not establish extraction precision/recall, source
correctness, temporal correctness or owner usefulness; those need the approved
rubric and still-pending human/model evaluation gates.
