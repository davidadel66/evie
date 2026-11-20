import { spawn } from "node:child_process"
import { appendFileSync, mkdirSync, writeFileSync } from "node:fs"
import { dirname } from "node:path"
import { homedir } from "node:os"
import type { Plugin, Tool } from "../core/types.ts"
import { resolvePath, statOrMissing } from "./files/paths.ts"

// The built-in plugin for running commands.
//
// A command runs with the permissions of whoever started Evie; the
// permissions plugin decides beforehand whether it may run at all.
//
// A command does not have to finish within the call that started it. One
// still running at its timeout, or started with `background`, is handed to
// the core as a job (see core/jobs.ts) and carries on.

const DEFAULT_TIMEOUT_SECONDS = 120
const LONGEST_TIMEOUT_SECONDS = 60 * 60
// How much output is kept in memory. Anything over the limit shown to the
// model is saved to a file by the shared tool handling; this only stops a
// runaway command from using up memory.
const MAX_COLLECTED_BYTES = 5 * 1024 * 1024

export function shell(): Plugin {
  return {
    name: "shell",
    setup(host) {
      host.tools.add(shellTool)
    },
  }
}

const shellTool: Tool = {
  name: "shell",
  description:
    "Run a shell command and return what it printed. The command runs in the project folder, or in workdir when " +
    "given. It cannot be typed into, so do not run anything that waits for input. A failing command is not an " +
    "error: its output and exit status are returned. To read, search or change files, prefer the read, grep, glob, " +
    "edit and write tools.\n\n" +
    "A command still running after its timeout, 120 seconds by default, is not stopped: it moves to the background " +
    "and carries on. For a command you know will run for a long time or never end, such as a server, a watcher or " +
    "a long build, set background to true and it starts there at once; do not add '&' yourself. You are told when a " +
    "background command finishes, with its output, so do not wait for it or keep checking on it.",
  inputSchema: {
    type: "object",
    properties: {
      command: { type: "string", description: "The command to run" },
      workdir: { type: "string", description: "The folder to run it in. Defaults to the project folder" },
      timeout: {
        type: "integer",
        description:
          "Seconds to wait before the command moves to the background. Defaults to 120; at most 3600. For a command " +
          "started in the background it is how long it may run before it is stopped, and by default it is not stopped",
      },
      background: { type: "boolean", description: "Start the command in the background and return at once" },
    },
    required: ["command"],
  },

  async run(input, context) {
    const command = input.command as string
    const background = input.background === true
    const given = typeof input.timeout === "number" && input.timeout > 0 ? Math.min(input.timeout, LONGEST_TIMEOUT_SECONDS) : undefined
    const seconds = given ?? DEFAULT_TIMEOUT_SECONDS

    // The general session has no project folder, so a command runs at home.
    let folder = context.folder ?? homedir()
    if (input.workdir !== undefined) {
      const workdir = input.workdir as string
      const resolved = resolvePath(workdir, { ...context, folder })
      if ("failure" in resolved) return resolved.failure
      const info = await statOrMissing(resolved.absolute)
      if (!info?.isDirectory()) return `The folder "${workdir}" does not exist, so the command was not run.`
      folder = resolved.absolute
    }

    const running = startCommand(command, folder)

    // Hands the command over to carry on as a background job. From here it
    // no longer belongs to the turn: cancelling the turn leaves it running.
    const handOver = (why: string) => {
      const job = context.jobs.start({
        kind: "shell",
        title: command.length > 80 ? `${command.slice(0, 80)}…` : command,
        done: running.ended.then((ended) => `${describe(ended, given, MOST_TOLD)}\n\nThe full output is in ${job.outputFile}.`),
        stop: () => running.stop("stopped"),
      })
      running.writeTo(job.outputFile)
      // A background command is only stopped by the clock when a time was asked for.
      if (background && given !== undefined) setTimeout(() => running.stop("timeout"), given * 1000).unref()
      return (
        `${why} It is background job ${job.id}, and its output is being written to ${job.outputFile}.\n\n` +
        "You will be told when it finishes, with its output. Do not wait for it, and do not keep checking on it: " +
        "carry on with work that does not depend on it, or end your reply if there is nothing else to do. Read the " +
        `output file only if what it holds so far lets you do something useful now. To stop it, use job_stop with the id "${job.id}". ` +
        "Do not start another copy while it is running."
      )
    }
    if (background) return handOver("The command was started in the background.")

    // Otherwise the call waits for the command, for as long as the timeout.
    let timer: ReturnType<typeof setTimeout> | undefined
    const tooLong = new Promise<"too long">((resolve) => (timer = setTimeout(() => resolve("too long"), seconds * 1000)))
    const onCancel = () => running.stop("cancelled")
    context.signal.addEventListener("abort", onCancel, { once: true })
    if (context.signal.aborted) onCancel()
    const first = await Promise.race([running.ended, tooLong])
    clearTimeout(timer)
    context.signal.removeEventListener("abort", onCancel)

    if (first === "too long") {
      return handOver(`The command did not finish within ${seconds} second${seconds === 1 ? "" : "s"}, so it was moved to the background, where it is still running.`)
    }
    return describe(first, given)
  },
}

// How much of a background command's output the model is told when it ends.
const MOST_TOLD = 6000

type Ended = {
  output: string
  dropped: boolean
  how: "exited" | "timeout" | "cancelled" | "stopped" | "failed"
  status?: number | null
  signal?: string | null
  problem?: string
}

// What the model is told about a command that has ended: what it printed,
// and how it ended when that was not a plain success. With `most`, only the
// end of long output is given.
function describe(ended: Ended, timeout: number | undefined, most?: number): string {
  let output = ended.output.trimEnd()
  const notes: string[] = []
  if (most !== undefined && output.length > most) {
    output = output.slice(-most)
    notes.push(`[Only the last ${most} characters of the output are shown.]`)
  }
  if (ended.dropped) notes.push("[Some output was dropped: the command printed more than 5 MiB.]")
  if (ended.how === "timeout") {
    notes.push(`[Stopped: the command ran past its timeout of ${timeout} second${timeout === 1 ? "" : "s"}.]`)
  } else if (ended.how === "cancelled") {
    notes.push("[Stopped: the turn was cancelled.]")
  } else if (ended.how === "stopped") {
    notes.push("[Stopped on request.]")
  } else if (ended.how === "failed") {
    notes.push(`[The command could not be started: ${ended.problem}]`)
  } else if (ended.signal) {
    notes.push(`[Ended by signal ${ended.signal}]`)
  } else if (ended.status !== 0) {
    notes.push(`[Exit status: ${ended.status}]`)
  }
  if (output === "" && notes.length === 0) return "[The command finished and printed nothing.]"
  return [output, notes.join("\n")].filter((part) => part !== "").join("\n\n")
}

type Running = {
  // Settles when the command has ended, however it ended.
  ended: Promise<Ended>
  // Stops the command and everything it started.
  stop(reason: "timeout" | "cancelled" | "stopped"): void
  // Writes what the command has printed so far to a file, and everything it
  // prints from now on.
  writeTo(file: string): void
}

function startCommand(command: string, folder: string): Running {
  // The command gets a process group of its own, so that stopping it also
  // stops anything it started. It has no input to read from.
  const child = spawn("/bin/sh", ["-c", command], { cwd: folder, stdio: ["ignore", "pipe", "pipe"], detached: true })

  const pieces: Buffer[] = []
  let collected = 0
  let dropped = false
  let file: string | undefined
  const collect = (piece: Buffer) => {
    if (file !== undefined) {
      try {
        appendFileSync(file, piece)
      } catch {
        // The file could not be written. What is kept in memory still is.
      }
    }
    if (collected >= MAX_COLLECTED_BYTES) {
      dropped = true
      return
    }
    pieces.push(piece)
    collected += piece.length
  }
  // What the command prints normally and what it prints as errors are kept
  // together, in the order they arrive.
  child.stdout.on("data", collect)
  child.stderr.on("data", collect)

  let how: Ended["how"] = "exited"
  const kill = (name: "SIGTERM" | "SIGKILL") => {
    try {
      if (child.pid !== undefined) process.kill(-child.pid, name)
    } catch {
      // The group has already gone.
    }
  }

  const ended = new Promise<Ended>((resolve) => {
    const finish = (rest: Partial<Ended>) => resolve({ output: Buffer.concat(pieces).toString("utf8"), dropped, how, ...rest })
    child.on("error", (problem) => finish({ how: "failed", problem: problem.message }))
    child.on("close", (status, endedBy) => finish({ status, signal: endedBy }))
  })

  return {
    ended,
    stop(reason) {
      if (how !== "exited") return
      how = reason
      kill("SIGTERM")
      // A command that ignores the polite request is stopped outright.
      setTimeout(() => kill("SIGKILL"), 2000).unref()
    },
    writeTo(path) {
      try {
        mkdirSync(dirname(path), { recursive: true })
        writeFileSync(path, Buffer.concat(pieces))
        file = path
      } catch {
        // Without the file the output is still told when the command ends.
      }
    },
  }
}
