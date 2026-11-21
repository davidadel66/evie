import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { runTerminal, type Evie, type EvieEvent, type Plugin } from "../src/index.ts"
import { nextEvent, say, stored } from "./support/harness.ts"
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

// A deploy tool that asks the owner before it runs, and the answers it was given.
function asksBeforeDeploy() {
  const answers: (string | null)[] = []
  const plugin: Plugin = {
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
  return { plugin, answers }
}
const callsDeploy = (): ScriptedReply[] => [{ text: [], calls: [{ name: "deploy", input: {} }] }, { text: ["Done."] }]

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

test("a question reaches the terminal over the network, and the number typed answers it", async () => {
  const { plugin: asksFirst, answers } = asksBeforeDeploy()
  const model = scriptedProvider(callsDeploy())
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

// ---- Which events each client is sent ------------------------------------------------------

test("a client opening a session is sent its own opening question, and never a question from another client's session", async () => {
  const trusted = await tempFolder("evie-project-")
  const served = await serveEvie([scriptedProvider(callsDeploy()).plugin, asksBeforeDeploy().plugin], {
    // Opening this project stops to ask whether to trust it, as a project with a settings file of its own does.
    projectSettings: async (project, { ask }) => {
      if (project.folder === trusted) await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
      return []
    },
  })
  cleanups.push(served.stop)
  const first = await served.connect()
  const second = await served.connect()
  const secondSaw: EvieEvent[] = []
  second.subscribe((event) => secondSaw.push(event))
  const { session: firstSession } = await first.send({ type: "open_session", folder: served.run.projectFolder, questions: true })

  // The second client starts opening its project, which stops to ask.
  const trustAsked = nextEvent({ evie: second }, "question")
  const opening = second.send({ type: "open_session", folder: trusted, questions: true })
  const trust = await trustAsked
  // While it is still opening, the first client's turn asks a question of its own.
  const deployAsked = nextEvent({ evie: first }, "question")
  await first.send({ type: "send_message", session: firstSession, text: "ship it" })
  const deploy = await deployAsked
  await second.send({ type: "answer_question", session: trust.session, id: trust.body.id, choice: "Yes" })
  const { session: secondSession } = await opening
  const firstEnded = nextEvent({ evie: first }, "turn_end")
  await first.send({ type: "answer_question", session: firstSession, id: deploy.body.id, choice: "Yes" })
  await firstEnded

  expect(trust).toMatchObject({ session: secondSession, body: { text: "Trust this project?" } })
  expect(deploy.body.text).toBe("Deploy to production?")
  expect(secondSaw.filter((event) => event.session === firstSession)).toEqual([])
})

test("a client that closes while its session is still opening has the opening question refused, so the project is not left half open", async () => {
  const trusted = await tempFolder("evie-project-")
  const served = await serveEvie([scriptedProvider([]).plugin], {
    projectSettings: async (project, { ask }) => {
      if (project.folder === trusted) await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
      return []
    },
  })
  cleanups.push(served.stop)
  const leaving = await served.connect()
  const trustAsked = nextEvent({ evie: leaving }, "question")
  void leaving.send({ type: "open_session", folder: trusted, questions: true }).catch(() => {})
  const trust = await trustAsked
  // The terminal's window is closed while the question is on screen.
  await leaving.close()

  const next = await served.connect()
  const opened = await Promise.race([next.send({ type: "open_session", folder: trusted, questions: true }), Bun.sleep(2000).then(() => "still waiting" as const)])

  expect(opened).not.toBe("still waiting")
  expect(stored(served.run.events).find((event) => event.type === "answer" && event.body.id === trust.body.id)?.body).toEqual({ id: trust.body.id, choice: null })
})

// ---- Joining a session another client left running -----------------------------------------

test("a terminal that continues a session left running is shown the question waiting in it, and answering it lets the turn finish", async () => {
  const deploy = asksBeforeDeploy()
  const served = await serveEvie([scriptedProvider(callsDeploy()).plugin, deploy.plugin])
  cleanups.push(served.stop)
  const left = await served.connect()
  const { session } = await left.send({ type: "open_session", folder: served.run.projectFolder, questions: true })
  const asked = nextEvent(served.run, "question")
  await left.send({ type: "send_message", session, text: "ship it" })
  await asked
  // The terminal that started the turn goes away without answering.
  await left.close()

  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    resume: true,
    lines: (async function* () {
      while (!printed.includes("Type a number.")) await Bun.sleep(1)
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain(
    "· continuing the last session\n[note] A turn is still running in this session. Ctrl-C stops it.\n[question] Deploy to production?\n",
  )
  expect(printed).toContain("[answered] Yes\n")
  expect(printed).toContain("Done.")
  expect(deploy.answers).toEqual(["Yes"])
})

test("Ctrl-C in a terminal that continues a session left running stops that session's turn", async () => {
  const served = await serveEvie([scriptedProvider([{ hang: true, text: ["Thinking"] }]).plugin])
  cleanups.push(served.stop)
  const left = await served.connect()
  const { session } = await left.send({ type: "open_session", folder: served.run.projectFolder })
  const thinking = nextEvent(served.run, "assistant_delta")
  await left.send({ type: "send_message", session, text: "think hard" })
  await thinking
  await left.close()

  let printed = ""
  let interrupt = (): string => "not set"
  const answers: string[] = []
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    resume: true,
    lines: (async function* () {
      while (!printed.includes("Ctrl-C stops it.")) await Bun.sleep(1)
      answers.push(interrupt())
    })(),
    write: (text) => (printed += text),
    onInterrupt: (handler) => (interrupt = handler),
  })

  expect(answers).toEqual(["cancelled"])
  expect(printed).toContain("[cancelled]\n")
  expect(stored(served.run.events).findLast((event) => event.type === "turn_end" && event.session === session)?.body).toEqual({ reason: "cancelled" })
})

// ---- Several clients on one session ------------------------------------------------------

// What a terminal says when it leaves with a question still waiting.
const WAITING = "[note] A question is waiting for an answer. `evie -c` in a terminal comes back to it."

// The questions asked so far, in the order they were asked.
const questionsAsked = (events: EvieEvent[]) => stored(events).flatMap((event) => (event.type === "question" ? [event] : []))

test("a pipe that joins a session leaves when its input ends, saying a question is waiting, and another client can still answer it", async () => {
  const deploy = asksBeforeDeploy()
  const served = await serveEvie([scriptedProvider([{ text: ["Hi."] }, ...callsDeploy()]).plugin, deploy.plugin])
  cleanups.push(served.stop)
  // The session is opened and used by a client that shows questions, as a real terminal does.
  const shows = await served.connect()
  const { session } = await shows.send({ type: "open_session", folder: served.run.projectFolder, questions: true })
  await say({ evie: shows, session }, "hello")

  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    resume: true,
    questions: false,
    waitForJobs: true,
    lines: (async function* () {
      yield "ship it"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[question] Deploy to production? (not answered here)")
  expect(printed).toContain(WAITING)
  expect(stored(served.run.events).some((event) => event.type === "answer")).toBe(false)
  // The question is still open: the other client answers it, and the turn finishes.
  const ended = nextEvent(served.run, "turn_end")
  await shows.send({ type: "answer_question", session, id: questionsAsked(served.run.events)[0]!.body.id, choice: "Yes" })
  expect(await ended).toMatchObject({ session, body: { reason: "completed" } })
  expect(deploy.answers).toEqual(["Yes"])
})

test("when one of two terminals on a session stops typing, the question waiting in it is left for the other to answer", async () => {
  const deploy = asksBeforeDeploy()
  const served = await serveEvie([scriptedProvider([{ text: ["Hi."] }, ...callsDeploy()]).plugin, deploy.plugin])
  cleanups.push(served.stop)
  let first = ""
  let second = ""
  let joined = () => {}
  const secondJoined = new Promise<void>((resolve) => (joined = resolve))

  const firstTerminal = runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "hello"
      await secondJoined
      yield "ship it"
      while (!first.includes("Type a number.")) await Bun.sleep(1)
      // The input ends here, with the question on screen.
    })(),
    write: (text) => (first += text),
  })
  while (!first.includes("Hi.")) await Bun.sleep(1)
  const secondTerminal = runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    resume: true,
    lines: (async function* () {
      while (!second.includes("continuing the last session")) await Bun.sleep(1)
      joined()
      while (!second.includes("Type a number.")) await Bun.sleep(1)
      // Answered only once the first terminal has gone.
      await firstTerminal
      yield "1"
    })(),
    write: (text) => (second += text),
  })
  await Promise.all([firstTerminal, secondTerminal])

  expect(first).toContain(WAITING)
  expect(stored(served.run.events).flatMap((event) => (event.type === "answer" ? [event.body.choice] : []))).toEqual(["Yes"])
  expect(second).toContain("[answered] Yes\n")
  expect(second).toContain("Done.")
  expect(deploy.answers).toEqual(["Yes"])
})

test("a pipe alone in its session, whose questions are refused at once, waits for the turn to finish after its input ends", async () => {
  const deploy = asksBeforeDeploy()
  const served = await serveEvie([scriptedProvider(callsDeploy()).plugin, deploy.plugin])
  cleanups.push(served.stop)
  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    questions: false,
    waitForJobs: true,
    lines: (async function* () {
      yield "ship it"
    })(),
    write: (text) => (printed += text),
  })

  expect(deploy.answers).toEqual([null])
  expect(printed).toContain("Done.")
  expect(printed).not.toContain("A question is waiting")
})

// ---- Leaving, and losing the connection ------------------------------------------------

test("closing a client stops nothing: a turn still running carries on in the server", async () => {
  let release = () => {}
  const gate = new Promise<void>((resolve) => (release = resolve))
  const slow: Plugin = {
    name: "slow-model",
    setup(host) {
      host.providers.add({
        name: "slow",
        async *stream() {
          yield { type: "text", text: "Working" }
          await gate
          yield { type: "text", text: " and done." }
          yield { type: "finish", reason: "stop" }
        },
      })
    },
  }
  const served = await serveEvie([slow])
  cleanups.push(served.stop)
  const client = await served.connect()
  const { session } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  const started = new Promise<void>((resolve) => client.subscribe((event) => event.type === "assistant_delta" && resolve()))
  await client.send({ type: "send_message", session, text: "do the work" })
  await started

  await client.close()
  const ended = nextEvent(served.run, "turn_end")
  release()

  expect(await ended).toMatchObject({ session, body: { reason: "completed" } })
  const reply = stored(served.run.events).findLast((event) => event.type === "assistant_message" && event.session === session)
  expect(reply?.body).toMatchObject({ text: "Working and done." })
})

test("when the connection drops, a command still waiting fails with a clear error, and the client is told why", async () => {
  let finish = (_text: string) => {}
  const waits: Plugin = {
    name: "waits",
    setup(host) {
      host.commands.add({ name: "wait", description: "Waits.", run: () => new Promise<string>((resolve) => (finish = resolve)) })
    },
  }
  const served = await serveEvie([scriptedProvider([]).plugin, waits])
  cleanups.push(served.stop)
  cleanups.push(async () => finish("done"))
  let dropped: string | undefined
  const client = await served.connect({ onDrop: (reason) => (dropped = reason) })
  const { session } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  const waiting = client.send({ type: "run_command", session, name: "wait", args: "" })

  await served.server.stop()

  await expect(waiting).rejects.toThrow("The connection to Evie was lost before it answered")
  expect(dropped).toBeString()
  await expect(client.send({ type: "cancel", session })).rejects.toThrow("Not connected to Evie")
})

test("a message that is not a command closes that connection only, saying why, and the server carries on", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  const socket = new WebSocket(`${served.server.url.replace(/^http/, "ws")}/ws`, { headers: { Authorization: `Bearer ${served.token}` } })
  await new Promise((resolve) => socket.addEventListener("open", resolve))
  const closed = new Promise<CloseEvent>((resolve) => socket.addEventListener("close", resolve))

  socket.send("not a command")

  expect((await closed).reason).toBe('Each message must be JSON of the form { "id": ..., "command": ... }')
  const client = await served.connect()
  expect((await client.send({ type: "open_session", folder: served.run.projectFolder })).session).toBeString()
})
