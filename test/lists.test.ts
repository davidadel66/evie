import { afterEach, expect, setSystemTime, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdir, mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { Plugin } from "../src/index.ts"
import { git } from "./support/git.ts"
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

async function start(script: ScriptedReply[], plugins: Plugin[] = [], options: { quietCheckEvery?: number } = {}) {
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...plugins], { questions: true, ...options })
  cleanups.push(run.stop)
  return { run, model }
}

// ---- Lists ------------------------------------------------------------------------------------

// The summaries the client was sent.
const summaries = (run: Run) => run.events.flatMap((event) => (event.type === "summary" ? [event] : []))

test("a session is listed with its project, its first message as its name, and when it was last active, and one that is idle and recent is active", async () => {
  const { run } = await start([{ text: ["Noted."] }])
  await say(run, "Keep a list of what to plant")

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed).toEqual([
    { session: run.session, project: run.project!.id, name: "Keep a list of what to plant", state: "active", lastActive: stored(run.events).at(-1)!.time },
  ])
})

test("sessions are listed the latest active first, and a general session has no project", async () => {
  const { run } = await start([{ text: ["ok"] }, { text: ["ok"] }])
  const general = (await run.evie.send({ type: "open_session" })).session
  await say(run, "in the general session", general)
  await Bun.sleep(5)
  await say(run, "in the project")

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed.map(({ session, project, name }) => ({ session, project, name }))).toEqual([
    { session: run.session, project: run.project!.id, name: "in the project" },
    { session: general, project: null, name: "in the general session" },
  ])
})

test("a session the owner has said nothing in is not listed, and no summary of it is sent, until its first message", async () => {
  // The clock is held still, so that the events of a turn are stored at the same moment.
  setSystemTime(new Date(Date.now()))
  const { run } = await start([{ text: ["ok"] }])
  const opened = (await run.evie.send({ type: "open_session", folder: run.projectFolder })).session
  const before = await run.evie.send({ type: "list_sessions" })

  await say(run, "now something", opened)

  // The session the test began with is never said anything in either.
  expect(before).toEqual([])
  expect((await run.evie.send({ type: "list_sessions" })).map((summary) => summary.session)).toEqual([opened])
  expect(summaries(run).map(({ session, name, state }) => ({ session, name, state }))).toEqual([
    { session: opened, name: "now something", state: "working" },
    { session: opened, name: "now something", state: "active" },
  ])
})

test("a session with nothing said in it is listed, with no name, once it has asked a question, such as one asked as it opens, and stays listed once it is answered", async () => {
  // A project that asks whether to trust it whenever a session opens in it, once told to.
  let asks = false
  const run = await startEvie([scriptedProvider([]).plugin], {
    projectSettings: async (_project, { ask }) => {
      if (asks) await ask({ text: "Trust this project?", choices: ["Yes", "No"] })
      return { plugins: [], forNow: true }
    },
  })
  cleanups.push(run.stop)
  asks = true
  const asked = nextEvent(run, "question")
  const answered = nextEvent(run, "answer")
  const opening = run.evie.send({ type: "open_session", folder: run.projectFolder, questions: true })
  const question = await asked

  const whileAsking = await run.evie.send({ type: "list_sessions" })
  await run.evie.send({ type: "answer_question", session: question.session, id: question.body.id, choice: "Yes" })
  const answer = await answered
  await opening

  const summary = { session: question.session, project: run.project!.id, name: null }
  expect(whileAsking).toEqual([{ ...summary, state: "needs_you", lastActive: question.time }])
  expect(await run.evie.send({ type: "list_sessions" })).toEqual([{ ...summary, state: "active", lastActive: answer.time }])
  // So a client keeping a list is told it no longer needs the owner.
  expect(summaries(run)).toEqual([
    { type: "summary", ...summary, state: "needs_you", lastActive: question.time },
    { type: "summary", ...summary, state: "active", lastActive: answer.time },
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

test("every project is listed, by name", async () => {
  const { run } = await start([])
  const folder = await realpath(await mkdtemp(join(tmpdir(), "a-first-project-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  const other = (await run.evie.send({ type: "open_session", folder })).project!

  const listed = await run.evie.send({ type: "list_projects" })

  expect(listed).toEqual([
    { ...other, git: false },
    { ...run.project!, git: false },
  ])
})

test("a project in a git repository is listed with the branch it is on as it is asked for, and one that is not with none", async () => {
  const { run } = await start([])
  const folder = await realpath(await mkdtemp(join(tmpdir(), "a-repository-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  await git(["init", "-q", "-b", "feat/lists"], folder)
  await git(["commit", "-q", "--allow-empty", "-m", "First"], folder)
  const repository = (await run.evie.send({ type: "open_session", folder })).project!

  const first = await run.evie.send({ type: "list_projects" })
  await git(["checkout", "-q", "-b", "fix/rows"], folder)
  const then = await run.evie.send({ type: "list_projects" })

  expect(first).toEqual([
    { ...repository, branch: "feat/lists", git: true },
    { ...run.project!, git: false },
  ])
  expect(first[1]).not.toHaveProperty("branch")
  expect(then[0]).toEqual({ ...repository, branch: "fix/rows", git: true })
})

test("a repository with no commits yet is listed on its branch, one with a commit checked out by itself on none, and a folder that has gone on none", async () => {
  const { run } = await start([])
  const made = async (name: string) => {
    const folder = await realpath(await mkdtemp(join(tmpdir(), name)))
    cleanups.push(() => rm(folder, { recursive: true, force: true }))
    return folder
  }
  const fresh = await made("a-fresh-")
  await git(["init", "-q", "-b", "main"], fresh)
  const detached = await made("b-detached-")
  await git(["init", "-q", "-b", "main"], detached)
  await git(["commit", "-q", "--allow-empty", "-m", "First"], detached)
  await git(["checkout", "-q", "--detach"], detached)
  const gone = await made("c-gone-")
  await git(["init", "-q", "-b", "main"], gone)
  for (const folder of [fresh, detached, gone]) await run.evie.send({ type: "open_session", folder })
  await rm(gone, { recursive: true, force: true })

  const listed = await run.evie.send({ type: "list_projects" })

  expect(listed.map((project) => project.branch)).toEqual(["main", undefined, undefined, undefined])
})

test("a project's branch is its own folder's, whatever git settings the server was started with, as GIT_DIR pointing at another repository", async () => {
  const made = async (name: string, branch: string) => {
    const folder = await realpath(await mkdtemp(join(tmpdir(), name)))
    cleanups.push(() => rm(folder, { recursive: true, force: true }))
    await git(["init", "-q", "-b", branch], folder)
    return folder
  }
  const elsewhere = await made("elsewhere-", "not-this-one")
  const folder = await made("a-repository-", "feat/own")

  // A server started with these set, as from a shell that exported them.
  const projects = join(import.meta.dir, "..", "src", "core", "projects.ts")
  const asked = Bun.spawn(["bun", "-e", `import { branchOf } from ${JSON.stringify(projects)}; console.log(await branchOf(${JSON.stringify(folder)}) ?? "none")`], {
    env: { ...process.env, GIT_DIR: join(elsewhere, ".git"), GIT_WORK_TREE: elsewhere },
    stdout: "pipe",
    stderr: "pipe",
  })
  const [said, problem] = await Promise.all([new Response(asked.stdout).text(), new Response(asked.stderr).text(), asked.exited])

  expect(problem).toBe("")
  expect(said.trim()).toBe("feat/own")
})

test("a project whose folder is inside another repository, below its top, is listed with no branch", async () => {
  const { run } = await start([])
  // The project is made first, and the folder above it becomes a repository afterwards.
  const outer = await realpath(await mkdtemp(join(tmpdir(), "an-outer-")))
  cleanups.push(() => rm(outer, { recursive: true, force: true }))
  const folder = join(outer, "inside")
  await mkdir(folder)
  const inside = (await run.evie.send({ type: "open_session", folder })).project!
  await git(["init", "-q", "-b", "outer-branch"], outer)

  const listed = await run.evie.send({ type: "list_projects" })

  expect(listed.find((project) => project.id === inside.id)).not.toHaveProperty("branch")
})

test("whether a project's folder is in a git work tree is said apart from its branch: at the top, on a commit by itself, or below the top of another repository", async () => {
  const { run } = await start([])
  const made = async (name: string) => {
    const folder = await realpath(await mkdtemp(join(tmpdir(), name)))
    cleanups.push(() => rm(folder, { recursive: true, force: true }))
    return folder
  }
  const top = await made("a-top-")
  await git(["init", "-q", "-b", "main"], top)
  const detached = await made("b-detached-")
  await git(["init", "-q", "-b", "main"], detached)
  await git(["commit", "-q", "--allow-empty", "-m", "First"], detached)
  await git(["checkout", "-q", "--detach"], detached)
  // The folder above this project becomes a repository after the project is made.
  const outer = await made("c-outer-")
  const inside = join(outer, "inside")
  await mkdir(inside)
  const plain = await made("d-plain-")
  for (const folder of [top, detached, inside, plain]) await run.evie.send({ type: "open_session", folder })
  await git(["init", "-q", "-b", "outer-branch"], outer)

  const listed = await run.evie.send({ type: "list_projects" })
  const said = (folder: string) => {
    const project = listed.find((each) => each.folder === folder)!
    return { git: project.git, branch: project.branch }
  }

  expect([top, detached, inside, plain].map(said)).toEqual([
    { git: true, branch: "main" },
    { git: true, branch: undefined },
    { git: true, branch: undefined },
    { git: false, branch: undefined },
  ])
})

// ---- A session's branch -----------------------------------------------------------------------

test("a session is listed with its own branch: the one its project was on as its latest turn started, recorded as a turn starts only when it changed", async () => {
  const { run } = await start([{ text: ["One."] }, { text: ["Two."] }, { text: ["Three."] }])
  await git(["init", "-q", "-b", "feat/permissions"], run.projectFolder)
  await git(["commit", "-q", "--allow-empty", "-m", "First"], run.projectFolder)

  await say(run, "Ask before risky tools")
  const first = (await run.evie.send({ type: "list_sessions" }))[0]
  await say(run, "And the shell")
  await git(["checkout", "-q", "-b", "dev"], run.projectFolder)
  // A checkout between turns is not the session's until a turn starts on it.
  const before = (await run.evie.send({ type: "list_sessions" }))[0]
  await say(run, "Now on dev")
  const after = (await run.evie.send({ type: "list_sessions" }))[0]

  expect([first?.branch, before?.branch, after?.branch]).toEqual(["feat/permissions", "feat/permissions", "dev"])
  const recorded = stored(run.events).flatMap((event) => (event.type === "branch" ? [event.body.branch] : []))
  expect(recorded).toEqual(["feat/permissions", "dev"])
  // Each straight after its turn's start and what the owner said.
  expect(stored(run.events).map((event) => event.type).slice(0, 4)).toEqual(["turn_start", "user_message", "branch", "assistant_message"])
  // The summary that told of the new branch came after the turn's start.
  const told = summaries(run).filter((summary) => summary.branch === "dev")
  expect(told[0]).toMatchObject({ state: "working" })
})

test("two sessions of one project each keep the branch they last worked on", async () => {
  const { run } = await start([{ text: ["On permissions."] }, { text: ["On dev."] }])
  await git(["init", "-q", "-b", "feat/permissions"], run.projectFolder)
  await git(["commit", "-q", "--allow-empty", "-m", "First"], run.projectFolder)
  await say(run, "Ask before risky tools")
  await git(["checkout", "-q", "-b", "dev"], run.projectFolder)
  const other = (await run.evie.send({ type: "open_session", folder: run.projectFolder })).session
  await say(run, "Design the undo checkpoint", other)

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed.map((summary) => [summary.session, summary.branch])).toEqual([
    [other, "dev"],
    [run.session, "feat/permissions"],
  ])
})

test("a session with no git repository, or no project, or on no branch, is listed with no branch", async () => {
  const { run } = await start([{ text: ["Plain."] }, { text: ["General."] }])
  await say(run, "A plain folder")
  const general = (await run.evie.send({ type: "open_session" })).session
  await say(run, "No project at all", general)

  const listed = await run.evie.send({ type: "list_sessions" })

  expect(listed).toHaveLength(2)
  for (const summary of listed) expect(summary).not.toHaveProperty("branch")
  expect(stored(run.events).filter((event) => event.type === "branch")).toEqual([])
})

test("a session moved to another project keeps no branch from the one it left, until a turn starts in the new one", async () => {
  const { run } = await start([{ text: ["Here."] }, { text: ["There."] }])
  await git(["init", "-q", "-b", "feat/permissions"], run.projectFolder)
  await say(run, "Ask before risky tools")
  const folder = await realpath(await mkdtemp(join(tmpdir(), "another-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  // On a branch of the same name, which is the new project's all the same.
  await git(["init", "-q", "-b", "feat/permissions"], folder)
  const other = (await run.evie.send({ type: "open_session", folder })).project!

  await run.evie.send({ type: "move_session", session: run.session, project: other.id })
  const moved = (await run.evie.send({ type: "list_sessions" })).find((summary) => summary.session === run.session)
  await say(run, "Carry on there")
  const then = (await run.evie.send({ type: "list_sessions" })).find((summary) => summary.session === run.session)

  expect(moved).not.toHaveProperty("branch")
  expect(then?.branch).toBe("feat/permissions")
  expect(stored(run.events).filter((event) => event.type === "branch").map((event) => event.seq > stored(run.events).find((each) => each.type === "moved")!.seq)).toEqual([false, true])
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

test("a session brought back by hand, however long it had been quiet, is active for a week from then, as after activity, and then settles by itself", async () => {
  const { run } = await start([{ text: ["ok"] }])
  await say(run, "hi")
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  setSystemTime(new Date(Date.now() + 30 * DAY))

  await run.evie.send({ type: "settle_session", session: run.session, settled: false })
  const back = stored(run.events).at(-1)!.time
  const broughtBack = await stateOf(run)
  setSystemTime(new Date(back + 7 * DAY - 1))
  const nearlyAWeek = await stateOf(run)
  setSystemTime(new Date(back + 7 * DAY))

  expect([broughtBack, nearlyAWeek]).toEqual(["active", "active"])
  expect(await stateOf(run)).toBe("settled")
})

test("anything that happens in a session settled by hand makes it active again", async () => {
  const { run } = await start([{ text: ["ok"] }, { text: ["Back to it."] }])
  await say(run, "hi")
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })

  await say(run, "one more thing")

  expect(await stateOf(run)).toBe("active")
})

test("settling an older session by hand, or bringing it back, does not make it the one --continue carries on", async () => {
  const { run } = await start([{ text: ["ok"] }, { text: ["ok"] }])
  await say(run, "the older one")
  await Bun.sleep(5)
  const newer = (await run.evie.send({ type: "open_session", folder: run.projectFolder })).session
  await say(run, "the newer one", newer)
  await Bun.sleep(5)
  const continued = async () => (await run.evie.send({ type: "open_session", folder: run.projectFolder, continue: true })).session

  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  const afterSettling = await continued()
  await Bun.sleep(5)
  await run.evie.send({ type: "settle_session", session: run.session, settled: false })
  const afterBringingBack = await continued()

  expect([afterSettling, afterBringingBack]).toEqual([newer, newer])
})

test("a session is not settled while a turn or a background job runs in it, since what it does next would make it active again", async () => {
  const work = backgroundWork()
  const { run } = await start([{ hang: true, text: ["Thinking"] }, { text: [], calls: [{ name: "start_work", input: {} }] }, { text: ["Started."] }, { text: ["It finished."] }], [work.plugin])
  const settle = () => run.evie.send({ type: "settle_session", session: run.session, settled: true })
  const refused = "This session is working, or waiting for an answer to a question. Wait until it has finished, then settle it."
  const writing = nextEvent(run, "assistant_delta")
  await run.evie.send({ type: "send_message", session: run.session, text: "think hard" })
  await writing

  await expect(settle()).rejects.toThrow(refused)
  const cancelled = nextEvent(run, "turn_end")
  await run.evie.send({ type: "cancel", session: run.session })
  await cancelled
  await say(run, "start the work")
  await expect(settle()).rejects.toThrow(refused)
  const dealtWith = nextEvent(run, "turn_end")
  work.finish("all done")
  await dealtWith
  await settle()

  expect(stored(run.events).filter((event) => event.type === "settled")).toHaveLength(1)
  expect(await stateOf(run)).toBe("settled")
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

// ---- Summaries --------------------------------------------------------------------------------

test("a session's summary is sent when its first message comes, and whenever its state, name or latest activity changes, and only then", async () => {
  // The clock is held still, so that the events of a turn are stored at the same moment.
  const now = Date.now()
  setSystemTime(new Date(now))
  const { run } = await start([{ text: ["Hel", "lo."] }, { text: ["Back."] }])
  await say(run, "hi")
  // Neither of these changes anything in its summary: bringing back a session that is active, or a settling with nothing after.
  await run.evie.send({ type: "settle_session", session: run.session, settled: false })
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  setSystemTime(new Date(now + 1))
  await say(run, "hello again")

  const summary = { type: "summary" as const, session: run.session, project: run.project!.id, name: "hi" }
  expect(summaries(run)).toEqual([
    { ...summary, state: "working", lastActive: now },
    { ...summary, state: "active", lastActive: now },
    { ...summary, state: "settled", lastActive: now },
    { ...summary, state: "working", lastActive: now + 1 },
    { ...summary, state: "active", lastActive: now + 1 },
  ])
  // Each is sent after the event that changed it.
  expect(run.events.findLastIndex((event) => event.type === "summary")).toBe(run.events.findLastIndex((event) => event.type === "turn_end") + 1)
})

test("the core looks every so often for sessions that a week of quiet has settled, and sends each one's summary once", async () => {
  const { run } = await start([{ text: ["ok"] }], [], { quietCheckEvery: 10 })
  await say(run, "hi")
  const settled = new Promise<void>((resolve) => run.evie.subscribe((event) => event.type === "summary" && event.state === "settled" && resolve()))

  setSystemTime(new Date(stored(run.events).at(-1)!.time + 7 * DAY))
  await settled
  // Long enough for the core to look several times more.
  await Bun.sleep(60)

  expect(summaries(run).filter((summary) => summary.state === "settled")).toEqual([
    { type: "summary", session: run.session, project: run.project!.id, name: "hi", state: "settled", lastActive: stored(run.events).at(-1)!.time },
  ])
})

test("a look for settled sessions that fails, as when the database cannot be read, is no error: the next look tries again", async () => {
  const { run } = await start([{ text: ["ok"] }], [], { quietCheckEvery: 5 })
  await say(run, "hi")
  const db = new Database(join(run.home, "evie.db"))
  cleanups.push(async () => db.close())
  // Another handle on the database takes away a column the summaries are read from, for long enough to be looked at several times.
  db.run("ALTER TABLE session RENAME COLUMN parent TO parent_gone")
  await Bun.sleep(40)
  db.run("ALTER TABLE session RENAME COLUMN parent_gone TO parent")
  const settled = new Promise<void>((resolve) => run.evie.subscribe((event) => event.type === "summary" && event.state === "settled" && resolve()))

  setSystemTime(new Date(stored(run.events).at(-1)!.time + 7 * DAY))

  await settled
})

test("a summary that cannot be worked out is skipped, the turn whose event it would describe goes on, and the next change sends one", async () => {
  const { run } = await start([{ text: ["Read it."] }, { text: ["Still here."] }])
  // Another handle on the database takes away a column the summaries are
  // read from, so that working one out fails, as when the database cannot be read.
  const db = new Database(join(run.home, "evie.db"))
  cleanups.push(async () => db.close())
  db.run("ALTER TABLE session RENAME COLUMN parent TO parent_gone")
  const ended = nextEvent(run, "turn_end")
  await run.evie.send({ type: "send_message", session: run.session, text: "hi" })
  const first = await Promise.race([ended, Bun.sleep(1000).then(() => undefined)])
  const sentMeanwhile = summaries(run)
  db.run("ALTER TABLE session RENAME COLUMN parent_gone TO parent")
  await say(run, "and again")

  expect(first?.body).toEqual({ reason: "completed" })
  expect(sentMeanwhile).toEqual([])
  expect(stored(run.events).flatMap((event) => (event.type === "assistant_message" ? [event.body.text] : []))).toEqual(["Read it.", "Still here."])
  expect(summaries(run).at(-1)).toMatchObject({ session: run.session, name: "hi", state: "active" })
})

test("Evie starts, and lists sessions, on a data folder whose log holds bodies of the wrong shape, which give no name and no choice by hand", async () => {
  const { run } = await start([])
  await run.evie.close()
  // Written straight into the log, as a damaged database could hold them: a
  // first message whose text is not text, then a settling that says neither
  // yes nor no, and in another session a first message that is not JSON at all.
  const now = Date.now()
  const other = crypto.randomUUID()
  const db = new Database(join(run.home, "evie.db"))
  const add = db.query("INSERT INTO event (session, seq, type, body, time) VALUES (?, ?, ?, ?, ?)")
  add.run(run.session, 1, "user_message", JSON.stringify({ text: 42 }), now - 2)
  add.run(run.session, 2, "settled", "null", now - 1)
  db.run("INSERT INTO session (id, project, created) VALUES (?, ?, ?)", [other, run.project!.id, now - 3])
  add.run(other, 1, "user_message", "not JSON", now)
  db.close()

  const again = await startEvie([scriptedProvider([]).plugin], { home: run.home })
  cleanups.push(again.stop)

  expect(await again.evie.send({ type: "list_sessions" })).toEqual([
    { session: other, project: run.project!.id, name: null, state: "active", lastActive: now },
    { session: run.session, project: run.project!.id, name: null, state: "active", lastActive: now - 2 },
  ])
})

// ---- History ------------------------------------------------------------------------------------

test("a session's history is read a page at a time, back from its latest events, each page in order, saying whether there are earlier ones", async () => {
  const { run } = await start([{ text: ["one"] }, { text: ["two"] }])
  await say(run, "first")
  await say(run, "second")
  // 1 turn_start, 2 user_message, 3 assistant_message, 4 turn_end, then 5 to 8 the same again.
  const all = stored(run.events)
  expect(all.map((event) => event.seq)).toEqual([1, 2, 3, 4, 5, 6, 7, 8])

  const latest = await run.evie.send({ type: "read_history", session: run.session, limit: 3 })
  const before = await run.evie.send({ type: "read_history", session: run.session, before: 6, limit: 3 })
  const first = await run.evie.send({ type: "read_history", session: run.session, before: 3, limit: 3 })
  const whole = await run.evie.send({ type: "read_history", session: run.session })

  expect(latest).toEqual({ events: all.slice(5, 8), earlier: true })
  expect(before).toEqual({ events: all.slice(2, 5), earlier: true })
  expect(first).toEqual({ events: all.slice(0, 2), earlier: false })
  expect(whole).toEqual({ events: all, earlier: false })
})

test("a page of history is 50 events unless it asks for another number, from 1 to 500", async () => {
  const { run } = await start([])
  await run.evie.close()
  // A long conversation, written straight into the log.
  const db = new Database(join(run.home, "evie.db"))
  const add = db.query("INSERT INTO event (session, seq, type, body, time) VALUES (?, ?, 'note', ?, ?)")
  db.transaction(() => {
    for (let seq = 1; seq <= 600; seq++) add.run(run.session, seq, JSON.stringify({ text: `note ${seq}` }), seq)
  })()
  db.close()
  const again = await startEvie([scriptedProvider([]).plugin], { home: run.home })
  cleanups.push(again.stop)
  const seqs = async (limit?: number) => {
    const page = await again.evie.send({ type: "read_history", session: run.session, ...(limit !== undefined && { limit }) })
    return [page.events.length, page.events[0]?.seq, page.events.at(-1)?.seq, page.earlier]
  }

  expect(await seqs()).toEqual([50, 551, 600, true])
  expect(await seqs(10)).toEqual([10, 591, 600, true])
  expect(await seqs(500)).toEqual([500, 101, 600, true])
  for (const limit of [0, 501, 2.5]) {
    await expect(again.evie.send({ type: "read_history", session: run.session, limit })).rejects.toThrow("`limit` is how many events to read, a whole number from 1 to 500.")
  }
})

test("the history of a session that is not open can be read, and so can a subagent's", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["Looked."] }, { text: ["Started it."] }], [delegate])
  await say(run, "hand it over")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body.text
  // The subagent's turn ends with its report: its last event is its turn's end.
  while (stored(run.events).filter((event) => event.session === child && event.type === "turn_end").length === 0) await Bun.sleep(1)
  await run.evie.close()

  const again = await startEvie([scriptedProvider([]).plugin], { home: run.home })
  cleanups.push(again.stop)
  const parent = await again.evie.send({ type: "read_history", session: run.session })
  const subagent = await again.evie.send({ type: "read_history", session: child })

  expect(parent.events.map((event) => event.type)).toEqual(stored(run.events).filter((event) => event.session === run.session).map((event) => event.type))
  expect(subagent.events.map((event) => event.type)).toEqual(["model_changed", "turn_start", "user_message", "assistant_message", "turn_end"])
  await expect(again.evie.send({ type: "read_history", session: "no-such-session" })).rejects.toThrow('There is no session with the id "no-such-session".')
})
