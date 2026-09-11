# Integrated operating diagnostic: fixed procedure

This procedure is declared before executing the opt-in operating measurement.
It supplements the integrated pilot with actual cancellation, live lease-loss
and caller-deadline tails. It measures public `Session.Send` behavior with real
SQLite and a scripted provider. It makes no reader-quality, inference-speed or
embedding-model claim.

## Frozen sample and gate contract

- Run exactly **20 repetitions of each of three conditions**, for 60 samples.
- Use the fixed condition list `cancellation`, `lease_replacement`,
  `caller_deadline`. For repetition `r`, visit the list starting at `r % 3` and
  wrap once. Run cases serially, without concurrent load or performance tests.
- Each case creates a fresh real SQLite database, accepts one owner statement
  (`spinel operating evidence`) through public preparation and approval,
  reconciles lexical indexes, and opens a fresh reader session.
- The embedding endpoint is explicitly unset. Automatic Recall is disabled by
  the existing model-directed fixture constructor. The provider first requests
  `memory_search` for `spinel`; the second actual request must already contain
  the exact accepted Claim and Source Link/event reference.
- Only after that second request reaches the scripted provider, select the
  operating boundary. Its response then attempts a conversation search and a
  syntactically valid approved-retirement request. A working fence must stop
  both attempted effects.
- Every condition has the same fixed **maximum tail of 1000 ms**. This is a
  per-sample maximum, not an average or percentile allowance. Every deterministic
  contract must pass; missing samples and incomplete boundaries fail readiness.

The proposal resource keys are `operating_repetitions=20` and
`cancellation_tail_ms_max=1000`; the latter gate applies equally to cancellation,
lease replacement and caller deadline. No other proposal threshold changes.

## Timing boundaries

Cancellation starts immediately before invoking the actual caller cancellation
function. Lease replacement starts immediately before releasing the reader's
live SQLite lease, then acquiring its replacement through the public Store API;
the tail includes those two SQLite operations. The old turn's cleanup must leave
the replacement lease unchanged.

The caller deadline is the actual `context.WithDeadline` instant, fixed at two
seconds after the time recorded immediately before `Session.Send`. Setup,
indexing and Session construction are complete before this clock starts. The
scripted provider waits on that context's actual `Done` channel. Deadline tail
starts at the declared deadline, so scheduler wake-up and cleanup delay remain
included; it does not start when the provider notices cancellation.

All tails end immediately when `Session.Send` returns, before post-return
inspection or fixture cleanup. Tail and whole-Send durations are retained in
integer nanoseconds using Go's monotonic clock. Wall-clock boundary/return
timestamps and the exact deadline are also retained. The two-second wait itself
is included in whole-Send duration and excluded from deadline tail. Negative
tails fail the gate.

## Observable boundary checks

Every sample must have exactly two actual provider calls, zero approval calls,
and the expected cancellation/deadline/lease-loss error classification. The
post-return accepted Claims, sources and canonical scope revisions must equal
their pre-boundary state. No tool event or context snapshot may be appended or
changed after boundary selection. There must be exactly one source-bearing
memory receipt; it must preserve the supplied Claim/source references, one
search attempt, and the exact second request's encoded byte count and SHA256.
The expected Claim, Source Link and original event IDs are recorded separately.

Each sample retains the exact encoded provider requests, before/after durable
session events, accepted scope revisions, source-bearing references, actual
terminal error and classification, and replacement lease where applicable.
These allow the recorded pass/fail findings to be checked against actual effects.

## Freeze, execute and retain

The root runner must freeze the executable and all compiled inputs, this
procedure, proposal gates and declared hardware **before** the measured run.
Record the executable hash, operating system, hardware, runtime, exact command
and environment, standard output, standard error and exit code. The test also
records Go version, OS/architecture, logical CPUs and `GOMAXPROCS`.

Execute the frozen test binary with only
`TestMemoryStage5IntegratedOperatingMeasurements` selected, a suitable overall
test timeout (the twenty real deadline waits require at least forty seconds),
and `EVIE_MEMORY_INTEGRATED_OPERATING_OUTPUT` naming an **absolute, nonexistent
output directory whose parent exists**. An existing directory is an error.
Never overwrite an attempt or modify gates after seeing its results. Without
that variable the measured entry point skips and generates no artifact.

The test writes `configuration.json` before samples. It appends and syncs each
raw sample to `samples.ndjson`, including failures, then writes `report.json`.
Failures do not stop later conditions or repetitions. A subtest that aborts
during setup still records an incomplete sample and a failure; preserve the
runner's execution log for the original fatal error. Write failures also fail
the run and are recorded in the final report where storage permits. An external
process kill can leave only the already flushed journal; missing samples make
that attempt incomplete, never passing.

The report uses nearest-rank p50/p95 and maximum over **all completed samples,
including failed samples**. It reports incomplete/failed counts and individual
failure records. Readiness requires 20 completed samples per condition, no
deterministic failure, and every tail in `[0, 1000 ms]`. An observed high tail is
retained and fails its condition; it is not dropped as an outlier.

The ordinary `TestMemoryIntegratedOperatingBoundaries` tracer runs one case per
condition without an output directory. It checks behavior only and provides no
operating-performance result or evidence that the twenty-sample gate passed.
