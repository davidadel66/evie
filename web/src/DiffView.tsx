import { useEffect, useMemo, useRef, useState, type ReactNode } from "react"
import type { FileDiff } from "../../src/core/types.ts"
import { drawable } from "./diff.ts"
import { CodeView, DIFF_WORKER_META, holdPool, sidesLoader, viewerItems, viewerOptions, WorkerPoolContext, type CodeViewHandle, type ReadSides } from "./diff-view.ts"
import { cn } from "./lib/utils.ts"

// The diff's screen, in the right panel: the files' diffs as T3 Code's
// diff panel draws them, in one view that scrolls and draws only the lines
// in sight, each file under a header that stays at the top while its file
// scrolls past, and folds or opens it when pressed; and after them, the
// files that have no diff, each saying why. The library that draws them,
// and colours the code in workers of its own, is behind diff-view.ts; the
// page loads both the first time the panel shows a diff.

export type DiffViewProps = {
  // The comparison's files, in the order to list them.
  files: readonly FileDiff[]
  layout: "stacked" | "split"
  wrap: boolean
  // The files folded, by their paths, and what folds or opens one.
  folded: ReadonlySet<string>
  fold(path: string): void
  // A file to scroll to once it is drawn, and what is told it has been.
  goTo: string | undefined
  wentTo(): void
  // How far the diff was scrolled when last shown, and what is told as it scrolls.
  scroll: number | undefined
  scrolled(top: number): void
  // What reads a file's whole sides, to open the lines folded in it, and
  // what is told why they could not be.
  read: ReadSides
  failed(why: string): void
}

// How many drawings a scroll put back waits for the files to be tall enough.
const RESTORE_TRIES = 20

export default function DiffView({ files, layout, wrap, folded, fold, goTo, wentTo, scroll, scrolled, read, failed }: DiffViewProps) {
  const viewer = useRef<CodeViewHandle<undefined, undefined>>(null)
  const scroller = useRef<HTMLDivElement>(null)
  const [pool] = useState(() => holdPool(document.querySelector<HTMLMetaElement>(`meta[name="${DIFF_WORKER_META}"]`)?.content))
  useEffect(() => pool.release, [pool])
  const [drawn, notes] = useMemo(() => [files.filter(drawable), files.filter((file) => !drawable(file))], [files])
  const items = useMemo(() => viewerItems(drawn, folded), [drawn, folded])

  // Scrolled to once the file is open and drawn, or to its note, for a file
  // with no diff; asked again after each drawing until it is. Without one
  // to go to, the diff is put back where it was scrolled to, once.
  const going = useRef(goTo)
  going.current = goTo
  const restoring = useRef(scroll !== undefined && scroll > 0 ? { to: scroll, tries: RESTORE_TRIES } : undefined)
  const go = () => {
    const handle = viewer.current
    if (handle === null) return
    const path = going.current
    if (path === undefined) {
      const restore = restoring.current
      const container = scroller.current
      if (restore === undefined || container === null || handle.getInstance() === undefined) return
      container.scrollTop = restore.to
      // Until the files are drawn tall enough, the scroll falls short: it is tried again after the next drawing.
      restore.tries -= 1
      if (Math.abs(container.scrollTop - restore.to) <= 2 || restore.tries === 0) restoring.current = undefined
      return
    }
    const item = handle.getItem(path)
    if (item === undefined) {
      const note = scroller.current?.querySelector<HTMLElement>(`[data-note-path="${CSS.escape(path)}"]`)
      if (note === null || note === undefined) return
      note.scrollIntoView({ block: "start" })
    } else {
      if (item.collapsed === true) return
      handle.scrollTo({ type: "item", id: path, align: "start" })
    }
    going.current = undefined
    restoring.current = undefined
    wentTo()
  }
  useEffect(go, [goTo, items])
  // How far it is scrolled is kept as the owner scrolls, but not while it
  // is put back, nor as it goes.
  const gone = useRef(false)
  useEffect(
    () => () => {
      gone.current = true
    },
    [],
  )

  // The library asks for a file's sides when its folded lines are first
  // opened; what goes wrong is said above the diff, as well as in the console.
  const telling = useRef(failed)
  telling.current = failed
  const reading = useRef(read)
  reading.current = read
  const load = useMemo(() => {
    const loader = sidesLoader((path, from) => reading.current(path, from))
    return async (diff: Parameters<typeof loader>[0]) => {
      try {
        return await loader(diff)
      } catch (cause) {
        telling.current(cause instanceof Error ? cause.message : String(cause))
        throw cause
      }
    }
  }, [])
  const options = useMemo(() => viewerOptions({ layout, wrap, drawn: () => go(), load }), [layout, wrap, load])
  return (
    <WorkerPoolContext.Provider value={pool.pool}>
      <CodeView<undefined>
        ref={viewer}
        containerRef={scroller}
        className="min-h-0 flex-1 overflow-auto outline-none"
        items={items}
        options={options}
        onScroll={(top) => restoring.current === undefined && !gone.current && scrolled(top)}
        renderCustomHeader={(item) => {
          const file = drawn.find((each) => each.path === item.id)
          return file === undefined ? null : <FileHeader file={file} open={item.collapsed !== true} fold={() => fold(file.path)} />
        }}
        renderCodeViewFooter={() => (notes.length > 0 ? <Notes files={notes} /> : null)}
      />
    </WorkerPoolContext.Provider>
  )
}

// A file's header, as T3 Code's: 32px, in 12px Geist; a caret that folds
// or opens the file, as a press anywhere on the header does; an icon for
// how it changed, named for a screen reader; its path, cut at the start
// where it is too long, and whole in its tooltip, after the path it had
// for a file renamed; and the lines it removed and added.
function FileHeader({ file, open, fold }: { file: FileDiff; open: boolean; fold(): void }) {
  const change = CHANGES[file.from !== undefined ? "renamed" : file.status]
  return (
    <div onClick={fold} className="flex h-row-height w-full min-w-0 cursor-pointer items-center gap-1.5 pr-3 pl-2 text-caption text-text">
      <button
        type="button"
        aria-expanded={open}
        aria-label={`${open ? "Fold" : "Unfold"} ${file.path}`}
        onClick={(event) => {
          event.stopPropagation()
          fold()
        }}
        className="flex size-5 shrink-0 items-center justify-center rounded-sm text-text-muted"
      >
        <PierreIcon path={CHEVRON} className={cn("size-icon-sm transition-transform motion-reduce:transition-none", !open && "-rotate-90")} />
      </button>
      <PierreIcon path={change.icon} label={change.label} className={cn("size-icon-sm shrink-0", change.colour)} />
      {file.from !== undefined && (
        <>
          <span title={file.from} className="min-w-0 shrink truncate text-text-muted [direction:rtl]">
            <bdi>{file.from}</bdi>
          </span>
          <PierreIcon path={ARROW} label="renamed to" className="size-icon-sm shrink-0 text-text-muted" />
        </>
      )}
      <span title={file.path} className="min-w-0 flex-1 truncate pl-0.5 text-left [direction:rtl]">
        <bdi>{file.path}</bdi>
      </span>
      <HeaderStat added={file.added} removed={file.removed} />
    </div>
  )
}

// The icons of the library's own file headers, copied from @pierre/diffs
// (Apache-2.0 licence, a copy in LICENSES/pierre-diffs-Apache-2.0.txt):
// the caret, the arrow of a rename, and how a file changed, each named.
const SQUIRCLE =
  "M1.788 4.296c.196-.88.478-1.381.802-1.706s.826-.606 1.706-.802C5.194 1.588 6.387 1.5 8 1.5s2.806.088 3.704.288c.88.196 1.381.478 1.706.802s.607.826.802 1.706c.2.898.288 2.091.288 3.704s-.088 2.806-.288 3.704c-.195.88-.478 1.381-.802 1.706s-.826.607-1.706.802c-.898.2-2.091.288-3.704.288s-2.806-.088-3.704-.288c-.88-.195-1.381-.478-1.706-.802s-.606-.826-.802-1.706C1.588 10.806 1.5 9.613 1.5 8s.088-2.806.288-3.704M8 0C1.412 0 0 1.412 0 8s1.412 8 8 8 8-1.412 8-8-1.412-8-8-8"
const CHEVRON = "M1.47 4.47a.75.75 0 0 1 1.06 0L8 9.94l5.47-5.47a.75.75 0 1 1 1.06 1.06l-6 6a.75.75 0 0 1-1.06 0l-6-6a.75.75 0 0 1 0-1.06"
const ARROW = "M8.47 4.22a.75.75 0 0 0 0 1.06l1.97 1.97H3.75a.75.75 0 0 0 0 1.5h6.69l-1.97 1.97a.75.75 0 1 0 1.06 1.06l3.25-3.25a.75.75 0 0 0 0-1.06L9.53 4.22a.75.75 0 0 0-1.06 0"
const CHANGES: Record<FileDiff["status"] | "renamed", { label: string; colour: string; icon: string }> = {
  added: { label: "Added", colour: "text-diff-added", icon: `M8 4a.75.75 0 0 1 .75.75v2.5h2.5a.75.75 0 0 1 0 1.5h-2.5v2.5a.75.75 0 0 1-1.5 0v-2.5h-2.5a.75.75 0 0 1 0-1.5h2.5v-2.5A.75.75 0 0 1 8 4${SQUIRCLE}` },
  removed: { label: "Removed", colour: "text-diff-removed", icon: `M4 8a.75.75 0 0 1 .75-.75h6.5a.75.75 0 0 1 0 1.5h-6.5A.75.75 0 0 1 4 8${SQUIRCLE}` },
  changed: {
    label: "Modified",
    colour: "text-project-slate",
    icon: "M1.5 8c0 1.613.088 2.806.288 3.704.196.88.478 1.381.802 1.706s.826.607 1.706.802c.898.2 2.091.288 3.704.288s2.806-.088 3.704-.288c.88-.195 1.381-.478 1.706-.802s.607-.826.802-1.706c.2-.898.288-2.091.288-3.704s-.088-2.806-.288-3.704c-.195-.88-.478-1.381-.802-1.706s-.826-.606-1.706-.802C10.806 1.588 9.613 1.5 8 1.5s-2.806.088-3.704.288c-.88.196-1.381.478-1.706.802s-.606.826-.802 1.706C1.588 5.194 1.5 6.387 1.5 8M0 8c0-6.588 1.412-8 8-8s8 1.412 8 8-1.412 8-8 8-8-1.412-8-8m8 3a3 3 0 1 0 0-6 3 3 0 0 0 0 6",
  },
  renamed: {
    label: "Renamed",
    colour: "text-project-slate",
    icon: `${SQUIRCLE}M8.495 4.695a.75.75 0 0 0-.05 1.06L10.486 8l-2.041 2.246a.75.75 0 0 0 1.11 1.008l2.5-2.75a.75.75 0 0 0 0-1.008l-2.5-2.75a.75.75 0 0 0-1.06-.051m-4 0a.75.75 0 0 0-.05 1.06l2.044 2.248-1.796 1.995a.75.75 0 0 0 1.114 1.004l2.25-2.5a.75.75 0 0 0-.002-1.007l-2.5-2.75a.75.75 0 0 0-1.06-.05`,
  },
}

// One of the library's 16px icons, named when `label` is given, and for
// the eye only otherwise.
function PierreIcon({ path, label, className }: { path: string; label?: string; className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" className={className} {...(label !== undefined ? { role: "img", "aria-label": label } : { "aria-hidden": true })}>
      {label !== undefined && <title>{label}</title>}
      <path d={path} />
    </svg>
  )
}

// What a file's header says of its lines: the removed, then the added, as
// T3 Code's, leaving out a side with none, and nothing when both have none.
function HeaderStat({ added, removed }: { added: number; removed: number }) {
  if (added === 0 && removed === 0) return null
  return (
    <span role="group" aria-label={`${added} added, ${removed} removed`} className="inline-flex shrink-0 gap-1.5 font-mono text-2xs tabular-nums">
      {removed > 0 && (
        <span aria-hidden="true" className="text-diff-removed">
          −{removed}
        </span>
      )}
      {added > 0 && (
        <span aria-hidden="true" className="text-diff-added">
          +{added}
        </span>
      )}
    </span>
  )
}

// The files that have no diff to draw, after those that have, each in a
// row as a file's header, saying why.
function Notes({ files }: { files: readonly FileDiff[] }) {
  return (
    <ul aria-label="Files not shown" className="flex flex-col pb-2">
      {files.map((file) => (
        <li key={file.path} data-note-path={file.path} className="flex h-row-height min-w-0 items-center gap-1.5 pr-3 pl-8 text-caption">
          <span title={file.path} className="min-w-0 flex-1 truncate text-text">
            {file.path}
          </span>
          <span className="shrink-0 text-text-muted">{noteOf(file)}</span>
          <HeaderStat added={file.added} removed={file.removed} />
        </li>
      ))}
    </ul>
  )
}

// Why a file has no diff to draw.
function noteOf(file: FileDiff): ReactNode {
  if (file.submodule) return "Submodule changed"
  if (file.tooMany) return "Not compared: one comparison reads at most 300 files"
  if (file.tooSlow) return "Not compared: it took too long"
  if (file.filtered) return "Filtered file, changes not shown"
  if (file.binary) return "Binary file, not shown"
  if (file.tooLarge) return "Too large to show"
  if (file.unknown) return "Changed outside evie, and no copy is kept"
  return file.status === "removed" ? "The file was empty" : "The file is empty"
}
