import { afterEach, expect, test } from "bun:test"
import { existsSync } from "node:fs"
import { mkdtemp, readdir, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { files, undo, type Plugin } from "../src/index.ts"
import { nextEvent, say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// Deleting a session for good: its conversation, its subagents' sessions,
// and the files kept for it in the data folder all go, and every client is
// told, so that each list drops it.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// A tool that starts a subagent and gives back the id of its session.
const delegate: Plugin = {
  name: "delegate",
  setup(host) {
    host.tools.add({ name: "delegate", description: "Starts a subagent.", inputSchema: { type: "object" }, run: (_input, context) => context.children.start({ text: "look around" }) })
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

async function start(script: ScriptedReply[], plugins: (home: string) => Plugin[] = () => []) {
  const home = await mkdtemp(join(tmpdir(), "evie-home-"))
  cleanups.push(() => rm(home, { recursive: true, force: true }))
  const run = await startEvie([scriptedProvider(script).plugin, ...plugins(home)], { home, questions: true })
  cleanups.push(run.stop)
  return run
}

// The sessions whose files are kept in one of the data folder's folders.
const keptFor = async (home: string, folder: string) => (await readdir(join(home, folder)).catch(() => [] as string[])).sort()

test("a session deleted for good takes its conversation, its subagents' sessions and the files kept for it with it, and every client is told", async () => {
  const work = backgroundWork()
  const run = await start(
    [
      { text: [], calls: [{ name: "write", input: { path: "beds.md", content: "North bed: greens.\n" } }, { name: "start_work", input: {} }, { name: "delegate", input: {} }] },
      { text: ["Looked around."] },
      { text: ["Written, and started."] },
      { text: ["The work is done."] },
    ],
    (home) => [files(), undo({ dataFolder: home }), work.plugin, delegate],
  )
  await say(run, "Write down the beds")
  const child = stored(run.events).findLast((event) => event.type === "tool_result")!.body.text
  work.finish("All done.")
  while (stored(run.events).filter((event) => event.type === "turn_end" && event.session === run.session).length < 2) await Bun.sleep(5)
  // Another session, which stays.
  const other = (await run.evie.send({ type: "open_session" })).session
  await say(run, "Something else", other)
  expect(await keptFor(run.home, "undo")).toEqual([run.session])
  expect(await keptFor(run.home, "tool-output")).toContain(run.session)

  const told = nextEvent(run, "deleted")
  await run.evie.send({ type: "delete_session", session: run.session })

  expect(await told).toEqual({ type: "deleted", session: run.session })
  expect((await run.evie.send({ type: "list_sessions" })).map((each) => each.session)).toEqual([other])
  for (const session of [run.session, child]) {
    await expect(run.evie.send({ type: "read_history", session })).rejects.toThrow(`There is no session with the id "${session}"`)
  }
  await expect(run.evie.send({ type: "open_session", session: run.session })).rejects.toThrow("There is no session")
  await expect(run.evie.send({ type: "send_message", session: run.session, text: "Still there?" })).rejects.toThrow()
  expect(await keptFor(run.home, "undo")).toEqual([])
  expect(await keptFor(run.home, "tool-output")).not.toContain(run.session)
  expect(await keptFor(run.home, "tool-output")).not.toContain(child)
  // The file the session wrote is the owner's, and stays.
  expect(existsSync(join(run.projectFolder, "beds.md"))).toBe(true)
  // The session that stays keeps its conversation.
  expect((await run.evie.send({ type: "read_history", session: other })).events.length).toBeGreaterThan(0)
})

test("a delete is refused while anything runs in the session, saying why, and nothing is deleted", async () => {
  const work = backgroundWork()
  const run = await start([{ hang: true, text: ["Thinking"] }, { text: [], calls: [{ name: "start_work", input: {} }] }, { text: ["Started."] }], () => [work.plugin])
  const remove = () => run.evie.send({ type: "delete_session", session: run.session })

  void run.evie.send({ type: "send_message", session: run.session, text: "Think it over" })
  while (!stored(run.events).some((event) => event.type === "turn_start")) await Bun.sleep(5)
  await expect(remove()).rejects.toThrow("This session cannot be deleted while a turn is running in it. Wait until it has finished, or stop it, then try again.")
  await run.evie.send({ type: "cancel", session: run.session })
  while (!stored(run.events).some((event) => event.type === "turn_end")) await Bun.sleep(5)
  await say(run, "Start the work")
  await expect(remove()).rejects.toThrow("while its background job 1 (some work) is running")

  expect((await run.evie.send({ type: "list_sessions" })).map((each) => each.session)).toEqual([run.session])
  expect(run.events.some((event) => event.type === "deleted")).toBe(false)
})

test("a subagent's session is deleted only with the session that started it, and a session that is not there cannot be", async () => {
  const run = await start([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["Looked around."] }, { text: ["Started it."] }], () => [delegate])
  await say(run, "Look around")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body.text

  await expect(run.evie.send({ type: "delete_session", session: child })).rejects.toThrow(`The session "${child}" is a subagent's, which is not listed. Delete the session that started it, "${run.session}", instead.`)
  await expect(run.evie.send({ type: "delete_session", session: "no-such-session" })).rejects.toThrow('There is no session with the id "no-such-session"')
  expect((await run.evie.send({ type: "read_history", session: child })).events.length).toBeGreaterThan(0)
})

// A tool that hands quick work to the background, which leaves the job's
// files in the session's folder of saved output.
const quickJob: Plugin = {
  name: "quick-job",
  setup(host) {
    host.tools.add({
      name: "quick_job",
      description: "Starts quick work in the background.",
      inputSchema: { type: "object" },
      run: async (_input, context) => `Started as job ${context.jobs.start({ kind: "work", title: "quick work", done: Promise.resolve("Done."), stop: () => {} }).id}.`,
    })
  },
}

test("deleting one session leaves another session's undo copies and saved output as they were", async () => {
  // The job ends within the turn, which is told of it at its next step.
  const turn = (file: string): ScriptedReply[] => [{ text: [], calls: [{ name: "write", input: { path: file, content: "kept\n" } }, { name: "quick_job", input: {} }] }, { text: ["Written."] }]
  const run = await start([...turn("one.md"), ...turn("two.md")], (home) => [files(), undo({ dataFolder: home }), quickJob])
  await say(run, "Write the first")
  const other = (await run.evie.send({ type: "open_session", folder: run.projectFolder })).session
  await say(run, "Write the second", other)
  const otherUndo = await readdir(join(run.home, "undo", other), { recursive: true })
  const otherOutput = await readdir(join(run.home, "tool-output", other))

  await run.evie.send({ type: "delete_session", session: run.session })

  expect(await keptFor(run.home, "undo")).toEqual([other])
  expect(await keptFor(run.home, "tool-output")).toEqual([other])
  expect(await readdir(join(run.home, "undo", other), { recursive: true })).toEqual(otherUndo)
  expect(await readdir(join(run.home, "tool-output", other))).toEqual(otherOutput)
})

test("a delete that cannot remove every file kept for the session still deletes it, and says what was left, naming the plugin", async () => {
  const stuck: Plugin = {
    name: "stuck",
    setup(host) {
      host.on("session_deleted", () => {
        throw new Error("its folder is locked")
      })
    },
  }
  const run = await start([{ text: ["Noted."] }], () => [stuck])
  await say(run, "Plan the beds for spring")

  const deleted = await run.evie.send({ type: "delete_session", session: run.session })

  expect(deleted).toEqual({ leftOver: ['The plugin "stuck" broke in its session_deleted hook: its folder is locked'] })
  expect(await run.evie.send({ type: "list_sessions" })).toEqual([])
})

test("deleting a session never sets up a project's plugins to do it, so it asks nothing, and the global plugins still remove their files", async () => {
  const home = await mkdtemp(join(tmpdir(), "evie-home-"))
  const here = await mkdtemp(join(tmpdir(), "evie-project-"))
  const there = await mkdtemp(join(tmpdir(), "evie-project-"))
  cleanups.push(async () => {
    for (const folder of [home, here, there]) await rm(folder, { recursive: true, force: true })
  })
  const setUp: string[] = []
  const options = { home, projectFolder: here, questions: true, projectSettings: async (project: { folder: string }) => (setUp.push(project.folder), []) }
  const plugins = (script: ScriptedReply[]) => [scriptedProvider(script).plugin, files(), undo({ dataFolder: home })]
  // A session in a second project, which writes a file, so undo keeps a copy.
  const first = await startEvie(plugins([{ text: [], calls: [{ name: "write", input: { path: "beds.md", content: "North bed.\n" } }] }, { text: ["Written."] }]), options)
  const session = (await first.evie.send({ type: "open_session", folder: there })).session
  await say(first, "Write down the beds", session)
  await first.stop()
  expect(await keptFor(home, "undo")).toEqual([session])

  // Evie starts again, and the second project is not opened.
  const second = await startEvie(plugins([]), options)
  cleanups.push(second.stop)
  setUp.length = 0
  await second.evie.send({ type: "delete_session", session })

  expect(setUp).toEqual([])
  expect(await keptFor(home, "undo")).toEqual([])
})
