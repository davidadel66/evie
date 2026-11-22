import { readFile, rename, writeFile } from "node:fs/promises"
import { dataFolder } from "./core/evie.ts"
import type { Project, SettingsEditor, SettingsView } from "./core/types.ts"
import { ownInstructionsFile } from "./plugins/instructions.ts"
import { loadServerSettings, loadSettingsFiles, type SettingsFiles } from "./settings-files.ts"
import { keysInFile, keyValueProblem, loadSettings, writeKey, type Settings } from "./settings.ts"

// What the settings window reads and changes, for the server: the keys in
// the data folder's `.env`, the plugins `evie.json` lists, the owner's
// instruction file and which projects are trusted. Most of them are read
// only when the server starts, so a change to one applies once it starts
// again, and until then it is said to be pending.

// The keys Evie's own plugins read from the `.env` file.
const KEYS = ["OPENROUTER_API_KEY", "BRAVE_API_KEY"]

export type SettingsEditorOptions = {
  // The settings the server started with: its environment and the `.env`
  // file as they were then.
  env: Settings
  // The environment the server runs in, which wins over the `.env` file,
  // and which it is started again with.
  environment: Settings
  // The settings files, as the server loaded them when it started.
  files: SettingsFiles
  // Stops the server and starts it again.
  restart: () => void
}

export function settingsEditor(options: SettingsEditorOptions): SettingsEditor {
  const home = dataFolder(options.env)
  const started = Date.now()
  // The keys the server took from the file when it started: the ones its
  // settings have that its environment does not.
  const fromFile = Object.keys(options.env).filter((name) => options.environment[name] === undefined)

  // Evie's own keys, then any other the file sets now or set at the start.
  async function keys(): Promise<SettingsView["keys"]> {
    const inFile = await keysInFile(home)
    const names = [...new Set([...KEYS, ...Object.keys(inFile), ...fromFile])]
    return names.map((name) => ({
      name,
      set: Boolean(inFile[name]),
      environment: options.environment[name] !== undefined,
      // What the server would start with now is not what it started with.
      pending: (options.environment[name] ?? inFile[name]) !== options.env[name],
    }))
  }

  async function setKey(name: string, value: string | null) {
    const known = (await keys()).map((key) => key.name)
    if (!known.includes(name)) throw new Error(`"${name}" is not one of the keys evie reads. They are: ${known.join(", ")}.`)
    if (value !== null && typeof value !== "string") throw new Error("A key's `value` is text, or null to clear it.")
    const given = value?.trim() ?? null
    const problem = given === null ? undefined : keyValueProblem(name, given)
    if (problem !== undefined) throw new Error(problem)
    await writeKey(home, name, given)
  }

  // The plugins listed now, in order, then any loaded when the server
  // started that are no longer listed.
  async function plugins(): Promise<SettingsView["plugins"]> {
    const listed = (await options.files.now()).plugins
    return [...new Set([...listed, ...options.files.listed])].map((name) => ({ name, listed: listed.includes(name), loaded: options.files.listed.includes(name) }))
  }

  // A plugin is named by text, which a client that is not type-checked may not send.
  const pluginName = (name: unknown) => {
    const given = typeof name === "string" ? name.trim() : ""
    if (given === "") throw new Error("Give a built-in plugin's name, or the full path to a plugin file.")
    return given
  }

  // The owner's instruction file is read before every request, so a change
  // to it applies from the next one. It is replaced in one step, so it is
  // never read half written.
  const instructionsFile = ownInstructionsFile(home)
  const instructions = () => readFile(instructionsFile, "utf8").catch((cause: { code?: string }) => (cause.code === "ENOENT" ? "" : Promise.reject(cause)))
  async function saveInstructions(text: unknown) {
    if (typeof text !== "string") throw new Error("`instructions` is the text of the instruction file.")
    await writeFile(`${instructionsFile}.new`, text)
    await rename(`${instructionsFile}.new`, instructionsFile)
  }

  // Whether each project was trusted before the window first changed that,
  // by its folder. A project is trusted or not as the owner's answer to
  // whether to trust it is: at once, for its sessions that open from then
  // on. A session already open keeps what it opened with until the server
  // starts again, so the change is pending until then.
  const trustedBefore = new Map<string, boolean>()
  async function projectsOf(projects: readonly Project[]): Promise<SettingsView["projects"]> {
    const { trust } = await options.files.now()
    return projects.map((project) => {
      const trusted = trust.includes(project.folder)
      const before = trustedBefore.get(project.folder)
      return { ...project, trusted, pending: before !== undefined && before !== trusted }
    })
  }
  async function trustProject(id: unknown, trusted: unknown, projects: readonly Project[]) {
    const project = projects.find((each) => each.id === id)
    if (!project) throw new Error(`There is no project with the id "${String(id)}".`)
    if (typeof trusted !== "boolean") throw new Error("`trusted` is true to trust the project's own settings, or false not to.")
    if (!trustedBefore.has(project.folder)) trustedBefore.set(project.folder, (await options.files.now()).trust.includes(project.folder))
    await (trusted ? options.files.trust(project.folder) : options.files.untrust(project.folder))
  }

  return {
    async read(projects) {
      const [listedKeys, listedPlugins, text, listedProjects] = await Promise.all([keys(), plugins(), instructions(), projectsOf(projects)])
      return {
        keys: listedKeys,
        plugins: listedPlugins,
        instructions: text,
        projects: listedProjects,
        restart: [...listedKeys, ...listedProjects].some((each) => each.pending) || listedPlugins.some((plugin) => plugin.listed !== plugin.loaded),
        started,
      }
    },
    async change(change, projects) {
      if ("key" in change) return setKey(change.key, change.value)
      if ("addPlugin" in change) return options.files.addPlugin(pluginName(change.addPlugin))
      if ("removePlugin" in change) return options.files.removePlugin(pluginName(change.removePlugin))
      if ("instructions" in change) return saveInstructions(change.instructions)
      if ("project" in change) return trustProject(change.project, change.trusted, projects)
      throw new Error(
        'A change is one of: { "key": ..., "value": ... }, { "addPlugin": ... }, { "removePlugin": ... }, { "instructions": ... } or { "project": ..., "trusted": ... }.',
      )
    },
    // The settings are loaded first, as the server will load them when it
    // starts again, so that one that would not start is not stopped. It is
    // stopped only once this command's reply has gone out.
    async restart(asked) {
      if (asked !== started) return
      try {
        const env = await loadSettings(options.environment)
        await loadSettingsFiles({ env })
        await loadServerSettings({ env })
      } catch (cause) {
        throw new Error(`evie's server was not restarted, since it would not start again. ${cause instanceof Error ? cause.message : String(cause)}`)
      }
      setTimeout(options.restart, 0)
    },
  }
}
