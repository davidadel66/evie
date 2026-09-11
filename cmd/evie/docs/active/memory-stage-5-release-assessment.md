# Frozen retrieval assessment (#168)

**The frozen held-out assessment is complete and release readiness is false.**
The exact passing #167 development-v4 executable and unchanged numerical gates
produced 1,051 passing, 23 failing and three incomplete gates out of 1,077.
The quality and final resource audit commands both exited 1. All planned
measurements and all 144 manual agent assessments are retained, including failed
turns and the two assessments rejected by the frozen validator. No corpus,
implementation, model, ranking, budget, rubric or scoring program was changed in
response to held-out outputs. This is the failed assessment deliverable for #168;
it does not establish release readiness or authorize deployment.

The parent is #154. The verified prerequisite is #167 commit
`efef0dba4d34bc8d28d0b2f80ab91e1a3621f1f6`. All child implementations remain
on one branch; GitHub issue closure was not used as a dependency gate.

## Fixed inputs and provenance

The held-out v2 seal completed at `2026-09-11T06:41:45.948626Z`:

| Input | SHA256 |
| --- | --- |
| Held-out freeze | `c6d6a2c8627580963fb7f2edb341173bd34487e09c80a3e9a9813f0fef169ed2` |
| Measured executable, identical to development v4 | `06ba5e56aecf5684f2209bd801074ea65ed032dc88181d07a4d016260532f072` |
| Held-out v2 corpus | `22cd641595f6aa9791f212db4f9ce95a745edbd68444a073d106b5bc3b52582b` |
| Development v4 freeze | `57a810541c66dbdc3ef09749b6ac786e360e0ab4c02473d529a9cf8f7c011a3f` |
| Passing development resource report | `30a2e068ded9637dc38a3a9ec015d756c7c5338f8fd0267a85ba378fdcfd8e43` |
| Compiled input manifest, 394 files | `1c65712b69a6c54e531eb753be7bb0d054c4ef28b89bbc21651ad990f1614447` |

The frozen configuration retains six conditions: no recall, recent context,
tool-only, automatic, automatic plus deeper search, and oracle. The corpus has
24 cases, two per family; 19 require personal evidence, with 28 source obligations
and 36 personal answer components. Fifteen cases are critical semantic cases,
two require material clarification, and two require clear-reference resolution.
There are 144 planned reader turns, 2,880 local turns, 24 index comparisons and
60 operating samples. No failed sample is removed or replaced.

The reader is `openai/gpt-6-astra-20260903`, low reasoning. The selected local
embedding model is `all-minilm:22m`, 384 dimensions, L2-normalized float32 vectors,
using the pinned Ollama 0.6.3 runtime on literal loopback. The machine is an
11-core Apple M3 Pro, 18 GiB memory, macOS 15.7.8 arm64. Full runtime/model hashes,
worker environment, request profile, numerical gates and timing definitions are
retained in the immutable freeze; see the adjacent integrated pilot for their
development measurement. Working context is 24,576 tokens, output reserve 768,
and margin 4,096; the measured profile permits 19,712 serialized input bytes.
Kernel calls, results, held evidence, expansion, cumulative delivery and deadlines
retain the exact adopted bounds.

## Corpus preparation and recorded protocol qualifications

A separate agent curator authored the v1 source statements and questions after
the passing, committed development v3 pilot. A subsequent implementation review
required the cross-scope #162 refresh correction; development v4 then reverified
that correction before release sealing. The original v1 corpus was not changed
in response to v4 development outputs. The two-fixture v2 preparation repair
described below happened before any release measurements. This chronology is
retained rather than claiming that corpus authoring began after the final v4
configuration. The actual curation records identify its context restrictions
and two qualifications: after authoring all 24 texts, an agent inventory exposed
an implementation-review summary to the curator; later, a report author reading
a curation metadata record saw one held-out source excerpt in a static-validation
reason. No development reader answers or release answers informed curation, and
no resulting implementation, scoring or gate changes were made. The records do
not establish a perfectly blind process or human assessment.

The first v1 preparation failed before a held-out freeze or measured index,
local, operating or reader output existed. Twenty-two cases prepared; two
compaction fixtures had only two completed owner/assistant roots, while the
frozen public compaction seam requires at least three and retains the newest
two. Source embedding during preparation did occur; no reader answer was
generated. The complete failed destination and command logs are retained in
`../fixtures/memory-stage5-integrated/runs/heldout-v1-failed-setup/`.

Before any release measurement, the original curator made a versioned structural
repair: append one neutral “One moment.” / “Okay.” completed exchange to each of
those two discussions. Removing precisely those two appended objects reconstructs
the original v1 bytes. All source records, existing discussions, continuity,
questions, labels, roles, and the other 22 cases remain unchanged. The correction
record and both corpus versions are retained in
`../fixtures/memory-stage5-integrated/heldout/`. V2 then prepared all 24 cases and
sealed successfully. Once measured outputs existed, its annotation conflict was
left unchanged.

The execution wrapper originally withheld reader generation after index and
local failures. At `2026-09-11T06:52:12.007551Z`, a separate continuation recorded
that stop-policy change explicitly before starting the **first** reader attempt.
It completes the independently required reader assessment but is a post-output
protocol deviation from the wrapper's declared stop policy. Both the withholding
record and continuation are preserved. It supplies no waiver for failed gates,
no successful-only replacement, and no basis for claiming release readiness.

## Reader execution and evidence quality

The first reader run ended at `2026-09-11T07:00:37.808390+00:00` with exit 1. All
144 planned case/condition results were retained: 141 turns returned without a
turn error and three failed before a required HTTP dispatch. There are 187 exact
prepared request/receipt records, 184 actual conversational HTTP bodies and 184
model response artifacts. Two separate metadata HTTP records are not counted
as conversational model calls. The three prepared-only requests are hold15 under
tool-only (call 2), automatic plus deeper (call 2), and oracle (call 1).

Every actual conversational HTTP body matches its encoded request byte-for-byte.
The frozen evidence auditor generated all 144 assessment packets and reported
10 violations, all associated with those three hold15 failures: missing actual
dispatch artifacts, the frozen forbidden-text check, failed reader turns, and
the oracle condition having no successful captured HTTP response. A successful
auditor process exit means its report was produced; it does not mean those
reported evidence boundaries passed.

| Condition | Initial / 28 | Final / 28 | Union / 28 | Complete / 19 | Unwanted / present | Actual calls | Turn p50 / p95 (ms) | Max request bytes |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| No recall | 0 | 0 | 0 | 0 | 0/0 (undefined) | 24 | 2795.693 / 5420.249 | 7155 |
| Recent context | 0 | 0 | 0 | 0 | 0/0 (undefined) | 24 | 2697.334 / 5394.201 | 8121 |
| Tool-only | 0 | 24 | 24 | 15 | 3/30 | 55 | 4860.748 / 7944.925 | 18449 |
| Automatic | 22 | 22 | 22 | 13 | 12/43 | 24 | 2836.226 / 4898.262 | 15206 |
| Automatic plus deeper | 22 | 25 | 25 | 16 | 12/46 | 34 | 2931.803 / 7532.579 | 18298 |
| Oracle | 27 | 27 | 27 | 18 | 0/27 | 23 | 2523.749 / 4363.476 | 13380 |

Automatic plus deeper reaches 25/28 required source obligations and 16/19
complete final support sets. The remaining gaps are hold12 (accepted errand-list
record), hold15 (historical pickup record), and hold20 (the original owner problem
alongside an assistant suggestion). Oracle supplies 27/28 obligations; hold15
remains in its denominator despite being blocked before its first HTTP call.
These source metrics are separate from whether an answer is truthful, complete,
correctly attributed, or properly cited.

In hold12, the complete original owner wording actually reaches the reader as a
Conversation Excerpt, and the automatic and automatic-plus-deeper answers
attribute and cite that wording. The frozen source metric requires the accepted
record declared by its gold binding, so it grants no accepted-record recall
credit. The two manual assessments truthfully retain their source-grounded
judgments; the frozen validator rejects them with 14 errors. This limitation is
recorded separately from the retrieval metric and is not repaired after outputs.

## Completed semantic assessment

All 144 assessments are present and marked review complete. They were authored
by three Codex agents reading actual answers, original source bindings and
supplied context; no human or second independent review is implied. Raw manual
judgments record 128 semantic passes and 16 failures. The frozen validator
accepts 142 assessments and rejects the two hold12 rows; validated metrics record
126 semantic passes and 18 nonpasses. Invalid rows remain in every planned
component and citation denominator.

Each condition has 24 planned answers, 36 personal answer components and 28
required original-source citations. Correct additional citations increase both
the citation numerator and denominator. General-answer components are excluded
from personal component recall. These are the exact validated quality results:

| Condition | Valid / 24 | Semantic pass / 24 | Personal components | Required citations | Additional citations | All citation accuracy | Personal grounding |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| No recall | 24 | 24 | 0/36 (0.00%) | 0/28 | 0/0 | 0/28 (0.00%) | Unknown |
| Recent context | 24 | 24 | 2/36 (5.56%) | 0/28 | 0/0 | 0/28 (0.00%) | 2/2 (100.00%) |
| Tool-only | 24 | 20 | 29/36 (80.56%) | 24/28 | 1/1 | 25/29 (86.21%) | 43/43 (100.00%) |
| Automatic | 23 | 18 | 27/36 (75.00%) | 22/28 | 2/2 | 24/30 (80.00%) | Unknown |
| Automatic plus deeper | 23 | 20 | 30/36 (83.33%) | 24/28 | 1/1 | 25/29 (86.21%) | Unknown |
| Oracle | 24 | 20 | 32/36 (88.89%) | 25/28 | 0/0 | 25/28 (89.29%) | 43/44 (97.73%) |

Automatic and automatic-plus-deeper grounding is null because each condition has
an invalid assessment. Observed valid-subset counts of 47/48 and 45/45 must not
replace those unknown totals. No-recall grounding is null because it makes no
personal propositions. Its 24 semantic passes, and the recent-context baseline's
24 passes, include honest abstention and clarification; missing personalized
components and citations still earn zero credit.

Each rejected hold12 row has seven validation messages: unsupported source
record and original-event proof for component 0, the same two proof failures for
propositions 0 and 1, and a citation without independently validated support.
The actual original owner statement is present as a Conversation Excerpt, while
the accepted Claim required by the gold record is absent. Raw assessment labels
were not falsified to satisfy the validator. The raw totals, kept separate from
validated scores, are 122/216 supported personal components, 184 grounded and two
unsupported personal propositions, 97/168 correct required citations and four
correct additional citations. Thirteen purported source quotations were
inventoried. The full original judgments and all 14 errors remain in the
[gate ledger](../fixtures/memory-stage5-integrated/heldout/v2/post-run-diagnostics/heldout-v2-final-gate-ledger.json).

The actual oracle answer in hold17 is a distinct reader error: it says the newer
owner report resolves the disagreement while both accepted conflicting records
remain active. Supplying all three sources and their correct original citations
does not authorize that resolution. The hard silent-conflict-resolution count
is one. Fabricated citations, retired-as-current assertions and authority/speaker
violations are each zero. The same conflict violation is checked by two gates;
it is not counted as two separate events.

Other semantic failures are missing original citations in hold03/oracle and
hold11/deeper and oracle; missed preferences or context in hold07/tool-only and
hold12/tool-only; the later choice missing in hold13/automatic; historical answer
coverage missing in all four memory-enabled hold15 conditions; newer owner
reports missing in hold17/automatic and hold18/automatic; and the original owner
problem and citation missing in hold20/tool-only, automatic and deeper. Hold20's
assistant quotation is correctly attributed as a tentative suggestion, but that
does not supply the missing owner context. Empty failed answers retain all
missing components and citations.

Both gated production and oracle conditions pass their two material-clarification
and two clear-reference obligations, with zero unnecessary clarifications. Their
unanswerable/unavailable and scope/no-memory-needed families pass. These bounded
successes do not waive the failed critical cases, missing citations or conflict
violation. The oracle's own failures demonstrate reader limitations separately
from candidate retrieval gaps.

## Known historical annotation conflict

The hold15 gold support set requires the old pickup record for a historical
date, while `forbidden_current_text` also contains that record’s location. The
frozen Go evaluator and Python scorer prohibit that phrase anywhere in the
request. Unlike their source-ID rules, the text check has no historical exception.
All three retained index variants fail only this text check on their second
prepared request. The matching source is marked historical and currently retired;
its known validity contains the requested date. Exact source hashes, inspection
and ordered canonical references agree across the variants, but the failed turns
still make the frozen restart/rebuild agreement gates fail.

This establishes a workload/evaluator inconsistency, not an observed current-view
retirement leak. Neither the annotation nor evaluator was repaired after outputs.
A later correction must clarify current-only versus unconditional prohibitions
and validate contradictory labels before a separately authorized fresh assessment.
It must preserve these failures as known regression evidence.

## Local, index and operating measurements

All 2,880 scripted local turns were retained: 2,860 completed without a turn
error; all 20 failures are hold15/oracle hitting the unchanged forbidden-text
check. Its final runtime accounting is unknown for every repetition, so its
20-sample runtime-accounting and p95-work gates are **incomplete**. Earlier
receipts and delegated search time are not substituted for the missing final
total. All 144 groups retain their 20 planned repetitions. These scripted runs
make no reader-model calls and establish no answer-quality result.

The largest measured cohort p95 for first dispatch is 39.863167 ms
(hold10/automatic plus deeper), below the 1,500 ms bound. Among the 143 groups
with complete runtime accounting, the largest p95 Kernel work is 55.730544 ms
(the same group); this does not supply a result for the missing group or a
cohort-wide pass. The largest whole-turn cohort p95 is 105.858542 ms.

Index comparison retains 24 cases and 51 source records; 23 cases pass and
hold15 fails. The maximum recorded initial build is 347.845916 ms, rebuild
48.539000 ms, and incremental refresh 16.952250 ms. The largest derived-storage
upper bound is 477,208 bytes, versus 32 MiB. The largest observed warm worker
RSS increase is 212,992 bytes, versus 512 MiB. The storage estimate includes
derived pages plus all WAL bytes; worker RSS excludes the embedding server.
Endpoint process-family RSS snapshots are retained separately and are not peak
memory measurements. Endpoint inference-request and input-byte counters are
unavailable because the unchanged public endpoint was not instrumented; no
counts are inferred. Initial model state is recorded, not assumed cold.

All 60 operating samples pass, including no late provider call, approval,
memory mutation or receipt after the boundary and retention of prior evidence.
Each condition has 20 samples and a 1,000 ms maximum-tail gate.

| Boundary | p50 (ms) | p95 (ms) | Maximum (ms) |
| --- | ---: | ---: | ---: |
| Cancellation | 0.720875 | 0.758541 | 0.816583 |
| Lease replacement | 0.403792 | 0.459041 | 0.482583 |
| Caller deadline | 8.123583 | 10.218875 | 12.749584 |

Exact per-case/cohort values and gate details are retained in the frozen
aggregator output. The supplementary local/index/operating reconstruction uses
the exact frozen resource methods; it is explicitly a partial diagnostic record,
not a complete readiness report. Failure strings mentioning an “actual provider
payload” in these scripted modes refer to the frozen verifier’s shared encoded
request check; they are not evidence of a remote HTTP transmission.

## Final gate decision and exact audit results

The final resource command closed at `2026-09-11T07:28:33.655409Z`, exit 1.
`complete`, `all_required_gates_pass` and `release_ready` are all false. Of 1,077
resource gates, 1,051 pass, 23 fail and three are incomplete. The quality report
has 45 passing and 11 nonpassing gates. There is no release-gate waiver.

| Audit | Started UTC on 2026-09-11 | Closed UTC | Exit |
| --- | --- | --- | ---: |
| index | 06:41:46.016967 | 06:41:57.099626 | 1 |
| local | 06:41:57.100098 | 06:43:33.380979 | 1 |
| operating | 06:43:33.381534 | 06:44:19.829804 | 0 |
| reader | 06:52:12.007995 | 07:00:37.903929 | 1 |
| evidence | 07:05:44.948992 | 07:05:45.689697 | 0 |
| grade | 07:28:30.592324 | 07:28:30.727833 | 1 |
| resource | 07:28:30.728206 | 07:28:33.655409 | 1 |

The [retained ledger](../fixtures/memory-stage5-integrated/heldout/v2/post-run-diagnostics/heldout-v2-final-gate-ledger.json)
contains the exact command arrays, closed execution records, report hashes,
quality tables and full failure details. Its SHA256 is
`33e3e58d12067a93469af9d67e0025751bdc4765d037a95242c8fd35fe3fdbfe`.
The corresponding [readable summary](../fixtures/memory-stage5-integrated/heldout/v2/post-run-diagnostics/heldout-v2-final-gate-summary.md)
is supplementary reporting, not a replacement evaluator. The frozen source-audit,
quality and resource report hashes are respectively
`204af7798748d28d4ba1911bd59102fca8be48244982424fec27472f24cb9ad3`,
`e9d010619277dedb925ba3183d01208416205031a1104bbc8fa4efda48500a12`, and
`8af85060b5e6f9d963c9067c9768062e0eb3b51de572e9eb76a95ad467767976`.

Every failed or incomplete gate is listed below. Unitless fractions are in [0,1];
`work_p95` uses nanoseconds. Aggregate and repeated checks retain their original
IDs rather than being collapsed into a falsely smaller failure count.

| Exact gate ID | Status | Actual | Required |
| --- | --- | ---: | ---: |
| `reader:selected_test` | fail | false | == true |
| `reader:exit_code` | fail | 1 | == 0 |
| `evidence:all_boundaries_and_denominators` | fail | 20 | == 0 |
| `quality:all_assessments_valid` | fail | 14 | == 0 |
| `quality:all_quality_gates_pass` | fail | false | == true |
| `quality:required_denominators` | fail | 2 | == 0 |
| `quality:None:all_planned_assessments_valid` | fail | 142 | == 144 |
| `quality:None:evidence_audit_violations` | fail | 10 | == 0 |
| `quality:automatic_deeper:supported_answer_component_recall` | fail | 0.8333333333333334 | >= 0.85 |
| `quality:automatic_deeper:personal_proposition_grounding` | incomplete | null | >= 0.95 |
| `quality:automatic_deeper:original_event_citation_accuracy` | fail | 0.8620689655172413 | >= 0.95 |
| `quality:automatic_deeper:role_passes:critical_semantic` | fail | 13 | == 15 |
| `quality:automatic_deeper:family_semantic_pass:graph_and_dense_paraphrase` | fail | 0.0 | >= 0.5 |
| `quality:oracle:supported_answer_component_recall` | fail | 0.8888888888888888 | >= 0.95 |
| `quality:oracle:original_event_citation_accuracy` | fail | 0.8928571428571429 | >= 0.95 |
| `quality:oracle:role_passes:critical_semantic` | fail | 12 | == 15 |
| `quality:None:silent_conflict_resolutions` | fail | 1 | <= 0 |
| `quality:hard_zero:silent_conflict_resolutions` | fail | 1 | <= 0 |
| `local:selected_test` | fail | false | == true |
| `local:exit_code` | fail | 1 | == 0 |
| `local:hold15_archive_historical-oracle:boundaries` | fail | 60 | == 0 |
| `local:hold15_archive_historical-oracle:runtime_accounting_samples` | incomplete | null | == 20 |
| `local:hold15_archive_historical-oracle:work_p95` | incomplete | null | <= 3000000000 |
| `index:selected_test` | fail | false | == true |
| `index:exit_code` | fail | 1 | == 0 |
| `index:hold15_archive_historical:boundaries` | fail | 14 | == 0 |

The evidence aggregate's 20 entries repeat the ten source-audit violations in
two formats; they are not 20 independent source incidents. The local boundary
count of 60 is three checks per failed hold15/oracle turn. The index count of 14
is multiple failed-turn, payload and agreement checks for one case. A false
`selected_test` value accompanies an executed test's failure; it does not mean
that test was never run. The generic `required_denominators` message identifies
the two invalid hold12 assessments; all 144 files are actually present, and no
denominator was removed or rewritten.

## Preservation and historical reproduction limits

The held-out preservation driver maps the exact frozen reports to
`../fixtures/memory-stage5-integrated/runs/heldout-v2/reports/` and retains
original execution records under `commands/`. It captures the supplemental
ledger and diagnosis in `raw-verification.tar.gz`; the separate directly linked
post-run copies above have their own hash manifest. Raw reader, source-audit,
assessment, local, index and operating archives retain failed artifacts alongside
successful ones. The preservation records, rather than renamed absolute paths,
define the copy and reconstruction layout.

Earlier development failures also remain evidence. Integrated development v1
retains 320/2,880 failed local turns and 8/24 failed index cases; no integrated
reader generation ran for that version. Its cold/warm and context diagnostics
are supplemental, not replacement cohorts. Development v2's genuine quotation
errors and frozen-evaluator disagreements remain failed; passing v3 and v4
results do not overwrite them. The separate held-out v1 preparation failure
above is not an executed held-out quality cohort.

Historical source retention has explicit limits. The failed #159 Qwen reader
v1–v3 source versions have four unique unavailable source versions; their exact
executables cannot be claimed rebuildable. The selected configured-reader v4
source inventory is complete. See the
[reader source supplement](../fixtures/memory-stage5-reader/source-preservation/v1/README.md).
The original #164 graph v1 and v2 recovered only 296/300 and 299/301 frozen Go
files. Their missing automatic-draft files, and two additional v1 graph files,
remain missing; their original freezes also did not bind the full transitive
dependency closure. The
[graph source record](../fixtures/memory-stage5-graph/reproduction/README.md)
retains exact hashes and forbids substituting a later source tree. These gaps
limit reproduction of those historical experiments; they do not describe the
394-file exact compiled-source archive used by the integrated v4/held-out run.
No new build at another path is asserted to reproduce an old executable digest.

## Repository verification and handoff records

The final [handoff verification record](../fixtures/memory-stage5-integrated/heldout/v2/final-verification-v2/handoff-verification.json)
contains exact commands and hashed logs after the browser fixture was added.
`./scripts/verify-change.sh` passed from the repository root on
`2026-09-11T07:55:58.601928Z` through `07:56:16.411781Z` (17.809853 seconds).
The Go test and vet suites, UI lint/build and whitespace checks exited zero.
`cmd/evie` tests took 12.523 seconds; other Go package results were cached.
Five existing Fast Refresh warnings and Vite's chunk-size warning above 500 kB
remain recorded. No required check was waived because held-out gates failed.

The eight complete public-turn/HTTP demonstration tests all passed with
`-count=1` from `07:56:16.412596Z` through `07:56:17.855959Z`. The seven selected
UI files passed 23 tests, with zero failed or pending and all 14 required
assertions present, from `07:56:17.856589Z` through `07:56:18.508125Z`.
All 394 frozen compiled inputs remain unchanged. The ordinary Go run skips the
interactive browser fixture unless explicitly enabled; that walkthrough is
recorded separately and is not inferred from the deterministic tests.

The root agent also completed all eight scenarios through the real browser UI.
The [browser demonstration record](../fixtures/memory-stage5-integrated/heldout/v2/browser-demonstration/README.md)
retains 11 complete accessibility observations and eight screenshots, plus exact
SQLite/request artifacts and the closed live-run metadata. This was an
agent-operated manual walkthrough with a scripted provider, not human review or
additional model-quality evidence. Its larger scripted CLI context profile is
separate from the frozen evaluation profile. The fixture exited zero after the
operator stopped it; its 1,576.97 seconds include browser wait and are not a
latency measurement.

| Required demonstration | Actual browser observation |
| --- | --- |
| Fresh-chat preference | Sent the ordinary dinner question in an empty reader and inspected its saved dietary Claim and original owner source; one new answer and request snapshot were persisted. |
| Uncompiled original statement | The fern source appeared as an attributed Conversation Excerpt with its original event and exact locator. |
| Cross-topic reference | After a parser detour, the birthday answer's source inspector showed Maya's accepted preference. |
| Bounded investigation | Request 2's original match and request 3's neighboring context were separately inspectable; the possible visit stayed tentative. |
| Historical conflict | The inspector showed conflicting accepted residence entries, a newer owner statement, and historical retired workplace evidence with its valid interval. |
| Retirement suppression | Compared the earlier original receipt with a fresh answer after retirement; the retired instruction was omitted and an unrelated notebook statement remained available. |
| Original sources after correction/restart | After approved correction, source retraction and actual SQLite close/reopen, the original azurefolio version remained inspectable with current superseded status, while restricted embermanifest evidence was unavailable. |
| Memory unavailable | The self-contained rewrite continued and the personal-memory question reported unavailability; both persisted opt-out receipts contained no supplied references. |

The live provider was scripted and made no actual model requests. Successful
presentation and provenance checks do not change the failed held-out decision.



The final check with all #168 artifacts staged also passed: `./scripts/verify-change.sh`
ran from `2026-09-11T08:03:53.717443Z` to `08:05:16.049754Z` (82.332311 seconds).
Its [closed execution and exact compressed log](../fixtures/memory-stage5-integrated/final-staged-verification/)
retain the same warnings. Two prior hashed raw Vite logs are marked binary using
narrow `.gitattributes` entries, preserving their reporter whitespace; maintained
source still passes staged, unstaged and whole-branch whitespace checks.

Preservation verified 2,438 original entries and 23 repository references. The
[transport verification](../fixtures/memory-stage5-integrated/runs/transport-verification/heldout-v2-transport-result.json)
also verifies the complete 394-file source and 48-file seed/map archives without
claiming that Git preserves direct-file timestamps.

The final parallel [Standards and Spec review](memory-stage-5-final-review.md)
covered all 13 commits from the user-confirmed `f27546d` baseline. Both axes
reported zero additional findings. The Spec reviewer independently passed the
focused agent/web boundary selection (3.331 seconds / 1.124 seconds) and the
whole-branch whitespace check. These checks do not change release readiness.

## Follow-up work proposed by the failures

These are concrete follow-up candidates, not authorized scope additions or
claims of implemented fixes. Any chosen change needs its own review and a newly
frozen, separately authorized assessment; this corpus is now observed evidence.

- Define current-only versus unconditional text prohibitions and reject
  contradictory historical gold annotations during fixture validation. Preserve
  hold15's original annotation, prepared payloads and failed gates.
- Separate semantic support for an original owner statement from retrieval credit
  for an accepted Claim. A future provenance rule must accept an independently
  verified original excerpt for the former without inventing the latter, with
  explicit positive and negative checks for the hold12 distinction.
- Investigate missing accepted paraphrases, newer owner reports and original
  neighboring owner context using the retained hold12, hold17, hold18 and hold20
  traces. Preserve scope, authority, temporal filters and existing bounds.
- Improve reader citation completeness for multi-source answers and prevent a
  newer owner report from silently resolving still-active accepted conflicts.
  The oracle hold03/hold11 citation omissions and hold17 resolution error need
  reader-level regressions; complete retrieval alone did not prevent them.
- Preserve the full source/dependency inventory before any future experiment,
  and verify it at freeze time. Historical missing bytes must remain explicit,
  rather than being filled with later code under an old experiment identity.

The declaration remains **not ready**. Passing deterministic checks, preserved
artifacts and successful individual demonstrations do not change that decision.
