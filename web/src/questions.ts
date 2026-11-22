import type { EventBodies } from "../../src/core/types.ts"
import { commandIn } from "./composer.ts"
import type { Item } from "./transcript.ts"

// The questions evie asks, apart from their screen: when one shows, and
// what the message box does while one waits. They show in the
// conversation, as the Session view drawing has them, and the box works
// with them as T3 Code's does.

export type Asked = EventBodies["question"]

// How long the owner must not have typed before a question that arrived
// while they were typing is shown, so that a stray key cannot answer it.
export const TYPING_IDLE = 1500

// The guard against that: when the owner last typed, the questions it has
// seen, and those it holds back.
export type Guard = { lastKey: number; seen: ReadonlySet<string>; held: ReadonlySet<string> }

export const freshGuard = (): Guard => ({ lastKey: -Infinity, seen: new Set(), held: new Set() })

// The owner typed a key.
export const guardTyped = (guard: Guard, now: number): Guard => ({ ...guard, lastKey: now })

// The questions waiting now, by id: one not seen before that arrives while
// the owner is typing is held. One no longer waiting is let go.
export function guardSees(guard: Guard, waiting: readonly string[], now: number): Guard {
  const typing = now - guard.lastKey < TYPING_IDLE
  const fresh = waiting.filter((id) => !guard.seen.has(id))
  const held = new Set([...guard.held].filter((id) => waiting.includes(id)))
  if (typing) for (const id of fresh) held.add(id)
  return { ...guard, seen: new Set([...guard.seen, ...waiting]), held }
}

// When the questions held can be shown, if any are held.
export const releaseAt = (guard: Guard): number | undefined => (guard.held.size === 0 ? undefined : guard.lastKey + TYPING_IDLE)

// The questions held are shown once the owner has not typed for long enough.
export function guardReleases(guard: Guard, now: number): Guard {
  const at = releaseAt(guard)
  return at !== undefined && now >= at ? { ...guard, held: new Set() } : guard
}

// The owner sent what they typed, or left the box: what is held is shown,
// and the keys typed before no longer count.
export const guardCleared = (guard: Guard): Guard => ({ ...guard, lastKey: -Infinity, held: new Set() })

// The question to show now: the oldest of those waiting that is not held.
// It shows at the end of the conversation, a permission prompt as any
// other. With more than one waiting, `count` says which this is of how
// many, as "1/3".
export function shownQuestion(waiting: readonly Asked[], held: ReadonlySet<string>): { question: Asked; count: string | undefined } | undefined {
  const at = waiting.findIndex((question) => !held.has(question.id))
  if (at === -1) return undefined
  return { question: waiting[at]!, count: waiting.length > 1 ? `${at + 1}/${waiting.length}` : undefined }
}

// The choice that refuses a permission prompt: its last. Its first allows.
export const refusingChoice = (question: Asked): string => question.choices.at(-1)!

// The answer a message sent from the box gives the question waiting, as the
// terminal takes a typed line: a number picks the choice with that number,
// and other words are the answer itself when the question takes the owner's
// own words, as a permission prompt does, where they refuse it and say what
// to do instead. Anything else is no answer.
export function answerTyped(typed: string, question: Asked): string | undefined {
  const text = typed.trim()
  if (/^\d+$/.test(text)) return question.choices[Number(text) - 1]
  return question.freeText && text !== "" ? text : undefined
}

// Where the focus is as a key is pressed: on nothing, in the message box
// with nothing typed in it, or with something typed, or anywhere else, as
// on a button or a row of the work log.
export type KeyFocus = "nothing" | "empty box" | "typed box" | "elsewhere"

// The choice a key from 1 to 9 picks, as T3 Code's keys do, though the
// choices show no numbers, as drawn: only with the focus on nothing or in
// the empty message box, so that a key meant for a button, a row or the
// text is never taken; and never for a permission prompt, which only a
// click answers.
export function choiceForKey(key: string, question: Asked, focus: KeyFocus): string | undefined {
  if (question.kind === "permission" || (focus !== "nothing" && focus !== "empty box") || !/^[1-9]$/.test(key)) return undefined
  return question.choices[Number(key) - 1]
}

// What sending from the box does, with the question showing, if one is.
// While a permission prompt waits, words decline it, with its refusing
// choice, and go to evie as a message, so that it does that instead, as
// T3 Code's "Send a message to decline and redirect" does. While another
// question waits, words that answer it, as the terminal takes a line,
// answer it; a number it has no choice for, or words a question with
// choices only does not take, are not sent, and the owner is asked to pick
// one, as the terminal asks again. Otherwise a slash and a word runs that
// command, and anything else is a message, which steers a turn in
// progress. Nothing, for a box with nothing in it.
export type SendDoes = { answer: string } | { decline: string; message: string } | { run: { name: string; args: string } } | { message: string } | { refuse: string }
export function sendDoes(typed: string, showing: Asked | undefined): SendDoes | undefined {
  const text = typed.trim()
  if (text === "") return undefined
  const command = commandIn(text)
  if (showing?.kind === "permission") return command ? { run: command } : { decline: refusingChoice(showing), message: text }
  if (showing !== undefined) {
    const answer = answerTyped(text, showing)
    if (answer !== undefined) return { answer }
    if (command) return { run: command }
    return { refuse: `Pick one of the choices above${showing.freeText ? ", or write your own answer" : ""}.` }
  }
  return command ? { run: command } : { message: text }
}

// What sending does with files attached: they go with a message, which
// then needs no words, or with words that decline a permission prompt. A
// command, or an answer to a question, takes no files, and neither does
// an empty box while a question waits to be answered: the box says so,
// and nothing is sent.
export function sendWithFiles(typed: string, showing: Asked | undefined, files: number): SendDoes | undefined {
  const does = sendDoes(typed, showing)
  if (files === 0) return does
  if (does === undefined) {
    if (showing?.kind === "permission") return { decline: refusingChoice(showing), message: "" }
    return showing === undefined ? { message: "" } : { refuse: "Answer the question first. Files attached go with a message." }
  }
  if ("message" in does || "refuse" in does) return does
  return { refuse: "Files attached go with a message, not with a command or an answer. Remove them to send this." }
}

// The conversation's items as the page draws them: a question the session
// is waiting on is left out where it was asked, since it shows where it is
// answered, at the end of the conversation. One the log has no answer
// to that the session no longer waits on, as one cut off by a restart, is
// drawn as it was asked, unanswered.
export function asAsked(items: readonly Item[], waiting: readonly Asked[]): Item[] {
  const open = new Set(waiting.map((question) => question.id))
  return items.flatMap((item) => (item.kind !== "question" || !item.waiting ? [item] : open.has(item.id) ? [] : [{ ...item, waiting: false }]))
}

// What the message box says while empty: what sending from it does now.
// With a permission prompt waiting in a session, words say no and what to
// do instead, as T3 Code's box says; with a question that takes the
// owner's words, they answer it; otherwise, what the box takes: what to
// tell evie as it works, or what to ask, and whether the project's files
// can be named with @ and, once the session has started, commands run
// with /.
export function boxPlaceholder(showing: Asked | undefined, box: { session: boolean; working: boolean; mentions: boolean }): string {
  if (showing?.kind === "permission" && box.session) return "Send a message to say no, and what to do instead"
  if (showing !== undefined && showing.kind !== "permission" && showing.freeText) return "Your own answer"
  if (box.working) return "Tell evie more as it works. Escape stops it"
  if (!box.session) return box.mentions ? "Ask anything. @ files" : "Ask anything"
  return box.mentions ? "Ask anything. @ files, / commands" : "Ask anything. / commands"
}
