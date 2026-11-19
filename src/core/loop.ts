import { join } from "node:path"
import type { EventLog } from "./log.ts"
import type { Inbox } from "./inbox.ts"
import { buildRequest } from "./request-builder.ts"
import { CancelledBeforeStart, runToolCall } from "./tools.ts"
import type { HookRunner } from "./host.ts"
import type { EventBodies, EvieEvent, HookContext, Host, Project, Provider, StoredEvent, ToolCall } from "./types.ts"

export type LoopOptions = {
  session: string
  // The session's project, or null in the general session.
  project: Project | null
  // Where tool output too long to show is saved in full.
  outputFolder: string
  // The model Evie was started with. A session can switch to another.
  model: string
  log: EventLog
  inbox: Inbox
  host: Host
  hooks: HookRunner
  provider: Provider
  // Runs a function as one database transaction: all of its writes happen, or none.
  atomically<Result>(work: () => Result): Result
  emit(event: EvieEvent): void
}

export type Loop = {
  // Tells the loop there may be input waiting. Throws when the loop has
  // stopped because the event log could not be written.
  wake(): void
  // Cancels the turn in progress, if there is one. Input still waiting
  // starts a new turn afterwards.
  cancel(): void
  // Stops taking new turns, cancels the running one and waits for it to end.
  stop(): Promise<void>
}

// The model a session is using: the last one it was switched to, or the one
// Evie was started with.
export function currentModel(events: readonly StoredEvent[], startedWith: string): string {
  const switched = events.findLast((event) => event.type === "model_changed")
  return switched?.type === "model_changed" ? switched.body.model : startedWith
}

// The turn loop. A turn is everything from a message arriving to the model's
// final answer. A step is one model request plus the tool calls it asks for;
// a turn takes as many steps as the model needs. The loop keeps no
// conversation in memory: each request is rebuilt from the event log.
export function createLoop(options: LoopOptions): Loop {
  const { session, project, outputFolder, model, log, inbox, host, hooks, provider, atomically, emit } = options
  const folder = project?.folder ?? null
  // True from the moment a run of turns starts until its last inbox check.
  let active = false
  // True once stop() has been called; no further turn starts.
  let stopped = false
  // Settles when the current run of turns has finished.
  let running: Promise<void> = Promise.resolve()
  // The cancel signal of the turn in progress, if there is one.
  let cancel: AbortController | undefined
  // Set when the event log could not be written. Nothing can be recorded
  // after that, so the loop takes no more turns.
  let broken: Error | undefined

  async function run() {
    try {
      while (!stopped && inbox.waiting(session)) await turn()
    } catch (cause) {
      // Only a failure to write the log escapes a turn.
      broken = cause instanceof Error ? cause : new Error(String(cause))
      stopped = true
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

    // Moving input from the inbox to the log is one transaction, so a crash
    // can neither lose a message nor record it twice. If it fails, the turn
    // has not started and the failure stops the loop.
    const started = atomically(() => {
      const events: StoredEvent[] = [log.append(session, "turn_start", {})]
      for (const message of inbox.take(session)) {
        events.push(log.append(session, "user_message", { text: message.text }))
      }
      return events
    })
    for (const event of started) emit(event)

    try {
      // One pass of this loop is one step.
      for (let step = 1; ; step++) {
        // Steering: a message sent while the turn was working is picked up
        // here, before the next request, so the model sees it without the
        // turn being stopped. The first step already has the turn's input.
        if (step > 1) {
          const steered = atomically(() =>
            inbox.take(session).map((message) => log.append(session, "user_message", { text: message.text })),
          )
          for (const event of steered) emit(event)
        }

        // What a hook is told about the moment it runs in.
        const context = (): HookContext => ({ session, project, events: log.events(session), signal })

        const events = log.events(session)
        const request = await hooks.beforeRequest(buildRequest(currentModel(events, model), events, host.tools.list()), context())
        // Only the tools the hooks left in the request can be called in this step.
        const offered = host.tools.list().filter((tool) => request.tools.some((left) => left.name === tool.name))
        let text = ""
        const calls: ToolCall[] = []
        let failure: string | undefined
        try {
          for await (const chunk of provider.stream(request, signal)) {
            if (signal.aborted) break
            if (chunk.type === "text") {
              text += chunk.text
              emit({ type: "assistant_delta", session, text: chunk.text })
            }
            if (chunk.type === "tool_call") calls.push({ id: chunk.id, name: chunk.name, arguments: chunk.arguments })
            // The provider reports a failed request as a value, not a throw.
            if (chunk.type === "finish" && chunk.reason === "error") failure = chunk.error
          }
        } catch (cause) {
          // Cancelling makes some providers throw. Anything else is a real failure.
          if (!signal.aborted) throw cause
        }

        if (signal.aborted) {
          // The text so far is kept, marked as cut short. Tool calls are
          // dropped: none of them had finished arriving.
          if (text !== "") emit(log.append(session, "assistant_message", { text, interrupted: true }))
          reason = "cancelled"
          break
        }
        if (failure !== undefined) {
          reason = "error"
          error = failure
          break
        }

        emit(log.append(session, "assistant_message", calls.length > 0 ? { text, toolCalls: calls } : { text }))
        // A reply that asks for no tool is the final answer.
        if (calls.length === 0) break

        // The tools run one at a time, in the order the model asked. Whatever
        // happens, every call gets a result, so the conversation stays one a
        // model will accept.
        const settleRest = (from: number, text: string) => {
          for (const call of calls.slice(from)) emit(log.append(session, "tool_result", { callId: call.id, name: call.name, text }))
        }
        for (const [index, call] of calls.entries()) {
          if (signal.aborted) {
            settleRest(index, "Cancelled: the turn was stopped before this tool ran.")
            break
          }
          let result: string
          try {
            result = await runToolCall(call, {
              tools: offered,
              context: { signal, folder },
              before: (checked) => hooks.beforeTool(checked, context()),
              after: (checked, output) => hooks.afterTool(checked, output, context()),
              outputFolder: join(outputFolder, session),
            })
          } catch (cause) {
            if (signal.aborted) {
              // Being cancelled makes some tools throw. That is not a bug.
              const when = cause instanceof CancelledBeforeStart ? "before this tool ran" : "while this tool was running"
              emit(log.append(session, "tool_result", { callId: call.id, name: call.name, text: `Cancelled: the turn was stopped ${when}.` }))
              settleRest(index + 1, "Cancelled: the turn was stopped before this tool ran.")
              break
            }
            // A tool or a plugin's hook broke, which ends the turn.
            reason = "error"
            error = cause instanceof Error ? cause.message : String(cause)
            emit(log.append(session, "tool_result", { callId: call.id, name: call.name, text: `${error} This is a bug in Evie or a plugin, not a problem with the input.` }))
            settleRest(index + 1, "Not run, because an earlier tool call in this step broke.")
            break
          }
          emit(log.append(session, "tool_result", { callId: call.id, name: call.name, text: result }))
        }
        if (reason === "error") break
        if (signal.aborted) {
          reason = "cancelled"
          break
        }
      }
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
      if (broken) throw new Error(`Evie stopped because its event log could not be written: ${broken.message}`)
      if (active || stopped) return
      active = true
      running = run()
    },
    cancel() {
      cancel?.abort()
    },
    async stop() {
      stopped = true
      cancel?.abort()
      await running
    },
  }
}
