import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { startServer, type Command, type Evie } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { openSessions, type SessionList, type Sessions } from "../web/src/sessions.ts"
import { git } from "./support/git.ts"
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

// Starts the server in front of a core whose every answer passes through
// `hold`, which can keep one back, as a slow server, or a line that brings
// it late, would. The connection is to that server.
async function startHolding(script: ScriptedReply[], hold: (command: Command, answer: () => Promise<unknown>) => Promise<unknown>) {
  const served = await serveEvie([scriptedProvider(script).plugin])
  cleanups.push(served.stop)
  const core = served.run.evie
  const evie: Evie = {
    send: (command) => hold(command, () => core.send(command)) as never,
    subscribe: (listener) => core.subscribe(listener),
    close: async () => {},
  }
  const server = startServer({ evie, tokens: served.tokens, host: "127.0.0.1", port: 0 })
  cleanups.push(() => server.stop())
  const connection = openConnection({ url: server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  return { served, connection }
}

// A promise to wait on, and the function that settles it.
function gate() {
  let open = () => {}
  const opened = new Promise<void>((resolve) => (open = resolve))
  return { opened, open }
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
  const listed = await until(sessions, (state) => state.listed && state.projects.length > 0)

  expect(listed.sessions).toEqual([{ session: served.run.session, project: served.run.project!.id, name: "Plan the beds for spring", state: "active", lastActive: expect.any(Number) }])
  expect(listed.projects).toEqual([{ ...served.run.project!, git: false }])
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
  expect(known.projects).toEqual([
    { ...opened.project!, git: false },
    { ...served.run.project!, git: false },
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

test("the projects are listed one request at a time, and once more for every ask meanwhile, so a late answer never takes a project back", async () => {
  // The first answer is the projects as they were when it was asked, and comes late.
  let asked = 0
  const late = gate()
  const { served, connection } = await startHolding([{ text: ["Noted."] }], async (command, answer) => {
    if (command.type !== "list_projects") return answer()
    const at = ++asked
    const answered = await answer()
    if (at === 1) await late.opened
    return answered
  })
  const sessions = list(connection)
  while (asked === 0) await Bun.sleep(5)
  const folder = await realpath(await mkdtemp(join(tmpdir(), "a-new-project-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))

  // Each summary of the new project's session asks for the projects again.
  const opened = await served.run.evie.send({ type: "open_session", folder })
  await say(served.run, "Sort the seed packets", opened.session)
  await until(sessions, (state) => state.sessions.some((each) => each.session === opened.session))
  await Bun.sleep(50)
  const askedWhileLate = asked
  late.open()
  await until(sessions, (state) => state.projects.length === 2)
  await Bun.sleep(50)

  expect(askedWhileLate).toBe(1)
  expect(asked).toBe(2)
  expect(sessions.state().projects).toEqual([
    { ...opened.project!, git: false },
    { ...served.run.project!, git: false },
  ])
})

test("a summary that comes while the sessions are being listed wins over the list's older line for its session, and a session it brings stays", async () => {
  // The first list is the sessions as they were when it was asked, and comes late.
  let asked = 0
  const late = gate()
  const { served, connection } = await startHolding([{ text: ["Noted."] }, { text: ["Noted."] }], async (command, answer) => {
    if (command.type !== "list_sessions") return answer()
    const at = ++asked
    const answered = await answer()
    if (at === 1) await late.opened
    return answered
  })
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  while (asked === 0) await Bun.sleep(5)

  await served.run.evie.send({ type: "settle_session", session: served.run.session, settled: true })
  const other = await served.run.evie.send({ type: "open_session" })
  await say(served.run, "What is the weather tomorrow?", other.session)
  await until(sessions, (state) => state.sessions.length === 2)
  late.open()
  await until(sessions, (state) => state.listed)

  expect(sessions.state().sessions.map((each) => [each.session, each.state])).toEqual([
    [other.session, "active"],
    [served.run.session, "settled"],
  ])
})

test("when the projects cannot be listed, the sessions are listed all the same, and the projects' failure is told on its own", async () => {
  const { served, connection } = await startHolding([{ text: ["Noted."] }], async (command, answer) => {
    if (command.type === "list_projects") throw new Error("The project table could not be read.")
    return answer()
  })
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)

  const listed = await until(sessions, (state) => state.listed && state.projectsProblem !== undefined)

  expect(listed.sessions.map((each) => each.name)).toEqual(["Plan the beds for spring"])
  expect(listed.problem).toBeUndefined()
  expect(listed.projectsProblem).toBe("The projects could not be listed: The project table could not be read.")
})

test("asked for again, the projects come with the branch each is on now, as after a checkout", async () => {
  const { served, connection } = await start()
  await git(["init", "-q", "-b", "main"], served.run.projectFolder)
  const sessions = list(connection)
  await until(sessions, (state) => state.projects[0]?.branch === "main")

  await git(["checkout", "-q", "-b", "feat/beds"], served.run.projectFolder)
  sessions.refreshProjects()

  expect((await until(sessions, (state) => state.projects[0]?.branch !== "main")).projects[0]!.branch).toBe("feat/beds")
})

test("a session is renamed from the list, which shows its new name once its summary comes, and a blank name is refused with the server's reason", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }])
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)

  expect(await sessions.rename(served.run.session, "Garden beds")).toBeUndefined()
  await until(sessions, (state) => state.sessions[0]?.name === "Garden beds")
  expect(await sessions.rename(served.run.session, "   ")).toBe("A session's name cannot be blank. Give it a name of a few words.")
})

test("a session is moved from the list into another project or none, and a move refused while it works says why", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }, { hang: true }])
  await say(served.run, "Plan the beds for spring")
  const sessions = list(connection)
  await until(sessions, (state) => state.listed)
  const projectOf = (state: SessionList, session: string) => state.sessions.find((each) => each.session === session)?.project

  expect(await sessions.move(served.run.session, null)).toBeUndefined()
  await until(sessions, (state) => projectOf(state, served.run.session) === null)
  expect(await sessions.move(served.run.session, served.run.project!.id)).toBeUndefined()
  await until(sessions, (state) => projectOf(state, served.run.session) === served.run.project!.id)

  await served.run.evie.send({ type: "send_message", session: served.run.session, text: "Work on this for a while" })
  await until(sessions, (state) => state.sessions[0]?.state === "working")
  expect(await sessions.move(served.run.session, null)).toBe("This session cannot be moved while a turn is running in it. Wait until it has finished, or stop it, then try again.")
})

test("a session deleted from one page is dropped from every page's list, whatever each has open", async () => {
  const { served, connection } = await start([{ text: ["Noted."] }, { text: ["Noted."] }])
  await say(served.run, "Plan the beds for spring")
  const other = await served.run.evie.send({ type: "open_session" })
  await say(served.run, "What is the weather tomorrow?", other.session)
  const sessions = list(connection)
  // A second page, which has the session open.
  const second = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => second.close())
  const secondList = list(second)
  await until(sessions, (state) => state.listed && state.sessions.length === 2)
  await until(secondList, (state) => state.listed && state.sessions.length === 2)
  const secondClient = second.state()
  if (secondClient.status !== "connected") throw new Error("the second page is not connected")
  await secondClient.evie.send({ type: "open_session", session: served.run.session, questions: true })

  expect(await sessions.delete(served.run.session)).toEqual({ leftOver: [] })

  for (const each of [sessions, secondList]) {
    const after = await until(each, (state) => state.sessions.length === 1)
    expect(after.sessions.map((one) => one.session)).toEqual([other.session])
    // So that a page with it open can say it was deleted.
    expect(after.deleted).toEqual([served.run.session])
  }
})
