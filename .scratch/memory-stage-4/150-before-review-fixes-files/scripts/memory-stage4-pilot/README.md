# Stage 4 integrated pilot measurements

This is experimental tooling for [ticket #150](https://github.com/davidadel66/evie/issues/150).
It measures the actual Kernel, SQLite, compiler host, Agent.Send and owner-review
paths against disposable synthetic load. It does not select a learned model,
grade meaning, or activate compilation in the owner's database.

Run the matrix from a frozen checkout after the complete Stage 4 conformance
runner passes:

```sh
python3 scripts/memory-stage4-pilot/measure.py \
  --conformance /absolute/path/to/conformance/report.json \
  --output /absolute/path/to/new-pilot-results \
  --repetitions 3
```

The output directory must not exist. The wrapper builds and hashes the Go
executable, checks the conformance source inventory against current Go source,
freezes a plan before work, and records a source inventory again afterwards.
Each report identifies the complete scripted generation, commands, workload,
environment and repetitions. Development corpus, gold and custody hashes bind
the experiment to the spike protocol without passing any corpus text to the
scripted extractor. Its artificial load is a separate infrastructure fixture.
`--only baseline,retained-events-1000000` selects declared variants for diagnosis;
it does not make an incomplete matrix a completed pilot.

Each variant runs compilation disabled, explicit new-evidence activation, and
new evidence competing with explicitly selected history. Order rotates across
repetitions. All modes contain the same declared source history and graph.
The baseline uses 10,000 archived events, 256-byte foreground inputs, one accepted
Claim, one exact session destination, a 25ms scripted inference delay, one worker
process, sixteen foreground turns and sixteen potential historical roots. Each
variant changes only one factor: 100,000 or 1,000,000 archived events; 4,096 or
12,000 input bytes; 100 or 1,000 accepted Claims; sixteen exact session
destinations; 0 or 250ms service delay; or two cooperating worker processes.

Archived events are loaded in bounded transactions outside measured intervals;
they retain the real schema, indexes and triggers. Foreground conversations are
separate, small sessions. Graph fixtures use public approved Semantic Operations.
This experiment measures retained database size rather than feeding a million
events to the foreground context composer. Session destinations vary scope
distribution; Workspace and project authorization remain deterministic
conformance cases, not additional measured throughput claims.

Workers use the production `RunCompilerHost` with its independent reconciliation
and extraction loops. The scripted service records each actual dispatch interval;
overlapping extraction intervals fail the run. The historical fixture contains
one failed older root, one zero-candidate success and later independent work.
Failed-gap state remains visible. The workload stops after finite catch-up,
not after assuming a contiguous frontier means success. Cancellation, crash,
stale delivery and retry recovery remain in the hashed #149 conformance report.
Measured attempts and actual outcomes remain available in each job's diagnostics.

Every measured foreground turn calls real `Agent.Send` with a constant local
conversational response. The terminal commit observation comes from the real
History adapter. Response finalization is marked only after the host writes its
committed response to `os.DevNull`; it is an actual host boundary with no browser,
SSE or network transport. The host uses the same fixed 50ms inter-turn delay in
all modes. These samples isolate infrastructure overhead and do not represent
conversational-provider latency. Browser and HTTP parity are separately checked
by deterministic conformance.

JSON retains queue, inference, validation, database completion, publication,
freshness, foreground commit/finalization and scripted review-operation samples.
Missing observations remain null. Counts distinguish selected and completed
events, no-candidate coverage, failed work, candidate backlog and attempts.
Database, WAL and shared-memory sizes are sampled before and after each run.
The wrapper samples the complete pilot process tree's RSS and CPU every 100ms,
including setup. These are sampled observations, not exact process peaks.
No model server exists in this mode, so its resource field is null.

Source-arrival rate and observed completion rate are reported separately.
Candidate-arrival rate and active human review capacity are also separate.
The latter remains null until actual owner sessions. Finite workload throughput
does not certify sustained capacity, and a candidate's unresolved age does not
measure human time. Scripted acceptance/rejection measures database resolution
cost only. It cannot supply useful precision, required recall, review capacity,
or an owner quality judgment.

## Real owner session preparation

Use the existing Stage 4 review surface for actual decisions. Start the explicit
timer in another terminal with the hash of the pinned integrated pilot
configuration. This command does not modify memory or make review decisions:

```sh
go run ./scripts/memory-stage4-pilot review-session \
  --operator David \
  --configuration-sha256 EXACT_64_HEX_CONFIGURATION_HASH \
  --output /absolute/path/to/new-private-owner-observations.jsonl
```

Enter one JSON command per line. The following values are placeholders to replace
with the exact inspected candidate and generation, not real observations:

```json
{"command":"start","candidate":{"candidate_id":"EXACT_CANDIDATE","interpretation_revision":1,"review_revision":0},"scope_key":"EXACT_SCOPE","generation_id":"EXACT_GENERATION"}
{"command":"pause"}
{"command":"resume"}
{"command":"finish","action":"defer","reason":"Need to resolve which Maya this refers to.","useful":null}
```

Use `accept`, `edit`, `reject`, or `defer` and the actual reason. State-changing
actions require `review_receipt`, pointing to the actual Evie decision/edit
receipt. Pause while distracted; resume when actively reviewing. The timer sums
only explicit active intervals, using Go's monotonic clock. It writes completed
observations immediately to an exclusive mode-0600 file. An unfinished candidate
is not silently converted into an observation on EOF or cancellation.

Operator identity, reasons, useful judgments and external receipt references
are self-attestations. `receipt_verified:false` remains in the observation until
a separate evaluation process verifies it against the exact Kernel record.
Do not count an agent's synthetic timer test as David's session. Do not commit
private source material or personal reasons without the owner's direction.

Use only development or pilot narratives under the spike's pinned scoring
contract. Preserve raw and retained proposal panels separately. The already
exposed development window IDs are recorded in the [pilot preparation
record](../../cmd/evie/docs/fixtures/memory-stage4-pilot/v1/preparation.json). The final holdout remains uncreated and unexposed. No numerical release
gate or chosen configuration is supplied by this infrastructure command.

## Verification

```sh
go test -count=1 ./scripts/memory-stage4-pilot
go test -race -count=1 ./scripts/memory-stage4-pilot
go vet ./scripts/memory-stage4-pilot
python3 scripts/memory-stage4-pilot/measure_test.py
./scripts/verify-change.sh
```

The smoke test uses real temporary SQLite and two real worker processes. It
checks disabled isolation, measured host boundaries, failed/empty/later work,
review resolution and cleanup. Timer tests prove pauses are excluded and missing
receipts/incomplete observations are refused. Python tests prove factor
independence, absent-metric semantics and refusal of incomplete conformance or
changed source. Workload measurements remain separate from these small checks.
