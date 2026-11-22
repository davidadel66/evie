import { spawn } from "node:child_process"
import { readFile, stat } from "node:fs/promises"
import { join } from "node:path"
import { dataFolder } from "./core/evie.ts"
import type { Project, SettingsEditor, SettingsView } from "./core/types.ts"
import { ownInstructionsFile } from "./plugins/instructions.ts"
import type { SettingsFiles } from "./settings-files.ts"
import { keysInFile, keyValueProblem, replaceFile, writeKey, type Settings } from "./settings.ts"

// What the settings window reads and changes, for the server: the keys in
// the data folder's `.env`, the plugins `evie.json` lists, the owner's
// instruction file and which projects are trusted. Most of them are read
// only when the server starts, so a change to one applies once it starts
// again, and until then it is said to be pending.

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
  // Whether the server would start, with these plugin files added: what
  // would stop it, in words, or nothing. A fresh `evie serve --check` when
  // left out, which tests can stand in for.
  check?: (adding: string[]) => Promise<string | undefined>
  // How long, in milliseconds, that check may take before it is stopped:
  // 30 seconds when left out, which is for tests to change.
  checkWithin?: number
}

// The settings editor, and `close`, which the server calls as it stops.
export type SettingsEditorWithClose = SettingsEditor & {
  // Stops any check still running, so that none outlives the server.
  close(): void
}

// How long a check may take, in milliseconds.
const CHECK_WITHIN = 30_000

// A check running in a process of its own, which can be stopped.
type Running = { stop(why: "time" | "stopping"): void }

// The `evie` command, run to check the settings in a fresh process.
const CLI = join(import.meta.dir, "cli.ts")

// Checks the settings as the server will load them when it starts again, in
// a process of its own: `evie serve --check`, which loads the settings files
// and sets the plugins up together, as starting does, with no port and no
// sessions. A process of its own loads each plugin file as it is now, where
// this one keeps each as it was first loaded. It runs in a process group of
// its own, which is stopped as one once it has ended, or when it takes
// longer than `within`, or when the server stops, so that nothing it
// started, such as a command a plugin ran, is left behind.
function checkInFreshProcess(environment: Settings, adding: string[], within: number, running: Set<Running>): Promise<string | undefined> {
  const env = Object.fromEntries(Object.entries(environment).filter((entry): entry is [string, string] => entry[1] !== undefined))
  const child = spawn(process.execPath, [CLI, "serve", "--check", ...adding], { env, stdio: ["ignore", "ignore", "pipe"], detached: true })
  const stopGroup = () => {
    try {
      process.kill(-child.pid!, "SIGKILL")
    } catch {
      // The group has gone already.
    }
  }
  let said = ""
  child.stderr!.on("data", (chunk: Buffer) => (said += chunk.toString()))
  return new Promise((resolve) => {
    let why: "time" | "stopping" | undefined
    const check: Running = {
      stop(reason) {
        why ??= reason
        stopGroup()
      },
    }
    running.add(check)
    const timer = setTimeout(() => check.stop("time"), within)
    child.on("exit", () => {
      clearTimeout(timer)
      running.delete(check)
      stopGroup()
    })
    // Once every process of the group has gone, what it said is all there.
    child.on("close", (code) => {
      if (why === "time") {
        resolve(`The check that evie's server would start did not finish within ${seconds(within)}, so it was stopped. A plugin may be waiting on something as it loads or is set up.`)
      } else if (why === "stopping") resolve("The check that evie's server would start was stopped, since the server is stopping.")
      else resolve(code === 0 ? undefined : endOf(said.trim()) || `The check stopped with exit code ${code}, saying nothing.`)
    })
    child.on("error", (cause) => {
      clearTimeout(timer)
      running.delete(check)
      resolve(`The check that evie's server would start could not be run: ${cause.message}`)
    })
  })
}

// What a check said, as much of it as is worth showing, which can reach a
// phone: its last 40 lines, and of those the last 4 KB, saying when the
// start was left out.
const LINES = 40
const CHARACTERS = 4096
export function endOf(said: string): string {
  const lines = said.split("\n")
  const kept = lines.slice(-LINES).join("\n").slice(-CHARACTERS)
  return kept === said ? said : `(Only the end of what the check said is shown.)\n${kept}`
}

const seconds = (ms: number) => `${ms / 1000} second${ms === 1000 ? "" : "s"}`

export function settingsEditor(options: SettingsEditorOptions): SettingsEditorWithClose {
  const home = dataFolder(options.env)
  const started = Date.now()
  // The keys the server took from the file when it started: the ones its
  // settings have that its environment does not.
  const fromFile = Object.keys(options.env).filter((name) => options.environment[name] === undefined)

  // The keys the plugins loaded at the start say they read, then any other
  // the file sets now or set at the start.
  const declared = new Map(options.files.plugins.flatMap((plugin) => plugin.keys ?? []).map((key) => [key.name, key.about]))
  async function keys(): Promise<SettingsView["keys"]> {
    const inFile = await keysInFile(home)
    const names = [...new Set([...declared.keys(), ...Object.keys(inFile), ...fromFile])]
    return names.map((name) => ({
      name,
      ...(declared.has(name) && { about: declared.get(name)! }),
      inFile: Boolean(inFile[name]),
      environment: options.environment[name] !== undefined,
      // What the server would start with now is not what it started with.
      pending: (options.environment[name] ?? inFile[name]) !== options.env[name],
    }))
  }
  // Whether only the owner's user can read the file, or there is none yet.
  const keysPrivate = () =>
    stat(join(home, ".env")).then(
      (file) => (file.mode & 0o077) === 0,
      () => true,
    )

  async function setKey(name: string, value: string | null) {
    const known = (await keys()).map((key) => key.name)
    if (!known.includes(name)) throw new Error(`"${name}" is not one of the keys evie reads. They are: ${known.join(", ")}.`)
    if (value !== null && typeof value !== "string") throw new Error("A key's `value` is text, or null to clear it.")
    const given = value?.trim() ?? null
    const problem = given === null ? undefined : keyValueProblem(name, given)
    if (problem !== undefined) throw new Error(problem)
    await writeKey(home, name, given)
  }

  const running = new Set<Running>()
  const check = options.check ?? ((adding: string[]) => checkInFreshProcess(options.environment, adding, options.checkWithin ?? CHECK_WITHIN, running))

  // The plugins listed now, in order, then any loaded when the server
  // started that are no longer listed.
  async function plugins(): Promise<SettingsView["plugins"]> {
    const { plugins: listed, added } = await options.files.now()
    return [...new Set([...listed, ...options.files.listed])].map((name) => ({
      name,
      listed: listed.includes(name),
      loaded: options.files.listed.includes(name),
      added: added.includes(name),
    }))
  }

  // A plugin file is added once the server is known to start with it.
  async function addPlugin(path: string) {
    const { file } = options.files
    if ((await options.files.now()).plugins.includes(path)) throw new Error(`"${path}" is in the list of plugins in ${file} already.`)
    const problem = await check([path])
    if (problem !== undefined) throw new Error(problem)
    await options.files.addPlugin(path)
  }

  // A plugin is named by text, which a client that is not type-checked may not send.
  const pluginName = (name: unknown) => {
    const given = typeof name === "string" ? name.trim() : ""
    if (given === "") throw new Error("Give a built-in plugin's name, or the full path to a plugin file.")
    return given
  }

  // The owner's instruction file is read before every request, so a change
  // to it applies from the next one.
  const instructionsFile = ownInstructionsFile(home)
  const instructions = () => readFile(instructionsFile, "utf8").catch((cause: { code?: string }) => (cause.code === "ENOENT" ? "" : Promise.reject(cause)))
  async function saveInstructions(text: unknown) {
    if (typeof text !== "string") throw new Error("`instructions` is the text of the instruction file.")
    await replaceFile(instructionsFile, text)
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
      const [listedKeys, secret, listedPlugins, text, listedProjects] = await Promise.all([keys(), keysPrivate(), plugins(), instructions(), projectsOf(projects)])
      return {
        keys: listedKeys,
        keysPrivate: secret,
        plugins: listedPlugins,
        instructions: text,
        projects: listedProjects,
        restart: [...listedKeys, ...listedProjects].some((each) => each.pending) || listedPlugins.some((plugin) => plugin.listed !== plugin.loaded),
        started,
      }
    },
    async change(change, projects) {
      if ("key" in change) return setKey(change.key, change.value)
      if ("addPlugin" in change) return addPlugin(pluginName(change.addPlugin))
      if ("removePlugin" in change) return options.files.removePlugin(pluginName(change.removePlugin))
      if ("instructions" in change) return saveInstructions(change.instructions)
      if ("project" in change) return trustProject(change.project, change.trusted, projects)
      throw new Error(
        'A change is one of: { "key": ..., "value": ... }, { "addPlugin": ... }, { "removePlugin": ... }, { "instructions": ... } or { "project": ..., "trusted": ... }.',
      )
    },
    // The settings are checked first, as the server will load them when it
    // starts again, so that one that would not start is not stopped. It is
    // stopped only once this command's reply has gone out.
    async restart(asked) {
      if (asked !== started) return
      const problem = await check([])
      if (problem !== undefined) throw new Error(`evie's server was not restarted, since it would not start again. ${problem}`)
      setTimeout(options.restart, 0)
    },
    close() {
      for (const each of running) each.stop("stopping")
    },
  }
}
