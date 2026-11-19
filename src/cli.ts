#!/usr/bin/env bun
// The `evie` command: starts Evie with its built-in plugins and runs the
// terminal client in the folder it was started in.
import { createInterface } from "node:readline"
import { createEvie, dataFolder, files, guard, loadSettings, models, openrouter, runTerminal, shell } from "./index.ts"

const env = await loadSettings(process.env)
const model = env.EVIE_MODEL
if (!model) {
  console.error(
    "Set EVIE_MODEL to the name of a model, as listed at https://openrouter.ai/models,\n" +
      "in the environment or in the .env file in Evie's data folder.",
  )
  process.exit(1)
}

const evie = await createEvie({ env, model, plugins: [files(), shell(), guard({ dataFolder: dataFolder(env) }), models(), openrouter({ env })] })

const input = createInterface({ input: process.stdin })
// Ctrl-C stops the turn in progress. With nothing running it ends the input,
// which quits. Pressed again while a turn is refusing to stop, it quits at once.
let interrupt = (): "cancelled" | "idle" | "again" => "idle"
const onCtrlC = () => {
  const did = interrupt()
  if (did === "idle") input.close()
  if (did === "again") process.exit(130)
}
input.on("SIGINT", onCtrlC)
process.on("SIGINT", onCtrlC)

try {
  await runTerminal({
    evie,
    folder: process.cwd(),
    resume: process.argv.includes("--continue") || process.argv.includes("-c"),
    lines: input,
    write: (text) => void process.stdout.write(text),
    onInterrupt: (handler) => (interrupt = handler),
  })
} finally {
  await evie.close()
}
