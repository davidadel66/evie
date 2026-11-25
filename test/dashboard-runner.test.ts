import { afterEach, expect, test } from "bun:test"
import { spawn } from "node:child_process"
import { mkdtemp, readdir, readFile, realpath, rm, writeFile } from "node:fs/promises"
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

// Evie killed rather than stopped: its runner is left as it is, never
// closed, and another is made over the same folder, as the next start does.
test("an app left running by an Evie that was killed is picked up by the next one, and stopped with it", async () => {
  const { runner: killed, app, logFolder } = await setUp()
  const left = app("left", (port) => appCommand(port))
  expect(await killed.ensure(left, running)).toEqual({ ok: true, started: true })

  const next = createDashboardRunner({ logFolder })
  cleanups.push(() => next.close())
  // It is the same app, not one started again.
  expect(await next.ensure(left, running)).toEqual({ ok: true, started: false })
  await next.close()
  expect(await untilDown(left.url)).toBe(true)
  expect((await readdir(logFolder)).filter((name) => name.endsWith(".running"))).toEqual([])
})

test("an app picked up after Evie was killed is stopped once no tab asks for it, and counts toward the most that run", async () => {
  const { runner: killed, app, logFolder } = await setUp()
  const first = app("first", (port) => appCommand(port))
  const second = app("second", (port) => appCommand(port))
  expect(await killed.ensure(first, running)).toMatchObject({ ok: true })
  expect(await killed.ensure(second, running)).toMatchObject({ ok: true })

  // The owner lowered the most that run before starting Evie again.
  const next = createDashboardRunner({ logFolder, mostRunning: 1, idleMinutes: 0.01 })
  cleanups.push(() => next.close())
  expect(await untilDown(first.url)).toBe(true)
  expect(await answers(second.url)).toBe(true)
  // No tab asks for the other one.
  expect(await untilDown(second.url)).toBe(true)
})

test("a record whose process number has passed to another program is dropped, and that program is left alone", async () => {
  const { runner: killed, app, logFolder } = await setUp()
  const left = app("left", (port) => appCommand(port))
  expect(await killed.ensure(left, running)).toMatchObject({ ok: true })
  const [record] = (await readdir(logFolder)).filter((name) => name.endsWith(".running"))
  const kept = JSON.parse(await readFile(join(logFolder, record!), "utf8"))
  // The app goes, and its number is given to a program that leads a group of its own.
  await killed.close()
  const other = spawn("sleep", ["30"], { detached: true, stdio: "ignore" })
  cleanups.push(async () => void other.kill("SIGKILL"))
  await writeFile(join(logFolder, `${other.pid}.running`), JSON.stringify({ ...kept, pid: other.pid, processStart: "Mon Jan  1 00:00:00 2001" }))

  const next = createDashboardRunner({ logFolder, idleMinutes: 0.005 })
  await Bun.sleep(400)
  await next.close()
  expect(() => process.kill(other.pid!, 0)).not.toThrow()
  expect((await readdir(logFolder)).filter((name) => name.endsWith(".running"))).toEqual([])
})
