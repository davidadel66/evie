import type { DashboardEntry, SessionSummary } from "../../src/core/types.ts"
import { ageWords, groupSessions } from "./session-list.ts"

// The workspace view's rules, apart from its screen. A workspace is a
// project's home: a strip of every workspace, the workspace itself with
// its tabs, and a side chat with one of its sessions.

// The workspace's tabs: its configured dashboards, Storage (its `.evie`
// files), and Sessions. Memory is to come.
export type WorkspaceTab = "dashboard" | "storage" | "sessions"

// Where the owner is in a workspace, as the page's address keeps it after
// #: `#workspace=<project id>`, with `&tab=dashboard&dashboard=<slug>`,
// `&tab=storage` (and `&file=<path>` for a file open in `.evie`), or
// `&tab=sessions`. When `tab` is undefined, the workspace opens on its
// default tab: its first dashboard when it has dashboards, or Sessions
// when it has none. `&chat=<session id>` keeps the session in its side
// chat, or `&chat=new` for a new one there; without `chat`, the side chat
// shows the workspace's latest session.
export type WorkspacePlace = {
  workspace: string
  tab?: WorkspaceTab
  dashboard?: string
  chat?: string
  file?: string
}

// The concrete tab the workspace shows, once its dashboards have been listed.
export type ResolvedWorkspaceTab = { tab: "dashboard"; dashboard: DashboardEntry } | { tab: "storage" } | { tab: "sessions" }

// The side chat's `chat` for a new session, which no session's id can be:
// ids are UUIDs.
export const NEW_CHAT = "new"

// Whether the side chat is shown. The owner hides it with its ×, or with
// the toggle in the workspace's header, which shows it again, and New
// session shows it too. This browser remembers which; one that keeps
// nothing, as in a private window, shows it.
const CHAT_SHOWN = "evie.side-chat"
let shown: boolean | undefined
const shownListeners = new Set<() => void>()
export function chatShown(): boolean {
  if (shown === undefined) {
    try {
      shown = localStorage.getItem(CHAT_SHOWN) !== "hidden"
    } catch {
      shown = true
    }
  }
  return shown
}
export function setChatShown(next: boolean) {
  const was = chatShown()
  shown = next
  try {
    localStorage.setItem(CHAT_SHOWN, next ? "shown" : "hidden")
  } catch {
    // Not remembered: the next visit shows it.
  }
  if (was !== next) for (const listener of [...shownListeners]) listener()
}
export function onChatShown(listener: () => void): () => void {
  shownListeners.add(listener)
  return () => void shownListeners.delete(listener)
}

// The workspace an address's part after # is in, or undefined for none.
// A missing, unknown, or incomplete tab is left undefined so the workspace
// opens on its default tab (its first dashboard, or Sessions when it has none).
export function workspaceIn(hash: string): WorkspacePlace | undefined {
  const named = new URLSearchParams(hash.replace(/^#/, ""))
  const workspace = named.get("workspace")
  if (workspace === null || workspace === "") return undefined
  const asked = named.get("tab")
  const dashboard = named.get("dashboard")
  const chat = named.get("chat")
  const file = named.get("file")
  let tab: WorkspaceTab | undefined
  if (asked === "storage" || asked === "sessions") tab = asked
  else if (asked === "dashboard" && dashboard !== null && dashboard !== "") tab = "dashboard"
  return {
    workspace,
    ...(tab !== undefined && { tab }),
    ...(tab === "dashboard" && dashboard !== null && dashboard !== "" && { dashboard }),
    ...(chat !== null && chat !== "" && { chat }),
    ...(tab === "storage" && file !== null && file !== "" && { file }),
  }
}

// Which tab a workspace shows for `place`, given the dashboards listed for it.
// Opens on the first dashboard when no tab is chosen or a dashboard slug is not
// found, and falls back to Sessions when the workspace has no dashboards.
export function resolveWorkspaceTab(place: WorkspacePlace, dashboards: readonly DashboardEntry[]): ResolvedWorkspaceTab {
  if (place.tab === "storage") return { tab: "storage" }
  if (place.tab === "sessions") return { tab: "sessions" }
  if (place.tab === "dashboard" && place.dashboard !== undefined) {
    const found = dashboards.find((entry) => entry.id === place.dashboard)
    if (found !== undefined) return { tab: "dashboard", dashboard: found }
  }
  const first = dashboards[0]
  return first !== undefined ? { tab: "dashboard", dashboard: first } : { tab: "sessions" }
}

// What New session does in a workspace, from the address's part after #:
// shows the side chat, and gives back the address of a new chat in it,
// which may be where the address is already. Elsewhere, nothing.
export function newChat(hash: string): string | undefined {
  const place = workspaceIn(hash)
  if (place === undefined) return undefined
  setChatShown(true)
  return addressOfWorkspace({ ...place, chat: NEW_CHAT })
}

// The part after # for a place in a workspace.
export function addressOfWorkspace(place: WorkspacePlace): string {
  const named = new URLSearchParams({ workspace: place.workspace })
  if (place.tab === "dashboard" && place.dashboard !== undefined && place.dashboard !== "") {
    named.set("tab", "dashboard")
    named.set("dashboard", place.dashboard)
  } else if (place.tab === "storage" || place.tab === "sessions") {
    named.set("tab", place.tab)
  }
  if (place.chat !== undefined) named.set("chat", place.chat)
  if (place.file !== undefined && place.tab === "storage") named.set("file", place.file)
  return `#${named}`
}

// A project's sessions, in the order the session list gives them: those
// waiting for the owner and the active ones, then the working ones, then
// the settled ones, each newest first.
export function sessionsOf(sessions: readonly SessionSummary[], project: string): SessionSummary[] {
  const groups = groupSessions(sessions.filter((each) => each.project === project))
  return [...groups.top, ...groups.working, ...groups.settled]
}

// The project's session last active, which its side chat shows until the
// owner picks another, or undefined when it has none.
export function latestSession(sessions: readonly SessionSummary[], project: string): SessionSummary | undefined {
  return sessions.filter((each) => each.project === project).sort((a, b) => b.lastActive - a.lastActive)[0]
}

// What a session's row in the Sessions tab says on its right, and in which
// colour: Needs you in the colour of what waits for the owner, Working in
// quiet words, Done in its own green once the session has settled, and
// otherwise how long ago it was active.
export type SessionStatus = { words: string; tone: "needs" | "done" | "muted" }
export function sessionStatus(session: SessionSummary, now: number): SessionStatus {
  if (session.state === "needs_you") return { words: "Needs you", tone: "needs" }
  if (session.state === "working") return { words: "Working", tone: "muted" }
  if (session.state === "settled") return { words: "Done", tone: "done" }
  return { words: ageWords(session.lastActive, now), tone: "muted" }
}

// Whether a session in the project is waiting for an answer from the owner.
export const needsYou = (sessions: readonly SessionSummary[], project: string) => sessions.some((each) => each.project === project && each.state === "needs_you")

// Where a new workspace's folder is suggested: in the home folder of the
// computer evie runs on, written from ~ for the server to read, named after
// the workspace, with any slash in its name made a dash and any dot at its
// start left off, so that it names a folder of its own. A name that leaves
// nothing suggests nothing.
export function suggestedFolder(name: string): string {
  const folder = name.trim().replace(/[/\\]+/g, "-").replace(/^[.\-\s]+/, "")
  return folder === "" || /^[.\-]*$/.test(folder) ? "" : `~/${folder}`
}

// The address that shows a session just started in a project's side chat:
// the workspace as the address has it now, as the owner may have changed
// its tab meanwhile, or nothing once the owner has left it.
export function chatAddress(hash: string, project: string, session: string): string | undefined {
  const now = workspaceIn(hash)
  return now?.workspace === project ? addressOfWorkspace({ ...now, chat: session }) : undefined
}

// A file's size as Storage lists it: in bytes below a kilobyte, then in
// KB or MB, to one place where it is not whole.
export function sizeWords(size: number): string {
  if (size < 1024) return `${size} B`
  const [unit, scaled] = size < 1024 * 1024 ? ["KB", size / 1024] : ["MB", size / (1024 * 1024)]
  return `${scaled.toFixed(1).replace(/\.0$/, "")} ${unit}`
}
