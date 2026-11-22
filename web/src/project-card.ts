import type { Job, ListedProject } from "../../src/core/types.ts"

// The project card's rules, apart from its screen: how it says where the
// session's project is, which of its actions apply and why not, and how it
// lists the session's subagents and background jobs.

// A project's folder as the card shows it: in a home folder, with ~ in
// its place. The page cannot ask the server's computer whose home it is,
// so a folder under /Users or /home is taken to be in its owner's.
export function shortFolder(folder: string): string {
  const home = /^\/(?:Users|home)\/[^/]+/.exec(folder)?.[0]
  return home === undefined ? folder : `~${folder.slice(home.length)}`
}

// What the card knows of the session when it decides what applies: whether
// the page is connected, the session's project, null when it has none and
// undefined while the projects have not been listed, and whether a turn is
// running in it.
export type CardFacts = { connected: boolean; project: ListedProject | null | undefined; turnRunning: boolean }

const NOT_CONNECTED = "Not connected to evie's server."

// Why commit, push and pull request do not apply now, or nothing when they
// do: they need a git repository, and evie to be free to take a request.
// The project's branch is how the card knows it is a repository: the list
// of projects gives none for a folder that is not one.
export function gitWhyNot({ connected, project, turnRunning }: CardFacts): string | undefined {
  if (!connected) return NOT_CONNECTED
  if (project === null) return "This session has no project, so there is no git repository to work in."
  if (project === undefined) return "The project's details have not been read yet."
  if (project.branch === undefined) return "The project's folder is not a git repository."
  if (turnRunning) return "evie is working. Wait until it has finished."
  return undefined
}

// Why undo does not apply now, or nothing when it does. It puts back what
// evie's file tools changed, in a project or not, once the turn is over.
export function undoWhyNot({ connected, turnRunning }: CardFacts): string | undefined {
  if (!connected) return NOT_CONNECTED
  if (turnRunning) return "evie is working. Undo when it has finished."
  return undefined
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

// One of the session's subagents: its session, its name, and how its
// latest round of work stands, with that round's job, which stops it.
export type Subagent = { session: string; name: string; status: Job["status"]; job: string }

// The session's subagents, newest first, from the jobs of their rounds of
// work: each once, however many rounds it has had, as its latest stands.
// Its name is what its first round was listed as, without the word that
// says it is a subagent's.
export function subagentsOf(jobs: readonly Job[]): Subagent[] {
  const found = new Map<string, Subagent>()
  for (const job of [...jobs].sort((a, b) => Number(a.id) - Number(b.id))) {
    if (job.subagent === undefined) continue
    const first = found.get(job.subagent)
    found.set(job.subagent, { session: job.subagent, name: first?.name ?? job.title.replace(/^subagent: /, ""), status: job.status, job: job.id })
  }
  return [...found.values()].reverse()
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

// Whether a key pressed is the one that shows and hides the card, as T3
// Code's right panel: mod+alt+B, mod being Cmd on a Mac and Ctrl elsewhere.
// The key is known by its place, since on a Mac Option changes the letter
// it types.
export type Keys = { code: string; key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; shiftKey: boolean }
export function isCardShortcut(keys: Keys, mac: boolean): boolean {
  return keys.code === "KeyB" && keys.altKey && !keys.shiftKey && (mac ? keys.metaKey && !keys.ctrlKey : keys.ctrlKey && !keys.metaKey)
}
