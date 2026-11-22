import { useMemo, useSyncExternalStore } from "react"
import { addressOf, selectedIn, settingsIn, withSettings, type Selected } from "./address.ts"
import { SETTINGS_PAGES, type SettingsPage } from "./settings-page.ts"
import { workspaceIn, type WorkspacePlace } from "./workspace.ts"

// What is selected in the list, as the page's address keeps it: a session,
// or a draft of a new one; the workspace open instead of the list; and the
// settings page, open over either.

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

// Where the owner is, when it is in a workspace, redrawn as the address
// changes. Elsewhere, the address is remembered, for the workspace strip's
// way back.
export function useWorkspacePlace(): WorkspacePlace | undefined {
  const hash = useAddress()
  const place = useMemo(() => workspaceIn(hash), [hash])
  if (place === undefined) sessionView = hash === "" ? "#" : hash
  return place
}
// The session view's address as the owner last left it for a workspace,
// or undefined when the page opened in one.
let sessionView: string | undefined
export const sessionViewLeft = () => sessionView

// Selects a session or a draft, as following its row's link does.
export function select(selected: Selected) {
  location.hash = addressOf(selected)
}

// ---- The settings page, as the address keeps it -------------------------------------------

// Opens the settings, on a page, over whatever is open, as a step of the
// page's history, so that Back closes them, as T3 Code's.
export function openSettings(page: SettingsPage = SETTINGS_PAGES[0]) {
  if (settingsIn(location.hash) !== undefined) return showSettingsPage(page)
  location.hash = withSettings(location.hash, page)
  history.replaceState({ ...(history.state as object | null), settingsOpened: true }, "")
}

// Moves to another page of the settings, in place, as T3 Code's pages do.
export const showSettingsPage = (page: SettingsPage) => replaceAddress(withSettings(location.hash, page))

// Closes the settings: back to where they were opened from, or, opened by
// their address, to what is open under them. Asked again before the page
// has gone back, as by Escape pressed twice, it goes back no further.
let goingBack = false
export function closeSettings() {
  if (goingBack) return
  const state = history.state as { settingsOpened?: boolean } | null
  if (state?.settingsOpened !== true) return replaceAddress(withSettings(location.hash, undefined))
  goingBack = true
  history.replaceState({ ...state, settingsOpened: false }, "")
  addEventListener("popstate", () => (goingBack = false), { once: true })
  history.back()
}

// The address changed in place, keeping the step of history it is on, and
// what the page heard of it, as a link followed would tell it.
function replaceAddress(hash: string) {
  history.replaceState(history.state, "", hash === "" ? location.pathname : hash)
  dispatchEvent(new HashChangeEvent("hashchange"))
}
