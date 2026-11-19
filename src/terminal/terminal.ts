import type { Evie, ToolCall } from "../core/types.ts"

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
  // Called once with a function to run when the owner presses Ctrl-C. That
  // function cancels the turn in progress and says what it did: "cancelled";
  // "idle" when nothing is running, which is the caller's cue to quit; or
  // "again" when a cancel was already asked for and the turn has still not
  // stopped, which is the caller's cue to quit without waiting.
  onInterrupt?: (interrupt: () => "cancelled" | "idle" | "again") => void
}

const PROMPT = "\n› "

const shorten = (text: string, length: number) => (text.length > length ? `${text.slice(0, length)}…` : text)

// The terminal client. It opens a session in the project of the folder it was
// started in, sends each line typed as a message, and prints the model's reply
// as it streams. It ends when the input ends, after any turn in progress.
export async function runTerminal(options: TerminalOptions): Promise<void> {
  const { evie } = options

  // Whether the last thing printed left the cursor part way along a line.
  let midLine = false
  const write = (text: string) => {
    if (text === "") return
    options.write(text)
    midLine = !text.endsWith("\n")
  }
  // Prints text on a line of its own.
  const line = (text: string) => write(`${midLine ? "\n" : ""}${text}\n`)
  const { session, project, resumed } = await evie.send({
    type: "open_session",
    folder: options.folder,
    ...(options.resume && { continue: true }),
  })

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

  const stop = evie.subscribe((event) => {
    if (event.session !== session) return
    if (event.type === "user_message") unanswered -= 1
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
    if (event.type === "turn_end") {
      if (event.body.reason === "error") line(`[error] ${event.body.error ?? "The turn failed."}`)
      if (event.body.reason === "cancelled") line("[cancelled]")
      cancelling = false
      if (midLine) write("\n")
      if (unanswered === 0) {
        idle = true
        write(PROMPT)
        whenIdle()
      }
    }
  })

  options.onInterrupt?.(() => {
    if (idle) return "idle"
    if (cancelling) return "again"
    cancelling = true
    evie.send({ type: "cancel", session }).catch((cause) => line(`[error] ${cause instanceof Error ? cause.message : String(cause)}`))
    return "cancelled"
  })

  const about = resumed
    ? " · continuing the last session"
    : options.resume
      ? " · no earlier session here, so this is a new one"
      : ""
  write(`evie · ${project?.name ?? "general"}${about}\n`)
  write(PROMPT)
  try {
    for await (const typed of options.lines) {
      // The Enter that ended the typed line moved the cursor to a new one.
      midLine = false
      const text = typed.trim()
      if (text === "") {
        if (idle) write(PROMPT)
        continue
      }
      // A slash and a plain word, such as /model, is a command: it runs at
      // once and is not sent to the model. A path such as /Users/me/file is
      // not a plain word, so a message may start with one.
      const command = /^\/([A-Za-z][\w-]*)(?:\s+(.*))?$/.exec(text)
      if (command) {
        await evie
          .send({ type: "run_command", session, name: command[1]!, args: command[2] ?? "" })
          .then((result) => line(result.text))
          .catch((cause) => line(`[error] ${cause instanceof Error ? cause.message : String(cause)}`))
        if (idle) write(PROMPT)
        continue
      }
      unanswered += 1
      idle = false
      await evie.send({ type: "send_message", session, text })
    }
    if (!idle) await new Promise<void>((resolve) => (whenIdle = resolve))
  } finally {
    stop()
    write("\n")
  }
}
