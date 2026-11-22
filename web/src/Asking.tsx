import { useEffect, useState } from "react"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { CaretDown } from "./icons.tsx"
import { choiceForKey, freshGuard, guardCleared, guardReleases, guardSees, guardTyped, permissionChoices, releaseAt, shownQuestion, type Asked, type Guard } from "./questions.ts"

// The questions evie asks, answered in place, as T3 Code answers them: a
// permission prompt in the conversation, with its choices as buttons, and
// any other question docked above the message box, its choices numbered.
// One shows at a time; one that arrives while the owner is typing waits
// until they stop.

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

// The question waits with no time limit, as the board's Question says.
function WaitingLine({ count, also }: { count: string | undefined; also?: string }) {
  return (
    <p className="flex items-center gap-2 text-caption text-text-muted">
      <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-status-needs" />
      <span>
        Waiting for your answer{count !== undefined && ` (${count})`}
        {also !== undefined && `. ${also}`}
      </span>
    </p>
  )
}

// A question's text: its first paragraph as the question, and what follows,
// such as a command or the files it would change, as code.
function QuestionText({ text }: { text: string }) {
  const [question = "", ...rest] = text.split(/\n\s*\n/)
  const detail = rest.join("\n\n").replace(/^ {2}/gm, "")
  return (
    <>
      <p className="text-label break-words whitespace-pre-wrap text-text">{question}</p>
      {detail.trim() !== "" && <pre className="font-mono text-code break-words whitespace-pre-wrap text-text">{detail}</pre>}
    </>
  )
}

// Why an answer was not taken, until the next try.
function useRefused(question: string) {
  const [refused, setRefused] = useState<string>()
  useEffect(() => setRefused(undefined), [question])
  return [refused, setRefused] as const
}

// A permission prompt, at the end of the conversation, as T3 Code shows an
// approval: what evie would do, then its choices as buttons, the one that
// refuses first, the one that allows as the main one, and any others,
// such as always allowing it, under More. In a session, words sent from the
// box refuse it and go to evie as what to do instead (`redirect`).
export function PermissionPrompt({ question, count, answer, redirect }: { question: Asked; count: string | undefined; answer: Answer; redirect: boolean }) {
  const { deny, allow, more } = permissionChoices(question)
  const [refused, setRefused] = useRefused(question.id)
  const pick = async (choice: string) => setRefused(await answer(question.id, choice))
  return (
    <section aria-label="Permission needed" className="flex flex-col gap-2">
      <QuestionText text={question.text} />
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="secondary" className="rounded-full" onClick={() => void pick(deny)}>
          {deny}
        </Button>
        <Button variant="primary" className="rounded-full" onClick={() => void pick(allow)}>
          {allow}
        </Button>
        {more.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="quiet" className="gap-1">
                More
                <CaretDown className="size-icon-control" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="max-w-menu-wide">
              {more.map((choice) => (
                <DropdownMenuItem key={choice} onSelect={() => void pick(choice)} className="h-auto min-h-control-sm py-1 whitespace-normal">
                  {choice}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
      <WaitingLine count={count} {...(redirect && { also: "Or send a message to say no, and what to do instead" })} />
      {refused !== undefined && (
        <p role="status" className="text-caption break-words text-text-muted">
          {refused}
        </p>
      )}
    </section>
  )
}

// Any other question, docked above the message box, as T3 Code docks one:
// it floats, so it has a surface. Its choices are numbered, and a key from
// 1 to 9 picks one while the focus is not in a text box, as T3 Code's do;
// in the box, a number sent picks it too, as in the terminal, and the
// owner's own words sent are the answer when it takes them.
export function DockedQuestion({ question, count, answer }: { question: Asked; count: string | undefined; answer: Answer }) {
  const [refused, setRefused] = useRefused(question.id)
  const pick = async (choice: string) => setRefused(await answer(question.id, choice))
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target?.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="menu"], [role="dialog"], [role="listbox"]')) return
      const choice = choiceForKey(event.key, question)
      if (choice === undefined) return
      event.preventDefault()
      void pick(choice)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [question])
  return (
    <section aria-label="Question from evie" className="flex flex-col gap-2 rounded-lg border border-line bg-surface p-3 shadow-float">
      <QuestionText text={question.text} />
      <ol className="flex flex-wrap gap-2">
        {question.choices.map((choice, at) => (
          <li key={choice}>
            <Button variant="secondary" className="h-auto min-h-control rounded-full py-1 text-left whitespace-normal" onClick={() => void pick(choice)}>
              <span className="text-text-muted">{at + 1}</span>
              {choice}
            </Button>
          </li>
        ))}
      </ol>
      <WaitingLine count={count} also={question.freeText ? "Choose one, or write your own answer below" : "Choose one, or type its number"} />
      {refused !== undefined && (
        <p role="status" className="text-caption break-words text-text-muted">
          {refused}
        </p>
      )}
    </section>
  )
}
