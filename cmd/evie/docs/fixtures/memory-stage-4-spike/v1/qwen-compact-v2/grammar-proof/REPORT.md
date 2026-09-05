# Ollama v0.6.3 closed reference grammar proof

The exact `qwen-compact-v2/output.schema.json` compiles with Ollama v0.6.3's unchanged JSON Schema converter. The unchanged v0.6.3 grammar parser and character acceptance engine pass all 62 full-output cases in `cases.json`. This was an offline source-level test; no server, inference, tokenizer, model download, or model load ran.

## Reproduce

From the repository root, with the pinned sources already present in the scratch cache:

```sh
python3 cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/grammar-proof/run.py --schema cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v2/output.schema.json --cache .scratch/memory-stage-4/ollama-v0.6.3-schema-proof --offline
```

After copying the package elsewhere, invoke its `run.py` with the same `--schema` and a scratch `--cache`. Omit `--offline` only to populate missing source files. Downloads use immutable commit URLs and verify SHA-256 before writing. Cached sources are verified on every run. The script never fetches runtime binaries or model data.

`run.py` records the exact compiler invocation in `build-command.txt`, compiler details in `compiler-version.txt`, and output in `build-output.txt` and `test-stderr.txt`. Final build and test output contain no warnings. Apple clang 17.0.0, arm64-apple-darwin24.6.0, compiled the proof with C++17, `-O2`, and `-Wl,-dead_strip`. The linker removes unused token/model paths. A first development compile failed because the standalone harness lacked `string_split`; adding the unchanged pinned helper resolved it.

## Provenance and harness scope

The [v0.6.3 tag](https://api.github.com/repos/ollama/ollama/git/ref/tags/v0.6.3) resolved to commit `e5d84fb90b21d71f8eb816656ca0b34191425216`. All 21 files in `sources.json` were fetched by tag, then independently compared byte for byte against their immutable commit URLs. The manifest records all paths, sizes, and hashes.

| File | SHA-256 |
| --- | --- |
| Tested output.schema.json | `ad6ce8c0ea12f11345a3d421d710f718d9535ba24616858d78c2f207d4288c6e` |
| json-schema-to-grammar.cpp | `82d8c5aa1a56b806e5119c422472c0fd802ab23f5f7046bbba570ea9286de014` |
| llama-grammar.cpp | `250385da4137be2361b9e213900f2b289f82d051b8ef462f2a377653d7ca8630` |
| Full output.gbnf | `4bc1014edc8eb48abcd5a58d9a8176996d69ed71f0bb7a40ed090e01b8ad5316` |
| Reference-only reference.gbnf | `69bbd8915bb4691f2b01e6cb3bc3a4af687422cdf6acef40e65b18466f5c7db6` |
| results.txt | `572d13182e5870cf5a5f1fc54baff294b8cb2b20d6ff9bff9b575862aa9ab8c6` |

`proof.cpp` parses the exact schema bytes with `nlohmann::ordered_json` and calls `json_schema_to_grammar(schema)` with its default argument. It additionally verifies that forced GBNF is identical. `LLAMA_USE_LLGUIDANCE` is not defined. It initializes the upstream grammar with a null vocabulary, explicitly allowed for testing in `llama-grammar.h`, consumes each ASCII test string with `llama_grammar_accept`, and accepts complete output only when an empty grammar stack remains.

`link-support.cpp` provides verbatim `string_join`, `string_split`, and `string_repeat` functions from the pinned `common.cpp`; `run.py` verifies those copies. Its only custom hooks print diagnostics or abort. No converter, grammar parsing, character matching, or grammar acceptance code is replaced. The full Ollama executable and token-level sampler are not exercised.

The pinned [CGo bridge](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/sampling_ext.cpp#L50) likewise uses `ordered_json` and the default converter call. [SchemaToGrammar](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.go#L672) supplies a 32,768-byte output buffer; this schema's generated grammar is 6,213 bytes.

## Observations

Both `sources` and `context` accept ref-only, whole, date, and integer range references. They reject unknown properties, mixed whole/date plus bounds, missing range bounds, negative start, nonpositive end, noninteger bounds, absent or nonstring refs, and unknown/null selectors. Source minimum size and both eight-item maxima also hold. These results exercise the pinned converter's [union, object, enum, and integer branches](https://github.com/ollama/ollama/blob/e5d84fb90b21d71f8eb816656ca0b34191425216/llama/llama.cpp/common/json-schema-to-grammar.cpp#L839).

The generated range rule enforces `end, ref, selector, start` order because the exact schema's `properties` are sorted. It follows `properties`, not the `required` array. The explicit `generator-limitation/schema-valid-range-property-order` case is valid JSON Schema data but is correctly expected to fail this narrower generation grammar when supplied as `ref, selector, start, end`.

Range numbers are emitted as up to 16 decimal digits. Unknown ref values and `start >= end` are accepted by the grammar, as the schema does not encode source membership or cross-field comparisons. Source capability, meaningful ranges, scope, and other semantic restrictions still require the existing compiler checks. The proof vectors are ASCII only and do not establish Unicode/tokenizer behavior or model output quality.

## Minimal package to retain

Retain `run.py`, `sources.json`, `proof.cpp`, `link-support.cpp`, `cases.json`, `REPORT.md`, `output.gbnf`, `reference.gbnf`, `results.txt`, and `artifacts.json`. Optionally retain `build-command.txt`, `compiler-version.txt`, and the empty build/test diagnostic logs for the recorded run. Sources and compiled binaries can remain in scratch; no vendored runtime dependency is needed. The schema is supplied using `--schema` and verified against its frozen hash, so it need not be duplicated in the package.
