import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import { QuestionPrompt, QuestionText, useAsking } from "./Asking.tsx"
import { TurnChangedFiles } from "./ChangedFiles.tsx"
import { holdChanges, type Changes } from "./changes.ts"
import { asAsked } from "./questions.ts"
import { QueuedMessages } from "./Queued.tsx"
import { Button } from "./components/ui/button.tsx"
import { Composer, FileChip } from "./Composer.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, openDraft, type Conversation as Opened, type Draft as OpenedDraft, type Outgoing } from "./conversation.ts"
import type { ShownAttachment } from "./attachments.ts"
import { Dialog, DialogContent, DialogTitle } from "./components/ui/dialog.tsx"
import { CaretRight, ChatsCircle, File as FileIcon, FileText, Globe, Hammer, Lightbulb, MagnifyingGlass, NotePencil, Robot, TerminalWindow, Wrench, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { markdownBlocks, renderMarkdown } from "./markdown.ts"
import { addressOf } from "./address.ts"
import type { Sessions } from "./sessions.ts"
import { Spinner } from "./Spinner.tsx"
import { runningSince, transcriptOf, type Call, type Item } from "./transcript.ts"
import type { StepKind } from "./step-words.ts"
import { serverNow, workingFor } from "./work-log.ts"
import { sizeWords } from "./workspace.ts"

// The conversation of the session selected, in the middle column: its
// latest page, with earlier pages read as the owner scrolls up to them,
// and what happens in it as it happens. The owner who is at the bottom is
// kept there as new text comes; one who has scrolled up is left where they
// are, and offered a way back down. `folder` is its project's, when known,
// which the work log names the project's files from.
export function Conversation({ connection, session, folder }: { connection: Connection; session: string; folder?: string | undefined }) {
  const [opened, setOpened] = useState<{ session: string; conversation: Opened; changes: Changes }>()
  useEffect(() => {
    const held = holdConversation(connection, session)
    const changes = holdChanges(connection, session)
    setOpened({ session, conversation: held.conversation, changes: changes.changes })
    return () => {
      changes.release()
      held.release()
    }
  }, [connection, session])
  if (opened?.session !== session) return null
  return <Reading key={session} conversation={opened.conversation} changes={opened.changes} folder={folder} />
}

// How near the bottom, in pixels, counts as at the bottom, so that a
// reply's last line wrapping does not count as scrolling up.
const AT_BOTTOM = 32
// How many of the first items are remembered to keep the owner's place.
const ANCHORS = 5

// A conversation as it reads, the session's own, with its message box, or
// one of its subagents', which is read only: it has no box, since nothing
// is said in a subagent's conversation from the page. With `changes`, each
// completed turn that changed files ends with them. A question waiting
// shows at the end, as part of evie's reply; while a turn runs with none,
// a line there says so, with a way to stop it, as the Session view drawing
// has it.
export function Reading({ conversation, readOnly = false, changes, folder }: { conversation: Opened; readOnly?: boolean; changes?: Changes; folder?: string | undefined }) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const { events, earlier, replySoFar, turnRunning, jobs, pending, notes } = state
  const changed = useSyncExternalStore(changes?.subscribe ?? noChanges.subscribe, changes?.state ?? noChanges.state).listed?.turns
  // A question waiting is shown where it is answered, at the end, so it is
  // left out where it was asked until it is answered.
  const items = useMemo(
    () => asAsked(transcriptOf({ events, earlier, replySoFar, turnRunning, jobs, pending, notes, ...(folder !== undefined && { folder }) }, changed), state.waiting),
    [events, earlier, replySoFar, turnRunning, jobs, pending, notes, changed, state.waiting, folder],
  )
  const asking = useAsking(state.waiting)
  const { shown } = asking
  // How tall the message box floats over the conversation's end, which
  // scrolls clear of it.
  const [floating, setFloating] = useState(0)

  const scroller = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  // Whether the owner is at the bottom, to be kept there; and where the
  // first few items were as last drawn, to keep the owner's place when
  // something is added above them, such as the page before.
  const stuck = useRef(true)
  const [scrolledUp, setScrolledUp] = useState(false)
  // What the owner has opened: a line of steps, by its calls, so that it
  // stays open as the page before is read and as calls join it, and a
  // call's result, by the call.
  const [opens, setOpens] = useState<ReadonlySet<string>>(new Set())
  const show = (ids: string[], open: boolean) =>
    setOpens((was) => {
      const now = new Set(was)
      for (const id of ids) if (open) now.add(id)
      else now.delete(id)
      return now
    })
  const drawn = useRef<{ key: string; top: number }[]>([])

  // Within half a screen of the top, the page before is read.
  const readIfNearTop = () => {
    const box = scroller.current
    if (box && box.scrollTop < box.clientHeight / 2) conversation.readEarlier()
  }
  function onScroll() {
    const box = scroller.current!
    stuck.current = box.scrollHeight - box.scrollTop - box.clientHeight <= AT_BOTTOM
    setScrolledUp(!stuck.current)
    readIfNearTop()
  }
  function toLatest() {
    const box = scroller.current!
    stuck.current = true
    box.scrollTop = box.scrollHeight
    setScrolledUp(false)
  }

  // After each change, before it is shown: kept at the bottom, or moved by
  // as far as the first of the items drawn last time that is still there.
  // A few are kept, since the first can change as the page before joins
  // its turn to the rest.
  useLayoutEffect(() => {
    const box = scroller.current
    if (!box) return
    const placed = (key: string) => box.querySelector<HTMLElement>(`[data-item="${CSS.escape(key)}"]`)
    if (stuck.current) box.scrollTop = box.scrollHeight
    else {
      const anchor = drawn.current.find((each) => placed(each.key) !== null)
      if (anchor) box.scrollTop += placed(anchor.key)!.offsetTop - anchor.top
    }
    drawn.current = [...box.querySelectorAll<HTMLElement>("[data-item]")].slice(0, ANCHORS).map((each) => ({ key: each.dataset.item!, top: each.offsetTop }))
    readIfNearTop()
  })
  // What grows by itself, as a reply rewrapping when the window is resized,
  // or a line of steps opened at the bottom.
  useEffect(() => {
    const box = scroller.current
    if (!box || !content.current) return
    const watch = new ResizeObserver(() => {
      if (stuck.current) box.scrollTop = box.scrollHeight
    })
    watch.observe(content.current)
    watch.observe(box)
    return () => watch.disconnect()
  }, [])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="relative flex min-h-0 flex-1 flex-col">
        {/* The browser's own anchoring is off, since this keeps the owner's place itself. */}
        <div ref={scroller} onScroll={onScroll} className="min-h-0 flex-1 overflow-y-auto [overflow-anchor:none]">
          <div ref={content} className="flex min-h-full flex-col px-gutter pt-5" style={{ paddingBottom: `calc(var(--spacing-5) + ${floating}px)` }}>
            <div className="mx-auto flex w-full max-w-conversation flex-1 flex-col">
              {state.readingEarlier && <Working words="Reading the earlier conversation" />}
              {/* What failed is tried again only when asked, or when the line comes back. */}
              {state.problem !== undefined && (
                <div className="mb-4 flex flex-col items-start">
                  <p className="text-body break-words text-text-muted">{state.problem}</p>
                  <Button variant="quiet" onClick={() => conversation.tryAgain()} className="-ml-3">
                    Try again
                  </Button>
                </div>
              )}
              {!state.opened && state.problem === undefined && <Working words="Opening the session" />}
              {state.opened && state.problem === undefined && items.length === 0 && shown === undefined && <Headline />}
              {items.map((item, at) => (
                <div key={item.key} data-item={item.key} className={`flex flex-col ${at === 0 ? "" : item.turn !== items[at - 1]!.turn ? "mt-4" : "mt-2"}`}>
                  <Drawn item={item} shown={opens} show={show} />
                </div>
              ))}
              {state.running.filter((name) => name !== "compact").map((name, at) => (
                <div key={`${name}${at}`} className={items.length > 0 || at > 0 ? "mt-2" : ""}>
                  <Working words={`Running /${name}`} />
                </div>
              ))}
              {shown !== undefined ? (
                <div data-item="question" className={items.length > 0 ? "mt-4" : ""}>
                  <QuestionPrompt question={shown.question} count={shown.count} answer={conversation.answer} />
                </div>
              ) : (
                turnRunning && (
                  <div className={items.length > 0 ? "mt-2" : ""}>
                    <RunningLine since={runningSince(state)} stopping={state.stopping} {...(!readOnly && { stop: () => conversation.stop() })} />
                  </div>
                )
              )}
              {!readOnly && state.queue.items.length > 0 && <QueuedMessages conversation={conversation} queue={state.queue} turnRunning={turnRunning} asked={shown !== undefined} />}
            </div>
          </div>
        </div>
        {scrolledUp && (
          <div className="pointer-events-none absolute inset-x-0 z-30 flex justify-center" style={{ bottom: `calc(var(--spacing-1) + ${floating}px)` }}>
            <Button variant="quiet" onClick={toLatest} className="pointer-events-auto h-control-sm rounded-full bg-surface px-3 text-caption shadow-float">
              Jump to the latest
            </Button>
          </div>
        )}
        {!readOnly && (
          <Composer
            conversation={conversation}
            state={state}
            model={{ current: state.model, list: conversation.listModels, pick: conversation.setModel }}
            shown={shown}
            answer={conversation.answer}
            asking={asking}
            send={(text, files) => conversation.send(text, { files })}
            mentions={state.project ? conversation.listFiles : undefined}
            floats={setFloating}
          />
        )}
      </div>
    </div>
  )
}

// For a conversation shown without its changes, as a subagent's is: the
// same state each time it is asked for, as React needs.
const NONE_LISTED = { listed: undefined, problem: undefined }
const noChanges = { subscribe: () => () => {}, state: () => NONE_LISTED }

// A draft of a new session, in the middle column: the drawn "What should
// evie do?", and the message box, whose first message starts it. While it starts, the
// message shows as the owner's, and a question asked as the session opens,
// such as whether to trust its project, is answered in place. Once its
// conversation has been read, the page moves to the session, or does what
// `moved` says instead, as a workspace's side chat does.
export function Draft({
  connection,
  sessions,
  project,
  moved,
}: {
  connection: Connection
  sessions: Sessions
  project: string | null
  moved?: (session: string) => void
}) {
  const [opened, setOpened] = useState<{ key: string; draft: OpenedDraft }>()
  const key = `${project}`
  useEffect(() => {
    const draft = openDraft(connection, project, { listed: (session) => sessions.state().sessions.some((each) => each.session === session) })
    setOpened({ key, draft })
    return () => draft.close()
  }, [connection, sessions, key])
  if (opened?.key !== key) return null
  return <Drafting key={key} draft={opened.draft} moved={moved ?? ((session) => location.replace(addressOf({ session })))} inProject={project !== null} />
}

function Drafting({ draft, moved, inProject }: { draft: OpenedDraft; moved(session: string): void; inProject: boolean }) {
  const state = useSyncExternalStore(draft.subscribe, draft.state)
  const asking = useAsking(state.waiting)
  const { shown } = asking
  const [floating, setFloating] = useState(0)
  const clear = { paddingBottom: `calc(var(--spacing-5) + ${floating}px)` }
  async function start(text: string, files: readonly Outgoing[]) {
    if (state.starting !== undefined) return "The session is starting. Send this once it has."
    const started = await draft.start(text, moved, files)
    return "problem" in started ? started.problem : undefined
  }
  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        {state.starting === undefined ? (
          <div className="flex min-h-full flex-col px-gutter pt-5" style={clear}>
            <Headline />
          </div>
        ) : (
          <div className="px-gutter pt-5" style={clear}>
            <div className="mx-auto flex max-w-conversation flex-col gap-4">
              <Drawn item={{ key: "starting", turn: 0, kind: "owner", text: state.starting }} shown={new Set()} show={() => {}} />
              {shown !== undefined ? <QuestionPrompt question={shown.question} count={shown.count} answer={draft.answer} /> : <Working words="Starting the session" />}
            </div>
          </div>
        )}
      </div>
      {/* The model picked here is the session's from its first message. */}
      <Composer
        model={{ current: state.model, list: draft.listModels, pick: async (model) => (draft.pick(model), undefined) }}
        shown={shown}
        answer={draft.answer}
        asking={asking}
        send={start}
        mentions={inProject ? draft.listFiles : undefined}
        floats={setFloating}
      />
    </div>
  )
}

// A conversation with nothing in it yet, as the Session view drawing has
// it: one line in its middle, over the message box.
function Headline() {
  return <h2 className="m-auto text-center text-page-title text-text">What should evie do?</h2>
}

// The middle column with nothing to read in it: a title and a line.
export function Placeholder({ title, text }: { title: string; text: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-1 px-gutter py-8 text-center">
      <h1 className="text-page-title text-text">{title}</h1>
      <p className="max-w-sign-in-column text-body text-text-muted">{text}</p>
    </div>
  )
}

// Which lines and results the owner has opened, and how to open or close them.
type Shown = { shown: ReadonlySet<string>; show: (ids: string[], open: boolean) => void }

function Drawn({ item, shown, show }: { item: Item } & Shown) {
  switch (item.kind) {
    case "owner":
      return (
        <>
          {item.text !== "" && <p className="max-w-4/5 self-end rounded-lg bg-surface-raised p-3 text-prose break-words whitespace-pre-wrap text-text">{item.text}</p>}
          {item.attachments !== undefined && <Attachments attachments={item.attachments} spaced={item.text !== ""} />}
          {item.sending !== undefined && <SendingFiles files={item.sending} spaced={item.text !== ""} />}
        </>
      )
    case "reply":
      return <Markdown text={item.text} streaming={item.streaming} className="text-text" />
    case "steps":
      return <WorkRun item={item} shown={shown} show={show} />
    case "thinking":
      return <ThinkingRow />
    case "fold":
      return <Fold item={item} shown={shown} show={show} />
    case "note":
      return <Note text={item.text} {...(item.more !== undefined && { more: item.more })} />
    // A question as it was asked. One waiting is drawn where it is answered.
    case "question":
      return <QuestionText text={item.text} />
    case "changes":
      return <TurnChangedFiles turn={item.number} files={item.files} />
    case "failed":
      return (
        <div className="text-body">
          <p className="text-status-failed">Failed</p>
          <p className="break-words text-text-muted">{item.error}</p>
        </div>
      )
  }
}

// The files of the owner's message on its way, under it, each as the box
// showed it, and the word Sending, with the small spinner Working has.
function SendingFiles({ files, spaced }: { files: readonly File[]; spaced: boolean }) {
  return (
    <ul aria-label="Files sending" className={`flex max-w-4/5 flex-wrap justify-end gap-x-4 self-end ${spaced ? "mt-2" : ""}`}>
      {files.map((file, at) => (
        <li key={at} className="flex h-control min-w-0 items-center gap-2">
          <FileChip file={file} />
        </li>
      ))}
      <li className="flex h-control items-center gap-2 text-caption text-status-working">
        <Spinner />
        Sending
      </li>
    </ul>
  )
}

// The files attached to the owner's message, under it: an image as a
// thumbnail that opens full size, any other file as its name and size.
function Attachments({ attachments, spaced }: { attachments: ShownAttachment[]; spaced: boolean }) {
  const [opened, setOpened] = useState<ShownAttachment>()
  const images = attachments.filter((each) => each.address !== undefined)
  const others = attachments.filter((each) => each.address === undefined)
  return (
    <div className={`flex max-w-4/5 flex-col items-end gap-2 self-end ${spaced ? "mt-2" : ""}`}>
      {images.length > 0 && (
        <ul aria-label="Images attached" className="flex flex-wrap justify-end gap-2">
          {images.map((each) => (
            <li key={each.path}>
              <button type="button" onClick={() => setOpened(each)} aria-label={`Open ${each.name}`} title={each.name} className="block overflow-hidden rounded-md">
                <img src={each.address} alt={each.name} loading="lazy" className="max-h-attachment-shown max-w-attachment-shown object-contain" />
              </button>
            </li>
          ))}
        </ul>
      )}
      {others.length > 0 && (
        <ul aria-label="Files attached" className="flex flex-col items-end">
          {others.map((each) => (
            <li key={each.path} title={each.path} className="flex min-h-control max-w-full items-center gap-2 text-caption text-text-muted">
              <FileIcon className="size-icon-sm shrink-0" />
              <span className="min-w-0 truncate text-text">{each.name}</span>
              <span className="shrink-0">{sizeWords(each.size)}</span>
            </li>
          ))}
        </ul>
      )}
      <Dialog open={opened !== undefined} onOpenChange={(open) => !open && setOpened(undefined)}>
        <DialogContent className="max-w-image-open gap-3 p-3">
          <DialogTitle className="truncate">{opened?.name}</DialogTitle>
          {opened !== undefined && <img src={opened.address} alt={opened.name} className="mx-auto max-h-image-open-height max-w-full object-contain" />}
        </DialogContent>
      </Dialog>
    </div>
  )
}

// A reply, or a piece of one, drawn from its Markdown. Drawn again only
// when its text changes, so that a reply streaming in does not draw every
// reply above it again. A reply still streaming is drawn by its blocks, so
// that each piece reads again only the last block, which is still open,
// and not the whole reply so far. Once the turn has ended, the reply is the
// one stored, drawn from one reading of the whole.
const Markdown = memo(function Markdown({ text, streaming = false, className }: { text: string; streaming?: boolean; className: string }) {
  // The blocks the reply was cut into at its last piece, which the next cut
  // starts from. It is written while drawing, on purpose: it only saves
  // work, since each cut checks that the reply still starts with it, so a
  // draw that React throws away leaves nothing wrong behind.
  const cut = useRef<string[]>([])
  if (streaming) cut.current = markdownBlocks(text, cut.current)
  return (
    <div className={`text-prose break-words [&>*+*]:mt-3 ${className}`}>
      {streaming ? cut.current.map((block, at) => <MarkdownBlock key={at} text={block} />) : <MarkdownBlock text={text} />}
    </div>
  )
})

// Markdown drawn as it is, again only when it changes. Its elements are
// the box's own children, as if they were drawn with the blocks around them.
const MarkdownBlock = memo(function MarkdownBlock({ text }: { text: string }) {
  return renderMarkdown(text)
})

// Working, as the design system says it: the word, with a small spinner,
// on a row of the work log.
function Working({ words }: { words: string }) {
  return (
    <p className="flex min-h-work-row items-center gap-2 text-body text-status-working">
      <Spinner />
      {words}
    </p>
  )
}

// A caret that turns as what it opens opens.
function Caret({ open }: { open: boolean }) {
  return <CaretRight className={`size-icon-caret shrink-0 transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
}

// A completed turn's work before its final reply, as T3 Code folds it: one
// quiet line saying how long it worked, over a rule, which opens to it all
// in place, in order: evie's words, and each run of calls, which opens as
// it does outside a fold.
function Fold({ item, shown, show }: { item: Extract<Item, { kind: "fold" }> } & Shown) {
  const open = shown.has(item.key)
  return (
    <div className="flex flex-col">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => show([item.key], !open)}
        className="flex min-h-work-row items-center gap-1.5 border-b border-line pt-1 pb-2 text-left text-body text-text-muted hover:text-text"
      >
        {item.label}
        <Caret open={open} />
      </button>
      {open && (
        <div className="flex flex-col gap-2 pt-2">
          {item.items.map((inside) => (
            <div key={inside.key} className="flex flex-col">
              <Drawn item={inside} shown={shown} show={show} />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// The time now on the server's clock, again each second while `ticking`.
function useNow(ticking: boolean) {
  const [now, setNow] = useState(serverNow)
  useEffect(() => {
    if (!ticking) return
    setNow(serverNow())
    const timer = setInterval(() => setNow(serverNow()), 1000)
    return () => clearInterval(timer)
  }, [ticking])
  return now
}

// While a turn runs, under the conversation, as the Session view drawing
// has it, its one sign that evie is working: how long the turn has worked,
// counting up each second on the server's clock, as T3 Code's timer does,
// also while a question waits, and a quiet Stop, which says Stopping until
// the turn has ended. A turn begun before the page read says only Working,
// and a subagent's has no Stop.
function RunningLine({ since, stopping, stop }: { since: number | undefined; stopping: boolean; stop?: () => void }) {
  const now = useNow(since !== undefined && !stopping)
  return (
    <p className="flex items-center gap-2 text-caption text-text-muted tabular-nums">
      <span>{stopping ? "Stopping…" : since === undefined ? "Working" : workingFor(now - since)}</span>
      {stop !== undefined && !stopping && (
        <Button variant="quiet" onClick={stop} className="px-2 text-caption font-semibold text-text">
          Stop
        </Button>
      )}
    </p>
  )
}

// The turn running now between its steps, as T3 Code's work log says it:
// Thinking, with a light sweeping across it.
function ThinkingRow() {
  return (
    <p className="flex min-h-work-row min-w-0 items-center gap-1.5 text-body text-text-muted">
      <span className="flex w-work-row shrink-0 justify-center">
        <Lightbulb className="size-icon-sm" />
      </span>
      <span className="live-shine">Thinking</span>
    </p>
  )
}

// The icon of each kind of thing a call does, as T3 Code's work log has
// them, and the one for a run of calls of several kinds.
const KIND_ICON: Record<StepKind | "mixed", typeof FileText> = {
  read: FileText,
  edit: NotePencil,
  command: TerminalWindow,
  search: MagnifyingGlass,
  web: Globe,
  agent: Robot,
  question: ChatsCircle,
  other: Wrench,
  mixed: Hammer,
}

// One row of the work log, as T3 Code's: a 24px row with an icon for what
// was done and one sentence, cut short with an ellipsis, and a caret at
// its end that opens what is under it. While what it says is being done,
// a light sweeps across its words. A call that failed has a red ✕, and a
// row shows when it happened while the pointer is over it.
function WorkRow({ kind, label, live, open, toggle, failed = false, time }: { kind: StepKind | "mixed"; label: string; live: boolean; open: boolean; toggle(): void; failed?: boolean; time?: number }) {
  const Icon = KIND_ICON[kind]
  return (
    <button type="button" aria-expanded={open} onClick={toggle} className="group flex min-h-work-row w-full min-w-0 items-center gap-1.5 rounded-md text-left text-body text-text-muted hover:text-text">
      <span className="flex w-work-row shrink-0 justify-center">
        <Icon className="size-icon-sm" />
      </span>
      <span className={cn("min-w-0 flex-1 truncate", live && "live-shine")}>{label}</span>
      {time !== undefined && <time className="hidden shrink-0 text-caption tabular-nums group-hover:inline">{clockWords(time)}</time>}
      {failed && (
        <span className="flex shrink-0 text-status-failed">
          <X className="size-icon-xs" />
          <span className="sr-only">tool call failed</span>
        </span>
      )}
      <Caret open={open} />
    </button>
  )
}

// A time of day as a row shows it on hover, in the browser's own way, such as "14:05" or "2:05 PM".
const clockWords = (time: number) => new Date(time).toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })

// An opened run's rows, as T3 Code's: no taller than min(18rem, 50dvh),
// then they scroll, fading out at an edge with more past it.
function RunRows({ children }: { children: ReactNode }) {
  const list = useRef<HTMLOListElement>(null)
  const [edges, setEdges] = useState({ top: false, bottom: false })
  const measure = () => {
    const box = list.current
    if (!box) return
    const next = { top: box.scrollTop > 1, bottom: box.scrollHeight - box.scrollTop - box.clientHeight > 1 }
    setEdges((was) => (was.top === next.top && was.bottom === next.bottom ? was : next))
  }
  useLayoutEffect(measure)
  return (
    <ol ref={list} onScroll={measure} className={cn("flex max-h-work-group flex-col overflow-y-auto", edges.top && edges.bottom ? "fade-both" : edges.top ? "fade-top" : edges.bottom && "fade-bottom")}>
      {children}
    </ol>
  )
}

// A run of tool calls, as T3 Code's work log shows one: a run of one call
// as that call's row; a run of several as one row counting them, which
// opens to a row for each; and while one of them is carried out, one row
// saying what it is doing, which opens the same way. The first run of a
// turn that ended without being folded says how long the turn worked, as
// a fold does.
function WorkRun({ item, shown, show }: { item: Extract<Item, { kind: "steps" }> } & Shown) {
  const ids = item.calls.map((call) => `line:${call.id}`)
  const open = ids.some((id) => shown.has(id))
  const toggle = () => show(ids, !open)
  const callRow = (call: Call, label: string) => <CallRow key={call.id} call={call} label={label} open={shown.has(`call:${call.id}`)} toggle={() => show([`call:${call.id}`], !shown.has(`call:${call.id}`))} />
  // Opened under a row that says what the running call is doing, the call is listed by what it works on alone.
  const rows = <RunRows>{item.calls.map((call) => callRow(call, call.running ? call.target : call.sentence))}</RunRows>
  if (item.worked !== undefined) {
    return (
      <div className="flex flex-col">
        <button type="button" aria-expanded={open} onClick={toggle} className="flex min-h-work-row items-center gap-1.5 self-start text-body text-text-muted hover:text-text">
          {item.worked}
          <Caret open={open} />
        </button>
        {open && rows}
      </div>
    )
  }
  // A run of one call is its row, which says, while it runs, what the run says.
  if (item.calls.length === 1) return <ol className="flex flex-col">{callRow(item.calls[0]!, item.label)}</ol>
  const running = item.calls.find((call) => call.running)
  const kinds = new Set(item.calls.map((call) => call.kind))
  const kind = running?.kind ?? (kinds.size === 1 ? item.calls[0]!.kind : "mixed")
  return (
    <div className="flex flex-col">
      <WorkRow kind={kind} label={item.label} live={running !== undefined} open={open} toggle={toggle} />
      {open && rows}
    </div>
  )
}

// One tool call, on a row of the work log, which opens to its result as
// stored, under the sentence, as T3 Code's does: in code, and scrolling
// past a height.
function CallRow({ call, label, open, toggle }: { call: Call; label: string; open: boolean; toggle: () => void }) {
  return (
    <li className="flex min-w-0 flex-col">
      <WorkRow kind={call.kind} label={label} live={call.running} open={open} toggle={toggle} failed={call.failed} time={call.time} />
      {open &&
        (call.result !== undefined && call.result !== "" ? (
          <pre className="ml-7 max-h-call-result overflow-auto py-1 font-mono text-code break-words whitespace-pre-wrap text-text-muted">{call.result}</pre>
        ) : (
          <p className="ml-7 py-1 text-caption text-text-muted">{call.result === undefined ? (call.running ? "No result yet." : "No result.") : "It gave nothing back."}</p>
        ))}
    </li>
  )
}

// A note in quiet words, which opens to say more when it has more.
function Note({ text, more }: { text: string; more?: string }) {
  const [open, setOpen] = useState(false)
  if (more === undefined) return <p className="text-body break-words text-text-muted">{text}</p>
  return (
    <div className="flex flex-col">
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="flex min-h-work-row items-center gap-2 self-start text-left text-body text-text-muted hover:text-text">
        <span className="break-words">{text}</span>
        <Caret open={open} />
      </button>
      {open && (
        <div className="border-l border-line pl-3">
          <Markdown text={more} className="text-text-muted" />
        </div>
      )}
    </div>
  )
}
