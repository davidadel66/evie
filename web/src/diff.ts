import type { ChangedFile, TurnChanges } from "../../src/core/types.ts"
import type { Keys } from "./project-card.ts"

// The diff panel's rules, apart from its screen: what it can compare, the
// line that says how much a turn changed, and a file's diff, as the server
// gives it, split into hunks and lines, and drawn stacked or side by side.

// One line of a diff: unchanged, added or removed, with its number in the
// file before and after, as it has one there. `noNewline` marks a last
// line with no line end.
export type Line = { kind: "same" | "added" | "removed"; text: string; old?: number; new?: number; noNewline?: true }
// A run of changed lines with the unchanged lines around them, under its
// `@@` line.
export type Hunk = { header: string; lines: Line[] }

// A unified diff's hunks, from its `@@` lines on.
export function hunksOf(patch: string): Hunk[] {
  const hunks: Hunk[] = []
  let old = 0
  let now = 0
  for (const written of patch.split("\n")) {
    const header = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(written)
    if (header) {
      hunks.push({ header: written, lines: [] })
      // A range of no lines starts at the line before it.
      old = Math.max(1, Number(header[1]))
      now = Math.max(1, Number(header[2]))
      continue
    }
    const hunk = hunks.at(-1)
    if (hunk === undefined || written === "") continue
    const text = written.slice(1)
    if (written.startsWith("\\")) {
      const last = hunk.lines.at(-1)
      if (last) last.noNewline = true
    } else if (written.startsWith("+")) hunk.lines.push({ kind: "added", text, new: now++ })
    else if (written.startsWith("-")) hunk.lines.push({ kind: "removed", text, old: old++ })
    else hunk.lines.push({ kind: "same", text, old: old++, new: now++ })
  }
  return hunks
}

// The lines of a file that are drawn: the first `most` of them, so that a
// file of tens of thousands of lines cannot stall the page; whether the
// rest were left out; and the widest line number among those drawn, for
// the room the numbers take.
export function drawnLines(hunks: readonly Hunk[], most: number): { hunks: Hunk[]; cut: boolean; widest: number } {
  const drawn: Hunk[] = []
  let left = most
  let widest = 0
  for (const hunk of hunks) {
    if (left === 0) return { hunks: drawn, cut: true, widest }
    const lines = hunk.lines.length <= left ? hunk.lines : hunk.lines.slice(0, left)
    left -= lines.length
    for (const line of lines) widest = Math.max(widest, line.old ?? 0, line.new ?? 0)
    drawn.push(lines === hunk.lines ? hunk : { ...hunk, lines })
    if (lines !== hunk.lines) return { hunks: drawn, cut: true, widest }
  }
  return { hunks: drawn, cut: false, widest }
}

// A hunk's lines side by side: before on the left, after on the right. An
// unchanged line is on both sides. Lines removed face the lines added in
// their place, one to one, and what is left over faces nothing.
export function sideBySide(lines: readonly Line[]): { left?: Line; right?: Line }[] {
  const rows: { left?: Line; right?: Line }[] = []
  for (let at = 0; at < lines.length; ) {
    const line = lines[at]!
    if (line.kind === "same") {
      rows.push({ left: line, right: line })
      at += 1
      continue
    }
    const removed: Line[] = []
    const added: Line[] = []
    while (lines[at]?.kind === "removed") removed.push(lines[at++]!)
    while (lines[at]?.kind === "added") added.push(lines[at++]!)
    for (let row = 0; row < Math.max(removed.length, added.length); row += 1) {
      rows.push({ ...(removed[row] && { left: removed[row] }), ...(added[row] && { right: added[row] }) })
    }
  }
  return rows
}

// Whether a file's lines added and removed were counted: not for one that
// is binary, whose contents are not known, past the most files read, or
// too large to compare at all. One left out of a diff for its size after
// its lines were counted has them.
export const hasCounts = (file: ChangedFile) => !file.binary && !file.unknown && !file.tooMany && !(file.tooLarge && file.added + file.removed === 0)

// What a turn's changed-files line says: how many files, and how many
// lines were added and removed across those whose lines were counted.
export function changedFilesLine(files: readonly ChangedFile[]): { files: string; added: number; removed: number } {
  const counted = files.filter(hasCounts)
  const sum = (count: (file: ChangedFile) => number) => counted.reduce((total, file) => total + count(file), 0)
  return { files: `${files.length} changed ${files.length === 1 ? "file" : "files"}`, added: sum((file) => file.added), removed: sum((file) => file.removed) }
}

// What the project card says of the session's changes: the line, or that
// there are none yet, or that they are not kept here; nothing until they
// have been listed.
export function sessionLine(listed: { kept: boolean; session?: readonly ChangedFile[] } | undefined): string | ReturnType<typeof changedFilesLine> {
  if (listed === undefined || listed.session === undefined) return ""
  if (!listed.kept) return "evie keeps no copies of the files it changes here, so its changes are not shown."
  return listed.session.length === 0 ? "No changes in this session yet" : changedFilesLine(listed.session)
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

// A row of the file tree beside the diff: a folder, or a file, by its
// whole path, each with how deep it is.
export type TreeRow = { kind: "folder"; name: string; depth: number } | { kind: "file"; name: string; depth: number; path: string }

// The files' paths as a tree: in each folder, its folders first, then its
// files, each in order of name. A folder that holds one folder and nothing
// else is joined to it, as "src/core", so the tree is not deeper than it
// needs to be. A path that starts with a slash, outside the project, is a
// tree of its own from the top.
export function fileTree(paths: readonly string[]): TreeRow[] {
  type Folder = { folders: Map<string, Folder>; files: Map<string, string> }
  const top: Folder = { folders: new Map(), files: new Map() }
  for (const path of paths) {
    const parts = path.split("/")
    const name = parts.pop()!
    let folder = top
    for (const part of parts) folder = folder.folders.get(part) ?? folder.folders.set(part, { folders: new Map(), files: new Map() }).get(part)!
    folder.files.set(name, path)
  }
  const byName = (a: string, b: string) => a.localeCompare(b)
  const rows: TreeRow[] = []
  const walk = (folder: Folder, depth: number) => {
    for (const name of [...folder.folders.keys()].sort(byName)) {
      let joined = name
      let inside = folder.folders.get(name)!
      while (inside.files.size === 0 && inside.folders.size === 1) {
        const [next, only] = [...inside.folders][0]!
        joined = `${joined}/${next}`
        inside = only
      }
      rows.push({ kind: "folder", name: joined, depth })
      walk(inside, depth + 1)
    }
    for (const name of [...folder.files.keys()].sort(byName)) rows.push({ kind: "file", name, depth, path: folder.files.get(name)! })
  }
  walk(top, 0)
  return rows
}

// Whether a key pressed is the one that shows and hides the diff, as T3
// Code's: mod+D, mod being Cmd on a Mac and Ctrl elsewhere, known by the
// key's place. A key held down toggles once.
export function isDiffShortcut(keys: Keys, mac: boolean): boolean {
  if (keys.repeat || keys.altKey || keys.shiftKey) return false
  return keys.code === "KeyD" && (mac ? keys.metaKey && !keys.ctrlKey : keys.ctrlKey && !keys.metaKey)
}
