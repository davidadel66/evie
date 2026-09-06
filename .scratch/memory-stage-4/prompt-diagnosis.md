# Standalone extractor prompt diagnosis

Read-only investigation for #135, 2026-09-05 UTC. No inference, installation,
source/gold/prompt modification, approval, or commit was performed. The only
written artifact is this note. This is development analysis, not output
adjudication or final-holdout evidence.

## Finding

One recorded prompt-contract revision is justified before attributing the
observed failures principally to model capability. The comparison has a concrete
information gap: neither arm puts the output schema in the model's prompt, and
the JSON arm receives no closed-schema specification at all. The existing text
does not fully define subject versus scope, text versus entity objects, resolved
identity, or ordinary facts versus decisions. These omissions plausibly explain
some repeated typed-meaning errors. They do not explain away every policy error
and do not establish that Mistral will become adequate.

The runner constructs `system=prompt.txt`, `prompt=window.input`, and
`format=output.schema.json` or `format="json"`
([request construction](/Users/davidboktor/code/evie/scripts/memory-extractor-spike/main.go:287)).
The prompt says a schema is supplied but omits the predicate/object-kind enums
and exact closed reference shape. In the pinned runtime, prompt construction
uses the system and user messages, while `Format` travels separately;
the schema becomes a decoding grammar rather than prompt text.
([Ollama 0.6.3 prompt construction](https://github.com/ollama/ollama/blob/v0.6.3/server/routes.go#L223-L287),
[format-to-grammar conversion](https://github.com/ollama/ollama/blob/v0.6.3/llm/server.go#L625-L661))

Therefore the existing runs demonstrate the behavior of these two exact
configurations. They do not fairly isolate whether JSON mode can follow the same
fully specified contract as schema mode. Supplying identical contract text in
both arms would fix that comparison defect; grammar enforcement should remain
the intentional arm difference.

## Concrete observations

References below are `runs` entries selected by case ID and repetition 1 in the
immutable [schema report](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/development-schema.json)
and [JSON report](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/reports/development-json.json).
Sources and reviewed expected meanings are in the separate
[development inputs](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/development.json)
and [development gold](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/development.gold.json).
The immutable gold file's old status strings are superseded by the exact-hash
[human annotation record](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/annotation-record.json).

| Case | Raw observation | What it establishes |
| --- | --- | --- |
| N01-a | For `I prefer tea.`, schema output chooses subject equal to the project scope UUID, `object_kind=entity`, `identity=unresolved`, and `kind=decision`. JSON chooses `object_kind=EntityID`, `identity=owner_statement`, and adds `session_id` to the reference. | The JSON values/extra reference field objectively violate the closed schema that arm never saw. Both arms show the same subject/scope and fact/decision confusion. The schema arm's field values are syntactically legal; judging its proposed meaning requires the approved meaning contract, not a JSON validator. |
| N01-b | Both arms repeat the old tea proposition. Schema also emits `object="not eating pear at lunch"`, `polarity=denied`, `effect=correct` for the new incidental meal. | Old-only support objectively violates new-evidence ownership. The pear output conflicts with the reviewed remember-nothing policy; its exact meaning/classification is not automatically human-adjudicated by this note. |
| N02-a | Schema emits Maya residence in Paris from the unendorsed report; another proposal uses a session UUID as `event_id`. JSON emits residence/move propositions and copies source metadata into references. | Unknown event IDs and extra reference fields are objective errors. Treating an unendorsed report as a residence assertion conflicts with the binding policy, which the existing prompt already states. Better output-field definitions alone may not fix that behavior. |
| N03-b | Both arms turn assent to a comparative preference into separate tea-affirmed and coffee-denied outputs, with project scope as subject. | The relative preference qualification appears lost. The reviewed gold is a single comparative meaning; this is a meaning concern, not a syntax failure. Do not silently score the novel pair as equivalent. |
| N04-b | Schema cites the new 23-byte pronoun event for an old cousin relation; its second reference is `24:51` against that same 23-byte event. | The second span is objectively out of bounds. The first citation is in bounds but does not itself establish entailment. Source hashing cannot fix a mistaken attachment of old meaning to new text. |
| N08-b | Schema generates several separate clock-date propositions and cuts off at 768 output tokens. JSON emits a date-use decision with the full clock timestamp rather than the meaningful habit change. | Truncation is an objective failed run. The schema output's already visible repetition concerns selection/representation, so merely enlarging output capacity is not the best first experiment. Partial JSON must remain rejected. |

Both repetitions produced byte-identical raw output for all ten cases within
each arm. Report statuses were schema: 18 `ok`, 2 `truncated_output`; JSON:
10 `ok`, 8 `schema_error`, 2 `truncated_output`. An `ok` status is not a quality
verdict. These counts are descriptive only and do not replace the corrected
offline scorer's denominators or pending human adjudications.

Immutable report SHA-256 values:

- Schema: `680ee4e97153f0ab2e2003894b84e46cd3993c507033a2c03e6c5954b0e8cadf`.
- JSON: `b1a65df7d54a4450ecf929dda8a30251a9ce58fe7e08ebb9cc8c27e2529af9af`.

## One proposed experiment

Freeze a new prompt revision that appends the following field-contract text,
then the exact existing schema serialized compactly as JSON, to the existing
policy prompt. Give identical resulting system text to both format arms. Keep
the schema, source windows, gold labels, and deterministic validator unchanged.
This adds no case-specific examples, expected answers, or evaluator notes.

```text
Output field contract:
subject identifies who or what the assertion is about. Use the literal owner for first-person personal assertions and the literal project for assertions about the project. scope is only the visibility boundary: copy input.scope there, never use it as a substitute for a personal subject. An accepted Entity ID requires explicit accepted identity context; otherwise a named person is new:Name.
object_kind=text for ordinary descriptions, names of places or organizations, preferences, and constraints expressed as text. Use entity only when object is an Entity reference, either an explicitly supported accepted ID or new:Name. Use date only when the object itself is a calendar date. A temporal qualifier normally belongs in temporal.
identity=resolved for owner/project and explicitly supported accepted identity references; identity=unresolved when the candidate introduces an unresolved Entity reference. This is not the source authority label.
kind=fact for standing states or preferences, world_change for a stated change, decision for an adopted choice, and consideration for an option still under consideration. Preserve qualifications in object and temporal; do not turn a comparison into an absolute denial.
Each sources/context item contains only event_id,start,end. Copy event_id and whole supplied start/end values when citing a whole field. Do not copy session_id, text, scope, authority, or hashes into a reference. Respect the existing new-evidence and context rules.
The exact output schema follows. All its required fields, enum values, limits, and prohibition of additional properties apply:
```

Recommended bounded run: the same ten development windows, one request per
window per arm (20 requests), temperature 0, seed 17, 768 output tokens, 60-second
timeout, one inference request at a time. Repeated seeds added no observed
diversity in the prior pass. Use context 8192 only after the bound below is
verified for every exact rendered prompt. This context change must be recorded;
the run tests a corrected configuration, not a clean numerical estimate of the
prompt's isolated causal effect.

Before dispatch, freeze the new prompt/hash, format settings, code identity,
input-budget proof, and reporting plan. Evaluate the objective schema/reference
failures separately from exact matches to already reviewed meanings and new
meanings awaiting adjudication. Preserve no-memory false proposals and output
truncations. A favorable smoke result warrants broader development/pilot
coverage; it does not itself select a production model or close #135.

## Predispatch token bound for this pinned tokenizer

The pinned GGUF was read directly through its metadata section (ending at byte
740404), without inference. It declares `tokenizer.ggml.model=llama`,
`pre=default`, BOS enabled and EOS disabled. Its normalized-space character
`▁` is NORMAL token 29473. Every unknown/control token spelling has at least
three UTF-8 bytes. These facts need to be checked in the runner's pinned
artifact evidence before relying on the following model-specific bound.

The pinned SPM path replaces each ASCII space with `▁`, optionally prefixes a
space to raw fragments, merges symbols, and falls back to byte tokens. Specials
partition the input and each produce one token. BOS adds one token. No other
normalization is applied by this SPM path.
([SPM implementation and fallback](https://github.com/ollama/ollama/blob/v0.6.3/llama/llama.cpp/src/llama-vocab.cpp#L105-L181),
[space escaping and fragment handling](https://github.com/ollama/ollama/blob/v0.6.3/llama/llama.cpp/src/llama-vocab.cpp#L2119-L2203))

**Derived bound:** for the complete, exact template-rendered prompt of B UTF-8
bytes, token count is at most `B+2`. Existing spaces each have a one-token
representation. Ordinary fallback needs at most one token per original byte;
merges reduce the count. Each recognized special replaces at least three bytes
with one token, covering the possible additional prefix-space token after it.
One initial prefix-space and one BOS remain. This proof is specific to the
verified tokenizer/vocabulary and rendered text, not a general tokens-per-byte
heuristic.

The compact existing schema is 1507 bytes, existing policy prompt 2447 bytes,
and largest compact UTF-8 development input 1263 bytes. The draft contract above
is 1632 bytes; joining policy, contract, and schema with one newline between
produces a 5588-byte system prompt. With a hypothetical 64-byte template
allowance, the total bound including input, BOS/prefix, 768 output tokens and
64 reserve is 7749, below 8192. The actual template allowance still must be
verified; this arithmetic is not proof of that rendering. The final prompt
must be measured together with separators and the actual runtime template;
do not reuse an old measured token count for it. Dispatch only if the pinned
template/model/runtime and all inputs match the recorded proof, and
`B + 2 + 768 + 64 <= 8192`. The 64 tokens are an explicit extra reserve, not a
replacement for accounting for the template. If the actual rendering contains
unaccounted preprocessing or additional messages, reject until an exact
tokenizer/rendering path or a stronger proven bound is available. This note
does not authorize weakening the existing context-fit check.
