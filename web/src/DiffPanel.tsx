import { lazy, Suspense, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ComponentProps, type ReactNode } from "react"
import type { ChangedFile, Commands, FileDiff, ListedProject, PullRequest } from "../../src/core/types.ts"
import { followPullRequest, pullRequestOf, type PullRequestState } from "./card.ts"
import { holdChanges, type Changes, type ChangesState } from "./changes.ts"
import { byPath, FilesTreeRows, Stat } from "./ChangedFiles.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { Sheet, SheetContent, SheetTitle } from "./components/ui/sheet.tsx"
import type { Connection } from "./connection.ts"
import { startedHere } from "./conversation.ts"
import { changedFilesLine, filesTree, hasCounts, scopeChoices, type Scope } from "./diff.ts"
import { closeDiff, compareLatest, compareOther, drawAs, showTab, showTree, toggleDiff, useDiffShown, wentToFile, widenTo, wrapLines } from "./diff-shown.ts"
import {
  ArrowClockwise,
  ArrowElbowDownLeft,
  ArrowSquareOut,
  CaretDown,
  CaretUpDown,
  CheckCircle,
  CircleDashed,
  Columns,
  GitDiff,
  GitPullRequest,
  Rows,
  SidebarRight,
  TreeView,
  X,
  XCircle,
} from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { BESIDE, panelPlace, panelWidth, PHONE, type PanelPlace } from "./right-panel.ts"
import { checksSummary, checkWord, pullRequestWord } from "./pull-request.ts"
import { useSelected } from "./selection.ts"
import type { Sessions } from "./sessions.ts"
import { label as keyLabel, onMac } from "./shortcuts.ts"
import { Spinner } from "./Spinner.tsx"

// The right panel, as T3 Code's, drawn as the owner's drawing of it: a
// column beside the conversation, 540px at first and as wide as the owner
// drags its left edge, which this browser remembers; at 980px or narrower a
// sheet over the page from the right, and at phone width the whole screen.
// A strip of tabs heads it: the diff, and the branch's pull request when it
// has one. The diff shows what the selected session changed, by turn or
// for the whole session, and in a git project the uncommitted work and the
// branch, under T3 Code's compact toolbar. The diff itself is drawn by the
// diff view, loaded the first time the panel shows one.

const DiffView = lazy(() => import("./DiffView.tsx"))

// The button in the conversation's header that shows and hides the panel,
// raised while it is shown.
export function PanelButton() {
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

// Where the panel is, as the window's width puts it, redrawn as it changes.
function usePlace(): PanelPlace {
  const [place, setPlace] = useState(() => panelPlace(innerWidth))
  useEffect(() => {
    const queries = [matchMedia(BESIDE), matchMedia(PHONE)]
    const change = () => setPlace(panelPlace(innerWidth))
    for (const query of queries) query.addEventListener("change", change)
    return () => {
      for (const query of queries) query.removeEventListener("change", change)
    }
  }, [])
  return place
}

export function RightPanel({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useDiffShown()
  const place = usePlace()
  const selected = useSelected()
  // Another session's turns are not this one's: it opens on its latest.
  const session = selected !== undefined && "session" in selected ? selected.session : undefined
  const was = useRef(session)
  useEffect(() => {
    if (was.current === session) return
    was.current = session
    compareLatest()
  }, [session])
  const panel = <Panel connection={connection} sessions={sessions} session={session} />
  if (place === "beside") return shown.open ? <Beside>{panel}</Beside> : null
  return (
    <Sheet open={shown.open} onOpenChange={(open) => !open && closeDiff()}>
      <SheetContent aria-label="Right panel" className={cn("bg-bg text-caption", place === "screen" ? "w-full border-l-0" : "w-panel-sheet")}>
        <SheetTitle className="sr-only">Right panel</SheetTitle>
        {panel}
      </SheetContent>
    </Sheet>
  )
}

// How far one press of an arrow key moves the panel's edge.
const KEY_STEP = 16

// The panel beside the conversation, as wide as it was made, within its
// room (panelWidth): the row it shares with the conversation is measured
// as it changes. Its left edge is dragged to resize it, as T3 Code's, or
// moved with the arrow keys once focused.
function Beside({ children }: { children: ReactNode }) {
  const shown = useDiffShown()
  const place = useRef<HTMLElement>(null)
  const [room, setRoom] = useState({ window: innerWidth, row: innerWidth })
  const [dragging, setDragging] = useState<number>()
  useLayoutEffect(() => {
    const aside = place.current
    const row = aside?.parentElement
    if (!aside || !row) return
    const measure = () => {
      const beside = aside.previousElementSibling instanceof HTMLElement ? aside.previousElementSibling.offsetWidth : 0
      setRoom((now) => {
        const next = { window: innerWidth, row: beside + aside.offsetWidth }
        return now.window === next.window && now.row === next.row ? now : next
      })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(row)
    addEventListener("resize", measure)
    return () => {
      observer.disconnect()
      removeEventListener("resize", measure)
    }
  }, [])
  const width = panelWidth(dragging ?? shown.width, room)
  function drag(event: React.PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.preventDefault()
    const handle = event.currentTarget
    handle.setPointerCapture(event.pointerId)
    const from = { x: event.clientX, width }
    let now = width
    const move = (moved: PointerEvent) => {
      now = panelWidth(from.width + from.x - moved.clientX, room)
      setDragging(now)
    }
    const end = () => {
      handle.removeEventListener("pointermove", move)
      handle.removeEventListener("pointerup", end)
      handle.removeEventListener("pointercancel", end)
      setDragging(undefined)
      widenTo(now)
    }
    handle.addEventListener("pointermove", move)
    handle.addEventListener("pointerup", end)
    handle.addEventListener("pointercancel", end)
  }
  return (
    <aside ref={place} aria-label="Right panel" style={{ width }} className="relative flex min-h-0 shrink-0 flex-col border-l border-line bg-bg text-caption text-text">
      <div
        role="separator"
        aria-label="Resize the right panel"
        aria-orientation="vertical"
        aria-valuenow={width}
        tabIndex={0}
        onPointerDown={drag}
        onKeyDown={(event) => {
          const by = event.key === "ArrowLeft" ? KEY_STEP : event.key === "ArrowRight" ? -KEY_STEP : 0
          if (by === 0) return
          event.preventDefault()
          widenTo(panelWidth(width + by, room))
        }}
        className="group absolute inset-y-0 left-0 z-40 w-1 -translate-x-1/2 cursor-col-resize touch-none outline-none select-none pointer-coarse:w-4"
      >
        <span aria-hidden="true" className={cn("pointer-events-none absolute inset-y-0 left-1/2 w-px bg-transparent transition-colors group-hover:bg-line group-focus-visible:bg-accent", dragging !== undefined && "bg-accent")} />
      </div>
      {children}
    </aside>
  )
}

// The panel for what is selected: its tabs, and the tab shown. The pull
// request's tab is there while the session's project's branch has one.
function Panel({ connection, sessions, session }: { connection: Connection; sessions: Sessions; session: string | undefined }) {
  const shown = useDiffShown()
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const summary = session === undefined ? undefined : list.sessions.find((each) => each.session === session)
  const projectId = session === undefined ? null : summary !== undefined ? summary.project : (startedHere(session) ?? null)
  const project = projectId === null ? null : list.projects.find((each) => each.id === projectId)
  const pull = usePullRequest(connection, project?.git === false ? undefined : (project?.id ?? undefined))
  const pullShown = pull !== null && pull !== undefined && !("unavailable" in pull) ? pull : undefined
  const tab = shown.tab === "pull request" && pullShown !== undefined ? "pull request" : "diff"
  return (
    <>
      <div role="tablist" aria-label="Right panel" className="flex h-header-height shrink-0 items-center gap-1 border-b border-line pr-3 pl-2">
        <Tab selected={tab === "diff"} onClick={() => showTab("diff")}>
          <GitDiff className="size-icon-caret shrink-0" />
          Diff
        </Tab>
        {pullShown !== undefined && (
          <Tab selected={tab === "pull request"} onClick={() => showTab("pull request")} label={`Pull request #${pullShown.number}`}>
            <GitPullRequest className="size-icon-caret shrink-0" />#{pullShown.number}
          </Tab>
        )}
      </div>
      {tab === "pull request" ? (
        <PullRequestTab pull={pullShown!} />
      ) : session === undefined ? (
        <>
          <Toolbar />
          <Quiet>No session is open. The changes a session makes are shown here.</Quiet>
        </>
      ) : (
        <SessionDiff key={session} connection={connection} session={session} project={project} />
      )}
    </>
  )
}

function Tab({ selected, onClick, label, children }: { selected: boolean; onClick(): void; label?: string; children: ReactNode }) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={selected}
      {...(label !== undefined && { "aria-label": label })}
      onClick={onClick}
      className={cn(
        "flex h-control-xs max-w-panel-tab min-w-0 items-center gap-1 truncate rounded-chip pr-2 pl-1.5 text-caption",
        selected ? "bg-surface-raised text-text" : "text-text-muted hover:bg-text/8 hover:text-text",
      )}
    >
      {children}
    </button>
  )
}

const nothing = { subscribe: () => () => {}, state: (): PullRequestState => undefined }

// The branch's pull request, read when the panel opens and each minute
// while it is shown, as the session details read it.
function usePullRequest(connection: Connection, project: string | undefined): PullRequestState {
  const [followed, setFollowed] = useState<ReturnType<typeof followPullRequest>>()
  useEffect(() => {
    if (project === undefined) return setFollowed(undefined)
    const following = followPullRequest(connection, project, { window, shown: () => document.visibilityState === "visible" })
    setFollowed(following)
    return following.close
  }, [connection, project])
  return useSyncExternalStore(followed?.subscribe ?? nothing.subscribe, followed?.state ?? nothing.state)
}

// A small square button in the toolbar, as T3 Code's: 28px, its 14px icon
// muted, and while `pressed`, raised.
function ToolButton({ label, pressed, className, children, ...props }: { label: string; pressed?: boolean } & ComponentProps<"button">) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      {...(pressed !== undefined && { "aria-pressed": pressed })}
      className={cn(
        "flex size-control-sm shrink-0 items-center justify-center rounded-md [&>svg]:size-icon-caret",
        pressed ? "bg-surface-raised text-text" : "text-text-muted hover:bg-text/8 hover:text-text",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  )
}

const NONE_FOLDED: ReadonlySet<string> = new Set()
const NOT_LISTED: ChangesState = { listed: undefined, problem: undefined }
const noChanges = { subscribe: () => () => {}, state: () => NOT_LISTED }
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// What has been read for what is compared now, by its key, so that what
// was read for something else is not shown under its name.
type Read = ({ scope: Scope } & Commands["read_changes"]["output"]) | { scope: Scope; problem: string }

// The diff's toolbar, as T3 Code's: what is compared, on the left; then the
// lines added and removed, reading again, stacked or side by side, wrapping
// long lines, the file tree, and closing, on the right. Without a session,
// only closing.
function Toolbar({ children }: { children?: ReactNode }) {
  return (
    <div className="flex h-diff-toolbar shrink-0 items-center gap-3 border-b border-line/60 px-2">
      {children ?? (
        <span className="flex flex-1 justify-end">
          <CloseButton />
        </span>
      )}
    </div>
  )
}

function CloseButton() {
  return (
    <ToolButton label="Close the diff" title={`Close the diff (${keyLabel("diff", onMac())})`} onClick={closeDiff}>
      <X />
    </ToolButton>
  )
}

function SessionDiff({ connection, session, project }: { connection: Connection; session: string; project: ListedProject | null | undefined }) {
  const shown = useDiffShown()
  const [changes, setChanges] = useState<Changes>()
  useEffect(() => {
    const held = holdChanges(connection, session)
    setChanges(held.changes)
    return held.release
  }, [connection, session])
  const { listed, problem: listProblem } = useSyncExternalStore(changes?.subscribe ?? noChanges.subscribe, changes?.state ?? noChanges.state)
  // Whether the project's folder is in git: null with no project, and
  // undefined when it is not known, when the server is asked.
  const git = project === null ? null : project?.git
  const choices = useMemo(() => scopeChoices({ turns: listed?.turns ?? [], git, now: Date.now() }), [listed, git])

  // Read again as it is picked, as each turn ends, and when asked.
  const [read, setRead] = useState<Read>()
  const [again, setAgain] = useState(0)
  const [reading, setReading] = useState(false)
  const base = project ? pullRequestOf(project.id)?.base : undefined
  const scope = shown.scope
  useEffect(() => {
    if (changes === undefined) return
    if ((scope === "uncommitted" || scope === "branch") && (git === false || git === null)) {
      setRead({ scope, files: [], unavailable: git === null ? "This session has no project, so there is no git repository to compare." : "Not a git repository" })
      return
    }
    // The latest turn is known once the list has been read.
    if (scope === "latest" && listed === undefined) return
    let current = true
    setReading(true)
    changes.read(scope, scope === "branch" ? base : undefined).then(
      (answer) => current && setRead({ scope, ...answer }),
      (cause: unknown) => current && setRead({ scope, problem: wordsOf(cause) }),
    )
    return () => {
      current = false
    }
  }, [changes, scope, listed, again, base, git])
  useEffect(() => setReading(false), [read])

  const shownRead = read?.scope === scope ? read : undefined
  const files = useMemo(() => (shownRead !== undefined && "files" in shownRead ? [...shownRead.files].sort((a, b) => byPath(a.path, b.path)) : undefined), [shownRead])
  // Each file is folded at first, but the one the panel was opened at, or
  // else the first, as the owner's drawing has it; one that comes later,
  // as when a turn ends, is folded too. What the owner folds or opens stays.
  const [folding, setFolding] = useState<{ scope: Scope; seen: ReadonlySet<string>; folded: ReadonlySet<string> }>({ scope, seen: new Set(), folded: new Set() })
  const target = shown.file
  useEffect(() => {
    if (files === undefined) return
    setFolding((was) => {
      const fresh = was.scope !== scope
      const seen = fresh ? new Set<string>() : new Set(was.seen)
      const folded = fresh ? new Set<string>() : new Set(was.folded)
      const open = target ?? (fresh || was.seen.size === 0 ? files[0]?.path : undefined)
      for (const file of files) {
        if (seen.has(file.path)) continue
        seen.add(file.path)
        if (file.path !== open) folded.add(file.path)
      }
      if (open !== undefined) folded.delete(open)
      return { scope, seen, folded }
    })
  }, [files, scope, target])
  const folded = folding.scope === scope ? folding.folded : NONE_FOLDED
  const fold = (path: string) =>
    setFolding((was) => {
      const now = new Set(was.folded)
      if (!now.delete(path)) now.add(path)
      return { ...was, folded: now }
    })
  // Opened at a file, from a turn's card: gone to once, when it is drawn.
  const [goTo, setGoTo] = useState<string>()
  const [current, setCurrent] = useState<string>()
  useEffect(() => {
    if (target === undefined || !files?.some((file) => file.path === target)) return
    setGoTo(target)
    setCurrent(target)
    wentToFile()
  }, [files, target])
  function toFile(path: string) {
    setFolding((was) => {
      const now = new Set(was.folded)
      now.delete(path)
      return { ...was, folded: now }
    })
    setGoTo(path)
    setCurrent(path)
  }

  const [treeFolded, setTreeFolded] = useState<ReadonlySet<string>>(new Set())
  // The session's latest turn, opened from its card, is the latest turn.
  const named: Scope = listed !== undefined && scope === `turn:${listed.latest}` ? "latest" : scope
  const label = choices.find((choice) => choice.key === named)?.label ?? (named.startsWith("turn:") ? `Turn ${named.slice(5)}` : "Latest turn")
  const totals = files !== undefined && files.length > 0 ? changedFilesLine(files) : undefined
  // The files with a diff to draw, and those that say why they have none.
  const [drawable, notes] = useMemo(() => {
    const drawn = (file: FileDiff) => file.patch !== undefined && file.patch !== ""
    return [files?.filter(drawn) ?? [], files?.filter((file) => !drawn(file)) ?? []]
  }, [files])
  const byTurns = scope === "latest" || scope === "session" || scope.startsWith("turn:")
  return (
    <>
      <Toolbar>
        <div className="flex min-w-0 flex-1 items-center">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                aria-label={`Diff scope: ${label}`}
                className="flex h-control-xs max-w-full min-w-0 items-center gap-1 rounded-md bg-text/5 pr-1.5 pl-2 text-caption font-medium text-text hover:bg-text/8 data-[state=open]:bg-text/8"
              >
                <span className="truncate">{label}</span>
                <CaretDown className="size-icon-caret shrink-0 opacity-70" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <DropdownMenuRadioGroup value={named} onValueChange={(value) => compareOther(value as Scope)}>
                {choices.map((choice) => (
                  <ScopeItem key={choice.key} choice={choice} />
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="flex shrink-0 items-center gap-0.5">
          {totals !== undefined && <Stat added={totals.added} removed={totals.removed} className="mr-1 text-2xs" />}
          <ToolButton label="Refresh diff" onClick={() => setAgain(again + 1)}>
            <ArrowClockwise className={cn(reading && "animate-spin motion-reduce:animate-none")} />
          </ToolButton>
          <div role="group" aria-label="Diff layout" className="mx-0.5 flex rounded-md bg-text/6 p-0.5">
            <Segment label="Stacked diff view" title="Stacked" pressed={shown.layout === "stacked"} onClick={() => drawAs("stacked")}>
              <Rows />
            </Segment>
            <Segment label="Split diff view" title="Side by side" pressed={shown.layout === "split"} onClick={() => drawAs("split")}>
              <Columns />
            </Segment>
          </div>
          <ToolButton label={shown.wrap ? "Disable line wrapping" : "Enable line wrapping"} pressed={shown.wrap} onClick={() => wrapLines(!shown.wrap)}>
            <ArrowElbowDownLeft />
          </ToolButton>
          <ToolButton label={shown.tree ? "Hide file tree" : "Show file tree"} pressed={shown.tree} onClick={() => showTree(!shown.tree)}>
            <TreeView />
          </ToolButton>
          <span aria-hidden="true" className="mx-1 h-4 w-px bg-line" />
          <CloseButton />
        </div>
      </Toolbar>
      {byTurns && listed !== undefined && !listed.kept ? (
        <Quiet>evie keeps no copies of the files it changes in this project, since its undo plugin is off, so what its turns changed cannot be shown.</Quiet>
      ) : shownRead === undefined ? (
        listProblem !== undefined ? <Problem text={listProblem} /> : <Loading />
      ) : "problem" in shownRead ? (
        <Problem text={shownRead.problem} />
      ) : shownRead.unavailable !== undefined ? (
        <Quiet>{shownRead.unavailable}</Quiet>
      ) : files === undefined || files.length === 0 ? (
        <Quiet>{nothingIn(scope, shownRead.base)}</Quiet>
      ) : (
        <div className="flex min-h-0 flex-1">
          <div className="min-h-0 min-w-0 flex-1">
            <Suspense fallback={<Loading />}>
              <DiffView
                files={drawable}
                layout={shown.layout}
                wrap={shown.wrap}
                folded={folded}
                fold={fold}
                goTo={goTo}
                wentTo={() => setGoTo(undefined)}
                {...(notes.length > 0 && { footer: <Notes files={notes} /> })}
              />
            </Suspense>
          </div>
          {shown.tree && (
            <nav aria-label="Changed files" className="flex w-diff-tree min-h-0 shrink-0 flex-col border-l border-line/60">
              <FilesHeader files={files} folded={treeFolded} setFolded={setTreeFolded} />
              <div className="flex min-h-0 flex-col overflow-y-auto px-1 pb-2">
                <FilesTreeRows
                  rows={filesTree(files, treeFolded)}
                  current={current}
                  fold={(path) =>
                    setTreeFolded((was) => {
                      const now = new Set(was)
                      if (!now.delete(path)) now.add(path)
                      return now
                    })
                  }
                  pick={toFile}
                />
              </div>
            </nav>
          )}
        </div>
      )}
    </>
  )
}

function ScopeItem({ choice }: { choice: ReturnType<typeof scopeChoices>[number] }) {
  return (
    <>
      {choice.key === "session" && <DropdownMenuSeparator />}
      <DropdownMenuRadioItem value={choice.key} disabled={choice.disabled !== undefined} className="data-disabled:text-text-muted">
        <span className="flex-1">{choice.label}</span>
        {choice.time !== undefined && <span className="pl-4 text-caption text-text-muted tabular-nums">{choice.time}</span>}
        {choice.disabled !== undefined && <span className="pl-4 text-caption text-text-muted">{choice.disabled}</span>}
      </DropdownMenuRadioItem>
    </>
  )
}

// One half of the stacked or side-by-side switch, as T3 Code's: 24px, and
// while pressed, on the panel's ground with a small shadow.
function Segment({ label, title, pressed, onClick, children }: { label: string; title: string; pressed: boolean; onClick(): void; children: ReactNode }) {
  return (
    <button
      type="button"
      aria-label={label}
      title={title}
      aria-pressed={pressed}
      onClick={onClick}
      className={cn("flex size-control-xs items-center justify-center rounded-chip [&>svg]:size-icon-caret", pressed ? "bg-bg text-text shadow-toggle" : "text-text-muted hover:text-text")}
    >
      {children}
    </button>
  )
}

// The tree's header, as T3 Code's: "Files", how many, and a button that
// folds or opens every folder.
function FilesHeader({ files, folded, setFolded }: { files: readonly ChangedFile[]; folded: ReadonlySet<string>; setFolded(folded: ReadonlySet<string>): void }) {
  const folders = filesTree(files, new Set()).flatMap((row) => (row.kind === "folder" ? [row.path] : []))
  const allOpen = folded.size === 0
  return (
    <div className="flex h-diff-toolbar shrink-0 items-center gap-2 pr-1 pl-3">
      <span className="text-caption font-medium text-text">Files</span>
      <span className="text-caption text-text-muted tabular-nums">{files.length}</span>
      <span className="flex-1" />
      {folders.length > 0 && (
        <ToolButton label={allOpen ? "Collapse all folders" : "Expand all folders"} onClick={() => setFolded(allOpen ? new Set(folders) : new Set())}>
          <CaretUpDown />
        </ToolButton>
      )}
    </div>
  )
}

// The files that have no diff to draw, after those that have, each in a
// row as a file's header, saying why.
function Notes({ files }: { files: readonly FileDiff[] }) {
  return (
    <ul aria-label="Files not shown" className="flex flex-col pb-2">
      {files.map((file) => (
        <li key={file.path} className="flex h-row-height min-w-0 items-center gap-1.5 pr-3 pl-8 text-caption">
          <span title={file.path} className="min-w-0 flex-1 truncate text-text">
            {file.path}
          </span>
          <span className="shrink-0 text-text-muted">{noteOf(file)}</span>
          {hasCounts(file) && <Stat added={file.added} removed={file.removed} className="ml-1 text-2xs" />}
        </li>
      ))}
    </ul>
  )
}

// Why a file has no diff to draw.
function noteOf(file: FileDiff): string {
  if (file.submodule) return "Submodule changed"
  if (file.tooMany) return "Not compared: one comparison reads at most 300 files"
  if (file.tooSlow) return "Not compared: it took too long"
  if (file.filtered) return "Filtered file, changes not shown"
  if (file.binary) return "Binary file, not shown"
  if (file.tooLarge) return "Too large to show"
  if (file.unknown) return "Changed outside evie, and no copy is kept"
  return file.status === "removed" ? "The file was empty" : "The file is empty"
}

// A line where the diff would be, as T3 Code's: in the middle, small and muted.
function Quiet({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-5 py-2 text-center text-caption text-text-muted/70">
      <p>{children}</p>
    </div>
  )
}

function Problem({ text }: { text: string }) {
  return <p className="px-3 py-2 text-2xs break-words text-status-failed/80">{text} Refresh to try again.</p>
}

// While the diff is read, or its view loads: bars where the files' headers
// and lines will be, as T3 Code's.
function Loading() {
  return (
    <div role="status" aria-label="Reading the changes" className="flex flex-col gap-2 px-3 py-3">
      {[60, 40].map((width) => (
        <div key={width} className="flex flex-col gap-2">
          <div className="h-4 rounded-sm bg-text/6" style={{ width: `${width}%` }} />
          <div className="h-3 w-11/12 rounded-sm bg-text/4" />
          <div className="h-3 w-3/4 rounded-sm bg-text/4" />
          <div className="h-3 w-5/6 rounded-sm bg-text/4" />
        </div>
      ))}
      <Spinner />
    </div>
  )
}

// What there is no change to show in, by what is compared.
function nothingIn(scope: Scope, base: string | undefined): string {
  if (scope === "latest" || scope.startsWith("turn:")) return "No changes in this turn."
  if (scope === "session") return "No changes in this session."
  if (scope === "uncommitted") return "No uncommitted changes."
  return base === undefined ? "No changes on this branch." : `No changes on this branch against ${base}.`
}

// The order T3 Code lists a pull request's checks in: those that need
// looking at, failing or waiting for someone, first; then those running;
// then the rest.
const CHECK_ORDER: Record<PullRequest["checks"][number]["state"], number> = { failed: 0, cancelled: 0, waiting: 0, running: 1, passed: 2, skipped: 2, neutral: 2 }
const PULL_COLOUR: Record<PullRequest["state"], string> = { open: "text-status-done", merged: "text-project-plum", closed: "text-status-failed", draft: "text-text-muted" }

// The branch's pull request, as T3 Code's view of one, read only: how it
// stands, its title, its number and the branch it goes into, and its
// checks, each with a link to its details; and a link to open it on GitHub.
function PullRequestTab({ pull }: { pull: PullRequest }) {
  const checks = [...pull.checks].sort((a, b) => CHECK_ORDER[a.state] - CHECK_ORDER[b.state])
  const state = pullRequestWord(pull.state)
  return (
    <>
      <Toolbar>
        <span className="flex min-w-0 flex-1 items-center gap-1.5 pl-1 text-caption font-medium text-text">
          <GitPullRequest className={cn("size-icon-caret shrink-0", PULL_COLOUR[pull.state])} />
          {state}
        </span>
        <span className="flex shrink-0 items-center gap-0.5">
          {pull.url !== undefined && (
            <a href={pull.url} target="_blank" rel="noreferrer" aria-label="Open the pull request on GitHub" title="Open on GitHub" className="flex size-control-sm items-center justify-center rounded-md text-text-muted hover:bg-text/8 hover:text-text">
              <ArrowSquareOut className="size-icon-caret" />
            </a>
          )}
          <span aria-hidden="true" className="mx-1 h-4 w-px bg-line" />
          <CloseButton />
        </span>
      </Toolbar>
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4">
        <div className="flex flex-col gap-2">
          <h2 className="text-body font-semibold break-words text-text">{pull.title}</h2>
          <p className="flex min-w-0 items-center gap-2 text-caption text-text-muted">
            <span className="tabular-nums">#{pull.number}</span>
            <span aria-hidden="true">·</span>
            <span className="min-w-0 truncate">
              into <span className="font-mono">{pull.base}</span>
            </span>
          </p>
        </div>
        <section aria-label="Checks" className="flex flex-col gap-2">
          <div className="flex items-baseline gap-2">
            <h3 className="text-caption font-medium text-text">Checks</h3>
            <span className="text-caption text-text-muted">{checksSummary(pull.checks)}</span>
          </div>
          {checks.length > 0 && (
            <ul className="grid grid-cols-[auto_minmax(0,1fr)_auto_auto] gap-x-2 gap-y-1">
              {checks.map((check, at) => (
                <li key={at} className="col-span-full grid min-h-control-xs grid-cols-subgrid items-center text-caption">
                  <CheckIcon state={check.state} />
                  <span className="min-w-0 truncate text-text">{check.name}</span>
                  <span className="justify-self-end whitespace-nowrap text-text-muted">{checkWord(check.state)}</span>
                  {check.url !== undefined ? (
                    <a href={check.url} target="_blank" rel="noreferrer" title={`${check.name}: open its details`} className="text-accent hover:underline">
                      Details
                    </a>
                  ) : (
                    <span />
                  )}
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </>
  )
}

// A check's state as T3 Code draws it: passed with a check in green, failed
// with a cross in red, running with the spinner, waiting with a dashed ring,
// and the rest muted.
function CheckIcon({ state }: { state: PullRequest["checks"][number]["state"] }) {
  if (state === "passed") return <CheckCircle className="size-icon-caret text-status-done" />
  if (state === "failed") return <XCircle className="size-icon-caret text-status-failed" />
  if (state === "running") return <Spinner />
  if (state === "waiting") return <CircleDashed className="size-icon-caret text-status-needs" />
  return <CircleDashed className="size-icon-caret text-text-muted" />
}
