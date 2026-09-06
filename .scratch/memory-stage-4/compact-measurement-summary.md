# Compact wire measurement comparison

Baseline prepared 2026-09-05T04:51:19.971274+00:00. Read-only summary of saved artifacts for #135; only this scratch note is written. No inference, tests, owner-file edits, or human output judgments. The completed compact run is summarized below; the original paired baseline remains separately recorded.

## Paired baseline: prior Qwen seed 17 only

The comparison baseline is repetition 1 / seed 17 from the [prior Qwen inference report](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development.json), not the full two-repetition total. All ten case IDs and their order match the [compact frozen plan](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/experiment-manifest.json). Prior whole-response statuses, raw/retained counts, and exact listed-gold matches were independently recalculated. Raw counts include every JSON-decodable proposal; retained counts mean only the declared standalone checks passed. Neither count by itself proves meaning/usefulness.

| Prior baseline case | Status | Raw | Retained | Exact required matches raw / retained | Required opportunities | Latency s | Prompt / output tokens |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| N01-a | `ok` | 1 | 1 | 1 / 1 | 1 | 20.229 | 1361 / 195 |
| N01-b | `ok` | 0 | 0 | 0 / 0 | 0 | 2.171 | 1575 / 6 |
| N02-a | `ok` | 1 | 1 | 0 / 0 | 0 | 12.215 | 1374 / 229 |
| N02-b | `ok` | 2 | 2 | 0 / 0 | 1 | 19.619 | 1598 / 381 |
| N03-b | `ok` | 2 | 1 | 0 / 0 | 1 | 23.346 | 1765 / 431 |
| N04-b | `ok` | 3 | 0 | 0 / 0 | 0 | 36.411 | 1572 / 755 |
| N05-b | `ok` | 3 | 1 | 0 / 0 | 0 | 34.731 | 1598 / 686 |
| N06-a | `ok` | 1 | 1 | 0 / 0 | 1 | 11.349 | 1370 / 234 |
| N08-b | `ok` | 1 | 0 | 0 / 0 | 1 | 16.846 | 1802 / 307 |
| N09-a | `ok` | 1 | 1 | 1 / 1 | 1 | 9.682 | 1366 / 196 |

| Prior sample | Planned / attempted / unexecuted | Raw / retained | Exact required matches raw / retained | Required opportunities | p50 / p95 / max s |
| --- | --- | ---: | ---: | ---: | ---: |
| Paired seed17 subset | 10 / 10 / 0 | 15 / 8 | 2 / 2 | 6 | 16.846 / 34.731 / 36.411 |
| Original complete seeds17/18 report | 20 / 20 / 0 | 30 / 16 | 4 / 4 | 12 | 14.137 / 36.224 / 36.411 |

The paired subset has ten `ok` responses and 13 unmatched raw proposal occurrences awaiting human judgments. Its two exact required matches are N01-a and N09-a. The original complete report has 20 `ok` responses and 26 unmatched raw occurrences representing 13 distinct case/proposal objects. All ten original pairs have identical raw bytes and proposal arrays, but their timing differs. Do not compare compact ten-request totals directly with the old twenty-request totals, count repetition2 as additional paired evidence, or treat the 13 novel objects as approved meanings.

Exact matching compares every canonical meaning field, scope and reviewed source/context reference sets against the unchanged approved gold. Labels from any `.proposed` adjudication file are excluded; these files are hashed only for the preservation audit. The six required opportunities per traversal remain in the denominator for all attempted cases, even if output fails or none survives. Planned-but-unexecuted compact cases must be separately disclosed and never assigned fabricated omissions or latency.

Quantiles use the existing scorer convention: ascending sorted latency at zero-based index `floor((n-1) × q)` for q=0.50/0.95, with no interpolation. All attempted request latencies, including first-load and failures, enter the corresponding all-attempt table. On a ten-request sample p95 is the ninth order statistic, not max. The paired baseline first request took20.229s including5.199s reported load. Sequential experiments on the changing shared host do not isolate a transport change's causal effect on latency or resources.

The [full prior resource summary](qwen-measurement-summary.md) remains the source for the old20-request resource observations. Those five-second samples cover both repetitions and do not provide a clean isolated seed17-only resource baseline; do not label the full-run RSS/swap peaks as paired-ten measurements.

## Compact interpretation and predispatch proof

Compact-v1 changes the complete input/output wire configuration, prompt and schema. It presents the same selected source fields in order with short aliases, typed subject components and a sealed deterministic canonical expansion. The model cannot set scope; alias conversion binds original coordinates/provenance but cannot manufacture semantic equivalence or fix an ambiguous name. Each JSON-decodable wire object remains a raw proposal even if canonical expansion fails; raw wire bytes, decoded object, expansion, retained flag and rejection reasons must be reported separately. A higher retention count alone cannot receive exact gold credit.

The frozen compact plan permits one seed17 traversal of the ten cases using the same Qwen weight artifact, Ollama0.6.3, schema format, temperature0, context8192, output cap768, timeout60s, one request at a time and stop-on-failure. It is a paired case/configuration observation, not a model selection or release gate. Eight other development windows and six pilot windows are outside this plan; the final holdout stays uncreated/unexposed.

The compact [input-budget proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/input-budget-proof.md) uses identical pinned runtime/template/tokenizer evidence and independent compact prompt/schema/request identities. It records the complete rendered UTF-8 bound `5561 + compact_input_bytes + 80 + 2 + 768 + 64 <=8192`. The ten inputs are282–696bytes; recorded bounds range6757–7171tokens, leaving at least1021tokens under8192. Those are conservative predispatch byte-derived bounds, not observed prompt-token counts. The initial comparison will check actual report request/seal hashes against this frozen plan and retain any budget/protocol failure.

## Preservation audit and pending measured section

At baseline preparation, all 64 paths in [pre-compact artifact baseline](pre-compact-artifact-baseline.json) matched their recorded SHA-256 bytes; no missing or changed prior artifact. Every compact manifest fixture hash also matched. The source/gold human approval is preserved, as are all original raw/score/model/review records. This audit does not apply proposed output labels.

- Prior Qwen report SHA-256: `15c64a287f0f46f33752137010897fc0c170ee6a133bda00e8e12a2c280be2ce`.
- Preservation baseline SHA-256: `a59e285a07444faa1da56c5ab9996a19a36de6f07aeb183a76b1bffdaad857d0`.
- Compact experiment-manifest SHA-256 at preparation: `a7ff9a886c9976568bb0e409f986ebe10ab018e09670b0fb9821d1476cc82fd1`.

Root subsequently notified completion. The following section uses the saved completed report, not a preflight prediction.

## Actual compact-v1 outcome and paired changes

The [completed compact report](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development.json) contains all10 planned cases once at seed17:10 attempted,0 unexecuted,10 whole-response `ok`, and10 request-specific `finished_response` release acknowledgments. There are no failed inference requests, retries, output truncations or timeouts. Twelve wire candidates decode as JSON, but **zero expand successfully and zero are retained**. The initial score therefore has0 exact required matches out of6 attempted required opportunities. All12 raw objects/12 occurrences await actual human judgments. No proposed output label is included in these totals.

The whole-response status validates the declared compact JSON shape, not all cross-field expansion rules. Recorded rejection counts contain only each candidate's first failure:10 `invalid_selector` and2 `invalid_subject`. Reading every raw object reveals overlapping reference defects on all12; first-error counts are not an exhaustive count of problems.

| Case | Prior→compact raw | Prior→compact retained | Prior→compact exact required matches (both axes) | Prior→compact latency s | Compact−prior latency s |
| --- | ---: | ---: | ---: | ---: | ---: |
| N01-a | 1→1 | 1→0 | 1→0 | 20.229→18.681 | -1.548 |
| N01-b | 0→1 | 0→0 | 0→0 | 2.171→6.915 | +4.744 |
| N02-a | 1→1 | 1→0 | 0→0 | 12.215→6.530 | -5.686 |
| N02-b | 2→1 | 2→0 | 0→0 | 19.619→7.027 | -12.591 |
| N03-b | 2→1 | 1→0 | 0→0 | 23.346→7.117 | -16.229 |
| N04-b | 3→1 | 0→0 | 0→0 | 36.411→6.794 | -29.617 |
| N05-b | 3→3 | 1→0 | 0→0 | 34.731→18.400 | -16.331 |
| N06-a | 1→1 | 1→0 | 0→0 | 11.349→6.481 | -4.868 |
| N08-b | 1→1 | 0→0 | 0→0 | 16.846→9.264 | -7.582 |
| N09-a | 1→1 | 1→0 | 1→0 | 9.682→6.515 | -3.167 |

| Same-case seed17 sample | Attempts | Raw / retained / expanded | Exact required matches / opportunities | p50 / p95 / max s | Summed request latency s | Output tokens |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Prior Qwen | 10 | 15 / 8 / not a wire-expansion metric | 2 / 6 | 16.846 / 34.731 / 36.411 | 186.599 | 3420 |
| Compact-v1 Qwen | 10 | 12 / 0 / 0 | 0 / 6 | 6.915 / 18.400 / 18.681 | 93.725 | 1666 |

The earlier original20-request figures remain separate above. Compact has no within-configuration repetition, so no compact repeatability estimate can be computed. Nine of the ten paired latencies are lower; N01-b is higher because the prior response was empty while compact emits an old tea candidate. Shorter overall latency accompanies shorter output and a wholly rejected transport representation; it is not evidence of faster usable memory extraction. Prompt/schema/presentation changed together and experiments ran sequentially on a changing host, so the timing difference is not an isolated causal estimate.

## Concrete transport-schema gap

Every compact candidate has one `sources` reference, and every one includes `start` but omits `end`. Eleven omit `selector` (which expansion treats as whole); N08-b explicitly uses `selector=whole`. N08-b adds one clock reference in `context` with `selector=date` and `start=0`. Thus all13 reference objects carry a coordinate even though their whole/date selector forbids coordinates. None requests a complete explicit range. No reference uses an unknown alias.

This exact malformed combination is allowed by the frozen [output schema](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/output.schema.json): each reference requires only `ref`, while `selector`, `start` and `end` are independent optional properties. For example, `{"ref":"s1","start":0}` satisfies its declared type/minimum/required/additional-property rules. There is no selector-dependent rule requiring both range coordinates or prohibiting them for whole/date. The adapter's [expansion rules](../../scripts/memory-extractor-spike/compact_wire.go) correctly reject those combinations, consistent with the prompt. The declared output grammar is underconstrained relative to the adapter contract. The resulting10 `ok` responses and0 retained candidates expose that transport design gap; they do not establish12 semantic failures.

The subject fields have a second underconstraint: the schema independently permits `identity=unresolved` with owner/project and a text object. N01-b and the N05-b PostgreSQL consideration use that incompatible combination. They first fail `invalid_subject: incompatible identity`, before the same invalid selector is evaluated. Both have empty subject_name/entity_ref as required for owner/project; neither failure is a missing subject name or unknown accepted alias.

Further independently observable source-policy issues overlap those first failures: N01-b tea and the N05-b SQLite/offline candidates cite only overlap support (3 objects). N08-b puts its overlap tool observation into the assistant-context axis (1 category mismatch). The latter also combines date with a start coordinate. Source alias existence is correct in all13 references, so replacing UUIDs with short aliases did not itself fail here. No output was repaired, re-expanded by guessing a missing field, or rescored as retained.

Before a new transport experiment, the schema should describe the same mutually exclusive reference forms as the adapter: whole/date with no coordinates, or range with both explicit coordinates; subject/identity alternatives should also avoid these impossible combinations. This is a recommendation to change a future frozen wire contract, not permission to repair this run or evidence that semantic problems are solved. Any runtime grammar-support assumption needs deterministic verification before another model call.

## Meaning observations and provisional-label review

These observations compare visible wire fields with full sources and approved gold. They are recommendations for human adjudication, **excluded from confirmed quality totals**. Incomplete canonical expansion still prevents exact automatic credit. The partial `candidate` record retained for diagnosis must not be mistaken for a valid expanded candidate.

| Case/object | Observation independent of reference repair | Provisional review recommendation |
| --- | --- | --- |
| N01-a tea | Subject/meaning fields exactly describe the approved owner preference; the nominated alias is the actual new assertion. | Raw required-useful credit is defensible after human approval, with locator error; retained stays0. |
| N01-b tea | Repeats the true prior standing preference using only overlap; identity is incorrectly unresolved. The newly owned pear event is not cited. | Explain ineligible repeated truth, identity and locator failures. `unsupported` under new-evidence policy versus `unwanted_but_true` is a taxonomy distinction; do not describe this as an invented tea preference. |
| N02-a Paris | Converts a fictional first-person quote/unendorsed report into the owner's residence. | Unsupported interpretation is a well-founded proposal, apart from the locator defect. |
| N02-b Paris | The new assertion is about Maya, but output says owner. | Unsupported subject attribution; fixing reference shape cannot fix the actor. |
| N03-b tea | The approved meaning is tea over coffee, interpreted using the assistant question. Output drops comparison/context and changes fact to decision. | Do not silently grant equivalent required credit; retain qualification/context and typed-meaning concerns for human review. |
| N04-b Paris | The source pronoun has two possible Maya antecedents; output asserts owner residence. | Unsupported identity/subject resolution remains independently of the reference defect. |
| N05-b SQLite | Old adopted project decision is cited from overlap alone. | Identify repeated truth barred by new-evidence ownership, rather than inventing factual falsity. |
| N05-b PostgreSQL | New source explicitly presents an unadopted long-term option; predicate/kind/object express consideration, but project identity is unresolved. | Raw optional-useful credit is defensible after human review, with identity/locator errors; no required-gold recall index or typed/retained credit. |
| N05-b offline | Lasting requirement is stated in the old overlap source, not newly supplied support. | Identify repeated truth barred by new-evidence ownership; locator also invalid. |
| N06-a employment | Source says the owner no longer works at Acme and left last month. Output is affirmed generic employment, loses Acme, and uses fact rather than world_change. | Unsupported/lost-negation/object and typed-meaning concerns are independent of locator repair. |
| N08-b coffee | Literal output says stopped drinking coffee as of2026-09-04; owner source and checked date support that reading. Predicate constraint/kind fact differ from approved habit/world_change, and clock is wrongly in context. | Reconsider blanket unsupported if raw semantic credit permits typed errors elsewhere. Offer an explicit human equivalence judgment, preserving Predicate/kind, category and locator errors; no retained/typed credit. |
| N09-a café | Output drops the coffee emoji from reviewed café☕ while retaining denied preference; reference is still invalid. | Make café↔café☕ equivalence explicit for David. If emoji is decorative, raw required credit may be appropriate after approval; it is not an automatic exact match. |

In particular, accepting PostgreSQL's intended raw meaning despite its identity error while rejecting N08-b solely for typed/locator errors would mix standards unless the reviewer considers the constraint Predicate to change the meaning materially. Keep that decision explicit. No assertion here pre-approves an output or changes the frozen gold.

## Resource, engine and preflight observations

The [compact resources](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development-resources.json) contain19 five-second samples plus baseline/final observations. Peaks cover all observations, and RSS converts KiB to MiB by dividing by1024. Host swap is macOS-reported M, not task-attributed consumption.

| Server RSS baseline / peak / final MiB | Go client peak MiB | Host used swap baseline / peak / final M | Harness wall s | Exit |
| ---: | ---: | ---: | ---: | ---: |
| 27.453 / 3302.484 / 1604.594 | 10.625 | 8253.88 / 8253.88 / 8253.88 | 95.533 | 0 |

RSS misses reliable Metal/unified GPU accounting, can overlap mapped pages and misses transients between samples. Stable sampled host swap does not establish no memory pressure, nor can a host-level delta be assigned to this task. Original Qwen resource files span20 requests; they are not a seed17-only control. Neither these resources nor lower latency establish production foreground overhead.

The [first-load engine record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/first-load-engine.json) records owned server8268/runner8589, the same pinned Qwen weight blob, one sequence, native llama.cpp initialization at8192 context, Metal and29/29 GPU layers. It includes x86 backend probe warnings followed by successful Metal load. Engine-reported Metal model4168.10MiB, KV448MiB, compute492MiB and required-full5.6GiB are allocation diagnostics, not independent total-memory peak measurements and must not simply be added to RSS. The first request took18.681s, with4.682s load,7.909s prompt evaluation and6.074s output evaluation. Actual prompt counts range1154–1286, output136–373; no returned output reaches768.

Two earlier owned-server starts ended during metadata-only observation, with zero generation requests: the [first recovery](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/metadata-preflight-recovery.json) describes raw-array versus frozen-summary encoding; the [second](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/metadata-preflight-recovery-v2.json) describes a summary JSON hash mismatch before generation. The [successful runtime observation](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/runtime-observations.json) records exact frozen metadata matching with the documented encoding before requests. These are preflight starts, not quality attempts, model retries or additional model calls.

All10 completed responses establish request-specific release. The [shutdown record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/owned-shutdown.json) records no outstanding inference, all owned group processes exited,0.033s controlled termination, earlier metadata-only PIDs absent, and caches preserved. No in-flight compact cancellation was exercised; orderly shutdown does not measure cancellation acknowledgment latency.

## Completed artifact/source audit

Audit repeated at 2026-09-05T05:05:13.851021+00:00: all64 pre-compact artifact bytes remain unchanged. The compact manifest still hashes to `a7ff9a886c9976568bb0e409f986ebe10ab018e09670b0fb9821d1476cc82fd1`; its fixture hashes match. All10 saved raw-text hashes, exact request hashes and alias-seal hashes match their frozen plans. Prepared and dispatched sealed requests are equal. Every sealed source object is an unchanged approved source/context projection; every model-visible field retains its exact text, byte positions, ownership and authority. The recorded request carries the exact compact prompt/schema, and every full-rendering byte bound recomputes to its manifest value at or below7171/8192. No model weight reread was used.

| Saved artifact | SHA-256 |
| --- | --- |
| [development.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development.json) | `67e4290e9c42b20726e90f56d919ccd703ddc5330cfb6bf7cb9119ebe70968f8` |
| [development-resources.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development-resources.json) | `01113133accd9ed8981b2e266475551356ccd78187b3b17bcc416ba1ec819033` |
| [development-initial-score.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/development-initial-score.json) | `503cad16e1baa77dad310216c1307b963bd2540f34bdd1a758f1271b70596efd` |
| [first-load-engine.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v1/reports/first-load-engine.json) | `9c8691d2e97554ac7d4d1b76cf8ab64aee5c2f1f3258860f9d2e1a3ac7078566` |

Scorer input identity, unapplied adjudication status, all-attempt quantiles, raw/expanded/retained counts, paired baseline/deltas and sampled resource extrema were independently checked with standard-library parsing. No test suite, new inference, browser research, proposed-label application, original artifact edit or commit. Whitespace and local-link checks are recorded in the handoff.
