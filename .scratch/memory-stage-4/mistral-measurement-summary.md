# Mistral standalone measurement summary

Read-only reporting for #135 from saved local artifacts, 2026-09-05 UTC. Only this scratch note was written. No inference, model loading, tests, human adjudication, owner-file changes, or commit was performed. Counts and quantiles below were recomputed with Python standard-library JSON parsing; cited report hashes and offline revalidation provenance were checked.

These measurements describe the pinned `mistral:latest` artifact (7.2B Q4_0, 4,113,289,152 weight bytes; full weight SHA-256 `ff82381e2bea77d91c1b824c7afb83f6fb73e9f7de9dda631bcdbca564aa5435`) under Ollama 0.6.3 on the busy 18 GiB Apple M3 Pro host. They do not establish the performance of other Mistral models, a production compiler, or accepted-memory retrieval.

## Comparisons and counting rules

Both passes used the same ten development windows: N01-a, N01-b, N02-a, N02-b, N03-b, N04-b, N05-b, N06-a, N08-b, N09-a. They cover eight of the nine development narrative families. Each traversal has six required gold opportunities and two optional expected meanings. Eight development windows, the entire N07 development family, and all six pilot windows were unexecuted by these Mistral comparisons. Final-holdout cases were not authored or exposed. This is subset development evidence.

The original pass requested each case twice, temperature 0, seeds 17 and 18, context 4096, output limit 768, and 60-second per-request timeout. The corrected pass requested each case once, seed 17 and temperature 0, with context 8192 and the same output/time limits. Both use one inference request at a time and an explicitly loopback endpoint. See the [original manifest](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/experiment-manifest.json) and [corrected manifest](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/experiment-manifest-v2.json). The original manifest timestamp (02:55:09 UTC) is after the original schema run started (02:50:39 UTC); its metadata is a recorded identity/configuration record, not evidence that that file existed before the first request. The corrected manifest precedes its first request.

**Raw proposals** below means every candidate in a JSON-decodable `candidates` array, including candidates from whole responses rejected by the closed-shape validator. **Retained** means the standalone shape/source checks retained the candidate. It does not mean the candidate is supported, useful, human-approved, accepted into memory, or exactly equal to gold. A run with status `ok` can have zero retained candidates. Truncated JSON remains an explicit failed run and is never salvaged.

| Configuration | Planned / attempted / unexecuted | Run statuses | Decodable raw proposals | Runner `raw_count` sum | Retained | Required opportunities | Exact gold matches, raw / retained |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| Original schema | 20 / 20 / 0 | 18 `ok`, 2 `truncated_output` | 34 | 34 | 20 | 12 | 0 / 0 |
| Original JSON | 20 / 20 / 0 | 10 `ok`, 8 `schema_error`, 2 `truncated_output` | 32 | 20 | 0 | 12 | 0 / 0 |
| Corrected schema | 10 / 10 / 0 | 9 `ok`, 1 `truncated_output` | 19 | 19 | 8 | 6 | 0 / 0 |
| Corrected JSON | 10 / 10 / 0 | 9 `ok`, 1 `schema_error` | 21 | 18 | 8 | 6 | 0 / 0 |

The `raw_count` column intentionally exposes a runner reporting distinction: original JSON contains 32 decodable candidates, but its per-run `raw_count` totals only 20; corrected JSON contains 21, but totals 18. The scorer reopens raw JSON so proposals in whole-response shape failures remain in the raw denominator. Do not use the runner total as the raw semantic-quality denominator.

The gold/source/review-packet hashes still match the [human annotation record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/annotation-record.json). The frozen gold file retains its pre-review status text; the separate exact-hash human approval supersedes that text. Required opportunities include attempted runs that fail parsing or retain nothing, but not unexecuted requests. An independent dictionary comparison of all meaning fields, scope, and normalized exact source/context references found zero exact listed gold matches in both raw and retained output for all four arms. This is **zero confirmed exact matches**, not a completed human judgment that every output is wrong or a final 0% usefulness/recall claim. Novel meanings remain pending.

The saved [original adjudication input](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/comparison-for-adjudication.json) has 33 unique case/proposal identities awaiting adjudication, representing 66 raw proposal occurrences. The [corrected adjudication input](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/corrected-comparison-initial-score.json) has 23 unique identities representing 40 raw occurrences. These are separate pass totals, not a deduplicated cross-pass total. Proposed adjudication files are not human approval; this note neither reads their proposed labels into the quality totals nor creates new judgments.

Original schema truncations are N08-b, both repetitions; original JSON truncations are N04-b, both repetitions. Corrected schema truncates N02-a; corrected JSON has a schema error on N08-b. These four case/configuration failure categories remain in the counts, with five truncated-output request occurrences across the two passes.

## Original confound and corrected configuration

The original prompt says a schema is supplied, but neither arm includes that schema in model-visible prompt text. Ollama passes `format` separately to decoding: the schema arm has a grammar constraint, and the JSON arm has no complete closed-schema specification in its prompt. Thus those original arm results do not isolate how well each format follows an equally visible output contract. They remain valid observations of the exact recorded configurations. The [read-only diagnosis](prompt-diagnosis.md) documents request construction and links to the pinned runtime source.

The corrected prompt adds a common field-contract glossary and the unchanged schema text to both arms. It is 5588 UTF-8 bytes; its SHA-256 is `f6f280093b15e1a1928db2737ec9bde8b2e0471ef66cbeab11de47b0ca57c832`, versus original `477dcc434de42bf38b88535036ea2dd9fcd820e4d890dfcb09f2b2ce9368d07a`. The output schema, source corpus, and human-approved gold are unchanged. Context also changes from 4096 to 8192, repetitions from two to one, and the executable includes validation/budget hardening. Therefore before/after differences are configuration observations, not an isolated causal estimate for prompt wording.

Original recorded prompt-token counts range from 796 to 1264; corrected counts range from 1564 to 2032. Corrected dispatch uses the pinned model-specific bound `full_rendered_utf8_bytes + 2 + 768 output + 64 reserve <= 8192`, including the exact 17-byte template overhead. See [input-budget proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/input-budget-proof.md). The original requests were later given empirical identity-pinned budgets; retrospective evidence must not be described as a predispatch guard used by the original binary. This bounded experiment proof does not supply a general production budgeting contract.

The original outputs were revalidated offline after enum, reference-scalar, and clock-ancestry hardening. In the `-v2` and later `-validated` reports, raw bytes, request identities, status, proposal arrays, retained counts, and measured latency are identical to the original runs. These derivative reports represent **zero additional model calls**. Later `-validated` files use validator code identity `028e08dfca7efc7edf05553c6515b460bca00ab98753448a2d5e4a30d1612ff8`, also recorded in the corrected inference reports. Original inference results must retain the original executable identity and original lack of the later predispatch guard.

## Latency and repetitions

Latencies include every attempted comparison request, including schema/truncation failures and load time. Units are seconds, rounded to three decimals from saved milliseconds. Quantiles match the offline scorer: sort all `n` observed latencies ascending and choose zero-based index `floor((n-1) × q)` for `q=0.50` or `0.95`, with no interpolation. This is a lower order statistic, not nearest-rank and not an average of the middle pair. For the ten-request corrected arms, p95 is the ninth observation; max is separately reported. These tiny development samples do not estimate a population tail or justify release gates.

| Configuration | n | p50 s | p95 s | Max s | First request load s | Maximum prompt / output tokens |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Original schema | 20 | 18.574 | 31.098 | 32.778 | 2.306 | 1264 / 768 |
| Original JSON | 20 | 21.231 | 32.025 | 32.106 | 0.006 | 1264 / 768 |
| Corrected schema | 10 | 24.638 | 35.095 | 35.411 | 1.296 | 2032 / 768 |
| Corrected JSON | 10 | 23.733 | 32.914 | 35.370 | 0.008 | 2032 / 681 |

The separate [smoke request](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/smoke-schema.json) returned in 25.650 s, including 11.342 s reported model load, and produced one raw/retained candidate. Its 259 output tokens took 9.369 s reported evaluation time. The smoke artifact has one observed run but no explicit planned-case/repetition fields; do not manufacture a planned count from that file. It is outside the 60 planned comparison requests and is not a quality selection.

The original schema and corrected schema arm baselines contain only the server process, and their first calls include 2.306 s and 1.296 s reported load respectively. The JSON arm baselines already include the model runner process. The two format arms ran sequentially, not randomized or interleaved, under a changing shared-host workload. Consequently latency and RSS differences cannot cleanly be attributed to decoding format or prompt changes. No warmed-only percentile was substituted for the all-request table.

Within each original arm, repetitions 1 and 2 produce byte-identical raw output, identical statuses, and identical proposal arrays for all ten cases: 10/10 paired agreements per arm. Temperature zero and only two seeds establish observed reproducibility, not stochastic robustness, error independence, or meaning accuracy. The corrected pass has one run per case/arm, so no within-configuration repetition statistic exists. As a separate cross-arm observation, corrected schema/JSON raw bytes match for 7/10 cases; this is not a repetition estimate.

## Resource observations

The sampling interval is five seconds. Server-family RSS sums the owned Ollama server and observed descendants; Go-runner RSS is the standalone extraction client family. Table peaks are the maximum over the baseline, sampled observations, and final snapshot, so a baseline can be the peak. RSS is converted from saved KiB to MiB by dividing by 1024. Swap numbers retain the units printed by macOS (`M`) rather than implying task-attributed allocations.

| Configuration | Timed observations | Server RSS baseline / peak / final MiB | Go runner peak MiB | Host used swap baseline / peak / final M | Measurement wall s |
| --- | ---: | ---: | ---: | ---: | ---: |
| Original schema | 82 | 28.609 / 3352.297 / 3287.016 | 7.828 | 6808.25 / 7518.50 / 7414.25 | 411.726 |
| Original JSON | 87 | 3266.078 / 3283.328 / 3260.953 | 8.750 | 7414.25 / 7414.25 / 7126.25 | 436.896 |
| Corrected schema | 52 | 17.125 / 1470.781 / 1456.125 | 12.531 | 7014.25 / 7268.12 / 7260.12 | 261.177 |
| Corrected JSON | 48 | 1365.609 / 1365.609 / 1070.328 | 17.938 | 7180.12 / 7852.12 / 7748.12 | 241.206 |

All four measurement commands returned exit code 0; that means the harness completed, not that all outputs passed shape checks or quality. The measurement wall durations include sampling/control overhead and differ from summed request latency. All Go-runner baseline/final RSS values are zero because the client was not running at those snapshots.

RSS does not reliably account for Metal/unified GPU allocations and shared mapped pages may be counted in multiple processes. Five-second sampling can miss transient peaks. Lower observed RSS is not evidence of a lower complete memory footprint. Swap is host-wide, reflects other applications and earlier activity, and cannot be charged to these requests. The [corrected preflight](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/corrected-pass-preflight.json) also records a transient full model-file read during metadata inspection before this pass; subsequent hashing streamed the file. That activity and the existing shared-host load limit clean resource comparison.

The much earlier [host preflight](local-spike-preflight.md) reported 4622.25 M used swap. Comparison baselines ranged from 6808.25 M to 7414.25 M, and the largest sampled value across these resource files was 7852.12 M during corrected JSON. This chronological change is a host observation, not proof the model caused that amount of swapping. At Mistral shutdown the disk still reported 12,113,328 KiB available and used swap 7716.06 M. The experiment did not measure production Evie foreground responsiveness, CPU/GPU utilization, owner review burden, integrated compiler throughput, or accepted-memory performance.

## Cancellation, server release, and shutdown

All 60 comparison requests and the separate smoke request record `server_release=finished_response`, including truncated or schema-invalid responses. Request completion and output validity are separate facts. No comparison request timed out.

The separate [actual timeout experiment](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/actual-timeout.json) planned two requests for N01-a, but attempted only one. The 500 ms client deadline returned at 502.494 ms with `cancelled_or_timeout` and `server_release=unknown`; zero proposals were returned. The second planned request was unexecuted because release remained unknown. This is not a model-quality failure sample, a successful server cancellation acknowledgment, or evidence that the capacity slot was immediately free.

The [capacity-release record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/capacity-release.json) states no second inference was sent before controlled restart. Immediately after timeout the owned server PID 55247 and runner PID 66162 were observed; the runner had exited by the subsequent server-restart inspection. No request-specific completion acknowledgment was observed. The record measured 0.147 s for the controlled owned-process termination observation and verified no observed owned process remained. A new owned server PID 66679, Ollama 0.6.3, was verified at the same loopback origin at 03:09:02 UTC. This demonstrates the exercised conservative restart recovery; it does not measure the exact time inference capacity became reusable after the client cancellation.

After all corrected requests completed, the [Mistral shutdown record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/mistral-owned-shutdown.json) states no inference was outstanding and all observed owned PIDs exited; controlled termination took 0.147 s. This separate shutdown prepared another local experiment and is not another Mistral inference or cancellation trial.

## Artifact audit and handoff

| Immutable or derivative report | SHA-256 |
| --- | --- |
| [development-schema.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/development-schema.json) | `680ee4e97153f0ab2e2003894b84e46cd3993c507033a2c03e6c5954b0e8cadf` |
| [development-json.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/development-json.json) | `b1a65df7d54a4450ecf929dda8a30251a9ce58fe7e08ebb9cc8c27e2529af9af` |
| [development-schema-validated.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/development-schema-validated.json) | `492d98e7aed23b87314bed8438a67277f3aac299a2318d4171cde709404c2bed` |
| [development-json-validated.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/development-json-validated.json) | `8c0e049154cc60bd01a3b7593ee2c650663d5be2d90d6495ca53e910f2926a26` |
| [corrected-schema.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/corrected-schema.json) | `1cf0a2a2aafe92c4735fd3bd86a142fcf570e28fcd50f11667059cb55db21233` |
| [corrected-json.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/corrected-json.json) | `83ba53b52bf08f6575a31082017521d9f458ce365ab39de775428f104f4de603` |

Resources are saved alongside their inference report as `development-schema-resources.json`, `development-json-resources.json`, `corrected-schema-resources.json`, and `corrected-json-resources.json`. They contain complete timestamped baseline/sample/final observations and command arguments. The final owner report should link those files and preserve the above denominator, quantile, confound, and resource limitations. No raw source, gold, runtime manifest, executable, or saved report was modified for this summary.

Verification for this note: recomputed planned/attempted counts, statuses, decodable raw and retained counts, source/gold exact-match comparison, source/annotation and derivative report hashes, all latency quantiles, process/swap maxima, paired output equality, and checked saved cancellation/restart records. No test suite or live inference was run because this is a read-only reporting subtask. Whitespace and Markdown-link checks are recorded in the handoff.
