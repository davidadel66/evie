# Integrated retrieval pilot (#167)

Development v4 passes all **1,077 declared gates** after the cross-scope refresh
correction. Production automatic-plus-deeper and oracle each pass all 24 semantic
cases, with complete required evidence, component coverage, grounding and citation
accuracy. All 2,880 local turns, 24 index cases, 60 operating samples and mapped
Go/UI checks pass. Across all six reader conditions, 143 of 144 semantic cases
pass; the automatic-only historical comparison failure remains visible below.
Release readiness is **false** until the separate frozen held-out assessment in
#168. These known development questions are not release evidence.

The parent is #154. This slice depends on committed and verified #159, #161,
#162, #163 and #166. The unchanged development workload/rubric live in
`../fixtures/memory-stage5-integrated/development/v1/`; the source-sufficiency
corrections and numerically unchanged gates are recorded in `development/v3/`,
and the current boundary matrix/results in `development/v4/`. Earlier v1–v3
results and their limitations remain historical records below; none is replaced
by the current pass. Raw preservation is organized under
`../fixtures/memory-stage5-integrated/runs/`.

## Comparison surface

`WithModelMemoryRetrieval(false)` removes `memory_search`,
`memory_search_conversations`, and `memory_expand_conversation` from both the
model schemas before composition and the actual executable Toolset. Automatic
Recall still discovers only the originally granted capabilities and retains its
normal source, scope and remote-memory checks. The default enables the existing
model reads. The option never adds a capability. Context inspection derives the
same narrowed surface, and legacy extra-tool resolution finishes before the
turn derives that surface.

The primary complete-turn regression uses real SQLite, an accepted preference,
a scripted provider which tries all three omitted reads, and an unrelated
allowed capability. It requires automatic evidence on the first request, no
execution of omitted reads, preserved unrelated dispatch, and exact durable
request hash/size agreement before every provider call. RED failed because
`memory_search` was still advertised (0.487 s); GREEN passed in 0.339 s.
`go test ./internal/agent -run '^TestAutomaticMemoryRecallCanNarrowModelReadsBeforeCompositionAndDispatch$|^TestModelMemoryRetrievalDefaultsToEnabledAndKeepsLegacyTurnResolution$' -count=1`
passed in 0.575 s, including default, explicitly enabled and legacy resolution
regressions. These scripted results verify the comparison boundary; they do not
measure reader quality.

## Preliminary two-axis review

The user selected `f27546d` as the fixed point for the final review. A preliminary
parallel review covered `git diff f27546d...89feb14` on an immutable export of the
first eleven issue commits. #167 and #168 were explicitly outside that interim
review. The final review will cover all thirteen issue commits.

### Standards

No confirmed documented-standard violation was found. One nonblocking judgement
call was possible Duplicated Code in the retrieval, conversation and expansion
serialization/budget loops. A narrow shared helper could reduce future drift;
no current observable failure was established, so this did not require an
unrelated refactor. Repository rules override the generic smell baseline.

### Spec

One P2 finding concerned #162 current-view refresh. Only newly created conflicting
Claims triggered refresh, so restoring an older conflicting Claim or its Source
Link could leave a continued turn unaware of the disagreement. A newly appended
same-area owner statement also lacked an invalidation check. #162 requires
refresh when new information changes what is needed; #159 requires active
conflicts and relevant newer owner excerpts with their authority and timestamps.
The owning fix and complete-turn regressions belong in #162 while explicit
historical and knowledge-time pins remain stable.

Interim counts: Standards 0 hard findings and 1 nonblocking heuristic; Spec 1 P2
finding. The review is not a substitute for deterministic verification or the
final all-issue review.

## Pre-pilot verification and integrity checks

The #162 finding above is resolved in the owning #162 commit: current views
refresh for restored conflicting Claims/Source Links and relevant new owner
statements, while explicit historical/knowledge-time pins stay stable. The
isolated pre-dense owner-commit check and retained v3 investigation measurement
are recorded in the #162 implementation document.

The first integrated repository verification attempt caught a historical index
diagnostic query exceeding the public 32-term bound. The shared diagnostic query
now takes the original question prefix through at most 32 Unicode letter/digit
runs and 1024 UTF-8 bytes, retaining intervening punctuation and exact IDs. It
changes neither the full user message nor the model's production search queries.
The complete-turn smoke and enabled/historical/unavailable index tracers passed
in 7.111 s; the opt-in index measurement skipped because no freeze existed.

`./scripts/verify-change.sh` then passed (log
`/tmp/evie-memory-stage5/167-verify-pre-pilot-v2.log`, exit 0): full Go tests
(`internal/agent` 23.529 s; other tested packages cached), full Go vet, UI lint,
TypeScript/Vite build, staged and unstaged whitespace checks. Lint retained five
existing Fast Refresh warnings, and Vite retained its existing >500 kB chunk
warning. The prior failed log remains available; it is not presented as a pass.

The versioned operating matrix's 86 distinct Go tests all ran and passed, with
zero missing, failed or skipped test events. The exact combined `go test -json`
command is retained in `/tmp/evie-memory-stage5/167-mapped-go-v1.command.json`;
package times were agent 7.941 s, localembedding 0.568 s and web 1.774 s. Its seven
UI files produced 23 passing tests, with no failed or pending tests; the exact
Vitest command and structured results are retained beside that log. These are
deterministic checks, not local performance or reader-quality measurements.

The source auditor was checked against retained actual #159/#161 original-source
formats, including whole-source/UTF-8 range locators, prefixed/bare hashes,
accepted alias support, nanosecond timestamps and exact source partitions. The
provider artifact auditor was checked with copies of actual #161 wire requests
and persisted receipts. The clean case passes; altered receipt status, a missing
final request, malformed JSON and a malformed memory projection all produce
explicit failure records and no final source credit. These temporary tamper
checks validate the measurement apparatus and add no reader-quality samples.

The numerical gates in `../fixtures/memory-stage5-integrated/adopted-gates-v1.json`
were adopted before any integrated reader output. The evaluation, index and
operating procedures, assessment schema and scoring programs must join the
immutable executable/input freeze before measurements start. No release-held-out
corpus has been created or inspected at this point.

The freeze runner's file naming, hash propagation, archive checks, explicit Go
worker settings and failure metadata were reviewed separately. Temporary controls
confirmed that a nonexistent executable leaves a start record and an explicit
failed-launch record, and that archive corruption changes its verified digest.
The semantic grader's ten synthetic controls passed in 0.157 s, including a
complete synthetic 144-assessment cohort, preserved missing-case denominators,
null grounding, exact quote/citation/context validation and arithmetic tampering.
These controls create no actual pilot answers and establish no measured quality.
The development resource report may mark its own gates passed, but release
readiness remains false until the fresh held-out assessment passes.

## Development freeze v1

The immutable freeze completed at `2026-09-11T03:41:06.487126Z`, before any
integrated reader generation. SHA256:

- Freeze: `ec6c2a6cc811f4d03ecf8e67f966fe3d74b7ae2ee423f93a5e9375d177f86e3b`.
- Executable: `fcb0e65893cc29136c842e22a8ef5e9d63d8ee1b48a27f07ea4dea3787275124`.
- Workload: `2e1aff23f8426b45b6a1cc3301c769026ecb84cc55a9d4f03ba922eff333d562`.
- Adopted gates: `06aaf6f4bff1486a78cef71e88dffeec54ae529f23c322fd8ec5d1bc04c928fd`.

The source archive covers 391 repository files needed to compile the test
executable. The input archive covers 24 closed canonical SQLite databases and
24 independently bound public source maps. Metadata-only production preflight
passed in 0.41 s, and all 24 public fixture preparations passed. Preparation
indexed sources with the selected local model; it generated no reader answers.
The Go worker is pinned to GOMAXPROCS=11, GOGC=100, GOMEMLIMIT=off and an empty
GODEBUG. The exact runtime/model/profile and hardware are retained in the freeze.

`/tmp/evie-memory-stage5/integrated-development-v1-deterministic.json` binds the
pre-pilot checks to this freeze after rechecking every compiled input against the
verified checkout. The matrix specifically names 86 Go tests and 14 UI
assertions; all passed. The seven UI files also ran nine additional assertions,
for 23 passing UI tests overall. Final repository verification remains required
before handoff. The first measured development results follow.


## Development v1 results and diagnosed failures

The first local run retained all 2,880 attempted turns (24 cases × six conditions
× 20 repetitions), including 320 failures. Every failed turn reported the same
configured-context overflow: no legal automatic compaction could satisfy the
target. Sixteen case/condition cohorts failed: `dev05`, `dev06`, `dev07`, `dev08`,
`dev11`, `dev12`, `dev17` and `dev18`, each in `tool_only` and `automatic_deeper`.
The observer recorded no source/request boundary errors; accepted revisions and
all 48 frozen canonical input files remained unchanged. This is a failed local
pilot, not a pass with the errors removed from its denominator.

The failed turns' last observed receipts do not establish total runtime work
before a later request-composition failure. v1 recorded zero or an earlier
receipt total for that field; these values must not be read as measured full-turn
work. The resource audit rejects the incomplete/undercounted cohorts. The v2
capture makes final work explicitly null for failed turns and makes a cohort's
runtime quantiles unavailable unless all twenty final totals are known. The
separately summed observed Kernel search time remains a lower bound.

The first index run retained all 24 cases and failed eight. Seven had the same
context overflow (`dev05`, `dev06`, `dev07`, `dev08`, `dev11`, `dev12`, `dev15`).
The remaining case, `dev01_fresh_tea`, kept exact source IDs, hashes, locators,
lifecycle and order across reopen/rebuild but failed the unchanged exact
`paths` agreement: its first/reopened accepted query used lexical discovery while
the rebuilt query also used dense discovery. That first query's local model was
unavailable within the bounded query deadline. The optional-generator fallback
kept eligible lexical sources and reported incomplete coverage, as #166 requires;
it does not guarantee dense availability during every cold load.

A separately retained two-run diagnostic observed no loaded model before the
first run (exit 1) and the selected loaded model before the immediate second run
(exit 0). The first observation may have overlapped a short independent Go check,
so cold-start causation is supported, not proven. Neither diagnostic replaces the
original failed index measurement or establishes a complete passing cohort.

All sixty operating samples passed: twenty cancellation, twenty lease replacement
and twenty caller-deadline samples, each within the unchanged one-second tail
gate. Cancellation p50/p95/max were 0.787042/1.090667/1.367792 ms; lease replacement
0.455750/0.601750/16.845750 ms; caller deadline 4.022542/8.708166/12.660459 ms.
No late protected effects or invalidated original receipts were observed.

The v1 partial resource report has `complete: false`,
`all_required_gates_pass: false` and `release_ready: false`. Reader generation,
evidence quality and semantic assessment were deliberately not run because the
local/context prerequisite failed. Missing quality results are explicit missing
inputs, never fabricated zero-error reports. Exact raw traces, commands, source
and canonical-input archives, failed reports and supplemental diagnostics remain
part of the retained pilot evidence.

## Corrected development v2 procedure

The context defect is corrected and verified in the owning #162 commit; #166
remains the next independently preserved commit. At the v2 freeze those commits
were `3dd7d59` and `fbabb1f`; later #159 fix folding changes their branch hashes
without changing the preserved v2 source archive.
The complete-request fit uses the existing profile and compaction trigger,
retains whole eligible originals, and reports explicit exhaustion. The exact
original `dev05_clear_reference/tool_only/01` replay passed in 0.75 s with three
requests of 10,323, 19,184 and 18,710 bytes. That one-case diagnostic confirms the
original reproduction is fixed; it establishes no full-cohort quality gate.

Before any integrated reader output, v2 declares the measurement order as
mandatory canonical source preparation → index recovery → local turns → operating
samples → reader turns. Actual selected-model state is recorded before and after
preparation and immediately before each run. There is no extra warming query,
forced unload, retry selection or removal of failed samples. All 24 index cases
still require exact path/source/order agreement under the same deadlines and
resource thresholds. This comparison establishes recovery after required
indexing; it does not claim cold first-read dense reliability. The release
assessment must repeat the same procedure.

The development workload, reader profile, numerical gates, ranking configuration
and semantic rubric remain unchanged. v2 adds the three complete-request
headroom regressions to the deterministic operating matrix. A new immutable
source/script/input freeze is required for the corrected implementation and
measurement capture; every v1 failure remains retained separately. No release
held-out workload has yet been created or inspected.


The corrected source passed `./scripts/verify-change.sh` again (exit 0), from
`2026-09-11T04:18:42.095388Z` to `04:20:05.909270Z`; the agent suite took 35.944 s.
Full Go tests/vet, UI lint, TypeScript/Vite build and whitespace checks passed,
with the same five existing Fast Refresh warnings and >500 kB bundle warning.
The v2 matrix's 89 Go tests passed (agent 8.835 s, localembedding 0.343 s, web
1.735 s), and all 23 tests in its seven UI files passed. Fourteen UI assertions
are specifically required by the matrix. Exact commands, structured output,
observed timestamps and hashes are retained under
`integrated-development-v2-checks/`; all 393 compiled inputs remained unchanged.

A pre-output measurement review also found that a wrong additional citation to
an existing source could escape semantic-pass and aggregate citation accuracy.
The grader now includes every additional citation in the accuracy denominator
and rejects wrong citations even for honest-absence baselines; missing required
citations keep their original denominator. Fourteen format-only controls passed
in 0.171 s. These checks use explicit synthetic apparatus examples, not reader
answers or invented pilot measurements. Release sealing independently replays
the frozen resource auditor against the retained inputs and requires exact
report agreement before any held-out source preparation.


The development-v2 freeze completed at `2026-09-11T04:26:38.186911+00:00`, before reader
generation. Its SHA256 is
`d1ee307d4afd81270a8cbceaeaf0a61269b9a77a18a240beb6478e5fa90c1080`;
the executable SHA256 is `f80a23aeb1c060a851f19962b2373abec547322247f764f2d423f50e22a41797`. The
workload and numerical-gate hashes are identical to v1. The v2 matrix SHA256 is
`1ef2f25d040fb33e599c3b8b6554a2cabb4ffe97286beb77a1bca724b674c6c6`. This freeze records the 393
verified compiled inputs, unchanged reader profile, exact selected model/runtime,
source and canonical-input archives, all scoring programs and the recorded
measurement order.

The first v2 index run passed all 24 cases (exit 0), from
`2026-09-11T04:26:38.378453Z` to `04:26:49.705033Z`. The selected MiniLM manifest
was actually loaded immediately before this run. Exact source/path ordering
survived restart and rebuild; all later local/resource and semantic gates remain
separate requirements. No failed first attempt was replaced.


The full v2 local run passed all 2,880 turns in 144 twenty-sample cohorts, with
zero failed turns, zero unknown final work totals, unchanged accepted revisions
and unchanged canonical inputs (exit 0;
`2026-09-11T04:26:49.868081Z` to `04:28:30.646481Z`). The largest cohort p95 was
41.194042 ms to the first dispatch and 58.753165 ms of final runtime retrieval
work; their frozen gates are 1,500 and 3,000 ms. The largest whole-turn cohort
p95 was 90.283625 ms. These are scripted local resource measurements, not reader
answer quality or a causal speedup claim against v1.

All sixty v2 operating samples also passed (exit 0;
`2026-09-11T04:28:30.881959+00:00` to `2026-09-11T04:29:16.337157+00:00`).
Cancellation p50/p95/max were 0.786125/1.002750/1.019833 ms; lease replacement
0.445667/0.540750/0.585625 ms; caller deadline
4.794875/8.191333/9.367875 ms. Every sample met its unchanged one-second tail
gate with no late protected effects. Actual reader generation began only after
these successful prerequisite executions, at `2026-09-11T04:29:16.569264Z`.
Its complete evidence audit, manual semantic assessment and final resource
aggregation remain pending.


## Completed v2 reader and assessment: readiness blocked

All 144 real production reader turns completed in 181 actual provider calls and
181 persisted dispatches, with zero execution errors, from
`2026-09-11T04:29:16.569264Z` to `04:37:31.684761Z`. The frozen provider/source
audit retained all request bodies, source receipts and answers and recorded zero
boundary violations. Under that v2 audit, automatic-plus-deeper initial/final/union
source recall was 24/27, 25/27 and 25/27; complete final support was 17/19 and
unwanted evidence was 11/46. These are the retained v2 calculations, including the
scoring limitations below; they are not a corrected retrospective pass.

Three explicitly identified Codex agents manually assessed all 144 answers,
48 each. This is agent assessment, not independent human review. They recorded
136 semantic passes and eight failures, retaining missing baseline components as
zero coverage even when abstention was appropriate. Five failures concern literal
quotation bytes: dev13 oracle/tool-only capitalization, dev14 automatic/oracle
in-quote punctuation, and dev17 automatic Markdown within the quote. Three other
comparison-condition answers lacked required support: dev11 automatic, dev12
tool-only and dev15 automatic. No manual fabricated-source, retired-as-current,
authority/speaker or silent-conflict violation was identified.

The frozen validator accepted 131 assessments and rejected 13 with 137 detailed
validation errors. The rejected records preserve truthful source/answer judgments
and explicitly identify evaluator conflicts. The source auditor treated oracle
query intent, exact read pin, or query identity-match annotations as universal
support requirements. This rejects active originals found through historical
search; a full accepted relationship/source without the oracle's name-match
annotation; an exact structured subject Entity ID without a duplicate query-match
annotation; and the correct retired historical fact read at noon rather than the
oracle's midnight on the requested date. Its exact source bytes, canonical
operation, actor, eligibility, lifecycle and fact-validity metadata remained
independently available. The oracle query parameters must not substitute for the
question's actual source-sufficiency requirement.

The exact frozen grader exited 1. The exact frozen resource aggregator also exited
1: 1,074 checks, 1,065 passes and nine failed/incomplete checks, all in quality.
Local, index, operating, execution-order and deterministic checks passed. The
remaining checks cover assessment validity/denominators, aggregate quality,
production grounding/citation accuracy and mandatory semantic roles, and the
oracle's mandatory-role and neighbor-investigation-family failures.
`complete`, `all_required_gates_pass` and `release_ready` are all false. The
complete report and all input hashes remain retained; no average waives a failure.

The next development version will retain the questions, source records, gold
support sets, numerical gates, context/resource budgets and model/ranking
configuration. It will correct generic source-sufficiency validation without
relaxing exact source, accepted identity origin, lifecycle, scope, egress or fact
validity checks. Separately, the #159 reader guide now prefers paraphrase with
original event citations and reserves quotation marks for exact source text.
Both changes require a new pre-output freeze and fresh complete reader comparison.
No release-held-out workload has been created or inspected.


## Development v3: corrections before the next freeze

The #159 guide correction is folded into its owning commit, with every descendant
reparented while preserving its issue-owned tree. It prefers paraphrase with
original event citations and requires exact case and punctuation for any source
quotation, with formatting outside the quote. The isolated owning-#159 export
passed ten complete-turn tests in 1.512 s. That deterministic check does not
establish fresh reader quality.

The source auditor now accepts independently sufficient delivered proof: exact
structured subject Entity IDs; full accepted alias origins with canonical
Alias/entity/operation/source/scope association; active original episodes read
through either current or historical retrieval; and the canonical fact-validity
interval covering the historical target. Every supplied identity marker remains
validated. Retracted canonical Source Links cannot regain accepted authority
from retained public originals. Explicitly acceptable context can support its
own wording but does not enter the required-gold numerator; required gold takes
precedence over an overlapping context label. All exact original-source,
lifecycle, scope, authority and retirement checks remain.

Seventeen retained-public-payload/adversarial controls passed in 0.403 s. They
cover the full v2 request cohort with zero new source violations and no prior
support lost. These are evaluator regressions using known development outputs,
not a corrected v2 reader result. The normative source-sufficiency rules are
embedded in the hash-frozen evaluation procedure; detailed RED/GREEN records
live in `../fixtures/memory-stage5-integrated/development/v3/`.

The v3 gate file corrects only the explanatory citation-denominator string to
match the already implemented v2 grader and assessment schema: required support
plus all additional citations, with missing required citations still counted.
All other JSON values, numerical gates, original adoption instant, roles and
budgets are byte-equivalent values. The original adopted-v1 file remains intact.
`development/v3/gate-description-correction.json` records the one changed JSON
pointer and both file hashes; this is no threshold relaxation.

Before the v3 freeze, `./scripts/verify-change.sh` passed from
`2026-09-11T05:00:43.576510Z` to `05:02:09.905883Z`, including full Go tests/vet,
UI lint/build and whitespace checks. The agent suite took 36.419 s. The same
five existing Fast Refresh warnings and >500 kB Vite chunk warning remain.
The 89 mapped Go tests passed (agent 10.052 s, localembedding 0.550 s, web
2.016 s), and all 23 UI tests passed, including the 14 required assertions.
The 393 compiled inputs remained identical across checks.

A separate pre-freeze review found two integrity gaps in release sealing:
mutable semantic-assessment packets/evidence summaries were not independently
rebuilt from retained raw reader outputs, and the purported #167 commit was not
compared with every compiled source file in the measured snapshot. Both must be
corrected and tested before v3 freezes. The independent harness audit found no
additional completeness blocker: all six conditions share canonical inputs,
reader turns use the actual production provider from request one, all planned
denominators remain enforced, and the required demonstrations are mapped to
complete-turn, HTTP and UI checks.

Both sealing gaps are now fixed before freezing. The source-commit protocol
suite passed eight tests in 0.943 s, plus eight existing report-seal controls in
0.015 s. Seven retained-raw controls passed in 4.596 s: they reconstruct all 144
packets and reject changed answer/context/evidence summaries, missing/extra
packets, and a preloaded auditor from another freeze. The analyzer loads its
exact adjacent auditor; pure recomputation leaves every output unchanged. The
old frozen v2 packet-tampering false pass remains recorded as a regression RED.
Six existing citation/resource-API controls also passed in 0.039 s. These checks
use known raw development outputs or explicit protocol fixtures; none generates
a reader answer or establishes a new measured quality result.


The v3 freeze completed at `2026-09-11T05:10:54.897313+00:00`, before reader generation.
Freeze SHA256: `932e95f04123c5935b432a47f8980db8629e62aa0a1cb056f82be35949bfdba0`.
Executable SHA256: `15e4d28e71df4cc3638890a3509675be0e192d850b69fa41f836f2f83569e6cb`.
The workload hash is unchanged at `2e1aff23f8426b45b6a1cc3301c769026ecb84cc55a9d4f03ba922eff333d562`.
The gate-description-only correction yields `bf730615b47e3acb2f9fc12fe57a1dad88d19169ce46967716ec6fde144b1ea9`; every
numerical gate remains identical to v1/v2. All 393 compiled inputs match the
verified checkout. The exact runner, auditor, scorer, grader, resource auditor,
source-sufficiency procedure and new canonical input archives are hash-bound.

Before required preparation, `/api/ps` reported no loaded model; afterward it
reported the exact selected MiniLM manifest. Preparation performed its required
source indexing without an extra warmup query. Index recovery immediately
followed, passing all 24 cases (exit 0) from `2026-09-11T05:10:55.081033+00:00` to
`2026-09-11T05:11:06.077858+00:00`. Its full source/path/order and resource audit remains
part of the eventual combined report.

All 2,880 v3 local turns passed across 144 twenty-sample cohorts, with zero
failed turns and zero unknown final retrieval-work totals. The largest cohort
p95 first-dispatch time was 41.526500 ms, final runtime retrieval work
56.786750 ms, and whole-turn time 97.245042 ms, each in
`dev10_exact_entity_id/automatic_deeper`. The first two unchanged gates are
1,500 and 3,000 ms. Execution ran from `2026-09-11T05:11:06.230115Z` to
`05:12:47.235063Z`, exit 0. These scripted measurements establish no reader
answer-quality or comparative causal-speedup claim.

All sixty v3 operating samples passed from `2026-09-11T05:12:47.400065+00:00` to
`2026-09-11T05:13:33.897349+00:00`, exit 0. Cancellation p50/p95/max were
0.757709/0.833583/0.855208 ms; lease replacement
0.425917/0.576959/0.583333 ms; caller deadline
7.977833/9.670292/10.785000 ms. Each mode retained all twenty samples under
the unchanged one-second maximum-tail gate, with zero failed samples.
Reader generation began after these prerequisite executions completed, at
`2026-09-11T05:13:34.046059Z`. Evidence and semantic quality remain unassessed
until all 144 reader turns close.


All 144 v3 reader turns completed in 178 actual model calls and 178 dispatches,
with zero execution errors, from `2026-09-11T05:13:34.046059Z` to
`05:21:50.398019Z`. The exact frozen evidence auditor completed successfully
and produced 144 assessment packets with zero source/request boundary
violations. It now reconstructs those packets from original raw outputs;
manual answer quality and the combined resource gates remain separate.

| Condition | First/final/union required sources | Complete final cases | Unwanted evidence | Reader p50/p95 ms | Maximum request bytes | Model calls |
| --- | --- | --- | --- | --- | --- | --- |
| `no_recall` | 0/0/0 of 27 | 0/19 | 0/0 | 2929.518375/4420.959125 | 7,215 | 24 |
| `recent_context` | 0/0/0 of 27 | 0/19 | 0/0 | 2864.619458/4894.889375 | 8,219 | 24 |
| `tool_only` | 0/27/27 of 27 | 19/19 | 5/36 | 4639.251167/8085.894791 | 18,377 | 52 |
| `automatic` | 25/25/25 of 27 | 17/19 | 12/45 | 2849.256917/5510.136333 | 15,316 | 24 |
| `automatic_deeper` | 25/27/27 of 27 | 19/19 | 12/47 | 3248.000667/6195.498625 | 18,871 | 30 |
| `oracle` | 27/27/27 of 27 | 19/19 | 0/27 | 2425.449042/4580.429208 | 13,360 | 24 |

The two no-retrieval conditions have no unwanted-evidence denominator, recorded
as null rather than a perfect precision score. These are availability and cost
measurements; they do not establish that the reader used a source or answered
correctly. In the production automatic-plus-deeper condition, all 27 required
source records reached the final request and all 19 answerable cases had their
complete gold support. The following semantic assessment decides whether the
answers handled that evidence correctly.


## Completed v3 semantic assessment and development decision

Three explicitly identified Codex agents assessed all 144 fresh answers,
48 each, with no implied human or second assessment. Every assessment passed
the exact frozen schema/provenance validator; zero evaluator conflicts remain.
They inventoried 192 personal propositions and 48 source quotations. Every
asserted proposition is grounded, every quoted source span is byte-exact, and
all fabricated-citation, retired-as-current, speaker/authority and silent-conflict
counts are zero. This verifies the five prior quotation failures on this fresh
development run; it does not establish repeated-generation reliability.

| Condition | Supported personal components | Grounded personal propositions | Correct original citations | Semantic cases |
| --- | --- | --- | --- | --- |
| `no_recall` | 0/37 | 1/1 | 0/27 | 24/24 |
| `recent_context` | 0/37 | 2/2 | 0/27 | 24/24 |
| `tool_only` | 37/37 | 48/48 | 28/28 | 24/24 |
| `automatic` | 33/37 | 47/47 | 28/30 | 22/24 |
| `automatic_deeper` | 37/37 | 47/47 | 28/28 | 24/24 |
| `oracle` | 37/37 | 47/47 | 28/28 | 24/24 |

Honest no-recall/recent-context abstentions count as appropriate reader behavior
while missing personal components and required citations receive zero credit.
They do not establish production-quality coverage. The two semantic failures
are automatic-only comparison cases: `dev11_graph_bridge` lacks the snack
support and `dev15_historical_validity` lacks the requested historical fact.
Both truthfully acknowledge the gap. They remain in every planned denominator.
Production deeper search and oracle each supply and correctly answer all required
personal components; all fifteen critical-semantic cases, both clear-reference
cases and both material-clarification cases pass in each required condition.

The frozen semantic grader and resource auditor each exit 0. The resource report
reconstructs the entire evidence report and all 144 packets from retained raw
outputs before validating all assessments and recomputing every resource, source,
recovery, execution-order and deterministic gate. It records **1,077 passes,
zero failures and zero incomplete gates**, with `complete: true`,
`all_required_gates_pass: true` and `release_ready: false` for development.
The final resource execution closed at `2026-09-11T05:31:00.887258Z`.

SHA256 identities:

- Final quality report: `efcd4c23c28ab0fd9cec49eb1e64460e8353aa831be081c7867756bae0ac0583`.
- Final resource report: `b9c936332bea8d50685e95a822cacc9ef4240613ea9e0f7d096a6c578f52d7ae`.
- Complete assessment manifest: `0fd221cbe155de62b88c32e239f0626fc9d0159853d0555304f4977622b7775d`.

The successful v3 configuration is frozen for #168. No fresh held-out corpus has
yet been created or inspected. The next step requires an independent curator
and the same executable, model/index identity, budgets, ranking, scoring rules,
numerical thresholds and measurement order. Any held-out failure must be
published without changing these inputs or manufacturing a passing threshold.

Limitations remain: one reader repetition per case/condition; agent semantic
assessment without independent human review; isolated per-case indexes;
hardware-specific local timings; no inference-counter or endpoint peak-memory
claim; and optional dense fallback during cold first-read model unavailability.
V1's cold-path disagreement, v1's context failures and v2's reader/evaluator
failures are retained. The passing v3 result neither removes those observations
nor claims a causal speedup against their failed runs.


The first final staging whitespace check found one extra blank line at EOF in
the maintained evaluation procedure and its frozen copy (exit 2). The maintained
document loses only that extra newline. The immutable frozen protocol remains
byte-identical and is marked as a binary Git artifact, like captured SSE streams;
the maintained procedure remains reviewable text. Its frozen hash and exact
release procedure are unchanged. This affects artifact presentation and the
maintained document's final newline, not any measured input, scoring rule,
threshold or source behavior. The corrected staged and unstaged checks both pass (exit 0); the exact observed
commands/results are retained in `development/v3/staging-whitespace-checks.json`.


## Preliminary twelve-commit review and cross-scope refresh correction

A separate parallel review examined `git diff f27546d...0269e11` across the
twelve committed issues before release evaluation. #168 was explicitly outside
this preliminary review. Neither reviewer read fresh held-out material.

### Standards

Zero actionable documented-standard violations or material baseline smells were
identified in the runtime, SQLite retrieval/indexing, receipts/HTTP/UI, and
current evaluation harness/scorers. The review was read-only; tests and models
were not run, and immutable raw experiment archives were excluded from manual
code inspection. Standards count: 0; worst issue: none identified.

### Spec

One P2 finding: `hasNewRetrievalConflict` restricted changed peer Claims to the
held Claim's own scope, whereas normal conflict discovery includes Global, the
reader context and its current Session. A Workspace reader holding a Global
owner fact could therefore miss a restored contradictory Workspace Claim or
Source Link. The held Global Claim remained eligible and generic restoration
wording did not necessarily trigger the new-owner-statement heuristic. #162
requires refresh when new information invalidates the supplied view; #159
requires contradictory active Claims to be supplied. The fix must preserve
explicit historical/knowledge pins, eligibility and the one shared eight-peer
bound across the authorized scope union. Spec count: 1; worst issue: P2.

Both complete-turn real-SQLite restoration regressions failed before the fix
(package 0.539 s). The correction belongs in #162. The v3 outputs and passing
report remain unchanged; that report cannot waive this newly confirmed boundary
gap. No release preparation or reader output has occurred. The independently
authored release corpus will remain fixed while the corrected implementation
is verified on the same known development questions under a new v4 freeze.


## Development v4: verified cross-scope refresh

The preliminary review's P2 is resolved in owning issue #162. The refresh check
now considers Global, the reader context and the current Session together, with
one eight-peer bound across that authorized union. Restoring a contradictory
Workspace Claim or Source Link refreshes a held Global interpretation; foreign
projects cannot consume that bound or enter the provider request. Explicit
historical and knowledge-time views remain pinned, and an explicit world-validity
date stays intact. The real-SQLite complete-turn regressions
`TestMemoryInvestigationRefreshesRestoredConflictsAcrossAuthorizedScopes` and
`TestMemoryInvestigationIgnoresRestoredConflictsOutsideAuthorizedScopes` are in the
passing V4 matrix. This closes the specific review finding; final review of all thirteen issues
remains a separate handoff requirement.

The fresh V4 freeze completed at `2026-09-11T05:59:09.519646+00:00`, before reader
outputs. All **394 compiled source inputs** equal the independently captured
verification manifest. The development workload, numerical gates, rubric,
assessment schema, source auditor, evidence scorer, semantic grader, resource
auditor and reader profile are byte-identical to their V3 counterparts. V4 changes
the reviewed scope-refresh implementation, its two matrix tests and its declared
configuration metadata; it does not tune against release answers or change the
semantic meaning of a question.

The actual reader remains `openai/gpt-6-astra-20260903` through its configured
`openai/gpt-6-astra` alias, with low reasoning, 24,576 working tokens and a
768-token output reserve. Local discovery retains `all-minilm:22m`, the selected
manifest `1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef`,
Ollama 0.6.3, SQLite/Go cosine retrieval and the existing limits. The worker uses
Go 1.26.3 on macOS 15.7.8 arm64, Mac15,6, 11 logical CPUs and 19,327,352,832 bytes
of memory; GOMAXPROCS=11, GOGC=100, GOMEMLIMIT=off and empty GODEBUG. Exact model
blobs, runtime, profile and all retrieval parameters remain in the freeze.

### Verification and closed execution

`./scripts/verify-change.sh` passed from `2026-09-11T05:57:07.473230+00:00` to
`05:58:38.579605+00:00`: full Go tests/vet, UI lint, TypeScript/Vite build and
staged/unstaged whitespace checks. Agent tests took 43.697 s and SQLite tests
83.353 s. The **91 named Go matrix tests** passed with zero failed or skipped
matrix events (agent 11.177 s, localembedding 0.544 s, web 1.921 s). Its seven UI
files passed **23 tests**, including all **14 explicitly required assertions**,
with zero failed or pending tests. Exact commands, structured outputs, start/end
records and source hashes are retained in `integrated-development-v4-checks/`.
The two longest Go package times are part of the full verification run, not
retrieval latency samples.

Warnings are unchanged: five existing Fast Refresh export warnings in
`Icon.tsx`/`memory/presentation.tsx` and Vite's >500 kB chunk warning. No required
V4 matrix check or measured stage was skipped. This report-only update receives
a scoped whitespace check; it does not rerun or replace the frozen measurements.

The runner enforced preparation → index → local → operating → reader. Before
required preparation, `/api/ps` reported no loaded model; after indexing it
reported the selected MiniLM manifest. Index testing immediately followed
preparation, with no extra warming query, forced unload or selected retry.
All times below are UTC on 2026-09-11; every execution exited 0.

| Stage | Start | End | Observed result |
| --- | --- | --- | --- |
| Canonical preparation | `05:59:06.170673` | `05:59:09.278151` | 24 closed source databases/maps; no reader answers |
| Index recovery | `05:59:09.707543` | `05:59:20.730209` | 24/24 cases; exact original source/path/order agreement |
| Local resource turns | `05:59:20.887880` | `06:01:00.013877` | 2,880/2,880; 144 cohorts × 20 repetitions |
| Operating boundaries | `06:01:00.241677` | `06:01:46.513325` | 60/60; 20 samples per boundary |
| Actual reader | `06:01:46.762432` | `06:10:27.160525` | 144 turns; 179 actual calls/dispatches; zero execution errors |
| Evidence audit | `06:10:36.071671` | `06:10:36.735183` | 144 packets; zero request/source boundary violations |
| Semantic grading | `06:21:05.166373` | `06:21:05.319275` | 144 valid assessments; all 56 quality gates pass |
| Combined resource audit | `06:21:05.319694` | `06:21:08.670660` | 1,077/1,077 gates; no incomplete result |

The largest local cohort p95 was **44.677084 ms to first dispatch** and
**63.451832 ms of final runtime retrieval work**, both in
`dev10_exact_entity_id/automatic_deeper`; their fixed gates are 1,500 and 3,000 ms.
Largest whole-turn cohort p95 was 104.933750 ms in
`dev07_compacted_clear/automatic_deeper`. All final retrieval-work totals are
known. Scripted resource repetitions do not add independent reader-quality
samples or establish a causal speedup against an earlier version.

| Operating boundary | Samples | p50 ms | p95 ms | Maximum ms | Fixed maximum gate ms |
| --- | --- | --- | --- | --- | --- |
| Cancellation | 20 | 0.736458 | 0.935542 | 1.093459 | 1,000 |
| Lease replacement | 20 | 0.409417 | 0.469875 | 0.539959 | 1,000 |
| Caller deadline | 20 | 8.063542 | 9.903959 | 10.027500 | 1,000 |

No late protected effects or invalidated original receipts were observed. The
24 isolated index cases cover 61 source records in total; they are not a single
61-record index benchmark. Maximum measured build, incremental refresh and
rebuild times were 325.267250, 18.064042 and 160.235167 ms. Maximum reported
derived-storage upper bound was 645,024 bytes (derived pages plus all WAL), and
maximum warm incremental Go-worker RSS growth was 212,992 bytes. Separately,
the largest observed embedding-process-family RSS snapshot was 150,159,360 bytes;
this is not endpoint peak memory and is excluded from the Go-worker RSS gate.
No embedding request/input-byte counter is claimed for this unproxied run.

### Evidence availability and actual reader cost

Each condition has 24 cases, 19 answerable cases and 27 required source records.
First/final/union below count required support, not all retrieved records.

| Condition | First/final/union required sources | Complete final cases | Unwanted / delivered | Reader p50/p95 ms | Maximum request bytes | Calls |
| --- | --- | --- | --- | --- | --- | --- |
| `no_recall` | 0/0/0 of 27 | 0/19 | — (0 delivered) | 2988.531833/5847.686625 | 7,215 | 24 |
| `recent_context` | 0/0/0 of 27 | 0/19 | — (0 delivered) | 2947.459875/5473.599292 | 8,219 | 24 |
| `tool_only` | 0/27/27 of 27 | 19/19 | 5/36 | 5672.912125/9139.179416 | 18,377 | 54 |
| `automatic` | 26/26/26 of 27 | 18/19 | 11/45 | 2885.408375/5095.593000 | 15,316 | 24 |
| `automatic_deeper` | 26/27/27 of 27 | 19/19 | 11/46 | 2798.312916/6964.228667 | 17,648 | 29 |
| `oracle` | 27/27/27 of 27 | 19/19 | 0/27 | 2518.767375/4507.163541 | 13,353 | 24 |

No-retrieval unwanted-evidence fractions are null, not perfect precision scores.
Production deeper retrieval supplies all 27 originals by the final request and
complete gold support in all 19 answerable cases; unwanted evidence is 11/46,
below the fixed 0.35 limit. The automatic first pass supplies 26/27 and 18/19
complete cases. Availability alone does not prove that an answer correctly used
or cited an original.

### Semantic outcomes and current development decision

Three explicitly identified Codex agents assessed 48 fresh answers each, with
no implied human or independent second review. All 144 assessments are valid;
there are zero evaluator conflicts. The complete inventory contains **194 grounded
personal propositions and 55 byte-exact source quotations**, with zero wrong
quotations. Counts come from the combined actual assessments, not an estimate.
All fabricated-citation, retired-as-current, authority/speaker and silent-conflict
counts are zero.

| Condition | Supported personal components | Grounded propositions | Correct original citations | Semantic cases |
| --- | --- | --- | --- | --- |
| `no_recall` | 0/37 | — (0 assertions) | 0/27 | 24/24 |
| `recent_context` | 2/37 | 2/2 | 0/27 | 24/24 |
| `tool_only` | 37/37 | 48/48 | 29/29 | 24/24 |
| `automatic` | 34/37 | 48/48 | 28/29 | 23/24 |
| `automatic_deeper` | 37/37 | 48/48 | 28/28 | 24/24 |
| `oracle` | 37/37 | 48/48 | 28/28 | 24/24 |

Citation denominators include every required source and every additional actual
citation; missing required citations retain zero credit. Honest no-recall/recent-
context abstentions remain appropriate behavior while earning no missing fact or
citation coverage. Two components in `recent_context` are supported by the current question or
original discussion; they do not recover the missing original personal records.
The zero-assertion no-recall condition has null grounding, not a 100% score.

The one semantic failure is retained: `dev15_historical_validity/automatic`
lacks the historical Willow-annex source, all three requested historical
components and its required citation. The answer appropriately declines to
substitute the current Harbor-loft fact, but that automatic-only comparison is
not covered by the honest-absence baseline exception. It therefore fails despite
correctly attributing the current context. Production deeper retrieval and oracle
each pass 24/24, all 37 personal components, and every mandatory critical-semantic,
clear-reference and material-clarification case. Each of the five previously failing V2 quotation case/conditions now passes
semantically; every literal quote actually made in V4 is exact. This is integrated
known-development verification, not isolated causal attribution to the #159 guide
or evidence of repeated-generation reliability.

The frozen grader and resource auditor both exit 0. The latter independently
reconstructs the evidence report and all packets from retained raw outputs,
validates the assessments, and recomputes source, resource, recovery, execution-
order and deterministic checks. It reports `complete: true`,
`all_required_gates_pass: true`, **1,077 passed gates, zero failed/incomplete**, and
`release_ready: false` for development.

### Commands, immutable identities and next boundary

The actual child commands all used the frozen `integrated.test` executable,
`-test.count=1 -test.v -test.timeout=3h`, and these exact selectors, in order:
`^TestMemoryStage5IntegratedIndexMeasurements$`,
`^TestMemoryStage5IntegratedLocalEvaluation$`,
`^TestMemoryStage5IntegratedOperatingMeasurements$`, and
`^TestMemoryStage5IntegratedReaderEvaluation$`. The frozen runner supplied each
mode's output/configuration environment; invoking the binary without that
environment is not a replay. Exact argv, working directory, result and UTC records
for these runs and the three Python audits are retained in
[development/v4/results-summary.json](../fixtures/memory-stage5-integrated/development/v4/results-summary.json).
The source/check manifests bind the 91-test `go test -json` selector and seven-file
Vitest command without abbreviating their actual recorded argv.

For an independently named replay on the same configured host, use the frozen
runner with a new output directory for each mode, preserving the declared order:

```sh
python3 -B /private/tmp/evie-memory-stage5/integrated-development-v4/run.py run \
  --freeze /private/tmp/evie-memory-stage5/integrated-development-v4/freeze.json \
  --mode index --output /tmp/evie-memory-stage5/new-v4-replay-index
```

Then use `local`, `operating` and `reader`, each with its own new directory. The
example is a replay instruction, not another measured attempt. A later standalone
index replay can start with different model residency; retain that recorded state
and any cold fallback rather than treating it as the original post-preparation
measurement. Frozen `scorer.py`
takes `--freeze --results --output`; `grader.py` takes
`--freeze --evidence-report --assessments --output`; `resources.py` takes
`--freeze --evidence-report --quality-report --local --index --operating
--deterministic --output`. The full executed argument values, including original
output paths, are in the linked summary. Assessment labels must again be explicit;
re-running scripts does not perform semantic judgment.

SHA256:

- V4 freeze: `57a810541c66dbdc3ef09749b6ac786e360e0ab4c02473d529a9cf8f7c011a3f`.
- Executable: `06ba5e56aecf5684f2209bd801074ea65ed032dc88181d07a4d016260532f072`.
- Compiled-source manifest: `1c65712b69a6c54e531eb753be7bb0d054c4ef28b89bbc21651ad990f1614447`.
- Compiled-source archive: `3b4e0cc90c9c82f2167b171672ffb05207f0a7ed3914c114400bef731445bec3`.
- Workload (unchanged): `2e1aff23f8426b45b6a1cc3301c769026ecb84cc55a9d4f03ba922eff333d562`.
- Gates (unchanged from V3): `bf730615b47e3acb2f9fc12fe57a1dad88d19169ce46967716ec6fde144b1ea9`.
- Evidence report: `ae9c5e5e99a73eac39eb45a185a05605dba06bd7ed17df33037d99b582e40586`.
- Quality report: `7c4893588b2ad4d7ace5925796fa9416be0ffbccc2649bccb4098e7d3db9f3fb`.
- Resource report: `30a2e068ded9637dc38a3a9ec015d756c7c5338f8fd0267a85ba378fdcfd8e43`.

V4 is the current passing development configuration for #168. Held-out source
preparation and reader evaluation have **not run**. A separate agent curator
already authored the release workload under passing committed V3; its bytes
remained fixed while the implementation owner corrected the independently found
scope bug and reran the unchanged development questions. The curator read no
development reader answers/traces, prior component-held-out answers or release
outputs. Its provenance record retains one qualification: after all cases were
authored, an agent inventory incidentally exposed the already completed review
finding. No pre-exposure hash was measured; recorded file modification time
precedes the inventory, and the authored bytes remained unchanged. This is agent
curation, not independent human curation. Detailed provenance qualification
belongs to the separate #168 record; this report did not inspect the workload.

The next stage must retain the frozen executable, model/index identity, budgets,
ranking, scoring rules, numerical gates and measurement order. Preserve any
held-out failure without tuning or dropping it. One reader repetition, agent
assessment, isolated case indexes and hardware-specific timings remain practical
limits. V1 context/cold-path failures, V2 quote/evaluator failures and V3's later
review finding remain visible in their original records. Historical artifact
recovery work is separate; no claim about an older missing source snapshot is
inferred from V4's complete manifest. Final review, complete preservation and the
held-out result remain required before release readiness can change.
