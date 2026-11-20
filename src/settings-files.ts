import { readFile, rename, writeFile } from "node:fs/promises"
import { homedir } from "node:os"
import { isAbsolute, join, resolve } from "node:path"
import { pathToFileURL } from "node:url"
import { dataFolder } from "./core/evie.ts"
import type { Ask, Plugin, Project, ProjectSettings } from "./core/types.ts"
import { ask } from "./plugins/ask.ts"
import { files, type FilesOptions } from "./plugins/files/index.ts"
import { guard } from "./plugins/guard.ts"
import { instructions } from "./plugins/instructions.ts"
import { models } from "./plugins/models/index.ts"
import { openrouter } from "./plugins/openrouter.ts"
import { shell } from "./plugins/shell.ts"
import { web } from "./plugins/web.ts"
import type { Settings } from "./settings.ts"

// Evie's settings files: which model to use, how long a reply may be, and
// which plugins to load. Keys are not kept here; they come from the
// environment, or the `.env` file beside this one.
//
// The global file is `evie.json` in the data folder. It applies to every
// project. A project can have a file of its own at `.evie/evie.json` in its
// folder, which can set that project's model and reply limit, add plugins
// for that project only, and switch off a global one.
//
//   { "model": "some/model", "maxTokens": 16000,
//     "plugins": ["files", { "use": "shell" }, "/path/to/my-plugin.ts"],
//     "trust": ["/path/to/a/project"] }
//
// A setting in the environment, `EVIE_MODEL` or `EVIE_MAX_TOKENS`, wins over
// both files, so one run can be pointed at another model.
//
// A plugin is a built-in one, named, or a file, given by its path. A plugin
// file is code that runs with Evie's full permissions, and switching a plugin
// off can remove a protection such as the guard. So a project's settings file
// is used only when the global file lists that project under "trust".
// Otherwise it is ignored as a whole, and opening someone else's repository
// can neither run their code nor change what Evie loads.
//
// The first time a project with a settings file is opened, the owner is
// asked whether to trust it. The answer is remembered in the global file:
// under "trust" for a yes and "untrusted" for a no. A question that could
// not be put, or was not answered, is a no for now and is asked again.

export type SettingsFiles = {
  // The global settings file, whether or not it exists yet.
  file: string
  // The model sessions start with, when one is set.
  model?: string
  // The longest a reply may be, when a limit is set.
  maxTokens?: number
  // When a long conversation is compacted, when the settings say.
  compaction?: { ceiling?: number; limits?: Record<string, number> }
  // The global plugins, in the order listed.
  plugins: Plugin[]
  // What a project adds and switches off.
  projectSettings(project: Project, context?: { ask: Ask }): Promise<ProjectSettings>
}

type Entry = string | { use: string; options?: Record<string, unknown> }
type SettingsFile = {
  model?: string
  maxTokens?: number
  // The size, in tokens, at which a conversation is compacted.
  compactAt?: number
  // What a model can read, for a model whose limit is not known or is wrong.
  contextLimits?: Record<string, number>
  plugins?: Entry[]
  trust?: string[]
  // Projects the owner chose not to trust, so they are not asked again.
  untrusted?: string[]
  disable?: string[]
}

// What a built-in plugin is made from: its options from the settings file,
// and the things only Evie knows. The API key is never taken from options.
type Context = { env: Settings; home: string }
const BUILT_IN: Record<string, (options: Record<string, unknown>, context: Context) => Plugin> = {
  files: (options) => files(options as FilesOptions),
  shell: () => shell(),
  guard: (_options, context) => guard({ dataFolder: context.home }),
  models: () => models(),
  ask: () => ask(),
  instructions: (_options, context) => instructions({ dataFolder: context.home }),
  web: (_options, context) => web({ env: context.env }),
  openrouter: (_options, context) => openrouter({ env: context.env }),
}
const DEFAULT: Entry[] = ["files", "shell", "guard", "models", "ask", "instructions", "web", "openrouter"]
const TRUST = "Yes, trust this project"
const DO_NOT_TRUST = "No, ignore its settings file"

export async function loadSettingsFiles(options: { env: Settings }): Promise<SettingsFiles> {
  const home = dataFolder(options.env)
  const context: Context = { env: options.env, home }
  const globalFile = join(home, "evie.json")
  const global = (await readSettingsFile(globalFile)) ?? {}

  const plugins: Plugin[] = []
  for (const entry of global.plugins ?? DEFAULT) plugins.push(await load(entry, home, context))
  const folders = (list: string[] | undefined) => new Set((list ?? []).map((folder) => resolve(expandHome(folder))))
  const trusted = folders(global.trust)
  const untrusted = folders(global.untrusted)

  // The environment wins over both files.
  const fromEnv = {
    model: options.env.EVIE_MODEL?.trim() || undefined,
    maxTokens: wholeNumber(options.env.EVIE_MAX_TOKENS),
  }
  const model = fromEnv.model ?? global.model
  const maxTokens = fromEnv.maxTokens ?? global.maxTokens

  return {
    file: globalFile,
    ...(model !== undefined && { model }),
    ...(maxTokens !== undefined && { maxTokens }),
    ...((global.compactAt !== undefined || global.contextLimits !== undefined) && {
      compaction: {
        ...(global.compactAt !== undefined && { ceiling: global.compactAt }),
        ...(global.contextLimits !== undefined && { limits: global.contextLimits }),
      },
    }),
    plugins,
    async projectSettings(project, asking) {
      const projectFile = join(project.folder, ".evie", "evie.json")
      const own = await readSettingsFile(projectFile)
      if (!own) return { plugins: [], disable: [], notices: [] }

      // A project that is on neither list is asked about, once.
      if (!trusted.has(project.folder) && !untrusted.has(project.folder) && asking) {
        const answer = await asking.ask({
          text:
            `This project has a settings file of its own, ${projectFile}. It can load plugins, which are code that ` +
            `runs with your full permissions, and it can switch off protections such as the guard. Use it?`,
          choices: [TRUST, DO_NOT_TRUST],
        })
        // Written down first: an answer that could not be saved does not count.
        if (answer === TRUST) {
          await remember(globalFile, "trust", project.folder)
          trusted.add(project.folder)
        }
        if (answer === DO_NOT_TRUST) {
          await remember(globalFile, "untrusted", project.folder)
          untrusted.add(project.folder)
        }
      }

      if (!trusted.has(project.folder)) {
        const how = untrusted.has(project.folder)
          ? `move "${project.folder}" from "untrusted" to "trust" in ${globalFile}`
          : `add "${project.folder}" to "trust" in ${globalFile}`
        return {
          plugins: [],
          disable: [],
          notices: [
            `This project has a settings file of its own, ${projectFile}, which was ignored because the project is ` +
              `not trusted. To use it, ${how}.`,
          ],
        }
      }

      const added: Plugin[] = []
      const disable = [...(own.disable ?? [])]
      for (const entry of own.plugins ?? []) {
        const plugin = await load(entry, project.folder, context)
        // Listing a plugin that is already global replaces it for this
        // project, with the options given here.
        if (plugins.some((global) => global.name === plugin.name)) disable.push(plugin.name)
        added.push(plugin)
      }
      const ownModel = fromEnv.model ? undefined : own.model
      const ownLimit = fromEnv.maxTokens ? undefined : own.maxTokens
      return {
        plugins: added,
        disable,
        notices: [],
        ...(ownModel !== undefined && { model: ownModel }),
        ...(ownLimit !== undefined && { maxTokens: ownLimit }),
      }
    },
  }
}

// Adds a project folder to one of the lists in the global settings file. The
// file is read again first, so nothing written to it since Evie started is
// lost, and replaced in one step, so it is never left half written. Answers
// are saved one at a time.
let saving: Promise<void> = Promise.resolve()
function remember(file: string, list: "trust" | "untrusted", folder: string): Promise<void> {
  const saved = saving.then(() => save(file, list, folder))
  saving = saved.catch(() => {})
  return saved
}
async function save(file: string, list: "trust" | "untrusted", folder: string): Promise<void> {
  const settings: SettingsFile = (await readSettingsFile(file)) ?? {}
  const other = list === "trust" ? "untrusted" : "trust"
  settings[list] = [...new Set([...(settings[list] ?? []), folder])]
  if (settings[other]) settings[other] = settings[other].filter((listed) => resolve(expandHome(listed)) !== folder)
  await writeFile(`${file}.new`, `${JSON.stringify(settings, null, 2)}\n`)
  await rename(`${file}.new`, file)
}

// ---- Loading one plugin -----------------------------------------------------------------

async function load(entry: Entry, baseFolder: string, context: Context): Promise<Plugin> {
  const name = typeof entry === "string" ? entry : entry.use
  const options = typeof entry === "string" ? {} : (entry.options ?? {})

  if (Object.hasOwn(BUILT_IN, name)) return BUILT_IN[name]!(options, context)

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

const wholeNumber = (text: string | undefined) => {
  const value = Number(text)
  return Number.isInteger(value) && value > 0 ? value : undefined
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
  let text: string
  try {
    text = await readFile(file, "utf8")
  } catch (cause) {
    // A file that is not there is fine. One that is there and cannot be read
    // is a problem to report, not to pass over.
    if ((cause as { code?: string }).code === "ENOENT") return undefined
    throw new Error(`${file} could not be read: ${cause instanceof Error ? cause.message : String(cause)}`)
  }

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
  listOf("untrusted", (item) => typeof item === "string", "project folders")
  listOf("disable", (item) => typeof item === "string", "plugin names")
  if (settings.model !== undefined && (typeof settings.model !== "string" || settings.model.trim() === "")) {
    throw new Error(`In ${file}, "model" must be the name of a model.`)
  }
  const positive = (value: unknown) => Number.isInteger(value) && (value as number) > 0
  for (const key of ["maxTokens", "compactAt"]) {
    if (settings[key] !== undefined && !positive(settings[key])) throw new Error(`In ${file}, "${key}" must be a whole number above zero.`)
  }
  const limits = settings.contextLimits
  if (limits !== undefined && (typeof limits !== "object" || limits === null || Array.isArray(limits) || !Object.values(limits).every(positive))) {
    throw new Error(`In ${file}, "contextLimits" must give a whole number above zero for each model name.`)
  }
  return settings as SettingsFile
}
