import { expect, test } from "bun:test"
import type { FileDiff } from "../src/core/types.ts"
import { diffOf } from "../web/src/diff-view.ts"

// The diff view, the one module that draws a diff with T3 Code's library,
// @pierre/diffs: what it makes of a file's diff as the server sends it,
// with the whole file around its changes.

// A file of thirty lines, "line 1" to "line 30", with lines 10 and 25 changed.
const numbered = (line: number) => (line === 10 ? ["-line 10", "+ten"] : line === 25 ? ["-line 25", "+twenty-five"] : [` line ${line}`])
const changed: FileDiff = {
  path: "src/notes.md",
  status: "changed",
  added: 2,
  removed: 2,
  patch: `@@ -1,30 +1,30 @@\n${Array.from({ length: 30 }, (_, at) => numbered(at + 1)).flat().join("\n")}\n`,
}

test("a file's unchanged lines are folded to three around each change, those folded are counted, and the whole file is known, so that they can be opened", () => {
  const diff = diffOf(changed)!

  expect(diff.name).toBe("src/notes.md")
  expect(diff.type).toBe("change")
  // Lines 1 to 6 are folded before the first change, and 14 to 21 between the two.
  expect(diff.hunks.map((hunk) => ({ folded: hunk.collapsedBefore, from: hunk.additionStart, lines: hunk.additionCount }))).toEqual([
    { folded: 6, from: 7, lines: 7 },
    { folded: 8, from: 22, lines: 7 },
  ])
  expect(diff.isPartial).toBe(false)
  expect(diff.additionLines).toHaveLength(30)
  expect(diff.deletionLines[9]).toBe("line 10\n")
  expect(diff.additionLines[29]).toBe("line 30\n")
})

test("a file added or removed whole is drawn as such, and one with no diff to draw is not drawn", () => {
  const added = diffOf({ path: "new.md", status: "added", added: 2, removed: 0, patch: "@@ -0,0 +1,2 @@\n+a\n+b\n" })!
  expect(added.type).toBe("new")
  expect(added.hunks.map((hunk) => hunk.additionLines)).toEqual([2])
  expect(diffOf({ path: "old.md", status: "removed", added: 0, removed: 1, patch: "@@ -1,1 +0,0 @@\n-gone\n" })!.type).toBe("deleted")
  expect(diffOf({ path: "logo.png", status: "changed", added: 0, removed: 0, binary: true })).toBeUndefined()
})
