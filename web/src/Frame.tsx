import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject } from "../../src/core/types.ts"
import { useCardShortcut } from "./card-shown.ts"
import type { Connection, ConnectionState } from "./connection.ts"
import { ConnectionLine, ConnectionUnderHeader } from "./ConnectionLine.tsx"
import { ConnectionShown } from "./connection-shown.ts"
import { Conversation, Draft, Placeholder } from "./Conversation.tsx"
import { startedHere } from "./conversation.ts"
import { cn } from "./lib/utils.ts"
import { LIST_ID, toggleList, useListShown } from "./list-shown.ts"
import { Notices } from "./Notices.tsx"
import { Palette } from "./Palette.tsx"
import { useDiffShortcut } from "./diff-shown.ts"
import { DiffPlace } from "./DiffPanel.tsx"
import { CardButton, CardPlace } from "./ProjectCard.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { CaretDown } from "./icons.tsx"
import { NoProjectTile, ProjectGlyph, ProjectTile } from "./ProjectIcon.tsx"
import { select, useSelected } from "./selection.ts"
import { projectLookedAt, titleOf } from "./session-list.ts"
import { ListButton, ListTop, SessionList } from "./SessionList.tsx"
import { HeaderName } from "./SessionRow.tsx"
import type { Sessions } from "./sessions.ts"
import { Settings } from "./Settings.tsx"
import { newSession, windowOver } from "./page-requests.ts"
import { onMac, pressed } from "./shortcuts.ts"
import { SubagentView } from "./Subagent.tsx"
import { workspaceIn } from "./workspace.ts"
import { useWorkspacePlace, WorkspaceView } from "./Workspace.tsx"

// The page once signed in: the session list, the conversation and the
// project card, in columns, as the Session view drawing lays them out. At
// 767px or narrower the columns stack, as the drawing has them: the list
// at the top of the page, and the conversation under it, a screen tall. At
// 980px or narrower the card is a sheet over the page from the right,
// opened from a button in the session's header. With a workspace in the
// address, the workspace view instead. The command palette and the
// notices float over either.
export function Frame({ state, connection, sessions }: { state: ConnectionState; connection: Connection; sessions: Sessions }) {
  useCardShortcut()
  useDiffShortcut()
  const [palette, setPalette] = useState(false)
  useFrameShortcuts(() => setPalette((open) => !open))
  const workspace = useWorkspacePlace()
  return (
    <ConnectionShown.Provider value={state}>
      {workspace !== undefined ? (
        <>
          <WorkspaceView place={workspace} state={state} connection={connection} sessions={sessions} />
          <Settings connection={connection} button={false} />
        </>
      ) : (
        <SessionView state={state} connection={connection} sessions={sessions} />
      )}
      <Palette connection={connection} sessions={sessions} open={palette} close={() => setPalette(false)} />
      <Notices connection={connection} sessions={sessions} />
    </ConnectionShown.Provider>
  )
}

// The session view's columns, which stack at phone width.
function SessionView({ state, connection, sessions }: { state: ConnectionState; connection: Connection; sessions: Sessions }) {
  const list = useListShown()
  const main = useRef<HTMLElement>(null)
  const selected = useSelected()
  // At phone width, a session or a draft picked brings its conversation
  // into sight, under the list; with nothing picked, the list is in sight.
  useEffect(() => {
    if (!list.wide && selected !== undefined) main.current?.scrollIntoView({ block: "start" })
  }, [selected, list.wide])
  return (
    <div className="flex flex-col wide:h-dvh wide:flex-row">
      <ListPlace picked={() => !list.wide && main.current?.scrollIntoView({ block: "start" })}>
        <ListTop connection={connection} sessions={sessions} settings={<Settings connection={connection} />} />
        {list.wide && <ConnectionLine state={state} />}
        <SessionList connection={connection} sessions={sessions} />
      </ListPlace>

      <main ref={main} className="flex h-dvh min-w-0 flex-col bg-bg wide:h-auto wide:flex-1">
        <OpenSession connection={connection} sessions={sessions} />
      </main>

      <DiffPlace connection={connection} sessions={sessions} />
      <CardPlace connection={connection} sessions={sessions} />
    </div>
  )
}

// The session list's place: the column on the left, with a line on its
// right edge, which the owner can hide, or at phone width the top of the
// page, as wide as it, with a line under it. The list is kept on the page
// while hidden, so that the shortcuts that move through it hold. A row
// tapped is `picked`, even the one open, whose address does not change.
function ListPlace({ picked, children }: { picked(): void; children: ReactNode }) {
  const shown = useListShown()
  return (
    <aside
      id={LIST_ID}
      aria-label="Sessions"
      onClick={(event) => event.target instanceof Element && event.target.closest("a[data-row]") !== null && picked()}
      hidden={shown.wide && !shown.column}
      className={cn("flex flex-col bg-bg-side px-2 pb-2", shown.wide ? "w-session-list shrink-0 border-r border-line" : "border-b border-line")}
    >
      {children}
    </aside>
  )
}

// The shortcuts of the frame itself: mod+K opens and closes the command
// palette; mod+shift+O makes a new session, wherever the owner is; and
// mod+B shows and hides the session list, in the session view, which has
// one. While another window is open over the page, such as the settings,
// they wait.
function useFrameShortcuts(togglePalette: () => void) {
  useEffect(() => {
    const mac = onMac()
    const onKey = (event: KeyboardEvent) => {
      const command = pressed(event, mac)
      if (command === undefined || event.defaultPrevented) return
      const over = windowOver()
      if (command === "palette" && (over === undefined || over === "palette")) {
        event.preventDefault()
        togglePalette()
      } else if (command === "new session" && over === undefined) {
        event.preventDefault()
        newSession()
      } else if (command === "session list" && over === undefined && workspaceIn(location.hash) === undefined) {
        event.preventDefault()
        toggleList()
      }
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [togglePalette])
}

// The middle column: the selected session's conversation, under its
// project and name, or one of its subagents', or for a draft, the new
// session that will start, in the project its header picks. With nothing
// open, the page is a draft in the project of the latest session, as T3
// Code opens one, fixed as the empty page first shows, so that another
// session becoming the latest neither moves it nor loses what is typed.
// Whatever it shows, its header has the button that shows and hides the
// project card.
function OpenSession({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  const projectOf = (project: string | null) => list.projects.find((each) => each.id === project)
  const empty = useRef<{ project: string | null }>(undefined)
  if (selected !== undefined) empty.current = undefined
  else if (list.listed) empty.current ??= { project: projectLookedAt(undefined, list.sessions) }
  if (selected === undefined || "draft" in selected) {
    // With nothing open, which project is not known until the list is read.
    if (empty.current === undefined && selected === undefined) return <Top />
    const project = selected === undefined ? empty.current!.project : selected.draft
    return (
      <>
        <Top project={{ id: project, listed: projectOf(project) }} title="New session" pick={{ projects: list.projects, choose: (to) => select({ draft: to }) }} />
        <Draft key={`${project}`} connection={connection} sessions={sessions} project={project} />
      </>
    )
  }
  // A session is opened once the list shows it is one: until the list is
  // read, nothing is shown, and one the list does not have is not opened,
  // unless this page has just started it, and its first message is on its way.
  if (!list.listed) return <Top />
  const session = list.sessions.find((each) => each.session === selected.session)
  const started = startedHere(selected.session)
  if (session === undefined && started === undefined) {
    return <Empty title="This session is not in the list" text="Nothing has been said in it yet, or it is no longer there." />
  }
  const title = session === undefined ? "New session" : titleOf(session)
  if (selected.subagent !== undefined) return <SubagentView connection={connection} parent={selected.session} parentTitle={title} subagent={selected.subagent} />
  const project = session !== undefined ? session.project : (started ?? null)
  return (
    <>
      <Top
        key={selected.session}
        project={{ id: project, listed: projectOf(project) }}
        title={title}
        {...(session !== undefined && { rename: (name: string) => sessions.rename(session.session, name) })}
      />
      <Conversation connection={connection} session={selected.session} folder={projectOf(project)?.folder} />
    </>
  )
}

// The middle column's header, as the Session view drawing has it: the
// project's tile and its name, quiet, then the session's name; after the
// button that shows and hides the session list, and before the one that
// shows and hides the project card, on the right. For a draft, the project
// is a menu to pick another, or none, as T3 Code's draft picks where a new
// thread goes. A name that can be renamed is typed in place after a
// double-click, and why a name was not taken is said on a line under the
// header. At phone width, the line that says the connection is down is
// under it too.
function Top({
  project,
  title,
  rename,
  pick,
}: {
  project?: { id: string | null; listed: ListedProject | undefined }
  title?: string
  rename?(name: string): Promise<string | undefined>
  pick?: { projects: readonly ListedProject[]; choose(project: string | null): void }
}) {
  const [note, setNote] = useState<string>()
  const named = project === undefined ? undefined : (
    <>
      {project.id === null ? <NoProjectTile /> : <ProjectTile project={project.id} size="row" />}
      <span className="truncate">{project.id === null ? "No project" : (project.listed?.name ?? "")}</span>
    </>
  )
  return (
    <>
      <header className="flex h-header-height shrink-0 items-center gap-2 border-b border-line px-3">
        <ListButton />
        {project !== undefined && (
          <span className="flex min-w-0 shrink items-center gap-2 text-body text-text-muted">
            {pick === undefined ? (
              named
            ) : (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button type="button" aria-label={`Start it in: ${project.id === null ? "No project" : (project.listed?.name ?? "its project")}`} className="flex min-w-0 items-center gap-2 rounded-md hover:text-text">
                    {named}
                    <CaretDown className="size-icon-xs shrink-0" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuLabel>Start it in</DropdownMenuLabel>
                  {pick.projects.map((each) => (
                    <DropdownMenuItem key={each.id} current={each.id === project.id} onSelect={() => pick.choose(each.id)}>
                      <ProjectGlyph project={each.id} />
                      {each.name}
                    </DropdownMenuItem>
                  ))}
                  {pick.projects.length > 0 && <DropdownMenuSeparator />}
                  <DropdownMenuItem current={project.id === null} onSelect={() => pick.choose(null)}>
                    No project
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
            <span aria-hidden="true">/</span>
          </span>
        )}
        {title === undefined ? (
          <span className="flex-1" />
        ) : rename === undefined ? (
          <h1 className="min-w-0 flex-1 truncate text-label text-text">{title}</h1>
        ) : (
          <HeaderName title={title} rename={rename} said={setNote} />
        )}
        <CardButton />
      </header>
      <ConnectionUnderHeader />
      <div role="status">{note !== undefined && <p className="px-gutter pt-3 text-caption text-text-muted">{note}</p>}</div>
    </>
  )
}

// The middle column with nothing to read in it, which says why below the header.
function Empty({ title, text }: { title: string; text: string }) {
  return (
    <>
      <Top />
      <Placeholder title={title} text={text} />
    </>
  )
}
