#!/usr/bin/env bun
// The `evie` command: starts Evie with its built-in plugins and runs the
// terminal client in the folder it was started in.
import { createInterface } from "node:readline"
import { createEvie, createLiveInput, loadPluginSettings, loadSettings, runTerminal, type Evie } from "./index.ts"

const env = await loadSettings(process.env)
const model = env.EVIE_MODEL
if (!model) {
  console.error(
    "Set EVIE_MODEL to the name of a model, as listed at https://openrouter.ai/models,\n" +
      "in the environment or in the .env file in Evie's data folder.",
  )
  process.exit(1)
}

// Which plugins load comes from the settings files. A problem with them,
// such as a plugin that cannot be loaded, stops Evie here with the reason.
let evie: Evie
try {
  const { plugins, projectPlugins } = await loadPluginSettings({ env })
  evie = await createEvie({ env, model, plugins, projectPlugins })
} catch (cause) {
  console.error(cause instanceof Error ? cause.message : String(cause))
  process.exit(1)
}

const session = { folder: process.cwd(), resume: process.argv.includes("--continue") || process.argv.includes("-c") }
// What the terminal client does on Ctrl-C: cancel the turn, or say that
// nothing is running, or that a cancel is already under way.
let interrupt = (): "cancelled" | "idle" | "again" => "idle"

try {
  if (process.stdin.isTTY && process.stdout.isTTY) {
    // A real terminal: keys arrive one at a time, and the line being typed is
    // kept at the bottom with Evie's output above it.
    const live = createLiveInput({
      write: (text) => void process.stdout.write(text),
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
      write: (text) => void process.stdout.write(text),
      onInterrupt: (handler) => (interrupt = handler),
    })
  }
} finally {
  await evie.close()
}
