import { lstat, readFile, readlink } from "node:fs/promises"
import { isAbsolute, relative } from "node:path"
import { compareApart } from "./diff.ts"
import { inGit } from "./projects.ts"
import type { ChangedFile, ChangeKeeper, Commands, FileDiff, Project, StoredEvent, TurnChanges } from "./types.ts"

// What a session changed, as the `list_changes` and `read_changes` commands
// give it: by turn and for the whole session, from the files a plugin
// keeps, such as undo's copies; and with git, the work not yet committed
// and the branch's changes against its base.

// A file's path as a client is shown it: from the project's folder when it
// is inside it, and in full otherwise.
function shown(path: string, project: Project | null): string {
  if (project === null) return path
  const inside = relative(project.folder, path)
  return inside === "" || inside.startsWith("..") || isAbsolute(inside) ? path : inside
}

// What is at a path now: a file's contents, a link's target, or null when
// there is nothing there. Undefined when it cannot be read, as for a folder.
export async function contentsNow(path: string): Promise<Uint8Array | null | undefined> {
  try {
    const kind = await lstat(path)
    if (kind.isSymbolicLink()) return new TextEncoder().encode(await readlink(path))
    return await readFile(path)
  } catch (cause) {
    const code = (cause as { code?: string }).code
    return code === "ENOENT" || code === "ENOTDIR" ? null : undefined
  }
}

// Each turn's number, from 1, with its start's and end's events.
function turnsIn(events: readonly StoredEvent[]): { start: StoredEvent; end?: StoredEvent }[] {
  const turns: { start: StoredEvent; end?: StoredEvent }[] = []
  for (const event of events) {
    if (event.type === "turn_start") turns.push({ start: event })
    if (event.type === "turn_end" && turns.length > 0 && turns.at(-1)!.end === undefined) turns.at(-1)!.end = event
  }
  return turns
}

const summed = ({ patch: _, ...file }: FileDiff): ChangedFile => file

// The most diff text one answer carries. The files past it are too large
// to show, so that an answer stays well within what a client takes in one
// message, and what a page can draw.
const MOST_IN_ONE = 4 * 1024 * 1024
function capped(files: FileDiff[]): FileDiff[] {
  let total = 0
  return files.map((file) => {
    total += file.patch?.length ?? 0
    return total <= MOST_IN_ONE || file.patch === undefined ? file : { ...summed(file), tooLarge: true }
  })
}

// A file past the most one comparison reads, listed by its path alone.
const tooManyAt = (path: string): FileDiff => ({ path, status: "changed", added: 0, removed: 0, tooMany: true })

// What the turns of sessions changed, from what a plugin keeps of them,
// such as undo's copies. A turn that has ended changes no more, so what it
// changed is worked out once and kept, for the sessions used last; the
// turn running now is worked out each time it is asked for. A turn undone
// is no longer kept by the plugin, and so no longer listed.
export function createTurnChanges() {
  type Kept = { paths: string; files: ChangedFile[] }
  const sessions = new Map<string, Map<number, Kept>>()
  // How many sessions' turns are kept.
  const SESSIONS = 100
  function keptFor(session: string): Map<number, Kept> {
    const kept = sessions.get(session) ?? new Map<number, Kept>()
    // The latest used goes last, so the first is the one used longest ago.
    sessions.delete(session)
    sessions.set(session, kept)
    if (sessions.size > SESSIONS) sessions.delete(sessions.keys().next().value!)
    return kept
  }

  // One turn's files, compared, with their diffs or only how many lines.
  async function turnFiles(keeper: ChangeKeeper, session: string, turn: number, project: Project | null, options: { patch: boolean }): Promise<FileDiff[]> {
    const files = await keeper.files(session, turn)
    const compared = await fewAtOnce(files.slice(0, MOST_FILES), (file) => compareApart(shown(file.path, project), file.before, file.after, options))
    return [...compared.filter((file) => file !== undefined), ...files.slice(MOST_FILES).map((file) => tooManyAt(shown(file.path, project)))]
  }

  // Every file the turns changed, each from what it held before the first
  // turn that changed it to what it holds now.
  async function sessionFiles(keeper: ChangeKeeper, session: string, project: Project | null, options: { patch: boolean }): Promise<FileDiff[]> {
    const first = new Map<string, number>()
    for (const { turn, paths } of await keeper.turns(session)) for (const path of paths) if (!first.has(path)) first.set(path, turn)
    const paths = [...first.keys()]
    const read = paths.slice(0, MOST_FILES)
    const turns = new Map<number, Map<string, Uint8Array | null | undefined>>()
    for (const turn of new Set(read.map((path) => first.get(path)!))) {
      turns.set(turn, new Map((await keeper.files(session, turn)).map((file) => [file.path, file.before])))
    }
    const compared = await fewAtOnce(read, async (path) => compareApart(shown(path, project), turns.get(first.get(path)!)!.get(path), await contentsNow(path), options))
    return [...compared.filter((file) => file !== undefined), ...paths.slice(MOST_FILES).map((path) => tooManyAt(shown(path, project)))]
  }

  return {
    async list(keeper: ChangeKeeper | undefined, session: string, events: readonly StoredEvent[], project: Project | null, whole: boolean): Promise<Commands["list_changes"]["output"]> {
      const turns = turnsIn(events)
      const listed: TurnChanges[] = []
      const kept = keptFor(session)
      const present = keeper === undefined ? [] : await keeper.turns(session)
      for (const { turn, paths } of present) {
        const known = turns[turn - 1]
        if (known === undefined) continue
        const { start, end } = known
        const key = paths.join("\0")
        let files = kept.get(turn)?.paths === key ? kept.get(turn)!.files : undefined
        if (files === undefined) {
          files = (await turnFiles(keeper!, session, turn, project, { patch: false })).map(summed)
          if (end !== undefined) kept.set(turn, { paths: key, files })
        }
        if (files.length > 0) listed.push({ turn, start: start.seq, ...(end !== undefined && { end: end.seq }), time: (end ?? start).time, files })
      }
      // Turns no longer kept, as when they are undone, are forgotten.
      for (const turn of kept.keys()) if (!present.some((each) => each.turn === turn)) kept.delete(turn)
      const output = { kept: keeper !== undefined, latest: turns.length, turns: listed }
      if (!whole) return output
      return { ...output, session: keeper === undefined ? [] : (await sessionFiles(keeper, session, project, { patch: false })).map(summed) }
    },
    async turn(keeper: ChangeKeeper | undefined, session: string, turn: number, project: Project | null): Promise<FileDiff[]> {
      if (keeper === undefined || !(await keeper.turns(session)).some((each) => each.turn === turn)) return []
      return capped(await turnFiles(keeper, session, turn, project, { patch: true }))
    },
    async session(keeper: ChangeKeeper | undefined, session: string, project: Project | null): Promise<FileDiff[]> {
      return keeper === undefined ? [] : capped(await sessionFiles(keeper, session, project, { patch: true }))
    },
  }
}

// The most files one comparison reads and compares. Those past it are
// listed by their paths, as too many, without being read.
const MOST_FILES = 300
// How many files are read at once.
const AT_ONCE = 8

// Each item worked out with `work`, a few at a time, in order.
async function fewAtOnce<Item, Done>(items: readonly Item[], work: (item: Item) => Promise<Done>): Promise<Done[]> {
  const done: Done[] = new Array(items.length)
  let next = 0
  const worker = async () => {
    while (next < items.length) {
      const at = next++
      done[at] = await work(items[at]!)
    }
  }
  await Promise.all(Array.from({ length: Math.min(AT_ONCE, items.length) }, worker))
  return done
}

// ---- With git -------------------------------------------------------------------

const NOT_GIT = "The project's folder is not in a git repository."

// Runs git in a folder and gives back what it printed, or throws with what
// it said. Git's own settings in the server's environment, such as GIT_DIR,
// which would point it elsewhere, are left out, as for the branch.
//
// A repository's own settings can name commands for git to run, and a
// repository may be one just cloned, so none of them is run: not a file
// system monitor (core.fsmonitor), a hook (core.hooksPath, and no index
// written that would run one), a pager, an external diff or a text
// conversion (diff.external, and a diff driver's command and textconv,
// which diffs are asked not to use), nor a filter, whose clean, smudge
// and process commands are each set to none, by the filter's name. The
// files themselves are read here, not through git.
async function git(folder: string, args: string[], input?: Uint8Array): Promise<Buffer> {
  return run(folder, [...SAFE, ...(await filtersOff(folder)), ...args], input)
}
const SAFE = ["--no-pager", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null", "-c", "diff.external="]
const DIFF_SAFE = ["--no-ext-diff", "--no-textconv"]

async function run(folder: string, args: string[], input?: Uint8Array): Promise<Buffer> {
  const env = { ...Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith("GIT_"))), GIT_OPTIONAL_LOCKS: "0" }
  const child = Bun.spawn(["git", ...args], { cwd: folder, env, stdin: input ?? "ignore", stdout: "pipe", stderr: "pipe", timeout: 30_000 })
  const [said, problem, code] = await Promise.all([new Response(child.stdout).arrayBuffer(), new Response(child.stderr).text(), child.exited])
  if (code !== 0) throw new Error(problem.trim() || `git ${args.find((arg) => !arg.startsWith("-") && !arg.includes("=")) ?? ""} stopped with code ${code}.`)
  return Buffer.from(said)
}

// The settings that turn off each filter git's settings name, wherever they
// are set. Reading the settings runs nothing.
async function filtersOff(folder: string): Promise<string[]> {
  const named = await run(folder, ["config", "--name-only", "--get-regexp", "^filter\\."]).catch(() => Buffer.from(""))
  const filters = new Set(named.toString("utf8").split("\n").flatMap((line) => /^filter\.(.+)\.[^.]+$/.exec(line.trim())?.[1] ?? []))
  return [...filters].flatMap((filter) => ["clean=", "smudge=", "process=", "required=false"].flatMap((setting) => ["-c", `filter.${filter}.${setting}`]))
}
const lines = (said: Buffer) => said.toString("utf8").split("\0").filter((each) => each !== "")

// The files that differ between a commit and the folder as it is now, each
// with what the commit held and what is there now: the changes git sees,
// staged or not, as git counts them (a file taken out of git is removed,
// though it is still in the folder), and the files it does not track and
// does not ignore, which count as added. A submodule moved to another
// commit is changed, with nothing inside it compared, and git is not run
// inside it. Paths are from the project's folder, and only what is inside
// it is compared.
async function sinceCommit(folder: string, commit: string): Promise<FileDiff[]> {
  const [changed, untracked] = await Promise.all([
    git(folder, ["diff", ...DIFF_SAFE, "--raw", "-z", "--no-renames", "--ignore-submodules=dirty", "--relative", commit, "--"]),
    git(folder, ["ls-files", "--others", "--exclude-standard", "-z"]),
  ])
  // Each change is ":<mode before> <mode after> <id> <id> <status>", then its path.
  type Listed = { status: "added" | "removed" | "changed"; submodule: boolean }
  const files = new Map<string, Listed>()
  const raw = lines(changed)
  for (let at = 0; at + 1 < raw.length; at += 2) {
    const [before, after, , , letter] = raw[at]!.slice(1).split(" ")
    files.set(raw[at + 1]!, { status: letter === "A" ? "added" : letter === "D" ? "removed" : "changed", submodule: before === SUBMODULE || after === SUBMODULE })
  }
  // A repository inside this one is listed with a slash at its end, and is not a file.
  for (const path of lines(untracked)) if (!path.endsWith("/") && !files.has(path)) files.set(path, { status: "added", submodule: false })
  const paths = [...files.keys()].sort()
  const read = paths.slice(0, MOST_FILES)
  const committed = await blobs(folder, commit, read.filter((path) => files.get(path)!.status !== "added" && !files.get(path)!.submodule))
  const compared = await fewAtOnce(read, async (path): Promise<FileDiff | undefined> => {
    const { status, submodule } = files.get(path)!
    if (submodule) return { path, status, added: 0, removed: 0, unknown: true, submodule: true }
    return compareApart(path, status === "added" ? null : committed.get(path), status === "removed" ? null : await contentsNow(`${folder}/${path}`), { patch: true })
  })
  const tooMany = paths.slice(MOST_FILES).map((path): FileDiff => ({ path, status: files.get(path)!.status, added: 0, removed: 0, tooMany: true }))
  return capped([...compared.filter((file) => file !== undefined), ...tooMany])
}
// The mode git gives a submodule.
const SUBMODULE = "160000"

// What a commit held at each path, from one `git cat-file`: the contents,
// null when it held nothing there, and undefined when what it held is not
// a file, as for a repository inside this one.
async function blobs(folder: string, commit: string, paths: string[]): Promise<Map<string, Uint8Array | null | undefined>> {
  const found = new Map<string, Uint8Array | null | undefined>()
  // `cat-file` reads one name a line, so a path with a line break in it
  // cannot be asked for: what it held is not known, rather than read wrong.
  for (const path of paths) if (path.includes("\n")) found.set(path, undefined)
  const asked = paths.filter((path) => !path.includes("\n"))
  if (asked.length === 0) return found
  const said = await git(folder, ["cat-file", "--batch"], new TextEncoder().encode(asked.map((path) => `${commit}:./${path}\n`).join("")))
  let at = 0
  for (const path of asked) {
    const end = said.indexOf(10, at)
    const header = said.subarray(at, end).toString("utf8")
    at = end + 1
    // An object found is "<id> <type> <size>", then its contents; anything
    // else, as "<name> missing", has no contents after it.
    const object = /^[0-9a-f]+ ([a-z]+) (\d+)$/.exec(header)
    if (!object) {
      found.set(path, header.endsWith(" missing") ? null : undefined)
      continue
    }
    const length = Number(object[2])
    found.set(path, object[1] === "blob" ? said.subarray(at, at + length) : undefined)
    at += length + 1
  }
  return found
}

// The commit the folder's work is on, or for a repository with no commits
// yet, the empty tree.
async function head(folder: string): Promise<string> {
  try {
    return (await git(folder, ["rev-parse", "--verify", "-q", "HEAD"])).toString().trim()
  } catch {
    return (await git(folder, ["mktree"], new Uint8Array())).toString().trim()
  }
}

// What a git comparison could not be made for, in words for the owner,
// given as the answer rather than as an error: it is not a fault.
type Unavailable = { unavailable: string }
const unavailable = (why: string): Commands["read_changes"]["output"] => ({ files: [], unavailable: why })

async function inRepository(project: Project | null): Promise<string | Unavailable> {
  if (project === null) return { unavailable: "This session has no project, so there is no git repository to compare." }
  if ((await inGit(project.folder)) === false) return { unavailable: NOT_GIT }
  return project.folder
}

export async function readUncommitted(project: Project | null): Promise<Commands["read_changes"]["output"]> {
  const folder = await inRepository(project)
  if (typeof folder !== "string") return unavailable(folder.unavailable)
  return { files: await sinceCommit(folder, await head(folder)) }
}

// The branch a branch's changes are compared with, as a commit git can
// name: the one given, as the remote has it when it has it, since the
// branch is merged there, or else as this repository has it. Without one,
// the base of the branch's pull request, when `pullRequestBase` finds one,
// or else the repository's main branch: the one the remote names as its
// own, or else main or master.
async function baseOf(folder: string, given: string | undefined, pullRequestBase: () => Promise<string | undefined>): Promise<{ base: string; ref: string } | Unavailable> {
  const exists = (ref: string) => git(folder, ["rev-parse", "--verify", "-q", `${ref}^{commit}`]).then(() => true, () => false)
  const named = async (base: string) => {
    // A name git would take as one of its options is no branch's.
    if (base === "" || base.startsWith("-")) return undefined
    for (const ref of [`refs/remotes/origin/${base}`, `refs/heads/${base}`]) if (await exists(ref)) return { base, ref }
    return undefined
  }
  if (given !== undefined) return (typeof given === "string" ? await named(given) : undefined) ?? { unavailable: `There is no branch "${String(given)}" to compare this one with.` }
  const remote = await git(folder, ["symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"]).then((said) => said.toString().trim().replace(/^origin\//, ""), () => undefined)
  for (const base of [await pullRequestBase(), remote, "main", "master"]) {
    const found = base === undefined ? undefined : await named(base)
    if (found) return found
  }
  return { unavailable: "This repository has no main branch to compare this one with: name the branch to compare it with." }
}

export async function readBranch(project: Project | null, given: string | undefined, pullRequestBase: () => Promise<string | undefined> = async () => undefined): Promise<Commands["read_changes"]["output"]> {
  const folder = await inRepository(project)
  if (typeof folder !== "string") return unavailable(folder.unavailable)
  const found = await baseOf(folder, given, pullRequestBase)
  if ("unavailable" in found) return unavailable(found.unavailable)
  const from = await git(folder, ["merge-base", found.ref, "HEAD"]).then((said) => said.toString().trim(), () => undefined)
  if (from === undefined) return unavailable(`This branch and ${found.base} have no commit in common to compare from.`)
  return { files: await sinceCommit(folder, from), base: found.base }
}
