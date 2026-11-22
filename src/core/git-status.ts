import { join } from "node:path"
import { branchStart, DIFF_SAFE } from "./changes.ts"
import { runGit } from "./git.ts"
import { inGit } from "./projects.ts"
import type { GitStatus, Project } from "./types.ts"

// How a project's git branch stands, for the web app's session details, as
// T3 Code's git status reads it (`statusDetails` in its GitVcsDriverCore.ts,
// MIT licence, notice in LICENSES/t3code-MIT.txt): `git status` for the
// branch, its upstream, how far it is from it and whether anything in the
// project's folder is not committed; the remotes; the main branch and how
// far ahead of it the branch is; and the lines changed, counted with `git
// diff --numstat` from where the diff starts, so that the card's row and
// the diff agree. Git runs none of the commands a repository's settings
// can name, and reaches no other computer (git.ts), and nothing is written
// into the repository.

// The branches offered to switch to, at most.
const MOST_BRANCHES = 20

// What git printed, or undefined when it stopped with an error.
async function said(folder: string, args: string[], options: { worktree?: boolean } = {}): Promise<string | undefined> {
  const run = await runGit(folder, args, options)
  return run.code === 0 ? run.stdout.toString("utf8") : undefined
}

// The same, for what must be read: one that fails is an error, with what git said.
async function must(folder: string, args: string[], options: { worktree?: boolean } = {}): Promise<string> {
  const run = await runGit(folder, args, options)
  if (run.code !== 0) throw new Error(`How the branch stands could not be read: ${run.stderr.trim() || `git ${args[0]} stopped with code ${run.code}.`}`)
  return run.stdout.toString("utf8")
}

// Null only for a folder known not to be in git; one that cannot be read
// is an error, so that what was read before is kept. `base` is the branch
// to count a branch's changes against, as the diff panel is given its pull
// request's base; without it, the main branch.
export async function readGitStatus(project: Project, base: string | undefined): Promise<GitStatus | null> {
  if ((await inGit(project.folder)) === false) return null
  const folder = project.folder
  const [status, remotes, refs] = await Promise.all([
    must(folder, ["status", "--porcelain=v2", "--branch", "-z", "--untracked-files=normal", "--ignore-submodules=dirty", "--", "."], { worktree: true }),
    said(folder, ["remote"]),
    said(folder, ["for-each-ref", "--sort=-committerdate", `--count=${MOST_BRANCHES}`, "--format=%(refname:short)", "refs/heads"]),
  ])
  const read = statusOf(status)
  const remote = (remotes ?? "").trim() !== ""
  const [main, unpushed] = await Promise.all([
    mainOf(folder, read.branch),
    // With no upstream, what pushing would send: the commits no remote branch has.
    read.upstream === undefined && remote ? said(folder, ["rev-list", "--count", "HEAD", "--not", "--remotes", "--"]).then((count) => Number(count?.trim() ?? 0)) : undefined,
  ])
  // On the main branch, or with none to compare with and none given, the work not committed.
  const changes = await changesOf(folder, base !== undefined || (main !== undefined && !main.on), base)
  return {
    ...(read.branch !== undefined && { branch: read.branch }),
    uncommitted: read.uncommitted,
    remote,
    ...(read.upstream !== undefined && { upstream: read.upstream }),
    ...(unpushed !== undefined && { unpushed }),
    ...(main !== undefined && { main }),
    changes,
    branches: (refs ?? "").split("\n").filter((name) => name !== ""),
  }
}

// What `git status --porcelain=v2 --branch` says: the branch, or
// "(detached)"; its upstream, which counts only with how far the branch is
// from it, since git leaves that out once the upstream has gone; and a line
// for each change, staged or not, and each file it does not track.
function statusOf(printed: string): { branch?: string; upstream?: { ahead: number; behind: number }; uncommitted: boolean } {
  let branch: string | undefined
  let upstream: { ahead: number; behind: number } | undefined
  let uncommitted = false
  for (const line of printed.split("\0")) {
    if (line.startsWith("# branch.head ")) {
      const name = line.slice("# branch.head ".length).trim()
      branch = name === "(detached)" || name === "(unknown)" ? undefined : name
    } else if (line.startsWith("# branch.ab ")) {
      const [ahead, behind] = line.slice("# branch.ab ".length).trim().split(" ")
      upstream = { ahead: Math.abs(Number(ahead)), behind: Math.abs(Number(behind)) }
    } else if (line !== "" && !line.startsWith("#")) {
      uncommitted = true
    }
  }
  return { ...(branch !== undefined && { branch }), ...(upstream !== undefined && { upstream }), uncommitted }
}

// The repository's main branch: the one its remote names as its own, or
// else main or master, as the remote has it when it has it; whether the
// folder is on it; and how many commits the folder's has that it lacks.
async function mainOf(folder: string, branch: string | undefined): Promise<GitStatus["main"]> {
  const named = (await said(folder, ["symbolic-ref", "-q", "--short", "refs/remotes/origin/HEAD"]))?.trim().replace(/^origin\//, "")
  for (const name of [named, "main", "master"]) {
    if (name === undefined || name === "") continue
    for (const ref of [`refs/remotes/origin/${name}`, `refs/heads/${name}`]) {
      const ahead = await said(folder, ["rev-list", "--count", `${ref}..HEAD`, "--"])
      if (ahead !== undefined) return { name, on: branch === name, ahead: Number(ahead.trim()) }
    }
  }
  return undefined
}

// The lines the Changes row sums: on a branch, from where it left the
// branch it is compared with, as the diff starts (branchStart), or why it
// cannot be; on the main branch, or with none, the work not yet committed.
// Every file is counted: those git sees changed, as `--numstat` counts
// them, and those it neither tracks nor ignores, as added. In a repository
// with no commits yet, every file in the folder is added; git no longer
// knows the empty tree to compare with unless it is written, and nothing
// is written here.
async function changesOf(folder: string, branch: boolean, base: string | undefined): Promise<GitStatus["changes"]> {
  const commit = await said(folder, ["rev-parse", "--verify", "-q", "HEAD"])
  if (commit === undefined) {
    const files = await must(folder, ["ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", "."], { worktree: true })
    return { against: null, added: await linesInAll(folder, files), removed: 0 }
  }
  const start = branch ? await branchStart(folder, base) : undefined
  if (start !== undefined && "unavailable" in start) return { against: base ?? null, added: 0, removed: 0, unavailable: start.unavailable }
  const [numstat, untracked] = await Promise.all([
    must(folder, ["diff", ...DIFF_SAFE, "--numstat", "-z", "--no-renames", "--ignore-submodules=dirty", "--relative", start?.from ?? commit.trim(), "--"], { worktree: true }),
    must(folder, ["ls-files", "--others", "--exclude-standard", "-z", "--", "."], { worktree: true }),
  ])
  let added = 0
  let removed = 0
  // Each is "<added>\t<removed>\t<path>", with "-" for a file that is not text.
  for (const entry of numstat.split("\0")) {
    const [plus, minus] = entry.split("\t")
    if (plus === undefined || minus === undefined || plus === "-") continue
    added += Number(plus) || 0
    removed += Number(minus) || 0
  }
  return { against: start?.base ?? null, added: added + (await linesInAll(folder, untracked)), removed }
}

// The lines of every file in a list git gave, each ended by a zero byte.
async function linesInAll(folder: string, listed: string): Promise<number> {
  let lines = 0
  // A repository inside this one is listed with a slash at its end, and is not a file.
  for (const path of listed.split("\0")) if (path !== "" && !path.endsWith("/")) lines += await linesIn(join(folder, path))
  return lines
}

// How many lines a file holds, as git counts an added one: its line breaks,
// and a last line without one; none for a file that is not text, which
// holds a zero byte in its first 8,000, as git judges it.
async function linesIn(path: string): Promise<number> {
  let lines = 0
  let last = 10
  let first = true
  try {
    for await (const piece of Bun.file(path).stream()) {
      if (first && piece.subarray(0, 8000).includes(0)) return 0
      first = false
      for (let at = piece.indexOf(10); at !== -1; at = piece.indexOf(10, at + 1)) lines += 1
      if (piece.length > 0) last = piece[piece.length - 1]!
    }
  } catch {
    // Gone, or not readable: not counted.
    return 0
  }
  return lines + (last === 10 ? 0 : 1)
}
