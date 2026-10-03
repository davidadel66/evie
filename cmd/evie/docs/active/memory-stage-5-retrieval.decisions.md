# Memory Stage 5 retrieval — decisions

- **2026-10-03 — Recall quality: fact-augmented keys and time-aware query
  expansion (memory architecture Stage 6).** Applies the two LongMemEval
  findings that `docs/memory-architecture.md` adopts for relevance recall.
  Both are deterministic, read existing durable data, add no model call to
  the hot path, and never widen what a reader may see; a key or a window only
  changes which eligible evidence is preferred. Measured on the scale corpus
  v5 (`memory-scale-eval.md`, Stage 6).
  - *Granularity.* Conversation excerpts are already message-level (one
    index row per user or assistant message), finer than LongMemEval's round.
    That is kept; nothing is indexed per session. Episodes stay one per
    session and are searched on demand only.
  - *Fact-augmented keys.* The conversation index moves to generation
    `conversation-fts-unicode61-v4`, whose rows gain a `keys` column: for a
    message that is the cited Source of an accepted Claim, the Claim's
    Predicate label and token, its literal value, and the canonical names and
    aliases of its Entities. Only Claims that are currently active, whose
    Source Link is eligible and not `evie_proposed`, and whose scope every
    reader of the message may read (Global, or the message's own Workspace or
    project; never a session scope) key a message. Entity names and aliases
    join only from those same scopes, and the owner's and Evie's own anchors
    never do. Keys are matching text only: the excerpt still renders the
    message's own words under the existing span, retirement and source rules.
    A hit is rechecked on read: keys are recomputed from accepted state at
    the read's knowledge time, so a stale index cannot match through a
    retired, corrected or retracted Claim, and a key-only match starts its
    excerpt at the cited span (up to 200 bytes before it, as a word match
    does). Ranking is `bm25` with weight 1 for the message's words and 0.5 for
    its keys. The new generation backfills every retained event before it
    serves queries; the v3 table is left unread so an older binary can still
    resume it. Claim and Source Link changes already mark the cited message
    for refresh; a change to a Claim's other words (an alias added, an Entity
    or alias retired) already marks the Claim dirty in the accepted index,
    and refreshing that Claim now marks the messages it cites, so key upkeep
    adds no trigger work to memory writes. A refresh reads keys only for the
    messages some Claim cites. Evidence matched through a key carries the
    path `conversation_fact_key`.
  - *Keys and Automatic Recall.* A model-directed conversation search always
    matches keys. Automatic Recall matches them only for a personal recall
    (`RetrievalRelevance.PersonalRecall`, the 2026-10-01 rule): a Claim's
    words describe the owner's world, and a general request sharing a
    Predicate label ("Explain how daily medication reminders work in phone
    apps.") is not asking about it. Otherwise the read, its relevance floor
    frequencies and its ranking use the message's own words exactly as
    before (an FTS column filter). For a personal recall the floor counts the
    words of the keys whose span the excerpt overlaps, and its frequencies
    count both columns, the population the read could return.
    Newer-statement companions and refresh validity still match the
    message's own words only.
  - *Time-aware query expansion.* `memory.FindRecallTimeReference` reads one
    explicit past time from a request; `Window` resolves it at the request
    time (the Store's clock) in the owner's zone (the process time zone,
    `time.Local`), as a half-open UTC range. Weeks start on Monday; the first
    supported expression in the text wins. Supported, read as past without
    further cues: `yesterday` (with or without a part of the day), `the day
    before yesterday`, `last night` (18:00 yesterday to 06:00 today),
    `earlier today`, `this morning`; `N days|weeks|months|years ago` with N a
    number 1–99 or a word `a`/`an`/`one`–`twelve` (that local day, calendar
    week, month or year), `a couple of days|weeks ago` (2–3 back) and `a few
    days|weeks ago` (2–5 back); `last|previous week|weekend|month|year` (the
    previous calendar unit; the weekend is its Saturday and Sunday);
    `earlier this week|month|year`; `last <weekday>` (that day of the
    previous calendar week); `last <month>` (the latest such month before the
    current one); `<month> <year>` and `in|during|early|late|mid <month>
    <year>`; `in|during <year>`; ISO `yyyy-mm-dd`; and `[on] <month> <day>
    [<year>]` or `[on] <day> <month> [<year>]` with a year (month names in
    full, or abbreviated beside a day or year, such as `Sept 14th`).
    Ambiguous names are read as past only when the request also speaks in
    the past (a closed list of past forms such as `did was were said told
    mentioned booked paid sent`, or a word of five or more letters ending in
    `ed`, not `-eed`) and the word before them is not a future marker (`next
    coming upcoming for by until till before after from`): `today`, `this
    week|month|year`, `on|this <weekday>` (the latest such day before today,
    or this week's if already past), `in|during|early|late|mid <month>` (the
    latest such month that has begun), and a month and day without a year
    (its latest past occurrence). A full month name is a time only after one
    of those prepositions or beside a day or year, so "I may have mentioned"
    and "Jan said" are not times. Not supported, and so never a window:
    future references (`tomorrow`, `next week`, `in two days`, `this
    weekend` before it starts, any range that has not begun), `since`,
    `recently`, `the other day`, `these days`, `nowadays`, bare weekday or
    month names, numeric dates other than ISO (`9/14` is ambiguous), and more
    than one expression per request.
  - *Windows on conversation excerpts.* Automatic Recall reads the reference
    from the raw request (its words stay in the query) and passes it to the
    Kernel (`RetrievalQuery.RecallTime`); a model-directed search has its
    query text read the same way. With a past window the Kernel first reads
    the query without its time words among messages recorded inside the
    window. If any excerpt there qualifies, only those are returned, each
    with the path `time_window`; otherwise the ordinary read runs unchanged,
    so a time word that named the topic rather than the moment loses
    nothing. Inside the window the relevance floor counts the request's
    dropped time words toward its size (as words history has not seen), so
    "What were the results of the run today?" still needs two matching words
    and a private message sharing one word is not injected; frequencies are
    counted inside the window. A request that names only a time is not
    searched by time in Automatic Recall. Dense hits join only the ordinary
    read; the window applies to lexical excerpts, whose recorded time it
    compares. The tool outcome carries `time_window` (expression, range,
    whether it matched).
  - *Windows on accepted Claims (valid time).* Among Claims matching the
    request's other words, those whose recorded Valid Time (their own, or
    the interval a `changed` correction closed) overlaps the window gain the
    path `valid_during` and rank first. When the window has ended, a Claim
    that was valid inside it but is not current now is read exactly as a
    model-directed `valid_at` read would read it: at the latest instant of the
    overlap, valid-at constrained, with its status (`superseded`) shown.
    Retired and erroneous Claims stay excluded, and a Claim without a
    recorded Valid Time is neither boosted nor excluded (observation time
    cannot stand in for an unknown fact date). An explicit `valid_at` wins.
  - *Windows on episodes.* `memory_search_episodes` reads the reference from
    its query text: episodes whose covered range overlaps the window are
    selected, the query's other words still match the summary, and a query
    that names only a time lists that range's episodes, most recent first.
    With nothing in the window the ordinary search runs. The result carries
    `time_window`.
  - *Diagnostics.* The automatic plan is `bounded-reference-lexical-v4`; its
    content-free interpretation records only the reference's kind
    (`time_reference`, such as `last_week`), never its words.
  - *Evaluation seam.* `Store.UseClock` sets the Store's clock and the
    owner's zone before the Store is shared, and event append now reads the
    Store's clock (production keeps the wall clock and process zone). The
    scale replay uses it to run the corpus on a synthetic timeline; replaying
    corpus v4 on it reproduced the committed v4 report exactly.
  - *Measured effect* (scale corpus v5, before → after; the full tables are
    in `memory-scale-eval.md`). Default tier: automatic recall 37/53 → 48/53,
    unwanted items 19 → 13, private items 3 → 1 (the one left is the new
    therapist probe, identical before and after; see the known limits);
    named-time probes 3/9 → 9/9 and fact-phrasing probes 5/10 → 10/10;
    conversation search recall 19/25 → 23/25 with unwanted items 67 → 35;
    stale misses 2 of 71 with 10/10 controls and no scope leaks, before and
    after. Large tier: automatic recall 38/53 → 50/53, unwanted 42 → 34,
    private 12 → 9. The frozen 24-case lexical workloads are unchanged,
    per case: held-out 22/28 automatic, 26/28 with deeper search;
    development 24/27 and 25/27.
  - *Known limits.* Keys come only from accepted Claims, so an uncompiled
    paraphrase ("When is my pup's yearly checkup?") is still not found
    lexically; they are not added to dense embeddings (that would need a new
    vector generation). An unrelated request that names a past time can
    prefer an in-window message that clears the floor over a stronger
    out-of-window one; the floor and fallback bound this. The past-tense test
    is a closed list plus an `-ed` rule ("What's booked on Friday?" reads
    Friday as past). Windows are calendar ranges in the owner's zone, so
    "last week" on a Monday means the previous Monday to Sunday, not the
    last seven days. A Claim's Valid Time is known only when recorded, and
    most remembered Claims have none. The new privacy probe "Write a short
    story about a therapist and a lighthouse keeper." injects one private
    message before and after: accepted recall matches the private Claim
    `therapist` (accepted Claims are not floored, decision of 2026-10-01) and
    its newer-statement companion search adds the owner's later therapist
    message through the Predicate phrase. That path is outside this change
    and is recorded for follow-up.

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
