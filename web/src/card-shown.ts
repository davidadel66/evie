import { useEffect, useSyncExternalStore } from "react"
import { diffShown, subscribeShown } from "./diff-shown.ts"
import { windowOver } from "./page-requests.ts"
import { cardPlace, cardRoom, keptChoices, shownFrom, type CardPlace, type DetailsItem, type Shown } from "./project-card.ts"
import { onMac, pressed } from "./shortcuts.ts"
import { WIDE } from "./wide.ts"

// Where the session details are, whether they are open, and what they
// show. They float at the top right of the conversation. Pinned, they stay
// open beside it while it has the room, and over it without the room, and
// this browser remembers whether they were closed; unpinned, or in a column
// too short to pin in, they open over it from the button in its header, or
// with mod+alt+B, and close again on a click outside or Escape, as T3
// Code's details card does. At phone width
// they are a sheet from the right. While the right panel is open, which
// takes the right side, they go behind their header button, and pinned,
// come back when it closes. This browser remembers whether they are pinned
// and what they show. `room` is the height the card has.

export type CardShown = {
  place: CardPlace
  open: boolean
  pinned: boolean
  shown: Shown
  room: number
}

// Where this browser keeps whether the pinned card is closed, whether it
// is unpinned, and the items chosen to show that differ from the default.
const CLOSED = "evie.card"
const PINNED = "evie.card-pinned"
const ITEMS = "evie.card-items"

// A browser can refuse to keep anything, as in a private window: then the
// card is pinned, open and shows everything.
function remembered(key: string): string | null {
  try {
    return localStorage.getItem(key)
  } catch {
    return null
  }
}
function remember(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key)
    else localStorage.setItem(key, value)
  } catch {
    // Not remembered: the next visit starts as the first did.
  }
}

const media = typeof matchMedia === "function" ? matchMedia(WIDE) : undefined

// The conversation's column's width and height, as the card measures them;
// whether the page is a phone's; whether the right panel is open; and
// whether the card is open in each place it can be: pinned, in place or
// over the conversation, or floating.
let state = {
  width: typeof innerWidth === "number" ? innerWidth : 1440,
  height: typeof innerHeight === "number" ? innerHeight : 900,
  phone: media !== undefined && !media.matches,
  panel: diffShown().open,
  pinned: remembered(PINNED) !== "unpinned",
  inline: remembered(CLOSED) !== "hidden",
  popover: false,
  sheet: false,
  shown: shownFrom(remembered(ITEMS)),
}
const placeOf = (now: typeof state) => cardPlace(now)
const snapshotOf = (now: typeof state): CardShown => {
  const place = placeOf(now)
  const open = place === "inline" || place === "over" ? now.inline : place === "popover" ? now.popover : now.sheet
  return { place, open, pinned: now.pinned, shown: now.shown, room: cardRoom(now.height) }
}
let snapshot = snapshotOf(state)
const listeners = new Set<() => void>()

function set(next: Partial<typeof state>) {
  const was = placeOf(state)
  state = { ...state, ...next }
  const place = placeOf(state)
  // Leaving a place puts away what was open over the page there, as T3
  // Code's card does when it finds the room to sit in place.
  if (place !== was) state = { ...state, ...(was === "popover" && { popover: false }), ...(was === "sheet" && { sheet: false }) }
  const now = snapshotOf(state)
  if (now.place === snapshot.place && now.open === snapshot.open && now.pinned === snapshot.pinned && now.shown === snapshot.shown && now.room === snapshot.room) return
  snapshot = now
  for (const listener of [...listeners]) listener()
}
function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => void listeners.delete(listener)
}
media?.addEventListener("change", () => set({ phone: !media.matches }))
subscribeShown(() => set({ panel: diffShown().open }))

// Where the card is now, redrawn as it changes.
export const useCardShown = () => useSyncExternalStore(subscribe, () => snapshot)

// The conversation's column's size, as the card measures it, which decides
// whether a pinned card has the room to sit beside it, and how much of it shows.
export const measured = (width: number, height: number) => set({ width, height })

// Opens the card or closes it, wherever it is now.
export function toggleCard() {
  const place = snapshot.place
  if (place === "inline" || place === "over") {
    set({ inline: !state.inline })
    remember(CLOSED, state.inline ? null : "hidden")
  } else if (place === "popover") set({ popover: !state.popover })
  else set({ sheet: !state.sheet })
}

// Closes the card where it floats over the page from the header, as when
// another session is picked; one pinned stays.
export const closeFloating = () => set({ popover: false, sheet: false })

// Pins the card beside the conversation, or unpins it: unpinned, it floats
// over the conversation, still open, until it is closed; pinned, it is in
// place, open, wherever there is the room.
export function pinCard(pinned: boolean) {
  if (pinned === state.pinned) return
  const open = snapshot.open
  set(pinned ? { pinned, inline: true, popover: open } : { pinned, popover: open })
  remember(PINNED, pinned ? null : "unpinned")
  if (pinned) remember(CLOSED, null)
}

// Shows one of the card's items or hides it, or all of them as they were
// at first. Only what differs from the default is kept.
export function showItem(item: DetailsItem, on: boolean) {
  setShown({ ...state.shown, [item]: on })
}
export const resetItems = () => setShown(shownFrom(null))
function setShown(shown: Shown) {
  set({ shown })
  const kept = keptChoices(shown)
  remember(ITEMS, Object.keys(kept).length === 0 ? null : JSON.stringify(kept))
}

// mod+alt+B opens and closes the card, wherever the focus is, but for a
// window open over the page, such as the settings, while which it waits.
export function useCardShortcut() {
  useEffect(() => {
    const mac = onMac()
    const onKey = (event: KeyboardEvent) => {
      if (pressed(event, mac) !== "card" || windowOver() !== undefined) return
      event.preventDefault()
      toggleCard()
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [])
}
