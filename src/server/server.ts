import type { HTMLBundle, ServerWebSocket } from "bun"
import { randomBytes } from "node:crypto"
import type { Command, Commands, Evie, EvieEvent } from "../core/types.ts"
import { OWN_NAME, type Tokens } from "./tokens.ts"
import type { WebApp } from "./web-app.ts"
import { hostInAddress, onThisComputer, PATHS, TAKEN_BACK, type ClientMessage, type ServerMessage } from "./wire.ts"

// Evie's server. It is a client of the core, as the terminal was: it uses
// only `send` and `subscribe`, and carries them over one WebSocket to each
// client that connects. Commands come up, replies and events go down.
export type ServerOptions = {
  // The core. The server never closes it; whoever started it does.
  evie: Evie
  // The devices' tokens, to check each connection against, and to make one
  // for each browser that signs in with a sign-in link.
  tokens: Pick<Tokens, "check" | "add">
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
  // How often, in milliseconds, each connection is sent a keepalive, so that
  // its client can tell a quiet line from a dead one. Every 15 seconds when
  // left out, which is for tests to change.
  keepaliveEvery?: number
  // The web app, served to anyone, since the page and its files hold no
  // secrets: what it shows comes over the WebSocket, which needs a token.
  app?: WebApp
  // How long, in milliseconds, the code in a sign-in link works. Five
  // minutes when left out, which is for tests to change.
  signInCodeLasts?: number
  // Whether a connection from this address comes from the computer the
  // server runs on: when it comes from 127.0.0.1 or ::1, when left out,
  // which is for tests to change. A connection from a web page counts only
  // when the page is at a local address too.
  fromThisComputer?: (address: string) => boolean
}

export type EvieServer = {
  // The address clients connect to, such as http://127.0.0.1:3843.
  url: string
  // Stops listening and closes every connection. The core keeps running.
  stop(): Promise<void>
}

// A device that is let in: the token it brought, and the device's name.
type Admitted = { token: string; device: string }

// The settings window's changes that only the computer evie runs on may
// make: a key typed in, so that a key never crosses the network, and the
// plugin files it runs and the projects it trusts. It is a seat belt, not a
// wall: a signed-in device can still ask evie to do anything evie can do,
// and can answer a question such as whether to trust a project, as opening
// a project's session from a phone asks. Any device may change the rest,
// and restart the server.
const ONLY_HERE = ["key", "addPlugin", "removePlugin", "project"]
const changesOnlyHere = (command: Command) => command.type === "change_settings" && ONLY_HERE.some((field) => field in command)
const ONLY_HERE_WORDS = "Keys, plugins and trusted projects can be changed only on the computer evie runs on."

// The addresses of this computer itself, as a connection from it comes
// from: 127.0.0.1, or any of 127.0.0.0/8, or ::1, written either way.
const isLoopback = (address: string) => /^(::ffff:)?127\./.test(address) || address === "::1"

// What the server keeps for each connection: who it is, the sessions it has
// opened, how many sessions it is opening right now, the sessions it is
// opening by their id right now, the questions it was sent while opening
// that are still waiting, by id, with their session, and whether it comes
// from the computer the server runs on.
type Connection = Admitted & {
  here: boolean
  unsubscribe: () => void
  sessions: Set<string>
  opening: number
  joining: Map<string, Joining>
  openingQuestions: Map<string, string>
}

// A session a connection is opening by its id: how many opens of it are
// under way, and its events and summaries, held back until they have all
// been answered.
type Joining = { opens: number; held: EvieEvent[] }

// The reply to a command a device sent, and whether it has settled.
type Remembered = { reply: Promise<ServerMessage>; settled: boolean }

// The browser's cookie that holds its token, named for the server's port,
// and how long the browser keeps it, in seconds: 400 days, the longest
// browsers allow. A browser keeps cookies apart by host but not by port, so
// two servers on one computer, such as the everyday one and a development
// copy, would otherwise each replace the other's cookie and sign the
// browser out of it. Taking the token back still signs the browser out at once.
const cookieName = (port: number) => `evie_token_${port}`
const COOKIE_LASTS = 400 * 24 * 60 * 60
// How many of each device's commands the server remembers the replies to,
// to answer the same command sent again after the line dropped. There is no
// time limit, since a laptop can sleep all night with a command unanswered.
// A device sends far fewer commands than this while its line is down, and
// each reply kept is small.
const REMEMBERED = 200
// How long the code in a sign-in link works: long enough for the browser to
// open, short enough that a link left in a browser's history is of no use.
const SIGN_IN_CODE_LASTS = 5 * 60_000

export function startServer(options: ServerOptions): EvieServer {
  const connections = new Set<ServerWebSocket<Connection>>()
  // Every session a client has opened through this server, whether or not
  // that client is still connected.
  const openedHere = new Set<string>()
  // The reply to each of the last commands a device has sent, by the
  // command's id, oldest first. A client whose line dropped before the reply
  // reached it sends the command again with the same id, and is answered
  // from here, or waits for the first one to finish, so it is carried out once.
  const replies = new Map<string, Map<string, Remembered>>()
  // The code in each sign-in link not yet used, with the time it stops
  // working. It is kept in memory only: a link outlives no restart.
  const codes = new Map<string, number>()

  function replyTo(device: string, message: ClientMessage): Promise<ServerMessage> {
    const carryOut = () =>
      options.evie.send(message.command).then(
        (result): ServerMessage => ({ reply: message.id, result }),
        (cause): ServerMessage => ({ reply: message.id, error: cause instanceof Error ? cause.message : String(cause) }),
      )
    // A command its client will never send again, such as its own catch-up
    // after a drop, whose reply can be large, is not remembered.
    if (message.once === true) return carryOut()
    let sent = replies.get(device)
    if (!sent) replies.set(device, (sent = new Map()))
    const remembered = sent.get(message.id)
    if (remembered) return remembered.reply
    const kept: Remembered = { reply: carryOut(), settled: false }
    void kept.reply.then(() => (kept.settled = true))
    sent.set(message.id, kept)
    // The oldest are forgotten first: a map keeps the order its keys were
    // added in. One still being carried out is kept however old it is, so
    // that the same command sent again waits for it, not carried out twice.
    for (const [id, old] of sent) {
      if (sent.size <= REMEMBERED) break
      if (old.settled) sent.delete(id)
    }
    return kept.reply
  }

  // Which events a connection is sent, summaries aside: those of the
  // sessions it has opened.
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
  // Holds back a connection's events for a session it is opening by its id,
  // and its summaries with them, and gives back what sends them, to be
  // called once the reply has been sent. When the connection opens the
  // session again before the first open is answered, they are held until the
  // last reply is sent, since the client can still be catching up from that
  // one. When no open of it succeeded, and the connection does not have it
  // open from before, its events are dropped: the connection is sent nothing
  // more of that session but its summaries, which go to every connection.
  function holdBack(socket: ServerWebSocket<Connection>, session: string): () => void {
    const joining = socket.data.joining.get(session) ?? { opens: 0, held: [] }
    socket.data.joining.set(session, joining)
    joining.opens += 1
    return () => {
      joining.opens -= 1
      if (joining.opens > 0) return
      socket.data.joining.delete(session)
      const opened = socket.data.sessions.has(session)
      for (const event of joining.held) if (opened || event.type === "summary") send(socket, { event })
    }
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
    // None of its commands can be sent again, so their replies are forgotten.
    replies.delete(socket.data.device)
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
    return refusedPage(request, port) ?? deviceFor(bearerIn(request) ?? cookieIn(request, port))
  }

  // This computer asks for a link that signs a browser in, as `evie web`
  // does, with its own token: a browser's cookie, or another device's token,
  // could otherwise make tokens for browsers anywhere. The link opens the
  // web app at the address this computer reached the server at, with a code
  // that works once, for a few minutes. The code is in the address's
  // fragment, after #, which a browser never sends to a server, and the page
  // wipes it from the address bar.
  function signInLink(request: Request, port: number): Response {
    const admitted = refusedPage(request, port) ?? deviceFor(bearerIn(request))
    if (admitted instanceof Response) return admitted
    if (admitted.device !== OWN_NAME) {
      return new Response(
        "Only this computer's own token can ask for a sign-in link. On another device, open evie's address in a browser and paste a token made for that device.",
        { status: 403 },
      )
    }
    const now = Date.now()
    for (const [code, until] of codes) if (until <= now) codes.delete(code)
    const code = randomBytes(32).toString("base64url")
    codes.set(code, now + (options.signInCodeLasts ?? SIGN_IN_CODE_LASTS))
    const link = new URL(PATHS.signIn, request.url)
    link.hash = `code=${code}`
    // A server listening on every address, as 0.0.0.0, is reached at it by
    // this computer, but a browser does not open it; this computer's own
    // address is the same server.
    if (WILDCARDS.includes(link.hostname)) link.hostname = "127.0.0.1"
    return Response.json({ link: link.href })
  }

  // A browser that brings the code of a sign-in link is given a token of its
  // own, named for the day, so that each browser can be taken back alone
  // with `evie token remove`. The token is made only now, so a link never
  // used leaves no token behind. The code is used up only once the token is
  // made: a token that cannot be made just now leaves it to be tried again.
  // A browser signed in already, as when `evie web` is run again, keeps the
  // token it has, so that each browser has one; when its token cannot be
  // checked just now, it is told to try again, not made another.
  function signedInBrowser(code: string, cookie: string | undefined): Admitted | Response {
    const until = codes.get(code)
    if (until === undefined || until <= Date.now()) {
      codes.delete(code)
      return new Response("This sign-in link has been used already or has expired. Run `evie web` for a new one.", { status: 401 })
    }
    const already = cookie === undefined ? undefined : deviceFor(cookie)
    if (already instanceof Response && already.status === 503) return already
    if (already !== undefined && !(already instanceof Response)) {
      codes.delete(code)
      return already
    }
    const day = localDay()
    try {
      for (let count = 1; ; count++) {
        const device = count === 1 ? `browser ${day}` : `browser ${day} (${count})`
        const made = options.tokens.add(device)
        if ("refused" in made) continue
        codes.delete(code)
        return { token: made.secret, device }
      }
    } catch {
      return new Response("The browser's token could not be made just now. Try again.", { status: 503 })
    }
  }

  // A browser trades its token, or the code of a sign-in link, for a cookie.
  // The cookie holds the token itself, so taking the token back ends the
  // cookie too. The page's scripts cannot read it (HttpOnly), and the
  // browser sends it only with requests that Evie's own pages make
  // (SameSite=Strict), and over HTTPS only when Evie is reached over HTTPS
  // (Secure).
  async function login(request: Request, port: number): Promise<Response> {
    const refused = refusedPage(request, port)
    if (refused) return refused
    const body = (await request.json().catch(() => undefined)) as { token?: unknown; code?: unknown } | undefined
    const admitted =
      typeof body?.code === "string" ? signedInBrowser(body.code, cookieIn(request, port)) : deviceFor(typeof body?.token === "string" ? body.token : undefined)
    if (admitted instanceof Response) return admitted
    const secure = new URL(request.url).protocol === "https:" ? "; Secure" : ""
    return new Response(null, {
      status: 204,
      headers: { "Set-Cookie": `${cookieName(port)}=${admitted.token}; Max-Age=${COOKIE_LASTS}; HttpOnly; SameSite=Strict; Path=/${secure}` },
    })
  }

  const server = Bun.serve<Connection>({
    hostname: options.host,
    port: options.port,
    routes: appRoutes(options.app),
    // Only while working on the web app, which Bun then bundles on each
    // request. Otherwise an error is answered without the details of Evie's code.
    development: options.app !== undefined && !("files" in options.app),
    fetch(request, server) {
      const path = new URL(request.url).pathname
      if (path === PATHS.socket) {
        // Checked before the WebSocket opens, so a connection that is
        // refused never reaches the core.
        const admitted = admit(request, server.port!)
        if (admitted instanceof Response) return admitted
        // Something on this computer that passes connections on, such as a
        // proxy serving the page over HTTPS, makes every connection come from
        // here; the page's own address, which the browser gives, does not.
        const address = server.requestIP(request)?.address ?? ""
        const origin = request.headers.get("origin")
        const here = (options.fromThisComputer ?? isLoopback)(address) && (origin === null || onThisComputer(origin))
        if (server.upgrade(request, { data: { ...admitted, here, unsubscribe: () => {}, sessions: new Set(), opening: 0, joining: new Map(), openingQuestions: new Map() } })) return undefined
        return new Response("Connect here with a WebSocket.", { status: 400 })
      }
      if (path === PATHS.check) {
        const admitted = admit(request, server.port!)
        return admitted instanceof Response ? admitted : new Response("This token is accepted.")
      }
      if (path === PATHS.login && request.method === "POST") return login(request, server.port!)
      if (path === PATHS.signInLink && request.method === "POST") return signInLink(request, server.port!)
      return new Response("Not found.", { status: 404 })
    },
    websocket: {
      open(socket) {
        connections.add(socket)
        socket.data.unsubscribe = options.evie.subscribe((event) => {
          // A session's summary goes to every connection, whatever it has
          // open, so that each can keep a list of every session. While the
          // session is being opened by its id it is held back with the
          // session's events, so that it never comes before an event it tells of.
          if (event.type === "summary") {
            const joining = socket.data.joining.get(event.session)
            return joining ? void joining.held.push(event) : send(socket, { event })
          }
          // So does a session's deletion, after which nothing more of it is sent.
          if (event.type === "deleted") {
            socket.data.sessions.delete(event.session)
            return send(socket, { event })
          }
          if (!sentTo(socket.data, event)) return
          // A question sent while opening, from a session not yet opened here,
          // is kept track of until it is answered: see `close`.
          if (event.type === "question" && !socket.data.sessions.has(event.session)) socket.data.openingQuestions.set(event.body.id, event.session)
          if (event.type === "answer") socket.data.openingQuestions.delete(event.body.id)
          // Held back while the session is being opened by its id: see
          // `message`. Not a question or an answer, which opening it can wait on.
          const joining = socket.data.joining.get(event.session)
          if (joining && event.type !== "question" && event.type !== "answer") return void joining.held.push(event)
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
        if (!socket.data.here && changesOnlyHere(message.command)) return void send(socket, { reply: message.id, error: ONLY_HERE_WORDS })
        const open = message.command.type === "open_session" ? message.command : undefined
        if (open) socket.data.opening += 1
        // A session opened by its id, as a client catching up does, is sent
        // to this connection from now, before the core reads where it
        // stands, so that no event falls between that reading and the reply.
        // Until the reply is sent its events are held back, so that what
        // reaches the client before the reply is in it, and what comes after
        // is new: see `holdBack`. A session already open is read in the same
        // step it is handed the command. One opened again is read once it is
        // open, so the events of its opening are in the reply and also come
        // after it, and the client skips the second by their numbers.
        const release = open?.session === undefined ? undefined : holdBack(socket, open.session)
        try {
          const reply = await replyTo(socket.data.device, message)
          // The session opened is learned from the reply, before the reply is
          // sent, so the client is sent its events from then on.
          if (open && "result" in reply) {
            const { session } = reply.result as Commands["open_session"]["output"]
            socket.data.sessions.add(session)
            openedHere.add(session)
          }
          send(socket, reply)
        } finally {
          if (open) socket.data.opening -= 1
          release?.()
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
  }, options.recheckEvery ?? 3000)
  const keepalive = setInterval(() => {
    for (const socket of connections) send(socket, { keepalive: true })
  }, options.keepaliveEvery ?? 15_000)

  return {
    url: `http://${hostInAddress(options.host)}:${server.port}`,
    async stop() {
      clearInterval(recheck)
      clearInterval(keepalive)
      await server.stop(true)
    },
  }
}

const send = (socket: ServerWebSocket<Connection>, message: ServerMessage) => socket.send(JSON.stringify(message))

// The token in the request's `Authorization` header, if it carries one.
const bearerIn = (request: Request) => /^Bearer\s+(\S+)$/i.exec(request.headers.get("authorization") ?? "")?.[1]

// Addresses that mean every address of this computer, as a URL writes them.
const WILDCARDS = ["0.0.0.0", "[::]"]

// Today's date where the server runs, as the owner reads it, such as 2026-11-21.
function localDay() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`
}

// The token in the browser's cookie for the server on this port, if the
// request carries one.
const cookieIn = (request: Request, port: number) => new Bun.CookieMap(request.headers.get("cookie") ?? "").get(cookieName(port)) ?? undefined

// Where the web app is served: its page, at its own address and at the
// address a sign-in link opens, and each file the page loads. A built
// file's name carries a hash of its contents, so a browser may keep it for
// good; the page is asked for afresh each time, so a new version is seen.
function appRoutes(app: WebApp | undefined): Record<string, Response | HTMLBundle> {
  if (!app) return {}
  if (!("files" in app)) return { [PATHS.page]: app.page, [PATHS.signIn]: app.page }
  const served = (file: Blob, cache: string) => new Response(file, { headers: { "Content-Type": file.type, "Cache-Control": cache } })
  const routes: Record<string, Response> = {}
  for (const [path, file] of Object.entries(app.files)) routes[path] = served(file, "public, max-age=31536000, immutable")
  for (const path of [PATHS.page, PATHS.signIn]) routes[path] = served(app.page, "no-cache")
  return routes
}

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
