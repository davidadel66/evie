// What the command palette asks of a part of the page that keeps its own
// state: the session list to start a new session where it would, the
// message box to open its model picker, the conversation's header to
// rename its session, and the settings window to open. Each part
// listens while it is on the page; a request no part is listening for does
// nothing, and `ask` says so.

export type PageRequest = "new session" | "pick model" | "rename" | "settings"

const listening = new Map<PageRequest, Set<() => void>>()

export function ask(request: PageRequest): boolean {
  const listeners = [...(listening.get(request) ?? [])]
  for (const listener of listeners) listener()
  return listeners.length > 0
}

// Calls `then` each time `request` is asked, until the function returned is called.
export function whenAsked(request: PageRequest, then: () => void): () => void {
  const listeners = listening.get(request) ?? listening.set(request, new Set()).get(request)!
  listeners.add(then)
  return () => void listeners.delete(then)
}

// The window open over the page, if one is: the command palette, or
// another, such as the settings or a question whether to delete, while
// which the page's own shortcuts wait. A sheet is not one.
export function windowOver(): "palette" | "other" | undefined {
  if (document.querySelector('[data-slot="dialog-content"][data-state="open"]') !== null) return "other"
  if (document.querySelector('[data-slot="palette"][data-state="open"]') !== null) return "palette"
  return undefined
}
