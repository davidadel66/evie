# Dense component regression restored after a generic candidate-budget fix

The v2 development run and known-regression replay pass the unchanged #165
component gates. The first run's 81.25% failure remains in `../v1`; it is neither
overwritten nor reclassified as passing. Replaying its previously evaluated
questions is **not fresh held-out evidence for #167/#168**.

The production fix was driven by the two development misses and a separate
neutral complete-turn regression: 70 unrelated messages matching `at`, an
original `runs before sunrise` statement, and `jogging at dawn`. Before the fix,
64 lexical candidates exhausted the shared work allowance before dense ranking.
The test failed, then passed with 24 lexical candidates reserved when dense
coverage is active, leaving 40 of the existing shared 64 slots for authoritative
dense candidate reads. Inactive dense retrieval preserves the lexical baseline's
64-candidate allowance. Candidate overflow now reports truncation explicitly.

This allocation is the only deliberate retrieval configuration change from v1.
The model, weights, vector normalization, threshold, RRF constant, source corpus,
questions, expected IDs, measured turn boundary, budgets and acceptance gates
remain unchanged. The three repetitions and deterministic condition rotation
also remain unchanged. No held-out question-specific optimization was performed.

## Frozen inputs and reproduction

The actual executable is `/tmp/evie-stage5-dense-integration-v2.test`, SHA256
`624388b9891ffccf2cf3b06ad921883c40b29c0a04acc9f999f7624f65bac04d`.
`frozen-v2/freeze.json` was written before the development run and reused without
changes for the subsequent authorized known-regression replay. The source
snapshot is `/tmp/evie-stage5-dense-integration-v2-source`; all 1039 Go files
were unchanged before/after export and compilation. The freeze also pins module,
model/runtime, corpus/question and runner hashes.

`frozen-v2/compiled-source.tar.gz` preserves the actual source independently of
later commit rewrites. Its supplemental manifest verifies all compiled inputs
and original fixture/runner files. The archive contains no executable, weights,
database or credentials. A fresh extraction was compiled with `GOPROXY=off`
and `GOSUMDB=off`; the exact command/result is retained in
`source-archive-verification.json`. Binary byte identity is not claimed when
rebuilding under another source path.

Use the same commands and explicit local runtime/model prerequisites described
in `../v1/README.md`, substituting this directory's `run.py` and fresh output
paths. A later code change requires a new source snapshot, executable and
freeze. Never bypass hash checks or overwrite an attempt.

The original v1 artifact format, HTTP observation semantics, timing boundaries,
source-prefix differences from #165, complete request capture and byte-count
limitations remain applicable. `traces.ndjson.gz` retains exact requests,
evidence and receipts; compact samples and endpoint/build observations remain
separate machine-readable files.

The corpus mapping's original `source_link_id` fields are empty because of the
v1/v2 metadata assignment bug described in `../v1/README.md`. Exact supplied
links remain in the original requests and receipts. The supplemental association
maps recover those observed links only. A one-field fix to the current harness
improves future reporting; it changes no frozen executable, recall result,
question, configuration or gate. The archived measured harness retains its
original code so provenance remains inspectable.

## Exact measured results

| Partition / condition | Evidence recovered | Paraphrase recovered | Whole-turn p50 / p95 |
|---|---:|---:|---:|
| Development lexical | 75/96 | 27/48 = 56.25% | 17.593 / 39.959 ms |
| Development hybrid | 96/96 | 48/48 = 100% | 34.183 / 51.456 ms |
| Known-regression lexical | 75/96 | 27/48 = 56.25% | 25.041 / 36.674 ms |
| Known-regression hybrid | 93/96 | 45/48 = 93.75% | 35.867 / 49.089 ms |

Every lexical control is recovered. The only remaining hybrid miss is
`heldout-13-paraphrase`, identical across all three repetitions and already
missed in the original #165 experiment. The known-regression paraphrase gain is
37.5 percentage points; both the unchanged 85% recall minimum and ten-point
gain requirement pass. Its complete-turn p95 is below the unchanged 250 ms cap.

| Observation | Development | Known-regression replay |
|---|---:|---:|
| Corpus construction | 1.527 s | 1.421 s |
| Retained maintenance | 4.040 s | 3.806 s |
| Bounded maintenance batches | 73 | 73 |
| Actual embedding requests / inputs | 169 / 869 | 169 / 869 |
| Total observed HTTP requests | 507 | 507 |
| Largest complete encoded provider request | 34414 bytes | 34567 bytes |
| Largest serialized memory message | 13318 bytes | 13463 bytes |

Each partition records 192 complete turns and 384 complete provider requests.
All returned evidence maps to original sources; accepted scope revisions remain
unchanged. There are zero forbidden/scope disclosures, secret-bearing embedding
inputs, secret-bearing provider evidence, delivered-count violations or memory
projection turn-cap violations. All observed HTTP requests succeeded. These
counts do not claim independent observation of the internal 12 KiB search-result
serialization or native reader-model tokens.

After timed comparison, the frozen v2 binary passed the separate actual-model
punctuation-heavy compatibility probe in 0.50 s. It checks bounded source input
handling and does not contribute to answer-quality or release-gate conclusions.

Fresh sealed quality evaluation, natural-language reader behavior and the final
repository verification remain separate assigned work. The v2 evidence restores
the selected component's known-regression expectations only.
