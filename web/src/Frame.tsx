import { useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject } from "../../src/core/types.ts"
import { settingsIn } from "./address.ts"
import { useCardShortcut } from "./card-shown.ts"
import { CardButton, CardLane, CardPlace } from "./CardPlace.tsx"
import { Button } from "./components/ui/button.tsx"
import type { Connection, ConnectionState } from "./connection.ts"
import { ConnectionLine, ConnectionUnderHeader } from "./ConnectionLine.tsx"
import { ConnectionShown } from "./connection-shown.ts"
import { Conversation, Draft, Placeholder } from "./Conversation.tsx"
import { startedHere } from "./conversation.ts"
import { Screen, whenIdle } from "./later.ts"
import { cn } from "./lib/utils.ts"
import { LIST_ID, toggleList, useListShown } from "./list-shown.ts"
import { Notices } from "./Notices.tsx"
import { toggleDiff, useDiffShortcut, useDiffShown } from "./diff-shown.ts"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { CaretDown, GearSix, SidebarRight } from "./icons.tsx"
import { NoProjectTile, ProjectGlyph, ProjectTile } from "./ProjectIcon.tsx"
import { Palette, RightPanel, SCREENS, SettingsPage, WorkspaceView } from "./screens.ts"
import { openSettings, select, useAddress, useSelected, useWorkspacePlace } from "./selection.ts"
import { projectLookedAt, titleOf } from "./session-list.ts"
import { ListButton, ListTop, SessionList } from "./SessionList.tsx"
import { HeaderName } from "./SessionRow.tsx"
import type { Sessions } from "./sessions.ts"
import { newSession, windowOver } from "./page-requests.ts"
import { label as keyLabel, onMac, pressed } from "./shortcuts.ts"
import { SubagentView } from "./Subagent.tsx"
import { workspaceIn } from "./workspace.ts"

// The page once signed in: the session list and the conversation, in
// columns, as the Session view drawing lays them out, with the session
// details floating at the conversation's top right, as T3 Code's do. At
// 767px or narrower the columns stack, as the drawing has them: the list
// at the top of the page, and the conversation under it, a screen tall.
// The details open from a button in the session's header. With a
// workspace in the address, the workspace view instead. The settings page,
// the command palette and the notices float over either.
//
// The screens opened rarely or later, the settings, the palette, the
// details, the right panel and the workspace view, are in files of their
// own (see screens.ts), which the page fetches once it is idle, so that
// each opens at once the first time; one opened before then shows once it
// is here.
export function Frame({ state, connection, sessions }: { state: ConnectionState; connection: Connection; sessions: Sessions }) {
  useCardShortcut()
  useDiffShortcut()
  const [palette, setPalette] = useState(false)
  useFrameShortcuts(() => setPalette((open) => !open))
  useEffect(() => whenIdle(SCREENS.preload), [])
  const workspace = useWorkspacePlace()
  const settings = settingsIn(useAddress()) !== undefined
  return (
    <ConnectionShown.Provider value={state}>
      {workspace !== undefined ? (
        // Until the view is here, the screen it will take, empty.
        <Screen loads={SCREENS} wanted fallback={<div className="h-dvh" />}>
          <WorkspaceView place={workspace} state={state} connection={connection} sessions={sessions} />
        </Screen>
      ) : (
        <SessionView state={state} connection={connection} sessions={sessions} />
      )}
      <Screen loads={SCREENS} wanted={settings}>
        <SettingsPage connection={connection} />
      </Screen>
      <Screen loads={SCREENS} wanted={palette}>
        <Palette connection={connection} sessions={sessions} open={palette} close={() => setPalette(false)} />
      </Screen>
      <Notices connection={connection} sessions={sessions} />
    </ConnectionShown.Provider>
  )
}

// The gear that opens the settings, as T3 Code's settings button.
function SettingsButton() {
  return (
    <Button variant="icon" aria-label="Settings" title="Settings" onClick={() => openSettings()}>
      <GearSix />
    </Button>
  )
}

// The right panel (see DiffPanel.tsx), on the page once it is first opened
// or its file is here.
function RightPanelPlace({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useDiffShown()
  return (
    <Screen loads={SCREENS} wanted={shown.open}>
      <RightPanel connection={connection} sessions={sessions} />
    </Screen>
  )
}

// The button in the conversation's header that shows and hides the right
// panel, raised while it is shown.
function PanelButton() {
  const shown = useDiffShown()
  return (
    <button
      type="button"
      aria-label="Show or hide the right panel"
      aria-pressed={shown.open}
      title={`Right panel (${keyLabel("diff", onMac())})`}
      onClick={toggleDiff}
      className={cn("flex size-control-sm shrink-0 items-center justify-center rounded-md text-text-muted hover:bg-text/8 hover:text-text", shown.open && "bg-surface-raised text-text")}
    >
      <SidebarRight className="size-icon-sm" />
    </button>
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
        <ListTop connection={connection} sessions={sessions} settings={<SettingsButton />} />
        {list.wide && <ConnectionLine state={state} />}
        <SessionList connection={connection} sessions={sessions} />
      </ListPlace>

      <main ref={main} className="relative flex h-dvh min-w-0 flex-col bg-bg wide:h-auto wide:flex-1">
        <OpenSession connection={connection} sessions={sessions} />
        <CardPlace connection={connection} sessions={sessions} />
      </main>

      <RightPanelPlace connection={connection} sessions={sessions} />
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
// session details.
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
        <CardLane>
          <Draft key={`${project}`} connection={connection} sessions={sessions} project={project} />
        </CardLane>
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
      <CardLane>
        <Conversation connection={connection} session={selected.session} folder={projectOf(project)?.folder} />
      </CardLane>
    </>
  )
}

// The middle column's header, as the Session view drawing has it: the
// project's tile and its name, quiet, then the session's name; after the
// button that shows and hides the session list, and before those that show
// and hide the session details and the right panel, on the right. For a
// draft, the project is a menu to pick another, or none, as T3 Code's draft
// picks where a new thread goes. A name that can be renamed is typed in
// place after a double-click, and why a name was not taken is said on a
// line under the header. At phone width, the line that says the connection
// is down is under it too.
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
        <PanelButton />
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
