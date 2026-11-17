import type { Plugin, Request } from "../../src/index.ts"

// A stand-in for the model. It replays a script of replies, one per request,
// and records every request it was sent, so a test can check what the model
// would have seen.
// `hang` never replies: it waits until the turn's cancel signal fires.
// `error` is a request the provider reports as failed, after any text;
// `fail` is a provider that breaks by throwing.
export type ScriptedReply = { text: string[]; error?: string } | { fail: string } | { hang: true }

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
          if (reply.error !== undefined) yield { type: "finish", reason: "error", error: reply.error, usage }
          else yield { type: "finish", reason: "stop", usage }
        },
      })
    },
  }
  return { plugin, requests }
}
