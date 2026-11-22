import { afterEach, expect, test } from "bun:test"
import { lstat, mkdir, mkdtemp, readFile, realpath, rm, stat, symlink, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { loadSettings, loadSettingsFiles } from "../src/index.ts"
import { settingsEditor } from "../src/settings-editor.ts"
import { startEvie } from "./support/harness.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"

// The settings window's commands, as the server carries them out: what
// they read from the data folder's files and what they write there. Each
// test has a temporary data folder, and every key in it is made up.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

async function tempFolder(prefix: string) {
  const folder = await realpath(await mkdtemp(join(tmpdir(), prefix)))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// Starts Evie as `evie serve` does, from a data folder holding these files:
// the `.env`, the `evie.json` (a short list of plugins when none is given)
// and the owner's instruction file. `environment` stands for the one the
// server runs in. Each restart asked for is counted.
async function start(files: { env?: string; settings?: unknown; instructions?: string } = {}, environment: Record<string, string> = {}) {
  const home = await tempFolder("evie-home-")
  if (files.env !== undefined) await writeFile(join(home, ".env"), files.env)
  if (files.instructions !== undefined) await writeFile(join(home, "AGENTS.md"), files.instructions)
  await writeFile(join(home, "evie.json"), JSON.stringify(files.settings ?? { plugins: ["ask", "openrouter", "web"] }))
  const real = { EVIE_HOME: home, ...environment }
  const env = await loadSettings(real)
  const settings = await loadSettingsFiles({ env })
  const restarts: number[] = []
  const editor = settingsEditor({ env, environment: real, files: settings, restart: () => restarts.push(Date.now()) })
  const run = await startEvie([scriptedProvider([]).plugin, ...settings.plugins], { home, projectSettings: settings.projectSettings, trust: settings.trust, settings: editor })
  cleanups.push(run.stop)
  return { run, home, restarts }
}

// What loading the settings files says, as `evie serve` does when it
// starts, of a data folder whose evie.json holds these settings.
async function startingSays(settings: unknown) {
  const elsewhere = await tempFolder("evie-home-")
  await writeFile(join(elsewhere, "evie.json"), JSON.stringify(settings))
  return loadSettingsFiles({ env: { EVIE_HOME: elsewhere } }).then(
    () => "loaded",
    (cause: Error) => cause.message,
  )
}

// ---- Keys --------------------------------------------------------------------------------------

// What the built-in plugins say their keys are for.
const OPENROUTER = { name: "OPENROUTER_API_KEY", about: "Reaches the models, through OpenRouter." }
const BRAVE = { name: "BRAVE_API_KEY", about: "Lets evie search the web, through Brave Search. Without it, evie can still read a page it is given the address of." }

test("a key set from the window is written to the data folder's .env, and only whether it is set comes back", async () => {
  const { run, home } = await start({ env: "BRAVE_API_KEY=made-up-brave\n" })

  const before = await run.evie.send({ type: "read_settings" })
  expect(before.keys).toEqual([
    { ...OPENROUTER, inFile: false, environment: false, pending: false },
    { ...BRAVE, inFile: true, environment: false, pending: false },
  ])
  // Written as most files are, others on this computer can read it.
  expect(before.keysPrivate).toBe(false)
  expect(before.restart).toBe(false)

  const after = await run.evie.send({ type: "change_settings", key: "OPENROUTER_API_KEY", value: "made-up-openrouter" })
  expect(await readFile(join(home, ".env"), "utf8")).toBe("BRAVE_API_KEY=made-up-brave\nOPENROUTER_API_KEY=made-up-openrouter\n")
  // Only the owner's user can read the file it is in.
  expect((await stat(join(home, ".env"))).mode & 0o777).toBe(0o600)
  expect(after.keysPrivate).toBe(true)
  // The server started without it, so it is used once the server starts again.
  expect(after.keys[0]).toEqual({ ...OPENROUTER, inFile: true, environment: false, pending: true })
  expect(after.restart).toBe(true)
  for (const view of [before, after, await run.evie.send({ type: "read_settings" })]) expect(JSON.stringify(view)).not.toContain("made-up")
})

test("a key cleared is taken out of the file, and one the server's own environment sets says so, whether or not the file does", async () => {
  const { run, home } = await start(
    { env: "# keys\nOPENROUTER_API_KEY=made-up-openrouter\nBRAVE_API_KEY=made-up-brave\n" },
    { BRAVE_API_KEY: "made-up-from-the-environment" },
  )

  const cleared = await run.evie.send({ type: "change_settings", key: "OPENROUTER_API_KEY", value: null })
  expect(await readFile(join(home, ".env"), "utf8")).toBe("# keys\nBRAVE_API_KEY=made-up-brave\n")
  expect(cleared.keys).toEqual([
    { ...OPENROUTER, inFile: false, environment: false, pending: true },
    { ...BRAVE, inFile: true, environment: true, pending: false },
  ])

  // Cleared from the file, it is still set where the server runs.
  const both = await run.evie.send({ type: "change_settings", key: "BRAVE_API_KEY", value: null })
  expect(both.keys[1]).toEqual({ ...BRAVE, inFile: false, environment: true, pending: false })
  expect(JSON.stringify(both)).not.toContain("made-up")
})

test("the keys are the ones the loaded plugins say they read, each with what it is for, then any other the .env file sets", async () => {
  const service = join(await tempFolder("evie-plugins-"), "service.ts")
  await writeFile(service, 'export default { name: "service", keys: [{ name: "MADE_UP_SERVICE_KEY", about: "Reaches a made-up service." }], setup() {} }')
  const { run } = await start({ settings: { plugins: ["ask", service] }, env: "SOMETHING_ELSE=made-up\n" })

  const { keys } = await run.evie.send({ type: "read_settings" })
  expect(keys).toEqual([
    { name: "MADE_UP_SERVICE_KEY", about: "Reaches a made-up service.", inFile: false, environment: false, pending: false },
    { name: "SOMETHING_ELSE", inFile: true, environment: false, pending: false },
  ])
  const refused = await run.evie.send({ type: "change_settings", key: "OPENROUTER_API_KEY", value: "made-up" }).catch((cause: Error) => cause.message)
  expect(refused).toBe('"OPENROUTER_API_KEY" is not one of the keys evie reads. They are: MADE_UP_SERVICE_KEY, SOMETHING_ELSE.')
})

test("with no .env file yet, the keys are private: the first one set makes the file, which only the owner's user can read", async () => {
  const { run } = await start()

  expect((await run.evie.send({ type: "read_settings" })).keysPrivate).toBe(true)
})

test("a key's value that would not be read back as it was given is refused, saying why, and nothing is written", async () => {
  const { run, home } = await start({ env: "OPENROUTER_API_KEY=made-up-openrouter\n" })
  const refused = (change: Record<string, unknown>) =>
    run.evie.send({ type: "change_settings", ...change } as never).then(
      () => "accepted",
      (cause: Error) => cause.message,
    )

  expect(await refused({ key: "OPENROUTER_API_KEY", value: "made-up-first\nEVIE_MODEL=made-up/model" })).toBe(
    "A key's value is one line, and this one has a line break in it.",
  )
  expect(await refused({ key: "OPENROUTER_API_KEY", value: '"made-up-quoted"' })).toBe("A key's value is one line, with no spaces or quotes around it.")
  expect(await refused({ key: "OPENROUTER_API_KEY", value: "   " })).toBe("Give the key's value, or clear it.")
  expect(await refused({ key: "PATH", value: "/made/up" })).toBe('"PATH" is not one of the keys evie reads. They are: OPENROUTER_API_KEY, BRAVE_API_KEY.')
  // The words never repeat the value.
  expect(await readFile(join(home, ".env"), "utf8")).toBe("OPENROUTER_API_KEY=made-up-openrouter\n")

  // Spaces around a value pasted in are not part of it, as when the file is read.
  await run.evie.send({ type: "change_settings", key: "OPENROUTER_API_KEY", value: "  made-up-pasted \n" })
  expect(await readFile(join(home, ".env"), "utf8")).toBe("OPENROUTER_API_KEY=made-up-pasted\n")
})

test("a key set where .env is a link to a file elsewhere is written to that file, and the link is kept", async () => {
  const { run, home } = await start()
  const elsewhere = join(await tempFolder("evie-dotfiles-"), "evie.env")
  await writeFile(elsewhere, "BRAVE_API_KEY=made-up-brave\n")
  await symlink(elsewhere, join(home, ".env"))

  await run.evie.send({ type: "change_settings", key: "OPENROUTER_API_KEY", value: "made-up-openrouter" })

  expect((await lstat(join(home, ".env"))).isSymbolicLink()).toBe(true)
  expect(await readFile(elsewhere, "utf8")).toBe("BRAVE_API_KEY=made-up-brave\nOPENROUTER_API_KEY=made-up-openrouter\n")
})

// ---- Plugins ---------------------------------------------------------------------------------

test("a plugin is added to evie.json's list by its name or its file's path, and taken off again; each says whether the server loaded it", async () => {
  const greeter = join(await tempFolder("evie-plugins-"), "greeter.ts")
  await writeFile(greeter, 'export default { name: "greeter", setup() {} }')
  const { run, home } = await start({ settings: { plugins: ["ask", "files"], model: "made-up/model" } })
  const file = async () => JSON.parse(await readFile(join(home, "evie.json"), "utf8"))

  expect((await run.evie.send({ type: "read_settings" })).plugins).toEqual([
    { name: "ask", listed: true, loaded: true },
    { name: "files", listed: true, loaded: true },
  ])

  await run.evie.send({ type: "change_settings", addPlugin: "web" })
  const added = await run.evie.send({ type: "change_settings", addPlugin: greeter })
  expect(await file()).toEqual({ plugins: ["ask", "files", "web", greeter], model: "made-up/model" })
  expect(added.plugins.slice(2)).toEqual([
    { name: "web", listed: true, loaded: false },
    { name: greeter, listed: true, loaded: false },
  ])
  expect(added.restart).toBe(true)

  // One taken off stays loaded until the server starts again.
  const removed = await run.evie.send({ type: "change_settings", removePlugin: "files" })
  expect((await file()).plugins).toEqual(["ask", "web", greeter])
  expect(removed.plugins).toEqual([
    { name: "ask", listed: true, loaded: true },
    { name: "web", listed: true, loaded: false },
    { name: greeter, listed: true, loaded: false },
    { name: "files", listed: false, loaded: true },
  ])
})

test("with no list in evie.json, a plugin is added to the built-in ones, or taken off them", async () => {
  const { run, home } = await start({ settings: { model: "made-up/model" } })

  await run.evie.send({ type: "change_settings", removePlugin: "web" })

  const listed = JSON.parse(await readFile(join(home, "evie.json"), "utf8")).plugins
  expect(listed).toEqual(["files", "shell", "jobs", "permissions", "undo", "models", "ask", "subagents", "skills", "instructions", "sync", "openrouter"])
})

test("a plugin that would not load when the server starts is refused with the words starting it would give, and nothing is written", async () => {
  const folder = await tempFolder("evie-plugins-")
  const notAPlugin = join(folder, "not-a-plugin.ts")
  await writeFile(notAPlugin, "export default 42")
  const { run, home } = await start({ settings: { plugins: ["ask"] } })
  const before = await readFile(join(home, "evie.json"), "utf8")
  const refused = (change: Record<string, unknown>) =>
    run.evie.send({ type: "change_settings", ...change } as never).then(
      () => "accepted",
      (cause: Error) => cause.message,
    )
  for (const entry of ["nonesuch", join(folder, "missing.ts"), notAPlugin]) {
    const said = await refused({ addPlugin: entry })
    expect(said).not.toBe("accepted")
    expect(said).toBe(await startingSays({ plugins: [entry] }))
  }
  expect(await refused({ addPlugin: "ask" })).toBe(`"ask" is in the list of plugins in ${join(home, "evie.json")} already.`)
  expect(await refused({ removePlugin: "shell" })).toBe(`"shell" is not in the list of plugins in ${join(home, "evie.json")}.`)
  expect(await readFile(join(home, "evie.json"), "utf8")).toBe(before)
})

// ---- The instruction file ---------------------------------------------------------------------

test("the owner's instruction file is shown, and saved as it is written, with no restart to wait for", async () => {
  const { run, home } = await start({ instructions: "Answer briefly.\n" })
  expect((await run.evie.send({ type: "read_settings" })).instructions).toBe("Answer briefly.\n")

  const saved = await run.evie.send({ type: "change_settings", instructions: "Answer briefly.\nAsk before pushing.\n" })

  expect(await readFile(join(home, "AGENTS.md"), "utf8")).toBe("Answer briefly.\nAsk before pushing.\n")
  expect(saved.instructions).toBe("Answer briefly.\nAsk before pushing.\n")
  expect(saved.restart).toBe(false)
})

test("an instruction file that is a link to a file elsewhere is saved to that file, and two saves at once are made one after the other", async () => {
  const { run, home } = await start()
  const elsewhere = join(await tempFolder("evie-dotfiles-"), "my-agents.md")
  await writeFile(elsewhere, "Answer briefly.\n")
  await symlink(elsewhere, join(home, "AGENTS.md"))

  await Promise.all([
    run.evie.send({ type: "change_settings", instructions: "First." }),
    run.evie.send({ type: "change_settings", instructions: "Second." }),
  ])

  expect((await lstat(join(home, "AGENTS.md"))).isSymbolicLink()).toBe(true)
  expect(await readFile(elsewhere, "utf8")).toBe("Second.")
})

test("with no instruction file there is nothing to show, and saving makes one", async () => {
  const { run, home } = await start()
  expect((await run.evie.send({ type: "read_settings" })).instructions).toBe("")

  await run.evie.send({ type: "change_settings", instructions: "Write in British English." })

  expect(await readFile(join(home, "AGENTS.md"), "utf8")).toBe("Write in British English.")
})

// ---- Trusted projects ---------------------------------------------------------------------------

test("each project says whether its own settings are trusted, and one trusted or not is written under trust or untrusted", async () => {
  const { run, home } = await start({ settings: { plugins: ["ask"] } })
  const project = run.project!
  const file = async () => JSON.parse(await readFile(join(home, "evie.json"), "utf8"))
  expect((await run.evie.send({ type: "read_settings" })).projects).toEqual([{ ...project, trusted: false, pending: false }])

  const trusted = await run.evie.send({ type: "change_settings", project: project.id, trusted: true })
  expect(await file()).toEqual({ plugins: ["ask"], trust: [project.folder] })
  // A session open in it keeps the settings it opened with until the server starts again.
  expect(trusted.projects).toEqual([{ ...project, trusted: true, pending: true }])
  expect(trusted.restart).toBe(true)

  const untrusted = await run.evie.send({ type: "change_settings", project: project.id, trusted: false })
  expect(await file()).toEqual({ plugins: ["ask"], trust: [], untrusted: [project.folder] })
  // As it was when the server started.
  expect(untrusted.projects).toEqual([{ ...project, trusted: false, pending: false }])
  expect(untrusted.restart).toBe(false)

  const refused = await run.evie.send({ type: "change_settings", project: "nonesuch", trusted: true }).catch((cause: Error) => cause.message)
  expect(refused).toBe('There is no project with the id "nonesuch".')
})

test("a project trusted from the window is not asked about again: its next session uses its own settings", async () => {
  const { run } = await start({ settings: { plugins: ["ask"] } })
  const folder = await tempFolder("evie-project-")
  await mkdir(join(folder, ".evie"))
  await writeFile(join(folder, ".evie", "evie.json"), JSON.stringify({ disable: ["ask"] }))
  // Opened by a client that shows no questions, it is not trusted for now.
  const first = await run.evie.send({ type: "open_session", folder })
  expect(first.notices).toEqual([expect.stringContaining("was ignored because the project is not trusted")])

  await run.evie.send({ type: "change_settings", project: first.project!.id, trusted: true })

  const second = await run.evie.send({ type: "open_session", folder })
  expect(second.notices).toEqual([])
})

test("a change of no known kind is refused, saying what a change is", async () => {
  const { run } = await start()

  const refused = await run.evie.send({ type: "change_settings", colour: "red" } as never).catch((cause: Error) => cause.message)

  expect(refused).toBe(
    'A change is one of: { "key": ..., "value": ... }, { "addPlugin": ... }, { "removePlugin": ... }, { "instructions": ... } or { "project": ..., "trusted": ... }.',
  )
})

// ---- Restarting ----------------------------------------------------------------------------------

test("a restart is asked of the server named by when it started, once the command's reply has gone, and not of another", async () => {
  const { run, restarts } = await start()
  const { started } = await run.evie.send({ type: "read_settings" })

  await run.evie.send({ type: "restart_server", started: started - 1 })
  await Bun.sleep(20)
  expect(restarts).toEqual([])

  await run.evie.send({ type: "restart_server", started })
  expect(restarts).toEqual([])
  await Bun.sleep(20)
  expect(restarts).toHaveLength(1)
})

test("a restart is refused, saying why, when the settings files would stop the server starting again", async () => {
  const { run, home, restarts } = await start()
  const { started } = await run.evie.send({ type: "read_settings" })
  await writeFile(join(home, "evie.json"), JSON.stringify({ plugins: ["nonesuch"] }))

  const refused = await run.evie.send({ type: "restart_server", started }).catch((cause: Error) => cause.message)

  expect(refused).toBe(`evie's server was not restarted, since it would not start again. ${await startingSays({ plugins: ["nonesuch"] })}`)
  await Bun.sleep(20)
  expect(restarts).toEqual([])
})
