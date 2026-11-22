import { afterEach, expect, test } from "bun:test"
import { mkdtemp, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { startServer, type Command, type Evie, type EvieOptions } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import { openConnection } from "../web/src/connection.ts"
import { openConversation, type Conversation, type ConversationState } from "../web/src/conversation.ts"
import { transcriptOf } from "../web/src/transcript.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The conversation the web app shows in its middle column, apart from its
// screen: a session opened by its id, its latest page of history, earlier
// pages as they are asked for, and its events as they come, across a drop
// of the line too. These tests connect with a token, as the connection's
// own tests do.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(script: ScriptedReply[] = [], options: Parameters<typeof serveEvie>[1] = {}) {
  const served = await serveEvie([scriptedProvider(script).plugin], options)
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  return { served, connection }
}

// Starts the server in front of a core whose commands pass through `refuse`,
// which can make one fail, as a server with a fault would. The connection
// is to that server; `restart` stops it and starts it again on its port.
async function startRefusing(script: ScriptedReply[], refuse: (command: Command) => boolean) {
  const served = await serveEvie([scriptedProvider(script).plugin])
  cleanups.push(served.stop)
  const core = served.run.evie
  const evie: Evie = {
    send: (command) => (refuse(command) ? Promise.reject(new Error("The event log could not be read.")) : core.send(command)) as never,
    subscribe: (listener) => core.subscribe(listener),
    close: async () => {},
  }
  let server = startServer({ evie, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const connection = openConnection({ url: server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  async function restart() {
    const port = Number(new URL(server.url).port)
    await server.stop()
    while (connection.state().status !== "reconnecting") await Bun.sleep(5)
    server = startServer({ evie, tokens: served.tokens, host: "127.0.0.1", port })
  }
  return { served, connection, restart }
}

// Opens the conversation of a session on a connection.
function open(connection: Parameters<typeof openConversation>[0], session: string) {
  const conversation = openConversation(connection, session)
  cleanups.push(async () => conversation.close())
  return conversation
}

// Settles once the conversation is as `ready` says, and gives it as it is then.
async function until(conversation: Conversation, ready: (state: ConversationState) => boolean) {
  while (!ready(conversation.state())) await Bun.sleep(5)
  return conversation.state()
}

const replies = (count: number): ScriptedReply[] => Array.from({ length: count }, (_, at) => ({ text: [`Reply ${at + 1}.`] }))
const seqs = (state: ConversationState) => state.events.map((event) => event.seq)

test("opening a session by its id gives its latest page of history, 50 events, saying there are earlier ones, and the page before it when asked", async () => {
  // Fourteen turns of four events each.
  const { served, connection } = await start(replies(14))
  for (let at = 1; at <= 14; at++) await say(served.run, `Message ${at}`)

  const conversation = open(connection, served.run.session)
  const opened = await until(conversation, (state) => state.opened)

  expect(seqs(opened)).toEqual(Array.from({ length: 50 }, (_, at) => at + 7))
  expect(opened.earlier).toBe(true)
  expect(opened.turnRunning).toBe(false)

  conversation.readEarlier()
  const whole = await until(conversation, (state) => !state.earlier)
  expect(seqs(whole)).toEqual(Array.from({ length: 56 }, (_, at) => at + 1))
  expect(whole.problem).toBeUndefined()
})

const types = (state: ConversationState) => state.events.map((event) => `${event.seq} ${event.type}`)

test("its events are followed as they come, and the reply streams in as it is written, until it is stored", async () => {
  const { served, connection } = await start([{ hang: true, text: ["Thinking about ", "the beds"] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  await served.run.evie.send({ type: "send_message", session: served.run.session, text: "Plan the beds" })
  const writing = await until(conversation, (state) => state.replySoFar === "Thinking about the beds")
  expect(writing.turnRunning).toBe(true)
  expect(types(writing)).toEqual(["1 turn_start", "2 user_message"])

  await served.run.evie.send({ type: "cancel", session: served.run.session })
  const stopped = await until(conversation, (state) => !state.turnRunning)
  expect(stopped.replySoFar).toBe("")
  expect(types(stopped)).toEqual(["1 turn_start", "2 user_message", "3 assistant_message", "4 turn_end"])
})

test("a session joined while its reply is being written shows the reply so far, and what follows", async () => {
  const { served, connection } = await start([{ hang: true, text: ["Thinking about ", "the beds"] }])
  await served.run.evie.send({ type: "send_message", session: served.run.session, text: "Plan the beds" })
  while (!served.run.events.some((event) => event.type === "assistant_delta" && event.text === "the beds")) await Bun.sleep(5)

  const conversation = open(connection, served.run.session)
  const joined = await until(conversation, (state) => state.opened)
  expect([joined.replySoFar, joined.turnRunning]).toEqual(["Thinking about the beds", true])
  expect(types(joined)).toEqual(["1 turn_start", "2 user_message"])

  await served.run.evie.send({ type: "cancel", session: served.run.session })
  const stopped = await until(conversation, (state) => !state.turnRunning)
  expect([stopped.replySoFar, types(stopped).at(-1)]).toEqual(["", "4 turn_end"])
})

test("when the line drops and comes back, what was missed is caught up on, each event once, and the reply that was being written is not written twice", async () => {
  const { served, connection } = await start([{ hang: true, text: ["Half way"] }, { text: ["Done."] }])
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  await served.run.evie.send({ type: "send_message", session: served.run.session, text: "First" })
  await until(conversation, (state) => state.replySoFar === "Half way")
  const port = Number(new URL(served.server.url).port)

  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  await served.run.evie.send({ type: "cancel", session: served.run.session })
  await say(served.run, "Second")
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  const caughtUp = await until(conversation, (state) => state.events.length >= 8 && !state.turnRunning)
  await Bun.sleep(50)
  const now = conversation.state()
  expect(caughtUp.replySoFar).toBe("")
  expect(seqs(now)).toEqual([1, 2, 3, 4, 5, 6, 7, 8])
  expect(transcriptOf(now).map((item) => ("text" in item ? item.text : item.kind))).toEqual(["First", "Half way", "Stopped. The reply was cut short.", "Second", "Done."])
})

test("a catch-up of megabytes, which comes in pages, shows every event missed once, in order, though more were missed than the latest page of history holds", async () => {
  // Replies of 200 KiB, five to a page of the catch-up. Compaction is put
  // off, so that the model is asked for nothing but the replies.
  const replies = Array.from({ length: 20 }, (_, at) => String(at % 10).repeat(200 * 1024))
  const { served, connection } = await start(replies.map((text) => ({ text: [text] })), { compaction: { ceiling: 1_000_000_000 } })
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)
  const port = Number(new URL(served.server.url).port)

  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  // Twenty turns of four events each, about 4 MB, while the line is down.
  for (let at = 1; at <= 20; at++) await say(served.run, `Message ${at}`)
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  await until(conversation, (state) => state.events.length >= 80 && connection.state().status === "connected")
  await Bun.sleep(50)
  const now = conversation.state()
  expect(seqs(now)).toEqual(Array.from({ length: 80 }, (_, at) => at + 1))
  expect(now.replySoFar).toBe("")
  // Each message and each reply once, in turn.
  expect(transcriptOf(now).flatMap((item) => ("text" in item ? [item.text.slice(0, 10)] : []))).toEqual(
    replies.flatMap((reply, at) => [`Message ${at + 1}`, reply.slice(0, 10)]),
  )
})

test("a session of large events opens on its latest megabyte of history, and reading earlier reaches every event", async () => {
  const replies = Array.from({ length: 12 }, (_, at) => String(at % 10).repeat(400 * 1024))
  const { served, connection } = await start(replies.map((text) => ({ text: [text] })), { compaction: { ceiling: 1_000_000_000 } })
  for (let at = 1; at <= 12; at++) await say(served.run, `Message ${at}`)

  const conversation = open(connection, served.run.session)
  const opened = await until(conversation, (state) => state.opened)
  // Two replies of 400 KiB fit in a page, a third does not.
  expect(seqs(opened)).toEqual([40, 41, 42, 43, 44, 45, 46, 47, 48])
  expect(opened.earlier).toBe(true)

  while (conversation.state().earlier) {
    conversation.readEarlier()
    const before = conversation.state().events.length
    await until(conversation, (state) => state.events.length > before || !state.earlier)
  }
  expect(seqs(conversation.state())).toEqual(Array.from({ length: 48 }, (_, at) => at + 1))
})

test("a session that cannot be opened, as one that is not there, says why in words for the owner", async () => {
  const { connection } = await start()
  const conversation = open(connection, "no-such-session")

  const failed = await until(conversation, (state) => state.problem !== undefined)
  expect(failed.problem).toBe('This session could not be opened: There is no session with the id "no-such-session".')
  expect(failed.opened).toBe(false)
})

// A project that asks whether to trust it each time its plugins are set up,
// as one with a settings file of its own does until the owner decides.
const asksTrust: NonNullable<EvieOptions["projectSettings"]> = async (_project, { ask }) => {
  const answer = await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
  return { plugins: [], ...(answer !== "Yes" && { forNow: true as const }) }
}

test("a session not running in the server, whose project asks to be trusted, opened from the page leaves the question waiting and shows it, and so does opening it again after the server is started again", async () => {
  const home = await mkdtemp(join(tmpdir(), "evie-home-"))
  const projectFolder = await mkdtemp(join(tmpdir(), "evie-project-"))
  cleanups.push(() => rm(projectFolder, { recursive: true, force: true }), () => rm(home, { recursive: true, force: true }))
  const tokens = await openTokens(home)
  cleanups.push(async () => tokens.close())
  const made = tokens.add("test device")
  if (!("secret" in made)) throw new Error(made.refused)
  // The session is made in one run of evie; the next sets its project up again when it is opened.
  const first = await startEvie([scriptedProvider([{ text: ["Noted."] }]).plugin], { home, projectFolder, projectSettings: asksTrust })
  await say(first, "Plan the beds")
  await first.stop()
  const serve = async (port = 0) => {
    const run = await startEvie([scriptedProvider([]).plugin], { home, projectFolder, projectSettings: asksTrust })
    const server = startServer({ evie: run.evie, tokens, host: "127.0.0.1", port })
    return { run, server, async stop() { await server.stop(); await run.stop() } }
  }
  let served = await serve()
  cleanups.push(() => served.stop())
  const connection = openConnection({ url: served.server.url, token: made.secret, firstWait: 10 })
  cleanups.push(() => connection.close())
  const said = (state: ConversationState) => transcriptOf(state).map((item) => (item.kind === "question" ? `${item.text}${item.waiting ? " (waiting)" : ""}` : "text" in item ? item.text : item.kind))
  const waiting = (state: ConversationState) => transcriptOf(state).some((item) => item.kind === "question" && item.waiting)
  const answerIt = (run: typeof served.run) => {
    const question = stored(run.events).findLast((event) => event.type === "question" && event.session === first.session)!
    return run.evie.send({ type: "answer_question", session: first.session, id: question.type === "question" ? question.body.id : "", choice: "Yes" })
  }

  const conversation = open(connection, first.session)
  const asking = await until(conversation, (state) => state.events.some((event) => event.type === "question"))
  await Bun.sleep(100)
  expect(conversation.state().opened).toBe(false)
  expect(waiting(conversation.state())).toBe(true)
  expect(asking.problem).toBeUndefined()

  await answerIt(served.run)
  const opened = await until(conversation, (state) => state.opened)
  // The first run's own opening, by a client that shows no questions, was refused.
  expect(said(opened)).toEqual(["Trust this project?", "Not answered.", "Plan the beds", "Noted.", "Trust this project?", "Yes"])

  const port = Number(new URL(served.server.url).port)
  await served.stop()
  served = await serve(port)
  await until(conversation, waiting)
  await Bun.sleep(100)
  expect(waiting(conversation.state())).toBe(true)
  await answerIt(served.run)
  const again = await until(conversation, (state) => !waiting(state) && state.events.filter((event) => event.type === "answer").length === 3)
  expect(said(again)).toEqual(["Trust this project?", "Not answered.", "Plan the beds", "Noted.", "Trust this project?", "Yes", "Trust this project?", "Yes"])
})

test("when its latest page cannot be read, the conversation says why, is not taken for empty, and reads it again when the line comes back, or when asked to", async () => {
  let failing = true
  const { served, connection, restart } = await startRefusing([{ text: ["Noted."] }], (command) => command.type === "read_history" && failing)
  await say(served.run, "Plan the beds")

  const conversation = open(connection, served.run.session)
  const failed = await until(conversation, (state) => state.problem !== undefined)
  expect(failed.problem).toBe("The conversation could not be read: The event log could not be read.")
  expect(failed.opened).toBe(false)

  failing = false
  await restart()
  const read = await until(conversation, (state) => state.opened)
  expect(read.problem).toBeUndefined()
  expect(transcriptOf(read).map((item) => ("text" in item ? item.text : item.kind))).toEqual(["Plan the beds", "Noted."])

  // Asked to, as by a Try again button.
  failing = true
  const second = open(connection, served.run.session)
  await until(second, (state) => state.problem !== undefined)
  failing = false
  second.tryAgain()
  expect((await until(second, (state) => state.opened)).problem).toBeUndefined()
})

test("when the page before cannot be read, it is not read again by itself, however often it is asked for, until the owner says to try again", async () => {
  // Only the page before the latest one fails, until it is allowed.
  let allowed = false
  const { served, connection } = await startRefusing(replies(14), (command) => command.type === "read_history" && command.before !== undefined && command.before < 50 && !allowed)
  for (let at = 1; at <= 14; at++) await say(served.run, `Message ${at}`)
  let asked = 0
  const conversation = open(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  conversation.readEarlier()
  const failed = await until(conversation, (state) => state.problem !== undefined && !state.readingEarlier)
  expect(failed.problem).toBe("The earlier conversation could not be read: The event log could not be read.")
  for (let at = 0; at < 20; at++) {
    conversation.readEarlier()
    if (conversation.state().readingEarlier) asked += 1
    await Bun.sleep(1)
  }
  expect(asked).toBe(0)
  expect(conversation.state().earlier).toBe(true)

  allowed = true
  conversation.tryAgain()
  const whole = await until(conversation, (state) => !state.earlier)
  expect(whole.problem).toBeUndefined()
  expect(whole.events.length).toBe(56)
})
