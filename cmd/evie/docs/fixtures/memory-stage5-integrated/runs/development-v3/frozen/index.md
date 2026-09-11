# Integrated index diagnostic, procedure version 2

`TestMemoryStage5IntegratedIndexMeasurements` is opt-in through the same absolute
`EVIE_MEMORY_INTEGRATED_FREEZE`, `EVIE_MEMORY_INTEGRATED_INPUTS`, and
`EVIE_MEMORY_READER_ARTIFACTS` paths as the integrated reader. It validates frozen
binary, workload, gate, source-map and seed hashes through `integratedFrozenInputs`.
It never writes to a canonical seed, alters the selected embedding endpoint,
starts a reader generation, or changes a release gate. Run only after the owner
has frozen the complete diagnostic and coordinated an otherwise quiet timing
window. No measured result exists merely because this procedure is written.

Version 2 predeclares mandatory canonical preparation → index → local → operating
→ reader. Start index immediately after mandatory source indexing and sealing;
record actual bounded `/api/ps` state before/after preparation and immediately
before index. There is no additional warmup query, forced unload, retry or
best-run substitution. Model absence is an observation, not permission to skip
or replace a sample. All 24 first-attempt cases must satisfy the unchanged exact
ordered reference comparison, including `Paths`, and every existing numerical
gate. Report startup cost within preparation only when it actually occurred.
This is an index comparison after required indexing, not a promise of cold
first-read dense availability. The same order applies to held-out evaluation.

The frozen v1 failure and supplemental cold/warm diagnostics remain retained:
initial partial dense availability changed an accepted Claim's discovery path
from `lexical` to `lexical,dense` after rebuild while exact source identities and
bytes remained equal. Its failed gate is not normalized away. The immediate
model-visible single-case follow-up demonstrated agreement was possible, but
cannot establish a full-cohort pass or erase the cold-start limitation.

For each frozen case, the output `index-<case-id>.json` contains:

- Original seed preparation's measured index wall time, batch count and coverage,
  copied with its frozen seed hash. This is previously captured preparation
  evidence, not a newly timed cold build.
- Initial database/WAL sizes and derived page accounting; reopened and rebuilt
  coverage and storage snapshots; public rebuild wall time and every bounded
  refresh batch's coverage, including pending work and failures.
- Complete public scripted `Session.Send` traces before restart, after restart,
  and after rebuild. The unchanged full case question is the public user input.
  Accepted and conversation searches use its original prefix through at most
  32 Unicode letter/digit runs and 1024 UTF-8 bytes, preserving punctuation and
  identifiers. This generic limit matches the Kernel query contract and avoids
  vacuous equality of failed overlong queries. Index and local resource probes share `integratedProbeQuery`; all three
  recovery runs use the identical bounded query, using the independently declared oracle intent/date only as
  temporal query metadata. This is a fixed resource/recovery script, not an oracle source
  selector or model-directed interpretation-quality measurement.
- Canonical reference equality across those three runs. Only unconstrained
  per-dispatch read timestamps and dense generation IDs are removed for
  comparison; explicitly constrained knowledge or validity dates remain exact.
  Original event IDs,
  Claim/operation IDs, locators/hashes, authority, lifecycle, scope, identity,
  relation support and ordering remain exact. All full request/receipt checks
  still run before canonicalization. Every delivered source's text and metadata
  must also match current authorized inspection of its exact reference.
- One public owner append and refresh on a fourth clean clone, followed by a
  complete public turn retrieving the newly appended marker and inspection of
  its original source locator. It measures append and incremental refresh
  separately and reports pending work. It does not claim the marker is a new
  accepted fact. A disabled-memory case instead requires both public tool grant
  denials, zero Kernel calls, no synthetic memory receipt and no delivered source
  identity; maintenance does not authorize bypassing that policy. The tool-only
  probe reaches the grant guard before the Kernel, so its denial is recorded
  distinctly from an Automatic Recall unavailable projection.
- Current-process RSS snapshots outside query timing and the signed/max-zero
  incremental delta, with measurement method, OS and PID. The local embedding
  server is a different process and is explicitly outside this RSS metric.
- Separate best-effort RSS observations of the frozen literal loopback listener
  and its owned descendants, before and after each case. These observations are
  not added to the Go worker's growth gate or called total unique physical memory.

The first, restarted, rebuilt and incremental branches use separate pristine
clones of the same closed seed. Prior diagnostic questions therefore cannot
contaminate the other restart/rebuild comparisons. The restart branch closes and
reopens its clone before its first diagnostic turn. The rebuilt branch calls
public `RebuildMemoryEmbeddings`, then bounded `RefreshMemoryIndex` batches. No
derived tables or generation rows are manually removed. Every branch keeps the
case's reader scope and disabled-memory policy during the measured query; local
maintenance is enabled separately so unavailable cases still test recovery of
their retained index.

Storage accounting is declared before measurement. Where SQLite `dbstat` is
available, count pages owned by `memory_dense_*` and `memory_retrieval_*` tables
and their indexes, including FTS shadow tables and retained generations. Report
that derived-page allocation separately from the full database and WAL. The
derived pages plus **all** WAL bytes form a conservative derived-storage upper
bound; WAL can also contain canonical writes, so that upper bound is not labeled
an exact derived-only measurement. It also conservatively bounds incremental
growth because it includes the initial derived allocation.

If `dbstat` is unavailable, report its error and use complete database plus WAL
bytes only as a conservative **whole-file upper bound**, never as a derived
storage estimate. Record full-file changes independently. No canonical bytes
are silently subtracted using an invented allocation model. Compare the maximum
observed upper bound with the frozen 32 MiB gate. Per-case sizes are measured on
isolated databases; totals are not represented as a single combined query index.

RSS uses `ps -o rss= -p <current-test-process-pid>` on the declared supported
platform, interpreted in KiB and converted to bytes. Do not force garbage
collection to improve a sample. Sample before and after public append plus
incremental maintenance, outside retrieval timing, after the previous branches
have exercised the selected endpoint. This is an observed warm-sequence worker
delta, not a guaranteed cold model measurement or the model server's RSS.
Missing RSS measurement is explicit and prevents a successful RSS gate. Report
absolute worker RSS as well as the 512 MiB growth comparison.

For separate endpoint RSS, resolve only the exact frozen literal loopback
listener through `lsof -nP -iTCP@<literal-address>:<port> -sTCP:LISTEN -t`.
Enumerate its descendants through parent-PID queries (`pgrep -P`) and read only
those processes' PID, parent PID and RSS through `ps`. Discovery is bounded to
32 processes and five seconds and runs outside query timing. No all-user command
lines, process arguments or unrelated process details are captured. Record each
process and the observed sum of RSS; shared pages can be counted in more than
one process, so the sum is not unique physical allocation or a peak measure.
Missing tools, ambiguous listeners, child exits or ownership races yield an
explicit unavailable sample. Unix endpoints require separately declared PID
telemetry and are unavailable to this listener-discovery procedure. The first
sample is not asserted cold, and the later sample is not asserted peak usage.

The endpoint remains exactly the frozen literal loopback/Unix endpoint. There is
no observing proxy and no production transport replacement. Inference request
counts and input bytes are recorded as unavailable because the public Store
maintenance API does not expose them and the endpoint has no frozen request
counter contract. Do not infer actual model calls from batch counts or vector
rows. Separate endpoint telemetry can be added only under a new declared
procedure before execution.

Index build/rebuild must be <=30000 ms, derived-storage upper bound <=33554432
bytes, warm incremental worker RSS growth <=536870912 bytes, active coverage
with zero pending work, and exact restart/rebuild reference agreement. Read
these values from the hash-verified gate file. Preserve every failed stage and
partial trace before reporting a failed test. The new diagnostic does not waive
component-scale latency, operating failure, source inspection or repository
verification requirements.

Compile/skip verification and a deterministic real-SQLite/scripted-provider
tracer using the existing scripted local embedding endpoint are permitted before
freeze. Such a tracer is labeled model-free and is not saved as quantitative
pilot evidence. Actual opt-in measurement awaits the frozen run.

The model-free setup traces are retained under `/tmp/evie-memory-stage5`.
An initial unavailable-policy tracer incorrectly expected a synthetic receipt;
the complete request showed the existing read-grant denial before the Kernel,
so the assertion now verifies that precise policy outcome. The added historical
tracer then exposed a real diagnostic setup failure: the original full question
exceeded the public 32-term search limit. The generic bounded-prefix procedure
was declared and shared with the local resource script before freezing or any
actual reader outputs. The full public question and independently labeled gold
sources remain unchanged. The historical tracer requires the original retired
Claim, original source inspection and exact explicit valid-time constraint after
rebuild, so empty or failed searches cannot satisfy recovery by equality.

Pre-freeze verification on the complete diagnostic source:

```sh
env -u EVIE_MEMORY_INTEGRATED_FREEZE go test ./internal/agent -run '^(TestMemoryStage5IntegratedFixtureSmoke|TestMemoryStage5IntegratedIndexMeasurements|TestIntegratedIndexDiagnosticPreservesPublicSourcesAndUnavailablePolicy)$' -count=1 -v
go vet ./internal/agent
git diff --check -- internal/agent/retrieval_integrated_index_test.go internal/agent/retrieval_integrated_probe_test.go internal/agent/retrieval_integrated_local_test.go cmd/evie/docs/fixtures/memory-stage5-integrated/index-procedure.md
```

Result: PASS, `internal/agent` 7.111s; all 24 development cases across six
conditions and the three enabled/historical/unavailable index tracers passed.
The actual index measurement test skipped because no immutable freeze was
provided, as intended. Vet and the scoped whitespace check passed. Logs are
`/tmp/evie-memory-stage5/167-shared-probe-model-free.log` and
`/tmp/evie-memory-stage5/167-index-vet.log`. No selected-endpoint timing run or
reader model generation was performed by these checks. Full repository
verification remains the implementation owner's handoff check.
