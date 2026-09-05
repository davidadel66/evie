# Compact Qwen wire configuration: predispatch input proof

This is a separate experimental transport configuration under ticket #135. The
source corpus, approved gold, cached model, runtime, tokenizer and output cap
are unchanged. No earlier request hash or measured token count is reused.

The byte proof is inherited only from the identical inspected Qwen model/runtime
artifacts in [the original Qwen proof](../qwen/input-budget-proof.md).
Copies of the original runtime manifest, API/template metadata and tokenizer
proof here preserve their exact bytes and pinned identities. The executable
checks those identities plus the independent compact prompt/schema constants;
changing the old or compact configuration does not authorize the other.

The new input is a deterministic presentation of exactly the selected frozen
support/context fields. It is ordered by session sequence and retains separate
root labels, content-free omitted sequence ranges, original roles, authority,
new/overlap/context ownership, original byte coordinates and unchanged text.
The complete alias seal retains canonical source identity, exact projection and
hash, original scope, observed time, event/policy version, window/cutoff and
accepted identity context. Hashes and destination scope remain harness-owned.
No gold, later event content or additional source field enters the request.

The request contains one system string, one compact input JSON string, the
schema decoding constraint, and the same pinned generate options. The full
installed Go-template rendering must match exactly:

```text
<|im_start|>system\n + system + <|im_end|>\n +
<|im_start|>user\n + compact_input + <|im_end|>\n +
<|im_start|>assistant\n
```

The rendering has 80 fixed UTF-8 bytes. The unchanged independent BPE proof
bounds input tokens by complete rendered UTF-8 bytes plus at most two added
tokens. With the compact system prompt, every selected request must satisfy:

`5561 + compact_input_utf8_bytes + 80 + 2 + 768 + 64 <= 8192`.

Thus the input JSON can occupy at most 1717 bytes. The ten approved development
windows consume 282–696 bytes; the largest is N08-b. Its complete conservative
bound is 7171 tokens including the full output cap and reserve, leaving 1021
of the configured 8192. These are deterministic predispatch bounds, not observed
token counts. All selected requests pass before the first network inference.
The 32 KiB serialized-request cap remains separate from this context proof.

`-preflight-only` writes the request strings, hashes and source seals without
inference; it uses the same complete-batch preflight as the normal command.
Tests also exercise ordinary inference mode with an oversized later window and
an unknown case to establish that the first request cannot be dispatched early.
Huge sequence gaps are represented directly from the bounded selected source
records rather than iterating every sequence number. Unknown prompt, schema,
runtime/tokenizer proof or configuration fails closed.

This proof permits only schema mode, the pinned Qwen model, temperature zero,
seed 17, context 8192, output cap 768, one traversal and stop-on-failure. The
measured run must use a 60-second timeout, one owned loopback server with proxies
disabled and one inference request at a time. Actual release remains unknown
after a disconnect/timeout until a request-specific completion or verified owned
server restart. No repair request, larger cap, model download or runtime upgrade
is authorized by this transport experiment.
