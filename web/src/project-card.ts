import type { GitStatus, Job, ListedProject, PullRequest } from "../../src/core/types.ts"

// The session details' rules, apart from their screen: which of their
// actions apply and why not, what the git button offers as the branch
// stands, which sections show and where the card is, and how it lists the
// session's subagents and background jobs.

// What the card knows of the session when it decides what applies: whether
// the page is connected; whether the session is open, still opening, or
// could not be opened; its project, null when it has none and undefined
// while the projects have not been listed; whether a turn is running in
// it; and whether a git button's request has been sent that evie has not
// started on yet.
export type CardFacts = {
  connected: boolean
  session: "opening" | "open" | "failed"
  project: ListedProject | null | undefined
  turnRunning: boolean
  requested: boolean
}

const NOT_CONNECTED = "Not connected to evie's server."

// What stops every button: no connection, or no session open.
function notOpen({ connected, session }: CardFacts): string | undefined {
  if (!connected) return NOT_CONNECTED
  if (session === "opening") return "Opening the session…"
  if (session === "failed") return "The session could not be opened."
  return undefined
}

// Why commit, push and pull request do not apply now, or nothing when they
// do: they need a git repository, and evie free to take a request, and
// one at a time. A folder git did not say about in time is given the
// benefit of the doubt: evie finds out when it tries.
export function gitWhyNot(facts: CardFacts): string | undefined {
  const { project, turnRunning, requested } = facts
  const closed = notOpen(facts)
  if (closed !== undefined) return closed
  if (project === null) return "This session has no project, so there is no git repository to work in."
  if (project === undefined) return "The project's details have not been read yet."
  if (project.git === false) return "The project's folder is not in a git repository."
  if (turnRunning) return "evie is working. Wait until it has finished."
  if (requested) return "Sent. evie is about to start on it."
  return undefined
}

// Why undo does not apply now, or nothing when it does. It puts back what
// evie's file tools changed, in a project or not, once the turn is over.
export function undoWhyNot(facts: CardFacts): string | undefined {
  const closed = notOpen(facts)
  if (closed !== undefined) return closed
  if (facts.turnRunning) return "evie is working. Undo when it has finished."
  return undefined
}

// What each git button asks evie, as a message in the session, in plain
// words. evie carries it out as it would if the owner had typed it, and
// asks before pushing, as it always does.
export const REQUESTS = {
  commit: "Commit the changes in this session with a short message.",
  push: "Push this branch to its remote.",
  "pull request": "Open a pull request for this branch, with a short title and a summary of its changes.",
  "commit and push": "Commit the changes in this session with a short message, and push this branch to its remote.",
  "commit, push and pull request":
    "Commit any changes in this session with a short message, push this branch to its remote, and open a pull request for it, with a short title and a summary of its changes.",
} as const
export type Request = keyof typeof REQUESTS

// What the branch row's menu asks evie, in words, to switch the project's
// folder to another of its branches.
export const switchRequest = (branch: string) => `Switch this project's folder to the branch ${JSON.stringify(branch)}.`

// The git button: its label, the request it sends, and why it is off, when
// it is. Its label follows how the branch stands, by T3 Code's rule
// (`resolveQuickAction` in its GitActionsControl.logic.ts, MIT licence,
// notice in LICENSES/t3code-MIT.txt), in the four labels evie's card has:
// where T3 Code's would push and create a pull request, or only create
// one, it is Commit, push & PR, which does what is left of that; where
// T3 Code's would pull or sync, or publish the repository, it is Push, off,
// saying why. `status` is undefined until it is read, and `pull` is the
// branch's pull request, null when it has none.
export type GitAction = { label: "Commit" | "Push" | "Commit & push" | "Commit, push & PR"; request: Request; why?: string }

const COMMIT = { label: "Commit", request: "commit" } as const
const PUSH = { label: "Push", request: "push" } as const
const COMMIT_PUSH = { label: "Commit & push", request: "commit and push" } as const
const ALL_THREE = { label: "Commit, push & PR", request: "commit, push and pull request" } as const
const UP_TO_DATE = "The branch is up to date. Nothing to do."

export function gitAction(status: GitStatus | undefined, pull: Pick<PullRequest, "state"> | null | undefined): GitAction {
  if (status === undefined) return { ...COMMIT, why: "Reading how the branch stands…" }
  if (status.branch === undefined) return { ...COMMIT, why: "Check out a branch before committing or pushing." }
  const facts = branchFacts(status, pull)
  const { openPull, onMain, ahead, behind } = facts
  if (status.uncommitted) {
    if (behind || (status.upstream === undefined && !status.remote)) return COMMIT
    return openPull || onMain ? COMMIT_PUSH : ALL_THREE
  }
  if (status.upstream === undefined) {
    if (!status.remote) return { ...PUSH, why: "The repository has no remote to push to." }
    if (!ahead) return openPull ? { ...COMMIT, why: UP_TO_DATE } : { ...PUSH, why: "No commits to push." }
    return openPull || onMain ? PUSH : ALL_THREE
  }
  if (ahead && behind) return { ...PUSH, why: "The branch has gone apart from its remote. Rebase or merge first." }
  if (behind) return { ...PUSH, why: "The branch is behind its remote. Pull before pushing." }
  if (ahead) return openPull || onMain ? PUSH : ALL_THREE
  if (!openPull && !onMain && facts.aheadOfMain) return ALL_THREE
  return { ...COMMIT, why: UP_TO_DATE }
}

// The git button's menu, after T3 Code's `buildMenuItems` (same licence):
// Commit, Push, and Create pull request where the branch has none open,
// each off while it does not apply, saying why. With no remote to push to,
// only Commit. Nothing until the branch's state is read.
export type GitMenuItem = { label: "Commit" | "Push" | "Create pull request"; request: Request; why?: string }

export function gitMenu(status: GitStatus | undefined, pull: Pick<PullRequest, "state"> | null | undefined): GitMenuItem[] {
  if (status === undefined) return []
  const { openPull, ahead, behind } = branchFacts(status, pull)
  const reachable = status.upstream !== undefined || status.remote
  const commit: GitMenuItem = { label: "Commit", request: "commit", ...(!status.uncommitted && { why: "Nothing to commit: the folder has no changes." }) }
  if (!reachable) return [commit]
  const pushWhy =
    status.branch === undefined
      ? "Check out a branch before pushing."
      : behind
        ? "The branch is behind its remote. Pull before pushing."
        : !ahead
          ? "No commits to push."
          : undefined
  const push: GitMenuItem = { label: "Push", request: "push", ...(pushWhy !== undefined && { why: pushWhy }) }
  if (openPull) return [commit, push]
  const pullWhy =
    status.branch === undefined
      ? "Check out a branch before opening a pull request."
      : status.main === undefined || status.main.on || status.main.ahead === 0
        ? `The branch has no commits that ${status.main?.name ?? "the main branch"} does not.`
        : behind
          ? "The branch is behind its remote. Pull before opening a pull request."
          : undefined
  return [commit, push, { label: "Create pull request", request: "pull request", ...(pullWhy !== undefined && { why: pullWhy }) }]
}

// What T3 Code's rule asks of the branch: whether it has a pull request
// open, a draft among them; whether it is the main branch; whether it has
// commits to push, which for a branch never pushed are those main lacks;
// whether it is behind its upstream; and whether it has commits main lacks.
function branchFacts(status: GitStatus, pull: Pick<PullRequest, "state"> | null | undefined) {
  const onMain = status.main?.on === true
  const aheadOfMain = status.main !== undefined && !status.main.on && status.main.ahead > 0
  return {
    openPull: pull?.state === "open" || pull?.state === "draft",
    onMain,
    ahead: status.upstream !== undefined ? status.upstream.ahead > 0 : (status.main?.ahead ?? 0) > 0,
    behind: (status.upstream?.behind ?? 0) > 0,
    aheadOfMain,
  }
}

// Newest first: ids count up within a session.
const newestFirst = (jobs: readonly Job[]) => [...jobs].sort((a, b) => Number(b.id) - Number(a.id))

// The session's background jobs, newest first: those running, and those
// that have ended, which fold away under their count. A subagent's rounds
// of work are jobs too, but are listed with the subagents instead.
export function backgroundJobs(jobs: readonly Job[]): { running: Job[]; ended: Job[] } {
  const own = newestFirst(jobs.filter((job) => job.subagent === undefined))
  return { running: own.filter((job) => job.status === "running"), ended: own.filter((job) => job.status !== "running") }
}

// A subagent as the session's stored subagents list it.
export type Listed = { session: string; name: string | null; started: number }

// One of the session's subagents: its session, its name, and how its
// latest round of work stands, with that round's job, which stops it, and
// when it started. One with no round since the server last started has
// ended, as far as anyone can tell.
export type Subagent = { session: string; name: string; status: Job["status"]; job?: string; started?: number }

// The session's subagents, newest first: those stored, and any its jobs
// name that were started since they were listed. Each is listed once,
// however many rounds of work it has had, and stands as its latest round
// does. It is named as its first round of this run was listed, without
// the word that says it is a subagent's, or else as stored, by its task.
export function subagentsOf(listed: readonly Listed[], jobs: readonly Job[]): Subagent[] {
  const rounds = new Map<string, Job[]>()
  for (const job of [...jobs].sort((a, b) => Number(a.id) - Number(b.id))) {
    if (job.subagent !== undefined) rounds.set(job.subagent, [...(rounds.get(job.subagent) ?? []), job])
  }
  const known = new Map(listed.map((each) => [each.session, each]))
  const sessions = new Set([...known.keys(), ...rounds.keys()])
  const started = (session: string) => known.get(session)?.started ?? rounds.get(session)![0]!.started
  return [...sessions]
    .sort((a, b) => started(b) - started(a))
    .map((session) => {
      const own = rounds.get(session) ?? []
      const latest = own.at(-1)
      const name = own[0]?.title.replace(/^subagent: /, "") ?? known.get(session)?.name ?? "Subagent"
      return { session, name, status: latest?.status ?? "unknown", ...(latest && { job: latest.id, started: latest.started }) }
    })
}

// How a job, or a subagent's latest round, stands, in a word.
export function stateWord(status: Job["status"]): string {
  return { running: "Working", finished: "Done", failed: "Failed", stopped: "Stopped", unknown: "Ended" }[status]
}

// A job's output as the card keeps it: the text, where in the file it was
// read up to, and whether what came before it is left out.
export type Output = { text: string; to: number; cut: boolean }

// The most of a job's output the card keeps, in characters: its latest part.
const KEPT = 64 * 1024

// A read of a job's output joined to what was read before. A read that
// starts where the last one ended adds to it; any other, as when some was
// skipped, or the output began again, replaces it. What is kept is the
// latest part, from the start of a line where one starts in it.
export function joinOutput(kept: Output | undefined, read: { text: string; from: number; to: number }): Output {
  if (kept !== undefined && read.from === kept.to && read.text === "") return kept
  const follows = kept !== undefined && read.from === kept.to
  const text = follows ? kept.text + read.text : read.text
  const cut = follows ? kept.cut : read.from > 0
  if (text.length <= KEPT) return { text, to: read.to, cut }
  const latest = text.slice(-KEPT)
  const line = latest.indexOf("\n")
  return { text: line === -1 || line === latest.length - 1 ? latest : latest.slice(line + 1), to: read.to, cut: true }
}

// What the session details show, as the owner chooses in their menu, as
// T3 Code's details card lets its items be chosen. The ids are kept in
// this browser, so one is never renamed.
export const DETAILS_ITEMS = [
  { id: "workspace", label: "Project and folder" },
  { id: "undo", label: "Undo last change" },
  { id: "branch", label: "Branch and pull request" },
  { id: "git", label: "Git actions" },
  { id: "changes", label: "Changes" },
  { id: "lineage", label: "Lineage" },
  { id: "jobs", label: "Background work" },
] as const
export type DetailsItem = (typeof DETAILS_ITEMS)[number]["id"]
export type Shown = Readonly<Record<DetailsItem, boolean>>

// Every item shows by default.
const DEFAULT_SHOWN = Object.fromEntries(DETAILS_ITEMS.map((item) => [item.id, true])) as Shown

// What shows, from what this browser kept, which may be missing, not JSON,
// or name items that are no longer there: the default for each item but
// those it says otherwise of.
export function shownFrom(kept: string | null | undefined): Shown {
  let read: unknown
  try {
    read = kept == null ? {} : JSON.parse(kept)
  } catch {
    read = {}
  }
  const choices = typeof read === "object" && read !== null ? (read as Record<string, unknown>) : {}
  return Object.fromEntries(DETAILS_ITEMS.map((item) => [item.id, typeof choices[item.id] === "boolean" ? choices[item.id] : DEFAULT_SHOWN[item.id]])) as Shown
}

// What is kept: only the choices that differ from the default, so that a
// later change of the default reaches the items not chosen, as T3 Code's.
export function keptChoices(shown: Shown): Partial<Record<DetailsItem, boolean>> {
  return Object.fromEntries(DETAILS_ITEMS.filter((item) => shown[item.id] !== DEFAULT_SHOWN[item.id]).map((item) => [item.id, shown[item.id]]))
}

export const isDefaultShown = (shown: Shown) => DETAILS_ITEMS.every((item) => shown[item.id] === DEFAULT_SHOWN[item.id])

// Which of the card's sections show, as its drawing has them: the
// workspace, with the project and its folder, and Undo while there is
// something to undo; version control, in a git repository, with the branch
// and its pull request, the git button and the changes; the lineage,
// while the session has a parent or subagents; and background work, while
// a job is listed. A section shows while one of its items is chosen and
// has something to show; with none, the card offers its menu instead.
export function sectionsShown(shown: Shown, has: { undo: boolean; git: boolean; related: boolean; jobs: boolean }) {
  const workspace = shown.workspace || (shown.undo && has.undo)
  const versionControl = has.git && (shown.branch || shown.git || shown.changes)
  const lineage = shown.lineage && has.related
  const jobs = shown.jobs && has.jobs
  return { workspace, versionControl, lineage, jobs, nothing: !workspace && !versionControl && !lineage && !jobs }
}

// Where the card is, as T3 Code places its details card: beside the
// conversation, which keeps clear of it, when pinned and the conversation
// has the room; otherwise floating over it, opened from the button in its
// header; and at phone width, a sheet. The room is measured on the
// conversation's own width, as T3 Code measures it, so that the card floats
// at 1100px wide or narrower with the session list beside it, as the
// owner's drawing has it, and wider with the list or the diff open.
export type CardPlace = "inline" | "popover" | "sheet"

// The card's width and the gap at its side, as T3 Code's; the room between
// it and the conversation; and the least the conversation keeps beside it.
const CARD_WIDTH = 280
const CARD_GAP = 12
const CARD_ROOM = 32
const LEAST_CONVERSATION = 520
export const CARD_CLEARANCE = CARD_WIDTH + CARD_GAP + CARD_ROOM

export function cardPlace({ width, phone, pinned }: { width: number; phone: boolean; pinned: boolean }): CardPlace {
  if (phone) return "sheet"
  return pinned && width - CARD_CLEARANCE > LEAST_CONVERSATION ? "inline" : "popover"
}

// The lineage as the card lists it: the subagents working, each with how
// long its round has run, as "Working · 31s"; those that have ended, which
// fold away under Previous agents, each with how it ended in a word; and
// the heading, which counts those working. `now` is the time to count to.
export function lineageOf(subagents: readonly Subagent[], now: number) {
  const running = subagents.filter((each) => each.status === "running").map((each) => ({ ...each, state: `Working · ${elapsed(now - (each.started ?? now))}` }))
  const previous = subagents.filter((each) => each.status !== "running").map((each) => ({ ...each, state: stateWord(each.status) }))
  return { running, previous, heading: running.length > 0 ? `Lineage · ${running.length} running` : "Lineage" }
}

// A length of time in its two largest units, as "31s", "1m 12s" or "3h 5m".
function elapsed(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000))
  const [hours, minutes, rest] = [Math.floor(seconds / 3600), Math.floor((seconds % 3600) / 60), seconds % 60]
  const words = hours > 0 ? [`${hours}h`, minutes > 0 ? `${minutes}m` : ""] : minutes > 0 ? [`${minutes}m`, rest > 0 ? `${rest}s` : ""] : [`${rest}s`]
  return words.filter((word) => word !== "").join(" ")
}
