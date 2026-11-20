import { join } from "node:path"
import type { EventLog } from "./log.ts"
import type { Inbox } from "./inbox.ts"
import {
  DEFAULT_CEILING,
  SUMMARY_PROMPT,
  chooseCut,
  clipToolResults,
  conversationOf,
  filesIn,
  mergeFiles,
  sizeOf,
  tailBudget,
  triggerSize,
} from "./compaction.ts"
import { ONLY_THE_CORE_WRITES, buildRequest } from "./request-builder.ts"
import { CancelledBeforeStart, runToolCall } from "./tools.ts"
import type { HookRunner } from "./host.ts"
import type { EventBodies, EvieEvent, HookContext, Host, Project, Provider, Question, StoredEvent, ToolCall, ToolContext, Usage } from "./types.ts"

export type LoopOptions = {
  session: string
  // The session's project, or null in the general session.
  project: Project | null
  // Where tool output too long to show is saved in full.
  outputFolder: string
  // The model the session starts with. It can switch to another.
  model: string
  // The longest a reply may be, when the settings give a limit.
  maxTokens?: number
  // When to compact: a ceiling in tokens, and limits for models whose own
  // limit the provider does not know or gets wrong.
  compaction?: { ceiling?: number; limits?: Record<string, number> }
  log: EventLog
  inbox: Inbox
  host: Host
  hooks: HookRunner
  provider: Provider
  // Runs a function as one database transaction: all of its writes happen, or none.
  atomically<Result>(work: () => Result): Result
  emit(event: EvieEvent): void
  // Asks the owner a question in this session and waits. The wait ends with
  // a refusal when the signal fires.
  ask(question: Question, signal: AbortSignal): Promise<string | null>
  // The session's background jobs, as tools are given them.
  jobs: ToolContext["jobs"]
  // The session's subagents, as tools are given them.
  children: ToolContext["children"]
  // The session that started this one as a subagent, when one did.
  parent?: string
  // For a subagent: the named agent it is, and whether it only reads.
  agent?: string
  readOnly?: boolean
}

export type Loop = {
  // Tells the loop there is input waiting. If the model is writing a reply,
  // the reply is stopped so the input can be taken in. Throws when the loop
  // has stopped because the event log could not be written.
  wake(): void
  // Cancels the turn in progress, if there is one. Input still waiting
  // starts a new turn afterwards.
  cancel(): void
  // Compacts the conversation now, on request, and says what happened.
  compactNow(): Promise<string>
  // Whether a turn, or a compaction, is in progress.
  busy(): boolean
  // Settles when the loop next has nothing left to do: at once when it is
  // idle, and otherwise when the turns it is running have finished.
  idle(): Promise<void>
  // Adds a note for the model to the conversation: now when the session is
  // idle, or at the turn's next step when it is not. With `wake`, an idle
  // session starts a turn so the model deals with the note.
  tell(text: string, wake?: boolean): void
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
  const { session, project, outputFolder, model, maxTokens, log, inbox, host, hooks, provider, atomically, emit } = options
  const folder = project?.folder ?? null
  // What a subagent's hooks are told about how it was started.
  const startedAs = {
    ...(options.parent !== undefined && { parent: options.parent }),
    ...(options.agent !== undefined && { agent: options.agent }),
    ...(options.readOnly && { readOnly: true as const }),
  }
  // True from the moment a run of turns starts until its last inbox check.
  let active = false
  // True once stop() has been called; no further turn starts.
  let stopped = false
  // Settles when the current run of turns has finished.
  let running: Promise<void> = Promise.resolve()
  // The cancel signal of the turn in progress, if there is one.
  let cancel: AbortController | undefined
  // Stops the reply being streamed, if one is, without ending the turn.
  let reply: AbortController | undefined
  // Set when the event log could not be written. Nothing can be recorded
  // after that, so the loop takes no more turns.
  let broken: Error | undefined

  // Notes for the model that have not been written yet. One that arrives
  // during a turn is written at the next step, where it cannot come between
  // a tool call and its result. One that is to wake the model is written at
  // the start of the turn it causes.
  const waitingNotes: { text: string; wake: boolean }[] = []
  const writeNotes = (which: "all" | "quiet") => {
    const writing = waitingNotes.filter((note) => which === "all" || !note.wake)
    for (const note of writing) waitingNotes.splice(waitingNotes.indexOf(note), 1)
    for (const { text } of writing) emit(log.append(session, "note", { text }))
  }
  // There is work for a turn: a message from the owner, or a note the model is to be woken for.
  const turnWanted = () => inbox.waiting(session) || waitingNotes.some((note) => note.wake)

  // ---- Compaction ---------------------------------------------------------------

  const ceiling = options.compaction?.ceiling ?? DEFAULT_CEILING
  const knownLimits = new Map<string, number>()

  // The size at which a conversation with this model is compacted. The
  // model's limit is looked up once and kept. A lookup that fails, or that
  // is still going when the work is cancelled, leaves the limit unknown for
  // now and is made again the next time.
  async function triggerFor(modelName: string, signal: AbortSignal): Promise<number> {
    if (!knownLimits.has(modelName)) {
      const cancelled = new Promise<undefined>((resolve) => signal.addEventListener("abort", () => resolve(undefined), { once: true }))
      const lookedUp = provider.contextLimit?.(modelName).catch(() => undefined)
      const limit = options.compaction?.limits?.[modelName] ?? (signal.aborted ? undefined : await Promise.race([lookedUp, cancelled]))
      if (limit !== undefined) knownLimits.set(modelName, limit)
    }
    return triggerSize(knownLimits.get(modelName), ceiling)
  }

  // Replaces the older part of the conversation with a summary written by
  // the model. The summary request is the conversation the model has already
  // seen, with one instruction added at the end, so the provider's prompt
  // cache covers almost all of it. A summary that is empty, cut short, or
  // that tries to carry on working is thrown away, and nothing changes.
  async function compact(signal: AbortSignal): Promise<"done" | "nothing" | "failed"> {
    const events = log.events(session)
    const modelName = currentModel(events, model)
    const { compaction: earlier, kept } = conversationOf(events)
    const keepFrom = chooseCut(kept, tailBudget(await triggerFor(modelName, signal)))
    if (keepFrom === undefined) return "nothing"

    // What is being summarised: the part still sent that falls before the
    // cut, led by the earlier summary when there is one. The earlier summary
    // was written later than some of what it sits in front of, so it is
    // placed by hand, not found by its position in the log.
    const old = [...(earlier ? [earlier] : []), ...kept.filter((event) => event.seq < keepFrom)]
    const base = await hooks.beforeRequest(buildRequest(modelName, old, host.tools.list(), maxTokens), {
      session,
      project,
      events,
      signal,
      ask: (question) => options.ask(question, signal),
      ...startedAs,
    })

    // Tried once as it is, then once more with long tool results shortened,
    // in case the summary request is itself more than the model can read.
    for (const messages of [base.messages, clipToolResults(base.messages, 2000)]) {
      const request = { ...base, messages: [...messages, { role: "user" as const, text: SUMMARY_PROMPT }] }
      let summary = ""
      let usable = true
      let tooLong = false
      try {
        for await (const chunk of provider.stream(request, signal)) {
          if (signal.aborted) return "failed"
          if (chunk.type === "text") summary += chunk.text
          // A summary does not use tools. One that asks for one is carrying on the task.
          if (chunk.type === "tool_call") usable = false
          if (chunk.type === "finish") {
            if (chunk.reason !== "stop") usable = false
            if (chunk.reason === "error" && chunk.tooLong) tooLong = true
          }
        }
      } catch {
        return "failed"
      }
      if (signal.aborted) return "failed"
      if (usable && summary.trim() !== "") {
        const files = mergeFiles(earlier?.body.files ?? [], filesIn(old))
        emit(log.append(session, "compaction", { summary: summary.trim(), keepFrom, files }))
        return "done"
      }
      if (!tooLong) return "failed"
    }
    return "failed"
  }

  // ---- Turns --------------------------------------------------------------------

  async function run() {
    try {
      while (!stopped && turnWanted()) await turn()
    } catch (cause) {
      // Only a failure to write the log escapes a turn.
      broken = cause instanceof Error ? cause : new Error(String(cause))
      stopped = true
    } finally {
      // Cleared in the same tick as the last inbox check, so a message that
      // arrives next always finds the loop idle and starts a new turn.
      active = false
      // A note that arrived between the last turn ending and this moment
      // found the loop busy and was held. It is written now, or it would
      // wait for a turn that may never come.
      try {
        writeNotes("quiet")
      } catch {
        // The log cannot be written, which the next thing to try will report.
      }
    }
  }

  async function turn() {
    const signal = (cancel = new AbortController()).signal
    let reason: EventBodies["turn_end"]["reason"] = "completed"
    let error: string | undefined
    // A compaction that fails is not tried again in the same turn, and a
    // request refused as too long is compacted and retried only once.
    let compactionFailed = false
    let retriedTooLong = false

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
        // A model cannot take in anything new while it is writing, so a
        // message that arrives during a reply stops that reply (see wake)
        // and the loop comes back here.
        writeNotes("all")
        if (step > 1) {
          const steered = atomically(() =>
            inbox.take(session).map((message) => log.append(session, "user_message", { text: message.text })),
          )
          for (const event of steered) emit(event)
        }

        // What a hook is told about the moment it runs in.
        const ask = (question: Question) => options.ask(question, signal)
        const context = (): HookContext => ({
          session,
          project,
          events: log.events(session),
          signal,
          ask,
          ...startedAs,
        })

        // A conversation that has grown too big is compacted before the
        // next request: its older part is replaced by a summary.
        const grown = log.events(session)
        if (!compactionFailed && sizeOf(grown) >= (await triggerFor(currentModel(grown, model), signal))) {
          compactionFailed = (await compact(signal)) === "failed"
        }
        if (signal.aborted) {
          reason = "cancelled"
          break
        }

        const events = log.events(session)
        const request = await hooks.beforeRequest(buildRequest(currentModel(events, model), events, host.tools.list(), maxTokens), context())
        // Only the tools the hooks left in the request can be called in this step.
        const offered = host.tools.list().filter((tool) => request.tools.some((left) => left.name === tool.name))
        // A message that arrived while the request was being put together
        // joins it: go round again and take it in.
        if (!signal.aborted && inbox.waiting(session)) continue

        let text = ""
        const calls: ToolCall[] = []
        let failure: string | undefined
        let tooLong = false
        let usage: Usage | undefined
        const replying = (reply = new AbortController())
        // The request is halted when the turn is cancelled or the reply is steered.
        const halted = AbortSignal.any([signal, replying.signal])
        try {
          for await (const chunk of provider.stream(request, halted)) {
            if (halted.aborted) break
            if (chunk.type === "text") {
              text += chunk.text
              emit({ type: "assistant_delta", session, text: chunk.text })
            }
            if (chunk.type === "tool_call") calls.push({ id: chunk.id, name: chunk.name, arguments: chunk.arguments })
            // The provider reports a failed request as a value, not a throw.
            if (chunk.type === "finish") {
              if (chunk.reason === "error") {
                failure = chunk.error
                tooLong = chunk.tooLong === true
              } else usage = chunk.usage
            }
          }
        } catch (cause) {
          // Stopping a request makes some providers throw. Anything else is a real failure.
          if (!halted.aborted) throw cause
        } finally {
          reply = undefined
        }

        if (signal.aborted) {
          // The text so far is kept, marked as cut short. Tool calls are
          // dropped: none of them had finished arriving.
          if (text !== "") emit(log.append(session, "assistant_message", { text, interrupted: true }))
          reason = "cancelled"
          break
        }
        if (replying.signal.aborted) {
          // Steered: the reply was stopped to let a new message in. What it
          // had written is kept, and the turn carries on with the message.
          if (text !== "") emit(log.append(session, "assistant_message", { text, interrupted: true }))
          continue
        }
        if (failure !== undefined) {
          // A request that was more than the model can read is compacted
          // and sent again, once.
          if (tooLong && !retriedTooLong) {
            retriedTooLong = true
            if ((await compact(signal)) === "done") continue
            if (signal.aborted) {
              reason = "cancelled"
              break
            }
          }
          reason = "error"
          error = failure
          break
        }

        // A provider that did not stop where it was asked to may let the
        // model write what only the core writes. It is cut off there, and so
        // are the tool calls: what follows such text is the model acting on
        // a result it made up.
        const forged = Math.min(...ONLY_THE_CORE_WRITES.map((opening) => text.indexOf(opening)).filter((at) => at !== -1))
        if (Number.isFinite(forged)) {
          text = text.slice(0, forged).trimEnd()
          calls.length = 0
        }

        emit(
          log.append(session, "assistant_message", {
            text,
            ...(calls.length > 0 && { toolCalls: calls }),
            ...(usage && { usage }),
          }),
        )
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
              context: { signal, folder, ask, jobs: options.jobs, children: options.children },
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
      // A note that is to wake the model is left for the turn that follows.
      writeNotes("quiet")
      emit(log.append(session, "turn_end", error === undefined ? { reason } : { reason, error }))
    }
  }

  return {
    wake() {
      if (broken) throw new Error(`Evie stopped because its event log could not be written: ${broken.message}`)
      // New input during a reply stops the reply, so the model can be asked
      // again with the input added.
      if (active) reply?.abort()
      if (active || stopped) return
      active = true
      running = run()
    },
    cancel() {
      cancel?.abort()
    },
    async compactNow() {
      if (active) return "Evie is in the middle of a turn. Compact when it has finished."
      if (stopped) return "Evie is closing, so nothing was changed."
      // Held as busy while it runs, so a message that arrives waits for it.
      // It is the running work, like a turn: cancel() stops it, and stop()
      // waits for it to end before the database is closed.
      active = true
      const stopping = (cancel = new AbortController())
      const work = compact(stopping.signal)
      running = work.then(
        () => {},
        () => {},
      )
      try {
        const outcome = await work
        if (outcome === "done") return "Compacted: the earlier part of the conversation is now a summary."
        if (stopping.signal.aborted) return "Compaction was cancelled, so nothing was changed."
        if (outcome === "nothing") return "There is not enough conversation to compact yet."
        return "The summary could not be written, so nothing was changed."
      } finally {
        cancel = undefined
        active = false
        writeNotes("quiet")
        if (!stopped && turnWanted()) this.wake()
      }
    },
    busy() {
      return active
    },
    idle() {
      return running
    },
    tell(text, wake = false) {
      waitingNotes.push({ text, wake })
      if (active) return
      if (wake && !stopped) this.wake()
      else writeNotes("quiet")
    },
    async stop() {
      stopped = true
      cancel?.abort()
      await running
    },
  }
}
