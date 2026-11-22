import type { PullRequest } from "../../src/core/types.ts"

// How the session details say how a branch's pull request stands, in words:
// its state, and its checks summed up, as T3 Code's pull request row says
// them. The summary's wording is adapted from T3 Code's
// `summarizePullRequestChecks` (MIT licence, notice in LICENSES/t3code-MIT.txt).

type Check = PullRequest["checks"][number]

export function pullRequestWord(state: PullRequest["state"]): string {
  return { open: "Open", draft: "Draft", merged: "Merged", closed: "Closed" }[state]
}

export function checkWord(state: Check["state"]): string {
  return { passed: "Passed", failed: "Failed", running: "Running", waiting: "Awaiting action", cancelled: "Cancelled", skipped: "Skipped", neutral: "Neutral" }[state]
}

// The checks in one line, by what most needs looking at: those failing,
// a cancelled one among them; those waiting for someone to act; those
// still running; or how many passed.
export function checksSummary(checks: readonly Check[]): string {
  if (checks.length === 0) return "No checks"
  const count = (...states: Check["state"][]) => checks.filter((check) => states.includes(check.state)).length
  const of = (some: number, word: string) => `${some} of ${checks.length} ${word}`
  const failing = count("failed", "cancelled")
  if (failing > 0) return of(failing, "failing")
  const waiting = count("waiting")
  if (waiting > 0) return `${waiting} awaiting action`
  const running = count("running")
  if (running > 0) return of(running, "running")
  const passed = count("passed")
  return passed === checks.length ? "All checks passed" : of(passed, "passing")
}

// How a check is drawn, as the design system's Status draws a state:
// passed as Done, with a check; failed as Failed, with its mark; running
// as Working, with the spinner; the rest in quiet words.
export function checkLook(state: Check["state"]): "done" | "failed" | "working" | "quiet" {
  return state === "passed" ? "done" : state === "failed" ? "failed" : state === "running" ? "working" : "quiet"
}

// What Fix asks evie, as a message in the session, as T3 Code's Fix hands
// the failing checks to a thread: the checks failing on the branch's pull
// request while it is open, by name. Nothing while none fails.
export function fixRequest(pull: Pick<PullRequest, "number" | "title" | "state" | "checks">): string | undefined {
  if (pull.state !== "open" && pull.state !== "draft") return undefined
  const failing = pull.checks.filter((check) => check.state === "failed" || check.state === "cancelled").map((check) => check.name)
  if (failing.length === 0) return undefined
  const names = failing.length === 1 ? failing[0] : `${failing.slice(0, -1).join(", ")} and ${failing.at(-1)}`
  return `Fix the failing checks on pull request #${pull.number}, "${pull.title}": ${names}. Read what each one says, fix what made it fail, then commit the fix and push it.`
}
