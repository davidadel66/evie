#!/usr/bin/env bun
// The `evie` command: starts Evie with its built-in plugins and runs the
// terminal client in the folder it was started in.
import { createInterface } from "node:readline"
import { createEvie, openrouter, runTerminal } from "./index.ts"

const env = process.env
const model = env.EVIE_MODEL
if (!model) {
  console.error("Set EVIE_MODEL to the name of a model, as listed at https://openrouter.ai/models.")
  process.exit(1)
}

const evie = await createEvie({ env, model, plugins: [openrouter({ env })] })

const input = createInterface({ input: process.stdin })
// Ctrl-C ends the input, which ends the terminal once the turn in progress is done.
input.on("SIGINT", () => input.close())
process.on("SIGINT", () => input.close())

try {
  await runTerminal({
    evie,
    folder: process.cwd(),
    lines: input,
    write: (text) => void process.stdout.write(text),
  })
} finally {
  await evie.close()
}
