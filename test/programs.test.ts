import { afterEach, expect, test } from "bun:test"
import { Database } from "bun:sqlite"
import { mkdir, mkdtemp, readdir, readFile, realpath, rm, stat, utimes, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { basename, join } from "node:path"
import { openStore } from "../src/core/store.ts"
import { connect } from "../src/server/connect.ts"
import type { ScriptedReply } from "./support/scripted-provider.ts"

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

// A data folder whose settings load the scripted model, giving the replies
// in turn (a plain text for a reply of that text alone), and put the server
// on the given port.
async function homeWith(port: number, replies: (string | ScriptedReply)[]) {
  const home = await tempFolder("evie-home-")
  await scriptIn(home, port, replies)
  return home
}

// Writes the settings of a data folder made by `homeWith` again, with other
// replies, and any built-in plugins named.
async function scriptIn(home: string, port: number, replies: (string | ScriptedReply)[], builtIn: string[] = []) {
  const script = replies.map((reply) => (typeof reply === "string" ? { text: [reply] } : reply))
  await writeFile(join(home, "evie.json"), JSON.stringify({ model: "test/model", plugins: [{ use: SCRIPTED, options: { script } }, ...builtIn], server: { port } }))
}

// Starts `evie` in a project folder with its input kept open, to be typed
// into a line at a time, and gathers what it prints as it goes.
function evieTyping(home: string, project: string, env: Record<string, string> = {}) {
  const run = Bun.spawn([process.execPath, CLI], {
    cwd: project,
    env: { PATH: process.env.PATH ?? "", HOME: home, EVIE_HOME: home, ...env },
    stdin: "pipe",
    stdout: "pipe",
    stderr: "pipe",
  })
  cleanups.push(async () => {
    run.kill()
    await run.exited
  })
  const printed = { out: "", err: "" }
  const gather = async (from: ReadableStream<Uint8Array>, into: "out" | "err") => {
    const decoder = new TextDecoder()
    for await (const chunk of from) printed[into] += decoder.decode(chunk)
  }
  const done = Promise.all([gather(run.stdout, "out"), gather(run.stderr, "err")])
  return {
    run,
    printed,
    type(line: string) {
      run.stdin.write(`${line}\n`)
      run.stdin.flush()
    },
    // Settles once this has been printed.
    async shows(text: string) {
      while (!printed.out.includes(text)) await Bun.sleep(10)
    },
    // Ends the input, and settles with the exit code once everything printed has been read.
    async ends() {
      run.stdin.end()
      const code = await run.exited
      await done
      return code
    },
    async exited() {
      const code = await run.exited
      await done
      return code
    },
  }
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
  expect(after.stderr).toContain(`No evie server is running at http://127.0.0.1:${port}. Start it with \`evie serve\`.`)
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
  expect(refused.stderr).toContain(`The evie server at http://127.0.0.1:${port} did not accept the token.`)
})

test("this computer's token is kept when the server starts again, stays taken back once taken back, and is made anew when its file is removed", async () => {
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
  await restart()
  expect(await ownToken()).toBe(first)
  const server = await serve(home)
  const refused = await evie([], { home, cwd: project, input: "hello\n" })
  expect(refused.code).toBe(1)
  expect(refused.stderr).toContain(
    `The Evie server at http://127.0.0.1:${port} refused this computer's token. Removing ${join(home, "server-token")} and ` +
      "starting `evie serve` again makes a new one.",
  )

  server.kill("SIGTERM")
  await server.exited
  await rm(join(home, "server-token"))
  await serve(home)
  expect(await ownToken()).not.toBe(first)
  expect((await evie([], { home, cwd: project, input: "hello\n" })).stdout).toContain("Still here.")
})

test("this computer's token is sent only to a server on this computer", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["Hello, this computer."])
  const project = await tempFolder("evie-project-")
  await serve(home)

  const here = await evie([], { home, cwd: project, input: "hello\n", env: { EVIE_SERVER: `127.0.0.1:${port}` } })
  expect(here.stdout).toContain("Hello, this computer.")

  // An address that never resolves, so nothing is sent anywhere.
  const elsewhere = await evie([], { home, cwd: project, input: "hello\n", env: { EVIE_SERVER: "evie-server.invalid:3843" } })
  expect(elsewhere.code).toBe(1)
  expect(elsewhere.stderr).toContain(
    "EVIE_SERVER names another computer, so this computer's own token is not sent to it. Set EVIE_TOKEN to a token " +
      "made with `evie token add <name>` on the server's computer.",
  )
})

test("evie serve with a server setting it cannot use stops, and says which", async () => {
  const home = await tempFolder("evie-home-")
  await writeFile(join(home, "evie.json"), JSON.stringify({ server: { port: "the usual" } }))

  const served = await evie(["serve"], { home })

  expect(served.code).toBe(1)
  expect(served.stderr).toContain(`In ${join(home, "evie.json")}, "server" has a "port" that is not a whole number from 1 to 65535.`)
})

test("evie serve that cannot make this computer's token stops, and says why in one line", async () => {
  const home = await homeWith(await freePort(), [])
  // Stands for a data folder where the token file cannot be read or written.
  await mkdir(join(home, "server-token"))

  const served = await evie(["serve"], { home })

  expect(served.code).toBe(1)
  expect(served.stdout).toBe("")
  expect(served.stderr).toStartWith(`This computer's token could not be made: ${join(home, "server-token")} could not be read:`)
  expect(served.stderr.trim()).not.toContain("\n")
})

test("evie serve deletes saved output older than a week once it is listening, and one that cannot listen leaves it alone", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  const saved = join(home, "tool-output", "a-session", "call_1.txt")
  await mkdir(join(saved, ".."), { recursive: true })
  await writeFile(saved, "output")
  const eightDaysAgo = new Date(Date.now() - 8 * 24 * 60 * 60 * 1000)
  for (const path of [saved, join(saved, "..")]) await utimes(path, eightDaysAgo, eightDaysAgo)

  // Something else has the port, as another `evie serve` would.
  const other = Bun.serve({ hostname: "127.0.0.1", port, fetch: () => new Response() })
  const refused = await evie(["serve"], { home })
  await other.stop(true)

  expect(refused.code).toBe(1)
  expect(refused.stderr).toContain("Is another `evie serve` running?")
  expect(await readFile(saved, "utf8")).toBe("output")

  await serve(home)
  while ((await readdir(join(home, "tool-output"))).length > 0) await Bun.sleep(1)
})

test("evie in a real terminal that is told to stop puts the terminal back as it was", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  const project = await tempFolder("evie-project-")
  await serve(home)
  // Whether the terminal shows what is typed. It is off while Evie draws
  // its own input line, and the bit is the same on Linux and macOS.
  const ECHO = 0o10

  for (const [signal, code] of [["SIGTERM", 143], ["SIGHUP", 129]] as const) {
    let printed = ""
    const terminal = new Bun.Terminal({ cols: 80, rows: 24, data: (_terminal, data) => (printed += new TextDecoder().decode(data)) })
    cleanups.push(async () => terminal.close())
    const client = Bun.spawn([process.execPath, CLI], { cwd: project, env: { PATH: process.env.PATH ?? "", HOME: home, EVIE_HOME: home }, terminal })
    while (!printed.includes("evie · ")) await Bun.sleep(10)
    expect(terminal.localFlags & ECHO).toBe(0)

    client.kill(signal)

    expect(await client.exited).toBe(code)
    expect(terminal.localFlags & ECHO).toBe(ECHO)
  }
})

test("evie on a computer where no server has run yet says to start one", async () => {
  // On a port of its own, where nothing listens.
  const home = await homeWith(await freePort(), [])

  const said = await evie([], { home, input: "hello\n" })

  expect(said.code).toBe(1)
  expect(said.stderr).toContain("No Evie server has run on this computer yet. Start one with `evie serve`, which also makes this computer's token.")
})

test("evie with a server running but this computer's token file gone says the file is missing, and how to make a new one", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  await serve(home)
  await rm(join(home, "server-token"))

  const said = await evie([], { home, input: "hello\n" })

  expect(said.code).toBe(1)
  expect(said.stderr).toContain(
    `An Evie server is running at http://127.0.0.1:${port}, but this computer's token file, ${join(home, "server-token")}, is missing. ` +
      "Stopping that server and starting `evie serve` again makes a new one.",
  )
})

// ---- When the line to the server drops ----------------------------------------------------

const DROPPED = "[note] The connection to Evie's server dropped. Trying again…"
const BACK = "[note] Connected to Evie's server again."

test("when evie serve is stopped and started again, evie connects again by itself, saying so, and the next line is answered in the same session", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["First reply."])
  const project = await tempFolder("evie-project-")
  const server = await serve(home)
  const terminal = evieTyping(home, project)
  terminal.type("first")
  await terminal.shows("First reply.")

  server.kill("SIGTERM")
  await server.exited
  await terminal.shows(DROPPED)
  // The server started again is a new process, so its scripted model starts its replies again.
  await scriptIn(home, port, ["Second reply."])
  await serve(home)
  await terminal.shows(BACK)
  terminal.type("second")

  expect(await terminal.ends()).toBe(0)
  expect(terminal.printed.err).toBe("")
  expect(terminal.printed.out).toContain(`${DROPPED}\n${BACK}\n`)
  expect(terminal.printed.out).toContain("Second reply.")
  const db = new Database(join(home, "evie.db"), { readonly: true })
  const said = db.query<{ session: string; body: string }, []>("SELECT session, body FROM event WHERE type = 'user_message' ORDER BY time").all()
  db.close()
  expect(said.map((row) => JSON.parse(row.body).text)).toEqual(["first", "second"])
  expect(new Set(said.map((row) => row.session)).size).toBe(1)
})

test("evie run from a pipe or a script gives up when the server stays down, saying why, and leaves with an error", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["Hello."])
  const project = await tempFolder("evie-project-")
  const server = await serve(home)
  // Two seconds in place of two minutes, so that the test does not wait that long.
  const terminal = evieTyping(home, project, { EVIE_GIVE_UP_AFTER: "2" })
  terminal.type("hello")
  await terminal.shows("Hello.")

  server.kill("SIGTERM")
  await server.exited
  await terminal.shows(DROPPED)

  expect(await terminal.exited()).toBe(1)
  expect(terminal.printed.err).toBe(`No evie server is running at http://127.0.0.1:${port}. Start it with \`evie serve\`.\n`)
})

test("when this computer's token is taken back while the connection is down, evie stops trying, and says why", async () => {
  const port = await freePort()
  const home = await homeWith(port, ["Hello."])
  const project = await tempFolder("evie-project-")
  const server = await serve(home)
  const terminal = evieTyping(home, project)
  terminal.type("hello")
  await terminal.shows("Hello.")
  server.kill("SIGTERM")
  await server.exited
  await terminal.shows(DROPPED)

  expect((await evie(["token", "remove", "this computer"], { home })).code).toBe(0)
  await serve(home)

  expect(await terminal.exited()).toBe(1)
  expect(terminal.printed.err).toContain(
    `The Evie server at http://127.0.0.1:${port} refused this computer's token. Removing ${join(home, "server-token")} and ` +
      "starting `evie serve` again makes a new one.",
  )
})

test("Ctrl-C while the connection is down stops evie trying, and it leaves", async () => {
  const port = await freePort()
  const home = await homeWith(port, [{ hang: true, text: ["Thinking"] }])
  const project = await tempFolder("evie-project-")
  const server = await serve(home)
  const terminal = evieTyping(home, project)
  terminal.type("think hard")
  await terminal.shows("Thinking")
  // The turn was still running, as far as evie knows, when the connection dropped.
  server.kill("SIGTERM")
  await server.exited
  await terminal.shows(DROPPED)

  terminal.run.kill("SIGINT")

  expect(await terminal.exited()).toBe(130)
  expect(terminal.printed.out).toContain("[note] Stopped trying to reach Evie's server. `evie -c` comes back to this session.\n")
})

// ---- evie new --------------------------------------------------------------------------------

test("evie new makes a project through the server, in a folder named from where it is run, and prints what it made", async () => {
  const port = await freePort()
  // A server whose plugins leave out the sync plugin.
  const home = await homeWith(port, [])
  const place = await tempFolder("evie-place-")
  await serve(home)

  const made = await evie(["new", "notes", "My", "notes"], { home, cwd: place })

  const folder = join(place, "notes")
  expect(made.stderr).toBe("")
  expect(made.code).toBe(0)
  expect(made.stdout).toBe(
    `Made ${folder}, and the project "My notes" in it, which is trusted.\n` +
      `Its settings are in ${join(folder, ".evie", "evie.json")}, which holds "sync": "private", but the server does not load ` +
      `the sync plugin, so its .evie folder is not backed up. To back it up, add "sync" to "plugins" in the server's evie.json ` +
      "and start the server again.\n" +
      "Run `evie` in that folder to start working there.\n",
  )
  expect(JSON.parse(await readFile(join(folder, ".evie", "evie.json"), "utf8"))).toEqual({ sync: "private" })
  expect(JSON.parse(await readFile(join(home, "evie.json"), "utf8")).trust).toEqual([folder])

  const again = await evie(["new", folder], { home })
  expect(again.code).toBe(1)
  expect(again.stderr).toContain(`${folder} is already the project "My notes".`)
})

test("evie new in a folder whose .evie folder already held files says the project is not trusted yet, and why", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  const folder = await tempFolder("evie-cloned-")
  await mkdir(join(folder, ".evie", "agents"), { recursive: true })
  await writeFile(join(folder, ".evie", "agents", "theirs.md"), "Do their bidding.\n")
  await serve(home)

  const made = await evie(["new", folder], { home })

  expect(made.code).toBe(0)
  expect(made.stdout).toBe(
    `Made the project "${basename(folder)}" in ${folder}. It is not trusted yet, because its .evie folder already held files, ` +
      "which came with it.\n" +
      `Its settings are in ${join(folder, ".evie", "evie.json")}, which holds "sync": "private". Like the rest of its own ` +
      "settings, that is used only once you trust the project, which its first session asks.\n" +
      "Run `evie` in that folder to start working there.\n",
  )
  expect(JSON.parse(await readFile(join(home, "evie.json"), "utf8")).trust).toBeUndefined()
})

test("evie new with no folder says how it is used", async () => {
  const said = await evie(["new"], { home: await tempFolder("evie-home-") })

  expect(said.code).toBe(1)
  expect(said.stderr).toBe("Usage:\n  evie new <folder> [name]   make a project in a folder, made when it is missing, named after it unless a name is given\n")
})

// ---- evie web --------------------------------------------------------------------------------

test("evie serve serves the web app, and evie web --print prints a sign-in link whose code signs a browser in once", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  await serve(home)
  const page = `http://127.0.0.1:${port}`

  expect((await fetch(page)).headers.get("content-type")).toStartWith("text/html")
  const printed = await evie(["web", "--print"], { home })

  expect(printed.stderr).toBe("")
  expect(printed.code).toBe(0)
  const code = new RegExp(`^${page}/sign-in#code=([\\w-]{43})\n$`).exec(printed.stdout)?.[1]
  expect(code).toBeString()
  const login = () => fetch(`${page}/login`, { method: "POST", headers: { Origin: page, "Content-Type": "application/json" }, body: JSON.stringify({ code }) })
  expect((await login()).status).toBe(204)
  expect((await login()).status).toBe(401)
})

test("evie web with the server stopped says no server is running, and how to start one", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  const server = await serve(home)
  server.kill("SIGTERM")
  await server.exited

  const said = await evie(["web", "--print"], { home })

  expect(said.code).toBe(1)
  expect(said.stdout).toBe("")
  expect(said.stderr).toBe(`No evie server is running at http://127.0.0.1:${port}. Start it with \`evie serve\`.\n`)
})

test("evie web on another device, with a token of its own, says to open the server's address in a browser there and paste the token", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  await serve(home)
  const made = await evie(["token", "add", "phone"], { home })
  const secret = /^ {2}(\S{43})$/m.exec(made.stdout)![1]!
  const device = await tempFolder("evie-device-")

  const said = await evie(["web"], { home: device, env: { EVIE_SERVER: `127.0.0.1:${port}`, EVIE_TOKEN: secret } })

  expect(said.code).toBe(1)
  expect(said.stdout).toBe("")
  expect(said.stderr).toBe(
    `\`evie web\` works on the computer evie's server runs on. Here, open http://127.0.0.1:${port} in a browser and paste this device's token.\n`,
  )
})

test("evie serve --dev started outside the repository's folder says where to start it, since Bun reads the Tailwind plugin from there", async () => {
  const home = await homeWith(await freePort(), [])

  const said = await evie(["serve", "--dev"], { home })

  expect(said.code).toBe(1)
  expect(said.stderr).toBe(
    `Start \`evie serve --dev\` in the repository's folder, ${join(import.meta.dir, "..")}: Bun takes the web app's Tailwind plugin from the bunfig.toml there.\n`,
  )
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

test("the name of this computer's own token is kept for the server, which makes that token itself", async () => {
  const home = await tempFolder("evie-home-")

  const added = await evie(["token", "add", "this", "computer"], { home })

  expect(added.code).toBe(1)
  expect(added.stderr).toContain('"this computer" is the name of the token `evie serve` makes for this computer itself. Choose another name.')
  expect((await evie(["token", "list"], { home })).stdout).toContain("There are no tokens.")
})

test("a token command whose database cannot be opened stops, and says why in one line", async () => {
  const home = await tempFolder("evie-home-")
  // Stands for a database that cannot be opened.
  await mkdir(join(home, "evie.db"))

  const listed = await evie(["token", "list"], { home })

  expect(listed.code).toBe(1)
  expect(listed.stdout).toBe("")
  expect(listed.stderr).toContain("database")
  expect(listed.stderr.trim()).not.toContain("\n")
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

// ---- Stopping, for a restart or for good ------------------------------------------------------

// A data folder and a project where `evie serve` has started a background
// command, and an `evie` that asked for it, left connected.
async function commandRunning(port: number, command: string) {
  const home = await tempFolder("evie-home-")
  await scriptIn(home, port, [{ text: [], calls: [{ name: "shell", input: { command, background: true } }] }, "Started."], ["shell", "jobs"])
  const project = await tempFolder("evie-project-")
  const server = await serve(home)
  const terminal = evieTyping(home, project)
  terminal.type("run it")
  await terminal.shows("Started.")
  const db = reading(home)
  const session = db.query<{ session: string }, []>("SELECT session FROM event WHERE type = 'user_message'").get()!.session
  db.close()
  const output = join(home, "tool-output", session, "job-1.log")
  return { home, server, session, output }
}

// The database of a data folder, to read while the server may be writing:
// a read that finds it busy waits, as the server's own do.
function reading(home: string) {
  const db = new Database(join(home, "evie.db"), { readonly: true })
  db.run("PRAGMA busy_timeout = 5000")
  return db
}

// The notes stored in a session, once there are as many as asked for.
async function notesOnceThere(home: string, session: string, count: number) {
  for (;;) {
    const db = reading(home)
    const notes = db.query<{ body: string }, [string]>("SELECT body FROM event WHERE session = ? AND type = 'note' ORDER BY seq").all(session)
    db.close()
    if (notes.length >= count) return notes.map((note) => JSON.parse(note.body).text as string)
    await Bun.sleep(20)
  }
}

// Whether any process of a group is there. Signal 0 only asks.
const groupThere = (group: number) => {
  try {
    process.kill(-group, 0)
    return true
  } catch {
    return false
  }
}

test("evie serve stopped as a restart stops it leaves a background command running, and the next evie serve tells of its end", async () => {
  const port = await freePort()
  const { home, server, session, output } = await commandRunning(port, "sleep 1; echo built")

  server.kill("SIGTERM")
  expect(await server.exited).toBe(143)
  await scriptIn(home, port, ["The build is done."], ["shell", "jobs"])
  await serve(home)

  expect(await notesOnceThere(home, session, 1)).toEqual([`Background job 1 (sleep 1; echo built) has finished.\n\nbuilt\n\nThe full output is in ${output}.`])
})

test("evie serve removes a stop-for-good file left from before it started, so a restart of it is not taken for a stop for good", async () => {
  const port = await freePort()
  const home = await homeWith(port, [])
  // Left by an uninstall while no server was running to read it.
  await writeFile(join(home, "stop-for-good"), "")

  await serve(home)

  expect(await Bun.file(join(home, "stop-for-good")).exists()).toBe(false)
})

test("evie serve stopped with Ctrl-C, by closing its terminal, or with a stop-for-good file in its data folder, stops its background commands", async () => {
  const codes = { "Ctrl-C": 130, "terminal closed": 129, "stop-for-good": 143 }
  for (const how of ["Ctrl-C", "terminal closed", "stop-for-good"] as const) {
    const port = await freePort()
    const { home, server, output } = await commandRunning(port, "ps -o pgid= -p $$ | tr -d ' '; sleep 30")
    while (!(await readFile(output, "utf8")).includes("\n")) await Bun.sleep(5)
    const group = Number((await readFile(output, "utf8")).split("\n")[0])
    cleanups.push(async () => void (groupThere(group) && process.kill(-group, "SIGKILL")))

    if (how === "Ctrl-C") server.kill("SIGINT")
    else if (how === "terminal closed") server.kill("SIGHUP")
    else {
      await writeFile(join(home, "stop-for-good"), "")
      server.kill("SIGTERM")
    }

    expect(await server.exited).toBe(codes[how])
    expect(groupThere(group)).toBe(false)
    expect(await Bun.file(join(home, "stop-for-good")).exists()).toBe(false)
  }
})

test("evie serve run by hand starts again in its own place when a restart is asked for: running work is picked up again, and Ctrl-C then stops it all", async () => {
  const port = await freePort()
  const { home, server, session, output } = await commandRunning(port, "ps -o pgid= -p $$ | tr -d ' '; sleep 30")
  while (!(await readFile(output, "utf8")).includes("\n")) await Bun.sleep(5)
  const group = Number((await readFile(output, "utf8")).split("\n")[0])
  cleanups.push(async () => void (groupThere(group) && process.kill(-group, "SIGKILL")))
  const client = await connect({ url: `http://127.0.0.1:${port}`, token: (await readFile(join(home, "server-token"), "utf8")).trim(), firstWait: 50 })
  cleanups.push(() => client.close())
  const { started } = await client.send({ type: "read_settings" })

  await client.send({ type: "restart_server", started })

  // The client connects again by itself, to the server started in its place.
  let now = started
  while (now === started) now = (await client.send({ type: "read_settings" })).started
  const opened = await client.send({ type: "open_session", session })
  expect(opened.jobs).toMatchObject([{ id: "1", status: "running" }])
  expect(groupThere(group)).toBe(true)

  server.kill("SIGINT")
  expect(await server.exited).toBe(130)
  expect(groupThere(group)).toBe(false)
  expect(await fetch(`http://127.0.0.1:${port}/`).then(() => "answered", () => "nothing there")).toBe("nothing there")
})
