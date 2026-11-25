import { afterEach, expect, test } from "bun:test"
import { startServer } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { afterOpening, openDashboards, type Dashboards, type DashboardsState, type LiveShown } from "../web/src/dashboards.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// A workspace's dashboards as the page keeps them, apart from its screen,
// against Evie's real server.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function until(dashboards: Dashboards, ready: (state: DashboardsState) => boolean) {
  while (!ready(dashboards.state())) await Bun.sleep(5)
  return dashboards.state()
}

test("a workspace's dashboards are listed once the page is connected, and again each time the line comes back", async () => {
  const setting = { now: [{ name: "Cash flow", url: "http://127.0.0.1:4199" }] as unknown }
  const served = await serveEvie([scriptedProvider([]).plugin], { dashboardsSetting: async () => setting.now })
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(async () => connection.close())
  const dashboards = openDashboards(connection, served.run.project!.id)
  cleanups.push(async () => dashboards.close())

  expect((await until(dashboards, (state) => state.listed)).dashboards.map((entry) => entry.id)).toEqual(["cash-flow"])

  // The owner edits the list while the line is down.
  const port = Number(new URL(served.server.url).port)
  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  setting.now = [{ name: "Cash flow", url: "http://127.0.0.1:4199" }, { name: "Budget", url: "http://127.0.0.1:4200" }]
  const restarted = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => restarted.stop())

  expect((await until(dashboards, (state) => state.dashboards.length === 2)).dashboards.map((entry) => entry.id)).toEqual(["cash-flow", "budget"])
})

test("dashboards that could not be listed say why", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin], {
    dashboardsSetting: async () => {
      throw new Error(".evie/evie.json is not valid JSON")
    },
  })
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(async () => connection.close())
  const dashboards = openDashboards(connection, served.run.project!.id)
  cleanups.push(async () => dashboards.close())

  expect(await until(dashboards, (state) => state.listed)).toEqual({ listed: true, dashboards: [], problem: expect.stringContaining("not valid JSON") })
})

test("a live dashboard's page stays as it is while its app runs, and is loaded again once its app had to be started again", () => {
  const opening: LiveShown = { status: "opening" }
  const shown = afterOpening(opening, { status: "ready", url: "/dashboard/pass/", started: true })
  expect(shown).toEqual({ status: "ready", url: "/dashboard/pass/", load: 0 })

  // Asked for again every 30 seconds while the app keeps running.
  expect(afterOpening(shown, { status: "ready", url: "/dashboard/pass/", started: false })).toBe(shown)
  // Its app stopped, as when it crashed or made room for another, and was started again.
  expect(afterOpening(shown, { status: "ready", url: "/dashboard/pass/", started: true })).toEqual({ status: "ready", url: "/dashboard/pass/", load: 1 })
  // Its pass had stopped working, as after the laptop slept, and a new one was given.
  expect(afterOpening(shown, { status: "ready", url: "/dashboard/other/", started: false })).toEqual({ status: "ready", url: "/dashboard/other/", load: 1 })
  // Its app could not be started again.
  expect(afterOpening(shown, { status: "failed", message: "It stopped." })).toEqual({ status: "failed", message: "It stopped." })
  // evie's server could not be reached for a moment: the page drawn may still work.
  expect(afterOpening(shown, { status: "unreachable", message: "evie's server could not be reached." })).toBe(shown)
  expect(afterOpening(opening, { status: "unreachable", message: "evie's server could not be reached." })).toEqual({ status: "failed", message: "evie's server could not be reached." })
})
