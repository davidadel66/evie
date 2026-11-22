import { useSyncExternalStore } from "react"
import { connectionLine, type Connection, type ConnectionState } from "./connection.ts"
import { Conversation, Placeholder } from "./Conversation.tsx"
import { useSelected } from "./selection.ts"
import { titleOf } from "./session-list.ts"
import { SessionList } from "./SessionList.tsx"
import type { Sessions } from "./sessions.ts"
import { Settings } from "./Settings.tsx"
import { Spinner } from "./Spinner.tsx"
import { Words } from "./Words.tsx"

// The page once signed in: the session list, the conversation and the
// project card, in three columns, which stack at phone width, where the
// conversation is a screen high. The project card holds, for now, a note
// of what will go there.
export function Frame({ state, connection, sessions }: { state: ConnectionState; connection: Connection; sessions: Sessions }) {
  return (
    <div className="flex min-h-dvh flex-col wide:h-dvh wide:flex-row">
      <aside aria-label="Sessions" className="flex flex-col bg-bg-side wide:w-session-list wide:shrink-0">
        {/* The settings button's own padding brings its icon near the page's 28px edge. */}
        <header className="flex h-header-height shrink-0 items-center gap-3 pr-4 pl-7">
          <span className="text-title text-text">evie</span>
          <div className="ml-auto">
            <Settings />
          </div>
        </header>
        <ConnectionLine state={state} />
        <SessionList sessions={sessions} />
      </aside>

      <main className="flex h-dvh min-w-0 flex-1 flex-col bg-bg">
        <OpenSession connection={connection} sessions={sessions} />
      </main>

      <aside aria-label="Project" className="flex flex-col border-t border-line bg-bg wide:w-project-card wide:shrink-0 wide:border-t-0 wide:border-l">
        <header className="flex h-header-height shrink-0 items-center px-7">
          <h2 className="text-title text-text">Project</h2>
        </header>
        <p className="px-7 pb-6 text-caption text-text-muted">The open session's project will be shown here: its folder, its branch and what has changed.</p>
      </aside>
    </div>
  )
}

// The middle column: the selected session's conversation, under its name,
// or for a draft, where the new session will start.
function OpenSession({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  if (selected === undefined) return <Placeholder title="No session open" text="The conversation will be here: your messages, evie's replies and the steps it took." />
  if ("draft" in selected) {
    const project = list.projects.find((each) => each.id === selected.draft)
    const where = selected.draft === null ? "with no project" : `in ${project?.name ?? "its project"}`
    return <Placeholder title="New session" text={`A new session ${where} starts when its first message is sent.`} />
  }
  // A session is opened once the list shows it is one: until the list is
  // read, nothing is shown, and one the list does not have is not opened.
  if (!list.listed) return null
  const session = list.sessions.find((each) => each.session === selected.session)
  if (session === undefined) return <Placeholder title="This session is not in the list" text="Nothing has been said in it yet, or it is no longer there." />
  return (
    <>
      <header className="flex h-header-height shrink-0 items-center border-b border-line px-7">
        <h1 className="truncate text-title text-text">{titleOf(session)}</h1>
      </header>
      <Conversation connection={connection} session={selected.session} />
    </>
  )
}

// Nothing while connected. While the line is not up, one quiet line, as
// Working is said, with why, until it is back. The status region is always
// on the page, and only its words come and go, so that a screen reader,
// which listens to a region already there, says them.
function ConnectionLine({ state }: { state: ConnectionState }) {
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
