import type { Message, Request, StoredEvent, Tool } from "./types.ts"

// Builds what the model sees from the session's events and the registered
// tools. It is a pure function: the same log always gives the same request,
// which is what keeps a provider's prompt cache working.
export function buildRequest(model: string, events: readonly StoredEvent[], tools: readonly Tool[]): Request {
  const messages: Message[] = []
  for (const event of events) {
    if (event.type === "user_message") messages.push({ role: "user", text: event.body.text })
    // A reply with no text is left out: providers reject an empty message.
    if (event.type === "assistant_message" && event.body.text !== "") {
      messages.push({ role: "assistant", text: event.body.text })
    }
  }
  return {
    model,
    messages,
    tools: tools.map((tool) => ({
      name: tool.name,
      description: tool.description,
      inputSchema: tool.inputSchema,
    })),
  }
}
