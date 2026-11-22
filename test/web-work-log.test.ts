import { expect, test } from "bun:test"
import { runSummary, workedSeconds, workingFor } from "../web/src/work-log.ts"

// The work log, as T3 Code writes it, apart from its screen, beside each
// call's own sentence (web-step-words.test.ts): a run of calls in one
// sentence that counts them by kind, and how long a turn has worked while
// it runs.

const call = (name: string, input: Record<string, unknown>) => ({ name, arguments: JSON.stringify(input) })

test("a run of calls counts what it did by kind, the two that matter most in the order they came, and the rest as other actions", () => {
  const read = call("read", { path: "a.ts" })
  const shell = call("shell", { command: "bun test" })

  expect(runSummary([read, read, shell])).toBe("Read 2 files and ran 1 command")
  expect(runSummary([shell])).toBe("Ran 1 command")
  // Edits count the files they changed, once each.
  expect(runSummary([call("edit", { path: "a.ts" }), call("edit", { path: "a.ts" }), call("write", { path: "b.ts" })])).toBe("Changed 2 files")
  // Commands and edits come before reading and searching; anything else last.
  expect(runSummary([read, call("grep", { pattern: "x" }), call("edit", { path: "a.ts" })])).toBe("Read 1 file, changed 1 file and performed 1 other action")
  expect(runSummary([call("deploy", {}), call("web_search", { query: "x" }), call("web_search", { query: "y" }), call("subagent", { description: "d" })])).toBe(
    "Searched the web 2 times, asked 1 subagent and performed 1 other action",
  )
})

test("how long a running turn has worked leaves out the time it has waited for the owner, and stops counting while it waits", () => {
  const started = 1_000_000

  expect(workedSeconds({ started, waited: 0 }, started + 12_400)).toBe(12)
  // Twenty seconds waited for an answer earlier in the turn are not work.
  expect(workedSeconds({ started, waited: 20_000 }, started + 32_000)).toBe(12)
  // While a question waits, the time stands where it was asked.
  expect(workedSeconds({ started, waited: 0, waitingSince: started + 5_000 }, started + 60_000)).toBe(5)
  // A browser clock behind the server's counts nothing.
  expect(workedSeconds({ started, waited: 0 }, started - 3_000)).toBe(0)
})

test("a running turn says how long it has worked as T3 Code's does: in seconds, then minutes and seconds", () => {
  expect([0, 12, 59, 60, 64, 3725].map(workingFor)).toEqual(["Working for 0s", "Working for 12s", "Working for 59s", "Working for 1m", "Working for 1m 4s", "Working for 1h 2m"])
})
