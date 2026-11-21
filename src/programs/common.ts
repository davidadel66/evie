// What the programs share: printing, stopping with a reason, and the exit
// code for each signal that stops them.

// A problem that stops the command, such as a plugin that cannot be loaded,
// is printed with its reason.
export const fail = (cause: unknown): never => {
  console.error(cause instanceof Error ? cause.message : String(cause))
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
