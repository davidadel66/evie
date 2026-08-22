# MEM-3 - Explicit temporal semantic graph

Status: approved; not started

## Outcome

Establish the source-linked temporal property graph and explicit memory lifecycle
before any model-driven extraction is allowed to propose semantic state.

## Specification references

- [Semantic memory](../../memory.spec.md#semantic-memory)
- [Scope And Authority](../../memory.spec.md#scope-and-authority)
- [Semantic Graph Model](../../memory.spec.md#semantic-graph-model)
- [SQLite Storage](../../memory.spec.md#sqlite-storage)
- [Explicit Memory Operations](../../memory.spec.md#explicit-memory-operations)
- [Stage 3 - Temporal graph domain and explicit memory](../../memory.spec.md#stage-3---temporal-graph-domain-and-explicit-memory)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- Graph and operation schema, domain encodings, and scope revisions.
- Explicit sourced claims, aliases, entities, links, and candidates.
- Valid-time and transaction-time lifecycle operations.
- Exact scope/reference constraints, promotion, FTS, provenance, traversal, and
  deterministic operation replay.

## Out of scope

- Automatic extraction and background compiler jobs.
- Dense embeddings and hybrid retrieval ranking.
- Automatic claim admission.
- Hard erasure.

## Stories

### MEM-3.R1 - Graphiti comparison and graph-encoding decisions

- Outcome: Close the evidence and encoding gaps that would otherwise make graph
  DDL and public typed operations speculative.
- Depends on: MEM-2 and the Stage 0 research note.
- Acceptance summary: Record an exact Graphiti version and observed pipeline plus
  canonical ID, typed-literal, UTC precision, predicate normalization,
  duplicate-equality, and scope-column decisions.
- Verification summary: Reproducible Graphiti commands or inspected fixtures and
  representative Evie row/operation examples for every encoding.
- Proposed PR boundary: Research note and `memory.decisions.md` updates only; no
  graph DDL.

### MEM-3.1 - Accepted-operation graph kernel

- Outcome: Add the relational graph, lifecycle, candidate, operation, revision,
  and derived-index-generation schema with one atomic accepted-operation kernel.
- Depends on: MEM-3.R1.
- Acceptance summary: Foreign keys and checks enforce proposition shape and
  immutable scope; accepted effects and scope revisions commit atomically;
  operation/idempotency conflicts fail without partial state.
- Verification summary: Schema, transaction rollback, idempotency, revision,
  append-only transition, and cross-connection tests.
- Proposed PR boundary: Domain types, additive schema, low-level operation API,
  and constraints; no user command or retrieval surface.

### MEM-3.2 - Sourced `/remember` and inspection

- Outcome: Deliver the first vertical semantic-memory slice: an explicit claim
  with evidence can be created and inspected within the bound scope.
- Depends on: MEM-3.1.
- Acceptance summary: `/remember` first appends a source event, then atomically
  creates entities/aliases/claim/source/state/operation/revision and FTS rows;
  inspection renders provenance and cannot widen scope.
- Verification summary: Global, project, and explicit current-session cases,
  source authority, typed literals, FTS round trips, approval gates, and local
  demonstration.
- Proposed PR boundary: Explicit create and local inspection only; no correction,
  promotion, links, model extraction, or ranked retrieval.

### MEM-3.3 - Temporal lifecycle and as-known queries

- Outcome: Support corrections, real-world changes, retirement/restoration, and
  source retraction/restoration through append-only history.
- Depends on: MEM-3.2.
- Acceptance summary: Error and changed corrections follow their distinct valid
  intervals; every transition compare-and-sets current revision/state; current,
  historical, and as-known-at queries preserve retire/restore cycles.
- Verification summary: Half-open time-boundary tables, supersession, stale CAS,
  restore eligibility, multi-source retraction, and transaction-time tests.
- Proposed PR boundary: Claim/source lifecycle operations and temporal queries;
  no promotion or graph links.

### MEM-3.4 - Promotion and cross-scope reference fencing

- Outcome: Enforce exact global/project/session visibility and allow only explicit
  project-to-global promotion.
- Depends on: MEM-3.2.
- Acceptance summary: Storage queries and mutations exclude other projects and
  sessions; endpoint/reference constraints prevent traversable private-to-global
  leaks; promotion creates eligible global identities and retains ID-only
  provenance.
- Verification summary: Two-project, two-session scope matrices, promotion
  approval, malicious model-argument attempts, and source-expansion tests.
- Proposed PR boundary: Scope predicates, constraints, and promotion operation;
  no research-topic scope.

### MEM-3.5 - Explicit links and two-hop traversal

- Outcome: Create, retire, restore, and inspect explicit graph links and sourced
  one/two-hop paths without a model extractor.
- Depends on: MEM-3.3 and MEM-3.4.
- Acceptance summary: Both endpoints satisfy the scope matrix; lifecycle changes
  are operation-backed; recursive CTE traversal returns deterministic paths and
  provenance without candidate rows.
- Verification summary: Directional indexes, cycles, depth limits, lifecycle,
  cross-scope rejection, and recursive-CTE path tests.
- Proposed PR boundary: Explicit link operations and database traversal; no
  adjacency cache or hybrid ranking.

### MEM-3.6 - Deterministic graph and FTS replay

- Outcome: Rebuild accepted graph and lexical projections exactly from semantic
  operations without rerunning a model.
- Depends on: MEM-3.3, MEM-3.4, and MEM-3.5.
- Acceptance summary: Dropping rebuildable graph/FTS projections and replaying
  operations yields the same accepted snapshot, revisions, lifecycle history,
  and query results; all memory relations remain fenced from generic tools.
- Verification summary: Golden snapshots, repeated replay/idempotency, FTS
  backfill, sidecar/symlink fence regressions, and `go test ./...`.
- Proposed PR boundary: Replay/rebuild commands and deterministic comparison
  tests; no compiler or vectors.

## Epic completion evidence

- Manually created claims answer current, historical, as-known-at, scoped, and
  two-hop sourced queries without a model extractor.
- Operation replay reproduces the accepted graph and FTS snapshot exactly.
- Global/project/session scope matrices and generic storage fences pass.
- Focused tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- MEM-3.R1 blocks DDL because identity, literal, time, predicate, duplicate, and
  scope encodings affect persistence and public behavior.
- Entity resolution must prefer duplicates over unsafe merges.
- Candidate rows exist outside accepted graph traversal and retrieval.
- Hard erasure remains explicitly deferred.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.
