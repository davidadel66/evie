import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"
import { PermissionPrompt, useAsking } from "./Asking.tsx"
import { TurnChangedFiles } from "./ChangedFiles.tsx"
import { holdChanges, type Changes } from "./changes.ts"
import { asAsked } from "./questions.ts"
import { Button } from "./components/ui/button.tsx"
import { Composer } from "./Composer.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, openDraft, type Conversation as Opened, type Draft as OpenedDraft } from "./conversation.ts"
import type { ShownAttachment } from "./attachments.ts"
import { Dialog, DialogContent, DialogTitle } from "./components/ui/dialog.tsx"
import { CaretRight, File as FileIcon } from "./icons.tsx"
import { renderMarkdown } from "./markdown.ts"
import { addressOf } from "./address.ts"
import type { Sessions } from "./sessions.ts"
import { Spinner } from "./Spinner.tsx"
import { transcriptOf, type Call, type Item } from "./transcript.ts"
import { sizeWords } from "./workspace.ts"

// The conversation of the session selected, in the middle column: its
// latest page, with earlier pages read as the owner scrolls up to them,
// and what happens in it as it happens. The owner who is at the bottom is
// kept there as new text comes; one who has scrolled up is left where they
// are, and offered a way back down.
export function Conversation({ connection, session }: { connection: Connection; session: string }) {
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
  return <Reading key={session} conversation={opened.conversation} changes={opened.changes} />
}

// How near the bottom, in pixels, counts as at the bottom, so that a
// reply's last line wrapping does not count as scrolling up.
const AT_BOTTOM = 32
// How many of the first items are remembered to keep the owner's place.
const ANCHORS = 5

// A conversation as it reads, the session's own, with its message box, or
// one of its subagents', which is read only: it has no box, since nothing
// is said in a subagent's conversation from the page. With `changes`, each
// completed turn that changed files ends with them.
export function Reading({ conversation, readOnly = false, changes }: { conversation: Opened; readOnly?: boolean; changes?: Changes }) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const { events, earlier, replySoFar, turnRunning, jobs, pending, notes } = state
  const changed = useSyncExternalStore(changes?.subscribe ?? noChanges.subscribe, changes?.state ?? noChanges.state).listed?.turns
  // A question waiting is shown where it is answered, at the end or docked
  // above the box, so it is left out where it was asked until it is answered.
  const items = useMemo(
    () => asAsked(transcriptOf({ events, earlier, replySoFar, turnRunning, jobs, pending, notes }, changed), state.waiting),
    [events, earlier, replySoFar, turnRunning, jobs, pending, notes, changed, state.waiting],
  )
  const asking = useAsking(state.waiting)
  const { shown } = asking

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
          <div ref={content} className="px-7 py-8">
            <div className="mx-auto flex max-w-conversation flex-col">
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
              {state.opened && state.problem === undefined && items.length === 0 && <p className="text-body text-text-muted">Nothing has been said in this session yet.</p>}
              {items.map((item, at) => (
                <div key={item.key} data-item={item.key} className={`flex flex-col ${at === 0 ? "" : item.turn !== items[at - 1]!.turn ? "mt-6" : "mt-4"}`}>
                  <Drawn item={item} shown={opens} show={show} />
                </div>
              ))}
              {state.running.filter((name) => name !== "compact").map((name, at) => (
                <div key={`${name}${at}`} className={items.length > 0 || at > 0 ? "mt-4" : ""}>
                  <Working words={`Running /${name}`} />
                </div>
              ))}
              {shown?.place === "inline" && (
                <div data-item="permission" className={items.length > 0 ? "mt-6" : ""}>
                  <PermissionPrompt question={shown.question} count={shown.count} answer={conversation.answer} redirect />
                </div>
              )}
            </div>
          </div>
        </div>
        {scrolledUp && (
          <div className="pointer-events-none absolute inset-x-0 bottom-4 flex justify-center">
            <Button variant="quiet" onClick={toLatest} className="pointer-events-auto rounded-full bg-surface px-4 shadow-float">
              Jump to the latest
            </Button>
          </div>
        )}
      </div>
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
        />
      )}
    </div>
  )
}

// For a conversation shown without its changes, as a subagent's is: the
// same state each time it is asked for, as React needs.
const NONE_LISTED = { listed: undefined, problem: undefined }
const noChanges = { subscribe: () => () => {}, state: () => NONE_LISTED }

// A draft of a new session, in the middle column: where it will start, and
// the message box, whose first message starts it. While it starts, the
// message shows as the owner's, and a question asked as the session opens,
// such as whether to trust its project, is answered in place. Once its
// conversation has been read, the page moves to the session, or does what
// `moved` says instead, as a workspace's side chat does.
export function Draft({
  connection,
  sessions,
  project,
  where,
  moved,
}: {
  connection: Connection
  sessions: Sessions
  project: string | null
  where: string
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
  return <Drafting key={key} draft={opened.draft} where={where} moved={moved ?? ((session) => location.replace(addressOf({ session })))} inProject={project !== null} />
}

function Drafting({ draft, where, moved, inProject }: { draft: OpenedDraft; where: string; moved(session: string): void; inProject: boolean }) {
  const state = useSyncExternalStore(draft.subscribe, draft.state)
  const asking = useAsking(state.waiting)
  const { shown } = asking
  async function start(text: string, files: readonly File[]) {
    if (state.starting !== undefined) return "The session is starting. Send this once it has."
    const started = await draft.start(text, moved, files)
    return "problem" in started ? started.problem : undefined
  }
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="min-h-0 flex-1 overflow-y-auto">
        {state.starting === undefined ? (
          <Placeholder title="New session" text={`A new session ${where} starts when its first message is sent.`} />
        ) : (
          <div className="px-7 py-8">
            <div className="mx-auto flex max-w-conversation flex-col gap-4">
              <Drawn item={{ key: "starting", turn: 0, kind: "owner", text: state.starting }} shown={new Set()} show={() => {}} />
              {shown?.place === "inline" ? <PermissionPrompt question={shown.question} count={shown.count} answer={draft.answer} redirect={false} /> : <Working words="Starting the session" />}
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
      />
    </div>
  )
}

// The middle column with nothing to read in it: a title and a line.
export function Placeholder({ title, text }: { title: string; text: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 px-7 py-8 text-center">
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
          {item.text !== "" && <p className="max-w-4/5 self-end rounded-lg bg-surface-raised px-4 py-2 text-body break-words whitespace-pre-wrap text-text">{item.text}</p>}
          {item.attachments !== undefined && <Attachments attachments={item.attachments} spaced={item.text !== ""} />}
        </>
      )
    case "reply":
      return <Markdown text={item.text} className="text-text" />
    case "steps":
      return <StepsLine calls={item.calls} label={item.label} running={item.running} shown={shown} show={show} />
    case "fold":
      return <Fold item={item} shown={shown} show={show} />
    case "note":
      return <Note text={item.text} {...(item.more !== undefined && { more: item.more })} />
    // A question as it was asked. One waiting is drawn where it is answered.
    case "question":
      return <p className="text-label break-words whitespace-pre-wrap text-text">{item.text}</p>
    case "changes":
      return <TurnChangedFiles turn={item.number} files={item.files} />
    case "failed":
      return (
        <div className="text-caption">
          <p className="text-status-failed">Failed</p>
          <p className="break-words text-text-muted">{item.error}</p>
        </div>
      )
  }
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
          <DialogTitle className="truncate text-caption text-text-muted">{opened?.name}</DialogTitle>
          {opened !== undefined && <img src={opened.address} alt={opened.name} className="mx-auto max-h-image-open-height max-w-full object-contain" />}
        </DialogContent>
      </Dialog>
    </div>
  )
}

// A reply, or a piece of one, drawn from its Markdown. Drawn again only
// when its text changes, so that a reply streaming in does not draw every
// reply above it again.
const Markdown = memo(function Markdown({ text, className }: { text: string; className: string }) {
  const drawn = useMemo(() => renderMarkdown(text), [text])
  return <div className={`text-body break-words [&>*+*]:mt-3 ${className}`}>{drawn}</div>
})

// Working, as the design system says it: the word, with a small spinner.
function Working({ words }: { words: string }) {
  return (
    <p className="flex min-h-control items-center gap-2 text-body text-status-working">
      <Spinner />
      {words}
    </p>
  )
}

// A caret that turns as what it opens opens.
function Caret({ open }: { open: boolean }) {
  return <CaretRight className={`size-icon-sm shrink-0 transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
}

// A completed turn's work before its final reply, as one quiet line that
// opens to it all, in order: evie's words, and each run of calls, which
// opens as it does outside a fold.
function Fold({ item, shown, show }: { item: Extract<Item, { kind: "fold" }> } & Shown) {
  const open = shown.has(item.key)
  return (
    <div className="flex flex-col">
      <button type="button" aria-expanded={open} onClick={() => show([item.key], !open)} className="flex min-h-control items-center gap-2 self-start text-body text-text-muted hover:text-text">
        {item.label}
        <Caret open={open} />
      </button>
      {open && (
        <div className="flex flex-col gap-4 border-l border-line pt-2 pb-2 pl-3">
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

// A run of tool calls as one quiet line, which opens to list each call.
function StepsLine({ calls, label, running, shown, show }: { calls: Call[]; label: string; running: boolean } & Shown) {
  if (calls.length === 0) return <Working words={label} />
  const ids = calls.map((call) => `line:${call.id}`)
  const open = ids.some((id) => shown.has(id))
  return (
    <div className="flex flex-col">
      <button type="button" aria-expanded={open} onClick={() => show(ids, !open)} className={`flex min-h-control items-center gap-2 self-start text-body hover:text-text ${running ? "text-status-working" : "text-text-muted"}`}>
        {running && <Spinner />}
        {label}
        <Caret open={open} />
      </button>
      {open && (
        <ol className="flex flex-col border-l border-line pl-3">
          {calls.map((call) => (
            <CallRow key={call.id} call={call} open={shown.has(`call:${call.id}`)} toggle={() => show([`call:${call.id}`], !shown.has(`call:${call.id}`))} />
          ))}
        </ol>
      )}
    </div>
  )
}

// One tool call: its name, a short form of its input, and its result in
// short, which opens to its result as stored.
function CallRow({ call, open, toggle }: { call: Call; open: boolean; toggle: () => void }) {
  return (
    <li className="flex min-w-0 flex-col">
      <button type="button" aria-expanded={open} onClick={toggle} className="flex min-h-control w-full min-w-0 items-center py-1 text-left">
        <span className="flex w-full min-w-0 items-start gap-3">
          <span className="min-w-call-name shrink-0 font-mono text-code text-text-muted">{call.name}</span>
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate font-mono text-code text-text">{call.input}</span>
            {call.short !== undefined && call.short !== "" && <span className="truncate text-caption text-text-muted">{call.short}</span>}
          </span>
          {call.running && (
            <span className="flex shrink-0 items-center gap-2 text-caption text-status-working">
              <Spinner />
              Working
            </span>
          )}
        </span>
      </button>
      {open &&
        (call.result !== undefined ? (
          <pre className="pb-3 font-mono text-code break-words whitespace-pre-wrap text-text-muted">{call.result}</pre>
        ) : (
          <p className="pb-3 text-caption text-text-muted">{call.running ? "No result yet." : "No result."}</p>
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
      <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="flex min-h-control items-center gap-2 self-start text-left text-body text-text-muted hover:text-text">
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
