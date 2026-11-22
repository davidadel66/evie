import type { ChangedFile, FileDiff, TurnChanges } from "../../src/core/types.ts"

// The diff's rules, apart from its screen: what the right panel can
// compare, the line that says how much a turn changed, which files are
// folded, a turn's files as its card lists them, and the changed files as
// a tree.

// Paths in the order git lists them, by their characters' codes, so that
// "README.md" comes before "recipes/".
export const byPath = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0)

// Whether a file has a diff to draw: its lines, or for a file renamed and
// not changed, its header. One binary, too large, filtered or not known
// has none, and says why instead.
export const drawable = (file: FileDiff) => file.patch !== undefined && (file.patch !== "" || file.from !== undefined)

// The first file, in the order of their paths, that has a diff to draw.
export const firstDrawn = (files: readonly FileDiff[]): string | undefined => files.filter(drawable).map((file) => file.path).sort(byPath)[0]

// Which files of a comparison are folded, and which have been seen. Each
// file is folded the first time it is seen, as T3 Code's are, but the file
// the diff is opened `at`, or else, the first time any is seen, the first
// that has a diff; and a file it is opened at later opens. What the owner
// folds or opens stays as they left it.
export type Folding = { seen: ReadonlySet<string>; folded: ReadonlySet<string> }
export function foldFiles(was: Folding | undefined, files: readonly FileDiff[], at: string | undefined): Folding {
  const seen = new Set(was?.seen)
  const folded = new Set(was?.folded)
  const open = at ?? (seen.size === 0 ? firstDrawn(files) : undefined)
  for (const file of files) {
    if (seen.has(file.path) || !drawable(file)) continue
    seen.add(file.path)
    if (file.path !== open) folded.add(file.path)
  }
  if (open !== undefined) folded.delete(open)
  return { seen, folded }
}

// The files a turn's card lists, as T3 Code's: at most 200, in the order
// of their paths, and how many more the full diff has.
const MOST_IN_CARD = 200
export function cardFiles<File extends ChangedFile>(files: readonly File[]): { listed: File[]; more: number } {
  const ordered = [...files].sort((a, b) => byPath(a.path, b.path))
  return { listed: ordered.slice(0, MOST_IN_CARD), more: Math.max(0, ordered.length - MOST_IN_CARD) }
}

// Whether a file's lines added and removed were counted: not for one that
// is binary, whose contents are not known, past the most files read or the
// time comparing may take, filtered, or too large to compare at all. One
// left out of a diff for its size after its lines were counted has them.
export const hasCounts = (file: ChangedFile) =>
  !file.binary && !file.unknown && !file.tooMany && !file.tooSlow && !file.filtered && !(file.tooLarge && file.added + file.removed === 0)

// What a turn's changed-files line says: how many files, and how many
// lines were added and removed across those whose lines were counted.
export function changedFilesLine(files: readonly ChangedFile[]): { files: string; added: number; removed: number } {
  const counted = files.filter(hasCounts)
  const sum = (count: (file: ChangedFile) => number) => counted.reduce((total, file) => total + count(file), 0)
  return { files: `${files.length} changed ${files.length === 1 ? "file" : "files"}`, added: sum((file) => file.added), removed: sum((file) => file.removed) }
}

// What the panel can compare, by a key of its own: the session's latest
// turn, one of its turns, the whole session, and in a git repository, the
// work not yet committed and the branch against its base.
export type Scope = "latest" | `turn:${number}` | "session" | "uncommitted" | "branch"
// `disabled` says why one cannot be picked.
export type ScopeChoice = { key: Scope; label: string; time?: string; disabled?: string }

// The choices, as the panel's menu lists them: the latest turn first, then
// each turn that changed files, newest first, with when it ended; the
// session; and in a project, the uncommitted work and the branch. `git` is
// whether the project's folder is in git, undefined when that is not known,
// when the server is asked and says, and null with no project. Known not
// to be, they are listed, but cannot be picked, and say why.
export function scopeChoices(given: { turns: readonly TurnChanges[]; git: boolean | undefined | null; now: number; timeZone?: string }): ScopeChoice[] {
  const turns = [...given.turns].reverse().map((turn): ScopeChoice => ({ key: `turn:${turn.turn}`, label: `Turn ${turn.turn}`, time: shortTime(turn.time, given.now, given.timeZone) }))
  const why = given.git === false ? { disabled: "Not a git repository" } : {}
  return [
    { key: "latest", label: "Latest turn" },
    ...turns,
    { key: "session", label: "This session" },
    ...(given.git === null ? [] : [{ key: "uncommitted", label: "Uncommitted", ...why } as const, { key: "branch", label: "This branch", ...why } as const]),
  ]
}

// A time as a turn's choice gives it: the hour and minute on the day it
// is now, and the day too before then.
function shortTime(time: number, now: number, timeZone?: string): string {
  const options = { ...(timeZone !== undefined && { timeZone }) }
  const day = (at: number) => new Intl.DateTimeFormat("en-GB", { ...options, day: "numeric", month: "short", year: "numeric" }).format(at)
  const clock = new Intl.DateTimeFormat("en-GB", { ...options, hour: "2-digit", minute: "2-digit", hour12: false }).format(time)
  return day(time) === day(now) ? clock : `${new Intl.DateTimeFormat("en-GB", { ...options, day: "numeric", month: "short" }).format(time)}, ${clock}`
}

// A row of a tree of changed files, as the changed-files card at the end
// of a turn and the file tree beside the diff draw them: a folder, known
// by its whole path, and whether it is open; or a file, by its whole path.
// Each has how deep it is, and the lines added and removed in it, which a
// folder sums across the files in it whose lines were counted; a file
// whose lines were not counted, such as one that is binary, has none.
type Counts = { added: number; removed: number }
export type TreeRow =
  | { kind: "folder"; path: string; name: string; depth: number; open: boolean; counts: Counts }
  | { kind: "file"; path: string; name: string; depth: number; counts: Counts | undefined }

// The files as a tree, as T3 Code's changed-files card lists them: in each
// folder, its folders first, then its files, each in order of name. A
// folder that holds one folder and nothing else is joined to it, as
// "src/core", so the tree is not deeper than it needs to be. Every folder
// is open, but those `folded`, by their whole paths, whose files are left
// out. A path that starts with a slash, outside the project, is a tree of
// its own from the top.
export function filesTree(files: readonly ChangedFile[], folded: ReadonlySet<string>): TreeRow[] {
  type Folder = { folders: Map<string, Folder>; files: Map<string, ChangedFile>; counts: Counts }
  const made = (): Folder => ({ folders: new Map(), files: new Map(), counts: { added: 0, removed: 0 } })
  const top = made()
  for (const each of files) {
    const parts = each.path.split("/")
    const name = parts.pop()!
    let folder = top
    const within = [top]
    for (const part of parts) {
      folder = folder.folders.get(part) ?? folder.folders.set(part, made()).get(part)!
      within.push(folder)
    }
    folder.files.set(name, each)
    if (!hasCounts(each)) continue
    for (const holding of within) {
      holding.counts.added += each.added
      holding.counts.removed += each.removed
    }
  }
  const byName = (a: string, b: string) => a.localeCompare(b)
  const rows: TreeRow[] = []
  const walk = (folder: Folder, depth: number, path: string) => {
    for (const name of [...folder.folders.keys()].sort(byName)) {
      let joined = name
      let inside = folder.folders.get(name)!
      while (inside.files.size === 0 && inside.folders.size === 1) {
        const [next, only] = [...inside.folders][0]!
        joined = `${joined}/${next}`
        inside = only
      }
      const whole = path === "" ? joined : `${path}/${joined}`
      const open = !folded.has(whole)
      rows.push({ kind: "folder", path: whole, name: joined, depth, open, counts: inside.counts })
      if (open) walk(inside, depth + 1, whole)
    }
    for (const name of [...folder.files.keys()].sort(byName)) {
      const each = folder.files.get(name)!
      rows.push({ kind: "file", path: each.path, name, depth, counts: hasCounts(each) ? { added: each.added, removed: each.removed } : undefined })
    }
  }
  walk(top, 0, "")
  return rows
}

// Every folder of the files' tree, by its whole path, and whether those
// shown are all open, which says whether a press of the tree's button
// folds every folder or opens them all.
export function treeFolders(files: readonly ChangedFile[], folded: ReadonlySet<string>): { all: string[]; open: boolean } {
  const all = filesTree(files, new Set()).flatMap((row) => (row.kind === "folder" ? [row.path] : []))
  const open = filesTree(files, folded).every((row) => row.kind !== "folder" || row.open)
  return { all, open }
}
