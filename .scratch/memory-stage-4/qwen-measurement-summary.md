# Qwen standalone measurement summary

Read-only reporting for #135 from saved artifacts, 2026-09-05 UTC. Only this sibling scratch note was written; no inference, model loading, tests, owner-file edits, human adjudication, or commit. This uses the same counting, quantile, and resource conventions as the [Mistral summary](mistral-measurement-summary.md), with separate observations and no pooled model-family conclusion.

## Configuration and verified outcomes

The pinned model is `qwen2.5:7b-instruct-q4_K_M`; its manifest metadata reports 7.6B parameters and Q4_K_M quantization, with 4,683,073,952 weight bytes and recorded full weight SHA-256 `2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730`. Runtime is installed Ollama 0.6.3 on the same busy 18 GiB Apple M3 Pro host. This summary verifies saved artifact identities; it does not rehash the large weight file or independently rerun the model. See [runtime manifest](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/runtime-manifest.json).

The [experiment manifest](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/experiment-manifest.json) was recorded at 03:54:41.676 UTC before the report started at 03:54:41.938 UTC. The case subset is the same ten windows as the Mistral comparisons: N01-a, N01-b, N02-a, N02-b, N03-b, N04-b, N05-b, N06-a, N08-b, N09-a. It covers eight of nine development narrative families and has six required opportunities per traversal, plus two optional expected meanings. Eight development windows, the entire N07 family, and all six pilot windows remain unexecuted by this run. The manifest says the final holdout remains uncreated and unexposed.

Each case has two requests: temperature 0, seeds 17 and 18, context 8192, output cap 768, 60-second request timeout, one schema-format inference request at a time, and stop-on-failure enabled. The common corrected `prompt-v2.txt`, output schema, source corpus, and human-approved gold match the files used in the corrected Mistral pass. The model, its template/tokenizer and the executable/budget implementation differ. This is an observation of another frozen configuration, not a randomized or isolated causal comparison.

| Configuration | Planned / attempted / unexecuted | Whole-response status | Decodable raw proposals | Runner `raw_count` sum | Retained proposals | Required opportunities | Exact required matches, raw / retained |
| --- | --- | --- | ---: | ---: | ---: | ---: | ---: |
| Qwen schema | 20 / 20 / 0 | 20 `ok` | 30 | 30 | 16 | 12 | 4 / 4 |

All 20 raw responses are JSON-decodable and all statuses are `ok`, with no whole-response schema error or truncation. Nevertheless, 14 of the 30 proposals are not retained by the standalone candidate checks. Whole-response `ok` does not mean each proposal passed source checks; retention does not mean semantic correctness or usefulness. No cutoff was triggered, and all 20 planned requests were actually attempted.

Independent comparison of all meaning fields, scope, and exact source/context reference sets verifies four required gold matches on both the raw and retained axes: N01-a and N09-a, each repeated twice. This is two distinct matched required expectations, not four independent kinds of memory. The approved-gold denominator is 12 request-level opportunities, retaining every attempted request even where zero proposals survived. N01-b returns an empty candidate array in both repetitions, matching that case's reviewed remember-nothing expectation; that observation is separate from the required-memory denominator.

The [initial score report](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development-initial-score.json) contains 13 unique case/proposal identities with 26 raw occurrences awaiting human output adjudication. The retained axis has 12 unadjudicated occurrences. Its confirmed match counts imply a current confirmed required-recall lower bound of 4/12, confirmed raw supported/useful lower bound of 4/30, and retained lower bound of 4/16. These are lower bounds from exact approved meanings, not final quality rates or statistical confidence bounds. The remaining outputs are neither accepted as good nor assigned proposed error labels by this summary. No proposed adjudication file is used.

## Latency and repeated requests

All attempted requests enter the latency calculations, including the first model load and requests with zero retained candidates. Sort the 20 saved latency values and use zero-based index `floor((n-1) × q)` for p50/p95, no interpolation; this matches the existing scorer and the Mistral summary. Units below are seconds, rounded from milliseconds.

| n | p50 s | p95 s | Maximum s | First request load s | Prompt token range | Output token range |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 20 | 14.137 | 36.224 | 36.411 | 5.199 | 1361–1802 | 6–755 |

The first N01-a request took 20.229 s, including 5.199 s reported load, 5.877 s prompt evaluation, and 9.131 s output evaluation; small transport/control overhead accounts for the remainder. Its repetition took 9.697 s and reported 0.013 s load. The baseline has only the server process, supporting a first-load interpretation. The slowest request is N04-b repetition 1 at 36.411 s. No request reached the 768-token cap; the largest recorded output is 755 tokens.

Within the configuration, all ten case pairs have byte-identical raw output, equal statuses, and equal proposal arrays, including retained flags. This is 10/10 observed pair agreements at temperature zero. The two seeds do not demonstrate stochastic robustness, meaning accuracy, or independent-error performance. Do not pool these repeated outputs as independent novel memories or derive a population p95/release threshold from this small selected sample.

## First-load engine and input context evidence

The [first-load engine record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/first-load-engine.json) records owned server PID 78270 and its child runner PID 84407. The launch explicitly set `OLLAMA_NEW_ENGINE=false`; the observed runner command does not contain `--ollama-engine`, and native `llama_init_from_model` output records `n_ctx = 8192` and `n_ctx_per_seq = 8192`. The command launches the pinned weight blob with `--ctx-size 8192`, `--batch-size 512`, `--n-gpu-layers 29`, `--threads 5`, and `--parallel 1`. This supports the recorded legacy llama.cpp-backed Go runner engine, rather than relying on a mutable model tag or a generic runtime version alone.

The [runtime observations](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/runtime-observations.json) include unavailable x86 CPU `.so` backend probe messages, followed by Metal loading and 29/29 layers offloaded to GPU. All requests then finished. They also record a 292.36 MiB CPU-mapped model buffer, 4168.10 MiB Metal-mapped model buffer, and f16 KV cache of 8192 positions. The runtime's `memory.required.full = 5.6 GiB` and `memory.required.kv = 448.0 MiB` are engine-reported planning values, not independently measured total-memory peaks. These figures must not be added to RSS as if all were disjoint allocations.

No input-truncation log was observed in the saved runtime observations. The maximum returned prompt-token count is 1802; this observation alone is not the predispatch safety proof. The separate [Qwen input-budget proof](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/input-budget-proof.md) and pinned token budgets account for this model/tokenizer and its exact template. The manifest records `full_rendered_utf8_bytes + 2 + 768 + 64 <= 8192`, 80 bytes of full template overhead, the 5588-byte common system prompt, and a 1690-byte maximum source-input bound. The request's nonempty system prompt replaces the model default system text. This bounded spike proof is not a general production budget contract.

## Sampled resources and shutdown

The [resource report](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development-resources.json) sampled every five seconds, with 69 timed samples plus baseline and final snapshots. Maxima below cover all those observations; RSS is converted from saved KiB to MiB by dividing by 1024. Swap values retain macOS `M` display units and refer to the whole host.

| Server RSS baseline / sampled peak / final MiB | Go client peak MiB | Host used swap baseline / sampled peak / final M | Measurement wall s | Exit code |
| ---: | ---: | ---: | ---: | ---: |
| 26.391 / 4487.062 / 4279.922 | 10.828 | 7516.06 / 8789.94 / 8685.88 | 346.754 | 0 |

The server-family RSS peak is 4,594,752 KiB at 03:54:51.796 UTC, comprising the owned Ollama server and observed descendants. The Go-client peak is 11,088 KiB at 03:59:58.311 UTC. Its baseline/final values are zero because that client was not running at those snapshots. Host used swap peaks at 8789.94 M at 03:55:32.048 UTC. RSS misses reliable Metal/unified GPU accounting, shared mapped pages may overlap, and five-second sampling can miss transient peaks. Host swap includes unrelated activity and cannot be assigned to Qwen or treated as a task-only memory delta. These observations do not establish production Evie foreground overhead or a safe release gate.

The pre-run disk record has 7,850,332 KiB available; the later shutdown record has 7,304,876 KiB available and host used swap 8501.88 M. These are changing host observations, not exact acquisition/run-attributed consumption. The measurement wall duration includes harness/sampling overhead and is not the sum of request latencies.

Every request records `server_release=finished_response`. There was no Qwen timeout/cancellation exercise in this run, so the earlier Mistral cancellation observations cannot be presented as a Qwen-specific cancellation measurement. The [owned shutdown record](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/owned-shutdown.json) verifies all 20 requests finished, no inference was outstanding, current owned PIDs exited, and the previously observed runner PID was absent. Controlled termination took 0.138 s; cache artifacts were preserved. This measures the observed orderly shutdown, not release latency after an in-flight cancellation.

## Artifact audit and handoff

| Saved artifact | SHA-256 |
| --- | --- |
| [development.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development.json) | `15c64a287f0f46f33752137010897fc0c170ee6a133bda00e8e12a2c280be2ce` |
| [development-resources.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development-resources.json) | `02b2521184b2d9643ea41ca1f2625770fe66fc1396d025381eddb4f8bce69bfe` |
| [development-initial-score.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/development-initial-score.json) | `672e1a81025eef3a966b101e5034056a424ce292cdf20227d6b7c37b4e79cfbe` |
| [first-load-engine.json](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen/reports/first-load-engine.json) | `92c08af94e61165e33dcc416041ab4159e400bd85fda4e930420da8d924f4718` |

Verified every fixture hash pinned by the experiment manifest, all 20 raw-response hashes, score/report identity, shutdown/report identity, count/denominator arithmetic, exact approved-gold matching, recorded latency quantiles, repetition equality, and baseline/peak/final resource calculations. Model weights were not loaded or rehashed in this reporting subtask. No suite tests or inference were run. The owner/root handoff records whitespace and Markdown-link checks.

The evidence supports a measured candidate for continued development evaluation. It does not select production configuration, adjudicate novel meanings, freeze pilot gates, or satisfy the still-unexecuted development/pilot/final-holdout work.
