# Memory evaluation at scale (M8)

Stage 11 of the 2026-09-30 harness review. This is a measuring instrument, not
a fix: it records how today's recall behaves at realistic history size so that
Stage 12 (recall relevance, M1/M7) and Stage 13 (currency and conflicts,
M2/M3/M4) are measured rather than guessed. Every unmet target below is
current behavior, recorded as such; none is accepted as correct. Stage 12's
and Stage 13's before/after results follow the Stage 11 baseline; the
committed baselines are the Stage 13 report on corpus v2.

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

The default tier runs in the ordinary suite (about 7 s here) and skips under
`-short`. The large tier (about 60 s) grows Global history past the former
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
