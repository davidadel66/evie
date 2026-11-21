import { afterEach, expect, test } from "bun:test"
import { runTerminal, type Evie, type Plugin } from "../src/index.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The terminal client run over the network: Evie behind its server, and the
// terminal given the client that `connect` returns in place of Evie itself.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// Runs the terminal over the network with the given lines typed, and
// returns everything it printed once the input has ended.
async function typeOverTheNetwork(lines: string[] | (() => AsyncGenerator<string>), script: ScriptedReply[]) {
  const model = scriptedProvider(script)
  const served = await serveEvie([model.plugin])
  cleanups.push(served.stop)
  const client = await served.connect()
  let printed = ""
  await runTerminal({
    evie: client,
    folder: served.run.projectFolder,
    lines: typeof lines === "function" ? lines() : (async function* () {
      yield* lines
    })(),
    write: (text) => (printed += text),
  })
  return { printed, model, served, client: client as Evie }
}

test("a line typed is sent over the network, and the model's reply streams back", async () => {
  const { printed, model } = await typeOverTheNetwork(["hello"], [{ text: ["Hi", " there"] }])

  expect(model.requests[0]?.messages).toEqual([{ role: "user", text: "hello" }])
  expect(printed).toContain("Hi there")
})

test("each tool the model uses is shown, with what came back, as when Evie ran in the terminal", async () => {
  const model = scriptedProvider([
    { text: ["Looking."], calls: [{ name: "lookup", input: { word: "evie" } }] },
    { text: ["Found it."] },
  ])
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
  const served = await serveEvie([model.plugin, lookup])
  cleanups.push(served.stop)
  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "look it up"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain('Looking.\n⏺ lookup {"word":"evie"}\n  ⎿ evie: a personal agent (+1 more lines)\nFound it.\n')
})

test("a command that fails in the core fails in the client, with the core's words", async () => {
  const { printed, client } = await typeOverTheNetwork(["/nothing-by-this-name"], [])

  expect(printed).toContain('[error] There is no command "/nothing-by-this-name". The commands are: /compact.')
  await expect(client.send({ type: "cancel", session: "no-such-session" })).rejects.toThrow('The session "no-such-session" is not open.')
})

test("Ctrl-C stops the turn in progress over the network", async () => {
  let printed = ""
  let interrupt = (): string => "not set"
  const answers: string[] = []
  const model = scriptedProvider([{ hang: true, text: ["Thinking"] }])
  const served = await serveEvie([model.plugin])
  cleanups.push(served.stop)
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "think hard"
      while (!printed.includes("Thinking")) await Bun.sleep(1)
      answers.push(interrupt())
    })(),
    write: (text) => (printed += text),
    onInterrupt: (handler) => (interrupt = handler),
  })

  expect(answers).toEqual(["cancelled"])
  expect(printed).toContain("Thinking\n[cancelled]\n")
})

test("a question reaches the terminal over the network, and the number typed answers it", async () => {
  const answers: (string | null)[] = []
  const asksFirst: Plugin = {
    name: "asks-before-deploy",
    setup(host) {
      host.tools.add({ name: "deploy", description: "Deploys.", inputSchema: { type: "object" }, run: async () => "deployed" })
      host.on("before_tool", async (_call, context) => {
        const answer = await context.ask({ text: "Deploy to production?", choices: ["Yes", "No"] })
        answers.push(answer)
        if (answer !== "Yes") return { deny: "The owner did not allow it." }
      })
    },
  }
  const model = scriptedProvider([{ text: [], calls: [{ name: "deploy", input: {} }] }, { text: ["Done."] }])
  const served = await serveEvie([model.plugin, asksFirst])
  cleanups.push(served.stop)
  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "ship it"
      while (!printed.includes("Type a number.")) await Bun.sleep(1)
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[question] Deploy to production?\n  1. Yes\n  2. No\nType a number.\n")
  expect(printed).toContain("[answered] Yes\n")
  expect(answers).toEqual(["Yes"])
})

test("a slash command runs over the network and its text is shown", async () => {
  const greeting: Plugin = {
    name: "greeting",
    setup(host) {
      host.commands.add({ name: "greet", description: "Says hello.", run: (args) => `Hello, ${args}.` })
    },
  }
  const served = await serveEvie([scriptedProvider([]).plugin, greeting])
  cleanups.push(served.stop)
  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "/greet the owner"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("Hello, the owner.\n")
})

test("--continue over the network carries on the last session of the project", async () => {
  const model = scriptedProvider([{ text: ["First reply."] }, { text: ["Second reply."] }])
  const served = await serveEvie([model.plugin])
  cleanups.push(served.stop)
  const terminal = async (line: string, resume: boolean) => {
    let printed = ""
    await runTerminal({
      evie: await served.connect(),
      folder: served.run.projectFolder,
      resume,
      lines: (async function* () {
        yield line
      })(),
      write: (text) => (printed += text),
    })
    return printed
  }
  await terminal("first", false)
  const printed = await terminal("second", true)

  expect(printed).toContain("· continuing the last session")
  expect(model.requests[1]?.messages.filter((message) => message.role === "user")).toEqual([
    { role: "user", text: "first" },
    { role: "user", text: "second" },
  ])
})
