import { readBranch, readUncommitted } from "./changes.ts"
import { runGit } from "./git.ts"
import { inGit } from "./projects.ts"
import type { GitStatus, Project } from "./types.ts"

// How a project's git branch stands, for the web app's session details, as
// T3 Code's git status reads it (`statusDetails` in its GitVcsDriverCore.ts,
// MIT licence, notice in LICENSES/t3code-MIT.txt): `git status` for the
// branch, its upstream, how far it is from it and whether anything is not
// committed; the remotes; the main branch and how far ahead of it the
// branch is; and the lines changed, counted as `read_changes` counts them,
// so that the card's row and the diff agree. Git runs none of the commands
// a repository's settings can name, and reaches no other computer (git.ts).

// The branches offered to switch to, at most.
const MOST_BRANCHES = 20

// What git printed, or undefined when it stopped with an error.
async function said(folder: string, args: string[], options: { worktree?: boolean } = {}): Promise<string | undefined> {
  const run = await runGit(folder, args, options)
  return run.code === 0 ? run.stdout.toString("utf8") : undefined
}

// `base` is the branch to count a branch's changes against, as the diff
// panel is given its pull request's base; without it, or when there is no
// such branch, the main branch.
export async function readGitStatus(project: Project, base: string | undefined): Promise<GitStatus | null> {
  if ((await inGit(project.folder)) === false) return null
  const folder = project.folder
  const [status, remotes, refs] = await Promise.all([
    said(folder, ["status", "--porcelain=v2", "--branch", "-z", "--untracked-files=normal", "--ignore-submodules=dirty"], { worktree: true }),
    said(folder, ["remote"]),
    said(folder, ["for-each-ref", "--sort=-committerdate", `--count=${MOST_BRANCHES}`, "--format=%(refname:short)", "refs/heads"]),
  ])
  if (status === undefined) return null
  const read = statusOf(status)
  const main = await mainOf(folder, read.branch)
  const onMain = main !== undefined && main.on
  const changes = await changesOf(project, onMain, base)
  return {
    ...(read.branch !== undefined && { branch: read.branch }),
    uncommitted: read.uncommitted,
    remote: (remotes ?? "").trim() !== "",
    ...(read.upstream !== undefined && { upstream: read.upstream }),
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

// The lines the Changes row sums: on a branch, against the branch it is
// compared with, as the diff compares it; on the main branch, or when the
// branch cannot be compared, the work not yet committed.
async function changesOf(project: Project, onMain: boolean, base: string | undefined): Promise<GitStatus["changes"]> {
  const against = (given: string | undefined) => readBranch(project, given).catch(() => undefined)
  let branch = onMain ? undefined : await against(base)
  if (base !== undefined && branch?.unavailable !== undefined) branch = await against(undefined)
  const compared = branch !== undefined && branch.unavailable === undefined ? branch : await readUncommitted(project).catch(() => ({ files: [] }))
  return {
    against: compared === branch ? (branch.base ?? null) : null,
    added: compared.files.reduce((sum, file) => sum + file.added, 0),
    removed: compared.files.reduce((sum, file) => sum + file.removed, 0),
  }
}
