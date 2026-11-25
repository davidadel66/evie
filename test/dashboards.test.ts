import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { readDashboards } from "../src/core/dashboards.ts"
import { loadSettingsFiles } from "../src/index.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

const cleanups: (() => Promise<void>)[] = []

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string): Promise<string> {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// A project folder with the given files in its `.evie` folder, by their paths there.
async function projectWith(files: Record<string, string> = {}) {
  const folder = await tempFolder("evie-dashboards-project-")
  for (const [path, text] of Object.entries(files)) {
    await mkdir(join(folder, ".evie", path, ".."), { recursive: true })
    await writeFile(join(folder, ".evie", path), text)
  }
  return { folder, dataFolder: await tempFolder("evie-dashboards-home-") }
}

test("a workspace's dashboards are its tabs, in the order listed, each with an id for its address", async () => {
  const { folder, dataFolder } = await projectWith({ "dashboards/cash.html": "<h1>cash</h1>", "report.html": "<h1>report</h1>" })
  const entries = await readDashboards(
    [
      { name: "Cash Flow", url: "http://127.0.0.1:4101/", start: " bun run dev " },
      { name: "Cash Flow", path: "dashboards/cash.html" },
      { id: "metrics", name: "Its Metrics", url: "http://[::1]:4103/app" },
      { name: "!!!", path: "report.html" },
      { name: "Local", url: "http://localhost:4104" },
    ],
    folder,
    dataFolder,
  )
  expect(entries).toEqual([
    { id: "cash-flow", name: "Cash Flow", kind: "live", url: "http://127.0.0.1:4101/", start: "bun run dev" },
    { id: "cash-flow-2", name: "Cash Flow", kind: "static", path: "dashboards/cash.html" },
    { id: "metrics", name: "Its Metrics", kind: "live", url: "http://[::1]:4103/app" },
    { id: "dashboard", name: "!!!", kind: "static", path: "report.html" },
    { id: "local", name: "Local", kind: "live", url: "http://localhost:4104" },
  ])
})

test("an entry that cannot be used keeps its tab, which says what to fix", async () => {
  const outside = await tempFolder("evie-dashboards-outside-")
  await writeFile(join(outside, "secret.html"), "<p>secret</p>")
  const { folder, dataFolder } = await projectWith({ "ok.html": "<p>ok</p>", ".git/config": "[core]\n" })
  await symlink(join(outside, "secret.html"), join(folder, ".evie", "escape.html"))

  const entries = await readDashboards(
    [
      { name: "Remote", url: "https://example.com" },
      { name: "File", url: "file:///etc/passwd" },
      { name: "Up", path: "../secret.html" },
      { name: "Git", path: ".git/config" },
      { name: "Escape", path: "escape.html" },
      { name: "Missing", path: "missing.html" },
      { url: "http://127.0.0.1:4100" },
      { name: "Both", url: "http://127.0.0.1:4100", path: "ok.html" },
      { name: "Neither" },
      { name: "Empty start", url: "http://127.0.0.1:4100", start: "  " },
      "not an object",
    ],
    folder,
    dataFolder,
  )
  expect(entries.map((entry) => [entry.id, entry.kind])).toEqual([
    ["remote", "invalid"],
    ["file", "invalid"],
    ["up", "invalid"],
    ["git", "invalid"],
    ["escape", "invalid"],
    ["missing", "invalid"],
    ["dashboard", "invalid"],
    ["both", "invalid"],
    ["neither", "invalid"],
    ["empty-start", "invalid"],
    ["dashboard-2", "invalid"],
  ])
  expect(entries[0]).toMatchObject({ error: expect.stringContaining("not an address on this computer") })
  expect(entries[5]).toMatchObject({ error: expect.stringContaining('There is no file "missing.html"') })
  expect(entries[6]).toMatchObject({ error: expect.stringContaining('"name"') })
})

test("a dashboards setting that is not a list is one tab that says so, and none set is no tabs", async () => {
  const { folder, dataFolder } = await projectWith()
  expect(await readDashboards({ name: "Not a list" }, folder, dataFolder)).toEqual([{ id: "dashboards", name: "Dashboards", kind: "invalid", error: 'In .evie/evie.json, "dashboards" must be a list.' }])
  expect(await readDashboards(undefined, folder, dataFolder)).toEqual([])
})

test("a static dashboard cannot be a file in a .evie folder that is Evie's own data folder", async () => {
  const { folder } = await projectWith({ "index.html": "<h1>hi</h1>" })
  const [entry] = await readDashboards([{ name: "Overlap", path: "index.html" }], folder, join(folder, ".evie"))
  expect(entry).toMatchObject({ id: "overlap", kind: "invalid" })
})

test("a project's dashboards are read from its settings file only while it is trusted, and a broken file says why", async () => {
  const home = await tempFolder("evie-dashboards-home-")
  const { folder } = await projectWith({ "evie.json": JSON.stringify({ dashboards: [{ name: "Overview", url: "http://127.0.0.1:4100" }] }) })
  const project = { id: "an-id", name: "a-name", folder }
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: home } })
  expect(await settings.dashboardsSetting(project)).toBeUndefined()

  await settings.trust(folder)
  expect(await settings.dashboardsSetting(project)).toEqual([{ name: "Overview", url: "http://127.0.0.1:4100" }])

  await writeFile(join(folder, ".evie", "evie.json"), "{ not json")
  await expect(settings.dashboardsSetting(project)).rejects.toThrow("is not valid JSON")
})

test("how long a live dashboard's app may go unused, and how many run at once, are set in the global file", async () => {
  const home = await tempFolder("evie-dashboards-home-")
  await writeFile(join(home, "evie.json"), JSON.stringify({ liveDashboards: { idleMinutes: 30, mostRunning: 5 } }))
  expect((await loadSettingsFiles({ env: { EVIE_HOME: home } })).liveDashboards).toEqual({ idleMinutes: 30, mostRunning: 5 })

  await writeFile(join(home, "evie.json"), JSON.stringify({ liveDashboards: { mostRunning: 0 } }))
  await expect(loadSettingsFiles({ env: { EVIE_HOME: home } })).rejects.toThrow('"liveDashboards" must be')
})

test("a page lists a workspace's dashboards over its connection, and a broken settings file is a failure that says why", async () => {
  const setting: { now: () => unknown } = { now: () => [{ name: "Overview", url: "http://127.0.0.1:4100" }] }
  const served = await serveEvie([scriptedProvider([]).plugin], { dashboardsSetting: async () => setting.now() })
  cleanups.push(served.stop)
  const client = await served.connect()
  const project = served.run.project!.id
  expect(await client.send({ type: "list_dashboards", project })).toEqual({ dashboards: [{ id: "overview", name: "Overview", kind: "live", url: "http://127.0.0.1:4100" }] })

  setting.now = () => {
    throw new Error("evie.json is not valid JSON")
  }
  await expect(client.send({ type: "list_dashboards", project })).rejects.toThrow("is not valid JSON")
})
