import { useEffect, useMemo, useRef, type ReactNode } from "react"
import type { FileDiff } from "../../src/core/types.ts"
import { CodeView, viewerItems, viewerOptions, type CodeViewHandle } from "./diff-view.ts"

// The diff's screen, in the right panel: the files' diffs as T3 Code's
// diff panel draws them, in one view that scrolls and draws only the lines
// in sight, each file under a header that stays at the top while its file
// scrolls past, and folds or opens it when pressed. The library that draws
// them is behind diff-view.ts; the page loads both the first time the
// panel shows a diff.

export type DiffViewProps = {
  // The files with a diff to draw, in the order to draw them.
  files: readonly FileDiff[]
  layout: "stacked" | "split"
  wrap: boolean
  // The files folded, by their paths, and what folds or opens one.
  folded: ReadonlySet<string>
  fold(path: string): void
  // A file to scroll to once it is drawn, and what is told it has been.
  goTo: string | undefined
  wentTo(): void
  // What follows the last file, such as the files that have no diff.
  footer?: ReactNode
}

export default function DiffView({ files, layout, wrap, folded, fold, goTo, wentTo, footer }: DiffViewProps) {
  const viewer = useRef<CodeViewHandle<undefined, undefined>>(null)
  const items = useMemo(() => viewerItems(files, folded), [files, folded])
  // Scrolled to once the file is open and drawn; asked again after each
  // drawing until it is.
  const going = useRef(goTo)
  going.current = goTo
  const go = () => {
    const path = going.current
    if (path === undefined || viewer.current === null) return
    const item = viewer.current.getItem(path)
    if (item === undefined || item.collapsed === true) return
    viewer.current.scrollTo({ type: "item", id: path, align: "start" })
    going.current = undefined
    wentTo()
  }
  useEffect(go, [goTo, items])
  const options = useMemo(() => viewerOptions({ layout, wrap, drawn: () => go() }), [layout, wrap])
  return (
    // A press anywhere on a file's header, but on a button in it, folds or
    // opens the file, as its caret does. The header is in the library's
    // shadow root; the caret, which names the file, is in the page.
    <div
      className="h-full min-h-0"
      onClickCapture={(event) => {
        const path = event.nativeEvent.composedPath()
        if (path.some((node) => node instanceof HTMLButtonElement || node instanceof HTMLAnchorElement)) return
        const header = path.find((node): node is HTMLElement => node instanceof HTMLElement && node.hasAttribute("data-diffs-header"))
        const root = header?.getRootNode()
        const file = root instanceof ShadowRoot ? root.host.querySelector("[data-fold-path]")?.getAttribute("data-fold-path") : null
        if (file) fold(file)
      }}
    >
      <CodeView<undefined>
        ref={viewer}
        className="h-full min-h-0 overflow-auto outline-none"
        items={items}
        disableWorkerPool
        options={options}
        // The caret that folds a file is the library's own chevron, as the
        // owner's drawing has it: its path is copied from @pierre/diffs
        // (Apache-2.0 licence, a copy in LICENSES/pierre-diffs-Apache-2.0.txt),
        // since the library's icons are inside its shadow root.
        renderHeaderPrefix={(item) => {
          const open = item.collapsed !== true
          return (
            <button
              type="button"
              aria-expanded={open}
              aria-label={`${open ? "Fold" : "Unfold"} ${item.id}`}
              data-fold-path={item.id}
              onClick={(event) => {
                event.stopPropagation()
                fold(item.id)
              }}
              className="flex size-5 shrink-0 items-center justify-center rounded-sm text-text-muted"
            >
              <svg viewBox="0 0 16 16" aria-hidden="true" fill="currentColor" className={`size-icon-sm transition-transform motion-reduce:transition-none ${open ? "" : "-rotate-90"}`}>
                <path d="M1.47 4.47a.75.75 0 0 1 1.06 0L8 9.94l5.47-5.47a.75.75 0 1 1 1.06 1.06l-6 6a.75.75 0 0 1-1.06 0l-6-6a.75.75 0 0 1 0-1.06" />
              </svg>
            </button>
          )
        }}
        renderHeaderMetadata={(item) => {
          const file = files.find((each) => each.path === item.id)
          return file === undefined ? null : <HeaderStat added={file.added} removed={file.removed} />
        }}
        {...(footer !== undefined && { renderCodeViewFooter: () => footer })}
      />
    </div>
  )
}

// What a file's header says of its lines: the removed, then the added, as
// T3 Code's, leaving out a side with none unless both have none.
function HeaderStat({ added, removed }: { added: number; removed: number }) {
  return (
    <span role="group" aria-label={`${added} added, ${removed} removed`} className="inline-flex shrink-0 gap-1.5 font-mono text-2xs tabular-nums">
      {(removed > 0 || added === 0) && (
        <span aria-hidden="true" className="text-diff-removed">
          −{removed}
        </span>
      )}
      {(added > 0 || removed === 0) && (
        <span aria-hidden="true" className="text-diff-added">
          +{added}
        </span>
      )}
    </span>
  )
}
