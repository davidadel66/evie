import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { startServer, type Command, type Evie } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { openSessions, type Sessions } from "../web/src/sessions.ts"
import { openStorage, type Storage, type StorageState } from "../web/src/storage.ts"
import { storageShown } from "../web/src/shown.ts"
import { followLooks, keptLook, onLooks } from "../web/src/project-look.ts"
import { makeWorkspace, pickIcon } from "../web/src/workspace-actions.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// The workspace view's work with evie's server, apart from its screen:
// Storage, listed a folder at a time and read a file at a time; picking a
// project's icon and colour; and making a workspace. These tests connect
// with a token, as the connection's own tests do.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function start() {
  const served = await serveEvie([scriptedProvider([]).plugin])
  cleanups.push(served.stop)
  const connection = openConnection({ url: served.server.url, token: served.token, firstWait: 10 })
  cleanups.push(() => connection.close())
  const evieFolder = join(served.run.projectFolder, ".evie")
  return { served, connection, evieFolder, project: served.run.project!.id }
}

function storageOf(connection: Parameters<typeof openStorage>[0], project: string) {
  const storage = openStorage(connection, project)
  cleanups.push(async () => storage.close())
  return storage
}

async function until(storage: Storage, ready: (state: StorageState) => boolean) {
  while (!ready(storage.state())) await Bun.sleep(5)
  return storage.state()
}

const names = (state: StorageState, folder: string) => state.folders.get(folder)?.entries?.map((entry) => entry.name)

test("once connected, Storage lists the whole .evie folder, every folder open, and a folder folds and opens again", async () => {
  const { connection, evieFolder, project } = await start()
  await mkdir(join(evieFolder, "files", "menus"), { recursive: true })
  await writeFile(join(evieFolder, "evie.json"), "{}\n")
  await writeFile(join(evieFolder, "files", "notes.md"), "# Beds\n")
  await writeFile(join(evieFolder, "files", "menus", "winter.md"), "# Soups\n")
  const storage = storageOf(connection, project)

  const tree = await until(storage, (state) => state.folders.get("files/menus")?.entries !== undefined)
  expect(names(tree, "")).toEqual(["files", "evie.json"])
  expect(names(tree, "files")).toEqual(["menus", "notes.md"])
  expect(names(tree, "files/menus")).toEqual(["winter.md"])
  expect(tree.closed.size).toBe(0)

  storage.toggle("files")
  expect(storage.state().closed.has("files")).toBe(true)
  storage.toggle("files")
  expect(storage.state().closed.has("files")).toBe(false)
})

test("a file is read to be shown, and one Storage cannot show says why, in the server's words", async () => {
  const { connection, evieFolder, project } = await start()
  await mkdir(evieFolder, { recursive: true })
  await writeFile(join(evieFolder, "notes.txt"), "Beds\n")
  const storage = storageOf(connection, project)

  storage.show("notes.txt")
  const read = await until(storage, (state) => state.file?.read !== undefined)
  expect(read.file).toMatchObject({ path: "notes.txt", read: { kind: "text", text: "Beds\n" } })

  storage.show("../secret.txt")
  const refused = await until(storage, (state) => state.file?.problem !== undefined)
  expect(refused.file).toEqual({ path: "../secret.txt", problem: "../secret.txt is outside this project's .evie folder. Storage holds only what is inside it." })

  storage.show(undefined)
  expect(storage.state().file).toBeUndefined()
})

test("when the line comes back after a drop, what is open in Storage is listed again", async () => {
  const { served, connection, evieFolder, project } = await start()
  await mkdir(join(evieFolder, "files"), { recursive: true })
  const storage = storageOf(connection, project)
  await until(storage, (state) => state.folders.get("files")?.entries !== undefined)
  const port = Number(new URL(served.server.url).port)

  await served.server.stop()
  while (connection.state().status !== "reconnecting") await Bun.sleep(5)
  await writeFile(join(evieFolder, "files", "late.md"), "written while the line was down\n")
  const again = startServer({ evie: served.run.evie, tokens: served.tokens, host: "127.0.0.1", port })
  cleanups.push(() => again.stop())

  const relisted = await until(storage, (state) => names(state, "files")?.includes("late.md") === true)
  expect(names(relisted, "files")).toEqual(["late.md"])
})

function listOf(connection: Parameters<typeof openSessions>[0]): Sessions {
  const sessions = openSessions(connection)
  cleanups.push(async () => sessions.close())
  return sessions
}

test("picking a project's icon and colour draws it at once and sends it, and the list of projects then has it; a refusal comes back in the server's words", async () => {
  const { connection, project } = await start()
  const sessions = listOf(connection)
  const unfollow = followLooks(sessions)
  cleanups.push(async () => unfollow())
  while (sessions.state().projects.length === 0) await Bun.sleep(5)

  const picking = pickIcon(connection, sessions, project, { icon: "leaf", colour: "moss" })
  expect(keptLook(project)).toEqual({ icon: "leaf", colour: "moss" })
  expect(await picking).toBeUndefined()
  expect(sessions.state().projects[0]).toMatchObject({ id: project, icon: "leaf", colour: "moss" })
  expect(await pickIcon(connection, sessions, project, { icon: "Not An Icon", colour: "moss" })).toBe(
    '"Not An Icon" is not the name of an icon. Give a Phosphor icon\'s name, in lower case with dashes, such as "folder" or "cooking-pot".',
  )
  expect(keptLook(project)).toEqual({ icon: "leaf", colour: "moss" })
})

test("a list of projects from before a pick never draws the old look over it, and a pick refused while a later one is out does not undo the later one", async () => {
  // Lists of projects after the first come back only when let, and the
  // later pick reaches the server only when let.
  const late = { list: gate(), pick: gate() }
  let lists = 0
  const { connection, project } = await startHolding(async (command, answer) => {
    if (command.type === "set_project_look" && command.colour === "slate") await late.pick.opened
    const answered = answer().then(
      (result) => ({ result }),
      (error: unknown) => ({ error }),
    )
    if (command.type === "list_projects" && ++lists > 1) await late.list.opened
    const done = await answered
    if ("error" in done) throw done.error
    return done.result
  })
  const sessions = listOf(connection)
  const unfollow = followLooks(sessions)
  cleanups.push(async () => unfollow())
  while (sessions.state().projects.length === 0) await Bun.sleep(5)
  const before = keptLook(project)
  const seen: string[] = []
  const unhear = onLooks(() => seen.push(`${keptLook(project).icon} ${keptLook(project).colour}`))
  cleanups.push(async () => unhear())

  // A list asked for before the picks, which brings the old look late.
  sessions.refreshProjects()
  const refused = pickIcon(connection, sessions, project, { icon: "leaf", colour: "green" as never })
  const kept = pickIcon(connection, sessions, project, { icon: "leaf", colour: "slate" })
  late.list.open()
  // Refused, and its list read, while the later pick has not reached the server.
  expect(await refused).toContain('"green" is not one of the project colours.')
  expect(keptLook(project)).toEqual({ icon: "leaf", colour: "slate" })
  late.pick.open()
  expect(await kept).toBeUndefined()

  expect(keptLook(project)).toEqual({ icon: "leaf", colour: "slate" })
  expect(seen).toEqual(["leaf green", "leaf slate"])
  expect(before).toEqual({ icon: "folder", colour: expect.any(String) })
})

test("making a workspace gives back the project once the list of projects has it; a refusal comes back in the server's words", async () => {
  const { served, connection } = await start()
  const sessions = listOf(connection)
  while (connection.state().status !== "connected") await Bun.sleep(5)
  const place = await realpath(await mkdtemp(join(tmpdir(), "evie-place-")))
  cleanups.push(() => rm(place, { recursive: true, force: true }))

  const made = await makeWorkspace(connection, sessions, { folder: join(place, "garden"), name: "Garden", sync: "none" })
  expect(made).toMatchObject({ project: { name: "Garden", folder: join(place, "garden") }, madeFolder: true })
  expect(sessions.state().projects.some((each) => each.name === "Garden")).toBe(true)

  expect(await makeWorkspace(connection, sessions, { folder: served.run.project!.folder })).toEqual({
    problem: expect.stringContaining(`${served.run.project!.folder} is already the project`),
  })
  expect(await makeWorkspace(connection, sessions, { folder: "garden" })).toEqual({
    problem: '"garden" is not a full path. Give the folder\'s full path, such as /Users/me/finances, or one in your home folder, such as ~/finances.',
  })
})

// Starts the server in front of a core whose every answer passes through
// `hold`, which can keep one back, as a line that brings it late would.
async function startHolding(hold: (command: Command, answer: () => Promise<unknown>) => Promise<unknown>) {
  const served = await serveEvie([scriptedProvider([]).plugin])
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
  return { served, connection, project: served.run.project!.id }
}

// A promise to wait on, and the function that settles it.
function gate() {
  let open = () => {}
  const opened = new Promise<void>((resolve) => (open = resolve))
  return { opened, open }
}

test("Storage lists what evie showed under shown, by the day, and opens each such file from the route, with the viewer of its kind", async () => {
  const { served, connection, evieFolder, project } = await start()
  await mkdir(join(evieFolder, "shown", "2026-11-22"), { recursive: true })
  await writeFile(join(evieFolder, "shown", "2026-11-22", "guide.md"), "# Guide\n")
  await writeFile(join(evieFolder, "notes.txt"), "Beds.\n")
  const storage = storageOf(connection, project)

  const tree = await until(storage, (state) => state.folders.get("shown/2026-11-22")?.entries !== undefined)
  expect(names(tree, "")).toEqual(["shown", "notes.txt"])
  expect(names(tree, "shown/2026-11-22")).toEqual(["guide.md"])

  const guide = storageShown(project, "shown/2026-11-22/guide.md")
  expect(guide).toMatchObject({ name: "guide.md", type: "text/markdown", folder: ".evie/shown/2026-11-22/", storage: { project, path: "shown/2026-11-22/guide.md" } })
  const fetched = await fetch(new URL(guide!.address!, served.server.url), { headers: { Authorization: `Bearer ${served.token}` } })
  expect(await fetched.text()).toBe("# Guide\n")
  // A file of no kind evie shows is read as Storage reads it.
  expect(storageShown(project, "notes.txt")).toBeUndefined()
})

test("a file the frame draws itself is not read over the line; one the route will not give is read as Storage reads any file, and says why when it cannot be", async () => {
  const { connection, evieFolder, project } = await start()
  await mkdir(join(evieFolder, "shown"), { recursive: true })
  await writeFile(join(evieFolder, "shown", "guide.md"), "# Guide\n")
  await writeFile(join(evieFolder, "shown", "not-really.md"), new Uint8Array([0x23, 0, 0x01, 0x02]))
  await writeFile(join(evieFolder, "shown", "logo.svg"), "an outline, not yet a drawing\n")
  const storage = storageOf(connection, project)
  await until(storage, (state) => state.folders.get("")?.entries !== undefined)

  storage.show("shown/guide.md")
  expect((await until(storage, (state) => state.file?.framed === true || state.file?.read !== undefined)).file).toEqual({ path: "shown/guide.md", framed: true })

  storage.show("shown/not-really.md")
  expect((await until(storage, (state) => state.file?.problem !== undefined || state.file?.framed === true)).file).toEqual({
    path: "shown/not-really.md",
    problem: "shown/not-really.md is neither text nor an image, so Storage cannot show it.",
  })

  storage.show("shown/logo.svg")
  const read = await until(storage, (state) => state.file?.path === "shown/logo.svg" && (state.file.read !== undefined || state.file.framed === true))
  expect(read.file).toMatchObject({ path: "shown/logo.svg", read: { kind: "image", type: "image/svg+xml" } })
})
