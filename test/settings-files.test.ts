import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, realpath, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { createEvie, loadSettingsFiles } from "../src/index.ts"
import { say, startEvie, stored } from "./support/harness.ts"
import { scriptedProvider, type ScriptedReply } from "./support/scripted-provider.ts"

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// A plugin file as someone outside the project would write one.
const toolPlugin = (name: string) => `
export default {
  name: ${JSON.stringify(name)},
  setup(host) {
    host.tools.add({
      name: ${JSON.stringify(name)},
      description: "A tool from outside.",
      inputSchema: { type: "object" },
      run: async () => "hello from ${name}",
    })
  },
}
`

// Starts Evie the way the evie command does: plugins come from the settings
// files. `global` is what evie.json in the data folder holds, or undefined
// for no file. The model is a stand-in.
async function start(global: unknown, script: ScriptedReply[] = Array.from({ length: 8 }, () => ({ text: ["ok"] }))) {
  const home = await tempFolder("evie-home-")
  if (global !== undefined) await writeFile(join(home, "evie.json"), typeof global === "string" ? global : JSON.stringify(global))
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: home } })
  const model = scriptedProvider(script)
  const run = await startEvie([model.plugin, ...settings.plugins], { home, projectSettings: settings.projectSettings })
  cleanups.push(run.stop)
  // The names of the tools the model is offered in a session's next turn.
  async function offered(session = run.session) {
    await say(run, "hi", session)
    return model.requests.at(-1)!.tools.map((tool) => tool.name)
  }
  return { run, model, home, offered, settings }
}

async function projectWith(settings: unknown, files: Record<string, string> = {}) {
  const folder = await tempFolder("evie-project-")
  await mkdir(join(folder, ".evie"), { recursive: true })
  await writeFile(join(folder, ".evie", "evie.json"), JSON.stringify(settings))
  for (const [name, text] of Object.entries(files)) await writeFile(join(folder, name), text)
  return folder
}

const usualTools = ["read", "write", "edit", "grep", "glob", "shell"]

// ---- The global settings file -------------------------------------------------------

test("with no settings file, the built-in plugins load", async () => {
  const { offered, settings } = await start(undefined)

  expect(settings.plugins.map((plugin) => plugin.name)).toEqual(["files", "shell", "guard", "models", "openrouter"])
  expect(await offered()).toEqual(usualTools)
})

test("leaving a built-in plugin out of the list removes what it registers", async () => {
  const { offered } = await start({ plugins: ["files", "models", "openrouter"] })

  expect(await offered()).toEqual(["read", "write", "edit", "grep", "glob"])
})

test("a plugin file from outside the repository adds a tool the model can call", async () => {
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "greeter.ts"), toolPlugin("greet"))
  const { run, offered } = await start({ plugins: ["files", "models", join(outside, "greeter.ts")] }, [
    { text: [], calls: [{ name: "greet", input: {} }] },
    { text: ["done"] },
    { text: ["ok"] },
  ])

  await say(run, "go")

  expect(stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))).toEqual(["hello from greet"])
  expect(await offered()).toContain("greet")
})

test("a path that is not a full path is taken from the folder the settings file is in", async () => {
  const { home, offered } = await (async () => {
    const home = await tempFolder("evie-home-")
    await mkdir(join(home, "plugins"))
    await writeFile(join(home, "plugins", "greeter.ts"), toolPlugin("greet"))
    await writeFile(join(home, "evie.json"), JSON.stringify({ plugins: ["./plugins/greeter.ts"] }))
    const settings = await loadSettingsFiles({ env: { EVIE_HOME: home } })
    const model = scriptedProvider([{ text: ["ok"] }])
    const run = await startEvie([model.plugin, ...settings.plugins], { home })
    cleanups.push(run.stop)
    return { home, offered: async () => (await say(run, "hi"), model.requests[0]!.tools.map((tool) => tool.name)) }
  })()

  expect(home).toBeString()
  expect(await offered()).toEqual(["greet"])
})

test("options given for a plugin reach it", async () => {
  const outside = await tempFolder("evie-outside-")
  await writeFile(
    join(outside, "named.ts"),
    `export default (options) => ({
      name: "named",
      setup(host) {
        host.tools.add({ name: options.toolName, description: "Named by its options.", inputSchema: { type: "object" }, run: async () => "ok" })
      },
    })`,
  )
  const { offered } = await start({ plugins: [{ use: join(outside, "named.ts"), options: { toolName: "chosen_name" } }] })

  expect(await offered()).toEqual(["chosen_name"])
})

test("plugins are set up in the order they are listed", async () => {
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "a.ts"), toolPlugin("alpha"))
  await writeFile(join(outside, "b.ts"), toolPlugin("beta"))
  const { offered } = await start({ plugins: [join(outside, "b.ts"), "shell", join(outside, "a.ts")] })

  expect(await offered()).toEqual(["beta", "shell", "alpha"])
})

test("a plugin that cannot be loaded stops Evie at start, with an error that names it", async () => {
  const outside = await tempFolder("evie-outside-")
  await writeFile(join(outside, "empty.ts"), "export const nothing = 1")
  await writeFile(join(outside, "broken.ts"), "export default { name: 'broken', setup() { throw new Error('could not connect') } }")

  await expect(start({ plugins: ["no-such-plugin"] })).rejects.toThrow(
    'There is no built-in plugin named "no-such-plugin". The built-in plugins are: files, shell, guard, models, openrouter.',
  )
  await expect(start({ plugins: ["/no/such/file.ts"] })).rejects.toThrow('The plugin "/no/such/file.ts" could not be loaded')
  await expect(start({ plugins: [join(outside, "empty.ts")] })).rejects.toThrow("does not export a plugin")
  await expect(start({ plugins: ["files", join(outside, "broken.ts")] })).rejects.toThrow(
    'The plugin "broken" could not be set up: could not connect',
  )
})

test("a settings file that is not valid says which file and why", async () => {
  await expect(start("{ not json")).rejects.toThrow(/evie\.json is not valid JSON/)
  await expect(start({ plugins: "files" })).rejects.toThrow(/"plugins" must be a list/)
})

test("the API key is never read from the settings file", async () => {
  const { run } = await start({ OPENROUTER_API_KEY: "sk-in-the-wrong-place", plugins: [{ use: "openrouter", options: { apiKey: "sk-also-wrong" } }] })
  // No stand-in model is in front this time: the real provider is the only one.
  const real = await loadSettingsFiles({ env: { EVIE_HOME: run.home } })
  const alone = await startEvie([...real.plugins], { home: await tempFolder("evie-home-") })
  cleanups.push(alone.stop)

  await say(alone, "hi")

  expect(stored(alone.events).at(-1)).toMatchObject({
    type: "turn_end",
    body: { reason: "error", error: "No API key. Set OPENROUTER_API_KEY to your OpenRouter key." },
  })
})

// ---- A project's own settings file ---------------------------------------------------

test("a project's settings file adds plugins for that project only, after the global ones", async () => {
  const folder = await projectWith({ plugins: ["./deploy.ts"] }, { "deploy.ts": toolPlugin("deploy") })
  const { run, offered } = await start({ plugins: ["files", "shell", "models"], trust: [folder] })
  const inProject = await run.evie.send({ type: "open_session", folder })
  const general = await run.evie.send({ type: "open_session" })

  expect(await offered(inProject.session)).toEqual([...usualTools, "deploy"])
  expect(await offered()).toEqual(usualTools)
  expect(await offered(general.session)).toEqual(usualTools)
})

test("a project's settings file can switch off a global plugin for that project", async () => {
  const folder = await projectWith({ disable: ["shell"] })
  const { run, offered } = await start({ trust: [folder] })
  const inProject = await run.evie.send({ type: "open_session", folder })

  expect(await offered(inProject.session)).toEqual(["read", "write", "edit", "grep", "glob"])
  expect(await offered()).toEqual(usualTools)
})

test("a project can give a global plugin its own options, which replaces it for that project", async () => {
  const folder = await projectWith({ plugins: [{ use: "files", options: { ripgrep: "/no/such/ripgrep" } }] })
  const { run, offered } = await start({ trust: [folder] })
  const inProject = await run.evie.send({ type: "open_session", folder })

  expect((await offered(inProject.session)).toSorted()).toEqual(usualTools.toSorted())
})

test("a project that is not trusted has its settings file ignored as a whole, and the owner is told", async () => {
  const folder = await projectWith(
    { plugins: ["./deploy.ts", "shell"], disable: ["guard", "shell"] },
    { "deploy.ts": toolPlugin("deploy") },
  )
  const { run, home, offered, model } = await start(undefined, [
    { text: [], calls: [{ name: "shell", input: { command: "sudo true" } }] },
    { text: ["ok"] },
    { text: ["ok"] },
  ])

  const opened = await run.evie.send({ type: "open_session", folder })
  await say(run, "go", opened.session)

  expect(opened.notices).toEqual([
    `This project has a settings file of its own, ${join(folder, ".evie", "evie.json")}, which was ignored because the ` +
      `project is not trusted. To use it, add "${folder}" to "trust" in ${join(home, "evie.json")}.`,
  ])
  // Nothing it asked for happened: no plugin of its own, and the guard is still on.
  expect(model.requests[0]?.tools.map((tool) => tool.name)).toEqual(usualTools)
  const results = stored(run.events).flatMap((event) => (event.type === "tool_result" ? [event.body.text] : []))
  expect(results[0]).toStartWith("Blocked by the guard:")
  expect(await offered(opened.session)).toEqual(usualTools)
})

test("a project with no settings file of its own opens with no notices", async () => {
  const { run } = await start(undefined)

  const opened = await run.evie.send({ type: "open_session", folder: run.projectFolder })

  expect(opened.notices).toEqual([])
})

test("a name that is not a built-in plugin is refused, even one every object has", async () => {
  await expect(start({ plugins: ["constructor"] })).rejects.toThrow('There is no built-in plugin named "constructor"')
})

test("a settings file that is there but cannot be read is reported, not passed over", async () => {
  const home = await tempFolder("evie-home-")
  await mkdir(join(home, "evie.json"))

  await expect(loadSettingsFiles({ env: { EVIE_HOME: home } })).rejects.toThrow(/evie\.json could not be read/)
})

// ---- The model and the reply limit -----------------------------------------------------

// Starts Evie with the model left for the settings to decide.
async function startWithSettings(global: unknown, env: Record<string, string> = {}) {
  const home = await tempFolder("evie-home-")
  if (global !== undefined) await writeFile(join(home, "evie.json"), JSON.stringify(global))
  const settings = await loadSettingsFiles({ env: { EVIE_HOME: home, ...env } })
  const model = scriptedProvider(Array.from({ length: 6 }, () => ({ text: ["ok"] })))
  const evie = await createEvie({
    env: { EVIE_HOME: home },
    plugins: [model.plugin],
    projectSettings: settings.projectSettings,
    settingsFile: settings.file,
    ...(settings.model !== undefined && { model: settings.model }),
    ...(settings.maxTokens !== undefined && { maxTokens: settings.maxTokens }),
  })
  cleanups.push(() => evie.close())
  // Opens a session in a folder, sends one message, and returns the request the model was sent.
  async function requestIn(folder?: string) {
    const opened = await evie.send({ type: "open_session", ...(folder && { folder }) })
    await say({ evie, session: opened.session }, "hi")
    return model.requests.at(-1)!
  }
  return { evie, home, requestIn }
}

test("the model and the reply limit come from the global settings file", async () => {
  const { requestIn } = await startWithSettings({ model: "some/model", maxTokens: 4000, plugins: [] })

  expect(await requestIn()).toMatchObject({ model: "some/model", maxTokens: 4000 })
})

test("a setting in the environment wins over the settings file", async () => {
  const { requestIn } = await startWithSettings(
    { model: "from/the-file", maxTokens: 4000, plugins: [] },
    { EVIE_MODEL: "from/the-environment", EVIE_MAX_TOKENS: "900" },
  )

  expect(await requestIn()).toMatchObject({ model: "from/the-environment", maxTokens: 900 })
})

test("a trusted project can set its own model and reply limit, and other projects keep the global ones", async () => {
  const special = await projectWith({ model: "project/model", maxTokens: 2000 })
  const ordinary = await tempFolder("evie-project-")
  const { requestIn } = await startWithSettings({ model: "global/model", plugins: [], trust: [special] })

  expect(await requestIn(special)).toMatchObject({ model: "project/model", maxTokens: 2000 })
  expect(await requestIn(ordinary)).toMatchObject({ model: "global/model" })
  expect((await requestIn(ordinary)).maxTokens).toBeUndefined()
})

test("the environment still wins over a project's own model", async () => {
  const special = await projectWith({ model: "project/model" })
  const { requestIn } = await startWithSettings({ model: "global/model", plugins: [], trust: [special] }, { EVIE_MODEL: "from/the-environment" })

  expect((await requestIn(special)).model).toBe("from/the-environment")
})

test("a project that is not trusted cannot choose the model", async () => {
  const special = await projectWith({ model: "expensive/model" })
  const { requestIn } = await startWithSettings({ model: "global/model", plugins: [] })

  expect((await requestIn(special)).model).toBe("global/model")
})

test("with no model set anywhere, opening a session says where to set one", async () => {
  const { evie, home } = await startWithSettings({ plugins: [] })

  await expect(evie.send({ type: "open_session" })).rejects.toThrow(
    `No model is set. Add "model" to ${join(home, "evie.json")}, with a name from https://openrouter.ai/models.`,
  )
})

test("a model or reply limit of the wrong kind in a settings file is refused", async () => {
  await expect(startWithSettings({ model: 5 })).rejects.toThrow(/"model" must be the name of a model/)
  await expect(startWithSettings({ model: "a/b", maxTokens: "lots" })).rejects.toThrow(/"maxTokens" must be a whole number above zero/)
})
