import type { ServerWebSocket } from "bun"
import type { Evie } from "../core/types.ts"
import type { Tokens } from "./tokens.ts"
import { PATHS, type ClientMessage, type ServerMessage } from "./wire.ts"

// Evie's server. It is a client of the core, as the terminal was: it uses
// only `send` and `subscribe`, and carries them over one WebSocket to each
// client that connects. Commands come up, replies and events go down.
export type ServerOptions = {
  // The core. The server never closes it; whoever started it does.
  evie: Evie
  // The devices' tokens, to check each connection against.
  tokens: Pick<Tokens, "check">
  // Where to listen. Port 0 takes any free port.
  host: string
  port: number
}

export type EvieServer = {
  // The address clients connect to, such as http://127.0.0.1:3843.
  url: string
  // Stops listening and closes every connection. The core keeps running.
  stop(): Promise<void>
}

// What the server keeps for each connection.
type Connection = { unsubscribe: () => void }

export function startServer(options: ServerOptions): EvieServer {
  const server = Bun.serve<Connection>({
    hostname: options.host,
    port: options.port,
    fetch(request, server) {
      const path = new URL(request.url).pathname
      if (path === PATHS.socket) {
        if (server.upgrade(request, { data: { unsubscribe: () => {} } })) return undefined
        return new Response("Connect here with a WebSocket.", { status: 400 })
      }
      return new Response("Not found.", { status: 404 })
    },
    websocket: {
      // Every connection is sent every event of every session, as the
      // terminal was when it ran the core itself.
      open(socket) {
        socket.data.unsubscribe = options.evie.subscribe((event) => send(socket, { event }))
      },
      async message(socket, text) {
        const message = JSON.parse(String(text)) as ClientMessage
        try {
          send(socket, { reply: message.id, result: await options.evie.send(message.command) })
        } catch (cause) {
          send(socket, { reply: message.id, error: cause instanceof Error ? cause.message : String(cause) })
        }
      },
      close(socket) {
        socket.data.unsubscribe()
      },
    },
  })

  return {
    url: `http://${options.host}:${server.port}`,
    async stop() {
      await server.stop(true)
    },
  }
}

const send = (socket: ServerWebSocket<Connection>, message: ServerMessage) => socket.send(JSON.stringify(message))
