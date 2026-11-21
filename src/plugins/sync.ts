import { existsSync, realpathSync } from "node:fs"
import { isAbsolute, join, relative, resolve, sep } from "node:path"
import { dataFolder } from "../core/evie.ts"
import type { CheckedToolCall, Plugin, Project } from "../core/types.ts"
import { patchFiles } from "./files/paths.ts"

// The built-in sync plugin. It backs up a project's `.evie/` folder, where
// Evie keeps what it adds to the project, for a project whose own settings
// say `"sync": "private"`. The folder is made a git repository of its own,
// and the first time, a private repository on the owner's GitHub account,
// named `evie-` and the project's name, with the owner's `gh`. A minute
// after Evie last changed something there, and once an hour, what changed
// is committed and pushed, without asking: the setting is the owner's
// permission. `/sync` does it at once, and says how it went.
//
// A try that fails, as with no `gh`, `gh` not logged in, or no network,
// never fails a turn: it is kept for `/sync` to tell, and the next try tries
// again. Nothing is ever pushed by force.

// How a command went: its exit code and what it printed.
export type Ran = { code: number; stdout: string; stderr: string }

export type SyncOptions = {
  // The folder Evie keeps its state in, which is never synced. Defaults to the usual place.
  dataFolder?: string
  // Where the project's `.evie/` syncs, asked before each try. Only
  // "private" is synced.
  setting(project: Project): Promise<"private" | "none" | undefined>
  // How long after Evie last changed something to sync, and how often to
  // sync whatever happens, in milliseconds: a minute and an hour unless given.
  after?: number
  every?: number
  // Runs a command, `git` or `gh`, in a folder. Replaceable, so tests never reach GitHub.
  run?: (command: string[], cwd: string) => Promise<Ran>
  // Stops the timers, for good.
  signal?: AbortSignal
}

// The tools that change the files they name.
const FILE_TOOLS = ["write", "edit", "patch"]

// One project's syncing: its timers, the try under way, and why the last
// one failed, when it did.
type State = { project: Project; soon?: ReturnType<typeof setTimeout>; hourly?: ReturnType<typeof setInterval>; queue: Promise<unknown>; problem: string | undefined }

// How a try went, in words for the owner, and why it failed, when it did.
type Outcome = { text: string; reason?: string }

// Runs one command of a try, named for the owner, and returns what it
// printed. One that fails throws, saying what it said, which ends the try.
type Step = (name: string, command: string[]) => Promise<string>

export function sync(options: SyncOptions): Plugin {
  const after = options.after ?? 60_000
  const every = options.every ?? 60 * 60_000
  const run = options.run ?? runCommand
  const data = options.dataFolder ?? dataFolder({})
  const states = new Map<string, State>()

  options.signal?.addEventListener("abort", () => {
    for (const state of states.values()) {
      clearTimeout(state.soon)
      clearInterval(state.hourly)
    }
  })

  // One try at a time for each project: a try asked for while one is under
  // way waits for it. Why the last one failed is kept until one does not.
  // `before` is why the try before this one failed, if it did.
  function attempt(state: State): Promise<Outcome & { before?: string }> {
    const tried = state.queue.then(async () => {
      const before = state.problem
      const outcome = await once(state.project)
      state.problem = outcome.reason
      return { ...outcome, ...(before !== undefined && { before }) }
    })
    state.queue = tried
    return tried
  }

  // Anything that goes wrong is how the try went, never an error: a try
  // runs on a timer, where an error would reach no one.
  async function once(project: Project): Promise<Outcome> {
    const evie = join(project.folder, ".evie")
    try {
      const setting = await options.setting(project)
      if (setting !== "private") return { text: notSynced(setting) }
      return { text: await push(project, evie) }
    } catch (cause) {
      const reason = cause instanceof Error ? cause.message : String(cause)
      return { text: `Could not sync ${evie}: ${reason} It is tried again a minute after Evie next changes something there, and every hour.`, reason }
    }
  }

  // Commits what changed in the `.evie/` folder and pushes it, making its
  // repositories the first time. Returns what happened, in words.
  async function push(project: Project, evie: string): Promise<string> {
    // As for a project at the home folder, whose .evie folder is the data folder's usual place.
    const [here, kept] = [real(evie), real(data)]
    if (within(here, kept) || within(kept, here)) throw new Error("It is Evie's data folder, which holds its keys and state, and is never synced.")
    const step: Step = async (name, command) => {
      const done = await run(command, evie)
      if (done.code !== 0) throw new Error(`\`${name}\` failed: ${(done.stderr.trim() || done.stdout.trim() || `it exited with ${done.code}`).replace(/\.*$/, ".")}`)
      return done.stdout
    }
    // Named in full, so that a `.evie/` folder that is not yet a repository
    // of its own can never be taken for the project's repository around it.
    const gitDir = join(evie, ".git")
    const git = (...args: string[]) => step(`git ${args[0]}`, ["git", `--git-dir=${gitDir}`, `--work-tree=${evie}`, ...args])

    if (!existsSync(gitDir)) await step("git init", ["git", "init", "--quiet", "--initial-branch=main", evie])
    // What changed, and how far the branch is ahead of what was last pushed:
    // all of it when it has never been pushed.
    const status = await git("status", "--porcelain=v2", "--branch", "--untracked-files=all")
    const lines = status.split("\n")
    const changed = lines.some((line) => line !== "" && !line.startsWith("#"))
    const pushedBefore = lines.some((line) => line.startsWith("# branch.upstream "))
    const ahead = Number(/^# branch\.ab \+(\d+)/m.exec(status)?.[1] ?? 0) > 0
    if (!changed && (lines.includes("# branch.oid (initial)") || (pushedBefore && !ahead))) {
      return `Nothing to sync: ${evie} has not changed since it was last pushed.`
    }
    if (changed) {
      await git("add", "--all")
      await git("commit", "--quiet", "--message", "Sync from evie")
    }
    const first = (await run(["git", `--git-dir=${gitDir}`, "remote", "get-url", "origin"], evie)).code !== 0 ? await connect(project, evie, step, git) : undefined
    await git("push", "--quiet", "--set-upstream", "origin", "HEAD")
    if (first !== undefined) return `${first} for ${evie}, and pushed what it holds there.`
    return `Pushed the changes in ${evie} to ${(await git("remote", "get-url", "origin")).trim()}.`
  }

  // Gives the `.evie/` repository its remote: the project's private
  // repository on GitHub, made now unless one of its name is already there.
  // Returns what was done, in words.
  async function connect(project: Project, evie: string, step: Step, git: (...args: string[]) => Promise<string>): Promise<string> {
    const name = repositoryName(project)
    const found = await run(["gh", "repo", "view", name, "--json", "isPrivate,url,sshUrl"], evie)
    if (found.code !== 0) {
      await step("gh repo create", ["gh", "repo", "create", name, "--private", "--source", evie, "--remote", "origin"])
      return `Made the private GitHub repository ${name}`
    }
    const there = JSON.parse(found.stdout) as { isPrivate: boolean; url: string; sshUrl: string }
    if (!there.isPrivate) {
      throw new Error(
        `A repository named ${name} is already on your GitHub account, and it is public, so nothing was pushed to it. ` +
          "Make it private, or rename it so that Evie makes a private one.",
      )
    }
    // By the way the owner's `gh` reaches GitHub, as `gh repo create` would:
    // over https unless it says ssh.
    const protocol = await run(["gh", "config", "get", "git_protocol"], evie)
    await git("remote", "add", "origin", protocol.code === 0 && protocol.stdout.trim() === "ssh" ? there.sshUrl : there.url)
    return `Used the private GitHub repository ${name}, already on the account,`
  }

  return {
    name: "sync",
    setup(host) {
      const project = host.project
      host.commands.add({
        name: "sync",
        description: "Back up this project's .evie folder to its private repository now.",
        async run() {
          if (project === null) return "This session belongs to no project, so there is nothing to sync."
          const { text, reason, before } = await attempt(stateOf(project))
          return before !== undefined && reason === undefined ? `${text} The try before this one had failed: ${before}` : text
        },
      })
      if (project === null) return
      const state = stateOf(project)
      // Each change puts the try off again, so a burst of changes is one commit.
      host.on("after_tool", (call) => {
        if (options.signal?.aborted || !changesEvie(call, project.folder)) return
        clearTimeout(state.soon)
        state.soon = setTimeout(() => void attempt(state), after)
        state.soon.unref()
      })
    },
  }

  function stateOf(project: Project): State {
    let state = states.get(project.id)
    if (!state) {
      const made: State = { project, queue: Promise.resolve(), problem: undefined }
      if (!options.signal?.aborted) made.hourly = setInterval(() => void attempt(made), every).unref()
      states.set(project.id, (state = made))
    }
    return state
  }
}

// Whether a tool call may have changed something in the project's `.evie/`
// folder: a file tool that names a file in it, or any command, since what a
// command changes cannot be told from its text.
function changesEvie(call: CheckedToolCall, folder: string): boolean {
  if (call.name === "shell") return true
  if (!FILE_TOOLS.includes(call.name)) return false
  const named = call.name === "patch" ? patchFiles(String(call.input.patchText ?? "")) : typeof call.input.path === "string" ? [call.input.path] : []
  const evie = join(folder, ".evie")
  return named.some((path) => within(resolve(folder, path), evie))
}

// Whether a path is a folder or inside it. A path inside it may have a name
// that starts with two dots, such as `..cache`.
function within(path: string, folder: string): boolean {
  const inside = relative(folder, path)
  return inside !== ".." && !inside.startsWith(`..${sep}`) && !isAbsolute(inside)
}

// A path with its links resolved, or as it is when it cannot be.
function real(path: string): string {
  try {
    return realpathSync(path)
  } catch {
    return resolve(path)
  }
}

// What `/sync` says for a project that is not synced.
function notSynced(setting: "none" | undefined): string {
  if (setting === "none") return 'This project\'s .evie folder is kept on this computer only: its "sync" setting is "none".'
  return (
    "This project's .evie folder is not synced. To back it up to a private GitHub repository, set \"sync\": \"private\" " +
    "in its .evie/evie.json; that file is used once the project is trusted."
  )
}

// The name of a project's private repository: `evie-` and its name, in
// lower case, with spaces and other characters made into dashes.
function repositoryName(project: Project): string {
  const name = project.name
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return `evie-${name || project.id.slice(0, 8)}`
}

// Runs a command, never waiting for a password to be typed, and for at most
// two minutes, so a network that has gone cannot hold a try up for ever.
async function runCommand(command: string[], cwd: string): Promise<Ran> {
  try {
    const child = Bun.spawn(command, {
      cwd,
      env: { ...process.env, GIT_TERMINAL_PROMPT: "0", GH_PROMPT_DISABLED: "1" },
      stdin: "ignore",
      stdout: "pipe",
      stderr: "pipe",
      timeout: 2 * 60_000,
    })
    const [stdout, stderr, code] = await Promise.all([new Response(child.stdout).text(), new Response(child.stderr).text(), child.exited])
    if (child.signalCode !== null) return { code: code || 1, stdout, stderr: `${stderr.trim()} It took more than two minutes, so it was stopped.`.trim() }
    return { code, stdout, stderr }
  } catch (cause) {
    return { code: 127, stdout: "", stderr: `${command[0]} could not be started: is it installed, and on the PATH Evie's server runs with? (${cause instanceof Error ? cause.message : String(cause)})` }
  }
}
