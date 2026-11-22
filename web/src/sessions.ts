import type { Evie, ListedProject, SessionSummary, SummaryEvent } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// The web app's list of sessions and projects, apart from its screens,
// which are told it and redrawn as it changes. It lists them from evie's
// server once connected, and keeps the list current from the summaries the
// server sends of every session as it changes.

// A session in the list: its summary, or for one started on this page with
// nothing said in it yet, which the server does not list, a line made here,
// marked `new`, until its first summary comes.
export type ListedSession = SessionSummary & { new?: true }

export type SessionList = {
  // Whether the sessions have been listed since the page connected.
  listed: boolean
  // Every session, as its latest summary says, the latest active first.
  sessions: ListedSession[]
  // Every project, by name, with the branch each is on.
  projects: ListedProject[]
  // Why the sessions could not be listed, in words for the owner, or
  // undefined when they could.
  problem: string | undefined
}

export type Sessions = {
  // The list now. It is the same object until the list changes.
  state(): SessionList
  // Calls the listener each time the list changes, until the function
  // returned is called.
  subscribe(listener: () => void): () => void
  // Starts a session in a project, or with null in none, and lists it at
  // once. Gives back its id, or why it could not be started.
  start(project: string | null): Promise<{ session: string } | { problem: string }>
  // Settles a session, or with false brings it back. Gives back why the
  // server refused, as it does while the session is working or waiting
  // for an answer, or nothing. The list changes when the session's summary
  // comes.
  settle(session: string, settled: boolean): Promise<string | undefined>
  // Stops following the connection.
  close(): void
}

export function openSessions(connection: Connection): Sessions {
  let state: SessionList = { listed: false, sessions: [], projects: [], problem: undefined }
  const listeners = new Set<() => void>()

  function set(next: Partial<SessionList>) {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }

  // A summary is a session's line as it is now, sent after whatever changed
  // it. One in a project the list does not know, as one made in the
  // terminal since, has the projects listed again.
  function keep(evie: Evie, summary: ListedSession) {
    const sessions = [summary, ...state.sessions.filter((each) => each.session !== summary.session)]
    set({ sessions: sessions.sort((a, b) => b.lastActive - a.lastActive) })
    if (summary.project !== null && !state.projects.some((project) => project.id === summary.project)) void listProjects(evie).catch(() => {})
  }

  // Each list is taken as its answer comes. Answers and summaries come on
  // one line, in the order the server sent them, so a summary that came
  // before an answer is in it already, and one that comes after is newer.
  function list(evie: Evie) {
    const sessions = evie.send({ type: "list_sessions" }).then((listed) => {
      // A session started here that the server does not list yet is kept.
      const unlisted = state.sessions.filter((each) => each.new && !listed.some((other) => other.session === each.session))
      set({ sessions: [...unlisted, ...listed].sort((a, b) => b.lastActive - a.lastActive) })
    })
    void Promise.all([sessions, listProjects(evie)]).then(
      () => set({ listed: true, problem: undefined }),
      (cause: unknown) => set({ problem: `The sessions could not be listed: ${wordsOf(cause)}` }),
    )
  }
  const listProjects = (evie: Evie) => evie.send({ type: "list_projects" }).then((projects) => set({ projects }))

  // Listed each time the connection is made, and kept current from then
  // on. A connection made again after the browser was signed out is a new
  // client, whose summaries are followed in place of the last one's.
  let connected: Evie | undefined
  let heard: Evie | undefined
  let unhear = () => {}
  function follow() {
    const now = connection.state()
    const evie = now.status === "connected" ? now.evie : undefined
    if (evie !== undefined && evie !== heard) {
      unhear()
      heard = evie
      unhear = evie.subscribe((event) => {
        if (event.type === "summary") keep(evie, summaryOf(event))
      })
    }
    if (evie !== undefined && evie !== connected) list(evie)
    connected = evie
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    // Both are sent on the client the list follows, so that one asked for
    // while the line is down waits for it, as the client's commands do.
    async start(project) {
      const evie = heard
      if (evie === undefined) return { problem: NOT_CONNECTED }
      try {
        const opened = await evie.send({ type: "open_session", ...(project !== null && { project }) })
        const known = state.sessions.some((each) => each.session === opened.session)
        if (!known) keep(evie, { session: opened.session, project: opened.project?.id ?? null, name: null, state: "active", lastActive: Date.now(), new: true })
        return { session: opened.session }
      } catch (cause) {
        return { problem: wordsOf(cause) }
      }
    },
    async settle(session, settled) {
      if (heard === undefined) return NOT_CONNECTED
      try {
        await heard.send({ type: "settle_session", session, settled })
        return undefined
      } catch (cause) {
        return wordsOf(cause)
      }
    },
    close() {
      unfollow()
      unhear()
    },
  }
}

const NOT_CONNECTED = "Not connected to evie's server yet. Wait until it is connected."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// A summary event's line, without the event's type.
const summaryOf = ({ type: _, ...summary }: SummaryEvent): SessionSummary => summary
