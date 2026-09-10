## Problem Statement

Evie retains conversations and accepted Semantic Memory, but ordinary answers
do not yet reliably receive the relevant evidence. David must repeat personal
preferences, reconstruct earlier decisions, or remember to ask for a lookup.
Useful information can remain in a conversation even when extraction never
produced an accepted Claim. Returning to a person after an unrelated debugging
discussion should not make that person impossible to identify.

Loading more history indiscriminately introduces noise and cost. Searching only
accepted Claims misses original wording; searching conversations without their
authority, scope, lifecycle, and time can revive retired information or present
an old possibility as a current fact. David needs useful continuity with clear
evidence, bounded investigation, and an honest distinction between missing
information and a failed search.

## Solution

Add Stage 5 relevance retrieval through a shared Kernel-owned read boundary.
Automatic Recall begins on each new user message and selects a small amount of
relevant accepted memory and attributed Conversation Excerpts. The model can
request further memory searches, conversation searches, and bounded neighboring
messages when the initial evidence is insufficient. It judges whether more
investigation helps; code enforces access, eligibility, and resource limits.

Recall uses the conversation's meaning, including relevant earlier discussion
and compaction continuity, rather than just its latest message. Evie resolves
references from available evidence and asks a focused clarification when
remaining ambiguity would materially change the answer. Search results never
accept, promote, or correct stored memory.

A compact memory activity card distinguishes Accepted memory from Conversation
excerpt and provides inspectable sources. Historical answers preserve
attribution, uncertainty, and meaningful conflicts. Retained evidence references
explain what was supplied for the original answer, even after memory changes.

## User Stories

1. As Evie's owner, I want relevant preferences recalled during ordinary requests, so that I do not have to ask Evie to remember them each time.
2. As Evie's owner, I want new conversations to benefit from eligible saved knowledge, so that useful continuity survives a fresh chat.
3. As Evie's owner, I want relevant original conversation excerpts recalled automatically, so that extraction omissions do not make useful information invisible.
4. As Evie's owner, I want Evie to understand references using earlier discussion, so that returning to a person after debugging still makes sense.
5. As Evie's owner, I want compaction continuity considered during interpretation, so that context reduction does not arbitrarily reset the subject.
6. As Evie's owner, I want Evie to investigate a reference before asking me, so that recoverable context does not create unnecessary questions.
7. As Evie's owner, I want a focused clarification when plausible identities would change the answer, so that Evie does not confidently guess whom I mean.
8. As Evie's owner, I want exact names, aliases, identifiers, meaning, dates, and relationships to help find evidence, so that retrieval works beyond matching literal words.
9. As Evie's owner, I want only a focused selection delivered to the answering model, so that unrelated memories do not crowd out my request.
10. As Evie's owner, I want meaningful uncertain evidence labeled, so that useful clues are neither hidden nor presented as settled facts.
11. As Evie's owner, I want Evie to search further when evidence is insufficient, so that difficult questions can receive a supported answer.
12. As Evie's owner, I want read-only searches to proceed without approval for every invocation, so that investigation feels like a native part of conversation.
13. As Evie's owner, I want the model to choose its next useful search, so that investigation is not constrained to a rigid semantic-first sequence.
14. As Evie's owner, I want search effort bounded, so that an uncertain answer does not cause unlimited work.
15. As Evie's owner, I want supported findings and unresolved gaps reported when a budget is exhausted, so that stopping does not turn uncertainty into a fabricated answer.
16. As Evie's owner, I want searches to return short excerpts first, so that whole conversations are not loaded unnecessarily.
17. As Evie's owner, I want Evie to expand around a matching passage when needed, so that isolated sentences are not interpreted without their context.
18. As Evie's owner, I want valid evidence reused during a turn's tool continuations, so that reconstructing a request does not repeat all retrieval work.
19. As Evie's owner, I want changed evidence needs and eligibility to trigger refresh or exclusion, so that reused results remain appropriate.
20. As Evie's owner, I want earlier conversations in the same Workspace available, so that continuity follows the area where I am working.
21. As Evie's owner, I want earlier conversations in the same project available, so that project decisions remain recoverable across sessions.
22. As Evie's owner, I want Global conversations to recall earlier Global conversations, so that unscoped personal discussion retains continuity.
23. As Evie's owner, I want Workspace and project conversations to use eligible accepted Global memories without gaining raw Global-history access, so that broad knowledge and transcript access remain distinct.
24. As Evie's owner, I want unrelated Workspaces and projects excluded mechanically, so that a relevant phrase cannot cause cross-area disclosure.
25. As Evie's owner, I want another session's session-scoped Claims to remain excluded, so that conversation search does not silently promote temporary semantic state.
26. As Evie's owner, I want a named General Workspace treated as a Workspace, so that its name does not grant Global-history access.
27. As Evie's owner, I want an excerpt to establish what was said without automatically becoming accepted memory, so that tentative statements retain their meaning.
28. As Evie's owner, I want newer conversation evidence that conflicts with saved memory surfaced, so that a stale Claim does not silently override what I later told Evie.
29. As Evie's owner, I want source authority and uncertainty preserved, so that a newer guess or quotation does not automatically become the truth.
30. As Evie's owner, I want contradictions and historical validity visible when relevant, so that Evie does not blend incompatible facts into one answer.
31. As Evie's owner, I want retiring memory to suppress its corresponding conversation evidence in ordinary recall, so that history search cannot undo retirement.
32. As Evie's owner, I want explicit historical questions to retrieve retired evidence with its status marked, so that retirement does not erase my history.
33. As Evie's owner, I want unrelated information in a conversation containing retired evidence to remain available, so that retirement does not hide the entire conversation.
34. As Evie's owner, I want successful empty searches distinguished from failures, so that an unavailable index does not imply that I never supplied information.
35. As Evie's owner, I want Evie to continue from sufficient current context when memory is unavailable, so that an optional lookup failure need not prevent a useful answer.
36. As Evie's owner, I want Evie to explain when unavailable memory prevents an answer, so that a failure does not lead to guessing.
37. As Evie's owner, I want a compact memory activity card, so that retrieval is inspectable without narrating every routine preference lookup.
38. As Evie's owner, I want Accepted memory and Conversation excerpt labeled separately, so that I can distinguish saved knowledge from attributed statements.
39. As Evie's owner, I want supporting sources for historical answers, so that I can inspect the evidence behind them.
40. As Evie's owner, I want original evidence references preserved after later corrections, so that I can understand the basis of an earlier answer.
41. As Evie's owner, I want source inspection to respect current access, so that an old answer cannot bypass a later restriction.
42. As Evie's owner, I want evidence supplied to the model distinguished from evidence cited in its answer, so that the UI does not claim knowledge of the model's internal reasoning.
43. As Evie's owner, I want supplemental memory sent remotely only under the existing opt-in and source rules, so that automatic retrieval does not expand egress authority.
44. As Evie's owner, I want secrets, opaque provider state, and arbitrary raw tool payloads excluded from supplemental evidence, so that retrieval does not bypass existing data boundaries.
45. As Evie's owner, I want retrieved instructions treated as quoted data, so that remembered text cannot grant permissions or approve actions.
46. As Evie's owner, I want index coverage, restart, and stale-result behavior verified, so that recall remains trustworthy as stored history grows.
47. As Evie's owner, I want retrieval quality measured separately from answer quality, so that improvements target the actual failure rather than an aggregate score.
48. As Evie's owner, I want latency and context cost measured alongside useful recall, so that richer memory remains practical for everyday conversation.

## Implementation Decisions

1. **Ownership and existing boundaries.** The Kernel retains authority over
   memory scope, accepted state, source eligibility, lifecycle, and persistence.
   Add a narrow retrieval interface consumed by the agent runtime and adapted
   by the First-party Memory Plugin. Automatic Recall and model-directed reads
   use the same eligibility and rendering policy. Do not create a general
   pluggable memory-provider framework or give plugins raw database access.

2. **Turn integration.** Start Automatic Recall on a new user message. Recompose
   each provider request through the existing context composer, reusing evidence
   while its interpretation, scope, temporal applicability, and relevant source
   and state revisions remain valid. New information can trigger targeted
   refresh. Recheck eligibility and egress before each dispatch; neither cached
   results nor request recomposition bypass those checks. Extraction and index
   backfill remain outside the foreground answering dependency chain.

3. **Interpretation.** Query planning can use relevant earlier conversation,
   the compaction summary, the current request, and eligible memory. A summary
   helps interpretation but is not new source evidence. Explicit IDs and dates
   have deterministic handling; model-proposed entity or temporal readings are
   hypotheses. Remaining material ambiguity produces clarification. Exact
   bounded input selection and any local interpretation-model configuration
   must be evaluated rather than silently fixed to the last message.

4. **Semantic scope matrix.** Global sessions may read their own session scope
   and Global memory. Workspace sessions additionally read their one Workspace;
   project sessions additionally read their one project. Other sessions'
   session-scoped Claims remain excluded. Scope is resolved from the durable
   session by the harness and cannot be widened through tool arguments or model
   text. Existing explicit Promotion rules remain unchanged.

5. **Conversation scope matrix.** Earlier conversations are searchable within
   the same Workspace, within the same project, or Global-to-Global. Accepted
   Global-memory access does not grant raw Global-conversation access from a
   Workspace or project. General is not a synonym for Global. This is the
   narrow conversation-evidence exception recorded in ADR 0066; it does not
   expose another session's session-scoped Claims, revive a source session, or
   widen write authority. No private-conversation setting is introduced.

6. **Search and expansion.** Support focused accepted-memory search and scoped
   conversation search through read-only tools, plus bounded expansion around
   an eligible excerpt. Initial Automatic Recall may draw from both evidence
   kinds. No fixed semantic-first sequence or fixed number of unsuccessful
   searches determines sufficiency. Whole conversations are not returned by
   default. Tools and expansion remain subject to existing Capability
   availability, source access, and invocation boundaries.

7. **Query and ranking contract.** A QueryPlan carries effective scopes,
   temporal intent, identifiers and entity candidates, lexical and dense query
   inputs, graph bounds, authority constraints, and result/context budgets.
   Independent exact/alias, FTS, temporal, graph, recent-episode, and enabled
   dense generators share cancellation and a deadline. Apply deterministic
   eligibility filters before fusion, then begin with Reciprocal Rank Fusion
   and transparent reranking by relevance, entity match, relationship support,
   authority, temporal applicability, corroboration, and evidence diversity.
   Recency is contextual rather than a universal truth ranking. Do not pad a
   result to its budget with weak evidence.

8. **Evidence identity.** Results distinguish accepted Claim evidence from
   Conversation Excerpts; an excerpt does not require a fabricated Claim ID.
   Retain source event/session identity, source scope, authority, observed time,
   applicable validity and lifecycle, retrieval path, and exact evidence
   locators and hashes. Reuse the existing EvidenceLocator contract, including
   canonical half-open UTF-8 byte ranges where applicable. Preserve uncertainty
   and speaker attribution. Hypotheticals, reported speech, assistant guesses,
   and incomplete tool actions cannot be relabeled as confirmed owner facts.

9. **Retirement and source filtering.** Corresponding evidence for a retired
   memory is excluded from ordinary retrieval, including Automatic Recall,
   conversation search, and neighboring-message expansion. Explicit historical
   retrieval may include it with retired status marked. Unrelated evidence in
   the same conversation remains eligible. Before enabling these paths, specify
   and test the durable Claim-to-evidence association using existing source
   identities and locators, including passages supporting multiple Claims and
   overlapping expansions. Source ineligibility and revoked access still apply;
   a historical question does not restore access. Retirement preserves history
   and does not implement hard erasure.

10. **Conflicts and time.** Distinguish current Claims, historical Claims, and
    attributed statements. An excerpt saying a trip was being considered cannot
    establish a confirmed trip. If a saved Claim says Boston and newer owner
    evidence says Chicago, expose the discrepancy with its sources rather than
    silently selecting the stored Claim. Recency does not override source
    authority or establish truth. Reads do not accept, correct, supersede, or
    promote memory; mutations retain their existing reviewed operation path.

11. **Bounded work and failure.** Enforce search deadlines, concurrency,
    candidate/result sizes, expansion limits, and cumulative retrieval context
    costs in code. Keep initial recall small and allow bounded deeper work.
    Distinguish success with no matches, search failure, unavailable search,
    cancellation, and exhausted budget in the result contract. A failed
    component cannot make incomplete coverage appear exhaustive. If current
    evidence suffices, continue with a small Memory unavailable indicator; if
    unavailable memory is necessary, explain the limitation. Exhaustion reports
    supported findings and unresolved gaps. Exact numerical defaults and
    accounting policy are measured deliverables, not values invented here.

12. **Context rendering and egress.** Supply bounded, source-bearing
    EVIE_MEMORY_DATA as a synthetic user-role message immediately before the
    actual current user message. Account for it in the complete request budget.
    Keep stable instructions and tool definitions consistent while appending
    ordinary conversation events; do not rewrite history for retrieval. The
    synthetic projection is not persisted as a new episode. Existing durable
    tool-call/outcome records retain their own contracts and do not become
    independent corroboration merely by repeating retrieved evidence.
    Supplemental reads require EVIE_REMOTE_MEMORY=on before remote delivery,
    including automatic excerpts, tool results, and expansion. Apply existing
    source fences and secret scans; exclude reasoning, opaque continuation, and
    arbitrary raw structured payloads. Quoted data cannot alter permissions.

13. **Derived indexes.** Add the umbrella's FTS projections, embedding metadata,
    immutable index configuration generations, coverage checkpoints, and
    idempotent refresh jobs. Backfill eligible retained rows before a generation
    serves queries. Subsequent eligible appends and accepted revisions update
    the appropriate redacted projections and enqueue enabled refresh work
    transactionally. Index only allowlisted evidence, not raw payloads. Recheck
    hits against current authoritative SQLite state; stale indexes cannot
    resurrect ineligible evidence as current or bypass the requested temporal
    and lifecycle rules for historical evidence.
    SQLite remains authoritative and derived indexes remain rebuildable.

14. **Dense retrieval selection.** Preserve the planned local embedding and
    vector-index spike: compare a simple SQLite/brute-force baseline with a
    local approximate index against useful recall, p95 latency, memory,
    persistence/rebuild behavior, and operational cost. Record the selected
    model, configuration, and index before adding the chosen production
    implementation. Local embedding endpoints remain loopback or Unix-socket
    only, with redirects denied and no remote fallback. Do not add a production
    dependency without the repository-required approval. Learned graph scoring
    is not part of this stage.

15. **Original evidence references.** Retain a durable association from each
    conversational request/context snapshot to the exact evidence supplied and
    from the answer to its relevant request sequence. Reuse immutable source
    locators, hashes, and Claim versions instead of rerunning current search to
    reconstruct the past. Record evidence kind, historical state, and rendering
    version as needed for faithful inspection. Current diagnostics stay
    content-free; do not copy excerpts or hidden reasoning into telemetry.
    Finalize the minimal receipt schema and its persistence ordering before
    implementation. Distinguish supplied evidence from explicit answer
    citations; neither proves the model's internal causal reasoning.

16. **User presentation.** Adapt existing chat activity and source-inspection
    surfaces to show a compact memory card with Accepted memory and Conversation
    excerpt labels. Routine preference use needs no lookup narration. Past-fact
    and decision answers expose supporting references; conflicts and retired
    historical evidence keep their labels. Inspection preserves original
    evidence state while showing later corrections separately and reapplying
    current access. Unavailable sources are shown as unavailable. Exact card
    grouping and receipt field names are implementation details, not new memory
    authority or a requirement for a separate dashboard.

17. **Delivery boundaries.** Treat this as the Stage 5 feature specification,
    not one large implementation change. Deliver independently reviewable
    outcomes for deterministic retrieval and eligibility, indexing and its
    measured dense spike, turn/tool integration, and source inspection/UI.
    Respect their dependencies and keep persistence, authorization, concurrency,
    and UI changes inspectable. Finish the bounded query/budget, evidence
    association, receipt, and evaluation contracts before their dependent code;
    publication does not declare those experiments complete.

## Testing Decisions

1. **Owner-confirmed primary seam:** exercise a complete agent
   conversation turn against real temporary SQLite history and accepted memory,
   with a scripted conversational client that captures outgoing requests and
   emits controlled tool calls. Invoke the same retrieval behavior through
   Automatic Recall and the Memory Plugin rather than independently mocking
   ranking, SQL, and rendering. Reuse existing constructors, turn execution,
   scope-bound memory operations, and context composition. Add only the narrow
   agent-consumed retrieval boundary required for integration. Isolate local
   embedding/model I/O with deterministic fixtures for model-free checks.

2. **Prior art:** existing durable-compaction acceptance tests already exercise
   real SQLite, scripted provider requests, tool continuations, and process
   reopen. The semantic scope-containment matrix covers multiple Global,
   Workspace, project, and session actors. Semantic lifecycle and replay tests
   provide the accepted-state baseline; Memory Plugin tests provide composition
   and approval-boundary examples. Extend those patterns rather than creating
   separate fake production stacks for each search generator.

3. **Observable deterministic acceptance:** verify exact allowed/forbidden
   evidence, source references, serialized request placement and bounds,
   unchanged accepted state, failure categories, and persisted inspection
   references. Tests should survive changing a private helper or ranking
   implementation. Do not assert arbitrary SQL shape, internal call ordering,
   or natural-language answers prewritten by a scripted client as proof of
   model quality.

4. **Core scenarios:** saved vegetarian preference in a fresh chat; a relevant
   uncompiled gardening excerpt; returning to mother after debugging; genuine
   mother/sister ambiguity; no useful evidence; exact-ID and alias matches;
   lexical versus semantic matches; supported one/two-hop relations; current
   versus historical questions; tentative trip wording; Boston/Chicago conflict;
   contradictory active Claims; and distractors exceeding the evidence budget.
   The deterministic layer proves inputs and evidence contracts. Separate
   model-backed cases assess actual interpretation, clarification, attribution,
   and grounded answering.

5. **Scope and lifecycle matrix:** test automatic, tool, expansion, and source
   inspection paths across same/different Workspaces and projects, Global
   history versus accepted Global memory, General Workspace, other sessions'
   session-scoped Claims, Promotion, retirement, restoration, source
   ineligibility, and access changes. Retirement cases must cover ordinary
   suppression, marked historical retrieval, neighboring expansion, and
   unrelated information in the same conversation. All deterministic access
   and lifecycle expectations must pass without relying on a model judge.

6. **Request and egress acceptance:** capture the complete provider-bound
   request for automatic and tool-directed retrieval. Check opt-in disabled and
   enabled, secret-bearing excerpts, raw/opaque payload exclusion, malicious
   source instructions, compounded expansion costs, and absent semantic writes.
   Check that continuation reuses still-valid evidence, changed state is
   revalidated before dispatch, and fresh lookup data does not replace the
   original answer's evidence receipt.

7. **Persistence and failure acceptance:** reopen SQLite and inspect original
   receipts; change or retire a Claim after an answer; revoke source access;
   interrupt retrieval; exhaust budgets; fail one generator; fail all search;
   return a successful empty result; replace an index generation; backfill old
   events; append during coverage work; and supply stale index hits. Verify
   bounded cancellation, no late effects after lease loss, correct incomplete
   coverage/failure reporting, and no fabricated negative conclusion.

8. **Focused adapter seams:** use the existing HTTP test-server and React
   component patterns only for behavior the primary seam cannot establish:
   source-inspection authorization and stale/unavailable responses, memory card
   labels, source opening, original-versus-current state, and Memory unavailable
   display. UI checks consume representative public responses; they do not
   duplicate the retrieval engine tests.

9. **Quality and tuning:** create versioned Evie-specific fixtures with expected
   evidence IDs, supported answers, and abstention/clarification cases. Compare
   no recall, recent context, tool-only retrieval, automatic hybrid retrieval,
   and automatic plus deeper search; use an oracle-evidence reader condition to
   separate retrieval failure from answering failure. Measure evidence recall
   within a fixed context budget, answer grounding, stale-fact errors, unwanted
   recall, clarification, p50/p95 latency, context cost, and index costs. Pin
   configurations and hold out evaluation cases. Establish numerical release
   thresholds before declaring Stage 5 complete; public benchmark results are
   evidence, not Evie's acceptance gate.

10. **Verification:** for implementation, run focused behavior checks and the
    repository's full verify-change script, including Go tests/vet, UI
    lint/build, and whitespace checks. For this documentation-only synthesis,
    run git diff --check and check the newly created files for whitespace.
    Scripted-provider acceptance is not a substitute for the separately pinned
    model-backed evaluation. Manual demonstration follows the fresh-chat
    preference, cross-topic reference, historical source, conflict, retirement,
    and unavailable-memory scenarios above.

## Out of Scope

- Implementing or changing the Stage 4 compiler, extraction admission policy,
  Candidate review, or automatic acceptance of semantic changes.
- Promoting memory, widening session/Workspace/project access, raw Global-history
  access from a Workspace/project, or adding private conversations.
- Hard erasure, cloud synchronization, multiple owners, or hosted memory stores.
- Learned graph models, speculative memory-provider abstractions, or Stage 6
  graph acceleration before a measured need.
- Redesigning compaction, provider protocols, task focus, all tool-output
  truncation, or prompt caching. Reuse their current contracts.
- A memory-management UI redesign, new usage analytics dashboard, or claims that
  retrieved evidence reveals the model's hidden reasoning.
- Starting feature implementation, committing, pushing, or opening a pull
  request as part of this specification-publication task.

## Further Notes

This specification synthesizes the accepted September 10 retrieval interview
Q1–Q19 and the existing memory umbrella. The repository glossary, adjacent
memory decision record, and ADRs 0034, 0047, 0049, 0050, 0051, 0053, 0056, 0059,
0061, and 0066 supply the governing scope, ownership, acceptance, source, and
lifecycle boundaries. The newer explicit conversation-recall exception applies
to excerpts; it does not generally replace older semantic-session exclusions.

Stage 3 accepted state and durable conversation/context composition are the
baseline. Stage 4 compilation is tracked in
[issue #131](https://github.com/davidadel66/evie/issues/131); initial retrieval
can be developed and verified with explicit accepted operations and retained
episodes without awaiting a successful extraction model. Candidate material
remains unaccepted, and foreground recall never waits for compilation.

The main risks are reference ambiguity, stale or overly broad evidence,
retirement bypass through excerpts, index coverage gaps, incorrect source
attribution, and excessive retrieval cost. The tests and prerequisite contracts
above make these measurable. Numeric budgets, the embedding/index selection,
and exact receipt/association schemas remain explicit engineering deliverables;
they are not silently treated as decisions already made in the interview.

Publication uses the ready-for-agent label as requested. It establishes a
feature specification with explicit delivery gates, not a claim that all
subordinate contracts, experiments, or release checks have already passed.
