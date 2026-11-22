import { expect, test } from "bun:test"
import type { GitStatus, Job } from "../src/core/types.ts"
import { backgroundJobs, CARD_CLEARANCE, cardPlace, DETAILS_ITEMS, gitAction, gitMenu, gitWhyNot, isDefaultShown, joinOutput, keptChoices, lineageOf, sectionsShown, shownFrom, stateWord, subagentsOf, undoWhyNot } from "../web/src/project-card.ts"

// The session details' rules, apart from their screen: which of their
// actions apply and why not, what the git button offers as the branch
// stands, which sections show and where, and how they list a session's
// subagents and background jobs.

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
    { session: "c3", name: "read the docs", status: "stopped", job: "5", started: 5000 },
    { session: "c4", name: "Read the changelog.", status: "unknown" },
    { session: "c2", name: "reviewer: look over the change", status: "failed", job: "3", started: 3000 },
    { session: "c1", name: "find the tests", status: "running", job: "4", started: 4000 },
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


// ---- The git button, as T3 Code's rule decides its label ------------------------

// A feature branch with an upstream, level with it, a commit ahead of
// main, with nothing to commit: the shape each case below changes.
const branchStatus: GitStatus = {
  branch: "feat/breads",
  uncommitted: false,
  remote: true,
  upstream: { ahead: 0, behind: 0 },
  main: { name: "main", on: false, ahead: 1 },
  changes: { against: "main", added: 71, removed: 6 },
  branches: ["feat/breads", "main"],
}
const openPull = { state: "open" } as const
// A status without one of its fields, as when the branch has no upstream.
function without(status: GitStatus, field: "upstream" | "branch"): GitStatus {
  const { [field]: _left, ...rest } = status
  return rest
}

test("with work to commit, the git button commits and pushes, and opens a pull request on a branch that has none", () => {
  const dirty = { ...branchStatus, uncommitted: true }
  expect(gitAction(dirty, null)).toEqual({ label: "Commit, push & PR", request: "commit, push and pull request" })
  expect(gitAction(dirty, openPull)).toEqual({ label: "Commit & push", request: "commit and push" })
  // On the main branch, no pull request is opened from it.
  expect(gitAction({ ...dirty, main: { name: "main", on: true, ahead: 0 } }, null)).toEqual({ label: "Commit & push", request: "commit and push" })
  // Behind its upstream, or with nowhere to push, it only commits.
  expect(gitAction({ ...dirty, upstream: { ahead: 0, behind: 2 } }, openPull)).toEqual({ label: "Commit", request: "commit" })
  expect(gitAction({ ...without(dirty, "upstream"), remote: false }, null)).toEqual({ label: "Commit", request: "commit" })
})

test("with nothing to commit, the git button pushes commits ahead of the upstream, and leads to a pull request on a branch that has none", () => {
  const ahead = { ...branchStatus, upstream: { ahead: 2, behind: 0 } }
  expect(gitAction(ahead, openPull)).toEqual({ label: "Push", request: "push" })
  expect(gitAction({ ...ahead, main: { name: "main", on: true, ahead: 2 } }, null)).toEqual({ label: "Push", request: "push" })
  // T3 Code's "Push & create PR", in the four words the drawing has.
  expect(gitAction(ahead, null)).toEqual({ label: "Commit, push & PR", request: "commit, push and pull request" })
  // A branch never pushed counts what it has that main does not.
  expect(gitAction(without(branchStatus, "upstream"), null)).toEqual({ label: "Commit, push & PR", request: "commit, push and pull request" })
  expect(gitAction({ ...without(branchStatus, "upstream"), main: { name: "main", on: false, ahead: 0 } }, null)).toEqual({
    label: "Push",
    request: "push",
    why: "No commits to push.",
  })
  // Pushed, with commits main does not have and no pull request: T3 Code's "Create PR".
  expect(gitAction(branchStatus, null)).toEqual({ label: "Commit, push & PR", request: "commit, push and pull request" })
})

test("when there is nothing the git button can do, it is off and says why, as T3 Code's does", () => {
  const upToDate = "The branch is up to date. Nothing to do."
  expect(gitAction(branchStatus, openPull)).toEqual({ label: "Commit", request: "commit", why: upToDate })
  expect(gitAction({ ...branchStatus, main: { name: "main", on: true, ahead: 0 } }, null)).toEqual({ label: "Commit", request: "commit", why: upToDate })
  expect(gitAction({ ...branchStatus, upstream: { ahead: 0, behind: 3 } }, null)).toEqual({ label: "Push", request: "push", why: "The branch is behind its remote. Pull before pushing." })
  expect(gitAction({ ...branchStatus, upstream: { ahead: 1, behind: 3 } }, null)).toEqual({ label: "Push", request: "push", why: "The branch has gone apart from its remote. Rebase or merge first." })
  expect(gitAction({ ...without(branchStatus, "upstream"), remote: false }, null)).toEqual({ label: "Push", request: "push", why: "The repository has no remote to push to." })
  expect(gitAction({ ...without(branchStatus, "branch"), uncommitted: true }, null)).toEqual({ label: "Commit", request: "commit", why: "Check out a branch before committing or pushing." })
  expect(gitAction(undefined, null)).toEqual({ label: "Commit", request: "commit", why: "Reading how the branch stands…" })
})

test("the git button's menu holds commit, push and a pull request, each off while it does not apply, saying why, and no pull request where one is open", () => {
  const dirty = { ...branchStatus, uncommitted: true, upstream: { ahead: 1, behind: 0 } }
  expect(gitMenu(dirty, null)).toEqual([
    { label: "Commit", request: "commit" },
    { label: "Push", request: "push" },
    { label: "Create pull request", request: "pull request" },
  ])
  expect(gitMenu(dirty, openPull).map((item) => item.label)).toEqual(["Commit", "Push"])
  expect(gitMenu(branchStatus, null)).toEqual([
    { label: "Commit", request: "commit", why: "Nothing to commit: the folder has no changes." },
    { label: "Push", request: "push", why: "No commits to push." },
    { label: "Create pull request", request: "pull request" },
  ])
  const behind = gitMenu({ ...dirty, upstream: { ahead: 1, behind: 2 } }, null)
  expect(behind.map((item) => item.why)).toEqual([undefined, "The branch is behind its remote. Pull before pushing.", "The branch is behind its remote. Pull before opening a pull request."])
  // With nowhere to push, only a commit.
  expect(gitMenu({ ...without(dirty, "upstream"), remote: false }, null)).toEqual([{ label: "Commit", request: "commit" }])
  const detached = gitMenu(without(dirty, "branch"), null)
  expect(detached.map((item) => item.why)).toEqual([undefined, "Check out a branch before pushing.", "Check out a branch before opening a pull request."])
  expect(gitMenu({ ...branchStatus, main: { name: "main", on: false, ahead: 0 } }, null)[2]).toEqual({ label: "Create pull request", request: "pull request", why: "The branch has no commits that main does not." })
  expect(gitMenu(undefined, null)).toEqual([])
})

// ---- What the card shows --------------------------------------------------------

const everything = { workspace: true, undo: true, branch: true, git: true, changes: true, lineage: true, jobs: true }
const full = { undo: true, git: true, related: true, jobs: true }

test("each section shows while one of its items is chosen and it has something to show, and none does when nothing would", () => {
  expect(sectionsShown(everything, full)).toEqual({ workspace: true, versionControl: true, lineage: true, jobs: true, nothing: false })
  // A folder not in git has no version control; a session with no subagents no lineage; no jobs running, no background work.
  expect(sectionsShown(everything, { undo: true, git: false, related: false, jobs: false })).toEqual({ workspace: true, versionControl: false, lineage: false, jobs: false, nothing: false })
  // The workspace section holds the project and Undo, and shows for either.
  expect(sectionsShown({ ...everything, workspace: false }, full).workspace).toBe(true)
  expect(sectionsShown({ ...everything, workspace: false }, { ...full, undo: false }).workspace).toBe(false)
  // Version control holds the branch, the git button and the changes, and shows for any of them.
  expect(sectionsShown({ ...everything, branch: false, git: false }, full).versionControl).toBe(true)
  expect(sectionsShown({ ...everything, branch: false, git: false, changes: false }, full).versionControl).toBe(false)
  const none = { workspace: false, undo: false, branch: false, git: false, changes: false, lineage: false, jobs: false }
  expect(sectionsShown(none, full)).toEqual({ workspace: false, versionControl: false, lineage: false, jobs: false, nothing: true })
})

test("what the card shows is the default but for what this browser was told otherwise, and only what differs from the default is kept", () => {
  expect(DETAILS_ITEMS.map((item) => item.label)).toEqual(["Project and folder", "Undo last change", "Branch and pull request", "Git actions", "Changes", "Lineage", "Background work"])
  expect(shownFrom(undefined)).toEqual(everything)
  expect(shownFrom('{"lineage":false}')).toEqual({ ...everything, lineage: false })
  // What cannot be read, or names no item, is left out.
  expect(shownFrom("not json")).toEqual(everything)
  expect(shownFrom('{"servers":false,"jobs":"no"}')).toEqual(everything)

  const kept = keptChoices({ ...everything, lineage: false, jobs: false })
  expect(kept).toEqual({ lineage: false, jobs: false })
  expect(shownFrom(JSON.stringify(kept))).toEqual({ ...everything, lineage: false, jobs: false })
  expect(keptChoices(everything)).toEqual({})
  expect(isDefaultShown(everything)).toBe(true)
  expect(isDefaultShown({ ...everything, undo: false })).toBe(false)
})

test("pinned, the card stays beside the conversation while the conversation keeps room beside it, and otherwise floats over it, opened from the header; at phone width it is a sheet", () => {
  // The conversation's width at a 1440px and a 1100px wide page, with the 256px session list beside it.
  expect(cardPlace({ width: 1440 - 256, phone: false, pinned: true })).toBe("inline")
  expect(cardPlace({ width: 1101 - 256, phone: false, pinned: true })).toBe("inline")
  expect(cardPlace({ width: 1100 - 256, phone: false, pinned: true })).toBe("popover")
  // The same page with the session list hidden has the room.
  expect(cardPlace({ width: 1100, phone: false, pinned: true })).toBe("inline")
  expect(cardPlace({ width: 1440 - 256, phone: false, pinned: false })).toBe("popover")
  expect(cardPlace({ width: 390, phone: true, pinned: true })).toBe("sheet")
  // What the conversation keeps clear beside a card in place: the card, the gap at its side, and room between.
  expect(CARD_CLEARANCE).toBe(324)
})

test("the lineage lists the subagents working, each with how long its round has run, and folds away those that have ended, and its heading counts those working", () => {
  const now = 100_000
  const subagents = [
    { session: "c3", name: "Proofread naan.md", status: "running" as const, job: "5", started: now - 31_000 },
    { session: "c2", name: "Check the roti quantities", status: "running" as const, job: "4", started: now - 72_400 },
    { session: "c1", name: "Find a naan recipe to adapt", status: "finished" as const, job: "1", started: 1_000 },
    { session: "c0", name: "One from before a restart", status: "unknown" as const },
  ]
  const shown = lineageOf(subagents, now)
  expect(shown.heading).toBe("Lineage · 2 running")
  expect(shown.running.map((each) => [each.name, each.state])).toEqual([
    ["Proofread naan.md", "Working · 31s"],
    ["Check the roti quantities", "Working · 1m 12s"],
  ])
  expect(shown.previous.map((each) => [each.name, each.state])).toEqual([
    ["Find a naan recipe to adapt", "Done"],
    ["One from before a restart", "Ended"],
  ])
  expect(lineageOf(subagents.slice(2), now).heading).toBe("Lineage")
  expect(lineageOf([{ ...subagents[0]!, started: now - 3 * 3_600_000 - 5 * 60_000 }], now).running[0]!.state).toBe("Working · 3h 5m")
})
