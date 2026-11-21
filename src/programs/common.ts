// What the programs share: printing, stopping with a reason, and the exit
// code for each signal that stops them.

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

// The file in the data folder that tells a stopping `evie serve` it is
// stopped for good, as when its login service is removed, rather than
// restarted. It is left by whatever stops the server, and removed by it.
export const STOP_FOR_GOOD = "stop-for-good"

// The code a program leaves with when a signal stops it: 128 and the
// signal's number, as a shell expects.
export const EXIT_CODES = { SIGINT: 130, SIGTERM: 143, SIGHUP: 129 } as const
export type StopSignal = keyof typeof EXIT_CODES
