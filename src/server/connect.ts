import type { Command, Commands, Evie, EvieEvent, Job, StoredEvent } from "../core/types.ts"
import { PATHS, TAKEN_BACK, type ClientMessage, type ServerMessage } from "./wire.ts"

export type ConnectOptions = {
  // The server's address, such as http://127.0.0.1:3843.
  url: string
  // This device's token. Left out in a browser, which cannot set headers on
  // a WebSocket and sends instead the cookie it was given at login.
  token?: string
  // Called when the line to the server drops without `close` being called,
  // as when the laptop sleeps or the server stops, with why. The client
  // tries again by itself, after one second and then twice as long each
  // time, up to ten seconds, until the line is back, `close` is called, or
  // `giveUpAfter` has passed. Commands sent meanwhile wait for the line.
  onDrop?: (reason: string) => void
  // Called when the line is back after a drop. What was missed while it was
  // down is passed on straight after.
  onBack?: () => void
  // Called when catching up on a session fails, as when its project's
  // plugins fail to load as it is opened again, with a note for the owner
  // saying that some of its events may be missing, and why. Its live events,
  // when any reach the client, are passed on as before.
  onMissing?: (note: string) => void
  // Called when the client stops for good without `close` being called,
  // because the server took this device's token back, or refuses it or the
  // page, or because the line stayed down past `giveUpAfter`, with why.
  // Commands still waiting fail, and so does any sent after.
  onEnd?: (cause: ConnectError) => void
  // How long, in milliseconds, to go on trying after the line drops, for a
  // client that has no one to stop it, such as a script. Then it ends, with
  // why the last try failed. Left out, it keeps trying.
  giveUpAfter?: number
  // Gives up connecting when it fires, such as at a time limit. It is used
  // only while connecting: once the connection is open it changes nothing.
  signal?: AbortSignal
  // For tests: the first wait, in milliseconds, before trying again after
  // a drop, one second when left out; and how long the line may carry
  // nothing before it is taken to have dropped, 45 seconds when left out.
  firstWait?: number
  deadAfter?: number
}

// Why a connection could not be made. The message says it in words for the
// owner; `reason` says it for a program that has more to add, such as which
// token it sent.
export class ConnectError extends Error {
  constructor(
    message: string,
    readonly reason: "no server" | "no answer" | "token" | "page" | "other",
  ) {
    super(message)
  }
}

type Opened = Commands["open_session"]["output"]
type OpenInput = Extract<Command, { type: "open_session" }>
// What the server sends back for one command.
type Reply = Extract<ServerMessage, { reply: string }>

// A command waiting for its reply: the owner's, sent again on each new line
// until answered, or the client's own (`own`), to catch up on a session,
// sent once and given up on when its line drops, so the server is told not
// to remember its reply. `sentOn` is the line it was last sent on; `resent`
// says it was sent on an earlier one too.
type Waiting = {
  message: ClientMessage
  own: boolean
  sentOn?: WebSocket
  resent: boolean
  answer(reply: Reply): void
  // No reply will come, for the reason given.
  lose(why: string): void
}

// What the client keeps for each session whose events it has passed on.
type Seen = {
  // The number of the last stored event passed on.
  seq: number
  // The text of the reply being written, as far as it has been passed on.
  shown: string
  // Each of the session's jobs, as it was last passed on, by id.
  jobs: Map<string, Job>
  // For a session the owner opened: whether it was opened as one that
  // shows questions, so that it is opened the same way after a drop.
  openAs?: { questions: boolean }
  // True while the client is catching up on the session, and the numbers
  // of the questions and answers passed on meanwhile, so that they are
  // skipped when they come round again, kept until the client's number has
  // passed them: see `onEvent` and `catchUp`.
  behind: boolean
  early: Set<number>
  // Whether the owner has been given where the session stands, in the reply
  // to an open of it. Until then a catch-up gives the owner where it stands
  // in that reply, in place of passing it on: see `onOpened`.
  told: boolean
  // The owner's opens of the session that wait for a catch-up on it to answer.
  opening: OpenWaiting[]
}

// An open of the owner's that waits for a catch-up on its session, to be
// given where the session stands as the catch-up read it, or nothing when
// the catch-up failed; or no reply, for the reason given.
type OpenWaiting = { caughtUp(now: Opened | undefined): void; lose(why: string): void }

// How long to wait before trying again after the line drops: one second,
// then twice as long each time, up to ten seconds.
const FIRST_WAIT = 1000
const LONGEST_WAIT = 10_000
// How long a try to connect again may take before it is given up on, for a
// server that takes the connection but never answers.
const CONNECT_TIMEOUT = 10_000
// A line can die without closing, as when a laptop sleeps or changes
// network. The server sends a keepalive every 15 seconds, so a line that
// has carried nothing for three times that long is taken to have dropped.
const DEAD_AFTER = 45_000

// Connects to Evie's server and returns the same client interface the core
// gives, `send`, `subscribe` and `close`, carried over one WebSocket. A
// client written for the core works over it unchanged. Closing it closes
// the connection only: the server, and anything running in it, carry on.
// When the connection is refused, or the signal fires before it opens, it
// rejects with a `ConnectError`.
//
// When the line drops after that, the client connects again by itself and
// catches up: for each session it has open it asks for the events after the
// last one it passed on, and the reply being written, and passes those on
// before any live event, so nothing is shown twice and nothing is lost. A
// command that was not answered when the line dropped is sent again with
// the same id, and the server, while it keeps running, answers it without
// carrying it out twice: it remembers the replies to the last 200 commands
// of each device, however long the line was down. Since that reply can be
// from long ago, a session opened by its folder, or to continue, is handed
// back as it stands when the client opens it again by its id straight after.
export async function connect(options: ConnectOptions): Promise<Evie> {
  const address = new URL(options.url)
  // A browser's WebSocket takes no headers, and has the cookie in place of the token.
  const headers = options.token === undefined ? undefined : { Authorization: `Bearer ${options.token}` }
  let socket: WebSocket | undefined = await openSocket(address, headers, options.signal)

  const listeners = new Set<(event: EvieEvent) => void>()
  // Commands waiting for their reply, by id, in the order sent.
  const waiting = new Map<string, Waiting>()
  const sessions = new Map<string, Seen>()
  // True from the line coming back until every session has been caught up
  // on. Meanwhile only answers to questions are sent: see `flush`.
  let rejoining = false
  // Fires when the client stops for good, with why, and stops it trying again.
  const stopping = new AbortController()
  let closing: Promise<void> | undefined
  // Fires when the line as it is now has carried nothing for too long.
  let silence: ReturnType<typeof setTimeout> | undefined
  // While the line is down: why, as the last try to connect again said, and when to give up.
  let down: ConnectError | undefined
  let giveUp: ReturnType<typeof setTimeout> | undefined

  // Hands an event to every listener. As in the core, a listener that
  // throws does not stop the others hearing it.
  const deliver = (event: EvieEvent) => {
    for (const listener of [...listeners]) {
      try {
        listener(event)
      } catch {}
    }
  }
  const seenOf = (session: string): Seen => {
    let seen = sessions.get(session)
    if (!seen) sessions.set(session, (seen = { seq: 0, shown: "", jobs: new Map(), behind: false, early: new Set(), told: false, opening: [] }))
    return seen
  }
  const isNew = (seen: Seen, event: StoredEvent) => event.seq > seen.seq && !seen.early.has(event.seq)

  // Passes an event on to the owner, once: a stored event whose number has
  // been passed on already is skipped, and so is a job that has not changed.
  function passOn(event: EvieEvent) {
    const seen = seenOf(event.session)
    if ("seq" in event) {
      if (!isNew(seen, event)) return
      seen.seq = event.seq
    }
    if (event.type === "job") {
      if (seen.jobs.get(event.job.id)?.status === event.job.status) return
      seen.jobs.set(event.job.id, event.job)
    }
    if (event.type === "assistant_delta") seen.shown += event.text
    // A reply's text ends with the reply, or with its turn when it failed.
    else if (event.type === "assistant_message" || event.type === "turn_end" || event.type === "compaction") seen.shown = ""
    deliver(event)
  }
  // Passes on, as one piece, the part of a reply's text that has not been passed on yet.
  function passOnText(session: string, text: string) {
    const rest = unshown(text, seenOf(session).shown)
    if (rest !== "") passOn({ type: "assistant_delta", session, text: rest })
  }

  // A live event from the server. While the client catches up on its
  // session, the server sends the reply after every event it describes, so
  // the event is in the reply on its way. A question or its answer is passed
  // on at once all the same, since opening a session again can wait on one,
  // such as whether to trust its project.
  //
  // A session's summary, which comes for every session, is passed on as it
  // comes. It is no part of a session's events: it is not counted, and those
  // sent while the line was down are not caught up on. A client that keeps a
  // list of sessions lists them again when the line is back (see `onBack`).
  function onEvent(event: EvieEvent) {
    if (event.type === "summary") return deliver(event)
    const seen = sessions.get(event.session)
    if (!seen?.behind) return passOn(event)
    if ((event.type === "question" || event.type === "answer") && isNew(seen, event)) {
      seen.early.add(event.seq)
      deliver(event)
    }
  }

  // The reply to a session the owner opened, and what the owner is given.
  function onOpened(command: Waiting, output: Opened): Opened | Promise<Opened> {
    const input = command.message.command as OpenInput
    const seen = seenOf(output.session)
    // A session opened by its folder, or to continue, is sent to this line
    // only once the server has learned which it is, a moment after the core
    // read where it stands, so that moment is caught up on.
    const unsent = !seen.openAs && input.session === undefined
    if (seen.openAs) seen.openAs.questions ||= input.questions === true
    else seen.openAs = { questions: input.questions === true }
    // A reply sent again can come from the server's memory of long ago, so
    // the owner who opened a session by its folder, or to continue, is given
    // where it stands as the catch-up that follows reads it.
    const line = socket
    if (line && input.session === undefined && (unsent || command.resent || seen.seq < output.lastSeq)) {
      return new Promise<Opened>((resolve, reject) => {
        seen.opening.push({
          caughtUp(now) {
            if (now) return resolve(standing(output, now))
            if (!seen.told) tell(seen, output)
            resolve(output)
          },
          lose: (why) => reject(lost(why)),
        })
        void catchUp(output.session, line)
      })
    }
    if (!seen.told) tell(seen, output)
    // What lies between is caught up on, as when the server answers a
    // command sent again from its memory of a moment before the line
    // dropped, and refused that question when it did.
    if (line && (command.resent || seen.seq < output.lastSeq)) void catchUp(output.session, line)
    return output
  }

  // Records that the owner has been given where a session stands, as this
  // reply to an open of it says: the reply so far, and the jobs.
  function tell(seen: Seen, output: Opened) {
    seen.told = true
    seen.shown = output.replySoFar ?? ""
    for (const job of output.jobs) seen.jobs.set(job.id, job)
    // The reply's number is where the client stands, unless it has passed
    // on events of the session already, such as the question whether to
    // trust its project: none after those is ever skipped.
    if (seen.seq === 0) seen.seq = output.lastSeq
  }

  // Asks the server for what was missed in a session since the last event
  // passed on, and passes it on. Live events follow the reply. When the
  // line drops first, it passes nothing on: the client catches up afresh on
  // the next line.
  async function catchUp(session: string, line: WebSocket): Promise<void> {
    const seen = seenOf(session)
    // One under way already covers it.
    if (seen.behind) return
    seen.behind = true
    // Of a session the owner has been told nothing of yet, nothing earlier
    // is asked for: the owner is given where it stands now.
    const fromNow = !seen.told && seen.seq === 0
    const reply = await ask(line, {
      type: "open_session",
      session,
      ...(!fromNow && { after: seen.seq }),
      ...(seen.openAs?.questions && { questions: true }),
    })
    seen.behind = false
    // A session that could not be opened again, as when its project's
    // plugins fail to load, is left as it is: a command sent to it says why,
    // and the owner is told that some of its events may be missing, since
    // any live ones that reach the client from then on move its number past
    // them. Then, or when the line dropped first, the events after the last one
    // passed on have not been, besides the questions and answers passed on
    // meanwhile. So the number stays where it is, for the next catch-up to
    // ask from, and those questions and answers stay in `early`, for it to skip.
    if (!reply) return
    const opening = seen.opening.splice(0)
    if ("error" in reply) {
      for (const open of opening) open.caughtUp(undefined)
      options.onMissing?.(`Catching up on the session ${session} failed, so some of its events may be missing here. ${reply.error}`)
      return
    }
    const output = reply.result as Opened
    // Jobs' events are not stored, so each job that has changed is passed
    // on, first, since the turns missed are often Evie dealing with a job
    // that ended. One missing from the list ran in the server before it was
    // stopped for good and started again, which stopped it.
    if (seen.told) {
      const listed = new Set(output.jobs.map((job) => job.id))
      const gone = [...seen.jobs.values()].filter((job) => job.status === "running" && !listed.has(job.id))
      for (const job of gone) passOn({ type: "job", session, job: { ...job, status: "stopped", ended: Date.now() } })
      for (const job of output.jobs) passOn({ type: "job", session, job })
    }
    for (const event of output.events ?? []) {
      // A client of the core always sees a reply's text as pieces before
      // the stored message, so the part of a reply that was missed is
      // passed on as one piece before it.
      if (event.type === "assistant_message" && isNew(seen, event)) passOnText(session, event.body.text)
      passOn(event)
    }
    // An owner not yet told where the session stands is given the jobs and
    // the reply so far in the reply to its open, so they are not passed on.
    if (seen.told) passOnText(session, output.replySoFar ?? "")
    else tell(seen, output)
    // The client stands where the reply read the session. A question or an
    // answer passed on meanwhile can be numbered past that, and so can the
    // events the server held back until the reply was sent, which follow
    // now, some of them numbered below that question. So the number goes no
    // further, and each such question or answer stays in `early`, to be
    // skipped if it comes round again.
    seen.seq = Math.max(seen.seq, output.lastSeq)
    for (const seq of seen.early) if (seq <= seen.seq) seen.early.delete(seq)
    for (const open of opening) open.caughtUp(output)
  }

  // Sends a command of the client's own on this line. It resolves with the
  // server's reply, or with nothing when the line drops first.
  function ask(line: WebSocket, command: Command): Promise<Reply | undefined> {
    const message: ClientMessage = { id: commandId(), command, once: true }
    return new Promise((resolve) => {
      waiting.set(message.id, { message, own: true, sentOn: line, resent: false, answer: resolve, lose: () => resolve(undefined) })
      sendOn(line, message)
    })
  }

  // Sends each of the owner's waiting commands that has not been sent on
  // the line as it is now, in the order they were sent. While rejoining,
  // only answers to questions go: opening a session again can wait on one,
  // and anything else could reach a session before it is open again.
  function flush() {
    const line = socket
    if (!line) return
    for (const command of waiting.values()) {
      if (command.sentOn === line || (rejoining && command.message.command.type !== "answer_question")) continue
      command.resent = command.sentOn !== undefined
      command.sentOn = line
      sendOn(line, command.message)
    }
  }

  // A line that is closing cannot send. What was not sent waits for the next.
  const sendOn = (line: WebSocket, message: ClientMessage) => {
    try {
      line.send(JSON.stringify(message))
    } catch {}
  }

  // Starts timing the line's silence again, from now.
  function heardFrom(line: WebSocket) {
    clearTimeout(silence)
    silence = setTimeout(() => {
      dropped("the server stopped answering")
      line.close()
    }, options.deadAfter ?? DEAD_AFTER)
  }

  // A message from the server, on the line it came on.
  function onMessage(line: WebSocket, text: string) {
    if (line !== socket) return
    heardFrom(line)
    const received = JSON.parse(text) as ServerMessage
    if ("keepalive" in received) return
    if ("event" in received) return onEvent(received.event)
    const command = waiting.get(received.reply)
    if (!command) return
    waiting.delete(received.reply)
    command.answer(received)
  }

  function use(line: WebSocket) {
    socket = line
    heardFrom(line)
    line.addEventListener("message", (message) => onMessage(line, String(message.data)))
    line.addEventListener("close", (event) => {
      // A line given up on, or closed by `close`, is no longer `socket`.
      if (line !== socket) return
      if (event.code === TAKEN_BACK) return end(new ConnectError(event.reason || "This device's token was taken back", "token"))
      dropped(event.reason || "the server went away")
    })
  }

  function dropped(reason: string) {
    socket = undefined
    clearTimeout(silence)
    rejoining = false
    for (const [id, command] of waiting) {
      if (!command.own) continue
      waiting.delete(id)
      command.lose(reason)
    }
    const why = (down = new ConnectError(`The connection to the evie server at ${address.origin} dropped: ${reason}.`, "other"))
    if (options.giveUpAfter !== undefined) giveUp = setTimeout(() => end(down ?? why), options.giveUpAfter)
    options.onDrop?.(reason)
    void tryAgain()
  }

  async function tryAgain() {
    for (let wait = options.firstWait ?? FIRST_WAIT; ; wait = Math.min(wait * 2, LONGEST_WAIT)) {
      await pause(wait, stopping.signal)
      if (stopping.signal.aborted) return
      let line: WebSocket
      try {
        line = await openSocket(address, headers, AbortSignal.any([stopping.signal, AbortSignal.timeout(CONNECT_TIMEOUT)]))
      } catch (cause) {
        if (!(cause instanceof ConnectError)) throw cause
        if (stopping.signal.aborted) return
        // A refusal that trying again will not change.
        if (cause.reason === "token" || cause.reason === "page") return end(cause)
        down = cause
        continue
      }
      if (stopping.signal.aborted) return line.close()
      return back(line)
    }
  }

  async function back(line: WebSocket) {
    use(line)
    down = undefined
    clearTimeout(giveUp)
    options.onBack?.()
    rejoining = true
    flush()
    const open = [...sessions].flatMap(([session, seen]) => (seen.openAs ? [session] : []))
    await Promise.all(open.map((session) => catchUp(session, line)))
    // The line dropped again first, and trying again has begun.
    if (socket !== line) return
    rejoining = false
    flush()
  }

  // Every command still waiting fails, saying why.
  const loseAll = (why: string) => {
    for (const command of waiting.values()) command.lose(why)
    waiting.clear()
    for (const seen of sessions.values()) for (const open of seen.opening.splice(0)) open.lose(why)
  }

  // Stops the client for good, saying why, and hands back the line it was on.
  function stop(why: string): WebSocket | undefined {
    stopping.abort(why)
    clearTimeout(silence)
    clearTimeout(giveUp)
    const line = socket
    socket = undefined
    return line
  }

  function end(cause: ConnectError) {
    if (stopping.signal.aborted) return
    stop(cause.message)
    loseAll(cause.message)
    options.onEnd?.(cause)
  }

  use(socket)

  return {
    send<Sent extends Command>(command: Sent) {
      if (stopping.signal.aborted) return Promise.reject(new Error(`Not connected to evie: ${String(stopping.signal.reason).replace(/\.$/, "")}.`))
      const message: ClientMessage = { id: commandId(), command }
      return new Promise<Commands[Sent["type"]]["output"]>((resolve, reject) => {
        const waited: Waiting = {
          message,
          own: false,
          resent: false,
          answer(reply) {
            if ("error" in reply) return reject(new Error(reply.error))
            const result = command.type === "open_session" ? onOpened(waited, reply.result as Opened) : reply.result
            resolve(result as Commands[Sent["type"]]["output"] | Promise<Commands[Sent["type"]]["output"]>)
          },
          lose: (why) => reject(lost(why)),
        }
        waiting.set(message.id, waited)
        flush()
      })
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    close() {
      closing ??= (async () => {
        const line = stop("the connection was closed")
        if (line && line.readyState !== WebSocket.CLOSED) {
          await new Promise<void>((resolve) => {
            line.addEventListener("close", () => resolve(), { once: true })
            line.close()
          })
        }
        loseAll("the connection was closed")
      })()
      return closing
    },
  }
}

// A new id for a command: 128 random bits, as hex. It is made from
// `getRandomValues`, since a browser leaves `crypto.randomUUID` out of a
// page served over plain HTTP from another computer, and this client runs
// in the web UI too.
export function commandId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")
}

// Why a command failed when no reply to it will come.
const lost = (why: string) => new Error(`The connection to evie was lost before it answered: ${why.replace(/\.$/, "")}.`)

// The reply to an open, with where the session stands as a later reading of
// it says: whether a turn is running, the questions waiting, the reply so
// far, the jobs, and the number of the last event, from which the client
// passes on every event after.
function standing(first: Opened, now: Opened): Opened {
  const { replySoFar: _, ...rest } = first
  return {
    ...rest,
    turnRunning: now.turnRunning,
    openQuestions: now.openQuestions,
    jobs: now.jobs,
    lastSeq: now.lastSeq,
    ...(now.replySoFar !== undefined && { replySoFar: now.replySoFar }),
  }
}

// The part of a reply's text that has not been passed on yet, given what
// has. Text that does not carry on from it, as when a request failed and
// was made again, is passed on whole, unless it is the start of what was
// passed on already, as when a reply was cut short after it streamed.
function unshown(text: string, shown: string): string {
  if (text.startsWith(shown)) return text.slice(shown.length)
  return shown.startsWith(text) ? "" : text
}

// Waits, or until the signal fires. A browser has no node:timers/promises,
// and this client runs in one too.
function pause(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const done = () => {
      clearTimeout(timer)
      signal.removeEventListener("abort", done)
      resolve()
    }
    const timer = setTimeout(done, ms)
    signal.addEventListener("abort", done, { once: true })
  })
}

// Bun's WebSocket takes headers, to send a token with. A browser's takes
// none, and needs none, since it sends its cookie instead. Named this way,
// the client type-checks against both.
type WithHeaders = new (address: URL, options: { headers: Record<string, string> }) => WebSocket

// Opens a WebSocket to the server, or says why it could not.
async function openSocket(address: URL, headers: Record<string, string> | undefined, signal: AbortSignal | undefined): Promise<WebSocket> {
  const socketAddress = new URL(PATHS.socket, address)
  socketAddress.protocol = address.protocol === "https:" ? "wss:" : "ws:"
  const socket = headers === undefined ? new WebSocket(socketAddress) : new (WebSocket as unknown as WithHeaders)(socketAddress, { headers })
  let giveUp = () => {}
  const outcome = await new Promise<"open" | "refused" | "gave up">((resolve) => {
    giveUp = () => resolve("gave up")
    socket.addEventListener("open", () => resolve("open"), { once: true })
    socket.addEventListener("close", () => resolve("refused"), { once: true })
    if (signal?.aborted) giveUp()
    else signal?.addEventListener("abort", giveUp, { once: true })
  })
  signal?.removeEventListener("abort", giveUp)
  if (outcome === "gave up") {
    socket.close()
    throw noAnswer(address)
  }
  if (outcome === "refused") throw await whyRefused(address, headers, signal)
  return socket
}

// A WebSocket that is refused tells the client only that it failed, so the
// server is asked why over plain HTTP, and the answer is put in words for
// the owner.
async function whyRefused(address: URL, headers: Record<string, string> | undefined, signal: AbortSignal | undefined): Promise<ConnectError> {
  let status: number
  try {
    const response = await fetch(new URL(PATHS.check, address), { headers: headers ?? {}, ...(signal && { signal }) })
    status = response.status
  } catch {
    return signal?.aborted ? noAnswer(address) : noServer(address)
  }
  // The token is accepted, yet the WebSocket was refused: it was the page
  // that was refused. A browser's request to its own page's server carries
  // no Origin, so the check lets it in while the WebSocket, which always
  // carries one, is turned away.
  if (status === 200) return refusal(address, 403, headers !== undefined)
  return refusal(address, status, headers !== undefined)
}

// Nothing answers at the address.
export const noServer = (address: URL) => new ConnectError(`No evie server is running at ${address.origin}. Start it with \`evie serve\`.`, "no server")

// Why the server refused a request, from the status it answered with, in
// words for the owner. `withToken` says whether the request brought a token.
export function refusal(address: URL, status: number, withToken: boolean): ConnectError {
  const where = address.origin
  if (status === 401 && !withToken) return new ConnectError(`The evie server at ${where} asks for a token, and none was given.`, "token")
  if (status === 401) {
    return new ConnectError(
      `The evie server at ${where} did not accept the token. If it was taken back, make a new one on the server's ` +
        `computer with \`evie token add <name>\`.`,
      "token",
    )
  }
  if (status === 403) {
    return new ConnectError(`The evie server at ${where} does not let this page connect. Add its address to "origins" under "server" in evie.json.`, "page")
  }
  if (status === 503) return new ConnectError(`The evie server at ${where} could not check the token just now. Try again.`, "other")
  return new ConnectError(`Could not connect to the evie server at ${where} (it answered ${status}).`, "other")
}

// Something took the connection but did not answer in time.
export const noAnswer = (address: URL) =>
  new ConnectError(`No evie server answered at ${address.origin}. Check the address, and that \`evie serve\` is running there.`, "no answer")
