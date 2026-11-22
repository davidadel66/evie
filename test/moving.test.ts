import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { Plugin, Project } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

// Moving a session into a project, out of one, or between two. The move is
// recorded, the model is told with a note where it now works, and from the
// next turn the session runs with the new project's plugins and folder.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// A plugin a project has of its own: a tool named after the project's folder.
const ownTool = (project: Project): Plugin => ({
  name: "own-tool",
  setup(host) {
    host.tools.add({ name: `tool_of_${project.name.replace(/\W/g, "_")}`, description: "The project's own.", inputSchema: { type: "object" }, run: async () => "ok" })
  },
})

// A tool that starts a subagent and gives back the id of its session.
const delegate: Plugin = {
  name: "delegate",
  setup(host) {
    host.tools.add({ name: "delegate", description: "Starts a subagent.", inputSchema: { type: "object" }, run: (_input, context) => context.children.start({ text: "look around" }) })
  },
}

// Records the project each of the session's requests was made in.
function watching() {
  const seen: (string | null)[] = []
  const plugin: Plugin = { name: "watching", setup: (host) => host.on("before_request", (_request, context) => void seen.push(context.project?.id ?? null)) }
  return { plugin, seen }
}

async function start(script: ScriptedReply[], plugins: Plugin[] = []) {
  const model = scriptedProvider(script)
  const watch = watching()
  const run = await startEvie([model.plugin, watch.plugin, ...plugins], { questions: true, projectSettings: (project) => [ownTool(project)] })
  cleanups.push(run.stop)
  // A second project, at a folder of its own.
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-other-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  const other = (await run.evie.send({ type: "open_session", folder })).project!
  return { run, model, seen: watch.seen, other }
}

const toolsOf = (request: { tools: { name: string }[] }) => request.tools.map((tool) => tool.name)
const own = (project: Project) => `tool_of_${project.name.replace(/\W/g, "_")}`

test("a session with no project moves into one: the move is recorded, the model is told where it now works, and its next turn runs there with that project's plugins", async () => {
  const { run, model, seen } = await start([{ text: ["Noted."] }, { text: ["Working there now."] }])
  const project = run.project!
  const { session } = await run.evie.send({ type: "open_session" })
  await say(run, "Plan the beds for spring", session)

  await run.evie.send({ type: "move_session", session, project: project.id })
  await say(run, "Carry on in the project", session)

  const note = `The owner moved this session into the project "${project.name}". You are now working in it: its folder is ${project.folder}, and a relative path is taken from there.`
  const events = stored(run.events).filter((event) => event.session === session)
  expect(events.filter((event) => event.type === "moved" || event.type === "note").map(({ type, body }) => ({ type, body }))).toEqual([
    { type: "moved", body: { from: null, to: project.id } },
    { type: "note", body: { text: note } },
  ])
  expect(model.requests[1]!.messages.at(-2)).toEqual({ role: "user", text: `[Note from Evie: ${note}]` })
  expect(toolsOf(model.requests[0]!)).not.toContain(own(project))
  expect(toolsOf(model.requests[1]!)).toContain(own(project))
  expect(seen).toEqual([null, project.id])
  expect((await run.evie.send({ type: "list_sessions" })).find((each) => each.session === session)?.project).toBe(project.id)
})

test("a session moves out of its project into none, and its next turn has no project folder and none of that project's plugins", async () => {
  const { run, model, seen } = await start([{ text: ["Noted."] }, { text: ["Out of it now."] }])
  const project = run.project!
  await say(run, "Plan the beds for spring")

  await run.evie.send({ type: "move_session", session: run.session, project: null })
  await say(run, "Carry on with no project")

  const note = `The owner moved this session out of the project "${project.name}". It is now in no project, so there is no project folder: give full paths to files and folders. Files named earlier in this conversation are still in the old folder, ${project.folder}.`
  expect(stored(run.events).filter((event) => event.session === run.session && (event.type === "moved" || event.type === "note")).map(({ body }) => body)).toEqual([{ from: project.id, to: null }, { text: note }])
  expect(toolsOf(model.requests[0]!)).toContain(own(project))
  expect(toolsOf(model.requests[1]!)).not.toContain(own(project))
  expect(seen).toEqual([project.id, null])
  expect((await run.evie.send({ type: "list_sessions" }))[0]?.project).toBeNull()
})

test("a session moves between two projects, with its subagents, which are told too", async () => {
  const { run, model, seen, other } = await start(
    [{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["Looked around."] }, { text: ["Started it."] }, { text: ["Looked again."] }],
    [delegate],
  )
  const project = run.project!
  await say(run, "Look around the garden plans")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body.text
  while (stored(run.events).filter((event) => event.type === "turn_end").length < 2) await Bun.sleep(5)

  await run.evie.send({ type: "move_session", session: run.session, project: other.id })
  await run.evie.send({ type: "move_session", session: run.session, project: other.id })
  await say(run, "Look again")

  const note = `The owner moved this session from the project "${project.name}" into the project "${other.name}". You are now working in it: its folder is ${other.folder}, and a relative path is taken from there. Files named earlier in this conversation are still in the old folder, ${project.folder}.`
  // Moving it again to where it is did nothing.
  for (const session of [run.session, child]) {
    expect(stored(run.events).filter((event) => event.session === session && (event.type === "moved" || event.type === "note")).map(({ body }) => body)).toEqual([{ from: project.id, to: other.id }, { text: note }])
  }
  expect(toolsOf(model.requests.at(-1)!)).toContain(own(other))
  expect(toolsOf(model.requests.at(-1)!)).not.toContain(own(project))
  expect(seen.at(-1)).toBe(other.id)
})

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

test("a move is refused while anything runs in the session, a turn, a background job or a subagent, saying why, and nothing is moved", async () => {
  const work = backgroundWork()
  const { run, other } = await start(
    [
      { hang: true, text: ["Thinking"] },
      { text: [], calls: [{ name: "start_work", input: {} }] },
      { text: ["Started."] },
      { text: ["The work is done."] },
      { text: [], calls: [{ name: "delegate", input: {} }] },
      { hang: true, text: ["Looking"] },
      { text: ["Started it."] },
    ],
    [work.plugin, delegate],
  )
  const move = () => run.evie.send({ type: "move_session", session: run.session, project: other.id })

  // A turn.
  void run.evie.send({ type: "send_message", session: run.session, text: "Think it over" })
  while (!stored(run.events).some((event) => event.type === "turn_start")) await Bun.sleep(5)
  await expect(move()).rejects.toThrow("This session cannot be moved while a turn is running in it. Wait until it has finished, or stop it, then try again.")
  await run.evie.send({ type: "cancel", session: run.session })
  while (!stored(run.events).some((event) => event.type === "turn_end")) await Bun.sleep(5)

  // A background job.
  await say(run, "Start the work")
  await expect(move()).rejects.toThrow("while its background job 1 (some work) is running")
  work.finish("All done.")
  while (stored(run.events).filter((event) => event.type === "turn_end").length < 3) await Bun.sleep(5)

  // A subagent.
  await say(run, "Look around")
  await expect(move()).rejects.toThrow("while a subagent it started is working")

  const listed = await run.evie.send({ type: "list_sessions" })
  expect(listed.find((each) => each.session === run.session)?.project).toBe(run.project!.id)
  expect(stored(run.events).some((event) => event.type === "moved")).toBe(false)
})

test("a session can only be moved into a project that is there, and a subagent's moves only with the session that started it", async () => {
  const { run } = await start([{ text: [], calls: [{ name: "delegate", input: {} }] }, { text: ["Looked around."] }, { text: ["Started it."] }], [delegate])
  await say(run, "Look around")
  const child = stored(run.events).find((event) => event.type === "tool_result")!.body.text

  await expect(run.evie.send({ type: "move_session", session: run.session, project: "no-such-project" })).rejects.toThrow('There is no project with the id "no-such-project"')
  await expect(run.evie.send({ type: "move_session", session: child, project: null })).rejects.toThrow("is a subagent's")
  await expect(run.evie.send({ type: "move_session", session: "no-such-session", project: null })).rejects.toThrow('There is no session with the id "no-such-session"')
})

test("a move is not activity: a session settled by hand stays settled and keeps its place, and is not the one --continue carries on, though the model is still told", async () => {
  const { run, model, other } = await start([{ text: ["Noted."] }, { text: ["Noted there."] }, { text: ["Carried on."] }])
  await say(run, "Plan the beds for spring")
  await Bun.sleep(5)
  const there = (await run.evie.send({ type: "open_session", folder: other.folder })).session
  await say(run, "Sort the seed packets", there)
  await run.evie.send({ type: "settle_session", session: run.session, settled: true })
  const before = (await run.evie.send({ type: "list_sessions" })).find((each) => each.session === run.session)!
  await Bun.sleep(5)

  await run.evie.send({ type: "move_session", session: run.session, project: other.id })

  expect((await run.evie.send({ type: "list_sessions" })).find((each) => each.session === run.session)).toEqual({ ...before, project: other.id })
  expect((await run.evie.send({ type: "open_session", folder: other.folder, continue: true })).session).toBe(there)
  await say(run, "Carry on in the new project")
  expect(model.requests.at(-1)!.messages.some((message) => message.role === "user" && message.text.startsWith("[Note from Evie: The owner moved this session"))).toBe(true)
})

test("a message sent the moment a move is recorded reaches the session in its new project: there is no moment when it is not open", async () => {
  const { run, seen, other } = await start([{ text: ["Noted."] }, { text: ["Working there."] }])
  await say(run, "Plan the beds for spring")
  let sent: Promise<string> | undefined
  const unsubscribe = run.evie.subscribe((event) => {
    if (event.type !== "moved" || event.session !== run.session) return
    unsubscribe()
    sent = run.evie.send({ type: "send_message", session: run.session, text: "Carry on there" }).then(
      () => "sent",
      (cause) => String(cause),
    )
  })

  await run.evie.send({ type: "move_session", session: run.session, project: other.id })

  expect(await sent).toBe("sent")
  while (stored(run.events).filter((event) => event.type === "turn_end").length < 2) await Bun.sleep(5)
  expect(seen).toEqual([run.project!.id, other.id])
})

test("a move or a delete while the session is still opening is refused, saying so, and goes once it has opened", async () => {
  const home = await mkdtemp(join(tmpdir(), "evie-home-"))
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-project-")))
  cleanups.push(async () => {
    for (const each of [home, folder]) await rm(each, { recursive: true, force: true })
  })
  const first = await startEvie([scriptedProvider([{ text: ["Noted."] }]).plugin], { home, projectFolder: folder })
  await say(first, "Plan the beds for spring")
  await first.stop()
  // Evie starts again, and the project's settings take their time the next time they are read.
  let reads = 0
  let release = () => {}
  const held = new Promise<void>((resolve) => (release = resolve))
  const second = await startEvie([scriptedProvider([]).plugin], {
    home,
    projectFolder: folder,
    projectSettings: async () => {
      if (++reads === 2) await held
      return { plugins: [], forNow: true }
    },
  })
  cleanups.push(second.stop)

  const opening = second.evie.send({ type: "open_session", session: first.session })
  while (reads < 2) await Bun.sleep(5)
  await expect(second.evie.send({ type: "move_session", session: first.session, project: null })).rejects.toThrow(
    "This session is opening, so it cannot be moved yet. Try again once it has opened.",
  )
  await expect(second.evie.send({ type: "delete_session", session: first.session })).rejects.toThrow("This session is opening, so it cannot be deleted yet.")
  release()
  await opening

  await second.evie.send({ type: "move_session", session: first.session, project: null })
  expect((await second.evie.send({ type: "list_sessions" })).find((each) => each.session === first.session)?.project).toBeNull()
})
