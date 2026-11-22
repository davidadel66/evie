import { useEffect, useRef, useState, type ReactNode } from "react"
import type { ListedProject, SessionSummary } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { DotsThree } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { ProjectLink } from "./ProjectIcon.tsx"
import { ageWords, titleOf } from "./session-list.ts"
import { Spinner } from "./Spinner.tsx"

// One session in the list. Its menu settles it or brings it back. When the
// owner has done that from the menu of the selected session, `refocus`
// asks for the focus back on the menu's button, wherever the row has moved.
export function SessionRow({
  session,
  project,
  selected,
  now,
  settle,
  refocus,
}: {
  session: SessionSummary
  // Undefined for a session with no project, and for a moment for one in a
  // project made since the projects were listed.
  project: ListedProject | undefined
  selected: boolean
  now: number
  // Settles the session, or brings it back, and gives back why not.
  settle(settled: boolean): Promise<string | undefined>
  refocus: { asked: boolean; done(): void }
}) {
  // Why the server would not settle it, shown under it until it changes.
  const [refused, setRefused] = useState<string>()
  useEffect(() => setRefused(undefined), [session.state])
  const title = titleOf(session)

  return (
    <Row
      project={session.project}
      listed={project}
      title={title}
      href={addressOf({ session: session.session })}
      selected={selected}
      state={<State session={session} now={now} />}
      menu={
        <RowMenu
          title={title}
          settled={session.state === "settled"}
          selected={selected}
          choose={async (settled) => setRefused(await settle(settled))}
          opened={() => setRefused(undefined)}
          refocus={refocus}
        />
      }
      note={refused}
    />
  )
}

// A new session not yet started, in a project or in none: the selected row,
// at the top of the list, until its first message is sent.
export function DraftRow({ project, listed, href }: { project: string | null; listed: ListedProject | undefined; href: string }) {
  return <Row project={project} listed={listed} title="New session" href={href} selected />
}

// A row in three short lines: the project's icon and name, with the state
// or how long ago the session was active; its title, ending in an ellipsis
// when cut, and its menu's button in a space of its own; and the branch its
// project is on, left out with no project. The whole row is the link that
// selects it; the selected row is raised. A note, such as why the server
// refused, goes on a line under it.
function Row({
  project,
  listed,
  title,
  href,
  selected,
  state,
  menu,
  note,
}: {
  project: string | null
  listed: ListedProject | undefined
  title: string
  href: string
  selected: boolean
  state?: ReactNode
  menu?: ReactNode
  note?: string | undefined
}) {
  return (
    <li className={cn("group relative flex flex-col rounded-md px-3 py-2", selected && "bg-surface-raised")}>
      <span className="flex items-center gap-2 text-caption text-text-muted">
        {project === null ? <span className="min-w-0 flex-1 truncate">No project</span> : <ProjectLink project={project} name={listed?.name} />}
        {state}
      </span>
      <span className="flex items-center">
        {/* Its box covers the whole row, so the row is the link; the menu sits above it. */}
        <a
          href={href}
          aria-current={selected ? "page" : undefined}
          className="min-w-0 flex-1 truncate text-label text-text outline-none after:absolute after:inset-0 after:rounded-md focus-visible:after:outline-(length:--indicator-width) focus-visible:after:outline-offset-2 focus-visible:after:outline-focus-ring"
        >
          {title}
        </a>
        {menu}
      </span>
      {project !== null && listed?.branch !== undefined && <span className="truncate text-caption text-text-muted">{listed.branch}</span>}
      {note !== undefined && (
        <span role="status" className="pt-1 text-caption text-text-muted">
          {note}
        </span>
      )}
    </li>
  )
}

// The state in words where the owner is needed or evie is at work, and
// otherwise how long ago the session was active.
function State({ session, now }: { session: SessionSummary; now: number }) {
  if (session.state === "needs_you") return <span className="shrink-0 text-status-needs">Needs you</span>
  if (session.state === "working") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-status-working">
        <Spinner />
        Working
      </span>
    )
  }
  return <span className="shrink-0">{ageWords(session.lastActive, now)}</span>
}

// The row's menu, behind an icon button at the end of its title, in a space
// of its own so that the title always ends in an ellipsis. Taller than its
// line, for the pointer, without making the line taller, it reaches into the
// row's padding on the right. It shows on the selected row, and on any row
// while the pointer is over it or it has the keyboard's focus; otherwise a
// tap goes through to the row.
function RowMenu({
  title,
  settled,
  selected,
  choose,
  opened,
  refocus,
}: {
  title: string
  settled: boolean
  selected: boolean
  choose(settled: boolean): void
  opened(): void
  refocus: { asked: boolean; done(): void }
}) {
  const button = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    if (!refocus.asked) return
    button.current?.focus()
    refocus.done()
  }, [refocus])
  return (
    <DropdownMenu onOpenChange={(open) => open && opened()}>
      <DropdownMenuTrigger asChild>
        <Button
          ref={button}
          variant="icon"
          aria-label={`Actions for ${title}`}
          className={cn(
            "relative z-10 -my-3 -mr-3",
            !selected &&
              "pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:pointer-events-auto data-[state=open]:opacity-100",
          )}
        >
          <DotsThree />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => choose(!settled)}>{settled ? "Bring back" : "Settle"}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
