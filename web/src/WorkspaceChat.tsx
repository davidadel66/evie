import { memo, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject } from "../../src/core/types.ts"
import { useAsking } from "./Asking.tsx"
import { Button, buttonVariants } from "./components/ui/button.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, openDraft, startedHere, type Conversation, type Draft } from "./conversation.ts"
import { ArrowsOut, ArrowUpBold, Plus, X } from "./icons.tsx"
import { renderMarkdown } from "./markdown.ts"
import { sendDoes, type Asked } from "./questions.ts"
import type { SessionList, Sessions } from "./sessions.ts"
import { onMac, pressed } from "./shortcuts.ts"
import { useWide } from "./wide.ts"
import { addressOfWorkspace, chatAddress, chatShown, latestSession, NEW_CHAT, onChatShown, setChatShown, type WorkspacePlace } from "./workspace.ts"
import { chatTitle, fullSize, givenBackWords, sideChatOf, type ChatLine } from "./workspace-chat.ts"

// The workspace's side chat: a small chat with evie about the workspace,
// beside it. Its header says "evie" over the chat's title, with the ways to
// open it full size in the session view and to close it. Under it, the
// conversation in short, a question waiting asked at its end, and a small
// box to write in. It shows the session the address keeps, or until one is
// kept, the workspace's latest, or a new chat in its project when it has
// none: kept in the address, so that another session becoming the latest
// does not change it. New chat in its header starts a new one, as T3 Code
// keeps New thread one click away. Hidden, it stays on the page, so that a
// chat starting goes on starting.

// Whether the side chat is shown, redrawn as it changes.
export const useChatShown = () => useSyncExternalStore(onChatShown, chatShown)

export function SideChat({ project, place, list, connection, sessions }: { project: ListedProject; place: WorkspacePlace; list: SessionList; connection: Connection; sessions: Sessions }) {
  const latest = latestSession(list.sessions, project.id)?.session
  useEffect(() => {
    if (place.chat !== undefined || !list.listed) return
    location.replace(addressOfWorkspace({ ...place, chat: latest ?? NEW_CHAT }))
  }, [place, list.listed, latest])
  const chat = place.chat
  if (chat === undefined) return <ChatHeader title="" />
  const fresh = addressOfWorkspace({ ...place, chat: NEW_CHAT })
  if (chat === NEW_CHAT) {
    return (
      <>
        <ChatHeader title={chatTitle(undefined, project.name)} full={fullSize(chat, project.id)} />
        <ChatDraft connection={connection} sessions={sessions} project={project.id} />
      </>
    )
  }
  const session = list.sessions.find((each) => each.session === chat)
  if (session === undefined && startedHere(chat) === undefined) {
    return (
      <>
        <ChatHeader title="" fresh={fresh} />
        {list.listed && <p className="px-gutter py-4 text-caption text-text-muted">This chat is not in the list. It may have been deleted. New chat, above, starts another.</p>}
      </>
    )
  }
  return (
    <>
      <ChatHeader title={chatTitle(session, project.name)} fresh={fresh} full={fullSize(chat, project.id)} />
      <ChatSession key={chat} connection={connection} session={chat} />
    </>
  )
}

function ChatHeader({ title, fresh, full }: { title: string; fresh?: string; full?: string }) {
  return (
    <header className="flex h-header-height shrink-0 items-center gap-1 border-b border-line pr-2 pl-gutter">
      <div className="flex min-w-0 flex-1 flex-col">
        <span className="text-label text-text">evie</span>
        <span className="truncate text-caption text-text-muted">{title}</span>
      </div>
      {fresh !== undefined && (
        <a href={fresh} aria-label="New chat" title="New chat" className={buttonVariants({ variant: "icon" })}>
          <Plus />
        </a>
      )}
      {full !== undefined && (
        <a href={full} aria-label="Open this chat full size" title="Open this chat full size" className={buttonVariants({ variant: "icon" })}>
          <ArrowsOut />
        </a>
      )}
      <Button variant="icon" aria-label="Close the chat" title="Close the chat" onClick={() => setChatShown(false)}>
        <X />
      </Button>
    </header>
  )
}

// A session's chat, with its conversation held while it shows.
function ChatSession({ connection, session }: { connection: Connection; session: string }) {
  const [opened, setOpened] = useState<{ session: string; conversation: Conversation }>()
  useEffect(() => {
    const held = holdConversation(connection, session)
    setOpened({ session, conversation: held.conversation })
    return () => held.release()
  }, [connection, session])
  if (opened?.session !== session) return null
  return <Talking conversation={opened.conversation} />
}

function Talking({ conversation }: { conversation: Conversation }) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const lines = useMemo(() => sideChatOf(state), [state])
  const asking = useAsking(state.waiting)
  const { shown } = asking
  // A message that could not be sent on the box's behalf, as a new chat's
  // first can be, comes back into the box, with why.
  const [given, setGiven] = useState<{ text: string; problem: string }>()
  useEffect(() => {
    if (state.givenBack === undefined) return
    const back = conversation.takeGivenBack()
    if (back !== undefined) setGiven({ text: back.text, problem: givenBackWords(back) })
  }, [state.givenBack, conversation])

  // What sending does, as the full message box does it: words answer the
  // question waiting, or refuse a permission and go to evie as what to do
  // instead; a slash and a word runs a command; anything else is a message,
  // which steers a turn in progress.
  async function send(text: string): Promise<string | undefined> {
    const does = sendDoes(text, shown?.question)
    if (does === undefined) return undefined
    if ("refuse" in does) return does.refuse
    if ("answer" in does) return conversation.answer(shown!.question.id, does.answer)
    if ("decline" in does) return (await conversation.answer(shown!.question.id, does.decline)) ?? conversation.send(does.message)
    if ("run" in does) return void conversation.run(does.run.name, does.run.args)
    return conversation.send(does.message)
  }

  return (
    <>
      <Lines lines={lines} readEarlier={() => conversation.readEarlier()}>
        {state.readingEarlier && <Step text="Reading the earlier chat" />}
        {state.problem !== undefined ? (
          <div className="flex flex-col items-start">
            <p className="text-caption break-words text-text-muted">{state.problem}</p>
            <Button variant="quiet" onClick={() => conversation.tryAgain()} className="-ml-3 text-caption">
              Try again
            </Button>
          </div>
        ) : !state.opened ? (
          <Step text="Opening the chat" />
        ) : shown !== undefined ? (
          <Question question={shown.question} count={shown.count} answer={conversation.answer} />
        ) : (
          state.turnRunning && <WorkingLine stopping={state.stopping} stop={() => conversation.stop()} />
        )}
      </Lines>
      <ChatBox send={send} asking={asking} given={given} {...(state.turnRunning && !state.stopping && { stop: () => conversation.stop() })} />
    </>
  )
}

// A new chat, in the workspace's project: empty until its first message
// starts its session. While it starts, the message shows as the owner's,
// and a question asked as the session opens, such as whether to trust its
// project, is answered in place. Once started, the side chat shows it.
function ChatDraft({ connection, sessions, project }: { connection: Connection; sessions: Sessions; project: string }) {
  const [opened, setOpened] = useState<{ project: string; draft: Draft }>()
  useEffect(() => {
    const draft = openDraft(connection, project, { listed: (session) => sessions.state().sessions.some((each) => each.session === session) })
    setOpened({ project, draft })
    return () => draft.close()
  }, [connection, sessions, project])
  if (opened?.project !== project) return null
  return <Drafting key={project} draft={opened.draft} project={project} />
}

function Drafting({ draft, project }: { draft: Draft; project: string }) {
  const state = useSyncExternalStore(draft.subscribe, draft.state)
  const asking = useAsking(state.waiting)
  const { shown } = asking
  // As the address is when the session has started, which the owner may have changed meanwhile.
  const moved = (session: string) => {
    const there = chatAddress(location.hash, project, session)
    if (there !== undefined) location.replace(there)
  }
  async function send(text: string): Promise<string | undefined> {
    const does = sendDoes(text, shown?.question)
    if (does === undefined) return undefined
    if ("refuse" in does) return does.refuse
    if ("answer" in does) return draft.answer(shown!.question.id, does.answer)
    if ("run" in does) return "Commands can be run once the chat has started. Send a message first."
    if ("decline" in does) {
      const refused = await draft.answer(shown!.question.id, does.decline)
      if (refused !== undefined) return refused
    }
    if (state.starting !== undefined) return "The chat is starting. Send this once it has."
    const started = await draft.start(does.message, moved)
    return "problem" in started ? started.problem : undefined
  }
  const lines: ChatLine[] = state.starting === undefined ? [] : [{ key: "starting", kind: "owner", text: state.starting, files: [] }]
  return (
    <>
      <Lines lines={lines}>
        {shown !== undefined ? <Question question={shown.question} count={shown.count} answer={draft.answer} /> : state.starting !== undefined && <Step text="Starting the chat" />}
      </Lines>
      <ChatBox send={send} asking={asking} />
    </>
  )
}

// How near the bottom, in pixels, counts as at the bottom.
const AT_BOTTOM = 32
// How many of the first lines are remembered to keep the owner's place.
const ANCHORS = 5

// The chat's lines, then what ends them, kept at the bottom as they come
// while the owner is there; one who has scrolled up is left where they
// are. Within half a screen of the top, the page before is read, with
// `readEarlier`, and the owner's place is kept as it joins above, as the
// full conversation keeps it.
function Lines({ lines, readEarlier, children }: { lines: ChatLine[]; readEarlier?: () => void; children?: ReactNode }) {
  const scroller = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  const stuck = useRef(true)
  // Where the first few lines were as last drawn.
  const drawn = useRef<{ key: string; top: number }[]>([])
  const readIfNearTop = () => {
    const box = scroller.current
    if (box && readEarlier && box.scrollTop < box.clientHeight / 2) readEarlier()
  }
  // After each change, before it is shown: kept at the bottom, or moved by
  // as far as the first of the lines drawn last time that is still there.
  useLayoutEffect(() => {
    const box = scroller.current
    if (!box) return
    const placed = (key: string) => box.querySelector<HTMLElement>(`[data-line="${CSS.escape(key)}"]`)
    if (stuck.current) box.scrollTop = box.scrollHeight
    else {
      const anchor = drawn.current.find((each) => placed(each.key) !== null)
      if (anchor) box.scrollTop += placed(anchor.key)!.offsetTop - anchor.top
    }
    drawn.current = [...box.querySelectorAll<HTMLElement>("[data-line]")].slice(0, ANCHORS).map((each) => ({ key: each.dataset.line!, top: each.offsetTop }))
    readIfNearTop()
  })
  // What grows by itself, as a reply rewrapping when the window is resized.
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
    // The browser's own anchoring is off, since this keeps the owner's place itself.
    <div
      ref={scroller}
      onScroll={() => {
        const box = scroller.current!
        stuck.current = box.scrollHeight - box.scrollTop - box.clientHeight <= AT_BOTTOM
        readIfNearTop()
      }}
      className="min-h-0 flex-1 overflow-y-auto [overflow-anchor:none]"
    >
      <div ref={content} className="relative flex flex-col gap-3 px-gutter py-4">
        {lines.map((line) => (
          <div key={line.key} data-line={line.key} className="flex flex-col">
            <Line line={line} />
          </div>
        ))}
        {children}
      </div>
    </div>
  )
}

function Line({ line }: { line: ChatLine }) {
  switch (line.kind) {
    case "owner":
      return (
        <div className="flex max-w-7/8 flex-col items-end gap-1 self-end">
          {line.text !== "" && <p className="rounded-lg bg-surface-raised px-3 py-2 text-body break-words whitespace-pre-wrap text-text">{line.text}</p>}
          {line.files.length > 0 && <p className="text-caption break-words text-text-muted">{line.files.join(", ")}</p>}
        </div>
      )
    case "text":
      return <Said text={line.text} />
    case "step":
      return <Step text={line.text} />
    case "failed":
      return (
        <p className="text-caption break-words">
          <span className="text-status-failed">Failed</span> <span className="text-text-muted">{line.text}</span>
        </p>
      )
  }
}

// evie's words, drawn from their Markdown, again only when they change.
const Said = memo(function Said({ text }: { text: string }) {
  const drawn = useMemo(() => renderMarkdown(text), [text])
  return <div className="text-body break-words text-text [&>*+*]:mt-3">{drawn}</div>
})

// One quiet line: a run of evie's work, a note, or what the chat is doing.
function Step({ text }: { text: string }) {
  return <p className="text-caption break-words text-text-muted">{text}</p>
}

// While evie works, a quiet line that says so, with Stop.
function WorkingLine({ stopping, stop }: { stopping: boolean; stop(): void }) {
  return (
    <div className="flex items-center gap-2 text-caption text-text-muted">
      <span>{stopping ? "Stopping" : "Working"}</span>
      {!stopping && (
        <Button variant="quiet" onClick={stop} className="h-control-sm px-2 text-caption font-semibold text-text">
          Stop
        </Button>
      )}
    </div>
  )
}

// A question waiting, at the end of the chat, as part of evie's reply: its
// question in bold, with what follows, such as a command, as code; its
// choices under it, the first the main one; and that it needs the owner.
function Question({ question, count, answer }: { question: Asked; count: string | undefined; answer(id: string, choice: string | null): Promise<string | undefined> }) {
  const [refused, setRefused] = useState<string>()
  useEffect(() => setRefused(undefined), [question.id])
  const [asked = "", ...rest] = question.text.split(/\n\s*\n/)
  const detail = rest.join("\n\n").replace(/^ {2}/gm, "")
  return (
    <section aria-label="Question from evie" className="flex flex-col gap-3">
      <p className="text-label break-words whitespace-pre-wrap text-text">{asked}</p>
      {detail.trim() !== "" && <pre className="font-mono text-code break-words whitespace-pre-wrap text-text">{detail}</pre>}
      <div className="flex flex-wrap gap-2">
        {question.choices.map((choice, at) => (
          <Button
            key={choice}
            variant={at === 0 ? "primary" : "secondary"}
            onClick={async () => setRefused(await answer(question.id, choice))}
            className="h-auto min-h-control max-w-full shrink rounded-full py-1 text-left whitespace-normal"
          >
            {choice}
          </Button>
        ))}
      </div>
      <p className="flex items-center gap-2 text-caption text-status-needs">
        <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-status-needs" />
        Needs you. Waiting for your answer{count !== undefined && ` (${count})`}
      </p>
      {refused !== undefined && (
        <p role="status" className="text-caption break-words text-text-muted">
          {refused}
        </p>
      )}
    </section>
  )
}

// The small box: the text and a round send button in one row. Enter sends
// and Shift+Enter starts a new line; Escape stops evie while it works. What
// could not be sent goes back into the box, with why above it.
function ChatBox({ send, asking, given, stop }: { send(text: string): Promise<string | undefined>; asking: { typed(): void; cleared(): void }; given?: { text: string; problem: string } | undefined; stop?: () => void }) {
  const [text, setText] = useState("")
  const [problem, setProblem] = useState<string>()
  const box = useRef<HTMLTextAreaElement>(null)
  const wide = useWide()
  // Focused as the chat opens, where there is a keyboard to type with and
  // the box is in sight, as it is not when it is stacked below the
  // workspace, which the page is not moved away from.
  useEffect(() => {
    const area = box.current
    if (!wide || area === null) return
    const at = area.getBoundingClientRect()
    if (at.height > 0 && at.bottom <= innerHeight) area.focus({ preventScroll: true })
  }, [])
  useEffect(() => {
    if (given === undefined) return
    setText((now) => (now === "" ? given.text : now))
    setProblem(given.problem)
  }, [given])
  // It grows with its text, from two lines, up to a height, then scrolls.
  // While the chat is hidden it has no height to measure, and keeps its
  // two lines.
  useLayoutEffect(() => {
    const area = box.current
    if (!area) return
    area.style.height = "auto"
    if (area.scrollHeight > 0) area.style.height = `${area.scrollHeight}px`
  }, [text])

  async function submit() {
    const was = text
    if (was.trim() === "") return
    asking.cleared()
    setProblem(undefined)
    setText("")
    const refused = await send(was)
    if (refused === undefined) return
    setProblem(refused)
    setText((now) => (now === "" ? was : now))
  }

  return (
    <div className="shrink-0 p-3">
      {problem !== undefined && (
        <p role="status" className="px-3 pb-2 text-caption break-words text-text-muted">
          {problem}
        </p>
      )}
      <form
        onSubmit={(event) => {
          event.preventDefault()
          void submit()
        }}
        className="flex items-end gap-2 rounded-lg border border-line bg-surface pt-3 pr-2 pb-2 pl-3"
      >
        <label className="flex min-w-0 flex-1 self-center">
          <span className="sr-only">Message evie about this workspace</span>
          <textarea
            ref={box}
            rows={2}
            value={text}
            placeholder="Ask evie to change something here"
            onChange={(event) => {
              setText(event.target.value)
              setProblem(undefined)
              asking.typed()
            }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing) return
              if (event.key === "Enter" && !event.shiftKey) {
                event.preventDefault()
                void submit()
              } else if (stop !== undefined && pressed(event.nativeEvent, onMac()) === "stop") {
                event.preventDefault()
                stop()
              }
            }}
            onBlur={() => asking.cleared()}
            className="max-h-composer-text w-full resize-none overflow-y-auto bg-transparent text-field text-text outline-none placeholder:text-text-muted"
          />
        </label>
        {/* Filled at all times, as drawn; with nothing to send it is dimmed. */}
        <Button type="submit" variant="primary" aria-label="Send" disabled={text.trim() === ""} className="size-control rounded-full p-0 disabled:bg-text disabled:text-bg disabled:opacity-50">
          <ArrowUpBold className="size-icon-sm" />
        </Button>
      </form>
    </div>
  )
}
