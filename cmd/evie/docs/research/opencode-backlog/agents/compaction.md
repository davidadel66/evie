# A05 - Compaction agent

Status: candidate within F09, unapproved. Kind: hidden internal agent.

## Purpose

Replace an old model-visible history span with a structured rolling summary
while retaining the complete durable transcript.

## How OpenCode does it

OpenCode registers `compaction` as hidden, tool-free, and normally uses the
model from the triggering user turn unless the profile overrides it. The
runtime:

- estimates the provider-formatted message size;
- keeps a recent tail budget equal to 25 percent of usable context, clamped to
  2,000-15,000 tokens unless configured;
- summarizes the older head with the previous summary available;
- handles oversized split turns;
- records compaction as normal persisted message/part state; and
- can continue the interrupted task after automatic compaction.

Older tool outputs can be cleared before summary generation. OpenCode protects
about 40,000 recent tool-output tokens and only prunes if at least 20,000 tokens
can be recovered.

Sources:

- [`Agent` registry, `compaction`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/agent.ts#L219-L233)
- [`session/compaction.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/compaction.ts)
- [`compaction.txt`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/agent/prompt/compaction.txt)

## Proposed EVIE adaptation

- Implement only after F08 durable events and provider usage/context limits.
- Give the profile no tools and no ability to alter semantic memory.
- Compact only the request projection, never canonical event history.
- Preserve complete recent turns and tool-call/result pairing.
- Store source event boundaries, model, prompt version, and summary text.
- Add manual compaction before automatic triggering.
- Treat a failed summary as a visible context-management failure; never discard
  history because summarization failed.

## Acceptance evidence

- Repeated compaction retains prior summary information.
- No tool result becomes orphaned.
- A deliberately oversized single turn follows a defined split-turn path.
- Restarting after compaction rebuilds the same model-visible projection.
- The compaction profile cannot call any tool.

## Open decisions

1. Use the active model or a configured cheaper model?
2. What reserve and retained-tail budgets fit EVIE's current model?
3. Should automatic continuation occur after compaction, or require one explicit
   user-visible event first?
