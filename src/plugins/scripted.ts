import type { Plugin, Provider, Request } from "../core/types.ts"

// A stand-in for the model. It replays a script of replies, one per request,
// and records every request it was sent, so a test can check what the model
// would have seen. `evie seed` makes its sessions with it, at no cost.
// `error` is a request the provider reports as failed, after any text;
// `fail` is a provider that breaks by throwing.
// `calls` are tool calls the model asks for; `input` is given as an object,
// or as raw text to stand for a model that wrote its input badly.
export type ScriptedCall = { name: string; input: Record<string, unknown> | string }
// `hang` never finishes: after any text, it waits until the turn's cancel
// signal fires. It then breaks by throwing, or with `reports` ends the reply
// with a failure the way a real provider does.
// `usage` is what the provider reports the request and reply came to.
// `tooLong` with `error` is a request refused as more than the model can
// read, and `cutOff` is a reply that hit its length limit.
export type ScriptedReply =
  | { text: string[]; error?: string; tooLong?: true; cutOff?: true; calls?: ScriptedCall[]; usage?: { inputTokens: number; outputTokens: number } }
  | { fail: string }
  | { hang: true; text?: string[]; reports?: true }

// `limit` is what the model can read. `lookUpLimit` stands in for the lookup
// itself, for a test where it fails or never answers. `models` are the
// models it offers, when it is to list some.
export function scriptedProvider(
  script: ScriptedReply[],
  options: { limit?: number; lookUpLimit?: () => Promise<number | undefined>; models?: { id: string; name: string }[] } = {},
) {
  const requests: Request[] = []
  const provider: Provider = {
    name: "scripted",
    // What the model can read, when a test gives it a limit.
    ...(options.limit !== undefined && { contextLimit: async () => options.limit }),
    ...(options.lookUpLimit && { contextLimit: options.lookUpLimit }),
    ...(options.models && { models: async () => structuredClone(options.models!) }),
    async *stream(request, signal) {
      requests.push(structuredClone(request))
      const reply = script[requests.length - 1]
      if (!reply) throw new Error("the script has no reply for this request")
      if ("fail" in reply) throw new Error(reply.fail)
      if ("hang" in reply) {
        for (const text of reply.text ?? []) yield { type: "text", text }
        await new Promise<void>((resolve) => {
          if (signal.aborted) resolve()
          else signal.addEventListener("abort", () => resolve(), { once: true })
        })
        if (!reply.reports) throw new Error("the request was cancelled")
        yield { type: "finish", reason: "error", error: "The request was cancelled." }
        return
      }
      // A reply with no `usage` in the script reports none, as some providers do.
      const usage = reply.usage && { usage: reply.usage }
      for (const text of reply.text) yield { type: "text", text }
      const calls = reply.calls ?? []
      for (const [index, call] of calls.entries()) {
        yield {
          type: "tool_call",
          id: `call_${requests.length}_${index}`,
          name: call.name,
          arguments: typeof call.input === "string" ? call.input : JSON.stringify(call.input),
        }
      }
      if (reply.error !== undefined) {
        yield { type: "finish", reason: "error", error: reply.error, ...usage, ...(reply.tooLong && { tooLong: true as const }) }
      } else if (reply.cutOff) yield { type: "finish", reason: "length", ...usage }
      else yield { type: "finish", reason: calls.length > 0 ? "tool_calls" : "stop", ...usage }
    },
  }
  const plugin: Plugin = { name: "scripted-provider", setup: (host) => host.providers.add(provider) }
  return { plugin, provider, requests }
}
