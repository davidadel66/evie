import { expect, test } from "bun:test"
import type { Job } from "../src/core/types.ts"
import { backgroundJobs, branchWords, gitWhyNot, joinOutput, stateWord, subagentsOf, undoWhyNot } from "../web/src/project-card.ts"

// The project card's rules, apart from its screen: how it says where the
// project is, which of its actions apply and why not, and how it lists a
// session's subagents and background jobs.

const repository = { id: "p1", name: "recipe-box", folder: "/Users/david/code/recipe-box", branch: "main", git: true }
const idle = { connected: true, session: "open" as const, project: repository, turnRunning: false, requested: false }

test("commit, push and pull request apply in a git project while evie is not working, and say why not otherwise", () => {
  expect(gitWhyNot(idle)).toBeUndefined()
  expect(gitWhyNot({ ...idle, project: null })).toBe("This session has no project, so there is no git repository to work in.")
  expect(gitWhyNot({ ...idle, project: { id: "p2", name: "notes", folder: "/Users/david/notes", git: false } })).toBe("The project's folder is not in a git repository.")
  expect(gitWhyNot({ ...idle, project: undefined })).toBe("The project's details have not been read yet.")
  expect(gitWhyNot({ ...idle, turnRunning: true })).toBe("evie is working. Wait until it has finished.")
  expect(gitWhyNot({ ...idle, connected: false })).toBe("Not connected to evie's server.")
})

test("a repository on no branch of its own, or one git could not tell about in time, keeps its git buttons on", () => {
  expect(gitWhyNot({ ...idle, project: { id: "p1", name: "recipe-box", folder: "/code/recipe-box", git: true } })).toBeUndefined()
  expect(gitWhyNot({ ...idle, project: { id: "p1", name: "recipe-box", folder: "/code/recipe-box" } })).toBeUndefined()
})

test("while the session opens, or once it could not be opened, every button says so", () => {
  for (const whyNot of [gitWhyNot, undoWhyNot]) {
    expect(whyNot({ ...idle, session: "opening" })).toBe("Opening the session…")
    expect(whyNot({ ...idle, session: "failed" })).toBe("The session could not be opened.")
  }
})

test("a request sent stays the only one until evie starts on it", () => {
  expect(gitWhyNot({ ...idle, requested: true })).toBe("Sent. evie is about to start on it.")
  expect(undoWhyNot({ ...idle, requested: true })).toBeUndefined()
})

test("undo applies in any session while evie is not working, project or not", () => {
  expect(undoWhyNot({ ...idle, project: null })).toBeUndefined()
  expect(undoWhyNot({ ...idle, turnRunning: true })).toBe("evie is working. Undo when it has finished.")
  expect(undoWhyNot({ ...idle, connected: false })).toBe("Not connected to evie's server.")
})

test("the branch row says the branch, or why there is none", () => {
  const project = { id: "p1", name: "recipe-box", folder: "/code/recipe-box" }
  expect(branchWords({ ...project, branch: "main", git: true })).toEqual({ text: "main", branch: true })
  expect(branchWords({ ...project, git: true })).toEqual({ text: "No branch", branch: false })
  expect(branchWords({ ...project, git: false })).toEqual({ text: "Not a git repository", branch: false })
  expect(branchWords(project)).toEqual({ text: "Not known", branch: false })
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

test("a session's subagents are those stored, and any its jobs name that are not listed yet, newest first, each once however many rounds it has had, standing as its latest round does, or ended with no round since the server started", () => {
  const listed = [
    { session: "c4", name: "Read the changelog.", started: 4500 },
    { session: "c2", name: "Look over the change.", started: 2500 },
    { session: "c1", name: "Find the tests.", started: 900 },
    { session: "c0", name: "One from before a restart.", started: 100 },
  ]
  const jobs = [
    round("1", "finished", "c1", "subagent: find the tests"),
    job("2", "running"),
    round("3", "failed", "c2", "subagent: reviewer: look over the change"),
    round("4", "running", "c1", "subagent: find the tests"),
    round("5", "stopped", "c3", "subagent: read the docs"),
  ]
  expect(subagentsOf(listed, jobs)).toEqual([
    { session: "c3", name: "read the docs", status: "stopped", job: "5" },
    { session: "c4", name: "Read the changelog.", status: "unknown" },
    { session: "c2", name: "reviewer: look over the change", status: "failed", job: "3" },
    { session: "c1", name: "find the tests", status: "running", job: "4" },
    { session: "c0", name: "One from before a restart.", status: "unknown" },
  ])
  expect(subagentsOf([{ session: "c5", name: null, started: 1 }], [])).toEqual([{ session: "c5", name: "Subagent", status: "unknown" }])
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

