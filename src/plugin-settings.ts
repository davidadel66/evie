import { readFile } from "node:fs/promises"
import { homedir } from "node:os"
import { isAbsolute, join, resolve } from "node:path"
import { pathToFileURL } from "node:url"
import { dataFolder } from "./core/evie.ts"
import type { Plugin, Project, ProjectPlugins } from "./core/types.ts"
import { files, type FilesOptions } from "./plugins/files/index.ts"
import { guard } from "./plugins/guard.ts"
import { models } from "./plugins/models/index.ts"
import { openrouter } from "./plugins/openrouter.ts"
import { shell } from "./plugins/shell.ts"
import type { Settings } from "./settings.ts"

// Which plugins Evie loads, read from settings files.
//
// The global file is `evie.json` in the data folder. It lists the plugins
// every project gets, in order. A project can have a file of its own at
// `.evie/evie.json` in its folder, which adds plugins for that project only
// and can switch off a global one.
//
//   { "plugins": ["files", { "use": "shell" }, "/path/to/my-plugin.ts"],
//     "trust": ["/path/to/a/project"] }
//
// A plugin is a built-in one, named, or a file, given by its path. A plugin
// file is code that runs with Evie's full permissions. So a project's own
// plugin files are loaded only when the global file lists that project under
// "trust": otherwise opening someone else's repository could run their code.

export type PluginSettings = {
  // The global plugins, in the order listed.
  plugins: Plugin[]
  // What a project adds and switches off.
  projectPlugins(project: Project): Promise<ProjectPlugins>
}

type Entry = string | { use: string; options?: Record<string, unknown> }
type SettingsFile = { plugins?: Entry[]; trust?: string[]; disable?: string[] }

// What a built-in plugin is made from: its options from the settings file,
// and the things only Evie knows. The API key is never taken from options.
type Context = { env: Settings; home: string }
const BUILT_IN: Record<string, (options: Record<string, unknown>, context: Context) => Plugin> = {
  files: (options) => files(options as FilesOptions),
  shell: () => shell(),
  guard: (_options, context) => guard({ dataFolder: context.home }),
  models: () => models(),
  openrouter: (_options, context) => openrouter({ env: context.env }),
}
const DEFAULT: Entry[] = ["files", "shell", "guard", "models", "openrouter"]

export async function loadPluginSettings(options: { env: Settings }): Promise<PluginSettings> {
  const home = dataFolder(options.env)
  const context: Context = { env: options.env, home }
  const globalFile = join(home, "evie.json")
  const global = (await readSettingsFile(globalFile)) ?? {}

  const plugins: Plugin[] = []
  for (const entry of global.plugins ?? DEFAULT) plugins.push(await load(entry, home, context))
  const trusted = new Set((global.trust ?? []).map((folder) => resolve(expandHome(folder))))

  return {
    plugins,
    async projectPlugins(project) {
      const own = await readSettingsFile(join(project.folder, ".evie", "evie.json"))
      if (!own) return { plugins: [], disable: [], notices: [] }

      const added: Plugin[] = []
      const notices: string[] = []
      for (const entry of own.plugins ?? []) {
        const name = typeof entry === "string" ? entry : entry.use
        // A built-in plugin is Evie's own code. A file is the project's.
        if (!(name in BUILT_IN) && !trusted.has(project.folder)) {
          notices.push(
            `This project lists a plugin of its own, ${name}, which was not loaded because the project is not trusted. ` +
              `To load it, add "${project.folder}" to "trust" in ${globalFile}.`,
          )
          continue
        }
        added.push(await load(entry, project.folder, context))
      }
      return { plugins: added, disable: own.disable ?? [], notices }
    },
  }
}

// ---- Loading one plugin -----------------------------------------------------------------

async function load(entry: Entry, baseFolder: string, context: Context): Promise<Plugin> {
  const name = typeof entry === "string" ? entry : entry.use
  const options = typeof entry === "string" ? {} : (entry.options ?? {})

  const builtIn = BUILT_IN[name]
  if (builtIn) return builtIn(options, context)

  if (!looksLikePath(name)) {
    throw new Error(`There is no built-in plugin named "${name}". The built-in plugins are: ${Object.keys(BUILT_IN).join(", ")}.`)
  }

  const expanded = expandHome(name)
  const file = isAbsolute(expanded) ? expanded : resolve(baseFolder, expanded)
  let module: { default?: unknown }
  try {
    module = await import(pathToFileURL(file).href)
  } catch (cause) {
    throw new Error(`The plugin "${name}" could not be loaded: ${cause instanceof Error ? cause.message : String(cause)}`)
  }

  // A plugin file exports a plugin, or a function that makes one from its options.
  const exported = typeof module.default === "function" ? await module.default(options) : module.default
  if (!isPlugin(exported)) {
    throw new Error(
      `The file ${file} does not export a plugin. Its default export must be an object with a name and a setup ` +
        `function, or a function that returns one.`,
    )
  }
  return exported
}

const looksLikePath = (name: string) => /[/\\]|^[.~]|\.(ts|js|mjs)$/.test(name)
const expandHome = (path: string) => path.replace(/^~(?=\/|$)/, homedir())
const isPlugin = (value: unknown): value is Plugin =>
  typeof value === "object" &&
  value !== null &&
  typeof (value as Plugin).name === "string" &&
  typeof (value as Plugin).setup === "function"

// ---- Reading a settings file ---------------------------------------------------------------

async function readSettingsFile(file: string): Promise<SettingsFile | undefined> {
  const text = await readFile(file, "utf8").catch(() => undefined)
  if (text === undefined) return undefined

  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (cause) {
    throw new Error(`${file} is not valid JSON: ${cause instanceof Error ? cause.message : String(cause)}`)
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) throw new Error(`${file} must hold one JSON object.`)
  const settings = parsed as Record<string, unknown>

  const listOf = (key: string, ok: (item: unknown) => boolean, what: string) => {
    const value = settings[key]
    if (value === undefined) return
    if (!Array.isArray(value) || !value.every(ok)) throw new Error(`In ${file}, "${key}" must be a list of ${what}.`)
  }
  const isEntry = (item: unknown) =>
    typeof item === "string" || (typeof item === "object" && item !== null && typeof (item as { use?: unknown }).use === "string")
  listOf("plugins", isEntry, 'plugin names, paths, or { "use": ..., "options": ... } entries')
  listOf("trust", (item) => typeof item === "string", "project folders")
  listOf("disable", (item) => typeof item === "string", "plugin names")
  return settings as SettingsFile
}
