import type { Evie, EvieEvent, StoredEvent } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import type { View } from "./transcript.ts"

// The conversation of the session open in the web app's middle column,
// apart from its screen, which is told it and redrawn as it changes. The
// session is opened by its id, which joins it as it stands; then its latest
// page of history is read, and earlier pages when the owner scrolls up to
// them. Its events are followed as they come, and the network client
// catches up on those missed while the line was down, passing on none twice.

export type ConversationState = View & {
  // Whether the session has been opened and its latest page read.
  opened: boolean
  // Whether the page before the earliest read is being read.
  readingEarlier: boolean
  // Why the session could not be opened, or a page of it read, in words
  // for the owner, or undefined. What failed is tried again when the line
  // comes back after a drop, or when the owner asks: nothing else tries it
  // again, so that a failure cannot repeat itself without end.
  problem: string | undefined
}

export type Conversation = {
  // The conversation now. It is the same object until it changes.
  state(): ConversationState
  // Calls the listener each time it changes, until the function returned is called.
  subscribe(listener: () => void): () => void
  // Reads the page before the earliest event read, when there is one, it is
  // not being read already, and nothing that failed waits to be tried again.
  readEarlier(): void
  // Tries again what failed: opening the session, or reading a page of it.
  tryAgain(): void
  // Stops following the session. It stays open in evie.
  close(): void
}

const FRESH: ConversationState = { opened: false, readingEarlier: false, problem: undefined, events: [], earlier: false, replySoFar: "", turnRunning: false, jobs: [] }

export function openConversation(connection: Connection, session: string): Conversation {
  let state = FRESH
  const listeners = new Set<() => void>()
  function set(next: Partial<ConversationState>) {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }

  // The client the session is open on. A connection made again after the
  // browser was signed out is a new client, on which it is opened again.
  let on: Evie | undefined
  let unhear = () => {}
  // What failed, to be tried again.
  let failed: (() => void) | undefined
  function fail(problem: string, again: () => void) {
    failed = again
    set({ problem })
  }
  // The number of the session's last event as it was opened: its latest
  // page is read up to it, and every event after it comes as it happens.
  let lastSeq = 0

  async function join(evie: Evie) {
    unhear()
    on = evie
    failed = undefined
    set(FRESH)
    // Until the open is answered, the session's events are in its answer:
    // the server holds back those that come after it until it is sent. A
    // question and its answer are not held back, since the open can wait on
    // one, such as whether to trust the session's project: they are kept as
    // they come, so that a question the open waits on is shown.
    let joined = false
    unhear = evie.subscribe((event) => {
      if (on !== evie || event.type === "summary" || event.session !== session) return
      if (joined) hear(event)
      else if (event.type === "question" || event.type === "answer") set({ events: merged(state.events, [event]) })
    })
    // Opened as a client that shows questions, so that one waits for an
    // answer, from here or from a terminal, and is not refused.
    let opened
    try {
      opened = await evie.send({ type: "open_session", session, questions: true })
    } catch (cause) {
      if (on === evie) fail(`This session could not be opened: ${wordsOf(cause)}`, () => void join(evie))
      return
    }
    if (on !== evie) return
    joined = true
    lastSeq = opened.lastSeq
    set({ replySoFar: opened.replySoFar ?? "", turnRunning: opened.turnRunning, jobs: opened.jobs })
    await readLatest(evie)
  }

  async function readLatest(evie: Evie) {
    try {
      const page = await evie.send({ type: "read_history", session, before: lastSeq + 1 })
      if (on === evie) set({ opened: true, events: merged(page.events, state.events), earlier: page.earlier })
    } catch (cause) {
      if (on === evie) fail(`The conversation could not be read: ${wordsOf(cause)}`, () => void readLatest(evie))
    }
  }

  function readEarlier(evie: Evie, before: number) {
    set({ readingEarlier: true })
    evie.send({ type: "read_history", session, before }).then(
      (page) => on === evie && set({ readingEarlier: false, events: merged(page.events, state.events), earlier: page.earlier }),
      (cause: unknown) => {
        if (on !== evie) return
        set({ readingEarlier: false })
        fail(`The earlier conversation could not be read: ${wordsOf(cause)}`, () => readEarlier(evie, before))
      },
    )
  }

  function tryAgain() {
    const again = failed
    failed = undefined
    set({ problem: undefined })
    again?.()
  }

  // An event of the session, as it comes. The reply being written is
  // stored whole once it is done, or ends with its turn or a compaction.
  function hear(event: Exclude<EvieEvent, { type: "summary" }>) {
    if (event.type === "assistant_delta") return set({ replySoFar: state.replySoFar + event.text })
    if (event.type === "job") return set({ jobs: [...state.jobs.filter((job) => job.id !== event.job.id), event.job] })
    const ends = event.type === "assistant_message" || event.type === "turn_end" || event.type === "compaction"
    set({
      events: merged(state.events, [event]),
      ...(ends && { replySoFar: "" }),
      ...(event.type === "turn_start" && { turnRunning: true }),
      ...(event.type === "turn_end" && { turnRunning: false }),
    })
  }

  // Opened once connected, and on the new client after signing in again.
  // When the line is back after a drop, what failed is tried again.
  let status = connection.state().status
  function follow() {
    const now = connection.state()
    const back = now.status === "connected" && status !== "connected"
    status = now.status
    if (now.status !== "connected") return
    if (now.evie !== on) void join(now.evie)
    else if (back && failed !== undefined) tryAgain()
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    readEarlier() {
      const first = state.events[0]?.seq
      if (on === undefined || first === undefined || !state.opened || !state.earlier || state.readingEarlier || failed !== undefined) return
      readEarlier(on, first)
    },
    tryAgain,
    close() {
      unfollow()
      unhear()
      on = undefined
    },
  }
}

// Two runs of a session's events as one, in order, each event once.
function merged(...runs: readonly (readonly StoredEvent[])[]): StoredEvent[] {
  const bySeq = new Map<number, StoredEvent>()
  for (const run of runs) for (const event of run) bySeq.set(event.seq, event)
  return [...bySeq.values()].sort((a, b) => a.seq - b.seq)
}

const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
