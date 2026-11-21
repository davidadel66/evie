import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { connect, startServer } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import { loadServerSettings } from "../src/settings-files.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// Who may connect to the server: only a device with a token, and from a web
// page only Evie's own or one the owner listed.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(options: Parameters<typeof serveEvie>[1] = {}) {
  const served = await serveEvie([scriptedProvider([]).plugin], options)
  cleanups.push(served.stop)
  return served
}

test("a connection with no token, or with one that is not a token, is refused before it opens, and the client says why", async () => {
  const { server } = await start()

  expect((await fetch(`${server.url}/ws`)).status).toBe(401)
  expect((await fetch(`${server.url}/ws`, { headers: { Authorization: "Bearer not-a-token" } })).status).toBe(401)
  await expect(connect({ url: server.url })).rejects.toThrow(`The Evie server at ${server.url} asks for a token, and none was given.`)
  await expect(connect({ url: server.url, token: "not-a-token" })).rejects.toThrow(`The Evie server at ${server.url} did not accept the token.`)
})

test("with no server running, the client says so and how to start one", async () => {
  // A port that was free a moment ago, with nothing listening on it now.
  const probe = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response() })
  const url = `http://127.0.0.1:${probe.port}`
  await probe.stop(true)

  await expect(connect({ url, token: "any" })).rejects.toThrow(`No Evie server is running at ${url}. Start it with \`evie serve\`.`)
})

test("a client gives up on an address where nothing answers when its signal fires, closes its connection, and says so", async () => {
  // Takes connections and never answers them.
  let closed = false
  const silent = Bun.listen({ hostname: "127.0.0.1", port: 0, socket: { data() {}, close: () => void (closed = true) } })
  cleanups.push(async () => silent.stop(true))
  const url = `http://127.0.0.1:${silent.port}`

  await expect(connect({ url, token: "any", signal: AbortSignal.timeout(100) })).rejects.toThrow(`No Evie server answered at ${url}.`)
  while (!closed) await Bun.sleep(5)
})

test("a client's signal has no effect once its connection is open", async () => {
  const served = await start()
  const client = await served.connect({ signal: AbortSignal.timeout(20) })

  await Bun.sleep(60)

  expect((await client.send({ type: "open_session", folder: served.run.projectFolder })).session).toBeString()
})

// Whether a WebSocket opens with the given headers, as a browser would send
// them. A browser always says which page is connecting, in `Origin`.
function opens(url: string, headers: Record<string, string>) {
  return new Promise<boolean>((resolve) => {
    const socket = new WebSocket(`${url.replace(/^http/, "ws")}/ws`, { headers })
    socket.addEventListener("open", () => {
      socket.close()
      resolve(true)
    })
    socket.addEventListener("close", () => resolve(false))
  })
}

test("a web page from another address is refused, even with a good token", async () => {
  const { server, token } = await start()
  const fromElsewhere = { Authorization: `Bearer ${token}`, Origin: "http://somewhere.example" }

  expect(await opens(server.url, fromElsewhere)).toBe(false)
  expect((await fetch(`${server.url}/ws`, { headers: fromElsewhere })).status).toBe(403)
  expect((await fetch(`${server.url}/check`, { headers: fromElsewhere })).status).toBe(403)
})

test("Evie's own page, and a page the owner listed, are let in", async () => {
  const { server, token } = await start({ origins: ["http://localhost:5173/"] })
  const port = new URL(server.url).port

  for (const origin of [`http://127.0.0.1:${port}`, `http://localhost:${port}`, "http://localhost:5173"]) {
    expect(await opens(server.url, { Authorization: `Bearer ${token}`, Origin: origin })).toBe(true)
  }
  // A client that is not a browser sends no Origin, and needs only its token.
  expect(await opens(server.url, { Authorization: `Bearer ${token}` })).toBe(true)
})

test("a web page whose origin is null is never let in, even when the settings list an origin that comes out as null", async () => {
  // An address without http:// or https:// has no origin of its own; its origin is the text "null".
  const { server, token } = await start({ origins: ["localhost:5173"] })

  expect(await opens(server.url, { Authorization: `Bearer ${token}`, Origin: "null" })).toBe(false)
})

// ---- Addresses ------------------------------------------------------------------------------

// The server's settings, as `evie serve` and `evie` read them, from a global
// settings file with the given "server" in a data folder of its own.
async function serverSettings(server: unknown, env: Record<string, string> = {}) {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  if (server !== undefined) await writeFile(join(home, "evie.json"), JSON.stringify({ server }))
  return loadServerSettings({ env: { EVIE_HOME: home, ...env } })
}

test("an origin in the settings must be a web address that starts with http:// or https://", async () => {
  await expect(serverSettings({ origins: ["localhost:5173"] })).rejects.toThrow(
    '"server" has "origins" that are not a list of web addresses, such as "http://localhost:5173".',
  )
  expect((await serverSettings({ origins: ["https://evie.example"] })).origins).toEqual(["https://evie.example"])
})

test("a host with colons, such as the IPv6 address ::1, is put in brackets in the address a client connects to", async () => {
  expect((await serverSettings({ host: "::1", port: 3843 })).address).toBe("http://[::1]:3843")
})

test("an EVIE_SERVER that is not a server's address is refused, saying it is EVIE_SERVER that is wrong", async () => {
  await expect(serverSettings(undefined, { EVIE_SERVER: "my mac:3843" })).rejects.toThrow(
    'EVIE_SERVER is not the address of a server, such as http://my-mac.local:3843: "my mac:3843".',
  )
  await expect(serverSettings(undefined, { EVIE_SERVER: "ftp://my-mac.local" })).rejects.toThrow("EVIE_SERVER is not the address of a server")
  expect((await serverSettings(undefined, { EVIE_SERVER: "my-mac.local:3843" })).address).toBe("http://my-mac.local:3843")
})

test("an EVIE_SERVER that names no port is taken to be on Evie's own port, 3843", async () => {
  const address = async (given: string) => (await serverSettings(undefined, { EVIE_SERVER: given })).address
  expect(await address("my-mac.local")).toBe("http://my-mac.local:3843")
  expect(await address("http://my-mac.local")).toBe("http://my-mac.local:3843")
  expect(await address("[::1]")).toBe("http://[::1]:3843")
  // A port that is given is kept, even the one the address would have anyway.
  expect(await address("http://my-mac.local:80")).toBe("http://my-mac.local:80")
  // Evie serves plain HTTP only, so an https address is something in front of it, on the usual port.
  expect(await address("https://evie.example")).toBe("https://evie.example")
})

test("a server listening on ::1 gives an address a client connects to, and lets its own page in", async () => {
  const served = await start({ host: "::1" })

  expect(served.server.url).toMatch(/^http:\/\/\[::1\]:\d+$/)
  const client = await served.connect()
  expect((await client.send({ type: "open_session", folder: served.run.projectFolder })).session).toBeString()
  expect(await opens(served.server.url, { Authorization: `Bearer ${served.token}`, Origin: served.server.url })).toBe(true)
})

// Takes a token back the way `evie token remove` does: from a database
// handle of its own, while the server holds the database open.
async function takeBack(home: string, name: string) {
  const tokens = await openTokens(home)
  try {
    return tokens.remove(name)
  } finally {
    tokens.close()
  }
}

test("a token taken back is refused from then on and its open connection is closed, while other devices carry on", async () => {
  const served = await start({ recheckEvery: 20 })
  const phone = served.tokens.add("phone")
  if (!("secret" in phone)) throw new Error(phone.refused)
  let dropped: string | undefined
  await served.connect({ onDrop: (reason) => (dropped = reason) })
  const phoneClient = await served.connect({ token: phone.secret })

  expect(await takeBack(served.run.home, "test device")).toBe(true)
  while (dropped === undefined) await Bun.sleep(5)

  expect(dropped).toBe("This device's token was taken back")
  await expect(served.connect()).rejects.toThrow("did not accept the token")
  const opened = await phoneClient.send({ type: "open_session", folder: served.run.projectFolder })
  expect(opened.session).toBeString()
})

test("a command sent with a token that was just taken back is not carried out", async () => {
  const served = await start({ recheckEvery: 60_000 })
  const client = await served.connect()

  await takeBack(served.run.home, "test device")

  await expect(client.send({ type: "open_session", folder: served.run.projectFolder })).rejects.toThrow(
    "The connection to Evie was lost before it answered: This device's token was taken back.",
  )
})

test("a token check that fails, such as with the database busy, keeps the connection, and it is checked again later", async () => {
  const served = await start()
  let failing = false
  const server = startServer({
    evie: served.run.evie,
    tokens: {
      check: (secret) => {
        if (failing) throw new Error("database is locked")
        return served.tokens.check(secret)
      },
    },
    host: "127.0.0.1",
    port: 0,
    recheckEvery: 10,
  })
  cleanups.push(() => server.stop())
  let dropped: string | undefined
  const client = await connect({ url: server.url, token: served.token, onDrop: (reason) => (dropped = reason) })
  cleanups.push(() => client.close())

  failing = true
  await Bun.sleep(50)
  expect((await client.send({ type: "open_session", folder: served.run.projectFolder })).session).toBeString()
  expect(dropped).toBeUndefined()

  failing = false
  await takeBack(served.run.home, "test device")
  while (dropped === undefined) await Bun.sleep(5)
  expect(dropped).toBe("This device's token was taken back")
})

// ---- The browser ---------------------------------------------------------------------------

const login = (url: string, origin: string, body: unknown) =>
  fetch(`${url}/login`, { method: "POST", headers: { Origin: origin, "Content-Type": "application/json" }, body: JSON.stringify(body) })

test("a browser trades its token for a cookie its scripts cannot read, and the WebSocket takes the cookie in place of the token", async () => {
  const { server, token } = await start()
  const page = `http://127.0.0.1:${new URL(server.url).port}`

  const answer = await login(server.url, page, { token })

  expect(answer.status).toBe(204)
  const [cookie, ...flags] = answer.headers.get("set-cookie")!.split("; ")
  expect(cookie).toBe(`evie_token=${token}`)
  // Over plain http there is no Secure flag; the browser would not keep the cookie.
  expect(flags.sort()).toEqual(["HttpOnly", "Path=/", "SameSite=Strict"])
  expect(await opens(server.url, { Origin: page, Cookie: cookie! })).toBe(true)
})

test("a login with a token that is not one, or from another page, sets no cookie, and a made-up cookie is refused", async () => {
  const { server, token } = await start()
  const page = `http://127.0.0.1:${new URL(server.url).port}`

  const wrong = await login(server.url, page, { token: "not-a-token" })
  const elsewhere = await login(server.url, "http://somewhere.example", { token })

  expect([wrong.status, elsewhere.status]).toEqual([401, 403])
  expect([wrong.headers.get("set-cookie"), elsewhere.headers.get("set-cookie")]).toEqual([null, null])
  expect(await opens(server.url, { Origin: page, Cookie: "evie_token=not-a-token" })).toBe(false)
})
