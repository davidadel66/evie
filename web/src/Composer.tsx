import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent as KeyEvent, type MouseEvent, type ReactNode } from "react"
import type { Shown } from "./Asking.tsx"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { attachable, contextMeter, earlierMessages, mentionAt, menuKey, partsOf, pickFile, rankCommands, rankFiles, recallStep, replaced, shownOf, slashAt, textOf, type MenuItem, type Part, type Recalled } from "./composer.ts"
import type { Conversation, ConversationState, Outgoing } from "./conversation.ts"
import { ArrowBendUpRight, ArrowUpBold, Check, File as FileIcon, ListPlus, Paperclip, StopFill, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { makersOf, modelName, searchModels, type Model } from "./models.ts"
import { whenAsked, windowOver } from "./page-requests.ts"
import { PromptEditor, type Box } from "./PromptEditor.tsx"
import { boxPlaceholder, sendWithFiles } from "./questions.ts"
import { dispatchOf } from "./queue.ts"
import { label, onMac, pressed } from "./shortcuts.ts"
import { useWide } from "./wide.ts"
import { Spinner } from "./Spinner.tsx"
import { sizeWords } from "./workspace.ts"

// The message box, floating over the end of the conversation, which
// scrolls on under it, as T3 Code's does, and drawn as the Session view
// drawing has it: the text two lines tall, then the paperclip on the left
// and the model's name and Send on the right. It works as T3 Code's:
// Enter sends and Shift and Enter starts a new line; Up in an empty box
// brings back the owner's earlier messages; a slash opens the menu of
// commands and skills, and an @ the menu of the project's files, each a
// drawer tucked into the box's top edge; a file picked is a chip in the
// text, sent as an @ and its path; /model, or mod+shift+M, opens the model
// picker. While evie works, Stop takes Send's place, and Escape stops it
// too; what is sent steers the turn, or with mod held, mod and Enter or a
// mod-click on Send, waits in a queue to be sent as a turn of its own.
// Under the text, the model's name and how full the conversation is, whose
// menu compacts it. At phone width it is one row until it is tapped.
// Files are attached with the paperclip, by pasting or by dropping them on
// the box: each is a chip above the text until it is sent.

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
  // A file the server took on a try that failed keeps where it is kept,
  // and is not sent again.
  send(text: string, files: readonly Outgoing[]): Promise<string | undefined>
  // The files of the session's project, which an @ picks from. A session
  // with no project has none, and no @ menu.
  mentions?: (() => Promise<{ files: string[]; more: boolean }>) | undefined
  // Told how tall the box floats, with the room round it, so that the
  // conversation under it can scroll its end clear of it.
  floats(height: number): void
}

// A file attached in the box, until it is sent.
type Attached = Outgoing & { key: number }
let attachedCount = 0
const attachedOf = (files: readonly Outgoing[]): Attached[] => files.map((file) => ({ ...file, key: (attachedCount += 1) }))

// The model, as the menu lists it: picking it opens the picker, as in T3 Code.
const MODEL: MenuItem = { name: "model", description: "Pick the model this session uses" }

export function Composer({ conversation, state, model, shown, answer, asking, send, mentions, floats }: Props) {
  const [box, setBox] = useState<Box>({ parts: [], caret: 0, drawn: 0 })
  const [problem, setProblem] = useState<string>()
  const [recalled, setRecalled] = useState<Recalled>()
  // The box at which Escape closed the menu, which stays closed until it changes.
  const [closedAt, setClosedAt] = useState<string>()
  const [highlight, setHighlight] = useState<number>()
  const [commands, setCommands] = useState<{ items?: MenuItem[]; problem?: string }>()
  const [picking, setPicking] = useState(false)
  const [focused, setFocused] = useState(false)
  const [attached, setAttached] = useState<Attached[]>([])
  const [dragging, setDragging] = useState(false)
  const [listedFiles, setListedFiles] = useState<{ files?: string[]; more?: boolean; problem?: string }>()
  // Whether the context meter's menu is open, which keeps the box whole
  // at phone width, as the picker does, though the focus has left it.
  const [meterOpen, setMeterOpen] = useState(false)
  const wide = useWide()
  const modHeld = useModHeld()
  const models = useModels(model)
  const root = useRef<HTMLDivElement>(null)
  const editor = useRef<HTMLDivElement>(null)
  const chooser = useRef<HTMLInputElement>(null)

  // The box as the caret and the menus read it, a chip one character, and
  // as it is sent.
  const typed = shownOf(box.parts)
  const text = textOf(box.parts)
  const caret = box.caret
  const slash = closedAt === typed ? undefined : slashAt(typed, caret)
  const menuOpen = slash !== undefined && !picking
  const working = state?.turnRunning === true
  const mention = closedAt === typed || mentions === undefined || picking ? undefined : mentionAt(typed, caret)
  const filesOpen = mention !== undefined
  // The queued message being edited in the box, if one is.
  const editing = state?.queue.editing

  // The box with new parts, drawn, the caret at its end or where given.
  const put = (parts: Part[], at = shownOf(parts).length) => setBox((was) => ({ parts, caret: at, drawn: was.drawn + 1 }))
  const moved = useCallback((at: number) => setBox((was) => (was.caret === at ? was : { ...was, caret: at })), [])

  // How tall it floats, told as it changes.
  useLayoutEffect(() => {
    const floating = root.current
    if (!floating) return
    const tell = () => floats(floating.offsetHeight)
    tell()
    const watch = new ResizeObserver(tell)
    watch.observe(floating)
    return () => watch.disconnect()
  }, [floats])

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

  // Focused as the conversation opens, where there is a keyboard to type with.
  useEffect(() => {
    if (wide) editor.current?.focus()
  }, [])
  // mod+shift+M opens the model picker, as in T3 Code, but for a window
  // open over the page, and so does Pick a model in the command palette.
  useEffect(() => {
    const mac = onMac()
    const onKey = (event: KeyboardEvent) => {
      if (pressed(event, mac) !== "model" || windowOver() !== undefined) return
      event.preventDefault()
      setPicking(true)
    }
    document.addEventListener("keydown", onKey)
    const unpick = whenAsked("pick model", () => setPicking(true))
    return () => {
      document.removeEventListener("keydown", onKey)
      unpick()
    }
  }, [])
  // A message that could not be sent on the box's behalf, such as a draft's
  // first, comes back into the box, with why, unless the owner has typed since.
  useEffect(() => {
    if (state?.givenBack === undefined || conversation === undefined) return
    const back = conversation.takeGivenBack()
    if (back === undefined) return
    setBox((now) => (now.parts.length === 0 ? { parts: partsOf(back.text), caret: shownOf(partsOf(back.text)).length, drawn: now.drawn + 1 } : now))
    if (back.files !== undefined) setAttached((now) => [...now, ...attachedOf(back.files!)])
    setProblem(back.problem)
  }, [state?.givenBack, conversation])
  // A queued message edited is put in the box, in place of what was in it,
  // which comes back once the edit is saved or given up, as T3 Code's edit
  // has a draft of its own.
  const aside = useRef<{ key: number; parts: Part[]; attached: Attached[] }>(undefined)
  useEffect(() => {
    if (editing === aside.current?.key) return
    const back = aside.current
    const item = state?.queue.items.find((each) => each.key === editing)
    aside.current = item === undefined ? undefined : { key: item.key, parts: back?.parts ?? box.parts, attached: back?.attached ?? attached }
    if (item !== undefined) {
      put(partsOf(item.text))
      setAttached(attachedOf(item.files))
      editor.current?.focus()
    } else if (back !== undefined) {
      put(back.parts)
      setAttached(back.attached)
    }
    setProblem(undefined)
  }, [editing])

  function pick(item: MenuItem) {
    if (slash === undefined) return
    const end = typed.slice(caret).startsWith(" ") ? caret + 1 : caret
    // The /model typed goes, and the picker opens in its place.
    const next = replaced(box.parts, slash.from, end, item.name === MODEL.name ? [] : [{ text: `/${item.name} ` }])
    put(next.parts, next.caret)
    if (item.name === MODEL.name) setPicking(true)
  }

  // Files added by the paperclip, a paste or a drop, but for any larger
  // than the server takes, which the box names.
  function attach(adding: readonly File[]) {
    if (adding.length === 0) return
    const { taken, problem } = attachable(adding)
    setProblem(problem)
    setAttached((now) => [...now, ...attachedOf(taken.map((file) => ({ file })))])
  }

  function chooseFile(path: string) {
    if (mention === undefined) return
    const next = pickFile(box.parts, mention, caret, path)
    put(next.parts, next.caret)
  }

  // Sends what is in the box, as `alternate` says: with mod held while
  // evie works, a message is queued rather than steering the turn.
  async function submit(alternate: boolean) {
    if (editing !== undefined && conversation !== undefined) {
      asking.cleared()
      return conversation.saveQueued(editing, text.trim(), attached)
    }
    const does = sendWithFiles(text, shown?.question, attached.length)
    if (does === undefined) return
    // Not sent: the text stays, and the box says why.
    if ("refuse" in does) return setProblem(does.refuse)
    const was = box.parts
    const wasAttached = attached
    const files = attached
    asking.cleared()
    setProblem(undefined)
    setRecalled(undefined)
    put([])
    setAttached([])
    if ("message" in does && conversation !== undefined && dispatchOf({ working, alternate }) === "queue") return conversation.queue(does.message, files)
    const restore = (refused: string | undefined) => {
      if (refused === undefined) return
      setProblem(refused)
      setBox((now) => (now.parts.length === 0 ? { parts: was, caret: shownOf(was).length, drawn: now.drawn + 1 } : now))
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

  function onKeyDown(event: KeyEvent<HTMLDivElement>) {
    if (event.nativeEvent.isComposing) return
    if (filesOpen) {
      const move = menuKey(event.key, highlight, files.length)
      if (move !== undefined) {
        event.preventDefault()
        if ("close" in move) setClosedAt(typed)
        else if ("highlight" in move) setHighlight(move.highlight)
        else chooseFile(files[move.pick]!)
        return
      }
    }
    if (menuOpen) {
      const move = menuKey(event.key, highlight, items.length)
      if (move !== undefined) {
        event.preventDefault()
        if ("close" in move) setClosedAt(typed)
        else if ("highlight" in move) setHighlight(move.highlight)
        else pick(items[move.pick]!)
        return
      }
    }
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault()
      void submit(onMac() ? event.metaKey : event.ctrlKey)
      return
    }
    // Escape gives up editing a queued message, before it stops evie.
    if (event.key === "Escape" && editing !== undefined) {
      event.preventDefault()
      conversation?.stopEditing()
      return
    }
    if (pressed(event.nativeEvent, onMac()) === "stop" && working && state?.stopping === false) {
      event.preventDefault()
      conversation?.stop()
      return
    }
    // Up on the first line, or Down on the last, brings back an earlier message.
    const onFirstLine = !typed.slice(0, caret).includes("\n")
    const onLastLine = !typed.slice(caret).includes("\n")
    if ((event.key === "ArrowUp" && onFirstLine) || (event.key === "ArrowDown" && onLastLine)) {
      const sent = [...(state?.events ?? []).flatMap((each) => (each.type === "user_message" ? [each.body.text] : [])), ...(state?.pending ?? []).map((each) => each.text)]
      const step = recallStep(event.key === "ArrowUp" ? "back" : "forward", earlierMessages(sent), recalled, text)
      if (step === undefined) return
      event.preventDefault()
      setRecalled(step.recalled)
      put(partsOf(step.text))
    }
  }

  // At phone width, one row until tapped, as T3 Code's, unless there is more to show.
  const collapsed = !wide && !focused && !typed.includes("\n") && !picking && !meterOpen
  const placeholder = boxPlaceholder(shown?.question, { session: conversation !== undefined, working, mentions: mentions !== undefined })
  const menuId = "composer-menu"
  const filesId = "composer-files"
  const collapsedNow = collapsed && attached.length === 0 && problem === undefined && !dragging
  const empty = text.trim() === "" && attached.length === 0

  return (
    <div ref={root} className="pointer-events-none absolute inset-x-0 bottom-0 z-20 px-gutter pt-1.5 pb-4 wide:pb-5">
      <div className="pointer-events-auto relative mx-auto max-w-conversation">
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
            more={listedFiles?.more === true ? listedFiles.files?.length : undefined}
            highlight={highlight}
            pick={chooseFile}
            setHighlight={setHighlight}
          />
        )}
        {picking && (
          <ModelPicker
            model={model}
            close={() => {
              setPicking(false)
              editor.current?.focus()
            }}
          />
        )}
        <form
          onSubmit={(event) => event.preventDefault()}
          // At phone width a tap on a button keeps the focus in the box,
          // as T3 Code's does, so that the box stays whole and the tap
          // is not lost as it folds to one row.
          onMouseDown={(event) => {
            if (!wide && event.target instanceof Element && event.target.closest("button") !== null) event.preventDefault()
          }}
          // Files dropped on the box are attached, and the box's edge
          // shows it takes them, as T3 Code's does.
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
          className={cn(
            "composer-glass relative z-10 flex rounded-lg border",
            dragging ? "border-accent" : "border-line",
            collapsedNow ? "items-center gap-2 py-1 pr-1 pl-4" : "flex-col gap-1 pt-3 pr-2 pb-2 pl-4",
          )}
        >
          {attached.length > 0 && <AttachedFiles attached={attached} remove={(key) => setAttached((now) => now.filter((each) => each.key !== key))} />}
          {/* At phone width the text's line is as tall as a button, on one row or whole. */}
          <div className="flex min-w-0 flex-1 items-center self-stretch max-wide:min-h-control">
            <PromptEditor
              ref={editor}
              box={box}
              changed={(parts, at) => {
                setBox((was) => ({ ...was, parts, caret: at }))
                setProblem(undefined)
                asking.typed()
              }}
              moved={moved}
              oneRow={collapsedNow}
              placeholder={placeholder}
              pasted={attach}
              onKeyDown={onKeyDown}
              onFocus={() => setFocused(true)}
              onBlur={() => {
                setFocused(false)
                asking.cleared()
              }}
              label="Message evie"
              menu={menuOpen || filesOpen ? { id: menuOpen ? menuId : filesId, active: highlight !== undefined ? `${menuOpen ? menuId : filesId}-${highlight}` : undefined } : undefined}
            />
          </div>
          {/* Why something was not sent, or that files dropped are taken, under the text, as T3 Code's. */}
          <div role="status" className="text-caption break-words text-text-muted empty:hidden">
            {dragging ? "Drop the files to attach them" : problem}
          </div>
          <div className={cn("flex min-w-0 items-center gap-2", !collapsedNow && "justify-end")}>
            <input
              ref={chooser}
              type="file"
              multiple
              hidden
              onChange={(event) => {
                attach([...(event.currentTarget.files ?? [])])
                event.currentTarget.value = ""
                editor.current?.focus()
              }}
            />
            {!collapsedNow && (
              // Pressed, the box keeps the focus, so that at phone width it
              // stays open while the files are chosen.
              <Button variant="icon" aria-label="Attach files" title="Attach files" onPointerDown={(event) => event.preventDefault()} onClick={() => chooser.current?.click()} className="mr-auto -ml-1 size-control-sm">
                <Paperclip />
              </Button>
            )}
            {!collapsedNow && (
              <div className="flex min-w-0 items-center gap-1 text-body text-text-muted">
                <ModelName model={model.current} models={models} toggle={() => setPicking((open) => !open)} />
                {conversation && state && <ContextMeter conversation={conversation} state={state} opened={setMeterOpen} />}
              </div>
            )}
            <PrimaryAction
              does={editing !== undefined ? "update" : working && empty ? "stop" : working ? (modHeld ? "queue" : "steer") : "send"}
              disabled={editing === undefined && empty}
              stopping={state?.stopping === true}
              stop={() => conversation?.stop()}
              submit={(event) => void submit(onMac() ? event.metaKey : event.ctrlKey)}
            />
          </div>
        </form>
      </div>
    </div>
  )
}

// Whether mod is held down, which turns Send while evie works from
// steering to queueing, as T3 Code's shows.
function useModHeld() {
  const [held, setHeld] = useState(false)
  useEffect(() => {
    const mac = onMac()
    const keys = (event: KeyboardEvent) => setHeld(mac ? event.metaKey : event.ctrlKey)
    const letGo = () => setHeld(false)
    addEventListener("keydown", keys)
    addEventListener("keyup", keys)
    addEventListener("blur", letGo)
    return () => {
      removeEventListener("keydown", keys)
      removeEventListener("keyup", keys)
      removeEventListener("blur", letGo)
    }
  }, [])
  return held
}

// The models to pick from, for the model chip's name, listed as the box
// opens, and again for a model the list does not have, as one picked from
// another provider. Until they are, or when they cannot be, none.
function useModels(model: ModelChoice) {
  const [models, setModels] = useState<Model[]>()
  const known = models?.some((each) => each.id === model.current) === true
  useEffect(() => {
    if (known || model.current === undefined) return
    let live = true
    model.list().then(
      (listed) => live && setModels(listed),
      () => {},
    )
    return () => void (live = false)
  }, [model.list, model.current, known])
  return models
}

// The one action of the box, round, as T3 Code's, in the drawn Send's
// colour, which is the design system's main button's: Send, with the
// drawn arrow; while evie works with something to send, the same steering
// the turn, with T3 Code's arrow that turns, or with mod held, queueing it,
// with T3 Code's list; while it works with nothing to send, Stop, in its
// place, which stays, unable to be pressed again, until the turn has
// ended; and while a queued message is edited, saving it.
function PrimaryAction({
  does,
  disabled,
  stopping,
  stop,
  submit,
}: {
  does: "send" | "steer" | "queue" | "stop" | "update"
  disabled: boolean
  stopping: boolean
  stop(): void
  submit(event: MouseEvent<HTMLButtonElement>): void
}) {
  const mac = onMac()
  const round = "size-control rounded-full p-0 disabled:bg-text/12"
  if (does === "stop") {
    return (
      <Button variant="primary" aria-label={stopping ? "Stopping" : "Stop"} title={stopping ? "Stopping…" : `Stop (${label("stop", mac)})`} disabled={stopping} onClick={stop} className={round}>
        <StopFill className="size-icon-xs" />
      </Button>
    )
  }
  const alternate = `${mac ? "⌘" : "Ctrl"}-click`
  const { name, title, icon } = {
    send: { name: "Send", title: "Send", icon: <ArrowUpBold className="size-icon-sm" /> },
    steer: { name: "Steer message", title: `Click to steer, ${alternate} to queue`, icon: <ArrowBendUpRight className="size-icon-sm" /> },
    queue: { name: "Queue message", title: `Click to steer, ${alternate} to queue`, icon: <ListPlus className="size-icon-sm" /> },
    update: { name: "Update queued message", title: "Update queued message", icon: <Check className="size-icon-sm" /> },
  }[does]
  return (
    <Button variant="primary" aria-label={name} title={title} disabled={disabled} onClick={submit} className={round}>
      {icon}
    </Button>
  )
}

// The model's name, as T3 Code's model chip shows it: its display name, as
// its provider lists it, as a chip 28px tall at every width, in the drawn
// quiet words, which opens the picker, and closes it again.
function ModelName({ model, models, toggle }: { model: string | undefined; models: readonly Model[] | undefined; toggle(): void }) {
  if (model === undefined) return null
  return (
    <Button
      variant="quiet"
      onClick={toggle}
      data-picker-toggle=""
      title={`${model}. Pick another with /model or ${label("model", onMac())}`}
      aria-label={`Model: ${modelName(models, model)}. Pick another`}
      className="h-control-sm min-w-0 px-2 text-control"
    >
      <span className="truncate">{modelName(models, model)}</span>
    </Button>
  )
}

// How full the conversation is, as a share of the size at which evie
// compacts it by itself, in words; its menu says the sizes and compacts it
// now. Its colour does not change: the board gives a colour no such job,
// so a state is said in words, as Compacting.
function ContextMeter({ conversation, state, opened }: { conversation: Conversation; state: ConversationState; opened(open: boolean): void }) {
  if (state.running.includes("compact")) {
    return (
      <span role="status" className="flex h-control-sm items-center gap-2 px-2 text-status-working">
        <Spinner />
        Compacting
      </span>
    )
  }
  if (state.context === undefined) return null
  return <MeterMenu conversation={conversation} meter={contextMeter(state.context)} opened={opened} />
}

// The meter and its menu. A menu taken off the page while open, as when
// compacting starts, has closed, which the box is told, so that it does not
// stay whole for it.
function MeterMenu({ conversation, meter, opened }: { conversation: Conversation; meter: ReturnType<typeof contextMeter>; opened(open: boolean): void }) {
  useEffect(() => () => opened(false), [opened])
  return (
    <DropdownMenu onOpenChange={opened}>
      <DropdownMenuTrigger asChild>
        <Button variant="quiet" aria-label={`Context ${meter.percent} full`} className="h-control-sm min-w-7 px-2 text-control">
          {meter.percent}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="end" className="max-w-menu-wide">
        <DropdownMenuLabel className="text-body text-text">
          {meter.size} of {meter.compactsAt} tokens
        </DropdownMenuLabel>
        <p className="px-2 pb-2 text-caption text-text-muted">At {meter.compactsAt}, evie replaces the older part of the conversation with a summary by itself.</p>
        <DropdownMenuItem onSelect={() => void conversation.run("compact", "")}>Compact now</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// A drawer tucked into the box's top edge, as T3 Code's menus are: a little
// narrower than the box, its top corners round, on the box's glass, its
// foot tucked under the box, and its rows at most so tall before they
// scroll. The box keeps the focus, and moves the highlight with Up and Down.
function Drawer({ children }: { children: ReactNode }) {
  return <div className="composer-glass drawer-tucked absolute right-drawer-inset bottom-full left-drawer-inset -mb-drawer-tuck flex flex-col rounded-t-lg border border-b-0 border-line pb-drawer-tuck">{children}</div>
}

// A line in a drawer in place of its rows, as while they are being listed.
const Quiet = ({ words }: { words: string }) => <p className="px-5 pt-3 pb-7 text-caption text-text-muted">{words}</p>

// One of a drawer's rows, as T3 Code's: 28px, or 32px at phone width, its
// label at most nearly half the row, in heavier small words, and what it
// is for after it on the same line.
function DrawerRow({ id, at, highlight, setHighlight, pick, children }: { id: string; at: number; highlight: number | undefined; setHighlight(at: number): void; pick(): void; children: ReactNode }) {
  return (
    <li
      id={`${id}-${at}`}
      role="option"
      aria-selected={at === highlight}
      // The box keeps the focus.
      onMouseDown={(event) => event.preventDefault()}
      onClick={pick}
      onMouseMove={() => at !== highlight && setHighlight(at)}
      className={cn("flex min-h-control-sm shrink-0 cursor-default items-center gap-2 rounded-md px-2 py-1 text-caption", at === highlight && "bg-surface-raised")}
    >
      {children}
    </li>
  )
}

// The menu a slash opens, in its drawer, as T3 Code's: each command and
// skill with what it does, filtered and ranked as the owner types.
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
  const later = "Other commands can be run once the session has started."
  return (
    <Drawer>
      {listing === "listing" ? (
        <Quiet words="Listing the commands…" />
      ) : listing !== undefined ? (
        <Quiet words={listing} />
      ) : items.length === 0 ? (
        <Quiet words={draft ? later : "No matching command."} />
      ) : (
        <ul ref={list} id={id} role="listbox" aria-label={slash.first ? "Commands and skills" : "Commands"} className="flex max-h-composer-menu flex-col overflow-y-auto p-2">
          {items.map((item, at) => (
            <DrawerRow key={item.name} id={id} at={at} highlight={highlight} setHighlight={setHighlight} pick={() => pick(item)}>
              <span className="max-w-9/20 shrink-0 truncate font-medium text-text">/{item.name}</span>
              <span title={item.description} className="min-w-0 flex-1 truncate text-text-muted">
                {item.description}
              </span>
              {item.kind !== undefined && <span className="shrink-0 rounded-sm bg-surface-raised px-1.5 text-text-muted">{item.kind}</span>}
            </DrawerRow>
          ))}
          {draft && <li className="px-2 pt-1 text-caption text-text-muted">{later}</li>}
        </ul>
      )}
    </Drawer>
  )
}

// The menu an @ opens, in its drawer, as T3 Code's: the project's files,
// filtered and ranked as the owner types, each by its name and the folder
// it is in, with the same keys as the menu of commands.
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
  // How many files were listed, when the project has more than that.
  more: number | undefined
  highlight: number | undefined
  pick(path: string): void
  setHighlight(at: number): void
}) {
  const list = useRef<HTMLUListElement>(null)
  useEffect(() => {
    if (highlight !== undefined) list.current?.querySelector(`#${id}-${highlight}`)?.scrollIntoView({ block: "nearest" })
  }, [highlight])
  return (
    <Drawer>
      {listing === "listing" ? (
        <Quiet words="Listing the files…" />
      ) : listing !== undefined ? (
        <Quiet words={listing} />
      ) : files.length === 0 ? (
        <Quiet words="No matching files." />
      ) : (
        <ul ref={list} id={id} role="listbox" aria-label="Files" className="flex max-h-composer-menu flex-col overflow-y-auto p-2">
          {files.map((path, at) => {
            const cut = path.lastIndexOf("/")
            return (
              <DrawerRow key={path} id={id} at={at} highlight={highlight} setHighlight={setHighlight} pick={() => pick(path)}>
                <FileIcon className="size-icon-sm shrink-0 text-text-muted" />
                <span className="max-w-9/20 shrink-0 truncate font-medium text-text">{path.slice(cut + 1)}</span>
                {cut > 0 && (
                  <span title={path} className="min-w-0 flex-1 truncate text-text-muted">
                    {path.slice(0, cut)}
                  </span>
                )}
              </DrawerRow>
            )
          })}
          {more !== undefined && <li className="px-2 pt-1 text-caption text-text-muted">Only the first {more.toLocaleString("en")} files are listed.</li>}
        </ul>
      )}
    </Drawer>
  )
}

// The model picker, just above the box, as T3 Code's: the makers in a
// column on the left, the current model's first, or at phone width in a
// row above the models that scrolls sideways, so that the models have the
// whole width; a search over every maker's models; the model in use
// marked with a check and "In use". As in a menu, the row Up and Down, or the pointer, moves to is
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
    <div ref={root} role="dialog" aria-label="Pick a model" className="absolute inset-x-0 bottom-full z-40 mb-2 flex h-composer-menu flex-col overflow-hidden rounded-md border border-line bg-surface shadow-float wide:flex-row">
      <nav
        aria-label="Makers"
        className="flex shrink-0 gap-1 overflow-x-auto border-b border-line p-1 wide:w-picker-makers wide:flex-col wide:gap-0 wide:overflow-x-hidden wide:overflow-y-auto wide:border-r wide:border-b-0"
      >
        {makers.map((each) => (
          <button
            key={each.key}
            type="button"
            aria-current={query.trim() === "" && each.key === chosen ? "true" : undefined}
            onClick={() => {
              setQuery("")
              setMaker(each.key)
            }}
            className="flex h-control-sm shrink-0 items-center rounded-sm px-2 text-left text-body text-text-muted hover:text-text aria-[current]:bg-surface-raised aria-[current]:text-text"
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
          className="h-control shrink-0 border-b border-line bg-transparent px-3 text-field text-text outline-none placeholder:text-text-muted"
        />
        <div className="min-h-0 flex-1 overflow-y-auto p-1">
          {listed.problem !== undefined ? (
            <p className="px-2 py-2 text-caption text-text-muted">{listed.problem}</p>
          ) : listed.models === undefined ? (
            <p className="flex min-h-control-sm items-center gap-2 px-2 text-caption text-status-working">
              <Spinner />
              Listing the models
            </p>
          ) : shown.length === 0 ? (
            <p className="px-2 py-2 text-caption text-text-muted">{query.trim() === "" ? "No models to pick from." : "No matching model."}</p>
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
                  className={cn("flex min-h-control-sm shrink-0 cursor-default items-center gap-2 rounded-sm px-2 py-1", at === highlight && "bg-surface-raised")}
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
            <p role="status" className="px-2 py-2 text-caption text-text-muted">
              {refused}
            </p>
          )}
        </div>
      </div>
    </div>
  )
}

// The files attached in the box, as chips above the text, as T3 Code's,
// drawn on the box itself, with no box of their own: an image's small
// thumbnail, or any other file's icon, its name and size, and a button
// that takes it off.
function AttachedFiles({ attached, remove }: { attached: Attached[]; remove(key: number): void }) {
  return (
    <ul aria-label="Files to send" className="flex flex-wrap gap-x-4 pb-1">
      {attached.map(({ key, file }) => (
        <li key={key} className="flex h-control max-w-full min-w-0 items-center gap-2">
          <FileChip file={file} />
          <Button variant="icon" aria-label={`Remove ${file.name}`} onClick={() => remove(key)} className="-ml-1 size-control-sm">
            <X />
          </Button>
        </li>
      ))}
    </ul>
  )
}

// A file in the browser as a chip shows it: an image's thumbnail, or any
// other file's icon, then its name and size.
export function FileChip({ file }: { file: File }) {
  return (
    <>
      {file.type.startsWith("image/") ? <Thumbnail file={file} /> : <FileIcon className="size-icon-sm shrink-0 text-text-muted" />}
      <span className="min-w-0 truncate text-caption text-text" title={file.name}>
        {file.name}
      </span>
      <span className="shrink-0 text-caption text-text-muted">{sizeWords(file.size)}</span>
    </>
  )
}

// A small picture of an image attached, read from the file in the browser
// until it is sent, with the corners of the design system's smaller things.
function Thumbnail({ file }: { file: File }) {
  const [address, setAddress] = useState<string>()
  useEffect(() => {
    const made = URL.createObjectURL(file)
    setAddress(made)
    return () => URL.revokeObjectURL(made)
  }, [file])
  return <span className="size-attachment-chip shrink-0 overflow-hidden rounded-sm">{address !== undefined && <img src={address} alt="" className="size-full object-cover" />}</span>
}

