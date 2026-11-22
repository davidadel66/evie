import { afterEach, expect, test } from "bun:test"
import { startServer } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import type { Evie } from "../src/core/types.ts"
import { connectionLine, openConnection, type Connection, type ConnectionState, type ConnectionOptions } from "../web/src/connection.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The web app's connection to the server, apart from its screens: what it
// tells them as it connects, drops, comes back and is signed out. A browser
// connects with its cookie; these tests, which have none, bring a token.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(options: Parameters<typeof serveEvie>[1] = {}) {
  const served = await serveEvie([scriptedProvider([]).plugin], options)
  cleanups.push(served.stop)
  return served
}

// Opens a connection, and keeps every state it tells of, in turn.
function open(options: ConnectionOptions) {
  const connection = openConnection({ firstWait: 10, ...options })
  cleanups.push(() => connection.close())
  const told: ConnectionState["status"][] = [connection.state().status]
  connection.subscribe(() => told.push(connection.state().status))
  return { connection, told }
}

// Settles once the connection's state is the one given.
async function until(connection: Connection, status: ConnectionState["status"]) {
  while (connection.state().status !== status) await Bun.sleep(5)
  return connection.state()
}

// A port that was free a moment ago.
async function freePort() {
  const probe = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response() })
  const port = probe.port!
  await probe.stop(true)
  return port
}

test("with a token the server accepts, it is connecting and then connected, and the client it gives sends commands", async () => {
  const served = await start()
  const { connection, told } = open({ url: served.server.url, token: served.token })

  const state = await until(connection, "connected")

  expect(told).toEqual(["connecting", "connected"])
  if (state.status !== "connected") throw new Error("not connected")
  expect((await state.evie.send({ type: "open_session", folder: served.run.projectFolder })).session).toBeString()
})

test("with no token or cookie the server accepts, it is signed out, with nothing more to say", async () => {
  const served = await start()
  const { connection } = open({ url: served.server.url })

  expect(await until(connection, "signed out")).toEqual({ status: "signed out" })
})

test("when the line drops it is reconnecting, and connected again once the server is back", async () => {
  const served = await start()
  const port = Number(new URL(served.server.url).port)
  const { connection, told } = open({ url: served.server.url, token: served.token })
  await until(connection, "connected")

  await served.server.stop()
  // Why, in a sentence for the page to show; the runtime says what ended the line.
  expect(await until(connection, "reconnecting")).toEqual({ status: "reconnecting", reason: expect.stringMatching(/^The connection to evie's server dropped: .+\.$/) })
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  await until(connection, "connected")
  expect(told).toEqual(["connecting", "connected", "reconnecting", "connected"])
})

test("when no server answers at first, it keeps trying, connecting and saying why, until one does", async () => {
  const served = await start()
  const port = await freePort()
  const { connection, told } = open({ url: `http://127.0.0.1:${port}`, token: served.token })

  while (!("reason" in connection.state())) await Bun.sleep(5)
  expect(connection.state()).toEqual({
    status: "connecting",
    reason: `No evie server is running at http://127.0.0.1:${port}. Start it with \`evie serve\`.`,
  })
  const server = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => server.stop())

  await until(connection, "connected")
  // It was never connected, so it was never reconnecting.
  expect(told).not.toContain("reconnecting")
})

test("when its token is taken back while it is connected, it is signed out, saying why", async () => {
  const served = await start({ recheckEvery: 20 })
  const { connection } = open({ url: served.server.url, token: served.token })
  await until(connection, "connected")

  const tokens = await openTokens(served.run.home)
  tokens.remove("test device")
  tokens.close()

  expect(await until(connection, "signed out")).toEqual({ status: "signed out", reason: "This browser was signed out: its token was taken back." })
})

test("a sign-in code that does not work leaves it signed out, saying why, and so does a pasted token the server refuses", async () => {
  const served = await start()
  const { connection } = open({ url: served.server.url, code: "not-a-code" })

  expect(await until(connection, "signed out")).toEqual({
    status: "signed out",
    reason: "This sign-in link has been used already or has expired. Run `evie web` for a new one.",
  })
  expect(await connection.signIn({ token: "not-a-token" })).toBe("evie did not accept that token. Check that it was copied whole.")
  expect(connection.state().status).toBe("signed out")
})

test("a page at an address the server does not let in, whose cookie is good, is signed out saying what to change, not left reconnecting", async () => {
  // As the server answers such a page: its WebSocket is refused for the
  // page's address, but its request to /check carries no Origin, as a
  // browser's request to its own page's server does not, and is accepted.
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch: (request) =>
      new URL(request.url).pathname === "/check"
        ? new Response("This token is accepted.")
        : new Response("Connections from this page are not allowed.", { status: 403 }),
  })
  cleanups.push(async () => server.stop(true))
  const url = `http://127.0.0.1:${server.port}`
  const { connection } = open({ url })

  expect(await until(connection, "signed out")).toEqual({
    status: "signed out",
    reason: `The evie server at ${url} does not let this page connect. Add its address to "origins" under "server" in evie.json.`,
  })
})

test("signing in twice at once, as a double press of Sign in does, or while it is connecting, opens no second connection", async () => {
  const served = await start()
  // A token the server refuses, so that it is signed out, and stays signed
  // out after each sign-in: this test has no browser to keep a cookie.
  const { connection, told } = open({ url: served.server.url, token: "not-a-token" })

  const whileConnecting = await connection.signIn({ token: served.token })
  await until(connection, "signed out")
  const both = await Promise.all([connection.signIn({ token: served.token }), connection.signIn({ token: served.token })])
  await until(connection, "signed out")
  await Bun.sleep(50)

  expect([whileConnecting, ...both]).toEqual([undefined, undefined, undefined])
  expect(told).toEqual(["connecting", "signed out", "connecting", "signed out"])
})

test("signing in gives up when the server does not answer in time, and says so", async () => {
  // A server that refuses the connection without a cookie, and answers a
  // sign-in only after the page has stopped waiting.
  const late = () => new Promise<Response>((resolve) => setTimeout(() => resolve(new Response(null, { status: 204 })), 300))
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch: (request) => (new URL(request.url).pathname === "/login" ? late() : new Response("No token.", { status: 401 })),
  })
  cleanups.push(async () => server.stop(true))
  const { connection } = open({ url: `http://127.0.0.1:${server.port}`, answerWithin: 50 })
  await until(connection, "signed out")

  expect(await connection.signIn({ token: "a-token" })).toBe("evie's server did not answer in time. Check that `evie serve` is running, and try again.")
})

test("the line under the header says nothing while connected or signed out, and while the line is not up, the word and why", () => {
  const dropped = "The connection to evie's server dropped: the server went away."
  const away = "No evie server is running at http://127.0.0.1:3843. Start it with `evie serve`."

  expect(connectionLine({ status: "connected", evie: {} as Evie })).toBeUndefined()
  expect(connectionLine({ status: "signed out", reason: "This browser was signed out: its token was taken back." })).toBeUndefined()
  expect(connectionLine({ status: "connecting" })).toEqual({ word: "Connecting" })
  expect(connectionLine({ status: "connecting", reason: away })).toEqual({ word: "Connecting", reason: away })
  expect(connectionLine({ status: "reconnecting", reason: dropped })).toEqual({ word: "Reconnecting", reason: dropped })
})
