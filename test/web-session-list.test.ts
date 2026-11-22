import { expect, test } from "bun:test"
import type { SessionState, SessionSummary } from "../src/core/types.ts"
import { addressAsOpened, addressOf, selectedIn } from "../web/src/address.ts"
import { ageWords, filterSessions, groupSessions, PROJECT_COLOURS, projectColour } from "../web/src/session-list.ts"

// The session list's own rules, apart from its screen: how sessions are
// grouped and ordered, and found by search and by project, and how a
// project is coloured, how long ago a session was active, and how the
// selected session is kept in the page's address.

const MINUTE = 60_000

// A session last active `ago` minutes before `NOW`.
const NOW = Date.UTC(2026, 10, 21, 12)
const summary = (session: string, state: SessionState, ago: number, project: string | null = "p1", name: string | null = session): SessionSummary => ({
  session,
  project,
  name,
  state,
  lastActive: NOW - ago * MINUTE,
})
const ids = (sessions: SessionSummary[]) => sessions.map((each) => each.session)

test("sessions that need the owner come first, then the active ones, each newest first, and working and settled ones are groups of their own, newest first", () => {
  const sessions = [
    summary("active-old", "active", 300),
    summary("settled-new", "settled", 20_000),
    summary("needs-old", "needs_you", 90),
    summary("working-old", "working", 60),
    summary("active-new", "active", 5),
    summary("needs-new", "needs_you", 30),
    summary("working-new", "working", 1),
    summary("settled-old", "settled", 40_000),
  ]

  const groups = groupSessions(sessions)

  expect(ids(groups.top)).toEqual(["needs-new", "needs-old", "active-new", "active-old"])
  expect(ids(groups.working)).toEqual(["working-new", "working-old"])
  expect(ids(groups.settled)).toEqual(["settled-new", "settled-old"])
})

test("a search keeps the sessions whose name has every word of it, in any case and any order, and a session with no name yet is found as a new session", () => {
  const sessions = [
    summary("plants", "active", 1, "p1", "Keep a list of what to plant"),
    summary("soup", "active", 2, "p1", "Lentil soup for four"),
    summary("unnamed", "active", 3, "p1", null),
  ]

  expect(ids(filterSessions(sessions, { search: "  PLANT list " }))).toEqual(["plants"])
  expect(ids(filterSessions(sessions, { search: "soup plant" }))).toEqual([])
  expect(ids(filterSessions(sessions, { search: "new" }))).toEqual(["unnamed"])
  expect(ids(filterSessions(sessions, { search: "" }))).toEqual(["plants", "soup", "unnamed"])
})

test("a project filter keeps that project's sessions, or with none those of no project, and without one keeps them all", () => {
  const sessions = [summary("in-p1", "active", 1, "p1"), summary("in-p2", "active", 2, "p2"), summary("general", "active", 3, null)]

  expect(ids(filterSessions(sessions, { search: "", project: "p2" }))).toEqual(["in-p2"])
  expect(ids(filterSessions(sessions, { search: "", project: null }))).toEqual(["general"])
  expect(ids(filterSessions(sessions, { search: "" }))).toEqual(["in-p1", "in-p2", "general"])
  expect(ids(filterSessions(sessions, { search: "general", project: "p1" }))).toEqual([])
})

test("a project's colour is one of the six project colours, the same each time for the same id, and ids spread over all six", () => {
  const id = "0b9f5c8e-3f43-4f6a-9d1e-6a2c1d7e9b10"
  const many = Array.from({ length: 60 }, (_, at) => projectColour(`00000000-0000-4000-8000-0000000000${String(at).padStart(2, "0")}`))

  expect(PROJECT_COLOURS).toEqual(["moss", "mustard", "maroon", "rust", "slate", "plum"])
  expect(projectColour(id)).toBe(projectColour(String(id)))
  expect(PROJECT_COLOURS).toContain(projectColour(id))
  expect(new Set(many).size).toBe(6)
})

test("how long ago a session was active is said in its largest whole unit, shortly: now, minutes, hours, days, weeks, then years", () => {
  const HOUR = 60 * MINUTE
  const DAY = 24 * HOUR
  const at = (ago: number) => ageWords(NOW - ago, NOW)

  expect([at(0), at(59_000), at(MINUTE), at(59 * MINUTE + 59_000)]).toEqual(["now", "now", "1m", "59m"])
  expect([at(HOUR), at(23 * HOUR + 59 * MINUTE)]).toEqual(["1h", "23h"])
  expect([at(DAY), at(6 * DAY + 23 * HOUR)]).toEqual(["1d", "6d"])
  expect([at(7 * DAY), at(364 * DAY)]).toEqual(["1w", "52w"])
  expect([at(365 * DAY), at(800 * DAY)]).toEqual(["1y", "2y"])
  // The browser's clock can be a little behind the server's.
  expect(at(-5 * MINUTE)).toBe("now")
})

test("the selected session is kept after # in the address, and an address naming none, or something else, selects none", () => {
  expect(addressOf("a1b2-c3")).toBe("#session=a1b2-c3")
  expect(selectedIn(addressOf("a1b2-c3"))).toBe("a1b2-c3")
  expect(selectedIn("")).toBeUndefined()
  expect(selectedIn("#code=abc")).toBeUndefined()
  expect(selectedIn("#session=")).toBeUndefined()
})

test("as the page opens, it is put at its root with a sign-in code taken out of its address, and a selected session kept", () => {
  expect(addressAsOpened("#code=abc")).toBe("/")
  expect(addressAsOpened("#session=s1")).toBe("/#session=s1")
  expect(addressAsOpened("")).toBe("/")
})
