import { afterEach, expect, test } from "bun:test"
import { mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { loadSettings, loadSettingsFiles, startServer } from "../src/index.ts"
import { openTokens } from "../src/server/tokens.ts"
import { settingsEditor } from "../src/settings-editor.ts"
import { openConnection } from "../web/src/connection.ts"
import { keyState, onThisComputer, openServerSettings, pluginState, projectState, type ServerSettings, type SettingsState } from "../web/src/server-settings.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// The web app's settings window, apart from its screen: the settings read
// from the server once connected, a change sent and what the server said,
// and a restart, followed until the server started in its place answers.
// Every key is made up.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// Evie and its server on a data folder, as `evie serve` starts them, on
// the port given or any free one. `restart` is what a restart asked for
// does. `elsewhere` makes every connection one from another device.
async function serveIn(home: string, port = 0, restart = () => {}, elsewhere = false) {
  const real = { EVIE_HOME: home }
  const env = await loadSettings(real)
  const files = await loadSettingsFiles({ env })
  const settings = settingsEditor({ env, environment: real, files, restart })
  const run = await startEvie([scriptedProvider([]).plugin, ...files.plugins], { home, settings, projectSettings: files.projectSettings, trust: files.trust })
  const tokens = await openTokens(home)
  const server = startServer({ evie: run.evie, tokens, host: "127.0.0.1", port, ...(elsewhere && { fromThisComputer: () => false }) })
  let stopped = false
  const stop = async () => {
    if (stopped) return
    stopped = true
    await server.stop()
    tokens.close()
    await run.stop()
  }
  cleanups.push(stop)
  return { tokens, server, stop }
}

async function dataFolder() {
  const home = await tempFolder("evie-home-")
  await writeFile(join(home, "evie.json"), JSON.stringify({ plugins: ["ask", "openrouter", "web"] }))
  return home
}

function follow(url: string, token: string) {
  const connection = openConnection({ url, token, firstWait: 10 })
  cleanups.push(() => connection.close())
  const settings = openServerSettings(connection)
  cleanups.push(async () => settings.close())
  return settings
}

// Settles once the settings are as `ready` says, and gives them as they are then.
async function until(settings: ServerSettings, ready: (state: SettingsState) => boolean) {
  while (!ready(settings.state())) await Bun.sleep(5)
  return settings.state()
}

test("once connected, the settings are read from the server, and a change made gives them as they are after it", async () => {
  const home = await dataFolder()
  const { tokens, server } = await serveIn(home)
  const made = tokens.add("test device") as { secret: string }
  const settings = follow(server.url, made.secret)

  const read = await until(settings, (state) => state.view !== undefined)
  expect(read.view!.keys[0]).toMatchObject({ name: "OPENROUTER_API_KEY", inFile: false, environment: false, pending: false })

  expect(await settings.change({ key: "OPENROUTER_API_KEY", value: "made-up-openrouter" })).toBeUndefined()
  expect(settings.state().view!.keys[0]).toMatchObject({ name: "OPENROUTER_API_KEY", inFile: true, environment: false, pending: true })
  expect(settings.state().view!.restart).toBe(true)
})

test("a change the server refuses gives back its words, and the settings stay as they were", async () => {
  const home = await dataFolder()
  const { tokens, server } = await serveIn(home)
  const made = tokens.add("test device") as { secret: string }
  const settings = follow(server.url, made.secret)
  const before = (await until(settings, (state) => state.view !== undefined)).view

  const said = await settings.change({ addPlugin: "/made/up/missing.ts" })

  expect(said).toStartWith('The plugin "/made/up/missing.ts" could not be loaded:')
  expect(settings.state().view).toBe(before)
})

test("a restart is under way from when it is asked for until the server started in its place answers, whose settings are then read", async () => {
  const home = await dataFolder()
  const first = await serveIn(home, 0, () => {
    void (async () => {
      const port = Number(new URL(first.server.url).port)
      await first.stop()
      await serveIn(home, port)
    })()
  })
  const made = first.tokens.add("test device") as { secret: string }
  const settings = follow(first.server.url, made.secret)
  const before = (await until(settings, (state) => state.view !== undefined)).view!

  expect(await settings.restart()).toBeUndefined()
  expect(settings.state().restarting).toBe(true)

  const after = await until(settings, (state) => !state.restarting)
  expect(after.view!.started).not.toBe(before.started)
})

test("each setting says in words how it stands, and when a change to it waits for the server to start again", () => {
  const key = { name: "OPENROUTER_API_KEY", inFile: true, environment: false, pending: false }
  expect(keyState(key)).toEqual({ word: "Set" })
  expect(keyState({ ...key, inFile: false, pending: true })).toEqual({ word: "Not set", note: "Applies once evie's server starts again." })
  expect(keyState({ ...key, inFile: false, environment: true })).toEqual({ word: "Set by the environment evie's server runs in", note: "That wins over a value saved here." })
  expect(keyState({ ...key, environment: true })).toEqual({
    word: "Set",
    note: "The environment evie's server runs in sets it too, and wins over this. Clearing it here does not unset it there.",
  })

  const plugin = { name: "/made/up/greeter.ts", listed: true, loaded: true, added: true }
  expect(pluginState(plugin)).toEqual({ word: "Loaded" })
  expect(pluginState({ ...plugin, loaded: false })).toEqual({ word: "Not loaded yet", note: "Loads once evie's server starts again." })
  expect(pluginState({ ...plugin, listed: false })).toEqual({ word: "Removed", note: "Still loaded until evie's server starts again." })

  const project = { id: "p1", name: "garden", folder: "/made/up/garden", trusted: true, pending: false }
  expect(projectState(project)).toEqual({ word: "Trusted" })
  expect(projectState({ ...project, trusted: false, pending: true })).toEqual({ word: "Not trusted", note: "Its open sessions keep their settings until evie's server starts again." })
})

test("from another device, keys, plugin files and trusted projects are refused, saying where they can be changed; the instructions and a restart are not", async () => {
  const home = await dataFolder()
  const { tokens, server } = await serveIn(home, 0, () => {}, true)
  const made = tokens.add("phone") as { secret: string }
  const settings = follow(server.url, made.secret)
  const { view } = await until(settings, (state) => state.view !== undefined)
  const here = "Keys, plugins and trusted projects can be changed only on the computer evie runs on."

  expect(await settings.change({ key: "OPENROUTER_API_KEY", value: "made-up" })).toBe(here)
  expect(await settings.change({ addPlugin: "/made/up/plugin.ts" })).toBe(here)
  expect(await settings.change({ removePlugin: "/made/up/plugin.ts" })).toBe(here)
  expect(await settings.change({ project: "made-up", trusted: true })).toBe(here)
  expect(settings.state().view).toBe(view)

  expect(await settings.change({ instructions: "Answer briefly." })).toBeUndefined()
  expect(settings.state().view!.instructions).toBe("Answer briefly.")
})

test("a page is on the computer evie runs on when its own address is", () => {
  expect(["http://127.0.0.1:3843", "http://localhost:3843", "http://[::1]:3843"].map(onThisComputer)).toEqual([true, true, true])
  expect(["http://my-mac.local:3843", "http://192.168.1.20:3843", "https://evie.example.com"].map(onThisComputer)).toEqual([false, false, false])
})

test("instructions being written are kept until saved, whatever a later read brings; with none being written, the text follows the server's", async () => {
  const home = await dataFolder()
  const { tokens, server } = await serveIn(home)
  const here = follow(server.url, (tokens.add("browser") as { secret: string }).secret)
  const elsewhere = follow(server.url, (tokens.add("laptop") as { secret: string }).secret)
  await until(here, (state) => state.view !== undefined)
  await until(elsewhere, (state) => state.view !== undefined)

  here.editInstructions("Answer briefly.")
  await elsewhere.change({ instructions: "From the laptop." })
  // Read again, as after any change made here.
  await here.change({ key: "OPENROUTER_API_KEY", value: "made-up" })
  expect(here.state().view!.instructions).toBe("From the laptop.")
  expect(here.state().draft).toBe("Answer briefly.")

  expect(await here.saveInstructions()).toBeUndefined()
  expect(here.state().draft).toBeUndefined()
  expect(here.state().view!.instructions).toBe("Answer briefly.")

  await elsewhere.change({ instructions: "Changed again." })
  await here.change({ key: "OPENROUTER_API_KEY", value: null })
  expect(here.state().draft).toBeUndefined()
  expect(here.state().view!.instructions).toBe("Changed again.")

  // Written back to what is saved, there is nothing being written.
  here.editInstructions("Changed again.")
  expect(here.state().draft).toBeUndefined()
})

// Sends one command over a WebSocket of its own, as a page at `origin`
// would, and gives back the server's reply.
async function sendFrom(url: string, token: string, origin: string, command: Record<string, unknown>) {
  const socket = new WebSocket(new URL("/ws", url).href.replace(/^http/, "ws"), { headers: { Origin: origin, Authorization: `Bearer ${token}` } } as never)
  await new Promise((resolve, reject) => {
    socket.onopen = resolve
    socket.onerror = reject
  })
  const reply = new Promise<{ result?: unknown; error?: string }>((resolve) => {
    socket.onmessage = (message) => {
      const parsed = JSON.parse(String(message.data))
      if ("reply" in parsed) resolve(parsed)
    }
  })
  socket.send(JSON.stringify({ id: "one", command }))
  const answered = await reply
  socket.close()
  return answered
}

test("a connection from this computer's own address counts as this computer only when the page it comes from, if any, is at a local address too", async () => {
  const home = await dataFolder()
  const real = { EVIE_HOME: home }
  const env = await loadSettings(real)
  const files = await loadSettingsFiles({ env })
  const settings = settingsEditor({ env, environment: real, files, restart: () => {} })
  const run = await startEvie([scriptedProvider([]).plugin, ...files.plugins], { home, settings })
  const tokens = await openTokens(home)
  // A page served by something in front of the server, such as a proxy on this computer.
  const server = startServer({ evie: run.evie, tokens, host: "127.0.0.1", port: 0, origins: ["https://evie.example.com"] })
  cleanups.push(async () => {
    await server.stop()
    tokens.close()
    await run.stop()
  })
  const token = (tokens.add("browser") as { secret: string }).secret
  const key = { type: "change_settings", key: "OPENROUTER_API_KEY", value: "made-up" }

  expect(await sendFrom(server.url, token, "https://evie.example.com", key)).toMatchObject({
    error: "Keys, plugins and trusted projects can be changed only on the computer evie runs on.",
  })
  expect(await sendFrom(server.url, token, server.url, key)).not.toHaveProperty("error")
})
