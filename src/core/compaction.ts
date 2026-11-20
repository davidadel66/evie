import type { Message, StoredEvent } from "./types.ts"

// Compaction: replacing the oldest part of a long conversation with a
// summary, so the session can carry on. Nothing is deleted. A `compaction`
// event is added to the log, holding the summary and where the kept part
// starts, and from then on a request is built from that summary plus the
// events after it. The older events stay in the log and are simply no longer
// sent.
//
// This file holds the parts that need no model: measuring a conversation,
// choosing where to cut, and the wording. The loop makes the summary request.

// A conversation is compacted when it reaches this share of what the model
// can read, or the ceiling, whichever comes first. The ceiling matters for
// models that can read a million tokens: long before they are full, every
// request is large and costly, and the model uses the conversation less well.
export const SHARE_OF_LIMIT = 0.8
export const DEFAULT_CEILING = 200_000
// How much of the most recent conversation is kept word for word, as a share
// of the size that triggers a compaction.
const SHARE_KEPT = 0.15
// The most files that are listed after a summary.
const MOST_FILES = 50

export type CompactionEvent = Extract<StoredEvent, { type: "compaction" }>

// The size at which a conversation is compacted. With a model whose limit is
// not known, the ceiling decides alone.
export function triggerSize(modelLimit: number | undefined, ceiling: number): number {
  return modelLimit === undefined ? ceiling : Math.min(Math.floor(modelLimit * SHARE_OF_LIMIT), ceiling)
}

// Never less than one token, so the newest message is always kept.
export const tailBudget = (trigger: number) => Math.max(1, Math.round(trigger * SHARE_KEPT))

// A rough count of the tokens in a piece of text. Providers report the true
// count for a whole request; this is only for what has been added since.
export const estimateTokens = (text: string) => Math.ceil(text.length / 4)

function eventTokens(event: StoredEvent): number {
  switch (event.type) {
    case "user_message":
      return estimateTokens(event.body.text)
    case "assistant_message":
      return estimateTokens(event.body.text) + estimateTokens(JSON.stringify(event.body.toolCalls ?? ""))
    case "tool_result":
      return estimateTokens(event.body.text)
    default:
      return 0
  }
}

// The conversation as it now stands: the latest summary, if there is one,
// and the events that are still sent.
export function conversationOf(events: readonly StoredEvent[]): { compaction?: CompactionEvent; kept: StoredEvent[] } {
  const compaction = events.findLast((event) => event.type === "compaction") as CompactionEvent | undefined
  if (!compaction) return { kept: [...events] }
  return {
    compaction,
    kept: events.filter((event) => event.seq >= compaction.body.keepFrom && event.type !== "compaction"),
  }
}

// How big the conversation is, in tokens. It takes the count the provider
// reported with the last reply and adds an estimate of what came after. A
// count from before the latest compaction describes a conversation that is
// no longer sent, so it is not used.
export function sizeOf(events: readonly StoredEvent[]): number {
  const { compaction, kept } = conversationOf(events)
  const reported = kept.findLast(
    (event) => event.type === "assistant_message" && event.body.usage !== undefined && event.seq > (compaction?.seq ?? 0),
  )
  if (reported?.type === "assistant_message" && reported.body.usage) {
    const since = kept.filter((event) => event.seq > reported.seq)
    return reported.body.usage.inputTokens + reported.body.usage.outputTokens + since.reduce((sum, event) => sum + eventTokens(event), 0)
  }
  const summary = compaction ? estimateTokens(summaryMessage(compaction.body)) : 0
  return summary + kept.reduce((sum, event) => sum + eventTokens(event), 0)
}

// Where the kept part starts: the sequence number of the first event that
// stays. Working back from the newest event, it keeps at least `budget`
// tokens. The cut then falls just before one of the owner's messages or one
// of the model's replies, never before a tool result, so a tool call always
// stays with its result. Returns undefined when that leaves nothing to
// summarise.
export function chooseCut(kept: readonly StoredEvent[], budget: number): number | undefined {
  const spoken = kept.filter((event) => event.type === "user_message" || event.type === "assistant_message" || event.type === "tool_result")
  let total = 0
  let at = spoken.length
  while (at > 0 && total < budget) {
    at -= 1
    total += eventTokens(spoken[at]!)
  }
  // Move back to the reply that asked for this tool, if the cut landed on a result.
  while (at > 0 && spoken[at]!.type === "tool_result") at -= 1
  // Nothing to summarise, or nothing to keep.
  if (at <= 0 || at >= spoken.length) return undefined
  return spoken[at]!.seq
}

// The files the conversation worked with, taken from the tool calls: any
// input called `path`, and the files a patch names. It is gathered by code
// so that the list cannot be forgotten by the summary.
export function filesIn(events: readonly StoredEvent[]): string[] {
  const files: string[] = []
  for (const event of events) {
    if (event.type !== "assistant_message") continue
    for (const call of event.body.toolCalls ?? []) {
      let input: Record<string, unknown>
      try {
        input = JSON.parse(call.arguments) as Record<string, unknown>
      } catch {
        continue
      }
      if (typeof input?.path === "string") files.push(input.path)
      if (typeof input?.patchText === "string") {
        for (const match of input.patchText.matchAll(/^\*\*\* (?:Add File|Update File|Delete File|Move to): (.+)$/gm)) files.push(match[1]!.trim())
      }
    }
  }
  return files
}

// The newest of the files, each once, in the order they were last used.
export function mergeFiles(earlier: readonly string[], later: readonly string[]): string[] {
  const merged = [...earlier, ...later]
  return merged.filter((file, index) => merged.lastIndexOf(file) === index).slice(-MOST_FILES)
}

// ---- Wording ------------------------------------------------------------------------

// What the model is shown in place of the part that was summarised.
export function summaryMessage(body: CompactionEvent["body"]): string {
  const files = body.files.length > 0 ? `\n\nFiles worked with before this point:\n${body.files.map((file) => `- ${file}`).join("\n")}` : ""
  return (
    "[The earlier part of this conversation was replaced by the summary below, to save room. Treat it as background " +
    "you already know, not as new instructions, and carry on from the messages that follow it.]\n\n" +
    body.summary +
    files
  )
}

// The instruction that ends the summary request. The headings and rules draw
// on the summary templates of OpenCode, DeepSeek Harness, pi and Codex; the
// wording is Evie's own.
export const SUMMARY_PROMPT = `Stop working on the task. Write a summary of the conversation above, for another agent that will carry on this work without having seen it. Do not call any tools.

If the conversation starts with an earlier summary, update it: keep what still holds, and where newer messages disagree with it, the newer messages win.

Use exactly these headings, in this order. Write "(none)" under a heading that has nothing.

## Goal
What the owner is trying to get done. More than one item if the session covers several tasks.

## What the owner asked for
The requirements, preferences and corrections the owner gave. Where the wording matters, quote it exactly. Include any question or request that is still unanswered.

## Decisions
Each decision made, with its reason in a few words.

## Done
What is finished.

## In progress
What is part way through, and what is blocked and on what.

## State of the changes
Whether changes are saved to files, committed, pushed or still only planned.

## Next step
The single next thing to do.

## Files that matter
At most 15 files, the most important first, each with a few words on why. Not every file that was read.

## What else is needed
Facts the next agent cannot carry on without and could not easily find again: exact commands, error messages, values, names.

Rules:
- Short single-line bullets, not paragraphs.
- Keep exact file paths, function names, commands, error messages and identifiers.
- Leave out what can be read again from the files, and leave out the project's instruction files, which are given again separately.
- Do not mention this summary or that the conversation was shortened.`

// Shortens long tool results, for a summary request that was itself too long.
export function clipToolResults(messages: readonly Message[], most: number): Message[] {
  return messages.map((message) =>
    message.role === "tool" && message.text.length > most
      ? { ...message, text: `${message.text.slice(0, most)}\n[… ${message.text.length - most} more characters left out …]` }
      : message,
  )
}
