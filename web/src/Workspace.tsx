import { useEffect, useMemo, useState, useSyncExternalStore } from "react"
import type { ListedProject, SessionSummary } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { buttonVariants } from "./components/ui/button.tsx"
import type { Connection, ConnectionState } from "./connection.ts"
import { Conversation, Draft, Placeholder } from "./Conversation.tsx"
import { startedHere } from "./conversation.ts"
import { ConnectionLine } from "./ConnectionLine.tsx"
import { IconPicker } from "./IconPicker.tsx"
import { ArrowSquareOut, ChatsCircle, Plus } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { NewWorkspace } from "./NewWorkspace.tsx"
import { ask, windowOver } from "./page-requests.ts"
import { ProjectTile } from "./ProjectIcon.tsx"
import { HeaderName } from "./SessionRow.tsx"
import { ageWords, titleOf } from "./session-list.ts"
import type { SessionList, Sessions } from "./sessions.ts"
import { onMac, pressed } from "./shortcuts.ts"
import { Spinner } from "./Spinner.tsx"
import { Later, StorageView } from "./StorageView.tsx"
import { addressOfWorkspace, chatAddress, latestSession, needsYou, NEW_CHAT, sessionsOf, workspaceIn, type WorkspacePlace, type WorkspaceTab } from "./workspace.ts"

// The workspace view: a project's home. The strip of every workspace on
// the left, the workspace itself in the middle, with its header and the
// tab open, and the side chat on the right, with one of its sessions. At
// phone width the strip is a row of tiles at the top, and the side chat
// stacks below the workspace, a screen high.

// Where the owner is, when it is in a workspace, redrawn as the address
// changes. Elsewhere, the address is remembered, for the strip's way back.
export function useWorkspacePlace(): WorkspacePlace | undefined {
  const hash = useSyncExternalStore(onAddressChange, () => location.hash)
  const place = useMemo(() => workspaceIn(hash), [hash])
  if (place === undefined) sessionView = hash === "" ? "#" : hash
  return place
}
function onAddressChange(listener: () => void) {
  addEventListener("hashchange", listener)
  return () => removeEventListener("hashchange", listener)
}
// The session view's address as the owner last left it for a workspace,
// or undefined when the page opened in one.
let sessionView: string | undefined

export function WorkspaceView({ place, state, connection, sessions }: { place: WorkspacePlace; state: ConnectionState; connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const project = list.projects.find((each) => each.id === place.workspace)
  const now = useMinute()
  // Back to the session view as it was left, or to the side chat's session there.
  const chat = place.chat !== undefined && place.chat !== NEW_CHAT ? place.chat : undefined
  const back = sessionView ?? (chat !== undefined ? addressOf({ session: chat }) : "#")
  return (
    <div className="flex min-h-dvh flex-col wide:h-dvh wide:flex-row">
      <Strip list={list} place={place} back={back} connection={connection} sessions={sessions} />
      <main className="flex min-w-0 flex-col bg-bg wide:min-h-0 wide:flex-1">
        {project === undefined ? (
          <>
            <header className="h-header-height shrink-0 border-b border-line" />
            <ConnectionLine state={state} />
            {/* Until the projects are listed, nothing is said. */}
            {list.projects.length > 0 && <Placeholder title="This workspace is not in the list" text="Pick another in the strip, or make a new one there." />}
          </>
        ) : (
          <>
            <Header project={project} place={place} connection={connection} sessions={sessions} />
            <ConnectionLine state={state} />
            <div className="flex min-h-0 flex-1 flex-col wide:overflow-y-auto">
              {place.tab === "storage" ? <StorageView connection={connection} project={project} place={place} now={now} /> : <SessionsTab list={list} place={place} now={now} />}
            </div>
          </>
        )}
      </main>
      <aside aria-label="Side chat" className="flex h-dvh shrink-0 flex-col bg-bg-side wide:h-auto wide:w-side-chat-width">
        {project !== undefined && <SideChat project={project} place={place} list={list} connection={connection} sessions={sessions} />}
      </aside>
    </div>
  )
}

// The strip: the way back to the session view, a tile for every workspace,
// the one open ringed in its colour, each with a dot while a session in it
// needs the owner, and New workspace at the end, each button as big as a
// tile. Moving to another workspace keeps the tab.
function Strip({ list, place, back, connection, sessions }: { list: SessionList; place: WorkspacePlace; back: string; connection: Connection; sessions: Sessions }) {
  return (
    <nav aria-label="Workspaces" className="flex shrink-0 items-center gap-3 overflow-x-auto bg-bg-side px-gutter py-2 wide:w-rail-width wide:flex-col wide:overflow-x-hidden wide:overflow-y-auto wide:px-0 wide:py-3">
      <a href={back} aria-label="Back to the sessions" title="Back to the sessions" className={cn(buttonVariants({ variant: "icon" }), "size-tile-strip [&>svg]:size-icon-md")}>
        <ChatsCircle />
      </a>
      <ul className="flex shrink-0 items-center gap-3 wide:flex-col">
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
        <button type="button" aria-label="New workspace" title="New workspace" className={cn(buttonVariants({ variant: "icon" }), "size-tile-strip [&>svg]:size-icon-md")}>
          <Plus />
        </button>
      </NewWorkspace>
    </nav>
  )
}

// The workspace's header, in one row: its tile and name, which open the
// picker of its icon and colour; its dashboards as tabs, none yet; and
// Storage, Sessions and Memory pinned to the right, Memory not yet there.
// Where the row is short, the name gives way, cut with an ellipsis, down
// to a width it always keeps, and then the tabs scroll sideways, as the
// board's tab rows do at phone width.
function Header({ project, place, connection, sessions }: { project: ListedProject; place: WorkspacePlace; connection: Connection; sessions: Sessions }) {
  return (
    <header className="flex h-header-height shrink-0 items-center gap-4 border-b border-line px-gutter wide:gap-6">
      <h1 className="flex flex-1 text-label text-text">
        <IconPicker connection={connection} sessions={sessions} project={project}>
          <button type="button" title="Pick its icon and colour" className="-ml-2 flex h-control items-center gap-2 rounded-md px-2">
            <ProjectTile project={project.id} size="header" />
            <span className="min-w-workspace-name truncate">{project.name}</span>
          </button>
        </IconPicker>
      </h1>
      {/* A workspace's dashboards will be tabs here, before the pinned ones. */}
      <nav aria-label="Workspace tabs" className="flex h-full min-w-0 items-stretch gap-6 overflow-x-auto">
        <Tab name="Storage" tab="storage" place={place} />
        <Tab name="Sessions" tab="sessions" place={place} />
        <span aria-disabled="true" className="flex shrink-0 items-center gap-2 text-label text-text-muted">
          Memory
          <Later />
        </span>
      </nav>
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

// The Sessions tab: the project's sessions, as rows, each with its state
// or how long ago it was active. Picking one shows it in the side chat,
// whose session is raised.
function SessionsTab({ list, place, now }: { list: SessionList; place: WorkspacePlace; now: number }) {
  const shown = sessionsOf(list.sessions, place.workspace)
  return (
    <section aria-label="Sessions" className="flex flex-col px-gutter py-4">
      <h2 className="px-3 pb-2 text-caption text-text-muted">Sessions</h2>
      {list.listed && shown.length === 0 ? (
        <p className="border-t border-line px-3 pt-3 text-caption text-text-muted">No sessions here yet. Write to evie in the side chat to start one.</p>
      ) : (
        <ul className="flex flex-col">
          {shown.map((session) => (
            <li key={session.session}>
              <a
                href={addressOfWorkspace({ workspace: place.workspace, tab: "sessions", chat: session.session })}
                aria-current={session.session === place.chat ? "true" : undefined}
                className="flex h-row-height items-center gap-4 border-t border-line px-3 aria-[current]:bg-surface-raised"
              >
                <span className="min-w-0 flex-1 truncate text-body font-medium text-text">{titleOf(session)}</span>
                <State session={session} now={now} />
              </a>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}

// A session's state in words where the owner is needed or evie is at work,
// and otherwise how long ago it was active.
function State({ session, now }: { session: SessionSummary; now: number }) {
  if (session.state === "needs_you") return <span className="shrink-0 text-caption text-status-needs">Needs you</span>
  if (session.state === "working") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-caption text-status-working">
        <Spinner />
        Working
      </span>
    )
  }
  return <span className="shrink-0 text-caption text-text-muted">{session.state === "settled" ? "Settled" : ageWords(session.lastActive, now)}</span>
}

// The side chat: the session picked, or until one is, the workspace's
// latest, or a new session in its project when it has none, with the
// message box, under a header with the way to a new session here and to
// the session in the session view. Which it shows is kept in the address,
// so that another session becoming the latest does not change it.
function SideChat({ project, place, list, connection, sessions }: { project: ListedProject; place: WorkspacePlace; list: SessionList; connection: Connection; sessions: Sessions }) {
  const latest = latestSession(list.sessions, project.id)?.session
  useEffect(() => {
    if (place.chat !== undefined || !list.listed) return
    location.replace(addressOfWorkspace({ ...place, chat: latest ?? NEW_CHAT }))
  }, [place, list.listed, latest])
  const chat = place.chat
  const at = (next: string) => addressOfWorkspace({ ...place, chat: next })
  if (chat === undefined) return <ChatHeader title="" />
  if (chat === NEW_CHAT) {
    return (
      <>
        <ChatHeader title="New session" full={addressOf({ draft: project.id })} />
        <Draft
          connection={connection}
          sessions={sessions}
          project={project.id}
          where={`in ${project.name}`}
          // As the address is when the session has started, which the owner may have changed meanwhile.
          moved={(session) => {
            const there = chatAddress(location.hash, project.id, session)
            if (there !== undefined) location.replace(there)
          }}
        />
      </>
    )
  }
  const session = list.sessions.find((each) => each.session === chat)
  if (session === undefined && startedHere(chat) === undefined) {
    return (
      <>
        <ChatHeader title="" add={at(NEW_CHAT)} />
        {list.listed && <Placeholder title="This session is not in the list" text="Pick another in Sessions, or start a new one here." />}
      </>
    )
  }
  return (
    <>
      <ChatHeader
        key={chat}
        title={session === undefined ? "New session" : titleOf(session)}
        add={at(NEW_CHAT)}
        full={addressOf({ session: chat })}
        {...(session !== undefined && { rename: (name: string) => sessions.rename(session.session, name) })}
      />
      <Conversation connection={connection} session={chat} />
    </>
  )
}

// The side chat's header: its session's name, which is renamed in place
// after a double-click, F2 or the command palette's Rename, as the session
// view's header's is, with why a name was not taken on a line under it;
// and the ways to a new session there, and to the session view.
function ChatHeader({ title, add, full, rename }: { title: string; add?: string; full?: string; rename?(name: string): Promise<string | undefined> }) {
  const [note, setNote] = useState<string>()
  // F2 renames the side chat's session, as it renames the session view's.
  useEffect(() => {
    if (rename === undefined) return
    const mac = onMac()
    const onKey = (event: KeyboardEvent) => {
      const focused = document.activeElement
      const typing = focused instanceof HTMLInputElement || focused instanceof HTMLTextAreaElement || (focused instanceof HTMLElement && focused.isContentEditable)
      if (pressed(event, mac) !== "rename" || event.defaultPrevented || typing || windowOver() !== undefined) return
      event.preventDefault()
      ask("rename")
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [rename !== undefined])
  return (
    <>
      <header className="flex h-header-height shrink-0 items-center gap-1 border-b border-line pr-2 pl-gutter">
        {rename === undefined ? <h2 className="min-w-0 flex-1 truncate text-label text-text">{title}</h2> : <HeaderName title={title} rename={rename} said={setNote} small />}
        {add !== undefined && (
          <a href={add} aria-label="New session here" title="New session here" className={buttonVariants({ variant: "icon" })}>
            <Plus />
          </a>
        )}
        {full !== undefined && (
          <a href={full} aria-label="Open in the session view" title="Open in the session view" className={buttonVariants({ variant: "icon" })}>
            <ArrowSquareOut />
          </a>
        )}
      </header>
      <div role="status">{note !== undefined && <p className="px-gutter pt-3 text-caption text-text-muted">{note}</p>}</div>
    </>
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
