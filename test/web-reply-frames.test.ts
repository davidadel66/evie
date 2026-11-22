import { afterEach, expect, test } from "bun:test"
import type { Command, EventBodies, Evie, EvieEvent, Job, StoredEvent } from "../src/core/types.ts"
import { openConnection, type Connection, type ConnectionState } from "../web/src/connection.ts"
import { openConversation, type Conversation, type ConversationState, type NextFrame } from "../web/src/conversation.ts"
import { transcriptOf } from "../web/src/transcript.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// A reply streams in uneven bursts, and each change to the conversation
// draws it again: so the pacer reveals it smoothly, at most one slice per
// frame. These tests draw the frames themselves, as a page would, or draw
// none, as a hidden tab does. Most follow a stand-in for the server, whose
// events the test sends itself, so that it says exactly when each comes.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// The page's frames, drawn when the test says; until then, as in a hidden
// tab, none comes.
function frames() {
  const asked = new Map<number, () => void>()
  let count = 0
  const next: NextFrame = (apply) => {
    const id = ++count
    asked.set(id, apply)
    return () => void asked.delete(id)
  }
  return {
    next,
    // How many are waiting for a frame.
    waiting: () => asked.size,
    draw() {
      const now = [...asked.values()]
      asked.clear()
      for (const apply of now) apply()
    },
    // Draws frames until nothing is waiting for one.
    drain(limit = 500) {
      let drawn = 0
      while (asked.size > 0 && drawn < limit) {
        this.draw()
        drawn += 1
      }
      return drawn
    },
  }
}

// A session two events in, a turn started by the owner's message.
const SESSION = "s1"
const stored = (session: string, seq: number, type: StoredEvent["type"], body: unknown) => ({ type, session, seq, time: 1_000 + seq, body }) as StoredEvent
const begun = (session: string) => [stored(session, 1, "turn_start", {}), stored(session, 2, "user_message", { text: "Plan the beds" })]
const piece = (text: string, session = SESSION): EvieEvent => ({ type: "assistant_delta", session, text })
const job = (status: Job["status"]): EvieEvent => ({ type: "job", session: SESSION, job: { id: "j1", session: SESSION, kind: "shell", title: "bun test", status, started: 1_002, outputFile: "/out" } })
const reply = (text: string, seq = 3) => stored(SESSION, seq, "assistant_message", { text } satisfies EventBodies["assistant_message"])
const end = (seq = 4) => stored(SESSION, seq, "turn_end", { reason: "completed" } satisfies EventBodies["turn_end"])

// A stand-in for the server's client: it answers what a conversation asks
// as it opens, with its reply so far, and `say` sends an event as the
// server would.
function client(replySoFar = "") {
  const listeners = new Set<(event: EvieEvent) => void>()
  const answer = (command: Command) => {
    if (command.type === "open_session") return { session: command.session, project: null, resumed: false, notices: [], turnRunning: true, openQuestions: [], lastSeq: 2, jobs: [], ...(replySoFar !== "" && { replySoFar }) }
    if (command.type === "read_history") return { events: begun(command.session!), earlier: false }
    if (command.type === "read_context") return { model: "stand-in", size: 0, compactsAt: 100 }
    throw new Error(`The stand-in does not answer ${command.type}.`)
  }
  const evie: Evie = {
    send: (async (command: Command) => answer(command)) as Evie["send"],
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    close: async () => {},
  }
  return { evie, say: (...events: EvieEvent[]) => events.forEach((event) => [...listeners].forEach((listener) => listener(event))) }
}

// A connection to the stand-in, connected from the start; `connect` makes a
// new client, as signing in again does.
function standIn() {
  let now = client()
  let state: ConnectionState = { status: "connected", evie: now.evie }
  const listeners = new Set<() => void>()
  const connection: Connection = {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    signIn: async () => undefined,
    close: async () => {},
    upload: async () => {
      throw new Error("The stand-in takes no files.")
    },
    fetchFile: async () => new Response("The stand-in gives no files.", { status: 404 }),
  }
  return {
    connection,
    say: (...events: EvieEvent[]) => now.say(...events),
    connect(replySoFar: string) {
      now = client(replySoFar)
      state = { status: "connected", evie: now.evie }
      for (const listener of [...listeners]) listener()
    },
  }
}

function open(connection: Connection, session: string, frame: NextFrame) {
  const conversation = openConversation(connection, session, frame)
  cleanups.push(async () => conversation.close())
  return conversation
}

async function until(conversation: Conversation, ready: (state: ConversationState) => boolean) {
  while (!ready(conversation.state())) await Bun.sleep(5)
  return conversation.state()
}

// Each state the conversation changes to, as its screen is told.
function changes(conversation: Conversation) {
  const seen: ConversationState[] = []
  conversation.subscribe(() => seen.push(conversation.state()))
  return seen
}

const said = (state: ConversationState) => transcriptOf(state).map((item) => ("text" in item ? item.text : item.kind))

test("pieces that arrive between frames reveal smoothly, at most once per frame, until all of them are shown", async () => {
  const { connection, say } = standIn()
  const frame = frames()
  const conversation = open(connection, SESSION, frame.next)
  await until(conversation, (state) => state.opened)
  const seen = changes(conversation)

  say(piece("Thinking "), piece("about "), piece("the beds"))
  expect([conversation.state().replySoFar, seen.length, frame.waiting()]).toEqual(["", 0, 1])

  // One frame reveals only a slice, changes the conversation once, and keeps
  // asking for the next frame until the burst has finished gliding out.
  frame.draw()
  expect(seen.length).toBe(1)
  expect(conversation.state().replySoFar.length).toBeGreaterThan(0)
  expect(conversation.state().replySoFar.length).toBeLessThan("Thinking about the beds".length)
  expect(frame.waiting()).toBe(1)

  const rest = frame.drain()
  expect(rest).toBeGreaterThan(1)
  expect([conversation.state().replySoFar, frame.waiting()]).toEqual(["Thinking about the beds", 0])

  // The next pieces wait for the frames after, and extra frames with nothing buffered change nothing.
  say(piece(" by the "), piece("fence"))
  const before = seen.length
  frame.drain()
  const after = seen.length
  frame.draw()
  frame.draw()
  expect(after).toBeGreaterThan(before)
  expect(seen.length).toBe(after)
  expect(conversation.state().replySoFar).toBe("Thinking about the beds by the fence")
})

test("an event after pieces not applied yet applies them first, in the same change, so the conversation changes in the order its events came", async () => {
  const { connection, say } = standIn()
  const frame = frames()
  const conversation = open(connection, SESSION, frame.next)
  await until(conversation, (state) => state.opened)
  const seen = changes(conversation)

  say(piece("Thinking "), piece("about"), job("running"))
  expect(seen.map((state) => [state.replySoFar, state.jobs.length])).toEqual([["Thinking about", 1]])

  // The stored reply ends the pieces before it, which are not applied after it.
  say(piece(" the beds."), reply("Thinking about the beds."), end())
  expect(seen.slice(1).map((state) => [state.replySoFar, state.events.at(-1)!.type, state.turnRunning])).toEqual([
    ["", "assistant_message", true],
    ["", "turn_end", false],
  ])
  expect(frame.waiting()).toBe(0)
  frame.draw()
  expect(seen.length).toBe(3)
  expect(said(conversation.state())).toEqual(["Plan the beds", "Background job j1 started: bun test", "Thinking about the beds."])
})

test("however the frames fall, the conversation goes through the states it would piece by piece, fewer of them, and ends the same", async () => {
  const { connection, say } = standIn()
  // Drawn after every event, as if each piece were applied as it came; and drawn now and then, with stretches of none.
  const everyEvent = frames()
  const nowAndThen = frames()
  const byPiece = open(connection, SESSION, everyEvent.next)
  const batched = open(connection, SESSION, nowAndThen.next)
  await until(byPiece, (state) => state.opened)
  await until(batched, (state) => state.opened)
  const piecewise = changes(byPiece)
  const together = changes(batched)

  const words = Array.from({ length: 60 }, (_, at) => `word${at} `)
  const events: EvieEvent[] = [...words.slice(0, 20).map((text) => piece(text)), job("running"), ...words.slice(20).map((text) => piece(text)), job("finished"), reply(words.join("")), end()]
  events.forEach((event, at) => {
    say(event)
    everyEvent.draw()
    if (at % 7 === 3 && (at < 25 || at > 45)) nowAndThen.draw()
  })

  expect(together.length).toBeLessThan(piecewise.length / 4)
  const shown = (seen: ConversationState[]) => seen.map((state) => JSON.stringify([state.replySoFar, state.events.length, state.jobs, state.turnRunning]))
  const all = shown(piecewise)
  let from = 0
  for (const state of shown(together)) {
    from = all.indexOf(state, from)
    expect(from).not.toBe(-1)
  }
  const { replySoFar, events: stored, jobs, turnRunning } = batched.state()
  expect({ replySoFar, events: stored, jobs, turnRunning }).toEqual({ replySoFar: byPiece.state().replySoFar, events: byPiece.state().events, jobs: byPiece.state().jobs, turnRunning: byPiece.state().turnRunning })
  expect(said(batched.state())).toEqual(said(byPiece.state()))
})

test("a hidden tab, which draws no frames, loses nothing: what came applies once the tab is shown, and the stored reply applies in its place", async () => {
  const { connection, say } = standIn()
  const frame = frames()
  const conversation = open(connection, SESSION, frame.next)
  await until(conversation, (state) => state.opened)
  const seen = changes(conversation)

  // A long while hidden: a thousand pieces, and only one frame asked for.
  const text = Array.from({ length: 1000 }, (_, at) => `piece ${at}. `)
  say(...text.map((each) => piece(each)))
  expect([conversation.state().replySoFar, seen.length, frame.waiting()]).toEqual(["", 0, 1])

  // Shown again: frames begin draining the backlog smoothly until all of it is shown.
  frame.draw()
  expect(seen.length).toBe(1)
  expect(conversation.state().replySoFar.length).toBeGreaterThan(0)
  expect(conversation.state().replySoFar.length).toBeLessThan(text.join("").length)
  frame.drain()
  expect(conversation.state().replySoFar).toBe(text.join(""))

  // Hidden again until the turn has ended: the reply is as stored, and nothing is left to apply.
  const beforeEnd = seen.length
  say(piece("The end."), reply(`${text.join("")}The end.`), end())
  frame.draw()
  expect(conversation.state().replySoFar).toBe("")
  expect(said(conversation.state())).toEqual(["Plan the beds", `${text.join("")}The end.`])
  expect(seen.length).toBe(beforeEnd + 2)
})

test("pieces not applied yet stay with their own session's conversation: switching to another shows none of them there, and the one left is not changed after", async () => {
  const { connection, say } = standIn()
  const frame = frames()
  const first = open(connection, SESSION, frame.next)
  await until(first, (state) => state.opened)
  const leftSeen = changes(first)
  say(piece("Thinking about the beds"))

  // The owner switches to another session before the frame, as the middle column does: the first is closed.
  first.close()
  const second = open(connection, "s2", frame.next)
  await until(second, (state) => state.opened)
  say(piece("A different reply", "s2"), piece(" goes on", "s2"))
  expect(frame.waiting()).toBe(1)
  frame.drain()

  expect(second.state().replySoFar).toBe("A different reply goes on")
  expect(first.state().replySoFar).toBe("")
  expect(leftSeen).toEqual([])
})

test("on a new connection, the reply is taken from the session as it stands, without the pieces kept from the client before", async () => {
  const line = standIn()
  const frame = frames()
  const conversation = open(line.connection, SESSION, frame.next)
  await until(conversation, (state) => state.opened)
  line.say(piece("Thinking "), piece("about"))

  // Signed in again before the frame: the session says how far its reply has come.
  line.connect("Thinking about the beds")
  await until(conversation, (state) => state.opened && state.replySoFar !== "")
  frame.draw()
  line.say(piece(" by the fence"))
  frame.drain()
  expect(conversation.state().replySoFar).toBe("Thinking about the beds by the fence")
})

test("in front of the real server, a tab hidden while the reply streams, which draws no frame, shows the reply as stored once its turn has ended, followed by the end", async () => {
  const served = await serveEvie([scriptedProvider([{ hang: true, text: ["Thinking about ", "the beds"] }] satisfies ScriptedReply[]).plugin])
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  const frame = frames()
  const conversation = open(connection, served.run.session, frame.next)
  await until(conversation, (state) => state.opened)
  // The page's client, heard after the conversation, which has each event first.
  const reached: string[] = []
  const line = connection.state()
  if (line.status !== "connected") throw new Error("not connected")
  const unhear = line.evie.subscribe((event) => void (event.type === "assistant_delta" && reached.push(event.text)))
  cleanups.push(async () => unhear())

  await served.run.evie.send({ type: "send_message", session: served.run.session, text: "Plan the beds" })
  while (reached.length < 2) await Bun.sleep(5)

  await served.run.evie.send({ type: "cancel", session: served.run.session })
  const stopped = await until(conversation, (state) => !state.turnRunning)
  expect([stopped.replySoFar, frame.waiting()]).toEqual(["", 0])
  expect(stopped.events.map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "turn_end"])
  expect(said(stopped)).toEqual(["Plan the beds", "Thinking about the beds", "Stopped. The reply was cut short."])
})
