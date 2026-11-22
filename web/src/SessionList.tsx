import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject, SessionState, SessionSummary } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { startedHere } from "./conversation.ts"
import { Button } from "./components/ui/button.tsx"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./components/ui/dropdown-menu.tsx"
import { Input } from "./components/ui/input.tsx"
import { CaretDown, CaretRight, MagnifyingGlass, Plus } from "./icons.tsx"
import { ProjectGlyph } from "./ProjectIcon.tsx"
import { select, useSelected } from "./selection.ts"
import { filterSessions, groupSessions, keepOpenGroups, openGroups, projectLookedAt, revealMove, type Filter, type OpenGroups } from "./session-list.ts"
import { DraftRow, SessionRow } from "./SessionRow.tsx"
import type { Sessions } from "./sessions.ts"

// The session list, in the left column: a new session, a search and a
// project filter above the sessions, which are grouped with those that
// need the owner and the active ones at the top, and the working and the
// settled ones in groups below that open and close. A new session is a
// draft, at the very top, until its first message is sent.
export function SessionList({ sessions }: { sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  const [filter, setFilter] = useState<Filter>({ search: "" })
  const [open, setOpen] = useState(openGroups)
  // The session whose menu should have the focus back once it has moved.
  const [refocus, setRefocus] = useState<string>()
  const now = useMinute()

  const selectedSession = selected !== undefined && "session" in selected ? selected.session : undefined
  const startingIn = selectedSession !== undefined && !list.sessions.some((each) => each.session === selectedSession) ? startedHere(selectedSession) : undefined
  const lookingAt = projectLookedAt(selected, list.sessions, filter)
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

  const groups = groupSessions(filterSessions(list.sessions, filter))
  const rows = (shown: SessionSummary[]) => (
    <ul className="flex flex-col gap-1">
      {shown.map((session) => (
        <SessionRow
          key={session.session}
          session={session}
          project={projectOf(session.project)}
          selected={session.session === selectedSession}
          now={now}
          settle={async (settled) => {
            const refused = await sessions.settle(session.session, settled)
            if (refused === undefined && session.session === selectedSession) setRefocus(session.session)
            return refused
          }}
          refocus={{ asked: refocus === session.session, done: () => setRefocus(undefined) }}
        />
      ))}
    </ul>
  )
  const nothing = groups.top.length + groups.working.length + groups.settled.length === 0

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-col gap-2 px-4 pb-3">
        <NewSession projects={list.projects} lookingAt={lookingAt} start={(project) => select({ draft: project })} />
        <div className="flex gap-1">
          <div className="relative min-w-0 flex-1">
            <MagnifyingGlass className="pointer-events-none absolute top-1/2 left-2 size-icon-sm -translate-y-1/2 text-text-muted" />
            <Input
              type="search"
              aria-label="Search sessions by name"
              placeholder="Search"
              className="pl-8"
              value={filter.search}
              onChange={(event) => setFilter({ ...filter, search: event.target.value })}
            />
          </div>
          <ProjectFilter projects={list.projects} project={filter.project} choose={(project) => setFilter({ search: filter.search, ...(project !== undefined && { project }) })} />
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4 pb-6">
        {[list.problem, list.projectsProblem].map(
          (problem) =>
            problem !== undefined && (
              <p key={problem} className="px-3 pb-2 text-caption text-text-muted">
                {problem}
              </p>
            ),
        )}
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
          <p className="px-3 text-caption text-text-muted">{list.sessions.length === 0 ? "No sessions yet. Start one with New session." : "No sessions match."}</p>
        )}
        {rows(groups.top)}
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
    </div>
  )
}

// A group at the bottom of the list, under a heading that opens and closes it.
function Group({ name, count, open, toggle, children }: { name: string; count: number; open: boolean; toggle(): void; children: ReactNode }) {
  return (
    <section aria-label={name} className="mt-4 flex flex-col gap-1">
      <Button variant="quiet" aria-expanded={open} onClick={toggle} className="justify-start text-caption">
        <CaretRight className={`size-icon-sm transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
        {name}
        <span>{count}</span>
      </Button>
      {open && children}
    </section>
  )
}

// The New session button, which makes a draft of a session in the project
// being looked at, and beside it a menu to make one in another project, or
// in none. Pressed again on a draft, it comes back to the same draft.
function NewSession({ projects, lookingAt, start }: { projects: ListedProject[]; lookingAt: string | null; start(project: string | null): void }) {
  const where = lookingAt === null ? "with no project" : `in ${projects.find((project) => project.id === lookingAt)?.name ?? "its project"}`
  return (
    <div className="flex gap-1">
      <Button variant="secondary" className="flex-1" title={`New session ${where}`} aria-label={`New session ${where}`} onClick={() => start(lookingAt)}>
        <Plus className="size-icon-sm" />
        New session
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="secondary" className="px-3" aria-label="New session in another project">
            <CaretDown className="size-icon-sm" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuLabel>New session in</DropdownMenuLabel>
          {projects.map((project) => (
            <DropdownMenuItem key={project.id} current={project.id === lookingAt} onSelect={() => start(project.id)}>
              <ProjectGlyph project={project.id} />
              {project.name}
            </DropdownMenuItem>
          ))}
          {projects.length > 0 && <DropdownMenuSeparator />}
          <DropdownMenuItem current={lookingAt === null} onSelect={() => start(null)}>
            No project
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}

// The menu that narrows the list to one project, or to the sessions of no
// project, or shows every project's.
function ProjectFilter({ projects, project, choose }: { projects: ListedProject[]; project: string | null | undefined; choose(project: string | null | undefined): void }) {
  const value = project === undefined ? ALL : project === null ? NONE : project
  const name = project === undefined ? "All projects" : project === null ? "No project" : (projects.find((each) => each.id === project)?.name ?? "A project")
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="quiet" aria-label={`Show sessions of: ${name}`} className="max-w-project-filter gap-1 px-3">
          <span className="truncate">{name}</span>
          <CaretDown className="size-icon-sm" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuRadioGroup value={value} onValueChange={(chosen) => choose(chosen === ALL ? undefined : chosen === NONE ? null : chosen)}>
          <DropdownMenuRadioItem value={ALL}>All projects</DropdownMenuRadioItem>
          {projects.map((each) => (
            <DropdownMenuRadioItem key={each.id} value={each.id}>
              <ProjectGlyph project={each.id} />
              {each.name}
            </DropdownMenuRadioItem>
          ))}
          <DropdownMenuRadioItem value={NONE}>No project</DropdownMenuRadioItem>
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
// The filter's values for every project and for no project, which no
// project's id can be: ids are UUIDs.
const ALL = "all projects"
const NONE = "no project"

// The time now, to the minute, so that each row's age moves on.
function useMinute() {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30_000)
    return () => clearInterval(timer)
  }, [])
  return now
}
