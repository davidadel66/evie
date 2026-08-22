# MEM-2 - Bounded working context and compaction

Status: approved; not started

## Outcome

Separate canonical durable history from the bounded request projection sent to
the model, and compact large tool-heavy sessions without breaking legal message
or call/result boundaries.

## Specification references

- [Working memory](../../memory.spec.md#working-memory)
- [Invariants 1, 2, and 9](../../memory.spec.md#invariants)
- [Stage 2 - Working context and compaction](../../memory.spec.md#stage-2---working-context-and-compaction)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- Approved model-window, reserve, estimation, and cut policies.
- A pure context composer with configurable budgets and diagnostics.
- Manual and automatic compaction with durable boundary/snapshot events.
- Complete tool-call/result structure and explicit summary-failure behavior.

## Out of scope

- Semantic retrieval and `EVIE_MEMORY_DATA`, which belong to MEM-5.
- Procedural instruction loading, which belongs to MEM-7.
- Deleting or rewriting canonical episodic events.

## Stories

### MEM-2.R1 - Context and compaction policy decision

- Outcome: Fix the material policies needed to implement bounded context without
  guessing.
- Depends on: MEM-1 and representative provider/tool-heavy transcripts.
- Acceptance summary: Record model-window and reserve defaults, estimation error
  handling, legal cut boundaries, split-turn policy, and summary-failure fallback
  in `memory.decisions.md`.
- Verification summary: Worked boundary examples demonstrate that every policy
  has deterministic behavior at, below, and above the budget.
- Proposed PR boundary: Decision and supporting research fixtures only; no
  production composer.

### MEM-2.1 - Pure context composer and diagnostics

- Outcome: Build the provider request projection from immutable events under
  explicit budgets and expose `/context` diagnostics.
- Depends on: MEM-2.R1.
- Acceptance summary: Selection is deterministic, preserves legal message and
  tool boundaries, reports included/omitted ranges and estimation error, and
  leaves canonical history unchanged.
- Verification summary: Table-driven selection tests covering exact boundaries,
  split turns, tool-only assistant messages, and configurable budgets.
- Proposed PR boundary: Agent-owned context interface, pure selector, diagnostics,
  and provider integration; no summarization side effect.

### MEM-2.2 - Manual compaction with durable snapshots

- Outcome: Compact an explicitly requested history range while preserving a
  durable record of the boundary and resulting projection.
- Depends on: MEM-2.1.
- Acceptance summary: Manual compaction appends compaction and context-snapshot
  events, replaces omitted tool results with structurally valid placeholders,
  and never deletes source history.
- Verification summary: Legal/illegal cut tests, placeholder projection tests,
  summary success/failure fixtures, restart checks, and a manual `/context`
  demonstration.
- Proposed PR boundary: Manual command, summarization seam, event types, and
  projection behavior; no automatic trigger.

### MEM-2.3 - Automatic and repeated compaction

- Outcome: Trigger compaction before the request exceeds the approved window and
  remain correct across repeated compactions and summary failures.
- Depends on: MEM-2.2.
- Acceptance summary: The configured threshold triggers once per eligible range,
  repeated compaction never nests invalid boundaries, and the approved failure
  fallback is visible and safe.
- Verification summary: Large synthetic tool sessions, repeated restart and
  compaction cases, failure injection, provider-request size assertions, and
  `go test ./...`.
- Proposed PR boundary: Automatic trigger and repeated-compaction state machine;
  no semantic or procedural context.

## Epic completion evidence

- A large tool-heavy session crosses the configured window without orphaned
  calls/results or loss of canonical events.
- `/context` explains the active projection and compaction boundaries.
- Focused tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- MEM-2.R1 is a blocking decision spike; implementation cannot responsibly fix
  token and failure behavior before it is approved.
- Token estimation remains approximate and must fail according to the recorded
  error policy rather than silently overfilling the request.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.

