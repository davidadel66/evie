import type { Command, Commands, Evie, EvieEvent } from "../core/types.ts"
import { PATHS, type ClientMessage, type ServerMessage } from "./wire.ts"

export type ConnectOptions = {
  // The server's address, such as http://127.0.0.1:3843.
  url: string
  // This device's token. Left out in a browser, which cannot set headers on
  // a WebSocket and sends instead the cookie it was given at login.
  token?: string
  // Called when the connection ends without `close` being called, such as
  // when the server stops, with why.
  onDrop?: (reason: string) => void
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

// Connects to Evie's server and returns the same client interface the core
// gives, `send`, `subscribe` and `close`, carried over one WebSocket. A
// client written for the core works over it unchanged. Closing it closes
// the connection only: the server, and anything running in it, carry on.
// When the connection is refused, or the signal fires before it opens, it
// rejects with a `ConnectError`.
export async function connect(options: ConnectOptions): Promise<Evie> {
  const address = new URL(options.url)
  const socketAddress = new URL(PATHS.socket, address)
  socketAddress.protocol = address.protocol === "https:" ? "wss:" : "ws:"
  // A browser's WebSocket takes no headers, and has the cookie in place of the token.
  const headers = options.token === undefined ? undefined : { Authorization: `Bearer ${options.token}` }
  const socket = headers === undefined ? new WebSocket(socketAddress) : new WebSocket(socketAddress, { headers })
  const { signal } = options
  let giveUp = () => {}
  const opened = await new Promise<"open" | "refused" | "gave up">((resolve) => {
    giveUp = () => resolve("gave up")
    socket.addEventListener("open", () => resolve("open"), { once: true })
    socket.addEventListener("close", () => resolve("refused"), { once: true })
    if (signal?.aborted) giveUp()
    else signal?.addEventListener("abort", giveUp, { once: true })
  })
  signal?.removeEventListener("abort", giveUp)
  if (opened === "gave up") {
    socket.close()
    throw noAnswer(address)
  }
  if (opened === "refused") throw await whyRefused(address, headers, signal)

  // Commands sent that have not been answered yet, by id.
  const waiting = new Map<string, { resolve(result: unknown): void; reject(error: Error): void }>()
  const listeners = new Set<(event: EvieEvent) => void>()
  let sent = 0
  // Set once the connection has ended: why it did.
  let ended: string | undefined
  let closing = false

  socket.addEventListener("message", (message) => {
    const received = JSON.parse(String(message.data)) as ServerMessage
    if ("event" in received) {
      // As in the core, a listener that throws does not stop the others hearing it.
      for (const listener of [...listeners]) {
        try {
          listener(received.event)
        } catch {}
      }
      return
    }
    const command = waiting.get(received.reply)
    if (!command) return
    waiting.delete(received.reply)
    if ("error" in received) command.reject(new Error(received.error))
    else command.resolve(received.result)
  })

  const closed = new Promise<void>((resolve) =>
    socket.addEventListener("close", (event) => {
      ended = closing ? "the connection was closed" : event.reason || "the server went away"
      for (const command of waiting.values()) command.reject(new Error(`The connection to Evie was lost before it answered: ${ended}.`))
      waiting.clear()
      if (!closing) options.onDrop?.(ended)
      resolve()
    }),
  )

  return {
    send<Sent extends Command>(command: Sent) {
      if (ended !== undefined) return Promise.reject(new Error(`Not connected to Evie: ${ended}.`))
      const id = `c${++sent}`
      const reply = new Promise<Commands[Sent["type"]]["output"]>((resolve, reject) => {
        waiting.set(id, { resolve: resolve as (result: unknown) => void, reject })
      })
      socket.send(JSON.stringify({ id, command } satisfies ClientMessage))
      return reply
    },
    subscribe(listener) {
      listeners.add(listener)
      return () => {
        listeners.delete(listener)
      }
    },
    async close() {
      closing = true
      if (ended === undefined) socket.close()
      await closed
    },
  }
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
