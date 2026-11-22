import type { SessionSummary } from "../../src/core/types.ts"

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

// What the list is narrowed to: the sessions whose title has every word of
// `search` in it, in any case, and those of one project, or with `project`
// null those of no project. Without `project`, every project's.
export type Filter = { search: string; project?: string | null }

export function filterSessions(sessions: readonly SessionSummary[], filter: Filter): SessionSummary[] {
  const words = filter.search.toLowerCase().split(/\s+/).filter((word) => word !== "")
  return sessions.filter((session) => {
    if (filter.project !== undefined && session.project !== filter.project) return false
    const title = titleOf(session).toLowerCase()
    return words.every((word) => title.includes(word))
  })
}

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
