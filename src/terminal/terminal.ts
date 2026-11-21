import type { Evie, EventBodies, ToolCall } from "../core/types.ts"

export type TerminalOptions = {
  // The client interface. The terminal uses nothing else of Evie.
  evie: Evie
  // The folder the terminal was started in; its project is the one worked on.
  folder: string
  // Carry on the last session of that project, not start a new one.
  resume?: boolean
  // The lines the owner types.
  lines: AsyncIterable<string>
  // Where to print.
  write(text: string): void
  // Whether to print a prompt before each line is typed. Leave it out when
  // the input line draws its own, as the live input line does.
  prompt?: boolean
  // Called once with a function to run when the owner presses Ctrl-C. That
  // function cancels the turn in progress and says what it did: "cancelled";
  // "idle" when nothing is running, which is the caller's cue to quit; or
  // "again" when a cancel was already asked for and the turn has still not
  // stopped, which is the caller's cue to quit without waiting.
  onInterrupt?: (interrupt: () => "cancelled" | "idle" | "again") => void
  // Whether the owner can be asked questions here. Leave it out for yes. It
  // is no when the input is not typed by a person, such as a pipe: a line
  // meant as a message must not be taken as an answer.
  questions?: boolean
  // What the owner has typed and not yet sent, when the input line can say.
  // It is used to tell a line that was being typed before a question
  // appeared from an answer to it.
  typing?: () => string
  // When the input ends, wait for background jobs to end, and for Evie to
  // deal with their results, before leaving. For input that is not typed
  // by a person, where leaving would stop the jobs. Ctrl-C ends the wait.
  waitForJobs?: boolean
}

const PROMPT = "\n› "

const shorten = (text: string, length: number) => (text.length > length ? `${text.slice(0, length)}…` : text)

// What waiting for a command gives in place of its result when the input
// ends while the command's question is waiting: the terminal is leaving.
const LEFT = Symbol("left")

// The terminal client. It opens a session in the project of the folder it was
// started in, sends each line typed as a message, and prints the model's reply
// as it streams. When Evie asks a question, the next line typed is the
// answer. It ends when the input ends, after any turn in progress, unless
// that turn is waiting on a question: then it leaves the question for another
// terminal, or for this one started again, to answer.
export async function runTerminal(options: TerminalOptions): Promise<void> {
  const { evie } = options

  // Whether the last thing printed left the cursor part way along a line.
  let midLine = false
  const write = (text: string) => {
    if (text === "") return
    options.write(text)
    midLine = !text.endsWith("\n")
  }
  // Shown when Evie is ready for the next message: a blank line, then the
  // prompt unless the input line draws its own.
  const showPrompt = () => write(options.prompt === false ? "\n" : PROMPT)
  // Prints text on a line of its own.
  const line = (text: string) => write(`${midLine ? "\n" : ""}${text}\n`)

  // The lines typed are read one at a time, here and nowhere else, because
  // a question can need a line before the session has finished opening.
  const typedLines = options.lines[Symbol.asyncIterator]()
  let reading: Promise<IteratorResult<string>> | undefined
  const nextLine = () => (reading ??= typedLines.next())

  // The session, once it is open.
  let session: string | undefined
  // The question waiting for the owner's answer, and the session it is in.
  let asked: { session: string; question: EventBodies["question"] } | undefined
  let whenAsked = () => {}
  // True when the owner was part way through typing a line as the question
  // appeared. That line was not meant as its answer.
  let typedBefore = false
  const showQuestion = () => {
    if (!asked) return
    const { text, choices, freeText } = asked.question
    line(`[question] ${text}`)
    choices.forEach((choice, index) => line(`  ${index + 1}. ${choice}`))
    line(freeText ? "Type a number, or your own answer." : "Type a number.")
    if (options.prompt !== false) write(PROMPT.trimStart())
  }
  // Whether the input has ended. A terminal never refuses a question by
  // leaving, since another client can share the session and answer it, with
  // one exception: a question asked while its session opens (see answeringDuring).
  let ended = false
  // Takes a typed line as the answer to the question that is waiting.
  const answer = async (typed: string) => {
    if (!asked) return
    if (typedBefore) {
      // Sent as the message it was meant to be, when there is a session to
      // send it to. The model reads it at its next step.
      typedBefore = false
      if (session !== undefined && typed !== "") {
        line("[That line was typed before the question appeared, so it was sent as a message, not taken as the answer.]")
        unanswered += 1
        idle = false
        await evie.send({ type: "send_message", session, text: typed })
      } else if (typed !== "") {
        line("[That line was typed before the question appeared, so it was not taken as the answer.]")
      }
      return showQuestion()
    }
    const { choices, freeText } = asked.question
    const picked = /^\d+$/.test(typed) ? choices[Number(typed) - 1] : undefined
    const choice = picked ?? (freeText && typed !== "" && !/^\d+$/.test(typed) ? typed : undefined)
    if (choice === undefined) return showQuestion()
    await evie
      .send({ type: "answer_question", session: asked.session, id: asked.question.id, choice })
      .catch((cause) => line(`[error] ${cause instanceof Error ? cause.message : String(cause)}`))
  }

  // Questions that have arrived and not been answered, the one on screen included.
  const waitingQuestions: NonNullable<typeof asked>[] = []
  // Queues a question, to be shown after any already waiting. One can reach
  // the terminal twice, as an event and in the reply that opened the session.
  const queueQuestion = (question: NonNullable<typeof asked>) => {
    if (waitingQuestions.some((waiting) => waiting.question.id === question.question.id)) return
    waitingQuestions.push(question)
    if (asked === undefined) showNextQuestion()
  }
  const showNextQuestion = () => {
    asked = waitingQuestions[0]
    if (!asked) return
    typedBefore = (options.typing?.() ?? "").trim() !== ""
    showQuestion()
    whenAsked()
  }

  // Every question asked and not yet answered, by id, with its session,
  // whether or not this terminal takes answers. While one is open in the
  // terminal's session, the turn there waits on it.
  const questionsOpen = new Map<string, string>()

  // Messages sent that no turn has taken up yet. Several can be answered by
  // one turn, so turns are not counted: the terminal is idle when a turn ends
  // and nothing sent is still waiting.
  let unanswered = 0
  let idle = true
  let whenIdle = () => {}
  // The tool calls of the current step that have not been shown yet.
  let pendingCalls: ToolCall[] = []
  const showNextCall = () => {
    const call = pendingCalls.shift()
    if (call) line(`⏺ ${call.name} ${shorten(call.arguments, 120)}`)
  }
  // True from a Ctrl-C until the turn it cancelled has ended.
  let cancelling = false
  // The background jobs still running, and whether the owner has said to stop waiting for them.
  const runningJobs = new Set<string>()
  let gaveUpWaiting = false
  // True while a command such as /compact is running.
  let commanding = false

  // Waits for work that can stop to ask a question: opening the session, or
  // running a command. Lines are read during it only while a question is
  // waiting, and each one is taken as the answer. When the input ends while
  // one is waiting, a command stops being waited for and gives `left`, and
  // the question is left open. A question asked while the session is opening,
  // such as whether to trust the project, is refused instead: no other client
  // can reach it, and while it waits every session opening in the project waits.
  async function answeringDuring<Result, Left = never>(work: Promise<Result>, left?: Left): Promise<Result | Left> {
    let finished = false
    const settled = work
      .then(
        () => {},
        () => {},
      )
      .then(() => {
        finished = true
        whenAsked()
      })
    while (!finished) {
      if (!asked) await new Promise<void>((resolve) => (whenAsked = resolve))
      if (finished || !asked) continue
      const typed = await Promise.race([nextLine(), settled])
      if (!typed) continue
      reading = undefined
      if (!typed.done) {
        midLine = false
        await answer(typed.value.trim())
        continue
      }
      ended = true
      if (left !== undefined) return left
      if (asked) evie.send({ type: "answer_question", session: asked.session, id: asked.question.id, choice: null }).catch(() => {})
      asked = undefined
    }
    return work
  }

  const stop = evie.subscribe((event) => {
    // Until the session is open, the only events that can be for this
    // terminal are a question asked while it opens, and its answer.
    const mine = session === undefined ? event.type === "question" || event.type === "answer" : event.session === session
    if (!mine) return
    // More than one can be open at once, such as from two subagents. They
    // are shown one at a time, in the order they came. Where nobody types,
    // as from a pipe, a question is only shown: the session may belong to
    // another terminal that does take answers, and a line meant as a message
    // must not be taken as one.
    if (event.type === "question" && options.questions === false) line(`[question] ${event.body.text} (not answered here)`)
    else if (event.type === "question") queueQuestion({ session: event.session, question: event.body })
    if (event.type === "question") {
      questionsOpen.set(event.body.id, event.session)
      whenIdle()
    }
    if (event.type === "answer") {
      questionsOpen.delete(event.body.id)
      const at = waitingQuestions.findIndex((waiting) => waiting.question.id === event.body.id)
      if (at !== -1) waitingQuestions.splice(at, 1)
      if (asked?.question.id === event.body.id) {
        asked = undefined
        line(event.body.choice === null ? "[not answered]" : `[answered] ${event.body.choice}`)
        showNextQuestion()
      }
    }
    // A turn can start without a message from the owner, when a background job ends.
    if (event.type === "turn_start") idle = false
    if (event.type === "job") {
      const { job } = event
      if (job.status === "running") runningJobs.add(job.id)
      else runningJobs.delete(job.id)
      whenIdle()
      const took = `${Math.max(0, Math.round(((job.ended ?? job.started) - job.started) / 1000))}s`
      line(job.status === "running" ? `[background] job ${job.id} started: ${job.title}` : `[background] job ${job.id} ${job.status} after ${took}: ${job.title}`)
    }
    // A message can also come from a command, such as one that starts a
    // skill, so the count never goes below nothing.
    if (event.type === "user_message") unanswered = Math.max(0, unanswered - 1)
    if (event.type === "assistant_delta") write(event.text)
    // One line for each tool the model asks for, and one for what came
    // back. The tools run one at a time, so each call is shown just before it
    // runs, with its result straight after.
    if (event.type === "assistant_message") {
      pendingCalls = [...(event.body.toolCalls ?? [])]
      showNextCall()
    }
    if (event.type === "tool_result") {
      const lines = event.body.text.split("\n")
      const more = lines.length > 1 ? ` (+${lines.length - 1} more lines)` : ""
      line(`  ⎿ ${shorten(lines[0] ?? "", 100)}${more}`)
      showNextCall()
    }
    if (event.type === "compaction") line("[The conversation was getting long, so its earlier part is now a summary.]")
    if (event.type === "turn_end") {
      if (event.body.reason === "error") line(`[error] ${event.body.error ?? "The turn failed."}`)
      if (event.body.reason === "cancelled") line("[cancelled]")
      cancelling = false
      if (midLine) write("\n")
      if (unanswered === 0) {
        idle = true
        showPrompt()
        whenIdle()
      }
    }
  })

  // Opening can stop to ask a question, such as whether to trust the project.
  const { project, resumed, notices, session: openedSession, turnRunning, openQuestions } = await answeringDuring(
    evie.send({
      type: "open_session",
      folder: options.folder,
      ...(options.questions !== false && { questions: true }),
      ...(options.resume && { continue: true }),
    }),
  ).catch((cause) => {
    stop()
    throw cause
  })
  session = openedSession
  for (const question of openQuestions) questionsOpen.set(question.id, openedSession)
  // The session can be one another terminal left running, with its turn
  // still going: the terminal waits for it, and Ctrl-C stops it.
  idle = !turnRunning

  options.onInterrupt?.(() => {
    if (idle && !commanding) {
      // Pressed while waiting for jobs after the input ended: stop waiting.
      gaveUpWaiting = true
      whenIdle()
      return "idle"
    }
    if (cancelling) return "again"
    cancelling = true
    evie.send({ type: "cancel", session: openedSession }).catch((cause) => line(`[error] ${cause instanceof Error ? cause.message : String(cause)}`))
    return "cancelled"
  })

  const about = resumed
    ? " · continuing the last session"
    : options.resume
      ? " · no earlier session here, so this is a new one"
      : ""
  write(`evie · ${project?.name ?? "general"}${about}\n`)
  for (const notice of notices) write(`[note] ${notice}\n`)
  if (idle) showPrompt()
  else line("[note] A turn is still running in this session. Ctrl-C stops it.")
  // Questions it asked while no terminal was connected are shown now, so
  // they can be answered. Not where nobody types, as from a pipe: a line
  // meant as a message must not be taken as an answer.
  if (options.questions !== false) for (const question of openQuestions) queueQuestion({ session: openedSession, question })
  try {
    while (!ended) {
      const read = await nextLine()
      reading = undefined
      if (read.done) break
      // The Enter that ended the typed line moved the cursor to a new one.
      midLine = false
      const text = read.value.trim()
      // While a question is waiting, what is typed is its answer.
      if (asked) {
        await answer(text)
        continue
      }
      if (text === "") {
        if (idle) showPrompt()
        continue
      }
      // A slash and a plain word, such as /model, is a command: it runs at
      // once and is not sent to the model. A path such as /Users/me/file is
      // not a plain word, so a message may start with one.
      const command = /^\/([A-Za-z][\w-]*)(?:\s+(.*))?$/.exec(text)
      if (command) {
        // A command can ask a question too, so lines are still read for answers.
        commanding = true
        await answeringDuring(evie.send({ type: "run_command", session: openedSession, name: command[1]!, args: command[2] ?? "" }), LEFT)
          .then((result) => result !== LEFT && line(result.text))
          .catch((cause) => line(`[error] ${cause instanceof Error ? cause.message : String(cause)}`))
        commanding = false
        if (ended) break
        if (idle) {
          cancelling = false
          showPrompt()
        }
        continue
      }
      unanswered += 1
      idle = false
      await evie.send({ type: "send_message", session: openedSession, text })
    }
    ended = true
    // The turn in progress is waited for. So are background jobs, when asked:
    // one that ends wakes Evie, and that turn is waited for too. A question
    // waiting in the session is not: nobody here will answer it, so the
    // terminal says so and leaves it for whoever comes back to the session.
    const busy = () => !idle || (options.waitForJobs === true && runningJobs.size > 0 && !gaveUpWaiting)
    const questionWaiting = () => [...questionsOpen.values()].includes(openedSession)
    for (;;) {
      if (questionWaiting()) {
        // A question can be refused the moment it is asked, when no client
        // that shows questions has opened the session, and over the network
        // the refusal arrives just after the question. A reply comes after
        // every event sent before it, so asking where the session stands, as
        // `evie -c` does, lets that refusal arrive first.
        await evie.send({ type: "open_session", folder: options.folder, continue: true }).catch(() => {})
        if (questionWaiting()) {
          line("[note] A question is waiting for an answer. `evie -c` in a terminal comes back to it.")
          break
        }
        continue
      }
      if (!busy()) break
      await new Promise<void>((resolve) => (whenIdle = resolve))
    }
  } finally {
    stop()
    write("\n")
  }
}
