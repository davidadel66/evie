import type { EventLog } from "./log.ts"
import type { Inbox } from "./inbox.ts"
import { buildRequest } from "./request-builder.ts"
import type { EvieEvent, Host, Provider } from "./types.ts"

export type LoopOptions = {
  session: string
  model: string
  log: EventLog
  inbox: Inbox
  host: Host
  provider: Provider
  emit(event: EvieEvent): void
}

export type Loop = {
  // Tells the loop there may be input waiting. Safe to call at any time.
  wake(): void
  // Stops taking new turns and waits for the running one to finish.
  stop(): Promise<void>
}

// The turn loop. A turn is everything from the owner's message to the model's
// final answer. The loop keeps no conversation in memory: each request is
// rebuilt from the event log.
export function createLoop(options: LoopOptions): Loop {
  const { session, model, log, inbox, host, provider, emit } = options
  let active = false
  let stopped = false
  let running: Promise<void> = Promise.resolve()

  async function run() {
    try {
      while (!stopped && inbox.waiting(session)) await turn()
    } finally {
      // Cleared in the same tick as the last inbox check, so a message that
      // arrives next always finds the loop idle and starts a new turn.
      active = false
    }
  }

  async function turn() {
    const cancel = new AbortController()
    let reason: "completed" | "error" = "completed"
    let error: string | undefined
    try {
      const messages = inbox.take(session)
      emit(log.append(session, "turn_start", {}))
      for (const message of messages) emit(log.append(session, "user_message", { text: message.text }))

      const request = buildRequest(model, log.events(session), host.tools.list())
      let text = ""
      for await (const chunk of provider.stream(request, cancel.signal)) {
        if (chunk.type === "text") {
          text += chunk.text
          emit({ type: "assistant_delta", session, text: chunk.text })
        }
      }
      emit(log.append(session, "assistant_message", { text }))
    } catch (cause) {
      reason = "error"
      error = cause instanceof Error ? cause.message : String(cause)
    } finally {
      // A turn always ends in the log, whatever happened inside it.
      emit(log.append(session, "turn_end", error === undefined ? { reason } : { reason, error }))
    }
  }

  return {
    wake() {
      if (active || stopped) return
      active = true
      running = run()
    },
    async stop() {
      stopped = true
      await running
    },
  }
}
