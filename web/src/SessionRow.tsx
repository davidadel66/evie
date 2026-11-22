import { useEffect, useRef, useState, type ReactNode } from "react"
import type { ListedProject, SessionSummary } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { Button } from "./components/ui/button.tsx"
import { Dialog, DialogClose, DialogContent, DialogTitle } from "./components/ui/dialog.tsx"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "./components/ui/dropdown-menu.tsx"
import { Check, DotsThree } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { whenAsked } from "./page-requests.ts"
import { ProjectGlyph, ProjectLink } from "./ProjectIcon.tsx"
import { ageWords, nameFieldKey, nameToSave, titleOf } from "./session-list.ts"
import { Spinner } from "./Spinner.tsx"

// What the owner can do to a session from its row, each giving back why
// the server refused, or nothing.
export type RowActions = {
  settle(settled: boolean): Promise<string | undefined>
  rename(name: string): Promise<string | undefined>
  move(project: string | null): Promise<string | undefined>
  // Asks whether to delete the session, in a dialog of the list's.
  askDelete(): void
}

// One session in the list. Its menu renames it, moves it to another project
// or none, settles it or brings it back, and deletes it. Its name is also
// typed in place after a double-click on the row, or F2. When the owner
// has settled or brought back the selected session from its menu,
// `refocus` asks for the focus back on the menu's button, wherever the row
// has moved.
export function SessionRow({
  session,
  project,
  projects,
  selected,
  now,
  actions,
  editing,
  refocus,
}: {
  session: SessionSummary
  // Undefined for a session with no project, and for a moment for one in a
  // project made since the projects were listed.
  project: ListedProject | undefined
  // Every project, for the session to be moved to.
  projects: ListedProject[]
  selected: boolean
  now: number
  actions: RowActions
  // Whether its name is being typed in place, and what starts and ends that.
  editing: { now: boolean; start(): void; end(): void }
  refocus: { asked: boolean; done(): void }
}) {
  // Why the server would not do what was asked, shown under it until its state changes.
  const [note, setNote] = useState<string>()
  useEffect(() => setNote(undefined), [session.state])
  // A name being saved shows until the server has it.
  const [saving, setSaving] = useState<string>()
  const title = saving ?? titleOf(session)

  async function save(typed: string) {
    editing.end()
    const result = nameToSave(typed, title)
    if (result === undefined) return
    if ("refused" in result) return setNote(result.refused)
    setSaving(result.name)
    setNote(await actions.rename(result.name))
    setSaving(undefined)
  }

  return (
    <Row
      project={session.project}
      listed={project}
      title={title}
      href={addressOf({ session: session.session })}
      session={session.session}
      selected={selected}
      state={<State session={session} now={now} />}
      field={editing.now ? <NameField title={title} save={save} cancel={editing.end} /> : undefined}
      edit={editing.start}
      menu={
        <RowMenu
          title={title}
          settled={session.state === "settled"}
          project={session.project}
          projects={projects}
          selected={selected}
          settle={async (settled) => setNote(await actions.settle(settled))}
          move={async (to) => setNote(await actions.move(to))}
          rename={editing.start}
          askDelete={actions.askDelete}
          opened={() => setNote(undefined)}
          refocus={refocus}
        />
      }
      note={note}
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
// selects it, and a double-click on it edits its name, when it has one to
// edit; while it is edited, a field takes the link's place. The selected
// row is raised. A note, such as why the server refused, goes on a line
// under it.
function Row({
  project,
  listed,
  title,
  href,
  session,
  selected,
  state,
  field,
  edit,
  menu,
  note,
}: {
  project: string | null
  listed: ListedProject | undefined
  title: string
  href: string
  session?: string
  selected: boolean
  state?: ReactNode
  field?: ReactNode
  edit?(): void
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
        {field ?? (
          // Its box covers the whole row, so the row is the link; the menu sits above it.
          <a
            href={href}
            data-row=""
            data-session={session}
            aria-current={selected ? "page" : undefined}
            onDoubleClick={
              edit &&
              ((event) => {
                event.preventDefault()
                edit()
              })
            }
            className="min-w-0 flex-1 truncate text-label text-text outline-none after:absolute after:inset-0 after:rounded-md focus-visible:after:outline-(length:--indicator-width) focus-visible:after:outline-offset-2 focus-visible:after:outline-focus-ring"
          >
            {title}
          </a>
        )}
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

// The session's name, typed in place of its title, as in T3 Code: Enter or
// leaving the field saves it, and Escape gives it up. It starts with the
// whole name chosen, to be typed over. `size` is the title's text style:
// a row's, or the conversation header's.
function NameField({ title, save, cancel, size = "text-label" }: { title: string; save(typed: string): void; cancel(): void; size?: "text-label" | "text-title" }) {
  const [typed, setTyped] = useState(title)
  // Enter and Escape end the edit, and the field then loses the focus as
  // it goes, which must not save it again.
  const ended = useRef(false)
  return (
    <input
      autoFocus
      aria-label="Session name"
      value={typed}
      onChange={(event) => setTyped(event.target.value)}
      onFocus={(event) => event.currentTarget.select()}
      onKeyDown={(event) => {
        // The keys are the field's, not the page's, such as Escape stopping a turn.
        event.stopPropagation()
        const action = nameFieldKey(event.key, event.nativeEvent.isComposing)
        if (action === undefined) return
        event.preventDefault()
        ended.current = true
        if (action === "save") save(typed)
        else cancel()
      }}
      onBlur={() => !ended.current && save(typed)}
      className={cn("relative z-10 -mx-1 min-w-0 flex-1 rounded-md bg-transparent px-1 text-text focus-visible:outline-offset-0", size)}
    />
  )
}

// The session's name as the conversation's header shows it. A double-click
// types it in place, as in T3 Code, with the same rules as in its row.
// `said` is told why a name was not taken, or with nothing that there is
// nothing to say.
export function HeaderName({ title, rename, said }: { title: string; rename(name: string): Promise<string | undefined>; said(note: string | undefined): void }) {
  const [editing, setEditing] = useState(false)
  // The command palette's Rename this session renames it here.
  useEffect(() => whenAsked("rename", () => setEditing(true)), [])
  const [saving, setSaving] = useState<string>()
  const shown = saving ?? title
  async function save(typed: string) {
    setEditing(false)
    const result = nameToSave(typed, shown)
    if (result === undefined) return
    if ("refused" in result) return said(result.refused)
    setSaving(result.name)
    said(await rename(result.name))
    setSaving(undefined)
  }
  if (editing) return <NameField title={shown} save={save} cancel={() => setEditing(false)} size="text-title" />
  return (
    <h1
      onDoubleClick={() => {
        said(undefined)
        setEditing(true)
      }}
      className="min-w-0 flex-1 truncate text-title text-text"
    >
      {shown}
    </h1>
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
//
// Rename and Delete… act once the menu has closed: the name's field then
// keeps the focus the menu would give back to its button, and the dialog
// that asks about deleting gives it back there when it closes.
function RowMenu({
  title,
  settled,
  project,
  projects,
  selected,
  settle,
  move,
  rename,
  askDelete,
  opened,
  refocus,
}: {
  title: string
  settled: boolean
  project: string | null
  projects: ListedProject[]
  selected: boolean
  settle(settled: boolean): void
  move(project: string | null): void
  rename(): void
  askDelete(): void
  opened(): void
  refocus: { asked: boolean; done(): void }
}) {
  const button = useRef<HTMLButtonElement>(null)
  const afterClosing = useRef<"rename" | "delete">(undefined)
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
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          const then = afterClosing.current
          afterClosing.current = undefined
          if (then === "rename") {
            event.preventDefault()
            rename()
          }
          if (then === "delete") setTimeout(askDelete)
        }}
      >
        <DropdownMenuItem onSelect={() => (afterClosing.current = "rename")}>Rename</DropdownMenuItem>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger>Move to…</DropdownMenuSubTrigger>
          <DropdownMenuSubContent>
            {projects.map((each) => (
              <DropdownMenuItem key={each.id} disabled={each.id === project} onSelect={() => move(each.id)}>
                <ProjectGlyph project={each.id} />
                {each.name}
                {each.id === project && <Current />}
              </DropdownMenuItem>
            ))}
            {projects.length > 0 && <DropdownMenuSeparator />}
            <DropdownMenuItem disabled={project === null} onSelect={() => move(null)}>
              No project
              {project === null && <Current />}
            </DropdownMenuItem>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuItem onSelect={() => settle(!settled)}>{settled ? "Bring back" : "Settle"}</DropdownMenuItem>
        <DropdownMenuSeparator />
        {/* The one destructive action, in the one colour for it. */}
        <DropdownMenuItem className="text-status-failed" onSelect={() => (afterClosing.current = "delete")}>
          Delete…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// The project a session is in, among those it can be moved to: marked, as
// the one it is in now, and not to be picked.
function Current() {
  return (
    <span className="ml-auto flex items-center gap-1 pl-4 text-caption text-text-muted">
      <Check className="size-icon-sm" />
      Current
    </span>
  )
}

// Asks whether to delete a session for good, naming it, with Cancel and
// Delete. It stays open while the session is deleted, and says why when
// the server refuses, as while the session is working.
export function DeleteDialog({ session, close, remove }: { session: SessionSummary | undefined; close(): void; remove(session: SessionSummary): Promise<string | undefined> }) {
  const [deleting, setDeleting] = useState(false)
  const [problem, setProblem] = useState<string>()
  useEffect(() => {
    setDeleting(false)
    setProblem(undefined)
  }, [session?.session])
  async function confirm() {
    if (session === undefined) return
    setDeleting(true)
    const refused = await remove(session)
    setDeleting(false)
    if (refused === undefined) close()
    else setProblem(refused)
  }
  return (
    <Dialog open={session !== undefined} onOpenChange={(open) => !open && close()}>
      <DialogContent>
        <DialogTitle>Delete this session for good?</DialogTitle>
        <p className="text-body text-text-muted">
          <span className="text-text">{session === undefined ? "" : titleOf(session)}</span> goes with everything in it: its conversation, its subagents and the files evie kept for it. Files it
          changed in a project stay as they are. This cannot be undone.
        </p>
        {problem !== undefined && (
          <p role="status" className="text-caption text-text-muted">
            {problem}
          </p>
        )}
        <div className="flex justify-end gap-2">
          <DialogClose asChild>
            <Button variant="secondary">Cancel</Button>
          </DialogClose>
          <Button variant="secondary" className="text-status-failed" disabled={deleting} onClick={confirm}>
            {deleting ? "Deleting…" : "Delete"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
