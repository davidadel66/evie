import type { Command, Commands, Evie, EvieEvent, StoredEvent } from "../core/types.ts"
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
  // time, up to ten seconds, until the line is back or `close` is called.
  // Commands sent meanwhile wait for the line.
  onDrop?: (reason: string) => void
  // Called when the line is back after a drop. What was missed while it was
  // down is passed on straight after.
  onBack?: () => void
  // Called when the client stops for good without `close` being called,
  // because the server took this device's token back, or refuses it or the
  // page, with why. Commands still waiting fail, and so does any sent after.
  onEnd?: (cause: ConnectError) => void
  // Gives up connecting when it fires, such as at a time limit. It is used
  // only while connecting: once the connection is open it changes nothing.
  signal?: AbortSignal
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

// A command waiting for its reply. The owner's are sent again on each new
// line until they are answered. The client's own, to catch up on a session
// (`own`), are sent once: when their line drops they are given up on, and
// the client catches up afresh on the next line.
type Waiting = {
  message: ClientMessage
  own: boolean
  // The line it was last sent on, and whether it was sent on an earlier one too.
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
  // For a session the owner opened: whether it was opened as one that
  // shows questions, so that it is opened the same way after a drop.
  openAs?: { questions: boolean }
  // While catching up on the session: its live events, held back until
  // what was missed has been passed on.
  held: EvieEvent[] | undefined
  // Questions and answers passed on while the session's events were held
  // back, which were not held because opening the session can wait on one.
  early: Set<number>
}

// How long to wait before trying again after the line drops: one second,
// then twice as long each time, up to ten seconds.
const FIRST_WAIT = 1000
const LONGEST_WAIT = 10_000
// How long a try to connect again may take before it is given up on, for a
// server that takes the connection but never answers.
const CONNECT_TIMEOUT = 10_000

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
// the same id, which the server answers without carrying it out twice.
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
    if (!seen) sessions.set(session, (seen = { seq: 0, shown: "", held: undefined, early: new Set() }))
    return seen
  }
  const isNew = (seen: Seen, event: StoredEvent) => event.seq > seen.seq && !seen.early.has(event.seq)

  // Passes an event on to the owner, once: a stored event whose number has
  // been passed on already is skipped.
  function passOn(event: EvieEvent) {
    const seen = seenOf(event.session)
    if ("seq" in event) {
      if (!isNew(seen, event)) return
      seen.seq = event.seq
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

  // A live event from the server.
  function onEvent(event: EvieEvent) {
    const seen = sessions.get(event.session)
    if (!seen?.held) return passOn(event)
    // A question is passed on at once: opening a session again can wait on
    // one, such as whether to trust its project.
    if (event.type === "question" || event.type === "answer") {
      if (isNew(seen, event)) {
        seen.early.add(event.seq)
        deliver(event)
      }
      return
    }
    seen.held.push(event)
  }

  // The reply to a session the owner opened. Its events are sent from the
  // moment the reply describes, so the number in it is where the client stands.
  function onOpened(command: Waiting, output: Opened) {
    const input = command.message.command as OpenInput
    const seen = seenOf(output.session)
    seen.seq = Math.max(seen.seq, output.lastSeq)
    if (seen.openAs) seen.openAs.questions ||= input.questions === true
    else {
      seen.openAs = { questions: input.questions === true }
      // The owner is given the reply so far with the reply, and shows it.
      seen.shown = output.replySoFar ?? ""
    }
    // A reply to a command sent again can describe a moment before the
    // line dropped, so the session is caught up on from there.
    if (command.resent && socket) void catchUp(output.session, socket)
  }

  // Asks the server for what was missed in a session since the last event
  // passed on, and passes it on, then the live events held back meanwhile.
  // When the line drops first, it passes nothing on: the client catches up
  // afresh on the next line.
  async function catchUp(session: string, line: WebSocket): Promise<void> {
    const seen = seenOf(session)
    seen.held = []
    const reply = await ask(line, {
      type: "open_session",
      session,
      after: seen.seq,
      ...(seen.openAs?.questions && { questions: true }),
    })
    if (!reply) return
    const held = seen.held ?? []
    seen.held = undefined
    // A session that could not be opened again, as when its project's
    // plugins fail to load, is left as it is: a command sent to it says why.
    if ("result" in reply) {
      const output = reply.result as Opened
      for (const event of output.events ?? []) {
        // A client of the core always sees a reply's text as pieces before
        // the stored message, so the part of a reply that was missed is
        // passed on as one piece before it.
        if (event.type === "assistant_message" && isNew(seen, event)) passOnText(session, event.body.text)
        passOn(event)
      }
      passOnText(session, output.replySoFar ?? "")
      seen.seq = Math.max(seen.seq, output.lastSeq)
    }
    for (const event of held) passOn(event)
    for (const seq of seen.early) seen.seq = Math.max(seen.seq, seq)
    seen.early.clear()
  }

  // Sends a command of the client's own on this line. It resolves with the
  // server's reply, or with nothing when the line drops first.
  function ask(line: WebSocket, command: Command): Promise<Reply | undefined> {
    const message: ClientMessage = { id: crypto.randomUUID(), command }
    return new Promise((resolve) => {
      waiting.set(message.id, { message, own: true, sentOn: line, resent: false, answer: resolve, lose: () => resolve(undefined) })
      sendOn(line, message)
    })
  }

  // Sends each of the owner's waiting commands that has not been sent on
  // the line as it is now, in the order they were sent. While rejoining,
  // only answers to questions go: opening a session again can wait on one,
  // and anything else could reach a session before it is open again. The
  // client's own commands are sent once, on their line, and never here.
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

  // A message from the server, on the line it came on.
  function onMessage(line: WebSocket, text: string) {
    if (line !== socket) return
    const received = JSON.parse(text) as ServerMessage
    if ("event" in received) return onEvent(received.event)
    const command = waiting.get(received.reply)
    if (!command) return
    waiting.delete(received.reply)
    command.answer(received)
  }

  function use(line: WebSocket) {
    socket = line
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
    rejoining = false
    // What was held back is in the log, or in the reply so far, and comes
    // again when the client catches up afresh.
    for (const seen of sessions.values()) seen.held = undefined
    for (const [id, command] of waiting) {
      if (!command.own) continue
      waiting.delete(id)
      command.lose(reason)
    }
    options.onDrop?.(reason)
    void tryAgain()
  }

  async function tryAgain() {
    for (let wait = FIRST_WAIT; ; wait = Math.min(wait * 2, LONGEST_WAIT)) {
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
        continue
      }
      if (stopping.signal.aborted) return line.close()
      return back(line)
    }
  }

  async function back(line: WebSocket) {
    use(line)
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
  }

  function end(cause: ConnectError) {
    if (stopping.signal.aborted) return
    stopping.abort(cause.message)
    socket = undefined
    loseAll(cause.message)
    options.onEnd?.(cause)
  }

  use(socket)

  return {
    send<Sent extends Command>(command: Sent) {
      if (stopping.signal.aborted) return Promise.reject(new Error(`Not connected to Evie: ${String(stopping.signal.reason).replace(/\.$/, "")}.`))
      const message: ClientMessage = { id: crypto.randomUUID(), command }
      return new Promise<Commands[Sent["type"]]["output"]>((resolve, reject) => {
        const waited: Waiting = {
          message,
          own: false,
          resent: false,
          answer(reply) {
            if ("error" in reply) return reject(new Error(reply.error))
            if (command.type === "open_session") onOpened(waited, reply.result as Opened)
            resolve(reply.result as Commands[Sent["type"]]["output"])
          },
          lose: (why) => reject(new Error(`The connection to Evie was lost before it answered: ${why.replace(/\.$/, "")}.`)),
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
        stopping.abort("the connection was closed")
        const line = socket
        socket = undefined
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

// Opens a WebSocket to the server, or says why it could not.
async function openSocket(address: URL, headers: Record<string, string> | undefined, signal: AbortSignal | undefined): Promise<WebSocket> {
  const socketAddress = new URL(PATHS.socket, address)
  socketAddress.protocol = address.protocol === "https:" ? "wss:" : "ws:"
  const socket = headers === undefined ? new WebSocket(socketAddress) : new WebSocket(socketAddress, { headers })
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
  const where = address.origin
  let status: number
  try {
    const response = await fetch(new URL(PATHS.check, address), { headers: headers ?? {}, ...(signal && { signal }) })
    status = response.status
  } catch {
    if (signal?.aborted) return noAnswer(address)
    return new ConnectError(`No Evie server is running at ${where}. Start it with \`evie serve\`.`, "no server")
  }
  if (status === 401 && headers === undefined) return new ConnectError(`The Evie server at ${where} asks for a token, and none was given.`, "token")
  if (status === 401) {
    return new ConnectError(
      `The Evie server at ${where} did not accept the token. If it was taken back, make a new one on the server's ` +
        `computer with \`evie token add <name>\`.`,
      "token",
    )
  }
  if (status === 403) {
    return new ConnectError(`The Evie server at ${where} does not let this page connect. Add its address to "origins" under "server" in evie.json.`, "page")
  }
  if (status === 503) return new ConnectError(`The Evie server at ${where} could not check the token just now. Try again.`, "other")
  return new ConnectError(`Could not connect to the Evie server at ${where} (it answered ${status}).`, "other")
}

// Something took the connection but did not answer in time.
const noAnswer = (address: URL) =>
  new ConnectError(`No Evie server answered at ${address.origin}. Check the address, and that \`evie serve\` is running there.`, "no answer")
