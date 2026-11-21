import type { ServerWebSocket } from "bun"
import type { Commands, Evie, EvieEvent } from "../core/types.ts"
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

// What the server keeps for each connection: the token it was opened with,
// the name of its device, the sessions it has opened, and how many sessions
// it is opening right now.
type Connection = { token: string; device: string; unsubscribe: () => void; sessions: Set<string>; opening: number }

// The close code for a connection whose token was taken back. Codes from
// 4000 are the application's own.
const TAKEN_BACK = 4001
// The browser's cookie that holds its token.
const COOKIE = "evie_token"

export function startServer(options: ServerOptions): EvieServer {
  const connections = new Set<ServerWebSocket<Connection>>()
  // Every session a client has opened through this server, whether or not
  // that client is still connected.
  const openedHere = new Set<string>()

  // Which events a connection is sent: those of the sessions it has opened.
  // A session can ask a question before it is open, such as whether to trust
  // its project, so while a connection is opening one it is also sent the
  // questions and answers of sessions that no client has opened here yet.
  // Two clients opening sessions at the same moment can each be sent the
  // other's opening question. That is accepted: it needs both to be opening
  // at once, and a question from a session already open never goes astray.
  function sentTo(connection: Connection, event: EvieEvent): boolean {
    if (connection.sessions.has(event.session)) return true
    return connection.opening > 0 && (event.type === "question" || event.type === "answer") && !openedHere.has(event.session)
  }
  // Closes the connection if its token has been taken back, and says whether it is still let in.
  const stillLetIn = (socket: ServerWebSocket<Connection>) => {
    if (options.tokens.check(socket.data.token) !== undefined) return true
    socket.close(TAKEN_BACK, "This device's token was taken back")
    return false
  }
  const listed = (options.origins ?? []).map((origin) => new URL(origin).origin)

  // Any web page open in a browser can try to reach a server on the same
  // computer, and the browser sends Evie's cookie with it. But a browser
  // always says which page is asking, in `Origin`, so a page other than
  // Evie's own or one the owner listed is refused. A client that is not a
  // browser sends no `Origin`, and needs only its token.
  function refusedPage(request: Request, port: number): Response | undefined {
    const origin = request.headers.get("origin")
    const own = [`http://127.0.0.1:${port}`, `http://localhost:${port}`, `http://${options.host}:${port}`]
    if (origin === null || own.includes(origin) || listed.includes(origin)) return undefined
    return new Response("Connections from this page are not allowed.", { status: 403 })
  }

  // The device a token belongs to, or the answer that refuses it.
  function deviceFor(token: string | undefined): { token: string; device: string } | Response {
    const device = token === undefined ? undefined : options.tokens.check(token)
    if (token === undefined || device === undefined) return new Response("This needs a token that Evie accepts.", { status: 401 })
    return { token, device }
  }

  // Who is asking: the device whose token the request carries, in the
  // `Authorization` header or the browser's cookie, or the answer that
  // refuses it.
  function admit(request: Request, port: number): { token: string; device: string } | Response {
    const bearer = /^Bearer\s+(\S+)$/i.exec(request.headers.get("authorization") ?? "")?.[1]
    const cookie = new Bun.CookieMap(request.headers.get("cookie") ?? "").get(COOKIE) ?? undefined
    return refusedPage(request, port) ?? deviceFor(bearer ?? cookie)
  }

  // A browser trades its token for a cookie. The cookie holds the token
  // itself, so taking the token back ends the cookie too. The page's scripts
  // cannot read it (HttpOnly), and the browser sends it only with requests
  // that Evie's own pages make (SameSite=Strict), and over HTTPS only when
  // Evie is reached over HTTPS (Secure).
  async function login(request: Request, port: number): Promise<Response> {
    const refused = refusedPage(request, port)
    if (refused) return refused
    const body = (await request.json().catch(() => undefined)) as { token?: unknown } | undefined
    const admitted = deviceFor(typeof body?.token === "string" ? body.token : undefined)
    if (admitted instanceof Response) return admitted
    const secure = new URL(request.url).protocol === "https:" ? "; Secure" : ""
    return new Response(null, { status: 204, headers: { "Set-Cookie": `${COOKIE}=${admitted.token}; HttpOnly; SameSite=Strict; Path=/${secure}` } })
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
        if (server.upgrade(request, { data: { ...admitted, unsubscribe: () => {}, sessions: new Set(), opening: 0 } })) return undefined
        return new Response("Connect here with a WebSocket.", { status: 400 })
      }
      if (path === PATHS.check) {
        const admitted = admit(request, server.port!)
        return admitted instanceof Response ? admitted : new Response("This token is accepted.")
      }
      if (path === PATHS.login && request.method === "POST") return login(request, server.port!)
      return new Response("Not found.", { status: 404 })
    },
    websocket: {
      open(socket) {
        connections.add(socket)
        socket.data.unsubscribe = options.evie.subscribe((event) => {
          if (sentTo(socket.data, event)) send(socket, { event })
        })
      },
      async message(socket, text) {
        // A command is carried out only while its device is still let in.
        if (!stillLetIn(socket)) return
        const message = parse(String(text))
        // A client that sends something else has a bug. Its connection is
        // closed, saying why, and nothing else is touched.
        if (!message) return socket.close(1007, 'Each message must be JSON of the form { "id": ..., "command": ... }')
        const opening = message.command.type === "open_session"
        if (opening) socket.data.opening += 1
        try {
          const result = await options.evie.send(message.command)
          // The session opened is learned from the reply, before the reply is
          // sent, so the client is sent its events from then on.
          if (opening) {
            const { session } = result as Commands["open_session"]["output"]
            socket.data.sessions.add(session)
            openedHere.add(session)
          }
          send(socket, { reply: message.id, result })
        } catch (cause) {
          send(socket, { reply: message.id, error: cause instanceof Error ? cause.message : String(cause) })
        } finally {
          if (opening) socket.data.opening -= 1
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

// A message from a client, when it has the shape of one. Whether the command
// is one the core knows is for the core to say, in its reply.
function parse(text: string): ClientMessage | undefined {
  try {
    const message = JSON.parse(text) as Partial<ClientMessage> | null
    return typeof message?.id === "string" && typeof message.command === "object" && message.command !== null ? (message as ClientMessage) : undefined
  } catch {
    return undefined
  }
}
