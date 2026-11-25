import { spawn } from "node:child_process"
import { closeSync, fstatSync, openSync, readSync } from "node:fs"
import { constants } from "node:os"

// Commands that run as programs of their own, each in a process group of
// its own, writing what they print to one file. The shell plugin runs the
// model's commands this way, and the dashboards runner the apps behind live
// dashboards. Stopping a command stops everything it started, and a command
// has ended only once all it started has ended too.

export type Ended = {
  // "unknown" is a command picked up again that has gone without leaving its exit status.
  how: "exited" | "timeout" | "cancelled" | "stopped" | "failed" | "unknown"
  status?: number | null
  signal?: string | null
  problem?: string
}

// What a command printed, or the end of it: `cut` says the start is missing.
export type Printed = { text: string; cut: boolean }

// The last `most` bytes of a file, starting at a whole character, or
// nothing when it cannot be read.
export function endOf(file: string, most: number): Printed {
  let fd: number | undefined
  try {
    fd = openSync(file, "r")
    const size = fstatSync(fd).size
    const buffer = Buffer.alloc(Math.min(size, most))
    readSync(fd, buffer, 0, buffer.length, size - buffer.length)
    // The bytes that carry on a character split by the cut come first.
    let from = 0
    while (size > most && from < buffer.length && (buffer[from]! & 0xc0) === 0x80) from++
    return { text: buffer.subarray(from).toString("utf8"), cut: size > most }
  } catch {
    return { text: "", cut: false }
  } finally {
    if (fd !== undefined) closeSync(fd)
  }
}

// The file a command's own shell writes its exit status to, beside its output.
export const exitFileOf = (outputFile: string) => outputFile.replace(/\.log$/, ".exit")

// The signals that end a process that does not catch them. A shell gives a
// command that died by one the status 128 and its number. Others, such as
// stop, continue or a child having ended, never end a process, so a status
// that stands for one of them is only a status the command chose.
const ENDING = ["SIGHUP", "SIGINT", "SIGQUIT", "SIGILL", "SIGTRAP", "SIGABRT", "SIGFPE", "SIGKILL", "SIGBUS", "SIGSEGV", "SIGSYS", "SIGPIPE", "SIGALRM", "SIGTERM", "SIGXCPU", "SIGXFSZ", "SIGVTALRM", "SIGPROF", "SIGUSR1", "SIGUSR2"]
const numbered = constants.signals as Record<string, number | undefined>
// The signal that a status stands for, when it stands for one that ends a process.
export const endedBy = (status: number) => (status > 128 ? ENDING.find((name) => numbered[name] === status - 128) : undefined)

// The command runs in a shell of its own, under one that writes how it
// ended to the exit file. When the command died by a signal that ends a
// process, the outer one then sends itself the same signal, so that it is
// seen to end the same way, and its own report of the death is kept out of
// the output. Stopped by a signal itself, the outer one ends too, and writes nothing.
// The command runs in a subshell that becomes it (`exec`), so that the shell
// waiting on it has its own error output on /dev/null while it waits. Some
// shells, such as dash, the usual /bin/sh on Linux, report a command killed
// by a signal ("Killed") on the error output in force while waiting, which
// would otherwise land in the command's output.
const RUNS_THEN_RECORDS =
  'exec 3>&2 2>/dev/null; ( exec /bin/sh -c "$1" 2>&3 3>&- ); status=$?; echo $status > "$2"; ' +
  `case $status in ${ENDING.flatMap((name) => (numbered[name] === undefined ? [] : [128 + numbered[name]!])).join("|")}) kill -$((status - 128)) $$;; esac; exit $status`

export type Running = {
  pid: number | undefined
  // Why the output file could not be opened, when it could not, and so the command was not run.
  unwritable?: string
  // Settles when the command has ended, however it ended.
  ended: Promise<Ended>
  // Stops the command and everything it started.
  stop(reason: "timeout" | "cancelled" | "stopped"): void
}

export function startCommand(command: string, folder: string, outputFile: string): Running {
  // The command gets a process group of its own, so that stopping it also
  // stops anything it started, and so that it does not end with Evie. It has
  // no input to read from, and what it prints, normally or as errors, goes
  // to the one file in the order it is printed.
  let out: number
  try {
    out = openSync(outputFile, "a")
  } catch (cause) {
    const unwritable = (cause as { code?: string }).code ?? (cause instanceof Error ? cause.message : String(cause))
    return { pid: undefined, unwritable, ended: Promise.resolve({ how: "failed", problem: unwritable }), stop: () => {} }
  }
  let child: ReturnType<typeof spawn>
  try {
    try {
      child = spawn("/bin/sh", ["-c", RUNS_THEN_RECORDS, "sh", command, exitFileOf(outputFile)], { cwd: folder, stdio: ["ignore", out, out], detached: true })
    } finally {
      closeSync(out)
    }
  } catch (cause) {
    const problem = cause instanceof Error ? cause.message : String(cause)
    return { pid: undefined, ended: Promise.resolve({ how: "failed", problem }), stop: () => {} }
  }

  let how: Ended["how"] = "exited"
  let over = false
  const ended = new Promise<Ended>((resolve) => {
    const finish = (rest: Partial<Ended>) => {
      over = true
      resolve({ how, ...rest })
    }
    child.on("error", (problem) => finish({ how: "failed", problem: problem.message }))
    // It has ended once all it started has ended too, since what it left
    // running in its group, such as a server it started with "&", is still
    // its work, and still writes to its output file.
    child.on("close", (status, endedBy) => {
      const whenAllGone = () => (child.pid !== undefined && groupThere(child.pid) ? setTimeout(whenAllGone, ALL_GONE_EVERY) : finish({ status, signal: endedBy }))
      whenAllGone()
    })
  })

  return {
    pid: child.pid,
    ended,
    stop(reason) {
      // Once the command has ended its process id may belong to something else.
      if (over || how !== "exited" || child.pid === undefined) return
      how = reason
      stopGroup(child.pid, () => !over)
    },
  }
}

// How often, in milliseconds, a command whose shell has ended is looked at
// to tell whether what it started has ended too.
const ALL_GONE_EVERY = 50

// Whether any process of a group is there. Signal 0 only asks. A group Evie
// may not signal is another user's, so not its command's: as far as the
// command goes, it has gone.
export function groupThere(group: number): boolean {
  try {
    process.kill(-group, 0)
    return true
  } catch {
    return false
  }
}

// Asks a process group to stop, and stops it outright if it has not after two seconds.
export function stopGroup(group: number, still: () => boolean = () => true) {
  const kill = (name: "SIGTERM" | "SIGKILL") => {
    try {
      process.kill(-group, name)
    } catch {
      // The group has already gone.
    }
  }
  kill("SIGTERM")
  setTimeout(() => still() && kill("SIGKILL"), 2000).unref()
}

// ---- A command an earlier run of Evie left ------------------------------------------------

// When a process started, as the system gives it, or undefined when there
// is no such process. With its id, it tells the process apart from a later
// one that has been given the same id. It is asked for in one time zone and
// language, so that it reads the same to every run of Evie. Asking takes a
// few milliseconds.
export function startOf(pid: number): string | undefined {
  try {
    const env = { PATH: process.env.PATH ?? "", TZ: "UTC", LC_ALL: "C" }
    const at = Bun.spawnSync(["ps", "-o", "lstart=", "-p", String(pid)], { env, stderr: "ignore" }).stdout.toString().trim()
    return at === "" ? undefined : at
  } catch {
    return undefined
  }
}

// Whether a process of this id is there. Signal 0 only asks.
function alive(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch (cause) {
    return (cause as { code?: string }).code === "EPERM"
  }
}

// Whether the group of this id is still the command that started at
// `processStart`. The command's own shell leads its group: while a process
// of that id is there, it is the command's unless it started at another
// time than the one recorded. One whose start time cannot be read is taken
// to be the command's. Once the shell has gone, a group left with that id
// is still the command's, since no new group can take the id while any of
// the old one is left. Asked afresh each time, as the id can pass on.
export function sameCommand(pid: number, processStart: string | undefined): boolean {
  if (!groupThere(pid)) return false
  if (!alive(pid) || processStart === undefined) return true
  const at = startOf(pid)
  return at === undefined || at === processStart
}

// How often, in milliseconds, a command picked up again is looked at to
// tell whether it has gone.
const PICKED_UP_EVERY = 1000

// A command an earlier run of Evie started, taken to be the caller's own
// again, which is for the caller to have checked with `sameCommand`. While
// it runs, only whether its group is there is asked, which costs next to
// nothing. Whether the group is still the command's is asked once more
// before it is stopped, so that a program since given its id is never
// signalled. How it ended on its own is not known.
export function pickUp(pid: number, processStart: string | undefined): Running {
  let how: Ended["how"] = "exited"
  let over = false
  let clock: ReturnType<typeof setTimeout> | undefined
  let finish = () => {}
  const ended = new Promise<Ended>((resolve) => {
    finish = () => {
      over = true
      clearTimeout(clock)
      resolve({ how: how === "exited" ? "unknown" : how })
    }
  })
  // Waiting for it to go on its own does not keep Evie running; waiting
  // for it to stop does, as someone waits for that.
  const look = () => {
    if (!groupThere(pid)) return finish()
    clock = setTimeout(look, how === "exited" ? PICKED_UP_EVERY : ALL_GONE_EVERY)
    if (how === "exited") clock.unref()
  }
  look()

  return {
    pid,
    ended,
    stop(reason) {
      if (over || how !== "exited") return
      how = reason
      clearTimeout(clock)
      if (!sameCommand(pid, processStart)) return finish()
      stopGroup(pid, () => !over)
      look()
    },
  }
}
