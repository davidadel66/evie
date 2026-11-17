import type { Plugin, Request } from "../../src/index.ts"

// A stand-in for the model. It replays a script of replies, one per request,
// and records every request it was sent, so a test can check what the model
// would have seen.
// `hang` never replies: it waits until the turn's cancel signal fires.
// `error` is a request the provider reports as failed, after any text;
// `fail` is a provider that breaks by throwing.
// `calls` are tool calls the model asks for; `input` is given as an object,
// or as raw text to stand for a model that wrote its input badly.
export type ScriptedCall = { name: string; input: Record<string, unknown> | string }
export type ScriptedReply =
  | { text: string[]; error?: string; calls?: ScriptedCall[] }
  | { fail: string }
  | { hang: true }

export function scriptedProvider(script: ScriptedReply[]) {
  const requests: Request[] = []
  const plugin: Plugin = {
    name: "scripted-provider",
    setup(host) {
      host.providers.add({
        name: "scripted",
        async *stream(request, signal) {
          requests.push(structuredClone(request))
          const reply = script[requests.length - 1]
          if (!reply) throw new Error("the script has no reply for this request")
          if ("fail" in reply) throw new Error(reply.fail)
          if ("hang" in reply) {
            await new Promise<void>((resolve) => signal.addEventListener("abort", () => resolve(), { once: true }))
            throw new Error("the request was cancelled")
          }
          const usage = { inputTokens: 0, outputTokens: 0 }
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
          if (reply.error !== undefined) yield { type: "finish", reason: "error", error: reply.error, usage }
          else yield { type: "finish", reason: calls.length > 0 ? "tool_calls" : "stop", usage }
        },
      })
    },
  }
  return { plugin, requests }
}
