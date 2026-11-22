// How Evie runs git, in the core and its built-in plugins. A repository's
// own settings, and the owner's, can name commands for git to run, and a
// project may be a repository just cloned, so every git Evie runs is given
// settings that turn each of them off, whichever settings file names it:
//
// - no file system monitor (core.fsmonitor), and no hooks (core.hooksPath),
//   with no index written that would run one (GIT_OPTIONAL_LOCKS);
// - no external diff (diff.external, and --no-ext-diff for a diff driver's
//   command, which a caller that diffs passes), and no pager (--no-pager);
// - no signing (commit.gpgSign, tag.gpgSign), and no automatic upkeep in
//   the background (gc.auto, maintenance.auto);
// - with `worktree`, for a command that reads the files in the folder, no
//   filter: each one the settings name has its clean, smudge and process
//   commands set to none, by the filter's name;
// - unless `remote`, nothing that reaches another computer: no transport at
//   all (protocol.allow), no fetching of an object a partial clone lacks
//   (GIT_NO_LAZY_FETCH, which is what --no-lazy-fetch sets), no credential
//   helper, ssh command or password program (credential.helper,
//   core.sshCommand, core.askPass, and SSH_ASKPASS), and never a prompt
//   (GIT_TERMINAL_PROMPT).
//
// The settings are given in the environment (GIT_CONFIG_COUNT, with a
// GIT_CONFIG_KEY_n and GIT_CONFIG_VALUE_n each), which git reads over every
// settings file and which, unlike `-c name=value`, holds a filter whose name
// has "=" in it. Git's own settings in the server's environment, such as
// GIT_DIR, which would point it elsewhere, are left out.

export type GitRun = { code: number; stdout: Buffer; stderr: string; stopped: boolean }
export type GitOptions = {
  // For a command that reads the files in the folder, such as diff, status,
  // add or commit: every filter is turned off too.
  worktree?: boolean
  // For a command that must reach a remote, such as push: transports,
  // credential helpers and the ssh command are left as the settings have them.
  remote?: boolean
  input?: Uint8Array
  // In milliseconds: stopped after this long. Thirty seconds unless given.
  timeout?: number
  signal?: AbortSignal
}

const ALWAYS: [string, string][] = [
  ["core.fsmonitor", "false"],
  ["core.hooksPath", "/dev/null"],
  ["diff.external", ""],
  ["commit.gpgSign", "false"],
  ["tag.gpgSign", "false"],
  ["gc.auto", "0"],
  ["maintenance.auto", "false"],
]
const LOCAL: [string, string][] = [
  ["protocol.allow", "never"],
  ["credential.helper", ""],
  ["core.sshCommand", ""],
  ["core.askPass", ""],
]

// The server's environment without git's own settings, nor, for a command
// that stays on this computer, a password program for ssh.
function baseEnv(remote: boolean): Record<string, string> {
  const kept = Object.entries(process.env).filter(([name, value]) => value !== undefined && !name.startsWith("GIT_") && (remote || name !== "SSH_ASKPASS"))
  return Object.fromEntries(kept) as Record<string, string>
}

// The environment that gives git the settings above.
function envWith(settings: [string, string][], remote: boolean): Record<string, string> {
  const env = baseEnv(remote)
  env.GIT_TERMINAL_PROMPT = "0"
  env.GIT_OPTIONAL_LOCKS = "0"
  if (!remote) env.GIT_NO_LAZY_FETCH = "1"
  env.GIT_CONFIG_COUNT = String(settings.length)
  for (const [at, [key, value]] of settings.entries()) {
    env[`GIT_CONFIG_KEY_${at}`] = key
    env[`GIT_CONFIG_VALUE_${at}`] = value
  }
  return env
}

// The environment for a git that stays on this computer and does not read
// the folder's files.
const gitEnvLocal = () => envWith([...ALWAYS, ...LOCAL], false)

// Only git's part of that environment, for another program that runs git,
// such as `gh`, to add to its own.
export const gitSettingsOnly = () => Object.fromEntries(Object.entries(gitEnvLocal()).filter(([name]) => name.startsWith("GIT_")))

// The filters git's settings name, wherever they are set. Reading the
// settings runs nothing.
async function filterNames(folder: string): Promise<string[]> {
  const named = await spawn(folder, ["config", "--name-only", "-z", "--get-regexp", "^filter\\."], gitEnvLocal(), {})
  // Each name is "filter.<name>.<key>", and the name may hold dots or "=".
  const names = named.stdout.toString("utf8").split("\0").flatMap((line) => /^filter\.(.+)\.[^.]+$/s.exec(line)?.[1] ?? [])
  return [...new Set(names)]
}

// The environment for a git command run as the options say.
export async function gitEnv(folder: string, options: { worktree?: boolean; remote?: boolean } = {}): Promise<Record<string, string>> {
  const filters = options.worktree ? await filterNames(folder) : []
  const off = filters.flatMap((filter): [string, string][] => [
    [`filter.${filter}.clean`, ""],
    [`filter.${filter}.smudge`, ""],
    [`filter.${filter}.process`, ""],
    [`filter.${filter}.required`, "false"],
  ])
  return envWith([...ALWAYS, ...(options.remote ? [] : LOCAL), ...off], options.remote === true)
}

// Runs git in a folder, as above, and gives back how it went. A git that
// cannot be started is code 127.
export async function runGit(folder: string, args: string[], options: GitOptions = {}): Promise<GitRun> {
  return spawn(folder, ["--no-pager", ...args], await gitEnv(folder, options), options)
}

async function spawn(folder: string, args: string[], env: Record<string, string>, options: GitOptions): Promise<GitRun> {
  try {
    const child = Bun.spawn(["git", ...args], {
      cwd: folder,
      env,
      stdin: options.input ?? "ignore",
      stdout: "pipe",
      stderr: "pipe",
      timeout: options.timeout ?? 30_000,
      ...(options.signal && { signal: options.signal }),
    })
    const [said, problem, code] = await Promise.all([new Response(child.stdout).arrayBuffer(), new Response(child.stderr).text(), child.exited])
    return { code, stdout: Buffer.from(said), stderr: problem, stopped: child.signalCode !== null }
  } catch (cause) {
    return { code: 127, stdout: Buffer.alloc(0), stderr: `git could not be started: ${cause instanceof Error ? cause.message : String(cause)}`, stopped: false }
  }
}

// The same, waiting for it, for a caller that cannot wait otherwise. It
// does not turn filters off, so it is not for a command that reads the
// folder's files.
export function runGitSync(folder: string, args: string[], options: { timeout?: number } = {}): { code: number; stdout: Buffer } {
  try {
    const done = Bun.spawnSync(["git", "--no-pager", ...args], { cwd: folder, env: gitEnvLocal(), stdin: "ignore", stdout: "pipe", stderr: "ignore", timeout: options.timeout ?? 30_000 })
    return { code: done.exitCode ?? 1, stdout: done.stdout }
  } catch {
    return { code: 127, stdout: Buffer.alloc(0) }
  }
}
