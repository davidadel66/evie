import { useSyncExternalStore } from "react"
import { addressOf, selectedIn } from "./address.ts"

// The session selected in the list, as the page's address keeps it.

// The session the address selects now, redrawn as it changes: by a click
// on a row's link, by the page itself, or by going back.
export function useSelected(): string | undefined {
  return useSyncExternalStore(onAddressChange, () => selectedIn(location.hash))
}
function onAddressChange(listener: () => void) {
  addEventListener("hashchange", listener)
  return () => removeEventListener("hashchange", listener)
}

// Selects a session, as following its row's link does.
export function select(session: string) {
  location.hash = addressOf(session)
}
