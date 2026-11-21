import type { ServerWebSocket } from "bun"
import type { Commands, Evie, EvieEvent } from "../core/types.ts"
import type { Tokens } from "./tokens.ts"
import { hostInAddress, PATHS, TAKEN_BACK, type ClientMessage, type ServerMessage } from "./wire.ts"

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
  // process, so the server cannot be told; it looks. Replies kept for
  // commands sent again are looked over at the same time, and forgotten
  // ten minutes after they were given.
  recheckEvery?: number
}

export type EvieServer = {
  // The address clients connect to, such as http://127.0.0.1:3843.
  url: string
  // Stops listening and closes every connection. The core keeps running.
  stop(): Promise<void>
}

// A device that is let in: the token it brought, and the device's name.
type Admitted = { token: string; device: string }

// What the server keeps for each connection: who it is, the sessions it has
// opened, how many sessions it is opening right now, the sessions it is
// opening by their id right now, and the questions it was sent while
// opening that are still waiting, by id, with their session.
type Connection = Admitted & {
  unsubscribe: () => void
  sessions: Set<string>
  opening: number
  joining: Set<string>
  openingQuestions: Map<string, string>
}

// The browser's cookie that holds its token.
const COOKIE = "evie_token"
// How long the reply to a command is kept after it was given, to answer the
// same command sent again after the line dropped.
const REMEMBER_FOR = 10 * 60_000

export function startServer(options: ServerOptions): EvieServer {
  const connections = new Set<ServerWebSocket<Connection>>()
  // Every session a client has opened through this server, whether or not
  // that client is still connected.
  const openedHere = new Set<string>()
  // The reply to each command a device has sent, by the command's id, with
  // when it was given. A client whose line dropped before the reply reached
  // it sends the command again with the same id, and is answered from here,
  // or waits for the first one to finish, so it is carried out once.
  const replies = new Map<string, Map<string, { reply: Promise<ServerMessage>; given?: number }>>()

  function replyTo(device: string, message: ClientMessage): Promise<ServerMessage> {
    let sent = replies.get(device)
    if (!sent) replies.set(device, (sent = new Map()))
    const remembered = sent.get(message.id)
    if (remembered) return remembered.reply
    const entry: { reply: Promise<ServerMessage>; given?: number } = {
      reply: options.evie.send(message.command).then(
        (result): ServerMessage => ({ reply: message.id, result }),
        (cause): ServerMessage => ({ reply: message.id, error: cause instanceof Error ? cause.message : String(cause) }),
      ),
    }
    void entry.reply.then(() => (entry.given = Date.now()))
    sent.set(message.id, entry)
    return entry.reply
  }

  // Replies kept longer than they are needed are forgotten.
  function forgetOldReplies() {
    const now = Date.now()
    for (const [device, sent] of replies) {
      for (const [id, { given }] of sent) if (given !== undefined && now - given > REMEMBER_FOR) sent.delete(id)
      if (sent.size === 0) replies.delete(device)
    }
  }

  // Which events a connection is sent: those of the sessions it has opened.
  // A session can ask a question before it is open, such as whether to trust
  // its project, so while a connection is opening one it is also sent the
  // questions and answers of sessions that no client has opened here yet.
  // Two clients opening sessions at the same moment can each be sent the
  // other's opening question. That is accepted: it needs both to be opening
  // at once, and a question from a session already open never goes astray.
  function sentTo(connection: Connection, event: EvieEvent): boolean {
    if (connection.sessions.has(event.session) || connection.joining.has(event.session)) return true
    return connection.opening > 0 && (event.type === "question" || event.type === "answer") && !openedHere.has(event.session)
  }
  // Closes the connection if its token has been taken back, and says whether
  // it did. A check that fails, such as when the database stays busy past
  // its wait, cannot say the token was taken back: the connection is kept,
  // and checked again next time.
  const closeIfTakenBack = (socket: ServerWebSocket<Connection>) => {
    let device: string | undefined
    try {
      device = options.tokens.check(socket.data.token)
    } catch {
      return false
    }
    if (device !== undefined) return false
    socket.close(TAKEN_BACK, "This device's token was taken back")
    return true
  }
  // An address without http:// or https://, such as "localhost:5173", has
  // the origin "null", which is also what a browser sends from a page that
  // has no address of its own, such as a file. It is never let in.
  const allowedOrigins = (options.origins ?? []).map((origin) => new URL(origin).origin).filter((origin) => origin !== "null")

  // Any web page open in a browser can try to reach a server on the same
  // computer, and the browser sends Evie's cookie with it. But a browser
  // always says which page is asking, in `Origin`, so a page other than
  // Evie's own or one the owner listed is refused. A client that is not a
  // browser sends no `Origin`, and needs only its token.
  function refusedPage(request: Request, port: number): Response | undefined {
    const origin = request.headers.get("origin")
    const own = [`http://127.0.0.1:${port}`, `http://localhost:${port}`, `http://${hostInAddress(options.host)}:${port}`]
    if (origin === null || own.includes(origin) || allowedOrigins.includes(origin)) return undefined
    return new Response("Connections from this page are not allowed.", { status: 403 })
  }

  // The device a token belongs to, or the answer that refuses it. A check
  // that fails, such as when the database stays busy past its wait, says
  // neither yes nor no, so the client is told to try again.
  function deviceFor(token: string | undefined): Admitted | Response {
    let device: string | undefined
    try {
      device = token === undefined ? undefined : options.tokens.check(token)
    } catch {
      return new Response("The token could not be checked just now. Try again.", { status: 503 })
    }
    if (token === undefined || device === undefined) return new Response("This needs a token that Evie accepts.", { status: 401 })
    return { token, device }
  }

  // Who is asking: the device whose token the request carries, in the
  // `Authorization` header or the browser's cookie, or the answer that
  // refuses it.
  function admit(request: Request, port: number): Admitted | Response {
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
        if (server.upgrade(request, { data: { ...admitted, unsubscribe: () => {}, sessions: new Set(), opening: 0, joining: new Set(), openingQuestions: new Map() } })) return undefined
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
          if (!sentTo(socket.data, event)) return
          // A question sent while opening, from a session not yet opened here,
          // is kept track of until it is answered: see `close`.
          if (event.type === "question" && !socket.data.sessions.has(event.session)) socket.data.openingQuestions.set(event.body.id, event.session)
          if (event.type === "answer") socket.data.openingQuestions.delete(event.body.id)
          send(socket, { event })
        })
      },
      async message(socket, text) {
        // A command is carried out only while its device is still let in.
        if (closeIfTakenBack(socket)) return
        const message = parse(String(text))
        // A client that sends something else has a bug. Its connection is
        // closed, saying why, and nothing else is touched.
        if (!message) return socket.close(1007, 'Each message must be JSON of the form { "id": ..., "command": ... }')
        const opening = message.command.type === "open_session"
        if (opening) socket.data.opening += 1
        // A session opened by its id, as a client catching up does, is sent
        // to this connection from now, before the core reads where it
        // stands, so that every event after that reading reaches the client.
        // A session already open is read in the same step it is handed the
        // command. One opened again is read once it is open, so the events
        // of its opening are sent both as they happen and in the reply, and
        // the client skips the second by their numbers.
        const joining = message.command.type === "open_session" ? message.command.session : undefined
        if (joining !== undefined) socket.data.joining.add(joining)
        try {
          const reply = await replyTo(socket.data.device, message)
          // The session opened is learned from the reply, before the reply is
          // sent, so the client is sent its events from then on.
          if (opening && "result" in reply) {
            const { session } = reply.result as Commands["open_session"]["output"]
            socket.data.sessions.add(session)
            openedHere.add(session)
          }
          send(socket, reply)
        } finally {
          if (opening) socket.data.opening -= 1
          if (joining !== undefined) socket.data.joining.delete(joining)
        }
      },
      close(socket) {
        connections.delete(socket)
        socket.data.unsubscribe()
        // Closing a client stops nothing, with one exception. A question asked
        // while a session is still opening, such as whether to trust its
        // project, belongs to a session no other client can reach, and the
        // project's setup waits on it, so every later session in that project
        // would wait too. It is refused, which counts as a no for now.
        for (const [id, session] of socket.data.openingQuestions) {
          if (!openedHere.has(session)) options.evie.send({ type: "answer_question", session, id, choice: null }).catch(() => {})
        }
      },
    },
  })

  const recheck = setInterval(() => {
    for (const socket of connections) closeIfTakenBack(socket)
    forgetOldReplies()
  }, options.recheckEvery ?? 3000)

  return {
    url: `http://${hostInAddress(options.host)}:${server.port}`,
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
