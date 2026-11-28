import { expect, test } from "bun:test"
import type { DashboardEntry, SessionSummary } from "../src/core/types.ts"
import { addressAsOpened, lookedAt, selectedIn } from "../web/src/address.ts"
import { PROJECT_ICONS, searchIcons } from "../web/src/project-icons.ts"
import { lookOf } from "../web/src/project-look.ts"
import { projectColour } from "../web/src/session-list.ts"
import {
  addressOfWorkspace,
  chatAddress,
  chatShown,
  latestSession,
  needsYou,
  newChat,
  resolveWorkspaceTab,
  sessionStatus,
  sessionsOf,
  setChatShown,
  sizeWords,
  suggestedFolder,
  workspaceIn,
} from "../web/src/workspace.ts"

// The workspace view's rules, apart from its screen: where the owner is,
// kept in the address; the project's icon and colour; the icons to pick
// from; and which sessions a workspace shows.

test("a workspace is kept in the address with its tab, dashboard, side chat session and Storage file", () => {
  expect(addressOfWorkspace({ workspace: "p1" })).toBe("#workspace=p1")
  expect(addressOfWorkspace({ workspace: "p1", tab: "sessions" })).toBe("#workspace=p1&tab=sessions")
  expect(addressOfWorkspace({ workspace: "p1", tab: "dashboard", dashboard: "cash-flow", chat: "s1" })).toBe("#workspace=p1&tab=dashboard&dashboard=cash-flow&chat=s1")
  expect(addressOfWorkspace({ workspace: "p1", tab: "storage", chat: "s1", file: "files/notes.md" })).toBe("#workspace=p1&tab=storage&chat=s1&file=files%2Fnotes.md")
  expect(workspaceIn("#workspace=p1&tab=storage&chat=s1&file=files%2Fnotes.md")).toEqual({ workspace: "p1", tab: "storage", chat: "s1", file: "files/notes.md" })
  expect(workspaceIn("#workspace=p1&tab=dashboard&dashboard=cash-flow&chat=s1")).toEqual({ workspace: "p1", tab: "dashboard", dashboard: "cash-flow", chat: "s1" })
  expect(workspaceIn("#workspace=p1&chat=new")).toEqual({ workspace: "p1", chat: "new" })
})

test("missing, unknown or incomplete workspace tabs leave tab undefined for the default tab, and stale params are dropped", () => {
  expect(workspaceIn("#workspace=p1")).toEqual({ workspace: "p1" })
  expect(workspaceIn("#workspace=p1&tab=memory")).toEqual({ workspace: "p1" })
  expect(workspaceIn("#workspace=p1&tab=dashboard")).toEqual({ workspace: "p1" })
  expect(workspaceIn("#workspace=p1&tab=dashboard&dashboard=")).toEqual({ workspace: "p1" })
  expect(workspaceIn("#workspace=p1&tab=dashboard&dashboard=cash-flow&file=a.md")).toEqual({ workspace: "p1", tab: "dashboard", dashboard: "cash-flow" })
  expect(workspaceIn("#workspace=p1&tab=sessions&file=a.md&dashboard=cash-flow")).toEqual({ workspace: "p1", tab: "sessions" })
  expect(addressOfWorkspace({ workspace: "p1", tab: "dashboard" })).toBe("#workspace=p1")
  expect(addressOfWorkspace({ workspace: "p1", tab: "sessions", dashboard: "cash-flow", file: "a.md" })).toBe("#workspace=p1&tab=sessions")
  expect(workspaceIn("#workspace=&tab=storage")).toBeUndefined()
  expect(workspaceIn("#session=s1")).toBeUndefined()
  expect(selectedIn("#workspace=p1&tab=storage")).toBeUndefined()
})

test("resolveWorkspaceTab opens on the first dashboard by default or when a slug is missing, and falls back to Sessions when there are none", () => {
  const cashFlow: DashboardEntry = { id: "cash-flow", name: "Cash Flow", kind: "static", path: "dashboards/cash.html" }
  const metrics: DashboardEntry = { id: "metrics", name: "Metrics", kind: "live", url: "http://127.0.0.1:4100" }
  const dashboards: DashboardEntry[] = [cashFlow, metrics]

  expect(resolveWorkspaceTab({ workspace: "p1" }, dashboards)).toEqual({ tab: "dashboard", dashboard: cashFlow })
  expect(resolveWorkspaceTab({ workspace: "p1" }, [])).toEqual({ tab: "sessions" })
  expect(resolveWorkspaceTab({ workspace: "p1", tab: "dashboard", dashboard: "metrics" }, dashboards)).toEqual({ tab: "dashboard", dashboard: metrics })
  expect(resolveWorkspaceTab({ workspace: "p1", tab: "dashboard", dashboard: "deleted" }, dashboards)).toEqual({ tab: "dashboard", dashboard: cashFlow })
  expect(resolveWorkspaceTab({ workspace: "p1", tab: "dashboard", dashboard: "deleted" }, [])).toEqual({ tab: "sessions" })
  expect(resolveWorkspaceTab({ workspace: "p1", tab: "storage" }, dashboards)).toEqual({ tab: "storage" })
  expect(resolveWorkspaceTab({ workspace: "p1", tab: "sessions" }, dashboards)).toEqual({ tab: "sessions" })
})

test("the page keeps a workspace in its address as it opens, without a sign-in code", () => {
  expect(addressAsOpened("#workspace=p1&tab=storage&file=a.md&code=abc")).toBe("/#workspace=p1&tab=storage&file=a.md")
  expect(addressAsOpened("#workspace=p1&tab=dashboard&dashboard=cash-flow&code=abc")).toBe("/#workspace=p1&tab=dashboard&dashboard=cash-flow")
  expect(addressAsOpened("#workspace=p1&code=abc")).toBe("/#workspace=p1")
  expect(addressAsOpened("#session=s1&code=abc")).toBe("/#session=s1")
})

test("a project is drawn with the icon and colour the owner picked, and until then with a folder in the colour its id gives it", () => {
  expect(lookOf({ id: "p1", icon: "leaf", colour: "slate" })).toEqual({ icon: "leaf", colour: "slate" })
  expect(lookOf({ id: "p1" })).toEqual({ icon: "folder", colour: projectColour("p1") })
})

test("the icons to pick from are about sixty of Phosphor's that suit a project, the folder first", () => {
  expect(PROJECT_ICONS.length).toBe(64)
  expect(PROJECT_ICONS[0]!.name).toBe("folder")
  for (const name of ["code", "book", "chart-bar", "house", "leaf", "briefcase"]) expect(PROJECT_ICONS.map((icon) => icon.name)).toContain(name)
})

test("searching the icons matches the start of a word of each icon's name or what Phosphor tags it with, names first", () => {
  expect(searchIcons("").map((icon) => icon.name)).toEqual(PROJECT_ICONS.map((icon) => icon.name))
  expect(searchIcons("Cook")[0]!.name).toBe("cooking-pot")
  expect(searchIcons("chart").slice(0, 3).map((icon) => icon.name)).toEqual(["chart-bar", "chart-line", "chart-pie"])
  expect(searchIcons(" piggy  bank ").map((icon) => icon.name)).toEqual(["piggy-bank"])
  expect(searchIcons("directory").map((icon) => icon.name)).toContain("folder")
  expect(searchIcons("zebra crossing")).toEqual([])
})

const summary = (session: string, project: string | null, state: SessionSummary["state"], lastActive: number): SessionSummary => ({ session, project, name: session, state, lastActive })

test("a workspace's sessions are its project's, in the order the session list gives them, and its latest is the one last active", () => {
  const sessions = [
    summary("old", "p1", "settled", 1),
    summary("busy", "p1", "working", 5),
    summary("asking", "p1", "needs_you", 2),
    summary("recent", "p1", "active", 4),
    summary("elsewhere", "p2", "active", 9),
  ]

  expect(sessionsOf(sessions, "p1").map((each) => each.session)).toEqual(["asking", "recent", "busy", "old"])
  expect(latestSession(sessions, "p1")?.session).toBe("busy")
  expect(latestSession(sessions, "p3")).toBeUndefined()
})

test("a workspace needs the owner while a session in it waits for an answer", () => {
  const sessions = [summary("asking", "p1", "needs_you", 2), summary("idle", "p2", "active", 3)]

  expect(needsYou(sessions, "p1")).toBe(true)
  expect(needsYou(sessions, "p2")).toBe(false)
})

test("a session in the Sessions tab says Needs you, Working or Done in words, and otherwise how long ago it was active", () => {
  const now = 10 * 60 * 60_000
  expect(sessionStatus(summary("asking", "p1", "needs_you", now), now)).toEqual({ words: "Needs you", tone: "needs" })
  expect(sessionStatus(summary("busy", "p1", "working", now), now)).toEqual({ words: "Working", tone: "muted" })
  expect(sessionStatus(summary("old", "p1", "settled", 0), now)).toEqual({ words: "Done", tone: "done" })
  expect(sessionStatus(summary("recent", "p1", "active", now - 60 * 60_000), now)).toEqual({ words: "1h", tone: "muted" })
})

test("a new workspace's folder is suggested in the home folder, named after it, never as the home folder or the one above", () => {
  expect(suggestedFolder("Family finances")).toBe("~/Family finances")
  expect(suggestedFolder(" a/b ")).toBe("~/a-b")
  expect(suggestedFolder(".config")).toBe("~/config")
  for (const name of ["", " ", ".", "..", " ... ", "/..", "../"]) expect(suggestedFolder(name)).toBe("")
})

test("a session started in the side chat is shown there in the workspace as the address has it then, and not at all once the owner has left", () => {
  expect(chatAddress("#workspace=p1&tab=storage&chat=new&file=a.md", "p1", "s9")).toBe("#workspace=p1&tab=storage&chat=s9&file=a.md")
  expect(chatAddress("#workspace=p2&tab=sessions", "p1", "s9")).toBeUndefined()
  expect(chatAddress("#session=s1", "p1", "s9")).toBeUndefined()
})

test("a file's size is said in bytes, then in KB and MB to one place, as Storage lists it", () => {
  expect(sizeWords(0)).toBe("0 B")
  expect(sizeWords(1023)).toBe("1023 B")
  expect(sizeWords(1024)).toBe("1 KB")
  expect(sizeWords(1536)).toBe("1.5 KB")
  expect(sizeWords(1024 * 1024 * 3.25)).toBe("3.3 MB")
})

// A stand-in for the browser's storage, for as long as `run` runs.
function inBrowser(run: (kept: Map<string, string>) => void) {
  const kept = new Map<string, string>()
  const page = globalThis as unknown as { localStorage?: unknown }
  page.localStorage = { getItem: (key: string) => kept.get(key) ?? null, setItem: (key: string, value: string) => void kept.set(key, value) }
  try {
    run(kept)
  } finally {
    delete page.localStorage
    setChatShown(true)
  }
}

test("the side chat hidden is remembered in this browser, and New session in a workspace shows it, though the address is on a new chat already", () => {
  inBrowser((kept) => {
    setChatShown(false)
    expect(chatShown()).toBe(false)
    expect(kept.get("evie.side-chat")).toBe("hidden")

    expect(newChat("#workspace=p1&tab=storage&chat=new")).toBe("#workspace=p1&tab=storage&chat=new")
    expect(chatShown()).toBe(true)
    expect(kept.get("evie.side-chat")).toBe("shown")
    expect(newChat("#workspace=p1&tab=sessions&chat=s1")).toBe("#workspace=p1&tab=sessions&chat=new")
    expect(newChat("#session=s1")).toBeUndefined()
  })
})

test("while the side chat is hidden, nothing in it is being looked at", () => {
  inBrowser(() => {
    setChatShown(false)
    expect(lookedAt("#workspace=p1&tab=sessions&chat=s2")).toBeUndefined()
    expect(lookedAt("#workspace=p1&tab=sessions&chat=new")).toBeUndefined()
    expect(lookedAt("#session=s1")).toEqual({ session: "s1", subagent: false })
    setChatShown(true)
    expect(lookedAt("#workspace=p1&tab=sessions&chat=s2")).toEqual({ session: "s2", subagent: false })
  })
})

test("the Settings tab is kept in the address and resolves to itself", () => {
  expect(addressOfWorkspace({ workspace: "p1", tab: "settings" })).toBe("#workspace=p1&tab=settings")
  expect(workspaceIn("#workspace=p1&tab=settings&chat=s1")).toEqual({ workspace: "p1", tab: "settings", chat: "s1" })
  expect(resolveWorkspaceTab({ workspace: "p1", tab: "settings" }, [])).toEqual({ tab: "settings" })
})
