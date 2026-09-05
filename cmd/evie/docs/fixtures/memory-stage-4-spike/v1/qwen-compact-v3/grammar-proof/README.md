# Pinned runtime proof for the actual compact-v3 request schemas

The exact schemas retained in `reports/predispatch.json` pass **177 offline
source/context and empty-array grammar cases** through the unchanged pinned
Ollama0.6.3 JSON-schema converter and grammar parser/character consumer. The
source harness first rebuilds and passes the inherited 62 closed-reference
cases. No model, tokenizer, server, inference or model download is used.

From the repository root, with the pinned sources already cached:

```sh
python3 cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3/grammar-proof/run.py \
  --cache .scratch/memory-stage-4/ollama-v0.6.3-schema-proof \
  --output .scratch/memory-stage-4/compact-v3-actual-grammar-proof --offline
```

The compiler harness, source URLs, immutable commit hashes and character-engine
scope are pinned in the [inherited source proof](../../qwen-compact-v2/grammar-proof/REPORT.md).
Omitting `--offline` permits only the listed small immutable source downloads;
it never acquires a runtime or model. Cache/output paths inside any frozen
fixture subtree are refused. The copied `observed` results record the actual
run; reproduction writes separate scratch output.

Every offered supporting and assistant alias retains all existing closed
reference alternatives. Unknown/cross-category aliases, dangling coordinates,
and nonempty context with no assistant field are rejected. A date selector still
requires the adapter's contracted clock checks, and source membership alone does
not prove newly owned support, meaning, range order, Unicode or authority.

A separate runtime regression demonstrates why empty arrays use `const:[]`:
`{"maxItems":0,"type":"array"}` without `items` incorrectly generates arbitrary
arrays in this pinned converter. The tested constant accepts exactly the empty
array. Runtime grammar also fixes property order and literal spelling (including
`[]` for that constant); the adapter continues accepting equivalent JSON syntax.
These generator limitations do not change the reference protocol's meaning.

The largest generated grammar is 6,750 bytes, below the runtime's 32,768-byte
buffer. The largest complete rendered-byte token bound is 7,750 of 8,192,
including output 768 and reserve 64. Both come from the actual sealed requests,
not the smaller static prompt prefix. `run.py` verifies each recorded derived
schema and system hash before invoking the native grammar proof.

These are ASCII generation-grammar checks. Existing public adapter tests retain
Unicode, source/clock eligibility, scope, authority, identity and response
binding boundaries. No semantic quality, model adequacy, output repair or new
human adjudication follows from this offline result.
