import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { basename, join } from "node:path"
import { runTerminal, show, type Plugin } from "../src/index.ts"
import { say, startEvie } from "./support/harness.ts"
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
  let interrupt = (): string => "not set"
  const answers: string[] = []
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

  expect(answers).toEqual(["idle", "cancelled"])
  expect(printed).toContain("Thinking\n[cancelled]\n")
})

test("when the model asks for several tools at once, each result is shown under its own call", async () => {
  const model = scriptedProvider([
    { text: [], calls: [{ name: "lookup", input: { word: "one" } }, { name: "lookup", input: { word: "two" } }] },
    { text: ["Done."] },
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
          run: async (input) => `meaning of ${input.word}`,
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
      yield "look both up"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain(
    '⏺ lookup {"word":"one"}\n  ⎿ meaning of one\n⏺ lookup {"word":"two"}\n  ⎿ meaning of two\nDone.\n',
  )
})

// ---- The earlier conversation, when continuing ---------------------------------------------------

// A tool that looks a word up.
const lookup: Plugin = {
  name: "lookup-tool",
  setup(host) {
    host.tools.add({
      name: "lookup",
      description: "Looks a word up.",
      inputSchema: { type: "object", properties: { word: { type: "string" } } },
      run: async () => "evie: a personal agent\nsecond line",
    })
  },
}

// Runs the terminal to continue the last session, with nothing typed, and returns what it printed.
async function continueIn(run: Awaited<ReturnType<typeof startEvie>>) {
  let printed = ""
  await runTerminal({ evie: run.evie, folder: run.projectFolder, resume: true, lines: (async function* () {})(), write: (text) => (printed += text) })
  return printed
}

test("--continue prints the earlier conversation before the prompt, drawn as it was when it streamed", async () => {
  const model = scriptedProvider([{ text: ["Looking."], calls: [{ name: "lookup", input: { word: "evie" } }] }, { text: ["Found it."] }, { text: ["Hello."] }])
  const run = await startEvie([model.plugin, lookup])
  stop = run.stop
  await say(run, "look it up")
  await say(run, "thanks")

  const printed = await continueIn(run)

  expect(printed).toContain(
    "continuing the last session\n" +
      '\n› look it up\nLooking.\n⏺ lookup {"word":"evie"}\n  ⎿ evie: a personal agent (+1 more lines)\nFound it.\n' +
      "\n› thanks\nHello.\n" +
      "\n› ",
  )
  expect(printed).not.toContain("earlier events")
})

test("--continue in a session longer than a page prints its last 50 events, and says how many earlier ones are not shown", async () => {
  // Thirteen turns of four events each: 52 events, so the first two are not shown.
  const turns = Array.from({ length: 13 }, (_, at) => at + 1)
  const model = scriptedProvider(turns.map((turn) => ({ text: [`Reply ${turn}.`] })))
  const run = await startEvie([model.plugin])
  stop = run.stop
  for (const turn of turns) await say(run, `Message ${turn}`)

  const printed = await continueIn(run)

  expect(printed).toContain("continuing the last session\nReply 1.\n\n› Message 2\nReply 2.\n")
  expect(printed).toContain("\n› Message 13\nReply 13.\n[note] 2 earlier events of this session are not shown.\n\n› ")
  expect(printed).not.toContain("Message 1\n")
})

test("--continue does not draw again a question asked while the session was opening, which was shown and answered as it was asked", async () => {
  // A project that asks whether to trust it whenever a session opens in it, once told to.
  let asks = false
  const projectSettings = async (_project: unknown, { ask }: { ask: (question: { text: string; choices: string[] }) => Promise<string | null> }) => {
    if (asks) await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
    return { plugins: [], forNow: true as const }
  }
  const first = await startEvie([scriptedProvider([{ text: ["Hello."] }]).plugin], { projectSettings })
  await say(first, "hi")
  await first.evie.close()
  // Started again, so that continuing opens the session afresh, and its project asks.
  const run = await startEvie([scriptedProvider([]).plugin], { home: first.home, projectFolder: first.projectFolder, projectSettings })
  stop = async () => {
    await run.stop()
    await first.stop()
  }
  asks = true

  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    resume: true,
    lines: (async function* () {
      while (!printed.includes("Type a number.")) await Bun.sleep(1)
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[answered] Yes\nevie · ")
  expect(printed).toContain("continuing the last session\n\n› hi\nHello.\n\n› ")
  expect(printed.split("Trust this project?")).toHaveLength(2)
  expect(printed.split("[answered] Yes")).toHaveLength(2)
})

test("a session deleted from another client while the terminal has it open is said so, and the terminal ends", async () => {
  const model = scriptedProvider([{ text: ["Noted."] }])
  const run = await startEvie([model.plugin])
  stop = run.stop
  // Once the reply is in, another client deletes the session.
  run.evie.subscribe((event) => {
    if (event.type === "turn_end") setTimeout(() => void run.evie.send({ type: "delete_session", session: event.session }), 20)
  })
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    // The owner types one line and then nothing more, so only the delete can end the terminal.
    lines: (async function* () {
      yield "hello"
      await new Promise(() => {})
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("Noted.")
  expect(printed).toContain("[note] This session was deleted from another client, so this terminal is done with it.")
})

test("a file evie shows is named on one line, with the folder it is kept in", async () => {
  const model = scriptedProvider([{ text: ["Drawing it."], calls: [{ name: "show", input: { paths: ["chart.md", "notes.md"] } }] }, { text: ["Here."] }])
  const home = await mkdtemp(join(tmpdir(), "evie-home-"))
  const run = await startEvie([model.plugin, show({ dataFolder: home })], { home })
  stop = async () => {
    await run.stop()
    await rm(home, { recursive: true, force: true })
  }
  await writeFile(join(run.projectFolder, "chart.md"), "# Chart\n")
  await writeFile(join(run.projectFolder, "notes.md"), "# Notes\n")
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "show me"
    })(),
    write: (text) => (printed += text),
  })

  const day = /shown\/(\d{4}-\d{2}-\d{2})\//.exec(printed)?.[1]
  expect(printed).toContain(`⏺ show {"paths":["chart.md","notes.md"]}\n  ⎿ Shown chart.md, in .evie/shown/${day}/\n  ⎿ Shown notes.md, in .evie/shown/${day}/\nHere.\n`)
})

test("with a frame clock, streamed replies pace across frames and flush cleanly before a tool call or turn end", async () => {
  const model = scriptedProvider([{ hang: true, text: ["Thinking about the beds by the fence."] }])
  const run = await startEvie([model.plugin])
  stop = run.stop
  let pending: (() => void) | undefined
  let interrupt = (): string => "not set"
  const writes: string[] = []
  let printed = ""

  const running = runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "plan"
      // Wait for the delta to buffer before stepping frames, then cancel to force a flush.
      while (!pending) await Bun.sleep(5)
      const beforeStep = printed
      const apply = pending
      pending = undefined
      apply()
      expect(printed.length).toBeGreaterThan(beforeStep.length)
      expect(printed).not.toContain("Thinking about the beds by the fence.")
      interrupt()
    })(),
    write: (text) => {
      writes.push(text)
      printed += text
    },
    frame: (apply) => {
      pending = apply
      return () => {
        if (pending === apply) pending = undefined
      }
    },
    onInterrupt: (handler) => (interrupt = handler),
  })
  await running

  expect(printed).toContain("Thinking about the beds by the fence.\n[cancelled]\n")
  // The buffered reply was split across at least two writes (one paced slice + the flush on cancel).
  expect(writes.filter((text) => "Thinking about the beds by the fence.".includes(text) && text !== "\n").length).toBeGreaterThanOrEqual(2)
})

test("with a frame clock, a fast completed reply still glides out across frames before printing the next prompt", async () => {
  const model = scriptedProvider([{ text: ["A short fast reply."] }])
  const run = await startEvie([model.plugin])
  stop = run.stop
  let pending: (() => void) | undefined
  const slices: string[] = []
  let printed = ""

  const running = runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "hi"
      // Wait for the fast reply to arrive and buffer. Even though the turn
      // already finished on the server, the pacer keeps gliding until drained.
      while (!pending) await Bun.sleep(5)
      await Bun.sleep(20)
      expect(printed).not.toContain("A short fast reply.")
      while (pending) {
        const apply = pending
        pending = undefined
        apply()
      }
    })(),
    write: (text) => {
      if ("A short fast reply.".includes(text) && text !== "\n") slices.push(text)
      printed += text
    },
    frame: (apply) => {
      pending = apply
      return () => {
        if (pending === apply) pending = undefined
      }
    },
  })
  await running

  expect(slices.length).toBeGreaterThan(2)
  expect(slices.join("")).toBe("A short fast reply.")
  expect(printed).toContain("A short fast reply.\n\n› ")
})
