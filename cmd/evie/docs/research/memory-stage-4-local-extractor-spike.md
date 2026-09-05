# Stage 4 standalone local extractor spike

Status: 110 bounded comparison requests finished for [ticket #135](https://github.com/davidadel66/evie/issues/135);
human output judgments remain pending. **No demonstrated usable configuration.**
No production extractor has been selected; model selection remains blocked. Final holdout contents remain
uncreated and unexposed in this tuning task. This report records standalone
behavior; it supplies no hidden dependency on the later production compiler.

## Corpus and human review

The [synthetic review packet](../fixtures/memory-stage-4-spike/v1/review-packet.md)
contains 24 windows from 12 whole narrative families: 18 development windows in
N01–N09 and six pilot windows in N10–N12. David approved its proposed labels; the
[annotation record](../fixtures/memory-stage-4-spike/v1/annotation-record.json)
pins the exact reviewed source/gold/packet hashes and approval provenance. The
frozen packet/gold preserve their earlier proposed-status text; the subsequent
annotation record is the approval authority. No later interpretations inherit
that approval automatically. Gold and review artifacts are separate files from
the source-only extractor input.

The starting corpus covers standing preferences without a remember instruction,
true incidental facts to omit, unendorsed reports/fiction, explicit endorsement,
short assent with exact assistant context, ambiguous identities, project choices
and optional unadopted considerations, world changes versus corrections,
relative/future dates, contracted clock lineage, failed/crashed closure,
Unicode source coordinates, excluded secrets/undefined tools, hostile quotations,
accepted identity context, and session separation. All narratives are synthetic.
Twenty-four windows are a bounded development choice, not a statistical sample
or a claim that every natural conversation pattern is represented.

The exploratory comparison selects ten windows:
N01-a, N01-b, N02-a, N02-b, N03-b, N04-b, N05-b, N06-a, N08-b, N09-a.
This exercises useful/optional/no-memory selection, interpretation context,
identity ambiguity, relative/clock time, overlap ownership and failed closure.
Eight development windows and all six pilot windows remain unexecuted. A favorable configuration must cover those remaining families before a
model-selection conclusion. No final-holdout case, gold, output or feedback is
available here; [custody protocol](../fixtures/memory-stage-4-spike/v1/holdout-custody.md)
requires separate curation and exposure logging.

## Configurations and evidence

The initial two-format comparisons use cached Mistral 7.2B Q4_0 under Ollama 0.6.3 on an Apple M3 Pro with
18 GiB physical memory. Actual model/runtime/template artifact hashes are in the
[runtime manifest](../fixtures/memory-stage-4-spike/v1/runtime-manifest.json).
The model GGUF is 4,113,289,152 bytes. No runtime upgrade was performed. The later authorized Qwen acquisition is
recorded separately below. The inference executable reads synthetic files only and calls the
owned loopback HTTP server with proxies disabled, no redirects, no remote
fallback, and a single request at a time. No Evie database or memory write path
is opened.

The [original experiment manifest](../fixtures/memory-stage-4-spike/v1/experiment-manifest.json)
records two output-format configurations: closed schema grammar and JSON mode.
Both use context 4096, maximum output 768 tokens, temperature zero, seeds 17/18,
a 60-second request timeout, and two repetitions. All 40 planned requests ran.
The original manifest was recorded at 02:55:09 UTC, after the first schema run
started at 02:50:39 UTC. It records identities/configuration; it is not evidence
that the manifest file existed before that request. The original token budgets
were also derived retrospectively. Those 40 requests were not protected by the
later full-batch predispatch guard.

Within each arm every pair of repetitions produced byte-identical raw output;
different seeds at temperature zero supplied reproducibility evidence, not
diverse samples.

A concrete prompt confound was found: neither original arm saw the complete
schema as prompt text, and JSON mode received no closed field contract. Ollama
uses `format` as a separate decoding constraint. The policy prompt also omitted
some field semantics, including subject versus scope, text versus Entity objects,
and fact versus decision. These omissions plausibly contribute to typed errors;
they do not excuse unsupported selection or establish model adequacy.
([Runtime prompt construction](https://github.com/ollama/ollama/blob/v0.6.3/server/routes.go#L241-L292),
[format handling](https://github.com/ollama/ollama/blob/v0.6.3/llm/server.go#L625-L661))

The [corrected experiment manifest](../fixtures/memory-stage-4-spike/v1/experiment-manifest-v2.json)
freezes a new 5588-byte common prompt: original policy plus explicit field
semantics and the unchanged minified schema. Both arms receive identical system
text. Context is 8192, output remains 768, and the same ten cases run once per
arm with seed 17/temperature 0. This also changes context, so it evaluates corrected
configurations rather than isolating the prompt's causal effect. One repetition
per corrected arm is an explicit limitation. The
[input-budget proof](../fixtures/memory-stage-4-spike/v1/input-budget-proof.md)
records the independently inspected pinned SPM/tokenizer/template bound;
all selected requests must fit before the first dispatch.

## Original comparison observations

| Arm | Requests | `ok` status | Whole-response shape errors | Truncated output | Complete raw JSON proposals | Structurally retained |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Schema | 20 | 18 | 0 | 2 | 34 | 20 |
| JSON | 20 | 10 | 8 | 2 | 32 | 0 |

An `ok` status means the response passed the whole-response structural shape
check, not that every candidate passed every schema/value/source check or that
its meaning was correct. Individual candidates can still be rejected. Raw counts
include proposals in JSON-decodable responses with shape failures: the runner's
legacy `raw_count` omits some such objects, so the scorer reopens raw JSON and
uses 34/32 here, rather than the runner's 34/20.
Malformed/truncated outputs remain failed attempts in required-recall
denominators; incomplete JSON is not repaired or salvaged. Planned unexecuted
cases do not enter measured recall as invented runs. The six required cases in
the selected set, repeated twice, give 12 required meanings per arm. Both arms
have zero exact approved-gold matches, raw and retained; unmatched meanings
await the separate human judgments below.

The original reports are immutable. Enum membership, null reference shape,
complete clock ancestry, all-request context preflight, and scoring provenance
were hardened after those runs. Corrected offline validation preserves original
raw bytes, request hashes and latencies, records the original report hash and
new validation identity, and does not pretend to rerun inference. Retained
counts above are unchanged by those corrections. Source retention is not
entailment or acceptance.

The [output adjudication packet](../fixtures/memory-stage-4-spike/v1/output-adjudication-packet.md)
deduplicates 66 complete raw proposals into 33 judgments; each occurred twice.
Human output review is pending. Its three proposed useful interpretations
(O12, O14 and O22) all need encoding repair. Even if human review grants raw
semantic credit, none has typed/Predicate agreement and that credit cannot
establish production adequacy. All unlisted meanings remain unadjudicated.

| Arm | p50 request latency | p95 request latency | Maximum | Max reported prompt/output tokens |
| --- | ---: | ---: | ---: | ---: |
| Original schema | 18.574 s | 31.098 s | 32.778 s | 1264 / 768 |
| Original JSON | 21.231 s | 32.025 s | 32.106 s | 1264 / 768 |

Quantiles match the scorer: sort n latencies and choose zero-based index
`floor((n−1)×q)`, without interpolation, for q=0.50/0.95. This lower order
statistic is not a middle-pair average. Timings include all attempted requests
and failures. Arms ran sequentially with different load/cache and shared-host
conditions; these tiny samples do not establish a population tail or isolated
format-speed effect. The separate initial
schema smoke completed in 25.650 s including 11.342 s reported model loading;
its incorrect subject/type/kind showed why citation correctness alone does not
establish useful memory.

## Corrected Mistral observations

| Arm | Planned/attempted/unexecuted | `ok` / shape error / truncated | Complete raw JSON proposals | Retained | Exact gold matches raw/retained |
| --- | --- | --- | ---: | ---: | --- |
| Corrected schema | 10/10/0 | 9/0/1 | 19 | 8 | 0/0 |
| Corrected JSON | 10/10/0 | 9/1/0 | 21 | 8 | 0/0 |

Both have six attempted required opportunities. Corrected schema truncates on
N02-a; corrected JSON fails whole-response shape on N08-b. The legacy runner
raw_count sums 19/18; the scorer's 19/21 counts all complete candidate objects.
All 40 raw occurrences remain subject to 23 deduplicated proposed judgments in
the [separate corrected packet](../fixtures/memory-stage-4-spike/v1/corrected-output-adjudication-packet.md).
There are no confirmed exact matches; this does not assert a final human 0%
usefulness rate. Proposed optional raw neighbor credit still has an identity
encoding error. No broader Mistral run was performed after these failures.

Despite explicit field guidance, personal assertions still use the scope UUID
as subject; comparative preference/context, employment change/relative time and
the coffee/clock meaning are lost. The schema-format p50/p95/max is
24.638/35.095/35.411s; JSON is 23.733/32.914/35.370s. Prompt counts range 1564–2032.
No extra inference was used for offline rescoring, and no truncated JSON was
repaired. Corrected arms have one repetition; their 7/10 byte-identical cross-arm
outputs are not a within-configuration reproducibility estimate.

## First alternate model

The authorized next experiment acquires only `qwen2.5:7b-instruct-q4_K_M` and
keeps Ollama 0.6.3. Actual 4,683,073,952-byte weights and every small manifest layer
were hashed. The [Qwen runtime manifest](../fixtures/memory-stage-4-spike/v1/qwen/runtime-manifest.json)
pins executable/libraries, stored template/default system, model identity and
explicit `OLLAMA_NEW_ENGINE=false` launch. It has a separate verified
[BPE/merge-closure and exact-template input budget](../fixtures/memory-stage-4-spike/v1/qwen/input-budget-proof.md);
Mistral's token counts/proof are not reused.

The [Qwen experiment manifest](../fixtures/memory-stage-4-spike/v1/qwen/experiment-manifest.json)
freezes the same common prompt/schema, context 8192/output 768, schema format,
temperature 0, seeds 17/18, and the same ten windows×two repetitions. At most 20
quality requests run; any non-ok load/protocol response stops the batch while
preserving its failed attempt and unexecuted plan. The first request is the load
smoke. [Observed engine evidence](../fixtures/memory-stage-4-spike/v1/qwen/reports/first-load-engine.json)
confirms the legacy llama.cpp-backed Go runner, parallel 1 and native context 8192.
A successful load establishes protocol feasibility, not extraction adequacy.
All 20 requests completed with whole-response status `ok`: 30 complete raw
proposals, 16 retained, and 4 exact required matches over 12 attempted required
opportunities. Those matches are the two personal preferences N01-a/N09-a,
each repeated twice. N01-b correctly abstains twice. Fourteen proposals fail
individual checks. The [initial Qwen score](../fixtures/memory-stage-4-spike/v1/qwen/reports/development-initial-score.json)
keeps 13 novel objects/26 raw occurrences pending in the
[separate Qwen review packet](../fixtures/memory-stage-4-spike/v1/qwen/output-adjudication-packet.md).
Current confirmed lower bounds are 4/30 raw useful precision, 4/16 retained useful
precision and 4/12 required recall; these are not final rates or statistical
confidence intervals. There are two confirmed retained omissions on N08-b.

The measured improvement on short preferences does not resolve the failure
slices: unendorsed-story residence, literal `new:Name` instead of Maya,
project-scope subjects for personal assertions, an affirmed employment claim
after explicit departure, and the coffee habit/time/source mismatch. No broader
Qwen development or pilot run was performed. Proposed optional raw PostgreSQL
credit still requires human judgment and includes an encoding defect.

All ten repeated case pairs are byte-identical, not independent error samples.
All-request p50/p95/max is 14.137/36.224/36.411s under the same lower-order-statistic
method. The first request took 20.229s, including 5.199s model load; reported prompt
counts span 1361–1802 and outputs 6–755. There was no whole-response truncation,
timeout or input-truncation log. This is feasibility/measurement evidence,
not evidence of model-family superiority or production release gates.

## Resources and capacity release

The original schema arm took 411.726 s including sampling overhead; JSON took
436.896 s. Maximum sampled owned server/child RSS was 3,432,752 KiB (schema) and
3,362,128 KiB (JSON). The standalone Go runner maxima were 8,016 and 8,960 KiB.
These are separate process observations, not GPU/unified-memory totals or a
production Evie foreground benchmark. Host swap was already 6808.25 MiB at the
first comparison baseline, reached 7414.25 MiB at the next arm's baseline, and
ended at 7126.25 MiB. This shared host has other workloads; changes cannot be
attributed to the experiment alone.

Before the corrected pass, read-only runtime/model/API verification and resource
observations were frozen in
[corrected-pass preflight](../fixtures/memory-stage-4-spike/v1/reports/corrected-pass-preflight.json):
12,342,284 KiB disk available, host swap 7030.25 MiB and macOS memory-pressure
free percentage 56%. Those host measurements are observations, not guarantees
of available model memory. Resource sampling continued during both arms. The
corrected-pass preflight briefly read the entire approximately 4 GB model file
into memory before timed inference; subsequent hashing streamed the file. That
prior allocation limits a clean resource comparison, and five-second RSS
sampling can miss transient peaks.

The completed [corrected schema resource record](../fixtures/memory-stage-4-spike/v1/reports/corrected-schema-resources.json)
and [corrected JSON resource record](../fixtures/memory-stage-4-spike/v1/reports/corrected-json-resources.json)
contain 52 and 48 five-second observations, respectively, plus baseline/final
snapshots. Both measurement commands exited 0; their wall time includes sampling
and control overhead, and does not imply every model output was valid.

| Corrected arm | Server-family RSS baseline / peak / final MiB | Standalone Go-client peak MiB | Host used swap baseline / peak / final M | Measurement wall time |
| --- | --- | ---: | --- | ---: |
| Schema | 17.125 / 1470.781 / 1456.125 | 12.531 | 7014.25 / 7268.12 / 7260.12 | 261.177 s |
| JSON | 1365.609 / 1365.609 / 1070.328 | 17.938 | 7180.12 / 7852.12 / 7748.12 | 241.206 s |

Peaks include baseline and final snapshots; the corrected JSON server peak is
its baseline. RSS is not a complete Metal/unified-memory footprint, and host
swap remains unattributed shared-host activity.

An actual local request was cancelled by a 500 ms client deadline and returned
in 502.494 ms with no retained output. Request-specific server release remained
unknown; the runner stopped after that single request, despite two planned
repetitions. No second inference probe was sent. The experiment-owned server
and observed runner were verified absent, then a new owned server was started
on the same loopback origin. The original server exited in 0.147 s; the observed
runner had already exited before restart. The
[capacity-release record](../fixtures/memory-stage-4-spike/v1/reports/capacity-release.json)
pins the cancelled report and process observations. `/api/ps` alone is never
used as evidence that no requests remain. This recovery behavior follows the
separate capacity-release contract without implementing its production ledger.

Qwen's 69 five-second samples plus baseline/final observations give server-family
RSS 26.391/4487.062/4279.922MiB (baseline/peak/final), standalone Go-client peak
10.828MiB, and host used swap 7516.06/8789.94/8685.88M. Measurement wall time was
346.754s. The [complete resource record](../fixtures/memory-stage-4-spike/v1/qwen/reports/development-resources.json)
keeps per-process observations separate. Runtime planning reported 5.6GiB full
allocation and 448MiB KV; these are engine estimates and must not be added to RSS
as disjoint measured memory. Startup probes reported unavailable x86 CPU `.so`
backends; Metal subsequently loaded 29/29 layers and all requests completed.
The saved [runtime observations](../fixtures/memory-stage-4-spike/v1/qwen/reports/runtime-observations.json)
record those messages.

Qwen had no actual in-flight cancellation trial; the Mistral cancellation
measurement is not presented as Qwen-specific. All 20 Qwen requests had completed
before [owned-server shutdown](../fixtures/memory-stage-4-spike/v1/qwen/reports/owned-shutdown.json).
Current owned PIDs exited, the previously observed runner was absent, and the
controlled termination observation took 0.138s. All experiment-owned Ollama
servers are stopped; downloaded/cached model artifacts remain intact.

## Verification and outcome before compact

The [runner documentation](../../../../scripts/memory-extractor-spike/README.md)
contains reproduction commands, bounds, scoring interpretation and revalidation
lineage. The original and alternate-model verification records below predate the compact transport; the additional compact verification is recorded in its section. Verification passed on those executable changes:

- `go test ./scripts/memory-extractor-spike -count=1`: passed; final focused run 4.624s. Public tests cover the executable/HTTP seam, source/context/lineage boundaries, enum/null regressions, refusal of nonlocal/redirect/proxy endpoints, timeout/release behavior, full-batch preflight, exact Qwen 1690/1691-byte boundary, and approval/evidence binding.
- `python3 -m py_compile scripts/memory-extractor-spike/measure.py scripts/memory-extractor-spike/freeze_budget.py scripts/memory-extractor-spike/inspect_qwen_gguf.py`: passed.
- `./scripts/verify-change.sh`: passed, exit 0, including full Go tests/vet, UI lint/typecheck/build and staged/unstaged whitespace. Existing unrelated UI warnings remain: `Icon.tsx:16` `react(only-export-components)`, and Vite chunks larger than 500 kB. No UI fix was made.
- New-file whitespace and local Markdown targets were separately checked because Git's normal diff does not inspect untracked additions. All owned files pass whitespace checks and all local targets resolve. Exactly one extra EOF newline was removed from each of two pending review packets; the [formatting revision record](../fixtures/memory-stage-4-spike/v1/output-packet-formatting-revision.json) preserves their original bytes, proposed records, and old/new hashes. Proposed records changed only their packet hashes; judgments and approval status remain unchanged. The pre-existing 220-path preservation audit passes; index remains empty.
- Independent Standards and Spec reviews, including focused Qwen/stop-on-failure and artifact-proof rechecks, report no actionable findings.

[Verification evidence](../fixtures/memory-stage-4-spike/v1/reports/verification.json)
records command outcomes and the full normalized verification log. There are no
skipped required repository checks. No production foreground benchmark,
owner-review burden measurement, integrated pilot, or final-holdout exposure was
performed; those are later work and cannot be inferred from standalone RSS or
latency.

Before the compact run, the outcome was **no demonstrated usable configuration**, not a forced
winner or a general rejection of either model family. Human output adjudication
was outstanding for all three pre-compact packets; proposed labels have not been applied to
reported quality/error totals. Eight development windows, the N07 family, all
six pilot windows and final holdout are unexecuted. A future candidate needs
appropriate coverage and actual human judgments before selection; no integrated
pilot/release threshold is invented here.

The 80-request snapshot motivated the separate compact experiment below. Its
chronological fields, short aliases and explicit subjects were a transport
hypothesis, with unchanged memory meaning/provenance requirements. The completed
compact measurement brings the actual comparison total to 90; the separate
Mistral cancellation trial and metadata-only server starts are not quality
comparison attempts. #135 still awaits actual output adjudication, and #136 has
no selected extractor to implement from these results.

## Separate compact ordered wire experiment

The [compact experiment manifest](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/experiment-manifest.json),
[sealed predispatch request plan](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/preflight.json),
[input proof](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/input-budget-proof.md)
and [execution plan](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/execution-plan.json)
were saved before its ten inference requests. The same approved source/gold and
cached Qwen artifact/runtime were retained. Chronological unchanged fields,
short sealed source aliases and explicit subject fields change the complete
wire/prompt/schema configuration; this does not isolate alias length as a cause.
All prior 80 outputs, reports, manifests and three pending judgment packets
remain unchanged.

This batch used the same ten windows once, seed 17, temperature zero, schema
mode, context 8192, output cap 768 and 60 seconds per request. The maximum complete
conservative predispatch bound was 7171 of 8192 including output/reserve. All ten
requests finished with request-specific `done`; none timed out, truncated or
failed whole-response shape. There were no retry, repair or probe generation
requests. [The recorded output](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development.json)
contains 12 decodable raw wire objects and **zero retained candidates**. The
[initial score](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development-initial-score.json)
assigns zero automatic exact canonical matches and six confirmed retained
required omissions. Raw meaning judgments for all 12 objects remain pending;
raw required omissions are not yet confirmed while those judgments are unknown.
Retained precision is undefined with no retained proposals, not zero precision.

The matched seed-17 comparison uses one request per case from the original Qwen
report, alongside its full repeated-run totals for context:

| Qwen configuration | Attempted / planned | Raw / retained proposals | Exact required matches / opportunities, raw and retained | p50 / p95 / maximum seconds |
| --- | --- | --- | --- | --- |
| Prior canonical, all seeds 17/18 | 20 / 20 | 30 / 16 | 4 / 12 in each panel | 14.137 / 36.224 / 36.411 |
| Prior canonical, seed 17 only | 10 / 10 | 15 / 8 | 2 / 6 in each panel | 16.846 / 34.731 / 36.411 |
| Compact, seed 17 | 10 / 10 | 12 / 0 | 0 / 6 in each panel | 6.915 / 18.400 / 18.681 |

These are descriptive configuration comparisons on the same small development
set. Lower latency does not compensate for unusable candidate references. The
experimental schema has a concrete design flaw: it makes `selector`, `start`
and `end` independently optional, so the dangling-start combinations are
schema-valid even though the prompt and adapter reject them. The grammar did
not enforce its own intended whole/date-versus-complete-range alternatives.
This confounds the transport comparison; zero retention is not evidence that
all 12 underlying meanings failed, or a fair rejection of compact transport
or the model family. Every
compact proposal supplies a `start` coordinate without a complete valid `range`
selector. The frozen contract permits omitted/whole selectors only with no
coordinates, so none is silently treated as a whole-field citation. Ten first
rejections are `invalid_selector`; two first fail the subject/identity check.
These are first-rejection counts, not an exhaustive count of defects: all 12
raw objects have the malformed reference combination. Every alias itself is
known and every returned window ID matches. N08-b also puts the tool clock alias
in assistant context and includes coordinates on its named date selector.

No literal `new:Name` placeholder appears, but N02-b assigns Maya's residence to
`owner`, and N04-b similarly substitutes the owner for ambiguous `She`. N01-b
repeats an old tea preference instead of abstaining, N03-b loses the
tea-over-coffee qualification and question context, N05-b repeats overlap-only
SQLite/offline claims, and N06-a affirms a generic employment object after the
explicit departure. These exact-output observations do not finalize human
semantic error totals. The new [12-object review packet](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/output-adjudication-packet.md)
keeps proposed meaning/usefulness judgments separate from the scored report.
Possible raw credit for tea, the café wording, the explicit dated coffee-change
wording and the PostgreSQL consideration
requires actual human judgment; it cannot repair references or create retention.

The source map retains original coordinates, exact text/hash, role/authority,
new/overlap/context ownership, observed time and window/policy identity. The
scorer regenerates it from the approved source corpus before validating recorded
requests, raw output and canonical expansions. Wrong response binding prevents
any expansion. New names remain unresolved and accepted aliases bind only their
offered IDs; these ten cases have no accepted alternatives, so that conversion
has deterministic test coverage only. No source/gold alteration or post-hoc
transport repair is part of this measurement.

[Resource sampling](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development-resources.json)
recorded 19 five-second observations plus baseline/final snapshots. Server-family
RSS was 28,112 / 3,381,744 / 1,643,104 KiB at baseline/peak/final; standalone Go
client peak was 10,880 KiB. Used host swap stayed at 8253.88 M in sampled records.
Measurement wall time was 95.533 seconds including sampling/control overhead.
Prompt counts were 1154–1286 tokens, outputs 136–373. The first request's API load
duration was 4.682 seconds. [Engine evidence](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/first-load-engine.json)
confirms the pinned legacy runner and 8192 context/KV with Metal. RSS excludes a
reliable total of unified GPU allocations; host swap and cache conditions remain
shared-host observations, not an Evie foreground benchmark or a controlled
memory-efficiency result.

Before generation, two metadata-only starts were stopped because the observation
script initially compared raw arrays to frozen summaries and then sorted tensor
object keys differently. The exact raw API payload was saved; compact JSON in
API key order matches every original frozen array/tensor hash. Runtime/model
artifacts had already matched by streaming hash. The [recovery records](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/metadata-preflight-recovery.json)
and [second recovery record](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/metadata-preflight-recovery-v2.json)
record zero generation for those starts. The third owned server, PID 8268, served
the ten approved requests. [Shutdown evidence](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/owned-shutdown.json)
records all request-specific completions and verified owned-group exit in 0.033
seconds; its runner had already exited. Cached artifacts remain intact. No
actual Qwen cancellation trial is inferred from these preflight-only restarts.

The compact-focused `go test ./scripts/memory-extractor-spike -count=1` passed in
7.608 seconds. Root independently ran `./scripts/verify-change.sh`, exit 0,
including full Go tests/vet, UI checks/build and staged/unstaged whitespace;
the package run there took 8.187 seconds. Existing Icon Fast Refresh and Vite
large-chunk warnings remain. Independent Standards/Spec reviews found and
rechecked response-binding and explicit-selector regressions; their final
results had no actionable findings. The [separate verification record](../fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/verification.json)
preserves command/source identities and normalized log evidence. Tests use
loopback stubs and offline scoring; no live inference occurred in them.

This tested compact configuration provides no usable retained output. A next
bounded experiment should first encode closed reference alternatives directly
in its new schema: whole/date with no coordinates, or range with both start and
end. The separate compact-v2 measurement below implements this recommendation; it cannot repair
this run or substitute for separate semantic evaluation. All four human output
packets remain unapproved and unapplied. No broader development/pilot run,
production selection, release threshold or further model acquisition follows
from this result. #135 remains incomplete at the human-adjudication boundary;
#136 remains blocked.

## Closed-reference schema correction: measured compact-v2

The preceding compact-v1 result remains immutable. The separately pinned
[compact-v2 manifest](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/experiment-manifest.json)
corrects its generator-schema mismatch. Each supporting/context reference has
three closed alternatives: ref only, ref plus selector whole/date, or ref plus
selector range and both start/end coordinates. Only the schema and its embedded
prompt text change. The source projection, alias seals, identity/source checks,
response binding, scope/authority and raw scoring denominators remain intact;
no previous response is repaired or reinterpreted as retained.

The [offline runtime proof](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/grammar-proof/REPORT.md)
uses the unchanged JSON-schema converter and grammar parser/character consumer
from the pinned Ollama0.6.3 commit. All 62 full-output vectors passed. It confirms
closed unions, enum alternatives, required fields, integer minima and array
bounds. The 6,213-byte generated grammar fits the runtime's 32,768-byte buffer.
Generation follows schema property order, so the range branch emits
end/ref/selector/start; JSON-equivalent property permutations remain valid for
the adapter but are not all permitted by this runtime grammar. Cross-field range
ordering, exact Unicode/source boundaries, alias membership and semantic
meaning remain outside that grammar proof.

The [actual predispatch request plan](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/predispatch.json)
and [complete input proof](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/input-budget-proof.md)
cover the same ten source windows once. The maximum conservative context bound
is 7,763/8192 including the unchanged 768 output cap and 64 reserve. The natural
language prompt before its embedded schema is byte-identical to compact-v1.
The [current streamed file preflight](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/artifact-preflight.json)
and [API verification](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/runtime-observations.json)
match every pinned runtime/model file and metadata field. Array/tensor summaries
use compact JSON with the API's dictionary insertion order; they are not sorted.
No generation was issued during this preparation.

Focused verification passed: `go test ./scripts/memory-extractor-spike -count=1`
(9.965s), including the public CLI's mixed/tampered/relabelled configuration
failures, exhaustive selector/coordinate combinations in the emitted schema,
unchanged exact source seals, and retained/raw denominator boundaries. The
actual-runtime grammar checks pass 62 cases offline. Root ran
`./scripts/verify-change.sh`, exit 0, against these frozen spike sources before
the next production ticket began; its spike package run took 9.223s. Full Go
tests/vet and UI lint/typecheck/build passed, with existing Icon Fast Refresh and
Vite large-chunk warnings unchanged. `python3 -m py_compile` for the offline
grammar runner and `git diff --check` also passed. Both independent review axes
reported no actionable findings before dispatch. The
[verification record](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/verification.json)
preserves source identities, commands and normalized full-check log. No required
check was skipped for these executable changes; later report edits received
whitespace, links, data/hash and preservation checks.

All ten requests completed with request-specific `done`, whole-response status
`ok`, no timeout, no truncation and no input-truncation diagnostic. The
[exact run](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/development.json)
contains **16 raw objects and 3 retained candidates**. Every returned window and
source alias is known, and no coordinate combination is malformed. The first
rejections are ten `reference_category` failures and three `invalid_subject`
failures. These are first-rejection counts, not exhaustive defect totals.
Owner/tool fields are repeatedly duplicated into assistant context; two owner
preferences/habits are incorrectly marked unresolved, and Acme is emitted as an
unoffered accepted Entity alias. The strict adapter rejects these without repair.

The [initial score](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/development-initial-score.json)
finds one automatic exact match, the tea preference N01-a, in both raw and
retained panels: required recall lower bound 1/6, useful precision lower bounds
1/16 raw and 1/3 retained. Fifteen raw objects, including two retained, remain
unadjudicated. Three retained required omissions are confirmed: N02-b, N06-a
and N08-b. The other two unmatched required cases still have retained meanings
awaiting judgment. No raw omission is yet confirmed while relevant raw judgments
remain unknown. These are incomplete-label bounds, not final quality rates or
statistical confidence intervals.

The retained N03-b tuple cites the assistant question correctly but emits
`preference=tea` and `kind=decision`, losing the reviewed tea-over-coffee/fact
encoding. Retained N09-a preserves denied café wording and exact whole Unicode
source; equivalence to the reviewed café-plus-emoji target requires human
judgment. The new [15-object packet](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/output-adjudication-packet.md)
and hash-bound proposed record separate those judgments, optional PostgreSQL
raw credit and transient writing-intent interpretation from score totals. No
proposed label has been applied. All four earlier packets also remain unchanged
and pending, making five pending output packets after this run.

The [paired comparison](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/paired-comparison.json)
keeps the original Qwen repeated totals alongside the matched seed-17 subset:

| Qwen configuration | Attempted / planned | Raw / retained | Exact required matches / opportunities, both panels | p50 / p95 / maximum seconds |
| --- | --- | --- | --- | --- |
| Canonical, all seeds 17/18 | 20 / 20 | 30 / 16 | 4 / 12 | 14.137 / 36.224 / 36.411 |
| Canonical, seed 17 only | 10 / 10 | 15 / 8 | 2 / 6 | 16.846 / 34.731 / 36.411 |
| Compact-v1, seed 17 | 10 / 10 | 12 / 0 | 0 / 6 | 6.915 / 18.400 / 18.681 |
| Compact-v2, seed 17 | 10 / 10 | 16 / 3 | 1 / 6 | 8.489 / 19.346 / 20.530 |

The corrected reference grammar resolves the measured dangling-coordinate
failure, but the remaining protocol and meaning failures do not demonstrate a
usable extractor. The full schema-conditioned request changes model behavior;
this is a small descriptive configuration comparison, not proof of model-family
superiority, production adequacy or within-configuration reproducibility.

[Resource sampling](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/development-resources.json)
recorded 24 five-second samples plus baseline/final snapshots. Server-family RSS
was 24,864 / 4,423,232 / 4,159,040 KiB at baseline/peak/final; standalone client
peak was 9,840 KiB. Sampled used host swap stayed at 7869.88 M. Measurement wall
time was 120.641s including sampling/control overhead. Reported prompt counts
span 1284–1416, outputs 129–447; the first request's API load duration was 5.453s.
[First-load evidence](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/first-load-engine.json)
confirms the pinned legacy Go/llama runner, parallel 1, native context 8192 and
Metal offload 29/29. RSS is not total GPU/unified memory, engine allocation
estimates cannot be added to it as disjoint measurements, and host swap remains
unattributed shared-host activity. No production foreground benchmark follows.

The [shutdown record](../fixtures/memory-stage-4-spike/v1/qwen-compact-v2/reports/owned-shutdown.json)
records all ten request-specific completions, then verified exit of owned server
52517 and observed runner 58025 in 0.147s. The final resource sample preceded
shutdown. There was no retry, repair or probe inference, actual cancellation
trial, larger cap, model download or runtime upgrade. All 97 prior artifacts and
the original 90 requests remain unchanged; the comparison total is now 100.
No broader development, pilot or final-holdout exposure occurred. Source/gold
approval remains intact, human output adjudication is outstanding, and no
production extractor or release threshold is selected by this result.

## Measured compact-v3 category-schema configuration

The next [independent configuration](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/experiment-manifest.json)
restricts supporting references to the sealed new/overlap aliases and assistant
context to the separately sealed assistant aliases. No assistant fields means
required `const:[]` context. No source/gold, projection, subject union or semantic
boundary changes are included. Each actual request records its derived schema
and full-system hashes; offline scoring reconstructs both from the reviewed
source corpus and pinned base files before trusting recorded expansion.

The [frozen predispatch requests](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/predispatch.json)
cover the same ten windows once, with a maximum complete conservative bound of
7,750/8,192 at N03-b, including output 768/reserve 64. These are generated-system
bounds, not estimates from the smaller static prompt prefix. The
[pinned runtime proof](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/grammar-proof/README.md)
passes 177 dynamic category/empty-array cases and 62 inherited closed-reference
cases offline. It specifically confirms that `const:[]` works, while
`maxItems:0` without `items` is ignored by this runtime. Runtime property-order
and constant-spelling restrictions remain explicit generation limitations.

`go test ./scripts/memory-extractor-spike -count=1` passed in 9.872s. New public
regressions cover per-request schema/system identity, complete generated prompt
budgets, whole-batch preflight, source/context enum membership, empty constants,
unchanged strict adapter checks and response binding, and scorer refusal of a
more permissive schema even when its prompt and request hashes are recomputed.
All prior 142 fixture artifacts and 100 comparison attempts remain unchanged.

Both independent review axes passed without findings. The coordinated
`./scripts/verify-change.sh` passed, exit 0, before measured dispatch, including
full Go tests/vet and UI checks/build. Existing Icon Fast Refresh and Vite
large-chunk warnings remained. The [verification record](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/verification.json)
pins the exact spike sources and full normalized log; no required check was
skipped for these changes. Current streaming artifact hashes and the owned
server's version/template/model-info/tensor metadata all matched before
[dispatch](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/dispatch.json).
The frozen predispatch manifest was not rewritten with later observations.

All ten requests completed with whole-response status `ok` and request-specific
`done`. The [exact run](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/development.json)
contains **14 raw objects and 9 retained**. Every returned reference is known,
in the correct support/context category, and free of dangling coordinates.
The first rejections are four cases of no newly owned support and one
`invalid_subject` for an unresolved project/text PostgreSQL consideration.
These are first-rejection counts, not exhaustive semantic error totals.

The [initial score](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/development-initial-score.json)
finds only the N01-a tea preference as an automatic exact gold match: required
recall lower bound 1/6 in both panels, useful precision lower bounds 1/14 raw and
1/9 retained. Thirteen raw meanings, including eight retained, remain pending.
No required omission is confirmed yet because every unmatched required case
has an unadjudicated retained interpretation. This is incomplete-label
accounting, not a claim of zero actual omissions, final quality rates or
statistical confidence intervals.

Retained outputs include owner residence from fiction, Maya's endorsed
residence reassigned to owner, and ambiguous She reassigned to owner. N03-b
again emits tea/decision rather than the reviewed tea-over-coffee/fact tuple.
N06-a retains affirmative employment/work-at-Acme with last month instead of
encoding the explicit departure; a historical-employment reading versus the
required change is a human judgment question. N08-b retains a decision to stop
coffee dated from the clock while citing only the owner's relative-date text,
omitting the necessary clock support and changing completed-change encoding.
Structural retention does not establish that these interpretations are useful
or supported. No proposed semantic errors have been added to score totals.

The separate [13-object review packet](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/output-adjudication-packet.md)
and hash-bound proposed record keep these judgments pending. C05 exactly repeats
compact-v2 V07, and C13 preserves the earlier café/emoji equivalence question;
these are identified for review coordination, not automatically approved or
presented as wholly new questions. All five earlier packets remain unchanged,
making six pending output packets. No proposed labels have been applied.

The [paired comparison](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/paired-comparison.json)
uses the same selected case/seed requests and the same lower-order latency
statistics:

| Qwen configuration | Attempted / planned | Raw / retained | Exact required matches / opportunities, both panels | p50 / p95 / maximum seconds |
| --- | --- | --- | --- | --- |
| Canonical, all seeds 17/18 | 20 / 20 | 30 / 16 | 4 / 12 | 14.137 / 36.224 / 36.411 |
| Canonical, seed 17 only | 10 / 10 | 15 / 8 | 2 / 6 | 16.846 / 34.731 / 36.411 |
| Compact-v1, seed 17 | 10 / 10 | 12 / 0 | 0 / 6 | 6.915 / 18.400 / 18.681 |
| Compact-v2, seed 17 | 10 / 10 | 16 / 3 | 1 / 6 | 8.489 / 19.346 / 20.530 |
| Compact-v3, seed 17 | 10 / 10 | 14 / 9 | 1 / 6 | 7.426 / 14.169 / 15.718 |

The category schema removed the measured category failures and increased
structural retention. It did not increase automatic exact matches over v2.
This is a small descriptive comparison of complete schema-conditioned requests,
not evidence of adequate semantic extraction or within-configuration
reproducibility. Pending human labels prevent a final quality ranking.

[Resource sampling](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/development-resources.json)
recorded 20 five-second samples plus baseline/final snapshots. Server-family RSS
was 27,808 / 3,156,848 / 3,104,176 KiB at baseline/peak/final; standalone client
peak was 22,736 KiB. Sampled used host swap stayed at 7789.88 M. Measurement wall
time was 100.520s including sampling/control overhead. Reported prompt counts
were 1174–1410, outputs 129–368; first-request API load time was 2.410s.
[First-load evidence](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/first-load-engine.json)
confirms the pinned legacy runner, parallel 1, context 8192 and Metal offload
29/29. RSS is not total GPU/unified memory; shared-host cache/swap and different
load times limit resource comparisons. No production foreground claim follows.

After the final resource sample and all ten request completions,
[owned shutdown](../fixtures/memory-stage-4-spike/v1/qwen-compact-v3/reports/owned-shutdown.json)
verified server 77395 and observed runner 77814 absent in 0.145s. There was no
retry, repair, probe inference, actual cancellation trial, larger cap, model
acquisition or runtime upgrade. All 142 prior artifacts and 100 comparisons
remain unchanged; the total is now 110. No further configuration, broader
development/pilot/final-holdout run or production model selection is part of
this result. #135 still requires actual human output adjudication and a usable
configuration before selection; the deterministic compiler tickets are separate.
