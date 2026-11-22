import * as DialogPrimitive from "@radix-ui/react-dialog"
import { Fragment, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { FileDiff, ListedProject } from "../../src/core/types.ts"
import { pullRequestOf } from "./card.ts"
import { useCardShown } from "./card-shown.ts"
import { holdChanges, type Changes, type ChangesState } from "./changes.ts"
import { Counts } from "./ChangedFiles.tsx"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import type { Connection } from "./connection.ts"
import { startedHere } from "./conversation.ts"
import { changedFilesLine, fileTree, hunksOf, scopeChoices, sideBySide, type Line, type Scope } from "./diff.ts"
import { closeDiff, compareLatest, compareOther, drawAs, showTree, useDiffShown, wentToFile } from "./diff-shown.ts"
import { ArrowClockwise, CaretDown, CaretLeft, CaretRight, Columns, Rows, TreeView, X } from "./icons.tsx"
import { useSelected } from "./selection.ts"
import type { Sessions } from "./sessions.ts"
import { Spinner } from "./Spinner.tsx"

// The diff panel, as T3 Code's: what the selected session changed, by turn
// or for the whole session, and in a git project the uncommitted work and
// the branch, picked from a menu at its top. Each file's header stays in
// view while its file scrolls past, and folds the file when pressed; the
// file tree beside the diff, which can be hidden, scrolls to a file. A diff
// is drawn stacked or side by side, as this browser remembers. At desktop
// width it is a column beside the conversation, in the project card's
// place, and wider than the card, since a diff needs the room; at 980px or
// narrower it fills the screen, with a way back.
export function DiffPlace({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useDiffShown()
  const { wide } = useCardShown()
  const selected = useSelected()
  // Another session's turns are not this one's: it opens on its latest.
  const session = selected !== undefined && "session" in selected ? selected.session : undefined
  const was = useRef(session)
  useEffect(() => {
    if (was.current === session) return
    was.current = session
    compareLatest()
  }, [session])
  if (!shown.open) return null
  const panel = <Panel connection={connection} sessions={sessions} session={session} narrow={!wide} />
  if (wide) {
    return (
      <aside aria-label="Diff" className="flex min-h-0 min-w-0 flex-[1.5_1.5_0%] flex-col border-l border-line bg-bg">
        {panel}
      </aside>
    )
  }
  return (
    <DialogPrimitive.Root open onOpenChange={(open) => !open && closeDiff()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Content aria-describedby={undefined} className="fixed inset-0 z-50 flex flex-col bg-bg text-body text-text">
          <DialogPrimitive.Title className="sr-only">Diff</DialogPrimitive.Title>
          {panel}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

// The panel for what is selected: a session's changes, or a line saying
// there is no session to show.
function Panel({ connection, sessions, session, narrow }: { connection: Connection; sessions: Sessions; session: string | undefined; narrow: boolean }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  if (session === undefined) {
    return (
      <>
        <Top narrow={narrow} />
        <Quiet>No session is open. The changes a session makes are shown here.</Quiet>
      </>
    )
  }
  const summary = list.sessions.find((each) => each.session === session)
  const projectId = summary !== undefined ? summary.project : (startedHere(session) ?? null)
  const project = projectId === null ? null : list.projects.find((each) => each.id === projectId)
  return <SessionDiff key={session} connection={connection} session={session} project={project} narrow={narrow} />
}

const NOT_LISTED: ChangesState = { listed: undefined, problem: undefined }
const noChanges = { subscribe: () => () => {}, state: () => NOT_LISTED }
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// What has been read for what is compared now, by its key, so that what
// was read for something else is not shown under its name.
type Read = { scope: Scope; files: FileDiff[]; base?: string } | { scope: Scope; problem: string }

function SessionDiff({ connection, session, project, narrow }: { connection: Connection; session: string; project: ListedProject | null | undefined; narrow: boolean }) {
  const shown = useDiffShown()
  const [changes, setChanges] = useState<Changes>()
  useEffect(() => {
    const held = holdChanges(connection, session)
    setChanges(held.changes)
    return held.release
  }, [connection, session])
  const { listed, problem: listProblem } = useSyncExternalStore(changes?.subscribe ?? noChanges.subscribe, changes?.state ?? noChanges.state)
  const git = project?.git !== false && project !== null
  const choices = useMemo(() => scopeChoices({ turns: listed?.turns ?? [], git, now: Date.now() }), [listed, git])

  // Read again as it is picked, as each turn ends, and when asked.
  const [read, setRead] = useState<Read>()
  const [again, setAgain] = useState(0)
  const base = project ? pullRequestOf(project.id)?.base : undefined
  const scope = shown.scope
  useEffect(() => {
    if (changes === undefined) return
    if ((scope === "uncommitted" || scope === "branch") && !git) {
      setRead({ scope, problem: project === null ? "This session has no project, so there is no git repository." : "Not a git repository" })
      return
    }
    // The latest turn is known once the list has been read.
    if (scope === "latest" && listed === undefined) return
    let current = true
    changes.read(scope, scope === "branch" ? base : undefined).then(
      (answer) => current && setRead({ scope, ...answer }),
      (cause: unknown) => current && setRead({ scope, problem: wordsOf(cause) }),
    )
    return () => {
      current = false
    }
  }, [changes, scope, listed, again, base, git])

  // Folded files, by path, and the tree's rows, for what is compared now.
  const [folded, setFolded] = useState<ReadonlySet<string>>(new Set())
  useEffect(() => setFolded(new Set()), [scope])
  const shownRead = read?.scope === scope ? read : undefined
  const files = shownRead !== undefined && "files" in shownRead ? shownRead.files : undefined
  const tree = useMemo(() => fileTree(files?.map((file) => file.path) ?? []), [files])
  const ordered = useMemo(() => {
    const byPath = new Map(files?.map((file) => [file.path, file]))
    return tree.flatMap((row) => (row.kind === "file" ? [byPath.get(row.path)!] : []))
  }, [files, tree])

  // At phone width the tree, when shown, takes the diff's place, and goes
  // once a file is picked in it.
  const [treeHere, setTreeHere] = useState(false)
  const treeShown = narrow ? treeHere : shown.tree
  const scroller = useRef<HTMLDivElement>(null)
  const [goTo, setGoTo] = useState<string>()
  function toFile(path: string) {
    setFolded((was) => {
      const now = new Set(was)
      now.delete(path)
      return now
    })
    if (narrow) setTreeHere(false)
    setGoTo(path)
  }
  // Opened at a file, from a turn's line: gone to once, when it is read.
  useEffect(() => {
    if (shown.file === undefined || !files?.some((file) => file.path === shown.file)) return
    toFile(shown.file)
    wentToFile()
  }, [files, shown.file])
  useLayoutEffect(() => {
    if (goTo === undefined) return
    scroller.current?.querySelector<HTMLElement>(`[data-file="${CSS.escape(goTo)}"]`)?.scrollIntoView({ block: "start" })
    setGoTo(undefined)
  })

  const label = choices.find((choice) => choice.key === scope)?.label ?? (scope.startsWith("turn:") ? `Turn ${scope.slice(5)}` : "Latest turn")
  const layout = narrow ? "stacked" : shown.layout
  return (
    <>
      <Top narrow={narrow}>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="secondary" aria-label={`Compare: ${label}`} className="min-w-0 gap-1 px-3">
              <span className="truncate">{label}</span>
              <CaretDown className="size-icon-sm text-text-muted" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuRadioGroup value={scope} onValueChange={(value) => compareOther(value as Scope)}>
              {choices.map((choice, at) => (
                <Fragment key={choice.key}>
                  {choice.key === "session" && <DropdownMenuSeparator />}
                  {at === 1 && choice.key.startsWith("turn:") && <DropdownMenuLabel>Turns</DropdownMenuLabel>}
                  <DropdownMenuRadioItem value={choice.key}>
                    <span className="flex-1">{choice.label}</span>
                    {choice.time !== undefined && <span className="pl-4 text-caption text-text-muted">{choice.time}</span>}
                  </DropdownMenuRadioItem>
                </Fragment>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <span className="flex-1" />
        {!narrow && (
          <>
            <Toggle label="Stacked" pressed={layout === "stacked"} onClick={() => drawAs("stacked")}>
              <Rows />
            </Toggle>
            <Toggle label="Side by side" pressed={layout === "split"} onClick={() => drawAs("split")}>
              <Columns />
            </Toggle>
          </>
        )}
        <Toggle label="File tree" pressed={treeShown} onClick={() => (narrow ? setTreeHere(!treeHere) : showTree(!shown.tree))}>
          <TreeView />
        </Toggle>
        <Button variant="icon" aria-label="Read the changes again" title="Read the changes again" onClick={() => setAgain(again + 1)}>
          <ArrowClockwise />
        </Button>
      </Top>
      <div className="flex min-h-0 flex-1">
        {!(narrow && treeShown) && (
          <div ref={scroller} className="min-h-0 min-w-0 flex-1 overflow-y-auto pb-8">
            <Body
              read={shownRead}
              files={ordered}
              listProblem={listProblem}
              kept={listed?.kept ?? true}
              scope={scope}
              layout={layout}
              folded={folded}
              fold={(path) =>
                setFolded((was) => {
                  const now = new Set(was)
                  if (!now.delete(path)) now.add(path)
                  return now
                })
              }
              tryAgain={() => setAgain(again + 1)}
            />
          </div>
        )}
        {treeShown && files !== undefined && files.length > 0 && (
          <nav aria-label="Changed files" className={`min-h-0 overflow-y-auto py-2 ${narrow ? "flex-1" : "w-diff-tree shrink-0 border-l border-line"}`}>
            <ul className="flex flex-col">
              {tree.map((row) => {
                const indent = { paddingLeft: `${12 + row.depth * 12}px` }
                if (row.kind === "folder") {
                  return (
                    <li key={`folder:${row.depth}:${row.name}`} style={indent} title={row.name} className="flex h-control min-w-0 items-center pr-3 text-caption text-text-muted">
                      <span className="truncate">{row.name}/</span>
                    </li>
                  )
                }
                const file = files.find((each) => each.path === row.path)!
                return (
                  <li key={row.path}>
                    <button type="button" style={indent} title={row.path} onClick={() => toFile(row.path)} className="flex h-control w-full min-w-0 items-center gap-2 pr-3 text-left hover:bg-surface-raised">
                      <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{row.name}</span>
                      {!file.binary && !file.unknown && <Counts added={file.added} removed={file.removed} />}
                    </button>
                  </li>
                )
              })}
            </ul>
          </nav>
        )}
      </div>
    </>
  )
}

// The panel's header: at phone width a way back first, then what it was
// given, then at desktop width the button that closes it.
function Top({ narrow, children }: { narrow: boolean; children?: ReactNode }) {
  return (
    <header className={`flex h-header-height shrink-0 items-center gap-1 border-b border-line pr-4 ${narrow ? "pl-2" : "pl-7"}`}>
      {narrow && (
        <Button variant="icon" aria-label="Back to the conversation" onClick={closeDiff}>
          <CaretLeft />
        </Button>
      )}
      {children ?? <h2 className="flex-1 text-title text-text">Diff</h2>}
      {!narrow && (
        <Button variant="icon" aria-label="Close the diff" title={`Close the diff (${/Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘D" : "Ctrl+D"})`} onClick={closeDiff}>
          <X />
        </Button>
      )}
    </header>
  )
}

// An icon button that stays pressed while what it stands for is on: its
// icon is bright then, and quiet otherwise.
function Toggle({ label, pressed, onClick, children }: { label: string; pressed: boolean; onClick(): void; children: ReactNode }) {
  return (
    <Button variant="icon" aria-label={label} title={label} aria-pressed={pressed} onClick={onClick} className={pressed ? "text-text" : ""}>
      {children}
    </Button>
  )
}

function Quiet({ children }: { children: ReactNode }) {
  return <p className="px-7 py-6 text-body text-text-muted">{children}</p>
}

// What there is no change to show in, by what is compared.
function nothingIn(scope: Scope, base: string | undefined): string {
  if (scope === "latest" || scope.startsWith("turn:")) return "No changes in this turn"
  if (scope === "session") return "No changes in this session"
  if (scope === "uncommitted") return "No uncommitted changes"
  return base === undefined ? "No changes on this branch" : `No changes on this branch against ${base}`
}

function Body({
  read,
  files,
  listProblem,
  kept,
  scope,
  layout,
  folded,
  fold,
  tryAgain,
}: {
  read: Read | undefined
  files: FileDiff[]
  listProblem: string | undefined
  kept: boolean
  scope: Scope
  layout: "stacked" | "split"
  folded: ReadonlySet<string>
  fold(path: string): void
  tryAgain(): void
}) {
  const byTurns = scope === "latest" || scope === "session" || scope.startsWith("turn:")
  if (byTurns && !kept) return <Quiet>evie keeps no copies of the files it changes in this project, since its undo plugin is off, so what its turns changed cannot be shown.</Quiet>
  if (read === undefined) {
    if (listProblem !== undefined) return <Problem text={listProblem} tryAgain={tryAgain} />
    return (
      <p className="flex min-h-control items-center gap-2 px-7 py-6 text-body text-status-working">
        <Spinner />
        Reading the changes
      </p>
    )
  }
  if ("problem" in read) return read.problem === "Not a git repository" ? <Quiet>Not a git repository</Quiet> : <Problem text={read.problem} tryAgain={tryAgain} />
  if (files.length === 0) return <Quiet>{nothingIn(scope, read.base)}</Quiet>
  const line = changedFilesLine(files)
  return (
    <>
      <p className="flex flex-wrap items-center gap-2 px-7 pt-4 pb-3 text-caption text-text-muted">
        {line.files}
        <Counts added={line.added} removed={line.removed} />
        {read.base !== undefined && <span>against {read.base}</span>}
      </p>
      {files.map((file) => (
        <FileSection key={file.path} file={file} layout={layout} folded={folded.has(file.path)} fold={() => fold(file.path)} />
      ))}
    </>
  )
}

function Problem({ text, tryAgain }: { text: string; tryAgain(): void }) {
  return (
    <div className="flex flex-col items-start px-7 py-6">
      <p className="text-body break-words text-text-muted">{text}</p>
      <Button variant="quiet" onClick={tryAgain} className="-ml-3">
        Try again
      </Button>
    </div>
  )
}

// One file: its header, which stays at the top of the panel while the file
// scrolls past and folds it when pressed, and its diff, or why there is none.
function FileSection({ file, layout, folded, fold }: { file: FileDiff; layout: "stacked" | "split"; folded: boolean; fold(): void }) {
  return (
    <section data-file={file.path} aria-label={file.path} className="border-t border-line">
      <h3 className="sticky top-0 z-10 bg-bg">
        <button type="button" aria-expanded={!folded} onClick={fold} title={file.path} className="flex h-row-height w-full min-w-0 items-center gap-2 border-b border-line pr-4 pl-7 text-left">
          <CaretRight className={`size-icon-sm shrink-0 text-text-muted transition-transform motion-reduce:transition-none ${folded ? "" : "rotate-90"}`} />
          <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{file.path}</span>
          {file.status !== "changed" && <span className="shrink-0 text-caption text-text-muted">{file.status === "added" ? "Added" : "Removed"}</span>}
          {!file.binary && !file.unknown && <Counts added={file.added} removed={file.removed} />}
        </button>
      </h3>
      {!folded && <FileDiffBody file={file} layout={layout} />}
    </section>
  )
}

function FileDiffBody({ file, layout }: { file: FileDiff; layout: "stacked" | "split" }) {
  const hunks = useMemo(() => hunksOf(file.patch ?? ""), [file.patch])
  const note = (text: string) => <p className="px-7 py-3 text-caption text-text-muted">{text}</p>
  if (file.binary) return note("This file is binary, so it is not shown.")
  if (file.tooLarge) return note("This file is too large to show.")
  if (file.unknown) return note("What this file held is not known: it has been changed outside evie's file tools since, and no copy of it is kept.")
  if (hunks.length === 0) return note(file.status === "removed" ? "The file was empty." : "The file is empty.")
  // Room for the longest line number in the file.
  const digits = String(Math.max(...hunks.flatMap((hunk) => hunk.lines.map((line) => Math.max(line.old ?? 0, line.new ?? 0))))).length
  const number = { width: `${digits + 2}ch` }
  return (
    <div className="pb-3 font-mono text-code">
      {hunks.map((hunk) => (
        <Fragment key={hunk.header}>
          <div className="px-7 py-1 wrap-anywhere text-text-muted select-none">{hunk.header}</div>
          {layout === "stacked"
            ? hunk.lines.map((line, at) => (
                <div key={at} className={`flex ${washOf(line)}`}>
                  <LineNumber value={line.old} style={number} />
                  <LineNumber value={line.new} style={number} />
                  <Text line={line} />
                </div>
              ))
            : sideBySide(hunk.lines).map((row, at) => (
                <div key={at} className="flex">
                  <Half line={row.left} side="old" style={number} />
                  <Half line={row.right} side="new" style={number} />
                </div>
              ))}
        </Fragment>
      ))}
    </div>
  )
}

// The faint wash of colour under a line added or removed.
const washOf = (line: Line | undefined) => (line?.kind === "added" ? "bg-diff-added-line" : line?.kind === "removed" ? "bg-diff-removed-line" : "")

function LineNumber({ value, style }: { value: number | undefined; style: { width: string } }) {
  return (
    <span style={style} className="shrink-0 pr-2 text-right text-text-muted select-none">
      {value ?? ""}
    </span>
  )
}

// A line's sign and text: + or − in its colour, and the text, wrapped
// where it is too long for the panel.
function Text({ line }: { line: Line }) {
  const sign = line.kind === "added" ? "+" : line.kind === "removed" ? "−" : " "
  const colour = line.kind === "added" ? "text-diff-added" : line.kind === "removed" ? "text-diff-removed" : ""
  return (
    <>
      <span className={`w-[2ch] shrink-0 select-none ${colour}`}>{sign}</span>
      <span className="min-w-0 flex-1 pr-4 whitespace-pre-wrap wrap-anywhere text-text">
        {line.text}
        {line.noNewline && <span className="text-text-muted select-none"> (no newline at the end)</span>}
      </span>
    </>
  )
}

// One side of a row side by side: before on the left, after on the right,
// or nothing, where the other side has a line with nothing facing it.
function Half({ line, side, style }: { line: Line | undefined; side: "old" | "new"; style: { width: string } }) {
  return (
    <div className={`flex min-w-0 flex-1 ${side === "new" ? "border-l border-line" : ""} ${washOf(line)}`}>
      {line !== undefined && (
        <>
          <LineNumber value={side === "old" ? line.old : line.new} style={style} />
          <Text line={line} />
        </>
      )}
    </div>
  )
}
