import { expect, test } from "bun:test"
import type { PullRequest } from "../src/core/types.ts"
import { checkWord, checksSummary, pullRequestWord } from "../web/src/pull-request.ts"

// How the project card says a pull request stands, in words, as T3 Code's
// pull request row and its checks say it.

type Check = PullRequest["checks"][number]
const checks = (...states: Check["state"][]): Check[] => states.map((state, at) => ({ name: `check ${at + 1}`, state }))

test("a pull request's state is a word", () => {
  expect((["open", "draft", "merged", "closed"] as const).map(pullRequestWord)).toEqual(["Open", "Draft", "Merged", "Closed"])
})

test("the checks are summed up by what most needs looking at: failing, then waiting, then running, then passing", () => {
  expect(checksSummary(checks("passed", "failed", "running", "failed", "passed"))).toBe("2 of 5 failing")
  // A cancelled check has not passed, and counts as failing.
  expect(checksSummary(checks("passed", "cancelled"))).toBe("1 of 2 failing")
  expect(checksSummary(checks("passed", "waiting", "running"))).toBe("1 awaiting action")
  expect(checksSummary(checks("passed", "running", "running"))).toBe("2 of 3 running")
  expect(checksSummary(checks("passed", "passed"))).toBe("All checks passed")
  expect(checksSummary(checks("passed", "skipped", "neutral"))).toBe("1 of 3 passing")
  expect(checksSummary([])).toBe("No checks")
})

test("each check says how it stands in a word", () => {
  expect((["passed", "failed", "running", "waiting", "cancelled", "skipped", "neutral"] as const).map(checkWord)).toEqual([
    "Passed",
    "Failed",
    "Running",
    "Awaiting action",
    "Cancelled",
    "Skipped",
    "Neutral",
  ])
})
