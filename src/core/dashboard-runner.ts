import { mkdirSync, writeFileSync } from "node:fs"
import { join, resolve } from "node:path"
import { endOf, startCommand, type Ended, type Running } from "./processes.ts"

// Runs the apps behind a workspace's live dashboards. An app is started from
// its dashboard's `start` command, in its project's folder, when its tab
// asks for it, and runs as a shell command does, in a process group of its
// own, so stopping it stops all it started. An app already answering at its
// address is used as it is and never stopped. An app Evie started is
// stopped once no tab has asked for it in `idleMinutes`, and at most
// `mostRunning` run at once: starting another stops the one asked for
// longest ago. All are stopped when Evie stops.

export type DashboardRunnerOptions = {
  // Where each started app's output goes, a file for each dashboard.
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
  | { ok: false; status: "not_running" | "failed"; message: string; log?: string }

export type DashboardRunner = {
  // Makes sure an app answers, starting it if it has a start command, and
  // counts as the tab asking for it, which keeps it from being stopped as idle.
  ensure(app: LiveApp, signal: AbortSignal): Promise<EnsureResult>
  // Stops every app it started.
  close(): Promise<void>
}

const IDLE_MINUTES = 10
const MOST_RUNNING = 3
// How long a started app has to answer before it is stopped as broken.
const START_WITHIN = 20_000
// How often a starting app is asked whether it answers yet, and how long it has to answer each time.
const ASK_EVERY = 150
const ANSWER_WITHIN = 1500
// How much of what a failed app printed is shown.
const LOG_SHOWN = 4000

type Started = { running: Running; ended?: Ended; idle?: ReturnType<typeof setTimeout> }

export function createDashboardRunner(options: DashboardRunnerOptions): DashboardRunner {
  const idleFor = (options.idleMinutes ?? IDLE_MINUTES) * 60_000
  const most = options.mostRunning ?? MOST_RUNNING
  const startWithin = options.startWithin ?? START_WITHIN
  // The apps started and still running, by folder and start command, the
  // one asked for longest ago first.
  const started = new Map<string, Started>()
  // Starts under way, so that tabs asking at once start an app once.
  const starting = new Map<string, Promise<EnsureResult>>()

  function asked(key: string, app: Started) {
    started.delete(key)
    started.set(key, app)
    clearTimeout(app.idle)
    app.idle = setTimeout(() => void stop(key, app), idleFor)
    app.idle.unref()
  }

  function stop(key: string, app: Started): Promise<unknown> {
    if (started.get(key) === app) started.delete(key)
    clearTimeout(app.idle)
    app.running.stop("stopped")
    return app.running.ended
  }

  async function ensure(app: LiveApp, signal: AbortSignal): Promise<EnsureResult> {
    if (signal.aborted) return STOPPING
    const key = app.start === undefined ? undefined : `${resolve(app.folder)}\0${app.start}`
    const already = () => {
      if (key === undefined) return undefined
      const pending = starting.get(key)
      if (pending !== undefined) return pending
      const mine = started.get(key)
      if (mine === undefined) return undefined
      asked(key, mine)
      return { ok: true as const, started: false }
    }
    const before = already()
    if (before !== undefined) return before
    if (await answersAt(app.url, signal)) return { ok: true, started: false }
    if (key === undefined || app.start === undefined) {
      return { ok: false, status: "not_running", message: `Nothing answers at ${app.url}, and the dashboard has no start command to start it with.` }
    }
    // Another tab may have started it while this one asked.
    const meanwhile = already()
    if (meanwhile !== undefined) return meanwhile
    const start = startApp(app, app.start, key, signal).finally(() => starting.delete(key))
    starting.set(key, start)
    return start
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
    const mine: Started = { running }
    asked(key, mine)
    void running.ended.then((ended) => {
      mine.ended = ended
      if (started.get(key) === mine) started.delete(key)
      clearTimeout(mine.idle)
    })

    const printed = () => {
      const { text } = endOf(logFile, LOG_SHOWN)
      return text.trim() === "" ? {} : { log: text }
    }
    const until = Date.now() + startWithin
    while (!signal.aborted) {
      if (await answersAt(app.url, signal)) return { ok: true, started: true }
      if (mine.ended !== undefined) {
        const how = mine.ended.signal ? `it was ended by ${mine.ended.signal}` : `exit status ${mine.ended.status}`
        return { ok: false, status: "failed", message: `Its start command stopped before anything answered at ${app.url} (${how}).`, ...printed() }
      }
      if (Date.now() >= until) {
        await stop(key, mine)
        return { ok: false, status: "failed", message: `Nothing answered at ${app.url} within ${startWithin / 1000} seconds of starting it, so it was stopped.`, ...printed() }
      }
      await sleep(ASK_EVERY, signal)
    }
    await stop(key, mine)
    return STOPPING
  }

  return {
    ensure,
    async close() {
      await Promise.all([...started].map(([key, app]) => stop(key, app)))
    },
  }
}

const STOPPING: EnsureResult = { ok: false, status: "failed", message: "Evie is stopping, so it starts no dashboards." }

// Whether anything answers at an address: any answer at all means a server is listening.
async function answersAt(url: string, signal: AbortSignal): Promise<boolean> {
  try {
    const answer = await fetch(url, { redirect: "manual", signal: AbortSignal.any([signal, AbortSignal.timeout(ANSWER_WITHIN)]) })
    await answer.body?.cancel()
    return true
  } catch {
    return false
  }
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
