import type { EventLog } from "./log.ts"
import type { Inbox } from "./inbox.ts"
import { buildRequest } from "./request-builder.ts"
import type { EventBodies, EvieEvent, Host, Provider, StoredEvent } from "./types.ts"

export type LoopOptions = {
  session: string
  model: string
  log: EventLog
  inbox: Inbox
  host: Host
  provider: Provider
  // Runs a function as one database transaction: all of its writes happen, or none.
  atomically<Result>(work: () => Result): Result
  emit(event: EvieEvent): void
}

export type Loop = {
  // Tells the loop there may be input waiting. Safe to call at any time.
  wake(): void
  // Stops taking new turns, cancels the running one and waits for it to end.
  stop(): Promise<void>
}

// The turn loop. A turn is everything from a message arriving to the model's
// final answer. The loop keeps no conversation in memory: each request is
// rebuilt from the event log.
export function createLoop(options: LoopOptions): Loop {
  const { session, model, log, inbox, host, provider, atomically, emit } = options
  // True from the moment a run of turns starts until its last inbox check.
  let active = false
  // True once stop() has been called; no further turn starts.
  let stopped = false
  // Settles when the current run of turns has finished.
  let running: Promise<void> = Promise.resolve()
  // The cancel signal of the turn in progress, if there is one.
  let cancel: AbortController | undefined

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
    const signal = (cancel = new AbortController()).signal
    let reason: EventBodies["turn_end"]["reason"] = "completed"
    let error: string | undefined
    try {
      // Moving input from the inbox to the log is one transaction, so a crash
      // can neither lose a message nor record it twice.
      const started = atomically(() => {
        const events: StoredEvent[] = [log.append(session, "turn_start", {})]
        for (const message of inbox.take(session)) {
          events.push(log.append(session, "user_message", { text: message.text }))
        }
        return events
      })
      for (const event of started) emit(event)

      const request = buildRequest(model, log.events(session), host.tools.list())
      let text = ""
      for await (const chunk of provider.stream(request, signal)) {
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
      cancel = undefined
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
      cancel?.abort()
      await running
    },
  }
}
