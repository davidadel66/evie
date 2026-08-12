# memory - local Letta-inspired memory for Evie

Status: draft (pending David's approval)

## Purpose

Give Evie durable, local memory that survives process restarts and keeps the
model's working context small. The implementation follows the already-built
ideas in MemGPT/Letta rather than inventing a new memory architecture:

- a durable transcript outside the prompt;
- a small, always-available working-memory layer;
- an on-demand project and reference hierarchy;
- context-window compaction;
- explicit memory operations;
- bounded background consolidation; and
- versioned, inspectable local state.

Evie remains the Go harness. No hosted memory provider, paid memory service, or
required third-party runtime is part of this feature.

## Design Target

The end state is:

```text
                  +----------------------+
                  |   Evie Go Session    |
                  |  model <-> tools     |
                  +----------+-----------+
                             |
          +------------------+------------------+
          |                  |                  |
          v                  v                  v
   SQLite event log   Memory filesystem   Context composer
   raw history       global/project docs  prompt-sized view
          |                  |                  |
          +------------------+------------------+
                             |
                    durable job outbox
                             |
                    background consolidator
```

The sources of truth are deliberately split:

- **SQLite event log:** every model-visible turn event and execution event.
- **Memory filesystem:** the current, human-readable semantic memory that Evie
  is allowed to use across sessions. Its Git history provides memory revision
  history.
- **Search index:** a disposable derived index over memory files. It can be
  rebuilt and is never the authority on what Evie remembers.
- **Prompt:** a disposable projection. Nothing learned only because it was
  placed in the prompt becomes durable automatically.

## Why This Shape

### Reference implementation, not a new theory

| Pattern | Reference | Evie adaptation |
|---|---|---|
| Context tiers and paging | [MemGPT](https://arxiv.org/abs/2310.08560) | SQLite history, pinned memory, project files, and compaction in the existing Go loop |
| Git-backed memory hierarchy | [Letta MemFS](https://docs.letta.com/concepts/memfs) | Local `~/.evie/memory` repository with `system/` and `projects/` scopes |
| Memory editing and dreaming | [Letta memory](https://docs.letta.com/agent-sdk/memory/) | Explicit tools plus a durable Go worker that creates reviewable proposals |
| Experience stream and reflection | [Generative Agents](https://arxiv.org/abs/2304.03442) | Session events and bounded consolidation; reflections are derived, never canonical evidence |
| Index, retrieve, read separation | [LongMemEval](https://arxiv.org/abs/2410.10813) | Separate event ingestion, memory search, and prompt assembly interfaces |
| Fine-grained evidence | [LoCoMo](https://arxiv.org/abs/2402.17753) | Source event IDs and complete turn boundaries instead of summary-only memory |
| Temporal claims and invalidation | [Zep paper](https://arxiv.org/abs/2501.13956) | `valid_from`, `valid_to`, `supersedes`, and recorded/source timestamps in local metadata |
| Associative links | [HippoRAG](https://arxiv.org/abs/2405.14831) | Explicit links and FTS first; graph traversal is a later measured extension |
| Write trust boundary | [AgentPoison](https://proceedings.neurips.cc/paper_files/paper/2024/hash/eb113910e9c3f6242541c1652e30dfd6-Abstract-Conference.html) | Harness-owned scope and admission rules; retrieved memory is data, never authority |

The design does **not** claim to reproduce Letta internally or to solve all
long-term memory research. It implements a small local subset whose behavior is
understandable and testable in Go.

The paper and implementation links below are design evidence, not proof that
their exact choices are correct for Evie. Stage 0 must record the Letta version
or commit actually inspected. The source-event, temporal-field, and approval
choices in this spec are Evie adaptations motivated by the references, not
requirements those papers independently establish.

## Definitions

- **Event:** one append-only fact about a session or execution, such as a user
  message, assistant message, tool call, tool result, approval, or compaction.
- **Transcript:** the ordered events belonging to a session. It is evidence,
  not automatically semantic memory.
- **Memory document:** a human-readable Markdown document containing a durable
  fact, preference, decision, lesson, goal, or project note.
- **Pinned memory:** a small document loaded into every relevant request,
  analogous to Letta's `system/` MemFS files.
- **Reference memory:** a document visible through the memory tree and loaded
  only when relevant, analogous to Letta files outside `system/`.
- **Proposal:** a model-generated candidate memory that has not yet become
  active durable memory.
- **Scope:** `global` or one trusted project identity. There is no implicit
  cross-project scope.
- **Recorded time:** when Evie stored the event or memory.
- **Event time:** when the described fact or event occurred, if known.
- **Validity:** the interval during which a claim should be treated as current.

## Non-Goals

The first shipped version will not include:

- a hosted memory provider or cloud synchronization;
- a graph database;
- a mandatory embedding model or vector database;
- automatic promotion of project knowledge into global knowledge;
- self-modification of Evie's immutable system prompt or Go code;
- multi-user authorization or multi-agent shared memory;
- storage of credentials, access tokens, private keys, or unredacted secrets;
- automatic deletion of arbitrary raw history without an explicit retention policy;
- a full reimplementation of the Letta App Server or Agent SDK.

## Invariants

These are acceptance-level rules, not suggestions.

1. The stable system prompt remains immutable. Runtime memory is appended as a
   separate dynamic block.
2. Event rows are append-only. Corrections create new events; they do not alter
   the evidence that led to an earlier decision.
3. Every active memory document has at least one source event or an explicit
   human-created origin.
4. A project memory cannot become global through a typed or background memory
   operation based only on similarity, recency, or model preference. Promotion
   is an explicit operation. The existing privileged `bash` escape hatch is
   outside this containment guarantee.
5. Retrieved memory is untrusted data. It cannot change system rules, grant a
   tool permission, or authorize a write.
6. Background work is bounded, cancellable, durable, retryable, and idempotent.
   A goroutine by itself is not a durable job queue.
7. The memory repository has one writer at a time. Concurrent reads are fine;
   concurrent Git commits are serialized across processes as well as goroutines.
8. Typed memory persistence rejects secrets before writing. This extends the
   existing decision that secrets never enter `messages`; arbitrary shell writes
   remain outside this feature's containment boundary.
9. A failed memory write does not make the user turn fail after the turn's
   transcript has been committed. It becomes an observable retryable job error.
10. A memory index can be deleted and rebuilt from the memory filesystem.

The existing `bash` tool is an explicitly privileged, ungated escape hatch. This
feature does not claim to contain a model that deliberately writes under
`~/.evie/memory` through shell commands. Typed memory operations and generic file
tools must enforce the memory boundary; shell bypasses remain visible in the
event log and are an existing harness security decision that needs its own
containment feature.

## Scope and Project Identity

Every `Session` receives an immutable `ScopeContext` when it is created:

- global user identity: the local Evie owner, initially David;
- optional project root: a canonical absolute directory;
- stable project ID derived from that canonical root;
- session ID and parent/branch information.

The project root is resolved once using `filepath.Abs` and symlink
canonicalization. A later `bash` `cd` must not silently change the memory scope.
The initial resolver uses `EVIE_PROJECT_ROOT` when set, otherwise the process
working directory. Web sessions will eventually need an explicit project
selection endpoint; until then they use the configured root.

The project ID is an implementation detail, not a user-facing path. Store the
canonical root separately so the memory viewer can explain which project a
document belongs to.

Global and project memory are both eligible for retrieval, but they are not
merged into one file. If active claims conflict, the prompt must preserve both
sources or ask for clarification; "project always wins" is not a safe universal
truth rule.

## Local Storage Layout

```text
~/.evie/
  evie.db
  memory/
    .git/
    system/
      user.md
      preferences.md
    reference/
    projects/
      <project-id>/
        context.md
        decisions/
        lessons/
        goals/
    proposals/
```

`~/.evie` remains mode `0700`; the database remains mode `0600`, matching
`internal/eviedb`. The memory repository is local and should receive the same
user-only filesystem permissions.

Pinned files must stay small. A large project note belongs under the project
tree and is retrieved on demand rather than pasted into every request.

Git is a required dependency when the memory feature is enabled. If the `git`
executable is missing, initialization fails with an actionable error and Evie
does not pretend that memory writes succeeded. The memory repository receives a
local commit identity (`Evie <evie@localhost>`) so the feature does not mutate
the user's global Git configuration. If the repository is dirty at startup,
Evie preserves the user's changes, records a quarantine diagnostic, and reads
only the last validated committed tree until an explicit reconciliation command
handles the changes.

## Memory Document Format

Each Markdown document begins with a JSON metadata block rather than YAML:

```text
---
{"schema_version":1,"id":"mem-...","scope":"project","project_id":"...","kind":"decision","status":"active","created_at":"...","recorded_at":"...","valid_from":"...","valid_to":null,"source_events":["evt-..."],"supersedes":null,"links":[]}
---
# Decision

Use the repository's existing Go test command before considering the task done.
```

JSON frontmatter keeps the file human-readable while avoiding a YAML dependency
in the Go core. The exact on-disk metadata type is versioned, and unknown fields
must be ignored when reading so future additions do not make old memory
unreadable.

Required metadata:

- stable memory ID;
- `schema_version`;
- scope and optional project ID;
- kind;
- status: `active`, `superseded`, `retired`, or `proposed`;
- created and recorded timestamps;
- source event IDs;
- optional event/validity timestamps;
- optional superseded memory ID;
- optional links to related memory IDs.

All persisted timestamps use RFC3339 in UTC. `recorded_at` is when Evie stored
the document; `valid_from` and `valid_to` describe when the claim applies. A
missing `valid_to` means the claim is open-ended, not necessarily true forever.
The path and metadata scope must agree. Invalid or manually edited documents
are quarantined rather than indexed as active memory.

Optional fields such as confidence, sensitivity, extractor version, and model
name may be added after the first schema review. They must not be used as a
replacement for source provenance.

## SQLite Event and Job Model

Extend `internal/eviedb` with additive, idempotent tables following its current
schema convention. The first version does not add a migration framework; every
new column/table change must be additive and tested against a fresh and an
existing database. If a future change needs destructive migration, stop and
record a separate database decision before implementing it.

### `sessions`

Stores stable session identity and scope metadata:

- `id` primary key;
- `project_id`, nullable;
- `project_root`, nullable;
- `created_at`, `updated_at`;
- `parent_id`, nullable, reserved for branching;
- `status`.

### `events`

Append-only transcript/execution history:

- `id` primary key;
- `session_id`;
- monotonic `sequence` within a session;
- `parent_id`, nullable;
- `event_type`;
- `execution_id`, nullable, for tool execution correlation;
- `role`, nullable;
- `created_at`;
- provider-neutral `content` where applicable;
- structured `payload_json` for tool calls, tool results, and approvals;
- `status`, nullable, for execution events such as `started`, `succeeded`, or
  `unknown`;
- `format_version`.

Provider payloads may be retained locally when needed to replay or diagnose a
turn, including reasoning details required by a provider's continuation
protocol. Redact secrets before writing them, and never pass reasoning fields to
the memory extractor or treat them as semantic evidence.

This feature supersedes the v1 process-only reasoning fence in
`docs/done/reasoning.spec.md` only for opaque, local provider-payload retention
needed by session recovery. Reasoning is still never rendered from history,
never injected into the prompt as memory, and never extracted as a fact. If a
provider payload cannot be safely redacted, the turn is recorded as non-resumable
instead of persisting it.

`UNIQUE(session_id, sequence)` is required. Tool execution events are written
before and after side effects. On restart, a `started` execution without a
terminal event becomes `unknown`; Evie must not automatically replay it because
the tool may have changed a file, database, or external system. The resumed
session is marked `blocked` until the user explicitly resolves the execution as
`succeeded`, `failed`, or `leave-unknown`. A `succeeded` resolution produces a
synthetic tool result marked `user-confirmed` and allows the model to treat the
side effect as confirmed without inventing details. A `failed` resolution
produces an error result and permits normal error handling. `leave-unknown`
produces a non-authoritative result and the model must not claim success.

Do not make event deletion a prerequisite for the first feature. We need an
explicit retention and forget policy before physically removing raw evidence.

### `memory_jobs`

Durable outbox for background work:

- `id` primary key and idempotency key;
- `job_type`;
- source session/event range;
- scope/project ID;
- `status`;
- `attempts`, `available_at`, `leased_until`;
- `lease_token`, nullable, and `worker_id`, nullable;
- `created_at`, `finished_at`;
- `last_error`.

Job states are `pending -> running -> succeeded` or `pending -> running ->
retryable -> pending`, with `failed` after the maximum attempt count. An expired
`running` row returns to `pending` through an atomic compare-and-swap update.
Claiming uses an atomic predicate on status and lease expiry, plus a unique
worker lease token. Heartbeats update the row only when the token matches. A
retryable failure clears the lease token, records `available_at` using backoff,
and returns to `pending`; the idempotency key has a unique constraint.

### `memory_operations`

This is the durable handoff between SQLite intent and the Git-backed memory
tree. It prevents a crash between a database commit, a file replacement, a Git
commit, and index maintenance from becoming silent state loss.

- operation ID and unique idempotency key;
- operation kind and source event ID;
- target memory ID/path and expected parent revision;
- validated operation payload or content hash;
- status: `pending`, `applying`, `committed`, `failed`, or `quarantined`;
- Git commit ID when committed;
- created/updated timestamps and last error.

An explicit synchronous memory write waits for its operation to reach
`committed` or returns a clear retryable error. Every memory commit includes the
operation ID and desired content hash in its commit message/trailer. Startup
reconciliation takes the process lock and follows this deterministic matrix:

- matching commit marker and target tree hash: mark the operation committed;
- no marker and clean parent tree: retry the operation with the same idempotency
  key;
- dirty or unexpected tree: preserve the worktree, quarantine the operation,
  and require explicit reconciliation;
- committed tree with a missing index: mark committed and rebuild the index.

SQLite and Git are not assumed to be one atomic transaction; the operation
marker, content hash, and recovery matrix are the protocol. Crash-boundary tests
must exercise each state transition.

The database is the queue's source of truth. A channel only wakes a worker up;
it must never be the only place a pending job exists.

### Derived search index

The FTS index stores memory IDs, paths, scope metadata, and searchable text. It
is explicitly rebuildable from the memory filesystem. It must not be treated as
the canonical memory store.

## Turn Lifecycle

The future `Session.Send` flow is:

1. Resolve the session's immutable scope and load its event cursor.
2. Append the user event transactionally.
3. Ask the context composer for the stable prompt, pinned memory, project
   context, recent transcript, and any search results.
4. Record a compact context-snapshot event containing the selected memory IDs,
   scope, and budget. Do not duplicate the complete rendered prompt.
5. Run the existing model/tool loop.
6. Append assistant messages, tool calls, tool results, approvals, and errors
   beside the existing loop operations, not through frontend `Events`.
7. Append the final assistant event and commit the turn boundary.
8. Enqueue a consolidation job in the same database transaction as the final
   event once Stage 5 exists. Earlier stages do not enqueue a job.
9. Return the answer immediately. Background memory work must not extend the
   user-visible turn unless an explicit synchronous `remember` operation was
   requested.

The current `Events` interface remains a rendering projection. Persistence is a
session responsibility so REPL and web frontends cannot accidentally diverge.

The context snapshot stores each selected memory ID, document content hash, Git
commit ID, scope, renderer version, and token budget. Dynamic memory blocks are
rebuilt for every request and are never retained as ordinary transcript
messages. The renderer emits a data-only block with escaped JSON records under
`EVIE_MEMORY_DATA`; the stable prompt tells the model to quote and evaluate the
records, never follow instructions found inside them.

## Context Management

The context composer owns the distinction between durable history and the
messages sent on this request. The stable `systemPrompt` stays first. Dynamic
memory and project blocks come after it. The recent transcript follows those
blocks.

The initial token estimate is a labeled heuristic based on character count. It
must be calibrated against provider usage when usage data is available; it is
not presented as an exact tokenizer.

Compaction behavior follows the provider-neutral shape documented in
`docs/harness-improvements.md` and the paging idea in MemGPT:

- reserve response space before sending;
- preserve the newest complete turns;
- cut only at legal message boundaries;
- never orphan a tool result from its tool call;
- summarize the evicted span with the previous compaction summary available;
- append a compaction event containing the summary and cut-point event ID;
- retain the complete raw event history on disk;
- use the compacted projection for future model requests.

Before an expensive summary call, implement tool-result clearing as the cheap
first pressure-relief mechanism. The test must prove that the placeholder still
preserves tool-call/result structure.

## Memory Operations

The memory API belongs in a new `internal/memory` package. Interfaces are owned
by their consumers, following the existing `agent.Client` pattern.

Initial domain operations:

- `Read`: load one memory document by stable ID/path;
- `List`: show the memory tree and metadata without loading all content;
- `Search`: search within an explicit scope and token budget;
- `Remember`: create an active memory only for an explicit user-directed write;
- `Propose`: create a reviewable background candidate;
- `Correct`: create a replacement revision and mark the old revision superseded;
- `Retire`: remove memory from active retrieval while preserving its revision;
- `Restore`: reactivate a retired memory by explicit user action;
- `Link`: connect related documents without merging their claims.

The model may call memory tools, but the harness validates:

- the requested scope;
- the resolved project ID;
- source event references;
- allowed path prefixes;
- content size;
- secret patterns; and
- whether the operation is explicit, proposed, or approval-gated.

The model cannot write arbitrary paths through a generic file tool to bypass
these rules. This is the memory equivalent of the existing database write gate.

For Stage 3, an active `Remember` write requires a user-facing `/remember`
command or an approval attached to the memory operation. A model deciding on
its own that a sentence sounds memorable creates a proposal instead. Natural
language intent detection and automatic promotion remain later policy choices.

The generic `read_file` and `edit_file` tools must reject paths inside the
memory root. Existing `bash` remains a privileged exception until the harness
gets a separate shell-containment feature. The memory store therefore validates
the committed Git tree at startup and quarantines dirty or malformed documents
instead of silently treating arbitrary shell edits as trusted active memory.

The first explicit operation contract is:

| Operation | Default authority | Result |
|---|---|---|
| `memory_read`, `memory_list`, `memory_search` | read-only | current scoped data |
| `/remember` or approved `remember` | user | active memory operation |
| background extraction | model proposal | `proposed` document |
| `approve_proposal` / `reject_proposal` | user | active or discarded proposal |
| `correct` | user command or approval | new revision plus supersession |
| `promote_memory` | explicit user command or approval | new global document with source link |
| `link` | explicit user command or approval | relation only; no claim merge |
| `retire` / `restore` | explicit user command | reversible active-state change |

Retirement is a state transition on a derived memory document, not deletion of
its source event. Legal transitions are `active -> retired -> active`; a
`superseded` document is historical and cannot be restored as the current
revision. A `valid_to` timestamp still excludes a restored document from
current-state queries after its validity ends. Hard erasure remains a future
retention feature named `forget`.

## Background Consolidation and Go Concurrency

The first worker is intentionally small:

- one bounded goroutine owned by the process;
- a wake-up channel with a non-blocking send;
- the durable `memory_jobs` table as the actual queue;
- `context.Context` for shutdown and per-job cancellation;
- a lease/heartbeat so a crashed worker leaves a retryable job;
- exponential backoff with a maximum attempt count;
- one `sync.Mutex` around filesystem/Git writes;
- one cross-process lock directory around filesystem/Git writes, with an owner
  token and stale-lock recovery;
- one SQLite transaction per state transition.

Do not start one goroutine per conversation or job. Go makes concurrency cheap,
not coordination free. The useful lesson here is separating fast event handling
from durable work while making ownership and shutdown explicit.

The in-process mutex is not enough because the REPL, web server, and cron-style
processes may share `~/.evie/memory`. The process lock is acquired after the
durable operation intent exists and released only after the Git commit and index
refresh (or quarantine) are recorded.

Memory extraction should use a separate `MemoryExtractor` interface rather than
calling the main `Session.Send` recursively. The extractor receives a bounded
source event range and returns typed proposals. It cannot directly change the
stable system prompt or tool permissions.

The default policy is conservative:

- explicit user `remember` writes are active immediately;
- automatically extracted global memories are proposals;
- automatically extracted project lessons may be proposed first and promoted to
  active only after the policy is approved in a later stage;
- tool output, fetched pages, and assistant speculation are evidence candidates,
  not user truth.

This preserves Letta's background-learning shape while addressing the poisoning
problem demonstrated by AgentPoison.

## Staged Build Order

Each stage is independently demoable. Do not begin the next stage until the
current stage's tests and learning checkpoint are complete.

### Existing `serve` Boundary

`cmd/evie/docs/active/serve.spec.md` intentionally leaves persistence, resume,
and multi-session web state out of its first version. This memory feature does
not change that contract during Stages 0-4: the durable backend and REPL resume
come first. Before the final web demonstration, the serve spec and its
decisions file must be amended to define session IDs, resume behavior, project
selection, and reload recovery. Until that amendment is approved, web requests
may use a process-local session while the memory package is tested through the
REPL and package-level integration tests.

### Stage 0 - Run the reference locally

**Goal:** observe Letta's current behavior before copying its shape.

**Tasks:**

- run Letta with its local backend and a local model endpoint if desired;
- inspect the local MemFS tree after `/init`, `/remember`, compaction, and a
  project change;
- observe which files stay pinned and which are loaded on demand;
- inspect Git commits and dreaming behavior;
- record the Letta package/CLI version or Git commit used for the experiment;
- record differences between current Letta and the MemGPT paper in
  `memory.decisions.md`.

**Learning checkpoint:** understand why pinned memory, on-demand files, and
context compaction are separate mechanisms. Do not write Evie code in this
stage.

**Done when:** we can explain one complete Letta turn from input through memory
read/write and identify the subset Evie will reproduce.

### Stage 1 - Stable identity and append-only transcript

**Goal:** make Evie restartable before adding semantic memory.

**Tasks:**

- add session and event tables to `internal/eviedb`; reserve job and memory
  operation tables for the stages that consume them;
- add a provider-neutral event envelope and format version;
- give `Session` a stable session ID and a history dependency;
- persist each event at the point it is appended to the model transcript;
- load a session's current message projection on startup;
- add a minimal `--resume` or equivalent session-selection path;
- persist tool execution IDs and terminal status before considering resume safe;
- capture provider usage when available, or add an explicit configurable context
  window/reserve budget for providers that do not report it;
- preserve existing REPL and web behavior while both use the same history code.

**Go lesson:** `database/sql` transactions provide the commit boundary; the
`Session` mutex protects one live turn; SQLite's busy timeout handles competing
local processes. Do not use a frontend event callback as persistence.

**Tests:** fresh database schema, restart/resume, event ordering, unique
per-session sequences, tool-call and tool-result pairing, failed request
behavior, unknown side-effect recovery, concurrent session access, and a
process-safe append boundary.

**Done when:** killing and restarting Evie loses at most the in-flight event and
the resumed session can continue with its prior model-visible history. An
in-flight side effect blocks for explicit resolution rather than being replayed.

### Stage 2 - Context composer and compaction

**Goal:** prevent the model transcript from growing until the provider rejects
the request.

**Tasks:**

- separate stable prompt, dynamic blocks, and transcript projection;
- add context token estimation and a `/context` inspection view;
- add provider usage capture/configurable window limits as a prerequisite for
  automatic triggering;
- implement tool-result clearing;
- implement legal-boundary compaction and compaction events;
- preserve the full event history while shrinking only the sent projection;
- add a manual `/compact` command before automatic compaction.

**Go lesson:** keep pure message-selection and cut-point functions separate from
the LLM summarization side effect. Pure functions make the hardest boundary
rules table-testable.

**Tests:** legal cut points, no orphan tool results, split turns, repeated
compaction, failed summary calls, manual compaction, and context-budget reports.

**Done when:** a deliberately oversized tool output can run past the configured
context budget without a provider error, and `/compact` is observable in the
transcript. Exact tokenizer calibration is optional until the active provider
reports compatible usage data.

### Stage 3 - Local MemFS-style memory and explicit writes

**Goal:** implement the useful core of Letta's memory model locally.

**Tasks:**

- initialize `~/.evie/memory` as a user-only Git repository;
- implement the memory document format and versioned metadata;
- add the `memory_operations` table and startup reconciliation;
- implement pinned `system/` memory loading;
- implement on-demand tree listing and document reads;
- add dedicated `remember`, `memory_read`, and `memory_list` operations;
- reserve the memory root from `read_file` and `edit_file`;
- validate the committed tree at startup and quarantine dirty or malformed
  documents;
- commit successful memory changes with source event IDs in metadata;
- add a small memory block to the dynamic prompt, never the stable prompt.

**Go lesson:** use `os`, `filepath`, `io/fs`, and `os/exec` around the existing
Git CLI rather than adding a Git library before the behavior is understood.
Atomic file replacement plus a serialized commit mutex is the simple local
correctness boundary.

**Tests:** repository initialization, path traversal rejection, generic-file
fencing, metadata round-trip, atomic write failure, Git commit history, stale
process-lock recovery, pinned-size limits, explicit remember across restart,
and secret rejection.

**Done when:** `/remember that I prefer X` or an approved memory operation
creates a versioned local memory document, and the next process can retrieve it
without the original conversation. A normal sentence that merely contains a
preference creates no active memory in this stage.

### Stage 4 - Global/project hierarchy

**Goal:** make scope a harness property rather than a model guess.

**Tasks:**

- implement canonical project resolution at session creation;
- load global pinned memory for every session;
- load only the active project's pinned/reference manifest;
- prevent a project session from writing outside its project path unless the
  operation is an explicit global promotion;
- add `promote_memory` that creates a new global document linked to the project
  source rather than moving or copying it silently;
- expose the active project in REPL diagnostics.

**Go lesson:** resolve paths and IDs at the boundary, then pass an immutable
  value through the session. Do not read process-global `cwd` from deep inside a
  tool or worker.

**Tests:** two projects cannot read one another by default; global memory is
visible in both; promotion preserves links; changing shell `cwd` does not change
the session scope; REPL diagnostics show the canonical project.

**Done when:** the same idea can exist as project-local knowledge and as a
separately approved global generalization without scope leakage.

### Stage 4b - Web session integration

**Goal:** connect the persistent memory/session model to `evie serve` without
silently changing the existing single-session web contract.

**Tasks:**

- amend `serve.spec.md` and `serve.decisions.md` with the approved session model;
- expose a stable session ID and selected project in the web API;
- resume a selected session after page reload or server restart;
- define the project-selection boundary before accepting web memory writes;
- add the same origin/approval protections to memory endpoints.

**Done when:** a web conversation can resume, inspect its active scope, and
perform the same explicit memory operations as the REPL.

### Stage 5 - Durable background consolidation

**Goal:** add Letta-style continuous improvement without blocking the main turn.

**Tasks:**

- add the `memory_jobs` table and startup reconciliation for background work;
- enqueue one idempotent consolidation job after each committed turn;
- implement the bounded worker, leases, retries, and graceful shutdown;
- implement a strict `MemoryExtractor` result format;
- create proposals with source event IDs and scope;
- add a proposal list/review operation;
- make the worker observable through logs and a small status command;
- ensure a failed worker never mutates the active memory tree partially.

Background extraction has no silent remote fallback. Until a local
OpenAI-compatible endpoint such as Ollama, LM Studio, or llama.cpp is configured,
the worker remains disabled or leaves jobs pending; it must not send transcript
content to the main OpenRouter client. A fully offline Evie run also requires a
local main model, but local memory storage is guaranteed independently of that
main-model choice. The worker must have an offline fake-model test before any
live extractor is enabled.

**Go lesson:** channels are notifications; SQLite is durable coordination. A
worker should own cancellation, retries, and error reporting rather than
leaking goroutines from `Send`.

**Tests:** job idempotency, restart with pending work, lease expiry, backoff,
worker cancellation, concurrent turns, Git serialization, provider failure, and
proposal provenance.

**Done when:** a completed session creates a proposal in the background while
the user-facing turn latency remains unchanged, and restarting Evie resumes the
pending job.

### Stage 6 - Scoped retrieval and FTS

**Goal:** retrieve relevant reference memory without loading the entire tree.

**Tasks:**

- add an FTS5 index as a rebuildable projection of active memory documents;
- search project scope and global scope with hard filters first;
- return memory IDs, paths, scope, timestamps, and source references with text;
- order selected evidence chronologically when the query needs history;
- assemble a bounded dynamic evidence block;
- add `memory_search` for deeper model-directed lookup;
- record retrieval diagnostics for later evaluation.

**Go lesson:** keep search, ranking, and prompt rendering separate functions.
Start with SQLite FTS because it is local, fast, and inspectable; do not add
vectors merely because a paper used them.

**Tests:** exact names and aliases, project isolation, global fallback, token
budget truncation, chronological ordering, empty results, rebuild after index
deletion, and prompt-injection text treated as data.

**Done when:** a project memory too large to pin can still be found from a new
session using a relevant query, with source provenance visible to the model.

### Stage 7 - Corrections, expiry, links, and retirement

**Goal:** make memory editable without pretending old evidence never existed.

**Tasks:**

- implement `memory_correct` by creating a new revision and superseding the old;
- implement event-time and recorded-time fields in retrieval;
- implement `memory_link` for related projects, decisions, and lessons;
- implement reversible `retire` and `restore` semantics;
- reserve the name `forget` for hard erasure of source events, Git history,
  summaries, and indexes in a separately approved retention feature;
- rebuild summaries/indexes from remaining sources when a source is retired.

**Go lesson:** model state transitions explicitly instead of mutating a struct
and losing history. A small state machine is easier to test than scattered
`if status == ...` checks.

**Tests:** correction selects the current revision, historical queries still
see prior revisions, conflicting claims are surfaced, and expired memories
cannot resurrect content through the index or a cached prompt.

**Done when:** Evie can answer both "what is current?" and "what did I believe
before the correction?" while honoring explicit retirement and restoration.

### Stage 8 - EVIE memory evaluation

**Goal:** measure memory behavior instead of trusting one good conversation.

**Tasks:**

- create 10-20 replayable tests from real Evie tasks;
- add LongMemEval-inspired cases for extraction, multi-session recall, temporal
  updates, and abstention;
- add LoCoMo-inspired multi-hop and speaker/source cases;
- add EVIE-specific project isolation, promotion, correction, deletion, and
  tool-result cases;
- add AgentPoison-inspired untrusted-content admission tests;
- report retrieval recall, grounded answer correctness, scope leakage,
  unsupported writes, deletion completeness, latency, and token usage;
- run deliberate evaluations with `go test -tags eval ./...`.

**Go lesson:** use normal table tests for deterministic memory behavior and a
separate build-tagged evaluation suite for model-backed tests that cost time or
money. Keep the corpus and model configuration versioned.

**Done when:** changing the memory writer or retrieval policy produces a
reviewable before/after result rather than a subjective impression.

### Future Stage - Local embeddings or graph retrieval

Only begin this stage if Stage 8 shows a real retrieval failure that FTS cannot
solve.

- add an embedding interface with a local provider such as Ollama;
- keep embeddings as a rebuildable index keyed by memory ID;
- compare hybrid FTS+dense retrieval against FTS on EVIE's eval set;
- add graph links/traversal only for measured multi-hop failures;
- never let a similarity edge become an identity, authority, or truth claim.

## Session Dependency Seams

The current `agent.New` only receives a provider client and model name. Memory
requires an explicit dependency boundary before Stage 1 implementation:

- `History`: append events, load a session projection, and record execution
  state;
- `ContextComposer`: build the request-specific dynamic prompt and compaction
  projection;
- `MemoryService`: read, search, and submit validated memory operations;
- immutable `ScopeContext`: session/project identity;
- per-session tool factory: binds memory tools to the session's scope and
  operation authorizer.

The `agent` package owns these interfaces; `internal/eviedb` and
`internal/memory` provide implementations. Memory tools must be constructed per
session rather than registered as a global singleton, because a global tool
cannot safely know which project it is allowed to touch. The existing flat
registry can remain for ordinary tools; memory tools are supplied through the
session's existing per-turn extras seam or a small session-scoped tool factory.

## Anticipated Package Seams

These are targets for the staged build, not permission to refactor everything
now:

```text
internal/agent/
  agent.go          existing model/tool loop
  history.go        session event projection
  context.go        prompt assembly and compaction

internal/eviedb/
  db.go             existing database entry point and schema
  events.go         append-only event operations
  jobs.go           durable memory outbox

internal/memory/
  document.go       metadata and document parsing
  store.go          memory filesystem interface
  git.go            serialized local repository operations
  project.go        canonical project identity
  search.go         FTS/index projection
  worker.go         bounded background consolidation

internal/tools/
  memory.go         explicit memory operations and validation
```

The `agent` package owns interfaces consumed by the agent loop. Concrete SQLite,
filesystem, and Git implementations live below those interfaces. This mirrors
the existing `agent.Client` design and keeps the core testable with fakes.

## Definition of Done

The feature is shipped when all of the following are true:

- Evie resumes a durable session after restart.
- The complete provider-neutral event history remains locally inspectable.
- Context compaction runs without orphaning tool messages.
- Global and project memory are separated by a trusted project identity.
- Explicit memory writes survive restart and are Git-versioned.
- Background consolidation is durable, bounded, cancellable, and reviewable.
- FTS retrieval is scoped, token-bounded, chronological when requested, and
  source-linked.
- Corrections, retirement, and restoration have tested semantics.
- No typed memory or generic file operation can bypass path, scope, approval, or
  secret checks. The existing privileged `bash` escape hatch is explicitly
  excluded until its separate containment feature lands.
- `go test ./...`, `go vet ./...`, and `go test -tags eval ./...` pass.
- The feature has a live REPL demonstration and a web-session demonstration.
- The final decisions file records which open questions were resolved.

## Open Questions

These must be resolved before the relevant stage, not silently decided in code.

1. Should the web UI select a project through an endpoint, or should v1 use one
   configured `EVIE_PROJECT_ROOT`?
2. Should project proposals become active automatically after a confidence rule,
   or remain user-reviewed until we have evaluation evidence?
3. Which local model/provider should run background extraction, and how should a
   provider failure affect the retry budget?
4. What is the maximum pinned-memory budget for the system and active project
   blocks on the current model?
5. What exact hard-erasure semantics should a future retention feature provide
   beyond v1 retirement?

## References

### Research papers

- Park et al., [Generative Agents: Interactive Simulacra of Human Behavior](https://arxiv.org/abs/2304.03442), UIST 2023. Memory stream, retrieval, reflection, and planning.
- Packer et al., [MemGPT: Towards LLMs as Operating Systems](https://arxiv.org/abs/2310.08560), revised 2024. Context tiers, paging, interrupts, and archival/recall storage.
- Maharana et al., [Evaluating Very Long-Term Conversational Memory of LLM Agents](https://aclanthology.org/2024.acl-long.747/), ACL 2024. LoCoMo benchmark and evidence-linked observations.
- Wu et al., [LongMemEval: Benchmarking Chat Assistants on Long-Term Interactive Memory](https://arxiv.org/abs/2410.10813), ICLR 2025. Indexing, retrieval, reading, temporal updates, and abstention.
- Gutiérrez et al., [HippoRAG](https://arxiv.org/abs/2405.14831), NeurIPS 2024. Graph-assisted associative retrieval; not a complete memory lifecycle.
- Rasmussen et al., [Zep: A Temporal Knowledge Graph Architecture for Agent Memory](https://arxiv.org/abs/2501.13956), 2025 preprint. Episodes, entities, temporal edges, and provenance.
- Chen et al., [AgentPoison](https://proceedings.neurips.cc/paper_files/paper/2024/hash/eb113910e9c3f6242541c1652e30dfd6-Abstract-Conference.html), NeurIPS 2024. Retrieval-mediated memory poisoning and trust boundaries.
- Hu et al., [MemoryAgentBench](https://arxiv.org/abs/2507.05257), revised 2026. Retrieval, test-time learning, long-range understanding, and selective forgetting.
- Li et al., [LoCoMo-Plus](https://arxiv.org/abs/2602.10715), 2026 preprint. Cognitive memory beyond explicit factual recall.

### Implementations and primary documentation

- [Letta MemFS](https://docs.letta.com/concepts/memfs) - current Git-backed memory hierarchy, pinned files, on-demand files, and versioning.
- [Letta memory and dreaming](https://docs.letta.com/agent-sdk/memory/) - explicit teaching and background consolidation.
- [Letta self-hosting](https://docs.letta.com/self-hosting) - fully local state, local model options, and local storage paths.
- [Letta sessions and durability](https://docs.letta.com/agent-sdk/sessions/) - persistent agent/conversation state and turn semantics.
- [EVIE harness improvements](../../../../docs/harness-improvements.md) - existing transcript, compaction, memory, and eval direction.
- [EVIE agent loop](../../../../internal/agent/agent.go) - current model/tool loop and single-writer session.
- [EVIE prompt](../../../../internal/agent/prompt.go) - immutable prompt prefix and dynamic-block seam.
- [EVIE database](../../../../internal/eviedb/db.go) - local SQLite state and busy-timeout convention.
- [Go `context` package](https://pkg.go.dev/context) - cancellation and shutdown propagation.
- [Go `database/sql` package](https://pkg.go.dev/database/sql) - transaction and connection-pool boundary.
- [Go `sync` package](https://pkg.go.dev/sync) - mutex and worker coordination primitives.
- [Go `io/fs` package](https://pkg.go.dev/io/fs) - filesystem abstraction for memory-tree reads.
- [Go `os/exec` package](https://pkg.go.dev/os/exec) - controlled Git CLI integration.

### Security and design sources

- [Letta local/self-hosting documentation](https://docs.letta.com/self-hosting) - local state does not imply local inference; provider choice still matters.
- [LongMemEval repository](https://github.com/xiaowu0162/LongMemEval) - benchmark code and evidence annotations.
- [LoCoMo repository](https://github.com/snap-research/locomo) - long conversational-memory data and evaluation code.
- [AgentPoison code and paper](https://github.com/BillChan226/AgentPoison) - red-team implementation and threat model.
