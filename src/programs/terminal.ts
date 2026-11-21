import { createInterface } from "node:readline"
import { connect, createLiveInput, dataFolder, runTerminal, type Settings } from "../index.ts"
import { readOwnToken } from "../server/tokens.ts"
import { loadServerSettings } from "../settings-files.ts"
import { EXIT_CODES, fail, print } from "./common.ts"

// `evie`: connects to the server and runs the terminal client over the
// connection, in the folder it is run in. The server is at the address in
// the settings, or `EVIE_SERVER`; the token is `EVIE_TOKEN`, or this
// computer's own.
export async function terminal(env: Settings, args: string[]) {
  const serverSettings = await loadServerSettings({ env }).catch(fail)
  const token =
    env.EVIE_TOKEN?.trim() ||
    (await readOwnToken(dataFolder(env)).catch(fail)) ||
    fail(
      "There is no token for this computer yet. `evie serve` makes one the first time it starts. To reach a server " +
        "on another computer, set EVIE_TOKEN to a token made there with `evie token add <name>`.",
    )
  // Puts the terminal back as it was, when the live input line has taken it over.
  let restore = () => {}
  const evie = await connect({
    url: serverSettings.address,
    token,
    // The server stopped, or took this device's token back. Connecting again
    // is not built yet, so the terminal says so and leaves.
    onDrop: (reason) => {
      restore()
      console.error(`\n[error] The connection to the Evie server was lost: ${reason}. \`evie --continue\` carries on with this session.`)
      process.exit(1)
    },
  }).catch(fail)

  // Leaving stops nothing in the server: being told to stop, or the terminal
  // window closing, closes the connection only.
  for (const signal of ["SIGTERM", "SIGHUP"] as const) {
    process.on(signal, () => {
      void evie.close().finally(() => process.exit(EXIT_CODES[signal]))
    })
  }

  const session = { folder: process.cwd(), resume: args.includes("--continue") || args.includes("-c") }
  // What the terminal client does on Ctrl-C: cancel the turn, or say that
  // nothing is running, or that a cancel is already under way.
  let interrupt = (): "cancelled" | "idle" | "again" => "idle"

  let problem: unknown
  try {
    if (process.stdin.isTTY && process.stdout.isTTY) {
      // A real terminal: keys arrive one at a time, and the line being typed is
      // kept at the bottom with Evie's output above it.
      const live = createLiveInput({
        write: print,
        columns: () => process.stdout.columns || 80,
      })
      live.onInterrupt(() => {
        const did = interrupt()
        // With nothing running, Ctrl-C quits. Pressed again while a turn is
        // refusing to stop, it quits at once.
        if (did === "idle") live.close()
        if (did === "again") {
          process.stdin.setRawMode(false)
          process.exit(EXIT_CODES.SIGINT)
        }
      })
      restore = () => {
        live.close()
        process.stdin.setRawMode(false)
      }
      process.stdin.setRawMode(true)
      process.stdin.setEncoding("utf8")
      process.stdin.on("data", (data: string) => live.key(data))
      try {
        await runTerminal({
          evie,
          ...session,
          lines: live.lines,
          write: live.write,
          prompt: false,
          typing: live.typing,
          onInterrupt: (handler) => (interrupt = handler),
        })
      } finally {
        live.close()
        process.stdin.setRawMode(false)
        process.stdin.pause()
      }
    } else {
      // Not a terminal, such as input from a pipe: read whole lines.
      const input = createInterface({ input: process.stdin })
      const onCtrlC = () => {
        const did = interrupt()
        if (did === "idle") input.close()
        if (did === "again") process.exit(EXIT_CODES.SIGINT)
      }
      input.on("SIGINT", onCtrlC)
      process.on("SIGINT", onCtrlC)
      await runTerminal({
        evie,
        ...session,
        lines: input,
        write: print,
        // Nobody is typing, so a line must not be taken as an answer.
        questions: false,
        // So that piped work is seen to its end: the turn, any command moved
        // to the background, and what Evie made of it.
        waitForJobs: true,
        onInterrupt: (handler) => (interrupt = handler),
      })
    }
  } catch (cause) {
    // A session that could not be opened, such as when no model is set.
    problem = cause
  } finally {
    // The connection only. Anything still running carries on in the server.
    await evie.close()
  }
  if (problem !== undefined) fail(problem)
}
