import { panel } from "./panel.ts"
import { WIDE } from "./wide.ts"

// Where the session list is, and whether it is shown: at desktop width the
// column on the left, which the owner can hide and show, with the button
// at the left of the middle column's header or mod+B, and this browser
// remembers which; at 767px or narrower, at the top of the page, with the
// conversation stacked under it, a screen tall, where the same button and
// keys scroll up to it.
const list = panel({ wide: WIDE, key: "evie.list" })

export const useListShown = list.use

// The list's element on the page, which the page scrolls to.
export const LIST_ID = "session-list"

export function toggleList() {
  if (typeof matchMedia !== "function" || matchMedia(WIDE).matches) list.toggle()
  else document.getElementById(LIST_ID)?.scrollIntoView({ block: "start" })
}
