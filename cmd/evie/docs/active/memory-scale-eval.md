# Memory evaluation at scale (M8)

Stage 11 of the 2026-09-30 harness review. This is a measuring instrument, not
a fix: it records how today's recall behaves at realistic history size so that
Stage 12 (recall relevance, M1/M7) and Stage 13 (currency and conflicts,
M2/M3/M4) are measured rather than guessed. Every unmet target below is
current behavior, recorded as such; none is accepted as correct. Stage 12's
and Stage 13's before/after results follow the Stage 11 baseline; the
committed baselines are the confirmation review report (version 4) on
corpus v4.

## What runs

`internal/memoryeval` generates a seeded synthetic history and replays it
through the production seams: real SQLite, real agent turns with a scripted
provider, the Memory Plugin tools, the real approval paths for remember,
correct and retire, and the real index refresh. Only the conversational
provider is scripted, and in the large tier the local embedding endpoint is a
loopback fake. Nothing reads or derives from a real Evie database.

- `scale_corpus.go`: the generator, gold labels and probes (pure, no I/O).
- `scale_score.go`: the scorer and report (pure; unit tested).
- `scale_replay_test.go`: the replay driver and baseline ratchet.
- `testdata/scale-baseline-default.json`, `testdata/scale-baseline-large.json`:
  the recorded baselines, including every probe's delivered items.

The corpus mixes 12 personal topics in Global (two marked private: health and
a relationship) with three project areas, owner and assistant turns,
low-content follow-ups, mid-session topic drift, ordinary asides, 14 planted
facts ("needles") among same-topic distractors, 15 accepted Claims, two
corrections (one `changed`, one `error`), two retirements, an unlinked mention
before a Claim, a restatement after retirement, a newer statement in different
words, and predicate label and cardinality drift. A project message reuses a
Global needle's exact words to test scope isolation. Corpus v2 (Stage 13)
adds two Claims whose later updates share the saved value or the Predicate's
words but not its phrase, and five distractors that share a saved value or a
Predicate word without updating or restating the Claim (see Stage 13).

Each probe runs in a fresh session on its own copy of the built database.
Messages are indexed when they are appended, so a shared database would let
one probe's question become another probe's evidence. Production event and
Claim IDs are random UUIDs, and the dense scan reads in ID order; the driver
seeds the UUID source so a given code version reproduces the same IDs. This
changes no production behavior. The default tier's report was checked to be
identical under a different UUID seed, so only the large tier's dense
reachability depends on ID order. Per-probe item lists are sorted because rank
order is not measured. Stage 12 made the dense scan cover every vector, so
dense reachability no longer depends on ID order either.

## Paths and labels

- **Paths.** `automatic` is Automatic Recall on a new user message.
  `memory_search` and `memory_search_conversations` are the model-directed
  tools, scripted one at a time with the query a model would plausibly
  choose. A turn holds at most eight evidence items, so calling both tools at
  once lets the second evict the first.
- **Item classes.** `required`: the probe's target. `tolerated`: on the
  probe's topic but not required. `unwanted`: everything else. Precision is
  (required + tolerated) / delivered; recall counts unique required keys.
- **Private.** An unwanted item from the health or relationship topics. This
  labels sensitivity only; no private-conversation feature is implied.
- **Same session.** Evidence re-injected from the probe's own session. Those
  messages are already in the provider request, so they count as unwanted.
- **Low-content follow-ups** ("thanks!", "ok", "perfect"): every injected
  item is unwanted, following the M1 default that injected items must match a
  distinctive current-message term.
- **Scan gap.** An observation keeps the coverage gaps reported by either
  model-visible surface: the tool outcome and `EVIE_MEMORY_DATA`. A missed
  dense target counts as *reported truncated* only when its outcome carried
  `dense_scan_budget`; `partial` alone does not count (report version 2).
- **Stale.** An item is *presented as current* when it has current intent,
  active status and current status, and no relation, conflict or
  historical-claim marking (report version 3). Correction and retirement
  checks miss when a stale item is presented as current. Contradiction and
  drift checks miss unless the two items are linked by a relation, conflict
  warning or historical-claim link. Over-linking checks (corpus v2) miss when
  a distractor arrives linked or flagged against the named Claim. Controls
  use wording the current code already handles; a failing control on a tool
  path fails the test, because then the instrument is wrong.

## Running it

```sh
go test ./internal/memoryeval/ -run TestMemoryScaleReplayDefaultTier -v
EVIE_MEMORY_SCALE_EVAL=large go test ./internal/memoryeval/ -run TestMemoryScaleReplayLargeTier -v -timeout 30m
```

The default tier runs in the ordinary suite (about 9 s here) and skips under
`-short`. The large tier (about 70 s) grows Global history past the former
single-read 4,096-vector dense bound and enables a deterministic concept-hash
embedder served on loopback, like the existing dense acceptance tests.

The frozen 24-case development and held-out workloads of the Stage 5
assessment can be re-run through the same scripted local probe, lexical only,
to check that a recall-policy change loses no source obligation:

```sh
EVIE_MEMORY_INTEGRATED_LEXICAL=1 go test ./internal/agent/ -run TestMemoryStage5IntegratedLexicalRecall -v
```

It makes no reader-model call. The original assessment used the real
`all-minilm:22m` endpoint, which this check does not need or reproduce.

The baseline is a ratchet: any change in the report, better or worse, fails
until the baseline is re-recorded in the same change with
`EVIE_MEMORY_SCALE_UPDATE=1`. The baseline diff is the before/after report.
Scope leaks or unmapped items always fail.

## Stage 11 baseline (corpus `memory-scale-replay-v1`, seed 20261001)

Default tier: 171 sessions (117 Global, 54 project), 1,142 owner and 1,127
assistant messages, 80 probes. Lexical retrieval only, the production default
without an embedding endpoint.

| Path | Family | Probes | Items | Unwanted | Private | Same session | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 36 | 81 | 30 | 4 | 7 | 0.6296 | 21/25 |
| automatic | relevant | 11 | 24 | 4 | 2 | 0 | 0.8333 | 11/11 |
| automatic | paraphrase | 2 | 2 | 0 | 0 | 0 | 1.0000 | 0/2 |
| automatic | follow-up | 2 | 4 | 2 | 0 | 2 | 0.5000 | 1/2 |
| automatic | low-content | 6 | 13 | 13 | 0 | 5 | 0.0000 | n/a |
| automatic | privacy | 3 | 5 | 3 | 1 | 0 | 0.4000 | n/a |
| automatic | unrelated | 3 | 6 | 6 | 1 | 0 | 0.0000 | n/a |
| automatic | stale | 9 | 27 | 2 | 0 | 0 | 0.9259 | 9/10 |
| memory_search | all | 22 | 15 | 0 | 0 | 0 | 1.0000 | 11/11 |
| memory_search_conversations | all | 22 | 97 | 10 | 1 | 0 | 0.8969 | 10/12 |

Automatic unwanted-item rate: 0.3704 (30/81); 17 of 36 automatic probes
injected at least one unwanted item. Every unrelated request ("What's 17 times
23?") injected two items. Private items reached four probes: a tyre-pressure
question (a blood-pressure reading), an ORM question (relationship trouble), a
translation of "good morning" (the blood-pressure reply), and a ferritin
question (an unrelated relationship message).

Stale-fact checks: 14 misses of 18; all 10 controls pass.

| Scenario | Issue | automatic | memory_search | conversations |
| --- | --- | --- | --- | --- |
| Correction (`changed`): old source | M2 | stale as current | absent | stale as current |
| Correction (`error`): old source | M2 | stale as current | absent | stale as current |
| Unlinked mention before a retired Claim | M2 | stale as current | absent | stale as current |
| Restatement after retirement | M2 | stale as current | absent | stale as current |
| Retired Claim's own source (control) | M2 | absent | absent | absent |
| Newer statement, different words | M3 | Claim only, unlinked | Claim only, unlinked | n/a |
| Newer statement, same words (control) | M3 | linked | linked | n/a |
| Predicate label drift | M4 | both, unlinked | both, unlinked | n/a |
| Predicate cardinality drift | M4 | both, unlinked | both, unlinked | n/a |
| Same predicate (control) | M4 | linked | linked | n/a |

Large tier: 493 sessions, 3,832 owner and 3,817 assistant messages, dense
retrieval on. Global holds 6,429 conversation vectors, so the scan reaches
4,096 (63.7%). 17 of 24 dense-only targets were found, and they are exactly the
17 that fall inside the scan; none beyond it was found. Every one of the 24
outcomes reported `partial`, because the probe's own just-appended message
leaves dense work pending, so `partial` cannot signal scan truncation. Dense
retrieval also recovered both paraphrase probes (automatic recall 22/25,
precision 0.6471). Stale misses: 13 of 18; the `error` correction's old
source was absent from automatic recall in this tier.

| Target (plan acceptance) | Met | Observed (default tier; M7 large tier) |
| --- | --- | --- |
| M1: low-content follow-ups inject nothing unwanted | no | 13 unwanted of 13 items, 6 probes |
| M1: privacy probes inject no private item | no | 1 private item, 3 probes |
| M2: corrected source not current | no | 4 misses of 6 checks |
| M2: retired fact restated is flagged | no | 4 misses of 6 checks |
| M3: different wording detected | no | 2 misses of 2 checks |
| M4: drift still warns | no | 4 misses of 4 checks |
| M7: dense covers all vectors | no | 17 of 24 targets |

One further finding falls outside the M rows: automatic recall re-injects
earlier messages from the current session (7 items in the default tier). In
both follow-up probes the re-injected prelude takes one of the two excerpt
slots, and in one of them the planted fact is missed.

## Stage 12: recall relevance (M1) and dense coverage (M7)

The rules and thresholds are recorded in
[the retrieval decisions](memory-stage-5-retrieval.decisions.md) (2026-10-01).
In short: low-content messages do not search; an earlier topic joins only a
short or referring follow-up, or ranks a standalone request it shares a word
with; a lexical Conversation Excerpt must match a distinctive request term,
and two terms for requests of three or more; messages still in the provider
request are not recalled; the dense scan pages through every vector and
reports `dense_scan_budget` if its budget ever cuts coverage. Model-directed
tools, accepted Claims and dense hits keep their existing selection.

Default tier, before → after (unchanged cells show one value):

| Path | Family | Items | Unwanted | Private | Same session | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 81 → 48 | 30 → 4 | 4 → 0 | 7 → 0 | 0.6296 → 0.9167 | 21/25 |
| automatic | relevant | 24 → 15 | 4 → 2 | 2 → 0 | 0 | 0.8333 → 0.8667 | 11/11 |
| automatic | paraphrase | 2 → 0 | 0 | 0 | 0 | 1.0000 → n/a | 0/2 |
| automatic | follow-up | 4 | 2 → 0 | 0 | 2 → 0 | 0.5000 → 1.0000 | 1/2 |
| automatic | low-content | 13 → 0 | 13 → 0 | 0 | 5 → 0 | 0.0000 → n/a | n/a |
| automatic | privacy | 5 → 4 | 3 → 2 | 1 → 0 | 0 | 0.4000 → 0.5000 | n/a |
| automatic | unrelated | 6 → 0 | 6 → 0 | 1 → 0 | 0 | 0.0000 → n/a | n/a |
| automatic | stale | 27 → 25 | 2 → 0 | 0 | 0 | 0.9259 → 1.0000 | 9/10 |
| memory_search | all | 15 | 0 | 0 | 0 | 1.0000 | 11/11 |
| memory_search_conversations | all | 97 | 10 | 1 | 0 | 0.8969 | 10/12 |

Automatic unwanted-item rate 0.3704 → 0.0833 (4/48); probes with an unwanted
item 17 → 2. The four left are two dinner-planning messages for "Suggest a
dinner for me tonight." (`dinner` is the request's only known word) and two
interval-run logs for the CI-results question, which share `today` and `run`.
The paraphrase probe that lost its two tolerated items had matched only the
word `jog`. Recall is unchanged at 21/25: the same two paraphrases, the
`work` stale probe (the request says "work", the saved Claim "employer") and
the basil follow-up remain missed; the dates follow-up still finds its booking, now without its own
prelude taking a slot. Stale outcomes are identical (14 misses of 18, all 10
controls pass): Stage 12 does not change M2, M3 or M4 behavior.

Large tier, before → after:

| Path | Family | Items | Unwanted | Private | Same session | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 85 → 69 | 30 → 12 | 4 | 6 → 0 | 0.6471 → 0.8261 | 22/25 → 23/25 |
| automatic | follow-up | 4 | 3 → 1 | 0 → 1 | 2 → 0 | 0.2500 → 0.7500 | 0/2 → 1/2 |
| automatic | low-content | 14 → 0 | 14 → 0 | 0 | 4 → 0 | 0.0000 → n/a | n/a |
| automatic | privacy | 5 → 4 | 3 → 2 | 1 → 0 | 0 | 0.4000 → 0.5000 | n/a |
| automatic | unrelated | 6 | 6 | 1 → 0 | 0 | 0.0000 | n/a |
| memory_search_conversations | all | 166 → 187 | 20 → 19 | 3 → 4 | 0 | 0.8795 → 0.8984 | 29/36 → 36/36 |
| memory_search_conversations | dense coverage | 51 → 72 | 0 | 0 | 0 | 1.0000 | 17/24 → 24/24 |

Automatic unwanted-item rate 0.3529 → 0.1739. All 6,429 Global vectors are
now compared and all 24 dense-only targets are found; no outcome reported a
scan gap, because none was cut. Ten of the twelve remaining automatic
unwanted items, and all four private ones, are dense-only hits: the
concept-hash proxy scores "What's my home city these days?" close to "I've
had a headache for a few days" because it embeds words, not meaning. The
lexical floor deliberately does not gate dense hits (it would remove
paraphrase recall), and calibrating an automatic dense floor against this
proxy would tune to an artifact; that needs the real model. The other two are
the CI probe's `today`/`run` match. With full coverage the conversation tool
also reaches more vectors: one more private message, one fewer unwanted item
overall.

Frozen 24-case workloads, lexical, scripted local probe:

| Workload | Condition | Before | After |
| --- | --- | ---: | ---: |
| Held-out v2 | automatic, initial obligations | 22/28 | 22/28 |
| Held-out v2 | automatic plus deeper, union | 26/28 | 26/28 |
| Held-out v2 | non-gold first-dispatch items | 24 of 54 | 19 of 48 |
| Development v1 | automatic, initial obligations | 24/27 | 23/27 |
| Development v1 | automatic plus deeper, union | 25/27 | 25/27 |

The held-out missing obligations are the same six before and after. The
development workload loses one automatic obligation: dev20's assistant
suggestion shares only the word "suggestion" with a fourteen-term question,
the same shape as the ORM question's one shared private word, so the floor
drops it; the owner message it answers is still delivered, and bounded
expansion recovers the suggestion (deeper union unchanged). The graph-bridge
Claim in dev11 depends on random-ID ties: it was missing in the one run of the
old code and in three of four runs of the new one (the fourth gave 24/27).

| Target (plan acceptance) | Stage 11 | Stage 12 | Observed (default tier; M7 large tier) |
| --- | --- | --- | --- |
| M1: low-content follow-ups inject nothing unwanted | no | yes | 0 items, 6 probes |
| M1: privacy probes inject no private item | no | yes | 0 private items, 3 probes (both tiers) |
| M2: corrected source not current | no | no | 4 misses of 6 checks |
| M2: retired fact restated is flagged | no | no | 4 misses of 6 checks |
| M3: different wording detected | no | no | 2 misses of 2 checks |
| M4: drift still warns | no | no | 4 misses of 4 checks |
| M7: dense covers all vectors or reports the gap | no | yes | 24 of 24 targets found |

## Stage 13: currency and conflicts (M2, M3, M4)

The rules are recorded in [the retrieval decisions](memory-stage-5-retrieval.decisions.md)
(M2, M3) and [the memory decisions](memory.decisions.md) (M4), all 2026-10-01.
In short: every Conversation Excerpt carries `historical_claims` naming a
retired or superseded Claim it is a source of (a corrected source is also
marked `superseded` and names the correction and replacement) or restates
(saved value plus a Predicate word, or a non-owner subject's name, in one
sentence); a later owner statement is linked to a saved Claim when one
first-person sentence names the saved value with a change cue, or the
Predicate's words with a change or novelty cue; conflicts, refresh validity
and correction refresh span every version of a Predicate token; labels that
differ only in case or spacing reuse the existing definition.

**Corpus v2.** Measuring precision needed cases the v1 corpus lacked, so the
corpus version is `memory-scale-replay-v2` (seed unchanged) and the scorer
report version is 3. Added: Claims `phone carrier: Verizon` and
`shoe size: 9` with updates "I finally dropped Verizon last week and switched
to T-Mobile." (saved value) and "My shoes are a size 10 now after the running
season." (Predicate words); distractors "The Boston marathon moved to a new
date this year." (no first person), "My Boston friends are visiting next
week." (no change cue), "Verizon sent me a new bill…" (`new` beside a value),
"I need new running shoes before the 10k." (one of two Predicate words) and
"Grabbed a Blue Bottle cold brew at the airport this morning." (a retired
value without its Predicate's words); and a `blue_bottle` probe that queries
the retired value so that mention is delivered and its flag can be checked.
All new assistant replies are "Okay, noted." so the additions exercise only
M2–M4. "Before" is the Stage 12 code on corpus v2; the Stage 12 numbers
above are corpus v1 and are not directly comparable.

Stale-fact checks, before → after:

| Tier | Misses | Controls passing | Scope leaks |
| --- | ---: | ---: | ---: |
| default | 20 of 36 → 2 of 36 | 10/10 | 0 |
| large | 19 of 36 → 2 of 36 | 10/10 | 0 |

| Target | Default before → after | Large before → after |
| --- | --- | --- |
| M2: corrected source not current | 4 of 6 → 0 of 6 misses | 3 of 6 → 0 of 6 |
| M2: retired fact restated is flagged | 6 of 9 → 0 of 9 | 6 of 9 → 0 of 9 |
| M2: no over-flagging | 0 of 3 → 0 of 3 | 0 of 3 → 0 of 3 |
| M3: different wording detected | 6 of 6 → 2 of 6 | 6 of 6 → 2 of 6 |
| M3: no over-linking | 0 of 8 → 0 of 8 | 0 of 8 → 0 of 8 |
| M4: drift still warns | 4 of 4 → 0 of 4 | 4 of 4 → 0 of 4 |

The two remaining misses are the same scenario on the automatic and
`memory_search` paths: "I moved to Chicago last month and I'm still unpacking
boxes." against `home city: Boston` ("Remember that I live in Boston."). It
shares no word with the saved value, the Predicate token or label, or the
query, so no lexical rule can relate them without a topic dictionary
("moved" means "home"), which the retrieval decisions exclude; the real
embedding model is the measured next step. The review's own example, "Big
news: I moved to Chicago last month, Boston is behind me.", names the saved
value and is linked (covered by the Stage 13 agent tests).

Precision: the over-linking checks pass (0 of 8, 0 of 3). Across every probe
of the default tier the new rule added exactly three items, all intended: the
carrier update on the automatic and `memory_search` paths and the shoe-size
update on `memory_search`; on the automatic path the already-delivered
shoe-size update gained its link. Every delivered historical link (11
deliveries over 11 probes) names the right Claim: they are the two corrected
sources and the two restated retired facts, nothing else.

Path totals, before → after (unchanged cells show one value):

| Tier | Path | Items | Unwanted | Private | Same session | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| default | automatic | 55 → 56 | 4 | 0 | 0 | 0.9273 → 0.9286 | 23/27 |
| default | memory_search | 17 → 19 | 0 | 0 | 0 | 1.0000 | 13/13 |
| default | memory_search_conversations | 110 | 18 | 1 | 0 | 0.8364 | 10/12 |
| large | automatic | 77 → 78 | 14 | 4 | 0 | 0.8182 → 0.8205 | 25/27 |
| large | memory_search | 19 → 21 | 1 | 0 | 0 | 0.9474 → 0.9524 | 13/13 |
| large | memory_search_conversations | 200 | 27 | 4 | 0 | 0.8650 | 36/36 |

Stage 12's automatic-recall numbers do not regress: the automatic unwanted
items, private items, same-session items and recall are identical before and
after on both tiers (the unwanted rate falls only because one wanted item is
added), every M1 target stays met, and M7 still finds 24 of 24 dense targets
over 6,445 vectors. The frozen 24-case lexical workloads are unchanged:
held-out 22/28 automatic, 26/28 with deeper search, 19 non-gold of 48
first-dispatch items; development 23–24/27 automatic (the dev11 random-ID tie
recorded in Stage 12) and 25/27 with deeper search.

| Target (plan acceptance) | Stage 12 | Stage 13 | Observed (default tier; M7 large tier) |
| --- | --- | --- | --- |
| M1: low-content follow-ups inject nothing unwanted | yes | yes | 0 items, 6 probes |
| M1: privacy probes inject no private item | yes | yes | 0 private items, 3 probes |
| M2: corrected source not current | no | yes | 0 misses of 6 checks |
| M2: retired fact restated is flagged | no | yes | 0 misses of 9 checks |
| M3: different wording detected | no | no | 2 misses of 6 checks (one scenario, above) |
| M4: drift still warns | no | yes | 0 misses of 4 checks |
| M7: dense covers all vectors or reports the gap | yes | yes | 24 of 24 targets found |

## Stage 14: memory authority and entities (M5, M6)

The rules are recorded in [the memory decisions](memory.decisions.md) and
[the retrieval decisions](memory-stage-5-retrieval.decisions.md) (2026-10-01).
In short: a remembered value cites the exact sentence span of the owner's
message that contains it (whole content when that is the entire message); a
value the message does not contain is `evie_proposed`, quotes nothing, and is
labelled on the approval card; sources render only their span; retirement
suppresses only the span; reused and same-named Entities are named on the
card and ambiguous names are marked in recall. A follow-up commit applies the
same binding to `memory_correct_claim` replacement values, and at read time
narrows a pre-Stage-14 whole-message Global source shown in a Workspace or
project session to the sentence holding its value (or no text), without
rewriting history. The scale numbers below were re-run after both commits
and are unchanged; the corpus's two corrections are single sentences that
state their replacement ("Correction: my sister's birthday is June 13, not
June 3."), so they are cited exactly as before.

**Regression check.** Both tiers were run on the Stage 13 code and on this
change with the committed corpus v2 and report v3 baselines. Both reports are
identical before and after (the ratchet passes without re-recording), so
every Stage 12 and 13 number above stands: stale misses 2 of 36 in each tier
with 10/10 controls, no scope leaks, automatic unwanted 4 (default) and 14
(large), no private items in the privacy probes, every M1 target met, and 24
of 24 dense targets. This is expected: every corpus remember command is one
sentence that states its value ("Remember that my barber is Luis."), so its
span is the whole message and it is cited exactly as before.

The frozen 24-case lexical workloads are also unchanged: held-out 22/28
automatic, 26/28 with deeper search, 19 non-gold of 48 first-dispatch items;
development 23–24/27 automatic (the dev11 random-ID tie recorded in Stage
12) and 25/27 with deeper search. Of the held-out workload's 29 accepted
records, one now cites a single sentence ("For audiobooks I prefer one
narrator throughout;") and one is Evie-proposed, because its saved value
paraphrases the record ("A3 sheets with seam allowances included" against
"A3 sheets with the seam allowances already included"); the development
workload's 33 are all whole-message owner statements. The workload builder
now accepts a bound span or an Evie-proposed reference as the record's exact
source.

**Probes not added.** The scale instrument cannot express M5 or M6 without
new machinery: its corpus has only Typed Literal Claims whose commands state
their value, no Entity Claims, and its scorer records item keys, status and
links, not source authority, source text or Entity identity. Adding them
needs a corpus v3 (Evie-proposed and multi-sentence remember steps, Entity
steps), new item fields and check kinds, and a report version bump. That is
deferred; the acceptance criteria are covered by focused tests through the
same production seams (real SQLite, real agent turns, the Memory Plugin and
approval path) in `internal/agent/memory_authority_test.go`:
a multi-sentence Global command is cited as its one sentence and a Workspace
reader sees nothing else of it; a value absent from the command is
Evie-proposed, retrievable, labelled and unquoted in retrieval and
inspection; a model tool call after "Read this article and remember what
matters." reaches the approval card as Evie-proposed with no quote; retiring a
span-bound memory keeps the message's other sentence recallable; retiring an
Evie-proposed memory neither hides nor labels its request; promotion keeps
the Evie-proposed label; Alias reuse, stable-ID reuse and a new same-named
Entity are shown for approval; and two Entities named Sarah are marked and
told apart in recall while a unique name is not. Corrections are covered
there too: a multi-sentence correction cites its one sentence, a replacement
absent from the request is Evie-proposed and replays, and a model correction
after "Read this article and update my payee if needed." reaches the card as
Evie-proposed with no quote. `internal/eviedb/semantic_legacy_source_test.go`
accepts pre-Stage-14 whole-message Global remembers and checks that a
Workspace reader sees only the value's sentence (or no text when none
matches) on search, Claim query, Claim and Source Link inspection and object
listing, while a Global reader still sees the whole message; it also checks
that the dense index embeds the plain Claim text, so a later same-named Entity
does not make an indexed vector look stale. The matching rules have table
tests in `internal/eviedb/semantic_source_binding_test.go`.

| Target (plan acceptance) | Met | Evidence |
| --- | --- | --- |
| M5: a proposed memory's source is the exact quoted span | yes | span locator and hash on the proposal; replay verified |
| M5: a value absent from the owner's words is Evie-proposed and the card says so | yes | approval arguments and card test |
| M5: Global text does not reach Workspace sessions | yes | Workspace recall carries only the span, including for pre-Stage-14 whole-message sources |
| M5: corrections bind like remembers | yes | correction span and Evie-proposed tests |
| M6: alias reuse is shown on the card | yes | `identities` on the proposal and card test |
| M6: ambiguous aliases are marked in recall | yes | `ambiguous_names` and identifying text |

**Final verification pass (same day).** A reviewer's probes found four gaps,
now fixed (rules in the 2026-10-01 memory decision):

- The span was the first sentence containing the value, not the one stating
  it: "My therapist in Boston says the panic attacks are getting worse.
  Remember that I live in Boston." cited the therapist sentence, a Workspace
  search for "home city" received it, and retiring the memory left "I live in
  Boston" recallable as current. The binder now picks the sentence that
  states the Claim (polarity, Predicate word, subject or memory cue; ranked by
  Predicate words, first person, statement, cue, recency) and otherwise
  records Evie-proposed; legacy narrowing uses the same selection.
- Number words bound anywhere ("Read this one article…" gave `kids=1`;
  "No one told me. My floor is 4." gave `floor=1`), booleans bound on topic
  words ("Read this article about peanut allergy treatments…"), and negation
  was ignored ("I'm not allergic to peanuts.", "I don't live in Boston
  anymore."). All are Evie-proposed now; a denied Claim or false boolean
  binds only to a negated clause.
- Operation history (`memory_inspect_object`, web and REPL inspection)
  showed Global request and source text to Workspace readers, and a Global
  Entity's history showed one Workspace's words to another. Quoted evidence
  in operation JSON now follows the Source rule for the reader.
- `InspectMemoryEvidence` reported every Evie-proposed receipt unavailable,
  so the UI's "Evie proposed this value" text never rendered; it is now
  available with the label and no quote.

Tests: the probe messages are table cases in
`semantic_source_binding_test.go`; `memory_authority_test.go` runs the
therapist probe through a real Global remember, Workspace search and retire,
and a Workspace `memory_inspect_object` of a retired Global memory;
`semantic_operation_history_test.go` covers operation history for Claim,
Source Link and Entity inspection and Evie-proposed receipt inspection. The
opt-in compiler still accepts whole-message support as `owner_statement`
without a value check (see the decision).

Both tiers were re-run on this change: both reports are identical to the
baseline (the ratchet passes without re-recording), and the frozen 24-case
lexical workloads are unchanged (held-out 22/28 automatic, 26/28 deeper, 19
non-gold of 48; development 23–24/27, the recorded dev11 random-ID tie, and
25/27). Every corpus remember
("Remember that my…" or "Remember that I…") and both corrections state their
value in an un-negated first-person clause, so their sources are unchanged. Of the lexical workloads' 62 accepted records the
same two held-out records as above are the only ones that are not
whole-message owner statements.

## Final verification pass: recall relevance and newer statements (M1, M3)

The final review pass found four problems in the Stage 12 and 13 rules, each
reproduced with probes before fixing:

1. The excerpt floor's two-word rule dropped the only answer to ordinary
   questions, because filler ("need", "know", "again", "remind", "ok so")
   counted as content words.
2. "got it, thanks", "that's it, thanks", "love it" and "ok do it" counted as
   follow-ups (because of "it") and revived earlier topics, whose words then
   pulled in other sessions' messages; "and when was it?" took the two
   *oldest* earlier topics on a tie.
3. The newer-statement rule linked "I left my umbrella in Boston." and "My
   sister moved to Boston now." to `home city: Boston`, and, being newer,
   they took both companion slots from the real update; the restatement rule
   flagged third-party sentences against a retired owner Claim.
4. The floor counted every occurrence of every request word, one query each:
   at 40,000 messages a follow-up's search passed the 500 ms read deadline.

The rules are recorded as amendments to the 2026-10-01 entries of
[the retrieval decisions](memory-stage-5-retrieval.decisions.md). In short:
conversational filler is not a content word; an acknowledgement or command
with "it" and no question is low content; tied earlier topics resolve to the
most recent; a request whose words history all knows may be explained by its
uniquely rarest word alone if that word is in at most 3 messages; a word in
256 or more messages is common and counting stops there; a change cue must
govern the saved value or Predicate word in the same clause and the owner
must speak in that clause, for newer statements and restatements alike;
companions rank by strength before recency.

The review proposed counting only words that occur in history toward the
three-word threshold (with request words mostly unknown to history still
needing two matches, to keep the ORM probe safe). That was measured and not
adopted: on the default tier it raised automatic unwanted items from 5 to 10
and private items from 0 to 3, failing the privacy target (two private "low
energy" messages for "What's the energy rating of the dishwasher?"). Filler
removal and the strong single match recover four of the reviewer's five
questions without it. The strong single match itself first allowed a word
tied for rarest; that let "...I didn't say anything" answer "What did the
doctor say about my ferritin?" (a Stage 12 test), so the word must be
uniquely rarest.

**Corpus v3, report v4.** `memory-scale-replay-v3` (seed unchanged) adds the
reviewer's probes: five one-word-answer needles and their questions (new
family `one_word_answer`: "When do I need to renew my passport?" / "My
passport expires in March 2029.", "Do you know where I parked the car?",
"What's my sister's birthday again?", "Remind me which vet we use for the
cat", "ok so what's my wifi password"); four low-content probes "got it,
thanks", "that's it, thanks", "love it", "ok do it", each after a prelude;
the follow-up "and when was it?" after three topics (Lisbon last); a privacy
probe "What's the energy rating of the dishwasher?"; "Big news: I moved to
Chicago last month, Boston is behind me." followed by "I left my umbrella in
Boston." and "My sister moved to Boston now." (a link check and two
over-link checks on `home_city`); and "My friend Sam says his favorite coffee
is Blue Bottle." and "The Blue Bottle coffee shop on Main Street closed
today." (over-flag checks on `coffee` and `blue_bottle`). Report version 4
adds the target `M1.one_word_answers_recalled`. "Before" is the Stage 14 code
on corpus v3; the v2 numbers above are not directly comparable.

Default tier, before → after (unchanged cells show one value):

| Path | Family | Items | Unwanted | Private | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 70 → 68 | 15 → 5 | 0 | 0.7857 → 0.9265 | 23/33 → 28/33 |
| automatic | one-word answer | 3 → 9 | 0 | 0 | 1.0000 | 0/5 → 4/5 |
| automatic | low-content | 8 → 0 | 8 → 0 | 0 | 0.0000 → n/a | n/a |
| automatic | follow-up | 6 | 2 → 1 | 0 | 0.6667 → 0.8333 | 1/3 → 2/3 |
| automatic | stale | 34 | 1 → 0 | 0 | 0.9706 → 1.0000 | 11/12 |
| automatic | privacy | 4 | 2 | 0 | 0.5000 | n/a |
| automatic | unrelated | 0 | 0 | 0 | n/a | n/a |
| automatic | relevant | 15 | 2 | 0 | 0.8667 | 11/11 |
| memory_search | all | 21 → 20 | 2 → 0 | 0 | 0.9048 → 1.0000 | 13/13 |
| memory_search_conversations | all | 115 | 20 | 1 | 0.8261 | 10/12 |

Large tier, before → after:

| Path | Family | Items | Unwanted | Private | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 101 → 93 | 29 → 17 | 8 → 6 | 0.7129 → 0.8172 | 28/33 → 30/33 |
| automatic | one-word answer | 10 | 2 → 0 | 2 → 0 | 0.8000 → 1.0000 | 3/5 → 4/5 |
| automatic | low-content | 8 → 0 | 8 → 0 | 0 | 0.0000 → n/a | n/a |
| automatic | follow-up | 6 | 3 → 2 | 1 | 0.5000 → 0.6667 | 1/3 → 2/3 |
| automatic | stale | 37 | 4 → 3 | 2 | 0.8919 → 0.9189 | 11/12 |
| automatic | privacy | 6 | 4 | 2 | 0.3333 | n/a |
| automatic | unrelated | 6 | 6 | 0 | 0.0000 | n/a |
| memory_search | all | 23 → 22 | 3 → 1 | 0 | 0.8696 → 0.9545 | 13/13 |
| memory_search_conversations | all | 205 | 28 | 4 | 0.8634 | 36/36 |

Stale-fact checks (both tiers): 11 misses of 54 → 2 of 54, 10/10 controls, no
scope leaks.

| Target | Default before → after | Large before → after |
| --- | --- | --- |
| M1: low-content follow-ups inject nothing unwanted | no → yes (8 → 0 items) | no → yes (8 → 0 items) |
| M1: privacy probes inject no private item | yes (0 private) | no (2 private, unchanged) |
| M1: one-word answers recalled | no → no (0/5 → 4/5) | no → no (3/5 → 4/5) |
| M2: no over-flagging | no → yes (4 → 0 of 15) | no → yes (4 → 0 of 15) |
| M3: different wording detected | no → no (4 → 2 of 8) | no → no (4 → 2 of 8) |
| M3: no over-linking | no → yes (3 → 0 of 12) | no → yes (3 → 0 of 12) |
| M2 corrected, M2 restated, M4 drift, M7 dense | yes | yes |

Every per-probe change in both tiers is one of the targeted probes: the four
acknowledgements now inject nothing (eight unwanted items, two each); four
one-word answers are recalled; "and when was it?" now finds the Lisbon
booking; `home_city` on the automatic and `memory_search` paths now delivers
"Big news: ... Boston is behind me." linked, and neither distractor. Every
other probe delivered exactly the same items, including all other relevant,
privacy, unrelated, stale and control probes and all 24 dense targets.

Remaining misses and known limits:

- *"What's my sister's birthday again?"* misses its planted answer "My sister
  Lena was born on June 13, 1994." in both tiers. After filler removal it is
  the two-word request [sister, birthday]; `sister` is in 13 Global messages
  and `birthday` in 7 (69 and 57 in the large tier), so only `birthday` is
  distinctive (Stage 12's rarer-half rule, kept on purpose: "Book the car
  service" must not answer "what dates did I book?"). The saved Claim
  `sister's birthday: June 13` is delivered.
- *"and when was it?"* still takes two earlier topics, so the second most
  recent (tyres) contributes one unwanted item.
- *The large tier's privacy target* is unmet only through the new energy
  probe, and identically before and after: its two private items are dense
  hits, because the concept-hash test embedder scores "energy rating" close
  to "low energy". The lexical floor does not gate dense hits (Stage 12), and
  the default (lexical) tier injects nothing for it.
- *The strong single match* trades some privacy margin for recall: a request
  whose words history all knows, whose rarest word is shared with one
  private message in at most 3 messages and nowhere else ("What were the
  results of the run today?" against a blood "test results" message), can
  now inject it. Ties, unknown words and the 3-message ceiling bound it.
- *M3* still misses "I moved to Chicago last month and I'm still unpacking
  boxes." (no shared word), as recorded in Stage 13. The governing-cue test
  also drops "I quit my job at Initech" against `employer: Initech` ("job"
  sits between the cue and the value, as "umbrella" does); it is not in the
  corpus.

**Timing.** `TestRelevanceFrequenciesStayFastAtScale` builds 20,000 Global
messages and checks that counts saturate at 256, are exact below it, and stay
well inside the deadline (fastest of three runs: counting under 50 ms, search
under 250 ms). Measured on this machine, fastest of three:

| Messages | Counting before → after | Whole search before → after |
| ---: | ---: | ---: |
| 20,000 | 170 ms → 9 ms | about 265 ms → 104 ms |
| 40,000 | 338 ms → 10 ms | about 520 ms (past the deadline) → 195 ms |

The remaining search time is the candidate read itself (BM25 over every
message matching any request word), which this pass does not change.

**Frozen 24-case lexical workloads** (`EVIE_MEMORY_INTEGRATED_LEXICAL=1`):
held-out 22/28 automatic, 26/28 with deeper search, 19 non-gold of 48
first-dispatch items, before and after; development 23–24/27 automatic and
25/27 with deeper search before and after. The development swing is the
dev11 random-ID tie recorded in Stage 12: over 24 runs each it missed 13
times before and 14 times after.

## Confirmation review: who is speaking (M1, M2, M3, M5)

A confirmation review of the final verification pass reproduced six defects
with probes, all fixed here (rules in the 2026-10-01 entries of
[the memory decisions](memory.decisions.md) and
[the retrieval decisions](memory-stage-5-retrieval.decisions.md), each
amended "confirmation review"):

1. *Binder tie-break (high).* "I live in Boston. My therapist in Boston says
   the panic attacks are getting worse." cited the therapist sentence (the
   latest qualifying one; a possessive "my" counted as the owner), a
   Workspace search for "home city" received it, and after retirement "I live
   in Boston." came back unlabelled. The oncologist, divorce-lawyer and
   "My manager at Initech put me on a performance plan" variants did the
   same, and a retire request "Forget that I live in Boston. My therapist in
   Boston says..." leaked through operation history.
2. *Strong single match (medium-high).* Requests that were not about the
   owner injected private messages through one rare shared word
   ("Explain technical debt to the new engineers." → credit card debt; an
   anxiety poem → a therapist message; a custody playlist → a divorce
   lawyer), and filler removal turned "Do I need to use a VPN for the bank?"
   into [vpn, bank], which matched the overdrawn-account message.
3. *Newer statements (medium).* Any content word between the cue and the
   value blocked the link ("I no longer live in Boston.", "I quit my job at
   Initech"), while someone else's update linked ("My ex left Boston.", "My
   sister said, Boston is no longer an option.", "My dad dropped
   Verizon.") and, being newer, took both companion slots.
4. *Plain answers Evie-proposed (medium).* A later negated clause ("I live
   in Boston and I don't plan to move.", "Alex works at Initech and doesn't
   like it.") and terse answers ("Boston.", "Teal, please.", "No, it's
   Chicago.", "Chicago, not Boston.") lost owner authority.
5. *Others' words as the owner's (medium-low).* "My sister is vegetarian.",
   "My friend said 'I live in Boston'.", "If I lived in Boston, I'd take the
   T.", "I wish I lived in Boston.", "I left Boston for good.", "I used to
   live in Boston." and the double negation "It's not that I don't live in
   Boston." (denied) kept owner authority.
6. *Review reason (medium-low).* A Global compiler review edit's free-text
   reason ("Keep it; my oncologist approved cocoa during chemo.") showed
   verbatim to a project reader in operation history.

One deterministic clause and subject analysis
(`internal/eviedb/retrieval_wording_clauses.go`) now serves both the source
binder and the M2/M3 wording rules, so they agree: clause boundaries
(including "and"/"but" before a new subject or verb, quotation marks and
reporting verbs), reported speech, conditional, wish, future and question
markers, per-clause negation scope, change words, and the clause's subject
(the owner; someone else such as "my sister", "my therapist", "our team",
"the doctor", "my dad's"; the Claim's own words; or nobody in particular).
Every rule is a heuristic and is designed to fail in the safe direction:
unsure authority is Evie-proposed (approval is still required; only the label
changes), an unsure newer statement is not linked, automatic recall injects
nothing on one shared word unless the request recalls something about the
owner (the model can still search), the binder prefers the shortest
qualifying span before the latest, and operation history blanks free text
that cites no value outside the scope it was written in.

**Corpus v4.** `memory-scale-replay-v4` (seed and report version 4
unchanged; every addition is a fixed scenario session, so the generated
filler history is unchanged) adds:

- *Strong-match privacy probes* (family `privacy`): "Explain technical debt
  to the new engineers.", "Write a short poem about anxiety for my
  newsletter.", "Suggest a playlist for a custody handover drive.", "What
  were the results of the run today?" and "Do I need to use a VPN for the
  bank?", with ordinary messages that make every other word of each request
  known to history (topics tolerated by the probe) and private messages
  sharing its rarest word: credit card debt and an overdrawn bank account
  (marked private, topic `money`), a therapist (health), a divorce lawyer
  (relationship); the run probe uses the existing blood-test needle.
- *M3 owner and third-party updates:* a new Claim `cloud storage: Dropbox`
  ("Remember that I use Dropbox for cloud storage.") with the update "I no
  longer use Dropbox." (a link check, probe `storage`); "My ex left Boston."
  and "My sister said, Boston is no longer an option." after the real
  `home_city` update (over-link checks); "My dad dropped Verizon." after the
  carrier update (an over-link check).
- *M2:* "My dad's favorite coffee shop is Blue Bottle." (over-flag checks on
  `coffee` and `blue_bottle`).

The binder cases (defects 1, 4, 5) and the review reason (defect 6) cannot
be scored by this instrument, which records item keys, links and status, not
source authority or source text (see Stage 14). They are table and
end-to-end tests: `TestOwnerSpanBindingFollowsTheClauseSubject` and
`TestOwnerSpanBindingForEntityClaimsFollowsTheClauseSubject`
(`semantic_source_binding_test.go`), `TestWordingRulesFollowTheClauseSubject`
(`retrieval_wording_test.go`), `TestRelevanceFloorThresholds`
(`retrieval_relevance_test.go`), `TestOperationHistoryNarrowsARequestToTheOwnersSentence`
and `TestOperationHistoryBlanksFreeTextOutsideItsScope`
(`semantic_operation_history_test.go`),
`TestCompilerReviewReasonIsNotShownOutsideItsScope`
(`candidate_review_reason_narrowing_test.go`), and through real agent turns
`TestRememberIgnoresLaterSentencesAboutSomeoneElse` and
`TestTerseOwnerAnswersKeepOwnerAuthority` (`memory_authority_test.go`),
`TestNewerOwnerStatementFollowsTheClauseSubject` and the extended
`TestThirdPartyMentionOfRetiredValueIsNotFlagged`
(`retrieval_currency_test.go`), and
`TestAutomaticMemoryRecallStrongSingleMatchNeedsAPersonalRecall` and the
extended `TestAutomaticRecallPlanGatesEarlierTopics`
(`retrieval_automatic_relevance_test.go`). Each uses the reviewer's exact
messages and failed before the fix.

"Before" is the final-pass code (93bbe1a) on corpus v4; the v3 numbers above
are not directly comparable (corpus v4 adds 1 automatic required item and
14 stale checks). Default tier, before → after (unchanged cells show one
value):

| Path | Family | Items | Unwanted | Private | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 79 → 76 | 14 → 8 | 3 → 0 | 0.8228 → 0.8947 | 29/34 |
| automatic | privacy | 13 → 9 | 9 → 4 | 3 → 0 | 0.3077 → 0.5556 | n/a |
| automatic | stale | 36 → 37 | 1 → 0 | 0 | 0.9722 → 1.0000 | 12/13 |
| automatic | one-word answer | 9 | 0 | 0 | 1.0000 | 4/5 |
| automatic | low-content | 0 | 0 | 0 | n/a | n/a |
| automatic | follow-up | 6 | 2 | 0 | 0.6667 | 2/3 |
| automatic | relevant | 15 | 2 | 0 | 0.8667 | 11/11 |
| memory_search | all | 23 → 22 | 2 → 0 | 0 | 0.9130 → 1.0000 | 14/14 |
| memory_search_conversations | all | 119 | 22 | 2 | 0.8151 | 10/12 |

Large tier, before → after:

| Path | Family | Items | Unwanted | Private | Precision | Recall |
| --- | --- | ---: | ---: | ---: | ---: | ---: |
| automatic | all | 107 → 108 | 26 → 22 | 10 → 8 | 0.7570 → 0.7963 | 31/34 |
| automatic | privacy | 18 | 12 → 9 | 6 → 4 | 0.3333 → 0.5000 | n/a |
| automatic | stale | 39 → 40 | 4 → 3 | 2 | 0.8974 → 0.9250 | 12/13 |
| automatic | one-word answer | 10 | 0 | 0 | 1.0000 | 4/5 |
| automatic | low-content | 0 | 0 | 0 | n/a | n/a |
| memory_search | all | 25 → 24 | 3 → 1 | 0 | 0.8800 → 0.9583 | 14/14 |
| memory_search_conversations | all | 209 | 30 | 5 | 0.8565 | 36/36 |

Stale-fact checks (both tiers): 16 misses of 68 → 2 of 68 (the two
`m.moved` checks, which share no word with the Claim), 10/10 controls, no
scope leaks.

| Target | Default before → after | Large before → after |
| --- | --- | --- |
| M1: low-content follow-ups inject nothing unwanted | yes | yes |
| M1: privacy probes inject no private item | no → yes (3 → 0) | no → no (6 → 4) |
| M1: one-word answers recalled | no (4/5, unchanged) | no (4/5, unchanged) |
| M2: no over-flagging | no → yes (4 → 0 of 21) | no → yes (4 → 0 of 21) |
| M3: different wording detected | no → no (7 → 2 of 10) | no → no (7 → 2 of 10) |
| M3: no over-linking | no → yes (5 → 0 of 18) | no → yes (5 → 0 of 18) |
| M2 corrected, M2 restated, M4 drift, M7 dense | yes | yes |

Every per-probe change in both tiers is a targeted probe: the debt, anxiety
and custody privacy probes no longer inject their private message (the run
probe did not leak in this corpus before either); the VPN probe recalls "The
VPN client needs an update." instead of the bank Claims' source messages;
`home_city` (automatic and `memory_search`) delivers the real update "Big
news: ... Boston is behind me." linked instead of the ex and sister
sentences; `carrier` keeps "I finally dropped Verizon..." linked and no
longer links "My dad dropped Verizon."; `storage` links "I no longer use
Dropbox."; and the dad's coffee shop is no longer flagged. All other probes,
including the four recalled one-word answers (passport, parked car, cat's
vet, wifi password), deliver exactly the same items. Relative to the
committed v3 report, the original 33 automatic required items are still
28/33 recalled; the 34th is the new `storage` Claim.

The large tier's privacy target stays unmet only through dense hits, which
the lexical floor does not gate (Stage 12): the concept-hash test embedder
scores "energy rating" close to "low energy" (two items, unchanged) and
"short poem" close to the private "I'm feeling a bit lonely this week. Keep
it short." (two items; the second replaced the lexical therapist leak).

**Frozen 24-case lexical workloads** (`EVIE_MEMORY_INTEGRATED_LEXICAL=1`),
before and after, with identical per-case lines: held-out 22/28 automatic,
26/28 with deeper search, 19 non-gold of 48 first-dispatch items;
development 24/27 automatic (within the recorded dev11 random-ID tie of
23–24), 25/27 with deeper search, 19 non-gold of 51. The workloads' 62
accepted records bind exactly as before (checked by binding every record's
text with the old and new rules).

Remaining misses and known limits (these are heuristics; the residual cases
below were probed and are recorded, not fixed):

- *Binder.* "I lived in Boston until 2019." and "When I lived in Boston,
  ..." keep owner authority (past tense is not detected, and "until" is how
  an owner states a Claim with an ended valid time, as the frozen workloads
  do); "I used to live in Boston." is Evie-proposed even for a Claim whose
  valid time has ended (the binder does not see valid time); "I have one
  kid... wait, no, I have three kids." cites the first sentence for 1;
  "Alex doesn't work at Globex; he works at Initech." is Evie-proposed
  (no pronoun resolution); first-person mentions such as "My car is in
  Boston." or "My plumber lives in Boston." (a person outside the closed
  list) still qualify; a memory cue still qualifies a topic mention
  ("Remember this article about Boston.", "Save this page: ..."); spans are
  whole sentences, so another clause of the stating sentence is quoted with
  it ("Restore my favorite color navy, my rehab counselor likes it.").
- *M3.* "We left Boston for a week of vacation." links (a trip and a move
  share their words); "My home is Chicago now." shares only one of the two
  `home city` words and does not link; "I moved to Chicago last month and
  I'm still unpacking boxes." shares no word; relation words are a closed
  list ("I no longer sing in the Riverside choir" does not link).
- *M1.* "What's my sister's birthday again?" still misses its needle (the
  final pass's rarer-half reason). A personal recall request can still be
  explained by a rare word it shares with an unrelated private message.
  Dense excerpts are not gated by the lexical floor.

## Decisions and spec relationship

- **Synthetic, not derived.** `memory.spec.md` Stage 9 asks for fixtures
  derived from real Evie tasks. This stage was constrained never to read real
  history, so it generates history shaped like a long-running assistant
  instead. It pulls forward Stage 9's versioned corpus, fixed configuration,
  metric formulas and comparable before/after report, but not its
  model-backed `-tags eval` runs, write-precision, or entity-resolution
  metrics.
- **Development set.** This corpus is visible to the Stage 12 and 13
  implementers, so it is a tuning instrument, not a held-out gate. Stage 12
  must still show no recall loss on the existing 24-case held-out assessment.
- **Labels follow the plan.** The low-content rule comes from the M1 default;
  the stale rules from the Stage 13 acceptance wording. When Stage 12 or 13
  defines its actual marking (for example a historical label or a truncation
  field), the scorer's *presented as current* rule and the M7 target must be
  taught that signal in the same change. Stage 12 taught the M7 target the
  `dense_scan_budget` gap; Stage 13 taught the stale rules the
  `historical_claims` link.

## Limitations

- Projects stand in for Workspaces. A Workspace session needs a composition
  receipt from the preset manager; projects follow the same conversation-scope
  rule (spec decision 5) and exercise the same isolation.
- M7 is measured on conversation vectors. Claim vectors use the same paged
  scan and budget; exceeding the old bound would need over 4,096 approved
  Claims, which is neither realistic nor fast, so the shared scan is covered
  by focused tests instead.
- The fake embedder is a deterministic concept-hash proxy, not a semantic
  model; dense precision numbers describe the mechanism, not model quality.
- Probes use scripted queries and a scripted provider. Answer quality,
  clarification and citation are out of scope here.
- Exact numbers depend on the seed and generator; any generator change bumps
  `ScaleCorpusVersion` and re-records both baselines.
