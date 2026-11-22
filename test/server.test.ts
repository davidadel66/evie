import { afterEach, expect, setSystemTime, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import {
  connect,
  ConnectError,
  runTerminal,
  startServer,
  type ClientMessage,
  type ConnectOptions,
  type Evie,
  type EvieEvent,
  type EvieOptions,
  type Plugin,
  type ServerMessage,
} from "../src/index.ts"
import { nextEvent, say, startEvie, stored, type Run } from "./support/harness.ts"
import { lineTo } from "./support/line.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"
import { commandId } from "../src/server/connect.ts"
import { openTokens } from "../src/server/tokens.ts"

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
  // Every client is sent each session's summary, but none of the events of a session it has not opened.
  expect(secondSaw.filter((event) => event.session === firstSession && event.type !== "summary")).toEqual([])
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

  // The earlier conversation comes first, without the question, which is shown after it with its choices.
  expect(printed).toContain(
    "· continuing the last session\n\n› ship it\n⏺ deploy {}\n[note] A turn is still running in this session. Ctrl-C stops it.\n[question] Deploy to production?\n",
  )
  expect(printed.split("Deploy to production?")).toHaveLength(2)
  // The tool's result, once the question is answered, comes under its call.
  expect(printed).toContain("[answered] Yes\n  ⎿ deployed\nDone.\n")
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

// A door a test opens, for a model to wait at part way through a reply.
function gate() {
  let open = () => {}
  const opened = new Promise<void>((resolve) => (open = resolve))
  return { open, opened }
}

// A model whose replies are written piece by piece: a string is text, a
// promise is waited for, so the test decides when the reply goes on, and
// `{ call }` asks for a tool with no input. One reply per request, in turn.
function piecemealModel(replies: (string | Promise<void> | { call: string })[][]): Plugin {
  let asked = 0
  return {
    name: "piecemeal-model",
    setup(host) {
      host.providers.add({
        name: "piecemeal",
        async *stream(_request, signal) {
          const pieces = replies[asked++] ?? []
          let calls = 0
          for (const piece of pieces) {
            if (signal.aborted) return
            if (typeof piece === "string") yield { type: "text", text: piece }
            else if ("call" in piece) yield { type: "tool_call", id: `call_${asked}_${calls++}`, name: piece.call, arguments: "{}" }
            else await piece
          }
          yield { type: "finish", reason: calls > 0 ? "tool_calls" : "stop" }
        },
      })
    },
  }
}

test("a terminal that continues a session part way through a reply shows the reply so far, then the rest as it comes", async () => {
  const more = gate()
  const served = await serveEvie([piecemealModel([["Work", "ing", more.opened, " and done."]])])
  cleanups.push(served.stop)
  const left = await served.connect()
  const { session } = await left.send({ type: "open_session", folder: served.run.projectFolder })
  const writing = new Promise<void>((resolve) => served.run.evie.subscribe((event) => event.type === "assistant_delta" && event.text === "ing" && resolve()))
  await left.send({ type: "send_message", session, text: "do the work" })
  await writing
  await left.close()

  let printed = ""
  await runTerminal({
    evie: await served.connect(),
    folder: served.run.projectFolder,
    resume: true,
    lines: (async function* () {
      while (!printed.includes("Ctrl-C stops it.")) await Bun.sleep(1)
      more.open()
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[note] A turn is still running in this session. Ctrl-C stops it.\nWorking and done.\n")
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

// ---- Catching up after the line drops ---------------------------------------------------------

// A tool that looks something up.
const lookup: Plugin = {
  name: "lookup-tool",
  setup(host) {
    host.tools.add({ name: "lookup", description: "Looks something up.", inputSchema: { type: "object" }, run: async () => "found it" })
  },
}

// How long a client in these tests waits before it first tries again, in
// place of a second, so that the tests do not wait for real seconds.
const FIRST_WAIT = 20

// Evie behind its server, and a line between them that the test can cut
// and mend while the server runs on.
async function serveOverALine(plugins: Plugin[], options: Parameters<typeof serveEvie>[1] = {}) {
  const served = await serveEvie(plugins, options)
  cleanups.push(served.stop)
  const line = await lineTo(served.server.url)
  cleanups.push(line.stop)
  return { served, line, connect: (given: Partial<ConnectOptions> = {}) => served.connect({ url: line.url, firstWait: FIRST_WAIT, ...given }) }
}

// Settles once Evie has written this piece of a reply, whether or not any client heard it.
function written(run: Run, text: string) {
  return new Promise<void>((resolve) => {
    const stop = run.evie.subscribe((event) => {
      if (event.type !== "assistant_delta" || event.text !== text) return
      stop()
      resolve()
    })
  })
}

test("when the line drops part way through a reply that uses a tool, the terminal shows every piece of text and every tool line once, in order, once it is back", async () => {
  const carryOn = gate()
  const finish = gate()
  const { served, line, connect } = await serveOverALine([
    piecemealModel([["Hel", "lo", carryOn.opened, " world.", { call: "lookup" }], ["Do", finish.opened, "ne."]]),
    lookup,
  ])
  const writingDo = written(served.run, "Do")
  let printed = ""
  await runTerminal({
    evie: await connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "look it up"
      while (!printed.includes("Hello")) await Bun.sleep(1)
      await line.cut()
      // While the line is down the reply goes on, the tool runs, and the next reply starts.
      carryOn.open()
      await writingDo
      line.mend()
      while (!printed.includes("Do")) await Bun.sleep(1)
      finish.open()
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("Hello world.\n⏺ lookup {}\n  ⎿ found it\nDone.\n")
  for (const piece of ["Hel", "world", "lookup", "found it", "Do"]) expect(printed.split(piece)).toHaveLength(2)
})

test("what is written while the reply with what was missed is on its way is shown once, after it", async () => {
  const carryOn = gate()
  const finish = gate()
  const release = gate()
  const { served, line, connect } = await serveOverALine([
    piecemealModel([["Hel", "lo", carryOn.opened, " world.", { call: "lookup" }], ["Do", finish.opened, "ne."]]),
    lookup,
  ])
  const writingDo = written(served.run, "Do")
  let printed = ""
  await runTerminal({
    evie: await connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      yield "look it up"
      while (!printed.includes("Hello")) await Bun.sleep(1)
      await line.cut()
      carryOn.open()
      await writingDo
      // What the client missed is held up on its way back, as by a slow
      // network, while the reply is written to its end.
      const caught = line.holdReplyTo("open_session", release.opened)
      line.mend()
      await caught
      const ended = nextEvent(served.run, "turn_end")
      finish.open()
      await ended
      release.open()
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("Hello world.\n⏺ lookup {}\n  ⎿ found it\nDone.\n")
  for (const piece of ["Hel", "world", "lookup", "found it", "Do", "ne."]) expect(printed.split(piece)).toHaveLength(2)
})

test("a session opened just as the line dropped is shown as it stands once the line is back, though the reply comes from the server's memory", async () => {
  const more = gate()
  const rest = gate()
  const { served, line, connect } = await serveOverALine([piecemealModel([["Work", "ing", more.opened, " and", rest.opened, " done."]])])
  // Another client leaves the session with its reply part written.
  const left = await served.connect()
  const { session } = await left.send({ type: "open_session", folder: served.run.projectFolder })
  const writing = written(served.run, "ing")
  await left.send({ type: "send_message", session, text: "do the work" })
  await writing
  await left.close()

  // The reply to the terminal's opening is lost, and the reply goes on while the line is down.
  const lost = line.loseReplyTo("open_session")
  const writingMore = written(served.run, " and")
  let printed = ""
  // The terminal reads no line until its session is open, so this goes on beside it.
  const meanwhile = (async () => {
    await lost
    more.open()
    await writingMore
    line.mend()
    while (!printed.includes(" and")) await Bun.sleep(1)
    rest.open()
  })()
  await runTerminal({
    evie: await connect(),
    folder: served.run.projectFolder,
    resume: true,
    lines: (async function* () {
      await meanwhile
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[note] A turn is still running in this session. Ctrl-C stops it.\nWorking and done.\n")
})

test("an open whose reply was lost, sent again after the session's turn has ended and its question was answered, is given where the session stands now, not what the server remembers", async () => {
  const { served, line, connect } = await serveOverALine([scriptedProvider(callsDeploy()).plugin, asksBeforeDeploy().plugin])
  // Another client leaves a turn running that stops to ask a question.
  const left = await served.connect()
  const { session } = await left.send({ type: "open_session", folder: served.run.projectFolder, questions: true })
  const asked = nextEvent(served.run, "question")
  await left.send({ type: "send_message", session, text: "ship it" })
  const question = await asked

  // The reply to this client's open is lost, with the turn running and the question waiting.
  const lost = line.loseReplyTo("open_session")
  const opening = (await connect()).send({ type: "open_session", folder: served.run.projectFolder, continue: true })
  await lost
  // While the line is down, the question is answered and the turn ends.
  const ended = nextEvent(served.run, "turn_end")
  await left.send({ type: "answer_question", session, id: question.body.id, choice: "Yes" })
  await ended
  line.mend()

  expect(await opening).toMatchObject({ session, resumed: true, turnRunning: false, openQuestions: [] })
})

// A tool that hands work to the background, which ends when the test says.
function backgroundWork() {
  const ending: ((result: string) => void)[] = []
  const plugin: Plugin = {
    name: "background-work",
    setup(host) {
      host.tools.add({
        name: "start_work",
        description: "Starts some work in the background.",
        inputSchema: { type: "object" },
        run: async (_input, context) => {
          let end = (_result: string) => {}
          const done = new Promise<string>((resolve) => (end = resolve))
          ending.push(end)
          const job = context.jobs.start({ kind: "work", title: "the build", done, stop: () => end("It was stopped.") })
          return `Started as job ${job.id}.`
        },
      })
    },
  }
  return { plugin, finish: (result: string) => ending.shift()?.(result) }
}
const startsWork = (): ScriptedReply[] => [{ text: [], calls: [{ name: "start_work", input: {} }] }, { text: ["Started it."] }]

// Settles once the log holds this many ends of turns.
async function turnsEnded(run: Run, count: number) {
  while (stored(run.events).filter((event) => event.type === "turn_end").length < count) await Bun.sleep(1)
}

test("a background job that ends while the line is down is shown as ended once it is back, and a pipe waiting for it leaves", async () => {
  const work = backgroundWork()
  const { served, line, connect } = await serveOverALine([scriptedProvider([...startsWork(), { text: ["The build passed."] }]).plugin, work.plugin])
  let printed = ""
  await runTerminal({
    evie: await connect(),
    folder: served.run.projectFolder,
    questions: false,
    waitForJobs: true,
    lines: (async function* () {
      yield "build it"
      await turnsEnded(served.run, 1)
      await line.cut()
      // The work ends while the line is down, and Evie deals with it.
      work.finish("All passed.")
      await turnsEnded(served.run, 2)
      line.mend()
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[background] job 1 started: the build\n")
  expect(printed).toMatch(/\[background\] job 1 finished after \d+s: the build\nThe build passed\./)
  expect(printed.split("[background] job 1 finished")).toHaveLength(2)
})

// A project that asks whether to trust it each time its plugins are set up,
// until the owner says yes, as one with a settings file of its own does.
const asksTrust: NonNullable<EvieOptions["projectSettings"]> = async (_project, { ask }) => {
  const answer = await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
  return { plugins: [], ...(answer !== "Yes" && { forNow: true as const }) }
}

test("a question whether to trust the project, refused because the line dropped while it was on screen, is shown as not answered once the line is back, and the session is open", async () => {
  const { served, line, connect } = await serveOverALine([scriptedProvider([{ text: ["Hi."] }]).plugin], { projectSettings: asksTrust })
  const refused = () => {
    const trust = questionsAsked(served.run.events).at(-1)
    return stored(served.run.events).some((event) => event.type === "answer" && event.body.id === trust?.body.id)
  }
  let printed = ""
  await runTerminal({
    evie: await connect(),
    folder: served.run.projectFolder,
    lines: (async function* () {
      while (!printed.includes("Type a number.")) await Bun.sleep(1)
      await line.cut()
      // The server refuses the question, so that the project is not left half set up.
      while (!refused()) await Bun.sleep(1)
      line.mend()
      while (!printed.includes("[not answered]")) await Bun.sleep(1)
      yield "hello"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[question] Trust this project?\n  1. Yes\n  2. No\nType a number.\n")
  expect(printed.split("[question] Trust this project?")).toHaveLength(2)
  expect(printed).toContain("[not answered]\n")
  expect(printed).toContain("Hi.")
})

test("a session continued by its folder is caught up on at once, so what is written while the server learns which session it is is shown, and nothing twice", async () => {
  const more = gate()
  const rest = gate()
  const served = await serveEvie([piecemealModel([["Work", "ing", more.opened, " and", rest.opened, " done."]])])
  cleanups.push(served.stop)
  // Another client leaves the session with its reply part written.
  const left = await served.connect()
  const { session } = await left.send({ type: "open_session", folder: served.run.projectFolder })
  const writing = written(served.run, "ing")
  await left.send({ type: "send_message", session, text: "do the work" })
  await writing
  await left.close()

  // The core, with the reply going on in the moment between its reading
  // where a session opened by its folder stands and the server learning
  // which session that is. The open by its id that follows is slow to be
  // answered once it has read where the session stands, and the rest of the
  // reply is written meanwhile.
  let gapped = false
  const core: Evie = {
    async send(command) {
      const reply = await served.run.evie.send(command)
      if (command.type === "open_session" && command.folder !== undefined && !gapped) {
        gapped = true
        const writingMore = written(served.run, " and")
        more.open()
        await writingMore
      }
      if (command.type === "open_session" && command.session !== undefined) {
        const ended = nextEvent(served.run, "turn_end")
        rest.open()
        await ended
      }
      return reply
    },
    subscribe: (listener) => served.run.evie.subscribe(listener),
    close: () => served.run.evie.close(),
  }
  const server = startServer({ evie: core, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const client = await connect({ url: server.url, token: served.token })
  cleanups.push(() => client.close())
  let printed = ""
  await runTerminal({
    evie: client,
    folder: served.run.projectFolder,
    resume: true,
    lines: (async function* () {})(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[note] A turn is still running in this session. Ctrl-C stops it.\nWorking and done.\n")
  for (const piece of ["Work", " and", " done."]) expect(printed.split(piece)).toHaveLength(2)
})

// How long the client in the test below lets its line carry nothing before
// it takes the line to have dropped, in place of 45 seconds. The server, the
// line and the client all run in the test's one process, so when a busy
// machine holds that process up, the server cannot send its keepalives
// either, and once the process runs again the client's wait is over before
// the next keepalive can reach it: it drops a line that was fine and
// connects again. So the wait is far longer than the 20 milliseconds between
// keepalives, and than the hold-ups of up to about half a second seen on a
// busy test machine.
const DEAD_AFTER = 1000

test("a line that goes silent without closing, as when a laptop wakes on another network, is noticed, and the client connects again", async () => {
  const { served, line, connect } = await serveOverALine([scriptedProvider([{ text: ["Hi."] }, { text: ["Hi again."] }]).plugin], { keepaliveEvery: 20 })
  const drops: string[] = []
  let back = 0
  const client = await connect({ deadAfter: DEAD_AFTER, onDrop: (reason) => drops.push(reason), onBack: () => back++ })
  const { session } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  await say({ evie: client, session }, "hello")
  // A line that is only quiet, for twice as long as the client waits, is
  // kept: the server's keepalives come through it.
  await Bun.sleep(DEAD_AFTER * 2)
  expect(drops).toEqual([])

  line.freeze()
  while (back === 0) await Bun.sleep(5)
  await say({ evie: client, session }, "are you there?")

  expect(drops).toEqual(["the server stopped answering"])
  const replies = stored(served.run.events).flatMap((event) => (event.type === "assistant_message" && event.session === session ? [event.body.text] : []))
  expect(replies).toEqual(["Hi.", "Hi again."])
})

test("a client closed while its line is down stops trying, and a command waiting for the line fails, saying so", async () => {
  const { served, line, connect } = await serveOverALine([scriptedProvider([]).plugin])
  let dropped = 0
  let back = 0
  const client = await connect({ onDrop: () => dropped++, onBack: () => back++ })
  const { session } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  await line.cut()
  while (dropped === 0) await Bun.sleep(1)
  const waiting = client.send({ type: "cancel", session })

  await client.close()
  line.mend()

  await expect(waiting).rejects.toThrow("The connection to evie was lost before it answered: the connection was closed.")
  await expect(client.send({ type: "cancel", session })).rejects.toThrow("Not connected to evie: the connection was closed.")
  // Past the first few times it would have tried again.
  await Bun.sleep(FIRST_WAIT * 10)
  expect(back).toBe(0)
})

test("a message whose reply was lost when the line dropped is sent again once the line is back, and carried out once", async () => {
  const { served, line, connect } = await serveOverALine([scriptedProvider([{ text: ["Hi."] }]).plugin])
  const client = await connect()
  const { session } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  const ended = nextEvent(served.run, "turn_end")

  const lost = line.loseReplyTo("send_message")
  const sent = client.send({ type: "send_message", session, text: "hello" })
  await lost
  line.mend()
  await sent
  await ended

  expect(stored(served.run.events).filter((event) => event.type === "user_message" && event.session === session).map((event) => event.body)).toEqual([
    { text: "hello" },
  ])
})

test("a command sent again while it is still being carried out waits for it, and is answered with its reply", async () => {
  let runs = 0
  const finishing: ((text: string) => void)[] = []
  const waits: Plugin = {
    name: "waits",
    setup(host) {
      host.commands.add({
        name: "wait",
        description: "Waits.",
        run: () => {
          runs += 1
          return new Promise<string>((resolve) => finishing.push(resolve))
        },
      })
    },
  }
  const { served, line, connect } = await serveOverALine([scriptedProvider([]).plugin, waits])
  cleanups.push(async () => finishing.forEach((finish) => finish("done")))
  const client = await connect()
  const { session } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  const waiting = client.send({ type: "run_command", session, name: "wait", args: "" })
  while (runs === 0) await Bun.sleep(1)

  await line.cut()
  const sentAgain = line.passedOn('"run_command"')
  line.mend()
  await sentAgain
  finishing[0]!("done")

  expect(await waiting).toEqual({ text: "done" })
  expect(runs).toBe(1)
})

test("a client makes its commands' ids without crypto.randomUUID, which a browser leaves out of a page served over plain HTTP from another computer", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  const client = await served.connect()
  Object.defineProperty(crypto, "randomUUID", { value: undefined, configurable: true })
  try {
    await client.send({ type: "cancel", session: served.run.session })
  } finally {
    delete (crypto as { randomUUID?: unknown }).randomUUID
  }

  // Each is 128 random bits, as hex, so no two are the same.
  const ids = Array.from({ length: 1000 }, () => commandId())
  for (const id of ids) expect(id).toMatch(/^[0-9a-f]{32}$/)
  expect(new Set(ids).size).toBe(1000)
  expect(typeof crypto.randomUUID).toBe("function")
})

// A command that counts the times it is carried out, and says which time it is.
function counted() {
  let runs = 0
  const plugin: Plugin = {
    name: "counted",
    setup(host) {
      host.commands.add({ name: "count", description: "Counts.", run: () => `run ${++runs}` })
    },
  }
  return { plugin, runs: () => runs }
}

test("a command whose reply was lost is carried out once when it is sent again, however long the line was down", async () => {
  const count = counted()
  // The server's regular checks run every few milliseconds, in place of every few seconds.
  const { served, line, connect } = await serveOverALine([scriptedProvider([]).plugin, count.plugin], { recheckEvery: 5 })
  const client = await connect()
  const lost = line.loseReplyTo("run_command")
  const sent = client.send({ type: "run_command", session: served.run.session, name: "count", args: "" })
  await lost

  // The line stays down overnight: the clock moves on twelve hours while the server's checks run.
  setSystemTime(new Date(Date.now() + 12 * 60 * 60_000))
  try {
    await Bun.sleep(50)
  } finally {
    setSystemTime()
  }
  line.mend()

  expect(await sent).toEqual({ text: "run 1" })
  expect(count.runs()).toBe(1)
})

// A connection of this device's own to the server, as a client written
// apart from `connect` would make, which sends commands with the ids the
// test chooses. `heard` is everything the server sent on it, in order.
async function ownLine(url: string, token: string) {
  const socket = new WebSocket(`${url.replace(/^http/, "ws")}/ws`, { headers: { Authorization: `Bearer ${token}` } })
  await new Promise((resolve) => socket.addEventListener("open", resolve, { once: true }))
  cleanups.push(async () => socket.close())
  const closed = new Promise<CloseEvent>((resolve) => socket.addEventListener("close", resolve, { once: true }))
  const heard: ServerMessage[] = []
  const waiting = new Map<string, (reply: Extract<ServerMessage, { reply: string }>) => void>()
  socket.addEventListener("message", (message) => {
    const received = JSON.parse(String(message.data)) as ServerMessage
    if ("keepalive" in received) return
    heard.push(received)
    if ("reply" in received) waiting.get(received.reply)?.(received)
  })
  return {
    heard,
    // Settles once the server has closed the connection.
    closed,
    // Sends a command, and settles with its reply.
    send(message: ClientMessage) {
      return new Promise<Extract<ServerMessage, { reply: string }>>((resolve) => {
        waiting.set(message.id, resolve)
        socket.send(JSON.stringify(message))
      })
    },
  }
}

// Runs the counting command with this id, as a command sent once when `once` is given, and gives back what it returned.
async function countAs(line: Awaited<ReturnType<typeof ownLine>>, session: string, id: string, once?: true) {
  const reply = await line.send({ id, command: { type: "run_command", session, name: "count", args: "" }, ...(once && { once }) })
  return "result" in reply ? reply.result : reply
}

test("the server remembers the replies to the last 200 commands of each device, and the 201st pushes out the first", async () => {
  const count = counted()
  const served = await serveEvie([scriptedProvider([]).plugin, count.plugin])
  cleanups.push(served.stop)
  const own = await ownLine(served.server.url, served.token)
  for (let n = 1; n <= 200; n++) await countAs(own, served.run.session, `command ${n}`)
  expect(await countAs(own, served.run.session, "command 1")).toEqual({ text: "run 1" })

  await countAs(own, served.run.session, "command 201")

  expect(await countAs(own, served.run.session, "command 2")).toEqual({ text: "run 2" })
  expect(await countAs(own, served.run.session, "command 1")).toEqual({ text: "run 202" })
  expect(count.runs()).toBe(202)
})

test("a command still being carried out is never pushed out of the server's memory, however many of the device's commands come after it", async () => {
  let runs = 0
  let finish = (_text: string) => {}
  const waits: Plugin = {
    name: "waits",
    setup(host) {
      host.commands.add({
        name: "wait",
        description: "Waits, the first time it is run.",
        run: () => (++runs === 1 ? new Promise<string>((resolve) => (finish = resolve)) : "carried out again"),
      })
    },
  }
  const count = counted()
  const served = await serveEvie([scriptedProvider([]).plugin, waits, count.plugin])
  cleanups.push(served.stop)
  const session = served.run.session
  const wait = { type: "run_command", session, name: "wait", args: "" } as const
  const own = await ownLine(served.server.url, served.token)
  const waiting = own.send({ id: "waiting", command: wait })
  while (runs === 0) await Bun.sleep(1)
  for (let n = 1; n <= 200; n++) await countAs(own, session, `command ${n}`)

  // Sent again on a new connection, as after a drop, it waits for the first.
  const again = (await ownLine(served.server.url, served.token)).send({ id: "waiting", command: wait })
  finish("done")

  expect(await waiting).toMatchObject({ result: { text: "done" } })
  expect(await again).toMatchObject({ result: { text: "done" } })
  expect(runs).toBe(1)
  // The oldest that had settled was pushed out in its place.
  expect(await countAs(own, session, "command 2")).toEqual({ text: "run 2" })
  expect(await countAs(own, session, "command 1")).toEqual({ text: "run 201" })
})

test("a device whose token is taken back has the replies to its commands forgotten, so a new token of the same name starts afresh", async () => {
  const count = counted()
  const served = await serveEvie([scriptedProvider([]).plugin, count.plugin], { recheckEvery: 5 })
  cleanups.push(served.stop)
  const own = await ownLine(served.server.url, served.token)
  expect(await countAs(own, served.run.session, "command 1")).toEqual({ text: "run 1" })

  served.tokens.remove("test device")
  await own.closed
  const made = served.tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)

  expect(await countAs(await ownLine(served.server.url, made.secret), served.run.session, "command 1")).toEqual({ text: "run 2" })
})

test("a command its client says it sends once, as a catch-up after a drop is, has its reply forgotten, and pushes out no other", async () => {
  const count = counted()
  const served = await serveEvie([scriptedProvider([]).plugin, count.plugin])
  cleanups.push(served.stop)
  const own = await ownLine(served.server.url, served.token)
  for (let n = 1; n <= 200; n++) await countAs(own, served.run.session, `command ${n}`)

  // Sent twice with one id, by a client that does not keep its promise, it is carried out twice.
  expect(await countAs(own, served.run.session, "catching up", true)).toEqual({ text: "run 201" })
  expect(await countAs(own, served.run.session, "catching up", true)).toEqual({ text: "run 202" })

  expect(await countAs(own, served.run.session, "command 1")).toEqual({ text: "run 1" })
  expect(count.runs()).toBe(202)
})

test("a session opened by its id twice at once on one connection has its events held back until the last reply is sent", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  // Stands for the core: each open of the session reads where it stands,
  // and the first finishes once the second has read too, which is then slow
  // to finish, as one that opens a session again can be.
  const listeners = new Set<(event: EvieEvent) => void>()
  let opens = 0
  const bothRead = gate()
  const secondFinish = gate()
  const core: Evie = {
    async send(command) {
      if (command.type !== "open_session" || command.session === undefined) return served.run.evie.send(command)
      const open = ++opens
      const reply = await served.run.evie.send(command)
      if (open === 1) await bothRead.opened
      if (open === 2) {
        bothRead.open()
        await secondFinish.opened
      }
      return reply
    },
    subscribe(listener) {
      listeners.add(listener)
      const stop = served.run.evie.subscribe(listener)
      return () => {
        listeners.delete(listener)
        stop()
      }
    },
    close: () => served.run.evie.close(),
  }
  const server = startServer({ evie: core, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const own = await ownLine(server.url, served.token)
  const open = { type: "open_session", session: served.run.session, after: 0 } as const
  const first = own.send({ id: "first", command: open })
  const second = own.send({ id: "second", command: open })
  await first

  // The session goes on once both have read where it stands, while the second is still finishing.
  for (const listener of listeners) listener({ type: "assistant_delta", session: served.run.session, text: "since" })
  secondFinish.open()
  await second
  while (!own.heard.some((message) => "event" in message)) await Bun.sleep(1)

  expect(own.heard.map((message) => ("reply" in message ? message.reply : "event" in message ? message.event.type : "other"))).toEqual([
    "first",
    "second",
    "assistant_delta",
  ])
})

test("a session opened by its id is sent to the client from the moment the core is asked, so no event falls in between, and those events follow the reply", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  // Stands for the core sending an event of the session in the very step it
  // is asked to open it.
  const listeners = new Set<(event: EvieEvent) => void>()
  const core: Evie = {
    send(command) {
      if (command.type === "open_session" && command.session !== undefined) {
        for (const listener of listeners) listener({ type: "assistant_delta", session: command.session, text: "just then" })
      }
      return served.run.evie.send(command)
    },
    subscribe(listener) {
      listeners.add(listener)
      const stop = served.run.evie.subscribe(listener)
      return () => {
        listeners.delete(listener)
        stop()
      }
    },
    close: () => served.run.evie.close(),
  }
  const server = startServer({ evie: core, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const client = await connect({ url: server.url, token: served.token })
  cleanups.push(() => client.close())
  const heard: string[] = []
  client.subscribe((event) => event.type === "assistant_delta" && heard.push(event.text))
  const delta = nextEvent({ evie: client }, "assistant_delta")

  await client.send({ type: "open_session", session: served.run.session }).then(() => heard.push("the reply"))
  await delta

  expect(heard).toEqual(["the reply", "just then"])
})

test("an event stored after a catch-up has read the session, and a question after it, are each passed on once, though the question reaches the client first", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  // Stands for the core storing an event of the session and then asking a
  // question in it, after it has read where the session stands for the
  // client's open of it by its id and before the reply is sent. The server
  // holds the event back until the reply is sent, and sends the question at once.
  const listeners = new Set<(event: EvieEvent) => void>()
  let caughtUp = false
  const core: Evie = {
    async send(command) {
      const reply = await served.run.evie.send(command)
      if (command.type === "open_session" && command.session !== undefined && !caughtUp) {
        caughtUp = true
        const { lastSeq } = reply as { lastSeq: number }
        const session = command.session
        const stored: EvieEvent = { type: "user_message", session, seq: lastSeq + 1, time: Date.now(), body: { text: "stored in between" } }
        const question: EvieEvent = { type: "question", session, seq: lastSeq + 2, time: Date.now(), body: { id: "q1", text: "Go on?", choices: ["Yes", "No"] } }
        for (const listener of listeners) listener(stored)
        for (const listener of listeners) listener(question)
      }
      return reply
    },
    subscribe(listener) {
      listeners.add(listener)
      const stop = served.run.evie.subscribe(listener)
      return () => {
        listeners.delete(listener)
        stop()
      }
    },
    close: () => served.run.evie.close(),
  }
  const server = startServer({ evie: core, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const client = await connect({ url: server.url, token: served.token })
  cleanups.push(() => client.close())
  const heard: EvieEvent[] = []
  client.subscribe((event) => heard.push(event))

  // Opened by its folder, the session is caught up on at once.
  const { session, lastSeq } = await client.send({ type: "open_session", folder: served.run.projectFolder })
  // What the server held back is sent before the reply to this.
  await client.send({ type: "cancel", session })

  expect(heard.flatMap((event) => (event.session === session && "seq" in event ? [[event.type, event.seq]] : []))).toEqual([
    ["question", lastSeq + 2],
    ["user_message", lastSeq + 1],
  ])
})

test("a session that fails to open by its id has the events held back meanwhile dropped, since the connection does not have it open, but not its summary, which every client is sent", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  // Stands for the core sending an event of the session, and its summary,
  // as it is asked to open it, and then failing, as when its project's
  // plugins fail to load.
  const listeners = new Set<(event: EvieEvent) => void>()
  const core: Evie = {
    send(command) {
      if (command.type !== "open_session" || command.session === undefined) return served.run.evie.send(command)
      for (const listener of listeners) listener({ type: "assistant_delta", session: command.session, text: "not for this client" })
      for (const listener of listeners) listener({ type: "summary", session: command.session, project: null, name: "hi", state: "active", lastActive: 1 })
      return Promise.reject(new Error("The project's plugins could not be loaded."))
    },
    subscribe(listener) {
      listeners.add(listener)
      const stop = served.run.evie.subscribe(listener)
      return () => {
        listeners.delete(listener)
        stop()
      }
    },
    close: () => served.run.evie.close(),
  }
  const server = startServer({ evie: core, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const own = await ownLine(server.url, served.token)

  expect(await own.send({ id: "open", command: { type: "open_session", session: served.run.session, after: 0 } })).toEqual({
    reply: "open",
    error: "The project's plugins could not be loaded.",
  })
  // Anything sent after the reply to the open comes before the reply to this.
  await own.send({ id: "after", command: { type: "cancel", session: served.run.session } })

  expect(own.heard.map((message) => ("reply" in message ? message.reply : "event" in message ? message.event.type : "other"))).toEqual(["open", "summary", "after"])
})

test("a session opened by its id has its summaries held back with its events, and sent after them, so none comes before the event it tells of", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  // Stands for the core storing an event of the session, and sending the
  // summary that event changed, after it has read where the session stands
  // for the open and before the reply is sent.
  const listeners = new Set<(event: EvieEvent) => void>()
  const core: Evie = {
    async send(command) {
      const reply = await served.run.evie.send(command)
      if (command.type === "open_session" && command.session !== undefined) {
        const { lastSeq } = reply as { lastSeq: number }
        const session = command.session
        for (const listener of listeners) listener({ type: "turn_start", session, seq: lastSeq + 1, time: 1, body: {} })
        for (const listener of listeners) listener({ type: "summary", session, project: null, name: "hi", state: "working", lastActive: 1 })
      }
      return reply
    },
    subscribe(listener) {
      listeners.add(listener)
      const stop = served.run.evie.subscribe(listener)
      return () => {
        listeners.delete(listener)
        stop()
      }
    },
    close: () => served.run.evie.close(),
  }
  const server = startServer({ evie: core, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const own = await ownLine(server.url, served.token)

  await own.send({ id: "open", command: { type: "open_session", session: served.run.session, after: 0 } })
  // Anything sent after the reply to the open comes before the reply to this.
  await own.send({ id: "after", command: { type: "cancel", session: served.run.session } })

  expect(own.heard.map((message) => ("reply" in message ? message.reply : "event" in message ? message.event.type : "other"))).toEqual(["open", "turn_start", "summary", "after"])
})

test("a background job the server stopped when it was stopped is shown as stopped once the client is back, and a pipe waiting for it leaves", async () => {
  const home = await tempFolder("evie-home-")
  const projectFolder = await tempFolder("evie-project-")
  const tokens = await openTokens(home)
  cleanups.push(async () => tokens.close())
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  const serve = async (script: ScriptedReply[], port = 0) => {
    const run = await startEvie([scriptedProvider(script).plugin, backgroundWork().plugin], { home, projectFolder })
    return { run, server: startServer({ evie: run.evie, tokens, host: "127.0.0.1", port }) }
  }
  let served = await serve(startsWork())
  cleanups.push(async () => {
    await served.server.stop()
    await served.run.evie.close()
  })
  const client = await connect({ url: served.server.url, token: made.secret, firstWait: FIRST_WAIT })
  cleanups.push(() => client.close())

  let printed = ""
  await runTerminal({
    evie: client,
    folder: projectFolder,
    questions: false,
    waitForJobs: true,
    lines: (async function* () {
      yield "build it"
      while (!printed.includes("Started it.")) await Bun.sleep(1)
      // The server is stopped, which stops the job, and started again on the same port and data folder.
      const port = Number(new URL(served.server.url).port)
      await served.server.stop()
      await served.run.evie.close()
      served = await serve([], port)
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toMatch(/\[background\] job 1 stopped after \d+s: the build\n/)
})

test("after the server is started again, a session whose project asks again whether to trust it shows the question while catching up, and carries on once it is answered", async () => {
  const home = await tempFolder("evie-home-")
  const projectFolder = await tempFolder("evie-project-")
  // The project asks each time its plugins are set up, as one with a settings
  // file of its own does until the owner decides; the question is asked again
  // when the server is started again.
  const projectSettings: NonNullable<EvieOptions["projectSettings"]> = async (_project, { ask }) => {
    const answer = await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
    return { plugins: [], ...(answer !== "Yes" && { forNow: true as const }) }
  }
  const tokens = await openTokens(home)
  cleanups.push(async () => tokens.close())
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  const serve = async (reply: string, port = 0) => {
    const run = await startEvie([scriptedProvider([{ text: [reply] }]).plugin], { home, projectFolder, projectSettings })
    const server = startServer({ evie: run.evie, tokens, host: "127.0.0.1", port })
    return { run, server }
  }
  let served = await serve("Hi.")
  cleanups.push(async () => {
    await served.server.stop()
    await served.run.evie.close()
  })
  const line = await lineTo(served.server.url)
  cleanups.push(line.stop)
  const client = await connect({ url: line.url, token: made.secret, firstWait: FIRST_WAIT })
  cleanups.push(() => client.close())

  let printed = ""
  const asked = () => printed.split("Type a number.").length - 1
  await runTerminal({
    evie: client,
    folder: projectFolder,
    lines: (async function* () {
      while (asked() < 1) await Bun.sleep(1)
      yield "1"
      yield "hello"
      while (!printed.includes("Hi.")) await Bun.sleep(1)
      // The server is stopped and started again on the same port and data folder.
      const port = Number(new URL(served.server.url).port)
      await served.server.stop()
      await served.run.evie.close()
      served = await serve("Hi again.", port)
      while (asked() < 2) await Bun.sleep(1)
      yield "1"
      yield "again"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed.split("[question] Trust this project?")).toHaveLength(3)
  expect(printed.split("[answered] Yes")).toHaveLength(3)
  expect(printed).toContain("Hi again.")
})

test("a catch-up that asks a question, has it answered, and then fails says that some events may be missing, and skips nothing: the next one shows what was missed before the question, and each question once", async () => {
  const home = await tempFolder("evie-home-")
  const projectFolder = await tempFolder("evie-project-")
  // The project asks whether to trust it each time its plugins are set up.
  // Once, after the owner has answered, its plugins fail to load.
  let failOnce = false
  const projectSettings: NonNullable<EvieOptions["projectSettings"]> = async (_project, { ask }) => {
    const answer = await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
    if (answer !== null && failOnce) {
      failOnce = false
      throw new Error("The project's plugins could not be loaded.")
    }
    return { plugins: [], ...(answer !== "Yes" && { forNow: true as const }) }
  }
  const tokens = await openTokens(home)
  cleanups.push(async () => tokens.close())
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  const serve = async (script: ScriptedReply[], port = 0) => {
    const run = await startEvie([scriptedProvider(script).plugin], { home, projectFolder, projectSettings })
    return { run, server: startServer({ evie: run.evie, tokens, host: "127.0.0.1", port }) }
  }
  let served = await serve([{ text: ["Hi."] }, { text: ["Said while the line was down."] }])
  cleanups.push(async () => {
    await served.server.stop()
    await served.run.evie.close()
  })
  const line = await lineTo(served.server.url)
  cleanups.push(line.stop)
  const notes: string[] = []
  const client = await connect({ url: line.url, token: made.secret, firstWait: FIRST_WAIT, onMissing: (note) => notes.push(note) })
  cleanups.push(() => client.close())

  let printed = ""
  let session = ""
  const count = (text: string) => printed.split(text).length - 1
  await runTerminal({
    evie: client,
    folder: projectFolder,
    lines: (async function* () {
      while (count("Type a number.") < 1) await Bun.sleep(1)
      yield "1"
      yield "hello"
      while (!printed.includes("Hi.")) await Bun.sleep(1)
      session = stored(served.run.events).find((event) => event.type === "user_message")!.session
      // While the line is down, another client's message is answered in the
      // session, and then the server is started again.
      await line.cut()
      await say(served.run, "are you there?", session)
      const port = Number(new URL(served.server.url).port)
      await served.server.stop()
      await served.run.evie.close()
      failOnce = true
      served = await serve([], port)
      // Catching up opens the session again, which asks, and fails once it is answered.
      line.mend()
      while (count("Type a number.") < 2) await Bun.sleep(1)
      yield "1"
      while (count("[answered] Yes") < 2) await Bun.sleep(1)
      // Sent once the catch-up is over, so it is answered after it, by a session that is not open.
      yield "/nothing"
      while (!printed.includes(`[error] The session "${session}" is not open.`)) await Bun.sleep(1)
      // The line drops again, and the next catch-up opens the session.
      await line.cut()
      line.mend()
      while (count("Type a number.") < 3) await Bun.sleep(1)
      yield "1"
      while (count("[answered] Yes") < 3) await Bun.sleep(1)
      // Answered after that catch-up too.
      yield "/nothing"
      while (!printed.includes('[error] There is no command "/nothing"')) await Bun.sleep(1)
    })(),
    write: (text) => (printed += text),
  })

  expect(notes).toEqual([`Catching up on the session ${session} failed, so some of its events may be missing here. The project's plugins could not be loaded.`])
  expect(printed.split("Said while the line was down.")).toHaveLength(2)
  expect(printed.split("[question] Trust this project?")).toHaveLength(4)
  expect(printed.split("[answered] Yes")).toHaveLength(4)
})

// ---- Lists and summaries ------------------------------------------------------------------

test("a client that has opened no session is sent the summary of every session as it changes, and none of its events", async () => {
  const served = await serveEvie([scriptedProvider([{ text: ["Hel", "lo."] }]).plugin])
  cleanups.push(served.stop)
  const watcher = await served.connect()
  const watched: EvieEvent[] = []
  watcher.subscribe((event) => watched.push(event))
  const worker = await served.connect()

  const { session } = await worker.send({ type: "open_session", folder: served.run.projectFolder })
  const ended = nextEvent({ evie: worker }, "turn_end")
  await worker.send({ type: "send_message", session, text: "hi" })
  await ended
  // The summary saying the turn has ended comes to the watcher on a line of its own.
  const told = (): boolean => watched.some((event) => event.type === "summary" && event.session === session && event.state === "active" && event.name === "hi")
  for (let waited = 0; !told() && waited < 2000; waited += 5) await Bun.sleep(5)

  expect(watched.filter((event) => event.type !== "summary")).toEqual([])
  const states = watched.flatMap((event) => (event.type === "summary" && event.session === session ? [event.state] : []))
  expect(states.filter((state, at) => state !== states[at - 1])).toEqual(["working", "active"])
})

test("over the network, a client lists projects and sessions, settles a session, and reads its history a page at a time", async () => {
  const served = await serveEvie([scriptedProvider([{ text: ["Paris."] }]).plugin])
  cleanups.push(served.stop)
  await say(served.run, "What is the capital of France?")
  const client = await served.connect()
  const { session, project } = served.run

  const projects = await client.send({ type: "list_projects" })
  await client.send({ type: "settle_session", session, settled: true })
  const sessions = await client.send({ type: "list_sessions" })
  const page = await client.send({ type: "read_history", session, limit: 2 })

  const events = stored(served.run.events)
  expect(projects).toEqual([{ ...project!, git: false }])
  expect(sessions).toEqual([{ session, project: project!.id, name: "What is the capital of France?", state: "settled", lastActive: events.at(-2)!.time }])
  expect(page).toEqual({ events: events.slice(-2), earlier: true })
  expect(page.events.map((event) => event.type)).toEqual(["turn_end", "settled"])
})

test("over the network, a command whose field is read as text is refused when that field is not text, and nothing is stored", async () => {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  const own = await ownLine(served.server.url, served.token)
  const { session } = served.run
  // Sent as a client that is not type-checked could send them.
  const replyTo = async (id: string, command: Record<string, unknown>) => {
    const reply = await own.send({ id, command: command as unknown as ClientMessage["command"] })
    return "error" in reply ? reply.error : "carried out"
  }

  expect(await replyTo("message", { type: "send_message", session, text: 42 })).toBe("`text` is the message, and must be text.")
  expect(await replyTo("model", { type: "set_model", session, model: ["some/model"] })).toBe("`model` is the name of a model, and must be text.")
  expect(await replyTo("answer", { type: "answer_question", session, id: "q1", choice: 1 })).toBe("`choice` is the answer, and must be text, or null to refuse the question.")
  expect(await replyTo("command", { type: "run_command", session, name: "compact", args: 3 })).toBe("`name` is the command's name and `args` what was typed after it, and both must be text.")
  expect(await replyTo("command name", { type: "run_command", session, name: null, args: "" })).toBe("`name` is the command's name and `args` what was typed after it, and both must be text.")
  expect(await replyTo("folder", { type: "open_session", folder: 7 })).toBe("`folder` is the path of a folder, and must be text.")
  // A turn the message had started would be under way by now.
  await Bun.sleep(20)
  expect(stored(served.run.events)).toEqual([])
})

// A server that lets the client in, and then, after the line drops, refuses
// its WebSocket the given number of times while still accepting its token,
// as a busy server can when it fails to take a connection.
function refusingAfterADrop() {
  const lines = new Set<import("bun").ServerWebSocket<unknown>>()
  let refusals = 0
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    fetch(request, server) {
      if (new URL(request.url).pathname === "/check") return new Response("This token is accepted.")
      if (refusals > 0) {
        refusals--
        return new Response("Connections from this page are not allowed.", { status: 403 })
      }
      if (server.upgrade(request)) return
      return new Response("Not a WebSocket.", { status: 400 })
    },
    websocket: { open: (line) => void lines.add(line), close: (line) => void lines.delete(line), message: () => {} },
  })
  cleanups.push(async () => server.stop(true))
  return {
    url: `http://127.0.0.1:${server.port}`,
    dropThenRefuse(times: number) {
      refusals = times
      for (const line of lines) line.close(1001, "going away")
    },
  }
}

test("a client whose WebSocket is refused once or twice as it connects again, while its token is still accepted, keeps trying, and is back", async () => {
  const server = refusingAfterADrop()
  let back = 0
  const ended: ConnectError[] = []
  const client = await connect({ url: server.url, token: "a-token", firstWait: 5, onBack: () => back++, onEnd: (cause) => ended.push(cause) })
  cleanups.push(() => client.close())

  server.dropThenRefuse(2)
  while (back === 0 && ended.length === 0) await Bun.sleep(5)

  expect(ended).toEqual([])
  expect(back).toBe(1)
})

test("a client whose WebSocket is refused three times in a row as it connects again, while its token is accepted, stops, saying the page is not let in", async () => {
  const server = refusingAfterADrop()
  const ended: ConnectError[] = []
  const client = await connect({ url: server.url, token: "a-token", firstWait: 5, onEnd: (cause) => ended.push(cause) })
  cleanups.push(() => client.close())

  server.dropThenRefuse(Infinity)
  while (ended.length === 0) await Bun.sleep(5)

  expect(ended.map((cause) => cause.reason)).toEqual(["page"])
})
