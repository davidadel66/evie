import type { SessionState, SessionSummary } from "../../src/core/types.ts"

// The command palette's rules, apart from its screen, as T3 Code's: it
// opens on the actions and the recent sessions, and what is typed searches
// the actions, every session and every project, ranked so that the best
// match comes first, and on an equal match, a command before a session.

// A row as the palette searches it: its name, and the words it is also known by.
export type Searched = { title: string; keywords?: readonly string[] }
export type Group<Item> = { name: string; items: Item[] }

// The groups narrowed to the rows that match every word typed, in any case
// and without minding accents, best first. A row whose whole name is what
// was typed matches best, then one whose name starts with it, then one
// with a word in its name that starts with each word, then one with each
// word, of four letters or more, anywhere in its name; and below all of
// those, the same in the words it is also known by. A group goes where its
// best row puts it, and on an equal match keeps its place, so that
// commands, given first, come before sessions. With nothing typed, the
// groups are as given.
export function rankGroups<Item extends Searched>(groups: readonly Group<Item>[], typed: string): Group<Item>[] {
  const query = fold(typed).split(/\s+/).filter((word) => word !== "").join(" ")
  if (query === "") return [...groups]
  const words = query.split(" ")
  return groups
    .map((group, at) => {
      const ranked = group.items
        .flatMap((item, place) => {
          const tier = tierOf(item, query, words)
          return tier === undefined ? [] : [{ item, tier, place }]
        })
        .sort((a, b) => a.tier - b.tier || a.place - b.place)
      return { group: { name: group.name, items: ranked.map((each) => each.item) }, best: ranked[0]?.tier, at }
    })
    .filter((each) => each.best !== undefined)
    .sort((a, b) => a.best! - b.best! || a.at - b.at)
    .map((each) => each.group)
}

function tierOf(item: Searched, query: string, words: string[]): number | undefined {
  const title = fold(item.title)
  if (title === query) return 0
  if (title.startsWith(query)) return 1
  const keywords = (item.keywords ?? []).map(fold)
  let worst = 0
  for (const word of words) {
    const inTitle = matchIn(title, word)
    const inKeywords = keywords.flatMap((each) => matchIn(each, word) ?? [])
    const tier = inTitle ?? (inKeywords.length > 0 ? Math.min(...inKeywords) + 2 : undefined)
    if (tier === undefined) return undefined
    worst = Math.max(worst, tier)
  }
  return worst
}

// How a word matches a text: 2 when a word in it starts with it, 3 when
// it is anywhere in it, with four letters or more, or nothing.
function matchIn(text: string, word: string): number | undefined {
  if (text.split(/[^\p{L}\p{N}]+/u).some((each) => each.startsWith(word))) return 2
  if (word.length >= 4 && text.includes(word)) return 3
  return undefined
}

const fold = (text: string) => text.normalize("NFKD").replace(/\p{M}/gu, "").toLowerCase()

// How many recent sessions the palette opens on, as T3 Code's.
const RECENT = 12

// The sessions the palette opens on: the latest active, newest first,
// without the one open, which would always lead them.
export function recentSessions(sessions: readonly SessionSummary[], open: string | undefined): SessionSummary[] {
  return sessions
    .filter((each) => each.session !== open)
    .sort((a, b) => b.lastActive - a.lastActive)
    .slice(0, RECENT)
}

// The actions the palette offers, by their ids, in the order it lists
// them, as T3 Code's offers what applies and leaves out the rest. A new
// session, the settings and the theme apply everywhere. Moving through
// the list, and showing and hiding the list, the card and the diff, apply
// in the session view, which has them. What acts on a session applies to
// the one the owner is looking at: the session view's, or a workspace's
// side chat's. Settling and moving it wait while it works or waits; what
// acts on its own conversation does not apply while one of its subagents'
// is open; and Pick a model applies to a draft too. The side chat is a small
// chat, which neither renames its session nor picks its model.
export type PaletteFacts = {
  workspace: boolean
  looking: "session" | "subagent" | "draft" | "none"
  state?: SessionState | undefined
  turnRunning: boolean
  stopping: boolean
  compacting: boolean
}
export type PaletteAction = "new" | "new in" | "previous" | "next" | "settings" | "theme" | "list" | "card" | "diff" | "settle" | "move" | "rename" | "compact" | "stop" | "model"

export function paletteActions(facts: PaletteFacts): PaletteAction[] {
  const { workspace, looking, state } = facts
  const own = looking === "session"
  const session = own || looking === "subagent"
  return [
    "new",
    "new in",
    ...(workspace ? [] : (["previous", "next"] as const)),
    "settings",
    "theme",
    ...(workspace ? [] : (["list", "card", "diff"] as const)),
    ...(session && (state === "active" || state === "settled") ? (["settle", "move"] as const) : []),
    ...(own && !workspace ? (["rename"] as const) : []),
    ...(own && !facts.compacting ? (["compact"] as const) : []),
    ...(own && facts.turnRunning && !facts.stopping ? (["stop"] as const) : []),
    ...((own || looking === "draft") && !workspace ? (["model"] as const) : []),
  ]
}
