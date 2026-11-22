import { expect, test } from "bun:test"
import type { SessionSummary } from "../src/core/types.ts"
import { rankGroups, recentSessions, type Searched } from "../web/src/palette.ts"

// The command palette's rules, apart from its screen, as T3 Code's: what it
// opens on, and how what is typed ranks commands, sessions and projects.

const actions = {
  name: "Actions",
  items: [
    { title: "New session", keywords: ["chat", "draft"] },
    { title: "New session in a project…", keywords: ["chat", "draft"] },
    { title: "Open settings", keywords: ["preferences", "keys", "plugins"] },
    { title: "Theme", keywords: ["dusk", "moss", "oxblood", "colours"] },
    { title: "Pick a model" },
  ],
}
const sessions = { name: "Sessions", items: [{ title: "New beds for the garden" }, { title: "Plan the spring beds" }, { title: "Theme" }] }
const projects = { name: "Projects", items: [{ title: "garden-planner" }, { title: "recipe-box" }] }
const titles = (groups: { name: string; items: { title: string }[] }[]) => groups.map((group) => [group.name, group.items.map((item) => item.title)])

test("with nothing typed, the groups are as given", () => {
  expect(titles(rankGroups([actions, sessions], ""))).toEqual(titles([actions, sessions]))
})

test("on an equal match, commands rank above sessions: a name that starts with what is typed, in either", () => {
  expect(titles(rankGroups([actions, sessions, projects], "new"))).toEqual([
    ["Actions", ["New session", "New session in a project…"]],
    ["Sessions", ["New beds for the garden"]],
  ])
  expect(titles(rankGroups([actions, sessions, projects], "theme"))).toEqual([
    ["Actions", ["Theme"]],
    ["Sessions", ["Theme"]],
  ])
})

test("a group moves up when its best match is better: a whole name first, then one that starts with it, a word that starts with it, and a long enough part of a word", () => {
  expect(titles(rankGroups([actions, sessions, projects], "garden-planner"))).toEqual([["Projects", ["garden-planner"]]])
  expect(titles(rankGroups([actions, sessions, projects], "spring"))).toEqual([["Sessions", ["Plan the spring beds"]]])
  expect(titles(rankGroups([actions, sessions, projects], "planner"))).toEqual([["Projects", ["garden-planner"]]])
  expect(titles(rankGroups([actions, sessions, projects], "odel"))).toEqual([["Actions", ["Pick a model"]]])
  // Three letters inside a word are too few: "del" is in too many words.
  expect(titles(rankGroups([actions, sessions, projects], "del"))).toEqual([])
})

test("every word typed must match, in the name or in what the row is also known by, which ranks below the name", () => {
  expect(titles(rankGroups([actions, sessions, projects], "spring beds"))).toEqual([["Sessions", ["Plan the spring beds"]]])
  expect(titles(rankGroups([actions, sessions, projects], "beds plan"))).toEqual([["Sessions", ["Plan the spring beds"]]])
  expect(titles(rankGroups([actions, sessions, projects], "moss"))).toEqual([["Actions", ["Theme"]]])
  expect(titles(rankGroups([actions, sessions, projects], "beds the garden"))).toEqual([["Sessions", ["New beds for the garden"]]])
  expect(titles(rankGroups([actions, sessions, projects], "chat"))).toEqual([["Actions", ["New session", "New session in a project…"]]])
  // A word in the name beats one only in what it is also known by.
  const card: { name: string; items: Searched[] } = { name: "Actions", items: [{ title: "Show the card", keywords: ["garden"] }] }
  expect(titles(rankGroups([card, projects], "garden"))).toEqual([
    ["Projects", ["garden-planner"]],
    ["Actions", ["Show the card"]],
  ])
})

test("case and accents are not minded", () => {
  expect(titles(rankGroups([sessions, { name: "Projects", items: [{ title: "Café menus" }] }], "CAFE"))).toEqual([["Projects", ["Café menus"]]])
})

const line = (session: string, lastActive: number): SessionSummary => ({ session, project: null, name: session, state: "active", lastActive })

test("the recent sessions are the latest twelve active, newest first, without the one open", () => {
  const listed = Array.from({ length: 15 }, (_, at) => line(`s${at}`, at))
  expect(recentSessions(listed, "s14").map((each) => each.session)).toEqual(["s13", "s12", "s11", "s10", "s9", "s8", "s7", "s6", "s5", "s4", "s3", "s2"])
  expect(recentSessions(listed.slice(0, 3), undefined).map((each) => each.session)).toEqual(["s2", "s1", "s0"])
})
