#!/usr/bin/env bun
// The `evie` command.
//
//   evie serve                 runs Evie: the core behind its server
//   evie                       the terminal client, in the folder it is run
//                              in; it connects to the server
//   evie token add <name>      makes a token for a device; list and remove
//                              list them and take one back
import { mkdir } from "node:fs/promises"
import { createInterface } from "node:readline"
import { join } from "node:path"
import { openStore } from "./core/store.ts"
import { connect, createEvie, createLiveInput, dataFolder, loadSettingsFiles, loadSettings, runTerminal, startServer, type EvieServer } from "./index.ts"
import { loadServerSettings } from "./settings-files.ts"
import { createTokens, ensureOwnToken, readOwnToken } from "./server/tokens.ts"

// Keys come from the environment and the .env file in the data folder.
const env = await loadSettings(process.env)

// A problem that stops the command, such as a plugin that cannot be loaded,
// is printed with its reason.
const fail = (cause: unknown): never => {
  console.error(cause instanceof Error ? cause.message : String(cause))
  process.exit(1)
}

// Prints to the terminal. If whatever was reading Evie's output has gone
// away, as when it is piped into a program that stops early, there is no one
// left to print for, so Evie quits quietly.
function print(text: string) {
  try {
    process.stdout.write(text)
  } catch (cause) {
    if ((cause as { code?: string }).code === "EPIPE") process.exit(0)
    throw cause
  }
}
process.stdout.on("error", (cause: { code?: string }) => {
  if (cause.code === "EPIPE") process.exit(0)
})

const [command, ...rest] = process.argv.slice(2)
if (command === "serve") await serve()
else if (command === "token") await token(rest)
else await terminal()

// ---- evie serve ---------------------------------------------------------------------------

// Starts the core with the plugins the settings list, and serves it until
// told to stop. The first time, it makes this computer's token.
async function serve() {
  const settings = await loadSettingsFiles({ env }).catch(fail)
  const where = await loadServerSettings({ env }).catch(fail)
  const evie = await createEvie({
    env,
    plugins: settings.plugins,
    projectSettings: settings.projectSettings,
    settingsFile: settings.file,
    ...(settings.model !== undefined && { model: settings.model }),
    ...(settings.maxTokens !== undefined && { maxTokens: settings.maxTokens }),
    ...(settings.compaction !== undefined && { compaction: settings.compaction }),
  }).catch(fail)

  const home = dataFolder(env)
  const db = openStore(join(home, "evie.db"))
  const tokens = createTokens(db)
  await ensureOwnToken(tokens, home)

  let server: EvieServer
  try {
    server = startServer({ evie, tokens, host: where.host, port: where.port, origins: where.origins })
  } catch (cause) {
    await evie.close()
    return fail(
      `Evie could not listen on ${where.host}, port ${where.port}: ${cause instanceof Error ? cause.message : String(cause)} ` +
        `Is another \`evie serve\` running? The port can be changed under "server" in ${settings.file}.`,
    )
  }
  print(`evie serve · listening on ${server.url}\n`)

  // Being told to stop closes Evie properly: turns are stopped, and so is any
  // command left running in the background. Told again while that is under
  // way, it stops at once.
  let stopping = false
  for (const [signal, code] of [["SIGINT", 130], ["SIGTERM", 143], ["SIGHUP", 129]] as const) {
    process.on(signal, () => {
      if (stopping) process.exit(code)
      stopping = true
      void server
        .stop()
        .then(() => evie.close())
        .finally(() => {
          db.close()
          process.exit(code)
        })
    })
  }
}

// ---- evie token ---------------------------------------------------------------------------

// Makes, lists and takes back the tokens devices connect with. It works on
// the data folder directly, so it works whether or not the server is
// running, and a token taken back is refused by a server already running.
async function token([action, ...words]: string[]) {
  const name = words.join(" ").trim()
  const home = dataFolder(env)
  await mkdir(home, { recursive: true })
  const tokens = createTokens(openStore(join(home, "evie.db")))

  if (action === "add" && name !== "") {
    const made = tokens.add(name)
    if ("refused" in made) fail(made.refused)
    else {
      print(
        `A token for "${name}". It is shown only this once, so copy it now:\n\n  ${made.secret}\n\n` +
          `On that device, set EVIE_TOKEN to it, and EVIE_SERVER to this server's address.\n`,
      )
    }
  } else if (action === "list" && name === "") {
    const list = tokens.list()
    if (list.length === 0) print("There are no tokens. Make one with `evie token add <name>`.\n")
    const width = Math.max(0, ...list.map((listed) => listed.name.length))
    for (const listed of list) print(`${listed.name.padEnd(width)}   made ${new Date(listed.created).toISOString().slice(0, 16).replace("T", " ")} UTC\n`)
  } else if (action === "remove" && name !== "") {
    if (!tokens.remove(name)) fail(`There is no token named "${name}". \`evie token list\` lists them.`)
    print(`The token for "${name}" is taken back. A device using it can no longer connect.\n`)
  } else {
    fail(
      [
        "Usage:",
        '  evie token add <name>      make a token for a device, such as "phone"',
        "  evie token list            list the tokens there are",
        "  evie token remove <name>   take a device's token back",
      ].join("\n"),
    )
  }
}

// ---- evie ---------------------------------------------------------------------------------

// Connects to the server and runs the terminal client over the connection.
// The server is at the address in the settings, or `EVIE_SERVER`; the token
// is `EVIE_TOKEN`, or this computer's own.
async function terminal() {
  const where = await loadServerSettings({ env }).catch(fail)
  const token =
    env.EVIE_TOKEN?.trim() ||
    (await readOwnToken(dataFolder(env))) ||
    fail(
      "There is no token for this computer yet. `evie serve` makes one the first time it starts. To reach a server " +
        "on another computer, set EVIE_TOKEN to a token made there with `evie token add <name>`.",
    )
  // Puts the terminal back as it was, when the live input line has taken it over.
  let restore = () => {}
  const evie = await connect({
    url: where.address,
    token,
    // The server stopped, or took this device's token back. Connecting again
    // is not built yet, so the terminal says so and leaves.
    onDrop: (reason) => {
      restore()
      console.error(`\n[error] The connection to the Evie server was lost: ${reason}. \`evie --continue\` carries on with this session.`)
      process.exit(1)
    },
  }).catch(fail)

  // Leaving stops nothing in the server: being told to stop, or the terminal
  // window closing, closes the connection only.
  for (const signal of ["SIGTERM", "SIGHUP"] as const) {
    process.on(signal, () => {
      void evie.close().finally(() => process.exit(signal === "SIGTERM" ? 143 : 129))
    })
  }

  const session = { folder: process.cwd(), resume: process.argv.includes("--continue") || process.argv.includes("-c") }
  // What the terminal client does on Ctrl-C: cancel the turn, or say that
  // nothing is running, or that a cancel is already under way.
  let interrupt = (): "cancelled" | "idle" | "again" => "idle"

  let problem: unknown
  try {
    if (process.stdin.isTTY && process.stdout.isTTY) {
      // A real terminal: keys arrive one at a time, and the line being typed is
      // kept at the bottom with Evie's output above it.
      const live = createLiveInput({
        write: print,
        columns: () => process.stdout.columns || 80,
      })
      live.onInterrupt(() => {
        const did = interrupt()
        // With nothing running, Ctrl-C quits. Pressed again while a turn is
        // refusing to stop, it quits at once.
        if (did === "idle") live.close()
        if (did === "again") {
          process.stdin.setRawMode(false)
          process.exit(130)
        }
      })
      restore = () => {
        live.close()
        process.stdin.setRawMode(false)
      }
      process.stdin.setRawMode(true)
      process.stdin.setEncoding("utf8")
      process.stdin.on("data", (data: string) => live.key(data))
      try {
        await runTerminal({
          evie,
          ...session,
          lines: live.lines,
          write: live.write,
          prompt: false,
          typing: live.typing,
          onInterrupt: (handler) => (interrupt = handler),
        })
      } finally {
        live.close()
        process.stdin.setRawMode(false)
        process.stdin.pause()
      }
    } else {
      // Not a terminal, such as input from a pipe: read whole lines.
      const input = createInterface({ input: process.stdin })
      const onCtrlC = () => {
        const did = interrupt()
        if (did === "idle") input.close()
        if (did === "again") process.exit(130)
      }
      input.on("SIGINT", onCtrlC)
      process.on("SIGINT", onCtrlC)
      await runTerminal({
        evie,
        ...session,
        lines: input,
        write: print,
        // Nobody is typing, so a line must not be taken as an answer.
        questions: false,
        // So that piped work is seen to its end: the turn, any command moved
        // to the background, and what Evie made of it.
        waitForJobs: true,
        onInterrupt: (handler) => (interrupt = handler),
      })
    }
  } catch (cause) {
    // A session that could not be opened, such as when no model is set.
    problem = cause
  } finally {
    // The connection only. Anything still running carries on in the server.
    await evie.close()
  }
  if (problem !== undefined) fail(problem)
}
