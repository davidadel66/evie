import { createInterface } from "node:readline"
import { connect, ConnectError, createLiveInput, dataFolder, runTerminal, type Settings } from "../index.ts"
import { ownTokenFile, readOwnToken } from "../server/tokens.ts"
import { loadServerSettings } from "../settings-files.ts"
import { EXIT_CODES, fail, print } from "./common.ts"

// `evie`: connects to the server and runs the terminal client over the
// connection, in the folder it is run in. The server is at the address in
// the settings, or `EVIE_SERVER`; the token is `EVIE_TOKEN`, or this
// computer's own.
export async function terminal(env: Settings, args: string[]) {
  const serverSettings = await loadServerSettings({ env }).catch(fail)
  const { token, own } = await tokenFor(env, serverSettings.address)
  // Puts the terminal back as it was, when the live input line has taken it over.
  let restore = () => {}
  const evie = await connect({
    url: serverSettings.address,
    token,
    // So that an address where something takes the connection but never
    // answers does not leave the terminal waiting for ever.
    signal: AbortSignal.timeout(10_000),
    // The server stopped, or took this device's token back. Connecting again
    // is not built yet, so the terminal says so and leaves.
    onDrop: (reason) => {
      restore()
      console.error(`\n[error] The connection to the Evie server was lost: ${reason}. \`evie --continue\` carries on with this session.`)
      process.exit(1)
    },
  }).catch((cause) => {
    // Taken back, or made by a server whose data folder has since changed.
    if (own && cause instanceof ConnectError && cause.reason === "token") {
      fail(
        `The Evie server at ${new URL(serverSettings.address).origin} refused this computer's token. Removing ` +
          `${ownTokenFile(dataFolder(env))} and starting \`evie serve\` again makes a new one.`,
      )
    }
    return fail(cause)
  })

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

// The names a client on this computer reaches its own server by.
const THIS_COMPUTER = ["127.0.0.1", "localhost", "[::1]"]

// The token to connect with: `EVIE_TOKEN`, or else this computer's own, which
// `evie serve` saved in the data folder. `own` says it is the latter. This
// computer's token is sent only to a server on this computer, so that it
// never reaches another.
async function tokenFor(env: Settings, address: string): Promise<{ token: string; own: boolean }> {
  const given = env.EVIE_TOKEN?.trim()
  if (given) return { token: given, own: false }
  const elsewhere = Boolean(env.EVIE_SERVER?.trim()) && !THIS_COMPUTER.includes(new URL(address).hostname)
  if (elsewhere) {
    fail(
      "EVIE_SERVER names another computer, so this computer's own token is not sent to it. Set EVIE_TOKEN to a token " +
        "made with `evie token add <name>` on the server's computer.",
    )
  }
  const own = await readOwnToken(dataFolder(env)).catch(fail)
  return own === undefined
    ? fail("No Evie server has run on this computer yet. Start one with `evie serve`, which also makes this computer's token.")
    : { token: own, own: true }
}
