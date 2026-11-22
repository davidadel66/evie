// The message box's rules, apart from its screen: the menu a slash opens,
// how it is ranked and moved through, what a message that is a command
// says, and bringing back the owner's earlier messages. They follow T3
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

// The items whose name or description has what was typed, in any case:
// an exact name first, then a name that starts with it, then one with a
// word that starts with it, then one with it anywhere, and then those
// whose description has it. Within each, the shorter name first, then the
// order they were listed in. With nothing typed, all of them as listed.
export function rankCommands<Item extends MenuItem>(items: readonly Item[], typed: string): Item[] {
  const query = typed.trim().toLowerCase()
  if (query === "") return [...items]
  const tierOf = (item: Item): number | undefined => {
    const name = item.name.toLowerCase()
    if (name === query) return 0
    if (name.startsWith(query)) return 1
    if (name.split(/[-_]/).slice(1).some((word) => word.startsWith(query))) return 2
    if (name.includes(query)) return 3
    if (item.description.toLowerCase().includes(query)) return 4
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
