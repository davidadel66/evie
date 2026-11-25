import { readFileSync, renameSync } from "node:fs"
import { homedir } from "node:os"
import { dirname, join } from "node:path"
import { EndNotKnown } from "../core/jobs.ts"
import { endedBy, endOf, exitFileOf, sameCommand, startCommand, startOf, stopGroup, type Ended, type Printed } from "../core/processes.ts"
import { safeName } from "../core/tools.ts"
import type { JobRecord, JobWork, Plugin, Tool } from "../core/types.ts"
import { resolvePath, statOrMissing } from "./files/paths.ts"

// The built-in plugin for running commands.
//
// A command runs with the permissions of whoever started Evie; the
// permissions plugin decides beforehand whether it may run at all.
//
// A command does not have to finish within the call that started it. One
// still running at its timeout, or started with `background`, is handed to
// the core as a job (see core/jobs.ts) and carries on.
//
// A command is a program of its own, in a process group of its own, and
// writes what it prints straight to its job's output file, so it carries on
// while Evie restarts. When it ends, its own shell writes its exit status to
// a file beside that one. A later run of Evie picks it up from the job's
// record: it watches the command again while it runs, and tells how it ended.

const DEFAULT_TIMEOUT_SECONDS = 120
const LONGEST_TIMEOUT_SECONDS = 60 * 60
// How much of what a command printed is read for its result. Anything over
// the limit shown to the model is saved to a file by the shared tool
// handling. A command that printed more than this has its output file kept
// instead, and only the end of it is shown.
const MAX_COLLECTED_BYTES = 5 * 1024 * 1024

export function shell(): Plugin {
  return {
    name: "shell",
    setup(host) {
      host.tools.add(shellTool)
      host.jobKinds.add({ name: "shell", resume: resumeCommand })
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
    "and carries on; the owner can also move a command there while you wait for it. For a command you know will run for a long time or never end, such as a server, a watcher or " +
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
  // Errors, test summaries and how the command ended come last.
  longOutput: "end",

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

    const title = command.length > 80 ? `${command.slice(0, 80)}…` : command
    // Its number, output file and record come first, so that if Evie
    // restarts while this call waits, the next start finds the command.
    const reserved = context.jobs.reserve({ kind: "shell", title, call: context.callId })
    const running = startCommand(command, folder, reserved.outputFile)
    if (running.unwritable !== undefined) {
      reserved.release()
      return (
        `The command was not run: evie could not write its output file in ${dirname(reserved.outputFile)}, the folder where ` +
        `it keeps what tools print (${running.unwritable}). Tell the owner that this folder cannot be written to: no command can run until it can.`
      )
    }
    if (running.pid !== undefined) reserved.keep({ pid: running.pid, group: running.pid, processStart: startOf(running.pid) })

    // Hands the command over to carry on as a background job. From here it
    // no longer belongs to the turn: cancelling the turn leaves it running.
    const handOver = (why: string) => {
      // A background command is only stopped by the clock when a time was asked for.
      const deadline = background && given !== undefined ? Date.now() + given * 1000 : undefined
      const job = reserved.start({
        kind: "shell",
        title,
        done: running.ended.then((ended) => toldOf(ended, given, reserved.outputFile)),
        stop: () => running.stop("stopped"),
        ...(deadline !== undefined && { keep: { deadline } }),
      })
      if (deadline !== undefined) {
        const clock = setTimeout(() => running.stop("timeout"), deadline - Date.now())
        clock.unref()
        void running.ended.then(() => clearTimeout(clock))
      }
      return (
        `${why} It is background job ${job.id}, and its output is being written to ${job.outputFile}.\n\n` +
        "You do not have its result yet. You will be told when it finishes, with its output. Do not guess what it " +
        "will print, do not wait for it, and do not keep checking on it: " +
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
    // The owner can also ask for it to go to the background while it is waited for.
    let stopListening = () => {}
    const asked = new Promise<"asked">((resolve) => (stopListening = context.jobs.whileWaiting(() => resolve("asked"))))
    const first = await Promise.race([running.ended, tooLong, asked])
    clearTimeout(timer)
    stopListening()
    context.signal.removeEventListener("abort", onCancel)

    if (first === "asked") return handOver("The owner moved the command to the background, where it is still running.")
    if (first === "too long") {
      return handOver(`The command did not finish within ${seconds} second${seconds === 1 ? "" : "s"}, so it was moved to the background, where it is still running.`)
    }
    // It ended within its call, so it never became a job: what it printed is
    // the call's result, and its files go. Output too long to read whole is
    // kept under the call's name, as other long output is, for a week.
    const printed = endOf(reserved.outputFile, MAX_COLLECTED_BYTES)
    if (!printed.cut) {
      reserved.release()
      return describe(first, printed, given)
    }
    const kept = join(dirname(reserved.outputFile), `${safeName(context.callId)}.log`)
    let where = `The full output is saved at ${kept}. Read that file with offset and limit to see more.`
    try {
      renameSync(reserved.outputFile, kept)
    } catch (cause) {
      where = `The full output could not be kept (${cause instanceof Error ? cause.message : String(cause)}).`
    }
    reserved.release()
    return `${describe(first, printed, given, MOST_TOLD)}\n\n[${where}]`
  },
}

// How much of a background command's output the model is told when it ends,
// and how much of it is read to tell it from.
const MOST_TOLD = 6000
const KEPT_OF_BACKGROUND = 4 * MOST_TOLD

// What the model is told about a background command that has ended: the end
// of what it printed, how it ended, and where the rest is. One that did not
// end well is a job that failed.
function toldOf(ended: Ended, timeout: number | undefined, outputFile: string): string {
  const told = `${describe(ended, endOf(outputFile, KEPT_OF_BACKGROUND), timeout, MOST_TOLD)}\n\nThe full output is in ${outputFile}.`
  if ((ended.how === "exited" && ended.status === 0) || ended.how === "stopped") return told
  throw ended.how === "unknown" ? new EndNotKnown(told) : new Error(told)
}

// What the model is told about a command that has ended: what it printed,
// and how it ended when that was not a plain success. With `most`, only the
// end of long output is given.
function describe(ended: Ended, printed: Printed, timeout: number | undefined, most?: number): string {
  let output = printed.text.trimEnd()
  const notes: string[] = []
  if (most !== undefined && (output.length > most || printed.cut)) {
    output = output.slice(-most)
    notes.push(`[Only the end of the output is shown.]`)
  }
  if (ended.how === "timeout") {
    notes.push(`[Stopped: the command ran past its timeout of ${timeout} second${timeout === 1 ? "" : "s"}.]`)
  } else if (ended.how === "cancelled") {
    notes.push("[Stopped: the turn was cancelled.]")
  } else if (ended.how === "stopped") {
    notes.push("[Stopped on request.]")
  } else if (ended.how === "failed") {
    notes.push(`[The command could not be started: ${ended.problem}]`)
  } else if (ended.how === "unknown") {
    notes.push(
      "[evie restarted while it was running, and it has since gone without leaving its exit status. Check whether it " +
        "took effect, and run it again only if it did not.]",
    )
  } else if (ended.signal) {
    notes.push(`[Ended by signal ${ended.signal}]`)
  } else if (ended.status !== 0) {
    notes.push(`[Exit status: ${ended.status}]`)
  }
  if (output === "" && notes.length === 0) return "[The command finished and printed nothing.]"
  return [output, notes.join("\n")].filter((part) => part !== "").join("\n\n")
}

// ---- Picking a command up again -----------------------------------------------------------

// How often a command picked up again is looked at, to tell when it has ended.
const WATCH_EVERY = 500

// A command an earlier run of Evie left, watched until it ends: until its
// process group has gone, as a command running here is. Then its exit file
// says how it ended; with none, it was stopped, or how it ended is not known.
function resumeCommand(job: JobRecord): JobWork {
  const { pid, processStart, deadline } = job.keep as { pid?: unknown; processStart?: unknown; deadline?: unknown }
  const exitFile = exitFileOf(job.outputFile)
  const timeout = typeof deadline === "number" ? Math.round((deadline - job.started) / 1000) : undefined
  const running = () => typeof pid === "number" && sameCommand(pid, typeof processStart === "string" ? processStart : undefined)
  let how: "exited" | "stopped" | "timeout" = "exited"
  let over = false
  const stop = (reason: "stopped" | "timeout") => {
    if (over || how !== "exited") return
    how = reason
    if (running()) stopGroup(pid as number, () => !over)
  }
  let clock: ReturnType<typeof setTimeout> | undefined
  if (typeof deadline === "number") clock = setTimeout(() => stop("timeout"), Math.max(0, deadline - Date.now()))
  clock?.unref()

  const done = (async () => {
    while (running()) await Bun.sleep(WATCH_EVERY)
    const status = exitStatus(exitFile)
    if (status !== undefined) return { how, ...status }
    return { how: how === "exited" ? ("unknown" as const) : how }
  })().finally(() => {
    over = true
    clearTimeout(clock)
  })
  return { kind: "shell", title: job.title, done: done.then((ended) => toldOf(ended, timeout, job.outputFile)), stop: () => stop("stopped") }
}

// How a command ended, from its exit file, or undefined while it has none:
// read as a command here is, so a status that stands for a signal that
// ends a process is a death by that signal.
function exitStatus(file: string): Pick<Ended, "status" | "signal"> | undefined {
  try {
    const text = readFileSync(file, "utf8").trim()
    if (text === "") return undefined
    const status = Number(text)
    const signal = endedBy(status)
    return signal !== undefined ? { signal } : { status }
  } catch {
    return undefined
  }
}
