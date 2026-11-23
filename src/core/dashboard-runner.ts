import { spawn } from "node:child_process"
import { closeSync, mkdirSync, openSync } from "node:fs"
import { readFile } from "node:fs/promises"
import { join, resolve } from "node:path"

// On-demand supervisor for live local dashboard processes. Starts a project's
// dashboard command in its own process group when a tab is opened, coalesces
// concurrent opens for the same app, adopts already-running servers without
// owning them, caps total managed apps with LRU eviction, and stops idle apps
// after their timeout or on shutdown.

export type DashboardRunnerOptions = {
  logFolder: string
  maxRunning?: number
  defaultIdleMs?: number
  startTimeoutMs?: number
  probeIntervalMs?: number
  idleSweepMs?: number
  probe?: (url: string) => Promise<boolean>
}

export type EnsureResult =
  | { ok: true }
  | { ok: false; status: "not_running" | "failed"; message: string; log?: string }

export type DashboardSpec = {
  projectId: string
  projectFolder: string
  dashboardId: string
  url: string
  start?: string
  idle?: number | "never"
}

export type DashboardRunner = {
  ensure(spec: DashboardSpec): Promise<EnsureResult>
  touch(projectFolder: string, start?: string): void
  runningCount(): number
  close(): Promise<void>
}

type ManagedProcess = {
  key: string
  pid: number
  exited: boolean
  exitCode: number | null
  exitSignal: NodeJS.Signals | null
  lastTouched: number
  idleMs: number
  logFile: string
}

const DEFAULT_MAX_RUNNING = 3
const DEFAULT_IDLE_MS = 10 * 60_000
const DEFAULT_START_TIMEOUT_MS = 20_000
const DEFAULT_PROBE_INTERVAL_MS = 150
const DEFAULT_IDLE_SWEEP_MS = 30_000
const PROBE_TIMEOUT_MS = 1500
const STOP_POLL_MS = 40
const STOP_GRACE_MS = 1500
const LOG_TAIL_LINES = 30
const LOG_TAIL_BYTES = 4096

export function createDashboardRunner(options: DashboardRunnerOptions): DashboardRunner {
  const logFolder = options.logFolder
  const maxRunning = options.maxRunning ?? DEFAULT_MAX_RUNNING
  const defaultIdleMs = options.defaultIdleMs ?? DEFAULT_IDLE_MS
  const startTimeoutMs = options.startTimeoutMs ?? DEFAULT_START_TIMEOUT_MS
  const probeIntervalMs = options.probeIntervalMs ?? DEFAULT_PROBE_INTERVAL_MS
  const idleSweepMs = options.idleSweepMs ?? DEFAULT_IDLE_SWEEP_MS
  const probe = options.probe ?? defaultProbe

  const managed = new Map<string, ManagedProcess>()
  const inFlight = new Map<string, Promise<EnsureResult>>()
  const stopping = new Set<string>()
  let closed = false

  const sweepTimer = setInterval(() => {
    void sweepIdle()
  }, idleSweepMs)
  sweepTimer.unref()

  async function sweepIdle(): Promise<void> {
    if (closed) return
    const now = Date.now()
    const expired: ManagedProcess[] = []
    for (const entry of managed.values()) {
      if (entry.idleMs !== Infinity && now - entry.lastTouched >= entry.idleMs && !stopping.has(entry.key)) {
        expired.push(entry)
      }
    }
    await Promise.all(expired.map((entry) => evict(entry)))
  }

  async function evict(entry: ManagedProcess): Promise<void> {
    if (stopping.has(entry.key)) return
    stopping.add(entry.key)
    try {
      if (managed.get(entry.key) === entry) {
        managed.delete(entry.key)
      }
      await stopProcessGroup(entry.pid)
    } finally {
      stopping.delete(entry.key)
    }
  }

  async function enforceCap(): Promise<void> {
    while (managed.size >= maxRunning) {
      const victim = pickLruVictim(managed.values())
      if (victim === undefined) break
      await evict(victim)
    }
  }

  async function ensure(spec: DashboardSpec): Promise<EnsureResult> {
    if (closed) {
      return { ok: false, status: "failed", message: "The dashboard runner has been shut down." }
    }

    const start = spec.start?.trim() || undefined
    const key = start !== undefined ? processKey(spec.projectFolder, start) : undefined

    if (key !== undefined) {
      const pending = inFlight.get(key)
      if (pending !== undefined) return pending
    }

    if (await probe(spec.url)) {
      if (key !== undefined) {
        const existing = managed.get(key)
        if (existing !== undefined) {
          existing.lastTouched = Date.now()
        }
      }
      return { ok: true }
    }

    if (start === undefined || key === undefined) {
      return { ok: false, status: "not_running", message: "This dashboard is not running and has no start command." }
    }

    // Re-check inFlight after the async probe in case a concurrent call started spawning.
    const racing = inFlight.get(key)
    if (racing !== undefined) return racing

    const promise = startAndWait(spec, start, key).finally(() => {
      if (inFlight.get(key) === promise) {
        inFlight.delete(key)
      }
    })
    inFlight.set(key, promise)
    return promise
  }

  async function startAndWait(spec: DashboardSpec, start: string, key: string): Promise<EnsureResult> {
    // If an old managed entry exists for this key (e.g. hung process no longer answering), stop it first.
    const stale = managed.get(key)
    if (stale !== undefined) {
      await evict(stale)
    }

    await enforceCap()
    if (closed) {
      return { ok: false, status: "failed", message: "The dashboard runner has been shut down." }
    }

    mkdirSync(logFolder, { recursive: true })
    const logFile = join(logFolder, `${safeFilePart(spec.projectId)}-${safeFilePart(spec.dashboardId)}.log`)

    let outFd: number
    try {
      outFd = openSync(logFile, "w")
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause)
      return { ok: false, status: "failed", message: `Could not open dashboard log file: ${message}` }
    }

    let child: ReturnType<typeof spawn>
    try {
      try {
        child = spawn("/bin/sh", ["-c", start], {
          cwd: spec.projectFolder,
          stdio: ["ignore", outFd, outFd],
          detached: true,
        })
      } finally {
        closeSync(outFd)
      }
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause)
      return { ok: false, status: "failed", message: `Could not start dashboard command: ${message}` }
    }

    if (child.pid === undefined) {
      return { ok: false, status: "failed", message: "Dashboard command did not produce a process id." }
    }

    const idleMs = spec.idle === "never" ? Infinity : spec.idle !== undefined ? spec.idle * 60_000 : defaultIdleMs
    const entry: ManagedProcess = {
      key,
      pid: child.pid,
      exited: false,
      exitCode: null,
      exitSignal: null,
      lastTouched: Date.now(),
      idleMs,
      logFile,
    }
    managed.set(key, entry)

    const markExited = (code: number | null, signal: NodeJS.Signals | null) => {
      entry.exited = true
      entry.exitCode = code
      entry.exitSignal = signal
      if (managed.get(key) === entry) {
        managed.delete(key)
      }
    }
    child.once("error", () => markExited(1, null))
    child.once("close", (code, signal) => markExited(code, signal))
    child.unref()

    const deadline = Date.now() + startTimeoutMs
    while (true) {
      if (closed) {
        await evict(entry)
        return { ok: false, status: "failed", message: "The dashboard runner has been shut down." }
      }
      if (await probe(spec.url)) {
        entry.lastTouched = Date.now()
        return { ok: true }
      }
      if (entry.exited) {
        // The shell exited, but a child in its process group may still be running
        // and coming up (e.g. a daemonised command). Only fail once the whole group
        // is gone and a final probe still does not answer.
        if (!groupAlive(entry.pid)) {
          if (await probe(spec.url)) {
            entry.lastTouched = Date.now()
            return { ok: true }
          }
          if (managed.get(key) === entry) managed.delete(key)
          const log = await readLogTail(logFile)
          const detail =
            entry.exitSignal !== null
              ? `terminated by signal ${entry.exitSignal}`
              : `exited with code ${entry.exitCode ?? "unknown"}`
          return {
            ok: false,
            status: "failed",
            message: `This dashboard stopped before answering (${detail}).`,
            ...(log !== undefined ? { log } : {}),
          }
        }
      }
      const remaining = deadline - Date.now()
      if (remaining <= 0) {
        await evict(entry)
        const log = await readLogTail(logFile)
        return {
          ok: false,
          status: "failed",
          message: "This dashboard did not answer in time.",
          ...(log !== undefined ? { log } : {}),
        }
      }
      await sleep(Math.min(probeIntervalMs, remaining))
    }
  }

  function touch(projectFolder: string, start?: string): void {
    const trimmed = start?.trim()
    if (!trimmed) return
    const entry = managed.get(processKey(projectFolder, trimmed))
    if (entry !== undefined) {
      entry.lastTouched = Date.now()
    }
  }

  function runningCount(): number {
    return managed.size
  }

  async function close(): Promise<void> {
    closed = true
    clearInterval(sweepTimer)
    const entries = [...managed.values()]
    managed.clear()
    await Promise.all(entries.map((entry) => stopProcessGroup(entry.pid)))
  }

  return { ensure, touch, runningCount, close }
}

// Default readiness probe: any HTTP response means the server is listening.
async function defaultProbe(url: string): Promise<boolean> {
  try {
    const res = await fetch(url, {
      method: "GET",
      redirect: "manual",
      signal: AbortSignal.timeout(PROBE_TIMEOUT_MS),
    })
    await res.body?.cancel().catch(() => {})
    return true
  } catch {
    return false
  }
}

function processKey(projectFolder: string, start: string): string {
  return `${resolve(projectFolder)}\0${start.trim()}`
}

function safeFilePart(part: string): string {
  const clean = part.replace(/[^a-zA-Z0-9._-]+/g, "_").replace(/^_+|_+$/g, "")
  return clean !== "" ? clean : "dashboard"
}

// Picks the least recently touched entry, preferring entries that are allowed
// to idle out over `idle: "never"` entries.
function pickLruVictim(entries: Iterable<ManagedProcess>): ManagedProcess | undefined {
  let oldestEvictable: ManagedProcess | undefined
  let oldestAny: ManagedProcess | undefined
  for (const entry of entries) {
    if (oldestAny === undefined || entry.lastTouched < oldestAny.lastTouched) {
      oldestAny = entry
    }
    if (entry.idleMs !== Infinity) {
      if (oldestEvictable === undefined || entry.lastTouched < oldestEvictable.lastTouched) {
        oldestEvictable = entry
      }
    }
  }
  return oldestEvictable ?? oldestAny
}

// Sends SIGTERM to the detached process group, polls briefly until it exits,
// and falls back to SIGKILL if anything in the group survives past the grace window.
async function stopProcessGroup(pid: number): Promise<void> {
  if (!signalGroup(pid, "SIGTERM")) return
  const deadline = Date.now() + STOP_GRACE_MS
  while (Date.now() < deadline) {
    if (!groupAlive(pid)) return
    await sleep(STOP_POLL_MS)
  }
  signalGroup(pid, "SIGKILL")
}

function signalGroup(pid: number, signal: NodeJS.Signals): boolean {
  try {
    process.kill(-pid, signal)
    return true
  } catch {
    return false
  }
}

function groupAlive(pid: number): boolean {
  try {
    process.kill(-pid, 0)
    return true
  } catch {
    return false
  }
}

async function readLogTail(logFile: string): Promise<string | undefined> {
  try {
    const content = await readFile(logFile, "utf8")
    if (content === "") return undefined
    const lines = content.split("\n")
    const tailLines = lines.slice(-LOG_TAIL_LINES).join("\n")
    const bytes = Buffer.from(tailLines, "utf8")
    if (bytes.byteLength <= LOG_TAIL_BYTES) return tailLines
    return bytes.subarray(bytes.byteLength - LOG_TAIL_BYTES).toString("utf8")
  } catch {
    return undefined
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}
