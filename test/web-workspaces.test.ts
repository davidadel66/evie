import { expect, test } from "bun:test"
import type { SessionSummary } from "../src/core/types.ts"
import { addressAsOpened, selectedIn } from "../web/src/address.ts"
import { PROJECT_ICONS, searchIcons } from "../web/src/project-icons.ts"
import { lookOf } from "../web/src/project-look.ts"
import { projectColour } from "../web/src/session-list.ts"
import { addressOfWorkspace, chatAddress, latestSession, needsYou, sessionsOf, sizeWords, suggestedFolder, workspaceIn } from "../web/src/workspace.ts"

// The workspace view's rules, apart from its screen: where the owner is,
// kept in the address; the project's icon and colour; the icons to pick
// from; and which sessions a workspace shows.

test("a workspace is kept in the address with its tab, the session in its side chat and the file open in Storage", () => {
  expect(addressOfWorkspace({ workspace: "p1", tab: "sessions" })).toBe("#workspace=p1&tab=sessions")
  expect(addressOfWorkspace({ workspace: "p1", tab: "storage", chat: "s1", file: "files/notes.md" })).toBe("#workspace=p1&tab=storage&chat=s1&file=files%2Fnotes.md")
  expect(workspaceIn("#workspace=p1&tab=storage&chat=s1&file=files%2Fnotes.md")).toEqual({ workspace: "p1", tab: "storage", chat: "s1", file: "files/notes.md" })
  expect(workspaceIn("#workspace=p1&chat=new")).toEqual({ workspace: "p1", tab: "sessions", chat: "new" })
})

test("a workspace opens on Sessions when its tab is missing or not one there is, and a file is open only in Storage", () => {
  expect(workspaceIn("#workspace=p1")).toEqual({ workspace: "p1", tab: "sessions" })
  expect(workspaceIn("#workspace=p1&tab=memory")).toEqual({ workspace: "p1", tab: "sessions" })
  expect(workspaceIn("#workspace=p1&tab=sessions&file=a.md")).toEqual({ workspace: "p1", tab: "sessions" })
  expect(workspaceIn("#workspace=&tab=storage")).toBeUndefined()
  expect(workspaceIn("#session=s1")).toBeUndefined()
  expect(selectedIn("#workspace=p1&tab=storage")).toBeUndefined()
})

test("the page keeps a workspace in its address as it opens, without a sign-in code", () => {
  expect(addressAsOpened("#workspace=p1&tab=storage&file=a.md&code=abc")).toBe("/#workspace=p1&tab=storage&file=a.md")
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
