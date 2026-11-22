import { expect, test } from "bun:test"
import { commandIn, contextMeter, earlierMessages, menuKey, rankCommands, recallStep, slashAt } from "../web/src/composer.ts"

// The message box's rules, apart from its screen: the menu a slash opens,
// and how it is filtered and ranked as the owner types.

const COMMANDS = [
  { name: "skills", description: "List your skills." },
  { name: "mode", description: "Show or change how freely evie may act." },
  { name: "model", description: "Show the model in use, or switch to the one named." },
  { name: "compact", description: "Replace the older part of the conversation with a summary, to save room." },
  { name: "jobs", description: "List the background jobs." },
  { name: "release-notes", description: "Write the release notes.", kind: "skill" },
  { name: "unmodelled", description: "A skill with model in its name.", kind: "skill" },
]
const names = (items: { name: string }[]) => items.map((item) => item.name)

test("a slash at the start of a word, up to the caret, opens the menu with what follows it, and anywhere else it does not", () => {
  expect(slashAt("/", 1)).toEqual({ from: 0, query: "", first: true })
  expect(slashAt("/mo", 3)).toEqual({ from: 0, query: "mo", first: true })
  expect(slashAt("Use /mo", 7)).toEqual({ from: 4, query: "mo", first: false })
  expect(slashAt("line one\n/co", 12)).toEqual({ from: 9, query: "co", first: false })
  // Once the word has ended, or the caret is before the slash, the menu is closed.
  expect(slashAt("/model ", 7)).toBeUndefined()
  expect(slashAt("/model", 0)).toBeUndefined()
  // A path, or a slash inside a word, is not a command.
  expect(slashAt("src/app", 7)).toBeUndefined()
  expect(slashAt("/Users/me", 9)).toBeUndefined()
  expect(slashAt("", 0)).toBeUndefined()
})

test("the commands are ranked as typed: an exact name, then the start of a name, the start of a word in it, anywhere in it, then the description", () => {
  expect(names(rankCommands(COMMANDS, ""))).toEqual(names(COMMANDS))
  expect(names(rankCommands(COMMANDS, "mode"))).toEqual(["mode", "model", "unmodelled"])
  expect(names(rankCommands(COMMANDS, "MOD"))).toEqual(["mode", "model", "unmodelled"])
  // The start of a word in a name wins over the same letters inside a word.
  expect(names(rankCommands([{ name: "footnotes", description: "" }, ...COMMANDS], "notes"))).toEqual(["release-notes", "footnotes"])
  expect(names(rankCommands(COMMANDS, "summary"))).toEqual(["compact"])
  expect(rankCommands(COMMANDS, "zzz")).toEqual([])
})

test("in the menu, Up and Down move the highlight round, Enter or Tab picks it, or the first row when none is, and Escape closes it", () => {
  expect(menuKey("ArrowDown", undefined, 3)).toEqual({ highlight: 0 })
  expect(menuKey("ArrowDown", 0, 3)).toEqual({ highlight: 1 })
  expect(menuKey("ArrowDown", 2, 3)).toEqual({ highlight: 0 })
  expect(menuKey("ArrowUp", undefined, 3)).toEqual({ highlight: 2 })
  expect(menuKey("ArrowUp", 0, 3)).toEqual({ highlight: 2 })
  expect(menuKey("Enter", 1, 3)).toEqual({ pick: 1 })
  expect(menuKey("Tab", undefined, 3)).toEqual({ pick: 0 })
  expect(menuKey("Escape", 1, 3)).toEqual({ close: true })
  // Other keys are the box's own, such as a letter typed to narrow the menu.
  expect(menuKey("m", 1, 3)).toBeUndefined()
  // With no matching command, nothing can be picked or moved to, and Enter is the box's: it sends.
  expect(menuKey("ArrowDown", undefined, 0)).toBeUndefined()
  expect(menuKey("Enter", undefined, 0)).toBeUndefined()
  expect(menuKey("Escape", undefined, 0)).toEqual({ close: true })
})

test("a message that is a slash and a plain word is a command, with what follows it, as the terminal reads one; a path is a message", () => {
  expect(commandIn("/compact")).toEqual({ name: "compact", args: "" })
  expect(commandIn("  /model some/model  ")).toEqual({ name: "model", args: "some/model" })
  expect(commandIn("/release-notes for the\nspring release")).toEqual({ name: "release-notes", args: "for the\nspring release" })
  expect(commandIn("/Users/me/notes.md says what?")).toBeUndefined()
  expect(commandIn("/2fa")).toBeUndefined()
  expect(commandIn("Run /compact")).toBeUndefined()
  expect(commandIn("/")).toBeUndefined()
})

test("Up in an empty box brings back the owner's earlier messages, the latest first, and Down goes forward again, to an empty box after the latest", () => {
  // The same message sent twice running is brought back once.
  const entries = earlierMessages(["Plan the beds", "Water them", "Water them", "Weed them"])
  expect(entries).toEqual(["Plan the beds", "Water them", "Weed them"])

  const first = recallStep("back", entries, undefined, "")!
  expect(first.text).toBe("Weed them")
  const second = recallStep("back", entries, first.recalled, first.text)!
  expect(second.text).toBe("Water them")
  const third = recallStep("back", entries, second.recalled, second.text)!
  expect(third.text).toBe("Plan the beds")
  // At the earliest, Up stays there.
  expect(recallStep("back", entries, third.recalled, third.text)).toEqual(third)

  const forward = recallStep("forward", entries, third.recalled, third.text)!
  expect(forward.text).toBe("Water them")
  const latest = recallStep("forward", entries, recallStep("forward", entries, forward.recalled, forward.text)!.recalled, "Weed them")!
  expect(latest).toEqual({ recalled: undefined, text: "" })
})

test("what the owner typed is never replaced: Up and Down are the box's own once the text is not empty or not what was brought back", () => {
  const entries = ["Plan the beds", "Weed them"]
  expect(recallStep("back", entries, undefined, "A new message")).toBeUndefined()
  expect(recallStep("forward", entries, undefined, "")).toBeUndefined()
  const recalled = recallStep("back", entries, undefined, "")!
  expect(recallStep("back", entries, recalled.recalled, "Weed them, then water")).toBeUndefined()
  expect(recallStep("back", [], undefined, "")).toBeUndefined()
})

test("the context meter says how full the conversation is as a share of the size the loop compacts it at, which is 100%, with both sizes in words", () => {
  expect(contextMeter({ size: 0, compactsAt: 160_000 })).toEqual({ percent: "0%", size: "0", compactsAt: "160k" })
  expect(contextMeter({ size: 45_200, compactsAt: 160_000 })).toEqual({ percent: "28%", size: "45k", compactsAt: "160k" })
  expect(contextMeter({ size: 999, compactsAt: 1000 })).toEqual({ percent: "100%", size: "999", compactsAt: "1k" })
  expect(contextMeter({ size: 1_550, compactsAt: 200_000 })).toEqual({ percent: "1%", size: "1.6k", compactsAt: "200k" })
  // A conversation can be past it until its next request compacts it.
  expect(contextMeter({ size: 170_000, compactsAt: 160_000 })).toEqual({ percent: "106%", size: "170k", compactsAt: "160k" })
  expect(contextMeter({ size: 2_400_000, compactsAt: 0 })).toEqual({ percent: "0%", size: "2.4M", compactsAt: "0" })
})
