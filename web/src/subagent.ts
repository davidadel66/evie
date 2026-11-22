import type { Evie, StoredEvent } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { merged, type Conversation, type ConversationState } from "./conversation.ts"

// The conversation of one of the session's subagents, which the middle
// column shows when the owner picks it in the project card, read only. A
// subagent's session is not opened by itself, so its events are not sent
// to the page: its latest page of history is read, earlier pages as the
// owner scrolls up, and while it works, what it has added since, asked for
// every `every` milliseconds. It is read only while the page is connected,
// one read at a time, and again once the line is back.

// `working` says whether the round of work it is on runs, as the session
// that started it lists its jobs; `poke` asks for what is new at once, as
// when a new round starts.
export type Subagent = Conversation & { poke(): void }

const FRESH: ConversationState = {
  opened: false,
  readingEarlier: false,
  stopping: false,
  waiting: [],
  running: [],
  givenBack: undefined,
  model: undefined,
  context: undefined,
  problem: undefined,
  events: [],
  earlier: false,
  replySoFar: "",
  turnRunning: false,
  jobs: [],
  pending: [],
  notes: [],
}

// Nothing is said in a subagent's conversation from the page: it is shown
// with no message box, and what would talk in it says so.
const READ_ONLY = "A subagent's conversation is read only. Send its session a message instead."

export function openSubagent(connection: Connection, session: string, options: { working(): boolean; every?: number; page?: number }): Subagent {
  let state = FRESH
  const listeners = new Set<() => void>()
  const set = (next: Partial<ConversationState>) => {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }
  // A turn is running when the last one read to start has not ended, or
  // while its round of work runs, whose turn may have begun before the page.
  const turnOpen = (events: readonly StoredEvent[]) => events.findLastIndex((event) => event.type === "turn_start") > events.findLastIndex((event) => event.type === "turn_end")
  const running = (events: readonly StoredEvent[]) => turnOpen(events) || options.working()

  let closed = false
  let reading = false
  let timer: ReturnType<typeof setTimeout> | undefined
  let failed: (() => void) | undefined
  const clientOf = (): Evie | undefined => {
    const now = connection.state()
    return now.status === "connected" ? now.evie : undefined
  }

  // Reads what is new: the latest page, and the pages before it back to
  // the last event read, so that nothing between is missed.
  async function readNew() {
    const evie = clientOf()
    if (closed || reading || evie === undefined || failed !== undefined) return
    reading = true
    clearTimeout(timer)
    const wasRunning = running(state.events)
    const last = state.events.at(-1)?.seq ?? 0
    try {
      const pages: StoredEvent[][] = []
      let before: number | undefined
      let earlier = false
      for (;;) {
        const page = await evie.send({ type: "read_history", session, limit: options.page ?? PAGE, ...(before !== undefined && { before }) })
        pages.push(page.events)
        if (pages.length === 1) earlier = page.earlier
        const first = page.events[0]?.seq
        // The first read is the latest page alone: earlier ones are read as the owner scrolls up.
        if (last === 0 || first === undefined || first <= last + 1 || !page.earlier) break
        before = first
      }
      if (closed) return
      const events = merged(state.events, ...pages)
      set({ opened: true, events, turnRunning: running(events), ...(last === 0 && { earlier }) })
    } catch (cause) {
      if (closed) return
      failed = () => void readNew()
      set({ problem: `The subagent's conversation could not be read: ${wordsOf(cause)}` })
      return
    } finally {
      reading = false
    }
    // Once more after it stops, for what it did last.
    if (running(state.events) || wasRunning) timer = setTimeout(() => void readNew(), options.every ?? EVERY)
  }

  // Reading what is new waits while a page before has failed, and carries
  // on once that page is read. Tried again, it is read on the client the
  // page is connected on then.
  function readEarlier(evie: Evie, before: number) {
    set({ readingEarlier: true })
    evie.send({ type: "read_history", session, limit: options.page ?? PAGE, before }).then(
      (page) => {
        if (closed) return
        set({ readingEarlier: false, events: merged(page.events, state.events), earlier: page.earlier })
        void readNew()
      },
      (cause: unknown) => {
        if (closed) return
        set({ readingEarlier: false })
        failed = () => {
          const now = clientOf()
          if (now !== undefined) readEarlier(now, before)
        }
        set({ problem: `The subagent's earlier conversation could not be read: ${wordsOf(cause)}` })
      },
    )
  }

  function tryAgain() {
    const again = failed
    failed = undefined
    set({ problem: undefined })
    again?.()
  }

  // Read as soon as connected, and when the line comes back, what failed
  // is tried again, or what is new is read.
  const unfollow = connection.subscribe(() => {
    if (clientOf() === undefined) return
    if (failed !== undefined) tryAgain()
    else void readNew()
  })
  void readNew()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    readEarlier() {
      const evie = clientOf()
      const first = state.events[0]?.seq
      if (evie === undefined || first === undefined || !state.opened || !state.earlier || state.readingEarlier || failed !== undefined) return
      readEarlier(evie, first)
    },
    tryAgain,
    send: async () => READ_ONLY,
    takeGivenBack: () => undefined,
    stop() {},
    answer: async () => READ_ONLY,
    async run() {},
    setModel: async () => READ_ONLY,
    listCommands: async () => {
      throw new Error(READ_ONLY)
    },
    listModels: async () => {
      throw new Error(READ_ONLY)
    },
    poke: () => void readNew(),
    close() {
      closed = true
      clearTimeout(timer)
      unfollow()
    },
  }
}

// How many events each read asks for, unless a test asks for fewer, and
// how often, in milliseconds, what is new is asked for while the subagent works.
const PAGE = 50
const EVERY = 1000

const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
