# memory - local temporal-graph memory for Evie

Status: approved by David on 2026-08-17; Stage 1 in progress as of 2026-08-21

## Purpose

Give Evie durable, continuously updated, local memory while keeping its model
context small and auditable. The design combines the strongest implemented
patterns from MemGPT/Letta, LongMemEval, Zep/Graphiti, HippoRAG, and Mem0 without
depending on a hosted memory provider or a graph database.

The target is not a basic `memory.md` prototype. It is a research-forward,
Go-native architecture with four distinct memory layers:

```text
working memory     request-specific model context
episodic memory    immutable session and tool events
semantic memory    temporal entities, claims, and relationships
procedural memory  reviewed instructions, skills, and workflows
```

Evie remains the agent runtime. SQLite is the local transactional store. Go owns
the asynchronous compiler, graph representation, retrieval fan-out, and context
assembly.

### Out of scope

The first release is single-owner and local-machine only. It does not add user
accounts, authentication, cloud sync, backups, session branching, inherited
parent-session memory, implicit cross-project imports, a remote graph service, or
a provider-neutral replacement for the current OpenRouter conversational client.
First-class research-topic workspaces are deferred to optional Stage 10. Hard
erasure is deferred separately from reversible retirement.

## Recommendation

Build a temporal property graph as Evie's semantic-memory layer, but keep the
graph derived from immutable evidence:

```text
messages and tool events
          |
          v
append-only event log                       canonical evidence
          |
          v
memory compiler
  extract -> resolve entities -> propose claims -> apply lifecycle
          |
          v
temporal property graph                     derived semantic memory
          |
          +----------+-----------+
          |          |           |
         FTS      embeddings   adjacency
          |          |           |
          +----------+-----------+
                     |
               hybrid retrieval
                     |
        sourced evidence block for the model
```

The graph is first-class for retrieval, but never the only copy of what happened.
Events are canonical evidence. An append-only semantic-operation stream records
every accepted graph mutation and can replay the accepted graph exactly. Running
a newer extractor over the events creates a separate candidate graph for
comparison; it does not silently replace accepted state.

## Why The Previous Draft Changed

The first draft used Git-backed Markdown as the main semantic-memory store and
deferred graph/vector retrieval. That closely followed current Letta MemFS, but
it was not the best fit for Evie's broader goal:

- factual and temporal claims need transactional updates and indexed queries;
- provenance and multiple sources are awkward when facts are files;
- SQLite plus Git creates a dual-write recovery problem for the same fact;
- global/project conflict handling needs structured scope and validity fields;
- current memory research converges on hybrid lexical, semantic, temporal, and
  relational retrieval rather than one document hierarchy;
- Git is excellent for procedural memory, where human review and rollback matter,
  but it should not be the database for every episodic or semantic fact.

Git remains in the design for procedural instructions and skills. SQLite becomes
the single home for episodic and semantic memory.

## Research Basis

| Pattern | Primary source | Evie adaptation |
|---|---|---|
| Context tiers and paging | [MemGPT](https://arxiv.org/abs/2310.08560) | Separate working context from durable episodic and semantic storage |
| Stateful agent and procedural files | [Letta MemFS](https://docs.letta.com/concepts/memfs) | Git-versioned instructions/skills, not the factual graph |
| Experience stream and reflection | [Generative Agents](https://arxiv.org/abs/2304.03442) | Immutable episodes plus source-linked derived claims |
| Index, retrieve, read separation | [LongMemEval](https://arxiv.org/abs/2410.10813) | Independent compiler, retrieval engine, and context renderer |
| Fine-grained sourced observations | [LoCoMo](https://aclanthology.org/2024.acl-long.747/) | Claims retain exact source event IDs and evidence quotes |
| Temporal entities and relationships | [Zep/Graphiti](https://arxiv.org/abs/2501.13956) | Bi-temporal claims and invalidation in SQLite |
| Associative multi-hop retrieval | [HippoRAG](https://arxiv.org/abs/2405.14831) | Graph expansion over Go adjacency structures, optionally PageRank later |
| Hybrid fact retrieval | [Mem0](https://arxiv.org/abs/2504.19413) | Lexical, dense, entity, temporal, and graph signals |
| Persistent-memory poisoning | [AgentPoison](https://proceedings.neurips.cc/paper_files/paper/2024/hash/eb113910e9c3f6242541c1652e30dfd6-Abstract-Conference.html) | Source authority, proposals, scope fences, and data-only rendering |
| Retrieval, learning, forgetting evaluation | [MemoryAgentBench](https://arxiv.org/abs/2507.05257) | Component and lifecycle evaluations, not recall alone |

These sources motivate the architecture; they do not prove Evie's exact schema
or policies. The choices below remain Evie engineering decisions and must be
validated on Evie workloads.

## Memory Layers

### Working memory

The exact context sent to the model for one request:

- immutable base system prompt;
- small current user/project profile;
- current task state and compaction summary;
- a recent complete transcript tail;
- retrieved semantic claims with source excerpts;
- tool schemas.

Working memory is a projection. It is rebuilt for every request and is never a
source of truth.

### Episodic memory

Append-only events describing what happened:

- user and assistant messages;
- provider continuation payloads needed for replay;
- tool calls and results;
- approvals;
- errors and cancellation;
- compaction boundaries;
- context snapshots;
- explicit memory operations.

Episodic memory is canonical evidence. Semantic extraction never changes these
events.

### Semantic memory

A derived temporal property graph:

- entities such as David, Evie, projects, tools, organizations, and concepts;
- aliases that refer to those entities;
- reified claims connecting a subject to an entity or literal value;
- validity and transaction timestamps;
- provenance linking every claim to one or more events;
- typed links between claims, entities, projects, and episodes;
- lifecycle states for candidates, current claims, superseded claims, and retired
  claims.

Claims are reified rows rather than anonymous direct edges. This costs one join,
but allows provenance, time, multiple sources, confidence, authority, and
correction history to belong to the relationship itself.

### Procedural memory

Reviewed instructions about how Evie should work:

- user working preferences;
- project conventions;
- reusable workflows;
- tool-use lessons;
- skills and checklists.

Procedural memory is Git-backed Markdown under `~/.evie/procedural`. It is
separate from facts because changing procedural memory changes agent behavior.
Background systems may propose procedural updates, but only explicit approval
activates them.

## Sources Of Truth

| Data | Authority |
|---|---|
| SQLite events | Canonical record of sessions and executions |
| SQLite semantic operations | Canonical record of accepted semantic mutations |
| SQLite entities/claims/sources/state | Replayable query projection of accepted semantic state |
| FTS tables | Rebuildable lexical index |
| Embedding index | Rebuildable semantic index |
| Go adjacency cache | Rebuildable graph-query acceleration |
| Git procedural repository | Canonical reviewed procedural memory |
| Prompt/context | Disposable request projection |

The accepted graph is rebuilt exactly from semantic operations. Events can be
recompiled to propose a different graph when extractor policy changes, but those
proposals require the normal admission path. Procedural memory cannot be silently
regenerated from transcripts because its approval history is part of its
authority.

## Invariants

1. The stable system prompt remains code-owned and immutable.
2. Session event envelopes are append-only and provider-neutral; optional opaque
   provider continuation data is namespaced transport state, not evidence.
3. Every accepted claim is created by a semantic operation and cites one or more
   source events. Explicit/imported human operations first create a source event.
4. Extracted claims are assertions with provenance, not automatically objective
   truth.
5. User statements, trusted tool observations, assistant inferences, and external
   content retain different source authority.
6. Project scope is resolved by the harness and cannot be selected by model text.
7. Project claims never become global without an explicit promotion operation.
8. Current-state retrieval filters scope, authority, lifecycle, and temporal
   validity before relevance ranking.
9. Retrieved memory is rendered as untrusted data. It cannot change permissions,
   system rules, or tool approval behavior.
10. Background execution is durable, concurrency-bounded, cancellable,
    idempotent, and observable. Durable event coverage cannot depend on a
    process-local queue.
11. Search indexes and graph caches are disposable and rebuildable.
12. Unknown side effects are never replayed automatically after a crash.
13. Raw episodic events may contain conversation or tool data that includes a
    secret. Detected secrets are excluded from compiler input and rejected from
    semantic/procedural promotion; secret detection is not a guarantee that the
    raw event log is secret-free.
14. Only the current durable per-session lease holder may start provider calls or
    tools and append turn events. Lease loss cancels the local turn context;
    already-sent provider work may finish remotely, but its response is discarded
    and it cannot start tools or commit events.
15. Generic file and SQL tools cannot read or mutate memory-owned storage. The
    existing privileged `bash` escape hatch remains outside this feature's
    containment guarantee until a separate shell-containment feature exists.

## Scope And Authority

V1 uses one fixed local owner identity. Projects live in a durable registry with
a random stable ID, display name, unique canonical root, and timestamps. A
project is selected explicitly when a session is created or resumed. The REPL
may canonicalize its launch cwd and exact-match it to one active registered root
as a discovery hint, but David must confirm the project before scope is granted.
An unmatched cwd offers explicit registration or global scope.
`EVIE_PROJECT_ROOT` does not grant project scope. Relocation is an explicit
operation that preserves the project ID, while existing sessions retain their
original root snapshot.

Every session receives an immutable scope context at creation:

- owner ID;
- optional canonical project root;
- optional stable project ID;
- session ID;
- optional parent session ID;
- harness-owned default write scope.

Registered roots are resolved with `filepath.Abs` and symlink evaluation.
Changing the persistent bash working directory must not change memory scope.

Claims use one of these scopes:

- `global`: durable personal knowledge intended across projects;
- `project:<id>`: isolated knowledge for one canonical project;
- `session:<id>`: temporary semantic state that should not survive task closure
  without promotion.

Allowed retrieval scopes are exact:

- a global session: `session:<id>` plus `global`;
- a project session: `session:<id>` plus its one `project:<id>` plus `global`.

Other sessions and projects are always excluded by the storage query, not by a
model instruction. A global claim may reference only global entities. A project
claim may additionally reference entities in that project, and a session claim
may additionally reference entities in that session and its selected project.
Promotion creates any required global identity/claim rather than adding a
traversable global edge into a private project. Cross-scope provenance IDs remain
auditable, but source text is expanded only when its scope is allowed.

The default write scope is `global` for a global session and its one
`project:<id>` for a project session. Model-called writes can target only that
bound value. A local command may explicitly choose the current `session:<id>`;
writing global from a project requires the typed promotion operation.

Memory tools are constructed per session with an unexported `ScopeContext` and
current turn-fencing token supplied by the harness. Scope is never a model
argument. Every operation checks those bound values at the storage boundary.

Source authority is explicit and ordered for conflict handling:

1. explicit user correction or approved memory operation;
2. direct user assertion;
3. deterministic trusted-tool observation;
4. assistant inference grounded in cited evidence;
5. external or imported content;
6. unsupported assistant text.

This ordering is a policy input, not a relevance score. A semantically similar
external document cannot overwrite a direct user correction.

### Optional research-topic scope

Stage 10 may add research workspaces without pretending every sustained inquiry
is a code project. A research topic has a random stable ID, title, archive state,
timestamps, and no required project root. Stage 10 extends `ScopeContext` with a
discriminated `global | project | research` binding: project ID and research ID
are mutually exclusive, and a research session binds immutably to one
`research:<id>` scope.

When enabled, its boundaries are exact:

- retrieval: `session:<id>` plus one `research:<id>` plus eligible `global`;
- default writes: the bound `research:<id>`;
- excluded: every project and other research topic;
- promotion: research to global requires an explicit typed operation;
- graph references: research claims may reference global or same-topic entities,
  while session claims may additionally reference same-session entities;
- mutation: research jobs cannot create or mutate global/project entities,
  aliases, summaries, claims, or links except through approved promotion;
- traversal: global/traversable edges cannot point into a private research scope.

The optional user-visible artifact workspace is separate from semantic truth:

```text
~/.evie/research/
  .objects/sha256/     immutable retained source versions
  <research-id>/
    manifest.md        generated ID/title/scope/source-policy projection
    brief.md           user-authored questions and research goals
    sources/           imported papers, pages, and datasets
    notes/             user and assistant working notes
    outputs/           reports and generated artifacts
```

SQLite is authoritative for research registry metadata and generates the
read-only manifest. The manifest and brief are budgeted context for that research
session; other files are loaded on demand through scope-bound research APIs.
Generic file tools reject the entire research root. Model-facing research reads
use the same remote-memory opt-in, secret scan, source taint, and untrusted-data
rendering as semantic retrieval; direct local inspection remains available.

Each ingestion event owns or content-addressably references an immutable retained
copy of the exact bytes and extracted text used as evidence. Editing a workspace
file creates a new source version and never rewrites prior evidence. SQLite stores
its hash, actor/authority, URL or path, ingestion timestamp, candidates, accepted
claims, temporal state, and provenance. Assistant notes and external sources
remain lower-authority evidence and cannot silently become global facts.
Evidence versions referenced by any event, candidate, claim, or accepted operation
remain retained; routine pruning may remove only unreferenced workspace copies.
Deleting referenced evidence requires the future hard-erasure policy.
`outputs/` and generated graph exports are compiler-ineligible by default;
explicit re-ingestion preserves root-source lineage and cannot increase authority
or count as independent corroboration. Graph exports remain read-only projections,
never a second semantic source of truth.

## Semantic Graph Model

### Entities

`entities` contains stable identities:

- ID;
- scope owner;
- canonical name;
- entity type;
- summary, optional and derived;
- created/updated timestamps;
- creating semantic-operation ID and current lifecycle projection.

`entity_aliases` maps names and identifiers to entities:

- stable alias ID and creating semantic-operation ID;
- alias text and normalized form;
- entity ID;
- scope;
- source event;
- confidence and resolution method;
- active/retired state.

Entity resolution must prefer duplicates over unsafe merges. Two people with the
same name remain separate until evidence is sufficient or David resolves them.

### Claims

`claims` is the graph's first-class relationship table:

- ID and schema version;
- scope;
- subject entity ID;
- normalized predicate;
- object entity ID or typed literal value, exactly one;
- human-readable claim text;
- polarity;
- extraction confidence;
- creating semantic-operation ID and scope revision;
- `recorded_at`: when Evie first accepted the claim;
- extractor/model/prompt version.

Claim proposition content is immutable. `memory_state_events` records each
lifecycle or temporal version for an entity, claim, source link, alias, or
explicit graph link:

- object kind/ID, semantic-operation ID, and monotonic scope revision;
- state and transaction timestamp;
- for claims, `valid_from` and `valid_to`, the half-open world-time interval;
- optional superseded claim ID and reason.

Current status and validity columns may be materialized for query speed, but they
are disposable projections of this append-only history. `valid_to` determines
whether a claim applies at a world time; there is no separate mutable `expired_at`
truth. Every state mutation compare-and-sets the expected latest object state and
scope revision.

`claim_sources` links claims to evidence:

- stable source-link ID and creating semantic-operation ID;
- claim ID;
- event ID;
- evidence quote or structured field path;
- source actor/type;
- source authority;
- observed/event timestamp.

Source-link retractions are also append-only state events. Claims can have
multiple independent sources, and effective claim authority is derived
deterministically from currently active sources rather than copied permanently
onto the claim. Retracting one source does not remove a claim supported by
another; a claim with no eligible active source is excluded from current
retrieval.

### Candidates

`claim_candidates` is outside the accepted graph and contains:

- raw typed proposition and optional resolved entity IDs;
- source event references and mechanically verified evidence locations;
- extractor/model/prompt version and confidence;
- immutable scope plus the base scope revision used for resolution;
- `pending`, `approved`, `rejected`, or `stale` review state.

Approval revalidates scope, entities, conflicts, sources, and the base revision
inside the graph transaction. A mismatch marks the candidate stale for
re-resolution; it is never applied against newer state blindly. Approval emits a
semantic operation and a new accepted claim. Candidates are never traversed,
indexed as accepted facts, or rendered by normal retrieval.

### Explicit graph links

`memory_links` supports relationships that are not ordinary subject-predicate-
object facts:

- stable link ID and creating semantic-operation ID;
- from object kind/ID;
- relation type;
- to object kind/ID;
- source event or user operation;
- scope and timestamps;
- lifecycle status.

Examples include `derived_from`, `generalizes`, `contradicts`, `supports`,
`related_to`, and `applies_to_project`.

Aliases and explicit links follow the same accepted-operation and append-only
state-transition rules as claims. Both link endpoints must satisfy the same scope
reference matrix as claim endpoints. Traversable cross-scope links are forbidden;
cross-scope provenance remains ID-only until an explicit promotion creates
eligible endpoints.

### Claim lifecycle

```text
active -> superseded
active -> retired -> active
```

- A correction requires `error` or `changed` mode. `error` inherits the corrected
  claim's validity interval unless David supplies an explicit replacement
  interval; `changed` requires an effective time, ends the old valid interval,
  and begins the new one there. Both append state versions and a replacement
  claim atomically.
- Model-detected corrections and contradictions remain candidates. Automatic
  admission is disabled until an evaluation-backed allowlist is approved.
- `retired` means hidden from normal retrieval but reversible.
- Restore uses compare-and-set against the latest `retired` state and fails if an
  intervening supersession exists.
- Expiry is computed from the requested valid time; it is not a lifecycle write.
- `superseded` is historical and cannot be restored as current without a new
  explicit claim.
- `forget` is reserved for future hard erasure across events, graph rows,
  indexes, caches, and procedural history.

## SQLite Storage

Extend `internal/eviedb` with additive, idempotent schema changes until a change
requires destructive migration. At that point, add a real migration mechanism
rather than hiding destructive SQL inside startup. Enable WAL and foreign-key
enforcement through DSN pragmas on every pooled connection. New memory relations
use foreign keys; the existing intentional `job_runs` orphan behavior remains
unconstrained.

### Session and event tables

`projects`:

- random stable ID, unique canonical root, display name, archive state, and
  timestamps.

`sessions`:

- stable ID, nullable project ID/root snapshot, parent ID, status, timestamps.

`session_turn_leases`:

- session ID, worker/process ID, fencing token, lease generation, and expiry.

A process must acquire and heartbeat the lease before the first turn event or
provider call. Provider and tool APIs receive the lease-bound `context.Context`.
Every append and tool start checks the current fencing token. Lease loss cancels
local work and discards any late provider response; fencing guarantees accepted
state and side-effect ordering, not cancellation of bytes already sent remotely.

`events`:

- stable ID;
- session ID;
- unique monotonic sequence per session;
- immutable scope and per-target-scope ingest sequence where applicable;
- parent ID;
- event type and optional role;
- optional execution ID;
- provider-neutral content;
- structured payload JSON;
- optional namespaced provider continuation payload, encrypted under an approved
  key policy or absent;
- recorded timestamp and format version.

Execution intent, terminal outcome, and manual resolution are separate immutable
events; execution status is a rebuildable projection. Intent commits before a
tool runs. `succeeded`, `failed`, or `cancelled` commits afterward. A started
execution without terminal evidence projects as `unknown`, blocks the session,
and is never replayed. David resolves it by appending `assumed_succeeded`,
`assumed_failed`, or `abandoned`. A synthetic result that states the resolution
and never invents unavailable output is sent only when the Stage 1 provider spike
proved that chain resumable without its opaque payload. Otherwise EVIE closes the
interrupted provider chain and starts a new provider-neutral turn after recording
the resolution.

Required event appends fail closed. User input commits before a provider call,
tool intent commits before invocation, approval commits before an approved tool
runs, and results commit before another provider iteration. The terminal turn
event and its idempotent compiler job commit atomically when compilation is
enabled.

### Compiler and operation tables

`memory_jobs` is a durable outbox:

- unique idempotency key;
- source event range, target scope, and monotonic scope sequence;
- compiler run/config hash and coverage-stream ID;
- base/expected graph revision;
- job type and status;
- attempts, `available_at`, and last error;
- worker ID, lease token, and lease expiry;
- versioned staged extraction result, optional;
- created/finished timestamps.

Job states are:

```text
pending -> running -> succeeded
pending -> running -> retryable -> pending
pending -> running -> failed
pending/running -> cancel_requested -> cancelled
failed/cancelled -> pending (explicit local retry)
failed/cancelled -> skipped (explicit local operation only)
```

Expired leases return to pending through an atomic compare-and-swap update.
Channels wake workers; they never own durable job state. Workers may stage model
results concurrently, but only the earliest eligible scope sequence may commit.
The final write transaction conditionally verifies the unexpired lease token,
expected graph revision, and scope sequence before writing any accepted effect;
a stale worker changes zero rows and must discard its result.
Failed or cancelled head jobs do not advance contiguous scope coverage and block
later accepted commits. David may retry them or append an explicit, reasoned skip
to the coverage ledger; only that durable skip lets the next sequence commit.

`memory_operations` is the append-only accepted semantic-operation stream:

- operation and idempotency IDs, schema version, scope, actor, and source events;
- complete normalized effect payload, including generated IDs;
- prior and resulting scope revisions;
- transaction timestamp.

Every explicit or automatic accepted mutation writes its operation, graph rows,
state transitions, FTS changes, and new scope revision in one SQLite transaction.
When an embedding generation is enabled, that transaction also writes its durable
vector-refresh job. Replaying operations reproduces accepted graph state without
rerunning a model.

`procedural_operations` separately journals Git intent, operation state, content
hash, expected parent commit, and resulting commit. Procedural Git operations use
commit markers and startup reconciliation because Git and SQLite cannot share a
transaction.

### Index tables

- FTS5 virtual tables for event text, claim text, predicates, entity names, and
  aliases;
- embedding metadata keyed by source ID, model, dimensions, and content hash;
- derived-index generations keyed by index kind and immutable config hash;
- durable idempotent vector-refresh jobs keyed by generation, source revision,
  and content hash;
- monotonic graph revisions per scope;
- retrieval diagnostics keyed by request/context snapshot.

Each derived index has an immutable configuration generation and a durable
coverage checkpoint. Enabling or changing a generation reconciles all eligible
retained rows before that index serves queries, including event FTS for events
written before Stage 3. Disabled vector generations create no refresh jobs;
accepted revisions provide the later backfill boundary.

After an event index is enabled, every eligible event append transactionally
updates its redacted/allowlisted FTS projection and enqueues idempotent refreshes
for each enabled event-vector generation. Thus the checkpoint covers both
backfill and continuous writes; raw structured payloads are not indexed.

SQLite should use WAL mode for concurrent readers and the existing busy timeout
for writer contention. SQLite serializes cross-process writes; expected revisions
and fencing tokens provide semantic conflict detection. Retrieval uses read-only
transactions.

## Memory Compiler

When compilation is enabled, every terminal turn and its idempotent compiler job
commit in one transaction. When no extractor is configured, events still commit
but no permanently pending job is created. Enabling the compiler runs a
transactional reconciliation that creates ordered, coalesced jobs for every
eligible event range not covered by that immutable compiler run/config hash,
including events written before Stage 4. A changed extractor/model/prompt creates
a new coverage stream and can reprocess all eligible events into a distinct
candidate group. The compiler is a pipeline, not one unconstrained reflection
prompt:

1. Load a bounded source event range and its immutable scope.
2. Project only allowlisted evidence fields and remove detected secrets.
3. Extract typed entity and claim candidates through structured model output.
4. Validate each source by event ID, JSON Pointer or byte range, and content hash.
5. Resolve entities against exact aliases and lexical candidates; add dense
   candidates only after Stage 5 supplies embeddings.
6. Detect duplicates, corrections, contradictions, and temporal updates.
7. Persist model-derived output as candidates; automatic admission remains off
   until an evaluation-backed allowlist is approved.
8. In one fenced SQLite transaction, persist candidates, advance the compiler
   checkpoint, and complete the job.

Eligible evidence is limited to direct user content, final assistant content,
explicit memory-operation events, and allowlisted structured fields from tool
results. Assistant content and external/tool data retain lower authority.
Reasoning, provider continuation payloads, context snapshots, compaction
summaries, retrieved-memory blocks, compiler output, and diagnostics cannot
become evidence. An evidence quote must match the cited immutable field exactly;
the model cannot validate its own paraphrase.

Explicit user operations bypass extraction but still emit a source event and an
accepted semantic operation. If a later automatic-admission policy is approved,
the compiler must revalidate its staged result against the current scope revision
and commit the semantic operation, graph transitions, FTS, vector job, revision,
and job completion atomically under the lease fence; the vector job is included
only when an embedding generation is enabled.

The compiler never writes global scope from a project or research job unless the
source is an explicit approved global promotion operation. A research job also
cannot write any project scope.

### Local inference boundary

The memory compiler has no silent remote fallback. It uses an explicitly
configured loopback or Unix-socket OpenAI-compatible endpoint such as Ollama, LM
Studio, or llama.cpp; redirects and non-loopback endpoints are rejected. Stage 4
starts with a fixture-based spike proving the selected model's structured output,
timeouts, cancellation, and malformed-response behavior. If no local extractor
is configured, explicit manual memory operations continue to work and later
reconciliation covers the retained events.

Retryable compiler failures use at most five attempts with exponential backoff
from 5 seconds to a 10-minute cap, then become `failed` until explicitly retried.
Cancellation is durable, and shutdown returns unfinished leased work to a
retryable state after lease expiry.

The conversational model remains OpenRouter in this feature. Retrieved memory is
therefore remote egress, even though storage and compilation are local. Memory
injection into a remote request is disabled unless David explicitly enables
`EVIE_REMOTE_MEMORY=on`. Before every remote request, the context composer builds
an egress-safe projection that excludes opaque payloads/reasoning/raw structured
event data, applies the existing file/database source fences, scans supplemental
memory and source excerpts for secrets, and records only IDs/hashes in
diagnostics. While opt-in is off, all model-facing semantic/procedural reads,
including memory tools and procedural context, are withheld or rejected; direct
local CLI inspection remains available. Supporting a local conversational model
requires a future provider-neutral client boundary and is out of scope here.

### Go concurrency model

Use concurrency where work is independent, not because goroutines are cheap:

- one coordinator owns job claiming and shutdown;
- a bounded worker pool handles model extraction and embedding calls;
- workers stage extraction concurrently, while durable scope sequence and graph
  revision serialize accepted effects across processes;
- `context.Context` carries cancellation and deadlines;
- a non-blocking channel wakes the coordinator after event commits;
- lease heartbeats preserve ownership, and the final transactional token check
  fences stale workers;
- shutdown stops claims, waits for bounded in-flight work, and leaves unfinished
  jobs retryable.

No goroutine or channel is the only owner of state that must survive restart.

## Graph Query Representation

SQLite stores the graph. Go accelerates traversal.

### Database traversal

Indexes support both directions:

- `(scope, status, subject_entity_id, predicate)`;
- `(scope, status, object_entity_id, predicate)`;
- `(claim_id, source_event_id)`;
- normalized aliases;
- validity and recorded timestamps.

Recursive CTEs provide cold-path traversal, debugging, and correctness tests.

### In-process adjacency

For repeated queries, load active scoped relationships into an immutable
adjacency snapshot:

```text
entity -> outgoing claim IDs
entity -> incoming claim IDs
claim  -> source event IDs
```

Build each snapshot from one SQLite read transaction and tag it with the captured
scope revision. A retrieval read transaction captures the full allowed-scope
revision vector and uses cached adjacency only on exact equality; otherwise it
falls back to recursive CTEs and schedules a rebuild. Publish only a newer
snapshot through an atomic pointer, so readers never observe partial mutation or
an out-of-order cross-process refresh. Large scopes may later move to CSR arrays
behind the same interface.

The cache is an optimization. A process restart can rebuild it entirely from
SQLite. Every FTS/vector/cache result is re-filtered against current accepted
SQLite state before rendering, so an asynchronous stale hit cannot resurrect a
superseded or out-of-scope claim.

## Hybrid Retrieval

Retrieval follows LongMemEval's index -> retrieve -> read separation.

### Query planning

A `QueryPlan` contains:

- allowed scopes;
- current or historical temporal intent;
- exact identifiers and entity candidates;
- lexical query;
- dense query text;
- graph relations/depth;
- authority floor;
- token/result budget.

Deterministic parsing handles explicit IDs, known projects, and RFC3339/calendar
dates. A local model may propose ambiguous entity or temporal interpretations,
but cannot widen scope.

### Candidate generation

Run independent generators concurrently with a shared context deadline:

- exact entity/alias lookup;
- FTS over claims and source events;
- dense-vector similarity;
- temporal/current-claim lookup;
- one- or two-hop graph expansion;
- recent relevant episodes.

Each result retains claim ID, source event IDs, scope, authority, validity, and
the signal that retrieved it.

### Fusion and reranking

Apply hard scope, lifecycle, authority, and temporal filters first. Then combine
candidate rankings using Reciprocal Rank Fusion initially. Rerank by:

- lexical and semantic relevance;
- exact entity match;
- graph distance/path support;
- source authority;
- temporal applicability;
- corroborating source diversity;
- recency only when the query asks for current/recent state;
- evidence diversity under the context budget.

Do not collapse contradictory active claims into one answer. Render both with
their sources or ask David to resolve them.

### Reading context

The model receives a bounded JSON block containing:

- current claims;
- historical claims when requested;
- source excerpts;
- timestamps and scope;
- retrieval reason/path;
- unresolved contradictions;
- memory and source IDs.

The context composer places `EVIE_MEMORY_DATA` in a synthetic user-role message
immediately before the actual current user message; it is never persisted back
into episodic history. The stable prompt instructs the model to treat the block
as quoted evidence, assess sufficiency, and abstain when unsupported. JSON
escaping and role placement reduce accidental instruction interpretation but do
not make prompt injection impossible; scope, write authority, and tool approval
remain mechanically enforced outside the model.

## Vector Retrieval

Dense retrieval is part of the target architecture, not a prerequisite for the
event log or graph schema.

Use two interfaces:

- `Embedder`: batches text through a configured local embedding endpoint;
- `VectorIndex`: upsert, delete, search, and rebuild by source ID/content hash.

`Embedder` enforces the same loopback/Unix-socket-only and no-redirect policy as
the compiler. Stage 5 tests that a non-loopback URL or redirect is rejected and
that no remote fallback occurs.

The first implementation choice must be selected through a local spike:

1. vectors stored in SQLite with brute-force cosine in Go, establishing a simple
   correctness baseline; and
2. a local HNSW implementation or SQLite vector extension, establishing scale
   and operational cost.

The spike compares recall, p95 latency, memory use, persistence/rebuild behavior,
CGO requirements, and compatibility with `modernc.org/sqlite`. The chosen index
is recorded in `memory.decisions.md`. Embeddings remain derived and
model-versioned. A vector worker upserts by source ID, content hash, model, and
scope revision, then marks the durable refresh job complete. Search hits always
rejoin current SQLite claim state before ranking or rendering.

## Procedural Memory

Local layout:

```text
~/.evie/procedural/
  .git/
  system/
    user.md
    evie.md
  projects/
    <project-id>/
      instructions.md
      workflows/
      skills/
  proposals/
```

Files under `system/` and the active project's `instructions.md` are pinned within
separate strict budgets on every request. The context composer loads these files
mechanically from the session's immutable project ID; the model never has to
remember to request them. A missing, unreadable, quarantined, or over-budget
required file fails the request visibly rather than silently omitting governing
instructions. Other project files are listed in the context manifest and loaded
on demand. Approved procedural content is rendered as a trusted supplemental
system-role block without modifying the code-owned base system prompt.
Procedural changes use dedicated tools; generic file tools reject the procedural
root. Existing `bash` remains a privileged exception.

Git is required only when procedural memory is enabled. The repository receives
a local identity (`Evie <evie@localhost>`) and never modifies global Git config.
The feature is disabled until explicitly initialized. The root is `0700`, files
are created `0600`, and symlinked roots or managed files are rejected. Dirty
worktrees are preserved and quarantined, never reset automatically.

Procedural operations follow this recovery protocol:

```text
pending -> applying -> committed
pending/applying -> retryable -> pending
pending/applying -> failed | quarantined
```

Each commit contains `Evie-Operation: <id>` and `Evie-Content-SHA256: <hash>`
trailers. Under a cross-process lock, startup marks a matching commit/hash as
committed, retries an absent marker only when the expected parent and clean tree
match, and quarantines every dirty, divergent, or malformed state. A quarantined
repository serves no procedural memory until David resolves it; EVIE never resets
or guesses the intended tree.

Background consolidation may propose a procedural lesson, but approval is
required because activating it changes future agent behavior.

## Explicit Memory Operations

| Operation | Authority | Effect |
|---|---|---|
| `memory_search` | read-only | sourced scoped retrieval |
| `memory_inspect` | read-only | entities, claims, paths, and provenance |
| `/remember` or approved `remember` | local user command/approval | active claim in harness-bound default or explicit local session scope |
| `correct_memory` | local user command/approval | mode/effective-time correction plus supersession |
| `promote_memory` | local user command/approval | new global claim linked to project evidence |
| `link_memory` | local user command/approval | explicit graph link, no claim merge |
| `retire_memory` | local user command/approval | retire an eligible claim/entity/alias/link |
| `restore_memory` | local user command/approval | compare-and-set restoration of eligible object |
| `retract_source` / `restore_source` | local user command/approval | append source-link state and recompute claim eligibility |
| `approve_candidate` / `reject_candidate` | local user command/approval | resolve compiler ambiguity |
| `approve_procedure` / `reject_procedure` | local user command/approval | activate/discard Git proposal |

Ordinary natural-language statements become source events. They may be compiled
into scoped semantic claims according to source authority, but only `/remember`,
correction, promotion, and approvals carry explicit lifecycle authority.
Only locally parsed commands count as direct user operations. A model call to any
mutating memory/procedural tool uses the existing approval gate; only scoped
read-only search and inspection are ungated.

## Security Boundary

- `read_file` and `edit_file` reject the procedural root plus `evie.db`, its WAL,
  and its shared-memory file. Symlink resolution is part of the check.
- `edit_db` cannot mutate memory-owned tables; semantic writes use typed APIs.
- Generic `query_db` keeps an explicit non-memory table allowlist for the Evie
  database. Only scope-bound, redacted `memory_search` and `memory_inspect` may
  expose memory rows to the model.
- `bash` is logged but remains an acknowledged privileged bypass.
- fetched pages and command output are stored as external/tool assertions, not
  user truth.
- retrieved text retains source taint, is scanned for remote egress, escaped, and
  rendered under `EVIE_MEMORY_DATA`.
- memory cannot authorize tools or change approval requirements.
- raw events are stored locally at `0600` for resume fidelity and may contain
  secrets; detected secrets are rejected from candidates, claims, procedures,
  remote supplemental context, and diagnostics.
- extraction prompts receive allowlisted event fields and exclude context
  projections, retrieved-memory echoes, provider payloads, and reasoning.

## Staged Build Order

Each stage is independently demoable and teaches one system-design seam. Do not
start the next stage until the current tests pass and David approves the result.

### Stage 0 - Read and run references

**Goal:** understand the systems being adapted.

**Tasks:**

- read Generative Agents and MemGPT architecture sections;
- run current Letta locally and inspect MemFS, compaction, Git commits, and
  dreaming;
- record the exact Letta version/commit;
- record observed differences from the papers;
- check in a research note with exact versions/commits, commands, observed
  schemas, and an applicability/difference matrix approved by David.

**Learning checkpoint:** explain the difference between context, episodes,
semantic claims, graph indexes, and procedural files.

### Stage 1 - Session identity and append-only events

**Goal:** make the existing agent loop restart-safe and observable.

**Progress (2026-08-21):** the durable event spine is integrated into the agent
loop and all current tests pass. The REPL still creates a fresh global session at
startup, so restart-safe selection/resume, leases, and unknown-execution recovery
remain unfinished. Exact canonical-root lookup for cwd discovery is implemented;
the terminal confirmation and session chooser are the current implementation
seam.

**Tasks:**

- [x] before the first event write, remove memory-owned tables from generic
  `query_db` and reject `evie.db`/WAL/SHM paths in every file tool;
- [x] enable/test WAL and foreign keys on every connection and preserve database mode
  `0600`;
- [x] add project registry/register/list/relocate/exact-root lookup, session,
  immutable scope, and event tables;
- [ ] add session-turn-lease and execution-projection tables;
- [x] define a provider-neutral event envelope and format version;
- [x] inject a durable `History` interface into `Session` and rebuild provider
  context from ordered events;
- [x] propagate cancellation contexts through the agent, provider client, and
  tool execution paths;
- [ ] inject immutable `ScopeContext` and lease-cancellable turn ownership into
  `Session`;
- [x] persist user/assistant messages plus tool intent, approval, success,
  failure, and cancellation events in before-action order;
- [ ] fence event appends with the durable turn lease and persist provider
  failure/interruption events;
- [ ] add cwd-assisted explicit project confirmation, session selection, and
  resume for the REPL;
- [ ] block unknown side effects and append explicit synthetic resolution events;
- [ ] capture provider usage;
- [ ] run a provider replay spike for opaque reasoning/continuation blocks. Until an
  encryption/key-management policy is approved, omit those blocks and mark
  dependent interrupted chains non-resumable rather than persisting them.

**Go lesson:** transaction boundaries, interfaces owned by consumers, and safe
recovery from partial side effects.

**Done when:** two processes cannot run one session concurrently, and Evie
resumes after restart without replaying or silently resolving an uncertain write.

### Stage 2 - Working context and compaction

**Goal:** separate durable history from what the model currently sees.

**Tasks:**

- create the context composer;
- before implementation, record model window/reserve defaults, estimation error
  policy, legal message cut boundaries, split-turn behavior, and summary-failure
  fallback in `memory.decisions.md`;
- add configurable budgets and `/context` diagnostics;
- clear old tool results without orphaning call/result structure;
- implement manual then automatic compaction;
- append compaction and context-snapshot events;
- retain complete source history;
- table-test legal cuts, split turns, repeated compaction, summary failure, and
  tool-result placeholder structure.

**Go lesson:** pure selection functions around an LLM side effect, token-budget
accounting, and table-driven boundary tests.

**Done when:** a large tool-heavy session crosses the configured window safely.

### Stage 3 - Temporal graph domain and explicit memory

**Goal:** establish the semantic graph before automatic extraction.

**Tasks:**

- before DDL, run a minimal Graphiti example locally or inspect its episode ->
  entity -> edge pipeline and database output, recording the exact version,
  observed schema, and differences from Evie's model in the research note;
- before DDL, record canonical ID, typed-literal, UTC timestamp/precision,
  predicate-normalization, duplicate-equality, and scope-column encodings;
- add entity, alias, claim, source, generic state-event, link, candidate, accepted
  operation, scope-revision, and derived-index-generation tables;
- implement append-only claim/source/link transitions and valid/transaction-time
  queries;
- implement exact project/global/session scope filters and cross-scope reference
  constraints;
- add `/remember`, inspect, correct, promote, link, retire, and restore;
- add FTS5 updates to every semantic operation and provenance rendering;
- regression-test the Stage 1 generic SQL/file fences against every memory table
  and database sidecar path;
- implement operation replay and recursive-CTE traversal tests.

**Go lesson:** relational modeling of a property graph, state machines, compound
indexes, and atomic semantic updates.

**Done when:** manually created claims support current, as-known-at, historical,
scoped, and two-hop sourced queries without a model extractor, and dropping the
graph projection then replaying operations produces the same accepted snapshot.

### Stage 4 - Durable asynchronous memory compiler

**Goal:** continuously derive graph candidates from new episodes without slowing
the user turn.

**Tasks:**

- add durable jobs, per-scope sequences/revisions, transactional lease fencing,
  fixed retries, cancellation, and graceful shutdown;
- key compiler checkpoints/jobs/candidate groups by immutable run/config hash and
  backfill/coalesce every uncovered eligible range;
- run the loopback local-model structured-output/cancellation spike, then define
  its strict extraction schema;
- implement allowlisted evidence projection and exact field/hash validation;
- resolve exact/lexical entities and quarantine ambiguous merges;
- persist model output as candidates; do not enable automatic admission;
- revalidate candidate base revisions during approval;
- expose candidate review and worker/queue diagnostics;
- test two processes, expired leases, stale commits, out-of-order staged results,
  blocked/skipped head jobs, versioned backfill, retries, cancellation, and
  no-extractor behavior.

**Go lesson:** bounded worker pools, fan-out/fan-in, cancellation, leases,
idempotency, and serialized per-scope commits.

**Done when:** a committed turn produces sourced graph candidates asynchronously,
survives restart, and never blocks the main response.

### Stage 5 - Hybrid retrieval

**Goal:** retrieve the right evidence through lexical, semantic, temporal, and
graph signals.

**Tasks:**

- implement `QueryPlan` and deterministic hard filters;
- run FTS, exact entity, temporal, graph, and recent-episode search concurrently;
- complete the vector-index spike and add local embeddings;
- enforce/test loopback or Unix-socket embedding endpoints with redirects denied;
- transactionally enqueue/reconcile revisioned vector refreshes and revalidate
  every vector hit against accepted SQLite state;
- backfill the selected FTS/vector generations before enabling their searches;
- implement RRF and a transparent reranker;
- build egress-scanned source-bearing context blocks under a token budget;
- require `EVIE_REMOTE_MEMORY=on` for OpenRouter injection and capture the exact
  test payload to prove opaque/raw event data is absent;
- record retrieval diagnostics and paths.

**Go lesson:** concurrent independent reads, immutable result types, ranking,
deadline propagation, and optimization behind interfaces.

**Done when:** EVIE-specific retrieval tests demonstrate multi-hop, temporal,
alias, exact-ID, and semantic recall with no cross-project leakage.

### Stage 6 - In-process graph acceleration

**Goal:** capitalize on Go for repeated graph queries without changing semantics.

**Tasks:**

- build immutable per-scope adjacency snapshots from active claims;
- tag snapshots with scope revisions and use only on exact read-snapshot revision
  vector equality;
- compare adjacency traversal with recursive CTE results;
- set p95 latency/memory targets from Stage 5 diagnostics before optimizing;
- benchmark maps versus packed/CSR representation.

**Go lesson:** immutable data, atomic publication, cache invalidation, benchmark-
driven optimization, and memory-layout effects.

**Done when:** cached and database traversals return equivalent paths and meet the
selected p95 latency/memory target.

### Stage 7 - Procedural Git memory

**Goal:** add reviewed, versioned learning about how Evie should behave.

**Tasks:**

- initialize the procedural repository and local Git identity;
- load budgeted global instructions and the active project's required
  `instructions.md` on every request, while representing other project files in
  an on-demand scoped manifest;
- add procedural proposal/approval tools;
- journal Git operations with commit markers and startup reconciliation;
- test the documented crash matrix, cross-process lock recovery, restrictive
  modes, symlink rejection, and dirty/malformed quarantine behavior;
- keep semantic graph facts out of the procedural tree.

**Go lesson:** filesystem boundaries, atomic replacement, process locks, Git CLI
integration, and recovery across non-transactional systems.

**Done when:** an approved workflow lesson is versioned, project-scoped, loaded
on demand, and rollbackable without changing factual claims.

### Stage 8 - Web session integration

**Goal:** extend the persistent session/scope model to `evie serve` deliberately.

**Tasks:**

- before implementation, approve amendments to `serve.spec.md` and
  `serve.decisions.md` defining session/project endpoints, reload recovery,
  history loading, approval flow, and concurrent-process ownership;
- expose memory inspection and candidate approvals;
- preserve existing origin and approval defenses.

**Done when:** the web UI resumes a selected scoped session and performs the same
memory operations as the REPL.

### Stage 9 - Memory evaluation and policy tuning

**Goal:** decide improvements with evidence.

**Tasks:**

- derive 10-20 redacted replayable fixtures from real Evie tasks; never commit
  raw event payloads;
- version a manifest of fixed model/prompt/index configurations, expected
  entities/claims/transitions/retrievals, metric formulas, and pass thresholds;
- measure write precision/recall, entity-resolution errors, lifecycle updates,
  retrieval recall, grounded answers, and abstention;
- add temporal, multi-session, and multi-hop cases inspired by LongMemEval and
  LoCoMo;
- add project-isolation, promotion, correction, retirement, and poisoning cases;
- report latency, token use, storage growth, and index rebuild time;
- run model-backed tests under `go test -tags eval ./...`;
- tune extraction, graph depth, ranking, and candidate admission only against
  versioned evaluations;
- consider bounded Personalized PageRank only if it improves the multi-hop set.

**Done when:** changing a memory policy produces a comparable before/after report.

### Optional Stage 10 - Research-topic workspaces

**Goal:** support durable, isolated research programs that are not tied to code
project roots.

**Tasks:**

- decide whether Git supplements mandatory immutable content-addressed evidence
  versions and record limits for unreferenced artifacts before implementation;
- add the research registry and harness-owned `research:<id>` session scope;
- enforce a global/project/research scope discriminator and reference matrix in
  storage constraints and typed operations;
- initialize research roots/directories as `0700` and managed regular files as
  `0600`; reject or quarantine permissive, symlinked, or non-regular paths;
- fence the research root from generic file tools and expose only scope-bound,
  opt-in-gated, secret-scanned typed research reads/writes;
- ingest local files, fetched pages, and structured datasets as hashed source
  events with immutable retained content, actor, URL/path, timestamp, and
  authority;
- reuse the existing `web_fetch` URL/credential/redirect/timeout/byte fences,
  bound regular-file ingestion by size, and approve an allowlist of source formats
  before enabling import;
- compile source-linked entities and claim candidates through the existing local
  compiler rather than a separate factual pipeline;
- exclude outputs/exports from compilation and preserve root lineage on explicit
  re-ingestion;
- add create/select/archive, source inspection, citation, promotion, and export
  operations to the REPL and web UI;
- test isolation across two research topics, projects, global memory, and session
  memory, including prompt injection and stale-source updates;
- evaluate research retrieval and grounded report generation against versioned
  citation-bearing fixtures.

**Go lesson:** optional domain extensions, content-addressed artifacts, provenance
boundaries, scoped retrieval, and reusing a pipeline without collapsing distinct
concepts into one abstraction.

**Done when:** a research session survives restart, retrieves only its topic plus
eligible global memory, produces a report whose claims trace to source artifacts,
and promotes a conclusion globally only through explicit approval.

## Anticipated Package Seams

```text
internal/agent/
  agent.go             existing loop
  history.go           durable event projection
  context.go           working-memory assembly and compaction

internal/eviedb/
  db.go                connection, WAL, schema setup
  events.go            projects/sessions/events/execution state
  graph.go             entities/claims/sources/links/state
  operations.go        accepted operation replay and scope revisions
  jobs.go              compiler outbox and leases

internal/memory/
  scope.go             canonical project/session scope
  model.go             entities, claims, candidates, lifecycle
  compiler.go          extraction pipeline
  resolver.go          entity and conflict resolution
  retrieval.go         query planning, fusion, reranking
  adjacency.go         immutable graph snapshots
  embeddings.go        local embedder/vector index interfaces
  procedural.go        Git-backed reviewed procedures
  worker.go            bounded background coordinator
  research.go          optional research registry/artifact workspace

internal/tools/
  memory.go            scoped explicit memory operations
```

The `agent` package owns the interfaces it consumes. Concrete SQLite, model,
vector, and Git implementations remain replaceable behind those interfaces.

## Definition Of Done

- Sessions resume from a provider-neutral append-only event history.
- Durable turn leases fence accepted events and tool starts to one process;
  lease-lost provider responses are discarded.
- Unknown side effects are blocked rather than replayed.
- Context compaction preserves complete tool boundaries.
- Accepted semantic operations replay the same source-linked temporal graph
  without rerunning a model.
- Global, project, and session scopes cannot leak through typed operations.
- Automatic extraction is local, asynchronous, durable, and candidate-only until
  an evaluated admission policy is approved.
- Entity ambiguity prefers duplicates/candidates over unsafe merges.
- Valid-time changes, corrections, supersession, source retraction, retirement,
  restoration, and as-known-at transaction queries are tested.
- Hybrid retrieval combines exact, FTS, vector, temporal, and graph evidence.
- Retrieved claims always retain source excerpts and IDs.
- FTS/vector/cache projections rebuild from accepted SQLite state, reject stale
  revisions, and match database-query semantics.
- Procedural memory is Git-versioned, approved, and separate from factual memory.
- Generic file/SQL tools cannot expose memory-owned storage; typed memory
  operations cannot bypass scope, approval, or egress checks. The existing
  privileged `bash` exception remains explicitly documented.
- Captured OpenRouter requests contain memory only after explicit opt-in and omit
  opaque provider state, reasoning, raw structured event payloads, and detected
  secrets from supplemental context.
- With remote-memory opt-in off, composed memory, procedural context, and
  model-facing memory read tools are unavailable.
- `go test ./...`, `go vet ./...`, and model-backed evaluations pass.
- REPL and web demonstrations both resume scoped memory across restart.
- One scripted end-to-end test creates and approves a candidate, verifies all
  three scope fences, races two processes, restarts, captures remote payload,
  drops/replays graph and index projections, and compares the accepted snapshot.

## Open Questions

These remain explicit until the relevant stage:

1. Which local extraction model and structured-output protocol should Stage 4
   use?
2. Which local embedding model should index events, claims, and entities?
3. Which vector-index implementation wins the Stage 5 spike without creating an
   unacceptable CGO or operational dependency?
4. How strict should predicate normalization be before distinct relations begin
   losing meaning?
5. Which trusted tools may create active claims automatically rather than
   candidates?
6. Should project conversation claims compile automatically to project scope, or
   should some classes remain session-scoped until promotion?
7. What graph depth and token budget should retrieval use by default?
8. How will the web UI select and display active project scope?
9. What hard-erasure semantics should a future `forget` feature provide?
10. Should optional research workspaces add Git history on top of mandatory
    immutable content-addressed evidence, what limits apply to unreferenced
    artifacts, and which source formats need first-class ingestion?

## References

### Research papers

- Park et al., [Generative Agents: Interactive Simulacra of Human Behavior](https://arxiv.org/abs/2304.03442), UIST 2023.
- Packer et al., [MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/abs/2310.08560), revised 2024.
- Maharana et al., [Evaluating Very Long-Term Conversational Memory of LLM Agents](https://aclanthology.org/2024.acl-long.747/), ACL 2024.
- Wu et al., [LongMemEval](https://arxiv.org/abs/2410.10813), ICLR 2025.
- Gutiérrez et al., [HippoRAG](https://arxiv.org/abs/2405.14831), NeurIPS 2024.
- Rasmussen et al., [Zep: A Temporal Knowledge Graph Architecture for Agent Memory](https://arxiv.org/abs/2501.13956), 2025 preprint.
- Chhikara et al., [Mem0: Building Production-Ready AI Agents with Scalable Long-Term Memory](https://arxiv.org/abs/2504.19413), 2025 preprint.
- Chen et al., [AgentPoison](https://proceedings.neurips.cc/paper_files/paper/2024/hash/eb113910e9c3f6242541c1652e30dfd6-Abstract-Conference.html), NeurIPS 2024.
- Hu et al., [MemoryAgentBench](https://arxiv.org/abs/2507.05257), revised 2026.
- Li et al., [LoCoMo-Plus](https://arxiv.org/abs/2602.10715), 2026 preprint.

### Implementations and documentation

- [Letta MemFS](https://docs.letta.com/concepts/memfs)
- [Letta memory and dreaming](https://docs.letta.com/agent-sdk/memory/)
- [Letta self-hosting](https://docs.letta.com/self-hosting)
- [Graphiti overview](https://help.getzep.com/graphiti/getting-started/overview.md)
- [Graphiti repository](https://github.com/getzep/graphiti)
- [Mem0 architecture](https://docs.mem0.ai/core-concepts/how-it-works)
- [LongMemEval repository](https://github.com/xiaowu0162/LongMemEval)
- [LoCoMo repository](https://github.com/snap-research/locomo)
- [AgentPoison repository](https://github.com/BillChan226/AgentPoison)
- [EVIE harness improvements](../../../../docs/harness-improvements.md)
- [EVIE agent loop](../../../../internal/agent/agent.go)
- [EVIE prompt](../../../../internal/agent/prompt.go)
- [EVIE database](../../../../internal/eviedb/db.go)

### Go references

- [context](https://pkg.go.dev/context)
- [database/sql](https://pkg.go.dev/database/sql)
- [sync](https://pkg.go.dev/sync)
- [sync/atomic](https://pkg.go.dev/sync/atomic)
- [io/fs](https://pkg.go.dev/io/fs)
- [os/exec](https://pkg.go.dev/os/exec)
