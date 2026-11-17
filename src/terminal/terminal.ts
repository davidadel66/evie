import type { Evie } from "../core/types.ts"

export type TerminalOptions = {
  // The client interface. The terminal uses nothing else of Evie.
  evie: Evie
  // The folder the terminal was started in; its project is the one worked on.
  folder: string
  // The lines the owner types.
  lines: AsyncIterable<string>
  // Where to print.
  write(text: string): void
  // Called once with a function to run when the owner presses Ctrl-C. That
  // function cancels the turn in progress and returns true, or returns false
  // when nothing is running, which is the caller's cue to quit.
  onInterrupt?: (interrupt: () => boolean) => void
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
  const { session, project } = await evie.send({ type: "open_session", folder: options.folder })

  // Messages sent that no turn has taken up yet. Several can be answered by
  // one turn, so turns are not counted: the terminal is idle when a turn ends
  // and nothing sent is still waiting.
  let unanswered = 0
  let idle = true
  let whenIdle = () => {}

  const stop = evie.subscribe((event) => {
    if (event.session !== session) return
    if (event.type === "user_message") unanswered -= 1
    if (event.type === "assistant_delta") write(event.text)
    // One line for each tool the model asks for, and one for what came back.
    if (event.type === "assistant_message") {
      for (const call of event.body.toolCalls ?? []) line(`⏺ ${call.name} ${shorten(call.arguments, 120)}`)
    }
    if (event.type === "tool_result") {
      const lines = event.body.text.split("\n")
      const more = lines.length > 1 ? ` (+${lines.length - 1} more lines)` : ""
      line(`  ⎿ ${shorten(lines[0] ?? "", 100)}${more}`)
    }
    if (event.type === "turn_end") {
      if (event.body.reason === "error") line(`[error] ${event.body.error ?? "The turn failed."}`)
      if (event.body.reason === "cancelled") line("[cancelled]")
      if (midLine) write("\n")
      if (unanswered === 0) {
        idle = true
        write(PROMPT)
        whenIdle()
      }
    }
  })

  options.onInterrupt?.(() => {
    if (idle) return false
    void evie.send({ type: "cancel", session })
    return true
  })

  write(`evie · ${project?.name ?? "general"}\n`)
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
