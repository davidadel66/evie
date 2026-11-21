import { afterEach, expect, setSystemTime, test } from "bun:test"
import { Database } from "bun:sqlite"
import { join } from "node:path"
import type { Plugin } from "../src/index.ts"
import { nextEvent, say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// Lists and history: what a client is told about every session without
// opening it, and the earlier conversation of one, a page at a time.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  // The clock is put back, for a test that moved it on.
  setSystemTime()
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

const DAY = 24 * 60 * 60_000

// A tool that starts a subagent and gives back the id of its session.
const delegate: Plugin = {
  name: "delegate",
  setup(host) {
    host.tools.add({ name: "delegate", description: "Starts a subagent.", inputSchema: { type: "object" }, run: (_input, context) => context.children.start({ text: "look around" }) })
  },
}

// A deploy tool that asks the owner before it runs.
const asksBeforeDeploy: Plugin = {
  name: "asks-before-deploy",
  setup(host) {
    host.tools.add({ name: "deploy", description: "Deploys.", inputSchema: { type: "object" }, run: async () => "deployed" })
    host.on("before_tool", async (_call, context) => {
      if ((await context.ask({ text: "Deploy to production?", choices: ["Yes", "No"] })) !== "Yes") return { deny: "The owner did not allow it." }
    })
  },
}

// A tool that hands work to the background, and a way to finish it.
function backgroundWork() {
  let finish = (_result: string) => {}
  const plugin: Plugin = {
    name: "background-work",
    setup(host) {
      host.tools.add({
        name: "start_work",
        description: "Starts some work in the background.",
        inputSchema: { type: "object" },
        run: async (_input, context) => {
          const done = new Promise<string>((resolve) => (finish = resolve))
          return `Started as job ${context.jobs.start({ kind: "work", title: "some work", done, stop: () => finish("It was stopped.") }).id}.`
        },
      })
    },
  }
  return { plugin, finish: (result: string) => finish(result) }
}

async function start(script: ScriptedReply[], plugins: Plugin[] = []) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins], { questions: true })
  cleanups.push(run.stop)
  return { run, model }
}

// ---- Lists ------------------------------------------------------------------------------------

test("a session is listed with its project, its first message as its name, and when it was last active, and one that is idle and recent is active", async () => {
  const { run } = await start([{ text: ["Noted."] }])
  await say(run, "Keep a list of what to plant")

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed).toEqual([
    { session: run.session, project: run.project!.id, name: "Keep a list of what to plant", state: "active", lastActive: stored(run.events).at(-1)!.time },
  ])
})

test("sessions are listed the latest active first, one with no message yet has no name, and a general session has no project", async () => {
  const { run } = await start([{ text: ["ok"] }, { text: ["ok"] }])
  const general = (await run.evie.send({ type: "open_session" })).session
  await say(run, "in the general session", general)
  await Bun.sleep(5)
  await say(run, "in the project")
  await Bun.sleep(5)
  const empty = (await run.evie.send({ type: "open_session", folder: run.projectFolder })).session

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed.map(({ session, project, name }) => ({ session, project, name }))).toEqual([
    { session: empty, project: run.project!.id, name: null },
    { session: run.session, project: run.project!.id, name: "in the project" },
    { session: general, project: null, name: "in the general session" },
  ])
})

test("a session's name is the opening words of its first message, cut at a word's end when its first line is long", async () => {
  const { run } = await start([{ text: ["ok"] }, { text: ["ok"] }])
  await say(run, "Plan the garden for spring, with beds for tomatoes and beans, and a path down the middle\nand more")
  await say(run, "A later message does not rename it")

  const [listed] = await run.evie.send({ type: "list_sessions" })

  expect(listed?.name).toBe("Plan the garden for spring, with beds for tomatoes and…")
})

test("a subagent's session is not listed: it is part of the session that started it", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["ok"] }, { text: ["Started it."] }], [delegate])
  await say(run, "hand it over")

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed.map((summary) => summary.session)).toEqual([run.session])
})

// ---- A session's state ------------------------------------------------------------------------

const stateOf = async (run: Run, session = run.session) => (await run.evie.send({ type: "list_sessions" })).find((summary) => summary.session === session)?.state

test("a session needs you while a question in it waits for an answer, and is active again once its turn has ended", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "deploy", input: {} }] }, { text: ["Done."] }], [asksBeforeDeploy])
  const asked = nextEvent(run, "question")
  await run.evie.send({ type: "send_message", session: run.session, text: "ship it" })
  const question = await asked

  const waiting = await stateOf(run)
  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "answer_question", session: run.session, id: question.body.id, choice: "Yes" })
  await ended

  expect(waiting).toBe("needs_you")
  expect(await stateOf(run)).toBe("active")
})

test("a session is working while a turn runs in it, and active once the turn has ended", async () => {
  const { run } = await start([{ hang: true, text: ["Thinking"] }])
  const writing = nextEvent(run, "assistant_delta")
  await run.evie.send({ type: "send_message", session: run.session, text: "think hard" })
  await writing

  const during = await stateOf(run)
  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "cancel", session: run.session })
  await ended

  expect(during).toBe("working")
  expect(await stateOf(run)).toBe("active")
})

test("a session whose turn was cut off when Evie stopped is not working once Evie is started again, since nothing runs in it", async () => {
  const { run } = await start([{ text: ["ok"] }])
  await say(run, "hi")
  await run.evie.close()
  // What the log holds when Evie is killed in the middle of a turn: one that started and never ended.
  const db = new Database(join(run.home, "evie.db"))
  db.run("INSERT INTO event (session, seq, type, body, time) VALUES (?, 5, 'turn_start', '{}', ?)", [run.session, Date.now()])
  db.close()

  const again = await startEvie([scriptedProvider([]).plugin], { home: run.home })
  cleanups.push(again.stop)

  expect(await stateOf(again, run.session)).toBe("active")
})

test("a session is working while a background job of its own runs, though no turn does", async () => {
  const work = backgroundWork()
  const { run } = await start([{ text: [], calls: [{ name: "start_work", input: {} }] }, { text: ["Started."] }, { text: ["It finished."] }], [work.plugin])
  await say(run, "start the work")

  const during = await stateOf(run)
  const dealtWith = nextEvent(run, "turn_end")
  work.finish("all done")
  await dealtWith

  expect(during).toBe("working")
  expect(await stateOf(run)).toBe("active")
})

test("a session settles once a week has passed with no activity in it", async () => {
  const { run } = await start([{ text: ["ok"] }])
  await say(run, "hi")
  const last = stored(run.events).at(-1)!.time

  setSystemTime(new Date(last + 7 * DAY - 1))
  const nearlyAWeek = await stateOf(run)
  setSystemTime(new Date(last + 7 * DAY))

  expect(nearlyAWeek).toBe("active")
  expect(await stateOf(run)).toBe("settled")
})

test("the owner can settle a session by hand, which is not activity in it, and is told of it as an event of the session", async () => {
  const { run } = await start([{ text: ["ok"] }])
  await say(run, "hi")
  const before = (await run.evie.send({ type: "list_sessions" }))[0]!

  await run.evie.send({ type: "settle_session", session: run.session, settled: true })

  expect(await run.evie.send({ type: "list_sessions" })).toEqual([{ ...before, state: "settled" }])
  expect(stored(run.events).at(-1)).toMatchObject({ type: "settled", session: run.session, body: { settled: true } })
})

test("a session brought back by hand is active, however long it has been quiet, until it is settled again", async () => {
  const { run } = await start([{ text: ["ok"] }])
  await say(run, "hi")
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  setSystemTime(new Date(Date.now() + 30 * DAY))

  await run.evie.send({ type: "settle_session", session: run.session, settled: false })
  const broughtBack = await stateOf(run)
  setSystemTime(new Date(Date.now() + 30 * DAY))
  const aMonthOn = await stateOf(run)
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })

  expect([broughtBack, aMonthOn]).toEqual(["active", "active"])
  expect(await stateOf(run)).toBe("settled")
})

test("anything that happens in a session settled by hand makes it active again", async () => {
  const { run } = await start([{ text: ["ok"] }, { text: ["Back to it."] }])
  await say(run, "hi")
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })

  await say(run, "one more thing")

  expect(await stateOf(run)).toBe("active")
})

test("only a session that is there, and not a subagent's, can be settled", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["ok"] }, { text: ["Started it."] }], [delegate])
  await say(run, "hand it over")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body.text

  await expect(run.evie.send({ type: "settle_session", session: "no-such-session", settled: true })).rejects.toThrow('There is no session with the id "no-such-session".')
  await expect(run.evie.send({ type: "settle_session", session: child, settled: true })).rejects.toThrow(
    `The session "${child}" is a subagent's, which is not listed. Settle the session that started it, "${run.session}", instead.`,
  )
})
