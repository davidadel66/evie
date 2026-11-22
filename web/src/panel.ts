import { useSyncExternalStore } from "react"

// A side of the page that is a column at desktop width, which the owner
// can hide and show and this browser remembers, and narrower, a sheet that
// slides over the page, as T3 Code's sidebar and right panel are: the
// session list on the left and the project card on the right.

// Whether the page is wide enough for the column, whether the column is
// shown, and whether the sheet is open.
export type PanelShown = { wide: boolean; column: boolean; sheet: boolean }

export type Panel = {
  // Where it is now, redrawn as it changes.
  use(): PanelShown
  // Shows it or hides it: the column at desktop width, which this browser
  // remembers, or the sheet when narrower.
  toggle(): void
  closeSheet(): void
}

// `wide` is the media query above which it is a column; `key` is where
// this browser keeps whether the column is hidden.
export function panel({ wide, key }: { wide: string; key: string }): Panel {
  const media = typeof matchMedia === "function" ? matchMedia(wide) : undefined
  // A browser can refuse to keep anything, as in a private window, and then
  // the column is shown.
  const remembered = () => {
    try {
      return localStorage.getItem(key) !== "hidden"
    } catch {
      return true
    }
  }
  let shown: PanelShown = { wide: media?.matches ?? true, column: remembered(), sheet: false }
  const listeners = new Set<() => void>()
  const set = (next: Partial<PanelShown>) => {
    shown = { ...shown, ...next }
    for (const listener of [...listeners]) listener()
  }
  const subscribe = (listener: () => void) => {
    listeners.add(listener)
    return () => void listeners.delete(listener)
  }
  // Wide again, the sheet is put away: the column shows as it was left.
  media?.addEventListener("change", () => set({ wide: media.matches, ...(media.matches && { sheet: false }) }))
  return {
    use: () => useSyncExternalStore(subscribe, () => shown),
    toggle() {
      if (!shown.wide) return set({ sheet: !shown.sheet })
      const column = !shown.column
      set({ column })
      try {
        localStorage.setItem(key, column ? "shown" : "hidden")
      } catch {
        // Not remembered: the next visit shows it.
      }
    },
    closeSheet() {
      if (shown.sheet) set({ sheet: false })
    },
  }
}
