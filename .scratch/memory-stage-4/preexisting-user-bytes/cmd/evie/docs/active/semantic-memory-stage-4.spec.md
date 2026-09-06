## Problem Statement

Evie preserves conversations as Episodic Memory and supports explicit, approved
Semantic Memory operations. Its owner must still identify useful information,
ask for each memory change, and inspect the resulting proposal. Useful
preferences, relationships, project decisions, and changes in circumstances
remain buried in conversations unless the owner performs that work manually.

Automatic extraction introduces a different problem: a system can reliably
produce well-formed suggestions while misidentifying people, treating a
hypothetical as a fact, citing text that does not support its interpretation, or
creating an inbox the owner cannot maintain. Processing years of history can
also compete with ordinary conversation for memory, inference capacity, and
database access. Restart safety alone does not establish useful memory.

The owner needs useful, sourced Memory Candidates produced in the background,
with explicit control over historical processing, inspectable uncertainty, and
review that remains available after the original conversation closes. Accepted
Semantic Memory must retain its existing scope, authority, temporal, approval,
and replay guarantees. Extraction quality, review effort, and resource costs
must be measurable before ongoing compilation is considered successful.

## Solution

Add a Kernel-owned asynchronous memory compiler and durable owner review of
Memory Candidates. The compiler processes selected Episodic Memory through a
local extractor, validates the returned structure and exact source references,
and proposes useful interpretations without changing accepted knowledge.

The owner activates a pinned Compiler Generation for new evidence and
separately selects bounded historical backfill. Independent jobs can finish out
of order; Compilation Coverage shows exactly what was processed and which gaps
remain. A failed older job does not prevent later independent candidates from
becoming reviewable. Foreground turns never wait for model extraction.

A scope-level inbox presents each proposed change with its evidence, original
authority, identity choices, scope, and temporal effects. The owner can accept,
edit, or reject candidates after their source conversations close. Acceptance
uses the Kernel's exact reviewed-effect, revision, and atomic Semantic Operation
guarantees. Candidate production and review do not widen memory scope or make
model output authoritative.

Evaluate this stage through one primary Kernel-owned compilation and review
seam, using real persistence and scripted extraction for deterministic checks.
Run local-model quality and resource experiments separately. The stage succeeds
when it produces supported useful candidates, survives interruption and
restart, preserves accepted-state guarantees, and meets measured foreground and
review budgets for the declared local workload.

## User Stories

1. As Evie's owner, I want useful information proposed from my conversations automatically, so that remembering it does not require a separate command for every fact.
2. As Evie's owner, I want enduring preferences identified, so that repeated conversations can build on what matters to me.
3. As Evie's owner, I want people and relationships represented carefully, so that useful personal context is retained without conflating identities.
4. As Evie's owner, I want Workspace and project decisions proposed with their context, so that durable constraints remain available in the appropriate area.
5. As Evie's owner, I want meaningful changes distinguished from incidental remarks, so that proposed updates reflect what I actually communicated.
6. As Evie's owner, I want conversations that contain no useful memory to produce no candidate, so that my review inbox remains relevant.
7. As Evie's owner, I want original episodes retained when extraction produces nothing, so that an omission does not destroy evidence.
8. As Evie's owner, I want changing Task and other domain records to retain their authoritative stores, so that memory does not create competing versions of operational state.
9. As Evie's owner, I want every candidate linked to exact source evidence, so that I can inspect what supports it.
10. As Evie's owner, I want copied evidence distinguished from a correct interpretation, so that a matching quote does not conceal an unsupported claim.
11. As Evie's owner, I want my assertions distinguished from quoted or hypothetical material, so that text I discuss is not automatically attributed to me as fact.
12. As Evie's owner, I want tool observations admitted only through defined evidence contracts, so that arbitrary tool output does not acquire factual authority.
13. As Evie's owner, I want assistant messages used only within the approved interpretation policy, so that Evie's guesses do not independently corroborate personal facts.
14. As Evie's owner, I want reported speech to preserve its attribution, so that someone else's statement is not relabeled as my assertion.
15. As Evie's owner, I want detected secrets excluded from compilation input and promotion, so that supplemental memory does not reproduce protected material.
16. As Evie's owner, I want embedded instructions treated as source data, so that remembered text cannot change permissions or approval behavior.
17. As Evie's owner, I want eligible committed evidence retained after failed or cancelled turns, so that provider failure does not erase useful input.
18. As Evie's owner, I want unfinished tool intent distinguished from a completed observation, so that Evie never remembers an unproven outcome.
19. As Evie's owner, I want reasoning, continuation state, compaction summaries, and retrieved-memory blocks excluded as supporting evidence, so that generated context cannot become self-confirming memory.
20. As Evie's owner, I want compilation scope determined by the harness and source context, so that model output cannot select a broader destination.
21. As Evie's owner, I want Workspace, project, session, and global knowledge to retain their existing isolation rules, so that background work cannot leak between areas.
22. As Evie's owner, I want broader reuse to require explicit Promotion, so that candidate review does not silently import narrower evidence into global memory.
23. As Evie's owner, I want unaccepted candidates excluded from normal Semantic Memory queries and traversal, so that suggestions are never presented as accepted knowledge.
24. As Evie's owner, I want same-name people shown as possible distinct identities, so that a familiar name alone does not cause a merge.
25. As Evie's owner, I want known Entities and Aliases reused where supported, so that recurring references do not unnecessarily fragment memory.
26. As Evie's owner, I want unresolved identity alternatives visible during review, so that I can make the intended choice explicitly.
27. As Evie's owner, I want extraction to start from a reviewed Predicate vocabulary, so that relationship meanings remain understandable and consistent.
28. As Evie's owner, I want proposed new Predicate definitions shown as effects requiring review, so that wording variations cannot silently redefine the graph.
29. As Evie's owner, I want unknown dates to remain unknown, so that extraction does not invent temporal precision.
30. As Evie's owner, I want future possibilities distinguished from completed changes, so that a plan does not become a statement that an event happened.
31. As Evie's owner, I want negation and Typed Literal meaning preserved, so that superficially similar sentences do not produce opposite facts.
32. As Evie's owner, I want corrections to preserve the distinction between an earlier error and a real-world change, so that accepted history remains accurate.
33. As Evie's owner, I want conflicting interpretations and evidence visible, so that the extractor does not silently select a winner.
34. As Evie's owner, I want additional support distinguished from a duplicate proposition, so that corroboration can reuse accepted knowledge appropriately.
35. As Evie's owner, I want model confidence kept separate from acceptance authority, so that an uncertain prediction cannot approve itself.
36. As Evie's owner, I want private-history extraction to stay on my machine, so that this stage does not introduce remote processing of that history.
37. As Evie's owner, I want explicit memory to remain available without a configured extractor, so that optional inference does not become a dependency of accepted knowledge.
38. As Evie's owner, I want an unavailable local endpoint reported clearly, so that compilation does not silently fall back to a remote provider.
39. As Evie's owner, I want conversational turns to finish without awaiting extraction, so that background memory work does not become a model-call dependency of every response.
40. As Evie's owner, I want foreground overhead measured under realistic load, so that background operation has a verifiable responsiveness budget.
41. As Evie's owner, I want inference capacity bounded across cooperating Evie processes, so that multiple sessions do not overload the local model.
42. As Evie's owner, I want input, output, queued work, and persistence batches bounded, so that a long conversation cannot create unbounded resource use.
43. As Evie's owner, I want new evidence prioritized over historical backfill, so that old history does not indefinitely delay recent suggestions.
44. As Evie's owner, I want cancellation and shutdown to prevent late accepted effects, so that interrupted workers cannot continue committing changes.
45. As Evie's owner, I want expired or replaced workers fenced out, so that stale execution cannot overwrite current durable work.
46. As Evie's owner, I want compilation to recover after restart, so that pending work does not depend on an in-memory queue.
47. As Evie's owner, I want repeated scheduling and delivery to be idempotent, so that retries do not duplicate completed candidate groups or accepted operations.
48. As Evie's owner, I want independent later candidates available when an earlier job fails, so that one problematic episode does not freeze a whole scope's inbox.
49. As Evie's owner, I want unresolved coverage gaps visible, so that progress never implies missing history was processed.
50. As Evie's owner, I want successful empty extraction distinguished from failure, so that no-memory results do not hide broken work.
51. As Evie's owner, I want failures, retries, cancellations, and any explicit skips distinguishable, so that I can understand the state of historical processing.
52. As Evie's owner, I want activation to record where new-evidence processing begins, so that compiler coverage has an explicit starting point.
53. As Evie's owner, I want historical backfill selected by bounded scope and range, so that I control its compute and review cost.
54. As Evie's owner, I want unselected history reported as outside selection, so that it is not confused with completed compilation.
55. As Evie's owner, I want each extraction configuration pinned as a Compiler Generation, so that I can understand which configuration produced a suggestion.
56. As Evie's owner, I want model and policy changes to produce distinct candidate generations, so that changing the extractor never rewrites accepted memory.
57. As Evie's owner, I want previous review decisions preserved when equivalent suggestions recur, so that upgrades do not erase my earlier choices.
58. As Evie's owner, I want a scope-level candidate inbox, so that review is organized by the area the proposed memory belongs to.
59. As Evie's owner, I want candidates reviewable after their original conversations close, so that useful suggestions are not stranded in inactive sessions.
60. As Evie's owner, I want exact source, scope, identity, and temporal effects shown before acceptance, so that approval applies to an understandable change.
61. As Evie's owner, I want to accept a candidate explicitly, so that only reviewed effects enter Semantic Memory.
62. As Evie's owner, I want to edit a candidate before accepting it, so that a useful interpretation can be corrected without losing its origin.
63. As Evie's owner, I want to reject a candidate while preserving the review decision, so that unwanted suggestions remain distinguishable from unreviewed work.
64. As Evie's owner, I want bounded batches to expose their exact effects and dependencies, so that efficient review does not conceal compound changes.
65. As Evie's owner, I want stale previews rejected, so that approval cannot silently apply a different effect against newer graph state.
66. As Evie's owner, I want approval authority kept separate from source authority, so that accepting a tool observation does not make it an owner assertion.
67. As Evie's owner, I want accepted changes to remain atomic and replayable, so that review preserves the deterministic foundation already established.
68. As Evie's owner, I want CLI and web review to share the same rules, so that changing surfaces cannot change scope, evidence, or approval semantics.
69. As Evie's owner, I want safe worker and inbox diagnostics, so that I can see progress and failures without exposing raw protected data.
70. As Evie's owner, I want generic SQL and file tools to remain fenced from memory-owned storage, so that new candidate records do not create a bypass.
71. As Evie's owner, I want supported useful precision and recall measured separately, so that neither speculative output nor universal abstention appears successful.
72. As Evie's owner, I want identity and temporal errors reported separately, so that an overall score does not hide damaging mistakes.
73. As Evie's owner, I want review time and inbox age measured, so that extraction throughput does not hide unsustainable human effort.
74. As Evie's owner, I want performance measured across growing history and graph sizes, so that years of usage have an explicit scaling baseline.
75. As Evie's owner, I want the local extractor chosen from measured fixtures, so that advertised model capabilities do not substitute for evidence on my workload.
76. As Evie's maintainer, I want deterministic failure and restart tests independent of a live model, so that regressions are reproducible.
77. As Evie's maintainer, I want frozen evidence, complete-history holdouts, and repeated model runs, so that prompt tuning does not contaminate the reported result.
78. As Evie's maintainer, I want versioned reports with counts, failures, and environment identities, so that different configurations can be compared honestly.
79. As Evie's maintainer, I want model quality and resource results separated from exact conformance, so that a quality gain cannot excuse a scope or persistence violation.
80. As Evie's maintainer, I want explicit prerequisite contracts and independently reviewable implementation outcomes, so that unresolved behavior is not invented inside a large implementation change.

## Implementation Decisions

### Stage boundary and authority

- Stage 4 produces and reviews Memory Candidates. Automatic admission remains
  disabled throughout this stage. Accepted Entities, Claims, Source Links,
  lifecycle transitions, and Semantic Operations retain Stage 3 semantics.
- The Kernel owns compilation supervision, durable coverage, candidate review,
  scope and source validation, acceptance authority, and recovery. The Memory
  Plugin remains an adapter for its existing model-visible capabilities; it
  does not become the owner of memory truth or worker authority.
- The primary external seam extends the existing Kernel-owned Memory
  prepare/apply/read approach with selected compilation, coverage and candidate
  inspection, and exact owner review. It represents one cohesive behavioral
  contract; it does not require one large language-level interface or expose
  every storage and worker helper to callers.
- Reuse the existing semantic inspector, approved-operation machinery, and
  replay verifier for accepted-state behavior. Candidate review after source
  session closure requires a new typed authority contract; it cannot bypass
  current validation or revive the original conversation merely to reuse a
  session-bound method.
- The extractor is a bounded input/output dependency behind the compiler seam.
  Real local inference and scripted test extraction supply that variation. The
  extractor cannot grant scope, approve effects, directly mutate accepted graph
  state, or calculate authoritative evidence hashes on behalf of the Kernel.
- One owner and one machine remain the deployment scope. SQLite remains the
  persistent store, with multiple cooperating Evie processes supported.

### Useful evidence and interpretation

- Initial selection targets enduring preferences, people and relationships,
  Workspace/project decisions and constraints, and meaningful changes. Retain
  original episodes when no candidate is produced. Operational Task and other
  domain records retain their existing authorities unless a specific memory
  use case and freshness policy are established.
- Initial supporting evidence consists of direct owner assertions and
  individually contracted tool observations. Assistant text may supply bounded
  interpretation context but cannot independently corroborate personal facts.
  Role labels alone cannot classify quotations, hypotheticals, or reported
  speech as owner assertions; the extraction rubric must distinguish them.
- Eligible committed evidence remains usable after failed or cancelled turns.
  An unfinished tool intent does not establish success or an observation. No
  terminal outcome may be synthesized after lease loss merely to advance
  compilation.
- Reasoning, opaque provider continuation state, context snapshots, compaction
  summaries, retrieved-memory blocks, compiler output, and diagnostics cannot
  become supporting evidence. Detected secrets are excluded from compiler input
  and semantic promotion. This does not claim the raw episode store contains
  no secrets or that detection is perfect.
- Source references preserve event identity, permitted field or range, exact
  evidence, content hash, scope, and original authority. Validation and review
  rendering must agree about the referenced source. Evidence presence and
  semantic entailment are separate: exact text matching does not establish that
  a proposed proposition follows from that text.
- Extraction and review preserve existing global, Workspace, project, and
  session reference rules. Destination scope is harness-bound. Broader reuse
  requires explicit Promotion; owner review does not create implicit imports
  or cross-scope source visibility.
- Preserve the distinction between newly covered evidence and bounded context
  used to interpret it. Exact source-window, overlap, oversized-input, and
  multi-source attribution contracts are prerequisite design outputs; unrelated
  sessions cannot be treated as one conversation by assuming shared scope is
  sufficient context.

### Candidate meaning and accepted effects

- Candidates are durable unaccepted interpretations with source references,
  generation identity, resolution context, uncertainty, and review state. They
  remain outside ordinary accepted Semantic Memory queries, traversal, and
  retrieval until explicit acceptance emits an appropriate Semantic Operation.
- Begin resolution with exact identities and lexical alternatives. Preserve
  same-name ambiguity instead of inferring a merge from text similarity. Dense
  resolution, embeddings, and learned graph models remain later work.
- Start from a small useful reviewed Predicate vocabulary. New definitions
  appear in the exact proposal and cannot silently redefine an existing global
  Predicate's meaning, allowed values, or cardinality.
- Preserve canonical Typed Literals, Claim Polarity, Valid Time, Transaction
  Time, and correction semantics. Unknown temporal bounds stay unknown;
  possible future changes do not become completed changes. Contradictions
  remain visible without an automatic winner or cascading invalidation.
- An accepted effect may create supported knowledge or reuse existing knowledge
  according to established exact equality and Source Link semantics. Neither
  every candidate nor every new source necessarily requires a new Claim.
- Model confidence and extractor configuration belong to candidates and audit
  context, not the truth of accepted Claims. An extractor update cannot change
  accepted semantics or require a model call during replay.

### Generations, coverage, and scheduling

- Activation captures an explicit frontier for new evidence under one pinned
  Compiler Generation. Historical backfill is selected separately by bounded
  scope and range, including eligible retained history predating this stage.
- Model, prompt, or evidence-policy changes create distinct generations and
  candidate groups. Preserve accepted memory and prior review decisions when
  equivalent suggestions recur. The material configuration identity and exact
  equivalence/re-presentation contract must be frozen before their dependent
  persistence implementation.
- Reconciliation recovers uncovered durable evidence inside selected ranges.
  History outside selection is reported as such, never counted as processed.
  With no extractor configured, events continue to commit without creating
  permanently pending extraction work.
- Selected terminal events and their idempotent scheduling records commit
  atomically. Durable reconciliation also needs an explicit eligibility and
  closure matrix for failed, interrupted, crashed, and command-only sequences;
  successful conversation completion is not the sole definition of usable
  evidence.
- SQLite owns job identity, selected source ranges, attempts, lease ownership,
  cancellation, generation association, staged results when used, completion,
  and Compilation Coverage. Channels may wake workers but never own the only
  durable record of work.
- Independent jobs may persist unaccepted candidates out of order. Record exact
  completed ranges and visible unresolved gaps. A contiguous frontier cannot
  cross an unresolved gap, and missing output cannot be counted as successful
  empty extraction. Later extraction cannot depend on earlier unaccepted
  candidates.
- Accepted Semantic Operations remain serialized under current revision and
  atomicity checks. Candidate job completion is not an accepted graph mutation
  and does not acquire authority from scheduling order.
- Lease fencing prevents a stale worker from committing after replacement or
  cancellation. Retry delivery must be idempotent. The existing retry ceiling
  is at most five attempts, with exponential backoff from five seconds to a
  ten-minute cap; classification, repair-attempt accounting, skip progression,
  and exact recovery transitions must be frozen in the job contract.
- Initial local inference capacity is one request at a time across cooperating
  Evie processes. Bound input, output, queued and staged work, and database
  batches. New evidence has priority; historical backfill uses remaining
  capacity. Exact fairness, lifecycle hosting, and retention policies remain
  prerequisite contract outputs.
- Foreground turns never await extraction. Shared resource and transaction
  overhead must meet an agreed measured budget; asynchronous execution alone
  does not establish zero interference.

### Owner review and local surfaces

- Provide a scope-level inbox with accept, edit, and reject. Review remains
  available after the source conversation closes without reviving it. Candidate
  access and evidence rendering continue to enforce the applicable scope and
  source policies.
- Present exact sources, original authority, scope, identity alternatives,
  Predicate additions, temporal effects, and dependent changes. Approval binds
  the exact reviewed effect, not a natural-language intention or an instruction
  to accept whatever a later extractor returns.
- Acceptance revalidates current source eligibility, target identity, scope,
  conflicts, and revisions. Stale previews cannot silently apply or be rebased
  into a different effect. The exact refresh and reapproval interaction is part
  of the review contract.
- Preserve original extraction and review origin when an owner edits a
  candidate. Approval authority remains separate from evidence authority; it
  does not convert a tool observation into an owner assertion.
- Support bounded approval batches whose exact effects and compound
  dependencies are visible. Atomic grouping, independent-item failure behavior,
  edit lineage, and resolution/audit transactions must be defined before the
  corresponding storage and UI implementation.
- CLI and web are adapters over the same Kernel contract. They must not
  implement separate authority, source, lifecycle, or temporal rules. Exact
  commands, routes, and interaction details are determined in their reviewable
  implementation stories after the shared review contract is frozen.
- Worker and review diagnostics expose progress, gaps, failures, generation
  identity, and inbox age using bounded safe projections. Generic SQL and file
  tools remain fenced from all memory-owned storage. Preserve the existing
  documented privileged-shell limitation rather than claiming new containment.

### Local inference and evaluation

- Private-history extraction uses an explicitly configured loopback or
  Unix-socket endpoint; redirects, nonlocal endpoints, and silent remote
  fallback are rejected. The conversational provider remains unchanged.
- The standalone local-model spike selects a runtime, model configuration,
  structured output protocol, and extraction schema from observed extraction
  quality and resource behavior. It cannot establish actual compiler foreground
  overhead or owner-review usability. No model, runtime, quality score, or
  resource result has been established by this spec.
- The spike demonstrates schema handling, malformed/truncated output, bounded
  input/output, timeouts, cancellation, unavailable endpoints, and repeated-run
  behavior. Separate client return, prevention of late durable effects, and
  eventual release of model-server capacity.
- A synthetic-only remote comparison remains optional. It is not permission
  to send personal history remotely. Any future remote private-history
  extraction requires its own explicit policy; retrieval egress opt-in does not
  provide that authority.
- Select configurations using supported useful precision and independently
  measured recall, plus entity, temporal, and source-attribution errors.
  Precision is favored for suggested changes to existing knowledge, while
  universal abstention cannot qualify as success.
- Measure active review time per useful accepted change, accept/edit/reject
  outcomes, and inbox age separately from compiler throughput. Approval rate
  alone is neither semantic accuracy nor usefulness.
- Measure candidate freshness, queue and inference latency, throughput,
  foreground overhead, model/host memory, and database/WAL growth across
  increasing event sizes, counts, accepted graph sizes, and scope distributions.
- After the compiler and review flow exist, an integrated pilot measures actual
  foreground interference, review burden, and backlog behavior. Freeze numerical
  release gates from that pilot before final-holdout evaluation and ongoing
  enablement. Standalone spike measurements do not substitute for this pilot.
- Preserve the separate accepted-state, learned-extraction, retrieval, and
  answer panels of the existing evaluation design. Add versioned Stage 4
  metrics and failure categories without changing Stage 3 conformance gates.
  Retrieval and production answer improvements remain later-stage claims.

## Testing Decisions

- Tests exercise externally observable behavior through the Kernel-owned
  compilation and review seam. Start with committed Episodic Memory, activate
  selected compilation, observe candidates and coverage, review an exact effect,
  and inspect/replay accepted state through existing semantic reads. Avoid
  coupling assertions to private SQL helpers, goroutine counts, or a particular
  worker implementation.
- Use real temporary SQLite databases and scripted extractor responses for
  deterministic tests. Control time and failures at existing seams where
  practical. A small local HTTP fixture verifies transport behavior; live model
  inference belongs to the separate spike and quality suite.
- One acceptance path must block a scripted extractor while a foreground turn
  finishes, restart the runtime, recover the selected work, review its candidate
  after the original session closes, and verify the accepted result and replay.
- Evidence fixtures cover owner assertions, quotation, reported speech,
  hypotheticals, negation, assistant echoes, named tool-field contracts,
  failed/cancelled turns, and incomplete tool intent. Test exact Unicode/range
  and field references, hashes, source visibility, detected secrets, and
  hostile instructions independently from semantic entailment grading.
- Selection and interpretation fixtures distinguish required useful memories,
  permitted optional suggestions, unsupported proposals, and unwanted but true
  information. Include no-memory outputs, same-name people, aliases, unknown
  dates, future possibilities, changed/error correction, duplicate propositions,
  additional sources, and uncertain new Predicate mappings.
- Scope tests extend the existing global, multiple Workspace, multiple project,
  and multiple session matrices to source projection, candidate listing,
  review, acceptance, and provenance expansion. Attempted implicit Promotion
  changes no accepted state.
- Coverage tests show later independent candidates becoming reviewable while an
  earlier failed range remains a gap. Verify frontier behavior, zero-candidate
  success, outside-selection history, explicit backfill, duplicate scheduling,
  generation changes, and recovery with an unavailable or disabled extractor.
- Concurrency and crash tests use multiple stores and processes. Exercise
  activation/reconciliation races, leases, expiry, stale completion, shutdown,
  retries, cancellation, concurrent approvals, and crash points around durable
  commits. Reopen must preserve all committed outcomes without duplicate
  operations, lost completed coverage, or partial accepted effects.
- Review tests cover inactive source sessions, exact preview binding, edits,
  rejection, defined batch behavior, stale revisions, changed source eligibility,
  original evidence authority, and preserved review decisions across equivalent
  suggestions. Expected outcomes must follow the frozen review contract.
- Accepted-state tests preserve existing canonical operation replay, idempotence,
  temporal/lifecycle semantics, quarantine/recovery behavior, and zero model or
  external-effect calls during replay. Candidates never appear in ordinary
  accepted queries before acceptance.
- CLI, HTTP, and frontend tests verify the same scope, source, preview, and
  acceptance results through their adapters. Retain existing approval/origin
  protections and generic memory-storage containment checks.
- Capacity tests observe at most one active local extraction request across
  cooperating processes. Validate declared bounds and show that foreground
  completion does not await a stalled extractor. Test runtime shutdown and
  local transport restrictions, including redirect rejection and no fallback.
- Model quality uses human-reviewed gold labels, explicit equivalence and
  ambiguity where appropriate, frozen evidence and selection rubrics,
  complete-history development/holdout separation, and repeated runs. Keep
  narrative variants together and keep future questions, answers, and evaluator
  metadata out of compiler inputs.
- Reports identify the exact model artifact/configuration, server, schema,
  prompt, generation, corpus, environment, repetition count, and baseline.
  Report counts, denominators, failure slices, uncertainty where meaningful,
  and paired deltas. Grade raw proposals and reviewable candidates separately
  so validation losses and semantic failures remain visible.
- Performance experiments compare compilation disabled, normal processing,
  and historical catch-up under a fixed foreground workload. Vary history
  size, source length, graph size, scope distribution, and available runtime
  capacity separately. Measure model-server resources as well as Evie's own.
- Exact scope, authority, source-binding, persistence, and replay failures are
  release blockers. Learned-quality and resource thresholds are fixed from
  pilot evidence before finalist holdout evaluation; they cannot be inferred
  from the absence of a deterministic failure or changed to hide a poor result.
- Prior art includes the existing real-SQLite event/reopen tests, turn-lease
  process races, scoped semantic conformance corpus, stale proposal and atomic
  operation tests, cross-surface Stage 3 acceptance path, memory tool fences,
  HTTP approval tests, and frontend request/snapshot handling tests.
- Documentation-only specification work requires whitespace and local-link
  checks. Each implementation slice must pass the repository's required full
  change verification, including Go tests/vet and UI lint/build, in addition to
  focused tests appropriate to that slice.

## Out of Scope

- Automatic acceptance, confidence-based acceptance, or a general automatic
  admission policy. High extraction scores do not authorize these behaviors.
- FTS generations, embeddings, vector indexes, hybrid retrieval, relevance
  ranking, context-budget selection, and automatic memory injection into
  conversational requests; these remain Stage 5.
- In-process graph acceleration, graph neural networks, learned graph
  completion, automatic Entity merging, and automatic contradiction cascades.
- Remote private-history extraction, cloud sync, multi-host coordination,
  multi-owner accounts, or replacing the conversational provider.
- Procedural Memory, Workflow Definitions, Workflow Run execution, or a
  generalized workflow engine for the compiler.
- Non-conversational file/web/research ingestion, arbitrary tool-result access,
  and independent assistant assertions as support for personal facts.
- Hard Erasure, deletion of accepted semantic history, and unspecified purging
  of Episodic Memory or old generations. Retention policy is a prerequisite to
  any cleanup implementation, not implicit permission to erase audit evidence.
- Changing Stage 3 canonical identities, Predicate semantics, Typed Literal
  equality, scope rules, explicit Promotion, or model-independent replay.
- A separate graph database, new production dependencies without the required
  approval, speculative provider frameworks, and unrelated UI redesign.
- Full external benchmark adapters and production answer-quality claims in
  this stage. Any small offline oracle-reader probe is diagnostic only.
- Promised model quality, throughput, or latency numbers before the spike and
  pilot measurements exist.

## Further Notes

This specification synthesizes the Stage 4 recommendations accepted on
2026-09-04. The binding decisions retain the memory Kernel, model-independent
accepted operations, explicit semantic approval, and separate evaluation
layers. ADRs 0062–0065 establish the narrowed evidence policy, independence of
candidate progress from contiguous coverage, owner review after source-session
closure, and separate activation/backfill choices. The older blocked-head and
automatic all-history defaults have been superseded for compilation; later
index-generation requirements are unchanged.

The prerequisite deterministic Semantic Memory stage was merged in
[PR #115](https://github.com/davidadel66/evie/pull/115), with its parent contract
in [issue #102](https://github.com/davidadel66/evie/issues/102). This document
defines the next stage's outcome and required design/experimental deliverables;
it does not claim that the compiler or model spike is already implemented.

Production implementation proceeds through independently reviewable outcomes.
The following prerequisite records must be completed before their dependent
stories are ready; this synthesis does not silently approve their earlier
provisional recommendations:

| Prerequisite deliverable | Required contents | Dependent work |
| --- | --- | --- |
| Evidence and closure contract | Supported fact classes and tool observations; exact field/range projection; source versus interpretation context; bounded windows and overlap; eligibility/closure for successful, failed, interrupted, crashed, and command-only histories | Source projection, scheduling integration, source inspection, extractor schema |
| Durable work and coverage contract | Source-unit and generation identity; cross-session ordering; activation/reconciliation races; idempotent staging; retry/repair accounting; cancellation/skip semantics; capacity and lifecycle hosting; retention and equivalent-suggestion behavior | Candidate/job storage, worker supervision, backfill and diagnostics |
| Owner review contract | Authority after source-session closure; exact preview and revision binding; edit lineage; batch dependencies and failure semantics; source authority versus approval audit; atomic resolution and accepted effects | Acceptance transactions, CLI and web review |
| Standalone local-model and evaluation spike | Pinned runtime/model/schema; malformed-output and cancellation behavior; reviewed selection/annotation rubric; repeated local runs; source/identity/temporal quality and standalone extraction resources | Extractor selection and implementation; fixture/report contracts |
| Integrated pilot and release gates | Actual compiler foreground overhead, candidate freshness, review burden, backlog behavior, and resource use; numerical release gates fixed before final-holdout evaluation | Ongoing compilation enablement and final acceptance, after the compiler and review flow exist |

The intended sequence is to define these contracts and fixtures, run the bounded
local spike, implement durable coverage and candidate production, extend exact
owner acceptance, add review adapters, run the integrated pilot, and complete
cross-surface and workload acceptance against its frozen release gates.
Decomposition may overlap independent work, but no single change
should combine every persistence, authorization, concurrency, and UI boundary.
Create implementation tickets separately; publishing this parent specification
does not create or start them.

Roughly minute-scale freshness, a small daily review session,
10,000/100,000/1,000,000-event stress levels, and an initial corpus of about
32 targeted synthetic windows plus 10–20 reviewed histories are pilot
hypotheses. They are not capacity promises, statistical certification, or frozen
acceptance thresholds. Runtime/model selection and the remaining behavioral
contracts must be recorded before their dependent implementation, and numerical
gates must be fixed before tuning against the final holdout.

The main risks are unsupported interpretation despite valid evidence locations,
identity mistakes, stale review effects, conflating coverage with accepted
knowledge, resource contention, and an inbox that grows faster than the owner
can review it. The deterministic tests, separate quality reports, and workload
experiments above make those risks observable without weakening explicit
acceptance or discarding original evidence.
