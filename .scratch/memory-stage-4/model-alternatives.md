# Two bounded local extractor alternatives

Researched 2026-09-05 UTC / 2026-09-04 America/Detroit for
[ticket #135](https://github.com/davidadel66/evie/issues/135), while the existing
Mistral experiment runs. This is a research shortlist, not model selection,
permission to acquire artifacts, or completion of #135/#136. No alternative
model was downloaded, installed, loaded, or tested.

If the current Mistral configuration fails supported-useful precision, the
first proposed next experiment is **Qwen2.5-7B-Instruct Q4_K_M on the existing
Ollama 0.6.3**. Its documented emphasis on JSON and structured data makes it a
plausible extraction candidate, and its architecture is supported by that
runtime. The second option is **Qwen3-4B-Instruct-2507 Q4_K_M**, with a smaller
weight artifact and an explicitly non-thinking instruction model, but it needs
a runtime upgrade. These are reasons to measure them, not evidence of faithful
selective memory extraction. [Qwen2.5 model card](https://huggingface.co/Qwen/Qwen2.5-7B-Instruct),
[Qwen3-2507 model card](https://huggingface.co/Qwen/Qwen3-4B-Instruct-2507),
[Ollama 0.6.3 architecture table](https://raw.githubusercontent.com/ollama/ollama/v0.6.3/llama/llama.cpp/src/llama-arch.cpp).

## Constraint and quality basis

The [local preflight](local-spike-preflight.md) records an M3 Pro with 18 GiB
unified RAM, about 12 GiB free disk, substantial compression/swap, and installed
Ollama 0.6.3. Those are historical snapshots; remeasure before acquiring or
loading a model. Disk bytes are not RAM requirements: model weights, KV cache,
runtime buffers, and the host compete for the same memory. Acquiring only the
first candidate initially avoids retaining both alternatives at once. Do not
convert the cached BF16 models or remove unrelated cached artifacts as part of
this proposal.

The [Stage 4 specification](../../cmd/evie/docs/active/semantic-memory-stage-4.spec.md)
requires supported useful precision and separate required-memory recall,
identity, temporal, attribution, unwanted-proposal, and omission measurements.
Exact citations and schema validity cannot establish entailment or usefulness;
universal abstention cannot qualify. Neither official card reports this Evie
task, its reviewed rubric, or this host. No precision, recall, throughput,
latency, or owner-review advantage is claimed for either alternative.

## Candidate artifacts and runtime compatibility

| Item | First trial: Qwen2.5-7B-Instruct | Conditional second trial: Qwen3-4B-Instruct-2507 |
| --- | --- | --- |
| Explicit Ollama tag | `qwen2.5:7b-instruct-q4_K_M` | `qwen3:4b-instruct-2507-q4_K_M` |
| Weight artifact | 4,683,073,952 bytes, about 4.36 GiB; advertised as 4.7 GB | 2,497,280,480 bytes, about 2.33 GiB; advertised as 2.5 GB |
| All manifest layers plus config | 4,683,087,332 bytes; excludes tiny manifest, filesystem overhead and temporary download state | 2,497,293,803 bytes; excludes tiny manifest, filesystem overhead and temporary download state |
| Quantization / architecture | Q4_K_M / `qwen2` | Q4_K_M / `qwen3` |
| License | Apache-2.0 | Apache-2.0 |
| GGUF context metadata | 32,768 tokens | 262,144 tokens |
| Proposed experiment context | Explicit `num_ctx=4096` | Explicit `num_ctx=4096` |
| Existing Ollama 0.6.3 | Source-supported architecture, tokenizer and JSON-schema path; actual load and extraction are unmeasured | Unsupported `qwen3` architecture in this version; upgrade required |

Artifact identity, size, quantization and license come from the official
[Qwen2.5 tag](https://ollama.com/library/qwen2.5:7b-instruct-q4_K_M) and
[Qwen3 tag](https://ollama.com/library/qwen3:4b-instruct-2507-q4_K_M).
Exact bytes and hashes below were read from public registry manifests, without
fetching weight blobs:
[Qwen2.5 manifest](https://registry.ollama.ai/v2/library/qwen2.5/manifests/7b-instruct-q4_K_M),
[Qwen3 manifest](https://registry.ollama.ai/v2/library/qwen3/manifests/4b-instruct-2507-q4_K_M).
Tags remain mutable; these observations are not verification of downloaded
weight bytes.

Both GGUF artifacts identify their embedded tokenizer as `gpt2` with the
`qwen2` pre-tokenizer, no added BOS, BOS/pad ID 151643 and EOS ID 151645. Their
tokenizers, vocabulary, merges and special-token behavior must be pinned with
the model bytes; Mistral token counts cannot be reused. The respective GGUF
context values in the table are artifact metadata, not a recommendation to
allocate those contexts. [Qwen2.5 GGUF metadata](https://ollama.com/library/qwen2.5:7b-instruct-q4_K_M/blobs/2bada8a74506),
[Qwen3 GGUF metadata](https://ollama.com/library/qwen3:4b-instruct-2507-q4_K_M/blobs/85e4a5b7b8ef).

The upstream tokenizer class is `Qwen2Tokenizer` for both. Qwen2.5's card
describes 131,072-token operation with YaRN beyond the default 32,768; do not
confuse that with the selected GGUF's default. Qwen3's card describes native
262,144 context; its tokenizer configuration's larger `model_max_length` is
not the model context contract. [Qwen2.5 tokenizer configuration](https://huggingface.co/Qwen/Qwen2.5-7B-Instruct/raw/main/tokenizer_config.json),
[Qwen3 tokenizer configuration](https://huggingface.co/Qwen/Qwen3-4B-Instruct-2507/raw/main/tokenizer_config.json),
[Qwen2.5 context instructions](https://huggingface.co/Qwen/Qwen2.5-7B-Instruct#processing-long-texts),
[Qwen3 model overview](https://huggingface.co/Qwen/Qwen3-4B-Instruct-2507#model-overview).

### What is and is not known about minimum versions

- **Qwen2.5:** the schema protocol was introduced in Ollama **0.5.0**.
  The installed **0.6.3** has `qwen2` architecture and pre-tokenizer support and
  converts JSON-schema `format` objects to grammar. Thus no runtime upgrade is
  indicated for this experiment by source inspection. This establishes a
  source-compatible trial target, not a successful load of this exact current
  artifact. The earliest historical version that loaded Qwen2.5 weights alone
  was not established. [Ollama 0.5.0 release](https://github.com/ollama/ollama/releases/tag/v0.5.0),
  [0.6.3 vocabulary](https://raw.githubusercontent.com/ollama/ollama/v0.6.3/llama/llama.cpp/src/llama-vocab.cpp),
  [0.6.3 completion implementation](https://raw.githubusercontent.com/ollama/ollama/v0.6.3/llm/server.go).
- **Qwen3:** `qwen3` is absent from the **0.6.3** and **0.6.5** architecture
  tables and present with model loading/building support in **0.6.6**. Treat
  **0.6.6 as the source-established architecture floor**, not a measured
  guarantee for the later 2507 package, its template, and the complete spike
  API. The exact end-to-end minimum for that combination remains unverified.
  [0.6.5 architecture table](https://raw.githubusercontent.com/ollama/ollama/v0.6.5/llama/llama.cpp/src/llama-arch.cpp),
  [0.6.6 architecture table](https://raw.githubusercontent.com/ollama/ollama/v0.6.6/llama/llama.cpp/src/llama-arch.cpp),
  [0.6.6 model implementation](https://raw.githubusercontent.com/ollama/ollama/v0.6.6/llama/llama.cpp/src/llama-model.cpp).

If a Qwen3 trial is chosen, a concrete current-runtime candidate is **Ollama
0.33.3**, the official latest release observed here, published 2026-09-02. Its
Darwin command-line archive is 159,236,337 bytes compressed; expanded files need
additional disk. Use an independently pinned installation for the experiment
and preserve the existing runtime/results. This version is a proposed smoke-test
target, not a tested recommendation; a runtime change requires repeating
transport, cancellation and capacity-release checks. [Official release](https://github.com/ollama/ollama/releases/tag/v0.33.3),
[release API including asset sizes](https://api.github.com/repos/ollama/ollama/releases/latest).

The selected Qwen3 artifact is the **2507 Instruct** release: its model card
states that it does not emit thinking blocks. Its shipped template ends at an
assistant message without a thinking prefix, and its defaults include
temperature 0.7, top-k 20, top-p 0.8 and repeat penalty 1. Preserve and record
these artifacts, then explicitly override the intended experiment parameters;
do not accidentally test the generic `qwen3:4b` tag or rely on a `think=false`
switch as an artifact identity. [Exact Qwen3 template](https://ollama.com/library/qwen3:4b-instruct-2507-q4_K_M/blobs/eade0a07cac7),
[exact Qwen3 parameters](https://ollama.com/library/qwen3:4b-instruct-2507-q4_K_M/blobs/0914c7781e00).

## Registry identities observed

All values below are SHA-256 digests. Manifests were hashed from fetched metadata;
weight identities are registry declarations only. A subsequent trial must hash
the actual local weight, template, parameter, system and runtime files.

| Artifact | Digest |
| --- | --- |
| Qwen2.5 manifest | `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e` |
| Qwen2.5 weights | `2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730` |
| Qwen2.5 template | `eb4402837c7829a690fa845de4d7f3fd842c2adee476d5341da8a46ea9255175` |
| Qwen2.5 system | `66b9ea09bd5b7099cbb4fc820f31b575c0366fa439b08245566692c6784e281e` |
| Qwen2.5 config | `2f15b3218f0552c60647ce60ada83632d2c09755b16259b13e3e4458e9ae419d` |
| Qwen3 manifest | `0edcdef34593eac1aa2be9c7d06c432dcf81945adca5eca2f27662c18f168ba0` |
| Qwen3 weights | `85e4a5b7b8ef0e48af0e8658f5aaab9c2324c76c1641493f4d1e25fce54b18b9` |
| Qwen3 template | `eade0a07cac7712787bbce23d12f9306adb4781d873d1df6e16f7840fa37afec` |
| Qwen3 parameters | `0914c7781e001948488d937994217538375b4fd8c1466c5e7a625221abd3ea7a` |
| Qwen3 config | `b72accf9724e93698c57cbd3b1af2d3341b3d05ec2089d86d273d97964853cd2` |

Sources: the two official registry manifests linked above. Qwen2.5 has no
parameter layer in this observed manifest; record the runtime defaults used.

## Proposed next trial, if separately authorized

1. Finish and adjudicate the existing Mistral experiment first. Record why its
   outputs are inadequate, including failure slices. Preserve its manifest,
   raw outputs and human decisions. A malformed-output failure and a
   supported-useful precision failure call for different interpretation.
2. Acquire **only Qwen2.5** initially, directly as the published quantized
   artifact; no BF16 intermediate or conversion. Check current disk and host
   pressure first. Pin actual bytes and effective template/system/defaults,
   and the unchanged 0.6.3 runtime. Do not infer fit from 18 GiB installed RAM.
3. Create a new experiment manifest, retaining the current ten development
   case IDs and reviewed input/gold hashes. Reuse the existing prompt, closed
   output schema and runner to isolate the model change. Run **one schema
   configuration, two repetitions, seeds 17/18: at most 20 quality requests**.
   Use the first scheduled case as the load/schema smoke test and stop on
   load/protocol failure. Keep `temperature=0`, `num_ctx=4096`,
   `num_predict=768`, `keep_alive=1m`, and 60-second request timeout. These
   mirror the current experiment; they are not a promised optimum. The
   corresponding Mistral schema run is the paired baseline.
4. Preserve explicit loopback binding, rejected redirects/nonlocal targets,
   and no remote fallback. Run one request at a time and keep only one model
   resident. Reserve at most two additional synthetic diagnostic requests for
   timeout/cancellation and observed capacity release, recorded separately
   from quality requests. Recheck prompt token counts for every model; reject
   input truncation or incomplete output rather than crediting a partial run.
5. Report raw versus retained proposals separately. Human-adjudicate unlisted
   interpretations; reviewed source labels do not approve all generated
   meanings. Compare supported-useful precision and required recall with
   denominators, attribution/identity/time/polarity errors, unsupported and
   unwanted proposals, omissions, truncations and repetition disagreement.
   Record latency and model/host memory/swap observations without inferring
   foreground or review budgets.
6. Only if this remains inadequate, consider a separate Qwen3 trial with its
   runtime change pinned and checks repeated. Start with the same bounded
   schema/temperature-zero comparison; the publisher's general sampling
   recommendation differs, so any later 0.7-temperature trial must be a named,
   pre-recorded configuration, not an unreported retry. A smaller weight file
   is not proof of better precision or overall memory use.

The current comparison basis is the
[experiment manifest](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/experiment-manifest.json)
and [runner contract](../../scripts/memory-extractor-spike/README.md).
Ollama's documented schema interface constrains output shape; it still calls
for response validation. [Structured-output documentation](https://docs.ollama.com/capabilities/structured-outputs).
Remaining development families and pilot coverage must precede selection.
Final-holdout contents remain outside this work. Neither a successful smoke
test nor this shortlist unblocks the production extractor decision.

## Scope and verification

Read the repository agent guide, parent specification, #135 ticket text,
preflight, current spike report/manifest, and relevant runner request code;
consulted current official model pages/cards, metadata and versioned runtime
source. Read-only network requests fetched documentation and small metadata
only. Wrote only this scratch file. No model/runtime acquisition, inference,
installation, production edit, commit, or private-history egress occurred.

Verification: `git diff --check` passed (exit 0).
`git diff --no-index --check /dev/null .scratch/memory-stage-4/model-alternatives.md`
reported no whitespace errors (exit 1 because the file is added). A Python
stdlib check resolved all four local Markdown links with no missing targets
(exit 0). Code tests and `./scripts/verify-change.sh` are skipped because this
subtask changes scratch documentation only, with no executable changes.
