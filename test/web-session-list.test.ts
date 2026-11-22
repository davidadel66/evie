import { expect, test } from "bun:test"
import type { SessionState, SessionSummary } from "../src/core/types.ts"
import { addressAsOpened, addressOf, lookedAt, selectedIn } from "../web/src/address.ts"
import { refreshWhileShown } from "../web/src/refresh.ts"
import { ageWords, besides, deletedSelection, filterSessions, groupSessions, inListOrder, nameFieldKey, nameToSave, PROJECT_COLOURS, projectColour, projectLookedAt, renamedByF2, revealMove, workspaceToOpen } from "../web/src/session-list.ts"

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

// The projects' names, as the list knows them.
const NAMES: Record<string, string> = { p1: "garden-planner", p2: "recipe-box" }
const nameOf = (project: string | null) => (project === null ? "No project" : NAMES[project])

test("a search keeps the sessions whose name has every word of it, in any case and any order, and a session with no name yet is found as a new session", () => {
  const sessions = [
    summary("plants", "active", 1, "p1", "Keep a list of what to plant"),
    summary("soup", "active", 2, "p1", "Lentil soup for four"),
    summary("unnamed", "active", 3, "p1", null),
  ]

  expect(ids(filterSessions(sessions, "  PLANT list ", nameOf))).toEqual(["plants"])
  expect(ids(filterSessions(sessions, "soup plant", nameOf))).toEqual([])
  expect(ids(filterSessions(sessions, "new", nameOf))).toEqual(["unnamed"])
  expect(ids(filterSessions(sessions, "", nameOf))).toEqual(["plants", "soup", "unnamed"])
})

test("a search matches the project's name as well as the session's, so it narrows the list to a project", () => {
  const sessions = [summary("beds", "active", 1, "p1", "Plan the beds"), summary("cake", "active", 2, "p2", "Apple cake"), summary("general", "active", 3, null, "Back up SQLite")]

  expect(ids(filterSessions(sessions, "recipe", nameOf))).toEqual(["cake"])
  expect(ids(filterSessions(sessions, "garden beds", nameOf))).toEqual(["beds"])
  expect(ids(filterSessions(sessions, "no project", nameOf))).toEqual(["general"])
  // A project not listed yet is matched by the session's name alone.
  expect(ids(filterSessions(sessions, "apple", () => undefined))).toEqual(["cake"])
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
  expect(addressOf({ session: "a1b2-c3" })).toBe("#session=a1b2-c3")
  expect(selectedIn(addressOf({ session: "a1b2-c3" }))).toEqual({ session: "a1b2-c3" })
  expect(selectedIn("")).toBeUndefined()
  expect(selectedIn("#code=abc")).toBeUndefined()
  expect(selectedIn("#session=")).toBeUndefined()
})

test("a subagent of the selected session opened in the middle is kept in the address beside it", () => {
  expect(addressOf({ session: "s1", subagent: "c1" })).toBe("#session=s1&subagent=c1")
  expect(selectedIn("#session=s1&subagent=c1")).toEqual({ session: "s1", subagent: "c1" })
  expect(selectedIn("#session=s1&subagent=")).toEqual({ session: "s1" })
  expect(selectedIn("#subagent=c1")).toBeUndefined()
  expect(addressAsOpened("#session=s1&subagent=c1&code=abc")).toBe("/#session=s1&subagent=c1")
})

test("a new session not yet started is a draft, kept in the address by its project, or by none", () => {
  expect(addressOf({ draft: "p1" })).toBe("#draft=p1")
  expect(addressOf({ draft: null })).toBe("#draft=none")
  expect(selectedIn("#draft=p1")).toEqual({ draft: "p1" })
  expect(selectedIn("#draft=none")).toEqual({ draft: null })
  expect(selectedIn("#draft=")).toBeUndefined()
})

test("as the page opens, it is put at its root with a sign-in code taken out of its address, and a selected session or draft kept", () => {
  expect(addressAsOpened("#code=abc")).toBe("/")
  expect(addressAsOpened("#session=s1")).toBe("/#session=s1")
  expect(addressAsOpened("#draft=none")).toBe("/#draft=none")
  expect(addressAsOpened("")).toBe("/")
})

test("the project being looked at is the selected draft's or session's, or with nothing open, the latest active session's, as T3 Code's new thread is, or none, so New session again keeps to the same draft", () => {
  const sessions = [summary("general", "active", 2, null), summary("in-p1", "active", 1, "p1")]

  expect(projectLookedAt({ draft: "p2" }, sessions)).toBe("p2")
  expect(projectLookedAt({ draft: null }, sessions)).toBeNull()
  expect(projectLookedAt({ session: "in-p1" }, sessions)).toBe("p1")
  expect(projectLookedAt({ session: "general" }, sessions)).toBeNull()
  expect(projectLookedAt({ session: "not-listed" }, sessions)).toBeNull()
  expect(projectLookedAt(undefined, sessions)).toBe("p1")
  expect(projectLookedAt(undefined, [summary("general", "active", 2, null)])).toBeNull()
  expect(projectLookedAt(undefined, [])).toBeNull()
  // Pressed again on a draft, New session comes back to the same draft.
  const draft = { draft: "p2" }
  expect(addressOf({ draft: projectLookedAt(draft, sessions) })).toBe(addressOf(draft))
})

test("the Workspaces button opens the workspace of the project being looked at, or else of the latest active session in one, or else the first project, and with no project none", () => {
  const projects = [{ id: "p1" }, { id: "p2" }]
  const sessions = [summary("general", "active", 1, null), summary("in-p2", "active", 5, "p2")]

  expect(workspaceToOpen("p1", sessions, projects)).toBe("p1")
  expect(workspaceToOpen(null, sessions, projects)).toBe("p2")
  expect(workspaceToOpen(null, [], projects)).toBe("p1")
  // A project the list no longer has is not opened.
  expect(workspaceToOpen("gone", [], projects)).toBe("p1")
  expect(workspaceToOpen(null, sessions, [])).toBeUndefined()
})

test("when the selected session moves into a closed group, that group opens so that it stays in sight, and nothing else opens or closes", () => {
  const closed = { working: false, settled: false }

  expect(revealMove(closed, "active", "settled")).toEqual({ working: false, settled: true })
  expect(revealMove(closed, "needs_you", "working")).toEqual({ working: true, settled: false })
  expect(revealMove(closed, "settled", "working")).toEqual({ working: true, settled: false })
  // Within its group, or to the top, or when the owner closes the group it is in, nothing changes.
  expect(revealMove(closed, "settled", "settled")).toBe(closed)
  expect(revealMove(closed, "settled", "active")).toBe(closed)
  expect(revealMove(closed, "working", "needs_you")).toBe(closed)
  // Seen for the first time, as when the list is first drawn, it has not moved.
  expect(revealMove(closed, undefined, "settled")).toBe(closed)
  const open = { working: true, settled: true }
  expect(revealMove(open, "active", "settled")).toBe(open)
})

test("what goes stale is asked for again when the window has the focus back, and at each interval while the page is shown, not while it is hidden", async () => {
  const window = new EventTarget()
  let shown = true
  let asked = 0
  const stop = refreshWhileShown(() => asked++, { window, shown: () => shown, every: 40 })

  window.dispatchEvent(new Event("focus"))
  expect(asked).toBe(1)
  await Bun.sleep(100)
  expect(asked).toBe(3)

  shown = false
  await Bun.sleep(100)
  expect(asked).toBe(3)

  shown = true
  stop()
  window.dispatchEvent(new Event("focus"))
  await Bun.sleep(100)
  expect(asked).toBe(3)
})

test("a name typed in a row is saved without the spaces at its ends, on one line; a blank one is refused, saying why, and the name it has already is not sent again", () => {
  expect(nameToSave("  Garden\n beds  ", "Plan the beds for spring")).toEqual({ name: "Garden beds" })
  expect(nameToSave(" \t ", "Plan the beds for spring")).toEqual({ refused: "A session's name cannot be blank, so it keeps the one it has." })
  expect(nameToSave(" Plan the beds for spring ", "Plan the beds for spring")).toBeUndefined()
})

test("in a row's name field, Enter saves and Escape gives up, and a key that is part of composing a character is typing", () => {
  expect(nameFieldKey("Enter", false)).toBe("save")
  expect(nameFieldKey("Escape", false)).toBe("cancel")
  expect(nameFieldKey("a", false)).toBeUndefined()
  expect(nameFieldKey("Enter", true)).toBeUndefined()
})

test("F2 renames, while the list is shown, the row that has the keyboard's focus, or else the selected session, but not a draft's row, nor while the owner types in a field", () => {
  const shown = { typing: false, listShown: true }
  expect(renamedByF2({ ...shown, focusedRow: "s2" }, "s1")).toEqual({ row: "s2" })
  expect(renamedByF2(shown, "s1")).toEqual({ row: "s1" })
  // A draft's row has the focus: it has no session to rename.
  expect(renamedByF2({ ...shown, focusedRow: null }, "s1")).toBeUndefined()
  expect(renamedByF2({ ...shown, typing: true }, "s1")).toBeUndefined()
  expect(renamedByF2(shown, undefined)).toBeUndefined()
})

test("with the list hidden, as in its sheet at phone width, F2 renames the selected session in the header, which is in sight", () => {
  const hidden = { typing: false, listShown: false }
  expect(renamedByF2(hidden, "s1")).toEqual({ header: true })
  expect(renamedByF2({ ...hidden, typing: true }, "s1")).toBeUndefined()
  expect(renamedByF2(hidden, undefined)).toBeUndefined()
})

test("a selected session that has been deleted is found, and so is one whose subagent's conversation is open, so the page can let it go", () => {
  expect(deletedSelection({ session: "s1" }, ["s0", "s1"])).toBe("s1")
  expect(deletedSelection({ session: "s1", subagent: "a1" }, ["s1"])).toBe("s1")
  expect(deletedSelection({ session: "s2" }, ["s1"])).toBeUndefined()
  expect(deletedSelection({ draft: null }, ["s1"])).toBeUndefined()
  expect(deletedSelection(undefined, ["s1"])).toBeUndefined()
})

test("the sessions in the list's order, for the shortcuts that move through it, are those shown: the top, then each open group's", () => {
  const groups = groupSessions([summary("a", "active", 1), summary("b", "working", 2), summary("c", "settled", 3), summary("d", "needs_you", 4)])
  expect(inListOrder(groups, { working: true, settled: false })).toEqual(["d", "a", "b"])
  expect(inListOrder(groups, { working: false, settled: true })).toEqual(["d", "a", "c"])
})

test("the previous and next session are the ones above and below the selected one, none past either end, nor from one not shown, and with none selected the last and the first, as in T3 Code", () => {
  const order = ["a", "b", "c"]
  expect(besides(order, "b", "previous")).toBe("a")
  expect(besides(order, "b", "next")).toBe("c")
  expect(besides(order, "a", "previous")).toBeUndefined()
  expect(besides(order, "c", "next")).toBeUndefined()
  expect(besides(order, undefined, "previous")).toBe("c")
  expect(besides(order, undefined, "next")).toBe("a")
  expect(besides(order, "not shown", "next")).toBeUndefined()
  expect(besides([], undefined, "next")).toBeUndefined()
})

test("what the owner is looking at is the session view's session, draft or subagent, or in a workspace, its side chat's session or new session", () => {
  expect(lookedAt("#session=s1")).toEqual({ session: "s1", subagent: false })
  expect(lookedAt("#session=s1&subagent=a1")).toEqual({ session: "s1", subagent: true })
  expect(lookedAt("#draft=p1")).toEqual({ draft: true })
  expect(lookedAt("#workspace=p1&tab=sessions&chat=s2")).toEqual({ session: "s2", subagent: false })
  expect(lookedAt("#workspace=p1&tab=storage&chat=new")).toEqual({ draft: true })
  expect(lookedAt("#workspace=p1&tab=sessions")).toBeUndefined()
  expect(lookedAt("")).toBeUndefined()
})
