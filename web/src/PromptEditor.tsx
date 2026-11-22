import { useEffect, useLayoutEffect, useRef, type ClipboardEvent, type DragEvent, type KeyboardEvent, type RefObject } from "react"
import { confirmsInput, NO_HISTORY, partsOf, recorded, redone, replaced, shownOf, sliceOf, textOf, undone, type Change, type History, type Part, type Snapshot } from "./composer.ts"
import { FILE_OUTLINE } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { onMac } from "./shortcuts.ts"

// The message box's text, as T3 Code's prompt editor: words, with each
// file named in them a chip, its icon and its name, which the caret steps
// over and Backspace takes off whole. It is an editable element of plain
// text and chips, and nothing else: whatever the browser puts in it, as a
// pasted page's styles, is read back as words and chips and drawn again.
// Copied, cut or dragged, a chip is its @ and its path, so that pasted or
// dropped back it is a chip again. It keeps an undo history of its own, in
// which each change is a step, since the browser's cannot undo a chip put
// in, and Mod+Z and Mod+Shift+Z go through it.
//
// `box` is what to draw: each time its `drawn` changes, the element is
// drawn from its parts, with the caret where it says; as a step of the
// undo history when `undoable` says so, such as a chip picked, and
// otherwise as a fresh start, such as the box emptied as a message is
// sent. What the owner types is read back into parts and told to
// `changed`, and where the caret moves to `moved`, without drawing the
// element again, so that typing, as with an input method, is the
// browser's own.

export type Box = { parts: Part[]; caret: number; drawn: number; undoable?: boolean }

type Props = {
  ref: RefObject<HTMLDivElement | null>
  box: Box
  changed(parts: Part[], caret: number): void
  moved(caret: number): void
  // One row, which runs on rather than wrapping, at phone width until tapped.
  oneRow: boolean
  placeholder: string
  // Files pasted, which the box attaches.
  pasted(files: File[]): void
  // A key the box is to act on: not one an input method takes, nor undo or redo.
  onKeyDown(event: KeyboardEvent<HTMLDivElement>): void
  onFocus(): void
  onBlur(): void
  label: string
  // The menu the box opens, when one is open, and the row raised in it.
  menu: { id: string; active: string | undefined } | undefined
}

export function PromptEditor({ ref, box, changed, moved, oneRow, placeholder, pasted, onKeyDown, onFocus, onBlur, label, menu }: Props) {
  // The box as last drawn or read, and its undo history.
  const known = useRef<Snapshot>({ parts: box.parts, caret: box.caret })
  const history = useRef<History>(NO_HISTORY)
  // When an input method last finished composing, and whether a drag
  // within the box has just taken its words out to put them back.
  const composedAt = useRef<number>(undefined)
  const moving = useRef(false)
  // The latest `changed`, for the handlers set up once.
  const tell = useRef(changed)
  tell.current = changed

  // Drawn from the parts when asked, with the caret placed if the box has the focus.
  useLayoutEffect(() => {
    const root = ref.current
    if (!root) return
    history.current = box.undoable === true ? recorded(history.current, known.current, "chip", Date.now()) : NO_HISTORY
    show(root, { parts: box.parts, caret: box.caret })
  }, [box.drawn])

  // The element and what is known of it, as a snapshot says.
  function show(root: HTMLElement, to: Snapshot) {
    draw(root, to.parts)
    if (document.activeElement === root) place(root, to.caret)
    known.current = to
  }
  // A change the box makes itself, as a step of the history.
  function apply(root: HTMLElement, kind: Change, to: Snapshot) {
    history.current = recorded(history.current, known.current, kind, Date.now())
    show(root, to)
    tell.current(to.parts, to.caret)
  }
  // Parts put in place of what is selected, or where `at` says.
  function put(root: HTMLElement, kind: Change, parts: Part[], at?: number) {
    const range = at !== undefined ? { start: at, end: at } : (selection(root) ?? { start: known.current.caret, end: known.current.caret })
    const next = replaced(known.current.parts, range.start, range.end, parts)
    apply(root, kind, next)
  }
  function step(root: HTMLElement, back: boolean) {
    const moved = back ? undone(history.current, known.current) : redone(history.current, known.current)
    if (moved === undefined) return
    history.current = moved.history
    show(root, moved.to)
    tell.current(moved.to.parts, moved.to.caret)
  }

  // What the browser would put in itself: a new line, and text dropped,
  // the box puts in as words; typing and deleting are the browser's, each
  // a step of the history, typing running into one; undo and redo go
  // through the box's own history; formatting is refused.
  useEffect(() => {
    const root = ref.current
    if (!root) return
    const before = (event: InputEvent) => {
      const kind = event.inputType
      if (kind === "insertParagraph" || kind === "insertLineBreak") {
        event.preventDefault()
        put(root, "line", [{ text: "\n" }])
      } else if (kind === "historyUndo" || kind === "historyRedo") {
        event.preventDefault()
        step(root, kind === "historyUndo")
      } else if (kind === "deleteByDrag") {
        // Words dragged within the box: taken out by the browser, and put
        // back where they are dropped, as one step.
        history.current = recorded(history.current, known.current, "drop", Date.now())
        moving.current = true
      } else if (kind === "insertFromDrop") {
        event.preventDefault()
        const text = event.dataTransfer?.getData("text/plain") ?? ""
        const target = event.getTargetRanges()[0]
        const at = target !== undefined && root.contains(target.startContainer) ? offsetOf(root, target.startContainer, target.startOffset) : undefined
        const parts = partsOf(text.replace(/\r\n?/g, "\n"))
        if (moving.current) {
          moving.current = false
          const next = replaced(known.current.parts, at ?? known.current.caret, at ?? known.current.caret, parts)
          show(root, next)
          tell.current(next.parts, next.caret)
        } else if (text !== "") put(root, "drop", parts, at)
      } else if (kind.startsWith("format")) event.preventDefault()
      else if (kind.startsWith("delete")) history.current = recorded(history.current, { ...known.current, caret: selection(root)?.start ?? known.current.caret }, "deleting", Date.now())
      else if (kind.startsWith("insert")) history.current = recorded(history.current, { ...known.current, caret: selection(root)?.start ?? known.current.caret }, "typing", Date.now())
    }
    root.addEventListener("beforeinput", before)
    return () => root.removeEventListener("beforeinput", before)
  }, [])
  // The caret moving, by a key or a click.
  useEffect(() => {
    const root = ref.current
    if (!root) return
    const selected = () => {
      const at = selection(root)
      if (at === undefined) return
      known.current = { ...known.current, caret: at.start }
      moved(at.start)
    }
    document.addEventListener("selectionchange", selected)
    return () => document.removeEventListener("selectionchange", selected)
  }, [moved])

  // What is selected, as text with each chip its @ and its path.
  function selectedText(root: HTMLElement): string | undefined {
    const at = selection(root)
    return at === undefined || at.start === at.end ? undefined : textOf(sliceOf(known.current.parts, at.start, at.end))
  }
  function onCopy(event: ClipboardEvent<HTMLDivElement>) {
    const text = selectedText(event.currentTarget)
    if (text === undefined) return
    event.preventDefault()
    event.clipboardData.setData("text/plain", text)
  }
  function onCut(event: ClipboardEvent<HTMLDivElement>) {
    const text = selectedText(event.currentTarget)
    if (text === undefined) return
    event.preventDefault()
    event.clipboardData.setData("text/plain", text)
    put(event.currentTarget, "cut", [])
  }
  function onPaste(event: ClipboardEvent<HTMLDivElement>) {
    event.preventDefault()
    if (event.clipboardData.files.length > 0) return pasted([...event.clipboardData.files])
    const text = event.clipboardData.getData("text/plain")
    if (text !== "") put(event.currentTarget, "paste", partsOf(text.replace(/\r\n?/g, "\n")))
  }
  // Words dragged out of the box carry their chips as @ and their paths.
  function onDragStart(event: DragEvent<HTMLDivElement>) {
    const text = selectedText(event.currentTarget)
    if (text !== undefined) event.dataTransfer.setData("text/plain", text)
  }
  function keyDown(event: KeyboardEvent<HTMLDivElement>) {
    // A key an input method takes, as the Enter that confirms what is
    // being composed, is the input method's.
    if (confirmsInput(event.nativeEvent, composedAt.current, Date.now())) return
    const mod = onMac() ? event.metaKey : event.ctrlKey
    const key = event.key.toLowerCase()
    if (mod && !event.altKey && (key === "z" || (key === "y" && !onMac()))) {
      event.preventDefault()
      return step(event.currentTarget, key === "z" && !event.shiftKey)
    }
    onKeyDown(event)
  }

  const empty = box.parts.length === 0
  return (
    <div className="relative min-w-0 flex-1">
      {empty && (
        <span aria-hidden="true" className="pointer-events-none absolute inset-x-0 top-0 truncate text-field text-text-muted">
          {placeholder}
        </span>
      )}
      <div
        ref={ref}
        contentEditable
        role="combobox"
        aria-label={label}
        aria-placeholder={placeholder}
        aria-expanded={menu !== undefined}
        aria-controls={menu?.id}
        aria-autocomplete="list"
        aria-activedescendant={menu?.active}
        // The keys 1 to 9 pick a question's choice while this is empty.
        data-message-box=""
        {...(empty && { "data-empty": "" })}
        spellCheck
        onInput={(event) => {
          const root = event.currentTarget
          const read = readBox(root)
          // Anything but words and chips, as the browser's own line breaks
          // or a pasted page's styles, is drawn again as words and chips.
          if (!read.clean && !(event.nativeEvent as InputEvent).isComposing) {
            draw(root, read.parts)
            place(root, read.caret)
          }
          known.current = { parts: read.parts, caret: read.caret }
          changed(read.parts, read.caret)
        }}
        onCompositionEnd={() => (composedAt.current = Date.now())}
        onCopy={onCopy}
        onCut={onCut}
        onPaste={onPaste}
        onDragStart={onDragStart}
        onKeyDown={keyDown}
        onFocus={onFocus}
        onBlur={onBlur}
        className={cn(
          "w-full overflow-y-auto break-words text-field text-text outline-none",
          oneRow ? "overflow-hidden whitespace-pre" : "max-h-composer-text min-h-composer-text-min whitespace-pre-wrap",
        )}
      />
    </div>
  )
}

// ---- The element and the parts ----------------------------------------------------------

// The element drawn from the parts: each word as text, each file as its
// chip, with a place for the caret, an empty word, on both sides of every
// chip, so that the caret can be put before or after one at either end of
// the box, in any browser. A line break at the very end is followed by one
// the browser draws, without which the empty line after it would not show.
function draw(root: HTMLElement, parts: readonly Part[]) {
  const nodes: Node[] = []
  for (const part of parts) {
    if ("file" in part) {
      if (!(nodes.at(-1) instanceof Text)) nodes.push(document.createTextNode(""))
      nodes.push(chip(part.file))
    } else nodes.push(document.createTextNode(part.text))
  }
  if (nodes.length > 0 && !(nodes.at(-1) instanceof Text)) nodes.push(document.createTextNode(""))
  if (shownOf(parts).endsWith("\n")) {
    const tail = document.createElement("br")
    tail.dataset.tail = ""
    nodes.push(tail)
  }
  root.replaceChildren(...nodes)
}

// A file as a chip in the text, as T3 Code's: the file's icon and its
// name, in the text's colour and a heavier weight, its whole path on
// hover, and named "File @path" to a screen reader. It is one thing,
// which the caret steps over.
function chip(path: string): HTMLElement {
  const made = document.createElement("span")
  made.dataset.file = path
  made.contentEditable = "false"
  made.spellcheck = false
  made.title = path
  made.setAttribute("role", "img")
  made.setAttribute("aria-label", `File @${path}`)
  made.className = "inline-flex max-w-full items-baseline gap-0.5 align-baseline font-medium text-text"
  const icon = document.createElementNS("http://www.w3.org/2000/svg", "svg")
  icon.setAttribute("viewBox", "0 0 256 256")
  icon.setAttribute("fill", "currentColor")
  icon.setAttribute("aria-hidden", "true")
  icon.setAttribute("class", "size-icon-row shrink-0 self-center text-text-muted")
  const outline = document.createElementNS("http://www.w3.org/2000/svg", "path")
  outline.setAttribute("d", FILE_OUTLINE)
  icon.append(outline)
  const name = document.createElement("span")
  name.className = "truncate"
  name.textContent = path.slice(path.lastIndexOf("/") + 1)
  made.append(icon, name)
  return made
}

// The parts the element holds, and where its caret is, counted as
// `shownOf` counts. `clean` says it holds only words and chips, as drawn.
// What else the browser may put in it is read as words: a line break as
// one, and a block, as a line of its own.
function readBox(root: HTMLElement): { parts: Part[]; caret: number; clean: boolean } {
  const parts: Part[] = []
  let length = 0
  let clean = true
  let caret: number | undefined
  const chosen = getSelection()
  const anchor = chosen !== null && chosen.rangeCount > 0 && root.contains(chosen.anchorNode) ? { node: chosen.anchorNode!, offset: chosen.anchorOffset } : undefined
  const words = (text: string) => {
    parts.push({ text })
    length += text.length
  }
  const walk = (node: Node, top: boolean) => {
    for (const [at, child] of [...node.childNodes].entries()) {
      if (anchor?.node === node && anchor.offset === at) caret = length
      if (child instanceof Text) {
        if (anchor?.node === child) caret = length + Math.min(anchor.offset, child.data.length)
        // A space the browser keeps from collapsing is a space.
        if (child.data.includes(" ")) clean = false
        words(child.data.replace(/ /g, " "))
      } else if (child instanceof HTMLElement && child.dataset.file !== undefined) {
        if (!top) clean = false
        parts.push({ file: child.dataset.file })
        length += 1
        if (anchor !== undefined && child.contains(anchor.node)) caret = length
      } else if (child instanceof HTMLBRElement) {
        if (child.dataset.tail !== undefined && top && child === root.lastChild) continue
        clean = false
        // A break the browser leaves at the very end, as when the last
        // chip is taken off, only holds the line open: it is no new line.
        if (top && child === root.lastChild) continue
        words("\n")
      } else {
        clean = false
        const block = child instanceof HTMLElement && /^(DIV|P|LI)$/.test(child.tagName)
        if (block && length > 0 && !shownOf(parts).endsWith("\n")) words("\n")
        walk(child, false)
      }
    }
    if (anchor?.node === node && anchor.offset >= node.childNodes.length) caret = length
  }
  walk(root, true)
  const tidy = replaced(parts, 0, 0, [])
  return { parts: tidy.parts, caret: caret ?? length, clean }
}

// Where the selection starts and ends in the element, counted as `shownOf`
// counts, or undefined when it is not in the element.
function selection(root: HTMLElement): { start: number; end: number } | undefined {
  const chosen = getSelection()
  if (chosen === null || chosen.rangeCount === 0) return undefined
  const range = chosen.getRangeAt(0)
  if (!root.contains(range.startContainer) || !root.contains(range.endContainer)) return undefined
  return { start: offsetOf(root, range.startContainer, range.startOffset), end: offsetOf(root, range.endContainer, range.endOffset) }
}

// How far a place in the element is from its start, counted as `shownOf`
// counts, in the element as drawn.
function offsetOf(root: HTMLElement, node: Node, offset: number): number {
  let at = 0
  for (const [index, child] of [...root.childNodes].entries()) {
    if (node === root && index === offset) return at
    if (child === node) return at + (child instanceof Text ? offset : 0)
    if (child.contains(node)) return at + lengthOf(child)
    at += lengthOf(child)
  }
  return at
}

const lengthOf = (node: Node): number => (node instanceof Text ? node.data.length : node instanceof HTMLElement && node.dataset.file !== undefined ? 1 : node instanceof HTMLBRElement && node.dataset.tail !== undefined ? 0 : (node.textContent?.length ?? 0))

// The caret put at a place in the element, counted as `shownOf` counts: in
// a word wherever there is one there, as the empty ones beside each chip.
function place(root: HTMLElement, caret: number) {
  const chosen = getSelection()
  if (chosen === null) return
  let at = 0
  for (const [index, child] of [...root.childNodes].entries()) {
    const length = lengthOf(child)
    if (child instanceof Text && caret <= at + length) return chosen.collapse(child, caret - at)
    if (!(child instanceof Text) && caret === at) return chosen.collapse(root, index)
    at += length
  }
  const last = root.lastChild
  chosen.collapse(root, last instanceof HTMLBRElement && last.dataset.tail !== undefined ? root.childNodes.length - 1 : root.childNodes.length)
}
