import { expect, test } from "bun:test"
import type { SessionState, SessionSummary, StoredEvent } from "../src/core/types.ts"
import { latestTurnEnd, shownAs, watchSessions } from "../web/src/notices.ts"

// Notifications, as T3 Code's: which change in the sessions' summaries
// makes which notice, and when one is skipped.

const line = (session: string, state: SessionState, lastActive: number, name = "Plan the beds"): SessionSummary => ({ session, project: null, name, state, lastActive })
const ending = (seq: number, time: number, reason: "completed" | "error" | "cancelled") => ({ seq, time, reason })

test("a session that comes to wait for the owner needs them; one first seen waiting does not make a notice", () => {
  const watch = watchSessions()
  expect(watch.see([line("a", "needs_you", 10), line("b", "working", 10)])).toEqual([])
  expect(watch.see([line("a", "needs_you", 10), line("b", "needs_you", 12)])).toEqual([{ session: line("b", "needs_you", 12), change: "needs you" }])
  // Still waiting, as when a second question comes before the first is answered.
  expect(watch.see([line("a", "needs_you", 14), line("b", "needs_you", 13)])).toEqual([])
})

test("a session that was working, or waiting, and is no longer may have ended its turn, which its log says; any other change says nothing", () => {
  const watch = watchSessions()
  watch.see([line("a", "working", 10), line("b", "needs_you", 10), line("c", "active", 10)])
  expect(watch.see([line("a", "active", 20), line("b", "settled", 20), line("c", "settled", 20)])).toEqual([
    { session: line("a", "active", 20), change: "ended" },
    { session: line("b", "settled", 20), change: "ended" },
  ])
  expect(watch.see([line("a", "working", 30), line("b", "settled", 20), line("c", "active", 30)])).toEqual([])
})

test("a turn that completed has finished, one that failed has failed, and one the owner stopped says nothing", () => {
  const watch = watchSessions()
  watch.see([line("a", "active", 10), line("b", "active", 10), line("c", "active", 10)])
  expect(watch.ended("a", ending(5, 20, "completed"))).toBe("finished")
  expect(watch.ended("b", ending(5, 20, "error"))).toBe("failed")
  expect(watch.ended("c", ending(5, 20, "cancelled"))).toBeUndefined()
})

test("a turn's end is told once, and not at all when it ended before the page first saw the session, as when a background job it left running is stopped", () => {
  const watch = watchSessions()
  watch.see([line("a", "working", 10)])
  expect(watch.ended("a", ending(4, 10, "completed"))).toBeUndefined()
  expect(watch.ended("a", ending(7, 25, "completed"))).toBe("finished")
  expect(watch.ended("a", ending(7, 25, "completed"))).toBeUndefined()
  expect(watch.ended("a", undefined)).toBeUndefined()
  expect(watch.ended("never seen", ending(1, 30, "completed"))).toBeUndefined()
})

test("a session listed again after the line came back is compared with how it was last seen, and one deleted is forgotten", () => {
  const watch = watchSessions()
  watch.see([line("a", "working", 10), line("b", "working", 10)])
  expect(watch.see([line("a", "active", 30)])).toEqual([{ session: line("a", "active", 30), change: "ended" }])
  expect(watch.see([line("a", "active", 30), line("b", "needs_you", 40)])).toEqual([])
})

test("a turn's end is the latest turn_end in the session's log, with what went wrong when it failed", () => {
  const event = (seq: number, type: string, body: object) => ({ type, session: "a", seq, time: seq * 10, body }) as StoredEvent
  const events = [event(1, "turn_end", { reason: "error", error: "The model could not be reached." }), event(2, "user_message", { text: "Again" }), event(3, "turn_end", { reason: "completed" }), event(4, "named", { name: "Beds" })]
  expect(latestTurnEnd(events)).toEqual({ seq: 3, time: 30, reason: "completed" })
  expect(latestTurnEnd(events.slice(0, 2))).toEqual({ seq: 1, time: 10, reason: "error", error: "The model could not be reached." })
  expect(latestTurnEnd(events.slice(1, 2))).toBeUndefined()
})

test("with the window in front, a notice of another session is shown in the page, and of the session the owner is looking at, nothing", () => {
  const on = { mode: "both" as const, inApp: true }
  expect(shownAs({ session: "b", lookingAt: "a", focused: true, settings: on, allowed: true })).toEqual({ inPage: true, system: false, sound: true })
  expect(shownAs({ session: "a", lookingAt: "a", focused: true, settings: on, allowed: true })).toEqual({ inPage: false, system: false, sound: false })
  expect(shownAs({ session: "b", lookingAt: undefined, focused: true, settings: { mode: "off", inApp: true }, allowed: false })).toEqual({ inPage: true, system: false, sound: false })
  expect(shownAs({ session: "b", lookingAt: "a", focused: true, settings: { mode: "on", inApp: false }, allowed: true })).toEqual({ inPage: false, system: false, sound: false })
})

test("with the window behind others, a notice is the system's, of any session, when notifications are on and the browser allows them", () => {
  const away = { lookingAt: "a", focused: false, allowed: true }
  expect(shownAs({ ...away, session: "a", settings: { mode: "on", inApp: true } })).toEqual({ inPage: false, system: true, sound: false })
  expect(shownAs({ ...away, session: "b", settings: { mode: "both", inApp: true } })).toEqual({ inPage: false, system: true, sound: true })
  expect(shownAs({ ...away, session: "b", settings: { mode: "sound", inApp: true } })).toEqual({ inPage: false, system: false, sound: true })
  expect(shownAs({ ...away, session: "b", settings: { mode: "off", inApp: true } })).toEqual({ inPage: false, system: false, sound: false })
  expect(shownAs({ ...away, allowed: false, session: "b", settings: { mode: "on", inApp: true } })).toEqual({ inPage: false, system: false, sound: false })
})
