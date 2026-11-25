import { mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from "node:fs"
import { connect } from "node:net"
import { join, resolve } from "node:path"
import { endOf, pickUp, sameCommand, startCommand, startOf, type Ended, type Running } from "./processes.ts"

// Runs the apps behind a workspace's live dashboards. An app is started from
// its dashboard's `start` command, in its project's folder, when its tab
// asks for it, and runs as a shell command does, in a process group of its
// own, so stopping it stops all it started. An app answers while something
// listens on its address's port. One already answering is used as it is
// and never stopped. An app Evie started is
// stopped once no tab has asked for it in `idleMinutes`, and at most
// `mostRunning` run at once: starting another stops the one asked for
// longest ago. Only a tab opening its dashboard starts an app: the asks
// an open tab repeats keep its app running, and never start one, or tabs
// open on more apps than may run would stop each other's in turn. All are
// stopped when Evie stops. Each one running has a
// small record beside its output, so that when Evie was killed instead, the
// next run picks its apps up again as its own, and none is left running
// with no one to stop it.

export type DashboardRunnerOptions = {
  // Where each started app's output goes, a file for each dashboard, and
  // the record of each one running.
  logFolder: string
  idleMinutes?: number
  mostRunning?: number
  // How long a started app has to answer, in milliseconds, which is for tests to change.
  startWithin?: number
}

// A live dashboard's app: where it answers, and the command that starts it, if any.
export type LiveApp = { project: string; folder: string; dashboard: string; url: string; start?: string }

// Whether the app answers now, and if so whether it had to be started for
// it; otherwise why not, with the end of what it printed.
export type EnsureResult =
  | { ok: true; started: boolean }
  | { ok: false; status: "not_running" | "stopped" | "failed"; message: string; log?: string }

export type DashboardRunner = {
  // Makes sure an app answers, and counts as the tab asking for it, which
  // keeps it from being stopped as idle. To "open" it starts it if it has a
  // start command; to "keep" it only says that it has stopped.
  ensure(app: LiveApp, signal: AbortSignal, how?: "open" | "keep"): Promise<EnsureResult>
  // Stops every app it started.
  close(): Promise<void>
}

const IDLE_MINUTES = 10
const MOST_RUNNING = 3
// How long a started app has to answer before it is stopped as broken.
const START_WITHIN = 20_000
// How often a starting app is looked at to tell whether it answers yet,
// and how long its port has to take the connection each time.
const ASK_EVERY = 150
const ANSWER_WITHIN = 1500
// How long a started app that has stopped answering, while something it
// started runs on, has to answer again, as while its watcher restarts its
// server, before it is stopped and started afresh.
const BACK_WITHIN = 2000
// How much of what a failed app printed is shown.
const LOG_SHOWN = 4000

type Look = { result: Promise<EnsureResult>; how: "open" | "keep" }
type Started = { running: Running; ended?: Ended; idle?: ReturnType<typeof setTimeout> }

export function createDashboardRunner(options: DashboardRunnerOptions): DashboardRunner {
  const idleFor = (options.idleMinutes ?? IDLE_MINUTES) * 60_000
  const most = options.mostRunning ?? MOST_RUNNING
  const startWithin = options.startWithin ?? START_WITHIN
  // The apps started and still running, by folder and start command, the
  // one asked for longest ago first.
  const started = new Map<string, Started>()
  // Looks at an app under way, so that tabs asking at once start it once.
  const looking = new Map<string, Look>()

  function asked(key: string, app: Started) {
    started.delete(key)
    started.set(key, app)
    clearTimeout(app.idle)
    app.idle = setTimeout(() => void stop(key, app), idleFor)
    app.idle.unref()
  }

  // Takes a running command as the app of `key`: it counts as just asked
  // for, and is forgotten, with its record, once it has ended.
  function keep(key: string, running: Running, record: string): Started {
    const mine: Started = { running }
    asked(key, mine)
    void running.ended.then((ended) => {
      mine.ended = ended
      if (started.get(key) === mine) started.delete(key)
      clearTimeout(mine.idle)
      rmSync(record, { force: true })
    })
    return mine
  }

  // The apps an Evie that was killed left running, the one started longest
  // ago first. A record of one that has gone, or whose process number is
  // another program's now, is dropped.
  for (const { file, key, pid, processStart } of recordsIn(options.logFolder)) {
    if (sameCommand(pid, processStart)) keep(key, pickUp(pid, processStart), file)
    else rmSync(file, { force: true })
  }
  while (started.size > most) {
    const [oldestKey, oldest] = started.entries().next().value!
    void stop(oldestKey, oldest)
  }

  function stop(key: string, app: Started): Promise<unknown> {
    if (started.get(key) === app) started.delete(key)
    clearTimeout(app.idle)
    app.running.stop("stopped")
    return app.running.ended
  }

  async function ensure(app: LiveApp, signal: AbortSignal, how: "open" | "keep" = "open"): Promise<EnsureResult> {
    if (signal.aborted) return STOPPING
    if (app.start === undefined) {
      if (await answersAt(app.url)) return { ok: true, started: false }
      return { ok: false, status: "not_running", message: `Nothing answers at ${app.url}, and the dashboard has no start command to start it with.` }
    }
    const key = `${resolve(app.folder)}\0${app.start}`
    const pending = looking.get(key)
    if (pending !== undefined) {
      // An ask to open is not answered by a look that starts nothing.
      if (pending.how === "open" || how === "keep") return pending.result
      await pending.result
      return ensure(app, signal, how)
    }
    const result = lookAt(app, app.start, key, signal, how).finally(() => looking.delete(key))
    looking.set(key, { result, how })
    return result
  }

  async function lookAt(app: LiveApp, command: string, key: string, signal: AbortSignal, how: "open" | "keep"): Promise<EnsureResult> {
    const mine = started.get(key)
    if (mine !== undefined) {
      if (await answersWithin(app.url, BACK_WITHIN, signal, () => mine.ended !== undefined)) {
        asked(key, mine)
        return { ok: true, started: false }
      }
      // Its server has stopped, though something it started may run on.
      await stop(key, mine)
    } else if (await answersAt(app.url)) {
      return { ok: true, started: false }
    }
    if (signal.aborted) return STOPPING
    if (how === "keep") return { ok: false, status: "stopped", message: "This dashboard's app was stopped. It may have made room for another dashboard, or stopped on its own." }
    return startApp(app, command, key, signal)
  }

  async function startApp(app: LiveApp, command: string, key: string, signal: AbortSignal): Promise<EnsureResult> {
    // Room is made and the app started with nothing awaited in between, so
    // that apps started at once never run more than the most allowed.
    while (started.size >= most) {
      const [oldestKey, oldest] = started.entries().next().value!
      void stop(oldestKey, oldest)
    }
    const logFile = join(options.logFolder, `${fileSafe(app.project)}-${fileSafe(app.dashboard)}.log`)
    try {
      mkdirSync(options.logFolder, { recursive: true })
      writeFileSync(logFile, "")
    } catch {
      // Opening it to start the app says why it cannot be written.
    }
    const running = startCommand(command, app.folder, logFile)
    if (running.pid === undefined) {
      const ended = await running.ended
      return { ok: false, status: "failed", message: `Its start command could not be run: ${ended.problem}` }
    }
    const record = join(options.logFolder, `${running.pid}${RECORD}`)
    try {
      writeFileSync(record, JSON.stringify({ key, pid: running.pid, processStart: startOf(running.pid) }))
    } catch {
      // Without its record the app only could not be picked up again.
    }
    const mine = keep(key, running, record)

    const printed = () => {
      const { text } = endOf(logFile, LOG_SHOWN)
      return text.trim() === "" ? {} : { log: text }
    }
    if (await answersWithin(app.url, startWithin, signal, () => mine.ended !== undefined)) return { ok: true, started: true }
    const ended = mine.ended
    await stop(key, mine)
    if (signal.aborted) return STOPPING
    if (ended !== undefined) {
      const how = ended.signal ? `it was ended by ${ended.signal}` : `exit status ${ended.status}`
      return { ok: false, status: "failed", message: `Its start command stopped before anything answered at ${app.url} (${how}).`, ...printed() }
    }
    return { ok: false, status: "failed", message: `Nothing answered at ${app.url} within ${startWithin / 1000} seconds of starting it, so it was stopped.`, ...printed() }
  }

  return {
    ensure,
    async close() {
      await Promise.all([...started].map(([key, app]) => stop(key, app)))
    },
  }
}

// The record of an app running: a file named by its process number,
// holding which app it is and when its process started, which with the
// number tells it apart from a later program given the same number.
const RECORD = ".running"
type AppRecord = { file: string; key: string; pid: number; processStart: string | undefined }

function recordsIn(folder: string): AppRecord[] {
  let names: string[]
  try {
    names = readdirSync(folder).filter((name) => name.endsWith(RECORD))
  } catch {
    return []
  }
  const records: (AppRecord & { written: number })[] = []
  for (const name of names) {
    const file = join(folder, name)
    try {
      const { key, pid, processStart } = JSON.parse(readFileSync(file, "utf8")) as Record<string, unknown>
      if (typeof key !== "string" || typeof pid !== "number" || !Number.isInteger(pid) || pid <= 1) throw new Error("not a record")
      records.push({ file, key, pid, processStart: typeof processStart === "string" ? processStart : undefined, written: statSync(file).mtimeMs })
    } catch {
      rmSync(file, { force: true })
    }
  }
  return records.sort((one, other) => one.written - other.written)
}

const STOPPING: EnsureResult = { ok: false, status: "failed", message: "Evie is stopping, so it starts no dashboards." }

// Whether something listens on an address's port. Its port is only
// connected to, so the app is sent no request, and one busy with a slow
// page still answers.
function answersAt(url: string): Promise<boolean> {
  const at = new URL(url)
  const port = Number(at.port) || (at.protocol === "https:" ? 443 : 80)
  return new Promise((done) => {
    const socket = connect({ host: at.hostname.replace(/^\[(.*)\]$/, "$1"), port })
    const end = (answers: boolean) => {
      clearTimeout(clock)
      socket.destroy()
      done(answers)
    }
    const clock = setTimeout(() => end(false), ANSWER_WITHIN)
    socket.once("connect", () => end(true))
    socket.once("error", () => end(false))
  })
}

// Whether an address answers within `within` milliseconds, looking again
// until then, unless Evie stops or `over` says there is no use waiting.
async function answersWithin(url: string, within: number, signal: AbortSignal, over: () => boolean): Promise<boolean> {
  const until = Date.now() + within
  while (!signal.aborted) {
    if (await answersAt(url)) return true
    if (over() || Date.now() >= until) return false
    await sleep(ASK_EVERY, signal)
  }
  return false
}

function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((done) => {
    const woken = () => {
      clearTimeout(clock)
      done()
    }
    const clock = setTimeout(() => {
      signal.removeEventListener("abort", woken)
      done()
    }, ms)
    signal.addEventListener("abort", woken, { once: true })
  })
}

const fileSafe = (part: string) => part.replace(/[^a-zA-Z0-9._-]+/g, "_") || "dashboard"
