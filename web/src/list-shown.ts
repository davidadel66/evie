import { panel } from "./panel.ts"
import { WIDE } from "./wide.ts"

// Where the session list is, and whether it is shown, as T3 Code's sidebar:
// at desktop width the column on the left, which the owner can hide and
// show, with the button at the left of the middle column's header or mod+B,
// and this browser remembers which; at 767px or narrower a sheet that
// slides over the page from the left, opened from the same button, which
// goes once a session is picked in it.
const list = panel({ wide: WIDE, key: "evie.list" })

export const useListShown = list.use
export const toggleList = list.toggle
export const closeListSheet = list.closeSheet
