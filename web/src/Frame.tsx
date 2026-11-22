import { createContext, useContext, useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import { useCardShortcut } from "./card-shown.ts"
import { Button } from "./components/ui/button.tsx"
import { connectionLine, type Connection, type ConnectionState } from "./connection.ts"
import { Conversation, Draft, Placeholder } from "./Conversation.tsx"
import { startedHere } from "./conversation.ts"
import { X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { closeListSheet, toggleList, useListShown } from "./list-shown.ts"
import { Notices } from "./Notices.tsx"
import { Palette } from "./Palette.tsx"
import { useDiffShortcut } from "./diff-shown.ts"
import { DiffPlace } from "./DiffPanel.tsx"
import { CardButton, CardPlace } from "./ProjectCard.tsx"
import { useSelected } from "./selection.ts"
import { titleOf } from "./session-list.ts"
import { ListButton, SessionList } from "./SessionList.tsx"
import { HeaderName } from "./SessionRow.tsx"
import type { Sessions } from "./sessions.ts"
import { Settings } from "./Settings.tsx"
import { windowOver } from "./page-requests.ts"
import { onMac, pressed } from "./shortcuts.ts"
import { Spinner } from "./Spinner.tsx"
import { SubagentView } from "./Subagent.tsx"
import { Words } from "./Words.tsx"
import { useWorkspacePlace, WorkspaceView } from "./Workspace.tsx"

// The page once signed in: the session list, the conversation and the
// project card, in three columns, as T3 Code lays them out. At 767px or
// narrower the list is a sheet over the page from the left, and at 980px
// or narrower the card is one from the right, each opened from a button in
// the session's header. With a workspace in the address, the workspace
// view instead. The command palette and the notices float over either.
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

// The session view's three columns.
function SessionView({ state, connection, sessions }: { state: ConnectionState; connection: Connection; sessions: Sessions }) {
  const list = useListShown()
  const sheetOpen = !list.wide && list.sheet
  return (
    <div className="flex h-dvh">
      <ListPlace>
        {/* The settings button's own padding brings its icon near the page's 28px edge. */}
        <header className="flex h-header-height shrink-0 items-center gap-3 pr-4 pl-7">
          <span className="text-title text-text">evie</span>
          <div className="ml-auto flex items-center">
            <Settings connection={connection} />
            {!list.wide && (
              <Button variant="icon" aria-label="Close the session list" onClick={closeListSheet}>
                <X />
              </Button>
            )}
          </div>
        </header>
        {list.wide && <ConnectionLine state={state} />}
        <SessionList connection={connection} sessions={sessions} />
      </ListPlace>

      <main inert={sheetOpen} className="flex min-w-0 flex-1 flex-col bg-bg">
        <OpenSession connection={connection} sessions={sessions} />
      </main>

      <DiffPlace connection={connection} sessions={sessions} />
      <DiffPlace connection={connection} sessions={sessions} />
      <CardPlace connection={connection} sessions={sessions} />
    </div>
  )
}

// How the connection stands, for the line that says it is down, which at
// phone width, with the list in its sheet, is under the session's header.
const ConnectionShown = createContext<ConnectionState | undefined>(undefined)

// The session list's place: the column on the left, which the owner can
// hide, or at phone width a sheet over the page from the left, as T3
// Code's sidebar. The list is kept on the page while hidden, so that what
// it is narrowed to, and the shortcuts that move through it, hold. While
// the sheet is open the page behind it cannot be reached; a tap outside
// it, Escape, or picking a session in it closes it, and the focus goes
// back where it was.
function ListPlace({ children }: { children: ReactNode }) {
  const shown = useListShown()
  const selected = useSelected()
  const place = useRef<HTMLElement>(null)
  const sheet = !shown.wide
  const open = sheet && shown.sheet
  useEffect(() => closeListSheet(), [selected])
  useEffect(() => {
    if (!open) return
    const before = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
    place.current?.focus()
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return
      event.preventDefault()
      closeListSheet()
    }
    addEventListener("keydown", onKey)
    return () => {
      removeEventListener("keydown", onKey)
      if (before?.isConnected) before.focus()
    }
  }, [open])
  return (
    <>
      {open && <div aria-hidden="true" onClick={closeListSheet} className="fixed inset-0 z-40" />}
      <aside
        ref={place}
        aria-label="Sessions"
        tabIndex={-1}
        {...(open && { role: "dialog", "aria-modal": true })}
        inert={sheet && !open}
        hidden={!sheet && !shown.column}
        className={cn(
          "flex flex-col outline-none",
          sheet
            ? "fixed inset-y-0 left-0 z-50 w-list-sheet border-r border-line bg-surface shadow-float transition-[translate,visibility] motion-reduce:transition-none"
            : "w-session-list shrink-0 bg-bg-side",
          sheet && !open && "invisible -translate-x-full",
        )}
      >
        {children}
      </aside>
    </>
  )
}

// The shortcuts of the frame itself: mod+K opens and closes the command
// palette, and mod+B shows and hides the session list. While another
// window is open over the page, such as the settings, they wait.
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
      } else if (command === "session list" && over === undefined) {
        event.preventDefault()
        toggleList()
      }
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [togglePalette])
}

// The middle column: the selected session's conversation, under its name,
// or one of its subagents', or for a draft, where the new session will
// start. Whatever it shows, its header has the button that shows and hides
// the project card.
function OpenSession({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  if (selected === undefined) return <Empty title="No session open" text="The conversation will be here: your messages, evie's replies and the steps it took." />
  if ("draft" in selected) {
    const project = list.projects.find((each) => each.id === selected.draft)
    const where = selected.draft === null ? "with no project" : `in ${project?.name ?? "its project"}`
    return (
      <>
        <Top />
        <Draft connection={connection} sessions={sessions} project={selected.draft} where={where} />
      </>
    )
  }
  // A session is opened once the list shows it is one: until the list is
  // read, nothing is shown, and one the list does not have is not opened,
  // unless this page has just started it, and its first message is on its way.
  if (!list.listed) return <Top />
  const session = list.sessions.find((each) => each.session === selected.session)
  if (session === undefined && startedHere(selected.session) === undefined) {
    return <Empty title="This session is not in the list" text="Nothing has been said in it yet, or it is no longer there." />
  }
  const title = session === undefined ? "New session" : titleOf(session)
  if (selected.subagent !== undefined) return <SubagentView connection={connection} parent={selected.session} parentTitle={title} subagent={selected.subagent} />
  return (
    <>
      <Top key={selected.session} title={title} {...(session !== undefined && { rename: (name: string) => sessions.rename(session.session, name) })} />
      <Conversation connection={connection} session={selected.session} />
    </>
  )
}

// The middle column's header: the button that shows and hides the session
// list, the session's name, when there is one to show, and the button that
// shows and hides the project card. A name that can be renamed is typed in
// place after a double-click, and why a name was not taken is said on a
// line under the header. At phone width, the line that says the connection
// is down is under it too.
function Top({ title, rename }: { title?: string; rename?(name: string): Promise<string | undefined> }) {
  const [note, setNote] = useState<string>()
  const state = useContext(ConnectionShown)
  const list = useListShown()
  return (
    <>
      <header className="flex h-header-height shrink-0 items-center gap-3 border-b border-line pr-4 pl-4">
        <ListButton />
        {title === undefined ? (
          <span className="flex-1" />
        ) : rename === undefined ? (
          <h1 className="min-w-0 flex-1 truncate text-title text-text">{title}</h1>
        ) : (
          <HeaderName title={title} rename={rename} said={setNote} />
        )}
        <CardButton />
      </header>
      {!list.wide && state !== undefined && (
        <div className="pt-3">
          <ConnectionLine state={state} />
        </div>
      )}
      <div role="status">{note !== undefined && <p className="px-7 pt-3 text-caption text-text-muted">{note}</p>}</div>
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

// Nothing while connected. While the line is not up, one quiet line, as
// Working is said, with why, until it is back. The status region is always
// on the page, and only its words come and go, so that a screen reader,
// which listens to a region already there, says them.
export function ConnectionLine({ state }: { state: ConnectionState }) {
  const line = connectionLine(state)
  return (
    <div role="status">
      {line !== undefined && (
        <p className="flex items-start gap-2 px-7 pb-3 text-caption text-text-muted">
          <span className="flex h-lh items-center">
            <Spinner />
          </span>
          <span>
            {line.word}
            {line.reason !== undefined && (
              <>
                {". "}
                <Words text={line.reason} />
              </>
            )}
          </span>
        </p>
      )}
    </div>
  )
}
