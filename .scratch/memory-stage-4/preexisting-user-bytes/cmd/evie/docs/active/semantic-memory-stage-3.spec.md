## Problem Statement

Evie has restart-safe Episodic Memory and bounded Working Memory, but it does not
yet have an implemented Semantic Memory layer. The owner cannot explicitly
record a durable proposition, inspect what Evie currently knows, distinguish a
past error from a real-world change, trace knowledge to its evidence, or verify
that accepted knowledge survives restart and projection rebuild.

The existing memory roadmap mixes deterministic semantic state with later model
extraction and relevance retrieval concerns. Implementing those together would
make it impossible to tell whether a failure came from the temporal graph, model
judgment, entity resolution, indexing, ranking, or answer generation. It would
also risk allowing a First-party Plugin or conversational model to become the
authority for scope, accepted knowledge, provenance, and recovery.

The owner needs Stage 3 to establish a trustworthy Semantic Memory foundation:
explicit, source-linked, scoped, temporal knowledge in SQLite; a canonical
accepted-operation history; deterministic replay; exact inspection; and one
Kernel-owned interface shared by every user surface. Automatic extraction and
relevance retrieval must remain later stages built on top of that foundation.

## Solution

Add a Kernel-owned Semantic Memory module backed by SQLite. Accepted Semantic
Operations are the canonical history, and a bitemporal property graph of
Entities, Aliases, Predicates, Claims, Source Links, structural Graph Links, and
lifecycle state is their deterministic query projection. Every mutation begins
as a typed Memory Operation Proposal in response to an explicit owner request,
passes the existing approval path, revalidates its exact Scope Revision vector,
and commits its complete effects atomically.

Expose a small prepare/apply/read interface. The REPL, basic web Memory tab,
deterministic evaluation fixtures, and a compiled First-party Memory Plugin all
use that same interface. Focused model-facing tools are adapters; they do not own
SQLite, scope, approval, replay, or semantic truth. Local inspection supports
current, historical, as-known-at, provenance, lifecycle, and deterministic
one- or two-hop views without relevance ranking.

Add read-only verification and owner-only shadow rebuild. Establish versioned,
model-free conformance fixtures and reproducible performance baselines so later
extraction, retrieval, and policy changes can be compared without hiding a
deterministic regression behind an aggregate model-quality score.

## User Stories

1. As Evie's owner, I want to explicitly ask Evie to remember a proposition, so that accepted knowledge is deliberate rather than inferred silently.
2. As Evie's owner, I want every proposed memory change shown before approval, so that I can inspect its scope, proposition, evidence, time, and dependent effects.
3. As Evie's owner, I want one approval to accept one complete compound change, so that creating required Entities, Aliases, Predicates, Claims, and Source Links is understandable and atomic.
4. As Evie's owner, I want ordinary conversation to remain Episodic Memory in Stage 3, so that Evie does not autonomously convert every statement into accepted Semantic Memory.
5. As Evie's owner, I want every accepted Claim linked to exact evidence, so that I can understand why Evie knows it.
6. As Evie's owner, I want accepted knowledge distinguished from objective truth, so that conflicting evidence can remain visible rather than being silently collapsed.
7. As Evie's owner, I want stable Entity identities, so that names can change without changing who or what a Claim refers to.
8. As Evie's owner, I want two people with the same name to remain separate Entities, so that ambiguous Aliases never force an unsafe merge.
9. As Evie's owner, I want an ambiguous Alias lookup to show every eligible Entity, so that I can select a stable identity explicitly.
10. As Evie's owner, I want canonical owner, Evie, Workspace, project, and session Context Entities, so that repeated operations do not invent duplicate anchors.
11. As Evie's owner, I want canonical versioned Predicates, so that the same relationship has one meaning across every memory scope.
12. As Evie's owner, I want a new Predicate definition shown during approval, so that the model cannot silently invent relationship semantics.
13. As Evie's owner, I want exact Typed Literals, so that dates, datetimes, decimals, integers, booleans, and text have reproducible equality.
14. As Evie's owner, I want explicit affirmed and denied Claim Polarity, so that absence remains unknown rather than being mistaken for negation.
15. As Evie's owner, I want Predicate Cardinality represented separately from negation, so that single-valued and multi-valued relationships are not conflated.
16. As Evie's owner, I want deterministic conflict warnings, so that opposite-polarity and overlapping single-cardinality Claims are visible without a model judge.
17. As Evie's owner, I want conflicting accepted Claims preserved with their sources, so that Evie never hides disagreement by silently selecting a winner.
18. As Evie's owner, I want ordinary world relationships represented as Claims, so that Graph Links have a narrow and predictable structural meaning.
19. As Evie's owner, I want explicit structural Links for derivation, generalization, and recognized contradiction, so that relationships among memory records are inspectable.
20. As Evie's owner, I want a Claim's Valid Time separated from its Transaction Time, so that Evie can answer both when something applied and when it learned it.
21. As Evie's owner, I want a correction to distinguish a prior error from a real-world change, so that history remains temporally accurate.
22. As Evie's owner, I want real-world changes to close the old validity interval and begin the replacement at an explicit effective time, so that historical queries remain correct.
23. As Evie's owner, I want corrections of errors to preserve the original validity interval unless I replace it explicitly, so that recording a correction does not invent a world-time change.
24. As Evie's owner, I want retirement to be reversible, so that excluding knowledge from current use does not destroy its history.
25. As Evie's owner, I want restoration to fail after an intervening supersession, so that an obsolete Claim cannot silently become current again.
26. As Evie's owner, I want Entity retirement to enumerate every affected Alias, Claim, and Graph Link, so that dependent state never changes through a hidden cascade.
27. As Evie's owner, I want Source Link retraction and restoration to be append-only, so that evidence eligibility has an auditable history.
28. As Evie's owner, I want an Unsupported Claim retained but excluded from normal current retrieval, so that restoring eligible evidence can recover it without inventing another Claim.
29. As Evie's owner, I want equivalent propositions to attach additional evidence to one Claim, so that corroboration does not create duplicate graph relationships.
30. As Evie's owner, I want global Semantic Memory available across Context Scopes, so that owner-wide knowledge remains reusable.
31. As Evie's owner, I want Workspace Semantic Memory isolated from projects and sibling Workspaces, so that ongoing areas of life and work cannot leak into one another.
32. As Evie's owner, I want project Semantic Memory isolated from sibling projects and Workspaces, so that repository-specific knowledge remains scoped.
33. As Evie's owner, I want session Semantic Memory visible only to the same session, so that temporary accepted context does not silently become durable elsewhere.
34. As Evie's owner, I want archived session memory preserved for audit and explicit inspection, so that session closure never deletes accepted history.
35. As Evie's owner, I want broader reuse to require Promotion, so that a narrower Claim never changes scope in place.
36. As Evie's owner, I want Promotion to preserve its narrower evidence without exposing disallowed source text, so that provenance survives without widening access.
37. As Evie's owner, I want model-called writes bound to the session's default Context Scope, so that model arguments cannot select or widen memory scope.
38. As Evie's owner, I want a local operation to choose the current session scope explicitly, so that temporary accepted memory remains possible without widening authority.
39. As Evie's owner, I want narrower and global Claims returned together with scope labels, so that contextual differences are not treated as silent overrides.
40. As Evie's owner, I want current memory inspection to have explicit temporal defaults, so that the meaning of current is reproducible.
41. As Evie's owner, I want historical and as-known-at queries, so that I can inspect both past world state and Evie's past accepted knowledge.
42. As Evie's owner, I want exact provenance and lifecycle timelines, so that I can trace every accepted transition and supporting source.
43. As Evie's owner, I want deterministic one- and two-hop traversal, so that I can inspect local graph structure before relevance retrieval exists.
44. As Evie's owner, I want stable paginated inspection, so that concurrent writes cannot mix multiple Scope Revisions into one listing.
45. As Evie's owner, I want a basic `/memory` command, so that I can inspect Semantic Memory without creating an Episodic Memory event or model call.
46. As Evie's owner, I want `/memory inspect` to show identity, scope, Claims, provenance, and history, so that individual records are understandable from the CLI.
47. As Evie's owner, I want a conventional web Memory tab, so that I can browse scopes, Entities, Claims, provenance, and history without reading SQL.
48. As Evie's owner, I want local inspection to select one scope explicitly, so that the UI never silently mixes sibling Context Scopes.
49. As Evie's owner, I want model-facing memory inspection bounded and source-safe, so that remote egress never bypasses scope, secret, or untrusted-data rules.
50. As Evie's owner, I want focused `memory.*` tools, so that model tool schemas and approval previews remain understandable.
51. As Evie's owner, I want disabling the Memory Plugin to remove model-facing tools without deleting or disabling local memory, so that plugin composition never owns semantic truth.
52. As Evie's owner, I want retries of the same operation to return the original result, so that timeouts and duplicate submissions do not duplicate memory.
53. As Evie's owner, I want stale proposals rejected, so that approval cannot apply an intent formed against older Semantic Memory.
54. As Evie's owner, I want multi-scope Promotion to validate both source and destination revisions, so that changed or retracted evidence cannot be widened through an old approval.
55. As Evie's owner, I want accepted Semantic Operations to reproduce the same graph without model calls, so that recovery and evaluation do not depend on nondeterministic extraction.
56. As Evie's owner, I want `/memory verify` to compare accepted operations with a shadow projection, so that divergence is detectable without changing live state.
57. As Evie's owner, I want an owner-only shadow rebuild, so that a verified projection can be recovered without deleting Episodic Memory or accepted operations.
58. As Evie's owner, I want only affected scopes quarantined after verification failure, so that unrelated memory and conversation remain operational.
59. As Evie's owner, I want unknown operation schema versions to fail closed, so that an older Evie never skips history it cannot understand.
60. As a maintainer, I want one deep Semantic Memory interface shared by callers and tests, so that scope, temporal, lifecycle, approval, and replay rules have one implementation.
61. As a maintainer, I want semantic graph conformance evaluated without model calls, so that deterministic regressions are attributable.
62. As a maintainer, I want extraction, retrieval, and answer quality reported separately from semantic conformance, so that one aggregate score cannot hide a safety failure.
63. As a maintainer, I want reproducible latency, storage, and replay baselines, so that future optimizations have comparable before-and-after evidence.
64. As a maintainer, I want generic SQL and file fences regression-tested against every semantic table and SQLite sidecar, so that typed inspection remains the only ordinary access path.
65. As a maintainer, I want restart and two-process tests, so that local concurrency and persistence guarantees do not rely on one process or in-memory locks.
66. As a plugin author, I want the Memory Plugin to receive a narrow Kernel-owned interface rather than a database handle, so that Capability code cannot bypass semantic invariants.

## Implementation Decisions

- Stage 3 is the deterministic Semantic Memory foundation. It includes accepted
  state, explicit mutation, exact inspection, temporal behavior, provenance,
  replay, recovery, and model-free evaluation. It does not include automatic
  extraction, Memory Candidates, fuzzy entity resolution, relevance retrieval,
  embeddings, or context injection.
- Workspace session identity and the canonical Workspace registry are a
  prerequisite. The semantic schema includes global, Workspace, project, and
  session scopes from its first version rather than adding Workspace through a
  later migration.
- The Kernel owns Semantic Memory truth, scope and authority enforcement,
  approval requirements, accepted operations, provenance, lifecycle, replay,
  quarantine, and recovery. None of those responsibilities becomes a Plugin.
- The primary seam is one deep Kernel-owned Semantic Memory interface. Its
  mutation path prepares a typed Memory Operation Proposal and applies the exact
  approved proposal after revalidation. Its read path exposes closed exact query
  variants for listing, inspection, Claim lookup, provenance, history, and
  traversal.
- CLI commands, web handlers, conformance fixtures, and focused Memory Plugin
  tools all cross that same interface. No surface edits semantic tables directly
  or implements its own scope, lifecycle, temporal, or approval rules.
- The First-party Memory Plugin is a tool adapter using the existing focused
  Tool Capability Provider role. It exposes separate, versioned capabilities for
  focused read and mutation tools. A general Memory Provider seam remains
  deferred until Stage 5 has a real context-composition consumer and multiple
  meaningful adapters.
- Disabling or replacing the Memory Plugin affects model-visible capabilities in
  new resolved compositions but cannot delete, reinterpret, quarantine, rebuild,
  or make local inspection unavailable. Composition Receipts identify the exact
  Memory Capabilities and tool schemas available to each session.
- The built-in standard Agent Preset lists the Stage 3 Memory Capabilities as
  optional requirements. When the compiled plugin is enabled and healthy, new
  standard sessions receive the focused tools and pin them in their Composition
  Receipts. Intentionally disabling the plugin leaves new standard sessions
  usable with an explicit composition warning and no model-facing memory tools.
- Stage 3 mutations are session-bound. Every accepted Claim cites the exact
  owner-request event or other already-eligible Episodic Memory evidence.
  Eventless inspection, verification, and maintenance operations cannot create
  Semantic Memory. Non-conversational imported Sources remain a later feature.
- A Memory Operation Proposal is unaccepted intent, not Semantic Memory. Stage 3
  adds no semantic proposal table. Session-bound proposals and approval decisions
  remain Episodic Memory; an accepted operation stores the proposal hash and
  complete normalized effect.
- A compound proposal enumerates every reused or created Predicate, Entity,
  Alias, Claim, Source Link, Graph Link, and lifecycle transition. One approval
  creates one atomic Semantic Operation; any failed precondition changes no
  semantic row or Scope Revision.
- Accepted Semantic Operations are the canonical Semantic Memory history. Each
  operation contains a random stable operation ID, idempotency key, schema
  version, actor, scope, source event identities, proposal hash, complete
  normalized effect including generated IDs, prior and resulting Scope
  Revisions, and normalized Transaction Time.
- The queryable property graph is a deterministic projection of accepted
  operations. Replaying it performs no model call, Capability call, extraction,
  network request, external effect, or Episodic Memory recompilation.
- Semantic objects and operations use random stable UUIDs. Production generates
  IDs once and records them in the accepted operation; replay reuses the recorded
  IDs. Deterministic fixtures provide fixed generated IDs.
- A canonical scope registry represents global, Workspace, project, and session
  scope identities. Every semantic row references one canonical scope identity
  rather than encoding ad hoc nullable scope combinations.
- Accepted Semantic Memory has a monotonic Scope Revision per scope. Each
  proposal records the revision vector on which its intent depends. Apply uses
  compare-and-set semantics; a stale vector rejects the complete operation and
  requires a fresh proposal.
- Idempotency is distinct from proposition equality. Repeating one idempotency
  key returns the original operation result without incrementing a revision.
  Independently proposing an equivalent Claim attaches eligible evidence to the
  existing Claim rather than being treated as the same request.
- Promotion reads a narrower source scope and writes a broader destination scope.
  It validates the complete source/destination revision vector atomically. It
  creates any required broader Entity and Claim identities, leaves narrower
  objects unchanged, and retains ID-level provenance while expanding source text
  only for callers allowed to inspect the source scope.
- Stage 3 creates stable global owner and Evie anchor Entities. It creates one
  scope-local Context Entity for each Workspace, project, or session when first
  needed and retains a one-to-one reference to the registered context identity.
  Archiving a context never deletes its Context Entity.
- Entities contain stable identity, scope, canonical name, type, provenance, and
  append-only lifecycle. They contain no model-derived summary, extractor
  confidence, prompt version, or model version.
- Aliases are accepted semantic objects with stable IDs, scope, provenance, and
  append-only lifecycle. Multiple Entities in the same scope may have the same
  normalized Alias; exact resolution returns ambiguity and mutation requires a
  stable Entity ID.
- Predicate definitions are global, append-only, and versioned. A definition
  contains a canonical validated token, human label, allowed object kind or
  type, and expected cardinality of one or many. Claims retain the Predicate
  definition version under which they were accepted. Stage 3 performs no synonym
  inference.
- Typed Literal kinds are initially closed to text, signed integer, exact
  decimal, boolean, calendar date, and UTC datetime. Arbitrary JSON,
  floating-point values, money, duration, quantity, and implicit coercion are
  rejected until their equality and unit semantics are specified.
- Transaction Time uses one normalized fixed UTC precision. Scope Revision is
  the deterministic ordering authority when wall-clock timestamps collide.
  Calendar-date Typed Literals retain date precision; UTC datetime values are
  normalized; unknown or open Valid Time bounds are null.
- Claims contain immutable scope, subject Entity, Predicate definition version,
  Entity or Typed Literal object, affirmed or denied polarity, and proposition
  content. Model confidence and extractor configuration belong to future Memory
  Candidates, not accepted Claims.
- Proposition equality is scope, subject, Predicate, object, polarity, and Valid
  Time. Provenance is excluded from equality so new eligible evidence creates a
  Source Link to the existing Claim rather than another Claim.
- Predicate Cardinality is diagnostic, not a database uniqueness rule. Exact
  inspection computes opposite-polarity warnings for the same proposition and
  overlapping single-cardinality warnings for distinct affirmed objects. It
  does not persist a contradiction Link, reject accepted conflicting evidence,
  or choose a winning Claim.
- Claims represent temporal propositions about the represented world. Graph
  Links use a closed structural relation set among semantic records for meanings
  such as derivation, generalization, and explicitly recognized contradiction.
  Generic Entity-to-Entity relationships remain Claims.
- Source Links retain exact source event identity and evidence location, source
  actor/type, deterministic authority class, observed time, and append-only
  eligibility state. Source authority is preserved for inspection and later
  evaluated policy; Stage 3 never silently resolves conflicting Claims from it.
- A Claim with no eligible active Source Link is an Unsupported Claim. It remains
  accepted historical state but is excluded from ordinary current results.
  Restoring an eligible Source Link may return it to current results without
  creating or restoring the Claim itself.
- Claims, Entities, Aliases, Source Links, and Graph Links use append-only state
  transitions. Cached current status is a disposable projection. Retirement is
  reversible; Hard Erasure is not implemented.
- Entity retirement never cascades silently. The proposal enumerates dependent
  Alias, Claim, and Graph Link transitions and applies them atomically, or the
  owner retires dependents separately. Restoration applies only to objects whose
  current state remains legally eligible and never bypasses a supersession.
- Claim corrections create a replacement Claim and append supersession state
  without rewriting proposition content. `error` mode inherits the prior Valid
  Time unless the owner supplies a replacement. `changed` mode requires an
  effective time that closes the old half-open interval and begins the new one.
- Current exact reads default Valid Time and Transaction Time to the captured
  current instant, require active lifecycle and at least one eligible Source
  Link, and echo the effective temporal parameters. Historical reads may set
  `valid_at`, `as_known_at`, or both.
- Exact reads include paginated scope/object listing, object inspection, Claim
  filtering, provenance, lifecycle/operation history, and deterministic one- or
  two-hop traversal. Keyword or semantic relevance search is not part of Stage
  3.
- Listing cursors are opaque and bound to the initial Scope Revision, query
  variant, filters, and ordering. A cursor that cannot preserve that snapshot
  fails visibly rather than continuing against newer state.
- Traversal reapplies allowed scope, lifecycle, Valid Time, Transaction Time,
  Source eligibility, and relation constraints at every hop. It never traverses
  ID-only cross-scope provenance. Result and path ordering is canonical.
- Exact queries return every allowed Claim with its scope. A narrower Context
  Scope is labeled more specific but does not silently override global memory.
  Later presentation may order current-context Claims first without removing
  the global record.
- Local CLI and web inspection may render eligible source excerpts and event
  details. Model-facing reads require remote-memory opt-in, bounded output,
  secret scanning, source-scope redaction, and explicit untrusted-data rendering.
  Disallowed source scope may expose a safe source identity but never source
  text.
- The REPL includes eventless `/memory` listing and `/memory inspect` behavior.
  The Stage 3 web Memory surface is read-only: explicit exact-scope selection,
  Entity and Claim records, current/history filters, detail, provenance, and
  operation history. The UI Data hub amendment may project its bounded current
  result as a deterministic owner-facing graph, with Records as the accessible
  fallback; that projection adds no memory semantics or mutation authority.
- `/memory verify` is an eventless, read-only owner operation. It replays accepted
  operations into a temporary shadow projection and compares canonical
  per-scope hashes, rows, paths, and revisions with live state without modifying
  live semantic tables.
- Owner-only rebuild acquires a fenced maintenance lock, replays into shadow
  tables, verifies the complete canonical result, and atomically swaps only a
  valid projection into service. The model cannot invoke verify, rebuild, drop,
  or maintenance behavior.
- Verification failure quarantines only affected scopes. Their semantic reads
  and writes fail visibly while Episodic Memory, Kernel management, and unrelated
  scopes continue operating. Recovery never silently treats a divergent live
  projection as canonical.
- Startup performs inexpensive schema, foreign-key, Scope Revision, and
  operation-frontier checks rather than full replay. Detected inconsistency
  quarantines affected semantic state. Full replay runs in verification,
  rebuild, deterministic tests, or explicit scheduled maintenance.
- Replay fails closed at an unknown or malformed operation schema version,
  reports the exact operation and required compatibility, and never skips it or
  partially swaps a shadow projection.
- Semantic operations, graph rows, state transitions, Source Links, and the new
  Scope Revision commit in one SQLite transaction. Additive idempotent schema
  evolution is acceptable; destructive changes require an explicit migration
  design rather than startup SQL that rewrites accepted history.
- Generic database tools remain allowlisted away from every semantic table.
  Generic file tools continue rejecting the Evie database and WAL/SHM sidecars
  after symlink resolution. Typed scoped interfaces are the ordinary access
  path; the documented privileged shell remains outside this containment
  guarantee.
- Stage 3 evaluation uses a small versioned corpus of human-authored typed
  Semantic Operations and synthetic source evidence. Each fixture defines scope
  registry state, source events, operation stream, fixed IDs/times, expected
  canonical projection, expected exact queries/paths, and expected failures.
- Deterministic gates require exact operation outcomes, replay equality,
  idempotence, scope isolation, temporal boundaries, lifecycle transitions,
  provenance completeness, query equivalence, restart behavior, and recovery.
  These failures are release blockers and are never averaged into a quality
  score.
- Stage 3 records reproducible operation, query, traversal, replay, storage, WAL,
  and cold/warm-open baselines at fixed fixture sizes. It records environment and
  paired deltas but does not set arbitrary performance budgets before a baseline
  exists.
- Evaluation reporting keeps semantic conformance, learned extraction, retrieval
  and provenance, and answer/abstention quality separate. Stage 4 and Stage 5
  extend the same fixtures and report contract instead of redefining success.
- Before DDL is finalized, inspect a pinned Graphiti episode-to-entity-to-edge
  example and record the exact observed version, schema, and differences from
  Evie's accepted-operation, scope, temporal, and provenance model. This is
  design evidence, not authority to copy Graphiti's schema.
- Implementation is divided into independently reviewable outcomes even though
  this specification defines the complete Stage 3 contract: prerequisite scope
  integration; domain schema and atomic operations; lifecycle and temporal
  queries; scope and Promotion; replay and recovery; explicit mutation adapters;
  owner inspection surfaces; First-party Memory Plugin composition; and
  deterministic evaluation.

## Testing Decisions

- Good tests assert externally observable behavior through the Kernel-owned
  Semantic Memory interface. They do not assert private SQL helper calls,
  internal table traversal order, plugin adapter wiring, or a particular package
  layout. The interface is the primary test seam for both callers and fixtures.
- Use real temporary SQLite databases for schema, foreign-key, transaction,
  restart, WAL, concurrent-store, replay, shadow projection, and migration tests.
  Pure fakes are appropriate only for deterministic ID and clock control at the
  Semantic Memory interface.
- Table-driven operation tests cover every legal and illegal lifecycle
  transition for Entity, Alias, Claim, Source Link, and Graph Link; compound
  operation rollback; equivalent-Claim source attachment; correction modes;
  Promotion; retirement; restoration; and unsupported state.
- Identity tests cover random stable IDs, canonical anchors, ambiguous normalized
  Aliases, same-name Entities, explicit ID resolution, Context Entity registry
  references, and replay with recorded generated IDs.
- Predicate and value tests cover global definition versioning, invalid tokens,
  new-definition approval previews, object-kind validation, one/many cardinality,
  every Typed Literal canonical encoding, exact decimal equality, rejected floats
  and arbitrary JSON, and date-versus-datetime precision.
- Temporal tests cover half-open Valid Time at before/start/inside/end/after
  boundaries, open bounds, colliding Transaction Times ordered by Scope Revision,
  current queries, as-known-at queries, corrections by error and changed mode,
  retirement/restore cycles, and source retraction/restore cycles.
- Conflict tests prove opposite-polarity and overlapping single-cardinality
  warnings are deterministic diagnostics only; conflicting Claims remain
  queryable, source-linked, and unchanged unless an explicit operation acts.
- Scope matrix tests use global, two Workspaces, two projects, and multiple
  sessions. They cover every allowed and forbidden Entity reference, read,
  mutation, Graph Link, traversal, provenance expansion, and Promotion path at
  the storage query rather than relying on prompt instructions.
- Concurrency tests use multiple SQLite stores or processes. They race same-scope
  mutations, retries, stale approvals, source retraction against Promotion,
  destination changes against Promotion, and maintenance locking. Exactly one
  valid operation commits and stale work changes zero semantic rows.
- Idempotency tests repeat accepted and failed submissions across process reopen
  and prove the original result is returned without duplicate rows, Source Links,
  lifecycle events, or Scope Revision increments.
- Exact-read tests cover each closed query variant, effective temporal parameters,
  canonical ordering, snapshot-pinned pagination, stale cursors, Unsupported
  Claims, scope specificity labels, source redaction, and deterministic one- and
  two-hop path sets.
- Replay tests start from the same operation stream and compare canonical live,
  first replay, and repeated replay snapshots including IDs, values, lifecycle,
  provenance, operation references, times, and Scope Revisions. They assert zero
  model, Capability, network, and external-effect calls.
- Recovery tests cover interrupted operations, malformed projection rows,
  mismatched frontiers, unknown operation versions, per-scope quarantine,
  successful shadow swap, failed shadow verification, process restart, and
  preservation of Episodic Memory and unrelated scopes.
- Tool-adapter tests prove focused model-facing schemas prepare the same typed
  proposals as local callers, mutation tools use the existing approval gate,
  read tools remain read-only, Scope Context is harness-bound rather than a model
  argument, and disabling the plugin removes only model-visible capabilities.
- Composition tests prove new sessions receive only the Memory Capabilities
  selected by their Agent Preset, Composition Receipts pin their contract and
  schema identities, the standard preset degrades explicitly when its optional
  Memory Plugin is disabled, and session resume never silently substitutes an
  incompatible memory tool contract.
- CLI acceptance tests cover eventless listing, object inspection, explicit
  current/history selection, verification diagnostics, stale/quarantined scope
  messages, and absence of provider calls or Episodic Memory writes for read-only
  commands.
- HTTP and frontend acceptance tests cover explicit scope navigation, paginated
  Entity and Claim lists, current/history filters, detail, provenance, operation
  history, inaccessible source rendering, process restart, and read-only
  behavior.
- Egress tests capture model-facing tool results and prove remote-memory opt-in,
  size bounds, secret rejection, untrusted-data marking, scope redaction, and
  absence of opaque provider state or raw disallowed event payloads.
- Generic-tool regression tests enumerate every new semantic table and the
  database sidecar paths through existing SQL/file containment seams. The
  privileged shell exception remains documented rather than misrepresented as
  contained.
- Evaluation fixtures run in the ordinary deterministic test suite. Model-backed
  extraction and benchmark runs remain separately tagged later evaluations and
  cannot replace Stage 3 conformance gates.
- Performance tests report fixed-size operation commit, exact query, historical
  query, one-/two-hop traversal, replay, database growth, WAL growth, and
  cold/warm-open measurements with environment metadata. Initial results are
  baselines, not ungrounded pass thresholds.
- Existing real-SQLite event/reopen tests, session scope tests, immediate
  transaction tests, turn-lease races, compaction acceptance tests, generic
  memory-fence tests, Plugin Manager composition tests, tool approval tests, and
  web approval/stream tests are prior art for the new suites.
- Documentation-only specification work requires whitespace and link checks.
  Every implementation slice must pass the repository's full change-verification
  command, including Go tests and vet plus UI lint and build.

## Out of Scope

- Automatic episodic-event extraction, Memory Candidate generation, compiler
  jobs, extractor model selection, fuzzy Entity resolution, candidate review,
  or automatic admission. Those belong to Stage 4.
- FTS over events or semantic objects, derived-index generations, embeddings,
  vector indexes, hybrid fusion, relevance ranking, context-budget selection,
  source-bearing model context injection, or retrieval diagnostics. Those belong
  to Stage 5.
- In-process adjacency snapshots, packed graph representations, or cache
  invalidation optimizations. Those belong to Stage 6 and must preserve the Stage
  3 exact traversal contract.
- Procedural Memory, Git-backed instructions, Skills, Workflow Definitions, or
  Workflow Run execution.
- Hard Erasure or a `forget` operation. Stage 3 implements reversible Retirement
  and source eligibility without deleting accepted history or Episodic Memory.
- Imported files, fetched pages, research artifacts, non-conversational source
  ingestion, source-format policies, or artifact retention beyond existing
  Episodic Memory evidence.
- Synonym inference, learned Predicate normalization, model-derived Entity
  summaries, model confidence on accepted Claims, or automatic contradiction
  interpretation.
- Money, duration, quantity, unit conversion, floating-point Typed Literals,
  arbitrary JSON values, or implicit value coercion.
- A general Memory Provider interface, third-party memory backends, a separate
  graph database, remote graph services, cloud sync, multi-owner authorization,
  or role-based access.
- Model-facing verify, rebuild, drop, migration, quarantine, or maintenance
  capabilities.
- Unbounded or mutable graph visualization, Three.js, whole-graph force
  layouts, or mutable Markdown memory editing. The UI Data hub's bounded,
  deterministic projection of this exact inspection contract is permitted; a
  richer Memory Explorer still requires a separate prototype-backed
  specification.
- Automatic narrower-scope override, automatic conflict winner selection, or
  silent cross-scope merge.
- Absolute latency or storage budgets chosen before representative Stage 3
  baselines exist.

## Further Notes

Episodic Memory and Working Memory are already implemented. Stage 3 begins only
after the first Workspace slice provides stable Workspace identity and explicit
Workspace session scope. It uses that canonical scope from the first semantic
DDL rather than shipping a global/project/session schema that immediately needs
revision.

This specification supersedes the older Stage 3 outline in the umbrella memory
roadmap wherever that outline assigns Memory Candidates, FTS, derived-index
generations, extractor metadata, or model-derived summaries to Stage 3. The
umbrella roadmap remains authoritative for the four-layer vision and later-stage
sequence; this specification is authoritative for the Stage 3 implementation
contract.

The existing Workspace-memory ticket contains both deterministic scope behavior
and later retrieval behavior. Its exact Workspace isolation, default-write, and
Promotion requirements are incorporated here. Its FTS, vector, context
injection, and compiler-worker criteria remain blocked on their corresponding
later stages and should be reconciled during ticket generation rather than
implemented prematurely.

The Memory surface uses progressive disclosure across graph, records, and the
shared Inspector, but Evie displays scoped SQLite Entities, Claims, provenance,
Valid Time, Transaction Time, and Semantic Operations instead of mutable
Markdown files. Its bounded deterministic graph is only a projection of this
contract, so layout and interaction cannot distort the semantic schema.

Evaluation follows a layered model: deterministic accepted-state conformance in
Stage 3; learned extraction and Entity-resolution quality in Stage 4; retrieval
and provenance quality in Stage 5; and pinned external benchmarks plus redacted
end-to-end Evie replays in the later evaluation stage. Reports never collapse
those layers into one memory score.

After this specification is approved, ticket generation should split it into
independently reviewable outcomes and preserve the Workspace prerequisite. No
single implementation ticket should combine the full schema, every mutation,
recovery, plugin composition, CLI, web UI, and evaluation harness.
