import { afterEach, expect, test } from "bun:test"
import { startServer } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import { openConnection, type Connection, type ConnectionState, type ConnectionOptions } from "../web/src/connection.ts"
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
  await until(connection, "reconnecting")
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  await until(connection, "connected")
  expect(told).toEqual(["connecting", "connected", "reconnecting", "connected"])
})

test("when no server answers at first, it keeps trying, saying why, until one does", async () => {
  const served = await start()
  const port = await freePort()
  const { connection } = open({ url: `http://127.0.0.1:${port}`, token: served.token })

  expect(await until(connection, "reconnecting")).toEqual({
    status: "reconnecting",
    reason: `No evie server is running at http://127.0.0.1:${port}. Start it with \`evie serve\`.`,
  })
  const server = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => server.stop())

  await until(connection, "connected")
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
