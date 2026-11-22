import { afterEach, expect, test } from "bun:test"
import { appendFile, mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, jobs, startServer, subagents, undo as undoPlugin, type Chunk, type Command, type Evie, type Job } from "../src/index.ts"
import type { GitStatus } from "../src/core/types.ts"
import { followGitStatus, followOutput, followPullRequest, followSubagents, openFolder, request, stopJob, undo, type FollowedGitStatus, type Following, type Lineage } from "../web/src/card.ts"
import { openConnection, type Connection } from "../web/src/connection.ts"
import { holdConversation, type Conversation, type ConversationState } from "../web/src/conversation.ts"
import { git } from "./support/git.ts"
import { say } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"
import type { Plugin } from "../src/index.ts"
import { REQUESTS, subagentsOf } from "../web/src/project-card.ts"
import { openSubagent } from "../web/src/subagent.ts"
import { transcriptOf } from "../web/src/transcript.ts"

// The project card's state, apart from its screen: the session it shows,
// shared with the middle column; its subagents, each opened read only; its
// background jobs' output as it grows, and Stop; undo; and the git
// buttons' requests. These tests connect with a token, as the
// connection's own tests do.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// A scripted model, or with `script` null, the model the plugins bring.
// With `refuse`, the connection is to a server in front of the core whose
// commands pass through it, which can make one fail, as a server with a
// fault would.
async function start(script: ScriptedReply[] | null, plugins: Plugin[] = [], refuse?: (command: Command) => boolean) {
  const model = scriptedProvider(script ?? [])
  const served = await serveEvie([...(script === null ? [] : [model.plugin]), ...plugins])
  cleanups.push(served.stop)
  let url = served.server.url
  if (refuse) {
    const core = served.run.evie
    const evie: Evie = {
      send: (command) => (refuse(command) ? Promise.reject(new Error("The disk is busy.")) : core.send(command)) as never,
      subscribe: (listener) => core.subscribe(listener),
      close: async () => {},
    }
    const server = startServer({ evie, tokens: served.tokens, host: "127.0.0.1", port: 0 })
    cleanups.push(() => server.stop())
    url = server.url
  }
  const connection = openConnection({ url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  return { served, connection, model }
}

// Holds the conversation of a session, as the card and the middle column do.
function hold(connection: Connection, session: string) {
  const held = holdConversation(connection, session)
  cleanups.push(async () => held.release())
  return held
}

async function until(conversation: Conversation, ready: (state: ConversationState) => boolean) {
  while (!ready(conversation.state())) await Bun.sleep(5)
  return conversation.state()
}

test("the card and the middle column share one conversation of a session, which follows it while either holds it", async () => {
  const { served, connection } = await start([{ text: ["Hello."] }])
  const card = holdConversation(connection, served.run.session)
  const middle = holdConversation(connection, served.run.session)
  expect(middle.conversation).toBe(card.conversation)
  await until(card.conversation, (state) => state.opened)

  middle.release()
  await say(served.run, "Hi")
  const followed = await until(card.conversation, (state) => state.events.length === 4)
  expect(followed.events.map((event) => event.type)).toEqual(["turn_start", "user_message", "assistant_message", "turn_end"])

  card.release()
  const again = hold(connection, served.run.session)
  expect(again.conversation).not.toBe(card.conversation)
})

// A folder of the test's own, for a plugin that keeps state in one.
async function folder() {
  const made = await mkdtemp(join(tmpdir(), "evie-card-"))
  cleanups.push(() => rm(made, { recursive: true, force: true }))
  return made
}

// What the conversation says, in order, as the page draws it.
const said = (state: ConversationState) => transcriptOf(state).flatMap((item) => (item.kind === "fold" ? item.items : [item])).map((item) => ("text" in item ? item.text : "label" in item ? item.label : item.kind))

test("undo runs /undo, and what it says is a quiet note in the conversation, as the terminal shows it", async () => {
  const { served, connection } = await start(
    [{ text: [], calls: [{ name: "write", input: { path: "notes.md", content: "one\n" } }] }, { text: ["Written."] }],
    [files(), undoPlugin({ dataFolder: await folder() })],
  )
  await say(served.run, "Write the notes")
  const { conversation } = hold(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  await undo(conversation)
  const undone = await until(conversation, (state) => said(state).at(-1)?.startsWith("Removed") === true)
  expect(said(undone).slice(-2)).toEqual([
    "Note to evie: The owner undid the file changes of an earlier turn. Removed, because the turn created it: notes.md. These files no longer hold what you wrote, so read a file again before you change it.",
    "Removed, because the turn created it: notes.md.",
  ])
  expect(await Bun.file(join(served.run.projectFolder, "notes.md")).exists()).toBe(false)
})

// A tool that hands work to the background, whose output the test writes
// and whose end the test decides, so nothing here depends on timing.
function backgroundWork() {
  const started: { job: Job; stopped: boolean }[] = []
  const plugin: Plugin = {
    name: "background-work",
    setup(host) {
      host.tools.add({
        name: "start_work",
        description: "Starts some work in the background.",
        inputSchema: { type: "object" },
        run: async (_input, context) => {
          let finish = (_result: string) => {}
          const done = new Promise<string>((resolve) => (finish = resolve))
          const entry = { job: undefined as unknown as Job, stopped: false }
          entry.job = context.jobs.start({ kind: "work", title: "the build", done, stop: () => ((entry.stopped = true), finish("Stopped.")) })
          started.push(entry)
          return `Started as job ${entry.job.id}.`
        },
      })
    },
  }
  return { plugin, started }
}

async function untilOutput(following: Following, ready: (text: string) => boolean) {
  while (!ready(following.state().output?.text ?? "")) await Bun.sleep(5)
  return following.state()
}

test("a job's output is read as it grows while it runs, and Stop stops it, which the job's own lines in the conversation say", async () => {
  const work = backgroundWork()
  const { served, connection } = await start([{ text: [], calls: [{ name: "start_work", input: {} }] }, { text: ["Started."] }], [work.plugin, jobs()])
  await say(served.run, "Build it")
  const { conversation } = hold(connection, served.run.session)
  await until(conversation, (state) => state.opened && state.jobs.length === 1)
  const file = work.started[0]!.job.outputFile
  await appendFile(file, "compiling\n")

  const running = () => conversation.state().jobs[0]?.status === "running"
  const following = followOutput(connection, served.run.session, "1", { running, every: 10 })
  cleanups.push(async () => following.close())
  expect((await untilOutput(following, (text) => text !== "")).output).toEqual({ text: "compiling\n", to: 10, cut: false })
  await appendFile(file, "linking\n")
  await untilOutput(following, (text) => text === "compiling\nlinking\n")

  expect(await stopJob(connection, served.run.session, "1")).toBeUndefined()
  const stopped = await until(conversation, (state) => state.jobs[0]?.status === "stopped")
  expect(work.started[0]!.stopped).toBe(true)
  const told = await until(conversation, (state) => said(state).some((line) => line.startsWith("Note to evie: Background job 1")))
  // The job's end and the note to evie can fall in the same millisecond, in either order.
  const lines = said(told).filter((line) => line.includes("job 1"))
  expect(lines).toHaveLength(3)
  expect(lines).toEqual(
    expect.arrayContaining([
      "Background job 1 started: the build",
      expect.stringMatching(/^Background job 1 stopped after \d+s: the build$/),
      "Note to evie: Background job 1 (the build) was stopped by the owner.",
    ]),
  )
  expect(stopped.notes).toEqual([])
})

test("a read of a job's output that fails keeps what was read, and is tried again, waiting longer each time, while the job runs", async () => {
  const work = backgroundWork()
  let failing = 0
  const { served, connection } = await start([{ text: [], calls: [{ name: "start_work", input: {} }] }, { text: ["Started."] }], [work.plugin, jobs()], (command) => {
    if (command.type !== "read_job_output" || failing === 0) return false
    failing -= 1
    return true
  })
  await say(served.run, "Build it")
  const { conversation } = hold(connection, served.run.session)
  await until(conversation, (state) => state.opened && state.jobs.length === 1)
  const file = work.started[0]!.job.outputFile
  await appendFile(file, "compiling\n")
  const running = () => conversation.state().jobs[0]?.status === "running"
  const following = followOutput(connection, served.run.session, "1", { running, every: 10 })
  cleanups.push(async () => following.close())
  await untilOutput(following, (text) => text === "compiling\n")

  failing = 3
  while (following.state().problem === undefined) await Bun.sleep(5)
  expect(following.state()).toEqual({ output: { text: "compiling\n", to: 10, cut: false }, problem: "The output could not be read: The disk is busy." })
  await appendFile(file, "linking\n")
  const caughtUp = await untilOutput(following, (text) => text === "compiling\nlinking\n")
  expect([caughtUp.problem, failing]).toEqual([undefined, 0])

  // A file that has gone is a failure too, which keeps what was read.
  await rm(file)
  while (following.state().problem === undefined) await Bun.sleep(5)
  expect(following.state().output?.text).toBe("compiling\nlinking\n")
  expect(following.state().problem).toStartWith("The output could not be read: The output of job 1 could not be read:")
})

test("a git button sends evie its request as the owner's message, shown at once, which starts a turn as typing it would", async () => {
  const { served, connection, model } = await start([{ text: ["Committed as 1a2b3c: Add the notes."] }])
  const { conversation } = hold(connection, served.run.session)
  await until(conversation, (state) => state.opened)

  const sent = request(conversation, "commit")
  expect(conversation.state().pending).toEqual([{ text: REQUESTS.commit }])
  expect(await sent).toBeUndefined()
  const answered = await until(conversation, (state) => state.events.some((event) => event.type === "turn_end"))
  expect(said(answered)).toEqual(["Commit the changes in this session with a short message.", "Committed as 1a2b3c: Add the notes."])
  expect(model.requests[0]?.messages.at(-1)).toEqual({ role: "user", text: "Commit the changes in this session with a short message." })
})

// A stand-in for the model with one script for the main agent and one for
// its subagents, whose requests say what they are, and a tool the subagent
// waits in until the test lets it go.
function withSubagent(parent: { text?: string; call?: { name: string; input: Record<string, unknown> } }[], child: typeof parent) {
  let release = () => {}
  const released = new Promise<void>((resolve) => (release = resolve))
  const counts = { parent: 0, child: 0 }
  const plugin: Plugin = {
    name: "with-subagent",
    setup(host) {
      host.providers.add({
        name: "with-subagent",
        async *stream(request): AsyncGenerator<Chunk> {
          const who = request.system?.includes("You are a subagent") ? "child" : "parent"
          const reply = (who === "child" ? child : parent)[counts[who]++]
          if (!reply) throw new Error(`the ${who} script has no reply for this request`)
          if (reply.text) yield { type: "text", text: reply.text }
          if (reply.call) yield { type: "tool_call", id: `${who}-${counts[who]}`, name: reply.call.name, arguments: JSON.stringify(reply.call.input) }
          yield { type: "finish", reason: reply.call ? "tool_calls" : "stop" }
        },
      })
      host.tools.add({ name: "wait", description: "Waits.", inputSchema: { type: "object" }, run: async () => (await released, "waited") })
    },
  }
  return { plugin, release }
}

// Settles once the session's stored subagents are listed as `ready` says.
async function untilListed(lineage: Lineage, ready: (listed: ReturnType<Lineage["state"]>["listed"]) => boolean) {
  while (!ready(lineage.state().listed)) await Bun.sleep(5)
  return lineage.state().listed
}

test("a session's subagents are known from those stored and from its jobs, and one opened reads its conversation, and follows it while it works", async () => {
  const model = withSubagent(
    [{ call: { name: "subagent", input: { description: "find the tests", task: "Find where the tests are." } } }, { text: "A subagent is looking." }, { text: "They are in test/." }],
    [{ call: { name: "wait", input: {} } }, { text: "They are in test/, one file a module." }],
  )
  const { served, connection } = await start(null, [model.plugin, subagents({ dataFolder: await folder() }), jobs()])
  await say(served.run, "Where are the tests?")
  const { conversation } = hold(connection, served.run.session)
  const parent = await until(conversation, (state) => state.opened && state.jobs.length === 1)
  const lineage = followSubagents(connection, served.run.session)
  cleanups.push(async () => lineage.close())
  const listed = await untilListed(lineage, (each) => each.length === 1)
  expect(listed).toEqual([{ session: parent.jobs[0]!.subagent!, name: "Find where the tests are.", started: expect.any(Number) }])
  const [found] = subagentsOf(listed, parent.jobs)
  expect(found).toMatchObject({ name: "find the tests", status: "running", job: "1" })

  const working = () => subagentsOf(listed, conversation.state().jobs)[0]?.status === "running"
  const child = openSubagent(connection, found!.session, { working, every: 10 })
  cleanups.push(async () => child.close())
  const reading = await until(child, (state) => state.opened && state.events.length >= 4)
  expect(said(reading)).toEqual(["Find where the tests are.", "Working"])
  expect(reading.turnRunning).toBe(true)

  model.release()
  const done = await until(child, (state) => !state.turnRunning)
  expect(said(done)).toEqual(["Find where the tests are.", "Used wait", "They are in test/, one file a module."])
  await until(conversation, (state) => state.jobs[0]?.status === "finished")
})

test("a subagent's conversation that could not read a page before carries on following it once that page is read again", async () => {
  const model = withSubagent(
    [{ call: { name: "subagent", input: { description: "find the tests", task: "Find where the tests are." } } }, { text: "A subagent is looking." }, { text: "They are in test/." }],
    [{ text: "I'll look.", call: { name: "wait", input: {} } }, { text: "They are in test/, one file a module." }],
  )
  let refusing = false
  const { served, connection } = await start(null, [model.plugin, subagents({ dataFolder: await folder() }), jobs()], (command) => refusing && command.type === "read_history" && command.before !== undefined)
  await say(served.run, "Where are the tests?")
  const { conversation } = hold(connection, served.run.session)
  const parent = await until(conversation, (state) => state.opened && state.jobs.length === 1)
  const working = () => conversation.state().jobs[0]?.status === "running"
  const child = openSubagent(connection, parent.jobs[0]!.subagent!, { working, every: 10, page: 2 })
  cleanups.push(async () => child.close())
  await until(child, (state) => state.opened && state.earlier)

  refusing = true
  child.readEarlier()
  await until(child, (state) => state.problem !== undefined)
  model.release()
  await until(conversation, (state) => state.jobs[0]?.status === "finished")

  refusing = false
  child.tryAgain()
  const done = await until(child, (state) => said(state).at(-1) === "They are in test/, one file a module.")
  expect(done.problem).toBeUndefined()
  expect(done.turnRunning).toBe(false)
})

async function untilStatus(following: FollowedGitStatus, ready: (status: GitStatus | null | undefined) => boolean) {
  while (!ready(following.state())) await Bun.sleep(5)
  return following.state()
}

test("how the project's branch stands is read once connected, again when asked, as when a turn ends, and when the window has the focus back, against the base the card knows", async () => {
  const { served, connection } = await start([])
  const folder = served.run.projectFolder
  await git(["init", "-q", "-b", "main"], folder)
  await writeFile(join(folder, "a.md"), "one\n")
  await git(["add", "-A"], folder)
  await git(["commit", "-q", "-m", "Start"], folder)
  const page = new EventTarget()
  let base: string | undefined
  const following = followGitStatus(connection, served.run.project!.id, { window: page, shown: () => true, base: () => base })
  cleanups.push(async () => following.close())

  expect(await untilStatus(following, (status) => status !== undefined)).toMatchObject({ branch: "main", uncommitted: false, changes: { against: null } })
  await writeFile(join(folder, "a.md"), "two\n")
  following.again()
  expect(await untilStatus(following, (status) => status?.uncommitted === true)).toMatchObject({ changes: { against: null, added: 1, removed: 1 } })

  await git(["branch", "dev"], folder)
  await git(["checkout", "-q", "-b", "feat/breads"], folder)
  page.dispatchEvent(new Event("focus"))
  expect(await untilStatus(following, (status) => status?.branch === "feat/breads")).toMatchObject({ changes: { against: "main" } })
  base = "dev"
  following.again()
  await untilStatus(following, (status) => status?.changes.against === "dev")

  // Followed again, as when the card is drawn again in another place, it starts from what was last read.
  const again = followGitStatus(connection, served.run.project!.id, { window: page, shown: () => true, base: () => base })
  cleanups.push(async () => again.close())
  expect(again.state()).toMatchObject({ branch: "feat/breads", changes: { against: "dev" } })
})

test("a project whose folder is not in git has no git status", async () => {
  const { served, connection } = await start([])
  const following = followGitStatus(connection, served.run.project!.id, { window: new EventTarget(), shown: () => true, base: () => undefined })
  cleanups.push(async () => following.close())
  expect(await untilStatus(following, (status) => status !== undefined)).toBeNull()
})

test("where pull requests cannot be read, the card is told why, and does not take it for a branch with none", async () => {
  const { served, connection } = await start([])
  const following = followPullRequest(connection, served.run.project!.id, { window: new EventTarget(), shown: () => true })
  cleanups.push(async () => following.close())
  while (following.state() === undefined) await Bun.sleep(5)
  expect(following.state()).toEqual({ unavailable: "No plugin reads pull requests: the github plugin is not loaded." })
})

test("the project's folder is opened on the computer evie runs on, and a folder that cannot be opened says why", async () => {
  const opened: string[] = []
  const served = await serveEvie([scriptedProvider([]).plugin], {
    openFolder: async (folder) => {
      if (opened.push(folder) > 1) throw new Error("The folder could not be opened: open stopped with code 1.")
    },
  })
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(async () => connection.close())
  while (connection.state().status !== "connected") await Bun.sleep(5)
  expect(await openFolder(connection, served.run.project!.id)).toBeUndefined()
  expect(opened).toEqual([served.run.project!.folder])
  expect(await openFolder(connection, served.run.project!.id)).toBe("The folder could not be opened: open stopped with code 1.")
})
