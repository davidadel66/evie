# Memory evaluation at scale (M8)

Stage 11 of the 2026-09-30 harness review. This is a measuring instrument, not
a fix: it records how today's recall behaves at realistic history size so that
Stage 12 (recall relevance, M1/M7) and Stage 13 (currency and conflicts,
M2/M3/M4) are measured rather than guessed. Every unmet target below is
current behavior, recorded as such; none is accepted as correct.

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
Global needle's exact words to test scope isolation.

Each probe runs in a fresh session on its own copy of the built database.
Messages are indexed when they are appended, so a shared database would let
one probe's question become another probe's evidence. Production event and
Claim IDs are random UUIDs, and the dense scan reads in ID order; the driver
seeds the UUID source so a given code version reproduces the same IDs. This
changes no production behavior. The default tier's report was checked to be
identical under a different UUID seed, so only the large tier's dense
reachability depends on ID order. Per-probe item lists are sorted because rank
order is not measured.

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
- **Stale.** An item is *presented as current* when it has current intent,
  active status and current status, and no relation or conflict marking.
  Correction and retirement checks miss when a stale item is presented as
  current. Contradiction and drift checks miss unless the two items are
  linked by a relation or conflict warning. Controls use wording the current
  code already handles; a failing control on a tool path fails the test,
  because then the instrument is wrong.

## Running it

```sh
go test ./internal/memoryeval/ -run TestMemoryScaleReplayDefaultTier -v
EVIE_MEMORY_SCALE_EVAL=large go test ./internal/memoryeval/ -run TestMemoryScaleReplayLargeTier -v -timeout 30m
```

The default tier runs in the ordinary suite (about 7 s here) and skips under
`-short`. The large tier (about 60 s) grows Global history past the
4,096-vector dense scan bound and enables a deterministic concept-hash
embedder served on loopback, like the existing dense acceptance tests.

The baseline is a ratchet: any change in the report, better or worse, fails
until the baseline is re-recorded in the same change with
`EVIE_MEMORY_SCALE_UPDATE=1`. The baseline diff is the before/after report.
Scope leaks or unmapped items always fail.

## Baseline (corpus `memory-scale-replay-v1`, seed 20261001)

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
  taught that signal in the same change.

## Limitations

- Projects stand in for Workspaces. A Workspace session needs a composition
  receipt from the preset manager; projects follow the same conversation-scope
  rule (spec decision 5) and exercise the same isolation.
- M7 is measured on conversation vectors. Claim vectors share the same
  4,096 bound, but exceeding it needs over 4,096 approved Claims, which is
  neither realistic nor fast.
- The fake embedder is a deterministic concept-hash proxy, not a semantic
  model; dense precision numbers describe the mechanism, not model quality.
- Probes use scripted queries and a scripted provider. Answer quality,
  clarification and citation are out of scope here.
- Exact numbers depend on the seed and generator; any generator change bumps
  `ScaleCorpusVersion` and re-records both baselines.
