import { useEffect, useLayoutEffect, useRef, type ClipboardEvent, type KeyboardEvent, type RefObject } from "react"
import { replaced, shownOf, type Part } from "./composer.ts"
import { FILE_OUTLINE } from "./icons.tsx"
import { cn } from "./lib/utils.ts"

// The message box's text, as T3 Code's prompt editor: words, with each
// file named in them a chip, its icon and its name, which the caret steps
// over and Backspace takes off whole. It is an editable element of plain
// text and chips, and nothing else: whatever the browser puts in it, as a
// pasted page's styles, is read back as words and chips and drawn again.
//
// `box` is what to draw: each time its `drawn` changes, the element is
// drawn from its parts, with the caret where it says. What the owner types
// is read back into parts and told to `changed`, and where the caret moves
// to `moved`, without drawing the element again, so that typing, as with
// an input method, is the browser's own.

export type Box = { parts: Part[]; caret: number; drawn: number }

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
  onKeyDown(event: KeyboardEvent<HTMLDivElement>): void
  onFocus(): void
  onBlur(): void
  label: string
  // The menu the box opens, when one is open, and the row raised in it.
  menu: { id: string; active: string | undefined } | undefined
}

export function PromptEditor({ ref, box, changed, moved, oneRow, placeholder, pasted, onKeyDown, onFocus, onBlur, label, menu }: Props) {
  const parts = useRef(box.parts)
  parts.current = box.parts
  // Drawn from the parts when asked, with the caret placed if the box has the focus.
  useLayoutEffect(() => {
    const root = ref.current
    if (!root) return
    draw(root, box.parts)
    if (document.activeElement === root) place(root, box.caret)
  }, [box.drawn])

  // A new line, and text dropped, are put in by the box itself, as words,
  // and the browser's own formatting is refused.
  useEffect(() => {
    const root = ref.current
    if (!root) return
    const before = (event: InputEvent) => {
      if (event.inputType === "insertParagraph" || event.inputType === "insertLineBreak") {
        event.preventDefault()
        put(root, "\n")
      } else if (event.inputType.startsWith("format") || event.inputType === "insertFromDrop") event.preventDefault()
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
      if (at !== undefined) moved(at.start)
    }
    document.addEventListener("selectionchange", selected)
    return () => document.removeEventListener("selectionchange", selected)
  }, [moved])

  // Words put in place of what is selected, and the box drawn again.
  function put(root: HTMLDivElement, text: string) {
    const at = selection(root) ?? { start: shownOf(parts.current).length, end: shownOf(parts.current).length }
    const next = replaced(parts.current, at.start, at.end, [{ text }])
    draw(root, next.parts)
    place(root, next.caret)
    changed(next.parts, next.caret)
  }

  function onPaste(event: ClipboardEvent<HTMLDivElement>) {
    event.preventDefault()
    if (event.clipboardData.files.length > 0) return pasted([...event.clipboardData.files])
    const text = event.clipboardData.getData("text/plain")
    if (text !== "") put(event.currentTarget, text.replace(/\r\n?/g, "\n"))
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
        aria-multiline="true"
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
          changed(read.parts, read.caret)
        }}
        onPaste={onPaste}
        onKeyDown={onKeyDown}
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
// chip. A line break at the very end is followed by one the browser draws,
// without which the empty line after it would not show.
function draw(root: HTMLElement, parts: readonly Part[]) {
  const nodes: Node[] = parts.map((part) => ("file" in part ? chip(part.file) : document.createTextNode(part.text)))
  if (shownOf(parts).endsWith("\n")) {
    const tail = document.createElement("br")
    tail.dataset.tail = ""
    nodes.push(tail)
  }
  root.replaceChildren(...nodes)
}

// A file as a chip in the text, as T3 Code's: the file's icon and its
// name, in the text's colour and a heavier weight, its whole path on
// hover. It is one thing, which the caret steps over.
function chip(path: string): HTMLElement {
  const made = document.createElement("span")
  made.dataset.file = path
  made.contentEditable = "false"
  made.spellcheck = false
  made.title = path
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

// The caret put at a place in the element, counted as `shownOf` counts.
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
