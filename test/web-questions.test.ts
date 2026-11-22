import { expect, test } from "bun:test"
import type { EventBodies } from "../src/core/types.ts"
import type { Item } from "../web/src/transcript.ts"
import { answerTyped, asAsked, boxPlaceholder, choiceForKey, sendDoes, sendWithFiles, freshGuard, guardCleared, guardReleases, guardSees, guardTyped, refusingChoice, releaseAt, shownQuestion } from "../web/src/questions.ts"

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

test("one question shows at a time, in the conversation, a permission prompt as any other, the oldest waiting first, with how many wait", () => {
  expect(shownQuestion([], new Set())).toBeUndefined()
  expect(shownQuestion([asked("q1")], new Set())).toEqual({ question: asked("q1"), count: undefined })
  expect(shownQuestion([permission("p1"), asked("q2"), asked("q3")], new Set())).toEqual({ question: permission("p1"), count: "1/3" })
  // One held while the owner types is passed over until it is let go.
  expect(shownQuestion([asked("q1"), asked("q2")], new Set(["q1"]))).toEqual({ question: asked("q2"), count: "2/2" })
  expect(shownQuestion([asked("q1")], new Set(["q1"]))).toBeUndefined()
})

test("a permission prompt is refused with its last choice", () => {
  expect(refusingChoice(permission("p1"))).toBe("No")
  expect(refusingChoice({ id: "t", text: "Use it?", choices: ["Yes, trust this project", "No, ignore its settings file"], kind: "permission" })).toBe("No, ignore its settings file")
})

test("what is sent from the box while a question waits answers it as the terminal takes a typed line: a number picks that choice, and other words are the answer when it takes them", () => {
  expect(answerTyped("2", asked("q1"))).toBe("South bed")
  expect(answerTyped(" 1 ", asked("q1", { freeText: true }))).toBe("North bed")
  expect(answerTyped("Both, north first", asked("q1", { freeText: true }))).toBe("Both, north first")
  // Words a question does not take, a number it has no choice for, and nothing at all are no answer.
  expect(answerTyped("Both", asked("q1"))).toBeUndefined()
  expect(answerTyped("3", asked("q1", { freeText: true }))).toBeUndefined()
  expect(answerTyped("  ", asked("q1", { freeText: true }))).toBeUndefined()
})

test("the keys 1 to 9 pick the choice with that number, while the focus is on nothing or in the message box with nothing typed in it", () => {
  expect(choiceForKey("1", asked("q1"), "nothing")).toBe("North bed")
  expect(choiceForKey("2", asked("q1"), "empty box")).toBe("South bed")
  expect(choiceForKey("3", asked("q1"), "nothing")).toBeUndefined()
  expect(choiceForKey("0", asked("q1"), "nothing")).toBeUndefined()
  expect(choiceForKey("a", asked("q1"), "nothing")).toBeUndefined()
  // Never on a button or a row of the work log, nor in a box with something typed.
  expect(choiceForKey("1", asked("q1"), "elsewhere")).toBeUndefined()
  expect(choiceForKey("1", asked("q1"), "typed box")).toBeUndefined()
  // Nor for a permission prompt, which only a click answers.
  expect(choiceForKey("1", permission("p1"), "nothing")).toBeUndefined()
})

test("sending from the box answers the question showing when the words answer it, as the terminal takes a line; otherwise a slash and a word runs a command, and anything else is a message", () => {
  expect(sendDoes("  Plan the beds  ", undefined)).toEqual({ message: "Plan the beds" })
  expect(sendDoes("/model some/model", undefined)).toEqual({ run: { name: "model", args: "some/model" } })
  expect(sendDoes("2", asked("q1"))).toEqual({ answer: "South bed" })
  expect(sendDoes("Neither, the herbs", asked("q1", { freeText: true }))).toEqual({ answer: "Neither, the herbs" })
  expect(sendDoes("/compact", asked("q1"))).toEqual({ run: { name: "compact", args: "" } })
  expect(sendDoes("   ", undefined)).toBeUndefined()
})

test("while a question with choices only waits, words or a number it has no choice for are not sent: the owner is asked to pick one, as the terminal asks again", () => {
  expect(sendDoes("Hold on", asked("q1"))).toEqual({ refuse: "Pick one of the choices above." })
  expect(sendDoes("3", asked("q1"))).toEqual({ refuse: "Pick one of the choices above." })
  expect(sendDoes("3", asked("q1", { freeText: true }))).toEqual({ refuse: "Pick one of the choices above, or write your own answer." })
})

test("while a permission prompt waits, words sent decline it with its refusing choice and go to evie as a message, so that it does that instead, as T3 Code's do", () => {
  expect(sendDoes("Use the staging branch", permission("p1"))).toEqual({ decline: "No", message: "Use the staging branch" })
  // A number is a message here too: the prompt's choices are not numbered.
  expect(sendDoes("1", permission("p1"))).toEqual({ decline: "No", message: "1" })
  expect(sendDoes("/mode auto", permission("p1"))).toEqual({ run: { name: "mode", args: "auto" } })
})

test("files attached go with a message, which then needs no words; a command or an answer does not take them, and the box says so", () => {
  expect(sendWithFiles("What is this?", undefined, 1)).toEqual({ message: "What is this?" })
  expect(sendWithFiles("  ", undefined, 2)).toEqual({ message: "" })
  expect(sendWithFiles("", permission("p1"), 1)).toEqual({ decline: "No", message: "" })
  expect(sendWithFiles("Use this one", permission("p1"), 1)).toEqual({ decline: "No", message: "Use this one" })
  const notTaken = { refuse: "Files attached go with a message, not with a command or an answer. Remove them to send this." }
  expect(sendWithFiles("/compact", undefined, 1)).toEqual(notTaken)
  expect(sendWithFiles("2", asked("q1"), 1)).toEqual(notTaken)
  expect(sendWithFiles("", asked("q1"), 1)).toEqual({ refuse: "Answer the question first. Files attached go with a message." })
  expect(sendWithFiles("Hold on", asked("q1"), 1)).toEqual({ refuse: "Pick one of the choices above." })
  // With none attached, sending does what it always does.
  expect(sendWithFiles("   ", undefined, 0)).toBeUndefined()
  expect(sendWithFiles("/compact", undefined, 0)).toEqual({ run: { name: "compact", args: "" } })
})

test("a question waiting is left out where it was asked, since it shows where it is answered; one with no answer that no longer waits, as after a restart, is drawn as asked", () => {
  const items: Item[] = [
    { key: "e1", turn: 1, kind: "owner", text: "Plan the beds" },
    { key: "e2", turn: 1, kind: "question", id: "q1", text: "Which bed first?", choices: ["North", "South"], waiting: true },
    { key: "e3", turn: 2, kind: "question", id: "q2", text: "And then?", choices: ["Herbs"], waiting: true },
  ]
  expect(asAsked(items, [asked("q2")])).toEqual([items[0]!, { ...items[1]!, waiting: false } as Item])
  expect(asAsked(items, [])).toEqual([items[0]!, { ...items[1]!, waiting: false } as Item, { ...items[2]!, waiting: false } as Item])
})

test("the message box's placeholder says what sending does: with a permission prompt waiting in a session, says no and what to do instead; with a question that takes words, answers it; otherwise as the session stands", () => {
  const session = { session: true, working: false, mentions: true }
  expect(boxPlaceholder(permission("p1"), session)).toBe("Send a message to say no, and what to do instead")
  expect(boxPlaceholder(asked("q1", { freeText: true }), session)).toBe("Your own answer")
  expect(boxPlaceholder(asked("q1"), session)).toBe("Ask anything. @ files, / commands")
  expect(boxPlaceholder(undefined, { ...session, working: true })).toBe("Tell evie more as it works. Escape stops it")
  expect(boxPlaceholder(undefined, { ...session, mentions: false })).toBe("Ask anything. / commands")
  // A draft runs no commands until it has started, and a permission prompt there is answered by a click.
  expect(boxPlaceholder(permission("p1"), { session: false, working: false, mentions: true })).toBe("Ask anything. @ files")
  expect(boxPlaceholder(undefined, { session: false, working: false, mentions: false })).toBe("Ask anything")
})

test("a message of more than 1 MB of text is not sent: it stays in the box, which says to attach it as a file", () => {
  const refused = { refuse: "This message is longer than the 1 MB a message can hold. Attach the text as a file instead: a file attached can be up to 10 MB, and evie reads it from there." }

  expect(sendWithFiles("x".repeat(1024 * 1024 + 1), undefined, 0)).toEqual(refused)
  // Two bytes a letter, so half as many letters are too many.
  expect(sendWithFiles("é".repeat(512 * 1024 + 1), undefined, 1)).toEqual(refused)
  expect(sendWithFiles("é".repeat(512 * 1024), undefined, 0)).toEqual({ message: "é".repeat(512 * 1024) })
})
