import { useEffect, useSyncExternalStore } from "react"
import type { Scope } from "./diff.ts"
import { windowOver } from "./page-requests.ts"
import { onMac, pressed } from "./shortcuts.ts"

// The right panel, as T3 Code's: whether it is open, and which of its tabs
// shows, the diff or the branch's pull request; what the diff compares, and
// the file it was opened at, if any; and how the diff is drawn, stacked or
// side by side, with long lines wrapped or not, and whether its file tree
// shows, and how wide the panel was made, which this browser remembers. It
// opens from the button in the conversation's header, from a turn's
// changed-files card, from the session details, and with mod+D.

export type PanelTab = "diff" | "pull request"
export type DiffShown = {
  open: boolean
  tab: PanelTab
  scope: Scope
  file: string | undefined
  layout: "stacked" | "split"
  wrap: boolean
  tree: boolean
  // The width the owner dragged it to, undefined until then.
  width: number | undefined
}

// Where this browser keeps how a diff is drawn, whether its lines wrap,
// whether the tree shows, and how wide the panel was made.
const LAYOUT = "evie.diff-layout"
const WRAP = "evie.diff-wrap"
const TREE = "evie.diff-tree"
const WIDTH = "evie.right-panel-width"

// A browser can refuse to keep anything, as in a private window: then the
// diff is stacked, its lines wrap, the tree is hidden, and the panel is as
// wide as at first.
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
const rememberedWidth = Number(remembered(WIDTH))

let shown: DiffShown = {
  open: false,
  tab: "diff",
  scope: "latest",
  file: undefined,
  layout: remembered(LAYOUT) === "split" ? "split" : "stacked",
  wrap: remembered(WRAP) !== "off",
  tree: remembered(TREE) === "shown",
  width: Number.isFinite(rememberedWidth) && rememberedWidth > 0 ? rememberedWidth : undefined,
}
const listeners = new Set<() => void>()
function set(next: Partial<DiffShown>) {
  shown = { ...shown, ...next }
  for (const listener of [...listeners]) listener()
}
function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => void listeners.delete(listener)
}

export const useDiffShown = () => useSyncExternalStore(subscribe, () => shown)

// Opens the panel on the diff, on what to compare and at a file, or as it
// was left.
export function openDiff(at: { scope?: Scope; file?: string } = {}) {
  set({ open: true, tab: "diff", ...(at.scope !== undefined && { scope: at.scope }), file: at.file })
}
export const closeDiff = () => set({ open: false })
// Opens the panel as it was left, or closes it.
export const toggleDiff = () => set({ open: !shown.open })
export const showTab = (tab: PanelTab) => set({ tab })
// Compares something else, from its top.
export const compareOther = (scope: Scope) => set({ scope, file: undefined })
// The file it was opened at has been gone to.
export const wentToFile = () => set({ file: undefined })
// Back to the latest turn, and to the diff, as for another session.
export const compareLatest = () => set({ scope: "latest", file: undefined, tab: "diff" })

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
