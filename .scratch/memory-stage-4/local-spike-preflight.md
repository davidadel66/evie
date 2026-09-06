# Local extractor spike preflight

Observed 2026-09-05T02:12:39Z (2026-09-04 in America/Detroit) on
`codex/memory-stage-4`, HEAD `9ef07713ea6bcc98a5074920efa620b8b4071840` with
pre-existing working-tree changes. This is preparatory evidence for
[ticket 04](issues/04-local-extractor-spike.md), not the completed spike or an
extractor selection. Ticket 04 still depends on the evidence/closure contract.

## Available resources

| Resource | Observation |
| --- | --- |
| Host | Apple M3 Pro, MacBook Pro Mac15,6, arm64, macOS 15.7.8 |
| CPU / GPU | 11 CPU cores (5 performance, 6 efficiency); 14 GPU cores; Metal 3 |
| Physical memory | 19,327,352,832 bytes (18 GiB), shared by host and GPU workloads |
| Current memory snapshot | About 0.81 GiB free pages, 2.43 GiB wired, 4.73 GiB occupied by compressor; swap 4,622.25 MiB used of 6,144 MiB. Free-page count is not total reclaimable memory. |
| Disk snapshot | Data volume reports 12 GiB available, 97% capacity used; future downloads, conversion copies, and evaluation outputs must account for this headroom. |
| Installed runtime | `/Applications/Ollama.app/Contents/Resources/ollama`, version 0.6.3, universal binary including arm64; app version also 0.6.3. It is not on the current PATH. |
| Runtime state | Ollama client reports no running instance; `127.0.0.1:11434/api/tags` refuses connections. No inference request was made. |
| Other runtime checks | No `llama-cli`, `llama-server`, MLX-LM CLI, `lms`, `vllm`, or `llamafile` on PATH. No LM Studio or Jan app in checked `/Applications` locations. |
| Python tooling | Default Python 3.13.4; `uv` exists but lists no installed tools. MLX, MLX-LM, llama-cpp-python, PyTorch, Transformers, vLLM, and ONNX Runtime are absent from this default interpreter. Other uninspected environments may exist. |

Read-only requests disabled proxies and refused redirects. LM Studio's common
`127.0.0.1:1234/v1/models` and `127.0.0.1:8000/v1/models` refused connections;
`127.0.0.1:8080/health` reset the connection. These are observations at checked
addresses, not an exhaustive endpoint inventory or proof of server behavior.

## Cached model artifacts

Ollama manifests and file sizes are present locally:

| Cache name | Manifest metadata | Model bytes | Recorded model digest |
| --- | --- | ---: | --- |
| `mistral:latest` | GGUF, 7.2B, Q4_0 | 4,113,289,152 (3.83 GiB) | `sha256:ff82381e2bea77d91c1b824c7afb83f6fb73e9f7de9dda631bcdbca564aa5435` |
| `mixtral:latest` | GGUF, 46.7B, Q4_0 | 26,443,590,560 (24.63 GiB) | `sha256:f2dc41fa964b42bfe34e9fb09c0acdcfbfd6e52f1332930b4eacc9d6ad1c6cd2` |

The digests above come from existing manifests and matching blob filenames;
the large weight files were not rehashed. Mutable `latest` tags do not pin a
reproducible spike. Verify actual bytes and pin all model/template/parameter
artifacts before reporting model results.

Mistral is a plausible already-cached starting configuration based only on its
weight footprint and the installed runtime. Runtime buffers, context/KV cache,
and host workload add to its footprint. No load, schema support, quality,
latency, throughput, cancellation, or capacity-release result has been measured.
Mixtral's weights alone exceed physical RAM; it is unsuitable as the first
bounded baseline on this currently busy host without separately measuring its
resource consequences. No model was started or selected.

The Hugging Face cache also contains:

- Llama-3.2-1B-Instruct snapshot `e9f8effbab1cbdc515c11ee6e098e3d5a9f51e14`,
  BF16 weights 2,471,645,608 bytes with tokenizer/config files.
- Llama-3.2-3B-Instruct snapshot `0cb88a4f764b7a12671c53f0838cd831a0843b95`,
  BF16 weights across two shards totaling 6,425,529,048 bytes with tokenizer/config
  files. Another cached snapshot has config/tokenizer files but no observed weights.
- Other cached artifacts primarily serve document processing, embeddings, or
  classification; their extraction suitability was not assessed.

These Hugging Face weights are not a verified runnable configuration in the
checked default Python environment. Their presence does not establish conversion
support, complete cache integrity, or model quality.

## Existing evaluation conventions

- [Fixture README](../../cmd/evie/docs/fixtures/semantic-memory/evaluation/README.md)
  describes the Stage 3 versioned JSON manifest and closed report schemas under
  `cmd/evie/docs/fixtures/semantic-memory/evaluation/v1/`. The current corpus has
  six real-SQLite conformance cases and imports seven frozen operation fixtures.
- [Shared report code](../../internal/memoryeval/evaluation.go) has separate
  `semantic_conformance`, `learned_extraction`, `retrieval_provenance`, and
  `answer_abstention` panels; fixture hashes, component identities, run/commit
  identity, environment, cardinality, case failures, repetition counts,
  p50/p95/max, and paired baseline deltas already exist. Stage 4 should preserve
  the conformance panel and version any required schema/failure-taxonomy extension.
- [Existing runner](../../internal/eviedb/semantic_evaluation_test.go) is
  `go test -run TestSemanticEvaluation -v ./internal/eviedb`. It uses temporary
  SQLite databases without a model/network and logs `semantic-evaluation-json=`
  plus Markdown. Its performance baseline uses five repetitions, not a Stage 4
  repetition policy. It supports `EVIE_EVALUATION_COMMIT` and
  `EVIE_EVALUATION_HARDWARE` labels. Reports are persisted externally for comparisons.
- No standalone extraction runner or human annotation/custody record was
  established by this preflight. Stage 3 operation fixtures are not human-reviewed
  extraction gold. Research documents are design evidence, not approval to freeze
  an extractor input contract.

## Remaining prerequisites and next steps

1. Complete ticket 01's binding projection, source-coordinate, eligibility,
   window/overlap, useful-memory, and named tool-observation rules. Freeze
   synthetic source events and projected input only against that contract.
2. Establish actual human annotation and meaning/usefulness adjudication. The
   ticket's required/optional/unwanted/unsupported distinctions, supported
   equivalence, identity alternatives, and uncertainty cannot be replaced by
   agent self-review. No human-reviewed Stage 4 labels were provided or observed
   in the assigned materials.
3. Assign final-holdout curator/custodian and define its location, access boundary,
   annotation record, and exposure log before authoring or exposing final cases.
   Separate development, pilot/model-selection, and final holdout by complete
   narrative and variant lineage. Keep final contents outside model/prompt
   selection and pilot tuning until configuration and pilot-derived gates freeze.
   No final-holdout contents were sought or inspected here.
4. After prerequisites, verify an explicitly loopback-bound Ollama startup using
   the installed binary and cached small model, with bounded context/output and
   one request at a time. Pin runtime/artifact/schema/prompt/decoding/environment
   and actual hashes. Record an idle resource baseline and preserve enough disk
   for results; additional runtime/model acquisition needs its own concrete plan.
5. Implement the standalone runner and scoring artifacts against the frozen
   contract; compare a recorded bounded set of configurations with repeated runs.
   Exercise malformed/truncated output, bounds, timeout, cancellation, unavailable
   endpoint, redirect/nonlocal rejection, and no fallback. Measure client return,
   simulated late-effect prevention, and real server capacity release separately.
6. Keep ticket 04 incomplete until actual local inference and human-reviewed
   quality/resource results exist. Do not infer owner-review burden or production
   foreground overhead from this standalone preflight or eventual spike.

## Scope and verification

Read only the assigned specification/decisions, ticket text, evaluation code,
hardware/runtime metadata, model manifests/configs/file sizes, and checked local
endpoint status. No private conversation store, secret configuration, environment
credential dump, or final holdout was read. No inference server was started; no
model inference, downloads, installs, production edits, or commits occurred.
Only this scratch report was written. Runtime/hardware observations used
`sysctl`, redacted `system_profiler`, `df`, `vm_stat`, `lsof`, runtime path checks,
Python stdlib metadata/loopback probes, `file`, `ollama --version`, and
`uv tool list`.

Code tests and `./scripts/verify-change.sh` were not run because this subtask is
read-only preflight plus scratch documentation, with no executable change.

Verification: `git diff --check` passed. The explicit untracked-file check,
`git diff --no-index --check /dev/null .scratch/memory-stage-4/local-spike-preflight.md`,
emitted no whitespace errors (exit 1 because the file is added). A Python stdlib
check resolved all four Markdown file links with no missing targets.
