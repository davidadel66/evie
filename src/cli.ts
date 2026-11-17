#!/usr/bin/env bun
// The `evie` command: starts Evie with its built-in plugins and runs the
// terminal client in the folder it was started in.
import { createInterface } from "node:readline"
import { createEvie, files, loadSettings, openrouter, runTerminal, shell } from "./index.ts"

const env = await loadSettings(process.env)
const model = env.EVIE_MODEL
if (!model) {
  console.error(
    "Set EVIE_MODEL to the name of a model, as listed at https://openrouter.ai/models,\n" +
      "in the environment or in the .env file in Evie's data folder.",
  )
  process.exit(1)
}

const evie = await createEvie({ env, model, plugins: [files(), shell(), openrouter({ env })] })

const input = createInterface({ input: process.stdin })
// Ctrl-C stops the turn in progress. With nothing running it ends the input,
// which quits.
let interrupt = () => false
const onCtrlC = () => {
  if (!interrupt()) input.close()
}
input.on("SIGINT", onCtrlC)
process.on("SIGINT", onCtrlC)

try {
  await runTerminal({
    evie,
    folder: process.cwd(),
    lines: input,
    write: (text) => void process.stdout.write(text),
    onInterrupt: (handler) => (interrupt = handler),
  })
} finally {
  await evie.close()
}
