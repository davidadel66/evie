import { expect, test } from "bun:test"
import type { ChangedFile, FileDiff, TurnChanges } from "../src/core/types.ts"
import { cardFiles, changedFilesLine, filesTree, firstDrawn, foldFiles, hasCounts, scopeChoices, treeFolders } from "../web/src/diff.ts"

// The diff's rules, apart from its screen: what the right panel compares,
// the changed-files line and tree, and a file's two sides from its diff.

const file = (path: string, added: number, removed: number, extra: Partial<ChangedFile> = {}): ChangedFile => ({ path, status: "changed", added, removed, ...extra })

test("the changed-files line counts the files, and the lines added and removed across those whose lines were counted", () => {
  const files = [file("a.ts", 10, 2), file("b.ts", 2, 2), file("c.png", 0, 0, { binary: true }), file("d.md", 0, 0, { unknown: true }), file("sub", 0, 0, { unknown: true, submodule: true })]
  expect(changedFilesLine(files)).toEqual({ files: "5 changed files", added: 12, removed: 4 })
  expect(changedFilesLine([file("a.ts", 1, 0)])).toEqual({ files: "1 changed file", added: 1, removed: 0 })
})

test("a file shows how many lines it added and removed only when they were counted", () => {
  expect(hasCounts(file("a.ts", 1, 1))).toBe(true)
  // Left out for size, after its lines were counted.
  expect(hasCounts(file("big.ts", 900, 0, { tooLarge: true }))).toBe(true)
  for (const extra of [{ binary: true }, { unknown: true }, { tooMany: true }, { tooLarge: true }, { tooSlow: true }, { filtered: true }] as const) expect(hasCounts(file("x", 0, 0, extra))).toBe(false)
})

// The time 14:02 on 22 November 2026 in UTC, and the day before.
const AT = Date.UTC(2026, 10, 22, 14, 2)
const DAY = 24 * 60 * 60_000
const changed = (turn: number, time: number): TurnChanges => ({ turn, start: turn * 10, end: turn * 10 + 5, time, files: [file("a.ts", 1, 1)] })

test("what to compare is picked from the latest turn, each turn that changed files, newest first, with its time, the session, and with git, the uncommitted work and the branch", () => {
  const turns = [changed(2, AT - DAY), changed(5, AT)]
  expect(scopeChoices({ turns, git: true, now: AT, timeZone: "UTC" })).toEqual([
    { key: "latest", label: "Latest turn" },
    { key: "turn:5", label: "Turn 5", time: "14:02" },
    { key: "turn:2", label: "Turn 2", time: "21 Nov, 14:02" },
    { key: "session", label: "This session" },
    { key: "uncommitted", label: "Uncommitted" },
    { key: "branch", label: "This branch" },
  ])
  // Outside git, the uncommitted work and the branch are there, but cannot be picked, and say why.
  expect(scopeChoices({ turns: [], git: false, now: AT, timeZone: "UTC" })).toEqual([
    { key: "latest", label: "Latest turn" },
    { key: "session", label: "This session" },
    { key: "uncommitted", label: "Uncommitted", disabled: "Not a git repository" },
    { key: "branch", label: "This branch", disabled: "Not a git repository" },
  ])
  // Not known yet, they can be picked, and the server says.
  expect(scopeChoices({ turns: [], git: undefined, now: AT, timeZone: "UTC" }).filter((choice) => choice.disabled !== undefined)).toEqual([])
  // With no project, there is no repository to compare.
  expect(scopeChoices({ turns: [], git: null, now: AT, timeZone: "UTC" }).map((choice) => choice.key)).toEqual(["latest", "session"])
})

test("the files' tree puts folders before files, each in order of name, joins a folder that holds only one folder to it, and sums each folder's lines", () => {
  const files = [file("README.md", 2, 1), file("src/core/b.ts", 3, 0), file("src/core/a.ts", 1, 4), file("src/web/c.tsx", 5, 5), file("src/web/logo.png", 0, 0, { binary: true }), file("docs/guide/start/one.md", 7, 0)]
  const drawn = (rows: ReturnType<typeof filesTree>) => rows.map((row) => `${"  ".repeat(row.depth)}${row.kind === "folder" ? `${row.name}/${row.open ? "" : " (folded)"}` : row.name} ${row.counts === undefined ? "" : `+${row.counts.added} −${row.counts.removed}`}`.trimEnd())
  expect(drawn(filesTree(files, new Set()))).toEqual([
    "docs/guide/start/ +7 −0",
    "  one.md +7 −0",
    "src/ +9 −9",
    "  core/ +4 −4",
    "    a.ts +1 −4",
    "    b.ts +3 −0",
    "  web/ +5 −5",
    "    c.tsx +5 −5",
    "    logo.png",
    "README.md +2 −1",
  ])
  // A folder folded hides what is in it; each is known by its whole path.
  expect(drawn(filesTree(files, new Set(["src/core", "docs/guide/start"])))).toEqual([
    "docs/guide/start/ (folded) +7 −0",
    "src/ +9 −9",
    "  core/ (folded) +4 −4",
    "  web/ +5 −5",
    "    c.tsx +5 −5",
    "    logo.png",
    "README.md +2 −1",
  ])
  const rows = filesTree(files, new Set())
  expect(rows.flatMap((row) => (row.kind === "file" ? [row.path] : []))).toEqual(["docs/guide/start/one.md", "src/core/a.ts", "src/core/b.ts", "src/web/c.tsx", "src/web/logo.png", "README.md"])
  expect(rows.flatMap((row) => (row.kind === "folder" ? [row.path] : []))).toEqual(["docs/guide/start", "src", "src/core", "src/web"])
  // Collapse all and Expand all read the folders shown: all open, they fold
  // every folder; one shown folded, they open them all. A folder folded out
  // of sight, or one of another comparison, does not count.
  expect(treeFolders(files, new Set())).toEqual({ all: ["docs/guide/start", "src", "src/core", "src/web"], open: true })
  expect(treeFolders(files, new Set(["src/core"])).open).toBe(false)
  expect(treeFolders(files, new Set(["src", "src/core"])).open).toBe(false)
  expect(treeFolders(files, new Set(["elsewhere"])).open).toBe(true)
})

const diffed = (path: string, extra: Partial<FileDiff> = {}): FileDiff => ({ path, status: "changed", added: 1, removed: 1, patch: "@@ -1,1 +1,1 @@\n-a\n+b\n", ...extra })

test("the diff opens with every file folded but the first that has a diff, or the one it was opened at; a file that comes later is folded, and what the owner folded or opened stays", () => {
  const picture = file("a.png", 0, 0, { binary: true })
  const files = [picture, diffed("b.md"), diffed("c.md")]
  expect(firstDrawn(files)).toBe("b.md")
  const first = foldFiles(undefined, files, undefined)
  expect([...first.folded]).toEqual(["c.md"])
  // Opened at a file, that one is open instead.
  expect([...foldFiles(undefined, files, "c.md").folded]).toEqual(["b.md"])
  // A file that comes later, as when a turn ends, is folded; the owner's own folding stays.
  const owned = { seen: first.seen, folded: new Set(["b.md"]) }
  expect([...foldFiles(owned, [...files, diffed("d.md")], undefined).folded].sort()).toEqual(["b.md", "d.md"])
  // Asked to go to a file folded, it opens.
  expect([...foldFiles(owned, files, "b.md").folded]).toEqual([])
  // A file renamed and not changed has a diff to draw: its header.
  expect(firstDrawn([picture, diffed("z.md", { from: "y.md", patch: "" })])).toBe("z.md")
  expect(firstDrawn([picture])).toBeUndefined()
})

test("a turn's card lists at most 200 files, in the order of their paths, and says how many more the full diff has", () => {
  const many = Array.from({ length: 205 }, (_, at) => file(`f${String(at).padStart(3, "0")}.md`, 1, 0)).reverse()
  const { listed, more } = cardFiles(many)
  expect(listed).toHaveLength(200)
  expect(listed[0]!.path).toBe("f000.md")
  expect(listed.at(-1)!.path).toBe("f199.md")
  expect(more).toBe(5)
  expect(cardFiles([file("b.md", 1, 0), file("a.md", 1, 0)])).toEqual({ listed: [file("a.md", 1, 0), file("b.md", 1, 0)], more: 0 })
})
