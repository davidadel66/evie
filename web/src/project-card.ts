import type { Job, ListedProject } from "../../src/core/types.ts"

// The project card's rules, apart from its screen: which of its actions
// apply and why not, how it says the project's branch, and how it lists the
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

// What the branch row says: the branch, or why there is none, and whether
// it is a branch's name, shown as code.
export function branchWords(project: ListedProject): { text: string; branch: boolean } {
  if (project.branch !== undefined) return { text: project.branch, branch: true }
  if (project.git === true) return { text: "No branch", branch: false }
  if (project.git === false) return { text: "Not a git repository", branch: false }
  return { text: "Not known", branch: false }
}

// What each git button asks evie, as a message in the session, in plain
// words. evie carries it out as it would if the owner had typed it, and
// asks before pushing, as it always does.
export const REQUESTS = {
  commit: "Commit the changes in this session with a short message.",
  push: "Push this branch to its remote.",
  "pull request": "Open a pull request for this branch, with a short title and a summary of its changes.",
} as const

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
// latest round of work stands, with that round's job, which stops it. One
// with no round since the server last started has ended, as far as
// anyone can tell.
export type Subagent = { session: string; name: string; status: Job["status"]; job?: string }

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
      return { session, name, status: latest?.status ?? "unknown", ...(latest && { job: latest.id }) }
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
