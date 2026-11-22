import type { Commands, EventBodies, Evie, EvieEvent, StoredEvent } from "../../src/core/types.ts"
import type { MenuItem } from "./composer.ts"
import type { Connection } from "./connection.ts"
import type { Model } from "./models.ts"
import type { View } from "./transcript.ts"

// The conversation of the session open in the web app's middle column,
// apart from its screen, which is told it and redrawn as it changes. The
// session is opened by its id, which joins it as it stands; then its latest
// page of history is read, and earlier pages when the owner scrolls up to
// them. Its events are followed as they come, and the network client
// catches up on those missed while the line was down, passing on none twice.

export type ConversationState = Required<View> & {
  // Whether the session has been opened and its latest page read.
  opened: boolean
  // Whether the page before the earliest read is being read.
  readingEarlier: boolean
  // True from when the owner asks to stop the turn until it has ended.
  stopping: boolean
  // The questions waiting for the owner's answer, oldest first.
  waiting: EventBodies["question"][]
  // The commands being run, by name, in the order they were run.
  running: readonly string[]
  // The model the session uses, and how full its conversation is, once
  // they have been read; read again as the conversation changes.
  model: string | undefined
  context: { size: number; compactsAt: number } | undefined
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
  // Sends the owner's message, which starts a turn, or while evie is
  // working steers the turn in progress: the turn takes it in at its next
  // step, as the terminal's does. It is shown as pending until the session
  // has it. Gives back why it could not be sent, in words for the owner, or
  // nothing. Sent while the line is down, it is sent once the line is back.
  send(text: string): Promise<string | undefined>
  // Stops the turn in progress, as Ctrl-C does in the terminal. The
  // conversation says it is stopping until the turn has ended.
  stop(): void
  // Answers a question waiting, with one of its choices, or the owner's own
  // words when it takes them, or null to refuse it. Gives back why the
  // server would not take it, in words for the owner, or nothing. The
  // question stops waiting when its answer comes, from here or elsewhere.
  answer(id: string, choice: string | null): Promise<string | undefined>
  // Runs a command, such as /compact, with what was typed after its name.
  // What it says, or why it failed, is shown as a quiet note where it was
  // run, and kept nowhere, as in the terminal.
  run(name: string, args: string): Promise<void>
  // Switches the session's model, from its next request. Gives back why
  // not, in words for the owner, or nothing.
  setModel(model: string): Promise<string | undefined>
  // The commands and skills that can be run in the session, and the models
  // its provider offers. Fail, saying why, when they cannot be listed.
  listCommands(): Promise<MenuItem[]>
  listModels(): Promise<Model[]>
  // Stops following the session. It stays open in evie.
  close(): void
}

const FRESH: ConversationState = { opened: false, readingEarlier: false, stopping: false, waiting: [], running: [], model: undefined, context: undefined, problem: undefined, events: [], earlier: false, replySoFar: "", turnRunning: false, jobs: [], pending: [], notes: [] }

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
    // What the owner sent and the session has not taken in yet is still on
    // its way, and what was said in this page is still where it was said.
    set({ ...FRESH, pending: state.pending, notes: state.notes })
    // Until the open is answered, the session's events are in its answer:
    // the server holds back those that come after it until it is sent. A
    // question and its answer are not held back, since the open can wait on
    // one, such as whether to trust the session's project: they are kept as
    // they come, so that a question the open waits on is shown.
    let joined = false
    unhear = evie.subscribe((event) => {
      if (on !== evie || event.type === "summary" || event.session !== session) return
      if (joined) hear(event)
      else if (event.type === "question" || event.type === "answer") set({ events: merged(state.events, [event]), waiting: waitingAfter(state.waiting, event) })
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
    // The questions it is waiting on, and any asked as it opened, which the answer leaves out once answered.
    const asked = [...opened.openQuestions, ...state.waiting.filter((question) => !opened.openQuestions.some((open) => open.id === question.id))]
    const answered = new Set(state.events.flatMap((event) => (event.type === "answer" ? [event.body.id] : [])))
    set({ replySoFar: opened.replySoFar ?? "", turnRunning: opened.turnRunning, jobs: opened.jobs, waiting: asked.filter((question) => !answered.has(question.id)) })
    measure(evie)
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

  // The model and how full the conversation is are read one request at a
  // time: asked for again while one is out, they are read once more after
  // it, on the client the session is open on then, so that none applied is
  // older than one applied before it. One that cannot be read leaves what
  // was read before, until the next.
  let measuring = false
  let measureAgain = false
  function measure(evie: Evie) {
    if (measuring) {
      measureAgain = true
      return
    }
    measuring = true
    evie
      .send({ type: "read_context", session })
      .then(
        (read: Commands["read_context"]["output"]) => on === evie && set({ model: read.model, context: { size: read.size, compactsAt: read.compactsAt } }),
        () => {},
      )
      .finally(() => {
        measuring = false
        if (!measureAgain || on === undefined) return
        measureAgain = false
        measure(on)
      })
  }

  // A note said in this page, at the time of the session's last event.
  let notesSaid = 0
  function note(text: string) {
    notesSaid += 1
    set({ notes: [...state.notes, { key: `said${notesSaid}`, time: state.events.at(-1)?.time ?? -Infinity, text }] })
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
    // What changes the conversation's size or model has them read again.
    if (MEASURED.has(event.type) && on !== undefined) measure(on)
    set({
      events: merged(state.events, [event]),
      ...(event.type === "model_changed" && { model: event.body.model }),
      ...(event.type === "user_message" && { pending: without(state.pending, event.body.text) }),
      ...((event.type === "question" || event.type === "answer") && { waiting: waitingAfter(state.waiting, event) }),
      ...(ends && { replySoFar: "" }),
      ...(event.type === "turn_start" && { turnRunning: true }),
      ...(event.type === "turn_end" && { turnRunning: false, stopping: false }),
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
    async send(text) {
      const evie = on
      if (evie === undefined) return NOT_CONNECTED
      set({ pending: [...state.pending, text] })
      try {
        await evie.send({ type: "send_message", session, text })
        return undefined
      } catch (cause) {
        set({ pending: without(state.pending, text) })
        return wordsOf(cause)
      }
    },
    stop() {
      if (on === undefined || !state.turnRunning) return
      set({ stopping: true })
      // A stop that cannot be sent leaves the turn running, and the owner can ask again.
      on.send({ type: "cancel", session }).catch(() => set({ stopping: false }))
    },
    async answer(id, choice) {
      if (on === undefined) return NOT_CONNECTED
      try {
        await on.send({ type: "answer_question", session, id, choice })
        return undefined
      } catch (cause) {
        return wordsOf(cause)
      }
    },
    async run(name, args) {
      if (on === undefined) return note(NOT_CONNECTED)
      set({ running: [...state.running, name] })
      try {
        note((await on.send({ type: "run_command", session, name, args })).text)
      } catch (cause) {
        note(wordsOf(cause))
      } finally {
        set({ running: without(state.running, name) })
      }
    },
    async setModel(model) {
      if (on === undefined) return NOT_CONNECTED
      try {
        await on.send({ type: "set_model", session, model })
        return undefined
      } catch (cause) {
        return wordsOf(cause)
      }
    },
    async listCommands() {
      if (on === undefined) throw new Error(NOT_CONNECTED)
      return on.send({ type: "list_commands", session })
    },
    async listModels() {
      if (on === undefined) throw new Error(NOT_CONNECTED)
      return on.send({ type: "list_models", session })
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

// The events that change how big the conversation is, or its model.
const MEASURED = new Set<StoredEvent["type"]>(["user_message", "assistant_message", "tool_result", "note", "compaction", "model_changed", "turn_end"])

// The questions waiting once a question has been asked, or answered.
function waitingAfter(waiting: readonly EventBodies["question"][], event: Extract<StoredEvent, { type: "question" | "answer" }>): EventBodies["question"][] {
  if (event.type === "answer") return waiting.filter((question) => question.id !== event.body.id)
  return waiting.some((question) => question.id === event.body.id) ? [...waiting] : [...waiting, event.body]
}

// The owner's messages on their way, less the first that is `text`.
function without(pending: readonly string[], text: string): string[] {
  const at = pending.indexOf(text)
  return at === -1 ? [...pending] : [...pending.slice(0, at), ...pending.slice(at + 1)]
}

const NOT_CONNECTED = "Not connected to evie's server yet. Wait until it is connected, and send it again."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// ---- A draft of a new session ----------------------------------------------------

// A draft as the page shows it: the first message, from when it is sent
// until the session has started or could not; the questions asked as the
// session opens, such as whether to trust its project, oldest first; and
// why it could not start, in words for the owner.
export type DraftState = { starting: string | undefined; waiting: EventBodies["question"][]; problem: string | undefined }

export type Draft = {
  state(): DraftState
  subscribe(listener: () => void): () => void
  // Starts the session, in the draft's project or in none, with its first
  // message: opens it on the server, and once its conversation has been
  // read, calls `moved` with its id, for the page to move to it, then sends
  // the message, which the conversation shows as on its way. The
  // conversation is handed over for the page to take (`takeStarted`). A
  // draft closed meanwhile sends the message without moving the page.
  // Gives back the session's id, or why it could not start, when the
  // message is the owner's again.
  start(text: string, moved: (session: string) => void): Promise<{ session: string } | { problem: string }>
  // Answers a question asked as the session opens, as a conversation does.
  answer(id: string, choice: string | null): Promise<string | undefined>
  close(): void
}

// The conversations of sessions this page started from a draft, until the
// page takes them to show; and the project each was started in, kept for
// as long as the page is open, so that the page can show the session
// before the list has it.
const handedOver = new Map<string, Conversation>()
const startedIn = new Map<string, string | null>()

// The conversation of a session this page has just started, once only.
export function takeStarted(session: string): Conversation | undefined {
  const conversation = handedOver.get(session)
  handedOver.delete(session)
  return conversation
}

// The project a session this page started is in, or null for none, or
// undefined for a session it did not start.
export const startedHere = (session: string): string | null | undefined => startedIn.get(session)

// `listed` says whether a session is in the page's list. A question asked
// while the draft's session opens comes before the server says which
// session it is: one from a session the list does not have is the draft's,
// since every other session the page has open is in the list, and the
// server sends the questions of sessions no client has open only while
// this one is opening.
export function openDraft(connection: Connection, project: string | null, options: { listed(session: string): boolean }): Draft {
  let state: DraftState = { starting: undefined, waiting: [], problem: undefined }
  const listeners = new Set<() => void>()
  function set(next: Partial<DraftState>) {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }
  // The session being started, once a question has said which it is.
  let asking: string | undefined
  let unhear = () => {}
  let closed = false
  // The client of the last connection made, which a command sent while the
  // line is down waits on until it is back.
  let client: Evie | undefined
  const follow = () => {
    const now = connection.state()
    if (now.status === "connected") client = now.evie
    if (now.status === "signed out") client = undefined
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    async start(text, moved) {
      const evie = client
      if (evie === undefined) return { problem: NOT_CONNECTED }
      set({ starting: text, problem: undefined })
      unhear()
      unhear = evie.subscribe((event) => {
        if (event.type !== "question" && event.type !== "answer") return
        if (asking === undefined && event.type === "question" && !options.listed(event.session)) asking = event.session
        if (event.session === asking) set({ waiting: waitingAfter(state.waiting, event) })
      })
      const failed = (problem: string) => {
        unhear()
        set({ starting: undefined, waiting: [], problem })
        return { problem }
      }
      let session: string
      try {
        ;({ session } = await evie.send({ type: "open_session", ...(project !== null && { project }), questions: true }))
      } catch (cause) {
        return failed(`The session could not be started: ${wordsOf(cause)}`)
      }
      unhear()
      set({ waiting: [] })
      startedIn.set(session, project)
      const conversation = openConversation(connection, session)
      // Read before the page moves to it, so that it shows the session read.
      await new Promise<void>((resolve) => {
        const stop = conversation.subscribe(() => {
          const read = conversation.state()
          if (!read.opened && read.problem === undefined) return
          stop()
          resolve()
        })
      })
      // Left meanwhile, as when the owner selected another session, the
      // page stays where the owner went, and the message is sent all the
      // same, so that nothing is lost: the session is in the list once it is.
      if (closed) {
        void conversation.send(text).finally(() => conversation.close())
        return { session }
      }
      handedOver.set(session, conversation)
      moved(session)
      set({ starting: undefined })
      // Sent once the page has moved, so that the list, which the message
      // adds the session to, never shows it beside the draft.
      void conversation.send(text)
      return { session }
    },
    async answer(id, choice) {
      if (client === undefined || asking === undefined) return NOT_CONNECTED
      try {
        await client.send({ type: "answer_question", session: asking, id, choice })
        return undefined
      } catch (cause) {
        return wordsOf(cause)
      }
    },
    close() {
      closed = true
      unhear()
      unfollow()
    },
  }
}
