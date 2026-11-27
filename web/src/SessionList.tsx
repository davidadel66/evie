import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { SessionState, SessionSummary } from "../../src/core/types.ts"
import { addressOf, selectedIn } from "./address.ts"
import { startedHere } from "./conversation.ts"
import { Button, buttonVariants } from "./components/ui/button.tsx"
import { CaretRight, MagnifyingGlass, PencilSimpleLine, SidebarLeft, SquaresFour } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { toggleList, useListShown } from "./list-shown.ts"
import { select, useSelected } from "./selection.ts"
import { besides, deletedSelection, filterSessions, groupSessions, inListOrder, keepOpenGroups, openGroups, projectLookedAt, renamedByF2, revealMove, titleOf, workspaceToOpen, type OpenGroups } from "./session-list.ts"
import { DeleteDialog, DraftRow, SessionRow } from "./SessionRow.tsx"
import type { Connection } from "./connection.ts"
import { NewWorkspace } from "./NewWorkspace.tsx"
import type { Sessions } from "./sessions.ts"
import { ask, newSession, whenAsked, windowOver } from "./page-requests.ts"
import { label as keyLabel, onMac, pressed } from "./shortcuts.ts"
import { addressOfWorkspace } from "./workspace.ts"

// The top of the session list, one row in the list's header: evie's name,
// filling the room, then the Workspaces button, which opens the workspace
// view, the New session button, which starts a draft with no project, and
// the settings button given. Workspaces
// opens the workspace of the project being looked at, or with no project at
// all, the New workspace dialog; until the projects have been listed, which
// it is not yet known, it waits.
export function ListTop({ connection, sessions, settings }: { connection: Connection; sessions: Sessions; settings: ReactNode }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  const lookingAt = projectLookedAt(selected, list.sessions)
  const workspace = workspaceToOpen(lookingAt, list.sessions, list.projects)
  const icon = cn(buttonVariants({ variant: "icon" }))
  return (
    <header className="flex h-header-height shrink-0 items-center gap-0.5 pl-row-inset">
      <span className="flex-1 text-title text-text">evie</span>
      {!list.projectsListed ? (
        <Button variant="icon" aria-label="Workspaces" title="Workspaces: the projects are being listed" disabled>
          <SquaresFour />
        </Button>
      ) : workspace !== undefined ? (
        <a href={addressOfWorkspace({ workspace })} aria-label="Workspaces" title="Workspaces" className={icon}>
          <SquaresFour />
        </a>
      ) : (
        <NewWorkspace connection={connection} sessions={sessions}>
          <Button variant="icon" aria-label="Workspaces" title="Workspaces: make the first one">
            <SquaresFour />
          </Button>
        </NewWorkspace>
      )}
      <Button variant="icon" aria-label="New session" title={`New session (${keyLabel("new session", onMac())})`} onClick={newSession}>
        <PencilSimpleLine />
      </Button>
      {settings}
    </header>
  )
}

// The session list, in the left column: a search above the sessions,
// which are grouped with those that need the owner and the active ones at
// the top, and the working and the settled ones in groups at the bottom of
// the column that open and close. A new session is a draft, at the very
// top, until its first message is sent.
export function SessionList({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  // Whether the New workspace dialog is open, from the empty list's New workspace… button.
  const [making, setMaking] = useState(false)
  const selected = useSelected()
  const [search, setSearch] = useState("")
  const [open, setOpen] = useState(openGroups)
  // The session whose menu should have the focus back once it has moved.
  const [refocus, setRefocus] = useState<string>()
  // The session whose name is being typed in its row, and the one asked
  // about deleting.
  const [editing, setEditing] = useState<string>()
  const [deleting, setDeleting] = useState<SessionSummary>()
  // A quiet word about the list, such as that the selected session was
  // deleted, until something else is selected.
  const [notice, setNotice] = useState<string>()
  const now = useMinute()

  const selectedSession = selected !== undefined && "session" in selected ? selected.session : undefined
  const startingIn = selectedSession !== undefined && !list.sessions.some((each) => each.session === selectedSession) ? startedHere(selectedSession) : undefined
  const projectOf = (project: string | null) => list.projects.find((each) => each.id === project)

  function setGroups(next: OpenGroups) {
    setOpen(next)
    keepOpenGroups(next)
  }

  // A selected session that moves into a closed group, as when the owner
  // settles it, opens that group, so that it stays in sight.
  const selectedState = list.sessions.find((each) => each.session === selectedSession)?.state
  const seen = useRef<{ session?: string; state?: SessionState }>({})
  useEffect(() => {
    const before = seen.current.session === selectedSession ? seen.current.state : undefined
    seen.current = { ...(selectedSession !== undefined && { session: selectedSession }), ...(selectedState !== undefined && { state: selectedState }) }
    if (selectedState === undefined) return
    const next = revealMove(open, before, selectedState)
    if (next !== open) setGroups(next)
  }, [selectedSession, selectedState])

  // F2 renames, as in T3 Code: while the list is shown, the row that has
  // the focus, or else the selected session's, when its row is in sight;
  // with the list hidden, or the row not in it, the selected session in
  // the header above its conversation.
  const listShown = useListShown()
  const listInSight = useRef(false)
  // At phone width, the list is on the page, above the conversation.
  listInSight.current = listShown.wide ? listShown.column : true
  useEffect(() => {
    function renameOnF2(event: KeyboardEvent) {
      if (pressed(event, onMac()) !== "rename" || event.defaultPrevented || windowOver() !== undefined) return
      const focused = document.activeElement
      const typing = focused instanceof HTMLInputElement || focused instanceof HTMLTextAreaElement || (focused instanceof HTMLElement && focused.isContentEditable)
      // A row's link, which has no session when it is a draft's.
      const focusedRow = focused instanceof HTMLAnchorElement && focused.dataset.row !== undefined ? (focused.dataset.session ?? null) : undefined
      const renamed = renamedByF2({ ...(focusedRow !== undefined && { focusedRow }), typing, listShown: listInSight.current }, selectedSession)
      if (renamed === undefined) return
      event.preventDefault()
      if ("row" in renamed && document.querySelector(`a[data-session="${CSS.escape(renamed.row)}"]`) !== null) setEditing(renamed.row)
      else ask("rename")
    }
    addEventListener("keydown", renameOnF2)
    return () => removeEventListener("keydown", renameOnF2)
  }, [selectedSession])

  // The selected session, deleted from here or elsewhere, is let go: its
  // conversation, a subagent's of it, and its card are no longer shown,
  // and the list says so.
  const gone = deletedSelection(selected, list.deleted)
  useEffect(() => {
    if (gone === undefined) return
    location.hash = ""
    setNotice("This session was deleted.")
  }, [gone])
  useEffect(() => {
    if (selected !== undefined) setNotice(undefined)
  }, [selected])

  // Gives back why the server refused, or nothing. A delete that could not
  // remove everything kept for the session still went, and the list says
  // what was left.
  async function remove(session: SessionSummary) {
    const done = await sessions.delete(session.session)
    if ("refused" in done) return done.refused
    if (session.session === selectedSession) location.hash = ""
    if (done.leftOver.length > 0) setNotice(`${titleOf(session)} was deleted, but not all that evie kept for it could be removed. ${done.leftOver.join(" ")}`)
    return undefined
  }

  const projectName = (project: string | null) => (project === null ? "No project" : projectOf(project)?.name)
  const groups = groupSessions(filterSessions(list.sessions, search, projectName))

  // The session above or below the selected one, by its shortcut or from
  // the palette, as T3 Code moves through its threads.
  const order = inListOrder(groups, open)
  const moves = useRef({ order })
  moves.current = { order }
  useEffect(() => {
    const mac = onMac()
    // As the address has it now, which a key pressed straight after
    // another can be ahead of the list's drawing.
    function step(direction: "previous" | "next") {
      const now = selectedIn(location.hash)
      const to = besides(moves.current.order, now !== undefined && "session" in now ? now.session : undefined, direction)
      if (to !== undefined) select({ session: to })
    }
    function onKey(event: KeyboardEvent) {
      const command = pressed(event, mac)
      if ((command !== "previous session" && command !== "next session") || event.defaultPrevented || windowOver() !== undefined) return
      event.preventDefault()
      step(command === "previous session" ? "previous" : "next")
    }
    addEventListener("keydown", onKey)
    const stops = [
      whenAsked("previous session", () => step("previous")),
      whenAsked("next session", () => step("next")),
    ]
    return () => {
      removeEventListener("keydown", onKey)
      for (const stop of stops) stop()
    }
  }, [])

  const rows = (shown: SessionSummary[]) => (
    <ul className="flex flex-col gap-1">
      {shown.map((session) => (
        <SessionRow
          key={session.session}
          session={session}
          project={projectOf(session.project)}
          projects={list.projects}
          selected={session.session === selectedSession}
          now={now}
          actions={{
            settle: async (settled) => {
              const refused = await sessions.settle(session.session, settled)
              if (refused === undefined && session.session === selectedSession) setRefocus(session.session)
              return refused
            },
            rename: (name) => sessions.rename(session.session, name),
            move: (project) => sessions.move(session.session, project),
            askDelete: () => setDeleting(session),
          }}
          editing={{ now: editing === session.session, start: () => setEditing(session.session), end: () => setEditing(undefined) }}
          refocus={{ asked: refocus === session.session, done: () => setRefocus(undefined) }}
        />
      ))}
    </ul>
  )
  const nothing = groups.top.length + groups.working.length + groups.settled.length === 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <NewWorkspace connection={connection} sessions={sessions} open={making} onOpenChange={setMaking} />
      {/* A bare field, with no box around it: a magnifier and the words typed. */}
      <label className="flex h-control shrink-0 items-center gap-2 px-row-inset text-text-muted">
        <MagnifyingGlass className="size-icon-sm shrink-0" />
        <span className="sr-only">Search sessions</span>
        <input
          type="search"
          placeholder="Search"
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="min-w-0 flex-1 bg-transparent text-field text-text outline-none placeholder:text-text-muted"
        />
      </label>

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto pt-1">
        {[list.problem, list.projectsProblem].map(
          (problem) =>
            problem !== undefined && (
              <p key={problem} className="px-row-inset pb-2 text-caption text-text-muted">
                {problem}
              </p>
            ),
        )}
        <div role="status">{notice !== undefined && <p className="px-row-inset pb-2 text-caption text-text-muted">{notice}</p>}</div>
        {selected !== undefined && "draft" in selected && (
          <ul className="mb-1 flex flex-col">
            <DraftRow project={selected.draft} listed={projectOf(selected.draft)} href={addressOf(selected)} />
          </ul>
        )}
        {/* A session started here from a draft stays in its place until its first message lists it. */}
        {startingIn !== undefined && (
          <ul className="mb-1 flex flex-col">
            <DraftRow project={startingIn} listed={projectOf(startingIn)} href={addressOf(selected!)} />
          </ul>
        )}
        {list.listed && nothing && (
          <p className="px-row-inset text-caption text-text-muted">{list.sessions.length === 0 ? "No sessions yet. Start one with New session." : "No sessions match."}</p>
        )}
        {list.listed && list.sessions.length === 0 && (
          <Button variant="quiet" className="self-start" onClick={() => setMaking(true)}>
            New workspace…
          </Button>
        )}
        {rows(groups.top)}
        {/* Pinned to the bottom of the column, or below the rows when the list is longer than the column. */}
        {groups.working.length + groups.settled.length > 0 && (
          <div className="mt-auto flex flex-col gap-1 pt-6">
            {groups.working.length > 0 && (
              <Group name="Working" count={groups.working.length} open={open.working} toggle={() => setGroups({ ...open, working: !open.working })}>
                {rows(groups.working)}
              </Group>
            )}
            {groups.settled.length > 0 && (
              <Group name="Settled" count={groups.settled.length} open={open.settled} toggle={() => setGroups({ ...open, settled: !open.settled })}>
                {rows(groups.settled)}
              </Group>
            )}
          </div>
        )}
      </div>
      <DeleteDialog session={deleting} close={() => setDeleting(undefined)} remove={remove} />
    </div>
  )
}

// The button at the left of the middle column's header that shows and
// hides the session list's column at desktop width, and at phone width,
// where the list is above the conversation, scrolls up to it.
export function ListButton() {
  const shown = useListShown()
  const label = !shown.wide ? "Show the session list" : shown.column ? "Hide the session list" : "Show the session list"
  return (
    <Button
      variant="icon"
      aria-label={label}
      {...(shown.wide && { "aria-expanded": shown.column })}
      title={`${label} (${keyLabel("session list", onMac())})`}
      onClick={toggleList}
      className="size-control-sm"
    >
      <SidebarLeft />
    </Button>
  )
}

// A group at the bottom of the list, under a heading that opens and closes
// it: its name on the left, its count and a caret at the right end. A row
// of the list, as T3 Code's, so it does not grow below 640px.
function Group({ name, count, open, toggle, children }: { name: string; count: number; open: boolean; toggle(): void; children: ReactNode }) {
  return (
    <section aria-label={name} className="flex flex-col gap-1">
      <Button variant="quiet" aria-expanded={open} onClick={toggle} className="h-row-height justify-start gap-2 px-row-inset text-caption">
        <span className="flex-1 text-left">{name}</span>
        <span>{count}</span>
        <CaretRight className={`size-icon-caret transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
      </Button>
      {open && children}
    </section>
  )
}

// The time now, to the minute, so that each row's age moves on.
function useMinute() {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30_000)
    return () => clearInterval(timer)
  }, [])
  return now
}
