import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { basename, join } from "node:path"
import { loadSettingsFiles } from "../src/index.ts"
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

test("opening a session for a file is refused", async () => {
  const { run } = await start()
  const file = join(run.projectFolder, "notes.txt")
  await Bun.write(file, "not a folder")

  await expect(run.evie.send({ type: "open_session", folder: file })).rejects.toThrow("is a file, not a folder")
})

test("a command Evie does not know is refused", async () => {
  const { run } = await start()
  const unknown = { type: "do_something_else" } as unknown as { type: "open_session" }

  await expect(run.evie.send(unknown)).rejects.toThrow('Unknown command "do_something_else"')
})

// ---- Evie's data folder is never a project ----------------------------------------------------

// Evie started the way `evie serve` starts it, with its settings files, its
// data folder `.evie` in a folder that stands for the owner's home folder,
// as `~/.evie` is in a real one, unless `dataFolder` says where it is.
async function startAtHome(dataFolder?: (home: string) => string) {
  const home = await realpath(await tempFolder("evie-owner-"))
  const data = dataFolder?.(home) ?? join(home, ".evie")
  await mkdir(data, { recursive: true })
  await writeFile(join(data, "evie.json"), JSON.stringify({ plugins: [] }))
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: data } })
  const run: Run = await startEvie([scriptedProvider([]).plugin], { home: data, projectSettings: settings.projectSettings, trust: settings.trust, syncSetting: settings.syncSetting })
  cleanups.push(run.stop)
  const open = (folder: string) => run.evie.send({ type: "open_session", folder, questions: true })
  const projects = async () => (await run.evie.send({ type: "list_projects" })).map((project) => project.folder)
  return { run, home, data, open, projects }
}

// What a session opened in such a folder says, once.
const notAProject = (folder: string, how: string) =>
  `${folder} ${how}, where evie keeps its own settings and state, so it is never made a project. This session belongs to no project.`

test("running evie in the home folder, which holds its data folder, starts a session with no project, says so once, and never asks to trust evie's own settings file", async () => {
  const { run, home, data, open, projects } = await startAtHome()

  const opened = await open(home)

  expect(opened.project).toBeNull()
  expect(opened.notices).toEqual([notAProject(home, `holds evie's data folder, ${data}`)])
  expect(run.events.filter((event) => event.type === "question")).toEqual([])
  expect(await projects()).not.toContain(home)
  // Opened again, it says so again, once.
  expect((await open(home)).notices).toEqual([notAProject(home, `holds evie's data folder, ${data}`)])
})

test("the data folder itself, a folder inside it, and a link to either, or to a folder holding it, are never made a project, as they really are", async () => {
  const { home, data, open, projects } = await startAtHome()
  await mkdir(join(data, "undo", "old"), { recursive: true })
  const links = await tempFolder("evie-links-")
  await symlink(home, join(links, "to-home"))
  await symlink(data, join(links, "to-data"))
  await symlink(join(data, "undo"), join(links, "to-undo"))

  expect((await open(data)).notices).toEqual([notAProject(data, "is evie's data folder")])
  expect((await open(join(data, "undo", "old"))).notices).toEqual([notAProject(join(data, "undo", "old"), `is inside evie's data folder, ${data}`)])
  expect((await open(join(links, "to-home"))).notices).toEqual([notAProject(home, `holds evie's data folder, ${data}`)])
  expect((await open(join(links, "to-data"))).notices).toEqual([notAProject(data, "is evie's data folder")])
  expect((await open(join(links, "to-undo"))).project).toBeNull()
  expect((await projects()).filter((folder) => folder.startsWith(home))).toEqual([])
})

test("a folder in a git repository whose top holds the data folder, as a home folder kept in git does, belongs to no project", async () => {
  const { home, data, open, projects } = await startAtHome()
  await mkdir(join(home, ".git"))
  await mkdir(join(home, "notes"))

  const opened = await open(join(home, "notes"))

  expect(opened.project).toBeNull()
  expect(opened.notices).toEqual([
    `The git repository at ${home}, which ${join(home, "notes")} is in, holds evie's data folder, ${data}, where evie keeps its own settings and state, ` +
      "so it is never made a project. This session belongs to no project.",
  ])
  expect(await projects()).not.toContain(home)
})

test("with EVIE_HOME set elsewhere, the folder that holds it is never a project, and the home folder is a project as any other folder is", async () => {
  const elsewhere = await realpath(await tempFolder("evie-elsewhere-"))
  const { home, data, open } = await startAtHome(() => join(elsewhere, "state", "evie"))

  const holding = await open(elsewhere)
  const atHome = await open(home)

  expect(holding.project).toBeNull()
  expect(holding.notices).toEqual([notAProject(elsewhere, `holds evie's data folder, ${data}`)])
  expect(atHome.project).toEqual({ id: expect.any(String), name: basename(home), folder: home })
  expect(atHome.notices).toEqual([])
})
