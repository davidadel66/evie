import { useEffect, useSyncExternalStore } from "react"
import type { Folding, Scope } from "./diff.ts"
import { windowOver } from "./page-requests.ts"
import { onMac, pressed } from "./shortcuts.ts"

// The right panel, as T3 Code's: whether it is open, and which of its tabs
// shows, the diff or the branch's pull request; what the diff compares, and
// the file it was opened at, or whether at its first file with a diff; and
// how the diff is drawn, stacked or side by side, with long lines wrapped
// or not, whether its file tree shows, and how wide the panel and the tree
// were made, which this browser remembers. It opens from the button in the
// conversation's header, from a turn's changed-files card, from the session
// details, and with mod+D.

export type PanelTab = "diff" | "pull request"
export type DiffShown = {
  open: boolean
  tab: PanelTab
  scope: Scope
  file: string | undefined
  first: boolean
  layout: "stacked" | "split"
  wrap: boolean
  tree: boolean
  // The widths the owner dragged the panel and the tree to, undefined until then.
  width: number | undefined
  treeWidth: number | undefined
}

// Where this browser keeps how a diff is drawn, whether its lines wrap,
// whether the tree shows, and how wide the panel and the tree were made.
const LAYOUT = "evie.diff-layout"
const WRAP = "evie.diff-wrap"
const TREE = "evie.diff-tree"
const WIDTH = "evie.right-panel-width"
const TREE_WIDTH = "evie.diff-tree-width"

// A browser can refuse to keep anything, as in a private window: then the
// diff is stacked, its lines wrap, the tree is hidden, and the panel and
// the tree are as wide as at first.
function remembered(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function remember(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // Not remembered: the next visit starts as the first did.
  }
}
function rememberedWidth(key: string): number | undefined {
  const width = Number(remembered(key))
  return Number.isFinite(width) && width > 0 ? width : undefined
}

let shown: DiffShown = {
  open: false,
  tab: "diff",
  scope: "latest",
  file: undefined,
  first: false,
  layout: remembered(LAYOUT) === "split" ? "split" : "stacked",
  wrap: remembered(WRAP) !== "off",
  tree: remembered(TREE) === "shown",
  width: rememberedWidth(WIDTH),
  treeWidth: rememberedWidth(TREE_WIDTH),
}
const listeners = new Set<() => void>()
function set(next: Partial<DiffShown>) {
  shown = { ...shown, ...next }
  for (const listener of [...listeners]) listener()
}
export function subscribeShown(listener: () => void) {
  listeners.add(listener)
  return () => void listeners.delete(listener)
}
export const diffShown = () => shown

export const useDiffShown = () => useSyncExternalStore(subscribeShown, diffShown)

// Opens the panel on the diff, on what to compare, and at a file, or at
// its first file with a diff, or as it was left.
export function openDiff(at: { scope?: Scope; file?: string; first?: boolean } = {}) {
  set({ open: true, tab: "diff", ...(at.scope !== undefined && { scope: at.scope }), file: at.file, first: at.first === true })
}
// Closed, it forgets the file it was opened at.
export const closeDiff = () => set({ open: false, file: undefined, first: false })
// Opens the panel as it was left, or closes it.
export const toggleDiff = () => (shown.open ? closeDiff() : set({ open: true }))
export const showTab = (tab: PanelTab) => set({ tab })
// Compares something else, as it was left there.
export const compareOther = (scope: Scope) => set({ scope, file: undefined, first: false })
// The file it was opened at has been gone to.
export const wentToFile = () => set({ file: undefined, first: false })
// Back to the latest turn, and to the diff, as for another session.
export const compareLatest = () => set({ scope: "latest", file: undefined, first: false, tab: "diff" })

export function drawAs(layout: DiffShown["layout"]) {
  set({ layout })
  remember(LAYOUT, layout)
}
export function wrapLines(wrap: boolean) {
  set({ wrap })
  remember(WRAP, wrap ? "on" : "off")
}
export function showTree(tree: boolean) {
  set({ tree })
  remember(TREE, tree ? "shown" : "hidden")
}
export function widenTo(width: number) {
  set({ width })
  remember(WIDTH, String(Math.round(width)))
}
export function widenTreeTo(treeWidth: number) {
  set({ treeWidth })
  remember(TREE_WIDTH, String(Math.round(treeWidth)))
}

// How the owner left each comparison, as T3 Code keeps it: which files are
// folded, how far the diff was scrolled, and which of the tree's folders
// are folded, by the session and what it compares, while the page is open,
// across switching what is compared and closing the panel. The latest are
// kept, and the oldest forgotten past this many.
export type DiffView = Folding & { scroll: number; tree: ReadonlySet<string> }
const VIEWS_KEPT = 50
const views = new Map<string, DiffView>()
export const viewOf = (key: string) => views.get(key)
export function keepView(key: string, view: DiffView) {
  views.delete(key)
  views.set(key, view)
  if (views.size > VIEWS_KEPT) views.delete(views.keys().next().value!)
}

// mod+D opens and closes the panel, wherever the focus is, but for a
// window open over the page, such as the settings, while which it waits.
export function useDiffShortcut() {
  useEffect(() => {
    const mac = onMac()
    const onKey = (event: KeyboardEvent) => {
      if (pressed(event, mac) !== "diff" || windowOver() !== undefined) return
      event.preventDefault()
      toggleDiff()
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [])
}
