import { spawn } from "node:child_process"
import { homedir } from "node:os"
import type { Plugin, Tool } from "../core/types.ts"
import { resolvePath, statOrMissing } from "./files/paths.ts"

// The built-in plugin for running commands.
//
// There is no approval step in this milestone: a command runs as soon as the
// model asks for it, with the permissions of whoever started Evie.

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
    "given. It cannot be typed into, so do not run anything that waits for input. A command that runs past its " +
    "timeout is stopped; the default is 120 seconds. A failing command is not an error: its output and exit status " +
    "are returned. To read, search or change files, prefer the read, grep, glob, edit and write tools.",
  inputSchema: {
    type: "object",
    properties: {
      command: { type: "string", description: "The command to run" },
      workdir: { type: "string", description: "The folder to run it in. Defaults to the project folder" },
      timeout: {
        type: "integer",
        description: "Seconds to allow before the command is stopped. Defaults to 120; at most 3600",
      },
    },
    required: ["command"],
  },

  async run(input, context) {
    const command = input.command as string
    const asked = typeof input.timeout === "number" && input.timeout > 0 ? input.timeout : DEFAULT_TIMEOUT_SECONDS
    const seconds = Math.min(asked, LONGEST_TIMEOUT_SECONDS)

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

    const ended = await runCommand(command, folder, seconds * 1000, context.signal)
    const output = ended.output.trimEnd()
    const notes: string[] = []
    if (ended.dropped) notes.push("[Some output was dropped: the command printed more than 5 MiB.]")
    if (ended.how === "timeout") {
      notes.push(`[Stopped: the command ran past its timeout of ${seconds} second${seconds === 1 ? "" : "s"}.]`)
    } else if (ended.how === "cancelled") {
      notes.push("[Stopped: the turn was cancelled.]")
    } else if (ended.how === "failed") {
      notes.push(`[The command could not be started: ${ended.problem}]`)
    } else if (ended.signal) {
      notes.push(`[Ended by signal ${ended.signal}]`)
    } else if (ended.status !== 0) {
      notes.push(`[Exit status: ${ended.status}]`)
    }
    if (output === "" && notes.length === 0) return "[The command finished and printed nothing.]"
    return [output, notes.join("\n")].filter((part) => part !== "").join("\n\n")
  },
}

type Ended = {
  output: string
  dropped: boolean
  how: "exited" | "timeout" | "cancelled" | "failed"
  status?: number | null
  signal?: string | null
  problem?: string
}

function runCommand(command: string, folder: string, timeout: number, signal: AbortSignal): Promise<Ended> {
  return new Promise((resolve) => {
    // The command gets a process group of its own, so that stopping it also
    // stops anything it started. It has no input to read from.
    const child = spawn("/bin/sh", ["-c", command], { cwd: folder, stdio: ["ignore", "pipe", "pipe"], detached: true })

    const pieces: Buffer[] = []
    let collected = 0
    let dropped = false
    const collect = (piece: Buffer) => {
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
    const stopGroup = (reason: "timeout" | "cancelled") => {
      if (how !== "exited") return
      how = reason
      kill("SIGTERM")
      // A command that ignores the polite request is stopped outright.
      setTimeout(() => kill("SIGKILL"), 2000).unref()
    }
    const kill = (name: "SIGTERM" | "SIGKILL") => {
      try {
        if (child.pid !== undefined) process.kill(-child.pid, name)
      } catch {
        // The group has already gone.
      }
    }

    const timer = setTimeout(() => stopGroup("timeout"), timeout)
    const onCancel = () => stopGroup("cancelled")
    signal.addEventListener("abort", onCancel, { once: true })
    if (signal.aborted) onCancel()

    const finish = (rest: Partial<Ended>) => {
      clearTimeout(timer)
      signal.removeEventListener("abort", onCancel)
      resolve({ output: Buffer.concat(pieces).toString("utf8"), dropped, how, ...rest })
    }
    child.on("error", (problem) => finish({ how: "failed", problem: problem.message }))
    child.on("close", (status, endedBy) => finish({ status, signal: endedBy }))
  })
}
