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
import { jobs } from "./plugins/jobs.ts"
import { models } from "./plugins/models/index.ts"
import { openrouter } from "./plugins/openrouter.ts"
import { MODES, permissions, type Mode, type Rule } from "./plugins/permissions/index.ts"
import { shell } from "./plugins/shell.ts"
import { skills } from "./plugins/skills.ts"
import { subagents } from "./plugins/subagents.ts"
import { undo } from "./plugins/undo.ts"
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
// `server` in the global file says where `evie serve` listens, and which web
// pages besides its own may connect:
//
//   { "server": { "host": "127.0.0.1", "port": 3843, "origins": ["http://localhost:5173"] } }
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
  // How freely Evie may act: the mode sessions start in, the rules, and the
  // owner's remembered "always" answers for each project. A project's own
  // file can set `mode` and `rules`.
  permissions?: { mode?: Mode; rules?: Rule[]; projects?: Record<string, Rule[]> }
  trust?: string[]
  // Projects the owner chose not to trust, so they are not asked again.
  untrusted?: string[]
  disable?: string[]
  server?: { host?: string; port?: number; origins?: string[] }
}

// What a built-in plugin is made from: its options from the settings file,
// and the things only Evie knows. The API key is never taken from options.
// `ownPermissions` is the mode and rules of the project a plugin is being
// loaded for, when its own settings file gives them.
type Context = {
  env: Settings
  home: string
  globalFile: string
  permissions: NonNullable<SettingsFile["permissions"]>
  ownPermissions?: { mode?: Mode; rules?: Rule[] }
  // Whether the owner has trusted a project, by its folder.
  trusted(folder: string): boolean
}
const BUILT_IN: Record<string, (options: Record<string, unknown>, context: Context) => Plugin> = {
  files: (options) => files(options as FilesOptions),
  shell: () => shell(),
  jobs: () => jobs(),
  guard: (_options, context) => guard({ dataFolder: context.home }),
  permissions: (_options, context) => permissionsFor(context, context.ownPermissions ?? {}),
  undo: (_options, context) => undo({ dataFolder: context.home }),
  models: () => models(),
  ask: () => ask(),
  subagents: (_options, context) => subagents({ dataFolder: context.home, trusted: context.trusted }),
  skills: (_options, context) => skills({ dataFolder: context.home, trusted: context.trusted }),
  instructions: (_options, context) => instructions({ dataFolder: context.home }),
  web: (_options, context) => web({ env: context.env }),
  openrouter: (_options, context) => openrouter({ env: context.env }),
}
// `undo` comes after `permissions`, so a call that is refused is not recorded.
const DEFAULT: Entry[] = ["files", "shell", "jobs", "permissions", "undo", "models", "ask", "subagents", "skills", "instructions", "web", "openrouter"]

// The permissions plugin as the settings describe it. A project's own mode
// wins over the global one, and its rules come after the global ones.
function permissionsFor(context: Context, own: { mode?: Mode; rules?: Rule[] }): Plugin {
  const mode = own.mode ?? context.permissions.mode
  return permissions({
    dataFolder: context.home,
    ...(mode !== undefined && { mode }),
    rules: [...(context.permissions.rules ?? []), ...(own.rules ?? [])],
    remembered: context.permissions.projects ?? {},
    remember: (project, rules) =>
      update(context.globalFile, (settings) => {
        const kept = ((settings.permissions ??= {}).projects ??= {})
        kept[project] = [...(kept[project] ?? []), ...rules]
      }),
  })
}
const TRUST = "Yes, trust this project"
const DO_NOT_TRUST = "No, ignore its settings file"

export async function loadSettingsFiles(options: { env: Settings }): Promise<SettingsFiles> {
  const home = dataFolder(options.env)
  const globalFile = join(home, "evie.json")
  const global = (await readSettingsFile(globalFile)) ?? {}
  const folders = (list: string[] | undefined) => new Set((list ?? []).map((folder) => resolve(expandHome(folder))))
  const trusted = folders(global.trust)
  const untrusted = folders(global.untrusted)
  // Asked each time it matters, since a project can be trusted while Evie runs.
  const context: Context = { env: options.env, home, globalFile, permissions: global.permissions ?? {}, trusted: (folder) => trusted.has(folder) }

  const plugins: Plugin[] = []
  for (const entry of global.plugins ?? DEFAULT) plugins.push(await load(entry, home, context))

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
          // On neither list: the owner has not decided, so this is for now,
          // and the next session in the project asks again.
          ...(!untrusted.has(project.folder) && { forNow: true as const }),
        }
      }

      const added: Plugin[] = []
      const disable = [...(own.disable ?? [])]
      for (const entry of own.plugins ?? []) {
        const plugin = await load(entry, project.folder, { ...context, ...(own.permissions && { ownPermissions: own.permissions }) })
        // Listing a plugin that is already global replaces it for this
        // project, with the options given here.
        if (plugins.some((global) => global.name === plugin.name)) disable.push(plugin.name)
        added.push(plugin)
      }
      // A project's own mode and rules: the permissions plugin is set up
      // again for this project with them added.
      // Unless the project lists the plugin itself, or switches it off.
      if (own.permissions && !disable.includes("permissions")) {
        if (plugins.some((global) => global.name === "permissions")) disable.push("permissions")
        added.push(permissionsFor(context, own.permissions))
        // Undo has to stay after permissions, so that it keeps no copy for a
        // call that is refused, and takes its copy after any question.
        if (plugins.some((global) => global.name === "undo") && !disable.includes("undo") && !added.some((plugin) => plugin.name === "undo")) {
          disable.push("undo")
          added.push(undo({ dataFolder: context.home }))
        }
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

// Changes the global settings file. The file is read again first, so nothing
// written to it since Evie started is lost, and replaced in one step, so it
// is never left half written. Changes are made one at a time.
let saving: Promise<void> = Promise.resolve()
function update(file: string, change: (settings: SettingsFile) => void): Promise<void> {
  const saved = saving.then(async () => {
    const settings: SettingsFile = (await readSettingsFile(file)) ?? {}
    change(settings)
    await writeFile(`${file}.new`, `${JSON.stringify(settings, null, 2)}\n`)
    await rename(`${file}.new`, file)
  })
  saving = saved.catch(() => {})
  return saved
}

// Adds a project folder to one of the two lists, and takes it off the other.
function remember(file: string, list: "trust" | "untrusted", folder: string): Promise<void> {
  return update(file, (settings) => {
    const other = list === "trust" ? "untrusted" : "trust"
    settings[list] = [...new Set([...(settings[list] ?? []), folder])]
    if (settings[other]) settings[other] = settings[other].filter((listed) => resolve(expandHome(listed)) !== folder)
  })
}

// ---- The server -----------------------------------------------------------------------------

// Where the server listens, the web pages besides its own that may connect
// to it, and the address a client on this computer connects to.
export type ServerSettings = { host: string; port: number; origins: string[]; address: string }

// The server listens on this computer only, unless the settings say
// otherwise. 3843 spells EVIE on a phone's keypad.
const SERVER_HOST = "127.0.0.1"
const SERVER_PORT = 3843

// The server's settings, from the global settings file. A client connects to
// `EVIE_SERVER` from the environment when it is set, as another device
// reaching this one does; an address given without `http://` is taken to
// have it.
export async function loadServerSettings(options: { env: Settings }): Promise<ServerSettings> {
  const global = (await readSettingsFile(join(dataFolder(options.env), "evie.json"))) ?? {}
  const host = global.server?.host ?? SERVER_HOST
  const port = global.server?.port ?? SERVER_PORT
  const given = options.env.EVIE_SERVER?.trim()
  const address = given ? (given.includes("://") ? given : `http://${given}`) : `http://${host}:${port}`
  return { host, port, origins: global.server?.origins ?? [], address }
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
  checkPermissions(settings.permissions, file)
  checkServer(settings.server, file)
  return settings as SettingsFile
}

function checkServer(value: unknown, file: string): void {
  if (value === undefined) return
  const wrong = (what: string): never => {
    throw new Error(`In ${file}, "server" ${what}`)
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) return wrong("must be an object.")
  const { host, port, origins } = value as Record<string, unknown>
  if (host !== undefined && (typeof host !== "string" || host.trim() === "")) wrong('has a "host" that is not an address, such as "127.0.0.1".')
  if (port !== undefined && !(Number.isInteger(port) && (port as number) >= 1 && (port as number) <= 65535)) {
    wrong('has a "port" that is not a whole number from 1 to 65535.')
  }
  const isPage = (origin: unknown) => typeof origin === "string" && URL.canParse(origin)
  if (origins !== undefined && !(Array.isArray(origins) && origins.every(isPage))) {
    wrong('has "origins" that are not a list of web addresses, such as "http://localhost:5173".')
  }
}

function checkPermissions(value: unknown, file: string): void {
  if (value === undefined) return
  const wrong = (what: string): never => {
    throw new Error(`In ${file}, "permissions" ${what}`)
  }
  if (typeof value !== "object" || value === null || Array.isArray(value)) return wrong("must be an object.")
  const { mode, rules, projects } = value as Record<string, unknown>
  if (mode !== undefined && !MODES.includes(mode as Mode)) wrong(`has a "mode" that is not one of: ${MODES.join(", ")}.`)
  const isRule = (rule: unknown) => {
    if (typeof rule !== "object" || rule === null) return false
    const { action, match, effect } = rule as Record<string, unknown>
    return typeof action === "string" && action !== "" && (match === undefined || typeof match === "string") && ["allow", "deny", "ask"].includes(effect as string)
  }
  const isRules = (list: unknown) => Array.isArray(list) && list.every(isRule)
  const shape = 'Each rule is { "action": ..., "match": ..., "effect": "allow", "deny" or "ask" }.'
  if (rules !== undefined && !isRules(rules)) wrong(`has "rules" that are not a list of rules. ${shape}`)
  if (projects !== undefined && (typeof projects !== "object" || projects === null || Array.isArray(projects) || !Object.values(projects).every(isRules))) {
    wrong(`has "projects" that do not give a list of rules for each project folder. ${shape}`)
  }
}
