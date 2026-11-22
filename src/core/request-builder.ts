import { shownName } from "./attachments.ts"
import { conversationOf, summaryMessage } from "./compaction.ts"
import type { Attachment, EventBodies, Message, Request, StoredEvent, Tool } from "./types.ts"

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
// How a note from Evie opens. Only the core writes one. A model that has
// put work in the background is inclined to write the note it is waiting
// for, and then to make up the result. So a reply is ended the moment the
// model starts to write one: the request names the opening as text to stop
// at, and the loop cuts off anything that got through.
//
// The note on a cut-short reply is not stopped at. A model has no reason to
// forge one, and stopping there cut a reply off when the model was only
// explaining how cut-short replies work.
export const NOTE_OPENING = "[Note from Evie:"
export const ONLY_THE_CORE_WRITES = [NOTE_OPENING]

// `shown` are the attached images the model is shown, by their path, as
// their bytes in base64. Every other attachment is named by its path, and
// any other image said not to be shown, unless it is not known whether the
// model takes images.
export type ShownImages = { images: ReadonlyMap<string, string>; unknown?: boolean }

export function buildRequest(
  model: string,
  events: readonly StoredEvent[],
  tools: readonly Tool[],
  maxTokens?: number,
  shown: ShownImages = { images: new Map() },
): Request {
  const { compaction, kept } = conversationOf(events)
  const messages: Message[] = compaction ? [{ role: "user", text: summaryMessage(compaction.body) }] : []
  for (const [index, event] of kept.entries()) {
    if (event.type === "user_message") messages.push(ownerMessage(event.body, shown))
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

// The owner's message, with a line for each file attached to it after its
// text, saying where it is kept: an image the model is shown follows the
// text, and the model reads any other file with its tools.
function ownerMessage(body: EventBodies["user_message"], { images, unknown }: ShownImages): Message {
  const attached = body.attachments ?? []
  if (attached.length === 0) return { role: "user", text: body.text }
  const shown = attached.filter((each) => images.has(each.path))
  // The name and the path are quoted, so that nothing in them can start a
  // line of its own; the name, which a client chose, loses what could close
  // the line it is in, or start another, too.
  const quoted = (text: string) => JSON.stringify(text)
  const lines = attached.map(
    (each) => `[Attached: ${quoted(shownName(each.name))} (${each.type}, ${sizeIn(each.size)}), kept at ${quoted(each.path)}. ${howShown(each, images, unknown === true)}]`,
  )
  return {
    role: "user",
    text: [...(body.text === "" ? [] : [body.text, ""]), ...lines].join("\n"),
    ...(shown.length > 0 && { images: shown.map((each) => ({ type: each.type, data: images.get(each.path)! })) }),
  }
}

function howShown(attachment: Attachment, images: ReadonlyMap<string, string>, unknown: boolean): string {
  if (images.has(attachment.path)) return "It is shown below."
  if (attachment.type.startsWith("image/") && !unknown) return "It cannot be shown to you as an image here, so only its path is given."
  return "Read it with your tools to see what it holds."
}

// A size in bytes, in short: 4 bytes, 120 KB, 2.1 MB.
function sizeIn(bytes: number): string {
  if (bytes < 1024) return `${bytes} byte${bytes === 1 ? "" : "s"}`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1).replace(/\.0$/, "")} MB`
}

// What cut a reply short is read from what follows it in the log: a turn
// that ended as cancelled, or a message from the owner in the same turn.
function cutShortNote(events: readonly StoredEvent[], at: number): string {
  const next = events.slice(at + 1).find((event) => event.type === "user_message" || event.type === "turn_end")
  if (next?.type === "turn_end" && next.body.reason === "cancelled") return "[This reply was cut short: the owner cancelled it before it was finished.]"
  if (next?.type === "user_message") return "[This reply was cut short: the owner sent a new message before it was finished.]"
  return "[This reply was cut short before it was finished.]"
}
