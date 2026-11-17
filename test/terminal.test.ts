import { afterEach, expect, test } from "bun:test"
import { basename } from "node:path"
import { runTerminal } from "../src/index.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

let stop: (() => Promise<void>) | undefined
afterEach(async () => {
  await stop?.()
  stop = undefined
})

// Runs the terminal client against Evie with the given lines typed, and
// returns everything it printed once the input has ended.
async function type(lines: string[], script: ScriptedReply[]) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin])
  stop = run.stop
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield* lines
    })(),
    write: (text) => (printed += text),
  })
  return { printed, model, run }
}

test("a line typed is sent to the model and its reply is printed as it streams", async () => {
  const { printed, model } = await type(["hello"], [{ text: ["Hi", " there"] }])

  expect(model.requests[0]?.messages).toEqual([{ role: "user", text: "hello" }])
  expect(printed).toContain("Hi there")
})

test("the terminal works in the project of the folder it was started in", async () => {
  const { printed, run } = await type([], [])

  expect(printed).toContain(`evie · ${basename(run.projectFolder)}`)
})

test("each line is its own message, and blank lines are not sent", async () => {
  const { model } = await type(["first", "   ", "second"], [{ text: ["one"] }, { text: ["two"] }])

  expect(model.requests.at(-1)?.messages.filter((message) => message.role === "user")).toEqual([
    { role: "user", text: "first" },
    { role: "user", text: "second" },
  ])
})

test("a turn that fails prints why", async () => {
  const { printed } = await type(["hello"], [{ text: [], error: "No API key. Set OPENROUTER_API_KEY to your OpenRouter key." }])

  expect(printed).toContain("[error] No API key. Set OPENROUTER_API_KEY to your OpenRouter key.")
})

test("each tool the model uses is shown on a line, with a short summary of what came back", async () => {
  const model = scriptedProvider([
    { text: ["Looking."], calls: [{ name: "lookup", input: { word: "evie" } }] },
    { text: ["Found it."] },
  ])
  const run = await startEvie([
    model.plugin,
    {
      name: "lookup-tool",
      setup(host) {
        host.tools.add({
          name: "lookup",
          description: "Looks a word up.",
          inputSchema: { type: "object", properties: { word: { type: "string" } } },
          run: async () => "evie: a personal agent\nsecond line\nthird line",
        })
      },
    },
  ])
  stop = run.stop
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "look it up"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain('Looking.\n⏺ lookup {"word":"evie"}\n  ⎿ evie: a personal agent (+2 more lines)\nFound it.\n')
})

test("Ctrl-C stops the turn in progress, and with nothing running says to quit", async () => {
  const model = scriptedProvider([{ hang: true, text: ["Thinking"] }])
  const run = await startEvie([model.plugin])
  stop = run.stop
  let printed = ""
  let interrupt = () => false
  const answers: boolean[] = []
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      answers.push(interrupt())
      yield "think hard"
      // Give the reply a moment to start, then press Ctrl-C.
      while (!printed.includes("Thinking")) await new Promise((resolve) => setTimeout(resolve, 1))
      answers.push(interrupt())
    })(),
    write: (text) => (printed += text),
    onInterrupt: (handler) => (interrupt = handler),
  })

  expect(answers).toEqual([false, true])
  expect(printed).toContain("Thinking\n[cancelled]\n")
})
