import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { connect, startServer } from "../src/index.ts"
import { openTokens, OWN_NAME } from "../src/server/tokens.ts"
import { buildWebApp, type WebApp } from "../src/server/web-app.ts"
import { loadServerSettings } from "../src/settings-files.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie, type Served } from "./support/served.ts"

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
  let ended: string | undefined
  await served.connect({ onEnd: (cause) => (ended = cause.message) })
  const phoneClient = await served.connect({ token: phone.secret })

  expect(await takeBack(served.run.home, "test device")).toBe(true)
  while (ended === undefined) await Bun.sleep(5)

  expect(ended).toBe("This device's token was taken back")
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
      add: served.tokens.add,
    },
    host: "127.0.0.1",
    port: 0,
    recheckEvery: 10,
  })
  cleanups.push(() => server.stop())
  let ended: string | undefined
  const client = await connect({ url: server.url, token: served.token, onEnd: (cause) => (ended = cause.message) })
  cleanups.push(() => client.close())

  failing = true
  await Bun.sleep(50)
  expect((await client.send({ type: "open_session", folder: served.run.projectFolder })).session).toBeString()
  expect(ended).toBeUndefined()

  failing = false
  await takeBack(served.run.home, "test device")
  while (ended === undefined) await Bun.sleep(5)
  expect(ended).toBe("This device's token was taken back")
})

test("a token that cannot be checked, such as with the database busy, is answered with try again, not let in", async () => {
  const served = await start()
  const server = startServer({
    evie: served.run.evie,
    tokens: {
      check: () => {
        throw new Error("database is locked")
      },
      add: served.tokens.add,
    },
    host: "127.0.0.1",
    port: 0,
  })
  cleanups.push(() => server.stop())
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const bearer = { Authorization: `Bearer ${served.token}` }

  for (const answer of [
    await fetch(`${server.url}/ws`, { headers: bearer }),
    await fetch(`${server.url}/check`, { headers: bearer }),
    await login(server.url, page, { token: served.token }),
  ]) {
    expect(answer.status).toBe(503)
    expect(await answer.text()).toBe("The token could not be checked just now. Try again.")
  }
  await expect(connect({ url: server.url, token: served.token })).rejects.toThrow(
    `The Evie server at ${server.url} could not check the token just now. Try again.`,
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
  // Over plain http there is no Secure flag; the browser would not keep the
  // cookie. It lasts 400 days, the longest a browser keeps one.
  expect(flags.sort()).toEqual(["HttpOnly", "Max-Age=34560000", "Path=/", "SameSite=Strict"])
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

// ---- The web app ----------------------------------------------------------------------------

// The web app, built once for the tests that serve it.
let built: Promise<WebApp> | undefined
const webApp = () => (built ??= buildWebApp())

test("the web app's page is served to anyone at the server's own address and at /sign-in, with the files it loads", async () => {
  const app = await webApp()
  if (!("files" in app)) throw new Error("the web app was not built")
  const { server } = await start({ app })

  for (const path of ["/", "/sign-in"]) {
    const answer = await fetch(`${server.url}${path}`)
    expect(answer.status).toBe(200)
    expect(answer.headers.get("content-type")).toStartWith("text/html")
    expect(await answer.text()).toBe(await app.page.text())
  }
  for (const [path, file] of Object.entries(app.files)) {
    const answer = await fetch(`${server.url}${path}`)
    expect(answer.status).toBe(200)
    expect(answer.headers.get("content-type")).toBe(file.type)
    expect(await answer.text()).toBe(await file.text())
  }
  // Everything else still needs a token.
  expect((await fetch(`${server.url}/ws`)).status).toBe(401)
  expect((await fetch(`${server.url}/nothing-here`)).status).toBe(404)
})

// Asks for a sign-in link, as `evie web` does, with the headers given.
const signInLink = (url: string, headers: Record<string, string>) => fetch(`${url}/sign-in-link`, { method: "POST", headers })

// This computer's own token, as `evie serve` makes it: the only one that may
// ask for a sign-in link.
function ownToken(tokens: Served["tokens"]) {
  const made = tokens.add(OWN_NAME)
  if (!("secret" in made)) throw new Error(made.refused)
  return { Authorization: `Bearer ${made.secret}` }
}

// The code in a sign-in link, which is at /sign-in on the server's own address.
async function codeIn(answer: Response, url: string) {
  expect(answer.status).toBe(200)
  const { link } = (await answer.json()) as { link: string }
  const code = new RegExp(`^${url}/sign-in#code=([\\w-]{43})$`).exec(link)?.[1]
  if (code === undefined) throw new Error(`not a sign-in link: ${link}`)
  return code
}

// Today's date where the server runs, as a browser's token is named.
function localDay() {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`
}

// The names of the tokens there are. Two made in the same millisecond can
// be listed either way round, so they are sorted.
const names = (tokens: Served["tokens"]) => tokens.list().map((made) => made.name).sort()

// The browser's cookie, from an answer that sets it.
const cookieFrom = (answer: Response) => answer.headers.get("set-cookie")!.split("; ")[0]!

test("this computer asks for a sign-in link, and its code signs a browser in once, with a token of the browser's own named for the local day", async () => {
  // A time zone whose date today is not the UTC date, so that a name made
  // from the UTC date would show.
  const zone = process.env.TZ
  process.env.TZ = new Date().getUTCHours() >= 10 ? "Pacific/Kiritimati" : "Pacific/Pago_Pago"
  cleanups.push(async () => void (zone === undefined ? delete process.env.TZ : (process.env.TZ = zone)))
  const { server, tokens } = await start()
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const code = await codeIn(await signInLink(server.url, ownToken(tokens)), server.url)

  const first = await login(server.url, page, { code })

  expect(first.status).toBe(204)
  const cookie = cookieFrom(first)
  expect(await opens(server.url, { Origin: page, Cookie: cookie })).toBe(true)
  // Named for the day it signed in, so that it can be taken back alone.
  expect(names(tokens)).toEqual([`browser ${localDay()}`, "test device", OWN_NAME])
  expect(tokens.check(cookie.replace(/^[^=]+=/, ""))).toBe(`browser ${localDay()}`)

  const again = await login(server.url, page, { code })
  expect(again.status).toBe(401)
  expect(again.headers.get("set-cookie")).toBeNull()
  expect(await again.text()).toBe("This sign-in link has been used already or has expired. Run `evie web` for a new one.")
})

test("only this computer's own token, sent as a token, asks for a sign-in link: another device's token and a browser's cookie are refused", async () => {
  const { server, token, tokens } = await start()
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const browser = cookieFrom(await login(server.url, page, { code: await codeIn(await signInLink(server.url, ownToken(tokens)), server.url) }))

  const otherDevice = await signInLink(server.url, { Authorization: `Bearer ${token}` })
  const fromBrowser = await signInLink(server.url, { Origin: page, Cookie: browser })

  expect(otherDevice.status).toBe(403)
  expect(await otherDevice.text()).toBe(
    "Only this computer's own token can ask for a sign-in link. On another device, open evie's address in a browser and paste a token made for that device.",
  )
  expect(fromBrowser.status).toBe(401)
})

test("the code in a sign-in link stops working after its few minutes, and makes no token", async () => {
  const { server, tokens } = await start({ signInCodeLasts: 30 })
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const code = await codeIn(await signInLink(server.url, ownToken(tokens)), server.url)

  await Bun.sleep(60)

  const late = await login(server.url, page, { code })
  expect(late.status).toBe(401)
  expect(late.headers.get("set-cookie")).toBeNull()
  expect(names(tokens)).toEqual(["test device", OWN_NAME])
})

test("a sign-in link is refused to a client with no token, and to a page from another address, which cannot use a code either", async () => {
  const { server, tokens } = await start()
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const bearer = ownToken(tokens)

  expect((await signInLink(server.url, {})).status).toBe(401)
  expect((await signInLink(server.url, { Authorization: "Bearer not-a-token" })).status).toBe(401)
  expect((await signInLink(server.url, { ...bearer, Origin: "http://somewhere.example" })).status).toBe(403)

  // A code brought by another site's page is refused, and is not used up.
  const code = await codeIn(await signInLink(server.url, bearer), server.url)
  const elsewhere = await login(server.url, "http://somewhere.example", { code })
  expect(elsewhere.status).toBe(403)
  expect(elsewhere.headers.get("set-cookie")).toBeNull()
  expect((await login(server.url, page, { code })).status).toBe(204)
})

test("a sign-in link asked for at the server's wildcard address points the browser to 127.0.0.1, which a browser opens", async () => {
  const { server, tokens } = await start({ host: "0.0.0.0" })
  const port = new URL(server.url).port

  const asked = await signInLink(`http://0.0.0.0:${port}`, ownToken(tokens))

  await codeIn(asked, `http://127.0.0.1:${port}`)
})

test("two browsers signed in on the same day each have a token of their own, and one is taken back alone", async () => {
  const served = await start({ recheckEvery: 20 })
  const { server, tokens } = served
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const bearer = ownToken(tokens)
  const signIn = async () => cookieFrom(await login(server.url, page, { code: await codeIn(await signInLink(server.url, bearer), server.url) }))

  const laptop = await signIn()
  const phone = await signIn()

  const day = localDay()
  expect(names(tokens)).toEqual([`browser ${day}`, `browser ${day} (2)`, "test device", OWN_NAME])
  expect(await takeBack(served.run.home, `browser ${day}`)).toBe(true)
  expect(await opens(server.url, { Origin: page, Cookie: laptop })).toBe(false)
  expect(await opens(server.url, { Origin: page, Cookie: phone })).toBe(true)
})

// Trades a sign-in code for the cookie, as a browser that holds the cookie given does.
const loginWith = (url: string, page: string, cookie: string, code: string) =>
  fetch(`${url}/login`, { method: "POST", headers: { Origin: page, "Content-Type": "application/json", Cookie: cookie }, body: JSON.stringify({ code }) })

test("a browser already signed in that opens a sign-in link keeps the token it has, and the link's code is used up", async () => {
  const { server, tokens } = await start()
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const bearer = ownToken(tokens)
  const newLink = async () => codeIn(await signInLink(server.url, bearer), server.url)
  const cookie = cookieFrom(await login(server.url, page, { code: await newLink() }))

  const code = await newLink()
  const again = await loginWith(server.url, page, cookie, code)

  expect(again.status).toBe(204)
  expect(cookieFrom(again)).toBe(cookie)
  expect(names(tokens)).toEqual([`browser ${localDay()}`, "test device", OWN_NAME])
  expect((await loginWith(server.url, page, cookie, code)).status).toBe(401)
})

test("a browser signed in already whose token cannot be checked just now is told to try again, and is not made a second token", async () => {
  const served = await start()
  let failing = false
  const server = startServer({
    evie: served.run.evie,
    tokens: {
      check: (secret) => {
        if (failing) throw new Error("database is locked")
        return served.tokens.check(secret)
      },
      add: served.tokens.add,
    },
    host: "127.0.0.1",
    port: 0,
  })
  cleanups.push(() => server.stop())
  const page = `http://127.0.0.1:${new URL(server.url).port}`
  const bearer = ownToken(served.tokens)
  const cookie = cookieFrom(await login(server.url, page, { code: await codeIn(await signInLink(server.url, bearer), server.url) }))
  const code = await codeIn(await signInLink(server.url, bearer), server.url)

  failing = true
  const busy = await loginWith(server.url, page, cookie, code)
  failing = false

  expect(busy.status).toBe(503)
  expect(names(served.tokens)).toEqual([`browser ${localDay()}`, "test device", OWN_NAME])
  // The code is kept, to be tried again.
  expect((await loginWith(server.url, page, cookie, code)).status).toBe(204)
})
