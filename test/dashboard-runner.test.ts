import { afterEach, expect, test } from "bun:test"
import { mkdtemp, readdir, readFile, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createDashboardRunner, type DashboardRunnerOptions, type LiveApp } from "../src/core/dashboard-runner.ts"
import { answers, appCommand, freePort, untilDown } from "./support/dashboard-apps.ts"

const cleanups: (() => Promise<void>)[] = []

afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string): Promise<string> {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// A runner with its own log folder, closed after the test, and an app for
// it in a folder of its own.
async function setUp(options: Partial<DashboardRunnerOptions> = {}) {
  const logFolder = join(await tempFolder("evie-dashboard-logs-"), "logs")
  const runner = createDashboardRunner({ logFolder, ...options })
  cleanups.push(() => runner.close())
  const folder = await tempFolder("evie-dashboard-app-")
  const app = (dashboard: string, start?: string | ((port: number) => string)): LiveApp & { port: number } => {
    const port = freePort()
    return {
      port,
      project: "project",
      folder,
      dashboard,
      url: `http://127.0.0.1:${port}/`,
      ...(start !== undefined && { start: typeof start === "string" ? start : start(port) }),
    }
  }
  return { runner, folder, app, logFolder }
}

const running = new AbortController().signal

test("an app already answering at its address is used as it is, and left running when Evie stops", async () => {
  const { runner, app } = await setUp()
  const theirs = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("theirs") })
  cleanups.push(async () => void theirs.stop(true))

  const opened = await runner.ensure({ ...app("theirs", "exit 1"), url: `http://127.0.0.1:${theirs.port}/` }, running)
  expect(opened).toEqual({ ok: true, started: false })
  await runner.close()
  expect(await answers(`http://127.0.0.1:${theirs.port}/`)).toBe(true)
})

test("an app that is not answering and has no start command is said to be not running", async () => {
  const { runner, app } = await setUp()
  const opened = await runner.ensure(app("off"), running)
  expect(opened).toEqual({ ok: false, status: "not_running", message: expect.stringContaining("no start command") })
})

test("an app is started when its tab asks for it, and stopped when Evie stops", async () => {
  const { runner, app } = await setUp()
  const started = app("started", (port) => appCommand(port, 100))

  expect(await runner.ensure(started, running)).toEqual({ ok: true, started: true })
  expect(await answers(started.url)).toBe(true)
  // Asked again while it runs, it is the same app.
  expect(await runner.ensure(started, running)).toEqual({ ok: true, started: false })
  await runner.close()
  expect(await untilDown(started.url)).toBe(true)
})

test("an app whose start command runs it in the background is still Evie's to stop", async () => {
  const { runner, app } = await setUp()
  const backgrounded = app("backgrounded", (port) => `${appCommand(port)} &`)

  expect(await runner.ensure(backgrounded, running)).toEqual({ ok: true, started: true })
  // Its shell has ended, but the app is still counted as running, so it is not started again.
  await Bun.sleep(200)
  expect(await runner.ensure(backgrounded, running)).toEqual({ ok: true, started: false })
  await runner.close()
  expect(await untilDown(backgrounded.url)).toBe(true)
})

test("tabs asking at once start an app once", async () => {
  const { runner, folder, app } = await setUp()
  const marker = join(folder, "starts")
  const shared = app("shared", (port) => `echo started >> ${JSON.stringify(marker)}; exec ${appCommand(port, 100)}`)

  const opened = await Promise.all([runner.ensure(shared, running), runner.ensure(shared, running), runner.ensure(shared, running)])
  expect(opened.every((each) => each.ok)).toBe(true)
  expect((await readFile(marker, "utf8")).trim().split("\n")).toEqual(["started"])
})

test("an app that stops before it answers fails with the end of what it printed", async () => {
  const { runner, app } = await setUp()
  const opened = await runner.ensure(app("crashes", "echo 'missing DATABASE_URL' >&2; exit 2"), running)
  expect(opened).toEqual({ ok: false, status: "failed", message: expect.stringContaining("exit status 2"), log: expect.stringContaining("missing DATABASE_URL") })
})

test("an app that does not answer in time is stopped, and fails with what it printed", async () => {
  const { runner, folder, app } = await setUp({ startWithin: 300 })
  const pidFile = join(folder, "pid")
  const opened = await runner.ensure(app("slow", `echo $$ > ${JSON.stringify(pidFile)}; echo 'still booting' >&2; sleep 30`), running)
  expect(opened).toEqual({ ok: false, status: "failed", message: expect.stringContaining("within 0.3 seconds"), log: expect.stringContaining("still booting") })
  const pid = Number((await readFile(pidFile, "utf8")).trim())
  const gone = () => {
    try {
      process.kill(pid, 0)
      return false
    } catch {
      return true
    }
  }
  const until = Date.now() + 3000
  while (!gone() && Date.now() < until) await Bun.sleep(25)
  expect(gone()).toBe(true)
})

test("starting another app past the most allowed stops the one asked for longest ago", async () => {
  const { runner, app } = await setUp({ mostRunning: 2 })
  const [first, second, third] = ["first", "second", "third"].map((name) => app(name, (port) => appCommand(port)))

  expect((await runner.ensure(first!, running)).ok).toBe(true)
  expect((await runner.ensure(second!, running)).ok).toBe(true)
  // Asking for the first again makes the second the one asked for longest ago.
  expect(await runner.ensure(first!, running)).toEqual({ ok: true, started: false })
  expect((await runner.ensure(third!, running)).ok).toBe(true)

  expect(await untilDown(second!.url)).toBe(true)
  expect(await answers(first!.url)).toBe(true)
  expect(await answers(third!.url)).toBe(true)
})

test("apps started at once never run more than the most allowed", async () => {
  const { runner, app } = await setUp({ mostRunning: 2 })
  const apps = ["one", "two", "three", "four"].map((name) => app(name, (port) => appCommand(port, 50)))

  await Promise.all(apps.map((each) => runner.ensure(each, running)))
  await Bun.sleep(300)
  const up = await Promise.all(apps.map((each) => answers(each.url)))
  expect(up.filter(Boolean)).toHaveLength(2)
})

test("an app no tab has asked for in its idle time is stopped, and one asked for stays", async () => {
  const { runner, app } = await setUp({ idleMinutes: 0.005 })
  const left = app("left", (port) => appCommand(port))
  const asked = app("asked", (port) => appCommand(port))
  expect((await runner.ensure(left, running)).ok).toBe(true)
  expect((await runner.ensure(asked, running)).ok).toBe(true)

  for (let at = 0; at < 6; at++) {
    await Bun.sleep(100)
    expect((await runner.ensure(asked, running)).ok).toBe(true)
  }
  expect(await untilDown(left.url, 100)).toBe(true)
  expect(await answers(asked.url)).toBe(true)
})

test("an app that stopped is started again the next time its tab asks, and the answer says so", async () => {
  const { runner, folder, app } = await setUp()
  const pidFile = join(folder, "pid")
  const dies = app("dies", (port) => `echo $$ > ${JSON.stringify(pidFile)}; exec ${appCommand(port)}`)
  expect(await runner.ensure(dies, running)).toEqual({ ok: true, started: true })

  process.kill(Number((await readFile(pidFile, "utf8")).trim()), "SIGKILL")
  expect(await untilDown(dies.url)).toBe(true)
  await Bun.sleep(100)
  expect(await runner.ensure(dies, running)).toEqual({ ok: true, started: true })
  expect(await answers(dies.url)).toBe(true)
})

test("Evie stopping ends a start that is under way, and no app is started after", async () => {
  const { runner, app } = await setUp()
  const stopping = new AbortController()
  const late = app("late", (port) => appCommand(port, 5000))
  const opening = runner.ensure(late, stopping.signal)
  await Bun.sleep(200)
  stopping.abort()
  const opened = await opening
  expect(opened).toEqual({ ok: false, status: "failed", message: expect.stringContaining("stopping") })
  expect(await runner.ensure(late, stopping.signal)).toEqual({ ok: false, status: "failed", message: expect.stringContaining("stopping") })
})

test("an app whose server stopped, while something its start command started runs on, is started again", async () => {
  const { runner, folder, app } = await setUp()
  const pidFile = join(folder, "pid")
  const watched = app("watched", (port) => `sleep 30 & echo $$ > ${JSON.stringify(pidFile)}; exec ${appCommand(port)}`)
  expect(await runner.ensure(watched, running)).toEqual({ ok: true, started: true })

  process.kill(Number((await readFile(pidFile, "utf8")).trim()), "SIGKILL")
  expect(await untilDown(watched.url)).toBe(true)
  expect(await runner.ensure(watched, running)).toEqual({ ok: true, started: true })
  expect(await answers(watched.url)).toBe(true)
})

test("an app briefly not listening, as while its watcher restarts it, is left as it is", async () => {
  const { runner, folder, app } = await setUp()
  const pidFile = join(folder, "pid")
  const restarts = app("restarts", (port) => `while :; do ${appCommand(port)} & echo $! > ${JSON.stringify(pidFile)}; wait $!; sleep 0.3; done`)
  expect(await runner.ensure(restarts, running)).toEqual({ ok: true, started: true })

  process.kill(Number((await readFile(pidFile, "utf8")).trim()), "SIGKILL")
  expect(await untilDown(restarts.url)).toBe(true)
  expect(await runner.ensure(restarts, running)).toEqual({ ok: true, started: false })
})

test("Evie stopping while an app is first looked at does not run its start command", async () => {
  const { runner, app, logFolder } = await setUp()
  const stopping = new AbortController()
  const opening = runner.ensure(app("never", (port) => appCommand(port)), stopping.signal)
  stopping.abort()
  expect(await opening).toEqual({ ok: false, status: "failed", message: expect.stringContaining("stopping") })
  // Starting it would have made its log file.
  expect(await readdir(logFolder).catch(() => [])).toEqual([])
})
