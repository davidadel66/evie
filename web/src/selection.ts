import { useMemo, useSyncExternalStore } from "react"
import { addressOf, selectedIn, type Selected } from "./address.ts"

// What is selected in the list, as the page's address keeps it: a session,
// or a draft of a new one.

// What the address selects now, redrawn as it changes: by a click on a
// row's link, by the page itself, or by going back.
export function useSelected(): Selected | undefined {
  const hash = useAddress()
  return useMemo(() => selectedIn(hash), [hash])
}

// The page's address after #, redrawn as it changes.
export const useAddress = () => useSyncExternalStore(onAddressChange, () => location.hash)
function onAddressChange(listener: () => void) {
  addEventListener("hashchange", listener)
  return () => removeEventListener("hashchange", listener)
}

// Selects a session or a draft, as following its row's link does.
export function select(selected: Selected) {
  location.hash = addressOf(selected)
}
