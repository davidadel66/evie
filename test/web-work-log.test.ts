import { expect, test } from "bun:test"
import { clockOffset, durationWords, workingFor } from "../web/src/work-log.ts"

// How long a turn has worked, as the work log says it, apart from its
// screen (the calls' own words are in web-step-words.test.ts): the time
// counted on the server's clock, as T3 Code counts it, and said as T3 Code
// says a duration.

test("a running turn says how long it has worked as T3 Code's timer does: whole seconds, then minutes and seconds", () => {
  expect([0, 999, 12_400, 59_999, 60_000, 64_000, 3_725_000].map(workingFor)).toEqual([
    "Working for 0s",
    "Working for 0s",
    "Working for 12s",
    "Working for 59s",
    "Working for 1m 0s",
    "Working for 1m 4s",
    "Working for 1h 2m 5s",
  ])
  // A browser clock ahead of the turn's start, as one not set right, counts nothing.
  expect(workingFor(-3000)).toBe("Working for 0s")
})

test("a duration is said as T3 Code says one", () => {
  expect([0, 850, 1000, 3400, 9960, 42_000, 59_600, 61_000].map(durationWords)).toEqual(["0ms", "850ms", "1s", "3.4s", "10s", "42s", "1m 0s", "1m 1s"])
  expect([3_600_000, 3_603_000, 3_780_000, 7_265_000].map(durationWords)).toEqual(["1h", "1h 3s", "1h 3m 0s", "2h 1m 5s"])
})

test("the server's clock is the browser's and an offset, the largest of each event's time less when it arrived, as the line only ever delays one", () => {
  // The server is 5s ahead, and each event took 20 to 200ms to come.
  const offset = [
    [10_000, 5_200],
    [20_000, 15_020],
    [30_000, 25_090],
  ].reduce<number | undefined>((known, [time, arrived]) => clockOffset(known, time!, arrived!), undefined)

  expect(offset).toBe(4_980)
})
