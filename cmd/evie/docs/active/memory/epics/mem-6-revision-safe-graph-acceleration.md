# MEM-6 - Revision-safe graph acceleration

Status: approved; not started

## Outcome

Accelerate repeated graph traversal with immutable Go snapshots while preserving
the exact semantics, scope boundaries, and revision freshness of SQLite queries.

## Specification references

- [In-process adjacency](../../memory.spec.md#in-process-adjacency)
- [Graph cache freshness decision](../../memory.decisions.md)
- [Stage 6 - In-process graph acceleration](../../memory.spec.md#stage-6---in-process-graph-acceleration)

## In scope

- Performance and memory targets derived from MEM-5 diagnostics.
- Immutable adjacency snapshots and atomic publication.
- Exact allowed-scope revision-vector validation and recursive-CTE fallback.
- Conditional representation optimization when evidence requires it.

## Out of scope

- Changing graph truth, retrieval ranking, or scope semantics.
- A remote graph database.
- Optimizing before a measured target exists.

## Stories

### MEM-6.R1 - Graph performance target

- Outcome: Establish the p95 latency and memory target that justifies and bounds
  graph-cache work.
- Depends on: Completed MEM-5 diagnostics and representative scoped graphs.
- Acceptance summary: Record cold/warm recursive-CTE latency, graph sizes,
  allocation/memory behavior, workload shapes, and the approved target.
- Verification summary: Reproducible Go benchmarks with environment and dataset
  versions plus variance and profiling notes.
- Proposed PR boundary: Benchmarks and target decision only; no cache.

### MEM-6.1 - Immutable revision-validated adjacency cache

- Outcome: Serve repeated graph paths from atomically published adjacency
  snapshots only when they exactly match the retrieval transaction's scope
  revision vector.
- Depends on: MEM-6.R1 and MEM-5.2.
- Acceptance summary: Snapshots are built from one SQLite read transaction;
  equality uses every allowed scope; mismatches fall back to recursive CTEs and
  schedule rebuild; older/out-of-order snapshots never replace newer state; all
  hits rejoin current accepted state.
- Verification summary: CTE equivalence, concurrent publication, cross-process
  revision changes, scope-vector mismatch, restart/rebuild, stale lifecycle, and
  race tests plus benchmarks.
- Proposed PR boundary: Map-based adjacency implementation behind the graph-read
  interface; no packed/CSR representation.

### MEM-6.2 - Conditional packed adjacency optimization

- Outcome: Replace or supplement map adjacency with a packed/CSR representation
  only if MEM-6.1 misses the approved latency or memory target.
- Depends on: MEM-6.1 benchmark evidence showing a target miss.
- Acceptance summary: The new representation remains hidden behind the same
  interface, preserves CTE/path equivalence and revision checks, and meets the
  approved target without unacceptable rebuild cost.
- Verification summary: Side-by-side benchmarks, equivalence/property tests,
  allocation profiles, publication/race tests, and restart rebuild.
- Proposed PR boundary: Representation change and evidence only; if MEM-6.1
  meets the target, close this story as unnecessary rather than implementing it.

## Epic completion evidence

- Cached and recursive-CTE paths are equivalent across lifecycle, time, and scope
  cases.
- Cross-process revision changes cannot serve stale cached relationships.
- The selected representation meets the approved p95 latency and memory target.
- Focused tests, race tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- MEM-6.R1 blocks optimization; architecture alone is not sufficient evidence.
- MEM-6.2 is conditional and should not be scheduled unless MEM-6.1 misses an
  approved target.
- Revision-vector checks must include all allowed scopes, not merely the query's
  primary scope.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.
