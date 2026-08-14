# F03 - Provider-neutral protocol

Status: candidate, unapproved. Priority: first.

## Purpose

Keep the agent loop, durable history, tools, and profiles independent from
OpenRouter's wire structs while retaining provider-specific continuation data
where it is actually required.

## How OpenCode does it

OpenCode exposes a normalized model/provider layer over many SDKs and translates
provider streams into common events for text, reasoning, tool input/calls,
results, usage, finish reasons, and errors. It also carries provider-specific
request transforms, auth, capability metadata, context limits, pricing, and
variants. That breadth is useful to OpenCode and far too large for EVIE to copy.

Sources:

- [`provider/provider.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/provider/provider.ts)
- [`session/llm.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/llm.ts#L224-L280)

## EVIE today

The consumer-owned `agent.Client` interface is a good test seam, but its method
accepts and returns `openrouter.ChatRequest`, `ChatResponse`, `Message`, and tool
types. Persisting those directly would make OpenRouter's format EVIE's database
contract.

EVIE also drops provider usage and ignores finish reason when deciding that a
response is complete.

## Proposed EVIE adaptation

Define only the neutral concepts the harness owns:

- role/content message projection;
- tool definition, call, and result;
- text/reasoning/tool/usage/error stream events;
- finish reason and refusal;
- input/output/cache/reasoning token usage and cost when reported;
- model capabilities and context/output limits;
- opaque provider continuation payload attached to a durable event when needed.

OpenRouter becomes an adapter from these values to its request/stream format.
The opaque continuation payload is local, redacted, versioned, and excluded from
semantic memory. A turn that cannot safely retain required provider state is
marked non-resumable rather than pretending neutrality.

This is not permission for a speculative all-provider framework. Keep one
adapter until a selected second provider proves which abstractions are real.

## Acceptance evidence

- Agent and history packages do not persist OpenRouter request structs.
- Captured Kimi streams still reconstruct reasoning and fragmented tool calls.
- Usage and finish reason reach the turn lifecycle.
- Unknown provider fields survive only in an explicitly opaque payload.
- A fake client can script every normalized stream event deterministically.

## Open decisions

1. Which parts of reasoning must be retained to resume Kimi safely?
2. Does the neutral message model support multimodal attachments now or add them
   only when EVIE accepts image input?
3. Where is model capability metadata sourced when OpenRouter omits it?
