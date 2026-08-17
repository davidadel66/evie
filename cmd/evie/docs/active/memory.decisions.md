# memory - decisions

- **2026-08-17 - research topics are an optional first-class scope after the core feature.**
  A sustained inquiry should not require a fake code-project root or pollute
  global memory. Optional Stage 10 adds registered `research:<id>` workspaces.
  Research sessions retrieve only their own session, topic, and eligible global
  claims; topic-to-global promotion is explicit. This extends, rather than
  weakens, the existing project/session isolation model.

- **2026-08-17 - research artifacts are inspectable files; research claims remain SQLite state.**
  Each topic may expose a manifest plus source, note, and output directories.
  SQLite owns registry metadata and generates the manifest. Every ingested source
  has a mandatory immutable content-addressed evidence version; workspace edits
  create new versions. Hashes, authority, candidates, temporal claims, and
  provenance remain in SQLite. Outputs and graph exports are compiler-ineligible
  by default. Referenced evidence remains retained until an explicit future
  hard-erasure policy permits deletion; optional Git history and limits for
  unreferenced artifacts remain Stage 10 decisions.

- **2026-08-17 - research access uses the same scope and egress fences as memory.**
  Project and research bindings are mutually exclusive. Generic file tools cannot
  access `~/.evie/research`; typed APIs bind the current topic and apply remote
  opt-in, secret scanning, and untrusted-data rendering. Research extraction may
  write only its topic; global promotion is explicit and approved.

- **2026-08-14 - semantic memory is a temporal property graph in SQLite.**
  This supersedes the 2026-08-12 decisions that made Git-backed documents the
  semantic source of truth and deferred graph/vector retrieval. Reified claims,
  entities, aliases, provenance, and temporal state need transactions and indexed
  queries. Git remains only for reviewed procedural instructions and skills.

- **2026-08-14 - the graph is local and relational, not a separate graph database.**
  EVIE's scale does not justify Neo4j or another service. SQLite stores both edge
  directions through indexed claim columns; recursive CTEs provide the correctness
  path, and immutable Go adjacency snapshots may accelerate repeated traversal.
  A specialized graph backend is considered only after measured local limits.

- **2026-08-14 - canonical evidence and accepted semantic state are distinct.**
  Append-only events preserve what happened. Every accepted graph mutation,
  whether automatic or explicit, is recorded as an idempotent semantic operation
  in the same transaction as the graph update. The operation log can reproduce
  the accepted graph exactly; recompiling source events with a new extractor may
  deliberately produce a different candidate graph for comparison.

- **2026-08-14 - extracted candidates do not become graph claims by accident.**
  Unresolved model output lives in a separate candidate table with its raw typed
  proposal, evidence references, extractor version, and review state. Approval or
  an allowlisted deterministic policy emits a semantic operation and an accepted
  claim. Candidate rows are not traversed or returned by normal retrieval.

- **2026-08-14 - claims keep immutable content and append-only lifecycle history.**
  A correction creates a new claim linked to the old one rather than rewriting
  its subject, predicate, object, or evidence. Retirement, restoration,
  supersession, and source retraction append state transitions; candidate
  rejection stays in the separate candidate lifecycle. A cached current status is
  only a query projection. This preserves what EVIE believed at an earlier
  transaction time, including a retire/restore cycle.

- **2026-08-14 - corrections distinguish errors from real-world changes.**
  A correction operation requires `error` or `changed` mode. An error inherits
  the old validity interval unless David supplies a replacement interval; a
  change requires an effective time that closes the old interval and starts the
  replacement. A restore is valid only when the latest state is retired and no
  supersession intervened.

- **2026-08-14 - valid time and transaction time answer different questions.**
  `valid_from`/`valid_to` describe when a claim applies in the represented world.
  Recorded claim and transition timestamps describe when EVIE learned and
  accepted each state. Temporal intervals are half-open, so `valid_to` excludes
  the endpoint; an unknown bound is `NULL`, not an invented timestamp.

- **2026-08-14 - hybrid retrieval is the target, but indexes remain derived.**
  Exact aliases, FTS5, local dense vectors, temporal lookup, and bounded graph
  expansion generate candidates; hard authority/scope/time filters precede RRF
  and transparent reranking. FTS, vectors, and adjacency caches must rebuild from
  accepted graph state and may never become identity or truth authority.

- **2026-08-14 - the vector implementation is intentionally undecided.**
  Stage 5 compares a brute-force Go baseline with local HNSW or a SQLite vector
  extension. Recall, p95 latency, memory, rebuild behavior, CGO, and compatibility
  with `modernc.org/sqlite` decide the implementation; architecture alone does
  not justify a dependency.

- **2026-08-14 - procedural memory is approved Git-backed Markdown.**
  User instructions, project workflows, skills, and checklists change agent
  behavior, so human review, readable diffs, and rollback are valuable. Semantic
  facts stay out of this repository. Git identity is repository-local as
  `Evie <evie@localhost>`, and dirty trees are preserved and quarantined rather
  than reset.

- **2026-08-14 - "local memory" does not imply a local conversational model.**
  Storage, compilation, embeddings, and indexes have no silent remote fallback.
  This feature keeps the existing OpenRouter conversational client, so bounded
  memory is remote egress and requires `EVIE_REMOTE_MEMORY=on`. A scanned
  supplemental projection is sent; opaque provider state, reasoning, and raw
  structured events are not. With opt-in off, all model-facing memory/procedural
  context and read tools are unavailable, though direct local CLI inspection
  remains available. A local conversational-model adapter is out of scope.

- **2026-08-14 - raw episodic storage and promoted memory have different secret policies.**
  Resume fidelity means the local event log may contain user messages and tool
  results that include secrets; the database and procedural repository require
  user-only permissions, and this risk must be documented. Detected secrets are
  excluded from compiler input and rejected from semantic/procedural promotion.
  Secret detection is defense in depth, not a guarantee that raw events are
  secret-free.

- **2026-08-14 - graph cache freshness is revisioned across processes.**
  Every graph transaction increments a durable scope revision. Retrieval captures
  the allowed-scope revision vector in its SQLite read snapshot and uses cached
  adjacency only on exact equality; a mismatch falls back to database traversal.
  SQLite serializes writes, while durable job/scope ordering and fencing tokens,
  not an in-process mutex, coordinate multiple EVIE processes.

- **2026-08-14 - generic tools cannot expose the memory store.**
  The Evie `query_db` surface remains allowlisted to non-memory tables, and file
  tools reject `evie.db`, WAL/SHM files, and the procedural root after symlink
  resolution. Scoped, redacted typed APIs provide inspection. `bash` remains the
  explicitly documented privileged bypass.

- **2026-08-14 - one durable turn lease owns a live session.**
  A process acquires and heartbeats a fenced session lease before calling the
  provider. Lease loss cancels local work; every append and tool start checks the
  token, and late provider responses are discarded. The lease cannot recall an
  HTTP request already sent, so it guarantees accepted state/side-effect ordering
  rather than zero overlap in remote computation.

- **2026-08-14 - compiler and index coverage are generation-keyed.**
  Compiler runs, FTS, and vector indexes carry immutable configuration hashes and
  durable coverage checkpoints. A new extractor can process all source events
  into a separate candidate group; a new index generation backfills before it
  serves queries. Disabled extractors/indexes create no permanently pending work.

- **2026-08-14 - local inference endpoints are mechanically local.**
  Extractors and embedders accept only loopback or Unix-socket endpoints and
  reject redirects. There is no remote fallback. Fixture spikes must prove
  structured output, cancellation, malformed-response, and endpoint behavior.

- **2026-08-12 - EVIE stays centralized; projects are registered session scopes.**
  Launch cwd and `EVIE_PROJECT_ROOT` do not grant a project scope. New
  conversations default to global scope; a project conversation explicitly
  selects one durable registry entry and keeps that scope immutable. Switching
  projects creates or resumes another session.

- **2026-08-12 - project IDs are durable random IDs, not path hashes.**
  Registration stores a unique canonical root, display name, and timestamps. An
  explicit relocation can preserve project identity without silently redirecting
  old sessions, which retain the root snapshot that defined their original
  authority. Archiving never deletes sessions or memory.

- **2026-08-12 - active session scope is always isolated.**
  A global session retrieves its own session claims plus eligible global claims.
  A project session adds only its selected project's claims. Other sessions and
  projects are excluded unless an explicit, authorized operation promotes or
  links their evidence.

- **2026-08-12 - write scope is harness-bound, not model-selected.**
  A global session defaults writes to global; a project session defaults to its
  selected project. Model-called operations cannot override that immutable
  value. A local command may choose the current session scope, while project to
  global writes require explicit promotion.

- **2026-08-12 - local Go-native implementation, not a hosted memory provider.**
  Zep, Mem0, Letta, and Graphiti are references rather than runtime dependencies.
  EVIE owns the memory lifecycle in the existing Go harness and local SQLite
  state so the system remains inspectable and teaches the underlying design.

- **2026-08-12 - SQLite is the canonical event log.**
  EVIE already owns `~/.evie/evie.db`, needs durable jobs and transactions, and
  uses `modernc.org/sqlite`. This supersedes the JSONL transcript choice in
  `docs/harness-improvements.md` for this feature. Hard erasure is deferred, so
  event rows remain append-only in the initial implementation.

- **2026-08-12 - background work uses a durable outbox and bounded workers.**
  Channels only wake workers. SQLite owns jobs, idempotency, ordered source
  ranges, attempts, leases, retry timing, and terminal state. Extraction and
  embedding calls may run concurrently, but accepted mutations are serialized
  per scope and fenced against stale workers. With no extractor configured, EVIE
  retains events rather than accumulating pending jobs; enabling compilation
  reconciles uncovered ranges from a durable checkpoint. A failed/cancelled head
  range blocks later commits until David explicitly retries or records a reasoned
  skip in the coverage ledger.

- **2026-08-12 - source authority controls admission, not relevance.**
  Explicit user corrections outrank user assertions, allowlisted trusted-tool
  observations, assistant inferences, and external content. Model extraction is
  proposed by default. Only explicit operations and later allowlisted,
  evaluation-backed deterministic policies may create active claims directly.

- **2026-08-12 - the prompt stays immutable and retrieved memory stays data.**
  The stable system prompt remains code-owned. Runtime memory is a later bounded
  block with IDs, scope, timestamps, and provenance; it cannot grant permissions,
  widen scope, alter tool approvals, or become system policy.

- **2026-08-12 - provider replay payloads are optional and fenced.**
  Events retain provider-neutral content. Stage 1 must first test whether opaque
  reasoning/continuation blocks are required for interrupted-chain replay. The
  default is to omit them and mark that provider chain non-resumable; persisting
  them requires a separately approved encryption/key policy. They are transport
  state only and never semantic evidence. Manual execution resolution resumes the
  existing chain only when the spike proves that safe without the opaque payload;
  otherwise it starts a new provider-neutral turn.

- **2026-08-12 - uncertain side effects are resolved, never replayed.**
  Tool execution intent is durable before execution and terminal status is a
  later event. After a crash, a started execution without terminal evidence is
  `unknown` and blocks continuation until David resolves it explicitly.

- **Open - extraction and embedding implementations.**
  Stage 4 must select a local extraction model and structured-output protocol.
  Stage 5 must select the embedding model and vector index from measured spikes.

- **Open - automatic admission policy.**
  Evaluation must decide which trusted tools, predicates, and confidence rules
  may bypass candidate review, and which project statements should remain
  session-scoped until explicit promotion.

- **Open - hard erasure semantics.**
  Retirement is reversible and decided. A future `forget` feature must define
  deletion across events, semantic operations, graph state, indexes, caches,
  context snapshots, backups, and procedural Git history without allowing the
  content to reappear through reconstruction.
