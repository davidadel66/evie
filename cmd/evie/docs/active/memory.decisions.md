# memory - decisions

- **2026-08-12 - local Go-native implementation, not a hosted memory provider.**
  EVIE is a learning project and a personal local tool. Zep Cloud and Mem0
  Platform are useful comparisons, but using them would hide the memory work we
  are trying to understand and would put durable personal state behind a third
  party. Letta's local runtime is a reference implementation; EVIE will port a
  small, understandable subset into the existing Go harness.

- **2026-08-12 - Letta/MemGPT is the primary reference architecture.**
  MemGPT gives the context-tier and compaction model; current Letta gives the
  concrete Git-backed MemFS hierarchy, explicit memory editing, sessions, and
  background dreaming. We are not reimplementing the Letta App Server or SDK.

- **2026-08-12 - SQLite is the canonical event log.**
  The append-only invariant matters more than the JSONL file format. EVIE already
  owns `~/.evie/evie.db`, needs durable jobs and transactions, and already uses
  `modernc.org/sqlite`. Event rows will never be updated or deleted in the first
  memory release. This adapts the append-only transcript pattern described in
  `docs/harness-improvements.md` to EVIE's existing state boundary; this feature
  supersedes that document's JSONL storage choice for the memory implementation.

- **2026-08-12 - the semantic memory is a local Git-backed filesystem.**
  Letta's current MemFS is the closest implemented pattern to the requested
  global/project hierarchy. Files make memory inspectable and editable by David;
  Git gives version history and rollback. Git is therefore a required local
  dependency for this feature rather than an optional enhancement. SQLite
  indexes and metadata may be derived, but the memory documents remain the
  human-readable state.

- **2026-08-12 - memory Git identity is repository-local.**
  Memory initialization writes `Evie <evie@localhost>` into the memory
  repository's local Git config instead of requiring or modifying David's global
  Git identity. Dirty worktrees are preserved and quarantined, never reset
  automatically.

- **2026-08-12 - global and project scopes are harness-owned.**
  Project identity is resolved from a canonical root at session creation. A
  project memory cannot become global through automatic similarity or model
  preference. Promotion creates a separately sourced global document.

- **2026-08-12 - no graph or vector index in the first release.**
  HippoRAG and Zep show why graph associations can help multi-hop retrieval, but
  neither establishes that a graph is necessary for EVIE's current scale. FTS5
  is local, fast, inspectable, and enough to prove the indexing/retrieval/reading
  loop before adding embedding infrastructure.

- **2026-08-12 - background work uses a durable outbox plus a bounded worker.**
  Go goroutines and channels are useful for responsiveness, but a channel is not
  persistence. SQLite records the job; a worker owns cancellation, leases,
  retries, and one-at-a-time Git writes. This is the learning surface where Go's
  concurrency model earns its complexity.

- **2026-08-12 - explicit memory writes are active; automatic writes begin as proposals.**
  Letta allows agent-owned memory edits, but AgentPoison demonstrates that
  retriever-backed memory is a security boundary. EVIE will first make a
  user-facing `/remember` command or approved memory operation immediately usable
  and keep background extraction reviewable until EVIE-specific evaluations
  justify a more automatic policy.

- **2026-08-12 - immutable prompt, writable supplemental memory.**
  The stable system prompt remains a code-owned prefix. Runtime memory is a
  later data block. This follows the existing `internal/agent/prompt.go` seam and
  prevents memory edits from becoming silent system-policy edits.

- **2026-08-12 - preserve evidence and derive current views.**
  LongMemEval separates indexing, retrieval, and reading; Zep models event and
  transaction time; LoCoMo links observations to source turns. EVIE will retain
  source events, revision metadata, and validity intervals rather than silently
  overwriting old memory.

- **2026-08-12 - v1 retirement is reversible; hard forget is deferred.**
  Removing content from active retrieval while preserving Git history and source
  evidence is a reversible `retire` operation, paired with `restore`. Hard
  erasure must be designed
  separately because it must cover SQLite events, Git history, indexes, cached
  prompts, and derived proposals without leaving a reconstruction path.

- **2026-08-12 - persist provider payloads for replay, exclude reasoning from semantic memory.**
  Durable events need enough provider payload to understand and recover tool-call
  state, but reasoning fields are not user facts and must never be sent to the
  memory extractor or treated as evidence. This feature supersedes the v1
  process-only reasoning fence only for opaque local continuation payloads;
  unsafe payloads make a turn non-resumable rather than being persisted.
  Redaction and exact provider replay behavior remain part of Stage 1 tests.

- **2026-08-12 - no silent remote background extraction.**
  EVIE may continue using OpenRouter for the main conversation while local-model
  work is developed, but the background memory worker must use an explicitly
  configured local endpoint or remain disabled. Durable memory itself never
  leaves the machine through a hidden fallback.

- **Open - web project selection.**
  The first implementation may use `EVIE_PROJECT_ROOT` or the process root. A
  web project-picker endpoint should be specified separately once persistent web
  sessions are implemented.

- **Open - hard erasure semantics.**
  V1 retirement is decided. A future retention feature must decide whether David
  can erase source events, indexes, summaries, proposals, and Git history, and
  must test that forgotten content cannot reappear.
