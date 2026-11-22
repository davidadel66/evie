import { expect, test } from "bun:test"
import type { Job } from "../src/core/types.ts"
import { backgroundJobs, gitWhyNot, isCardShortcut, joinOutput, shortFolder, stateWord, subagentsOf, undoWhyNot } from "../web/src/project-card.ts"

// The project card's rules, apart from its screen: how it says where the
// project is, which of its actions apply and why not, and how it lists a
// session's subagents and background jobs.

test("a folder in a home folder is shortened with ~, and any other is shown whole", () => {
  expect(shortFolder("/Users/david/code/recipe-box")).toBe("~/code/recipe-box")
  expect(shortFolder("/home/david/garden")).toBe("~/garden")
  expect(shortFolder("/Users/david")).toBe("~")
  expect(shortFolder("/private/tmp/evie-project")).toBe("/private/tmp/evie-project")
  expect(shortFolder("/Users")).toBe("/Users")
})

const repository = { id: "p1", name: "recipe-box", folder: "/Users/david/code/recipe-box", branch: "main" }
const idle = { connected: true, project: repository, turnRunning: false }

test("commit, push and pull request apply in a git project while evie is not working, and say why not otherwise", () => {
  expect(gitWhyNot(idle)).toBeUndefined()
  expect(gitWhyNot({ ...idle, project: null })).toBe("This session has no project, so there is no git repository to work in.")
  expect(gitWhyNot({ ...idle, project: { id: "p2", name: "notes", folder: "/Users/david/notes" } })).toBe("The project's folder is not a git repository.")
  expect(gitWhyNot({ ...idle, project: undefined })).toBe("The project's details have not been read yet.")
  expect(gitWhyNot({ ...idle, turnRunning: true })).toBe("evie is working. Wait until it has finished.")
  expect(gitWhyNot({ ...idle, connected: false })).toBe("Not connected to evie's server.")
})

test("undo applies in any session while evie is not working, project or not", () => {
  expect(undoWhyNot({ ...idle, project: null })).toBeUndefined()
  expect(undoWhyNot({ ...idle, turnRunning: true })).toBe("evie is working. Undo when it has finished.")
  expect(undoWhyNot({ ...idle, connected: false })).toBe("Not connected to evie's server.")
})

const job = (id: string, status: Job["status"], extra: Partial<Job> = {}): Job => ({
  id,
  session: "s1",
  kind: "shell",
  title: `command ${id}`,
  status,
  started: Number(id) * 1000,
  outputFile: `/data/tool-output/s1/job-${id}.log`,
  ...extra,
})
const round = (id: string, status: Job["status"], child: string, title = "subagent: find the tests") => job(id, status, { kind: "subagent", title, subagent: child })

test("background jobs are listed newest first, the running ones apart from the ended ones, which fold away, and a subagent's rounds are left to its own list", () => {
  const jobs = [job("1", "finished"), round("2", "running", "c1"), job("3", "running"), job("4", "failed"), job("5", "running"), job("6", "stopped")]
  const listed = backgroundJobs(jobs)
  expect(listed.running.map((each) => each.id)).toEqual(["5", "3"])
  expect(listed.ended.map((each) => each.id)).toEqual(["6", "4", "1"])
})

test("a session's subagents are listed newest first, each once however many rounds of work it has had, with its name and how its latest round stands", () => {
  const jobs = [
    round("1", "finished", "c1", "subagent: find the tests"),
    job("2", "running"),
    round("3", "failed", "c2", "subagent: reviewer: look over the change"),
    round("4", "running", "c1", "subagent: find the tests"),
    round("5", "stopped", "c3", "subagent: read the docs"),
  ]
  expect(subagentsOf(jobs)).toEqual([
    { session: "c3", name: "read the docs", status: "stopped", job: "5" },
    { session: "c2", name: "reviewer: look over the change", status: "failed", job: "3" },
    { session: "c1", name: "find the tests", status: "running", job: "4" },
  ])
})

test("how a job or a subagent stands is said in a word", () => {
  expect((["running", "finished", "failed", "stopped", "unknown"] as const).map(stateWord)).toEqual(["Working", "Done", "Failed", "Stopped", "Ended"])
})

test("a job's output read since the last read joins what was read before, and an answer that skipped some replaces it", () => {
  const first = joinOutput(undefined, { text: "compiling\n", from: 0, to: 10 })
  expect(first).toEqual({ text: "compiling\n", to: 10, cut: false })
  const more = joinOutput(first, { text: "linking\n", from: 10, to: 18 })
  expect(more).toEqual({ text: "compiling\nlinking\n", to: 18, cut: false })
  expect(joinOutput(more, { text: "", from: 18, to: 18 })).toBe(more)
  // Some was skipped, or the output began again, or it was never read from its start.
  expect(joinOutput(more, { text: "tests\n", from: 90_000, to: 90_006 })).toEqual({ text: "tests\n", to: 90_006, cut: true })
  expect(joinOutput(more, { text: "again\n", from: 0, to: 6 })).toEqual({ text: "again\n", to: 6, cut: false })
  expect(joinOutput(undefined, { text: "tail\n", from: 500, to: 505 })).toEqual({ text: "tail\n", to: 505, cut: true })
})

test("what is kept of a job's output is its latest 64 KiB of text, cut at a line where it can be", () => {
  const kept = joinOutput({ text: `${"a".repeat(40_000)}\n${"b".repeat(30_000)}\n`, to: 70_002, cut: false }, { text: "c\n", from: 70_002, to: 70_004 })
  expect(kept).toEqual({ text: `${"b".repeat(30_000)}\nc\n`, to: 70_004, cut: true })
})

test("mod+alt+B shows and hides the card, as in T3 Code: Cmd on a Mac, Ctrl elsewhere, by the key's place, since Option changes the letter a Mac types", () => {
  const keys = { code: "KeyB", key: "b", metaKey: false, ctrlKey: false, altKey: true, shiftKey: false }
  expect(isCardShortcut({ ...keys, metaKey: true, key: "∫" }, true)).toBe(true)
  expect(isCardShortcut({ ...keys, ctrlKey: true }, false)).toBe(true)
  expect(isCardShortcut({ ...keys, ctrlKey: true }, true)).toBe(false)
  expect(isCardShortcut({ ...keys, metaKey: true }, false)).toBe(false)
  expect(isCardShortcut({ ...keys, metaKey: true, shiftKey: true }, true)).toBe(false)
  expect(isCardShortcut({ ...keys, metaKey: true, altKey: false }, true)).toBe(false)
  expect(isCardShortcut({ ...keys, metaKey: true, code: "KeyN" }, true)).toBe(false)
})
