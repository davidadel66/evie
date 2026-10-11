import { describe, expect, it } from "vitest";
import type { ComparedFile } from "../api/changes";
import { isViewed, markViewed, parsePatch, readLayout, readViewed, splitPath, splitRows, wordSpans, writeLayout, writeViewed } from "./diffView";

const patch = `@@ -41,6 +41,7 @@ describe("restore", () => {
 const a = 1;
-const timeout = 100;
-retry(3);
+const timeout = 2000;
+retry(5);
+waitForIdle();
 done();
\\ No newline at end of file
@@ -90 +91 @@
-old
+new
`;

// changes.spec.md story 6a: the review page's diff.
describe("diffs", () => {
  it("parse hunks with old and new line numbers", () => {
    const hunks = parsePatch(patch);
    expect(hunks.map((hunk) => hunk.header)).toEqual([`@@ -41,6 +41,7 @@ describe("restore", () => {`, "@@ -90 +91 @@"]);
    expect(hunks[0].lines).toEqual([
      { kind: "context", text: "const a = 1;", old: 41, new: 41 },
      { kind: "del", text: "const timeout = 100;", old: 42 },
      { kind: "del", text: "retry(3);", old: 43 },
      { kind: "add", text: "const timeout = 2000;", new: 42 },
      { kind: "add", text: "retry(5);", new: 43 },
      { kind: "add", text: "waitForIdle();", new: 44 },
      { kind: "context", text: "done();", old: 44, new: 45 },
      { kind: "note", text: "No newline at end of file" },
    ]);
    expect(hunks[1].lines).toEqual([{ kind: "del", text: "old", old: 90 }, { kind: "add", text: "new", new: 91 }]);
    expect(parsePatch("")).toEqual([]);
  });

  it("pair removed lines with the added lines after them for side by side", () => {
    const rows = splitRows(parsePatch(patch)[0]);
    expect(rows.map((row) => row.note ?? `${row.left?.old ?? "·"}|${row.right?.new ?? "·"}`)).toEqual(["41|41", "42|42", "43|43", "·|44", "44|45", "No newline at end of file"]);
  });

  it("mark the changed part of a changed line when enough of it is the same", () => {
    expect(wordSpans("const timeout = 100;", "const timeout = 2000;")).toEqual({
      before: ["const timeout = ", "1", "00;"], after: ["const timeout = ", "20", "00;"],
    });
    expect(wordSpans("abc", "xyz")).toBeUndefined();
    expect(wordSpans("same", "same")).toBeUndefined();
  });

  it("dim a file's directory beside its name", () => {
    expect(splitPath("ui/src/workspaces/Workspace.tsx")).toEqual({ dir: "ui/src/workspaces/", name: "Workspace.tsx" });
    expect(splitPath("README.md")).toEqual({ dir: "", name: "README.md" });
  });
});

class MemoryStorage {
  private values = new Map<string, string>();
  getItem(key: string) { return this.values.get(key) ?? null; }
  setItem(key: string, value: string) { this.values.set(key, value); }
}

describe("Viewed marks", () => {
  const file: ComparedFile = { path: "a.ts", status: "modified", additions: 1, deletions: 0, blob: "b1" };
  it("remember the blob a file was viewed at, so a new commit clears them", () => {
    const storage = new MemoryStorage() as unknown as Storage;
    let marks = markViewed(readViewed(storage, "w", "c"), file, true);
    writeViewed(storage, "w", "c", marks);
    marks = readViewed(storage, "w", "c");
    expect(isViewed(marks, file)).toBe(true);
    expect(isViewed(marks, { ...file, blob: "b2" })).toBe(false);
    expect(isViewed(readViewed(storage, "w", "other"), file)).toBe(false);
    expect(isViewed(markViewed(marks, file, false), file)).toBe(false);
    expect(readViewed(undefined, "w", "c")).toEqual({});
  });

  it("keep the diff layout the owner chose", () => {
    const storage = new MemoryStorage() as unknown as Storage;
    expect(readLayout(storage)).toBe("split");
    writeLayout(storage, "unified");
    expect(readLayout(storage)).toBe("unified");
  });
});
