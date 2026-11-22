import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { homedir, tmpdir } from "node:os"
import { join } from "node:path"
import { startServer } from "../src/index.ts"
import { openConnection } from "../web/src/connection.ts"
import { openSessions, type Sessions } from "../web/src/sessions.ts"
import { openStorage, type Storage, type StorageState } from "../web/src/storage.ts"
import { makeWorkspace, pickIcon, readHome } from "../web/src/workspace-actions.ts"
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

test("once connected, Storage lists the top of the .evie folder, and a folder opens in place, listed as it opens, and closes again", async () => {
  const { connection, evieFolder, project } = await start()
  await mkdir(join(evieFolder, "files"), { recursive: true })
  await writeFile(join(evieFolder, "evie.json"), "{}\n")
  await writeFile(join(evieFolder, "files", "notes.md"), "# Beds\n")
  const storage = storageOf(connection, project)

  const top = await until(storage, (state) => state.folders.get("")?.entries !== undefined)
  expect(names(top, "")).toEqual(["files", "evie.json"])
  expect(top.open.has("files")).toBe(false)

  storage.toggle("files")
  const opened = await until(storage, (state) => state.folders.get("files")?.entries !== undefined)
  expect(opened.open.has("files")).toBe(true)
  expect(names(opened, "files")).toEqual(["notes.md"])

  storage.toggle("files")
  expect(storage.state().open.has("files")).toBe(false)
})

test("a file is read to be shown, and one Storage cannot show says why, in the server's words", async () => {
  const { connection, evieFolder, project } = await start()
  await mkdir(evieFolder, { recursive: true })
  await writeFile(join(evieFolder, "notes.md"), "# Beds\n")
  const storage = storageOf(connection, project)

  storage.show("notes.md")
  const read = await until(storage, (state) => state.file?.read !== undefined)
  expect(read.file).toMatchObject({ path: "notes.md", read: { kind: "text", text: "# Beds\n" } })

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
  await until(storage, (state) => state.folders.get("")?.entries !== undefined)
  storage.toggle("files")
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

test("picking a project's icon and colour sends them, and the list of projects then has them; a refusal comes back in the server's words", async () => {
  const { connection, project } = await start()
  const sessions = listOf(connection)
  while (sessions.state().projects.length === 0) await Bun.sleep(5)

  expect(await pickIcon(connection, sessions, project, { icon: "leaf", colour: "moss" })).toBeUndefined()
  while (sessions.state().projects[0]?.icon !== "leaf") await Bun.sleep(5)
  expect(sessions.state().projects[0]).toMatchObject({ id: project, icon: "leaf", colour: "moss" })
  expect(await pickIcon(connection, sessions, project, { icon: "Not An Icon", colour: "moss" })).toBe(
    '"Not An Icon" is not the name of an icon. Give a Phosphor icon\'s name, in lower case with dashes, such as "folder" or "cooking-pot".',
  )
})

test("making a workspace gives back the project, which the list of projects then has; a refusal comes back in the server's words", async () => {
  const { served, connection } = await start()
  const sessions = listOf(connection)
  while (connection.state().status !== "connected") await Bun.sleep(5)
  const place = await realpath(await mkdtemp(join(tmpdir(), "evie-place-")))
  cleanups.push(() => rm(place, { recursive: true, force: true }))

  const made = await makeWorkspace(connection, sessions, { folder: join(place, "garden"), name: "Garden", sync: "none" })
  expect(made).toMatchObject({ project: { name: "Garden", folder: join(place, "garden") }, madeFolder: true })
  while (!sessions.state().projects.some((each) => each.name === "Garden")) await Bun.sleep(5)

  expect(await makeWorkspace(connection, sessions, { folder: served.run.project!.folder })).toEqual({
    problem: expect.stringContaining(`${served.run.project!.folder} is already the project`),
  })
  expect(await makeWorkspace(connection, sessions, { folder: "garden" })).toEqual({
    problem: '"garden" is not a full path. Give the folder\'s full path, such as /Users/me/finances.',
  })
})

test("the page is told the home folder of the computer evie runs on", async () => {
  const { connection } = await start()
  while (connection.state().status !== "connected") await Bun.sleep(5)

  expect(await readHome(connection)).toBe(homedir())
})
