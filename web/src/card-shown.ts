import { useEffect, useSyncExternalStore } from "react"
import { isCardShortcut } from "./project-card.ts"

// Where the project card is, and whether it is shown. At desktop width it is
// the column on the right, which the owner can hide and show, with the
// button in the session's header or mod+alt+B, and this browser remembers
// which. At phone width it is a sheet that slides over the page from the
// right, opened from the session's header, as T3 Code's right panel is at
// narrow widths.

export type CardShown = { wide: boolean; column: boolean; sheet: boolean }

// The width from which the columns sit side by side, as `wide` in the styles.
const WIDE = "(min-width: 960px)"
// Where this browser keeps whether the column is hidden.
const KEY = "evie.card"

const media = typeof matchMedia === "function" ? matchMedia(WIDE) : undefined
// A browser can refuse to keep anything, as in a private window, and then
// the column is shown.
function remembered(): boolean {
  try {
    return localStorage.getItem(KEY) !== "hidden"
  } catch {
    return true
  }
}

let shown: CardShown = { wide: media?.matches ?? true, column: remembered(), sheet: false }
const listeners = new Set<() => void>()
function set(next: Partial<CardShown>) {
  shown = { ...shown, ...next }
  for (const listener of [...listeners]) listener()
}
function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => void listeners.delete(listener)
}
// Wide again, the sheet is put away: the column shows as it was left.
media?.addEventListener("change", () => set({ wide: media.matches, ...(media.matches && { sheet: false }) }))

// Where the card is now, redrawn as it changes.
export const useCardShown = () => useSyncExternalStore(subscribe, () => shown)

// Shows the card or hides it: the column at desktop width, which this
// browser remembers, or the sheet at phone width.
export function toggleCard() {
  if (!shown.wide) return set({ sheet: !shown.sheet })
  const column = !shown.column
  set({ column })
  try {
    localStorage.setItem(KEY, column ? "shown" : "hidden")
  } catch {
    // Not remembered: the next visit shows it.
  }
}

export const closeSheet = () => set({ sheet: false })

// mod+alt+B shows and hides the card, wherever the focus is.
export function useCardShortcut() {
  useEffect(() => {
    const mac = /Mac|iPhone|iPad/.test(navigator.userAgent)
    const pressed = (event: KeyboardEvent) => {
      if (!isCardShortcut(event, mac)) return
      event.preventDefault()
      toggleCard()
    }
    addEventListener("keydown", pressed)
    return () => removeEventListener("keydown", pressed)
  }, [])
}
