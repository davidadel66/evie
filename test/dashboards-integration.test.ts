import { afterEach, expect, test } from "bun:test"
import { mkdir, realpath, writeFile } from "node:fs/promises"
import { join } from "node:path"
import { loadSettingsFiles } from "../src/index.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

const cleanups: (() => Promise<void>)[] = []

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// Reserve an unused ephemeral TCP port on 127.0.0.1.
function freePort(): number {
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("ok") })
  const port = server.port!
  server.stop(true)
  return port
}

// Command that starts a tiny Bun HTTP server echoing method, pathname, query, headers, and body.
function echoServerCommand(port: number): string {
  const code = [
    `Bun.serve({`,
    `  hostname: "127.0.0.1",`,
    `  port: ${port},`,
    `  async fetch(req) {`,
    `    const url = new URL(req.url);`,
    `    const headers = {};`,
    `    req.headers.forEach((v, k) => { headers[k.toLowerCase()] = v; });`,
    `    const body = req.method === "GET" || req.method === "HEAD" ? "" : await req.text();`,
    `    if (url.pathname === "/api/echo") {`,
    `      return Response.json({ method: req.method, pathname: url.pathname, search: url.search, headers, body });`,
    `    }`,
    `    return new Response("<h1>Live App</h1>", { headers: { "Content-Type": "text/html; charset=utf-8" } });`,
    `  },`,
    `});`,
  ].join(" ")
  return `${JSON.stringify(process.execPath)} -e ${JSON.stringify(code)}`
}

async function isListening(url: string): Promise<boolean> {
  try {
    const res = await fetch(url, { method: "GET", redirect: "manual", signal: AbortSignal.timeout(400) })
    await res.body?.cancel().catch(() => {})
    return true
  } catch {
    return false
  }
}

async function waitForDown(url: string, timeoutMs = 2500): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (!(await isListening(url))) return true
    await Bun.sleep(25)
  }
  return !(await isListening(url))
}

async function writeProjectConfig(projectFolder: string, config: unknown) {
  const evieDir = join(await realpath(projectFolder), ".evie")
  await mkdir(evieDir, { recursive: true })
  await writeFile(join(evieDir, "evie.json"), JSON.stringify(config, null, 2))
  return evieDir
}

test("list_dashboards over WS returns entries in order for a trusted project, and [] for an untrusted project", async () => {
  const trusted = await serveEvie([scriptedProvider([]).plugin], { isTrusted: () => true })
  cleanups.push(trusted.stop)
  const trustedEvie = await writeProjectConfig(trusted.run.projectFolder, {
    dashboards: [
      { name: "Overview", path: "dashboards/overview.html" },
      { name: "Metrics", url: "http://127.0.0.1:4199", start: "bun run dev", idle: 15 },
      { name: "Bad Entry", url: "https://example.com" },
    ],
  })
  await mkdir(join(trustedEvie, "dashboards"), { recursive: true })
  await writeFile(join(trustedEvie, "dashboards", "overview.html"), "<h1>Overview</h1>")

  const trustedClient = await trusted.connect()
  const listed = await trustedClient.send({ type: "list_dashboards", project: trusted.run.project!.id })
  expect(listed.dashboards).toEqual([
    { id: "overview", name: "Overview", kind: "static", path: "dashboards/overview.html" },
    { id: "metrics", name: "Metrics", kind: "live", url: "http://127.0.0.1:4199", start: "bun run dev", idle: 15 },
    {
      id: "bad-entry",
      name: "Bad Entry",
      kind: "invalid",
      error: expect.stringContaining("loopback"),
    },
  ])

  const untrusted = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(untrusted.stop)
  const untrustedEvie = await writeProjectConfig(untrusted.run.projectFolder, {
    dashboards: [
      { name: "Overview", path: "dashboards/overview.html" },
      { name: "Metrics", url: "http://127.0.0.1:4199" },
    ],
  })
  await mkdir(join(untrustedEvie, "dashboards"), { recursive: true })
  await writeFile(join(untrustedEvie, "dashboards", "overview.html"), "<h1>Overview</h1>")

  const untrustedClient = await untrusted.connect()
  expect(await untrustedClient.send({ type: "list_dashboards", project: untrusted.run.project!.id })).toEqual({ dashboards: [] })
})

test("open_dashboard for a static dashboard returns { kind: 'static', path } which can mint a working /preview pass", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin], { isTrusted: () => true })
  cleanups.push(served.stop)
  const evieDir = await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Summary", path: "dashboards/summary.html" }],
  })
  await mkdir(join(evieDir, "dashboards"), { recursive: true })
  await writeFile(join(evieDir, "dashboards", "summary.html"), "<h1>Summary</h1><script src='app.js'></script>")
  await writeFile(join(evieDir, "dashboards", "app.js"), "console.log('hi')")

  const client = await served.connect()
  const project = served.run.project!.id
  const opened = await client.send({ type: "open_dashboard", project, id: "summary" })
  expect(opened).toEqual({ kind: "static", path: "dashboards/summary.html" })

  // Mint a /preview pass using the returned path and verify it serves the HTML and companion asset.
  const mintRes = await fetch(
    `${served.server.url}/preview?${new URLSearchParams({ project, path: opened.kind === "static" ? opened.path : "" })}`,
    { method: "POST", headers: { Authorization: `Bearer ${served.token}` } },
  )
  expect(mintRes.status).toBe(200)
  const { url: previewUrl } = (await mintRes.json()) as { url: string }
  expect(previewUrl.startsWith("/preview/")).toBe(true)

  const htmlRes = await fetch(`${served.server.url}${previewUrl}`)
  expect(htmlRes.status).toBe(200)
  expect(await htmlRes.text()).toContain("<h1>Summary</h1>")
})

test("open_dashboard for a live dashboard starts on demand, returns proxied pass URL, forwards GET+POST, and strips Evie credentials", async () => {
  const port = freePort()
  const upstreamUrl = `http://127.0.0.1:${port}`
  const served = await serveEvie([scriptedProvider([]).plugin], {
    isTrusted: () => true,
    dashboards: { probeIntervalMs: 25, startTimeoutMs: 5000 },
  })
  cleanups.push(served.stop)
  await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Echo App", url: upstreamUrl, start: echoServerCommand(port) }],
  })

  const client = await served.connect()
  const project = served.run.project!.id
  const opened = await client.send({ type: "open_dashboard", project, id: "echo-app" })
  expect(opened.kind).toBe("live")
  if (opened.kind !== "live" || opened.status !== "ready") throw new Error("expected live ready dashboard")
  expect(opened.url).toBe(`/dashboard/${opened.passId}/`)
  expect(opened.url).not.toContain("127.0.0.1")
  expect(opened.passId).toBeTruthy()

  // GET HTML through the proxy.
  const pageRes = await fetch(`${served.server.url}${opened.url}`)
  expect(pageRes.status).toBe(200)
  expect(await pageRes.text()).toBe("<h1>Live App</h1>")
  expect(pageRes.headers.get("content-security-policy")).toContain("sandbox")

  // GET + POST to /api/echo with query params, JSON body, Evie cookie, and Authorization header.
  const serverPort = new URL(served.server.url).port
  const getRes = await fetch(`${served.server.url}${opened.url}api/echo?from=get&n=1`, {
    headers: {
      Authorization: `Bearer ${served.token}`,
      Cookie: `evie_token_${serverPort}=${served.token}; theme=dark`,
    },
  })
  expect(getRes.status).toBe(200)
  const getEcho = (await getRes.json()) as { method: string; pathname: string; search: string; headers: Record<string, string>; body: string }
  expect(getEcho.method).toBe("GET")
  expect(getEcho.pathname).toBe("/api/echo")
  expect(getEcho.search).toBe("?from=get&n=1")
  expect(getEcho.headers.authorization).toBeUndefined()
  expect(getEcho.headers.cookie).toBe("theme=dark")

  const postRes = await fetch(`${served.server.url}${opened.url}api/echo?from=post`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${served.token}`,
      Cookie: `evie_token_${serverPort}=${served.token}; app_session=abc`,
    },
    body: JSON.stringify({ hello: "world", count: 3 }),
  })
  expect(postRes.status).toBe(200)
  const postEcho = (await postRes.json()) as { method: string; pathname: string; search: string; headers: Record<string, string>; body: string }
  expect(postEcho.method).toBe("POST")
  expect(postEcho.pathname).toBe("/api/echo")
  expect(postEcho.search).toBe("?from=post")
  expect(postEcho.headers.authorization).toBeUndefined()
  expect(postEcho.headers.cookie).toBe("app_session=abc")
  expect(JSON.parse(postEcho.body)).toEqual({ hello: "world", count: 3 })
})

test("touch_dashboard slides pass expiry, expires without touch, and returns { ok: false } for wrong passId", async () => {
  const upstream = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch: () => new Response("alive"),
  })
  cleanups.push(async () => upstream.stop(true))
  const upstreamUrl = `http://127.0.0.1:${upstream.port}`

  const served = await serveEvie([scriptedProvider([]).plugin], {
    isTrusted: () => true,
    dashboardPassLasts: 150,
  })
  cleanups.push(served.stop)
  await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Alive", url: upstreamUrl }],
  })

  const client = await served.connect()
  const project = served.run.project!.id
  const opened = await client.send({ type: "open_dashboard", project, id: "alive" })
  if (opened.kind !== "live" || opened.status !== "ready" || !opened.passId) throw new Error("expected live ready dashboard")

  // Wrong passId returns { ok: false }.
  expect(await client.send({ type: "touch_dashboard", project, id: "alive", passId: "wrong-pass-id" })).toEqual({ ok: false })

  // Touch halfway through TTL to slide expiry past original 150ms deadline.
  await Bun.sleep(90)
  expect(await client.send({ type: "touch_dashboard", project, id: "alive", passId: opened.passId })).toEqual({ ok: true })
  await Bun.sleep(90)
  expect((await fetch(`${served.server.url}${opened.url}`)).status).toBe(200)

  // Let it expire without touching.
  await Bun.sleep(180)
  expect((await fetch(`${served.server.url}${opened.url}`)).status).toBe(404)
  expect(await client.send({ type: "touch_dashboard", project, id: "alive", passId: opened.passId })).toEqual({ ok: false })
})

test("revoking the device token immediately invalidates /dashboard/<pass>/", async () => {
  const upstream = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch: () => new Response("secret-dashboard"),
  })
  cleanups.push(async () => upstream.stop(true))

  const served = await serveEvie([scriptedProvider([]).plugin], { isTrusted: () => true })
  cleanups.push(served.stop)
  await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Secret", url: `http://127.0.0.1:${upstream.port}` }],
  })

  const client = await served.connect()
  const opened = await client.send({ type: "open_dashboard", project: served.run.project!.id, id: "secret" })
  if (opened.kind !== "live" || opened.status !== "ready") throw new Error("expected live ready dashboard")

  expect((await fetch(`${served.server.url}${opened.url}`)).status).toBe(200)

  // Revoke the device token ("test device" from serveEvie).
  expect(served.tokens.remove("test device")).toBe(true)
  expect((await fetch(`${served.server.url}${opened.url}`)).status).toBe(404)
})

test("serveEvie.stop() (evie.close()) stops live dashboard processes started by Evie", async () => {
  const port = freePort()
  const upstreamUrl = `http://127.0.0.1:${port}`
  const served = await serveEvie([scriptedProvider([]).plugin], {
    isTrusted: () => true,
    dashboards: { probeIntervalMs: 25, startTimeoutMs: 5000 },
  })
  let stopped = false
  cleanups.push(async () => {
    if (!stopped) await served.stop()
  })
  await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Stoppable", url: upstreamUrl, start: echoServerCommand(port) }],
  })

  const client = await served.connect()
  const opened = await client.send({ type: "open_dashboard", project: served.run.project!.id, id: "stoppable" })
  expect(opened.kind).toBe("live")
  if (opened.kind !== "live" || opened.status !== "ready") throw new Error("expected live ready dashboard")
  expect(await isListening(upstreamUrl)).toBe(true)

  stopped = true
  await served.stop()
  expect(await waitForDown(upstreamUrl)).toBe(true)
})

test("invalid dashboard id in open_dashboard returns { kind: 'invalid', error }", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin], { isTrusted: () => true })
  cleanups.push(served.stop)
  await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Broken", url: "https://not-loopback.example" }],
  })

  const client = await served.connect()
  const project = served.run.project!.id

  const missing = await client.send({ type: "open_dashboard", project, id: "does-not-exist" })
  expect(missing).toEqual({ kind: "invalid", error: 'There is no dashboard "does-not-exist" in this workspace.' })

  const broken = await client.send({ type: "open_dashboard", project, id: "broken" })
  expect(broken.kind).toBe("invalid")
  if (broken.kind === "invalid") expect(broken.error).toContain("loopback")
})

test("settings-files accepts dashboards field (even malformed) and exposes live isTrusted(folder)", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  const projectFolder = await realpath(served.run.projectFolder)
  await writeProjectConfig(projectFolder, {
    dashboards: [{ name: "Broken", url: "https://bad.example" }, "not-an-object"],
  })

  const settings = await loadSettingsFiles({ env: { EVIE_HOME: served.run.home } })
  expect(settings.isTrusted(projectFolder)).toBe(false)

  await settings.trust(projectFolder)
  expect(settings.isTrusted(projectFolder)).toBe(true)

  // projectSettings still loads without rejecting the malformed dashboards field.
  const own = await settings.projectSettings({ id: served.run.project!.id, name: "proj", folder: projectFolder })
  expect(own.notices).toEqual([])

  await settings.untrust(projectFolder)
  expect(settings.isTrusted(projectFolder)).toBe(false)
})
