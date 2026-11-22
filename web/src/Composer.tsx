import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent as KeyEvent } from "react"
import type { Shown } from "./Asking.tsx"
import { DockedQuestion } from "./Asking.tsx"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { attachable, contextMeter, earlierMessages, mentionAt, menuKey, putMention, rankCommands, rankFiles, recallStep, slashAt, type MenuItem, type Recalled } from "./composer.ts"
import type { Conversation, ConversationState } from "./conversation.ts"
import { ArrowUpBold, Check, File as FileIcon, Paperclip, StopFill, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { makersOf, searchModels, type Model } from "./models.ts"
import { sendWithFiles } from "./questions.ts"
import { Spinner } from "./Spinner.tsx"
import { sizeWords } from "./workspace.ts"

// The message box at the bottom of the conversation, to the board's
// Composer, working as T3 Code's does: Enter sends and Shift and Enter
// starts a new line; Up in an empty box brings back the owner's earlier
// messages; a slash opens the menu of commands and skills; /model, or
// mod+shift+M, opens the model picker. While evie works, what is sent
// steers the turn, and Stop, or Escape, stops it. Under the text, the
// model's name and how full the conversation is, whose menu compacts it.
// At phone width it is one row until it is tapped. An @ opens the menu of
// the project's files, and files are attached with the paperclip, by
// pasting or by dropping them on the box, as T3 Code's are: each is a chip
// above the text until it is sent.

// The model under the text, which /model picks: the one in use, the models
// to pick from, and the picking, which gives back why not, or nothing.
export type ModelChoice = { current: string | undefined; list(): Promise<Model[]>; pick(model: string): Promise<string | undefined> }

type Props = {
  // The session's conversation and how it stands. A draft has neither: it
  // runs no commands but /model until its session has started.
  conversation?: Conversation | undefined
  state?: ConversationState | undefined
  model: ModelChoice
  // The question showing, which what is sent may answer, and how to answer it.
  shown: Shown
  answer(id: string, choice: string | null): Promise<string | undefined>
  // Tells the guard on questions that a key was typed, or that the owner
  // sent or left the box.
  asking: { typed(): void; cleared(): void }
  // Sends a message, with the files attached to it. Gives back why it
  // could not be sent, when the text and the files go back into the box.
  send(text: string, files: readonly File[]): Promise<string | undefined>
  // The files of the session's project, which an @ picks from. A session
  // with no project has none, and no @ menu.
  mentions?: (() => Promise<{ files: string[]; more: boolean }>) | undefined
}

// A file attached in the box, until it is sent.
type Attached = { key: number; file: File }
let attachedCount = 0

// The model, as the menu lists it: picking it opens the picker, as in T3 Code.
const MODEL: MenuItem = { name: "model", description: "Pick the model this session uses" }

export function Composer({ conversation, state, model, shown, answer, asking, send, mentions }: Props) {
  const [text, setText] = useState("")
  const [caret, setCaret] = useState(0)
  const [problem, setProblem] = useState<string>()
  const [recalled, setRecalled] = useState<Recalled>()
  // The text at which Escape closed the menu, which stays closed until the text changes.
  const [closedAt, setClosedAt] = useState<string>()
  const [highlight, setHighlight] = useState<number>()
  const [commands, setCommands] = useState<{ items?: MenuItem[]; problem?: string }>()
  const [picking, setPicking] = useState(false)
  const [focused, setFocused] = useState(false)
  const [attached, setAttached] = useState<Attached[]>([])
  const [dragging, setDragging] = useState(false)
  const [listedFiles, setListedFiles] = useState<{ files?: string[]; more?: boolean; problem?: string }>()
  const wide = useWide()
  const box = useRef<HTMLTextAreaElement>(null)
  const chooser = useRef<HTMLInputElement>(null)
  // Where the caret goes once the text set has been drawn.
  const caretTo = useRef<number>(undefined)

  const slash = closedAt === text ? undefined : slashAt(text, caret)
  const menuOpen = slash !== undefined && !picking
  const working = state?.turnRunning === true
  const mention = closedAt === text || mentions === undefined || picking ? undefined : mentionAt(text, caret)
  const filesOpen = mention !== undefined

  // The commands are listed again each time the menu opens, since a skill
  // written a moment ago can be started at once; meanwhile the menu shows
  // those listed last time.
  useEffect(() => {
    setHighlight(undefined)
    if (!menuOpen || conversation === undefined) return
    let current = true
    conversation.listCommands().then(
      (items) => current && setCommands({ items }),
      (cause: unknown) => current && setCommands((was) => (was?.items !== undefined ? was : { problem: cause instanceof Error ? cause.message : String(cause) })),
    )
    return () => void (current = false)
  }, [menuOpen, conversation])
  // At the start of the message every command can be run; elsewhere, and
  // in a draft, only the model picked.
  const items = useMemo(() => {
    if (slash === undefined) return []
    if (conversation === undefined || !slash.first) return rankCommands([MODEL], slash.query)
    if (commands?.items === undefined) return []
    return rankCommands([MODEL, ...commands.items.filter((item) => item.name !== MODEL.name)], slash.query)
  }, [slash?.query, slash?.first, commands, conversation])
  useEffect(() => setHighlight(undefined), [slash?.query])
  // The project's files are listed again each time the @ menu opens, since
  // a file may have been made a moment ago; meanwhile the menu shows those
  // listed last time.
  useEffect(() => {
    setHighlight(undefined)
    if (!filesOpen || mentions === undefined) return
    let current = true
    mentions().then(
      (listed) => current && setListedFiles(listed),
      (cause: unknown) => current && setListedFiles((was) => (was?.files !== undefined ? was : { problem: cause instanceof Error ? cause.message : String(cause) })),
    )
    return () => void (current = false)
  }, [filesOpen, mentions])
  const files = useMemo(() => (mention === undefined || listedFiles?.files === undefined ? [] : rankFiles(listedFiles.files, mention.query)), [mention?.query, listedFiles])
  useEffect(() => setHighlight(undefined), [mention?.query])

  // The box grows with its text, up to a height, then scrolls.
  useLayoutEffect(() => {
    const area = box.current
    if (!area) return
    area.style.height = "auto"
    area.style.height = `${area.scrollHeight}px`
    if (caretTo.current !== undefined) {
      area.setSelectionRange(caretTo.current, caretTo.current)
      setCaret(caretTo.current)
      caretTo.current = undefined
    }
  }, [text, focused, wide])
  // Focused as the conversation opens, where there is a keyboard to type with.
  useEffect(() => {
    if (wide) box.current?.focus()
  }, [])
  // mod+shift+M opens the model picker, as in T3 Code.
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === "m") {
        event.preventDefault()
        setPicking(true)
      }
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [])
  // A message that could not be sent on the box's behalf, such as a draft's
  // first, comes back into the box, with why, unless the owner has typed since.
  useEffect(() => {
    if (state?.givenBack === undefined || conversation === undefined) return
    const back = conversation.takeGivenBack()
    if (back === undefined) return
    setText((now) => (now === "" ? back.text : now))
    if (back.files !== undefined) attach(back.files)
    setProblem(back.problem)
  }, [state?.givenBack, conversation])

  function put(next: string, at = next.length) {
    caretTo.current = at
    setText(next)
  }

  function pick(item: MenuItem) {
    if (slash === undefined) return
    const after = text.slice(caret)
    if (item.name === MODEL.name) {
      // The /model typed goes, and the picker opens in its place.
      put(`${text.slice(0, slash.from)}${after.replace(/^ /, "")}`, slash.from)
      setPicking(true)
      return
    }
    const filled = `${text.slice(0, slash.from)}/${item.name} `
    put(`${filled}${after.replace(/^ /, "")}`, filled.length)
  }

  // Files added by the paperclip, a paste or a drop, but for any larger
  // than the server takes, which the box names.
  function attach(adding: readonly File[]) {
    if (adding.length === 0) return
    const { taken, problem } = attachable(adding)
    setProblem(problem)
    setAttached((now) => [...now, ...taken.map((file) => ({ key: (attachedCount += 1), file }))])
  }

  function pickFile(path: string) {
    if (mention === undefined) return
    const filled = putMention(text, mention, caret, path)
    put(filled.text, filled.caret)
  }

  async function submit() {
    const does = sendWithFiles(text, shown?.question, attached.length)
    if (does === undefined) return
    // Not sent: the text stays, and the box says why.
    if ("refuse" in does) return setProblem(does.refuse)
    const was = text
    const wasAttached = attached
    const files = attached.map((each) => each.file)
    asking.cleared()
    setProblem(undefined)
    setRecalled(undefined)
    put("")
    setAttached([])
    const restore = (refused: string | undefined) => {
      if (refused === undefined) return
      setProblem(refused)
      setText((now) => (now === "" ? was : now))
      setAttached((now) => (now.length === 0 ? wasAttached : now))
    }
    if ("answer" in does) return restore(await answer(shown!.question.id, does.answer))
    if ("decline" in does) {
      const refused = await answer(shown!.question.id, does.decline)
      if (refused !== undefined) return restore(refused)
      return restore(await send(does.message, files))
    }
    if ("run" in does) {
      if (conversation === undefined) return restore("Commands can be run once the session has started. Send a message first.")
      return void conversation.run(does.run.name, does.run.args)
    }
    restore(await send(does.message, files))
  }

  function onKeyDown(event: KeyEvent<HTMLTextAreaElement>) {
    if (event.nativeEvent.isComposing) return
    if (filesOpen) {
      const move = menuKey(event.key, highlight, files.length)
      if (move !== undefined) {
        event.preventDefault()
        if ("close" in move) setClosedAt(text)
        else if ("highlight" in move) setHighlight(move.highlight)
        else pickFile(files[move.pick]!)
        return
      }
    }
    if (menuOpen) {
      const move = menuKey(event.key, highlight, items.length)
      if (move !== undefined) {
        event.preventDefault()
        if ("close" in move) setClosedAt(text)
        else if ("highlight" in move) setHighlight(move.highlight)
        else pick(items[move.pick]!)
        return
      }
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault()
      void submit()
      return
    }
    if (event.key === "Escape" && working && state?.stopping === false) {
      event.preventDefault()
      conversation?.stop()
      return
    }
    // Up on the first line, or Down on the last, brings back an earlier message.
    const area = event.currentTarget
    const onFirstLine = !text.slice(0, area.selectionStart).includes("\n")
    const onLastLine = !text.slice(area.selectionEnd).includes("\n")
    if ((event.key === "ArrowUp" && onFirstLine) || (event.key === "ArrowDown" && onLastLine)) {
      const sent = [...(state?.events ?? []).flatMap((each) => (each.type === "user_message" ? [each.body.text] : [])), ...(state?.pending ?? [])]
      const step = recallStep(event.key === "ArrowUp" ? "back" : "forward", earlierMessages(sent), recalled, text)
      if (step === undefined) return
      event.preventDefault()
      setRecalled(step.recalled)
      put(step.text)
    }
  }

  const docked = shown?.place === "docked" ? shown : undefined
  // At phone width, one row until tapped, as T3 Code's, unless there is more to show.
  const collapsed = !wide && !focused && !text.includes("\n") && docked === undefined && !picking
  const placeholder =
    shown?.place === "inline" && conversation !== undefined
      ? "Send a message to say no, and what to do instead"
      : docked?.question.freeText
        ? "Your own answer, or a number"
        : working
          ? "Tell evie more as it works. Escape stops it"
          : conversation === undefined
            ? mentions === undefined
              ? "Ask anything"
              : "Ask anything. @ files"
            : mentions === undefined
              ? "Ask anything. / commands"
              : "Ask anything. @ files, / commands"
  const menuId = "composer-menu"
  const filesId = "composer-files"
  const collapsedNow = collapsed && attached.length === 0

  return (
    <div className="shrink-0 px-4 pb-4 wide:px-7 wide:pb-6">
      <div className="mx-auto flex max-w-conversation flex-col gap-2">
        {docked && <DockedQuestion question={docked.question} count={docked.count} answer={answer} />}
        {problem !== undefined && (
          <p role="status" className="px-4 text-caption break-words text-text-muted">
            {problem}
          </p>
        )}
        {dragging && (
          <p role="status" className="px-4 text-caption text-text-muted">
            Drop the files to attach them
          </p>
        )}
        <div className="relative">
          {menuOpen && (
            <CommandMenu
              id={menuId}
              slash={slash}
              items={items}
              draft={conversation === undefined}
              listing={conversation !== undefined && slash.first && commands?.items === undefined ? (commands?.problem ?? "listing") : undefined}
              highlight={highlight}
              pick={pick}
              setHighlight={setHighlight}
            />
          )}
          {filesOpen && (
            <FileMenu
              id={filesId}
              files={files}
              listing={listedFiles?.files === undefined ? (listedFiles?.problem ?? "listing") : undefined}
              more={listedFiles?.more === true}
              highlight={highlight}
              pick={pickFile}
              setHighlight={setHighlight}
            />
          )}
          {picking && (
            <ModelPicker
              model={model}
              close={() => {
                setPicking(false)
                box.current?.focus()
              }}
            />
          )}
          <form
            onSubmit={(event) => {
              event.preventDefault()
              void submit()
            }}
            // Files dropped on the box are attached.
            onDragOver={(event) => {
              if (!event.dataTransfer.types.includes("Files")) return
              event.preventDefault()
              setDragging(true)
            }}
            onDragLeave={(event) => {
              if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false)
            }}
            onDrop={(event) => {
              if (event.dataTransfer.files.length === 0) return
              event.preventDefault()
              setDragging(false)
              attach([...event.dataTransfer.files])
            }}
            className={cn("flex rounded-lg border border-line bg-surface", collapsedNow ? "items-center gap-2 py-1 pr-1 pl-4" : "flex-col gap-1 pt-3 pr-2 pb-2 pl-4")}
          >
            {attached.length > 0 && <AttachedFiles attached={attached} remove={(key) => setAttached((now) => now.filter((each) => each.key !== key))} />}
            <label className="flex min-w-0 flex-1">
              <span className="sr-only">Message evie</span>
              <textarea
                ref={box}
                rows={1}
                value={text}
                placeholder={placeholder}
                role="combobox"
                aria-expanded={menuOpen || filesOpen}
                aria-controls={menuOpen ? menuId : filesOpen ? filesId : undefined}
                aria-autocomplete="list"
                aria-activedescendant={(menuOpen || filesOpen) && highlight !== undefined ? `${menuOpen ? menuId : filesId}-${highlight}` : undefined}
                onChange={(event) => {
                  setText(event.target.value)
                  setCaret(event.target.selectionStart)
                  setProblem(undefined)
                  asking.typed()
                }}
                onSelect={(event) => setCaret(event.currentTarget.selectionStart)}
                onKeyDown={onKeyDown}
                // Files pasted are attached, as an image copied is.
                onPaste={(event) => {
                  if (event.clipboardData.files.length === 0) return
                  event.preventDefault()
                  attach([...event.clipboardData.files])
                }}
                onFocus={() => setFocused(true)}
                onBlur={() => {
                  setFocused(false)
                  asking.cleared()
                }}
                // One row until tapped: the text runs on, not onto a second line.
                wrap={collapsedNow ? "off" : "soft"}
                className="max-h-composer-text w-full resize-none overflow-y-auto bg-transparent text-body text-text outline-none placeholder:text-text-muted"
              />
            </label>
            <div className={cn("flex min-w-0 items-center gap-2", !collapsedNow && "justify-end")}>
              <input
                ref={chooser}
                type="file"
                multiple
                hidden
                onChange={(event) => {
                  attach([...(event.currentTarget.files ?? [])])
                  event.currentTarget.value = ""
                  box.current?.focus()
                }}
              />
              {!collapsedNow && (
                // Pressed, the box keeps the focus, so that at phone width it
                // stays open while the files are chosen.
                <Button variant="icon" aria-label="Attach files" title="Attach files" onPointerDown={(event) => event.preventDefault()} onClick={() => chooser.current?.click()} className="mr-auto -ml-2">
                  <Paperclip />
                </Button>
              )}
              {!collapsedNow && (
                <div className="flex min-w-0 items-center text-caption text-text-muted">
                  <ModelName model={model.current} toggle={() => setPicking((open) => !open)} />
                  {conversation && state && <ContextMeter conversation={conversation} state={state} />}
                </div>
              )}
              {working && <StopButton stopping={state?.stopping === true} stop={() => conversation?.stop()} />}
              <Button type="submit" variant="primary" aria-label={working ? "Send, to steer the turn" : "Send"} disabled={text.trim() === "" && attached.length === 0} className="size-control rounded-full p-0">
                <ArrowUpBold className="size-icon-sm" />
              </Button>
            </div>
          </form>
        </div>
      </div>
    </div>
  )
}

// Whether the page is wide enough for its three columns, where there is
// room for the whole box at all times.
function useWide() {
  const [wide, setWide] = useState(() => matchMedia(WIDE).matches)
  useEffect(() => {
    const query = matchMedia(WIDE)
    const change = () => setWide(query.matches)
    query.addEventListener("change", change)
    return () => query.removeEventListener("change", change)
  }, [])
  return wide
}
const WIDE = "(min-width: 960px)"

// Stop, as T3 Code's: a round button with a square in it, beside Send. Once
// pressed, it says Stopping until the turn has ended.
function StopButton({ stopping, stop }: { stopping: boolean; stop(): void }) {
  if (stopping) {
    return (
      <span role="status" className="flex h-control items-center gap-2 px-2 text-caption text-status-working">
        <Spinner />
        Stopping…
      </span>
    )
  }
  return (
    <Button variant="secondary" aria-label="Stop" title="Stop (Escape)" onClick={stop} className="size-control rounded-full p-0">
      <StopFill className="size-icon-sm" />
    </Button>
  )
}

// The model's name, the part of its id after the maker's, which opens the
// picker, and closes it again.
function ModelName({ model, toggle }: { model: string | undefined; toggle(): void }) {
  if (model === undefined) return null
  return (
    <Button
      variant="quiet"
      onClick={toggle}
      data-picker-toggle=""
      title={`${model}. Pick another with /model or ${MOD}+Shift+M`}
      aria-label={`Model: ${model}. Pick another`}
      className="min-w-0 px-2 text-caption"
    >
      <span className="truncate">{model.slice(model.indexOf("/") + 1)}</span>
    </Button>
  )
}
const MOD = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform) ? "⌘" : "Ctrl"

// How full the conversation is, as a share of the size at which evie
// compacts it by itself, in words; its menu says the sizes and compacts it
// now. Its colour does not change: the board gives a colour no such job,
// so a state is said in words, as Compacting.
function ContextMeter({ conversation, state }: { conversation: Conversation; state: ConversationState }) {
  if (state.running.includes("compact")) {
    return (
      <span role="status" className="flex h-control items-center gap-2 px-2 text-status-working">
        <Spinner />
        Compacting
      </span>
    )
  }
  if (state.context === undefined) return null
  const meter = contextMeter(state.context)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="quiet" aria-label={`Context ${meter.percent} full`} className="px-2 text-caption">
          {meter.percent}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="end" className="max-w-dialog">
        <DropdownMenuLabel className="text-body text-text">
          {meter.size} of {meter.compactsAt} tokens
        </DropdownMenuLabel>
        <p className="px-3 pb-2 text-caption text-text-muted">At {meter.compactsAt}, evie replaces the older part of the conversation with a summary by itself.</p>
        <DropdownMenuItem onSelect={() => void conversation.run("compact", "")}>Compact now</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// The menu a slash opens, just above the box, as T3 Code's: each command
// and skill with what it does, filtered and ranked as the owner types. The
// box keeps the focus, and moves the highlight with Up and Down.
function CommandMenu({
  id,
  slash,
  items,
  draft,
  listing,
  highlight,
  pick,
  setHighlight,
}: {
  id: string
  slash: NonNullable<ReturnType<typeof slashAt>>
  items: MenuItem[]
  // In a draft, which runs no command but /model.
  draft: boolean
  // While the commands are being listed, "listing", or why they could not be.
  listing: string | undefined
  highlight: number | undefined
  pick(item: MenuItem): void
  setHighlight(at: number): void
}) {
  const list = useRef<HTMLUListElement>(null)
  useEffect(() => {
    if (highlight !== undefined) list.current?.querySelector(`#${id}-${highlight}`)?.scrollIntoView({ block: "nearest" })
  }, [highlight])
  const quiet = (words: string, working = false) => (
    <p className={cn("flex min-h-control items-center gap-2 px-3 text-caption", working ? "text-status-working" : "text-text-muted")}>
      {working && <Spinner />}
      {words}
    </p>
  )
  return (
    <div className="absolute inset-x-0 bottom-full z-40 mb-2 flex max-h-composer-menu flex-col overflow-y-auto rounded-md border border-line bg-surface p-2 shadow-float">
      {listing === "listing" ? (
        quiet("Listing the commands", true)
      ) : listing !== undefined ? (
        quiet(listing)
      ) : items.length === 0 ? (
        quiet(draft ? "Other commands can be run once the session has started." : "No matching command.")
      ) : (
        <ul ref={list} id={id} role="listbox" aria-label={slash.first ? "Commands and skills" : "Commands"} className="flex flex-col">
          {items.map((item, at) => (
            <li
              key={item.name}
              id={`${id}-${at}`}
              role="option"
              aria-selected={at === highlight}
              // The box keeps the focus.
              onMouseDown={(event) => event.preventDefault()}
              onClick={() => pick(item)}
              onMouseMove={() => at !== highlight && setHighlight(at)}
              className={cn("flex h-control shrink-0 cursor-default items-center gap-3 rounded-md px-3", at === highlight && "bg-surface-raised")}
            >
              <span className="shrink-0 text-label text-text">/{item.name}</span>
              <span title={item.description} className="min-w-0 flex-1 truncate text-caption text-text-muted">
                {item.description}
              </span>
              {item.kind !== undefined && <span className="shrink-0 rounded-sm bg-surface-raised px-2 text-caption text-text-muted">{item.kind}</span>}
            </li>
          ))}
        </ul>
      )}
      {draft && items.length > 0 && quiet("Other commands can be run once the session has started.")}
    </div>
  )
}

// The model picker, just above the box, as T3 Code's: the makers in a
// column on the left, the current model's first; a search over every
// maker's models on the right; the model in use marked with a check and
// "In use". As in a menu, the row Up and Down, or the pointer, moves to is
// raised; Enter picks it, and Escape, or a click outside it other than on
// the model's name, which closes it too, closes it, giving the focus back.
function ModelPicker({ model, close }: { model: ModelChoice; close(): void }) {
  const current = model.current
  const [listed, setListed] = useState<{ models?: Model[]; problem?: string }>({})
  const [query, setQuery] = useState("")
  const [maker, setMaker] = useState<string>()
  const [highlight, setHighlight] = useState(0)
  const [refused, setRefused] = useState<string>()
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => {
    let live = true
    model.list().then(
      (models) => live && setListed({ models }),
      (cause: unknown) => live && setListed({ problem: cause instanceof Error ? cause.message : String(cause) }),
    )
    return () => void (live = false)
  }, [])
  // A click outside closes it, as a menu's does, but for one on the model's
  // name, which closes it itself.
  useEffect(() => {
    const outside = (event: PointerEvent) => {
      const target = event.target as Element
      if (!root.current?.contains(target) && target.closest?.("[data-picker-toggle]") == null) close()
    }
    document.addEventListener("pointerdown", outside)
    return () => document.removeEventListener("pointerdown", outside)
  }, [close])

  const makers = useMemo(() => makersOf(listed.models ?? []), [listed.models])
  const currentMaker = current?.split("/")[0]
  const chosen = maker ?? (makers.some((each) => each.key === currentMaker) ? currentMaker : makers[0]?.key)
  const shown: (Model & { maker?: string })[] = query.trim() !== "" ? searchModels(listed.models ?? [], query) : (makers.find((each) => each.key === chosen)?.models ?? [])
  useEffect(() => setHighlight(0), [query, chosen])

  async function pick(id: string) {
    const problem = await model.pick(id)
    if (problem !== undefined) setRefused(problem)
    else close()
  }
  function onKeyDown(event: KeyEvent<HTMLInputElement>) {
    if (event.key === "Escape") {
      event.preventDefault()
      close()
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      if (shown.length > 0) setHighlight((at) => (at + (event.key === "ArrowDown" ? 1 : shown.length - 1)) % shown.length)
    } else if (event.key === "Enter") {
      event.preventDefault()
      const chosen = shown[highlight]
      if (chosen) void pick(chosen.id)
    }
  }
  useEffect(() => {
    root.current?.querySelector(`[data-at="${highlight}"]`)?.scrollIntoView({ block: "nearest" })
  }, [highlight])

  return (
    <div ref={root} role="dialog" aria-label="Pick a model" className="absolute inset-x-0 bottom-full z-40 mb-2 flex h-composer-menu overflow-hidden rounded-md border border-line bg-surface shadow-float">
      <nav aria-label="Makers" className="flex w-picker-makers shrink-0 flex-col overflow-y-auto border-r border-line p-2">
        {makers.map((each) => (
          <button
            key={each.key}
            type="button"
            aria-current={query.trim() === "" && each.key === chosen ? "true" : undefined}
            onClick={() => {
              setQuery("")
              setMaker(each.key)
            }}
            className="flex h-control shrink-0 items-center rounded-md px-3 text-left text-body text-text-muted hover:text-text aria-[current]:bg-surface-raised aria-[current]:text-text"
          >
            <span className="truncate">{each.name}</span>
          </button>
        ))}
      </nav>
      <div className="flex min-w-0 flex-1 flex-col">
        <input
          autoFocus
          type="search"
          aria-label="Search models"
          placeholder="Search models"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onKeyDown={onKeyDown}
          role="combobox"
          aria-expanded="true"
          aria-controls="model-list"
          aria-activedescendant={shown.length > 0 ? `model-${highlight}` : undefined}
          className="h-control shrink-0 border-b border-line bg-transparent px-4 text-body text-text outline-none placeholder:text-text-muted"
        />
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {listed.problem !== undefined ? (
            <p className="px-3 py-2 text-caption text-text-muted">{listed.problem}</p>
          ) : listed.models === undefined ? (
            <p className="flex min-h-control items-center gap-2 px-3 text-caption text-status-working">
              <Spinner />
              Listing the models
            </p>
          ) : shown.length === 0 ? (
            <p className="px-3 py-2 text-caption text-text-muted">{query.trim() === "" ? "No models to pick from." : "No matching model."}</p>
          ) : (
            <ul id="model-list" role="listbox" aria-label="Models" className="flex flex-col">
              {shown.map((each, at) => (
                <li
                  key={each.id}
                  id={`model-${at}`}
                  data-at={at}
                  role="option"
                  aria-selected={at === highlight}
                  // The search keeps the focus.
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => void pick(each.id)}
                  onMouseMove={() => at !== highlight && setHighlight(at)}
                  className={cn("flex min-h-control shrink-0 cursor-default items-center gap-2 rounded-md px-3 py-1", at === highlight && "bg-surface-raised")}
                >
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-body text-text">{each.name}</span>
                    <span className="truncate text-caption text-text-muted">{each.maker !== undefined ? `${each.maker} · ${each.id}` : each.id}</span>
                  </span>
                  {each.id === current && (
                    <span className="flex shrink-0 items-center gap-1 text-caption text-text-muted">
                      <Check className="size-icon-sm" />
                      In use
                    </span>
                  )}
                </li>
              ))}
            </ul>
          )}
          {refused !== undefined && (
            <p role="status" className="px-3 py-2 text-caption text-text-muted">
              {refused}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

// The files attached in the box, as chips above the text, as T3 Code's: an
// image with a small thumbnail, any other file with its icon, each with
// its name and size, and a button that takes it off.
function AttachedFiles({ attached, remove }: { attached: Attached[]; remove(key: number): void }) {
  return (
    <ul aria-label="Files to send" className="-ml-2 flex flex-wrap gap-2 pb-1">
      {attached.map(({ key, file }) => (
        <li key={key} className="flex h-control max-w-full min-w-0 items-center gap-2 rounded-md bg-surface-raised pl-1">
          {file.type.startsWith("image/") ? <Thumbnail file={file} /> : <FileIcon className="mx-2 size-icon-sm shrink-0 text-text-muted" />}
          <span className="min-w-0 truncate text-caption text-text" title={file.name}>
            {file.name}
          </span>
          <span className="shrink-0 text-caption text-text-muted">{sizeWords(file.size)}</span>
          <Button variant="icon" aria-label={`Remove ${file.name}`} onClick={() => remove(key)} className="[&>svg]:size-icon-sm">
            <X />
          </Button>
        </li>
      ))}
    </ul>
  )
}

// A small picture of an image attached, read from the file in the browser
// until it is sent.
function Thumbnail({ file }: { file: File }) {
  const [address, setAddress] = useState<string>()
  useEffect(() => {
    const made = URL.createObjectURL(file)
    setAddress(made)
    return () => URL.revokeObjectURL(made)
  }, [file])
  return <span className="size-attachment-chip shrink-0 overflow-hidden rounded-sm bg-surface">{address !== undefined && <img src={address} alt="" className="size-full object-cover" />}</span>
}

// The menu an @ opens, just above the box, as T3 Code's: the project's
// files, filtered and ranked as the owner types, each by its name and the
// folder it is in. The box keeps the focus, and moves the highlight with
// Up and Down, as in the menu of commands.
function FileMenu({
  id,
  files,
  listing,
  more,
  highlight,
  pick,
  setHighlight,
}: {
  id: string
  files: string[]
  // While the files are being listed, "listing", or why they could not be.
  listing: string | undefined
  // Whether the project has more files than were listed.
  more: boolean
  highlight: number | undefined
  pick(path: string): void
  setHighlight(at: number): void
}) {
  const list = useRef<HTMLUListElement>(null)
  useEffect(() => {
    if (highlight !== undefined) list.current?.querySelector(`#${id}-${highlight}`)?.scrollIntoView({ block: "nearest" })
  }, [highlight])
  const quiet = (words: string, working = false) => (
    <p className={cn("flex min-h-control items-center gap-2 px-3 text-caption", working ? "text-status-working" : "text-text-muted")}>
      {working && <Spinner />}
      {words}
    </p>
  )
  return (
    <div className="absolute inset-x-0 bottom-full z-40 mb-2 flex max-h-composer-menu flex-col overflow-y-auto rounded-md border border-line bg-surface p-2 shadow-float">
      {listing === "listing" ? (
        quiet("Listing the files", true)
      ) : listing !== undefined ? (
        quiet(listing)
      ) : files.length === 0 ? (
        quiet("No matching file.")
      ) : (
        <ul ref={list} id={id} role="listbox" aria-label="Files" className="flex flex-col">
          {files.map((path, at) => {
            const cut = path.lastIndexOf("/")
            return (
              <li
                key={path}
                id={`${id}-${at}`}
                role="option"
                aria-selected={at === highlight}
                // The box keeps the focus.
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => pick(path)}
                onMouseMove={() => at !== highlight && setHighlight(at)}
                className={cn("flex h-control shrink-0 cursor-default items-center gap-3 rounded-md px-3", at === highlight && "bg-surface-raised")}
              >
                <FileIcon className="size-icon-sm shrink-0 text-text-muted" />
                <span className="shrink-0 text-label text-text">{path.slice(cut + 1)}</span>
                {cut > 0 && (
                  <span title={path} className="min-w-0 flex-1 truncate text-caption text-text-muted">
                    {path.slice(0, cut)}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
      )}
      {more && listing === undefined && quiet("The project has more files than are listed. Type more of a name to find one.")}
    </div>
  )
}
