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
  // Web pages, besides Evie's own address, that may connect, such as
  // http://localhost:5173. Any other page a browser has open is refused.
  origins?: readonly string[]
  // How often, in milliseconds, open connections are checked against the
  // tokens, so that one whose token was taken back is closed even while it
  // sends nothing. A token is taken back by `evie token remove`, in another
  // process, so the server cannot be told; it looks.
  recheckEvery?: number
}

export type EvieServer = {
  // The address clients connect to, such as http://127.0.0.1:3843.
  url: string
  // Stops listening and closes every connection. The core keeps running.
  stop(): Promise<void>
}

// What the server keeps for each connection: the token it was opened with
// and the name of its device.
type Connection = { token: string; device: string; unsubscribe: () => void }

// The close code for a connection whose token was taken back. Codes from
// 4000 are the application's own.
const TAKEN_BACK = 4001

export function startServer(options: ServerOptions): EvieServer {
  const connections = new Set<ServerWebSocket<Connection>>()
  // Closes the connection if its token has been taken back, and says whether it is still let in.
  const stillLetIn = (socket: ServerWebSocket<Connection>) => {
    if (options.tokens.check(socket.data.token) !== undefined) return true
    socket.close(TAKEN_BACK, "This device's token was taken back")
    return false
  }
  const listed = (options.origins ?? []).map((origin) => new URL(origin).origin)

  // Who is asking: the device whose token the request carries, or the
  // answer that refuses it.
  //
  // Any web page open in a browser can try to reach a server on the same
  // computer, and the browser sends it the cookie too. But a browser always
  // says which page is asking, in `Origin`, so a page other than Evie's own
  // or one the owner listed is refused. A client that is not a browser sends
  // no `Origin`, and needs only its token.
  function admit(request: Request, port: number): { token: string; device: string } | Response {
    const origin = request.headers.get("origin")
    const own = [`http://127.0.0.1:${port}`, `http://localhost:${port}`, `http://${options.host}:${port}`]
    if (origin !== null && !own.includes(origin) && !listed.includes(origin)) {
      return new Response("Connections from this page are not allowed.", { status: 403 })
    }
    const token = /^Bearer\s+(\S+)$/i.exec(request.headers.get("authorization") ?? "")?.[1]
    const device = token === undefined ? undefined : options.tokens.check(token)
    if (token === undefined || device === undefined) return new Response("This needs a token that Evie accepts.", { status: 401 })
    return { token, device }
  }

  const server = Bun.serve<Connection>({
    hostname: options.host,
    port: options.port,
    fetch(request, server) {
      const path = new URL(request.url).pathname
      if (path === PATHS.socket) {
        // Checked before the WebSocket opens, so a connection that is
        // refused never reaches the core.
        const admitted = admit(request, server.port!)
        if (admitted instanceof Response) return admitted
        if (server.upgrade(request, { data: { ...admitted, unsubscribe: () => {} } })) return undefined
        return new Response("Connect here with a WebSocket.", { status: 400 })
      }
      if (path === PATHS.check) {
        const admitted = admit(request, server.port!)
        return admitted instanceof Response ? admitted : new Response("This token is accepted.")
      }
      return new Response("Not found.", { status: 404 })
    },
    websocket: {
      // Every connection is sent every event of every session, as the
      // terminal was when it ran the core itself.
      open(socket) {
        connections.add(socket)
        socket.data.unsubscribe = options.evie.subscribe((event) => send(socket, { event }))
      },
      async message(socket, text) {
        // A command is carried out only while its device is still let in.
        if (!stillLetIn(socket)) return
        const message = JSON.parse(String(text)) as ClientMessage
        try {
          send(socket, { reply: message.id, result: await options.evie.send(message.command) })
        } catch (cause) {
          send(socket, { reply: message.id, error: cause instanceof Error ? cause.message : String(cause) })
        }
      },
      close(socket) {
        connections.delete(socket)
        socket.data.unsubscribe()
      },
    },
  })

  const recheck = setInterval(() => {
    for (const socket of connections) stillLetIn(socket)
  }, options.recheckEvery ?? 3000)

  return {
    url: `http://${options.host}:${server.port}`,
    async stop() {
      clearInterval(recheck)
      await server.stop(true)
    },
  }
}

const send = (socket: ServerWebSocket<Connection>, message: ServerMessage) => socket.send(JSON.stringify(message))
