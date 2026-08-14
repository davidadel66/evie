# F09 - Context accounting and compaction

Status: candidate, unapproved. Priority: first, after F08.

## Purpose

Keep long development sessions inside the model's context window while making
token/cost pressure visible and retaining complete local history.

## How OpenCode does it

OpenCode records usage and cost per step, computes model-specific usable
context, prunes old tool output, and invokes the hidden `compaction` profile near
the limit. It retains a bounded recent tail, feeds the previous summary into the
next summary, handles oversized split turns, and keeps canonical history in the
database.

See [A05 - Compaction](../agents/compaction.md) for the profile behavior.

Sources:

- [`session/overflow.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/overflow.ts)
- [`session/compaction.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/session/compaction.ts)

## EVIE today

Every inner-loop request resends the entire transcript and every tool schema.
No usage, context limit, reserve, cost, clearing, compaction, or `/context` view
exists. One verbose tool call can permanently consume the session.

## Proposed EVIE adaptation

Implement in this order:

1. Capture real provider usage and model context/output limits.
2. Add an estimated `/context` breakdown by stable prompt, instructions, tools,
   transcript, and tool outputs.
3. Store complete large tool results outside the prompt and clear old result
   bodies while preserving call/result structure.
4. Add manual compaction with legal boundary tests.
5. Trigger automatic compaction only from measured pressure.

The request composer owns the projection:

```text
stable EVIE prompt
profile block
environment/project instructions
memory/reference data
latest compaction summary
retained complete recent turns
```

Compaction never edits the durable event log and never promotes summarized text
into semantic memory automatically.

## Acceptance evidence

- `/context` approximately reconciles with provider input usage and labels
  estimates honestly.
- Tool-result clearing never orphans a tool call.
- A session can cross its nominal context budget through compaction without a
  provider overflow.
- Repeated compactions preserve earlier decisions, pending tasks, changed files,
  and verification state.
- Failed compaction leaves full history intact and produces an actionable error.

## Open decisions

1. Context reserve and compaction trigger for each supported model.
2. Tool-result retention rules for file diffs, test output, and child verdicts.
3. Whether prompt caching is supported by the active OpenRouter route and worth
   explicit breakpoint plumbing.
