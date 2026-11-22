import { expect, test } from "bun:test"
import type { ChangedFile, TurnChanges } from "../src/core/types.ts"
import { changedFilesLine, drawnLines, fileTree, hasCounts, hunksOf, scopeChoices, sessionLine, sideBySide } from "../web/src/diff.ts"

// The diff panel's rules, apart from its screen: a file's diff split into
// hunks and lines, drawn stacked or side by side.

const PATCH = ["@@ -1,4 +1,4 @@", " one", "-two", "-three", "+2", "+3", "+3.5", " four", "@@ -10,2 +11,2 @@", " ten", "-eleven", "\\ No newline at end of file", "+11", ""].join("\n")

test("a diff is split into hunks, each line numbered as it stands before and after", () => {
  expect(hunksOf(PATCH)).toEqual([
    {
      header: "@@ -1,4 +1,4 @@",
      lines: [
        { kind: "same", text: "one", old: 1, new: 1 },
        { kind: "removed", text: "two", old: 2 },
        { kind: "removed", text: "three", old: 3 },
        { kind: "added", text: "2", new: 2 },
        { kind: "added", text: "3", new: 3 },
        { kind: "added", text: "3.5", new: 4 },
        { kind: "same", text: "four", old: 4, new: 5 },
      ],
    },
    {
      header: "@@ -10,2 +11,2 @@",
      lines: [
        { kind: "same", text: "ten", old: 10, new: 11 },
        { kind: "removed", text: "eleven", old: 11, noNewline: true },
        { kind: "added", text: "11", new: 12 },
      ],
    },
  ])
})

test("side by side, what was removed faces what was added in its place, line by line, and the rest faces nothing", () => {
  const [first] = hunksOf(PATCH)
  expect(sideBySide(first!.lines).map(({ left, right }) => [left?.text ?? null, right?.text ?? null])).toEqual([
    ["one", "one"],
    ["two", "2"],
    ["three", "3"],
    [null, "3.5"],
    ["four", "four"],
  ])
  // Lines added on their own, with nothing removed before them, face nothing.
  const [added] = hunksOf("@@ -0,0 +1,2 @@\n+a\n+b\n")
  expect(sideBySide(added!.lines).map(({ left, right }) => [left?.text ?? null, right?.text ?? null])).toEqual([
    [null, "a"],
    [null, "b"],
  ])
})

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

test("the project card sums up the session's changes, or says it has none, or that they are not kept", () => {
  expect(sessionLine({ kept: true, session: [file("a.ts", 3, 1), file("b.ts", 1, 1)] })).toEqual({ files: "2 changed files", added: 4, removed: 2 })
  expect(sessionLine({ kept: true, session: [] })).toBe("No changes in this session yet")
  expect(sessionLine({ kept: false, session: [] })).toBe("evie keeps no copies of the files it changes here, so its changes are not shown.")
  expect(sessionLine(undefined)).toBe("")
})

test("a file is drawn up to a few thousand lines, and the rest is left out, which is said", () => {
  const long = `@@ -0,0 +1,5000 @@\n${Array.from({ length: 5000 }, (_, at) => `+line ${at + 1}`).join("\n")}\n`
  const drawn = drawnLines(hunksOf(long), 3000)
  expect(drawn.cut).toBe(true)
  expect(drawn.hunks.flatMap((hunk) => hunk.lines)).toHaveLength(3000)
  expect(drawn.widest).toBe(3000)
  const short = drawnLines(hunksOf(PATCH), 3000)
  expect(short).toEqual({ hunks: hunksOf(PATCH), cut: false, widest: 12 })
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

test("the file tree puts folders before files, each in order of name, and joins a folder that holds only one folder to it", () => {
  const rows = fileTree(["README.md", "src/core/b.ts", "src/core/a.ts", "src/web/c.tsx", "test/x.test.ts", "docs/guide/start/one.md"])
  expect(rows.map((row) => `${"  ".repeat(row.depth)}${row.kind === "folder" ? `${row.name}/` : row.name}`)).toEqual([
    "docs/guide/start/",
    "  one.md",
    "src/",
    "  core/",
    "    a.ts",
    "    b.ts",
    "  web/",
    "    c.tsx",
    "test/",
    "  x.test.ts",
    "README.md",
  ])
  // Each file is known by its whole path.
  expect(rows.flatMap((row) => (row.kind === "file" ? [row.path] : []))).toEqual(["docs/guide/start/one.md", "src/core/a.ts", "src/core/b.ts", "src/web/c.tsx", "test/x.test.ts", "README.md"])
})
