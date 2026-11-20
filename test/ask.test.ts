import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { ask, loadSettingsFiles, runTerminal, type Plugin, type Question } from "../src/index.ts"
import { nextEvent, say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const DEPLOY: Question = { text: "Deploy to production?", choices: ["Yes", "No"] }

// A plugin that asks the owner before the `deploy` tool runs, the way a
// permissions plugin would, and records what it was told.
function asksBeforeDeploy(question: Question = DEPLOY) {
  const answers: (string | null)[] = []
  const plugin: Plugin = {
    name: "asks-before-deploy",
    setup(host) {
      host.tools.add({ name: "deploy", description: "Deploys.", inputSchema: { type: "object" }, run: async () => "deployed" })
      host.on("before_tool", async (_call, context) => {
        const answer = await context.ask(question)
        answers.push(answer)
        if (answer !== "Yes") return { deny: "The owner did not allow it." }
      })
    },
  }
  return { plugin, answers }
}

const callsDeploy: ScriptedReply[] = [{ text: [], calls: [{ name: "deploy", input: {} }] }, { text: ["Done."] }]

async function start(script: ScriptedReply[], plugins: Plugin[], options: { questions?: boolean } = { questions: true }) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins], options)
  cleanups.push(run.stop)
  return { run, model }
}

// Sends a message and answers the question the turn asks.
async function sayAndAnswer(run: Run, text: string, choice: string | null) {
  const asked = nextEvent(run, "question")
  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text })
  const question = await asked
  await run.evie.send({ type: "answer_question", session: run.session, id: question.body.id, choice })
  await ended
  return question
}

const toolResults = (run: Run) => stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))

// ---- The mechanism --------------------------------------------------------------------

test("a question goes to the client as an event, and the answer comes back as a command", async () => {
  const deploy = asksBeforeDeploy()
  const { run } = await start(callsDeploy, [deploy.plugin])

  const question = await sayAndAnswer(run, "ship it", "Yes")

  expect(question.body).toEqual({ id: question.body.id, text: "Deploy to production?", choices: ["Yes", "No"] })
  expect(deploy.answers).toEqual(["Yes"])
  expect(toolResults(run)).toEqual(["deployed"])
})

test("the question and its answer are recorded in the event log, in the turn that asked", async () => {
  const { run } = await start(callsDeploy, [asksBeforeDeploy().plugin])

  const question = await sayAndAnswer(run, "ship it", "No")

  expect(stored(run.events).map((event) => event.type)).toEqual([
    "turn_start",
    "user_message",
    "assistant_message",
    "question",
    "answer",
    "tool_result",
    "assistant_message",
    "turn_end",
  ])
  expect(stored(run.events).find((event) => event.type === "answer")?.body).toEqual({ id: question.body.id, choice: "No" })
  expect(toolResults(run)).toEqual(["The owner did not allow it."])
})

test("a client that does not show questions has every question refused at once", async () => {
  const deploy = asksBeforeDeploy()
  const { run } = await start(callsDeploy, [deploy.plugin], {})

  await say(run, "ship it")

  expect(deploy.answers).toEqual([null])
  expect(stored(run.events).find((event) => event.type === "answer")?.body).toMatchObject({ choice: null })
  expect(toolResults(run)).toEqual(["The owner did not allow it."])
})

test("a turn that is waiting on a question can be cancelled, which refuses the question", async () => {
  const deploy = asksBeforeDeploy()
  const { run } = await start(callsDeploy, [deploy.plugin])

  const asked = nextEvent(run, "question")
  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text: "ship it" })
  await asked
  await run.evie.send({ type: "cancel", session: run.session })

  expect((await ended).body.reason).toBe("cancelled")
  expect(deploy.answers).toEqual([null])
})

test("closing Evie refuses a question that is still open", async () => {
  const deploy = asksBeforeDeploy()
  const { run } = await start(callsDeploy, [deploy.plugin])

  const asked = nextEvent(run, "question")
  await run.evie.send({ type: "send_message", session: run.session, text: "ship it" })
  await asked
  await run.evie.close()

  expect(deploy.answers).toEqual([null])
  expect(stored(run.events).find((event) => event.type === "answer")?.body).toMatchObject({ choice: null })
})

test("an answer must be one of the choices, and a question is answered only once", async () => {
  const { run } = await start(callsDeploy, [asksBeforeDeploy().plugin])
  const asked = nextEvent(run, "question")
  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text: "ship it" })
  const { id } = (await asked).body
  const answer = (choice: string) => run.evie.send({ type: "answer_question", session: run.session, id, choice })

  await expect(answer("Maybe")).rejects.toThrow('"Maybe" is not one of the choices: Yes, No.')
  await answer("Yes")
  await ended
  await expect(answer("No")).rejects.toThrow(`There is no open question "${id}" in this session.`)
})

test("a question that allows it can be answered in the owner's own words", async () => {
  const deploy = asksBeforeDeploy({ ...DEPLOY, freeText: true })
  const { run } = await start(callsDeploy, [deploy.plugin])

  await sayAndAnswer(run, "ship it", "Only to staging")

  expect(deploy.answers).toEqual(["Only to staging"])
})

test("a question left open when Evie stopped is closed off when the session is continued", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  // A plugin whose question is never settled, to stand for Evie being killed while it waited.
  const first = await startEvie([scriptedProvider(callsDeploy).plugin, asksBeforeDeploy().plugin], { home, projectFolder, questions: true })
  const asked = nextEvent(first, "question")
  await first.evie.send({ type: "send_message", session: first.session, text: "ship it" })
  const { id } = (await asked).body
  // The log as a crash would leave it: copy the data folder while the question is open.
  const crashed = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(crashed, { recursive: true, force: true }))
  for (const file of ["evie.db", "evie.db-wal", "evie.db-shm"]) {
    await writeFile(join(crashed, file), await readFile(join(home, file)).catch(() => new Uint8Array()))
  }
  await first.stop()

  const model = scriptedProvider([{ text: ["Carrying on."] }])
  const second = await startEvie([model.plugin, asksBeforeDeploy().plugin], { home: crashed, projectFolder })
  cleanups.push(second.stop)
  const reopened = await second.evie.send({ type: "open_session", folder: projectFolder, continue: true })

  expect(reopened.resumed).toBe(true)
  expect(stored(second.events).find((event) => event.type === "answer")?.body).toEqual({ id, choice: null })
})

// ---- The model's own questions -----------------------------------------------------------

const modelAsks: ScriptedReply[] = [
  { text: [], calls: [{ name: "ask", input: { question: "Which database?", choices: ["SQLite", "Postgres"] } }] },
  { text: ["Going with that."] },
]

test("the model can ask the owner a question with choices, and is told the answer", async () => {
  const { run, model } = await start(modelAsks, [ask()])

  const question = await sayAndAnswer(run, "set up storage", "Postgres")

  expect(model.requests[0]?.tools.map((tool) => tool.name)).toEqual(["ask"])
  expect(question.body).toMatchObject({ text: "Which database?", choices: ["SQLite", "Postgres"], freeText: true })
  expect(toolResults(run)).toEqual(["The owner chose: Postgres"])
})

test("the model is told when the owner answered in their own words, or did not answer", async () => {
  const inWords = await start(modelAsks, [ask()])
  await sayAndAnswer(inWords.run, "set up storage", "Neither, use plain files")
  expect(toolResults(inWords.run)).toEqual(["The owner answered in their own words: Neither, use plain files"])

  const unanswered = await start(modelAsks, [ask()], {})
  await say(unanswered.run, "set up storage")
  expect(toolResults(unanswered.run)[0]).toStartWith("The owner did not answer")
})

test("a question from the model with no choices is sent back to be asked properly", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "ask", input: { question: "Which database?", choices: ["SQLite"] } }] }, { text: ["ok"] }], [ask()])

  await say(run, "set up storage")

  expect(toolResults(run)[0]).toStartWith("Give between two and 6 choices.")
  expect(stored(run.events).some((event) => event.type === "question")).toBe(false)
})

// ---- Trusting a project ---------------------------------------------------------------------

const toolPlugin = `export default { name: "deploy", setup(host) { host.tools.add({ name: "deploy", description: "Deploys.", inputSchema: { type: "object" }, run: async () => "deployed" }) } }`

// Evie started the way the evie command does, with a project that has a
// settings file of its own and is on neither list.
// Pass `again` to stand for Evie being started once more on the same data and project.
async function startWithProject(again?: { home: string; folder: string }) {
  const home = again?.home ?? (await realpath(await mkdtemp(join(tmpdir(), "evie-home-"))))
  const folder = again?.folder ?? (await realpath(await mkdtemp(join(tmpdir(), "evie-project-"))))
  if (!again) {
    cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(folder, { recursive: true, force: true }))
    await mkdir(join(folder, ".evie"))
    await writeFile(join(folder, ".evie", "evie.json"), JSON.stringify({ plugins: ["./deploy.ts"] }))
    await writeFile(join(folder, "deploy.ts"), toolPlugin)
    await writeFile(join(home, "evie.json"), JSON.stringify({ model: "some/model", plugins: [] }))
  }
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: home } })
  const model = scriptedProvider(Array.from({ length: 4 }, () => ({ text: ["ok"] })))
  const run = await startEvie([model.plugin, ...settings.plugins], { home, projectSettings: settings.projectSettings })
  cleanups.push(run.stop)
  const globalFile = async () => JSON.parse(await readFile(join(home, "evie.json"), "utf8"))
  // Opens a session in the project as a client that shows questions, answering the one it is asked.
  async function open(choice: string | null) {
    const asked = nextEvent(run, "question")
    const opening = run.evie.send({ type: "open_session", folder, questions: true })
    const question = await asked
    await run.evie.send({ type: "answer_question", session: question.session, id: question.body.id, choice })
    return { question, opened: await opening }
  }
  async function offered(session: string) {
    await say(run, "hi", session)
    return model.requests.at(-1)!.tools.map((tool) => tool.name)
  }
  return { run, home, folder, globalFile, open, offered }
}

test("opening a project that has its own settings file asks whether to trust it", async () => {
  const { folder, globalFile, open, offered } = await startWithProject()

  const { question, opened } = await open("Yes, trust this project")

  expect(question.body.text).toContain(`This project has a settings file of its own, ${join(folder, ".evie", "evie.json")}.`)
  expect(question.body.choices).toEqual(["Yes, trust this project", "No, ignore its settings file"])
  // The question is in the log of the session that was opening.
  expect(question.session).toBe(opened.session)
  expect(opened.notices).toEqual([])
  expect(await offered(opened.session)).toEqual(["deploy"])
  // The answer is remembered, and the rest of the file is kept.
  expect(await globalFile()).toEqual({ model: "some/model", plugins: [], trust: [folder] })
})

test("a no is remembered too: the settings file is ignored and the project is not asked about again", async () => {
  const first = await startWithProject()
  const { opened } = await first.open("No, ignore its settings file")

  expect(await first.offered(opened.session)).toEqual([])
  expect(opened.notices[0]).toContain(`To use it, move "${first.folder}" from "untrusted" to "trust"`)
  expect(await first.globalFile()).toEqual({ model: "some/model", plugins: [], untrusted: [first.folder] })

  // Started again with that answer on file, nothing is asked.
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: first.home } })
  const again = await startEvie([scriptedProvider([]).plugin], { projectSettings: settings.projectSettings })
  cleanups.push(again.stop)
  const reopened = await again.evie.send({ type: "open_session", folder: first.folder, questions: true })
  expect(stored(again.events).some((event) => event.type === "question")).toBe(false)
  expect(reopened.notices).toHaveLength(1)
})

test("a trust question that is not answered is a no for now, and nothing is remembered", async () => {
  const dismissed = await startWithProject()
  const { opened } = await dismissed.open(null)
  expect(await dismissed.offered(opened.session)).toEqual([])
  expect(opened.notices[0]).toContain(`To use it, add "${dismissed.folder}" to "trust"`)
  expect(await dismissed.globalFile()).toEqual({ model: "some/model", plugins: [] })

  // A client that does not show questions is not kept waiting.
  const silent = await startWithProject()
  const quietly = await silent.run.evie.send({ type: "open_session", folder: silent.folder })
  expect(quietly.notices).toHaveLength(1)
  expect(await silent.globalFile()).toEqual({ model: "some/model", plugins: [] })
})

// ---- The terminal ---------------------------------------------------------------------------

test("the terminal shows a question with numbered choices and takes the number typed as the answer", async () => {
  const deploy = asksBeforeDeploy()
  const { run } = await start(callsDeploy, [deploy.plugin], {})
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "ship it"
      while (!printed.includes("Type a number.")) await Bun.sleep(1)
      // Not a choice: the question is shown again.
      yield "5"
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("[question] Deploy to production?\n  1. Yes\n  2. No\nType a number.\n")
  expect(printed.split("[question]")).toHaveLength(3)
  expect(printed).toContain("[answered] Yes\n")
  expect(deploy.answers).toEqual(["Yes"])
})

test("the terminal takes an answer in the owner's own words when the question allows it", async () => {
  const { run } = await start(modelAsks, [ask()], {})
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "set up storage"
      while (!printed.includes("Type a number, or your own answer.")) await Bun.sleep(1)
      yield "plain files"
    })(),
    write: (text) => (printed += text),
  })

  expect(toolResults(run)).toEqual(["The owner answered in their own words: plain files"])
})

test("when the input ends with a question still open, the terminal refuses it and the turn finishes", async () => {
  const deploy = asksBeforeDeploy()
  const { run } = await start(callsDeploy, [deploy.plugin], {})
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "ship it"
    })(),
    write: (text) => (printed += text),
  })

  expect(deploy.answers).toEqual([null])
  expect(printed).toContain("[not answered]")
})

test("the terminal asks whether to trust a project while it is opening, before the first prompt", async () => {
  const { run, folder, globalFile } = await startWithProject()
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder,
    lines: (async function* () {
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("  1. Yes, trust this project\n  2. No, ignore its settings file\nType a number.\n")
  expect(printed.indexOf("[answered] Yes, trust this project")).toBeLessThan(printed.indexOf("evie · "))
  expect((await globalFile()).trust).toEqual([folder])
})

// ---- Found in review ------------------------------------------------------------------------

test("a session that holds nothing but a question is not the one --continue carries on", async () => {
  const first = await startWithProject()
  const { opened } = await first.open(null)
  await say(first.run, "remember this", opened.session)
  await first.run.evie.close()

  // Started again: opening asks once more, in a new session that never gets a message.
  const second = await startWithProject(first)
  await second.run.evie.send({ type: "open_session", folder: first.folder })
  const continued = await second.run.evie.send({ type: "open_session", folder: first.folder, continue: true })

  expect(continued).toMatchObject({ session: opened.session, resumed: true })
})

test("two clients continuing the same session at once share one session, and its questions are still asked", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  const projectFolder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }), () => rm(projectFolder, { recursive: true, force: true }))
  const first = await startEvie([scriptedProvider([{ text: ["Hello."] }]).plugin], { home, projectFolder })
  await say(first, "hi")
  await first.stop()

  const deploy = asksBeforeDeploy()
  const again = await startEvie([scriptedProvider(callsDeploy).plugin, deploy.plugin], { home, projectFolder: await realpath(await mkdtemp(join(tmpdir(), "evie-other-"))) })
  cleanups.push(again.stop)
  const [shows, silent] = await Promise.all([
    again.evie.send({ type: "open_session", folder: projectFolder, continue: true, questions: true }),
    again.evie.send({ type: "open_session", folder: projectFolder, continue: true }),
  ])
  expect(silent.session).toBe(shows.session)

  await sayAndAnswer({ ...again, session: shows.session }, "ship it", "Yes")
  expect(deploy.answers).toEqual(["Yes"])
})

test("a command that stops to ask a question can be answered in the terminal", async () => {
  // A plugin that asks before a request when told to. /compact makes a request of its own.
  const answers: (string | null)[] = []
  let askNow = false
  const asksFirst: Plugin = {
    name: "asks-first",
    setup(host) {
      host.on("before_request", async (_request, context) => {
        if (askNow) answers.push(await context.ask(DEPLOY))
      })
    },
  }
  const model = scriptedProvider([{ text: ["A1"] }, { text: ["A2"] }, { text: ["## Goal\nCarry on."] }])
  // Compacted at 2,000 tokens, so the two messages below are left alone until /compact.
  const run = await startEvie([model.plugin, asksFirst], { compaction: { ceiling: 2000 } })
  cleanups.push(run.stop)
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    lines: (async function* () {
      yield "x".repeat(4000)
      while (!printed.includes("A1")) await Bun.sleep(1)
      yield "y".repeat(1600)
      while (!printed.includes("A2")) await Bun.sleep(1)
      askNow = true
      yield "/compact"
      while (!printed.includes("Type a number.")) await Bun.sleep(1)
      yield "2"
    })(),
    write: (text) => (printed += text),
  })

  expect(answers).toEqual(["No"])
  expect(printed).toContain("Compacted: the earlier part of the conversation is now a summary.")
})

test("where nobody is typing, a question is refused and the line is sent as the message it was", async () => {
  const { run, folder } = await startWithProject()
  let printed = ""
  await runTerminal({
    evie: run.evie,
    folder,
    questions: false,
    lines: (async function* () {
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("which was ignored because the project is not trusted")
  expect(stored(run.events).find((event) => event.type === "user_message")?.body).toEqual({ text: "1" })
})

test("a line the owner was already typing when a question appeared is sent as a message, not taken as the answer", async () => {
  const deploy = asksBeforeDeploy({ ...DEPLOY, freeText: true })
  const { run, model } = await start(callsDeploy, [deploy.plugin], {})
  let printed = ""
  let typing = ""
  await runTerminal({
    evie: run.evie,
    folder: run.projectFolder,
    typing: () => typing,
    lines: (async function* () {
      yield "ship it"
      // Part way through a line as the question appears.
      typing = "also update the"
      while (!printed.includes("Type a number, or your own answer.")) await Bun.sleep(1)
      typing = ""
      yield "also update the docs"
      yield "1"
    })(),
    write: (text) => (printed += text),
  })

  expect(deploy.answers).toEqual(["Yes"])
  expect(printed).toContain("so it was sent as a message, not taken as the answer.")
  expect(model.requests.at(-1)?.messages.filter((message) => message.role === "user").at(-1)).toEqual({ role: "user", text: "also update the docs" })
})
