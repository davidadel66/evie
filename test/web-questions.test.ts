import { expect, test } from "bun:test"
import type { EventBodies } from "../src/core/types.ts"
import { answerTyped, choiceForKey, sendDoes, freshGuard, guardCleared, guardReleases, guardSees, guardTyped, permissionChoices, releaseAt, shownQuestion } from "../web/src/questions.ts"

// The questions evie asks, as the message box shows them, apart from its
// screen: when one shows, and where.

test("a question that arrives while the owner is typing is held until they have not typed for a second and a half", () => {
  let guard = guardTyped(freshGuard(), 10_000)
  guard = guardSees(guard, ["q1"], 10_500)
  expect([...guard.held]).toEqual(["q1"])
  expect(releaseAt(guard)).toBe(11_500)

  // Another key puts it off again.
  guard = guardTyped(guard, 11_000)
  expect(releaseAt(guard)).toBe(12_500)
  expect([...guardReleases(guard, 12_499).held]).toEqual(["q1"])
  guard = guardReleases(guard, 12_500)
  expect([...guard.held]).toEqual([])
  expect(releaseAt(guard)).toBeUndefined()

  // A question that arrives once they have stopped shows at once, and so
  // does one that was already showing when they started typing again.
  guard = guardSees(guard, ["q1", "q2"], 14_000)
  guard = guardTyped(guard, 14_100)
  guard = guardSees(guard, ["q1", "q2"], 14_200)
  expect([...guard.held]).toEqual([])
})

test("sending, or leaving the box, shows the questions held at once", () => {
  const held = guardSees(guardTyped(freshGuard(), 1000), ["q1"], 1200)
  expect([...held.held]).toEqual(["q1"])
  expect([...guardCleared(held).held]).toEqual([])
  // The keys typed before then no longer hold a question that arrives after.
  expect([...guardSees(guardCleared(held), ["q1", "q2"], 1300).held]).toEqual([])
})

test("a question answered while held is let go", () => {
  const held = guardSees(guardTyped(freshGuard(), 1000), ["q1", "q2"], 1200)
  expect([...guardSees(held, ["q2"], 1300).held]).toEqual(["q2"])
})

type Asked = EventBodies["question"]
const asked = (id: string, more: Partial<Asked> = {}): Asked => ({ id, text: `Question ${id}?`, choices: ["North bed", "South bed"], ...more })
const permission = (id: string): Asked => ({ id, text: "Run this command?\n\n  git push", choices: ["Yes, once", "Always in this project: allow git push", "No"], freeText: true, kind: "permission" })

test("one question shows at a time, the oldest waiting first: a permission prompt in place in the conversation, any other docked above the box, with how many wait", () => {
  expect(shownQuestion([], new Set())).toBeUndefined()
  expect(shownQuestion([asked("q1")], new Set())).toEqual({ question: asked("q1"), place: "docked", count: undefined })
  expect(shownQuestion([permission("p1"), asked("q2"), asked("q3")], new Set())).toEqual({ question: permission("p1"), place: "inline", count: "1/3" })
  // One held while the owner types is passed over until it is let go.
  expect(shownQuestion([asked("q1"), asked("q2")], new Set(["q1"]))).toEqual({ question: asked("q2"), place: "docked", count: "2/2" })
  expect(shownQuestion([asked("q1")], new Set(["q1"]))).toBeUndefined()
})

test("a permission prompt's choices: the one that refuses, its last, first; the one that allows, its first, as the main one; any others under More", () => {
  expect(permissionChoices(permission("p1"))).toEqual({ deny: "No", allow: "Yes, once", more: ["Always in this project: allow git push"] })
  expect(permissionChoices({ id: "t", text: "Use it?", choices: ["Yes, trust this project", "No, ignore its settings file"], kind: "permission" })).toEqual({
    deny: "No, ignore its settings file",
    allow: "Yes, trust this project",
    more: [],
  })
})

test("what is sent from the box while a question waits answers it as the terminal takes a typed line: a number picks that choice, and other words are the answer when it takes them", () => {
  expect(answerTyped("2", asked("q1"))).toBe("South bed")
  expect(answerTyped(" 1 ", asked("q1", { freeText: true }))).toBe("North bed")
  expect(answerTyped("Both, north first", asked("q1", { freeText: true }))).toBe("Both, north first")
  // Words a question does not take, a number it has no choice for, and nothing at all are no answer.
  expect(answerTyped("Both", asked("q1"))).toBeUndefined()
  expect(answerTyped("3", asked("q1", { freeText: true }))).toBeUndefined()
  expect(answerTyped("  ", asked("q1", { freeText: true }))).toBeUndefined()
  // Words sent while a permission prompt waits refuse it, and say what to do instead.
  expect(answerTyped("Use the staging branch", permission("p1"))).toBe("Use the staging branch")
})

test("the keys 1 to 9 pick the choice with that number", () => {
  expect(choiceForKey("1", asked("q1"))).toBe("North bed")
  expect(choiceForKey("2", asked("q1"))).toBe("South bed")
  expect(choiceForKey("3", asked("q1"))).toBeUndefined()
  expect(choiceForKey("0", asked("q1"))).toBeUndefined()
  expect(choiceForKey("a", asked("q1"))).toBeUndefined()
})

test("sending from the box answers the question showing when the words answer it, as the terminal takes a line; otherwise a slash and a word runs a command, and anything else is a message", () => {
  expect(sendDoes("  Plan the beds  ", undefined)).toEqual({ message: "Plan the beds" })
  expect(sendDoes("/model some/model", undefined)).toEqual({ run: { name: "model", args: "some/model" } })
  expect(sendDoes("2", asked("q1"))).toEqual({ answer: "South bed" })
  expect(sendDoes("Neither, the herbs", asked("q1", { freeText: true }))).toEqual({ answer: "Neither, the herbs" })
  // Words the question does not take are a message, for the turn to read once it is answered.
  expect(sendDoes("Hold on", asked("q1"))).toEqual({ message: "Hold on" })
  expect(sendDoes("/compact", asked("q1"))).toEqual({ run: { name: "compact", args: "" } })
  expect(sendDoes("   ", undefined)).toBeUndefined()
})
