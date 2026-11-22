import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"
import { Button } from "./components/ui/button.tsx"
import type { Connection } from "./connection.ts"
import { openConversation, type Conversation as Opened } from "./conversation.ts"
import { CaretRight } from "./icons.tsx"
import { renderMarkdown } from "./markdown.ts"
import { Spinner } from "./Spinner.tsx"
import { transcriptOf, type Call, type Item } from "./transcript.ts"

// The conversation of the session selected, in the middle column: its
// latest page, with earlier pages read as the owner scrolls up to them,
// and what happens in it as it happens. The owner who is at the bottom is
// kept there as new text comes; one who has scrolled up is left where they
// are, and offered a way back down.
export function Conversation({ connection, session }: { connection: Connection; session: string }) {
  const [opened, setOpened] = useState<{ session: string; conversation: Opened }>()
  useEffect(() => {
    const conversation = openConversation(connection, session)
    setOpened({ session, conversation })
    return () => conversation.close()
  }, [connection, session])
  if (opened?.session !== session) return null
  return <Reading key={session} conversation={opened.conversation} />
}

// How near the bottom, in pixels, counts as at the bottom, so that a
// reply's last line wrapping does not count as scrolling up.
const AT_BOTTOM = 32
// How many of the first items are remembered to keep the owner's place.
const ANCHORS = 5

function Reading({ conversation }: { conversation: Opened }) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const { events, earlier, replySoFar, turnRunning, jobs } = state
  const items = useMemo(() => transcriptOf({ events, earlier, replySoFar, turnRunning, jobs }), [events, earlier, replySoFar, turnRunning, jobs])

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
  const [shown, setShown] = useState<ReadonlySet<string>>(new Set())
  const show = (ids: string[], open: boolean) =>
    setShown((was) => {
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
                <Drawn item={item} shown={shown} show={show} />
              </div>
            ))}
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
      {/* The message box goes here, below the conversation. */}
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
      return <p className="max-w-4/5 self-end rounded-lg bg-surface-raised px-4 py-2 text-body break-words whitespace-pre-wrap text-text">{item.text}</p>
    case "reply":
      return <Markdown text={item.text} className="text-text" />
    case "steps":
      return <StepsLine calls={item.calls} label={item.label} running={item.running} shown={shown} show={show} />
    case "note":
      return <Note text={item.text} {...(item.more !== undefined && { more: item.more })} />
    case "question":
      return (
        <div className="flex flex-col gap-2">
          <p className="text-label text-text">{item.text}</p>
          {item.waiting && (
            <>
              <ul className="list-disc pl-6 text-body text-text-muted">
                {item.choices.map((choice) => (
                  <li key={choice}>{choice}</li>
                ))}
              </ul>
              <p className="flex items-center gap-2 text-caption text-text-muted">
                <span aria-hidden="true" className="size-2 rounded-full bg-status-needs" />
                Waiting for your answer
              </p>
            </>
          )}
        </div>
      )
    case "failed":
      return (
        <div className="text-caption">
          <p className="text-status-failed">Failed</p>
          <p className="break-words text-text-muted">{item.error}</p>
        </div>
      )
  }
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
