import type { Evie, GitStatus, PullRequest } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import type { Conversation } from "./conversation.ts"
import { joinOutput, REQUESTS, type Listed, type Output } from "./project-card.ts"
import { refreshWhileShown } from "./refresh.ts"

// The session details' work with evie's server, apart from their screen:
// undo, the git buttons' requests, stopping a job, following a job's
// output, listing the session's subagents, and following the branch's pull
// request and how the branch stands.

// The client to send on, while the page is connected.
const clientOf = (connection: Connection): Evie | undefined => {
  const now = connection.state()
  return now.status === "connected" ? now.evie : undefined
}
const NOT_CONNECTED = "Not connected to evie's server."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// Puts back the files evie's last turn that changed any changed, with
// /undo, whose answer the conversation shows as a quiet note, as the
// terminal shows it.
export const undo = (conversation: Conversation) => conversation.run("undo", "")

// Sends evie a git button's request, as the owner typing it in the
// message box would: it shows at once, and starts a turn. Gives back why
// it could not be sent, or nothing.
export const request = (conversation: Conversation, kind: keyof typeof REQUESTS) => conversation.send(REQUESTS[kind])

// Stops a background job, or a subagent's round of work, with /jobs stop.
// What /jobs says is not shown: the job's own lines in the conversation
// say that it stopped. Gives back why it could not be asked for, or nothing.
export async function stopJob(connection: Connection, session: string, id: string): Promise<string | undefined> {
  const evie = clientOf(connection)
  if (evie === undefined) return NOT_CONNECTED
  try {
    await evie.send({ type: "run_command", session, name: "jobs", args: `stop ${id}` })
    return undefined
  } catch (cause) {
    return `Job ${id} could not be stopped: ${wordsOf(cause)}`
  }
}

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
// read at a time, and again once the line is back. A read that fails
// keeps what was read before, and is tried again while the job runs,
// waiting twice as long each time, up to ten times `every`.
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
  // How many reads in a row have failed.
  let failures = 0
  const every = options.every ?? EVERY

  async function read() {
    const evie = clientOf(connection)
    if (closed || reading || !due || evie === undefined) return
    reading = true
    due = false
    // A read asked for early, as when the line comes back, takes the place of the one waiting.
    clearTimeout(timer)
    const wasRunning = options.running()
    try {
      const answer = await evie.send({ type: "read_job_output", session, id, ...(state.output !== undefined && { after: state.output.to }) })
      if (closed) return
      const output = joinOutput(state.output, answer)
      if (output !== state.output || state.problem !== undefined) set({ output, problem: undefined })
      failures = 0
    } catch (cause) {
      // Tried again when the line comes back, if it was down, and while the job runs.
      due = true
      failures += 1
      if (closed) return
      set({ problem: `The output could not be read: ${wordsOf(cause)}` })
      if (options.running()) timer = setTimeout(() => void read(), Math.min(every * 2 ** failures, every * 10))
      return
    } finally {
      reading = false
    }
    if (options.running() || wasRunning) {
      due = true
      timer = setTimeout(() => void read(), every)
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

// The session's stored subagents, read once connected, and again each time
// the line comes back, since the server may have started again since. The
// subagents started meanwhile are known from the session's jobs.
export type Lineage = {
  state(): { listed: Listed[]; problem: string | undefined }
  subscribe(listener: () => void): () => void
  close(): void
}

export function followSubagents(connection: Connection, session: string): Lineage {
  let state: ReturnType<Lineage["state"]> = { listed: [], problem: undefined }
  const listeners = new Set<() => void>()
  const set = (next: Partial<typeof state>) => {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }
  let closed = false
  let listedOn: Evie | undefined
  function list() {
    const evie = clientOf(connection)
    if (closed || evie === undefined || evie === listedOn) return
    listedOn = evie
    evie.send({ type: "list_subagents", session }).then(
      (listed) => !closed && set({ listed, problem: undefined }),
      (cause: unknown) => {
        if (closed) return
        listedOn = undefined
        set({ problem: `The subagents could not be listed: ${wordsOf(cause)}` })
      },
    )
  }
  let status = connection.state().status
  const unfollow = connection.subscribe(() => {
    const now = connection.state().status
    // Back after a drop, the server may have started again, with new jobs.
    if (now === "connected" && status !== "connected") listedOn = undefined
    status = now
    list()
  })
  list()
  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

// A project's pull request, as last read, by the project's id: the diff
// panel compares the branch with the pull request's base.
const pullRequests = new Map<string, PullRequest | null>()
export const pullRequestOf = (project: string) => pullRequests.get(project)

// How often the pull request is read again while the card is shown, in
// milliseconds: its checks change as they run.
const PULL_REQUEST_EVERY = 60_000

// Follows a project's pull request: read once connected, and again each
// minute while the page is shown, when the window has the focus back, and
// when the line comes back. Until it is first read it is undefined; null
// says there is none, or that gh cannot be asked on the server. One that
// could not be read keeps what was read before.
export type FollowedPullRequest = {
  state(): PullRequest | null | undefined
  subscribe(listener: () => void): () => void
  close(): void
}

// `page` is the window, and whether the page is shown, as the clock asks.
export function followPullRequest(connection: Connection, project: string, page: { window: EventTarget; shown(): boolean; every?: number }): FollowedPullRequest {
  let state: PullRequest | null | undefined = pullRequests.get(project)
  const listeners = new Set<() => void>()
  let closed = false
  let reading = false
  async function read() {
    const evie = clientOf(connection)
    if (closed || reading || evie === undefined) return
    reading = true
    try {
      const read = await evie.send({ type: "read_pull_request", project })
      pullRequests.set(project, read)
      if (closed) return
      state = read
      for (const listener of [...listeners]) listener()
    } catch {
      // Kept as it was, and read again at the next turn of the clock.
    } finally {
      reading = false
    }
  }
  let status = connection.state().status
  const unfollow = connection.subscribe(() => {
    const now = connection.state().status
    if (now === "connected" && status !== "connected") void read()
    status = now
  })
  const stop = refreshWhileShown(() => void read(), { window: page.window, shown: page.shown, every: page.every ?? PULL_REQUEST_EVERY })
  void read()
  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    close() {
      closed = true
      unfollow()
      stop()
    },
  }
}

// How often how the branch stands is read again while the card is shown,
// in milliseconds: a commit or a checkout made outside evie sends no event.
const GIT_STATUS_EVERY = 60_000

// Follows how a project's git branch stands: read once connected, again
// when asked, as when a turn ends, when the window has the focus back, each
// minute while the page is shown, and when the line comes back. A branch's
// changes are counted against `base`, the branch's pull request's base as
// the card last read it, as the diff panel compares them. Until it is first
// read it is undefined, unless it was read before on this page, as when
// the card is drawn again in another place; null says the folder is not in
// git. One that could not be read keeps what was read before. One read at
// a time, and once more after one asked for while another was out.
export type FollowedGitStatus = {
  state(): GitStatus | null | undefined
  subscribe(listener: () => void): () => void
  again(): void
  close(): void
}

const gitStatuses = new Map<string, GitStatus | null>()

export function followGitStatus(connection: Connection, project: string, page: { window: EventTarget; shown(): boolean; base(): string | undefined; every?: number }): FollowedGitStatus {
  let state: GitStatus | null | undefined = gitStatuses.get(project)
  const listeners = new Set<() => void>()
  let closed = false
  let reading = false
  let asked = false
  async function read() {
    const evie = clientOf(connection)
    if (closed || evie === undefined) return
    if (reading) {
      asked = true
      return
    }
    reading = true
    try {
      const base = page.base()
      const read = await evie.send({ type: "read_git_status", project, ...(base !== undefined && { base }) })
      gitStatuses.set(project, read)
      if (closed) return
      state = read
      for (const listener of [...listeners]) listener()
    } catch {
      // Kept as it was, and read again when next asked.
    } finally {
      reading = false
      if (asked) {
        asked = false
        void read()
      }
    }
  }
  let status = connection.state().status
  const unfollow = connection.subscribe(() => {
    const now = connection.state().status
    if (now === "connected" && status !== "connected") void read()
    status = now
  })
  const stop = refreshWhileShown(() => void read(), { window: page.window, shown: page.shown, every: page.every ?? GIT_STATUS_EVERY })
  void read()
  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    again: () => void read(),
    close() {
      closed = true
      unfollow()
      stop()
    },
  }
}
