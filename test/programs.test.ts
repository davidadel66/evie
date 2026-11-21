import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdtemp, readFile, realpath, rm, stat, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { openStore } from "../src/core/store.ts"

// The real programs, run as the owner runs them: `evie serve`, `evie` and
// `evie token`, each as a process of its own, with a temporary data folder.

const CLI = join(import.meta.dir, "..", "src", "cli.ts")
const SCRIPTED = join(import.meta.dir, "support", "scripted-plugin.ts")

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

// A port that was free a moment ago.
async function freePort() {
  const probe = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response() })
  const port = probe.port!
  await probe.stop(true)
  return port
}

// A data folder whose settings load the scripted model, replying with the
// given texts in turn, and put the server on the given port.
async function homeWith(port: number, replies: string[]) {
  const home = await tempFolder("evie-home-")
  const script = replies.map((text) => ({ text: [text] }))
  await writeFile(join(home, "evie.json"), JSON.stringify({ model: "test/model", plugins: [{ use: SCRIPTED, options: { script } }], server: { port } }))
  return home
}

// Starts `evie serve` and resolves once it is listening. What it prints is
// read for as long as it runs, so it never finds its output closed.
async function serve(home: string) {
  const run = Bun.spawn([process.execPath, CLI, "serve"], {
    cwd: home,
    env: { PATH: process.env.PATH ?? "", HOME: home, EVIE_HOME: home },
    stdout: "pipe",
    stderr: "pipe",
  })
  cleanups.push(async () => {
    run.kill()
    await run.exited
  })
  let printed = ""
  const listening = new Promise<void>((resolve, reject) => {
    void (async () => {
      const decoder = new TextDecoder()
      for await (const chunk of run.stdout) {
        printed += decoder.decode(chunk)
        if (printed.includes("listening on")) resolve()
      }
      reject(new Error(`evie serve stopped before it listened: ${printed}${await new Response(run.stderr).text()}`))
    })()
  })
  await listening
  return run
}

// ---- evie serve and evie --------------------------------------------------------------------

test("evie serve runs Evie, and evie in a project folder sends it a message and shows the reply", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["Hello from the server."])
  const project = await tempFolder("evie-project-")
  const server = await serve(home)

  const said = await evie([], { home, cwd: project, input: "hello\n" })

  expect(said.stderr).toBe("")
  expect(said.code).toBe(0)
  expect(said.stdout).toContain("Hello from the server.")
  // This computer's own token, which only the owner's user can read.
  expect((await stat(join(home, "server-token"))).mode & 0o777).toBe(0o600)

  server.kill("SIGTERM")
  expect(await server.exited).toBe(143)
  const after = await evie([], { home, cwd: project, input: "hello\n" })
  expect(after.code).toBe(1)
  expect(after.stderr).toContain(`No Evie server is running at http://127.0.0.1:${port}. Start it with \`evie serve\`.`)
})

test("another device connects with EVIE_SERVER and a token of its own, until the token is taken back", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["Hello, phone."])
  await serve(home)
  // Made while the server has the database open.
  const made = await evie(["token", "add", "phone"], { home })
  const secret = /^ {2}(\S{43})$/m.exec(made.stdout)![1]!
  // The other device has a data folder of its own, with none of this one's settings.
  const device = await tempFolder("evie-device-")
  const reach = { EVIE_SERVER: `127.0.0.1:${port}`, EVIE_TOKEN: secret }

  const said = await evie([], { home: device, input: "hello\n", env: reach })
  expect(said.stdout).toContain("Hello, phone.")

  expect((await evie(["token", "remove", "phone"], { home })).code).toBe(0)
  const refused = await evie([], { home: device, input: "hello\n", env: reach })
  expect(refused.code).toBe(1)
  expect(refused.stderr).toContain(`The Evie server at http://127.0.0.1:${port} did not accept the token.`)
})

test("this computer's token is kept when the server starts again, and made anew once it has been taken back", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["Still here."])
  const project = await tempFolder("evie-project-")
  const ownToken = () => readFile(join(home, "server-token"), "utf8")
  const restart = async () => {
    const server = await serve(home)
    server.kill("SIGTERM")
    await server.exited
  }

  await restart()
  const first = await ownToken()
  await restart()
  expect(await ownToken()).toBe(first)

  expect((await evie(["token", "remove", "this computer"], { home })).code).toBe(0)
  await serve(home)
  expect(await ownToken()).not.toBe(first)
  expect((await evie([], { home, cwd: project, input: "hello\n" })).stdout).toContain("Still here.")
})

test("evie serve with a server setting it cannot use stops, and says which", async () => {
  const home = await tempFolder("evie-home-")
  await writeFile(join(home, "evie.json"), JSON.stringify({ server: { port: "the usual" } }))

  const served = await evie(["serve"], { home })

  expect(served.code).toBe(1)
  expect(served.stderr).toContain(`In ${join(home, "evie.json")}, "server" has a "port" that is not a whole number from 1 to 65535.`)
})

test("evie on a computer with no token yet says how to get one", async () => {
  const home = await tempFolder("evie-home-")

  const said = await evie([], { home, input: "hello\n" })

  expect(said.code).toBe(1)
  expect(said.stderr).toContain("There is no token for this computer yet. `evie serve` makes one the first time it starts.")
})

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
