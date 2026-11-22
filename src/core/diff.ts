import { structuredPatch } from "diff"
import type { FileDiff } from "./types.ts"

// One file's change, from what it held before to what it holds after, as
// a client is shown it: whether it was added, removed or changed, how many
// lines were added and removed, and its diff. The lines are compared with
// `diff`, the jsdiff library. A file that is binary, or too large to
// compare in good time, says so instead.

// A file larger than this, before or after, is too large to show.
const LARGEST = 1024 * 1024
// The most lines a diff may add and remove, and the longest, in
// milliseconds, comparing may take: past either, the file is too large to show.
const MOST_EDITS = 10_000
const LONGEST = 1000
// How many unchanged lines are shown around each change.
const CONTEXT = 3

// A file holding a NUL byte in its first 8000 bytes is binary, as git
// decides it.
const isBinary = (contents: Uint8Array) => contents.subarray(0, 8000).includes(0)

const decoder = new TextDecoder()

// Contents are null when there was no file, and undefined when they are
// not known. Gives back nothing when the file did not change. Without
// `patch`, only how many lines changed is worked out.
export function compare(path: string, before: Uint8Array | null | undefined, after: Uint8Array | null | undefined, options: { patch: boolean }): FileDiff | undefined {
  if (before === null && after === null) return undefined
  if (before !== undefined && before !== null && after !== undefined && after !== null && Buffer.from(before).equals(after)) return undefined
  const status = before === null ? "added" : after === null ? "removed" : "changed"
  const file = { path, status, added: 0, removed: 0 } as const
  if (before === undefined || after === undefined) return { ...file, unknown: true }
  const sides = [before ?? new Uint8Array(), after ?? new Uint8Array()] as const
  if (sides.some(isBinary)) return { ...file, binary: true }
  if (sides.some((side) => side.length > LARGEST)) return { ...file, tooLarge: true }
  const [old, now] = sides.map((side) => decoder.decode(side))
  // A file added or removed whole is every line of it, with nothing to work out.
  const compared = old === "" || now === "" ? whole(old!, now!) : structuredPatch(path, path, old!, now!, undefined, undefined, { context: CONTEXT, maxEditLength: MOST_EDITS, timeout: LONGEST })
  if (compared === undefined) return { ...file, tooLarge: true }
  let added = 0
  let removed = 0
  const lines: string[] = []
  for (const hunk of compared.hunks) {
    // A range of no lines starts at the line before it, as unified diffs write it.
    const range = (start: number, count: number) => `${count === 0 ? start - 1 : start},${count}`
    if (options.patch) lines.push(`@@ -${range(hunk.oldStart, hunk.oldLines)} +${range(hunk.newStart, hunk.newLines)} @@`)
    for (const line of hunk.lines) {
      if (line.startsWith("+")) added += 1
      else if (line.startsWith("-")) removed += 1
      if (options.patch) lines.push(line)
    }
  }
  return { ...file, added, removed, ...(options.patch && { patch: lines.length === 0 ? "" : `${lines.join("\n")}\n` }) }
}

// The one hunk of a file that is all added or all removed, as jsdiff gives
// a hunk: its lines each marked, and a mark after a last line with no end.
function whole(old: string, now: string): { hunks: { oldStart: number; oldLines: number; newStart: number; newLines: number; lines: string[] }[] } {
  const text = old === "" ? now : old
  if (text === "") return { hunks: [] }
  const sign = old === "" ? "+" : "-"
  const lines = text.split("\n")
  const ended = lines.at(-1) === ""
  if (ended) lines.pop()
  const marked = [...lines.map((line) => sign + line), ...(ended ? [] : ["\\ No newline at end of file"])]
  return { hunks: [{ oldStart: 1, oldLines: old === "" ? 0 : lines.length, newStart: 1, newLines: old === "" ? lines.length : 0, lines: marked }] }
}

// The same, worked out on a thread of its own when lines have to be
// compared, so that a long comparison does not hold up the server, whose
// one thread serves every session. What needs no comparing, such as a
// binary file, one too large, or one added or removed whole, is answered
// here. Should the thread fail, the file is compared here instead.
export async function compareApart(path: string, before: Uint8Array | null | undefined, after: Uint8Array | null | undefined, options: { patch: boolean }): Promise<FileDiff | undefined> {
  const quick = before == null || after == null || before.length === 0 || after.length === 0 || before.length > LARGEST || after.length > LARGEST || isBinary(before) || isBinary(after)
  if (quick || Buffer.from(before).equals(after)) return compare(path, before, after, options)
  try {
    return await onThread(path, before, after, options.patch)
  } catch {
    return compare(path, before, after, options)
  }
}

let thread: Worker | undefined
const waiting = new Map<number, { resolve(file: FileDiff | undefined): void; reject(cause: unknown): void }>()
let asked = 0
function onThread(path: string, before: Uint8Array, after: Uint8Array, patch: boolean): Promise<FileDiff | undefined> {
  if (thread === undefined) {
    const made = new Worker(new URL("./diff-worker.ts", import.meta.url).href)
    // The thread is no reason on its own to keep the server running.
    made.unref()
    made.onmessage = (event: MessageEvent<{ id: number; file: FileDiff | undefined }>) => {
      waiting.get(event.data.id)?.resolve(event.data.file)
      waiting.delete(event.data.id)
    }
    made.onerror = (event) => {
      // Every comparison waiting on it is made here instead, and the next starts a new thread.
      thread = undefined
      for (const each of waiting.values()) each.reject(event)
      waiting.clear()
    }
    thread = made
  }
  const id = ++asked
  return new Promise((resolve, reject) => {
    waiting.set(id, { resolve, reject })
    thread!.postMessage({ id, path, before, after, patch })
  })
}
