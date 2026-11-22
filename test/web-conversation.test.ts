import { afterEach, expect, test } from "bun:test"
import { startServer } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { openConversation, type Conversation, type ConversationState } from "../web/src/conversation.ts"
import { transcriptOf } from "../web/src/transcript.ts"
import { say } from "./support/harness.ts"
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

async function start(script: ScriptedReply[] = []) {
  const served = await serveEvie([scriptedProvider(script).plugin])
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  return { served, connection }
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

test("a session that cannot be opened, as one that is not there, says why in words for the owner", async () => {
  const { connection } = await start()
  const conversation = open(connection, "no-such-session")

  const failed = await until(conversation, (state) => state.problem !== undefined)
  expect(failed.problem).toBe('This session could not be opened: There is no session with the id "no-such-session".')
  expect(failed.opened).toBe(false)
})
