# Memory Stage 5: local hybrid evidence (#166)

This slice integrates the configuration selected by [the local retrieval
experiment](../../../../docs/experiments/memory-retrieval-spike/DECISION.md)
into the existing `memory_search`, `memory_search_conversations`, Automatic
Recall, original-request receipts, and source inspection. Similarity proposes
candidates. SQLite still establishes acceptance, scope, lifecycle, temporal
meaning, source eligibility, and the text that may leave the Kernel.

## Configuration and opt-in

`EVIE_MEMORY_EMBEDDING_ENDPOINT` is unset by default, which disables dense
retrieval and its maintenance without changing the exact/lexical baseline.
Enabling it also requires `EVIE_REMOTE_MEMORY=on`. Accepted endpoint forms are
plain HTTP with a literal loopback address or `unix:/absolute/socket/path`.
Host names, non-loopback addresses, credentials, redirects, proxy resolution,
query strings, and non-root HTTP paths are rejected. There is no remote fallback
or model acquisition in this implementation.

The adapter uses `all-minilm:22m`, the selected manifest SHA256
`1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef`, and model layer
SHA256 `797b70c4edf85907fe0a49eb85811256f65fa0f7bf52166b147fd16be2be4662`.
It requests `truncate:false`, four CPU threads and a 30-second keepalive.
The manifest is checked before and after inference. A changed tag, wrong model,
wrong result count, wrong dimensions, nonfinite/zero vectors, malformed JSON, or
oversized response cannot produce an indexed vector. Both manifest reads and
inference share a ten-second ceiling, shortened by caller cancellation or an
earlier deadline. The runtime maintenance worker has its own shorter batch
deadline.

| Bound | Value |
|---|---:|
| Normalized vector | 384 float32 values |
| Candidate similarity | Cosine, minimum 0.25 |
| Dense generator result list | 8 eligible candidates |
| Active dense conversation candidate work | 24 lexical suggestions, remaining 40 of the shared 64 for dense validation |
| Vector scoring scan | 4096 stored parts per selected scope query |
| Fusion | Reciprocal rank, k=60, after authoritative eligibility |
| New embedding inputs per maintenance call | 16 total across Claims and events |
| Projection chunk / overlap | 240 / 48 UTF-8 bytes |
| Adapter input / response ceiling | 1024 bytes per input / 1 MiB response |
| Foreground shared query deadline | 500 ms |
| Dense HTTP share | At most half the remaining deadline, never over 250 ms |
| Model-directed result / turn context | Existing 12 KiB / 36 KiB budgets |

The selected manifest configures `num_ctx=256`; the model architecture metadata
reports `context_length=512`. A development probe using the real selected
endpoint rejected a valid punctuation-heavy source under the earlier 800/1024
byte projection bounds with HTTP 400. Conservative 240-byte chunks passed the
same complete-turn probe. This is measured compatibility with this selected
model, not a general theorem about tokenizer behavior. Overlap reduces boundary
loss. Exact UTF-8 ranges and hashes map each event part back to its original
content; accepted evidence still renders the complete approved Claim. A Claim
that cannot fit the existing model-context budget remains explicitly exhausted.

## Durable projection and recovery

The embedding configuration and its hash are immutable generation records.
The selected generation starts in `building`, with independent durable Claim
and event checkpoints. Source, revision and content hashes accompany each
vector; Claim parts also retain the full proposition hash. Model inference
runs outside the SQLite write transaction. Before persisting its output,
maintenance reconstructs the source again and compares its fingerprints.
Concurrent appends, corrections, retirements, source restrictions and context
lifecycle changes enqueue work in the accepting transaction.

Maintenance respects both a bounded retained-row budget and the sixteen-input
embedding budget. Long sources resume from already matching stored parts.
Incomplete or stale source parts remain dirty and cannot become candidates.
Unchanged parts are not embedded again. A generation cannot become active until
its required retained coverage and dirty work are reconciled. Once active,
new pending changes are excluded and the read reports partial coverage.

`Store.RebuildMemoryEmbeddings(ctx)` selects a fresh immutable building
generation; it does no inference or backfill. Repeated bounded
`RefreshMemoryIndex` calls reconcile it. Restart resumes the durable selection
and checkpoints. Disabling maintenance stops its dirty queue, and re-enabling
requires retained reconciliation. Query-time configuration/hash checks prevent
an obsolete active generation from serving before replacement. Old generation
records preserve their configuration rather than being relabeled.

Accepted and conversation FTS use separate physical v3 tables. Older v1/v2
projections remain inert derived artifacts, so their term frequencies cannot
affect v3 BM25 statistics. Startup creates building v3 generations; bounded
maintenance performs the retained migration. A fully active dense generation
can supply evidence during an independent lexical rebuild, with partial status.
Unreconciled generators never contribute their partial rows.

## Query and provenance contract

HTTP query embedding can run concurrently with exact, lexical and temporal
candidate work. SQLite work remains ordered in one read transaction; no worker
goroutine accesses that transaction. Every scored candidate is checked against
the original scope, source, revision, content hash and selected temporal intent
before it enters a ranked list. Dense hits share the existing bounded candidate
work and graph/fusion path. They do not acquire acceptance or owner authority.

Conversation vectors cover only allowlisted public user/assistant content.
Whole-field secret screening precedes projection. Context equality, live-root
cutoffs, retired-range suppression, historical labeling, current source access
and exact original byte/hash checks are the same as lexical conversation reads.
Assistant statements remain assistant inference. Synthetic memory messages,
tool results, payloads and reasoning are not indexed as conversation evidence.

An enabled but incomplete or failed dense path produces partial status when a
baseline path succeeded or established an empty result. If all paths are
unavailable, the result is unavailable. Caller cancellation remains
identifiable; endpoint timeouts preserve eligible baseline evidence rather than
consuming the entire rendering deadline. Truncated source passages are explicit.

Evidence and content-free receipt references record `retrieval_generation` and
the `dense` or `conversation_dense` reason. Inspection reconstructs the original
source under its recorded read pins and current access policy. The Sources UI
shows original discovery details and states that discovery does not establish
acceptance or authority. Restricted sources retain the existing unavailable
guard.

## Verification record

The primary deterministic seam is a complete agent turn, real SQLite and public
owner approval/Compiler review, with scripted external embedding HTTP. It covers
accepted and raw-conversation paraphrases, Automatic Recall, exact source
receipts, the full context/current-session matrix, indexed live-root exclusion,
UTF-8 retired intervals, source retraction before/after refresh, historical read
pins, concurrent retirement/append during inference, single-row liveness,
sixteen-input resumption, disabled generation reconciliation, replacement,
restart, old-configuration rejection and independent FTS rebuilding.

Focused results while developing:

- `go test -race ./internal/agent -run '^(TestDense|TestMemorySearchTurnFindsParaphrase|TestConversationSearchTurnFindsParaphrase|TestConversationDenseTurn|TestRetrievalUpgrade)' -count=1 -timeout=90s`: final PASS, 44.574 s, including the candidate-starvation regression.
- `go test -race ./internal/localembedding -count=1 -timeout=30s`: final PASS, 11.389 s. Its real HTTP/Unix tests include pre/post manifest checks, opt-out between calls, redirects, proxies, malformed/oversized responses, selected protocol, cancellation and one shared ten-second deadline.
- `go vet ./internal/localembedding ./internal/eviedb ./internal/agent`: PASS.
- `git diff --check`: PASS.
- The opt-in selected-model punctuation-heavy-source probe and long UTF-8 source tests passed after the recorded HTTP 400 regression was fixed. They are development conformance checks, not held-out quality measurements.
- `npx --no-install vitest run src/artifacts/MemoryEvidenceDense.test.tsx` from the UI directory: one UI test PASS, 160 ms, after its missing-discovery-details RED.
- `npx --no-install tsc -b` from the UI directory: PASS.
- `npm run lint` from the UI directory: PASS with five `react(only-export-components)` warnings in unchanged `src/memory/presentation.tsx` and `src/ui/Icon.tsx`.
- `python3 cmd/evie/docs/fixtures/memory-stage5-dense/verify_artifacts.py`: PASS; verifies 768 complete turns, 1536 exact encoded requests and both source archives, retaining the v1 replay's failed gate.

The operating-matrix review added `TestDenseWrongManifestTurnPreservesLexicalEvidenceAndStopsEmbeddingInput` at the complete-turn seam. A real retained generation is reconciled under the selected manifest, then the scripted loopback endpoint reports either a different model name or a changed digest before search. Both cases retain the exact accepted lexical source and report `partial` in the tool outcome, provider memory projection and immutable receipt. The test independently verifies complete provider request bytes/hash and observes one metadata GET with zero body bytes and zero embedding POSTs after replacement. `go test ./internal/agent -run '^TestDenseWrongManifestTurn' -count=1` passes in 0.426s; `go test -race ./internal/agent -run '^TestDenseWrongManifestTurn' -count=1 -timeout=30s` passes in 4.110s. `go vet ./internal/agent` and `git diff --check` pass. This is regression coverage for existing passing production behavior, not a production RED/fix; only the test's decoding of the existing tool-output envelope needed correction. No live model/performance run was needed.

## Actual-model replay and retained failure

The first complete-turn replay used frozen executable SHA256
`8e312acedaf9a177bf01faa3555cf57d9770d8be14b20789c2f77abc0e3cb13d` and verified
the selected model/runtime hashes before measurement. The
[immutable freeze](../fixtures/memory-stage5-dense/v1/frozen-v1/freeze.json),
complete request traces, source mappings, endpoint observations and reports are
retained. The #165 questions, expected evidence and gates were unchanged.
Production's 8-result/12 KiB result/36 KiB turn limits differ from that spike's
4-result/8 KiB evidence-array limits. The replay also includes real turn
persistence, the public approval helper's source prefix, and passive local HTTP
observation overhead. It is a regression replay of an existing partition,
not a new untouched #167/#168 release evaluation.

| Frozen run | Hybrid paraphrase | Lexical paraphrase | Hybrid total | Hybrid whole-turn p50 / p95 |
|---|---:|---:|---:|---:|
| [v1 development](../fixtures/memory-stage5-dense/v1/development-v1/report.json) | 42/48 (87.5%) | 27/48 (56.25%) | 90/96 | 29.859 / 46.888 ms |
| [v1 existing held-out replay](../fixtures/memory-stage5-dense/v1/heldout-v1/report.json) | 39/48 (81.25%) | 27/48 (56.25%) | 87/96 | 33.021 / 47.426 ms |

Each partition has sixteen paraphrase and sixteen lexical-control questions,
with three repetitions. All lexical controls were recovered. No forbidden
scope, secret, or context-cap violation was observed. The first held-out replay
**failed the unchanged 85% paraphrase gate**. Its failure is preserved; it is
not declared complete or replaced by the passing latency result.

Diagnosis used only the two development misses and the general integration
difference from #165. Both failed traces contained exclusively lexical paths:
up to 64 raw lexical suggestions consumed the entire shared candidate allowance
before dense validation. A neutral complete-turn test with seventy unrelated
messages containing `at`, an original `runs before sunrise` source, and the query
`jogging at dawn` reproduced the starvation (RED 0.749 s). Reserving the existing
24-candidate lexical allowance when a dense generation is active leaves forty
of the same total sixty-four reads for dense suggestions (GREEN 0.739 s).
Inactive dense retrieval retains the earlier sixty-four lexical allowance.
The cutoff is explicitly truncated. No model, threshold, gate, or held-out
answer was used to choose this correction.

The revised deterministic dense suite passed in 3.504 s, with vet and whitespace
checks passing. The revised configuration was frozen before running development
again, using executable SHA256
`624388b9891ffccf2cf3b06ad921883c40b29c0a04acc9f999f7624f65bac04d`.
Development passed; the same unchanged executable and freeze then ran the
authorized known-regression replay. Model, weights, thresholds, corpus,
questions, expected IDs and gates remained unchanged.

| Revised frozen run | Hybrid paraphrase | Lexical paraphrase | Hybrid total | Hybrid whole-turn p50 / p95 |
|---|---:|---:|---:|---:|
| [v2 development](../fixtures/memory-stage5-dense/v2/development-v2/report.json) | 48/48 (100%) | 27/48 (56.25%) | 96/96 | 34.183 / 51.456 ms |
| [v2 known-regression replay](../fixtures/memory-stage5-dense/v2/heldout-regression-v2/report.json) | 45/48 (93.75%) | 27/48 (56.25%) | 93/96 | 35.867 / 49.089 ms |

The revised component passes the unchanged 85% paraphrase minimum, ten-point
improvement requirement and 250 ms complete-turn p95 cap. All lexical controls
were recovered, with no observed forbidden/scope disclosure, secret-bearing
embedding input or provider evidence, delivered-count violation, or memory
projection turn-cap violation. One paraphrase already missed by the original
#165 experiment remains missed in all three repetitions. The original v1
failure remains intact. This previously evaluated partition is known-regression
evidence, **not fresh held-out evidence for #167/#168**.

The v2 replay used 73 bounded maintenance batches and 169 actual embedding
requests for 869 inputs. Maintenance took 4.040 s for development and 3.806 s for
the known-regression replay. The largest complete encoded provider request was
34,567 bytes; the largest serialized memory message was 13,463 bytes. That
escaped message measurement differs from the internal 12 KiB result bound.
The [v2 record](../fixtures/memory-stage5-dense/v2/README.md) provides full
condition timings, request traces, byte-count limitations and reproduction
details. Source-only archives preserve both measured builds independently of
later commit rewrites; supplemental manifests verify their compiled inputs.
The original corpus mapping files have a blank accepted `source_link_id` field
because the reporting helper selected the wrong proposal field. Claim/event IDs
and the exact Source Link IDs in provider evidence and receipts remain intact.
The original mapping files are preserved; supplemental associations use only
Source Link IDs actually present in the retained traces. No unobserved mapping
is inferred, and this reporting correction does not change a measurement.

After timed comparisons, the frozen v2 executable passed the separate
[actual-model punctuation-heavy compatibility probe](../fixtures/memory-stage5-dense/v2/compatibility-v2.json)
in 0.50 s. This checks bounded source handling and does not measure reader quality.
Required repository verification, the final two-axis review and the fresh Stage
5 release evaluation remain part of the overall handoff.

Review entry points are `internal/localembedding/client.go`,
`internal/eviedb/retrieval_dense_index.go`, `retrieval_dense_conversation.go`,
`retrieval_dense_query.go`, and the complete-turn `retrieval_dense_*_test.go`
files. To demonstrate the feature locally, configure the selected local model
endpoint, wait for bounded index maintenance to reconcile, record an approved
fact or an original conversation, and ask with different wording. Inspect the
saved request's Sources view to compare the original source and discovery
generation. No external message is sent by this demonstration.


## Legacy fixture compatibility after integration

The first `./scripts/verify-change.sh` run on the eleven-issue branch failed
three legacy-stage tests. Two Issue 104 downgrade fixtures retained a new dense
trigger while removing its canonical source table, and the frozen Stage 3
cross-surface assertion rejected the two now-authorized derived vector tables.
The fixture cleanup now removes both retrieval and dense projections while
asserting byte-for-byte preservation of canonical operations and events. The
Stage 3 assertion permits only the two specific new vector tables; its frozen
1.1.0 capability surface and zero-model-call checks remain intact.

After the fixture changes,
`go test ./internal/eviedb ./cmd/evie -run 'TestSemanticSchemaUpgradesLiteralClaimsFromIssue104AndAcceptsEntityClaims|TestSemanticObjectScopeMigrationSupportsConcurrentLegacyOpens|TestSemanticMemoryStage3CrossSurfaceAcceptance' -count=1`
passed (`internal/eviedb` 0.909 s; `cmd/evie` 0.771 s).
The failed full run had passed UI lint/build and every other Go package. It
stopped at the Go failures, before full Go vet and whitespace checks. UI lint
reported the five existing Fast Refresh warnings; build reported the existing
large-chunk warning. The required full verification will be rerun for #167 and
the final handoff; these focused results do not replace it.
