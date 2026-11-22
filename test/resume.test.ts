import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { runTerminal, type Commands, type Plugin } from "../src/index.ts"
import { CATCH_UP_PAGE } from "../src/core/types.ts"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
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
// Resolves when a turn of the session ends as turns do, not closed off because Evie stopped.
function turnCompleted(run: { evie: Run["evie"] }, session: string) {
  return new Promise<void>((resolve) => {
    run.evie.subscribe((event) => event.type === "turn_end" && event.session === session && event.body.reason === "completed" && resolve())
  })
}

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

test("a turn cut off when Evie stopped is closed off, so the conversation can carry on, and is carried on when its session opens", async () => {
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
  const carriedOn = turnCompleted(second.run, first.run.session)
  await second.run.evie.send({ type: "open_session", folder: projectFolder, continue: true })
  await carriedOn

  const asked = second.model.requests[0]!.messages
  expect(asked.slice(-3, -1)).toEqual([
    { role: "tool", callId: "call_x", text: "first done" },
    { role: "tool", callId: "call_y", text: "evie stopped before this tool finished, so it has no result." },
  ])
  expect(asked.at(-1)?.text).toStartWith("[Note from Evie: evie's server restarted while you were working, and your turn was cut short.")
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

// A session with replies this large in it, one per turn, as when evie
// writes large files: its log runs to megabytes. Compaction is put off, so
// that the scripted model is asked for nothing but the replies.
async function largeSession(replies: string[]) {
  const model = scriptedProvider(replies.map((text) => ({ text: [text] })))
  const run = await startEvie([model.plugin], { compaction: { ceiling: 1_000_000_000 } })
  cleanups.push(run.stop)
  for (const [at] of replies.entries()) await say(run, `Message ${at + 1}`)
  return run
}

// Catches up on a session through the core the way a client does: from the
// event given, then from the last event of each page, until a page says no
// more follow. Gives back every page.
async function catchUpInPages(run: Run, after: number) {
  const pages: Awaited<ReturnType<Run["evie"]["send"]>>[] = []
  for (let from = after; pages.length < 50; ) {
    const page = await run.evie.send({ type: "open_session", session: run.session, after: from })
    pages.push(page)
    if (!page.more || !page.events?.length) break
    from = page.events.at(-1)!.seq
  }
  return pages as Commands["open_session"]["output"][]
}

const KiB = 1024

test("a catch-up of many megabytes is answered in pages of at most about 1 MB of events, in order, each but the last saying more follow", async () => {
  // Twelve turns, of four events each, whose replies are 400 KiB: two fit in a page, a third does not.
  const run = await largeSession(Array.from({ length: 12 }, (_, at) => String(at % 10).repeat(400 * KiB)))

  const pages = await catchUpInPages(run, 0)

  expect(pages.map((page) => page.events?.filter((event) => event.type === "assistant_message").length)).toEqual([2, 2, 2, 2, 2, 2])
  expect(pages.flatMap((page) => page.events!.map((event) => event.seq))).toEqual(Array.from({ length: 48 }, (_, at) => at + 1))
  expect(pages.map((page) => page.more)).toEqual([true, true, true, true, true, undefined])
  // Each page stands where the session does as it is read.
  for (const page of pages) expect(page).toMatchObject({ session: run.session, lastSeq: 48, turnRunning: false })
})

// Counts the rows evie reads from its database, through every statement
// made from now on, until `stop` is called.
function countRowsRead() {
  let rows = 0
  const { query, prepare } = Database.prototype
  // The same statement is handed out again for the same SQL, and counted once.
  const counted = new WeakSet<object>()
  const counting = (make: Database["query"]) =>
    function (this: Database, ...args: Parameters<Database["query"]>) {
      const statement = make.apply(this, args)
      if (counted.has(statement)) return statement
      counted.add(statement)
      const { all, get, iterate } = statement
      statement.all = (...values) => {
        const found = all.apply(statement, values)
        rows += found.length
        return found
      }
      statement.get = (...values) => {
        const found = get.apply(statement, values)
        if (found !== null) rows += 1
        return found
      }
      statement.iterate = function* (...values) {
        for (const row of iterate.apply(statement, values)) {
          rows += 1
          yield row
        }
      }
      return statement
    } as Database["query"]
  Database.prototype.query = counting(query)
  Database.prototype.prepare = counting(prepare)
  const stop = async () => {
    Database.prototype.query = query
    Database.prototype.prepare = prepare
  }
  cleanups.push(stop)
  return { rows: () => rows, stop }
}

test("each page of a catch-up reads only its own part of the log, not the whole of it", async () => {
  const counter = countRowsRead()
  const run = await largeSession(Array.from({ length: 12 }, (_, at) => String(at % 10).repeat(400 * KiB)))

  const read: { events: number; rows: number }[] = []
  for (let from = 0; ; ) {
    const before = counter.rows()
    const page = await run.evie.send({ type: "open_session", session: run.session, after: from })
    read.push({ events: page.events!.length, rows: counter.rows() - before })
    if (!page.more) break
    from = page.events!.at(-1)!.seq
  }
  await counter.stop()

  // The log holds 48 events; a page reads its own, the one after them, and a few rows besides.
  expect(read.length).toBe(6)
  for (const page of read) expect(page.rows).toBeLessThanOrEqual(page.events + 6)
})

test("everything a page carries counts in its size, and the reply being written comes only with the last page, which the client applies", async () => {
  const replySoFar = "r".repeat(300 * KiB)
  const model = scriptedProvider([...Array.from({ length: 12 }, (_, at) => ({ text: [String(at % 10).repeat(400 * KiB)] })), { hang: true, text: [replySoFar] }])
  const run = await startEvie([model.plugin], { compaction: { ceiling: 1_000_000_000 } })
  cleanups.push(run.stop)
  for (let at = 1; at <= 12; at++) await say(run, `Message ${at}`)
  const writing = new Promise<void>((resolve) => run.evie.subscribe((event) => event.type === "assistant_delta" && resolve()))
  await run.evie.send({ type: "send_message", session: run.session, text: "One more" })
  await writing

  const pages = await catchUpInPages(run, 0)

  for (const page of pages) expect(Buffer.byteLength(JSON.stringify(page))).toBeLessThanOrEqual(CATCH_UP_PAGE)
  expect(pages.flatMap((page) => page.events!.map((event) => event.seq))).toEqual(Array.from({ length: 50 }, (_, at) => at + 1))
  for (const page of pages.slice(0, -1)) expect([page.replySoFar, page.openQuestions]).toEqual([undefined, []])
  expect(pages.at(-1)).toMatchObject({ replySoFar, turnRunning: true, lastSeq: 50 })
  expect(pages.at(-1)!.more).toBeUndefined()
  await run.evie.send({ type: "cancel", session: run.session })
})

test("an event larger than a page goes in a page of its own", async () => {
  const run = await largeSession(["Short.", "x".repeat(1536 * KiB), "Short again."])

  const pages = await catchUpInPages(run, 0)

  // 1 to 4 the first turn, 5 turn_start, 6 user_message, 7 the large reply, 8 turn_end, 9 to 12 the last turn.
  expect(pages.map((page) => page.events?.map((event) => event.seq))).toEqual([[1, 2, 3, 4, 5, 6], [7], [8, 9, 10, 11, 12]])
  expect(pages.map((page) => page.more)).toEqual([true, true, undefined])
})

test("a session opened by its id that is not open, as after Evie was started again, is opened in its own project, with a turn cut off closed first", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain()
  await say(first.run, "hi")
  await first.quit()
  // Evie was stopped in the middle of a tool: a turn that started, a reply
  // asking for a tool, and nothing after.
  const db = new Database(join(home, "evie.db"))
  const add = (seq: number, type: string, body: unknown) =>
    db.run("INSERT INTO event (session, seq, type, body, time) VALUES (?, ?, ?, ?, ?)", [first.run.session, seq, type, JSON.stringify(body), Date.now() + seq])
  add(5, "turn_start", {})
  add(6, "user_message", { text: "run the tool" })
  add(7, "assistant_message", { text: "Running.", toolCalls: [{ id: "call_x", name: "work", arguments: "{}" }] })
  db.close()

  // Started again somewhere else, so nothing ties it to the session's project.
  const second = await startAgain([{ text: ["Carrying on."] }])
  const elsewhere = await second.run.evie.send({ type: "open_session" })
  const carriedOn = turnCompleted(second.run, first.run.session)
  const reopened = await second.run.evie.send({ type: "open_session", session: first.run.session, after: 4 })
  await carriedOn

  expect(elsewhere.project).toBeNull()
  // Closed off, then carried on by itself, which has started.
  expect(reopened).toMatchObject({ session: first.run.session, project: first.run.project, resumed: true, turnRunning: true, lastSeq: 11 })
  expect(reopened.events?.map((event) => [event.seq, event.type])).toEqual([
    [5, "turn_start"],
    [6, "user_message"],
    [7, "assistant_message"],
    [8, "tool_result"],
    [9, "turn_end"],
    [10, "turn_start"],
    [11, "note"],
  ])
  expect(second.model.requests[0]?.messages.at(-1)?.text).toStartWith("[Note from Evie: evie's server restarted while you were working")
})

test("opening by id refuses a subagent's session, saying to open the one that started it, and refuses an id that is no session's", async () => {
  const { home, projectFolder } = await machine()
  // A tool that starts a subagent and gives back the id of its session.
  const delegate: Plugin = {
    name: "delegate",
    setup(host) {
      host.tools.add({ name: "delegate", description: "Starts a subagent.", inputSchema: { type: "object" }, run: (_input, context) => context.children.start({ text: "look around" }) })
    },
  }
  const model = scriptedProvider([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["ok"] }, { text: ["ok"] }])
  const run = await startEvie([model.plugin, delegate], { home, projectFolder })
  cleanups.push(() => run.evie.close())
  await say(run, "hand it over")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body.text

  await expect(run.evie.send({ type: "open_session", session: child })).rejects.toThrow(
    `The session "${child}" is a subagent's. Open the session that started it, "${run.session}", instead.`,
  )
  await expect(run.evie.send({ type: "open_session", session: "no-such-session" })).rejects.toThrow('There is no session with the id "no-such-session".')
})

test("a session's id is given alone, without a folder, a project or `continue`, and a number to catch up from only with a session's id", async () => {
  const { startAgain } = await machine()
  const { run } = await startAgain()
  const alone = "Open a session by its id alone, without a project, a folder or `continue`."

  // The compiler refuses these, and the core refuses them from a client that is not type-checked.
  // @ts-expect-error
  await expect(run.evie.send({ type: "open_session", session: run.session, folder: run.projectFolder })).rejects.toThrow(alone)
  // @ts-expect-error
  await expect(run.evie.send({ type: "open_session", session: run.session, project: run.project!.id })).rejects.toThrow(alone)
  // @ts-expect-error
  await expect(run.evie.send({ type: "open_session", session: run.session, continue: true })).rejects.toThrow(alone)
  // @ts-expect-error
  await expect(run.evie.send({ type: "open_session", folder: run.projectFolder, after: 3 })).rejects.toThrow(
    "`after` is the number of the last event seen in a session, so it is given only with `session`.",
  )
})

test("a session's id may come with `continue: false`, which asks for nothing more, from a client that sends every field", async () => {
  const { startAgain } = await machine()
  const { run } = await startAgain()

  // @ts-expect-error The compiler refuses it, but a client that is not type-checked can send it.
  const opened = await run.evie.send({ type: "open_session", session: run.session, continue: false })

  expect(opened).toMatchObject({ session: run.session, project: run.project, resumed: true })
})

test("a session whose project has gone from the database is refused when opened by its id, not opened as a general session", async () => {
  const { home, startAgain } = await machine()
  const first = await startAgain()
  await say(first.run, "hi")
  await first.quit()
  const db = new Database(join(home, "evie.db"))
  db.run("DELETE FROM project WHERE id = ?", [first.run.project!.id])
  db.close()

  const second = await startAgain()

  await expect(second.run.evie.send({ type: "open_session", session: first.run.session })).rejects.toThrow(
    `The session "${first.run.session}" belongs to the project "${first.run.project!.id}", which is missing from evie's database.`,
  )
})
