import type { SessionState, SessionSummary } from "../../src/core/types.ts"
import type { Selected } from "./address.ts"

// The session list's rules, apart from its screen.

// The list in its groups: at the top the sessions waiting for the owner,
// then the active ones; below, the working ones and the settled ones, in
// groups that open and close. Each is newest first.
export type Groups = { top: SessionSummary[]; working: SessionSummary[]; settled: SessionSummary[] }

export function groupSessions(sessions: readonly SessionSummary[]): Groups {
  const newest = [...sessions].sort((a, b) => b.lastActive - a.lastActive)
  const inState = (state: SessionSummary["state"]) => newest.filter((each) => each.state === state)
  return { top: [...inState("needs_you"), ...inState("active")], working: inState("working"), settled: inState("settled") }
}

// What a session is called in the list: its name, which is the opening
// words of its first message until it is named, or for a session with no
// message yet, such as one just started, "New session".
export const titleOf = (session: SessionSummary) => session.name ?? "New session"

// The sessions a search keeps: those whose title and project's name,
// together, have every word of it in them, in any case, so that a project's
// name narrows the list to it. `projectName` names a project by its id, or
// for null "No project", or gives nothing for one not listed yet.
export function filterSessions(sessions: readonly SessionSummary[], search: string, projectName: (project: string | null) => string | undefined): SessionSummary[] {
  const words = search.toLowerCase().split(/\s+/).filter((word) => word !== "")
  return sessions.filter((session) => {
    const named = `${titleOf(session)} ${projectName(session.project) ?? ""}`.toLowerCase()
    return words.every((word) => named.includes(word))
  })
}

// What saving a name typed in a row comes to: the name to send, on one
// line and without the spaces at its ends; why not, when it is blank; or
// nothing to do, when it is the title the session has already.
export function nameToSave(typed: string, title: string): { name: string } | { refused: string } | undefined {
  const name = typed.replace(/\s+/g, " ").trim()
  if (name === "") return { refused: "A session's name cannot be blank, so it keeps the one it has." }
  return name === title ? undefined : { name }
}

// What a key pressed in a row's name field does: Enter saves the name and
// Escape gives it up, as in T3 Code; any other key is typing, and so is one
// that is part of composing a character, as in Japanese.
export function nameFieldKey(key: string, composing: boolean): "save" | "cancel" | undefined {
  if (composing) return undefined
  return key === "Enter" ? "save" : key === "Escape" ? "cancel" : undefined
}

// What F2 renames: while the list is shown, the row that has the
// keyboard's focus, or else the selected session's row; with the list
// hidden, as when it is a sheet at phone width, the selected session in
// the header above its conversation, which is in sight. `focusedRow` is
// null for a row with no session to rename, a draft's, which F2 leaves
// alone. Not while the owner is typing in a field, where F2 is the field's.
export function renamedByF2(focus: { focusedRow?: string | null; typing: boolean; listShown: boolean }, selected: string | undefined): { row: string } | { header: true } | undefined {
  if (focus.typing || focus.focusedRow === null) return undefined
  if (!focus.listShown) return selected === undefined ? undefined : { header: true }
  const row = focus.focusedRow ?? selected
  return row === undefined ? undefined : { row }
}

// The selected session, when it is one of those deleted: it may be shown
// in the middle, or one of its subagents' conversations, or in the card.
export function deletedSelection(selected: Selected | undefined, deleted: readonly string[]): string | undefined {
  if (selected === undefined || !("session" in selected)) return undefined
  return deleted.includes(selected.session) ? selected.session : undefined
}

// The project being looked at, whose workspace the Workspaces button
// opens: the selected draft's or session's, or with nothing selected, the
// latest active session's, or none. A session not in the list is taken to
// have none. A new session does not start in it: it starts with no project.
export function projectLookedAt(selected: Selected | undefined, sessions: readonly SessionSummary[]): string | null {
  if (selected === undefined) return latest(sessions)?.project ?? null
  if ("draft" in selected) return selected.draft
  return sessions.find((each) => each.session === selected.session)?.project ?? null
}

// The workspace the Workspaces button at the top of the list opens: the
// project being looked at's, or else that of the latest active session in
// a project, or else the first project's; with no project, none.
export function workspaceToOpen(lookingAt: string | null, sessions: readonly SessionSummary[], projects: readonly { id: string }[]): string | undefined {
  const listed = (project: string | null | undefined) => projects.some((each) => each.id === project)
  if (listed(lookingAt)) return lookingAt!
  return latest(sessions.filter((each) => listed(each.project)))?.project ?? projects[0]?.id
}

// The session last active, or none.
const latest = (sessions: readonly SessionSummary[]) => sessions.reduce<SessionSummary | undefined>((best, each) => (best === undefined || each.lastActive > best.lastActive ? each : best), undefined)

// The six project colours, by their names in the design system. Until the
// owner picks a project's colour, each project is given one from its id,
// so it keeps it from one visit to the next.
export const PROJECT_COLOURS = ["moss", "mustard", "maroon", "rust", "slate", "plum"] as const
export type ProjectColour = (typeof PROJECT_COLOURS)[number]

// FNV-1a, a small hash that spreads ids that differ in one letter.
export function projectColour(id: string): ProjectColour {
  let hash = 0x811c9dc5
  for (let at = 0; at < id.length; at++) hash = Math.imul(hash ^ id.charCodeAt(at), 0x01000193)
  return PROJECT_COLOURS[(hash >>> 0) % PROJECT_COLOURS.length]!
}

// How long ago a time was, as a row says it: "now" within the minute, then
// in its largest whole unit, as "5m", "3h", "2d", "6w" or "1y". A time
// after `now`, as when the browser's clock is behind the server's, is now.
export function ageWords(time: number, now: number): string {
  const minutes = Math.floor((now - time) / 60_000)
  if (minutes < 1) return "now"
  const [unit, size] = AGE_UNITS.find(([, size]) => minutes >= size)!
  return `${Math.floor(minutes / size)}${unit}`
}
// Each unit and its length in minutes, the largest first.
const AGE_UNITS: [string, number][] = [
  ["y", 365 * 24 * 60],
  ["w", 7 * 24 * 60],
  ["d", 24 * 60],
  ["h", 60],
  ["m", 1],
]

// Which of the groups at the bottom are open, as this browser last left
// them. Working starts open and Settled closed.
export type OpenGroups = { working: boolean; settled: boolean }

// The groups to have open once the selected session's state has gone from
// `before` to `after`: a closed group it moved into is opened, so that the
// session stays in sight, as when the owner settles it. The same groups
// when it has not moved from one group to another, or moved to the top.
export function revealMove(open: OpenGroups, before: SessionState | undefined, after: SessionState): OpenGroups {
  const group = groupOf(after)
  if (before === undefined || group === groupOf(before) || group === "top" || open[group]) return open
  return { ...open, [group]: true }
}
const groupOf = (state: SessionState) => (state === "working" || state === "settled" ? state : "top")

// Where this browser keeps them.
const GROUPS_KEY = "evie.groups"

// A browser can refuse to keep anything, as in a private window, and then
// the groups open as they start.
export function openGroups(): OpenGroups {
  try {
    const kept = JSON.parse(localStorage.getItem(GROUPS_KEY) ?? "{}") as Partial<OpenGroups>
    return { working: kept.working !== false, settled: kept.settled === true }
  } catch {
    return { working: true, settled: false }
  }
}

export function keepOpenGroups(groups: OpenGroups) {
  try {
    localStorage.setItem(GROUPS_KEY, JSON.stringify(groups))
  } catch {
    // Not kept: the next visit opens them as they start.
  }
}

// The sessions shown, in the list's order, by their ids: the top ones,
// then those of each group that is open. mod+1 to mod+9 jump to the first
// nine, and the previous and next session are moved to through them.
export function inListOrder(groups: Groups, open: OpenGroups): string[] {
  return [...groups.top, ...(open.working ? groups.working : []), ...(open.settled ? groups.settled : [])].map((each) => each.session)
}

// The session above or below `current` in `order`, as T3 Code moves
// through its threads: none past either end, nor from a session not shown,
// and with none selected, as with a draft, the last or the first.
export function besides(order: readonly string[], current: string | undefined, direction: "previous" | "next"): string | undefined {
  if (current === undefined) return direction === "previous" ? order.at(-1) : order[0]
  const at = order.indexOf(current)
  if (at === -1) return undefined
  return order[direction === "previous" ? at - 1 : at + 1]
}
