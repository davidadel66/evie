import { ATTACHMENT_LIMIT } from "../../src/core/types.ts"

// The message box's rules, apart from its screen: the menu a slash opens,
// how it is ranked and moved through, what a message that is a command
// says, and bringing back the owner's earlier messages; the menu of the
// project's files an @ opens, and the files attached. They follow T3
// Code's message box, which the owner chose as the model for evie's.

// A command or a skill as the menu lists it.
export type MenuItem = { name: string; description: string; kind?: string }

// Where the slash the caret is in begins, and what has been typed after it
// up to the caret, when the caret is in a word that starts with a slash:
// one at the start of the box, or after a space or a new line. `first`
// says it starts the message, where any command can be run; elsewhere only
// the model can be picked. A word with anything in it a command's name
// cannot have, such as a path's second slash, opens no menu.
export function slashAt(text: string, caret: number): { from: number; query: string; first: boolean } | undefined {
  const before = text.slice(0, caret)
  const from = Math.max(before.lastIndexOf(" "), before.lastIndexOf("\n"), before.lastIndexOf("\t")) + 1
  if (before[from] !== "/") return undefined
  const query = before.slice(from + 1)
  if (!/^[\w-]*$/.test(query)) return undefined
  return { from, query, first: text.slice(0, from).trim() === "" }
}

// The items whose name has what was typed, in any case: an exact name
// first, then a name that starts with it, then one with a word that starts
// with it, then one with it anywhere. Within each, the shorter name first,
// then the order they were listed in. With nothing typed, all of them as
// listed. Descriptions are not searched: a short word is in most of them.
export function rankCommands<Item extends MenuItem>(items: readonly Item[], typed: string): Item[] {
  const query = typed.trim().toLowerCase()
  if (query === "") return [...items]
  const tierOf = (item: Item): number | undefined => {
    const name = item.name.toLowerCase()
    if (name === query) return 0
    if (name.startsWith(query)) return 1
    if (name.split(/[-_]/).slice(1).some((word) => word.startsWith(query))) return 2
    if (name.includes(query)) return 3
    return undefined
  }
  return items
    .flatMap((item, at) => {
      const tier = tierOf(item)
      return tier === undefined ? [] : [{ item, tier, at }]
    })
    .sort((a, b) => a.tier - b.tier || a.item.name.length - b.item.name.length || a.at - b.at)
    .map(({ item }) => item)
}

// What a key does while the menu is open, with `highlighted` the row
// highlighted, if one is, of `count` rows: Up and Down move the highlight,
// going round at either end; Enter or Tab picks the highlighted row, or the
// first when none is; Escape closes the menu. Any other key, and Enter or a
// move with no rows, is left to the box.
export type MenuMove = { highlight: number } | { pick: number } | { close: true }
export function menuKey(key: string, highlighted: number | undefined, count: number): MenuMove | undefined {
  if (key === "Escape") return { close: true }
  if (count === 0) return undefined
  if (key === "ArrowDown") return { highlight: highlighted === undefined ? 0 : (highlighted + 1) % count }
  if (key === "ArrowUp") return { highlight: highlighted === undefined ? count - 1 : (highlighted - 1 + count) % count }
  if (key === "Enter" || key === "Tab") return { pick: highlighted ?? 0 }
  return undefined
}

// The command a message is, when it is a slash and a plain word, such as
// /model, with what follows it: run at once and not sent to the model, as
// the terminal reads a line. A path such as /Users/me/file is not a plain
// word, so a message may start with one.
export function commandIn(message: string): { name: string; args: string } | undefined {
  const command = /^\/([A-Za-z][\w-]*)(?:\s+([\s\S]*))?$/.exec(message.trim())
  return command ? { name: command[1]!, args: command[2] ?? "" } : undefined
}

// The owner's earlier messages in the session, the earliest first, to bring
// back into the box: the same message sent twice running is one.
export function earlierMessages(sent: readonly string[]): string[] {
  return sent.filter((text, at) => text.trim() !== "" && text !== sent[at - 1])
}

// Which earlier message is in the box, by its place among them, and its text.
export type Recalled = { at: number; text: string } | undefined

// Brings back the message before, with Up, or after, with Down, the one in
// the box: the latest, with Up in an empty box; an empty box again, with
// Down past the latest. Once the owner has typed, or changed what was
// brought back, the keys are the box's own, and nothing is brought back.
export function recallStep(direction: "back" | "forward", entries: readonly string[], recalled: Recalled, current: string): { recalled: Recalled; text: string } | undefined {
  const showing = recalled !== undefined && current === recalled.text && entries[recalled.at] === recalled.text ? recalled.at : undefined
  const at = (index: number) => ({ recalled: { at: index, text: entries[index]! }, text: entries[index]! })
  if (direction === "back") {
    if (showing !== undefined) return at(Math.max(0, showing - 1))
    return current === "" && entries.length > 0 ? at(entries.length - 1) : undefined
  }
  if (showing === undefined) return undefined
  return showing + 1 < entries.length ? at(showing + 1) : { recalled: undefined, text: "" }
}

// What the context meter beside the model's name says: how full the
// conversation is, as a share of the size at which the loop compacts it by
// itself, which is 100%; and both sizes in tokens, in short. It can pass
// 100% for a moment, until the next request compacts it.
export function contextMeter({ size, compactsAt }: { size: number; compactsAt: number }): { percent: string; size: string; compactsAt: string } {
  const percent = compactsAt > 0 ? Math.round((size / compactsAt) * 100) : 0
  return { percent: `${percent}%`, size: tokens(size), compactsAt: tokens(compactsAt) }
}

// A number of tokens in short: 320, 1.6k, 45k, 2.4M.
function tokens(count: number): string {
  const short = (value: number, unit: string) => `${value.toFixed(1).replace(/\.0$/, "")}${unit}`
  if (count < 1000) return String(Math.round(count))
  if (count < 10_000) return short(count / 1000, "k")
  if (count < 1_000_000) return `${Math.round(count / 1000)}k`
  return short(count / 1_000_000, "M")
}

// ---- @ mentions ---------------------------------------------------------------------

// Where the @ the caret is in begins, and what has been typed after it up
// to the caret, when the caret is in a word that starts with one: at the
// start of the box, or after a space or a new line. An @ inside a word,
// as in an email address, opens no menu.
export function mentionAt(text: string, caret: number): { from: number; query: string } | undefined {
  const before = text.slice(0, caret)
  const from = Math.max(before.lastIndexOf(" "), before.lastIndexOf("\n"), before.lastIndexOf("\t")) + 1
  if (before[from] !== "@") return undefined
  return { from, query: before.slice(from + 1) }
}

// How many files the menu shows at once: the owner types more to narrow it.
const FILES_SHOWN = 50

// The project's files that have what was typed, in any case, ranked as
// the commands are: an exact name or path first, then a name or a path
// that starts with it, then one with a word in its path that starts with
// it, then one with it anywhere in its path. Within each, the shorter path
// first, then the order they were listed in. With nothing typed, the first
// of them as listed.
export function rankFiles(paths: readonly string[], typed: string): string[] {
  const query = typed.trim().toLowerCase()
  if (query === "") return paths.slice(0, FILES_SHOWN)
  const tierOf = (path: string): number | undefined => {
    const full = path.toLowerCase()
    const name = full.slice(full.lastIndexOf("/") + 1)
    if (name === query || full === query) return 0
    if (name.startsWith(query) || full.startsWith(query)) return 1
    if (full.split(/[/._ -]/).some((word) => word.startsWith(query))) return 2
    if (full.includes(query)) return 3
    return undefined
  }
  return paths
    .flatMap((path, at) => {
      const tier = tierOf(path)
      return tier === undefined ? [] : [{ path, tier, at }]
    })
    .sort((a, b) => a.tier - b.tier || a.path.length - b.path.length || a.at - b.at)
    .slice(0, FILES_SHOWN)
    .map(({ path }) => path)
}

// ---- The text, with its files as chips --------------------------------------------------

// What is in the box: words, and the files named in them, each drawn as a
// chip in the text, as T3 Code's mention chips are, and sent as an @ and
// its path, as `@src/app.ts`. Nothing else is sent for one: evie reads the
// file with its tools.
export type Part = { text: string } | { file: string }

// A chip, as the caret and the menus count it: one character, the one
// that stands for an object in text, which no command or @ word can have.
export const CHIP = "\uFFFC"

// The message the box sends.
export const textOf = (parts: readonly Part[]): string => parts.map((part) => ("file" in part ? `@${part.file}` : part.text)).join("")

// The box as the caret and the menus read it, each chip one character.
export const shownOf = (parts: readonly Part[]): string => parts.map((part) => ("file" in part ? CHIP : part.text)).join("")

// Text put in the box, as a message brought back with Up: an @ at the start
// of a word, with what follows it up to a space or the end, is a chip, as
// T3 Code reads its prompt's mentions.
export function partsOf(text: string): Part[] {
  const parts: Part[] = []
  let at = 0
  for (const found of text.matchAll(/(^|\s)@([^\s@"]+)(?=\s|$)/g)) {
    const start = found.index + found[1]!.length
    parts.push({ text: text.slice(at, start) }, { file: found[2]! })
    at = start + 1 + found[2]!.length
  }
  parts.push({ text: text.slice(at) })
  return tidied(parts)
}

// The box with the stretch from `from` to `to`, counted as `shownOf` counts,
// replaced by `put`, and where the caret goes: after what was put.
export function replaced(parts: readonly Part[], from: number, to: number, put: readonly Part[]): { parts: Part[]; caret: number } {
  const before: Part[] = []
  const after: Part[] = []
  let at = 0
  for (const part of parts) {
    const length = "file" in part ? 1 : part.text.length
    if ("file" in part) {
      if (at + 1 <= from) before.push(part)
      else if (at >= to) after.push(part)
    } else {
      if (at < from) before.push({ text: part.text.slice(0, from - at) })
      if (at + length > to) after.push({ text: part.text.slice(Math.max(0, to - at)) })
    }
    at += length
  }
  const head = tidied([...before, ...put])
  return { parts: tidied([...head, ...after]), caret: shownOf(head).length }
}

// The box once a file is picked from the @ menu: its chip in place of the
// @ and what was typed after it, with a space after it, and the caret
// after the space.
export function pickFile(parts: readonly Part[], mention: { from: number; query: string }, caret: number, path: string): { parts: Part[]; caret: number } {
  const after = shownOf(parts).slice(caret)
  const end = after.startsWith(" ") ? caret + 1 : caret
  return replaced(parts, mention.from, end, [{ file: path }, { text: " " }])
}

// The stretch from `from` to `to`, counted as `shownOf` counts, as parts:
// what is copied or cut, which as text says its files as @ and their paths,
// so that pasted back, they are chips again.
export function sliceOf(parts: readonly Part[], from: number, to: number): Part[] {
  const out: Part[] = []
  let at = 0
  for (const part of parts) {
    if ("file" in part) {
      if (at >= from && at < to) out.push(part)
      at += 1
    } else {
      const piece = part.text.slice(Math.max(0, from - at), Math.max(0, to - at))
      if (piece !== "") out.push({ text: piece })
      at += part.text.length
    }
  }
  return tidied(out)
}

// The box at a moment: its parts and where its caret is.
export type Snapshot = { parts: Part[]; caret: number }

// The box's own undo history, since the browser's cannot undo a chip put
// in: the box as it was before each change, and as it was before each
// undo, for Redo; and the last change, so that typing runs into one step.
export type History = { past: Snapshot[]; future: Snapshot[]; last?: { kind: Change; at: number } }
export type Change = "typing" | "deleting" | "paste" | "cut" | "drop" | "line" | "chip" | "command"
export const NO_HISTORY: History = { past: [], future: [] }

// How long after the last key typing or deleting still runs into its step,
// in milliseconds, and how many steps are kept.
const TYPING_RUNS = 1000
const STEPS = 100

// The history with the box as it was before a change of `kind`, at `at`.
// A change forgets what was undone.
export function recorded(history: History, before: Snapshot, kind: Change, at: number): History {
  const runsOn = (kind === "typing" || kind === "deleting") && history.last?.kind === kind && at - history.last.at < TYPING_RUNS
  return { past: runsOn ? history.past : [...history.past, before].slice(-STEPS), future: [], last: { kind, at } }
}

// The box one step back, with `now` kept for Redo, or nothing to undo.
export function undone(history: History, now: Snapshot): { history: History; to: Snapshot } | undefined {
  const to = history.past.at(-1)
  return to === undefined ? undefined : { history: { past: history.past.slice(0, -1), future: [...history.future, now] }, to }
}

// The box one step forward again, or nothing to redo.
export function redone(history: History, now: Snapshot): { history: History; to: Snapshot } | undefined {
  const to = history.future.at(-1)
  return to === undefined ? undefined : { history: { past: [...history.past, now], future: history.future.slice(0, -1) }, to }
}

// Parts with no empty words, and words side by side as one.
function tidied(parts: readonly Part[]): Part[] {
  const out: Part[] = []
  for (const part of parts) {
    const last = out.at(-1)
    if ("text" in part && part.text === "") continue
    if ("text" in part && last !== undefined && "text" in last) out[out.length - 1] = { text: last.text + part.text }
    else out.push(part)
  }
  return out
}

// ---- Attachments ----------------------------------------------------------------------

// The files that can be attached, and why the others cannot, in words: a
// file larger than the limit is not, as the server would refuse it.
export function attachable<File extends { name: string; size: number }>(files: readonly File[]): { taken: File[]; problem: string | undefined } {
  const large = files.filter((file) => file.size > ATTACHMENT_LIMIT)
  const named = large.map((file) => file.name)
  const list = named.length > 1 ? `${named.slice(0, -1).join(", ")} and ${named.at(-1)}` : named[0]
  return {
    taken: files.filter((file) => file.size <= ATTACHMENT_LIMIT),
    problem: large.length === 0 ? undefined : `${list} ${large.length === 1 ? "is" : "are"} larger than the ${ATTACHMENT_LIMIT / 1024 / 1024} MB a file attached can be.`,
  }
}

// ---- The box's one action -------------------------------------------------------------

// What the round button at the box's end does, as T3 Code's: Send, off
// while there is nothing to send; while evie works with nothing to send,
// Stop, in its place; with something, steering the turn, or with mod held,
// queueing it; while a queued message is edited, saving it. For a moment
// after a send, it stays Send, off, as T3 Code's guard keeps it, so that a
// second click, as the box empties and evie works, stops nothing.
export function primaryOf(box: { editing: boolean; working: boolean; empty: boolean; modHeld: boolean; justSent: boolean }): { does: "send" | "steer" | "queue" | "stop" | "update"; disabled: boolean } {
  if (box.editing) return { does: "update", disabled: false }
  if (box.justSent && box.empty) return { does: "send", disabled: true }
  if (box.working) return box.empty ? { does: "stop", disabled: false } : { does: box.modHeld ? "queue" : "steer", disabled: false }
  return { does: "send", disabled: box.empty }
}

// Whether a key is one an input method takes, as the Enter that confirms
// what is being composed in Japanese or Chinese: while composing, or as
// Safari says it, with keyCode 229, or just after the composition ended,
// which Safari tells before the key.
export const confirmsInput = (key: { isComposing: boolean; keyCode: number }, endedAt: number | undefined, now: number): boolean =>
  key.isComposing || key.keyCode === 229 || (endedAt !== undefined && now - endedAt < COMPOSITION_ENDS)
const COMPOSITION_ENDS = 100
