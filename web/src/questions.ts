import type { EventBodies } from "../../src/core/types.ts"
import { commandIn } from "./composer.ts"
import type { Item } from "./transcript.ts"

// The questions evie asks, as the message box shows them, apart from its
// screen: when one shows, and where. They follow T3 Code's, which the
// owner chose as the model for evie's.

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
// A permission prompt shows in place, at the end of the conversation; any
// other question docks above the message box. With more than one waiting,
// `count` says which this is of how many, as "1/3".
export function shownQuestion(waiting: readonly Asked[], held: ReadonlySet<string>): { question: Asked; place: "inline" | "docked"; count: string | undefined } | undefined {
  const at = waiting.findIndex((question) => !held.has(question.id))
  if (at === -1) return undefined
  const question = waiting[at]!
  return { question, place: question.kind === "permission" ? "inline" : "docked", count: waiting.length > 1 ? `${at + 1}/${waiting.length}` : undefined }
}

// A permission prompt's choices, as its buttons show them: the one that
// refuses, which is its last; the one that allows, its first, which is the
// main one; and any others, which go under More.
export function permissionChoices(question: Asked): { deny: string; allow: string; more: string[] } {
  return { allow: question.choices[0]!, deny: question.choices.at(-1)!, more: question.choices.slice(1, -1) }
}

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

// The choice a key from 1 to 9 picks.
export const choiceForKey = (key: string, question: Asked): string | undefined => (/^[1-9]$/.test(key) ? question.choices[Number(key) - 1] : undefined)

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
  if (showing?.kind === "permission") return command ? { run: command } : { decline: permissionChoices(showing).deny, message: text }
  if (showing !== undefined) {
    const answer = answerTyped(text, showing)
    if (answer !== undefined) return { answer }
    if (command) return { run: command }
    return { refuse: `Pick one of the choices, 1 to ${showing.choices.length}${showing.freeText ? ", or write your own answer" : ""}.` }
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
    if (showing?.kind === "permission") return { decline: permissionChoices(showing).deny, message: "" }
    return showing === undefined ? { message: "" } : { refuse: "Answer the question first. Files attached go with a message." }
  }
  if ("message" in does || "refuse" in does) return does
  return { refuse: "Files attached go with a message, not with a command or an answer. Remove them to send this." }
}

// The conversation's items as the page draws them: a question the session
// is waiting on is left out where it was asked, since it shows where it is
// answered, at the end or docked above the box. One the log has no answer
// to that the session no longer waits on, as one cut off by a restart, is
// drawn as it was asked, unanswered.
export function asAsked(items: readonly Item[], waiting: readonly Asked[]): Item[] {
  const open = new Set(waiting.map((question) => question.id))
  return items.flatMap((item) => (item.kind !== "question" || !item.waiting ? [item] : open.has(item.id) ? [] : [{ ...item, waiting: false }]))
}
