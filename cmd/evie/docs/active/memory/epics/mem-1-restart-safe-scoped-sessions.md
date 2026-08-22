# MEM-1 - Restart-safe scoped sessions

Status: approved; in progress

## Outcome

Make the existing agent loop restart-safe and observable while ensuring that one
durable lease holder owns a live session and uncertain side effects are never
replayed or silently resolved.

## Specification references

- [Invariants 1, 2, 6, 12, 14, and 15](../../memory.spec.md#invariants)
- [Scope And Authority](../../memory.spec.md#scope-and-authority)
- [Session and event tables](../../memory.spec.md#session-and-event-tables)
- [Stage 1 - Session identity and append-only events](../../memory.spec.md#stage-1---session-identity-and-append-only-events)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- Explicit REPL scope selection and durable session selection.
- Cross-process turn leases and fencing.
- Provider interruption evidence and usage capture.
- Rebuildable execution state, unknown-side-effect blocking, and explicit
  recovery.
- Provider replay evidence needed to choose the safe recovery path.

The existing event spine, scope value, history projection, event ordering,
cancellation propagation, and generic storage fences are completed foundation.

## Out of scope

- Context compaction.
- Semantic graph state or memory retrieval.
- Web session selection, which belongs to MEM-8.
- Persisting opaque provider state without a separately approved encryption and
  key-management policy.

## Stories

### MEM-1.R0 - Reference-system adaptation note

- Outcome: Complete the Stage 0 evidence required to distinguish the memory
  layers and record which reference-system behavior Evie is deliberately
  adapting or rejecting.
- Depends on: Access to the cited MemGPT, Generative Agents, and current Letta
  sources plus a reproducible local Letta inspection path.
- Acceptance summary: Record exact paper sections and Letta version/commit,
  commands, observed MemFS/compaction/Git/dreaming behavior, differences from the
  papers, and an Evie applicability matrix; obtain David's approval of the note.
- Verification summary: Re-run documented commands or fixtures, verify every
  factual observation has a source/version, check affected links, and run
  `git diff --check`.
- Proposed PR boundary: Research note and approval record only; no production
  code, schema, dependency, or behavior change.

### MEM-1.1 - Explicit REPL scope selection and new scoped sessions

- Outcome: Replace unconditional global-session creation with an explicit startup
  choice that creates a confirmed global or project-scoped session.
- Depends on: Existing project registry, canonical-root lookup, session store,
  and immutable `ScopeContext`.
- Acceptance summary: A matching launch cwd may suggest one active project but
  never grants it silently; unmatched cwd offers explicit registration or global
  scope; later bash cwd changes cannot alter the selected scope.
- Verification summary: Table-driven startup/terminal tests, project-store scope
  tests, `go test ./cmd/evie ./internal/eviedb ./internal/memory`, `go test ./...`,
  and matched/unmatched cwd demonstrations.
- Proposed PR boundary: Startup and REPL selection plus the minimum store query
  seams; no existing-session resume, leases, execution recovery, or graph work.

### MEM-1.2 - Durable turn-lease storage

- Outcome: Provide durable acquire, heartbeat, release, expiry, and monotonic
  fencing-token operations for session turn ownership.
- Depends on: Existing session schema and SQLite connection guarantees.
- Acceptance summary: One current owner is observable across connections;
  expired ownership can be replaced atomically; stale tokens cannot renew,
  release, or authorize writes.
- Verification summary: Store and schema tests with controlled time,
  cross-connection contention, stale-token cases, and `go test ./internal/eviedb`.
- Proposed PR boundary: Lease schema, domain values, store API, and deterministic
  tests; no agent/provider/tool integration.

### MEM-1.3 - Lease-owned agent turns

- Outcome: Require a durable lease before provider or tool work and cancel local
  turn execution when ownership is lost.
- Depends on: MEM-1.2.
- Acceptance summary: Every required append and tool start checks the live
  fencing token; a late response from a lease-lost provider call is discarded;
  provider failure and interruption events are appended fail-closed.
- Verification summary: Agent/store integration tests for lease races, heartbeat
  loss, late provider responses, append failures, and tool-start fences plus
  `go test ./...`.
- Proposed PR boundary: Agent-owned turn-ownership interface and its SQLite
  implementation; no REPL resume chooser or unknown-execution resolution UI.

### MEM-1.4 - Safe session selection and resume

- Outcome: Let the REPL list and resume an existing scoped session after restart
  without bypassing durable ownership.
- Depends on: MEM-1.1 and MEM-1.3.
- Acceptance summary: Selection preserves the session's immutable scope/root
  snapshot, rebuilds provider-neutral history in order, and reports a live lease
  conflict rather than opening a competing turn.
- Verification summary: Restart, relocation-snapshot, global/project selection,
  and competing-process tests plus a manual resume demonstration.
- Proposed PR boundary: Session listing/resume store queries and REPL flow; no
  execution-resolution behavior.

### MEM-1.5 - Execution projection and unknown-side-effect gate

- Outcome: Rebuild execution status from immutable events and block continuation
  when a tool intent has no terminal evidence.
- Depends on: Existing event spine.
- Acceptance summary: Succeeded, failed, cancelled, resolved, and unknown states
  project deterministically; unknown executions survive restart and block new
  provider/tool activity without being replayed.
- Verification summary: Projection rebuild tables, malformed/duplicate sequence
  cases, restart tests, and `go test ./internal/eviedb ./internal/agent`.
- Proposed PR boundary: Execution projection and harness gate; no manual
  resolution command or provider-chain decision.

### MEM-1.R1 - Provider continuation and replay spike

- Outcome: Establish whether an interrupted OpenRouter/Kimi tool chain can safely
  resume without opaque reasoning or continuation payloads.
- Depends on: Existing captured-provider fixture seams; access to the selected
  provider/model for the bounded experiment.
- Acceptance summary: Record exact requests, model/provider versions, observed
  continuation requirements, cancellation behavior, and the approved fallback in
  `memory.decisions.md`; do not persist opaque state.
- Verification summary: Reproducible fixture commands and captured request tests
  that distinguish provider-neutral restart from same-chain continuation.
- Proposed PR boundary: Research fixtures, note, and decision update only; no
  production recovery implementation.

### MEM-1.6 - Explicit unknown-execution resolution

- Outcome: Let David resolve an unknown execution by appending
  `assumed_succeeded`, `assumed_failed`, or `abandoned` evidence.
- Depends on: MEM-1.5 and MEM-1.R1.
- Acceptance summary: Resolution is append-only and idempotent, never invents
  unavailable output, and either resumes safely or starts a new provider-neutral
  turn according to the approved replay decision.
- Verification summary: State-transition, duplicate-resolution, restart,
  synthetic-result, and provider-request tests plus a manual crash/recovery demo.
- Proposed PR boundary: Local resolution commands and agent recovery behavior;
  no web UI.

### MEM-1.7 - Provider usage persistence

- Outcome: Preserve provider usage as immutable terminal-turn evidence for later
  context and evaluation diagnostics.
- Depends on: MEM-1.3's terminal provider-event path.
- Acceptance summary: Successful and failed turns record all available usage
  fields without treating opaque provider state as semantic evidence; absent
  provider usage remains explicitly absent.
- Verification summary: Captured response fixtures, event round trips, restart
  checks, and `go test ./internal/openrouter ./internal/agent ./internal/eviedb`.
- Proposed PR boundary: Provider response mapping and event payloads only; no
  budgeting or evaluation policy.

## Epic completion evidence

- A scripted two-process race proves that only one live lease holder can start or
  commit a turn.
- A global and a project-scoped session each resume after process restart with
  their original scope and ordered provider-neutral history.
- A deliberately interrupted tool execution survives restart, blocks
  continuation, and proceeds only after an explicit resolution event.
- All focused tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- MEM-1.R0 owns the missing Stage 0 reference-system research note and must be
  completed before this epic is closed.
- MEM-1.R1 may prove same-chain continuation impossible without opaque state; the
  already-approved safe fallback is a new provider-neutral turn.
- Durable leases fence accepted work but cannot recall provider bytes already
  sent remotely.
- All story contracts are queued on GitHub; MEM-1.1 is selected first, and every
  later story remains gated by its named dependencies and material decisions.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.
