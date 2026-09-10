# GPT-6 Astra decisions

- **2026-09-10 — retain OpenRouter and use its stateless Responses endpoint.**
  David selected OpenRouter and approved implementing the migration after the
  API comparison. The verified configured alias is `openai/gpt-6-astra`;
  upstream tool use requires Responses. The existing Go client owns the adapter,
  with the established Chat request/response structs as its agent-facing seam.
  No SDK or general provider framework is added. Only this verified alias selects
  Responses; other configured models retain their current transport.

- **2026-09-10 — low reasoning is the Astra baseline, including compaction.**
  Unset/`on` conversation effort means `low`; explicit `low`, `medium`, `high`,
  `xhigh`, and `max` are supported. Reject `off` and unknown settings before a
  turn. Astra compaction uses `low` and omits temperature. This supersedes the
  no-reasoning/zero-temperature clauses of the August 30 memory decisions only
  for Astra. Same model, no tools/retry, 4,096 output tokens, two-minute timeout,
  validation, lease fencing, and generation rules remain binding. Legacy model
  reasoning and compaction settings retain their existing behavior.

- **2026-09-10 — keep opaque state per turn and preserve public output phases.**
  Live assistant output items are replayed in memory until the tool loop ends.
  Request encrypted reasoning with `include:["reasoning.encrypted_content"]`
  and disable response storage. Durable assistant payloads gain optional
  `text_parts` with public `text`, optional `phase` (`commentary`/`final_answer`),
  and `after_tool_calls` offsets. Ordered concatenation must equal event Content;
  offsets are nondecreasing and within the assistant's tool-call list. Empty
  phase retains legacy behavior. Existing event/payload versions remain valid
  because the extension is optional and shared by the strict compiler decoder.
  Provider output IDs, raw items, and encrypted reasoning never enter SQLite.
  Request-local synthetic item IDs allow replay after restart; original
  `call_id`s preserve tool pairing. This extends public serialization only and
  does not authorize opaque persistence under the August 23 memory decision.

- **2026-09-10 — freeze the actual wire request before admission.**
  Responses encoding produces one immutable body reused for size limits, SHA-256,
  the pre-request snapshot, and dispatch. Continuation is part of those bytes.
  New receipts use `context-composer-v2` and `canonical-provider-json-bytes-v2`;
  snapshot schema remains one. Kimi's fallback model identity remains separate
  from the application default. Route discovery accepts verified advertised and
  canonical Astra IDs and takes the minimum compatible route window/prompt cap.
  Compactor admission subtracts the larger of the conversation and compaction
  reserves so a smaller compaction output cap cannot reclaim prompt-limited
  space. This is conservative even for explicit context overrides.
  `provider.require_parameters:true` prevents silently dropping required settings.

- **2026-09-10 — retain loose tool schemas and nullable usage semantics.**
  Responses functions explicitly set `strict:false` so optional nested arguments
  keep their established meaning. The harness retains execution validation and
  authority. Normalize Responses counter names without merging duplicate aliases
  or adding cache/reasoning subsets to totals. Non-null usage replaces rather
  than merges prior stream observations; null does not erase them. No billing
  inference or new runtime-usage coverage is introduced.

- **2026-09-10 — restore thinking activity and opt into public summaries.**
  David reported the missing thinking row and asked to see reasoning there.
  Astra conversations request `reasoning.summary:auto`; the compactor does not
  request a display summary. Two synthetic OpenRouter probes (low and medium)
  accepted the option but returned empty public summaries despite reasoning
  token usage, so text cannot gate activity. An actual-client probe announced
  the reasoning item 5.023 seconds after dispatch, only 30 ms before text.
  Therefore conversational dispatch invokes the existing
  `OnReasoning("")`/`Reasoning("")` seam to begin timing the visible wait;
  public summary deltas append through the same seam. The thinking row
  retains its elapsed-time label and expands only with public text. Its tooltip
  describes browser-observed provider/network wait, not internal compute time. Empty
  starts close normally on errors without a discarded-text warning. This
  extends the historical reasoning display contract while preserving the
  monotonic presentation and callback-lifetime rules. No private chain or
  encrypted content is displayed or persisted.

- **2026-09-10 — use the public summary as the completed row label.**
  David requested `<reasoning summary> - 4s`. The completed row shows the
  whitespace-normalized public summary followed by elapsed duration. Long text
  uses a single-line visual ellipsis while the duration remains visible; the
  full provider text stays available on expansion. Empty or whitespace-only
  summaries retain the timer fallback. Streaming presentation and the existing
  expansion behavior stay unchanged. This is a display change only, with no
  additional model calls or generated replacement summaries.

- **2026-09-10 — request concise summaries after verifying actual delivery.**
  The running application still showed the timer fallback with `summary:auto`.
  OpenRouter accepted those requests and reported effective `detailed`, but
  all three earlier probes returned empty summaries. One isolated request with
  `summary:concise`, low effort, and the same synthetic arithmetic problem
  returned a 32-character public summary before answer text. Use `concise`
  for Astra conversations, superseding the earlier `auto` choice. Compaction
  still omits the display-summary option. This is observed interoperability,
  not a guarantee that every response contains a summary. No extra model call,
  generated fallback, or change to the display/persistence boundary is added.

Sources checked 2026-09-10: [OpenAI reasoning summaries](https://developers.openai.com/api/docs/guides/reasoning),
[OpenAI model migration](https://developers.openai.com/api/docs/guides/latest-model),
[OpenRouter Astra catalog](https://openrouter.ai/openai/gpt-6-astra),
[Responses basic usage](https://openrouter.ai/docs/api_reference/responses/basic-usage),
[Responses tool calling](https://openrouter.ai/docs/api_reference/responses/tool-calling).
