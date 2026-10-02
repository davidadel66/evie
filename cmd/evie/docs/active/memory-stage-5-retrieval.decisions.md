# Memory Stage 5 retrieval — decisions

- **2026-10-01 — The cumulative memory budget charges each delivered byte once
  per turn (harness review C2, Stage 4).** The 36 KiB per-turn bound still
  limits new memory evidence, but admission no longer charges a request's
  complete memory message and every replayed memory-tool outcome again on each
  provider request. A unit is one evidence item of `EVIE_MEMORY_DATA`, charged
  for the escaped bytes it adds to that message and keyed by its exact
  encoding, or one replayed memory-tool outcome message. It is charged the
  first time its exact bytes reach the provider in the turn; resending it is
  not new delivery. The envelope (status, gaps, reading guide, historical IDs)
  is harness metadata and is not charged. A turn with one search followed by
  ordinary tool calls therefore no longer stops with a misleading
  `context_overflow` around its eleventh request. When new evidence would
  exceed the remaining budget, already delivered evidence stays, only new
  evidence that fits is admitted, and the projection is marked `exhausted`.
  The former early close of investigation when one request used more than
  half the remaining budget is removed, because the reader's resend is free.
  `CumulativeMemoryBytes` in `turn-evidence-v1` receipts records this distinct
  delivery. A hard overflow of new delivery still stops the turn before
  another provider request. Request-headroom fitting is unchanged.

- **2026-10-01 — Bound spans, Evie-proposed sources and ambiguous names in
  recall (harness review M5, M6, Stage 14).** Follows the 2026-10-01 memory
  decisions on owner-span binding and Entity identity.
  - A Claim's source renders only its bound span in recall, identity
    matches, Claim queries, object and Source Link inspection, object
    listings and operation history, so a Global memory read in a Workspace
    or project session no longer carries the rest of the Global message. A
    whole-message owner Source accepted before Stage 14 is narrowed at read
    time for such readers to the sentence holding its Claim's value (same
    matcher, including the final pass's stating-sentence rules), or to no
    text; its reference (locator and hash) is unchanged, and Global readers
    still see the whole message. The bound span is the sentence that states
    the value, so a retired memory suppresses and labels that sentence, not
    an earlier one that only mentions the value (final verification pass).
  - An approved `evie_proposed` Claim stays retrievable (the owner approved
    it): the accepted-memory authority allowlist gains `evie_proposed`, its
    source carries the label and no text, and the cited message's hash is
    still verified. Inspecting its receipt (`InspectMemoryEvidence`) is
    available with the label and no quote (final verification pass; it was
    previously reported unavailable because the source had no text).
  - Conversation associations ignore `evie_proposed` Source Links: retiring
    such a Claim suppresses nothing in the request message, never labels it a
    historical source, and does not count it as represented provenance for
    newer-statement companions. Span-bound links suppress and label only their
    span, so unrelated sentences of the same message stay recallable
    (decision Q19's "unrelated information remains eligible").
  - A Claim whose Entity name is ambiguous among the reader's visible Entities
    carries `ambiguous_names` and identifies the Entity in its text; the
    lexical index and generators are unchanged.
  - *Confirmation review (2026-10-01).* The stating sentence is chosen by
    the clause's subject and the shortest span (see the memory decision),
    so "I live in Boston. My therapist in Boston says..." binds and
    renders "I live in Boston.", a Workspace search for "home city" no
    longer receives the therapist sentence, and retiring the memory
    suppresses the owner's own sentence. Operation history blanks free text
    that cites no value (a review edit's reason) for readers outside the
    scope it was written in.
  Measured on the scale corpus (`memory-scale-eval.md`, Stage 14): both tiers'
  reports are unchanged.

- **2026-10-01 — Corrected and retired facts are labelled, not presented as
  current (harness review M2, Stage 13).** Applies implementation decision 10
  and David's M2 default: corrected sources stay retrievable but are labelled
  historical and linked to the correction. Every Conversation Excerpt, on
  every path (Automatic Recall, conversation search, newer-statement
  companions, expansion, dense hits, revalidation and source inspection),
  carries `historical_claims`, recomputed from accepted state on each read and
  never copied from a reference:
  - *source*: the excerpt overlaps an eligible Source Link of a retired or
    superseded Claim. A superseded Claim also sets the excerpt's `status`
    (pinned) and `current_status` to `superseded`, and the link names the
    correction mode and `replacement_claim_id`. Retirement suppression of a
    retired Claim's own source is unchanged.
  - *restatement*: the excerpt is not a Source of the Claim, and one sentence
    repeats the saved value (literal value or object Entity name, as a word
    sequence after inflection folding) with at least one of the Predicate
    token's or label's content words, or, for a non-owner subject, the
    subject's canonical name. Booleans and lone numbers never anchor a match.
    An event that is a Source of an active Claim in the same subject and
    Predicate family states the current value and is exempt (so the
    correction message itself is not flagged).
  Only Claims in scopes the reader may read are named. Labels never suppress
  evidence. At most 1,024 retired or superseded readable Claims are compared
  with one excerpt; beyond that the excerpt is withheld and the result is
  partial rather than shown unlabelled. `EVIE_MEMORY_DATA` lists superseded
  items in `historical_only` beside retired ones, and the projection version
  becomes `memory-retrieval-v3`.
  - *Amended in the final verification pass (2026-10-01).* A restatement must
    also be the subject speaking, by the subject rule of the newer-statement
    entry below, applied to the clause holding the value: "My friend Sam says
    his favorite coffee is Blue Bottle." and "The Blue Bottle coffee shop on
    Main Street closed today." no longer restate the owner's retired
    `favorite coffee shop: Blue Bottle`; "Honestly, Blue Bottle is still my
    favorite coffee shop." still does. For a non-owner subject the sentence
    must name it.
  - *Amended in the confirmation review (2026-10-01).* The subject rule now
    comes from the clause analysis shared with the owner-span binder (see
    the newer-statement amendment below): a possessive chain names whose
    preference it is ("My dad's favorite coffee shop is Blue Bottle." and
    "My mom's favorite coffee is Blue Bottle." are not the owner
    restating), and a reported clause never restates.

- **2026-10-01 — Newer owner statements in different words (harness review
  M3, Stage 13).** Q18 and implementation decision 10 ask that newer owner
  evidence contradicting a saved Claim be surfaced. The companion search now
  groups the selected Claims by subject and Predicate *token* (every version)
  and keeps the earlier rule unchanged: the token, any label, or the whole
  query as a phrase. It adds one deterministic rule, checked per declarative
  sentence (sentences end at `. ! ? ;` or a line break before whitespace; a
  period after `dr mr mrs ms st jr sr prof vs etc mt` or a single letter does
  not end one; questions never qualify):
  - the subject speaks: first person (`i me my mine myself we us our ours`)
    for the owner, or the subject's canonical name otherwise; and
  - a saved value of the family with a change cue, or at least
    min(2, n) of one Predicate word group's n content words with a change cue
    or the novelty cue `new`.
  Change cues: `now nowadays anymore instead moved moving relocated relocating
  switched switching changed changing left quit quitting former formerly
  previously` and the phrases `no longer`, `used to`, `behind me`,
  `these days`. `new` is excluded beside a saved value, where it is usually
  news about that value ("Verizon sent me a new bill"). Matching folds
  inflection with the Stage 12 `relevanceKey`; FTS fetches candidates with
  prefix terms and every hit is re-checked in Go. The companion bound (two per
  search, shared candidate budget) is unchanged, and refresh validity uses
  the same rule. A companion remains a candidate discrepancy, never an
  accepted contradiction. Known limit: a statement sharing no word with the
  saved value or Predicate ("I moved to Chicago" against `home city: Boston`)
  is not linked; that needs meaning, which a topic dictionary or the real
  embedding model could supply and this rule deliberately does not.
  Measured on the scale corpus v2 (`memory-scale-eval.md`, Stage 13).
  - *Amended in the final verification pass (2026-10-01).* Naming the value
    and a cue somewhere in a first-person sentence over-linked: "I left my
    umbrella in Boston." and "My sister moved to Boston now." were linked to
    `home city: Boston`, and, being newer, took both companion slots from the
    real update. The rule now requires:
    - *Clauses.* A sentence splits into clauses at `, : ( )`, an en or em
      dash, and a hyphen with spaces on both sides.
    - *The cue governs the Claim's words.* The change cue (value rule) or
      change or novelty cue (Predicate rule) sits in the same clause as the
      saved value or one of the Predicate's words, with nothing between them
      but light words (articles, prepositions and particles such as `to from
      away back out`, `is are was were be been am`, contraction fragments,
      `not no still just finally officially really also already then`,
      `there here`, first-person pronouns), numbers, other cue words or the
      Claim's own words. "Boston is behind me", "moved to Boston", "I dropped
      Verizon" and "a size 10 now" qualify; "left my umbrella in Boston" and
      "left it in Boston" do not. `dropped` joins the change cues. Known
      limit: the same test drops "I quit my job at Initech" against
      `employer: Initech` ("job" is a content word between), since nothing
      lexical tells a job from an umbrella; "I quit Initech" or "I left
      Initech" still qualify.
    - *The subject speaks in that clause.* For the owner: a first-person
      pronoun (`i me myself we us mine ours`) or a possessive `my`/`our` with
      a Predicate or value word among the next three words ("my favorite
      coffee shop", "my new barber"); or the clause names no third party (a
      third-person pronoun, or a possessive like "my sister", "my friend Sam")
      and the sentence speaks for the owner ("I'm in Chicago, and Boston is
      no longer home."). For another subject: the sentence names it.
    - *Ranking.* Candidates are ordered by strength before recency: an update
      (a governed cue with the subject speaking) first, then the number of
      the Claim's words the sentence names (a saved value counts one, plus
      the Predicate words), then whether the subject speaks; recency breaks
      the remaining ties. Phrase matches (the earlier rule) are ranked the
      same way, so newer mentions of "home city" cannot crowd out an older
      "Boston is behind me". The two-companion bound is unchanged.
    Measured on the scale corpus v3 (`memory-scale-eval.md`, final pass).
  - *Amended in the confirmation review (2026-10-01).* Any content word
    between the cue and the value blocked the link, so "I no longer live in
    Boston.", "I used to live in Boston.", "I no longer work at Initech.",
    "I no longer use Verizon.", "I stopped using Verizon, switched to
    Mint..." and "I quit my job at Initech." were lost, while someone
    else's update still linked ("My ex left Boston.", "My boss quit
    Initech.", "Our team left Boston yesterday.", "My dad dropped
    Verizon.", and "My sister said, Boston is no longer an option.", whose
    second clause names no third party); in an end-to-end probe the ex and
    sister sentences took both companion slots from the real update. The
    rule now uses the clause and subject analysis shared with the owner-span
    binder (`retrieval_wording_clauses.go`, described in the 2026-10-01
    memory decision's confirmation-review amendment), so both agree on who
    said what:
    - *Subject.* The clause carrying the cue is not reported speech, a
      conditional or a question, and its subject is the owner ("I", "we",
      "my"/"our" before the Predicate's words); or its subject is the
      Claim's own words or nobody in particular, it names no third party,
      and the owner speaks in it or in another clause of the sentence ("I'm
      in Chicago, and Boston is no longer home"). A clause whose subject is
      someone else ("my ex", "our team", "the train", "my flight", "my
      dad's") never links; a possessive that does not own the Claim's words
      is read up to its first verb or function word ("my flight left
      Boston" is the flight's).
    - *Governing.* Between the cue and the Claim's words the clause may also
      hold a relation word: an ordinary verb of living, working or using
      (`live`, `work`, `use`, `stay`, `shop`, `bank`, `go`, `attend`,
      `rent`, `see`, `study`, `belong`, `call`, `have`, `based`, `reside` and
      their forms) or a noun for the owner's tie to it (`job`, `role`,
      `position`, `post`, `contract`, `plan`, `subscription`, `membership`,
      `account`, `service`, `lease`, `apartment`, `flat`, `house`, `home`,
      `place`, `office`, `company`, `firm`). "I left my umbrella in
      Boston", "We switched hotels in Boston", "I'm no longer worried about
      Boston traffic" and "I moved my car to Boston garage" still do not
      link. `stopped` and `cancelled`/`canceled` join the change cues (and
      the FTS candidate query).
    - *Safe failure.* A wrong link is a visible but misleading companion
      that can crowd out the real update, so an unsure subject does not
      link. Known limits: "We left Boston for a week of vacation." links (a
      trip and a move use the same words); "My home is Chicago now." does
      not (it shares only "home" with `home city`, one of two Predicate
      words); "I moved to Chicago last month..." still shares no word.
      Relation words are a closed list, so "I no longer sing in the
      Riverside choir" does not link.
    Measured on the scale corpus v4 (`memory-scale-eval.md`, confirmation
    review).

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
  - *Amended in the final verification pass (2026-10-01).* The two-word rule
    dropped the only answer to ordinary questions ("When do I need to renew
    my passport?" against "My passport expires in March 2029."), and
    acknowledgements containing "it" revived earlier topics.
    - *Filler.* The planner's noise list gains conversational filler: `so
      again also just really actually maybe anyway oh um uh hmm btw remind
      know need use`. "When do I need to renew my passport?", "Do you know
      where I parked the car?", "What's my sister's birthday again?",
      "Remind me which vet we use for the cat" and "ok so what's my wifi
      password" become two-word requests.
    - *Unknown words still count.* The review proposed counting only request
      words that occur in history toward the three-word threshold. Measured
      (with requests mostly unknown to history still needing two matches, so
      the ORM question stays safe), that reopens the one-shared-word leak: on
      the scale corpus's default tier automatic private items went from 0 to
      3, two of them "low energy" messages for "What's the energy rating of
      the dishwasher?" (`rating` unknown). An unknown word says the request
      is about something history has not discussed, so it keeps counting.
    - *Strong single match.* When every request word occurs in the
      searchable history and the request has three or more, an excerpt
      matching exactly one word qualifies if that word is the request's
      uniquely rarest (no other word ties its count) and occurs in at most 3
      searchable messages (`relevanceStrongDocuments`). "Remind me which vet
      we use for the cat" (all words known) recalls "Our vet is Dr. Rivera".
      A word tied for rarest does not qualify alone ("What did the doctor
      say about my ferritin?" must not pull in "...I didn't say anything"),
      nor does one beside an unknown word ("Summarize today's CI results."
      against a blood "test results" message).
    - *Common words.* A word counted in 256 or more searchable messages
      (`relevanceCommonDocuments`) is common and never distinctive. Counting
      stops there: each word is one prepared `count(*)` over at most 256 rows
      instead of every occurrence. At 20,000 messages the frequency counts
      fell from 170 ms to 9 ms and the whole search from about 265 ms to
      104 ms; at 40,000 from 338 ms to 10 ms (search about 195 ms, where it
      was about 520 ms, past the 500 ms read deadline, which maps to
      `exhausted`).
    - *Acknowledgements.* A message with no question mark, none of `what
      which who whom whose how when where why tell remind recall remember
      show explain find search look describe`, at most one content term and
      the word "it" ("got it, thanks", "that's it, thanks", "love it", "ok do
      it") is low content: no search and no earlier topic revived. Such
      messages, and earlier roots with no content term, are never earlier
      topics either. "And the basil?" (a question) and "her birthday" (no
      "it") stay follow-ups. The other-session items these phrases injected
      came from the search run with a revived topic's words; revived roots
      are always the bound session's own messages.
    - *Recency.* Earlier roots that tie on shared words and distinctiveness
      now resolve to the most recent ("and when was it?" after three topics
      takes the last two, not the first two).
    Measured on the scale corpus v3 (`memory-scale-eval.md`, final pass): no
    change outside the targeted probes in either tier; the frozen 24-case
    lexical workloads are unchanged.
  - *Amended in the confirmation review (2026-10-01).* The strong single
    match let one rare word explain any request whose words history knew:
    "Explain technical debt to the new engineers." injected "I owe forty
    thousand in credit card debt...", and likewise an anxiety poem (a
    therapist message), a custody playlist (a divorce lawyer message) and
    "What were the results of the run today?" (a blood test). Filler
    removal had a related effect: "Do I need to use a VPN for the bank?"
    became the two-word request [vpn, bank], and "bank" matched the
    overdrawn-account message instead of the VPN message.
    - *Personal recall.* The planner marks a request as a personal recall
      (`RetrievalRelevance.PersonalRecall`) when it refers to the owner (`I
      me my mine myself we us our ours`) and asks a recall question or
      request (`what when where which who whom whose how`, `again`, `remind
      me`, `do you know`, `do you remember`). Only such a request may be
      explained by one nearly unique word, and only such a request has its
      filler dropped; every other request keeps "need", "use", "know" and
      the rest as content words, as before the final pass. The VPN question
      now needs two matches and recalls "The VPN client needs an update."
      (`vpn` and `need`); the four recovered one-word answers ("When do I
      need to renew my passport?", "Do you know where I parked the car?",
      "Remind me which vet we use for the cat", "ok so what's my wifi
      password") are personal recalls and still recalled.
    - *Safe failure.* Automatic recall fails toward not injecting: an
      unrelated request that shares one rare word with private history gets
      nothing for that word, and the model can still search explicitly. The
      cost is a personal-sounding request without those markers ("Is my
      passport still valid?" has no question word) or an impersonal recall
      question ("What was the Lisbon hotel?"), which again needs two
      matching words when it has three or more.
    - *Known limits.* A personal recall request whose rarest word happens to
      be shared with an unrelated private message can still inject it ("What
      did I say about the debt?" against a private debt message is,
      arguably, recall). The marker lists are closed English lists, not
      intent detection.
    Measured on the scale corpus v4 (`memory-scale-eval.md`, confirmation
    review).

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
