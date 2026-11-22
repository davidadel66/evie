import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readdir, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { ask, models, show, startServer, type Attachments, type Command, type Evie, type EvieOptions, type Plugin } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import { startEvie } from "./support/harness.ts"
import { openConnection } from "../web/src/connection.ts"
import { holdConversation, openConversation, openDraft, takeStarted, type Conversation, type ConversationState, type Draft, type DraftState, type Outgoing } from "../web/src/conversation.ts"
import { openSessions } from "../web/src/sessions.ts"
import { transcriptOf } from "../web/src/transcript.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// Talking in a conversation from the web app, apart from its screen:
// sending, steering and stopping a turn, answering questions, running
// commands and switching the model, against evie's server with the
// scripted model. These tests connect with a token, as the connection's
// own tests do.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(script: ScriptedReply[] = [], { plugins = [], models, ...options }: Parameters<typeof serveEvie>[1] & { plugins?: Plugin[]; models?: { id: string; name: string }[] } = {}) {
  const model = scriptedProvider(script, { ...(models && { models }) })
  const served = await serveEvie([model.plugin, ...plugins], options)
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  return { served, connection, model }
}

function open(connection: Parameters<typeof openConversation>[0], session: string) {
  const conversation = openConversation(connection, session)
  cleanups.push(async () => conversation.close())
  return conversation
}

async function until(conversation: Conversation, ready: (state: ConversationState) => boolean) {
  while (!ready(conversation.state())) await Bun.sleep(5)
  return conversation.state()
}

// What the page draws, in short: each item's text, or its kind.
const said = (state: ConversationState) => transcriptOf(state).map((item) => ("text" in item ? item.text : item.kind))
const types = (state: ConversationState) => state.events.map((event) => event.type)

test("a message sent from the box shows at once as the owner's, until the session has it, and evie's reply follows", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  const sending = conversation.send("Plan the beds")
  expect(said(conversation.state())).toEqual(["Plan the beds"])
  expect(conversation.state().pending).toEqual([{ text: "Plan the beds" }])
  expect(await sending).toBeUndefined()

  const replied = await until(conversation, (state) => !state.turnRunning && state.events.some((event) => event.type === "turn_end"))
  expect(said(replied)).toEqual(["Plan the beds", "Noted."])
  expect(replied.pending).toEqual([])
})

test("a message sent while evie is working steers the turn: the reply being written stops, and the turn carries on with the message", async () => {
  const { served, connection, model } = await start([{ hang: true, text: ["Thinking about the beds"] }, { text: ["North first, then."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  await conversation.send("Plan the beds")
  await until(conversation, (state) => state.replySoFar === "Thinking about the beds")
  await conversation.send("Start with the north bed")
  const steered = await until(conversation, (state) => state.events.filter((event) => event.type === "turn_end").length === 1)

  // One turn, with both messages in it.
  expect(types(steered)).toEqual(["turn_start", "user_message", "assistant_message", "user_message", "assistant_message", "turn_end"])
  expect(said(steered)).toEqual(["Plan the beds", "Thinking about the beds", "The reply was cut short.", "Start with the north bed", "North first, then."])
  expect(model.requests).toHaveLength(2)
})

test("a turn can be stopped: it says it is stopping until the turn has ended, and the reply so far is kept, cut short", async () => {
  const { served, connection } = await start([{ hang: true, text: ["Thinking about the beds"] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  await until(conversation, (state) => state.replySoFar === "Thinking about the beds")
  expect(conversation.state().stopping).toBe(false)

  conversation.stop()
  expect(conversation.state().stopping).toBe(true)
  const stopped = await until(conversation, (state) => !state.turnRunning)

  expect(stopped.stopping).toBe(false)
  expect(said(stopped)).toEqual(["Plan the beds", "Thinking about the beds", "Stopped. The reply was cut short."])
})

// A turn in which evie asks the owner which bed to start with.
const asking = (): ScriptedReply[] => [{ text: [], calls: [{ name: "ask", input: { question: "Which bed first?", choices: ["North bed", "South bed"] } }] }, { text: ["Starting there."] }]

test("a question evie asks waits for the owner's answer, which answers it in place: the choices go, and the answer is the owner's message", async () => {
  const { served, connection } = await start(asking(), { plugins: [ask()] })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  await conversation.send("Plan the beds")
  const waiting = await until(conversation, (state) => state.waiting.length === 1)
  expect(waiting.waiting).toEqual([{ id: expect.any(String), text: "Which bed first?", choices: ["North bed", "South bed"], freeText: true }])

  expect(await conversation.answer(waiting.waiting[0]!.id, "South bed")).toBeUndefined()
  const answered = await until(conversation, (state) => !state.turnRunning && state.events.some((event) => event.type === "turn_end"))
  expect(answered.waiting).toEqual([])
  expect(said(answered).slice(-3)).toEqual(["Which bed first?", "South bed", "Starting there."])
})

test("a question answered from another client, or refused, is no longer waiting, and one already waiting when the session is opened is", async () => {
  const { served, connection } = await start([...asking(), ...asking()], { plugins: [ask()] })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  const { waiting } = await until(conversation, (state) => state.waiting.length === 1)

  await served.run.evie.send({ type: "answer_question", session: served.run.session, id: waiting[0]!.id, choice: "North bed" })
  await until(conversation, (state) => state.waiting.length === 0 && !state.turnRunning)

  await conversation.send("And the next one?")
  await until(conversation, (state) => state.waiting.length === 1)
  // Opened again, as by a reload, the question is waiting there too.
  const again = open(connection, served.run.session)
  const reopened = await until(again, (state) => state.opened)
  expect(reopened.waiting.map((question) => question.text)).toEqual(["Which bed first?"])

  expect(await again.answer(reopened.waiting[0]!.id, null)).toBeUndefined()
  const refused = await until(conversation, (state) => state.waiting.length === 0 && !state.turnRunning)
  expect(said(refused).slice(-3)).toEqual(["Which bed first?", "Not answered.", "Starting there."])
  expect(again.state().waiting).toEqual([])
})

test("an answer the server refuses, as to a question answered already, says why", async () => {
  const { served, connection } = await start()
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  expect(await conversation.answer("gone", "Yes")).toBe('There is no open question "gone" in this session. It may already have been answered.')
})

test("a command run from the box shows its text as a quiet note where it was run, which is not kept, as in the terminal, and one that fails says why", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }, { text: ["Then the south one."] }], { plugins: [models()] })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))

  const running = conversation.run("model", "")
  expect(conversation.state().running).toEqual(["model"])
  await running
  await conversation.run("nothing", "at all")
  const ran = conversation.state()
  expect(ran.running).toEqual([])
  expect(said(ran)).toEqual(["Plan the beds", "Noted.", "The model is test/model (profile: default).", 'There is no command "/nothing". The commands are: /model, /compact.'])
  expect(ran.events.map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "turn_end"])

  // What comes after the note follows it.
  await conversation.send("And then?")
  await until(conversation, (state) => state.events.filter((event) => event.type === "turn_end").length === 2)
  expect(said(conversation.state()).slice(2)).toEqual(["The model is test/model (profile: default).", 'There is no command "/nothing". The commands are: /model, /compact.', "And then?", "Then the south one."])
})

test("the conversation knows the session's model and how full it is, kept current as replies come and the model is switched, and lists the commands and models to pick from", async () => {
  const { served, connection } = await start([{ text: ["Noted."], usage: { inputTokens: 1200, outputTokens: 34 } }], { plugins: [models()], models: [{ id: "maker/model", name: "Maker: Model" }] })
  const conversation = open(connection, served.run.session)
  const opened = await until(conversation, (state) => state.context !== undefined)
  expect([opened.model, opened.context]).toEqual(["test/model", { size: 0, compactsAt: 200_000 }])

  await conversation.send("Plan the beds")
  await until(conversation, (state) => state.context?.size === 1234)

  expect(await conversation.setModel("maker/model")).toBeUndefined()
  await until(conversation, (state) => state.model === "maker/model")

  expect(await conversation.listCommands()).toEqual([
    { name: "model", description: "Show the model in use, or switch to the one named." },
    { name: "compact", description: "Replace the older part of the conversation with a summary, to save room." },
  ])
  expect(await conversation.listModels()).toEqual([{ id: "maker/model", name: "Maker: Model" }])
})

test("compacting now from the conversation runs /compact, and says what happened", async () => {
  const { served, connection } = await start()
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  await conversation.run("compact", "")
  expect(said(conversation.state())).toEqual(["There is not enough conversation to compact yet."])
})

// A draft of a new session, on the list's connection: a question asked as
// it opens belongs to it when its session is not in the list.
async function draft(connection: Parameters<typeof openSessions>[0], project: string | null) {
  while (connection.state().status !== "connected") await Bun.sleep(5)
  const sessions = openSessions(connection)
  cleanups.push(async () => sessions.close())
  const opened = openDraft(connection, project, { listed: (session) => sessions.state().sessions.some((each) => each.session === session) })
  cleanups.push(async () => opened.close())
  return { draft: opened, sessions }
}
async function untilDraft(draft: Draft, ready: (state: DraftState) => boolean) {
  while (!ready(draft.state())) await Bun.sleep(5)
  return draft.state()
}

test("a draft becomes a session in its project on its first message: the page moves to it, then the message is sent, and its conversation is handed over", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  const { draft: started, sessions } = await draft(connection, served.run.project!.id)

  const moves: string[] = []
  const starting = started.start("Plan the beds", (session) => moves.push(`moved to ${session}, ${sessions.state().sessions.some((each) => each.session === session) ? "listed" : "not listed yet"}`))
  expect(started.state().starting).toBe("Plan the beds")
  const result = await starting
  if (!("session" in result)) throw new Error(result.problem)

  expect(moves).toEqual([`moved to ${result.session}, not listed yet`])
  const conversation = takeStarted(result.session)!
  cleanups.push(async () => conversation.close())
  expect(takeStarted(result.session)).toBeUndefined()
  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  expect(said(replied)).toEqual(["Plan the beds", "Noted."])
  const listed = sessions.state().sessions.find((each) => each.session === result.session)
  expect(listed).toMatchObject({ project: served.run.project!.id, name: "Plan the beds" })
})

test("a draft with no project starts a session with none", async () => {
  const { connection } = await start([{ text: ["Noted."] }])
  const { draft: started, sessions } = await draft(connection, null)

  const result = await started.start("What is the weather tomorrow?", () => {})
  if (!("session" in result)) throw new Error(result.problem)
  takeStarted(result.session)?.close()
  const listed = await (async () => {
    while (!sessions.state().sessions.some((each) => each.session === result.session)) await Bun.sleep(5)
    return sessions.state().sessions.find((each) => each.session === result.session)
  })()
  expect(listed).toMatchObject({ project: null, name: "What is the weather tomorrow?" })
})

// A project that asks whether to trust it each time its plugins are set up.
const asksTrust: NonNullable<EvieOptions["projectSettings"]> = async (_project, { ask }) => {
  const answer = await ask({ text: "Trust this project?", choices: ["Yes", "No"], kind: "permission" })
  return { plugins: [], ...(answer !== "Yes" && { forNow: true as const }) }
}

test("a question asked as a draft's session opens, such as whether to trust its project, waits in the draft and is answered there", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }], { projectSettings: asksTrust })
  const { draft: started } = await draft(connection, served.run.project!.id)

  const starting = started.start("Plan the beds", () => {})
  const asked = await untilDraft(started, (state) => state.waiting.length === 1)
  expect(asked.waiting[0]).toMatchObject({ text: "Trust this project?", choices: ["Yes", "No"], kind: "permission" })
  expect(await started.answer(asked.waiting[0]!.id, "Yes")).toBeUndefined()

  const result = await starting
  if (!("session" in result)) throw new Error(result.problem)
  expect(started.state().waiting).toEqual([])
  const conversation = takeStarted(result.session)!
  cleanups.push(async () => conversation.close())
  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  expect(said(replied)).toEqual(["Trust this project?", "Yes", "Plan the beds", "Noted."])
})

test("a draft whose session cannot be started says why, and gives the message back", async () => {
  const { connection } = await start()
  const { draft: started } = await draft(connection, "no-such-project")

  expect(await started.start("Plan the beds", () => {})).toEqual({ problem: 'The session could not be started: There is no project with the id "no-such-project".' })
  expect(started.state().starting).toBeUndefined()
})

test("a draft left while its session starts still sends the first message, so nothing is lost, and does not move the page", async () => {
  const { connection } = await start([{ text: ["Noted."] }])
  const { draft: started, sessions } = await draft(connection, null)

  const moves: string[] = []
  const starting = started.start("Plan the beds", (session) => moves.push(session))
  started.close()
  const result = await starting
  if (!("session" in result)) throw new Error(result.problem)

  expect(moves).toEqual([])
  expect(takeStarted(result.session)).toBeUndefined()
  while (!sessions.state().sessions.some((each) => each.session === result.session)) await Bun.sleep(5)
  expect(sessions.state().sessions.find((each) => each.session === result.session)?.name).toBe("Plan the beds")
})

test("while two commands run, the first to finish leaves the other running", async () => {
  let finish = () => {}
  const held = new Promise<void>((resolve) => (finish = resolve))
  const slow: Plugin = { name: "slow", setup: (host) => host.commands.add({ name: "slow", description: "Takes its time.", run: async () => (await held, "Slow, done.") }) }
  const { served, connection } = await start([], { plugins: [models(), slow] })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  const slowly = conversation.run("slow", "")
  await conversation.run("model", "")
  expect(conversation.state().running).toEqual(["slow"])
  finish()
  await slowly
  expect(conversation.state().running).toEqual([])
})

test("a draft started again after its session could not start answers the new session's question, not the first one's", async () => {
  // The first time its settings are read, the project asks, then fails; then it asks and loads.
  let tries = 0
  const failsOnce: NonNullable<EvieOptions["projectSettings"]> = async (_project, { ask }) => {
    tries += 1
    const answer = await ask({ text: "Trust this project?", choices: ["Yes", "No"], kind: "permission" })
    if (tries === 2) throw new Error("its settings file could not be read")
    return { plugins: [], ...(answer !== "Yes" && { forNow: true as const }) }
  }
  const { served, connection } = await start([{ text: ["Noted."] }], { projectSettings: failsOnce })
  const { draft: started } = await draft(connection, served.run.project!.id)

  const first = started.start("Plan the beds", () => {})
  const asked = await untilDraft(started, (state) => state.waiting.length === 1)
  await started.answer(asked.waiting[0]!.id, "Yes")
  expect(await first).toMatchObject({ problem: expect.stringContaining("its settings file could not be read") })
  expect(started.state().waiting).toEqual([])

  const again = started.start("Plan the beds", () => {})
  const askedAgain = await untilDraft(started, (state) => state.waiting.length === 1)
  expect(askedAgain.waiting[0]!.id).not.toBe(asked.waiting[0]!.id)
  expect(await started.answer(askedAgain.waiting[0]!.id, "Yes")).toBeUndefined()
  const result = await again
  if (!("session" in result)) throw new Error(result.problem)
  takeStarted(result.session)?.close()
})

test("a draft shows the model a new session would use, lists the models, and a model picked there is the session's before its first message", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }], { models: [{ id: "maker/model", name: "Maker: Model" }] })
  const { draft: started } = await draft(connection, served.run.project!.id)

  await untilDraft(started, (state) => state.model === "test/model")
  expect(await started.listModels()).toEqual([{ id: "maker/model", name: "Maker: Model" }])
  started.pick("maker/model")
  expect(started.state().model).toBe("maker/model")

  const result = await started.start("Plan the beds", () => {})
  if (!("session" in result)) throw new Error(result.problem)
  const conversation = takeStarted(result.session)!
  cleanups.push(async () => conversation.close())
  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end") && state.model === "maker/model")
  expect(types(replied).slice(0, 2)).toEqual(["model_changed", "turn_start"])
})

test("a draft's first message that cannot be sent once the page has moved to the session is given back for the box, with why, so nothing is lost", async () => {
  // A server whose core refuses to take a message.
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  const core = served.run.evie
  const evie: Evie = {
    send: (command: Command) => (command.type === "send_message" ? Promise.reject(new Error("The event log could not be written.")) : core.send(command)) as never,
    subscribe: (listener) => core.subscribe(listener),
    close: async () => {},
  }
  const server = startServer({ evie, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const connection = openConnection({ url: server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  const { draft: started } = await draft(connection, null)

  const result = await started.start("Plan the beds", () => {})
  if (!("session" in result)) throw new Error(result.problem)
  const conversation = takeStarted(result.session)!
  cleanups.push(async () => conversation.close())
  const given = await until(conversation, (state) => state.givenBack !== undefined)

  expect(given.givenBack).toEqual({ text: "Plan the beds", problem: "The event log could not be written." })
  expect(given.pending).toEqual([])
  expect(conversation.takeGivenBack()).toEqual({ text: "Plan the beds", problem: "The event log could not be written." })
  expect(conversation.takeGivenBack()).toBeUndefined()
  expect(conversation.state().givenBack).toBeUndefined()
})

// ---- Files ---------------------------------------------------------------------------

const file = (name: string, text: string, type = "text/plain") => new File([text], name, { type })
// Today's date, as attachments are kept under it.
const today = () => {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`
}

test("a message sent with files sends each to the server first, and the message the session records carries them", async () => {
  const { served, connection, model } = await start([{ text: ["Seen."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  expect(await conversation.send("Read these", { files: [{ file: file("plan.txt", "Beds.\n") }, { file: file("notes.txt", "Water.\n") }] })).toBeUndefined()

  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  const message = replied.events.find((event) => event.type === "user_message")
  expect(message?.type === "user_message" && message.body.attachments?.map(({ name, type, size }) => ({ name, type, size }))).toEqual([
    { name: "plan.txt", type: "text/plain", size: 6 },
    { name: "notes.txt", type: "text/plain", size: 7 },
  ])
  expect(model.requests[0]?.messages[0]?.text).toContain(`[Attached: "plan.txt" (text/plain, 6 bytes), kept at "`)
})

test("a file the server refuses stops the message, which is not sent, and says why", async () => {
  const { served, connection, model } = await start([{ text: ["Seen."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  const plan: Outgoing = { file: file("plan.txt", "Beds.\n") }
  const bad: Outgoing = { file: file("..", "x") }
  const refused = await conversation.send("Read this", { files: [plan, bad] })

  expect(refused).toBe('".." is not a plain file name. A file is attached by its name alone, such as "plan.pdf".')
  expect(conversation.state().pending).toEqual([])
  await Bun.sleep(20)
  expect(model.requests).toHaveLength(0)

  // What was sent before the refusal is kept with the file, and sent again
  // with the message, not uploaded a second time.
  // Read as a value: Bun's toMatchObject with a matcher in it changes what it is given.
  expect(plan.path).toEndWith("/plan.txt")
  expect(await conversation.send("Read this", { files: [plan] })).toBeUndefined()
  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  const message = replied.events.find((event) => event.type === "user_message")
  expect(message?.type === "user_message" && message.body.attachments?.map((each) => each.name)).toEqual(["plan.txt"])
  expect(await readdir(join(served.run.projectFolder, ".evie", "attachments", today()))).toEqual(["plan.txt"])
})

test("a message with files shows at once with them, as sending, until the session has it", async () => {
  const { served, connection } = await start([{ text: ["Seen."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  const photo = file("photo.png", "x", "image/png")

  const sending = conversation.send("Look", { files: [{ file: photo }] })
  const after = conversation.send("And this")
  // One list of what is on its way, in the order it was sent.
  expect(conversation.state().pending).toEqual([{ text: "Look", files: [photo] }, { text: "And this" }])
  expect(transcriptOf(conversation.state()).slice(-2)).toMatchObject([
    { kind: "owner", text: "Look", sending: [photo] },
    { kind: "owner", text: "And this" },
  ])
  expect(await sending).toBeUndefined()
  expect(await after).toBeUndefined()

  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  expect(replied.pending).toEqual([])
  const owners = transcriptOf(replied).filter((item) => item.kind === "owner")
  expect(owners.map((item) => item.kind === "owner" && item.text)).toEqual(["Look", "And this"])
})

test("a conversation knows its session's project and lists the project's files for @; one with no project has none to list", async () => {
  const { served, connection } = await start()
  await Bun.write(`${served.run.projectFolder}/plan.md`, "Beds.\n")
  const conversation = open(connection, served.run.session)
  const opened = await until(conversation, (state) => state.opened)

  expect(opened.project).toBe(served.run.project!.id)
  expect(await conversation.listFiles()).toEqual({ files: ["plan.md"], more: false })

  const general = open(connection, (await served.run.evie.send({ type: "open_session" })).session)
  expect((await until(general, (state) => state.opened)).project).toBeNull()
  await expect(general.listFiles()).rejects.toThrow("This session is in no project, so it has no files to mention.")
})

test("a draft's first message carries its files, and a draft in a project lists the project's files", async () => {
  const { served, connection } = await start([{ text: ["Seen."] }])
  await Bun.write(`${served.run.projectFolder}/plan.md`, "Beds.\n")
  const { draft: started } = await draft(connection, served.run.project!.id)

  expect(await started.listFiles()).toEqual({ files: ["plan.md"], more: false })
  const result = await started.start("Read this", () => {}, [{ file: file("plan.txt", "Beds.\n") }])
  if (!("session" in result)) throw new Error(result.problem)
  const conversation = takeStarted(result.session)!
  cleanups.push(async () => conversation.close())

  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  const message = replied.events.find((event) => event.type === "user_message")
  expect(message?.type === "user_message" && message.body.attachments?.map((each) => each.name)).toEqual(["plan.txt"])
})

test("a file kept for the message but since gone is sent again from the box, once, and the message goes with it", async () => {
  const { served, connection } = await start([{ text: ["Seen."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  const plan: Outgoing = { file: file("plan.txt", "Beds.\n"), path: join(served.run.projectFolder, ".evie", "attachments", "gone", "plan.txt") }

  expect(await conversation.send("Read this", { files: [plan] })).toBeUndefined()

  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  const message = replied.events.find((event) => event.type === "user_message")
  expect(message?.type === "user_message" && message.body.attachments?.map((each) => each.name)).toEqual(["plan.txt"])
  expect(plan.path).toContain(today())
})

test("an upload the server does not take in time fails, saying so; the message comes back, and the next is not held by it", async () => {
  const run = await startEvie([scriptedProvider([{ text: ["Noted."] }]).plugin])
  cleanups.push(run.stop)
  const tokens = await openTokens(run.home)
  cleanups.push(async () => tokens.close())
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  // A server that takes files in and never answers.
  const stalls: Attachments = { save: () => new Promise(() => {}), refusal: () => undefined, open: async () => undefined }
  const server = startServer({ evie: run.evie, tokens, host: "127.0.0.1", port: 0, attachments: stalls })
  cleanups.push(() => server.stop())
  const connection = openConnection({ url: server.url, token: made.secret, firstWait: 10, uploadWithin: 200 })
  cleanups.push(() => connection.close())
  const conversation = open(connection, run.session)
  await until(conversation, (state) => state.opened)

  const stalled = conversation.send("Read this", { files: [{ file: file("plan.txt", "Beds.\n") }], giveBack: true })
  const next = conversation.send("Something else")

  expect(await stalled).toBe("plan.txt could not be sent: evie's server did not take it in time. Send it again.")
  expect(conversation.takeGivenBack()?.files?.map((each) => each.file.name)).toEqual(["plan.txt"])
  expect(await next).toBeUndefined()
  const replied = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  expect(replied.events.filter((event) => event.type === "user_message").map((event) => event.type === "user_message" && event.body.text)).toEqual(["Something else"])
})

// ---- Queued messages ----------------------------------------------------------------------

const queued = (state: ConversationState) => state.queue.items.map((item) => item.text)
const turnEnds = (state: ConversationState) => state.events.filter((event) => event.type === "turn_end").length
const ownMessages = (state: ConversationState) => state.events.flatMap((event) => (event.type === "user_message" ? [event.body.text] : []))

test("a message queued while evie works waits in the page, not steering the turn, and is sent as a turn of its own once the turn has finished", async () => {
  const { served, connection, model } = await start([...asking(), { text: ["Watered."] }], { plugins: [ask()] })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  const { waiting } = await until(conversation, (state) => state.waiting.length === 1)

  conversation.queue("Then water them", [])
  expect(queued(conversation.state())).toEqual(["Then water them"])
  expect(conversation.state().pending).toEqual([])

  await conversation.answer(waiting[0]!.id, "South bed")
  const done = await until(conversation, (state) => turnEnds(state) === 2)
  expect(done.queue.items).toEqual([])
  expect(types(done).filter((type) => type === "turn_start")).toHaveLength(2)
  expect(said(done).slice(-4)).toEqual(["South bed", "Starting there.", "Then water them", "Watered."])
  expect(model.requests).toHaveLength(3)
})

test("a turn stopped holds the queue: a turn the box starts and finishes leaves it held, and only a message sent from it by hand lets it go on", async () => {
  const { served, connection, model } = await start([{ hang: true, text: ["Thinking about the beds"] }, { text: ["Hello."] }, { text: ["Watered."] }, { text: ["Weeded."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  await until(conversation, (state) => state.replySoFar === "Thinking about the beds")
  conversation.queue("Then water them", [])
  conversation.queue("Then weed the paths", [])

  conversation.stop()
  await until(conversation, (state) => !state.turnRunning && turnEnds(state) === 1)
  expect(conversation.state().queue.held).toBe("stopped")

  // A turn started from the box, which finishes, lets nothing go.
  await conversation.send("Hello there")
  await until(conversation, (state) => turnEnds(state) === 2)
  await Bun.sleep(400)
  expect(queued(conversation.state())).toEqual(["Then water them", "Then weed the paths"])
  expect(conversation.state().queue.held).toBe("stopped")
  expect(model.requests).toHaveLength(2)

  // Sent by hand, the first goes, and the second after its turn.
  await conversation.sendQueuedNow(conversation.state().queue.items[0]!.key)
  const sent = await until(conversation, (state) => turnEnds(state) === 4)
  expect(sent.queue.items).toEqual([])
  expect(ownMessages(sent).slice(-2)).toEqual(["Then water them", "Then weed the paths"])
})

test("a queued message edited into a command runs as the command when its turn comes, as one typed would", async () => {
  const { served, connection } = await start([...asking()], { plugins: [ask(), models()] })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  const { waiting } = await until(conversation, (state) => state.waiting.length === 1)
  conversation.queue("Then say which model", [])
  const key = conversation.state().queue.items[0]!.key
  conversation.editQueued(key)
  conversation.saveQueued(key, "/model", [])

  await conversation.answer(waiting[0]!.id, "South bed")
  const ran = await until(conversation, (state) => said(state).includes("The model is test/model (profile: default)."))
  expect(ownMessages(ran)).not.toContain("/model")
  expect(ran.queue.items).toEqual([])
})

test("leaving the session gives up an edit and holds its queue, which the page keeps, held, for when the owner comes back; nothing is sent meanwhile", async () => {
  const { served, connection, model } = await start([...asking(), { text: ["Watered."] }], { plugins: [ask()] })
  const held = holdConversation(connection, served.run.session)
  const { conversation } = held
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  const { waiting } = await until(conversation, (state) => state.waiting.length === 1)
  conversation.queue("Then water them", [])
  conversation.editQueued(conversation.state().queue.items[0]!.key)

  conversation.leave()
  held.release()
  // Answered elsewhere, as in a terminal, the turn finishes.
  await served.run.evie.send({ type: "answer_question", session: served.run.session, id: waiting[0]!.id, choice: "South bed" })
  while (!(await served.run.evie.send({ type: "read_history", session: served.run.session, before: 1000 })).events.some((event) => event.type === "turn_end")) await Bun.sleep(5)
  await Bun.sleep(400)
  expect(model.requests).toHaveLength(2)

  const back = holdConversation(connection, served.run.session)
  cleanups.push(async () => back.release())
  expect(back.conversation).not.toBe(conversation)
  const returned = await until(back.conversation, (state) => state.opened)
  expect(queued(returned)).toEqual(["Then water them"])
  expect(returned.queue.held).toBe("left")
  expect(returned.queue.editing).toBeUndefined()

  await back.conversation.sendQueuedNow(returned.queue.items[0]!.key)
  await until(back.conversation, (state) => turnEnds(state) === 2)
  expect(model.requests).toHaveLength(3)
})

// Stops the server in front of a session, so that the page's line drops,
// does what is given meanwhile, and starts it again on its port.
async function whileDown(served: Awaited<ReturnType<typeof start>>["served"], connection: ReturnType<typeof openConnection>, meanwhile: () => Promise<void>) {
  const port = Number(new URL(served.server.url).port)
  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  await meanwhile()
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())
}

test("a turn that finished while the line was down lets the queue go on once the session is open again, one message at a time", async () => {
  const { served, connection, model } = await start([{ text: ["Planned."] }, { text: ["Watered."] }, { text: ["Weeded."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  conversation.queue("Then water them", [])
  conversation.queue("Then weed the paths", [])

  await whileDown(served, connection, async () => {
    await served.run.evie.send({ type: "send_message", session: served.run.session, text: "Plan the beds" })
    while (!(await served.run.evie.send({ type: "read_history", session: served.run.session, before: 1000 })).events.some((event) => event.type === "turn_end")) await Bun.sleep(5)
  })

  // Once back, one goes; the next waits for its turn to end.
  const first = await until(conversation, (state) => ownMessages(state).includes("Then water them"))
  expect(ownMessages(first)).not.toContain("Then weed the paths")
  const done = await until(conversation, (state) => turnEnds(state) === 3)
  expect(ownMessages(done)).toEqual(["Plan the beds", "Then water them", "Then weed the paths"])
  expect(model.requests).toHaveLength(3)
})

test("a turn stopped while the line was down holds the queue once the session is open again, as the last turn's end that was read says", async () => {
  const { served, connection, model } = await start([{ hang: true, text: ["Thinking"] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await conversation.send("Plan the beds")
  await until(conversation, (state) => state.replySoFar === "Thinking")
  conversation.queue("Then water them", [])

  await whileDown(served, connection, async () => {
    await served.run.evie.send({ type: "cancel", session: served.run.session })
    while (!(await served.run.evie.send({ type: "read_history", session: served.run.session, before: 1000 })).events.some((event) => event.type === "turn_end")) await Bun.sleep(5)
  })

  const back = await until(conversation, (state) => connection.state().status === "connected" && !state.turnRunning && turnEnds(state) === 1)
  await Bun.sleep(400)
  expect(conversation.state().queue.held).toBe("stopped")
  expect(queued(back)).toEqual(["Then water them"])
  expect(model.requests).toHaveLength(1)
})

test("a file evie shows is drawn under its reply once the turn has ended, and the address the page fetches it from gives it to the signed-in page", async () => {
  const home = await realpath(await mkdtemp(join(tmpdir(), "evie-home-")))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const script: ScriptedReply[] = [{ text: ["Drawing it."], calls: [{ name: "show", input: { paths: ["out/chart.png"] } }] }, { text: ["Here is the chart."] }]
  const { served, connection } = await start(script, { plugins: [show({ dataFolder: home })], home })
  const png = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52, 0, 0, 0, 4, 0, 0, 0, 3])
  await mkdir(join(served.run.projectFolder, "out"))
  await writeFile(join(served.run.projectFolder, "out", "chart.png"), png)
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  await conversation.send("Draw the chart")
  const ended = await until(conversation, (state) => !state.turnRunning && state.events.some((event) => event.type === "turn_end"))
  const items = transcriptOf(ended)

  expect(items.map((item) => item.kind)).toEqual(["owner", "fold", "reply", "shown"])
  const shown = items.at(-1)
  if (shown?.kind !== "shown") throw new Error("Nothing was shown.")
  expect(shown.file).toMatchObject({ name: "chart.png", type: "image/png", storage: { project: served.run.project!.id } })
  const fetched = await fetch(new URL(shown.file.address!, served.server.url), { headers: { Authorization: `Bearer ${served.token}` } })
  expect([fetched.status, fetched.headers.get("content-type")]).toEqual([200, "image/png"])
  expect([...new Uint8Array(await fetched.arrayBuffer())]).toEqual([...png])
})
