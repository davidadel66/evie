import { useEffect, useState } from "react"
import type { ListedProject } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { DotsThree } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { ProjectIcon } from "./ProjectIcon.tsx"
import { ageWords, titleOf } from "./session-list.ts"
import type { ListedSession } from "./sessions.ts"
import { Spinner } from "./Spinner.tsx"

// One session in the list, in three short lines: its project's icon and
// name, with its state or how long ago it was active; its title; and the
// branch its project is on, left out for a session with no project. The
// whole row is the link that selects it; the selected row is raised. Its
// menu settles the session or brings it back.
export function SessionRow({
  session,
  project,
  selected,
  now,
  settle,
}: {
  session: ListedSession
  // Undefined for a session with no project, and for a moment for one in a
  // project made since the projects were listed.
  project: ListedProject | undefined
  selected: boolean
  now: number
  // Settles the session, or brings it back, and gives back why not.
  settle(settled: boolean): Promise<string | undefined>
}) {
  // Why the server would not settle it, shown under it until it changes.
  const [refused, setRefused] = useState<string>()
  useEffect(() => setRefused(undefined), [session.state])
  const title = titleOf(session)

  return (
    <li className={cn("group relative flex flex-col rounded-md px-3 py-2", selected && "bg-surface-raised")}>
      <span className="flex items-center gap-2 text-caption text-text-muted">
        {session.project !== null && <ProjectIcon project={session.project} />}
        <span className="min-w-0 flex-1 truncate">{session.project === null ? "No project" : project?.name}</span>
        <State session={session} now={now} />
      </span>
      <span className="flex">
        {/* Its box covers the whole row, so the row is the link; the menu sits above it. */}
        <a
          href={addressOf(session.session)}
          aria-current={selected ? "page" : undefined}
          className="min-w-0 flex-1 truncate text-label text-text outline-none after:absolute after:inset-0 after:rounded-md focus-visible:after:outline-2 focus-visible:after:outline-offset-2 focus-visible:after:outline-focus-ring"
        >
          {title}
        </a>
        {!session.new && (
          <RowMenu
            title={title}
            settled={session.state === "settled"}
            selected={selected}
            choose={async (settled) => setRefused(await settle(settled))}
            opened={() => setRefused(undefined)}
          />
        )}
      </span>
      {session.project !== null && project?.branch !== undefined && <span className="truncate text-caption text-text-muted">{project.branch}</span>}
      {refused !== undefined && (
        <span role="status" className="pt-1 text-caption text-text-muted">
          {refused}
        </span>
      )}
    </li>
  )
}

// The state in words where the owner is needed or evie is at work, and
// otherwise how long ago the session was active.
function State({ session, now }: { session: ListedSession; now: number }) {
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

// The row's menu, behind a quiet button over the end of its title. The
// button shows on the selected row, and on any row while the pointer is
// over it or it has the keyboard's focus; otherwise the title has the whole
// width, and a tap goes through to the row. Shown, it hides the end of the
// title under the row's own ground, and takes a click a little around it.
function RowMenu({
  title,
  settled,
  selected,
  choose,
  opened,
}: {
  title: string
  settled: boolean
  selected: boolean
  choose(settled: boolean): void
  opened(): void
}) {
  return (
    <DropdownMenu onOpenChange={(open) => open && opened()}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Actions for ${title}`}
          className={cn(
            "relative z-10 -ml-8 flex w-8 shrink-0 items-center justify-center rounded-sm text-text-muted before:absolute before:-inset-x-1 before:-inset-y-3 hover:text-text [&>svg]:size-icon-md",
            selected ? "bg-surface-raised" : "bg-bg-side",
            !selected &&
              "pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 focus-visible:opacity-100 data-[state=open]:pointer-events-auto data-[state=open]:opacity-100",
          )}
        >
          <DotsThree />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => choose(!settled)}>{settled ? "Bring back" : "Settle"}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
