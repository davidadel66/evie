import { afterEach, expect, test } from "bun:test"
import { gzipSync } from "node:zlib"
import type { DashboardOpened } from "../src/core/types.ts"
import { appCommand, freePort, untilDown } from "./support/dashboard-apps.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

const cleanups: (() => Promise<void>)[] = []

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// An app already running on this computer, answering as `answer` says.
function runningApp(answer: (request: Request) => Response | Promise<Response>) {
  const app = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: answer })
  cleanups.push(async () => void app.stop(true))
  return `http://127.0.0.1:${app.port}`
}

// Evie behind its server, with a project whose settings list `dashboards`,
// as a trusted project's file would. Changing `setting.now` is the owner
// editing the file, or no longer trusting the project.
async function serveDashboards(dashboards: unknown, options: { passLasts?: number } = {}) {
  const setting = { now: dashboards }
  const served = await serveEvie([scriptedProvider([]).plugin], { dashboardsSetting: async () => setting.now, ...options })
  cleanups.push(served.stop)
  const project = served.run.project!.id
  const signedIn = { Authorization: `Bearer ${served.token}` }
  // Opens a dashboard as its tab does, and every 30 seconds while it is open.
  const open = async (id: string, headers: Record<string, string> = signedIn) => {
    const answer = await fetch(`${served.server.url}/dashboard?${new URLSearchParams({ project, id })}`, { method: "POST", headers })
    return { status: answer.status, opened: answer.ok ? ((await answer.json()) as DashboardOpened) : undefined }
  }
  const page = (path: string, init?: RequestInit) => fetch(`${served.server.url}${path}`, { redirect: "manual", ...init })
  return { served, setting, open, page }
}

const ready = (opened: DashboardOpened | undefined) => {
  if (opened?.status !== "ready") throw new Error(`The dashboard did not open: ${JSON.stringify(opened)}`)
  return opened
}

test("a live dashboard is started when its tab opens it, and its page is reached through a pass of its own", async () => {
  const port = freePort()
  const { served, open, page } = await serveDashboards([{ name: "Cash flow", url: `http://127.0.0.1:${port}/`, start: appCommand(port) }])

  const opened = ready((await open("cash-flow")).opened)
  expect(opened).toEqual({ status: "ready", url: expect.stringMatching(/^\/dashboard\/[\w-]{43}\/$/), started: true })
  expect(await (await page(opened.url)).text()).toBe("ok")

  // Evie stopping stops the app it started.
  await served.stop()
  expect(await untilDown(`http://127.0.0.1:${port}/`)).toBe(true)
})

test("a page that is not signed in cannot open a dashboard", async () => {
  const { open } = await serveDashboards([{ name: "Cash flow", url: runningApp(() => new Response("ok")) }])
  expect((await open("cash-flow", {})).status).toBe(401)
})

test("a dashboard that is not running and has no start command says so", async () => {
  const { open } = await serveDashboards([{ name: "Off", url: `http://127.0.0.1:${freePort()}/` }])
  expect((await open("off")).opened).toEqual({ status: "not_running", message: expect.stringContaining("no start command") })
})

test("a request is passed on to the app without Evie's sign-in, and the app's answer comes back", async () => {
  const url = runningApp(async (request) =>
    Response.json({
      method: request.method,
      path: new URL(request.url).pathname,
      search: new URL(request.url).search,
      cookie: request.headers.get("cookie"),
      authorization: request.headers.get("authorization"),
      body: await request.text(),
    }),
  )
  const { served, open, page } = await serveDashboards([{ name: "Echo", url }])
  const { url: at } = ready((await open("echo")).opened)
  const port = new URL(served.server.url).port

  const answer = await page(`${at}api/echo?n=1`, {
    method: "POST",
    headers: { Authorization: `Bearer ${served.token}`, Cookie: `evie_token_${port}=${served.token}; theme=dark` },
    body: JSON.stringify({ hello: "world" }),
  })
  expect(await answer.json()).toEqual({ method: "POST", path: "/api/echo", search: "?n=1", cookie: "theme=dark", authorization: null, body: '{"hello":"world"}' })
})

test("asking again while the tab is open gives the same pass, working for longer, and it stops working once no one asks", async () => {
  const { open, page } = await serveDashboards([{ name: "Kept", url: runningApp(() => new Response("kept")) }], { passLasts: 400 })
  const first = ready((await open("kept")).opened).url
  for (let at = 0; at < 4; at++) {
    await Bun.sleep(150)
    expect(ready((await open("kept")).opened).url).toBe(first)
  }
  expect((await page(first)).status).toBe(200)
  await Bun.sleep(500)
  expect((await page(first)).status).toBe(404)
})

test("a pass stops working once its device's token is taken back", async () => {
  const { served, open, page } = await serveDashboards([{ name: "Secret", url: runningApp(() => new Response("secret")) }])
  const { url } = ready((await open("secret")).opened)
  expect((await page(url)).status).toBe(200)
  expect(served.tokens.remove("test device")).toBe(true)
  expect((await page(url)).status).toBe(404)
})

test("a pass stops working once its project no longer lists the dashboard, or is no longer trusted", async () => {
  const url = runningApp(() => new Response("ok"))
  const { setting, open, page } = await serveDashboards([{ name: "Listed", url }])
  const first = ready((await open("listed")).opened).url
  setting.now = undefined
  expect((await page(first)).status).toBe(404)

  setting.now = [{ name: "Listed", url }]
  const second = ready((await open("listed")).opened).url
  setting.now = [{ name: "Listed", url: `http://127.0.0.1:${freePort()}/` }]
  expect((await page(second)).status).toBe(404)
})

test("a compressed page from the app reaches the browser as one it can read", async () => {
  const html = "<h1>Cash flow</h1>".repeat(50)
  const url = runningApp(() => new Response(gzipSync(html), { headers: { "Content-Type": "text/html", "Content-Encoding": "gzip" } }))
  const { open, page } = await serveDashboards([{ name: "Zipped", url }])
  const answer = await page(ready((await open("zipped")).opened).url)
  expect(answer.headers.get("content-encoding")).toBeNull()
  expect(await answer.text()).toBe(html)
})

test("a cookie the app sets is not passed back, since it would be Evie's own address's", async () => {
  const url = runningApp(() => new Response("ok", { headers: { "Set-Cookie": "evie_token_3843=forged; Path=/" } }))
  const { open, page } = await serveDashboards([{ name: "Cookies", url }])
  const answer = await page(ready((await open("cookies")).opened).url)
  expect(answer.headers.get("set-cookie")).toBeNull()
})

test("every answer is sandboxed, so a page the app serves runs as no site even when opened on its own", async () => {
  const url = runningApp(() => new Response("<svg xmlns='http://www.w3.org/2000/svg'><script>alert(1)</script></svg>", { headers: { "Content-Type": "image/svg+xml", "Content-Security-Policy": "default-src *" } }))
  const { open, page } = await serveDashboards([{ name: "Drawing", url }])
  const answer = await page(`${ready((await open("drawing")).opened).url}drawing.svg`)
  expect(answer.headers.get("content-security-policy")).toBe("frame-ancestors 'self'; sandbox allow-scripts allow-forms allow-downloads allow-popups")
})

test("a redirect of an app at a sub-path stays under its pass, and one to another site is left as it is", async () => {
  const url = runningApp((request) => {
    const path = new URL(request.url).pathname
    if (path === "/app/") return new Response(null, { status: 302, headers: { Location: "/app/login?next=home" } })
    if (path === "/app/away") return new Response(null, { status: 302, headers: { Location: "https://example.com/" } })
    if (path === "/app/relative") return new Response(null, { status: 302, headers: { Location: "login" } })
    return new Response(`at ${path}`)
  })
  const { open, page } = await serveDashboards([{ name: "Sub", url: `${url}/app/` }])
  const at = ready((await open("sub")).opened).url

  const redirected = await page(at)
  expect(redirected.headers.get("location")).toBe(`${at}login?next=home`)
  expect(await (await page(redirected.headers.get("location")!)).text()).toBe("at /app/login")
  expect((await page(`${at}away`)).headers.get("location")).toBe("https://example.com/")
  expect((await page(`${at}relative`)).headers.get("location")).toBe("login")
})

test("a pass's address without its last slash is sent to the one with it, and a page's preflight is answered", async () => {
  const { open, page } = await serveDashboards([{ name: "Plain", url: runningApp(() => new Response("ok")) }])
  const at = ready((await open("plain")).opened).url

  const bare = await page(`${at.slice(0, -1)}?tab=1`)
  expect(bare.status).toBe(308)
  expect(bare.headers.get("location")).toBe(`${at}?tab=1`)

  const preflight = await page(`${at}api`, { method: "OPTIONS", headers: { "Access-Control-Request-Method": "PUT", "Access-Control-Request-Headers": "content-type" } })
  expect(preflight.status).toBe(204)
  expect(preflight.headers.get("access-control-allow-headers")).toBe("content-type")
})

test("an app that has stopped answering is reached with a plain failure, not Evie's own page", async () => {
  const port = freePort()
  const app = Bun.serve({ hostname: "127.0.0.1", port, fetch: () => new Response("ok") })
  const { open, page } = await serveDashboards([{ name: "Gone", url: `http://127.0.0.1:${port}/` }])
  const { url } = ready((await open("gone")).opened)
  app.stop(true)
  const answer = await page(url)
  expect(answer.status).toBe(502)
  expect(await answer.text()).toContain("not answering")
})
