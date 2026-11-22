import { lstat, readFile, readlink } from "node:fs/promises"
import { isAbsolute, relative } from "node:path"
import { compare } from "./diff.ts"
import { inGit } from "./projects.ts"
import type { ChangedFile, ChangeKeeper, Commands, FileDiff, KeptTurn, Project, StoredEvent } from "./types.ts"

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

// Every file the turns changed, each from what it held before the first
// turn that changed it to what it holds now.
async function sessionFiles(kept: readonly KeptTurn[], project: Project | null, options: { patch: boolean }): Promise<FileDiff[]> {
  const first = new Map<string, Uint8Array | null | undefined>()
  for (const { files } of kept) for (const file of files) if (!first.has(file.path)) first.set(file.path, file.before)
  const files = await Promise.all([...first].map(async ([path, before]) => compare(shown(path, project), before, await contentsNow(path), options)))
  return files.filter((file) => file !== undefined)
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

export async function listChanges(keeper: ChangeKeeper | undefined, session: string, events: readonly StoredEvent[], project: Project | null): Promise<Commands["list_changes"]["output"]> {
  const turns = turnsIn(events)
  const kept = keeper === undefined ? [] : await keeper.turns(session)
  const listed = kept.flatMap(({ turn, files }) => {
    const known = turns[turn - 1]
    if (known === undefined) return []
    const changed = files.flatMap((file) => compare(shown(file.path, project), file.before, file.after, { patch: false }) ?? [])
    if (changed.length === 0) return []
    const { start, end } = known
    return [{ turn, start: start.seq, ...(end !== undefined && { end: end.seq }), time: (end ?? start).time, files: changed }]
  })
  return { kept: keeper !== undefined, latest: turns.length, turns: listed, session: (await sessionFiles(kept, project, { patch: false })).map(summed) }
}

export async function readTurn(keeper: ChangeKeeper | undefined, session: string, turn: number, project: Project | null): Promise<FileDiff[]> {
  const kept = keeper === undefined ? [] : await keeper.turns(session)
  const files = kept.find((each) => each.turn === turn)?.files ?? []
  return capped(files.flatMap((file) => compare(shown(file.path, project), file.before, file.after, { patch: true }) ?? []))
}

export async function readSession(keeper: ChangeKeeper | undefined, session: string, project: Project | null): Promise<FileDiff[]> {
  return capped(await sessionFiles(keeper === undefined ? [] : await keeper.turns(session), project, { patch: true }))
}

// ---- With git -------------------------------------------------------------------

const NOT_GIT = "The project's folder is not in a git repository."

// Runs git in a folder and gives back what it printed, or throws with what
// it said. Git's own settings in the server's environment, such as GIT_DIR,
// which would point it elsewhere, are left out, as for the branch.
async function git(folder: string, args: string[], input?: Uint8Array): Promise<Buffer> {
  const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith("GIT_")))
  const run = Bun.spawn(["git", ...args], { cwd: folder, env, stdin: input ?? "ignore", stdout: "pipe", stderr: "pipe", timeout: 30_000 })
  const [said, problem, code] = await Promise.all([new Response(run.stdout).arrayBuffer(), new Response(run.stderr).text(), run.exited])
  if (code !== 0) throw new Error(problem.trim() || `git ${args[0]} stopped with code ${code}.`)
  return Buffer.from(said)
}
const lines = (said: Buffer) => said.toString("utf8").split("\0").filter((each) => each !== "")

// The files that differ between a commit and the folder as it is now, each
// with what the commit held and what is there now: the changes git sees,
// staged or not, and the files it does not track and does not ignore,
// which count as added. Paths are from the project's folder, and only
// what is inside it is compared.
async function sinceCommit(folder: string, commit: string): Promise<FileDiff[]> {
  const [changed, untracked] = await Promise.all([
    git(folder, ["diff", "--name-status", "-z", "--no-renames", "--relative", commit, "--"]),
    git(folder, ["ls-files", "--others", "--exclude-standard", "-z"]),
  ])
  const statuses = lines(changed)
  const files = new Map<string, boolean>()
  for (let at = 0; at + 1 < statuses.length; at += 2) files.set(statuses[at + 1]!, statuses[at] !== "A")
  // A repository inside this one is listed with a slash at its end, and is not a file.
  for (const path of lines(untracked)) if (!path.endsWith("/")) files.set(path, false)
  const paths = [...files.keys()].sort()
  const committed = await blobs(folder, commit, paths.filter((path) => files.get(path)))
  const compared = await Promise.all(paths.map(async (path) => compare(path, files.get(path) ? committed.get(path) : null, await contentsNow(`${folder}/${path}`), { patch: true })))
  return capped(compared.filter((file) => file !== undefined))
}

// What a commit held at each path, from one `git cat-file`: the contents,
// null when it held nothing there, and undefined when what it held is not
// a file, as for a repository inside this one.
async function blobs(folder: string, commit: string, paths: string[]): Promise<Map<string, Uint8Array | null | undefined>> {
  const found = new Map<string, Uint8Array | null | undefined>()
  if (paths.length === 0) return found
  const said = await git(folder, ["cat-file", "--batch"], new TextEncoder().encode(paths.map((path) => `${commit}:./${path}\n`).join("")))
  let at = 0
  for (const path of paths) {
    const end = said.indexOf(10, at)
    const header = said.subarray(at, end).toString("utf8")
    at = end + 1
    if (header.endsWith(" missing")) {
      found.set(path, null)
      continue
    }
    const [, type, size] = header.split(" ")
    const length = Number(size)
    found.set(path, type === "blob" ? said.subarray(at, at + length) : undefined)
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
    return (await git(folder, ["hash-object", "-t", "tree", "/dev/null"])).toString().trim()
  }
}

async function inRepository(project: Project | null): Promise<string> {
  if (project === null) throw new Error("This session has no project, so there is no git repository to compare.")
  if ((await inGit(project.folder)) === false) throw new Error(NOT_GIT)
  return project.folder
}

export async function readUncommitted(project: Project | null): Promise<FileDiff[]> {
  const folder = await inRepository(project)
  return sinceCommit(folder, await head(folder))
}

// The branch a branch's changes are compared with, as a commit git can
// name: the one given, as the remote has it when it has it, since the
// branch is merged there, or else as this repository has it. Without one,
// the repository's main branch: the one the remote names as its own, or
// else main or master.
async function baseOf(folder: string, given: string | undefined): Promise<{ base: string; ref: string }> {
  const exists = (ref: string) => git(folder, ["rev-parse", "--verify", "-q", `${ref}^{commit}`]).then(() => true, () => false)
  const named = async (base: string) => {
    for (const ref of [`refs/remotes/origin/${base}`, `refs/heads/${base}`]) if (await exists(ref)) return { base, ref }
    return undefined
  }
  if (given !== undefined) {
    // A name git would take as one of its options is no branch's.
    const found = typeof given === "string" && given !== "" && !given.startsWith("-") ? await named(given) : undefined
    if (!found) throw new Error(`There is no branch "${String(given)}" to compare this one with.`)
    return found
  }
  const remote = await git(folder, ["symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"]).then((said) => said.toString().trim().replace(/^origin\//, ""), () => undefined)
  for (const base of [remote, "main", "master"]) {
    const found = base === undefined ? undefined : await named(base)
    if (found) return found
  }
  throw new Error("This repository has no main branch to compare this one with: name the branch to compare it with.")
}

export async function readBranch(project: Project | null, given: string | undefined): Promise<{ files: FileDiff[]; base: string }> {
  const folder = await inRepository(project)
  const { base, ref } = await baseOf(folder, given)
  const from = await git(folder, ["merge-base", ref, "HEAD"]).then((said) => said.toString().trim(), () => undefined)
  if (from === undefined) throw new Error(`This branch and ${base} have no commit in common to compare from.`)
  return { files: await sinceCommit(folder, from), base }
}
