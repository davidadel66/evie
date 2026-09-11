# Investigation development diagnostics (#162)

Runs v1 and v2 preserve thirty complete scripted turns each, exact per-request serialized byte counts and SHA-256, original source IDs/locators and immutable receipts. The test validates those receipts against actual provider messages. No remote model, embedding endpoint, answer-quality metric or release-held-out cases are used.

Start from the implementation commit mapped to #162 in the PR. For either run, extract that run's `frozen-go-inputs.tar.gz` over an isolated checkout, then verify every `files_sha256` entry in `freeze.json`. The archives retain exact Go/module inputs even after later review fixes are folded into issue commits. Non-Go repository files come from that owning commit. Verify `archive.sha256` before extraction. Build with the recorded Go version using `go test -c -o /tmp/investigation-tests ./internal/agent`. Binary hashes identify the original local binaries; a different checkout path can affect a rebuild hash. Source hashes are the portable input check.

Before each original measurement, the versioned freeze was written and the binary compiled. The exact command was:

```sh
EVIE_MEMORY_INVESTIGATION_MEASURE_DIR=/Users/davidboktor/code/evie-memory-stage-5/cmd/evie/docs/fixtures/memory-stage5-investigation/v2 /tmp/evie-memory-stage5/investigation-v2-tests -test.run '^TestMemoryInvestigationDevelopmentMeasurements$' -test.count=1 -test.v
```

v1 used matching `v1` output/binary names. For reproduction, use a new output directory; do not overwrite these original results. The fixture's report schema remains `investigation-development-v1`; the freeze's v1/v2 label identifies the implementation run. Normal `go test` skips this opt-in diagnostic with an explicit reason.

Percentiles use nearest rank (sorted sample at `ceil(p*N)-1`), with no warmup exclusion and all thirty samples included. Each sample is a fresh reader in one growing SQLite corpus. Refresh occurs outside each timed turn and has its own samples; its maximum is the initial forty-two-record backfill. Complete-request size is `openrouter.RequestBytes`; memory delivery is actual escaped synthetic-memory and replayed current-turn retrieval-outcome messages. Repeated requests are not independent evidence: two original targets remain exactly two sources.

v1 PASS 1.03s; v2 PASS 0.96s. v2 adds canonical new-conflict invalidation while preserving explicit knowledge pins and adds the HTTP regression; no numerical gate, workload or expected result changed. Final v2 has 120 requests, 90 receipts, zero target/accounting failures, whole-turn p50/p95 24.529/28.110 ms and Kernel-work p50/p95 11.260/12.546 ms. Maximum complete request 25,396 B, memory/request 3,985 B and cumulative/turn 11,955 B. This validates the chosen bounded reuse diagnostic and does not establish production-reader quality or #167/#168 readiness.


Version v3 preserves the later review correction for restored conflicting Claims
or Source Links and newly added same-area owner statements. It uses the same
workload, bounds, percentiles and expectation counts. Its isolated pre-#166
snapshot passed the public regression in 1.780 s before the frozen executable
was measured. The actual executable SHA256 is
`247357e08bde46f20f7b92bd87b602e9467ab7377d4797c6be28529fd37fb25d`.
Its `change_from_v1` field retains the earlier v2 annotation; the v3 `scope` and
hashed source inputs include the additional restored-support/owner-statement
changes. No original freeze or measurement has been rewritten after execution.

```sh
EVIE_MEMORY_INVESTIGATION_MEASURE_DIR=/Users/davidboktor/code/evie-memory-stage-5/cmd/evie/docs/fixtures/memory-stage5-investigation/v3 /tmp/evie-memory-stage5/investigation-v3-tests -test.run=^TestMemoryInvestigationDevelopmentMeasurements$ -test.count=1 -test.v
```

v3 PASS 1.07 s; 30 turns, 120 requests and 90 receipts. Whole-turn p50/p95
28.078/32.094 ms; Kernel-work p50/p95 13.114/14.185 ms. Index initial refresh
19.059 ms; refresh p50/p95 0.290/0.508 ms. Maximum complete request 25,396 B,
memory/request 3,985 B and cumulative/turn 11,955 B. All original evidence,
unchanged-reuse and actual-byte checks pass. Concurrent test/model work was
paused during this timing run. No reader-quality or held-out claim is made.
