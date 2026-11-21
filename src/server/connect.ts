import type { Command, Commands, Evie, EvieEvent } from "../core/types.ts"
import { PATHS, type ClientMessage, type ServerMessage } from "./wire.ts"

export type ConnectOptions = {
  // The server's address, such as http://127.0.0.1:3843.
  url: string
  // This device's token.
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
  const socket =
    options.token === undefined ? new WebSocket(socketAddress) : new WebSocket(socketAddress, { headers: { Authorization: `Bearer ${options.token}` } })
  await new Promise<void>((resolve) => socket.addEventListener("open", () => resolve(), { once: true }))

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
