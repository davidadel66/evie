# Corrected prompt input budget

This is a conservative, model-specific predispatch bound for the standalone
spike. It does not supply the production compiler's input-budget contract.
The original 4096-context runs retain their separate empirical request budgets.

The implementation owner independently inspected the pinned GGUF metadata and
Ollama v0.6.3 source before the corrected comparison. The complete GGUF SHA-256
is `ff82381e2bea77d91c1b824c7afb83f6fb73e9f7de9dda631bcdbca564aa5435`.
[Tokenizer evidence](tokenizer-proof.json) records the exact metadata-region hash,
32768 tokens, SPM/llama tokenizer, default preprocessor, BOS enabled, EOS disabled,
NORMAL space token `▁` at ID 29473, and all 771 unknown/control/user-defined
special token spellings at least three UTF-8 bytes. The model's stored Jinja
chat template is not the Ollama template: the installed template layer and
observed API template both hash to
`491dfa501e59ed17239711477601bdc7f559de5407fbd4a2a79078b271045621`.

In this SPM implementation, ASCII spaces become a known one-token symbol;
ordinary UTF-8 symbols either merge into a token or fall back to at most one
token per byte. Recognized specials consume at least three original bytes and
produce one token, paying for a possible prefix space in the following raw
fragment. Only one initial prefix-space token and one BOS remain. Therefore,
for **B bytes of the complete rendered UTF-8 prompt, input tokens are at most
B + 2**. This derives from inspected tokenizer/vocabulary behavior, not an
estimated tokens-per-byte ratio.
([SPM splitting, merging and byte fallback](https://github.com/ollama/ollama/blob/v0.6.3/llama/llama.cpp/src/llama-vocab.cpp#L105-L201),
[special-token partition and SPM preprocessing](https://github.com/ollama/ollama/blob/v0.6.3/llama/llama.cpp/src/llama-vocab.cpp#L2152-L2382))

The generate request has one nonempty system string and one user prompt, no
images, tools, suffix, stored model messages, raw mode, template override, or
prior token context. The installed manifest contains no system/message layer.
The runtime collects the system string and keeps the two messages; its stored
Go template renders exactly `[INST] ` + system + two newlines + input + `[/INST] `.
The spike executes that exact template using Go's standard template engine and
requires byte-for-byte equality with this rendering before accepting its bound.
The fixed overhead is 17 UTF-8 bytes. The schema/JSON format travels separately
to decoding and does not add prompt text.
([Generate request rendering](https://github.com/ollama/ollama/blob/v0.6.3/server/routes.go#L241-L292),
[template execution and collation](https://github.com/ollama/ollama/blob/v0.6.3/template/template.go#L209-L303),
[format handling](https://github.com/ollama/ollama/blob/v0.6.3/llm/server.go#L625-L661))

`prompt-v2.txt` is 5588 bytes, appending the common output-field glossary and
minified unchanged schema to the original policy. Both format arms receive the
same complete system text. Before **any** inference, the executable validates
all selected source windows and all requested repetitions, verifies the pinned
runtime/model/template/proof identities, and requires for each full rendering:

`B + 2 + 768 output tokens + 64 reserve tokens <= 8192 context tokens`.

The 64 tokens are extra reserve, not an allowance for unaccounted rendering.
An unknown prompt/proof/template/runtime, unsupported configuration, or excessive
input fails before the first HTTP inference. Public executable tests exercise
both valid formats, changed prompt identity, a valid but oversized later source,
and all-request preflight; no live model is used by those tests. The installed
runtime/model tag/API metadata were independently rechecked immediately before
freezing the corrected experiment in [preflight observations](reports/corrected-pass-preflight.json).
The executable pins the inspected evidence files; the experiment manifest pins
the actual executable. These are controlled local-experiment identities, not a
claim that an arbitrary external server implements the same model or tokenizer.
