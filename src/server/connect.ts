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
}

// Connects to Evie's server and returns the same client interface the core
// gives, `send`, `subscribe` and `close`, carried over one WebSocket. A
// client written for the core works over it unchanged. Closing it closes
// the connection only: the server, and anything running in it, carry on.
export async function connect(options: ConnectOptions): Promise<Evie> {
  const address = new URL(options.url)
  const socketAddress = new URL(PATHS.socket, address)
  socketAddress.protocol = address.protocol === "https:" ? "wss:" : "ws:"
  // A browser's WebSocket takes no headers, and has the cookie in place of the token.
  const headers = options.token === undefined ? undefined : { Authorization: `Bearer ${options.token}` }
  const socket = headers === undefined ? new WebSocket(socketAddress) : new WebSocket(socketAddress, { headers })
  const opened = await new Promise<boolean>((resolve) => {
    socket.addEventListener("open", () => resolve(true), { once: true })
    socket.addEventListener("close", () => resolve(false), { once: true })
  })
  if (!opened) throw new Error(await whyRefused(address, headers))

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
async function whyRefused(address: URL, headers: Record<string, string> | undefined): Promise<string> {
  const where = address.origin
  let status: number
  try {
    const response = await fetch(new URL(PATHS.check, address), {
      headers: headers ?? {},
      signal: AbortSignal.timeout(5000),
    })
    status = response.status
  } catch {
    return `No Evie server is running at ${where}. Start it with \`evie serve\`.`
  }
  if (status === 401 && headers === undefined) return `The Evie server at ${where} asks for a token, and none was given.`
  if (status === 401) {
    return (
      `The Evie server at ${where} did not accept the token. If it was taken back, make a new one on the server's ` +
      `computer with \`evie token add <name>\`; this computer's own token is made again when \`evie serve\` restarts.`
    )
  }
  if (status === 403) return `The Evie server at ${where} does not let this page connect. Add its address to "origins" under "server" in evie.json.`
  return `Could not connect to the Evie server at ${where} (it answered ${status}).`
}
