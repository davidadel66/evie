import { useEffect, useSyncExternalStore } from "react"
import type { Scope } from "./diff.ts"
import { windowOver } from "./page-requests.ts"
import { onMac, pressed } from "./shortcuts.ts"

// Whether the diff panel is open, what it compares, and the file it was
// opened at, if any; and how it draws a diff, stacked or side by side,
// and whether its file tree shows, which this browser remembers. It opens
// from the project card, from a turn's changed-files line, and with mod+D,
// as T3 Code's does.

export type DiffShown = { open: boolean; scope: Scope; file: string | undefined; layout: "stacked" | "split"; tree: boolean }

// Where this browser keeps how a diff is drawn, and whether the tree shows.
const LAYOUT = "evie.diff-layout"
const TREE = "evie.diff-tree"

// A browser can refuse to keep anything, as in a private window: then the
// diff is stacked and the tree shows.
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

let shown: DiffShown = { open: false, scope: "latest", file: undefined, layout: remembered(LAYOUT) === "split" ? "split" : "stacked", tree: remembered(TREE) !== "hidden" }
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

// Opens the panel, on what to compare and at a file, or as it was left.
export function openDiff(at: { scope?: Scope; file?: string } = {}) {
  set({ open: true, ...(at.scope !== undefined && { scope: at.scope }), file: at.file })
}
export const closeDiff = () => set({ open: false })
export const toggleDiff = () => (shown.open ? closeDiff() : openDiff())
// Compares something else, from its top.
export const compareOther = (scope: Scope) => set({ scope, file: undefined })
// The file it was opened at has been gone to.
export const wentToFile = () => set({ file: undefined })
// Back to the latest turn, as for another session.
export const compareLatest = () => set({ scope: "latest", file: undefined })

export function drawAs(layout: DiffShown["layout"]) {
  set({ layout })
  remember(LAYOUT, layout)
}
export function showTree(tree: boolean) {
  set({ tree })
  remember(TREE, tree ? "shown" : "hidden")
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
