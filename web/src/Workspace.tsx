import { useEffect, useState, useSyncExternalStore } from "react"
import type { ListedProject } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { Button, buttonVariants } from "./components/ui/button.tsx"
import type { Connection, ConnectionState } from "./connection.ts"
import { Placeholder } from "./Conversation.tsx"
import { ConnectionLine } from "./ConnectionLine.tsx"
import { IconPicker } from "./IconPicker.tsx"
import { GearSix, List, Plus, SidebarRight } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { NewWorkspace } from "./NewWorkspace.tsx"
import { ProjectTile } from "./ProjectIcon.tsx"
import { openSettings, sessionViewLeft } from "./selection.ts"
import { titleOf } from "./session-list.ts"
import type { SessionList, Sessions } from "./sessions.ts"
import { Later, StorageView } from "./StorageView.tsx"
import { addressOfWorkspace, NEW_CHAT, needsYou, sessionStatus, sessionsOf, setChatShown, type SessionStatus, type WorkspacePlace, type WorkspaceTab } from "./workspace.ts"
import { SideChat, useChatShown } from "./WorkspaceChat.tsx"

// The workspace view: a project's home. The strip of every workspace on
// the left, the workspace itself, with its header and the tab open, and
// the side chat beside it, which the owner can hide. Where the two no
// longer fit side by side, as on a phone or an upright tablet, the strip
// stays a column on the left, and the side chat stacks below the
// workspace, a screen high, the two scrolling together.

// The page loads this view later, once it is idle or the owner opens a
// workspace (see screens.ts); where the owner is, which the page needs from
// the start, is kept in selection.ts.
export function WorkspaceView({ place, state, connection, sessions }: { place: WorkspacePlace; state: ConnectionState; connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const project = list.projects.find((each) => each.id === place.workspace)
  const now = useMinute()
  const chatShown = useChatShown()
  // Back to the session view as it was left, or to the side chat's session there.
  const chat = place.chat !== undefined && place.chat !== NEW_CHAT ? place.chat : undefined
  const back = sessionViewLeft() ?? (chat !== undefined ? addressOf({ session: chat }) : "#")
  return (
    <div className="flex h-dvh overflow-hidden">
      <Strip list={list} place={place} back={back} connection={connection} sessions={sessions} />
      <div className="@container min-w-0 flex-1 overflow-y-auto">
        <div className="flex min-h-full flex-col @workspace-columns:h-full @workspace-columns:flex-row">
          <main className="flex min-w-0 flex-1 flex-col bg-bg @workspace-columns:min-h-0">
            {project === undefined ? (
              <>
                <header className="h-header-height shrink-0 border-b border-line" />
                <ConnectionLine state={state} />
                {/* Until the projects are listed, nothing is said. */}
                {list.projects.length > 0 && <Placeholder title="This workspace is not in the list" text="Pick another in the strip, or make a new one there." />}
              </>
            ) : (
              <>
                <Header project={project} place={place} chatShown={chatShown} connection={connection} sessions={sessions} />
                <ConnectionLine state={state} />
                <div className="flex min-h-0 flex-1 flex-col @workspace-columns:overflow-y-auto">
                  {place.tab === "storage" ? <StorageView connection={connection} project={project} place={place} now={now} /> : <SessionsTab list={list} place={place} now={now} />}
                </div>
              </>
            )}
          </main>
          {/* Hidden, it stays on the page, so that a chat starting goes on starting. */}
          <aside
            aria-label="Chat with evie"
            hidden={!chatShown}
            className="flex h-dvh shrink-0 flex-col border-t border-line bg-bg-side @workspace-columns:h-auto @workspace-columns:w-side-chat-width @workspace-columns:border-t-0 @workspace-columns:border-l"
          >
            {project !== undefined && <SideChat project={project} place={place} list={list} connection={connection} sessions={sessions} />}
          </aside>
        </div>
      </div>
    </div>
  )
}

// The strip: the way back to the session list, under it a line, then a
// tile for every workspace, the one open with a line in its colour on its
// edge, each with a dot while a session in it needs the owner, then Add a
// workspace as a dashed tile, and the settings at the bottom. Moving to
// another workspace keeps the tab.
function Strip({ list, place, back, connection, sessions }: { list: SessionList; place: WorkspacePlace; back: string; connection: Connection; sessions: Sessions }) {
  return (
    <nav aria-label="Workspaces" className="flex w-rail-width shrink-0 flex-col items-center gap-2 overflow-y-auto border-r border-line bg-bg-side py-2">
      <a href={back} aria-label="Back to the session list" title="Back to the session list" className={cn(buttonVariants({ variant: "icon" }), "size-tile-strip [&>svg]:size-icon-md")}>
        <List />
      </a>
      <div aria-hidden="true" className="h-(--line-width) w-7 shrink-0 bg-line" />
      <ul className="flex shrink-0 flex-col items-center gap-2">
        {list.projects.map((project) => {
          const waiting = needsYou(list.sessions, project.id)
          const current = project.id === place.workspace
          return (
            <li key={project.id} className="flex shrink-0">
              <a
                href={addressOfWorkspace({ workspace: project.id, tab: place.tab })}
                aria-current={current ? "page" : undefined}
                aria-label={waiting ? `${project.name}, Needs you` : project.name}
                title={project.name}
                className="relative flex rounded-tile"
              >
                <ProjectTile project={project.id} size="strip" selected={current} />
                {waiting && <span aria-hidden="true" className="absolute -top-1 -right-1 size-3 rounded-full border-(length:--indicator-width) border-bg-side bg-status-needs" />}
              </a>
            </li>
          )
        })}
      </ul>
      <NewWorkspace connection={connection} sessions={sessions}>
        <button
          type="button"
          aria-label="Add a workspace"
          title="Add a workspace"
          className="flex size-tile-strip shrink-0 items-center justify-center rounded-tile border border-dashed border-line text-text-muted hover:text-text [&>svg]:size-icon-control"
        >
          <Plus />
        </button>
      </NewWorkspace>
      {/* The settings page, which the frame keeps on the page. */}
      <Button variant="icon" aria-label="Settings" title="Settings" onClick={() => openSettings()} className="mt-auto size-tile-strip">
        <GearSix />
      </Button>
    </nav>
  )
}

// The workspace's header, in one row: its tile and name, which open the
// picker of its icon and colour; its dashboards as tabs, none yet; Storage,
// Sessions and Memory pinned to the right, Memory not yet there; and the
// toggle that hides and shows the side chat. Where the row is short, the
// name gives way, cut with an ellipsis, down to a width it always keeps,
// and then the tabs scroll sideways. The open tab's line lies on the
// header's own line.
function Header({ project, place, chatShown, connection, sessions }: { project: ListedProject; place: WorkspacePlace; chatShown: boolean; connection: Connection; sessions: Sessions }) {
  const toggle = chatShown ? "Hide the chat" : "Show the chat"
  return (
    <header className="flex h-header-height shrink-0 items-stretch gap-4 border-b border-line pr-3 pl-gutter @workspace-columns:gap-6">
      <h1 className="flex flex-1 items-center">
        <IconPicker connection={connection} sessions={sessions} project={project}>
          <button type="button" title="Pick its icon and colour" className="-ml-2 flex h-control items-center gap-2 rounded-md px-2">
            <ProjectTile project={project.id} size="header" />
            <span className="min-w-workspace-name truncate text-title text-text">{project.name}</span>
          </button>
        </IconPicker>
      </h1>
      {/* A workspace's dashboards will be tabs here, after its name. */}
      <nav aria-label="Workspace" className="-mb-(--line-width) flex min-w-0 items-stretch gap-6 overflow-x-auto">
        <Tab name="Storage" tab="storage" place={place} />
        <Tab name="Sessions" tab="sessions" place={place} />
        <span aria-disabled="true" className="flex shrink-0 items-center gap-2 text-label text-text-muted">
          Memory
          <Later />
        </span>
      </nav>
      <Button variant="icon" aria-label={toggle} title={toggle} onClick={() => setChatShown(!chatShown)} className="shrink-0 self-center">
        <SidebarRight />
      </Button>
    </header>
  )
}

// A tab, the one open in `text` over a 2px line, the others muted.
function Tab({ name, tab, place }: { name: string; tab: WorkspaceTab; place: WorkspacePlace }) {
  const current = place.tab === tab
  return (
    <a
      href={addressOfWorkspace({ workspace: place.workspace, tab, ...(place.chat !== undefined && { chat: place.chat }) })}
      aria-current={current ? "page" : undefined}
      className="relative flex shrink-0 items-center text-label text-text-muted hover:text-text aria-[current=page]:text-text"
    >
      {name}
      {current && <span aria-hidden="true" className="absolute inset-x-0 bottom-0 h-(--indicator-width) bg-text" />}
    </a>
  )
}

const TONE: Record<SessionStatus["tone"], string> = { needs: "text-status-needs", done: "text-status-done", muted: "text-text-muted" }

// The Sessions tab: the project's sessions, as rows, each with its title
// and, on the right, its state or how long ago it was active. Picking one
// opens it in the session view.
function SessionsTab({ list, place, now }: { list: SessionList; place: WorkspacePlace; now: number }) {
  const shown = sessionsOf(list.sessions, place.workspace)
  return (
    <section aria-label="Sessions" className="flex flex-col px-gutter py-5">
      {list.listed && shown.length === 0 ? (
        <p className="text-caption text-text-muted">No sessions here yet. Write to evie in the chat to start one.</p>
      ) : (
        <ul className="flex flex-col">
          {shown.map((session) => {
            const status = sessionStatus(session, now)
            return (
              <li key={session.session}>
                <a href={addressOf({ session: session.session })} className="flex min-h-row-height items-center gap-3 border-t border-line py-2">
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-label text-text">{titleOf(session)}</span>
                  </span>
                  <span className={cn("shrink-0 text-caption", TONE[status.tone])}>{status.words}</span>
                </a>
              </li>
            )
          })}
        </ul>
      )}
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
