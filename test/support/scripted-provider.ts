import type { Plugin, Request } from "../../src/index.ts"

// A stand-in for the model. It replays a script of replies, one per request,
// and records every request it was sent, so a test can check what the model
// would have seen.
export type ScriptedReply = { text: string[] } | { fail: string }

export function scriptedProvider(script: ScriptedReply[]) {
  const requests: Request[] = []
  const plugin: Plugin = {
    name: "scripted-provider",
    setup(host) {
      host.providers.add({
        name: "scripted",
        async *stream(request) {
          requests.push(structuredClone(request))
          const reply = script[requests.length - 1]
          if (!reply) throw new Error("the script has no reply for this request")
          if ("fail" in reply) throw new Error(reply.fail)
          for (const text of reply.text) yield { type: "text", text }
          yield { type: "finish", reason: "stop", usage: { inputTokens: 0, outputTokens: 0 } }
        },
      })
    },
  }
  return { plugin, requests }
}
