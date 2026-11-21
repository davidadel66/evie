import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdtemp, realpath, rm } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { openStore } from "../src/core/store.ts"

// The real programs, run as the owner runs them: `evie serve`, `evie` and
// `evie token`, each as a process of its own, with a temporary data folder.

const CLI = join(import.meta.dir, "..", "src", "cli.ts")

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// Runs `evie` with the given arguments to the end. Nothing of the real
// environment is passed on but the path, so the owner's own settings and
// data folder are never used.
async function evie(args: string[], options: { home: string; cwd?: string; input?: string; env?: Record<string, string> }) {
  const run = Bun.spawn([process.execPath, CLI, ...args], {
    cwd: options.cwd ?? options.home,
    env: { PATH: process.env.PATH ?? "", HOME: options.home, EVIE_HOME: options.home, ...options.env },
    stdin: options.input === undefined ? "ignore" : new Blob([options.input]),
    stdout: "pipe",
    stderr: "pipe",
  })
  const [stdout, stderr, code] = await Promise.all([new Response(run.stdout).text(), new Response(run.stderr).text(), run.exited])
  return { stdout, stderr, code }
}

// ---- evie token ------------------------------------------------------------------------------

test("tokens are made, listed and taken back by name, each without touching the others", async () => {
  const home = await tempFolder("evie-home-")

  const laptop = await evie(["token", "add", "laptop"], { home })
  expect(laptop.code).toBe(0)
  expect(laptop.stdout).toMatch(/^ {2}[A-Za-z0-9_-]{43}$/m)

  const again = await evie(["token", "add", "laptop"], { home })
  expect(again.code).toBe(1)
  expect(again.stderr).toContain('There is already a token named "laptop". Remove it first with `evie token remove laptop`.')

  expect((await evie(["token", "add", "my", "phone"], { home })).code).toBe(0)
  const both = await evie(["token", "list"], { home })
  expect(both.stdout).toContain("laptop")
  expect(both.stdout).toContain("my phone")

  expect((await evie(["token", "remove", "laptop"], { home })).code).toBe(0)
  const left = await evie(["token", "list"], { home })
  expect(left.stdout).not.toContain("laptop")
  expect(left.stdout).toContain("my phone")

  const gone = await evie(["token", "remove", "laptop"], { home })
  expect(gone.code).toBe(1)
  expect(gone.stderr).toContain('There is no token named "laptop".')
})

test("a token command waits while the server is writing to the database, instead of failing", async () => {
  const home = await tempFolder("evie-home-")
  openStore(join(home, "evie.db")).close()
  // Stands for the server in the middle of writing.
  const server = new Database(join(home, "evie.db"))
  server.run("BEGIN IMMEDIATE")

  const adding = evie(["token", "add", "laptop"], { home })
  await Bun.sleep(1500)
  server.run("COMMIT")
  server.close()

  const added = await adding
  expect(added.stderr).toBe("")
  expect(added.code).toBe(0)
})
