import type { Command, Commands, Evie, EvieEvent } from "../core/types.ts"
import { PATHS, type ClientMessage, type ServerMessage } from "./wire.ts"

export type ConnectOptions = {
  // The server's address, such as http://127.0.0.1:3843.
  url: string
  // This device's token.
  token?: string
}

// Connects to Evie's server and returns the same client interface the core
// gives, `send`, `subscribe` and `close`, carried over one WebSocket. A
// client written for the core works over it unchanged.
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

  socket.addEventListener("message", (message) => {
    const received = JSON.parse(String(message.data)) as ServerMessage
    if ("event" in received) {
      for (const listener of [...listeners]) listener(received.event)
      return
    }
    const command = waiting.get(received.reply)
    if (!command) return
    waiting.delete(received.reply)
    if ("error" in received) command.reject(new Error(received.error))
    else command.resolve(received.result)
  })

  return {
    send<Sent extends Command>(command: Sent) {
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
      socket.close()
    },
  }
}
