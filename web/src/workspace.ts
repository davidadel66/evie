import type { SessionSummary } from "../../src/core/types.ts"
import { groupSessions } from "./session-list.ts"

// The workspace view's rules, apart from its screen. A workspace is a
// project's home: a strip of every workspace, the workspace itself with
// its tabs, and a side chat with one of its sessions.

// The workspace's tabs that are there now: Storage, its files, and Sessions.
// Memory is to come; dashboards will be tabs too.
export type WorkspaceTab = "storage" | "sessions"
const TABS: readonly WorkspaceTab[] = ["storage", "sessions"]

// Where the owner is in a workspace, as the page's address keeps it after
// #: `#workspace=<project id>&tab=<tab>`, with `&chat=<session id>` for the
// session in its side chat, or `&chat=new` for a new one there, and in
// Storage, `&file=<path>` for the file open, by its path in the `.evie`
// folder. Without `chat`, the side chat shows the workspace's latest session.
export type WorkspacePlace = { workspace: string; tab: WorkspaceTab; chat?: string; file?: string }

// The side chat's `chat` for a new session, which no session's id can be:
// ids are UUIDs.
export const NEW_CHAT = "new"

// The workspace an address's part after # is in, or undefined for none.
// It opens on Sessions, until it has dashboards to open on.
export function workspaceIn(hash: string): WorkspacePlace | undefined {
  const named = new URLSearchParams(hash.replace(/^#/, ""))
  const workspace = named.get("workspace")
  if (workspace === null || workspace === "") return undefined
  const asked = named.get("tab")
  const tab = TABS.find((each) => each === asked) ?? "sessions"
  const chat = named.get("chat")
  const file = named.get("file")
  return { workspace, tab, ...(chat !== null && chat !== "" && { chat }), ...(tab === "storage" && file !== null && file !== "" && { file }) }
}

// The part after # for a place in a workspace.
export function addressOfWorkspace(place: WorkspacePlace): string {
  const named = new URLSearchParams({ workspace: place.workspace, tab: place.tab })
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
