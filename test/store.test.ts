import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createEventLog } from "../src/core/log.ts"
import { MIGRATIONS, openStore, type Migration } from "../src/core/store.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await mkdtemp(join(tmpdir(), prefix))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

const version = (db: Database) => db.query<{ user_version: number }, []>("PRAGMA user_version").get()!.user_version

test("a data folder from before versions were recorded is adopted, and its projects are still there", async () => {
  const home = await tempFolder("evie-home-")
  const folder = await realpath(await tempFolder("evie-old-project-"))
  // The tables exactly as Evie made them before it recorded a version.
  const old = new Database(join(home, "evie.db"), { create: true })
  old.run("CREATE TABLE project (id TEXT PRIMARY KEY, name TEXT NOT NULL, folder TEXT NOT NULL UNIQUE, created INTEGER NOT NULL)")
  old.run("CREATE TABLE session (id TEXT PRIMARY KEY, project TEXT REFERENCES project (id), created INTEGER NOT NULL)")
  old.run("CREATE TABLE event (session TEXT NOT NULL, seq INTEGER NOT NULL, type TEXT NOT NULL, body TEXT NOT NULL, time INTEGER NOT NULL, PRIMARY KEY (session, seq))")
  old.run("CREATE TABLE inbox (id INTEGER PRIMARY KEY AUTOINCREMENT, session TEXT NOT NULL, body TEXT NOT NULL, time INTEGER NOT NULL)")
  old.run("INSERT INTO project (id, name, folder, created) VALUES ('kept-project', 'old', ?, 1)", [folder])
  old.close()

  const run = await startEvie([scriptedProvider([]).plugin], { home })
  cleanups.push(run.stop)
  const opened = await run.evie.send({ type: "open_session", folder })

  expect(opened.project?.id).toBe("kept-project")
})

test("a data folder written by a newer Evie is refused", async () => {
  const home = await tempFolder("evie-home-")
  const newer = new Database(join(home, "evie.db"), { create: true })
  newer.run("PRAGMA user_version = 999")
  newer.close()

  await expect(startEvie([scriptedProvider([]).plugin], { home })).rejects.toThrow(
    "was written by a newer version of Evie",
  )
})

test("an older database is brought up to date one step at a time, keeping what it holds", async () => {
  const path = join(await tempFolder("evie-store-"), "test.db")
  const steps: Migration[] = [
    (db) => db.run("CREATE TABLE note (id INTEGER PRIMARY KEY, text TEXT NOT NULL)"),
    (db) => db.run("ALTER TABLE note ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0"),
  ]
  const first = openStore(path, steps.slice(0, 1))
  first.run("INSERT INTO note (text) VALUES ('keep me')")
  first.close()

  const second = openStore(path, steps)
  cleanups.push(async () => second.close())

  expect(version(second)).toBe(2)
  expect(second.query("SELECT text, pinned FROM note").all()).toEqual([{ text: "keep me", pinned: 0 }])
})

test("a step that fails leaves the database as it was", async () => {
  const path = join(await tempFolder("evie-store-"), "test.db")
  const good: Migration = (db) => db.run("CREATE TABLE note (id INTEGER PRIMARY KEY, text TEXT NOT NULL)")
  const bad: Migration = (db) => {
    db.run("ALTER TABLE note ADD COLUMN pinned INTEGER NOT NULL DEFAULT 0")
    throw new Error("this step breaks half way")
  }
  openStore(path, [good]).close()

  expect(() => openStore(path, [good, bad])).toThrow("this step breaks half way")

  const after = new Database(path)
  cleanups.push(async () => after.close())
  expect(version(after)).toBe(1)
  expect(after.query<{ name: string }, []>("PRAGMA table_info(note)").all().map((column) => column.name)).toEqual(["id", "text"])
})

test("a data folder from before projects had icons keeps its projects, each with no icon or colour until the owner picks them", async () => {
  const home = await tempFolder("evie-home-")
  const folder = await realpath(await tempFolder("evie-old-project-"))
  // The tables as the four steps before this one left them.
  const old = openStore(join(home, "evie.db"), MIGRATIONS.slice(0, 4))
  old.run("INSERT INTO project (id, name, folder, created) VALUES ('kept-project', 'old', ?, 1)", [folder])
  old.close()

  const run = await startEvie([scriptedProvider([]).plugin], { home })
  cleanups.push(run.stop)

  expect(await run.evie.send({ type: "list_projects" })).toContainEqual({ id: "kept-project", name: "old", folder, git: false })
  const db = new Database(join(home, "evie.db"))
  cleanups.push(async () => db.close())
  expect(db.query<{ name: string }, []>("PRAGMA table_info(project)").all().map((column) => column.name)).toEqual(["id", "name", "folder", "created", "icon", "colour"])
  expect(db.query("SELECT icon, colour FROM project WHERE id = 'kept-project'").get()).toEqual({ icon: null, colour: null })
})

test("the files attached to a session's messages are read from those messages alone, oldest first", async () => {
  const folder = await mkdtemp(join(tmpdir(), "evie-store-"))
  const db = openStore(join(folder, "evie.db"))
  try {
    const log = createEventLog(db)
    const plan = { path: "/p/.evie/attachments/2026-11-22/plan.txt", name: "plan.txt", type: "text/plain", size: 6 }
    const photo = { path: "/p/.evie/attachments/2026-11-22/photo.png", name: "photo.png", type: "image/png", size: 33 }
    log.append("s1", "user_message", { text: "Read this", attachments: [plan] })
    log.append("s1", "assistant_message", { text: 'It mentions "attachments": none.' })
    log.append("s1", "user_message", { text: "And this" })
    log.append("s1", "user_message", { text: "", attachments: [photo] })
    log.append("s2", "user_message", { text: "Another", attachments: [plan] })

    expect(log.attached("s1")).toEqual([plan, photo])
    expect(log.attached("s3")).toEqual([])
  } finally {
    db.close()
    await rm(folder, { recursive: true, force: true })
  }
})
