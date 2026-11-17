import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { basename, join } from "node:path"
import { say, startEvie, stored, type Run } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start(script: Parameters<typeof scriptedProvider>[0] = [], home?: string) {
  const model = scriptedProvider(script)
  const run: Run = await startEvie([model.plugin], home ? { home } : {})
  cleanups.push(run.stop)
  return { run, model }
}

async function tempFolder(prefix: string) {
  const folder = await mkdtemp(join(tmpdir(), prefix))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

test("opening a session in a folder creates a project for that folder", async () => {
  const { run } = await start()

  expect(run.project).toEqual({
    id: expect.any(String),
    name: basename(run.projectFolder),
    folder: await realpath(run.projectFolder),
  })
})

test("a second session in the same folder joins the same project", async () => {
  const { run } = await start()

  const second = await run.evie.send({ type: "open_session", folder: run.projectFolder })

  expect(second.project?.id).toBe(run.project!.id)
  expect(second.session).not.toBe(run.session)
})

test("a folder inside a git repository belongs to the project at the repository's root", async () => {
  const { run } = await start()
  const repository = await tempFolder("evie-repo-")
  await mkdir(join(repository, ".git"))
  await mkdir(join(repository, "src", "deep"), { recursive: true })

  const atRoot = await run.evie.send({ type: "open_session", folder: repository })
  const inside = await run.evie.send({ type: "open_session", folder: join(repository, "src", "deep") })

  expect(inside.project).toEqual(atRoot.project)
  expect(inside.project?.folder).toBe(await realpath(repository))
})

test("a project is remembered after Evie is started again", async () => {
  const home = await tempFolder("evie-home-")
  const folder = await tempFolder("evie-shared-project-")
  const first = await start([], home)
  const before = await first.run.evie.send({ type: "open_session", folder })
  await first.run.evie.close()

  const second = await start([], home)
  const after = await second.run.evie.send({ type: "open_session", folder })

  expect(after.project).toEqual(before.project)
})

test("a session can be opened by project id, wherever Evie was started", async () => {
  const { run } = await start()

  const opened = await run.evie.send({ type: "open_session", project: run.project!.id })

  expect(opened.project).toEqual(run.project)
})

test("opening a session for an unknown project is refused", async () => {
  const { run } = await start()

  await expect(run.evie.send({ type: "open_session", project: "no-such-project" })).rejects.toThrow(
    'There is no project with the id "no-such-project"',
  )
})

test("opening a session for a folder that does not exist is refused", async () => {
  const { run } = await start()

  await expect(run.evie.send({ type: "open_session", folder: "/no/such/folder" })).rejects.toThrow(
    'The folder "/no/such/folder" does not exist',
  )
})

test("a session opened with no project is the general session, and it can take a turn", async () => {
  const { run, model } = await start([{ text: ["Hello."] }])

  const general = await run.evie.send({ type: "open_session" })
  await say(run, "hi", general.session)

  expect(general.project).toBeNull()
  expect(model.requests[0]?.messages).toEqual([{ role: "user", text: "hi" }])
})

test("two sessions keep separate conversations, and every event names its session", async () => {
  const { run, model } = await start([{ text: ["one"] }, { text: ["two"] }])
  const other = await run.evie.send({ type: "open_session", folder: run.projectFolder })

  await say(run, "to the first")
  await say(run, "to the second", other.session)

  expect(model.requests[1]?.messages).toEqual([{ role: "user", text: "to the second" }])
  const sessions = stored(run.events).map((event) => event.session)
  expect(sessions).toEqual([...Array(4).fill(run.session), ...Array(4).fill(other.session)])
  expect(stored(run.events).map((event) => event.seq)).toEqual([1, 2, 3, 4, 1, 2, 3, 4])
})

test("a message to a session that is not open is refused", async () => {
  const { run } = await start()

  await expect(run.evie.send({ type: "send_message", session: "no-such-session", text: "hi" })).rejects.toThrow(
    'The session "no-such-session" is not open',
  )
})
