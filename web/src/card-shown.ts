import { useEffect } from "react"
import { panel, type PanelShown } from "./panel.ts"
import { onMac, pressed } from "./shortcuts.ts"

// Where the project card is, and whether it is shown. At desktop width it is
// the column on the right, which the owner can hide and show, with the
// button in the middle column's header or mod+alt+B, and this browser
// remembers which. At 980px or narrower it is a sheet that slides over the
// page from the right, opened from the same button, as T3 Code's right
// panel is at narrow widths.

export type CardShown = PanelShown

const card = panel({ wide: "(min-width: 981px)", key: "evie.card" })

// Where the card is now, redrawn as it changes.
export const useCardShown = card.use

// Shows the card or hides it: the column at desktop width, which this
// browser remembers, or the sheet at phone width.
export const toggleCard = card.toggle
export const closeSheet = card.closeSheet

// mod+alt+B shows and hides the card, wherever the focus is.
export function useCardShortcut() {
  useEffect(() => {
    const mac = onMac()
    const onKey = (event: KeyboardEvent) => {
      if (pressed(event, mac) !== "card") return
      event.preventDefault()
      toggleCard()
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [])
}
