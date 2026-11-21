import { createInterface } from "node:readline"
import { connect, ConnectError, createLiveInput, dataFolder, runTerminal, type Settings } from "../index.ts"
import { ownTokenFile, readOwnToken } from "../server/tokens.ts"
import { PATHS } from "../server/wire.ts"
import { loadServerSettings } from "../settings-files.ts"
import { EXIT_CODES, fail, print, reason } from "./common.ts"

// `evie`: connects to the server and runs the terminal client over the
// connection, in the folder it is run in. The server is at the address in
// the settings, or `EVIE_SERVER`; the token is `EVIE_TOKEN`, or this
// computer's own.
export async function terminal(env: Settings, args: string[]) {
  const serverSettings = await loadServerSettings({ env }).catch(fail)
  const { token, own } = await tokenFor(env, serverSettings.address)
  // Why the server could not be reached, in words. When it refused this
  // computer's own token, that was taken back, or made by a server whose
  // data folder has since changed, and the way to a new one is given.
  const why = (cause: unknown) =>
    own && cause instanceof ConnectError && cause.reason === "token"
      ? `The Evie server at ${new URL(serverSettings.address).origin} refused this computer's token. Removing ` +
        `${ownTokenFile(dataFolder(env))} and starting \`evie serve\` again makes a new one.`
      : reason(cause)
  // Puts the terminal back as it was, when the live input line has taken it over.
  let restore = () => {}
  // Prints a note on a line of its own. The terminal client does it once it runs.
  let note = (text: string) => print(`[note] ${text}\n`)
  // True while the connection to the server is down.
  let down = false
  const evie = await connect({
    url: serverSettings.address,
    token,
    // So that an address where something takes the connection but never
    // answers does not leave the terminal waiting for ever.
    signal: AbortSignal.timeout(10_000),
    // Not typed in by a person, as from a pipe or a script, there is no
    // one to press Ctrl-C, and trying for ever would leave the script hung
    // for good, so evie gives up after a while. A terminal a person types
    // in keeps trying, since a laptop can sleep for hours and they come back.
    ...(!process.stdin.isTTY && { giveUpAfter: giveUpAfter(env) }),
    // As when the laptop sleeps or the server is stopped. The client
    // connects again by itself, and shows what was missed meanwhile.
    onDrop: () => {
      down = true
      note("The connection to Evie's server dropped. Trying again…")
    },
    onBack: () => {
      down = false
      note("Connected to Evie's server again.")
    },
    // Catching up on what was missed failed, so some of it may not be shown.
    onMissing: (text) => note(text),
    // The server took this device's token back, or refuses it now, which
    // trying again will not change, or the line stayed down too long.
    onEnd: (cause) => {
      restore()
      fail(why(cause))
    },
  }).catch((cause) => fail(why(cause)))

  // Leaving stops nothing in the server: being told to stop, or the terminal
  // window closing, closes the connection only.
  for (const signal of ["SIGTERM", "SIGHUP"] as const) {
    process.on(signal, () => {
      void evie.close().finally(() => process.exit(EXIT_CODES[signal]))
    })
  }

  // Ctrl-C while the connection is down stops trying, and leaves, whatever
  // the terminal client was waiting for. Nothing in the server is stopped.
  const leave = () => {
    note("Stopped trying to reach Evie's server. `evie -c` comes back to this session.")
    restore()
    void evie.close().finally(() => process.exit(EXIT_CODES.SIGINT))
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
        if (down) return leave()
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
          onNote: (show) => (note = show),
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
        if (down) return leave()
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
        onNote: (show) => (note = show),
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

// How long evie, when nobody types in it, goes on trying to reach the
// server after the line drops: two minutes, which rides out a restart.
// `EVIE_GIVE_UP_AFTER`, in seconds, changes it, for tests.
const giveUpAfter = (env: Settings) => (Number(env.EVIE_GIVE_UP_AFTER) || 120) * 1000

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
  if (own !== undefined) return { token: own, own: true }
  // No token file: either no server has run here yet, or the file was removed
  // after one started, as the refusal of a token taken back advises.
  if (await evieAnswers(address)) {
    fail(
      `An Evie server is running at ${new URL(address).origin}, but this computer's token file, ${ownTokenFile(dataFolder(env))}, ` +
        "is missing. Stopping that server and starting `evie serve` again makes a new one.",
    )
  }
  return fail("No Evie server has run on this computer yet. Start one with `evie serve`, which also makes this computer's token.")
}

// Whether an Evie server answers at the address. Asked with no token, it
// says that one is needed.
async function evieAnswers(address: string): Promise<boolean> {
  try {
    return (await fetch(new URL(PATHS.check, address), { signal: AbortSignal.timeout(3000) })).status === 401
  } catch {
    return false
  }
}
