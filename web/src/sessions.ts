import type { Command, Evie, ListedProject, SessionSummary, SummaryEvent } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// The web app's list of sessions and projects, apart from its screens,
// which are told it and redrawn as it changes. It lists them from evie's
// server once connected, and keeps the list current from the summaries the
// server sends of every session as it changes.

export type SessionList = {
  // Whether the sessions have been listed since the page connected.
  listed: boolean
  // Every session, as its latest summary says, the latest active first.
  sessions: SessionSummary[]
  // Every project, by name, with the branch each is on.
  projects: ListedProject[]
  // Why the sessions could not be listed, in words for the owner, or
  // undefined when they could.
  problem: string | undefined
  // Why the projects could not be listed, the last time they were asked
  // for, or undefined when they could. The sessions are listed all the
  // same, without their projects' names and branches.
  projectsProblem: string | undefined
  // The sessions deleted while the page has been open, from here or
  // elsewhere, oldest first, so that a page with one open can say so.
  deleted: readonly string[]
}

export type Sessions = {
  // The list now. It is the same object until the list changes.
  state(): SessionList
  // Calls the listener each time the list changes, until the function
  // returned is called.
  subscribe(listener: () => void): () => void
  // Lists the projects again, for what changes with no event to tell of
  // it, such as the branch each is on, when connected. Settles once a list
  // asked for after this call has been applied, or has failed, or at once
  // while not connected.
  refreshProjects(): Promise<void>
  // Settles a session, or with false brings it back. Gives back why the
  // server refused, as it does while the session is working or waiting
  // for an answer, or nothing. The list changes when the session's summary
  // comes.
  settle(session: string, settled: boolean): Promise<string | undefined>
  // Renames a session, moves it into a project or with null out of one, or
  // deletes it for good. Each gives back why the server refused, as it does
  // a blank name, or a move or a delete while the session is working, or
  // nothing. The list changes when the session's summary comes, or for a
  // delete, the word that it is gone, which every page is sent.
  rename(session: string, name: string): Promise<string | undefined>
  move(session: string, project: string | null): Promise<string | undefined>
  // A delete that went gives back what could not be removed once the
  // session was gone, a sentence each, which is nothing when all of it went.
  delete(session: string): Promise<{ refused: string } | { leftOver: string[] }>
  // Stops following the connection.
  close(): void
}

export function openSessions(connection: Connection): Sessions {
  let state: SessionList = { listed: false, sessions: [], projects: [], problem: undefined, projectsProblem: undefined, deleted: [] }
  const listeners = new Set<() => void>()

  function set(next: Partial<SessionList>) {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }

  // A summary is a session's line as it is now, sent after whatever changed
  // it. One in a project the list does not know, as one made in the
  // terminal since, has the projects listed again.
  function keep(evie: Evie, summary: SessionSummary) {
    for (const heard of listing) heard.set(summary.session, summary)
    const sessions = [summary, ...state.sessions.filter((each) => each.session !== summary.session)]
    set({ sessions: sessions.sort((a, b) => b.lastActive - a.lastActive) })
    if (summary.project !== null && !state.projects.some((project) => project.id === summary.project)) listProjects(evie)
  }

  // A session deleted is taken out of the list, and out of any list that
  // is out, which can be older than its deletion.
  function drop(session: string) {
    for (const heard of listing) heard.set(session, "deleted")
    set({ sessions: state.sessions.filter((each) => each.session !== session), deleted: [...state.deleted.filter((each) => each !== session), session] })
  }

  // The summaries heard while each list of sessions is out. The list can be
  // older than any of them, as when the server answers it from before a
  // drop, so each of them wins over the list's line for its session.
  const listing = new Set<Map<string, SessionSummary | "deleted">>()

  function list(evie: Evie) {
    const heard = new Map<string, SessionSummary | "deleted">()
    listing.add(heard)
    const sessions = evie.send({ type: "list_sessions" }).finally(() => listing.delete(heard)).then((listed) => {
      const lines = new Map<string, SessionSummary>(listed.map((each) => [each.session, each]))
      for (const [session, summary] of heard) {
        if (summary === "deleted") lines.delete(session)
        else lines.set(session, summary)
      }
      set({ sessions: [...lines.values()].sort((a, b) => b.lastActive - a.lastActive) })
    })
    void sessions.then(
      () => set({ listed: true, problem: undefined }),
      (cause: unknown) => set({ problem: `The sessions could not be listed: ${wordsOf(cause)}` }),
    )
    listProjects(evie)
  }

  // The projects are listed one request at a time. Asked for again while
  // one is out, however often, they are listed once more after it: the
  // answers then come in the order they were asked for, so none applied is
  // older than one applied before it.
  // Whoever waits for a list is told once one asked for after they asked
  // has been applied: those waiting on the one out, and those waiting on
  // the one after it.
  let projectsOut = false
  let projectsAgain = false
  let waitingOut: (() => void)[] = []
  let waitingAgain: (() => void)[] = []
  function listProjects(evie: Evie, done?: () => void) {
    if (projectsOut) {
      projectsAgain = true
      if (done) waitingAgain.push(done)
      return
    }
    projectsOut = true
    if (done) waitingOut.push(done)
    void evie
      .send({ type: "list_projects" })
      .then(
        (projects) => set({ projects, projectsProblem: undefined }),
        (cause: unknown) => set({ projectsProblem: `The projects could not be listed: ${wordsOf(cause)}` }),
      )
      .finally(() => {
        projectsOut = false
        for (const told of waitingOut.splice(0)) told()
        if (!projectsAgain) return
        projectsAgain = false
        ;[waitingOut, waitingAgain] = [waitingAgain, waitingOut]
        listProjects(evie)
      })
  }

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
        if (event.type === "deleted") drop(event.session)
      })
    }
    if (evie !== undefined && evie !== connected) list(evie)
    connected = evie
  }
  const unfollow = connection.subscribe(follow)
  follow()

  // Sent on the client the list follows, so that a command asked for while
  // the line is down waits for it, as the client's commands do. Gives back
  // why the server refused, or nothing.
  async function carryOut(command: Command): Promise<string | undefined> {
    if (heard === undefined) return NOT_CONNECTED
    try {
      await heard.send(command)
      return undefined
    } catch (cause) {
      return wordsOf(cause)
    }
  }

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    refreshProjects() {
      const evie = connected
      if (evie === undefined) return Promise.resolve()
      return new Promise<void>((resolve) => listProjects(evie, resolve))
    },
    settle: (session, settled) => carryOut({ type: "settle_session", session, settled }),
    rename: (session, name) => carryOut({ type: "rename_session", session, name }),
    move: (session, project) => carryOut({ type: "move_session", session, project }),
    async delete(session) {
      if (heard === undefined) return { refused: NOT_CONNECTED }
      try {
        return await heard.send({ type: "delete_session", session })
      } catch (cause) {
        return { refused: wordsOf(cause) }
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
