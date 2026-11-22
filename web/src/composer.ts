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

// The box once a file is picked: an @ and its path in place of what was
// typed, with a space after it, and where the caret goes, after the space.
export function putMention(text: string, mention: { from: number; query: string }, caret: number, path: string): { text: string; caret: number } {
  const filled = `${text.slice(0, mention.from)}@${path} `
  return { text: `${filled}${text.slice(caret).replace(/^ /, "")}`, caret: filled.length }
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

// A file's size in short: 4 bytes, 120 KB, 2.1 MB.
export function fileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} byte${bytes === 1 ? "" : "s"}`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1).replace(/\.0$/, "")} MB`
}
