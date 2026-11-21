import type { EventLog } from "./log.ts"
import type { EventBodies, EvieEvent, Question } from "./types.ts"

// Questions: how Evie asks the owner something and waits for the answer.
//
// A question goes out to the client as a `question` event, and the answer
// comes back as an `answer_question` command. Both are written to the
// session's event log. Asking never fails and never waits for ever on its
// own account: a question that cannot be shown, or that is still open when
// the turn is cancelled or Evie closes, is answered with null, which whoever
// asked must treat as a refusal.

export type Questions = {
  // Asks the owner and waits. Returns the choice, or null for a refusal.
  // `canAnswer` says whether the session's client shows questions at all.
  ask(session: string, question: Question, options: { canAnswer: boolean; signal?: AbortSignal }): Promise<string | null>
  // The owner's answer to an open question, or null to refuse it.
  answer(session: string, id: string, choice: string | null): void
  // The questions in a session still waiting for an answer, oldest first.
  waiting(session: string): EventBodies["question"][]
  // Refuses every question that is still open. Used when Evie closes.
  refuseAll(): void
}

type Open = { session: string; question: Question; settle(choice: string | null): void }

export function createQuestions(options: { log: EventLog; emit(event: EvieEvent): void }): Questions {
  const { log, emit } = options
  const open = new Map<string, Open>()

  return {
    ask(session, question, { canAnswer, signal }) {
      if (question.text.trim() === "") throw new Error("A question needs some text.")
      if (question.choices.length === 0 && !question.freeText) throw new Error("A question needs choices, or to allow an answer in words.")

      const id = crypto.randomUUID().slice(0, 8)
      const asked: Question = { text: question.text, choices: [...question.choices], ...(question.freeText && { freeText: true as const }) }
      const event = log.append(session, "question", { id, ...asked })

      return new Promise((resolve) => {
        // The answer is written to the log in the same step that ends the
        // wait, so it is recorded even when Evie is closing.
        const settle = (choice: string | null) => {
          if (!open.delete(id)) return
          signal?.removeEventListener("abort", refuse)
          emit(log.append(session, "answer", { id, choice }))
          resolve(choice)
        }
        const refuse = () => settle(null)
        // The question is open before the client hears of it, so a client
        // that answers at once is not told there is no such question.
        open.set(id, { session, question: asked, settle })
        emit(event)
        if (!canAnswer || signal?.aborted) refuse()
        else if (open.has(id)) signal?.addEventListener("abort", refuse, { once: true })
      })
    },

    answer(session, id, choice) {
      // Checked, since a client that is not type-checked can send anything, and the answer is stored.
      if (choice !== null && typeof choice !== "string") throw new Error("`choice` is the answer, and must be text, or null to refuse the question.")
      const asked = open.get(id)
      if (!asked || asked.session !== session) throw new Error(`There is no open question "${id}" in this session. It may already have been answered.`)
      const { question } = asked
      if (choice !== null && !question.choices.includes(choice) && !(question.freeText && choice.trim() !== "")) {
        throw new Error(`"${choice}" is not one of the choices: ${question.choices.join(", ")}.`)
      }
      asked.settle(choice)
    },

    waiting(session) {
      return [...open].flatMap(([id, asked]) => (asked.session === session ? [{ id, ...asked.question }] : []))
    },

    refuseAll() {
      for (const asked of [...open.values()]) asked.settle(null)
    },
  }
}
