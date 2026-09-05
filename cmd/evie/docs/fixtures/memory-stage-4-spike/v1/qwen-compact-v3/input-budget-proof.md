# Compact-v3 request-specific schema and input proof

This independently versioned configuration changes only the generator's alias
category membership. `generator-policy.json` pins `compact-category-v1`.
The static base `prompt.txt` is the unchanged natural-language prefix through
“Closed output schema:”; `output.schema.json` is the unchanged compact-v2 closed
reference template. Neither static file alone is the emitted system prompt.

The generator takes the trusted compact seal, whose projections, aliases,
source text, order, original scope/authority, ownership, observed times, window
and policy identities are unchanged. Each supporting `ref` enum contains only
new/overlap support aliases. Context `ref` enums contain only assistant context
aliases. With no assistant field, context is required to be `const:[]`; with no
support, candidates is `const:[]`. There is no empty enum, inferred alias, extra
source selection, gold label or future-content input. Subject/identity fields
are unchanged, including their strict adapter checks.

The emitted schema is canonical Go `encoding/json` output. The exact same bytes
are passed as the decoding format and appended to the static prompt prefix.
The report's top-level prompt/schema hashes identify the base files. Every
prepared/actual request separately retains derived schema and complete system
SHA-256, derivation version, unchanged alias seal and exact serialized request.
Offline scoring loads the pinned base files and budget, regenerates the schema
from the independently reviewed source corpus/window, and compares the complete
request and all derived identities. Rehashing a more permissive schema, prompt
or alias map cannot authorize it.

The unchanged [Qwen BPE and runtime proof](../qwen/input-budget-proof.md) bounds
full rendered UTF-8 bytes by bytes plus at most two tokens, for exactly the
pinned runtime/model/template/tokenizer artifacts. The copied historical
runtime manifests pin those identities; they do not claim a fresh server
preflight or prescribe this configuration's actual prompt. Current file/API
identity must be verified again before live dispatch.

The executable renders the actual runtime Go template and verifies exactly:

```text
<|im_start|>system\n + derived_system + <|im_end|>\n +
<|im_start|>user\n + unchanged_compact_input + <|im_end|>\n +
<|im_start|>assistant\n
```

The fixed template overhead is 80 bytes. Every selected request must satisfy:

`derived_system_bytes + input_bytes + 80 + 2 + 768 + 64 <= 8192`.

For the same ten development windows, the maximum is **7,750**, at N03-b:
6,252 system bytes + 584 input bytes + 80 template bytes + 2 + 768 + 64.
That leaves 442 of the configured context. Other selected windows need at most
7,287. These are conservative complete-input bounds, not observed model token
counts. No blanket input-only byte cap substitutes for actual generated schema
and system size; larger/different requests still fail preflight when over budget.
The separate serialized-request cap remains 32 KiB.

All selected requests are derived and checked before the first HTTP inference.
Public tests establish that a later window fitting the static-prefix budget
but exceeding the complete generated-system budget prevents the earlier
request from being sent. They also cover unknown later cases, immutable seals,
category aliases, empty constants, policy/configuration pinning, response
binding, unchanged raw denominators, and offline rejection of a modified schema
and prompt even after their hashes and request hash are recomputed.

The [actual-runtime grammar proof](grammar-proof/README.md) passes 177 dynamic
and empty-array cases plus 62 inherited closed-reference cases. It uses the
actual predispatch schemas and preserves known runtime property-order and
constant-spelling limitations. No new production dependency is introduced.

This configuration remains Qwen2.5:7b-instruct-q4_K_M, schema mode, temperature
zero, seed 17, context 8192, output 768, exact 60s, one pass and stop-on-failure.
A live experiment requires independently reviewed/frozen source and binary
identities, current runtime/API preflight and parent-task dispatch. Any timeout,
disconnect or non-ok response stops the batch; unknown request release requires
verified owned-server cleanup without a probe generation. No retries, repair
calls, larger caps, subject union, model download or runtime upgrade are added.
