import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { runTerminal } from "../src/index.ts"
import { say, startEvie } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// One data folder and one project folder, with Evie started in them as many
// times as a test needs. Each start stands for running `evie` again.
async function machine() {
  const home = await tempFolder("evie-home-")
  const projectFolder = await tempFolder("evie-project-")
  async function startAgain(script: ScriptedReply[] = [{ text: ["ok"] }, { text: ["ok"] }]) {
    const model = scriptedProvider(script)
    const run = await startEvie([model.plugin], { home, projectFolder })
    let closed = false
    const quit = async () => {
      if (!closed) await run.evie.close()
      closed = true
    }
    cleanups.push(quit)
    return { run, model, quit }
  }
  return { home, projectFolder, startAgain }
}

test("continuing reopens the last session of the project, and the model sees the earlier conversation", async () => {
  const { projectFolder, startAgain } = await machine()
  const first = await startAgain([{ text: ["Paris."] }])
  await say(first.run, "What is the capital of France?")
  await first.quit()

  const second = await startAgain([{ text: ["About two million."] }])
  const reopened = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await say(second.run, "How many people live there?", reopened.session)

  expect(reopened).toMatchObject({ session: first.run.session, resumed: true })
  expect(second.model.requests[0]?.messages).toEqual([
    { role: "user", text: "What is the capital of France?" },
    { role: "assistant", text: "Paris." },
    { role: "user", text: "How many people live there?" },
  ])
})

test("without continuing, a new session starts with an empty conversation", async () => {
  const { projectFolder, startAgain } = await machine()
  const first = await startAgain()
  await say(first.run, "remember this")
  await first.quit()

  const second = await startAgain()
  const fresh = await second.run.evie.send({ type: "open_session", folder: projectFolder })
  await say(second.run, "hello", fresh.session)

  expect(fresh.session).not.toBe(first.run.session)
  expect(fresh.resumed).toBe(false)
  expect(second.model.requests[0]?.messages).toEqual([{ role: "user", text: "hello" }])
})

test("the session continued is the one that was used last, not the one that was opened last", async () => {
  const { projectFolder, startAgain } = await machine()
  const first = await startAgain([{ text: ["a"] }, { text: ["b"] }, { text: ["c"] }])
  const older = first.run.session
  const newer = (await first.run.evie.send({ type: "open_session", folder: projectFolder })).session
  await say(first.run, "in the newer one", newer)
  await new Promise((resolve) => setTimeout(resolve, 5))
  await say(first.run, "back in the older one", older)
  await first.quit()

  const second = await startAgain()
  const reopened = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })

  expect(reopened.session).toBe(older)
})

test("continuing in a project with no earlier conversation starts a new session", async () => {
  const { startAgain } = await machine()
  const { run } = await startAgain()
  const elsewhere = await tempFolder("evie-untouched-project-")

  const opened = await run.evie.send({ type: "open_session", folder: elsewhere, continue: true })

  expect(opened.resumed).toBe(false)
  expect(opened.project?.folder).toBe(elsewhere)
})

test("the general session can be continued too, and is kept apart from every project", async () => {
  const { startAgain } = await machine()
  const first = await startAgain([{ text: ["noted"] }, { text: ["in the project"] }])
  const general = await first.run.evie.send({ type: "open_session" })
  await say(first.run, "a general note", general.session)
  await new Promise((resolve) => setTimeout(resolve, 5))
  await say(first.run, "project work")
  await first.quit()

  const second = await startAgain()
  const reopened = await second.run.evie.send({ type: "open_session", continue: true })

  expect(reopened).toMatchObject({ session: general.session, project: null, resumed: true })
})

test("a session that is already open is handed back, not opened twice", async () => {
  const { projectFolder, startAgain } = await machine()
  const { run } = await startAgain()
  await say(run, "hi")

  const again = await run.evie.send({ type: "open_session", folder: projectFolder, continue: true })

  expect(again).toMatchObject({ session: run.session, resumed: true })
})

test("the model a session was switched to is still in use after Evie is started again", async () => {
  const { projectFolder, startAgain } = await machine()
  const first = await startAgain()
  await say(first.run, "hi")
  await first.run.evie.send({ type: "set_model", session: first.run.session, model: "other/model" })
  await first.quit()

  const second = await startAgain()
  const reopened = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await say(second.run, "still there?", reopened.session)

  expect(second.model.requests[0]?.model).toBe("other/model")
})

test("a turn cut off when Evie stopped is closed off, so the conversation can carry on", async () => {
  const { home, projectFolder, startAgain } = await machine()
  const first = await startAgain()
  await say(first.run, "hi")
  await first.quit()
  // What the log holds when Evie is killed in the middle of running a tool:
  // a turn that started, a reply asking for two tools, and nothing after.
  const db = new Database(join(home, "evie.db"))
  const add = (seq: number, type: string, body: unknown) =>
    db.run("INSERT INTO event (session, seq, type, body, time) VALUES (?, ?, ?, ?, ?)", [first.run.session, seq, type, JSON.stringify(body), Date.now() + seq])
  add(5, "turn_start", {})
  add(6, "user_message", { text: "run the tools" })
  add(7, "assistant_message", {
    text: "Running.",
    toolCalls: [
      { id: "call_x", name: "work", arguments: "{}" },
      { id: "call_y", name: "work", arguments: "{}" },
    ],
  })
  add(8, "tool_result", { callId: "call_x", name: "work", text: "first done" })
  db.close()

  const second = await startAgain([{ text: ["Carrying on."] }])
  const reopened = await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await say(second.run, "what happened?", reopened.session)

  const asked = second.model.requests[0]!.messages
  expect(asked.slice(-3)).toEqual([
    { role: "tool", callId: "call_x", text: "first done" },
    { role: "tool", callId: "call_y", text: "Evie stopped before this tool finished, so it has no result." },
    { role: "user", text: "what happened?" },
  ])
})

test("a message that was waiting when Evie stopped is answered when its session is continued", async () => {
  const { home, projectFolder, startAgain } = await machine()
  const first = await startAgain()
  await say(first.run, "hi")
  await first.quit()
  const db = new Database(join(home, "evie.db"))
  db.run("INSERT INTO inbox (session, body, time) VALUES (?, ?, ?)", [first.run.session, JSON.stringify({ text: "left waiting" }), Date.now()])
  db.close()

  const second = await startAgain([{ text: ["Here now."] }])
  const answered = new Promise<void>((resolve) => {
    second.run.evie.subscribe((event) => {
      if (event.type === "turn_end" && event.session === first.run.session) resolve()
    })
  })
  await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await answered

  expect(second.model.requests[0]?.messages.at(-1)).toEqual({ role: "user", text: "left waiting" })
})

test("the terminal continues the last session when asked to, and says so", async () => {
  const { projectFolder, startAgain } = await machine()
  const first = await startAgain([{ text: ["Paris."] }])
  await say(first.run, "What is the capital of France?")
  await first.quit()

  const second = await startAgain([{ text: ["Yes."] }])
  let printed = ""
  await runTerminal({
    evie: second.run.evie,
    folder: projectFolder,
    resume: true,
    lines: (async function* () {
      yield "Are you sure?"
    })(),
    write: (text) => (printed += text),
  })

  expect(printed).toContain("continuing the last session")
  expect(second.model.requests[0]?.messages.map((message) => message.role)).toEqual(["user", "assistant", "user"])
})

// ---- Opening a session by its id ---------------------------------------------------------
// How a client catches up after its connection dropped: it names the session
// and the last event it saw, and is given what came after.

test("a session opened by its id is joined as it is, with the events after the number given and the reply being written", async () => {
  const { startAgain } = await machine()
  const { run } = await startAgain([{ text: ["Paris."] }, { hang: true, text: ["Madr", "id is"] }])
  await say(run, "What is the capital of France?")
  const writing = new Promise<void>((resolve) => run.evie.subscribe((event) => event.type === "assistant_delta" && event.text === "id is" && resolve()))
  await run.evie.send({ type: "send_message", session: run.session, text: "And of Spain?" })
  await writing

  // 1 turn_start, 2 user_message, 3 assistant_message "Paris.", then the rest.
  const joined = await run.evie.send({ type: "open_session", session: run.session, after: 3 })

  expect(joined).toMatchObject({ session: run.session, project: run.project, resumed: true, turnRunning: true, replySoFar: "Madrid is", lastSeq: 6 })
  expect(joined.events?.map((event) => [event.seq, event.type])).toEqual([
    [4, "turn_end"],
    [5, "turn_start"],
    [6, "user_message"],
  ])
})
