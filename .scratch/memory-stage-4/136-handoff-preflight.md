# Ticket #136 implementation preflight

Prepared by a read-only subagent on `codex/memory-stage-4`, with HEAD
`a41aa6189191ed1ca3cdaa53e496269db489085d`. This is preparation for
[ticket #136](https://github.com/davidadel66/evie/issues/136), not implementation,
an extractor selection, or ticket completion. **#135 still blocks production
implementation.** No production files, branches, index entries, or commits were
changed; no tests, model inference, downloads, or installation were run.

## Authoritative scope read

- `/Users/davidboktor/.agents/skills/implement/SKILL.md` and repository `AGENTS.md`.
- `.scratch/memory-stage-4/published-bodies/05-bounded-candidate-compilation.md`.
- `cmd/evie/docs/active/memory-stage-4-evidence-contract.decisions.md` (#132).
- `cmd/evie/docs/active/memory-stage-4-work-contract.decisions.md` (#133).
- `cmd/evie/docs/active/memory-stage-4-review-contract.decisions.md` (#134).

The independently demonstrable outcome is one explicitly selected, bounded
source unit becoming a durable, unaccepted group or an explicit successful
zero-candidate completion, using either the selected local extractor or a
scripted extractor. Restrict proposed meanings to supported owner assertions,
existing unambiguous Entity identities, and already reviewed Predicates. The
implementation must include the first durable request guard and fences; it
cannot postpone those invariants to the recovery ticket.

Do not implement the supervisor, ongoing live scheduling, automatic recovery,
general historical backfill interface, broad candidate creation, owner
accept/reject, web inbox, generation upgrades, or release-gate evaluation here.
Those remain dependent outcomes. Inspection is read-only; it does not invoke
Stage 3 prepare/apply as a way of manufacturing accepted effects.

## Existing Kernel and adapter ownership

There is **no `internal/kernel` package**. The current Kernel-owned semantic
behavior is implemented by `eviedb.Store`, behind narrow interfaces owned by
its consumers:

| Current location | Relevant seam or behavior |
| --- | --- |
| `internal/eviedb/store.go:9` | `Store`, `NewStore`, and its shared `withImmediateTransaction` wrapper. Existing DB transaction hooks support deterministic failure checks. |
| `internal/agent/semantic_memory.go:15` | `SemanticMemory` prepare/apply/read interface; separate `SemanticGraphMemory` and `SemanticPromotionMemory` extensions demonstrate narrow optional capabilities. |
| `internal/plugins/memory.go:83` | Model-facing `SemanticMemoryKernel`. Compilation and candidate inspection do not belong in this tool-granted accepted-memory interface. |
| `internal/memory/semantic.go:172` | `EvidenceLocator` and semantic domain values already represent whole/range/pointer locators; existence of a type is not source admission. |
| `internal/memory/scope.go:58` | `ScopeContext`, registered IDs, and `Session.ScopeContext()`. Names and paths cannot substitute for registry identity. |
| `cmd/evie/main.go:57` | Composition root constructs `kernelStore`. Management dispatch at line 81 occurs before conversational OpenRouter construction. |
| `cmd/evie/management.go:28` | Existing short owner command parsing takes consumer interfaces and an `io.Writer`; a useful pattern for a bounded compile/inspect command. |
| `cmd/evie/repl.go:779` | Existing `/memory` handler and exact semantic rendering. Inspection is eventless. |
| `cmd/evie/repl_semantic_memory_test.go:26` | Public REPL/real-SQLite tests prove explicit memory and inspection make zero model calls. |

Minimal ownership recommendation for the future ticket owner, subject to the
selected extractor contract:

1. Put new transport-independent generation, source-window, candidate, and
   inspection records in a separate `internal/memory` file. Avoid enlarging the
   already dirty Stage 3 `semantic.go` file merely to add unrelated structs.
2. Keep scope resolution, evidence admission/projection, source-window sealing,
   accepted-context selection, durable attempt ownership, staging, and atomic
   publication behind an `eviedb.Store` compilation seam. Use new focused files
   for these rules and schema. Do not create a generic workflow subsystem.
3. The component that actually calls the extractor should own a small interface
   accepting only the sealed bounded request and returning an untrusted bounded
   result plus transport completion/release information. The scripted and real
   local adapters implement that same seam. Do not feed an ordinary agent
   session, tool registry, remote OpenRouter client, or mutable DB handle into
   extraction. Exact Go names/package placement are implementation choices,
   not decisions made by this preflight.
4. Add the smallest CLI compile/inspect adapter through the shared Kernel
   methods. A short owner command can avoid both conversational client startup
   and reviving a source session. Keep unavailable extraction local to the
   compilation feature; explicit Semantic Memory must remain available.

The review contract will later need a typed owner context independent of source
turn leases. This ticket must preserve the source/destination IDs and unresolved
review state needed by that seam, without implementing its approval protocol.

## Persistence and migration touchpoints

`internal/eviedb/db.go:1139` opens the DB, runs base schema, Workspace migration,
`ensureSemanticSchema`, and startup accepted-projection verification.
`internal/eviedb/semantic.go:272` shows the current additive/versioned schema
setup pattern. A separate compiler schema initializer called during DB open is
the smallest obvious attachment point; candidate storage must remain outside
accepted projection/replay tables and verification.

The new persistence needs, stated as roles rather than invented final table
names, are:

- Immutable full generation manifests with deterministic encoding/hash/version.
- The installed event append-position facility and explicit legacy cohort.
- Explicit source selection/ownership, sealed root cutoff/window manifest, and
  accepted-context snapshot bound to the request.
- Job identity, monotonically increasing attempt fence, attempt count, lease,
  pause/failure reason, and the shared runtime request/capacity reservation.
- Entire bounded staging envelope and hash, its consumption receipt, stable
  candidate group/items, unresolved review metadata, and exact coverage outcome.

The work contract specifies group identity as job identity and separate stable
item identities. A duplicate selection/delivery/reopen must find the same
logical result; it cannot call the extractor again merely because a process
restarted. Missing output, invalid structure, and source errors are not empty
success. Deterministic exclusion has its own outcome and content-free reasons.

`internal/eviedb/events.go:232` wraps `AppendEventWithLease` in a fenced
`BEGIN IMMEDIATE` transaction; private `appendEvent` at line 254 performs the
immutable event insert at line 313. This is the existing append integration
point. Its executor exposes only `queryRowContext`, so an implementation must
either stay within that transaction abstraction or use a schema-owned append
mechanism; a second independent DB write after return would violate atomicity.

Install the event-ID-to-commit-position side record without rewriting episodes
or assigning historical rows an invented order. Appends after installation need
their positions even when extraction is disabled. Do not turn this necessary
storage foundation into live activation/scheduling in #136. A later #138 append
obligation must be able to participate in the same transaction.

`internal/eviedb/immediate_tx.go:78` owns `BEGIN IMMEDIATE`, cancellation before
commit, bounded transaction resolution, rollback, and poisoned-connection
discard. Use this existing boundary for lease/slot claims, staging, and
publication; never hold its transaction open during inference. The finished
group, completed interval, and consumed stage must commit together, with no
accepted operation or semantic revision advance.

Even this one-unit path must claim its request durably before dispatch, check
the current unexpired fence before effects, and preserve an uncertain runtime
slot as `release_pending`. Client cancellation, process death, timeout, and
lease expiry are insufficient proof of server release. The #135-selected
runtime protocol must supply the concrete release evidence. Automatic lease
recovery and retry scheduling can remain for #137; dropping the guard or
fencing from the first request cannot.

No accepted-operation schema-version bump is needed merely to store an
unaccepted group. Later owner acceptance and replay must introduce their
versioned authority envelope together, under their own ticket.

## Source, scope, and secret boundaries

- `internal/eviedb/events.go:680` currently loads all events for a session.
  Production window construction needs bounded SQL reads by captured sequence;
  loading an entire lifetime history and slicing afterwards misses the stated
  memory/transaction bounds.
- `internal/eviedb/turn_leases.go:325` reads a lease, while line 251 is the
  existing fenced write helper. Closure must observe source cutoff and no-live-
  lease status in one serialized decision, not two independent calls. Do not
  acquire a conversational lease or fabricate terminal events to make a source
  eligible.
- `internal/eviedb/semantic.go:1031` checks **active** source sessions for Stage 3
  mutation. Do not weaken that validator to compile retained sources or inspect
  a closed session. Implement the compilation/inspection lineage check for its
  own authority and exact scope.
- `internal/eviedb/semantic.go:1069` and
  `internal/eviedb/semantic_entity.go:272` prepare explicit accepted-memory
  proposals; they may create identities/Predicates and bind whole owner events.
  They are useful semantic-validation references, not a ready compilation API.
- `internal/eviedb/semantic_lifecycle.go:895` currently obtains source evidence
  directly from `events.content`. It is **not** a range projection resolver.
  New candidate inspection must use the same exact resolver as extraction input
  and validation, never this full-event fallback. Existing accepted whole-source
  behavior need not be broadened before #140 introduces accepted range sources.
- Existing secret patterns are in `internal/plugins/memory.go:230`, applied to
  serialized model-facing accepted reads in `renderMemoryRead`. This plugin
  gate is not the required code-owned, versioned compiler detector. Keep the
  detector at the source/candidate boundary, pin its policy, and use synthetic
  exact fixtures; reuse patterns where appropriate without making the plugin a
  dependency of `eviedb`. Whole detected fields must be excluded before input,
  not redacted into replacement evidence. Scan proposed text and projections
  before reviewable persistence and inspection.

#136 only admits its narrow owner-assertion case. Tool observation creation is
#143; the #132 clock contract does not authorize implementing broad tool support
now. Omit prohibited tool/compaction/reasoning/diagnostic content, and distinguish
assistant interpretation context from supporting evidence. Structure checks
cannot establish truth, usefulness, or entailment.

The Kernel, not extractor output, binds destination, source-session/context IDs,
authority, exact projected bytes, hashes, versions, and observation times. The
latest required support must belong to newly owned evidence. Accepted-state
context uses already accepted, permitted identities/Predicates and pinned scope
revisions; candidates cannot become identity-resolution input for later jobs.

## Generic storage fences

`internal/tools/db.go:24` has an **allowlist**, not a semantic-table denylist:
Evie SQL tools can read only `jobs` and `job_runs`. New compiler tables should
remain denied automatically. Extend `internal/tools/memory_fence_test.go:25`
with every new memory-owned table/view, including joined, nested, qualified,
and quoted attempts. `edit_db` already refuses Evie writes entirely.

`internal/tools/file.go:38` protects `~/.evie/evie.db`, its WAL, and SHM files;
`resolvePath` at line 121 checks lexical and resolved symlink paths. Keeping
compiler data in the existing DB inherits these file-tool fences. If the ticket
introduces another storage artifact, its paths and sidecars must be included
and exercised through read/edit/write file operations. Avoid adding such an
artifact without a concrete need.

The pre-existing **untracked** `internal/eviedb/database_browser.go:319` permits
generic records only for jobs/job_runs; unknown tables default to no rows, and
`semantic_` tables require a typed view. New compiler tables therefore do not
require editing this user-owned file merely for protection. Its
`internal/web/database.go` adapter and associated tests are also pre-existing
untracked work; preserve them.

Do not claim these fences sandbox arbitrary shell execution:
`internal/tools/bash.go:47` expressly documents a separate, ungated shell
boundary. This ticket's generic SQL/file criterion does not authorize a shell
policy redesign.

## Deterministic implementation checks to carry forward

Use the pre-agreed public Kernel seam, real temporary SQLite, a scripted
extractor, CLI adapters, and local HTTP fixtures. Existing examples are
`internal/eviedb/semantic_public_test.go:15`,
`internal/eviedb/semantic_process_race_test.go:42`,
`internal/eviedb/immediate_tx_test.go:15`, and the REPL test cited above.

The bounded suite should show:

1. Existing owner/Entity and reviewed Predicate setup, eligible committed root,
   one narrow result, exact inspection, durable reopen, and repeated delivery
   without a second logical outcome or model call; explicit empty is separate.
2. Unicode byte ranges including combining marks/emoji; malformed/null/missing
   locators; wrong event/hash/version/session/scope; prohibited/secret content;
   assistant context never support; no full-event leakage. Use #132 fixture
   hashes and scenarios as oracles.
3. Source cutoff and live-lease closure; no invented terminal; overlap has no
   coverage; oversized/invalid sources leave gaps and do not call extraction.
4. Two stores/processes contend for the same job and one shared inference slot;
   stale results cannot stage, publish, alter coverage, or release a replacement
   request. Publication failure/cancellation rolls back the entire outcome.
5. Local HTTP success, explicit zero, malformed/truncated/oversize response,
   nonlocal URL, redirect, proxy/environment escape, cancellation, unverifiable
   runtime/model identity, and unknown server-release state. A configured Unix
   endpoint is conditional on the selected adapter's supported contract.
6. Candidate persistence changes no accepted query/traversal/replay state; all
   new storage remains protected; disabled/unavailable extraction leaves Stage 3
   explicit commands working without invoking it.

Use focused red/green checks during implementation. Format changed Go files;
run `./scripts/verify-change.sh` once at stable code handoff, then independent
Standards/Spec review. The root owns exact index selection and the single #136
commit window. No tests were executed for this read-only preparation.

## Dirty files to preserve and commit boundaries

The complete pre-task preservation record is
`.scratch/memory-stage-4/implementation-baseline.json` (`preexisting_paths`),
owned by the root. Do not overwrite or regenerate it. It includes both tracked
changes and untracked/deleted files; root-owned scratch coordination changes
are accounted separately.

Especially relevant collisions at inspection time:

| Pre-existing path | Observed change to preserve |
| --- | --- |
| `AGENTS.md` | Modified user guidance; do not stage as part of #136. |
| `cmd/evie/main.go:201` | Existing `ServeContextManaged` call adds another `kernelStore` argument for the user's database UI. If CLI wiring changes this file, stage only the #136 hunk. |
| `internal/memory/semantic.go:374` | Existing `SemanticObjectSummary.Subject` and `.ObjectEntity` additions. Prefer a new compiler type file. |
| `internal/eviedb/semantic_graph.go:618` | Existing population of those subject/object fields in exact object collection. Do not overwrite or stage. |
| `internal/eviedb/database_browser.go`, `database_browser_test.go` | Pre-existing untracked database schema/row browser. Do not take ownership. |
| `internal/web/database.go`, `database_test.go` | Pre-existing untracked web database adapter/tests. |
| `internal/web/context_sessions.go`, `context_sessions_test.go`, `memory_test.go`, `serve.go` | Modified web behavior outside this ticket. |
| `internal/web/ui/**` | Many modified/untracked/deleted UI files. No #136 UI work is required. |
| `cmd/evie/docs/active/memory.spec.md`, `memory.decisions.md`, and untracked `semantic-memory-stage-4.spec.md` | Existing design work; consume as authorized context, never sweep into the ticket commit. |
| `scripts/memory-extractor-spike/**`, `cmd/evie/docs/fixtures/memory-stage-4-spike/**`, `cmd/evie/docs/research/memory-stage-4-local-extractor-spike.md` | Concurrent #135 owner work. Do not edit, import its executable as production code, stage, or evaluate independently. |

Other pre-existing research/ADR/output files and the Vite test result cache also
remain user-owned. Repository verification may update a generated cache;
report it to the root rather than sweeping it into the commit or restoring
unrelated changes.

## Extractor-dependent unknowns before implementation

#135 must deliver an adequate selected model/runtime/protocol and complete
generation identity. No selection is inferred from cached artifacts or partial
results. The future ticket owner needs these specific outputs:

- Final model artifact/quantization digests, runtime/protocol compatibility,
  tokenizer/chat-template identity, prompt/schema bytes, decoding parameters,
  output validation version, and request completion marker.
- A production **general-input** context-fit mechanism. Empirical token budgets
  for frozen spike cases cannot authorize arbitrary real episodes. Reserve
  output and template/schema overhead and reject unknown/oversized inputs before
  dispatch; never silently truncate source context.
- Concrete runtime identity verification and endpoint configuration. A mutable
  model alias or an HTTP response's model name alone cannot complete the pinned
  identity guarantee; proxy/redirect/fallback behavior must be closed in code.
- The measured cancellation and request-release protocol, including what can
  prove a controlled server restart. Without evidence of release, keep the
  durable capacity block; do not invent an idle API or a time-based release.
- Candidate schema mapping for existing IDs/reviewed Predicates. The script's
  synthetic corpus/output schema may need a narrower production adapter; keep
  schema validation and semantic admission separate and avoid importing gold
  fixtures or evaluator adjudications into runtime input.
- Final experiment limitations and next approved option if the cached model
  fails. Do not lower requirements, claim adequacy from schema compliance, or
  begin #136 against an unselected model to bypass its dependency.

No unresolved item above is a request for a new product decision from the user;
this document identifies the concrete handoff #135 must supply or the technical
work #136 must implement under the approved contracts.
