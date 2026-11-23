import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { readDashboards } from "../src/core/dashboards.ts"
import { createDashboardRunner, type DashboardRunner } from "../src/core/dashboard-runner.ts"

const cleanups: (() => Promise<void>)[] = []

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string): Promise<string> {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

async function setupProject(config?: unknown): Promise<{ projectFolder: string; dataFolder: string; evieFolder: string }> {
  const projectFolder = await tempFolder("evie-dash-proj-")
  const dataFolder = await tempFolder("evie-dash-data-")
  const evieFolder = join(projectFolder, ".evie")
  if (config !== undefined) {
    await mkdir(evieFolder, { recursive: true })
    const raw = typeof config === "string" ? config : JSON.stringify(config, null, 2)
    await writeFile(join(evieFolder, "evie.json"), raw)
  }
  return { projectFolder, dataFolder, evieFolder }
}

// Reserve an unused ephemeral TCP port on 127.0.0.1.
function freePort(): number {
  const server = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("ok") })
  const port = server.port!
  server.stop(true)
  return port
}

// A tiny Bun one-liner that listens on `port` after an optional delay.
function serverCommand(port: number, delayMs = 0): string {
  const start = `Bun.serve({ hostname: "127.0.0.1", port: ${port}, fetch: () => new Response("ok") });`
  const body = delayMs > 0 ? `setTimeout(() => { ${start} }, ${delayMs});` : start
  return `${JSON.stringify(process.execPath)} -e ${JSON.stringify(body)}`
}

async function isListening(url: string): Promise<boolean> {
  try {
    const res = await fetch(url, { method: "GET", redirect: "manual", signal: AbortSignal.timeout(500) })
    await res.body?.cancel().catch(() => {})
    return true
  } catch {
    return false
  }
}

async function waitForDown(url: string, timeoutMs = 2000): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    if (!(await isListening(url))) return true
    await Bun.sleep(25)
  }
  return !(await isListening(url))
}

function makeRunner(logFolder: string, overrides: Partial<Parameters<typeof createDashboardRunner>[0]> = {}): DashboardRunner {
  const runner = createDashboardRunner({
    logFolder,
    probeIntervalMs: 25,
    startTimeoutMs: 3000,
    idleSweepMs: 500,
    ...overrides,
  })
  cleanups.push(() => runner.close())
  return runner
}

// ---- readDashboards -----------------------------------------------------------------------

test("untrusted project returns [] even with valid evie.json", async () => {
  const { projectFolder, dataFolder } = await setupProject({
    dashboards: [{ name: "Overview", url: "http://127.0.0.1:4100" }],
  })
  expect(await readDashboards(projectFolder, dataFolder, false)).toEqual([])
})

test("missing evie.json, invalid JSON, or missing dashboards field returns []", async () => {
  const missing = await setupProject()
  expect(await readDashboards(missing.projectFolder, missing.dataFolder, true)).toEqual([])

  const badJson = await setupProject("{ not json")
  expect(await readDashboards(badJson.projectFolder, badJson.dataFolder, true)).toEqual([])

  const noDashboards = await setupProject({ sync: "private" })
  expect(await readDashboards(noDashboards.projectFolder, noDashboards.dataFolder, true)).toEqual([])

  const rootArray = await setupProject([{ name: "Nope", url: "http://127.0.0.1:4100" }])
  expect(await readDashboards(rootArray.projectFolder, rootArray.dataFolder, true)).toEqual([])
})

test("valid live and static entries preserve order, slugify, deduplicate, and respect explicit id", async () => {
  const { projectFolder, dataFolder, evieFolder } = await setupProject({
    dashboards: [
      { name: "Cash Flow", url: "http://127.0.0.1:4101", start: "bun run dev", idle: 15 },
      { name: "Cash Flow", path: "dashboards/cash.html" },
      { name: "Cash Flow", url: "http://localhost:4102", idle: "never" },
      { id: "custom-id", name: "Ignored For Slug", url: "http://[::1]:4103/app" },
      { name: "!!!", path: "dashboards/report.html" },
    ],
  })
  await mkdir(join(evieFolder, "dashboards"), { recursive: true })
  await writeFile(join(evieFolder, "dashboards", "cash.html"), "<h1>cash</h1>")
  await writeFile(join(evieFolder, "dashboards", "report.html"), "<h1>report</h1>")

  const entries = await readDashboards(projectFolder, dataFolder, true)
  expect(entries).toEqual([
    { id: "cash-flow", name: "Cash Flow", kind: "live", url: "http://127.0.0.1:4101", start: "bun run dev", idle: 15 },
    { id: "cash-flow-2", name: "Cash Flow", kind: "static", path: "dashboards/cash.html" },
    { id: "cash-flow-3", name: "Cash Flow", kind: "live", url: "http://localhost:4102", idle: "never" },
    { id: "custom-id", name: "Ignored For Slug", kind: "live", url: "http://[::1]:4103/app" },
    { id: "dashboard", name: "!!!", kind: "static", path: "dashboards/report.html" },
  ])
})

test("invalid entries become kind: invalid with clear errors and never throw", async () => {
  const outside = await tempFolder("evie-dash-outside-")
  await writeFile(join(outside, "secret.html"), "<p>secret</p>")

  const { projectFolder, dataFolder, evieFolder } = await setupProject({
    dashboards: [
      { name: "Remote", url: "https://example.com" },
      { name: "File Scheme", url: "file:///etc/passwd" },
      { name: "Traversal", path: "../secret.html" },
      { name: "Git File", path: ".git/config" },
      { name: "Symlink Escape", path: "escape.html" },
      { name: "Missing File", path: "dashboards/missing.html" },
      { url: "http://127.0.0.1:4100" },
      { name: "Both", url: "http://127.0.0.1:4100", path: "dashboards/ok.html" },
      { name: "Neither" },
      { name: "Negative Idle", url: "http://127.0.0.1:4100", idle: -1 },
      { name: "Bad Idle String", url: "http://127.0.0.1:4100", idle: "soon" },
      { name: "Empty Start", url: "http://127.0.0.1:4100", start: "   " },
      "not-an-object",
    ],
  })
  await mkdir(join(evieFolder, ".git"), { recursive: true })
  await writeFile(join(evieFolder, ".git", "config"), "[core]\n")
  await mkdir(join(evieFolder, "dashboards"), { recursive: true })
  await writeFile(join(evieFolder, "dashboards", "ok.html"), "<p>ok</p>")
  await symlink(join(outside, "secret.html"), join(evieFolder, "escape.html"))

  const entries = await readDashboards(projectFolder, dataFolder, true)
  expect(entries).toHaveLength(13)
  for (const entry of entries) {
    expect(entry.kind).toBe("invalid")
    if (entry.kind === "invalid") {
      expect(entry.error.length).toBeGreaterThan(0)
    }
  }
  expect(entries[0]).toMatchObject({ id: "remote", name: "Remote", kind: "invalid" })
  expect(entries[1]).toMatchObject({ id: "file-scheme", name: "File Scheme", kind: "invalid" })
  expect(entries[6]).toMatchObject({ name: "Dashboard", kind: "invalid" })
  expect(entries[12]).toMatchObject({ name: "Dashboard", kind: "invalid" })
  expect(entries[6]!.id).not.toBe(entries[12]!.id)
})

test("non-array dashboards field returns a single invalid entry", async () => {
  const { projectFolder, dataFolder } = await setupProject({
    dashboards: { name: "Not A List", url: "http://127.0.0.1:4100" },
  })
  expect(await readDashboards(projectFolder, dataFolder, true)).toEqual([
    {
      id: "dashboards",
      name: "Dashboards",
      kind: "invalid",
      error: 'In .evie/evie.json, "dashboards" must be a list.',
    },
  ])
})

// ---- createDashboardRunner ----------------------------------------------------------------

test("adopts an already-listening server without spawning and leaves it alive after close()", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const external = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("external") })
  cleanups.push(async () => {
    external.stop(true)
  })
  const url = `http://127.0.0.1:${external.port}`

  const runner = makeRunner(logFolder)
  const res = await runner.ensure({
    projectId: "proj-1",
    projectFolder,
    dashboardId: "ext",
    url,
    start: "echo should-not-run && exit 1",
  })

  expect(res).toEqual({ ok: true })
  expect(runner.runningCount()).toBe(0)

  await runner.close()
  expect(await isListening(url)).toBe(true)
})

test("returns not_running when stopped and no start command", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const port = freePort()
  const runner = makeRunner(logFolder)

  const res = await runner.ensure({
    projectId: "proj-1",
    projectFolder,
    dashboardId: "stopped",
    url: `http://127.0.0.1:${port}`,
  })

  expect(res).toEqual({
    ok: false,
    status: "not_running",
    message: "This dashboard is not running and has no start command.",
  })
  expect(runner.runningCount()).toBe(0)
})

test("starts a stopped server on demand, probes until ready, and stops it on close()", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const port = freePort()
  const url = `http://127.0.0.1:${port}`
  const runner = makeRunner(logFolder)

  const res = await runner.ensure({
    projectId: "proj-1",
    projectFolder,
    dashboardId: "app",
    url,
    start: serverCommand(port, 80),
  })

  expect(res).toEqual({ ok: true })
  expect(runner.runningCount()).toBe(1)
  expect(await isListening(url)).toBe(true)

  await runner.close()
  expect(runner.runningCount()).toBe(0)
  expect(await waitForDown(url)).toBe(true)
})

test("coalesces concurrent ensure() calls for the same app into 1 process", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const port = freePort()
  const url = `http://127.0.0.1:${port}`
  const markerFile = join(projectFolder, "starts.log")
  const start = `echo started >> ${JSON.stringify(markerFile)} && exec ${serverCommand(port, 100)}`
  const runner = makeRunner(logFolder)

  const spec = { projectId: "proj-1", projectFolder, dashboardId: "shared", url, start }
  const results = await Promise.all([runner.ensure(spec), runner.ensure(spec), runner.ensure(spec)])

  expect(results).toEqual([{ ok: true }, { ok: true }, { ok: true }])
  expect(runner.runningCount()).toBe(1)
  const lines = (await Bun.file(markerFile).text()).trim().split("\n")
  expect(lines).toHaveLength(1)
})

test("early crash returns failed with stderr text in log", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const port = freePort()
  const runner = makeRunner(logFolder)

  const res = await runner.ensure({
    projectId: "proj-1",
    projectFolder,
    dashboardId: "crashy",
    url: `http://127.0.0.1:${port}`,
    start: "echo 'boom: missing DATABASE_URL' >&2; exit 2",
  })

  expect(res.ok).toBe(false)
  if (!res.ok) {
    expect(res.status).toBe("failed")
    expect(res.log).toContain("boom: missing DATABASE_URL")
  }
  expect(runner.runningCount()).toBe(0)
})

test("start timeout kills process group and returns failed", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const port = freePort()
  const runner = makeRunner(logFolder, { startTimeoutMs: 200, probeIntervalMs: 30 })

  const res = await runner.ensure({
    projectId: "proj-1",
    projectFolder,
    dashboardId: "slow",
    url: `http://127.0.0.1:${port}`,
    start: "echo 'still booting...' >&2; sleep 30",
  })

  expect(res.ok).toBe(false)
  if (!res.ok) {
    expect(res.status).toBe("failed")
    expect(res.message).toBe("This dashboard did not answer in time.")
    expect(res.log).toContain("still booting...")
  }
  expect(runner.runningCount()).toBe(0)
})

test("LRU cap evicts and stops the oldest running server when maxRunning is exceeded", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const p1 = freePort()
  const p2 = freePort()
  const p3 = freePort()
  const runner = makeRunner(logFolder, { maxRunning: 2 })

  const s1 = { projectId: "p", projectFolder, dashboardId: "d1", url: `http://127.0.0.1:${p1}`, start: serverCommand(p1) }
  const s2 = { projectId: "p", projectFolder, dashboardId: "d2", url: `http://127.0.0.1:${p2}`, start: serverCommand(p2) }
  const s3 = { projectId: "p", projectFolder, dashboardId: "d3", url: `http://127.0.0.1:${p3}`, start: serverCommand(p3) }

  expect(await runner.ensure(s1)).toEqual({ ok: true })
  await Bun.sleep(20)
  expect(await runner.ensure(s2)).toEqual({ ok: true })
  await Bun.sleep(20)
  expect(await runner.ensure(s3)).toEqual({ ok: true })

  expect(runner.runningCount()).toBe(2)
  expect(await waitForDown(s1.url)).toBe(true)
  expect(await isListening(s2.url)).toBe(true)
  expect(await isListening(s3.url)).toBe(true)
})

test("idle sweep stops untouched server, touch() keeps it alive, and idle: 'never' stays alive", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const pIdle = freePort()
  const pTouched = freePort()
  const pNever = freePort()
  const runner = makeRunner(logFolder, { defaultIdleMs: 200, idleSweepMs: 40, maxRunning: 5 })

  const sIdle = { projectId: "p", projectFolder, dashboardId: "idle", url: `http://127.0.0.1:${pIdle}`, start: serverCommand(pIdle) }
  const sTouched = { projectId: "p", projectFolder, dashboardId: "touched", url: `http://127.0.0.1:${pTouched}`, start: serverCommand(pTouched) }
  const sNever = { projectId: "p", projectFolder, dashboardId: "never", url: `http://127.0.0.1:${pNever}`, start: serverCommand(pNever), idle: "never" as const }

  expect(await runner.ensure(sIdle)).toEqual({ ok: true })
  expect(await runner.ensure(sTouched)).toEqual({ ok: true })
  expect(await runner.ensure(sNever)).toEqual({ ok: true })
  expect(runner.runningCount()).toBe(3)

  // Keep sTouched alive past sIdle's timeout while leaving sIdle untouched.
  for (let i = 0; i < 4; i++) {
    await Bun.sleep(80)
    runner.touch(projectFolder, sTouched.start)
  }

  expect(await waitForDown(sIdle.url)).toBe(true)
  expect(await isListening(sTouched.url)).toBe(true)
  expect(await isListening(sNever.url)).toBe(true)
  expect(runner.runningCount()).toBe(2)
})

test("LRU cap prefers evicting an idle-able server over an older idle: 'never' server", async () => {
  const logFolder = await tempFolder("evie-dash-logs-")
  const projectFolder = await tempFolder("evie-dash-work-")
  const pNever = freePort()
  const pNormal = freePort()
  const pNew = freePort()
  const runner = makeRunner(logFolder, { maxRunning: 2 })

  const sNever = { projectId: "p", projectFolder, dashboardId: "never", url: `http://127.0.0.1:${pNever}`, start: serverCommand(pNever), idle: "never" as const }
  const sNormal = { projectId: "p", projectFolder, dashboardId: "normal", url: `http://127.0.0.1:${pNormal}`, start: serverCommand(pNormal) }
  const sNew = { projectId: "p", projectFolder, dashboardId: "new", url: `http://127.0.0.1:${pNew}`, start: serverCommand(pNew) }

  expect(await runner.ensure(sNever)).toEqual({ ok: true })
  await Bun.sleep(20)
  expect(await runner.ensure(sNormal)).toEqual({ ok: true })
  await Bun.sleep(20)
  expect(await runner.ensure(sNew)).toEqual({ ok: true })

  expect(runner.runningCount()).toBe(2)
  expect(await isListening(sNever.url)).toBe(true)
  expect(await waitForDown(sNormal.url)).toBe(true)
  expect(await isListening(sNew.url)).toBe(true)
})

test("readDashboards rejects static path when .evie overlaps dataFolder", async () => {
  const projectFolder = await tempFolder("evie-dash-overlap-")
  const evieFolder = join(projectFolder, ".evie")
  await mkdir(evieFolder, { recursive: true })
  await writeFile(join(evieFolder, "index.html"), "<h1>hi</h1>")
  await writeFile(
    join(evieFolder, "evie.json"),
    JSON.stringify({ dashboards: [{ name: "Overlap", path: "index.html" }] }),
  )

  const entries = await readDashboards(projectFolder, evieFolder, true)
  expect(entries).toHaveLength(1)
  expect(entries[0]).toMatchObject({ id: "overlap", name: "Overlap", kind: "invalid" })
})
