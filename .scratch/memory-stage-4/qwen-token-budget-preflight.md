# Qwen2.5 token-budget preflight for Stage 4 spike

Researched 2026-09-05 UTC for #135. This is a conditional proof and acquisition
checklist for the exact Qwen2.5 artifact in [the alternatives note](model-alternatives.md).
It does not authorize acquisition, establish compatibility by execution, select
a model, or unblock #136. No model bytes were downloaded or tokenized, and no
inference was requested. Mistral's SPM proof and measured counts are not used.

**Recommendation:** a Qwen-specific UTF-8 byte bound can cover the current
5,588-byte system prompt at context 8,192, but only after checking the acquired
GGUF's byte alphabet and merge closure. A tokenizer family name alone is not
enough. Keep the conservative allowance of two tokenizer-added tokens; require
the entire rendered prompt plus 768 output tokens and 64 reserve tokens to fit
before sending the first quality request. These are experiment settings, not
production memory capacity or quality gates.

## Pinned target and evidence

The registry still returns these identities for
`qwen2.5:7b-instruct-q4_K_M`. The manifest, template, system and config bytes were
fetched and hashed during this preflight. The weight digest is a registry
declaration, not a local verification. The observed manifest has no messages,
parameters, adapter or projector layer. [Official manifest](https://registry.ollama.ai/v2/library/qwen2.5/manifests/7b-instruct-q4_K_M).

| Item | SHA-256 | Bytes |
| --- | --- | ---: |
| Manifest | `845dbda0ea48ed749caafd9e6037047aa19acfcfd82e704d7ca97d631a0b697e` | — |
| GGUF weights | `2bada8a7450677000f678be90653b85d364de7db25eb5ea54136ada5f3933730` | 4,683,073,952 |
| Go template | `eb4402837c7829a690fa845de4d7f3fd842c2adee476d5341da8a46ea9255175` | 1,482 |
| Default system | `66b9ea09bd5b7099cbb4fc820f31b575c0366fa439b08245566692c6784e281e` | 68 |
| Config | `2f15b3218f0552c60647ce60ada83632d2c09755b16259b13e3e4458e9ae419d` | 487 |

The published GGUF metadata identifies `general.architecture=qwen2`,
`tokenizer.ggml.model=gpt2`, `tokenizer.ggml.pre=qwen2`, context 32,768 and
`tokenizer.ggml.add_bos_token=false`. It exposes vocabulary and merge arrays,
but this preflight has not verified their completeness against local bytes.
The experiment should explicitly request 8,192 rather than inherit the
artifact's context. [Published GGUF metadata](https://ollama.com/library/qwen2.5:7b-instruct-q4_K_M/blobs/2bada8a74506).

Ollama's `v0.6.3` tag resolves to commit
`e5d84fb90b21d71f8eb816656ca0b34191425216`.
[Official tag reference](https://api.github.com/repos/ollama/ollama/git/ref/tags/v0.6.3).
The relevant raw source bytes were independently fetched and hashed:

| Source under that commit | SHA-256 |
| --- | --- |
| `server/routes.go` | `e6680b0b00d435482dd5e2f078c13e1f8c6db0f882150c93b32c70d14ce13b99` |
| `template/template.go` | `57b4d859357cb844e133c60b7171b00507901879146dc8d6625a9c3b4f8ae1ba` |
| `llm/server.go` | `4aac4500a4b87a8ecf7506af23786b3bf056015c2a0bc44c38671f26606e756b` |
| `llama/llama.cpp/src/llama-vocab.cpp` | `647d544bb3c8ed4da62b4234300e5e1bcf0f31a69faf8ce45db956449566674f` |
| `llama/llama.cpp/src/unicode.cpp` | `3cbd22328b318da057942f608d7a1f8dc304a77ce51b08d85b6f89d2be944328` |
| `runner/llamarunner/runner.go` | `39f254a313c5153841c5c7b603eee3befe22a6b73eddb16869cb32a22ef222d3` |
| `llama/llama.go` | `ca4587225bb7cafef4ca7f1260f378d6b9f5c1837e7e2c64cb88add7c4dda73e` |

The installed executable and its runner libraries still need their own recorded
identities. A source tag is not proof that an arbitrary server implements it.

## Exact generate-request rendering

Scope this proof to the existing spike's `/api/generate` request: one nonempty
`system`, one nonempty `prompt`, `raw=false`, and no template override, suffix,
prior token context, images, tools or stored model messages. In 0.6.3 the
nonempty request system replaces the model's default system; it is not appended
to it. A missing or empty request system would select the model default and
must fail this experiment's preflight. [Generate route](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/server/routes.go#L240-L294).

Template execution collects the system text while retaining the system and
user messages. The Qwen template's message loop renders the user role; the
system message is already rendered through `.System`, so it is not duplicated.
There are no consecutive same-role messages to coalesce. `.Response` is empty.
[Execution and collation](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/template/template.go#L221-L319).

For this input shape the exact expected rendering is the following string
concatenation, including each newline:

```text
"<|im_start|>system\n" + system + "<|im_end|>\n" +
"<|im_start|>user\n" + prompt + "<|im_end|>\n" +
"<|im_start|>assistant\n"
```

The fixed overhead is **80 UTF-8 bytes**. This is derived from the fetched
template, including its whitespace-trimming actions. Before dispatch, execute
the pinned template using Go's template engine and require byte-for-byte
equality with that rendering. This preflight did not execute a template helper
while the timed Mistral run was active. The stored GGUF Jinja template is not
the template selected by this Ollama manifest.
[Exact template blob](https://registry.ollama.ai/v2/library/qwen2.5/blobs/sha256:eb4402837c7829a690fa845de4d7f3fd842c2adee476d5341da8a46ea9255175),
[manifest template loading](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/server/images.go#L279-L296).

The JSON-schema `format` field is converted to decoding grammar separately; it
does not add a second schema copy to the input. The current system text already
contains the schema. Pin the effective options and the actual runner path;
set `OLLAMA_NEW_ENGINE=false` for this proof. The selection code uses the C++
runner when no alternate text processor is selected, and only `gemma3` forces
the newer engine at this version. [Format and engine selection](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llm/server.go),
[engine-required check](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/fs/ggml/ggml.go#L136-L138).

## Conditional byte/BPE proof

For valid UTF-8, this runtime partitions the original code points, reconstructs
their original bytes, then maps each byte to one encoded Unicode symbol.
Category collapsing is used for regex matching, not as replacement input. Thus
the initial symbol count equals the original byte count, without normalization
expansion. This requires rejecting invalid UTF-8 before dispatch.
[Unicode partition and byte encoding](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/unicode.cpp#L159-L255),
[partition reconstruction](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/unicode.cpp#L692-L870).

The BPE algorithm merges symbols. However, an absent final vocabulary symbol
falls back over its *encoded UTF-8 bytes*, which can expand the count relative
to original bytes. Verify all 256 byte-map symbols exist and every merge's
concatenated result exists, using the runtime's merge parsing rule. Then every
reachable final symbol is a vocabulary entry and emits one token. Recognized
specials replace nonempty source spans with one token; require all active
special spellings nonempty. The BPE path adds at most one BOS and one EOS and
does not apply the SPM space-prefix transformation. Hence, conditionally,
**input tokens ≤ B + 2**, where B is the complete rendering's UTF-8 byte count.
This is a proof from the inspected implementation, not a typical-language
tokens-per-byte estimate. [BPE merges and fallback](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/llama-vocab.cpp#L443-L587),
[special partition and BPE dispatch](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/src/llama-vocab.cpp#L2152-L2408).

This closure check matters even if the published tokenizer is expected to be
well formed. Failure means reject the bound and inspect or use a verified exact
local tokenizer; do not quietly reuse Mistral's evidence or an online token
estimator. No Qwen tokenizer request, including a vocabulary-only load, has
been made in this subtask.

## Fit for the frozen source-only corpus

The local [corrected system prompt](../../cmd/evie/docs/fixtures/memory-stage-4-spike/v1/prompt-v2.txt)
is 5,588 bytes with SHA-256
`f6f280093b15e1a1928db2737ec9bde8b2e0471ef66cbeab11de47b0ca57c832`.
Let J be the exact UTF-8 byte count of the request's decoded `prompt` value:
the compact source-input JSON string. Count neither the raw source file nor
the outer HTTP JSON's escape overhead.

```text
B = 5588 + J + 80
accept only if B + 2 + 768 + 64 <= 8192
therefore J <= 1690 bytes
```

An independent Python reconstruction of the current source-only fixtures,
including Go-style HTML and U+2028/U+2029 escapes, gives these maxima. This
calculation is preparation; the runner must repeat the bound on its actual
request bytes after all validation and serialization.

| Split | Largest window | J | B | Conservative input + output + reserve |
| --- | --- | ---: | ---: | ---: |
| Development, 18 windows | `N08-b` | 1,263 | 6,931 | 7,765 |
| Pilot, 6 windows | `N11-b` | 946 | 6,614 | 7,448 |

The largest current window leaves 427 tokens of additional headroom under
this conservative bound. This does not promise that 768 output tokens suffice;
output truncation remains a quality/protocol failure. No gold labels or final
holdout contents were read for this calculation.

## Required verification after separately authorized acquisition

The implementation owner should produce a new Qwen-specific proof artifact
and experiment manifest before any Qwen quality inference:

1. Hash the actual model and every installed manifest layer. Require the exact
   identities above, and reject unexpected messages, parameters, adapters or
   projectors. Record current runtime executable/libraries, version, server
   launch configuration, effective model metadata and template. Preserve
   Mistral's immutable artifacts.
2. Parse the local GGUF metadata without loading tensors. Record the metadata
   region hash, architecture, tokenizer model/preprocessor, token and merge
   array hashes/counts, special IDs/types/spellings and BOS/EOS flags. Check
   the byte-alphabet, merge-closure and nonempty-special conditions above;
   reject malformed merge entries. A hash of the proof file alone does not
   prove the inspected model bytes.
3. Execute the pinned template on the exact request shape and compare the full
   rendering with the derived string. Check all selected windows/repetitions
   before the first HTTP inference. Reject changed system/template/model,
   unknown options/fields, invalid UTF-8, absent proof or excessive input.
   The 64-token reserve must not hide unspecified template overhead.
4. Exercise the public executable with a local scripted HTTP server: the
   largest allowed boundary, one byte over, changed prompt/model/template,
   missing or corrupted proof, and an oversized later case must all behave
   correctly before any request is emitted. No new production dependency is
   needed to establish the byte bound.
5. Only then use the first scheduled synthetic quality case as the actual
   load/schema smoke test. Confirm the C++ runner and effective context 8,192,
   record logs and response token counts, and stop on load/protocol problems.
   This smoke test remains unperformed and its success cannot be inferred
   from architecture support.

The C++ runner tokenizes text with special-token parsing, truncates oversized
input to the configured cache context and stores the resulting input count in
`prompt_eval_count`. Consequently a count observed only after inference is not
the primary predispatch safeguard. Capture truncation logs as a diagnostic,
and never credit a truncated prompt. [Runner input handling and result count](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/runner/llamarunner/runner.go#L103-L188),
[response reporting](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/runner/llamarunner/runner.go#L650-L662).

This proof is scoped to the pinned standalone Qwen experiment. #136 still
needs an input-budget design that supports its own bounded source windows and
model/runtime policy; this scratch note is not a production implementation.

## Verification of this research subtask

Read the alternatives note, existing spike request/budget code, source-only
development/pilot fixtures and versioned primary sources. Network requests
fetched documentation, source and small registry metadata only. Wrote only
this scratch note. No server interaction, model acquisition, inference,
runtime installation, source/gold edits, Git staging or commit occurred.

`git diff --check` passed. The new-file whitespace check
`git diff --no-index --check /dev/null .scratch/memory-stage-4/qwen-token-budget-preflight.md`
reported no whitespace errors. Local Markdown targets were checked to exist.
Code tests and `./scripts/verify-change.sh` were skipped because this subtask
only creates scratch documentation and must not disturb the active timed run.
