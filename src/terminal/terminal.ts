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
}

const PROMPT = "\n› "

// The terminal client. It opens a session in the project of the folder it was
// started in, sends each line typed as a message, and prints the model's reply
// as it streams. It ends when the input ends, after any turn in progress.
export async function runTerminal(options: TerminalOptions): Promise<void> {
  const { evie, write } = options
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
    if (event.type === "turn_end") {
      if (event.body.reason === "error") write(`\n[error] ${event.body.error ?? "The turn failed."}`)
      write("\n")
      if (unanswered === 0) {
        idle = true
        write(PROMPT)
        whenIdle()
      }
    }
  })

  write(`evie · ${project?.name ?? "general"}\n`)
  write(PROMPT)
  try {
    for await (const line of options.lines) {
      const text = line.trim()
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
