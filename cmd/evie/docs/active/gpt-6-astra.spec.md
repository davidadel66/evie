# GPT-6 Astra through OpenRouter

Status: implemented and verified; migration approved by David on 2026-09-10.
Decisions: [gpt-6-astra.decisions.md](gpt-6-astra.decisions.md).
Origin and rollout evidence: [migration plan](../../../../docs/gpt-6-astra-migration-plan.md).

## Outcome

Evie conversations and compaction use `openai/gpt-6-astra` through OpenRouter's
stateless Responses endpoint. Existing OpenRouter credentials, durable history,
tool authorization, cancellation, memory scope, and usage reporting continue to
work. Explicit `EVIE_MODEL=moonshotai/kimi-k3` remains a manual rollback using
the existing Chat Completions transport.

## Acceptance

- Astra requests use `/api/v1/responses`, `store:false`, flat function schemas
  with `strict:false`, `max_output_tokens`, and supported reasoning effort.
  Sampling parameters and `previous_response_id` are absent. Unset/`on`
  reasoning resolves to `low`; `low`, `medium`, `high`, `xhigh`, and `max` are
  accepted. Other values, including `off`, fail before a turn starts.
- Both protocols preserve optional nested tool arguments. A completed Responses
  output maps to public assistant text and function calls paired by `call_id`.
  Streamed argument fragments and announced identities must agree with the
  authoritative completed output. Malformed, failed, incomplete, cancelled,
  oversized, or prematurely ended responses cannot execute tools. The first
  valid completion is terminal; trailing transport events are not processed.
- Public text streams to the existing frontends. Conversation requests opt into
  `reasoning.summary:concise`; compaction omits this unused display option.
  Conversational dispatch starts the thinking indicator even without a summary.
  Only public reasoning-summary deltas may enter its text display; private
  reasoning and encrypted continuation are transport state. Response bodies and
  error messages remain bounded and do not expose raw provider error bodies.
- The completed thinking row shows a single-line public-summary preview followed
  by `- <duration>`, with long previews visually truncated so the duration stays
  visible. Expansion shows the complete text. Without public text, the existing
  `Thought for <duration>` label remains. Streaming retains `Thinking…` and the
  live summary body. The row expands only when public text is available. Its
  duration is browser-observed wait from dispatch to the response, including
  provider and network time, not measured internal reasoning time. Empty activity
  closes on completion or failure without claiming that unshown text was
  discarded. Existing callback lifetime, post-content suppression, and
  non-persistence rules still apply.
- Context admission, snapshots, and HTTP dispatch use the same immutable encoded
  Responses bytes, including live continuation. Component diagnostics reflect
  the selected protocol. Historical receipts remain readable. Astra discovers
  eligible route limits, including prompt and output caps; it never inherits
  Kimi's built-in fallback when discovery fails. Working context stays 262,144
  by default, with the existing reserve/margin settings and explicit overrides.
- Opaque output items exist only within the active tool loop. New turns and
  restarts reconstruct public history from SQLite, retaining message phases and
  their ordering relative to function calls. Old assistant payloads and accepted
  compaction chains remain readable. Complete tool groups remain atomic;
  unfinished effects are never automatically replayed.
- Assistant evidence still commits before tool preparation/execution. Tool
  intent, approval, completion, lease fencing, callback lifetime, and cancellation
  behavior retain their existing contracts. No new event is inserted between a
  context snapshot and its attributed assistant usage.
- Responses input/output/detail counters map to the existing nullable usage
  fields. Missing and zero remain different; malformed counters remain unknown.
  The last non-null streamed usage observation replaces earlier observations.
  Compaction/extraction/failure coverage exclusions remain visible and unchanged.
- Astra manual and automatic compaction use low reasoning without temperature,
  no tools, a 4,096-token output reserve, no retry, and a two-minute maximum.
  Existing whole-turn cuts, pressure thresholds, summary validation, generations,
  and failure behavior apply. Incomplete output never activates a summary.

## Dependencies and risks

OpenRouter must expose the verified Astra alias and compatible Responses routes.
The dated canonical slug is metadata provenance, not another supported configured
alias. Unsupported custom model identifiers continue through the legacy path.
Transport storage is disabled; this setting does not override OpenRouter or
upstream provider retention policies. Persisting opaque state would require a
separate encryption/key-management decision and is outside this change.

Live synthetic probes establish interoperability, not broad quality, latency,
or cost guarantees. Compaction shares its output budget with reasoning, so a
budget-exhausted summary must fail safely. Provider changes can affect discovery
or model behavior; manual Kimi rollback remains available.

## Verification

Use local HTTP fixtures for request bytes, fragmented tool calls, stream errors,
usage, cancellation, output bounds, and route discovery. Use agent-loop tests for
continuation admission and authorization/recovery boundaries. Temporary SQLite
tests cover mixed old/new history, phase/order preservation, restart, rollback,
and accepted prior summaries. Run focused package tests, race tests for agent/web,
then `./scripts/verify-change.sh`. Record exact results and live probe limits in
the migration plan. No production data or real side effects are used for probes.

## Non-goals

No new SDK/dependency, provider framework, database migration, hosted tools,
async tools, steering, subagent orchestration, UI redesign, remote semantic
extraction/embedding, automatic model fallback, or caching policy change.
The existing `EVIE_REMOTE_MEMORY=on` opt-in is independent of this migration.
