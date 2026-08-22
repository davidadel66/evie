# MEM-4 - Durable asynchronous candidate compiler

Status: approved; not started

## Outcome

Continuously derive source-validated semantic candidates from eligible episodic
events without slowing the user turn, losing work on restart, or allowing stale
workers or model output to mutate accepted graph state.

## Specification references

- [Compiler and operation tables](../../memory.spec.md#compiler-and-operation-tables)
- [Memory Compiler](../../memory.spec.md#memory-compiler)
- [Local inference boundary](../../memory.spec.md#local-inference-boundary)
- [Go concurrency model](../../memory.spec.md#go-concurrency-model)
- [Stage 4 - Durable asynchronous memory compiler](../../memory.spec.md#stage-4---durable-asynchronous-memory-compiler)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- A versioned durable job, coverage, lease, retry, cancellation, and skip model.
- Local structured extraction with mechanically local endpoint enforcement.
- Allowlisted and secret-scanned evidence projection with exact citations.
- Conservative entity resolution and candidate-only persistence.
- Candidate review, revision revalidation, and worker diagnostics.

## Out of scope

- Automatic claim admission.
- Remote extraction or a silent remote fallback.
- Dense-vector entity resolution, which follows in MEM-5.
- Retrieval ranking or model-facing memory injection.

## Stories

### MEM-4.R1 - Local extractor and structured-output spike

- Outcome: Select a local extraction model and strict structured-output protocol
  from reproducible evidence.
- Depends on: MEM-3 and representative eligible event fixtures.
- Acceptance summary: Loopback or Unix-socket endpoints work; non-loopback URLs
  and redirects are rejected; selected fixtures cover valid output, malformed
  output, timeout, cancellation, and unsupported schema behavior.
- Verification summary: Reproducible fixture server/model commands, captured
  requests/responses, and the approved model/protocol decision in
  `memory.decisions.md`.
- Proposed PR boundary: Spike client, fixtures, research evidence, and decision
  update only; no production compiler worker.

### MEM-4.1 - Durable job and coverage ledger

- Outcome: Represent compiler work and contiguous evidence coverage durably by
  immutable run/configuration hash.
- Depends on: MEM-3.1 and MEM-4.R1's configuration identity requirements.
- Acceptance summary: Terminal turns can enqueue idempotent ordered work;
  enabling a configuration reconciles coalesced uncovered ranges; no extractor
  creates no permanently pending work; a new hash owns a separate coverage
  stream and candidate group.
- Verification summary: Schema/state-machine, idempotency, old-event backfill,
  coalescing, disabled/enabled configuration, and changed-generation tests.
- Proposed PR boundary: Job/coverage schema and reconciliation store API; no
  coordinator, model call, or candidate persistence.

### MEM-4.2 - Bounded worker lifecycle

- Outcome: Claim and execute durable jobs through a concurrency-bounded,
  cancellable coordinator that survives process loss.
- Depends on: MEM-4.1.
- Acceptance summary: Claims use expiring leases and CAS; retries use the
  approved five-attempt exponential schedule; cancellation is durable; shutdown
  stops claims and leaves unfinished work recoverable after lease expiry.
- Verification summary: Fake-clock lease expiry, retry timing, cancellation,
  bounded-concurrency, shutdown, restart, and goroutine-leak tests.
- Proposed PR boundary: Coordinator, worker lifecycle, job claiming, and
  diagnostics; use a fixture worker rather than the real extractor.

### MEM-4.3 - Fenced per-scope sequencing and coverage

- Outcome: Serialize accepted compiler effects per scope while allowing model
  calls and staging to run concurrently.
- Depends on: MEM-4.2 and MEM-3's scope revisions.
- Acceptance summary: Only the earliest eligible scope sequence can commit;
  unexpired worker token, expected graph revision, and sequence are checked in
  the final transaction; stale workers write nothing; failed/cancelled head work
  blocks later commits until retry or an explicit reasoned skip.
- Verification summary: Two-process races, expired leases, stale commits,
  out-of-order staging, blocked heads, explicit skips, and coverage-contiguity
  tests.
- Proposed PR boundary: Staging/finalization transactions and coverage ordering;
  no evidence projection or model extraction.

### MEM-4.4 - Evidence-safe compiler projection

- Outcome: Produce the only event projection the local extractor may inspect and
  mechanically validate every returned citation.
- Depends on: MEM-4.1 and the accepted event/evidence rules.
- Acceptance summary: Only eligible allowlisted fields enter the projection;
  detected secrets, reasoning, provider state, context snapshots, summaries,
  retrieved-memory echoes, compiler output, and diagnostics are excluded; every
  citation matches an immutable field location and content hash.
- Verification summary: Eligibility matrices, JSON Pointer/byte-range checks,
  secret fixtures, hash mismatch, source-authority preservation, and prompt
  capture tests.
- Proposed PR boundary: Pure projection, redaction, citation validation, and
  fixtures; no entity resolution or candidate write.

### MEM-4.5 - Structured extraction and candidate persistence

- Outcome: Extract typed entities/claims locally, resolve exact or lexical
  identities conservatively, and persist only validated candidates.
- Depends on: MEM-4.R1, MEM-4.3, and MEM-4.4.
- Acceptance summary: Output conforms to the strict schema, cites validated
  evidence, retains extractor/model/prompt versions and immutable scope/base
  revision, prefers duplicates or ambiguity over unsafe merges, and never enters
  accepted traversal or FTS.
- Verification summary: Fixture-backed extraction, duplicate/ambiguous identity,
  corrections/contradictions, malformed output, scope isolation, restart, and
  candidate-group tests.
- Proposed PR boundary: Extractor adapter, resolver, candidate writer, and
  worker integration; no review operation or automatic admission.

### MEM-4.6 - Candidate review and diagnostics

- Outcome: Let David inspect, approve, reject, or detect stale candidates while
  observing durable compiler health.
- Depends on: MEM-4.5.
- Acceptance summary: Approval revalidates scope, sources, conflicts, identities,
  and base revision in the accepted graph transaction; mismatches become stale;
  rejection changes only candidate lifecycle; queue/worker/coverage state is
  inspectable without exposing raw secrets.
- Verification summary: Approval/rejection/stale-revision races, accepted
  operation atomicity, redacted diagnostics, approval-gate, and restart tests.
- Proposed PR boundary: Local candidate review operations and diagnostics; no web
  UI or automatic admission.

## Epic completion evidence

- A committed terminal turn produces source-linked candidates asynchronously and
  the foreground response does not wait for extraction.
- Killing and restarting workers preserves durable ordering, retries, and
  coverage.
- Two processes, stale workers, blocked/skipped heads, and configuration backfill
  behave according to the specification.
- Focused tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- MEM-4.R1 blocks production extractor integration.
- Local model quality may be insufficient; the safe behavior is candidate
  failure or ambiguity, never remote fallback or silent admission.
- Automatic admission remains disabled until MEM-9 produces approved evidence.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.

