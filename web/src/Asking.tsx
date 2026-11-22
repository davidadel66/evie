import { useEffect, useState } from "react"
import { Button } from "./components/ui/button.tsx"
import { choiceForKey, freshGuard, type KeyFocus, guardCleared, guardReleases, guardSees, guardTyped, releaseAt, shownQuestion, type Asked, type Guard } from "./questions.ts"

// The questions evie asks, answered in place in the conversation, as part
// of its reply, as the Session view drawing has them: the question, its
// choices as buttons under it, the first the main one, and under them that
// evie needs the owner. A permission prompt is drawn the same way. One
// shows at a time; one that arrives while the owner is typing waits until
// they stop.

export type Shown = ReturnType<typeof shownQuestion>

// The question to show of those waiting, held back while the owner types,
// and what the message box tells the guard: a key typed, and the owner
// sending or leaving the box.
export function useAsking(waiting: readonly Asked[]): { shown: Shown; typed(): void; cleared(): void } {
  const ids = waiting.map((question) => question.id).join(" ")
  const seen = (guard: Guard) => guardSees(guard, ids === "" ? [] : ids.split(" "), Date.now())
  const [kept, setKept] = useState(() => ({ ids, guard: seen(freshGuard()) }))
  // Worked out as the questions are drawn, not after, so that a question
  // that arrives while the owner types is never shown, even for a moment,
  // when a key such as Enter could answer it. React draws again at once,
  // before anything is shown, with what is kept here.
  let guard = kept.guard
  if (kept.ids !== ids) {
    guard = seen(kept.guard)
    setKept({ ids, guard })
  }
  const keep = (change: (guard: Guard) => Guard) => setKept((was) => ({ ...was, guard: change(was.guard) }))
  // Shown once the owner has not typed for long enough.
  const at = releaseAt(guard)
  useEffect(() => {
    if (at === undefined) return
    const timer = setTimeout(() => keep((was) => guardReleases(was, Date.now())), Math.max(0, at - Date.now()))
    return () => clearTimeout(timer)
  }, [at])
  return {
    shown: shownQuestion(waiting, guard.held),
    typed: () => keep((was) => guardTyped(was, Date.now())),
    cleared: () => keep(guardCleared),
  }
}

// What answering does: gives back why the server would not take it, or nothing.
type Answer = (id: string, choice: string | null) => Promise<string | undefined>

// The question waits with no time limit, as the board's Question says: a
// dot and the words in the colour for what needs the owner.
function NeedsYou({ count }: { count: string | undefined }) {
  return (
    <p className="mt-4 flex items-center gap-2 text-caption text-status-needs">
      <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-status-needs" />
      <span>Needs you. Waiting for your answer{count !== undefined && ` (${count})`}</span>
    </p>
  )
}

// A question's text: its first paragraph as the question, and what follows,
// such as a command or the files it would change, as code. The same once
// it has been answered.
export function QuestionText({ text }: { text: string }) {
  const [question = "", ...rest] = text.split(/\n\s*\n/)
  const detail = rest.join("\n\n").replace(/^ {2}/gm, "")
  return (
    <>
      <p className="text-label break-words whitespace-pre-wrap text-text">{question}</p>
      {detail.trim() !== "" && <pre className="mt-2 font-mono text-code break-words whitespace-pre-wrap text-text">{detail}</pre>}
    </>
  )
}

// A question at the end of the conversation, as the Session view drawing
// has one: the question in bold, its choices as round buttons under it,
// the first the main one and the others secondary, and under them that
// evie needs the owner. A key from 1 to 9 picks a choice while the focus is
// on nothing or in the empty message box, as T3 Code's keys do; the
// owner's own words sent from the box are the answer when it takes them.
// A permission prompt looks the same, and only a click answers it: words
// sent from the box refuse it, and go to evie as what to do instead.
export function QuestionPrompt({ question, count, answer }: { question: Asked; count: string | undefined; answer: Answer }) {
  const [refused, setRefused] = useState<string>()
  useEffect(() => setRefused(undefined), [question.id])
  const pick = async (choice: string) => setRefused(await answer(question.id, choice))
  const keys = question.kind !== "permission"
  useEffect(() => {
    if (!keys) return
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || event.isComposing) return
      const choice = choiceForKey(event.key, question, focusOf(event.target))
      if (choice === undefined) return
      event.preventDefault()
      void pick(choice)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [question, keys])
  return (
    <section aria-label={question.kind === "permission" ? "Permission needed" : "Question from evie"} className="flex flex-col">
      <QuestionText text={question.text} />
      <div className="mt-3 flex flex-wrap gap-2">
        {question.choices.map((choice, at) => (
          <Button
            key={choice}
            variant={at === 0 ? "primary" : "secondary"}
            {...(keys && at < 9 && { title: `${choice} (${at + 1})` })}
            className="h-auto min-h-control rounded-full py-1 text-left whitespace-normal"
            onClick={() => void pick(choice)}
          >
            {choice}
          </Button>
        ))}
      </div>
      {refused !== undefined && (
        <p role="status" className="mt-2 text-caption break-words text-text-muted">
          {refused}
        </p>
      )}
      <NeedsYou count={count} />
    </section>
  )
}

// Where the focus is as a key is pressed, for the keys 1 to 9: on nothing,
// in the message box, which marks itself, and whether it is empty, or with
// something typed, or anywhere else, as on a button or a row of the work log.
function focusOf(target: EventTarget | null): KeyFocus {
  if (!(target instanceof HTMLElement) || target === document.body || target === document.documentElement) return "nothing"
  if (target.dataset.messageBox !== undefined) return target.dataset.empty !== undefined ? "empty box" : "typed box"
  return "elsewhere"
}
