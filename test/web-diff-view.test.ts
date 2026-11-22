import { expect, test } from "bun:test"
import type { FileDiff } from "../src/core/types.ts"
import { diffOf, sidesLoader } from "../web/src/diff-view.ts"

// The diff view, the one module that draws a diff with T3 Code's library,
// @pierre/diffs: what it makes of a file's diff as the server sends it,
// with three lines around each change, and of the file's whole sides, read
// when the unchanged lines between the changes are first opened.

// A file of thirty lines, "line 1" to "line 30", and the same with lines 10 and 25 changed.
const thirty = (change: (line: number) => string = (line) => `line ${line}`) => Array.from({ length: 30 }, (_, at) => `${change(at + 1)}\n`).join("")
const after = thirty((line) => (line === 10 ? "ten" : line === 25 ? "twenty-five" : `line ${line}`))
const changed: FileDiff = {
  path: "src/notes.md",
  status: "changed",
  added: 2,
  removed: 2,
  patch: [
    "@@ -7,7 +7,7 @@",
    ...[" line 7", " line 8", " line 9", "-line 10", "+ten", " line 11", " line 12", " line 13"],
    "@@ -22,7 +22,7 @@",
    ...[" line 22", " line 23", " line 24", "-line 25", "+twenty-five", " line 26", " line 27", " line 28"],
    "",
  ].join("\n"),
}

test("a file's unchanged lines before and between its changes are folded and counted, and the file can be read whole to open them", () => {
  const diff = diffOf(changed)!

  expect(diff.name).toBe("src/notes.md")
  expect(diff.type).toBe("change")
  // Lines 1 to 6 are folded before the first change, and 14 to 21 between the two.
  expect(diff.hunks.map((hunk) => ({ folded: hunk.collapsedBefore, from: hunk.additionStart, lines: hunk.additionCount }))).toEqual([
    { folded: 6, from: 7, lines: 7 },
    { folded: 8, from: 22, lines: 7 },
  ])
  // Only the lines around the changes came with it: the rest is read when asked for.
  expect(diff.isPartial).toBe(true)
})

test("the file's sides, read when its folded lines are first opened, are handed to the library, and sides that no longer fit its diff, or cannot be read, say why", async () => {
  const asked: { path: string; from: string | undefined }[] = []
  const load = sidesLoader(async (path, from) => {
    asked.push({ path, from })
    return { before: thirty(), after }
  })
  expect(await load(diffOf(changed)!)).toEqual({ oldFile: { name: "src/notes.md", contents: thirty() }, newFile: { name: "src/notes.md", contents: after } })
  expect(asked).toEqual([{ path: "src/notes.md", from: undefined }])

  // Changed since the diff was read: its lines are not where the diff says.
  const since = sidesLoader(async () => ({ before: thirty(), after: thirty((line) => (line === 3 ? "three" : `line ${line}`)) }))
  await expect(since(diffOf(changed)!)).rejects.toThrow("src/notes.md has changed since its diff was read: refresh the diff to see it as it is now.")
  const large = sidesLoader(async () => ({ unavailable: "src/notes.md is too large to show in full." }))
  await expect(large(diffOf(changed)!)).rejects.toThrow("src/notes.md is too large to show in full.")
})

test("a file renamed is drawn from the path it had, and one renamed without a change is drawn as its header alone", () => {
  const moved = diffOf({ ...changed, path: "docs/notes.md", from: "src/notes.md" })!
  expect([moved.type, moved.prevName, moved.name]).toEqual(["rename-changed", "src/notes.md", "docs/notes.md"])
  const pure = diffOf({ path: "docs/old.md", from: "old.md", status: "changed", added: 0, removed: 0, patch: "" })!
  expect([pure.type, pure.prevName, pure.hunks.length]).toEqual(["rename-pure", "old.md", 0])
})

test("a file added or removed whole is drawn as such, and one with no diff to draw is not drawn", () => {
  const added = diffOf({ path: "new.md", status: "added", added: 2, removed: 0, patch: "@@ -0,0 +1,2 @@\n+a\n+b\n" })!
  expect(added.type).toBe("new")
  expect(added.hunks.map((hunk) => hunk.additionLines)).toEqual([2])
  expect(diffOf({ path: "old.md", status: "removed", added: 0, removed: 1, patch: "@@ -1,1 +0,0 @@\n-gone\n" })!.type).toBe("deleted")
  expect(diffOf({ path: "logo.png", status: "changed", added: 0, removed: 0, binary: true })).toBeUndefined()
})
