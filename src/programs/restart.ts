import { spawn } from "node:child_process"
import { constants } from "node:os"
import type { Settings } from "../index.ts"
import { EXIT_CODES, reason, type StopSignal } from "./common.ts"
import { LABEL } from "./service.ts"

// What starts `evie serve` again once a restart has stopped it: launchd, for
// the login service, which starts it again whenever it stops; or the
// `evie serve` that started this one and watches over it. Run by hand,
// there is nothing to, so that process becomes the watcher: see `watchOver`.

// The exit code of a server that stopped to be started again.
export const RESTART = 75

// Set in the environment of an `evie serve` that another watches over, to
// that one's process id.
export const WATCHED_BY = "EVIE_WATCHED_BY"

// Who starts this server again when it stops for a restart, or undefined
// when nothing does. launchd is process 1, and names the service it starts
// in its environment, which is looked at too, since a process whose parent
// has gone is given to process 1 as well; and a command the service's
// server runs keeps the name, under another parent.
export function startedAgainBy(process: { ppid: number; env: Settings }): "login service" | "watcher" | undefined {
  if (process.ppid === 1 && process.env.XPC_SERVICE_NAME === LABEL) return "login service"
  if (process.env[WATCHED_BY] === String(process.ppid)) return "watcher"
  return undefined
}

// A server the watcher started: a way to send it a signal, and its exit code
// once it has gone.
export type Watched = { kill(signal: StopSignal): void; exited: Promise<number> }

// Watches over servers: starts one, and another each time the one before
// stops for a restart, and settles with the exit code of the last. Each
// signal that stops a server is passed on to the one running, and kept: a
// server that then stops for a restart is not started again, and a signal
// that comes while none is running ends the watching with its own code.
export async function watchOver(start: () => Watched, signals: { on(listener: (signal: StopSignal) => void): void }): Promise<number> {
  let stopped: StopSignal | undefined
  let current: Watched | undefined
  signals.on((signal) => {
    stopped ??= signal
    current?.kill(signal)
  })
  for (;;) {
    if (stopped !== undefined) return EXIT_CODES[stopped]
    current = start()
    const code = await current.exited
    current = undefined
    if (code !== RESTART) return code
  }
}

// The watching of a server run by hand, from now on in this process: each
// new `evie serve` is started with the same arguments, in the same folder,
// printing to the same place, in a process group of its own, so that the
// terminal's Ctrl-C reaches it once, through this process. Its environment
// names this process, so that it stops for a restart by exiting, and stops
// for good if this process goes. This process ends as the last one does.
export async function watchFromHere(): Promise<never> {
  for (const signal of Object.keys(EXIT_CODES) as StopSignal[]) process.removeAllListeners(signal)
  const start = (): Watched => {
    const child = spawn(process.execPath, process.argv.slice(1), {
      stdio: ["ignore", "inherit", "inherit"],
      detached: true,
      env: { ...process.env, [WATCHED_BY]: String(process.pid) },
    })
    const exited = new Promise<number>((resolve) => {
      child.on("exit", (code, signal) => resolve(code ?? 128 + (signal ? constants.signals[signal] : 0)))
      child.on("error", (cause) => {
        console.error(`evie serve could not be started again: ${reason(cause)}`)
        resolve(1)
      })
    })
    return { kill: (signal) => child.kill(signal), exited }
  }
  const signals = {
    on(listener: (signal: StopSignal) => void) {
      for (const signal of Object.keys(EXIT_CODES) as StopSignal[]) process.on(signal, () => listener(signal))
    },
  }
  process.exit(await watchOver(start, signals))
}
