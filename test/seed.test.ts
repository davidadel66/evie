import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { existsSync } from "node:fs"
import { mkdir, mkdtemp, readdir, readFile, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createGuard } from "../src/plugins/guard.ts"

// `evie seed`, run as the owner runs it: a process of its own, with a
// temporary data folder, no key and nothing of the real environment but the path.

const CLI = join(import.meta.dir, "..", "src", "cli.ts")

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// A temporary data folder. The seed puts its projects beside it, in the
// same name with "-seed" added, which goes with it.
async function tempFolder() {
  const folder = await realpath(await mkdtemp(join(tmpdir(), "evie-seed-")))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  cleanups.push(() => rm(`${folder}-seed`, { recursive: true, force: true }))
  return folder
}

// Runs `evie` with `home` as both the home folder and the data folder,
// unless `env` says otherwise.
async function evie(args: string[], home: string, env: Record<string, string | undefined> = {}) {
  const run = Bun.spawn([process.execPath, CLI, ...args], {
    cwd: home,
    env: { PATH: process.env.PATH ?? "", HOME: home, EVIE_HOME: home, ...env },
    stdin: "ignore",
    stdout: "pipe",
    stderr: "pipe",
  })
  const [stdout, stderr, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
  return { stdout, stderr, code }
}

type Row = { type: string; body: string }
type Event = { type: string; body: Record<string, any> }

// What a seeded session shows, read from its record in the database.
function kindOf(db: Database, session: { id: string; parent: string | null }): string {
  const events: Event[] = db
    .query<Row, [string]>("SELECT type, body FROM event WHERE session = ? ORDER BY seq")
    .all(session.id)
    .map((row) => ({ type: row.type, body: JSON.parse(row.body) }))
  const calls: { name: string; arguments: string }[] = events.flatMap((event) => (event.type === "assistant_message" ? (event.body.toolCalls ?? []) : []))
  const answered = new Set(events.flatMap((event) => (event.type === "answer" ? [event.body.id] : [])))
  if (session.parent !== null) return "a subagent's own conversation"
  if (events.some((event) => event.type === "compaction")) return "a compacted conversation"
  if (events.some((event) => event.type === "question" && !answered.has(event.body.id))) return "a question still waiting"
  if (events.some((event) => event.type === "answer" && event.body.choice !== null)) return "a question answered"
  if (calls.some((call) => call.name === "subagent")) return "one that started a subagent"
  const inBackground = calls.some((call) => call.name === "shell" && JSON.parse(call.arguments).background === true)
  const finished = events.some((event) => event.type === "note" && String(event.body.text).includes("has finished"))
  if (inBackground && finished) return "a background job that finished"
  if (events.some((event) => event.type === "tool_result")) return "tool calls"
  return "a plain conversation"
}

test("evie seed fills an empty data folder with made-up projects and sessions of every kind, at no cost", async () => {
  const home = await tempFolder()

  const seeded = await evie(["seed"], home)

  expect(seeded.stderr).toBe("")
  expect(seeded.code).toBe(0)
  const db = new Database(join(home, "evie.db"), { readonly: true })
  cleanups.push(async () => db.close())

  // The projects are git repositories that the seed made beside the data
  // folder, where the guard lets the tools work in them, and the sessions
  // are in them.
  const made = (await readdir(`${home}-seed`)).map((name) => join(`${home}-seed`, name))
  expect(made.length).toBeGreaterThanOrEqual(2)
  const guard = createGuard({ dataFolder: home })
  for (const folder of made) {
    expect(existsSync(join(folder, ".git"))).toBe(true)
    expect(guard({ id: "1", name: "read", input: { path: "README.md" } }, folder)).toBeUndefined()
  }
  const projects = db.query<{ folder: string }, []>("SELECT folder FROM project").all()
  expect(projects.map((project) => project.folder).sort()).toEqual(made.sort())

  const sessions = db.query<{ id: string; parent: string | null }, []>("SELECT id, parent FROM session").all()
  expect(sessions.map((session) => kindOf(db, session)).sort()).toEqual(
    [
      "a plain conversation",
      "tool calls",
      "a question answered",
      "a background job that finished",
      "one that started a subagent",
      "a subagent's own conversation",
      "a compacted conversation",
    ].sort(),
  )

  // Every turn that ended went as its script says.
  const ends = db.query<Row, []>("SELECT type, body FROM event WHERE type = 'turn_end'").all()
  expect(ends.map((row) => JSON.parse(row.body).reason)).not.toContain("error")
  expect(ends.length).toBeGreaterThan(0)
})

test("evie seed refuses a data folder that already has sessions, and leaves it as it was", async () => {
  const home = await tempFolder()
  expect((await evie(["seed"], home)).code).toBe(0)
  const count = () => {
    const db = new Database(join(home, "evie.db"), { readonly: true })
    const { sessions } = db.query<{ sessions: number }, []>("SELECT COUNT(*) AS sessions FROM session").get()!
    db.close()
    return sessions
  }
  const before = count()

  const again = await evie(["seed"], home)

  expect(again.code).toBe(1)
  expect(again.stderr).toContain(`${home} already has sessions, so nothing was seeded.`)
  expect(count()).toBe(before)
})

test("evie seed refuses to fill the everyday data folder: EVIE_HOME must name another", async () => {
  const home = await tempFolder()
  const everyday = join(home, ".evie")

  const unset = await evie(["seed"], home, { EVIE_HOME: undefined })
  expect(unset.code).toBe(1)
  expect(unset.stderr).toContain(`EVIE_HOME is not set, so the seed would fill the everyday data folder, ${everyday}.`)

  const named = await evie(["seed"], home, { EVIE_HOME: everyday })
  expect(named.code).toBe(1)
  expect(named.stderr).toContain(`EVIE_HOME names the everyday data folder, ${everyday}.`)

  expect(existsSync(everyday)).toBe(false)
  expect(existsSync(`${everyday}-seed`)).toBe(false)
})

test("evie seed refuses when one of its project folders is already there, and leaves it alone", async () => {
  const home = await tempFolder()
  const taken = join(`${home}-seed`, "recipe-box")
  await mkdir(taken, { recursive: true })
  await writeFile(join(taken, "notes.txt"), "someone's notes")

  const seeded = await evie(["seed"], home)

  expect(seeded.code).toBe(1)
  expect(seeded.stderr).toContain(`${taken} is already there, so nothing was seeded. Remove it first`)
  expect(await readFile(join(taken, "notes.txt"), "utf8")).toBe("someone's notes")
  expect(await readdir(`${home}-seed`)).toEqual(["recipe-box"])
})
