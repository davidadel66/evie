import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { startServer } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { openSessions, type SessionList, type Sessions } from "../web/src/sessions.ts"
import { say } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The web app's list of sessions, apart from its screen: listed from the
// server once connected, kept current from the summaries it sends, and
// listed again when the line comes back. These tests connect with a token,
// as the connection's own tests do.

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

// Opens the list on a connection.
function list(connection: Parameters<typeof openSessions>[0]) {
  const sessions = openSessions(connection)
  cleanups.push(async () => sessions.close())
  return sessions
}

// Settles once the list is as `ready` says, and gives it as it is then.
async function until(sessions: Sessions, ready: (list: SessionList) => boolean) {
  while (!ready(sessions.state())) await Bun.sleep(5)
  return sessions.state()
}

test("once connected, every session is listed, and every project with the branch it is on", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  await say(served.run, "Plan the beds for spring")

  const sessions = list(connection)
  const listed = await until(sessions, (state) => state.listed)

  expect(listed.sessions).toEqual([{ session: served.run.session, project: served.run.project!.id, name: "Plan the beds for spring", state: "active", lastActive: expect.any(Number) }])
  expect(listed.projects).toEqual([served.run.project!])
  expect(listed.problem).toBeUndefined()
})

test("a summary the server sends keeps the list current, newest first: a session that changes takes its new place, and a new one is added", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }, { text: ["Started."] }])
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)

  await served.run.evie.send({ type: "settle_session", session: served.run.session, settled: true })
  const other = await served.run.evie.send({ type: "open_session" })
  await say(served.run, "What is the weather tomorrow?", other.session)
  const kept = await until(sessions, (state) => state.sessions.length === 2)

  expect(kept.sessions).toEqual([
    { session: other.session, project: null, name: "What is the weather tomorrow?", state: "active", lastActive: expect.any(Number) },
    { session: served.run.session, project: served.run.project!.id, name: "Plan the beds for spring", state: "settled", lastActive: expect.any(Number) },
  ])
})

test("when the line comes back after a drop, the list is listed again, since the summaries sent meanwhile were not caught up on", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)
  const port = Number(new URL(served.server.url).port)

  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  await served.run.evie.send({ type: "settle_session", session: served.run.session, settled: true })
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  const relisted = await until(sessions, (state) => state.sessions[0]?.state === "settled")
  expect(relisted.sessions.map((each) => each.state)).toEqual(["settled"])
})

test("a session in a project made since the list was listed brings that project into the list", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)
  const folder = await realpath(await mkdtemp(join(tmpdir(), "a-new-project-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))

  const opened = await served.run.evie.send({ type: "open_session", folder })
  await say(served.run, "Sort the seed packets", opened.session)

  const known = await until(sessions, (state) => state.projects.length === 2)
  expect(known.projects).toEqual([opened.project!, served.run.project!])
})

// The client the connection gives once connected.
function client(connection: ReturnType<typeof openConnection>) {
  const state = connection.state()
  if (state.status !== "connected") throw new Error("not connected")
  return state.evie
}

test("a session started from the list, in a project or in none, is listed at once as new, and its summary takes its place once something is said in it", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)

  const inProject = await sessions.start(served.run.project!.id)
  const general = await sessions.start(null)
  if (!("session" in inProject) || !("session" in general)) throw new Error("not started")

  expect(sessions.state().sessions).toEqual([
    { session: general.session, project: null, name: null, state: "active", lastActive: expect.any(Number), new: true },
    { session: inProject.session, project: served.run.project!.id, name: null, state: "active", lastActive: expect.any(Number), new: true },
  ])
  await client(connection).send({ type: "send_message", session: inProject.session, text: "Water the tomatoes" })
  const said = await until(sessions, (state) => state.sessions.some((each) => each.session === inProject.session && each.name !== null))
  expect(said.sessions.find((each) => each.session === inProject.session)).toEqual({
    session: inProject.session,
    project: served.run.project!.id,
    name: "Water the tomatoes",
    state: expect.any(String),
    lastActive: expect.any(Number),
  })
})

test("a session that cannot be started says why", async () => {
  const { connection } = await start()
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)

  expect(await sessions.start("no-such-project")).toEqual({ problem: expect.stringContaining("no-such-project") })
  expect(sessions.state().sessions).toEqual([])
})

test("a session started here with nothing said in it yet stays listed when the list is listed again", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)
  const started = await sessions.start(null)
  if (!("session" in started)) throw new Error(started.problem)
  const port = Number(new URL(served.server.url).port)

  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  await served.run.evie.send({ type: "settle_session", session: served.run.session, settled: true })
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  const relisted = await until(sessions, (state) => state.sessions.some((each) => each.state === "settled"))
  expect(relisted.sessions.map((each) => [each.session, each.state])).toEqual([
    [started.session, "active"],
    [served.run.session, "settled"],
  ])
})

test("a session is settled from the list and brought back, and one that is working is not settled, with the server's reason", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }, { hang: true }])
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)
  const stateOf = (state: SessionList, session: string) => state.sessions.find((each) => each.session === session)?.state

  expect(await sessions.settle(served.run.session, true)).toBeUndefined()
  await until(sessions, (state) => stateOf(state, served.run.session) === "settled")
  expect(await sessions.settle(served.run.session, false)).toBeUndefined()
  await until(sessions, (state) => stateOf(state, served.run.session) === "active")

  const busy = await served.run.evie.send({ type: "open_session" })
  await served.run.evie.send({ type: "send_message", session: busy.session, text: "Work on this for a while" })
  await until(sessions, (state) => stateOf(state, busy.session) === "working")
  expect(await sessions.settle(busy.session, true)).toBe("This session is working, or waiting for an answer to a question. Wait until it has finished, then settle it.")
})
