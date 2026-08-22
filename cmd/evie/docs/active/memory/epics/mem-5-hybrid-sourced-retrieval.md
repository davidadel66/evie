# MEM-5 - Hybrid sourced retrieval

Status: approved; not started

## Outcome

Retrieve the right scoped evidence through exact, lexical, semantic, temporal,
episodic, and graph signals, then render a bounded source-bearing data block that
can reach OpenRouter only after explicit opt-in and egress scanning.

## Specification references

- [Hybrid Retrieval](../../memory.spec.md#hybrid-retrieval)
- [Vector Retrieval](../../memory.spec.md#vector-retrieval)
- [Graph Query Representation](../../memory.spec.md#graph-query-representation)
- [Local inference boundary](../../memory.spec.md#local-inference-boundary)
- [Stage 5 - Hybrid retrieval](../../memory.spec.md#stage-5---hybrid-retrieval)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- Deterministic query planning and hard eligibility filters.
- Concurrent exact, FTS, temporal, graph, episodic, and dense candidate sources.
- RRF, transparent reranking, diagnostics, and source-bearing context.
- Local embeddings and revisioned rebuildable vector generations.
- Remote-memory opt-in and supplemental-context egress controls.

## Out of scope

- Adjacency caching, which belongs to MEM-6.
- Procedural instruction context, which belongs to MEM-7.
- Policy tuning without MEM-9 evaluation evidence.
- A local conversational-model adapter.

## Stories

### MEM-5.1 - Deterministic query plan and hard filters

- Outcome: Translate a memory query into a scope-bound, temporal, authority, and
  budget plan that no model output can widen.
- Depends on: MEM-4 and MEM-3's current/historical query semantics.
- Acceptance summary: Explicit IDs, known projects, and dates parse
  deterministically; ambiguous local-model suggestions remain bounded; scope,
  lifecycle, authority, and temporal filters run before relevance ranking.
- Verification summary: Global/project/session matrices, current/historical
  dates, authority floors, malicious widening suggestions, and budget tests.
- Proposed PR boundary: `QueryPlan`, deterministic parsing, and hard-filter API;
  no search generators or context injection.

### MEM-5.2 - Concurrent lexical, temporal, graph, and episodic retrieval

- Outcome: Fan out to all non-vector candidate generators and combine eligible
  results through initial RRF and transparent reranking.
- Depends on: MEM-5.1 and MEM-3's FTS/traversal projections.
- Acceptance summary: Exact entity/alias, FTS, temporal, one/two-hop graph, and
  recent-episode reads share one deadline; every result retains sources, scope,
  validity, authority, and retrieval signal; contradictory active claims remain
  distinct.
- Verification summary: Alias, exact-ID, multi-hop, temporal, contradiction,
  deadline/cancellation, deterministic fusion, and no-cross-project tests.
- Proposed PR boundary: Non-vector generators, RRF, reranker, and diagnostics;
  no embeddings or model-facing context.

### MEM-5.R1 - Embedding and vector-index spike

- Outcome: Select the embedding model and vector implementation from measured
  local evidence rather than architectural preference.
- Depends on: MEM-5.2 diagnostics and a representative accepted-memory corpus.
- Acceptance summary: Compare SQLite-stored brute-force cosine with local HNSW or
  a SQLite extension on recall, p95 latency, memory, persistence/rebuild, CGO,
  and `modernc.org/sqlite` compatibility; record the decision.
- Verification summary: Versioned corpus, commands, metric calculations, raw
  results, loopback/Unix-socket endpoint checks, redirect rejection, and
  `memory.decisions.md` update.
- Proposed PR boundary: Benchmark adapters, fixtures, report, and decision only;
  no serving vector generation.

### MEM-5.3 - Revisioned local embedding and vector generations

- Outcome: Backfill and continuously refresh derived embeddings without making
  vector state authoritative.
- Depends on: MEM-5.R1 and MEM-4's durable worker/coverage model.
- Acceptance summary: Generation identity includes model/dimensions/config;
  enabling or changing it reconciles retained rows before search serves;
  refreshes are idempotent by source/content hash/revision; every hit rejoins
  current SQLite state.
- Verification summary: Old-row backfill, continuous writes, changed generation,
  stale hit, disabled generation, rebuild, endpoint, redirect, and restart tests.
- Proposed PR boundary: Selected `Embedder`/`VectorIndex`, refresh jobs, coverage,
  and dense generator; no context injection.

### MEM-5.4 - Sourced memory context and read tools

- Outcome: Render bounded claims and source excerpts for provider context and
  expose scope-bound `memory_search`/`memory_inspect` reads.
- Depends on: MEM-5.2 and MEM-5.3.
- Acceptance summary: Context contains IDs, scope, time, source excerpts,
  retrieval reason/path, and unresolved contradictions; it is encoded as
  `EVIE_MEMORY_DATA` immediately before the current user message and is never
  persisted into episodic history.
- Verification summary: Token/result budgets, evidence diversity, JSON/role
  escaping, provenance, contradiction, tool-scope, and history-nonpersistence
  tests.
- Proposed PR boundary: Context reader/renderer and read-only memory tools; leave
  model-facing delivery disabled until MEM-5.5.

### MEM-5.5 - Remote-memory opt-in and egress fence

- Outcome: Permit memory-bearing OpenRouter requests only through explicit
  `EVIE_REMOTE_MEMORY=on` opt-in and a scanned supplemental projection.
- Depends on: MEM-5.4.
- Acceptance summary: With opt-in off, all model-facing semantic reads and tools
  are withheld or rejected while local CLI inspection remains available; with it
  on, supplemental content is scanned and captured requests omit reasoning,
  opaque provider state, raw structured events, and detected secrets.
- Verification summary: Exact request capture, on/off process configuration,
  secret and tainted-source fixtures, prompt-injection data, tool availability,
  and diagnostics redaction tests.
- Proposed PR boundary: Egress composer gate and captured-request tests; no
  procedural context or policy tuning.

## Epic completion evidence

- Versioned Evie fixtures demonstrate exact-ID, alias, temporal, multi-hop, and
  semantic recall with no cross-project leakage.
- FTS and vector generations backfill/rebuild before serving and stale results
  cannot resurrect ineligible claims.
- Captured OpenRouter requests prove the opt-in and exclusion rules.
- Focused tests, `go test ./...`, and `go vet ./...` pass.

## Risks and open decisions

- MEM-5.R1 blocks the production vector path and any new dependency requires
  separate approval under the repository working agreement.
- Default graph depth and token budget remain evaluation inputs, not guesses.
- JSON escaping reduces instruction interpretation but mechanical scope and tool
  gates remain the actual security boundary.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.

