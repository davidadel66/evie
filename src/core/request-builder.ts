import { conversationOf, summaryMessage } from "./compaction.ts"
import type { Message, Request, StoredEvent, Tool } from "./types.ts"

// Builds what the model sees from the session's events and the registered
// tools. It is a pure function: the same log always gives the same request,
// which is what keeps a provider's prompt cache working.
//
// A reply that was cut short is stored as far as it got, marked interrupted.
// The model is shown it with a note saying so, or it would take the half
// reply for one it chose to end there. The note is only in the request: the
// stored reply is not changed.
//
// Once a conversation has been compacted, the request is the latest summary
// followed by the events after it. The older events are still in the log.
// How a note from Evie opens, and how the note on a cut-short reply does.
// Only the core writes these. A model that has put work in the background
// is inclined to write the note it is waiting for, and then to make up the
// result. So a reply is ended the moment the model starts to write either:
// the request names them as text to stop at, and the loop cuts off anything
// that got through.
export const NOTE_OPENING = "[Note from Evie:"
const CUT_SHORT_OPENING = "[This reply was cut short"
export const ONLY_THE_CORE_WRITES = [NOTE_OPENING, CUT_SHORT_OPENING]

export function buildRequest(
  model: string,
  events: readonly StoredEvent[],
  tools: readonly Tool[],
  maxTokens?: number,
): Request {
  const { compaction, kept } = conversationOf(events)
  const messages: Message[] = compaction ? [{ role: "user", text: summaryMessage(compaction.body) }] : []
  for (const [index, event] of kept.entries()) {
    if (event.type === "user_message") messages.push({ role: "user", text: event.body.text })
    if (event.type === "assistant_message") {
      const { toolCalls } = event.body
      const text = event.body.interrupted ? `${event.body.text}\n\n${cutShortNote(kept, index)}` : event.body.text
      if (toolCalls && toolCalls.length > 0) messages.push({ role: "assistant", text, toolCalls })
      // A reply with no text and no tool call is left out: providers reject
      // an empty message.
      else if (text !== "") messages.push({ role: "assistant", text })
    }
    // A note is Evie's own word to the model, marked so it is not taken for the owner's.
    if (event.type === "note") messages.push({ role: "user", text: `${NOTE_OPENING} ${event.body.text}]` })
    if (event.type === "tool_result") {
      messages.push({ role: "tool", callId: event.body.callId, text: event.body.text })
    }
  }
  return {
    model,
    ...(maxTokens !== undefined && { maxTokens }),
    stop: [...ONLY_THE_CORE_WRITES],
    messages,
    tools: tools.map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.inputSchema,
    })),
    // Only what the model is told about a tool goes in: not its function.
  }
}

// What cut a reply short is read from what follows it in the log: a turn
// that ended as cancelled, or a message from the owner in the same turn.
function cutShortNote(events: readonly StoredEvent[], at: number): string {
  const next = events.slice(at + 1).find((event) => event.type === "user_message" || event.type === "turn_end")
  if (next?.type === "turn_end" && next.body.reason === "cancelled") return "[This reply was cut short: the owner cancelled it before it was finished.]"
  if (next?.type === "user_message") return "[This reply was cut short: the owner sent a new message before it was finished.]"
  return "[This reply was cut short before it was finished.]"
}
