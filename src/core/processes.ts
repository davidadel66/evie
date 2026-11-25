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
