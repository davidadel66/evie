# Memory Stage 5 retrieval — decisions

- **2026-10-01 — Automatic Recall relevance floor (harness review M1, Stage 12).**
  Implementation decisions 7 and 11 deferred the selection thresholds to
  measurement; these are the measured values, applying the plan's M1 default
  and memory decision Q6 (do not include weakly related material). All rules
  are deterministic; no learned ranker is involved.
  - *Low content.* A message with no content term after function words,
    pronouns and acknowledgements (`thanks`, `ok`, `sounds good`, `perfect`)
    and no explicit selector does not search: `status:"empty"`, no evidence,
    and no earlier topic is revived.
  - *Follow-up.* A message is a follow-up when it has at most 2 content terms
    or contains a referring pronoun (`he she him her his hers they them their
    theirs it its`). Only a follow-up takes its subject from an earlier topic
    it shares no word with: up to 2 earlier roots (5 terms each) and the
    compaction continuity (6 terms) join the query, each as its own group.
    For a standalone request, an earlier root joins only if it shares a term
    with the request, and then only to rank, never to qualify evidence.
  - *Excerpt floor.* A lexical Conversation Excerpt must match at least one
    distinctive current-request term, and two terms when the request has 3
    or more content terms. Distinctive is the rarer half of the request's
    terms that occur in the searchable history: document frequency at or
    below the lower median of those terms, counted over exactly the
    population the search could return (scope, generation, observation time,
    live-context exclusion). Terms absent from history neither qualify nor
    move the median. A follow-up's earlier-topic group qualifies an excerpt
    through one of its own distinctive terms. Matching folds common
    inflection (`runs`/`running`, `booked`/`book`); candidate fetching stays
    exact. The floor never pads: an empty slot stays empty.
  - *Live context.* Automatic Recall does not return the bound session's
    messages that are still in the provider request: all of them without
    compaction, and those from the retained frontier onward after it.
    Compacted-away messages remain recallable.
  - *Scope of the floor.* It applies to lexical Conversation Excerpts, the
    noisy kind. Accepted Claims keep their existing generators (exact, alias,
    lexical, temporal, graph, dense, conflict) and dense excerpts keep their
    0.25 cosine floor; the stricter term rules would remove paraphrase recall,
    and a calibrated automatic dense floor needs the real embedding model.
    Model-directed reads are unchanged.
  - *Measured effect.* Scale replay (`memory-scale-eval.md`): default tier
    automatic unwanted items 30→4 (rate 0.3704→0.0833), private 4→0,
    same-session 7→0, recall unchanged at 21/25; large tier unwanted 30→12,
    recall 22/25→23/25. Frozen 24-case held-out workload, lexical: automatic
    initial source obligations 22/28 before and after, automatic-plus-deeper
    union 26/28 before and after, non-gold first-dispatch items 24→19. The
    development workload loses one automatic obligation (24/27→23/27): its
    assistant suggestion that shares a single word with a 14-term question
    (dev20) is no longer injected automatically; bounded expansion still
    recovers it (deeper union 25/27 before and after). Known limit: a request whose distinctive words genuinely
    recur in unrelated history (a CI "test results" question against a blood
    "test results" message) still qualifies; ranking, not the floor, kept the
    private message out of that probe.

- **2026-10-01 — Dense recall covers the whole vector table (harness review
  M7, Stage 12).** The single 4,096-row scan read vectors in random-UUID order
  and dropped the rest silently, reporting a budget cut as a successful empty
  search. Dense generators now page through the selected generation in
  primary-key order, 1,024 vectors per read, keeping a running top-k, up to a
  budget of 65,536 compared vectors per generator per search. A scan also
  stops when less than a quarter of the 500 ms query deadline (125 ms)
  remains. A stop with vectors left marks the result `partial` and adds
  `gaps:["dense_scan_budget"]`, which both the tool outcome and
  `EVIE_MEMORY_DATA` carry. `partial` alone is not a truncation signal,
  because pending index work (including the active request's own message)
  also produces it. In the large scale tier all 24 dense-only targets across
  6,429 Global vectors are found, against 17 before.

- **2026-09-10 — Ticket breakdown accepted and published.** David accepted the
  proposed breakdown and requested a new-session implement prompt with one PR
  and one commit per GitHub issue. The 13 approved child issues are #156–#168,
  each labeled ready-for-agent and linked to #154 with native blocking edges.
  The implementation handoff uses the implement skill and treats a verified
  predecessor commit on the shared branch as satisfying a local dependency;
  issue closure is not required between commits in the same PR.

- **2026-09-10 — Specification published.** The confirmed specification is
  [issue #154](https://github.com/davidadel66/evie/issues/154), labeled
  ready-for-agent. Its published body and label were verified. David next
  requested the to-tickets workflow; ticket publication follows review of the
  proposed vertical slices and their blocking edges.

- **2026-09-10 — Synthesize the accepted retrieval design.** David requested
  the to-spec workflow after accepting retrieval interview Q1–Q19. The adjacent
  specification consolidates those choices and existing umbrella/ADR
  requirements; its engineering contracts and measured defaults are identified
  as deliverables rather than previously approved values. This task authorizes
  specification publication, not feature implementation.

- **2026-09-10 — Owner confirmed the testing seams.** David accepted the draft
  with "looks good" and requested the to-tickets workflow. Use a
  complete agent turn with real SQLite and a scripted provider as the primary
  acceptance seam, following existing durable-compaction and semantic-scope
  tests. Add focused HTTP/UI checks for source inspection and the memory card.
  This proposal does not introduce separate mocked implementations of each
  retrieval generator. The testing-seam check required before specification
  publication is satisfied.

- **Authority.** The September 10 entries in memory.decisions.md remain the
  binding interview record; ADR 0066 records the scoped conversation-recall
  exception. The adjacent specification does not supersede unrelated memory,
  provider, Task, or compiler decisions.
