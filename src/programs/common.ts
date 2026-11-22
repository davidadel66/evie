import { ConnectError, dataFolder, type Settings } from "../index.ts"
import { ownTokenFile, readOwnToken } from "../server/tokens.ts"
import { PATHS } from "../server/wire.ts"
import { loadServerSettings } from "../settings-files.ts"

// What the programs share: printing, stopping with a reason, the exit code
// for each signal that stops them, and how a client reaches the server.

// What went wrong, in words.
export const reason = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// A problem that stops the command, such as a plugin that cannot be loaded,
// is printed with its reason.
export const fail = (cause: unknown): never => {
  console.error(reason(cause))
  process.exit(1)
}

// Prints to the terminal. If whatever was reading Evie's output has gone
// away, as when it is piped into a program that stops early, there is no one
// left to print for, so Evie quits quietly.
export function print(text: string) {
  try {
    process.stdout.write(text)
  } catch (cause) {
    if ((cause as { code?: string }).code === "EPIPE") process.exit(0)
    throw cause
  }
}

// The code a program leaves with when a signal stops it: 128 and the
// signal's number, as a shell expects.
export const EXIT_CODES = { SIGINT: 130, SIGTERM: 143, SIGHUP: 129 } as const
export type StopSignal = keyof typeof EXIT_CODES

// The file that tells a server it is being stopped for good, as when its
// login service is removed, rather than to be started again. It is written,
// empty, in the server's data folder just before the server is stopped, and
// the server removes it as it stops.
export const STOP_FOR_GOOD = "stop-for-good"

// ---- Reaching the server ---------------------------------------------------------------------

// Where the server is, the token to connect with, and why a connection
// failed, in words. The server is at the address in the settings, or
// `EVIE_SERVER`; the token is `EVIE_TOKEN`, or this computer's own. When the
// server refused this computer's own token, that was taken back, or made by
// a server whose data folder has since changed, and the way to a new one is
// given. `own` says the token is this computer's own.
export async function reachServer(env: Settings): Promise<{ address: string; token: string; own: boolean; why(cause: unknown): string }> {
  const { address } = await loadServerSettings({ env }).catch(fail)
  const { token, own } = await tokenFor(env, address)
  const why = (cause: unknown) =>
    own && cause instanceof ConnectError && cause.reason === "token"
      ? `The Evie server at ${new URL(address).origin} refused this computer's token. Removing ` +
        `${ownTokenFile(dataFolder(env))} and starting \`evie serve\` again makes a new one.`
      : reason(cause)
  return { address, token, own, why }
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
