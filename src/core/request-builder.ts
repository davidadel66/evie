import { conversationOf, summaryMessage } from "./compaction.ts"
import type { Message, Request, StoredEvent, Tool } from "./types.ts"

// Builds what the model sees from the session's events and the registered
// tools. It is a pure function: the same log always gives the same request,
// which is what keeps a provider's prompt cache working.
//
// Once a conversation has been compacted, the request is the latest summary
// followed by the events after it. The older events are still in the log.
export function buildRequest(
  model: string,
  events: readonly StoredEvent[],
  tools: readonly Tool[],
  maxTokens?: number,
): Request {
  const { compaction, kept } = conversationOf(events)
  const messages: Message[] = compaction ? [{ role: "user", text: summaryMessage(compaction.body) }] : []
  for (const event of kept) {
    if (event.type === "user_message") messages.push({ role: "user", text: event.body.text })
    if (event.type === "assistant_message") {
      const { text, toolCalls } = event.body
      if (toolCalls && toolCalls.length > 0) messages.push({ role: "assistant", text, toolCalls })
      // A reply with no text and no tool call is left out: providers reject
      // an empty message.
      else if (text !== "") messages.push({ role: "assistant", text })
    }
    if (event.type === "tool_result") {
      messages.push({ role: "tool", callId: event.body.callId, text: event.body.text })
    }
  }
  return {
    model,
    ...(maxTokens !== undefined && { maxTokens }),
    messages,
    tools: tools.map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.inputSchema,
    })),
    // Only what the model is told about a tool goes in: not its function.
  }
}
