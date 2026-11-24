import { afterEach, expect, test } from "bun:test"
import { mkdir, realpath, writeFile } from "node:fs/promises"
import { join } from "node:path"
import { startServer } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { dashboardPreviewMintUrl, openDashboard, openDashboards, startDashboardHeartbeat, type Dashboards, type DashboardsState } from "../web/src/dashboards.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The workspace dashboards loader and live heartbeat, apart from their screen,
// run against Evie's real server over a token-authenticated connection.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function writeProjectConfig(projectFolder: string, config: unknown) {
  const evieDir = join(await realpath(projectFolder), ".evie")
  await mkdir(evieDir, { recursive: true })
  await writeFile(join(evieDir, "evie.json"), JSON.stringify(config, null, 2))
  return evieDir
}

async function until(dashboards: Dashboards, ready: (state: DashboardsState) => boolean) {
  while (!ready(dashboards.state())) await Bun.sleep(5)
  return dashboards.state()
}

test("openDashboards loads dashboards for a trusted project, returns empty for untrusted, and re-lists on reconnect", async () => {
  const trusted = await serveEvie([scriptedProvider([]).plugin], { isTrusted: () => true })
  cleanups.push(trusted.stop)
  const evieDir = await writeProjectConfig(trusted.run.projectFolder, {
    dashboards: [
      { name: "Overview", path: "dashboards/overview.html" },
      { name: "Cash Flow", url: "http://127.0.0.1:4199" },
    ],
  })
  await mkdir(join(evieDir, "dashboards"), { recursive: true })
  await writeFile(join(evieDir, "dashboards", "overview.html"), "<h1>Overview</h1>")

  const connection = openConnection({ url: trusted.server.url, token: trusted.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  const project = trusted.run.project!.id

  const dashboards = openDashboards(connection, project)
  cleanups.push(async () => dashboards.close())

  let notifications = 0
  const unsub = dashboards.subscribe(() => notifications++)
  cleanups.push(async () => unsub())

  const initial = await until(dashboards, (s) => s.listed)
  expect(notifications).toBeGreaterThan(0)
  expect(initial.problem).toBeUndefined()
  expect(initial.dashboards.map((d) => ({ id: d.id, kind: d.kind }))).toEqual([
    { id: "overview", kind: "static" },
    { id: "cash-flow", kind: "live" },
  ])

  // Static preview mint URL points at /preview with project and path.
  expect(dashboardPreviewMintUrl(project, "dashboards/overview.html")).toBe(
    `/preview?${new URLSearchParams({ project, path: "dashboards/overview.html" })}`,
  )

  // Drop the server, edit evie.json, restart on the same port, and check reconnect re-lists.
  const port = Number(new URL(trusted.server.url).port)
  await trusted.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)

  await writeProjectConfig(trusted.run.projectFolder, {
    dashboards: [
      { name: "Overview", path: "dashboards/overview.html" },
      { name: "Budget", path: "dashboards/budget.html" },
    ],
  })
  const restarted = startServer({ evie: trusted.run.evie, tokens: trusted.tokens, host: "127.0.0.1", port })
  cleanups.push(() => restarted.stop())

  const relisted = await until(dashboards, (s) => s.dashboards.some((d) => d.id === "budget"))
  expect(relisted.dashboards.map((d) => d.id)).toEqual(["overview", "budget"])

  // Untrusted project yields an empty list.
  const untrusted = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(untrusted.stop)
  await writeProjectConfig(untrusted.run.projectFolder, {
    dashboards: [{ name: "Overview", path: "dashboards/overview.html" }],
  })
  const untrustedConn = openConnection({ url: untrusted.server.url, token: untrusted.token, firstWait: 10 })
  cleanups.push(() => untrustedConn.close())
  const untrustedDash = openDashboards(untrustedConn, untrusted.run.project!.id)
  cleanups.push(async () => untrustedDash.close())

  const empty = await until(untrustedDash, (s) => s.listed)
  expect(empty.dashboards).toEqual([])
})

test("startDashboardHeartbeat keeps a live pass alive with touch_dashboard and calls onExpired when { ok: false }", async () => {
  const upstream = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch: () => new Response("<h1>Live</h1>"),
  })
  cleanups.push(async () => upstream.stop(true))

  const served = await serveEvie([scriptedProvider([]).plugin], {
    isTrusted: () => true,
    dashboardPassLasts: 160,
  })
  cleanups.push(served.stop)
  await writeProjectConfig(served.run.projectFolder, {
    dashboards: [{ name: "Alive", url: `http://127.0.0.1:${upstream.port}` }],
  })

  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  while (connection.state().status !== "connected") await Bun.sleep(5)

  const project = served.run.project!.id
  const opened = await openDashboard(connection, project, "alive")
  expect(opened?.kind).toBe("live")
  if (!opened || opened.kind !== "live" || opened.status !== "ready" || !opened.passId) {
    throw new Error("expected live ready dashboard")
  }

  // Heartbeat running faster than the 160ms TTL keeps the pass valid past the original TTL.
  let expiredCount = 0
  const hb = startDashboardHeartbeat(connection, { project, id: "alive", passId: opened.passId }, () => expiredCount++, 50)
  cleanups.push(async () => hb.stop())

  await Bun.sleep(240)
  expect(expiredCount).toBe(0)
  expect((await fetch(`${served.server.url}${opened.url}`)).status).toBe(200)
  hb.stop()

  // With a bogus/expired passId, touch_dashboard returns { ok: false } and invokes onExpired once.
  let expiredFired = 0
  const expiredHb = startDashboardHeartbeat(connection, { project, id: "alive", passId: "expired-pass-id" }, () => expiredFired++, 25)
  cleanups.push(async () => expiredHb.stop())

  const deadline = Date.now() + 1000
  while (expiredFired === 0 && Date.now() < deadline) await Bun.sleep(10)
  expect(expiredFired).toBe(1)
  // Stays 1 because the heartbeat stops itself after expiry.
  await Bun.sleep(70)
  expect(expiredFired).toBe(1)
})
