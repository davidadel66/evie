import type { ComparedFile } from "../api/changes";

// A file's diff as the review page draws it (changes.spec.md story 6a):
// git's hunks parsed into lines with their old and new numbers, paired for
// the side-by-side view, with the changed part of a changed line marked.

export type DiffLine = { kind: "context" | "add" | "del" | "note"; text: string; old?: number; new?: number };
export type Hunk = { header: string; lines: DiffLine[] };

const hunkHeader = /^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/;

/** Parses a file's hunks, from its first "@@" line. */
export function parsePatch(patch = ""): Hunk[] {
  const hunks: Hunk[] = [];
  let hunk: Hunk | undefined;
  let oldLine = 0;
  let newLine = 0;
  const lines = patch.split("\n");
  if (lines[lines.length - 1] === "") lines.pop();
  for (const line of lines) {
    const header = hunkHeader.exec(line);
    if (header) {
      hunk = { header: line, lines: [] };
      hunks.push(hunk);
      oldLine = Number(header[1]);
      newLine = Number(header[2]);
      continue;
    }
    if (!hunk) continue;
    switch (line[0]) {
      case "+":
        hunk.lines.push({ kind: "add", text: line.slice(1), new: newLine++ });
        break;
      case "-":
        hunk.lines.push({ kind: "del", text: line.slice(1), old: oldLine++ });
        break;
      case "\\":
        hunk.lines.push({ kind: "note", text: "No newline at end of file" });
        break;
      default:
        hunk.lines.push({ kind: "context", text: line.slice(1), old: oldLine++, new: newLine++ });
    }
  }
  return hunks;
}

/** One row of the side-by-side view: the old line, the new line, or a note across both. */
export type SplitRow = { left?: DiffLine; right?: DiffLine; note?: string };

/** Pairs each run of removed lines with the added lines that follow it. */
export function splitRows(hunk: Hunk): SplitRow[] {
  const rows: SplitRow[] = [];
  let removed: DiffLine[] = [];
  let added: DiffLine[] = [];
  const flush = () => {
    for (let i = 0; i < Math.max(removed.length, added.length); i++) rows.push({ left: removed[i], right: added[i] });
    removed = [];
    added = [];
  };
  for (const line of hunk.lines) {
    if (line.kind === "del") {
      if (added.length > 0) flush();
      removed.push(line);
    } else if (line.kind === "add") {
      added.push(line);
    } else {
      flush();
      rows.push(line.kind === "note" ? { note: line.text } : { left: line, right: line });
    }
  }
  flush();
  return rows;
}

/** A line cut around what changed: [before, changed, after]. */
export type WordSpans = [string, string, string];

/**
 * The changed part of a removed line and the added line paired with it:
 * what lies between their common start and end, shown only when that
 * common part is at least 30% of the longer line.
 */
export function wordSpans(before: string, after: string): { before: WordSpans; after: WordSpans } | undefined {
  if (before === after) return undefined;
  let start = 0;
  while (start < before.length && start < after.length && before[start] === after[start]) start++;
  let end = 0;
  while (end < before.length - start && end < after.length - start && before[before.length - 1 - end] === after[after.length - 1 - end]) end++;
  if (start + end < 0.3 * Math.max(before.length, after.length)) return undefined;
  const cut = (text: string): WordSpans => [text.slice(0, start), text.slice(start, text.length - end), text.slice(text.length - end)];
  return { before: cut(before), after: cut(after) };
}

/** A file's directory and name, for a header that dims the directory. */
export function splitPath(path: string): { dir: string; name: string } {
  const slash = path.lastIndexOf("/");
  return slash < 0 ? { dir: "", name: path } : { dir: path.slice(0, slash + 1), name: path.slice(slash + 1) };
}

/**
 * "Viewed" marks, per browser (changes.spec.md Q23): each file of a Change
 * remembers the blob it was viewed at, so a new commit that changes the
 * file clears its mark.
 */
export type ViewedMarks = Record<string, string>;

/** The browser's storage, when it keeps any. */
export function browserStorage(): Storage | undefined {
  try {
    return typeof window === "undefined" ? undefined : window.localStorage;
  } catch {
    return undefined;
  }
}

const viewedKey = (workspaceId: string, changeId: string) => `evie.changes.viewed:${workspaceId}:${changeId}`;
const fileVersion = (file: ComparedFile) => file.blob ?? `deleted:${file.path}`;

export function isViewed(marks: ViewedMarks, file: ComparedFile): boolean {
  return marks[file.path] === fileVersion(file);
}

export function markViewed(marks: ViewedMarks, file: ComparedFile, viewed: boolean): ViewedMarks {
  const next = { ...marks };
  if (viewed) next[file.path] = fileVersion(file);
  else delete next[file.path];
  return next;
}

export function readViewed(storage: Storage | undefined, workspaceId: string, changeId: string): ViewedMarks {
  try {
    const value = JSON.parse(storage?.getItem(viewedKey(workspaceId, changeId)) ?? "{}");
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

export function writeViewed(storage: Storage | undefined, workspaceId: string, changeId: string, marks: ViewedMarks): void {
  try {
    storage?.setItem(viewedKey(workspaceId, changeId), JSON.stringify(marks));
  } catch {
    // A browser that keeps nothing still shows the marks until it reloads.
  }
}

export type DiffLayout = "split" | "unified";
const layoutKey = "evie.changes.diffLayout";

export function readLayout(storage: Storage | undefined): DiffLayout {
  try {
    return storage?.getItem(layoutKey) === "unified" ? "unified" : "split";
  } catch {
    return "split";
  }
}

export function writeLayout(storage: Storage | undefined, layout: DiffLayout): void {
  try {
    storage?.setItem(layoutKey, layout);
  } catch {
    // As above.
  }
}
