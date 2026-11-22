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
  // for the owner, or undefined.
  problem: string | undefined
}

export type Conversation = {
  // The conversation now. It is the same object until it changes.
  state(): ConversationState
  // Calls the listener each time it changes, until the function returned is called.
  subscribe(listener: () => void): () => void
  // Reads the page before the earliest event read, when there is one and
  // it is not being read already.
  readEarlier(): void
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
  async function join(evie: Evie) {
    unhear()
    on = evie
    set(FRESH)
    // Until the open is answered, the session's events are in its answer:
    // the server holds back those that come after it until it is sent.
    let joined = false
    unhear = evie.subscribe((event) => {
      if (on === evie && joined && event.type !== "summary" && event.session === session) hear(event)
    })
    let opened
    try {
      opened = await evie.send({ type: "open_session", session })
    } catch (cause) {
      if (on === evie) set({ problem: `This session could not be opened: ${wordsOf(cause)}` })
      return
    }
    if (on !== evie) return
    joined = true
    set({ replySoFar: opened.replySoFar ?? "", turnRunning: opened.turnRunning, jobs: opened.jobs })
    try {
      const page = await evie.send({ type: "read_history", session, before: opened.lastSeq + 1 })
      if (on === evie) set({ opened: true, events: merged(page.events, state.events), earlier: page.earlier })
    } catch (cause) {
      if (on === evie) set({ opened: true, problem: `The conversation could not be read: ${wordsOf(cause)}` })
    }
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

  function follow() {
    const now = connection.state()
    if (now.status === "connected" && now.evie !== on) void join(now.evie)
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
      const evie = on
      const first = state.events[0]?.seq
      if (evie === undefined || first === undefined || !state.opened || !state.earlier || state.readingEarlier) return
      set({ readingEarlier: true })
      evie.send({ type: "read_history", session, before: first }).then(
        (page) => on === evie && set({ readingEarlier: false, events: merged(page.events, state.events), earlier: page.earlier }),
        (cause: unknown) => on === evie && set({ readingEarlier: false, problem: `The earlier conversation could not be read: ${wordsOf(cause)}` }),
      )
    },
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
