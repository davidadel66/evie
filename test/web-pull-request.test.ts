import { expect, test } from "bun:test"
import type { PullRequest } from "../src/core/types.ts"
import { checkLook, checkWord, checksSummary, fixRequest, pullRequestWord } from "../web/src/pull-request.ts"

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

test("each check is drawn as the design system's Status draws a state: passed as Done, failed as Failed, running as Working, the rest in quiet words", () => {
  expect((["passed", "failed", "running", "waiting", "cancelled", "skipped", "neutral"] as const).map(checkLook)).toEqual(["done", "failed", "working", "quiet", "quiet", "quiet", "quiet"])
})

test("Fix asks evie, in words, to fix the checks failing on an open pull request, by name, and is not offered while none fails", () => {
  const pull = { number: 12, title: "Add a breads section", state: "open" as const, base: "main", checks: [...checks("passed", "failed", "running", "cancelled", "passed")] }
  expect(fixRequest(pull)).toBe(
    'Fix the failing checks on pull request #12, "Add a breads section": check 2 and check 4. Read what each one says, fix what made it fail, then commit the fix and push it.',
  )
  expect(fixRequest({ ...pull, checks: checks("passed", "failed") })).toContain(": check 2. Read")
  expect(fixRequest({ ...pull, checks: checks("passed", "running") })).toBeUndefined()
  expect(fixRequest({ ...pull, state: "merged" })).toBeUndefined()
})
