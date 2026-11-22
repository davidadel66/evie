import { isAbsolute, relative } from "node:path"
import { compareApart, contentsAt, LARGEST, TOO_LARGE } from "./diff.ts"
import { runGit } from "./git.ts"
import { inGit } from "./projects.ts"
import type { ChangedFile, ChangeKeeper, Commands, Contents, FileDiff, Project, StoredEvent, TurnChanges } from "./types.ts"

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

// Files as a client is shown them: by their paths from the project's
// folder, as it is when it is asked, since a session can be moved to
// another project. They are worked out, and kept, by their full paths.
const relocated = <File extends ChangedFile>(files: File[], project: Project | null): File[] => files.map((file) => ({ ...file, path: shown(file.path, project) }))

// The longest one answer spends comparing lines, in milliseconds: the
// files left after it are listed as too slow, so that one answer cannot
// hold the thread that compares lines for minutes.
const ANSWER_WITHIN = 20_000
const budget = () => Date.now() + ANSWER_WITHIN

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

  // One turn's files, compared, with their diffs or only how many lines,
  // by their full paths.
  async function turnFiles(keeper: ChangeKeeper, session: string, turn: number, options: { patch: boolean }): Promise<FileDiff[]> {
    const files = await keeper.files(session, turn)
    const until = budget()
    const compared = await fewAtOnce(files.slice(0, MOST_FILES), (file) => compareApart(file.path, file.before, file.after, { ...options, until }))
    return [...compared.filter((file) => file !== undefined), ...files.slice(MOST_FILES).map((file) => tooManyAt(file.path))]
  }

  // Every file the turns changed, each from what it held before the first
  // turn that changed it to what it holds now, by their full paths.
  async function sessionFiles(keeper: ChangeKeeper, session: string, options: { patch: boolean }): Promise<FileDiff[]> {
    const first = new Map<string, number>()
    for (const { turn, paths } of await keeper.turns(session)) for (const path of paths) if (!first.has(path)) first.set(path, turn)
    const paths = [...first.keys()]
    const read = paths.slice(0, MOST_FILES)
    const turns = new Map<number, Map<string, Contents>>()
    for (const turn of new Set(read.map((path) => first.get(path)!))) {
      turns.set(turn, new Map((await keeper.files(session, turn)).map((file) => [file.path, file.before])))
    }
    const until = budget()
    const compared = await fewAtOnce(read, async (path) => compareApart(path, turns.get(first.get(path)!)!.get(path), await contentsAt(path), { ...options, until }))
    return [...compared.filter((file) => file !== undefined), ...paths.slice(MOST_FILES).map((path) => tooManyAt(path))]
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
          files = (await turnFiles(keeper!, session, turn, { patch: false })).map(summed)
          // One that a comparison ran out of time on is worked out again when next asked.
          if (end !== undefined && !files.some((file) => file.tooSlow)) kept.set(turn, { paths: key, files })
        }
        if (files.length > 0) listed.push({ turn, start: start.seq, ...(end !== undefined && { end: end.seq }), time: (end ?? start).time, files: relocated(files, project) })
      }
      // Turns no longer kept, as when they are undone, are forgotten.
      for (const turn of kept.keys()) if (!present.some((each) => each.turn === turn)) kept.delete(turn)
      const output = { kept: keeper !== undefined, latest: turns.length, turns: listed }
      if (!whole) return output
      return { ...output, session: keeper === undefined ? [] : relocated((await sessionFiles(keeper, session, { patch: false })).map(summed), project) }
    },
    async turn(keeper: ChangeKeeper | undefined, session: string, turn: number, project: Project | null): Promise<FileDiff[]> {
      if (keeper === undefined || !(await keeper.turns(session)).some((each) => each.turn === turn)) return []
      return relocated(capped(await turnFiles(keeper, session, turn, { patch: true })), project)
    },
    async session(keeper: ChangeKeeper | undefined, session: string, project: Project | null): Promise<FileDiff[]> {
      return keeper === undefined ? [] : relocated(capped(await sessionFiles(keeper, session, { patch: true })), project)
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

// Runs git in a folder, with none of the commands a repository's settings
// can name, nor any reaching another computer (see git.ts), and gives back
// what it printed, or throws with what it said. `worktree` is for a
// command that reads the folder's files.
async function git(folder: string, args: string[], options: { input?: Uint8Array; worktree?: boolean } = {}): Promise<Buffer> {
  const done = await runGit(folder, args, options)
  if (done.code !== 0) throw new Error(done.stderr.trim() || `git ${args[0]} stopped with code ${done.code}.`)
  return done.stdout
}
export const DIFF_SAFE = ["--no-ext-diff", "--no-textconv"]

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
    git(folder, ["diff", ...DIFF_SAFE, "--raw", "-z", "--no-renames", "--ignore-submodules=dirty", "--relative", commit, "--"], { worktree: true }),
    git(folder, ["ls-files", "--others", "--exclude-standard", "-z"], { worktree: true }),
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
  const filtered = await withFilters(folder, read.filter((path) => !files.get(path)!.submodule))
  const committed = await blobs(folder, commit, read.filter((path) => files.get(path)!.status !== "added" && !files.get(path)!.submodule && !filtered.has(path)))
  const until = budget()
  const compared = await fewAtOnce(read, async (path): Promise<FileDiff | undefined> => {
    const { status, submodule } = files.get(path)!
    if (submodule) return { path, status, added: 0, removed: 0, unknown: true, submodule: true }
    if (filtered.has(path)) return { path, status, added: 0, removed: 0, filtered: true }
    return compareApart(path, status === "added" ? null : committed.get(path), status === "removed" ? null : await contentsAt(`${folder}/${path}`), { patch: true, until })
  })
  const tooMany = paths.slice(MOST_FILES).map((path): FileDiff => ({ path, status: files.get(path)!.status, added: 0, removed: 0, tooMany: true }))
  return capped([...compared.filter((file) => file !== undefined), ...tooMany])
}
// The mode git gives a submodule.
const SUBMODULE = "160000"

// What a commit held at each path: the contents; too large, for a file
// over the limit, which is not read; and undefined when it is not known,
// as when what it held is not a file, as for a repository inside this
// one, or is not in this repository, as a partial clone can leave it,
// since nothing is fetched. Every path asked for is one git says the
// commit holds. How large each is comes first (`cat-file --batch-check`),
// and only those within the limit are read (`cat-file --batch`, by their
// ids), each into a copy of its own.
async function blobs(folder: string, commit: string, paths: string[]): Promise<Map<string, Contents>> {
  const found = new Map<string, Contents>()
  // `cat-file` reads one name a line, so a path with a line break in it
  // cannot be asked for: what it held is not known, rather than read wrong.
  for (const path of paths) found.set(path, undefined)
  const asked = paths.filter((path) => !path.includes("\n"))
  if (asked.length === 0) return found
  const checked = (await git(folder, ["cat-file", "--batch-check"], { input: new TextEncoder().encode(asked.map((path) => `${commit}:./${path}\n`).join("")) })).toString("utf8").split("\n")
  const toRead: { path: string; id: string }[] = []
  for (const [at, path] of asked.entries()) {
    // An object found is "<id> <type> <size>"; anything else, as "<name> missing", is not known.
    const object = /^([0-9a-f]+) ([a-z]+) (\d+)$/.exec(checked[at] ?? "")
    if (!object || object[2] !== "blob") continue
    if (Number(object[3]) > LARGEST) found.set(path, TOO_LARGE)
    else toRead.push({ path, id: object[1]! })
  }
  if (toRead.length === 0) return found
  const said = await git(folder, ["cat-file", "--batch"], { input: new TextEncoder().encode(toRead.map(({ id }) => `${id}\n`).join("")) })
  let at = 0
  for (const { path } of toRead) {
    const end = said.indexOf(10, at)
    const object = /^[0-9a-f]+ blob (\d+)$/.exec(said.subarray(at, end).toString("utf8"))
    at = end + 1
    if (!object) continue
    const length = Number(object[1])
    found.set(path, new Uint8Array(said.subarray(at, at + length)))
    at += length + 1
  }
  return found
}

// The paths of those files that git turns with a filter, as git-lfs does,
// as their attributes say. Reading attributes runs nothing.
async function withFilters(folder: string, paths: string[]): Promise<Set<string>> {
  if (paths.length === 0) return new Set()
  const said = lines(await git(folder, ["check-attr", "-z", "--stdin", "filter"], { input: new TextEncoder().encode(paths.map((path) => `${path}\0`).join("")) }))
  const filtered = new Set<string>()
  // Each is "<path>", "filter", "<value>".
  for (let at = 0; at + 2 < said.length; at += 3) if (said[at + 2] !== "unspecified" && said[at + 2] !== "unset") filtered.add(said[at]!)
  return filtered
}

// The commit the folder's work is on, or for a repository with no commits
// yet, the empty tree, which git 2.46 no longer knows without its being
// written, so it is written once. The diff is read when it is opened; the
// session details' Changes row, read as it changes, writes nothing.
async function head(folder: string): Promise<string> {
  try {
    return (await git(folder, ["rev-parse", "--verify", "-q", "HEAD"])).toString().trim()
  } catch {
    return (await git(folder, ["mktree"], { input: new Uint8Array() })).toString().trim()
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
  const start = await branchStart(folder, given, pullRequestBase)
  if ("unavailable" in start) return unavailable(start.unavailable)
  return { files: await sinceCommit(folder, start.from), base: start.base }
}

// Where a branch's changes are counted from: where it left the branch it
// is compared with, picked as baseOf picks it, or why it cannot be. The
// diff and the session details' Changes row both start here, so that they
// agree.
export async function branchStart(folder: string, given: string | undefined, pullRequestBase: () => Promise<string | undefined> = async () => undefined): Promise<{ base: string; from: string } | Unavailable> {
  const found = await baseOf(folder, given, pullRequestBase)
  if ("unavailable" in found) return found
  const from = await git(folder, ["merge-base", found.ref, "HEAD"]).then((said) => said.toString().trim(), () => undefined)
  if (from === undefined) return { unavailable: `This branch and ${found.base} have no commit in common to compare from.` }
  return { base: found.base, from }
}
