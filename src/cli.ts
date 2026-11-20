#!/usr/bin/env bun
// The `evie` command: starts Evie with its built-in plugins and runs the
// terminal client in the folder it was started in.
import { createInterface } from "node:readline"
import { createEvie, createLiveInput, loadSettingsFiles, loadSettings, runTerminal, type Evie } from "./index.ts"

// Keys come from the environment and the .env file in the data folder.
const env = await loadSettings(process.env)

// The model, the reply limit and the plugins come from the settings files.
// A problem with them, such as a plugin that cannot be loaded, stops Evie
// here with the reason.
const fail = (cause: unknown): never => {
  console.error(cause instanceof Error ? cause.message : String(cause))
  process.exit(1)
}
async function start(): Promise<Evie> {
  const settings = await loadSettingsFiles({ env })
  return createEvie({
    env,
    plugins: settings.plugins,
    projectSettings: settings.projectSettings,
    settingsFile: settings.file,
    ...(settings.model !== undefined && { model: settings.model }),
    ...(settings.maxTokens !== undefined && { maxTokens: settings.maxTokens }),
    ...(settings.compaction !== undefined && { compaction: settings.compaction }),
  })
}
const evie = await start().catch(fail)

// Prints to the terminal. If whatever was reading Evie's output has gone
// away, as when it is piped into a program that stops early, there is no one
// left to print for, so Evie quits quietly.
function print(text: string) {
  try {
    process.stdout.write(text)
  } catch (cause) {
    if ((cause as { code?: string }).code === "EPIPE") process.exit(0)
    throw cause
  }
}
process.stdout.on("error", (cause: { code?: string }) => {
  if (cause.code === "EPIPE") process.exit(0)
})

const session = { folder: process.cwd(), resume: process.argv.includes("--continue") || process.argv.includes("-c") }
// What the terminal client does on Ctrl-C: cancel the turn, or say that
// nothing is running, or that a cancel is already under way.
let interrupt = (): "cancelled" | "idle" | "again" => "idle"

let problem: unknown
try {
  if (process.stdin.isTTY && process.stdout.isTTY) {
    // A real terminal: keys arrive one at a time, and the line being typed is
    // kept at the bottom with Evie's output above it.
    const live = createLiveInput({
      write: print,
      columns: () => process.stdout.columns || 80,
    })
    live.onInterrupt(() => {
      const did = interrupt()
      // With nothing running, Ctrl-C quits. Pressed again while a turn is
      // refusing to stop, it quits at once.
      if (did === "idle") live.close()
      if (did === "again") {
        process.stdin.setRawMode(false)
        process.exit(130)
      }
    })
    process.stdin.setRawMode(true)
    process.stdin.setEncoding("utf8")
    process.stdin.on("data", (data: string) => live.key(data))
    try {
      await runTerminal({
        evie,
        ...session,
        lines: live.lines,
        write: live.write,
        prompt: false,
        typing: live.typing,
        onInterrupt: (handler) => (interrupt = handler),
      })
    } finally {
      live.close()
      process.stdin.setRawMode(false)
      process.stdin.pause()
    }
  } else {
    // Not a terminal, such as input from a pipe: read whole lines.
    const input = createInterface({ input: process.stdin })
    const onCtrlC = () => {
      const did = interrupt()
      if (did === "idle") input.close()
      if (did === "again") process.exit(130)
    }
    input.on("SIGINT", onCtrlC)
    process.on("SIGINT", onCtrlC)
    await runTerminal({
      evie,
      ...session,
      lines: input,
      write: print,
      // Nobody is typing, so a line must not be taken as an answer.
      questions: false,
      onInterrupt: (handler) => (interrupt = handler),
    })
  }
} catch (cause) {
  // A session that could not be opened, such as when no model is set.
  problem = cause
} finally {
  await evie.close()
}
if (problem !== undefined) fail(problem)
