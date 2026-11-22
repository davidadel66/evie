import { structuredPatch } from "diff"
import { lstat, open, readlink } from "node:fs/promises"
import type { Contents, FileDiff } from "./types.ts"

// One file's change, from what it held before to what it holds after, as
// a client is shown it: whether it was added, removed or changed, how many
// lines were added and removed, and its diff. The lines are compared with
// `diff`, the jsdiff library. A file that is binary, or too large to
// compare in good time, says so instead.

// A file larger than this, before or after, is too large to show, and is
// not read.
export const LARGEST = 1024 * 1024
export const TOO_LARGE = { tooLarge: true } as const
const isTooLarge = (contents: Contents): contents is typeof TOO_LARGE => contents != null && !(contents instanceof Uint8Array)

// What is at a path now: a file's contents, a link's target, or null when
// there is nothing there; too large, for a file over the limit, whose size
// is looked at before anything is read, and of which no more is ever read
// than the limit; and undefined when it cannot be read, as for a folder.
export async function contentsAt(path: string): Promise<Contents> {
  try {
    const kind = await lstat(path)
    if (kind.isSymbolicLink()) return new TextEncoder().encode(await readlink(path))
    if (!kind.isFile()) return undefined
    if (kind.size > LARGEST) return TOO_LARGE
    const file = await open(path, "r")
    try {
      // Read up to one byte past the limit, in case it grew since it was looked at.
      const room = new Uint8Array(LARGEST + 1)
      let read = 0
      for (;;) {
        const { bytesRead } = await file.read(room, read, room.length - read, null)
        if (bytesRead === 0) break
        read += bytesRead
        if (read > LARGEST) return TOO_LARGE
      }
      return room.slice(0, read)
    } finally {
      await file.close()
    }
  } catch (cause) {
    const code = (cause as { code?: string }).code
    return code === "ENOENT" || code === "ENOTDIR" ? null : undefined
  }
}
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

// Gives back nothing when the file did not change. Without `patch`, only
// how many lines changed is worked out.
export function compare(path: string, before: Contents, after: Contents, options: { patch: boolean }): FileDiff | undefined {
  if (before === null && after === null) return undefined
  if (before instanceof Uint8Array && after instanceof Uint8Array && Buffer.from(before).equals(after)) return undefined
  const file = { path, status: statusOf(before, after), added: 0, removed: 0 } as const
  if (before === undefined || after === undefined) return { ...file, unknown: true }
  if (isTooLarge(before) || isTooLarge(after)) return { ...file, tooLarge: true }
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

// One file's whole contents, before and after, as text for a client to
// show the unchanged lines between its changes, or why they cannot be:
// nothing changed, or a side is not known, binary, or larger than a diff
// shows. `name` is the file as the client knows it.
export function fileSides(name: string, before: Contents, after: Contents): { before: string | null; after: string | null } | { unavailable: string } {
  const same = before instanceof Uint8Array && after instanceof Uint8Array && Buffer.from(before).equals(after)
  if ((before === null && after === null) || same) return { unavailable: `${name} is not among the files this comparison changed.` }
  if (before === undefined || after === undefined) return { unavailable: `What ${name} held is not known.` }
  if (isTooLarge(before) || isTooLarge(after) || [before, after].some((side) => side !== null && side.length > LARGEST)) return { unavailable: `${name} is too large to show in full.` }
  if ([before, after].some((side) => side !== null && isBinary(side))) return { unavailable: `${name} is binary, so it is not shown.` }
  return { before: before === null ? null : decoder.decode(before), after: after === null ? null : decoder.decode(after) }
}

const statusOf = (before: Contents, after: Contents) => (before === null ? "added" : after === null ? "removed" : "changed")

// The same, worked out on a thread of its own when lines have to be
// compared, so that a long comparison does not hold up the server, whose
// one thread serves every session. What needs no comparing, such as a
// binary file, one too large, or one added or removed whole, is answered
// here. A comparison sent to the thread that has no answer `within` its
// limit is too slow, and so is one asked for once the whole answer's time
// is up (`until`, a time): it is listed, and not compared. Should the
// thread fail or stop, the file is compared here instead, and the next
// comparison starts a new thread.
export async function compareApart(path: string, before: Contents, after: Contents, options: { patch: boolean; until?: number; within?: number }): Promise<FileDiff | undefined> {
  if (!(before instanceof Uint8Array) || !(after instanceof Uint8Array)) return compare(path, before, after, options)
  const quick = before.length === 0 || after.length === 0 || before.length > LARGEST || after.length > LARGEST || isBinary(before) || isBinary(after)
  if (quick || Buffer.from(before).equals(after)) return compare(path, before, after, options)
  const tooSlow: FileDiff = { path, status: statusOf(before, after), added: 0, removed: 0, tooSlow: true }
  if (options.until !== undefined && Date.now() > options.until) return tooSlow
  try {
    return await onThread(path, before, after, options.patch, options.within ?? WITHIN)
  } catch (cause) {
    if (cause === TOO_SLOW) return tooSlow
    return compare(path, before, after, options)
  }
}

// How long one comparison sent to the thread may take, in milliseconds,
// waiting behind the others sent with it included. Each is given a second
// to compare its lines, and a few are sent at once.
const WITHIN = 15_000
const TOO_SLOW = Symbol("too slow")

let thread: Worker | undefined
const waiting = new Map<number, { resolve(file: FileDiff | undefined): void; reject(cause: unknown): void; timer: ReturnType<typeof setTimeout> }>()
let asked = 0

// The thread is let go, and what was sent to it is given up on, each for
// the reason given.
function letGo(why: unknown) {
  const was = thread
  thread = undefined
  for (const each of waiting.values()) {
    clearTimeout(each.timer)
    each.reject(why)
  }
  waiting.clear()
  was?.terminate()
}

function onThread(path: string, before: Uint8Array, after: Uint8Array, patch: boolean, within: number): Promise<FileDiff | undefined> {
  if (thread === undefined) {
    const made = new Worker(new URL("./diff-worker.ts", import.meta.url).href)
    // The thread is no reason on its own to keep the server running.
    made.unref()
    made.onmessage = (event: MessageEvent<{ id: number; file: FileDiff | undefined }>) => {
      const done = waiting.get(event.data.id)
      if (!done) return
      clearTimeout(done.timer)
      waiting.delete(event.data.id)
      done.resolve(event.data.file)
    }
    // Failed or stopped: what it was doing is made here instead.
    made.onerror = (event) => thread === made && letGo(event)
    made.addEventListener("close", () => thread === made && letGo(new Error("The thread comparing lines stopped.")))
    thread = made
  }
  const id = ++asked
  return new Promise((resolve, reject) => {
    // Past its limit, the thread is taken to be stuck: it is stopped, and
    // what was sent to it is too slow; the next comparison starts a new one.
    const timer = setTimeout(() => letGo(TOO_SLOW), within)
    waiting.set(id, { resolve, reject, timer })
    // Each side is sent as a copy of its own, since a part of a larger
    // buffer would send the whole of it.
    thread!.postMessage({ id, path, before: new Uint8Array(before), after: new Uint8Array(after), patch })
  })
}
