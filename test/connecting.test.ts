import { afterEach, expect, test } from "bun:test"
import { connect } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
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
