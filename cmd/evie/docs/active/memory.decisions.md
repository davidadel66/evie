# memory - decisions

- **2026-08-30 - conversational requests are canonically composed and snapshotted before transport.**
  Every ordinary and post-tool conversational iteration rebuilds one complete
  `ChatRequest` from the immutable code-owned system prompt, an optional
  validated accepted rolling summary supplied by the compaction stage, current
  tool schemas, and a suffix of durable root-user turns ending with the active
  turn. The summary occupies a distinct system message immediately after the
  base prompt and is charged by the same complete-request estimator; its durable
  compaction identity and content-free byte count are recorded in the snapshot.
  The stream flag is set before estimation. Canonical request bytes are the
  standard Go JSON encoding of the exact request value passed to the OpenRouter
  client; the replaceable estimator charges one token per UTF-8 byte and reports
  `ceil(bytes / 4)` only as a rough diagnostic. Usable input is
  `min(hard window, working ceiling) - output reserve - fixed margin`. A composer
  may remove only whole older root-user turns, oldest first; it never removes
  the active root turn or splits an assistant tool-call group from its matching
  terminal results. Structurally incomplete tool groups retain the Stage 1
  omit-whole projection. If the active turn cannot fit, the provider is not
  called and the turn records `context_overflow` at `context_compose` with the
  fixed safe message.

- **2026-08-30 - context snapshots and local diagnostics contain manifests, never request content.**
  After final composition and before provider authorization, every conversational
  iteration appends one lease-fenced `context_snapshot` parented to the root user
  message or terminal tool result that triggered it. Snapshot v1 records only
  composer/estimator versions, iteration, selected and canonical model identity,
  profile provenance and budgets, canonical byte counts and rough estimate,
  request SHA-256, retained event frontier, message/tool counts, content-free
  component byte counts, optional compaction identity/failure category, and
  content-free placeholder manifests. It stores no prompt, message, summary,
  tool schema, excerpt, secret, usage, or provider payload. Persistence is a
  required pre-transport write and fails closed; an accepted snapshot is never
  rolled back when authorization, transport, cancellation, or later persistence
  fails. `/context` is an exact local command that performs only durable reads
  and a hypothetical composition with no lease, event, activity update, or
  provider call. It prints the latest validated snapshot and current projection,
  approved budgets, safe counts/frontiers, byte diagnostics, headroom,
  provenance, and fallback/staleness warnings. Invalid durable history or
  snapshot data makes the command visibly fail rather than silently falling
  back.

- **2026-08-30 - one startup-resolved profile bounds every conversational request.**
  Each process resolves one immutable context profile before opening or resuming
  a conversational session. The profile keeps the configured model, canonical
  model identity, advertised model window, effective route-safe hard window,
  working ceiling, output reserve, fixed estimation margin, and stable source
  provenance as distinct diagnostics. `remote_metadata`, `explicit_override`,
  and `builtin_fallback` are the closed provenance values. Resume always uses
  the current process profile; there is no durable cache, session override,
  runtime setter, or migration. The working ceiling defaults to 262,144 tokens,
  the output reserve to 16,384, and the estimation margin is fixed at 4,096.
  `EVIE_CONTEXT_WINDOW_TOKENS` is an optional positive hard-window override
  which skips metadata discovery; `EVIE_CONTEXT_WORKING_TOKENS` and
  `EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS` override their respective defaults.
  Every value must fit a positive signed 64-bit integer, working must not exceed
  hard, and reserve plus margin must fit and remain strictly below working.
  Conversational requests send the profile reserve as `max_tokens`.

- **2026-08-30 - remote route safety uses focused canonical metadata and eligible endpoints.**
  Without a hard override, startup performs one authenticated focused-model
  lookup followed by one authenticated endpoint lookup for the returned
  canonical slug, under caller cancellation and one shared three-second
  deadline. The advertised window comes only from focused-model metadata. The
  route-safe hard window is the minimum positive context length across active
  endpoints that advertise `max_tokens` and a maximum completion limit at least
  as large as the configured output reserve. Non-serving endpoints and endpoints
  that cannot honor that output limit are ineligible; malformed identity,
  window, completion-limit, or eligible-endpoint metadata fails discovery.
  Caller cancellation and caller deadline expiry always abort startup. Other
  discovery failures use the checked-in 262,144-token `builtin_fallback` only
  when the configured model is Evie's exact built-in model; an unknown custom
  model fails startup unless a hard override was supplied.

- **2026-08-25 - provider token usage is immutable per-assistant episodic diagnostics.**
  Each successful provider iteration may attach one optional provider-neutral
  usage object to its accepted `assistant_message` payload in the same existing
  lease-fenced append. The allowlist maps OpenRouter prompt, completion, total,
  reasoning-completion, cached-prompt, and cache-write-prompt counts to input,
  output, total, reasoning-output, cached-input, and cache-write-input tokens.
  Missing counters remain absent and reported zero remains present. Each
  recognized counter is normalized independently to a non-negative signed
  64-bit integer; null, negative, fractional or exponent-form, overflowing,
  duplicate, and non-number values omit only that counter without rejecting an
  otherwise valid assistant. Null, non-object, empty, excluded-only, and
  invalid-only containers contribute no durable usage. Duplicate top-level
  usage members normalize usage to absent, while repeated detail containers
  count duplicates by full recognized JSON path and preserve valid siblings.
  Streaming uses the last non-null usage occurrence for the iteration and
  replaces rather than merges earlier occurrences, including when the final
  occurrence normalizes to absent. Usage-only chunks are parsed even without a
  choice. Cost, BYOK, modalities, provider/model/routing identity, service or
  server-tool metadata, reasoning-related accounting details, unknown fields,
  and raw transport payloads are excluded from normalized `TokenUsage` and the
  durable `AssistantMessagePayload`. The pre-existing transient
  `openrouter.Message.Reasoning` and `Message.ReasoningDetails` behavior remains
  unchanged for live presentation; neither field becomes durable evidence.
  Usage survives payload storage and restart but is ignored by provider-history
  projection and is never content, continuation state, semantic compiler
  evidence, claim provenance, authorization, billing, budget, aggregation, or
  frontend protocol state.

- **2026-08-24 - REPL session selection uses one explicit combined hierarchy.**
  Startup lists registered project headings and their active sessions, followed
  by global sessions. Project labels include a terminal-safe display name and
  escaped canonical root; relocated sessions also show a differing stored root
  snapshot. Selecting a numbered existing/new entry or current-directory
  registration is the explicit scope grant. An exact cwd match may annotate one
  unarchived project but never preselects it. Projects and sessions have stable
  deterministic ordering, and stale registration/archive state refreshes the
  chooser without silently creating a session or switching scope.

- **2026-08-24 - session titles are durable deterministic metadata.**
  `sessions.title` is nullable and added through an idempotent,
  concurrent-start-safe additive upgrade. Existing sessions are backfilled from
  their earliest nonblank accepted root user event. A new title is initialized
  atomically with that event inside the turn-lease fence, never overwrites a
  populated title, and does not change `sessions.updated_at`. Titles are
  normalized to one terminal-safe line of at most 80 Unicode code points;
  untitled sessions use a generated display fallback. This persistence behavior
  applies to every session, including web-created sessions, while rename APIs,
  title history, web presentation, and editing remain deferred.

- **2026-08-24 - project archival and session resumability remain separate.**
  Resume eligibility is determined by `sessions.status = active`, not the
  project's archive flag. An archived project may therefore expose its active
  sessions under an archived heading, but it is never a cwd suggestion and
  cannot create a new session. Closed sessions are not listed. A held turn lease
  reports `Session busy; message not sent.` and keeps the selected REPL prompt;
  an after-selection inactive session reports `Session unavailable; message not
  sent.` Neither path starts or records turn work. Broader project archival,
  restoration, and session-closing UX remain separate work.

- **2026-08-23 - prior reference-system research satisfies Stage 0.**
  David confirmed that he completed the necessary Letta and reference-system
  research before the current delivery plan. Stage 0 therefore requires no new
  checked-in research note, version log, or command transcript and does not
  block Stage 1. The retained references continue to motivate the design without
  becoming runtime dependencies.

- **2026-08-21 - startup cwd discovers project scope but never grants it silently.**
  The REPL canonicalizes its launch directory and may suggest one matching active
  registered project, but David must explicitly confirm project scope before a
  session is created or resumed. An unmatched directory offers registration or
  global scope. Later `bash` cwd changes never alter the immutable session scope.

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

- **2026-08-22 - session turn leases retain one monotonic acquisition epoch.**
  A successful acquisition of an active session starts a new ownership epoch:
  both the fencing token and lease generation begin at one and advance together
  on every successful acquisition, including takeover after release or expiry.
  Release clears the holder and expiry while retaining the row and counters
  across restart. Heartbeat sets expiry to the later of the stored expiry and
  `now + duration`, so it never shortens a lease; callers own operational TTL
  bounds while storage rejects non-positive or overflowing durations. Expiry is
  half-open (`now >= expiry` is expired), and observations are persisted
  snapshots rather than proof of current ownership. Closed sessions cannot
  acquire, renew, or authorize writes; the matching unexpired holder may still
  release for cleanup.

- **2026-08-23 - turn terminal evidence separates failures from interruptions.**
  Request construction, transport, non-2xx HTTP, and response-body or scanner
  I/O failures record `turn_failed` with classification `provider_error`.
  Malformed streamed JSON, no chunks, no usable choice, response-assembly
  failure, or a structurally incomplete provider tool call record `turn_failed`
  with classification `provider_response_invalid`. A structurally valid tool
  call has contiguous assembled indices, unique non-empty IDs, type `function`,
  and a non-empty function name. Tool argument syntax and shape remain the
  called tool's validation responsibility and produce an ordinary model-visible
  tool failure. Caller cancellation records `turn_interrupted` as
  `caller_cancelled`, and caller deadline expiry records it as
  `caller_deadline_exceeded`. Tool failures remain tool events, a required
  storage-write failure fails closed locally, and a final accepted assistant
  event remains the success evidence without a separate `turn_succeeded` event.
  If an ordinary non-lease assistant append fails after output was rendered, the
  local presentation cause is `assistant_persistence_failed`; it produces no
  durable terminal event. The first terminal cause observed among provider,
  caller, heartbeat, lease, and assistant-persistence paths wins atomically.

- **2026-08-23 - terminal payloads are allowlisted, redacted, and rooted in accepted evidence.**
  Durable failure/interruption payloads contain only the root user-event ID as
  `turn_id`, a stable classification, a stable lifecycle stage, and an optional
  numeric provider HTTP status. Their content is a generic safe description.
  Raw provider bodies, URLs, headers, prompts, partial output, tool arguments,
  reasoning, continuation state, and Go error strings are never persisted in
  terminal evidence; detailed errors may remain local. The root user-message
  event ID identifies the turn without a schema column. Assistant and terminal
  events parent to the latest durable event that triggered their provider
  request, so a failure before assistant acceptance never points to an event
  that did not commit. Existing execution IDs remain per tool call. The closed
  lifecycle-stage vocabulary is `turn_start`, `provider`, `assistant_commit`,
  `tool_prepare`, `tool_approval`, `tool_execute`, and `tool_commit`. The
  coordinator sets the stage immediately before that phase starts and the
  winning terminal cause captures it; unknown values are rejected. The complete
  transition matrix is:

  - `turn_start` begins when the root user event commits and ends immediately
    before the first provider-cycle history load.
  - `provider` begins immediately before history load for any provider cycle and
    covers history projection, provider authorization and call, streaming,
    response assembly, and structural validation. It ends when a structurally
    valid response is available and before assistant-event construction.
  - `assistant_commit` begins before assistant-event construction and covers its
    fenced append. A committed no-tool assistant ends the turn successfully
    before frontend callbacks; a committed tool-calling assistant transitions
    immediately to `tool_prepare` before any post-commit callback. Every
    committed assistant then emits exactly one authoritative `AssistantDone`
    durable-acceptance notification, even when cancellation or lease loss was
    reserved as its append committed. That notification is not live provider or
    tool work; it completes before the selected error is returned, while every
    later tool/provider callback and start remains suppressed.
  - `tool_prepare` covers the post-assistant callback, execution-ID and intent
    construction, intent append, tool-call callback, durable preparation
    authorization, and optional preparation. It ends before approval begins for
    a gated tool or before final execution authorization for an ungated tool.
  - `tool_approval` begins immediately before invoking the approver and covers
    approval waiting, observation, and the fenced approval-event append. An
    approved decision transitions to `tool_execute`; a declined or expired
    decision transitions to `tool_commit`.
  - `tool_execute` begins immediately before the final durable execution
    authorization and covers direct or prepared execution through its return.
    It then transitions to `tool_commit`.
  - `tool_commit` begins after execution returns, preparation returns an ordinary
    tool failure, or a non-approved decision commits. It covers outcome
    construction, fenced outcome append, and the tool-result callback. After a
    callback, another call in the same assistant group transitions immediately
    to `tool_prepare`; after the final call, the next provider cycle enters
    `provider` immediately before history load.

  `http_status` appears only for a non-2xx provider response and is otherwise
  omitted.

  Before acquisition succeeds, failure is local and no release or terminal
  evidence is attempted. After acquisition but before the root user event
  commits, cancellation or a rolled-back append permits release only because no
  `turn_id` exists. Once the root event commits, its returned ID is retained even
  when cancellation is observed immediately afterward and may authorize a
  terminal-evidence attempt. Once a final no-tool assistant event commits, the
  turn is durably successful; later callback cancellation creates neither an
  interruption event nor a discarded-response marker.

- **2026-08-23 - lease uncertainty fails closed with fixed v1 timing.**
  V1 uses a 30-second turn lease and heartbeats every 10 seconds. Any heartbeat
  error cancels the local turn immediately. A competing unexpired holder fails
  fast with a typed conflict; callers do not wait, retry indefinitely, or force
  takeover. Lease loss is a typed local cause, suppresses all later turn work,
  and is never persisted by bypassing the stale fence. A stale owner appends no
  substitute event and a later owner does not synthesize one. Expected heartbeat
  goroutine shutdown after the coordinator has selected a terminal result is not
  an error. Definitive ownership loss produces local `lease_lost`; any other
  heartbeat/storage error produces local `lease_heartbeat_failed`. Neither
  heartbeat classification becomes durable terminal evidence, before or after
  the root event. Caller, heartbeat, and provider paths participate in the same
  first-terminal-cause arbitration.

- **2026-08-23 - live output is marked when it cannot become accepted history.**
  Reasoning and content may stream while the lease-bound context remains live.
  After cancellation or lease loss, later callbacks are suppressed and a late
  response cannot append or start tools. If any response text was already shown
  but its assistant event did not commit, the REPL and web stream emit a local
  `response_discarded` notification stating that the interrupted response was
  not saved. The marker is presentation state, not durable history.

- **2026-08-24 - committed assistant acceptance is always presented once.**
  A successful fenced assistant append owns one authoritative
  `AssistantDone(content)` notification. Cancellation or lease loss reserved at
  that commit boundary does not suppress the notification, because it presents
  accepted durable state rather than starting live work. For a tool-calling
  assistant it precedes the selected local error; tool intents, tool callbacks,
  execution, and later provider work remain suppressed. Provider reasoning is a
  monotonic presentation phase: same-chunk reasoning precedes content, and any
  adversarial reasoning callback after content begins is not rendered.

- **2026-08-23 - every provider and tool start has a current durable fence.**
  Acquisition and heartbeat begin before the user event. Every provider
  iteration is authorized immediately before it starts, every event append is
  fenced in its write transaction, gated tools are authorized before preparation
  and again after approval immediately before execution, and ungated tools are
  authorized immediately before execution. Context checks complement but never
  replace these durable fences.

- **2026-08-23 - terminal and release cleanup is bounded and preserves accepted state.**
  Provider failure or caller interruption gets one independent five-second
  attempt to append terminal evidence through the current fence. Every
  successfully acquired turn gets exactly one independent five-second release
  attempt, including an attempt whose stale token is rejected by the ordinary
  fence. An acquisition error or held-lease conflict makes no release call.
  Terminal-append failure is joined with the original turn error; release failure
  is reported without rolling back accepted events, and lease expiry remains the
  recovery path. A definitively stale owner cannot append terminal evidence;
  release may only succeed through the ordinary matching-token fence.

- **2026-08-23 - incomplete tool-call groups remain evidence but are not replayed.**
  Durable assistant, intent, approval, and partial outcome events are preserved.
  If an assistant tool-call group lacks a terminal tool outcome for every call,
  provider-history projection omits that entire assistant group and its partial
  tool messages. EVIE neither synthesizes missing outcomes nor blocks later
  turns. Complete groups retain their normal provider-neutral ordering.

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

- **2026-08-23 - opaque provider continuation payloads remain deferred.**
  Events retain provider-neutral content. EVIE does not currently persist opaque
  reasoning or continuation blocks. Adding them requires demonstrated need plus
  a separately approved encryption and key-management policy; they would remain
  transport state rather than semantic evidence.

- **2026-08-23 - unfinished tool intent does not block later turns.**
  Tool execution intent remains durable before execution and terminal status is
  a later event. After restart, an intent without terminal evidence is treated as
  unfinished without synthesizing success or failure, and later turns may
  continue. A stronger recovery policy is deferred until observed workflows show
  that it is necessary.

- **2026-08-23 - uncertain cron cancellation cleanup preserves the jobs row.**
  When parent cancellation is observed after a cron mutation starts, cleanup gets
  one independent 10-second attempt. If cleanup cannot establish both that
  launchd accepted the bootout and that the plist is absent, EVIE preserves the
  `jobs` row as the durable recovery handle and returns parent cancellation joined
  with the cleanup error. `cron_list` therefore continues to expose the uncertain
  job, and a later ordinary `cron_remove` retries cleanup and then removes the row.
  This is a cancellation-only exception to the completed cron contract; ordinary
  add failures still roll back their row, and ordinary remove still ignores
  uninstall errors and deletes its row.

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
