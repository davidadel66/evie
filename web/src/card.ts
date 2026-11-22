import type { Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import type { Conversation } from "./conversation.ts"
import { joinOutput, type Output } from "./project-card.ts"

// The project card's work with evie's server, apart from its screen: undo,
// the git buttons' requests, stopping a job, and following a job's output.

// The client to send on, while the page is connected.
const clientOf = (connection: Connection): Evie | undefined => {
  const now = connection.state()
  return now.status === "connected" ? now.evie : undefined
}
const NOT_CONNECTED = "Not connected to evie's server."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// Runs a command in the session, as the terminal does when the owner types
// it, and shows what it says as a quiet note in its conversation.
async function command(connection: Connection, conversation: Conversation, session: string, name: string, args: string) {
  const evie = clientOf(connection)
  if (evie === undefined) return conversation.note(NOT_CONNECTED)
  try {
    conversation.note((await evie.send({ type: "run_command", session, name, args })).text)
  } catch (cause) {
    conversation.note(`/${name} could not be run: ${wordsOf(cause)}`)
  }
}

// Puts back the files evie's last turn that changed any changed, with /undo.
export const undo = (connection: Connection, conversation: Conversation, session: string) => command(connection, conversation, session, "undo", "")

// Sends the session a request in words, as the owner typing it would,
// such as a git button's. Gives back why it could not be sent, or nothing.
export async function ask(connection: Connection, session: string, text: string): Promise<string | undefined> {
  const evie = clientOf(connection)
  if (evie === undefined) return NOT_CONNECTED
  try {
    await evie.send({ type: "send_message", session, text })
    return undefined
  } catch (cause) {
    return `It could not be sent: ${wordsOf(cause)}`
  }
}

// Stops a background job, or a subagent's round of work, with /jobs stop.
export const stopJob = (connection: Connection, conversation: Conversation, session: string, id: string) => command(connection, conversation, session, "jobs", `stop ${id}`)

// A job's output as the card shows it, read so far, or why it could not be.
export type FollowingState = { output: Output | undefined; problem: string | undefined }
export type Following = {
  state(): FollowingState
  subscribe(listener: () => void): () => void
  close(): void
}

// Follows a job's output: it is read once, and again every `every`
// milliseconds while `running` says the job runs, and once more after it
// has ended, for what it wrote last. The output has no event of its own,
// so it is asked for. It is read only while the page is connected, one
// read at a time, and again once the line is back.
export function followOutput(connection: Connection, session: string, id: string, options: { running(): boolean; every?: number }): Following {
  let state: FollowingState = { output: undefined, problem: undefined }
  const listeners = new Set<() => void>()
  const set = (next: Partial<FollowingState>) => {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }
  let closed = false
  let reading = false
  let timer: ReturnType<typeof setTimeout> | undefined
  // Whether a read is due: the first, and each while the job runs.
  let due = true

  async function read() {
    const evie = clientOf(connection)
    if (closed || reading || !due || evie === undefined) return
    reading = true
    due = false
    const wasRunning = options.running()
    try {
      const answer = await evie.send({ type: "read_job_output", session, id, ...(state.output !== undefined && { after: state.output.to }) })
      if (closed) return
      const output = joinOutput(state.output, answer)
      if (output !== state.output || state.problem !== undefined) set({ output, problem: undefined })
    } catch (cause) {
      // Tried again when the line comes back, if it was down.
      due = true
      if (!closed) set({ problem: `The output could not be read: ${wordsOf(cause)}` })
      return
    } finally {
      reading = false
    }
    if (options.running() || wasRunning) {
      due = true
      timer = setTimeout(() => void read(), options.every ?? EVERY)
    }
  }
  const unfollow = connection.subscribe(() => void read())
  void read()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    close() {
      closed = true
      clearTimeout(timer)
      unfollow()
    },
  }
}

// How often a running job's output is asked for, in milliseconds.
const EVERY = 1000
