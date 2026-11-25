import type { DashboardEntry, DashboardOpened, Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// A workspace's dashboards, apart from its screen: listed from its
// project's settings once connected, and again each time the line comes
// back, since nothing tells the page of an edit to them. And what a live
// dashboard's tab shows, as it opens the dashboard and asks for it again
// while it is open.

export type DashboardsState = { listed: boolean; dashboards: DashboardEntry[]; problem?: string }

export type Dashboards = {
  state(): DashboardsState
  subscribe(listener: () => void): () => void
  close(): void
}

export function openDashboards(connection: Connection, project: string): Dashboards {
  let state: DashboardsState = { listed: false, dashboards: [] }
  const listeners = new Set<() => void>()
  let closed = false
  let asked = 0
  let listedOver: Evie | undefined

  function set(next: DashboardsState) {
    state = next
    for (const listener of [...listeners]) listener()
  }

  function follow() {
    const now = connection.state()
    const evie = now.status === "connected" ? now.evie : undefined
    if (evie === listedOver) return
    listedOver = evie
    if (evie === undefined) return
    const ask = ++asked
    evie.send({ type: "list_dashboards", project }).then(
      ({ dashboards }) => !closed && ask === asked && set({ listed: true, dashboards }),
      (cause: unknown) => !closed && ask === asked && set({ listed: true, dashboards: state.dashboards, problem: cause instanceof Error ? cause.message : String(cause) }),
    )
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

// Opens a live dashboard through the server, with this browser's sign-in
// cookie: its app is started if need be, and the page is given the address
// its frame reaches it at. Asked again while the tab is open, it keeps the
// app running and the address working.
export async function openLive(project: string, id: string): Promise<DashboardOpened> {
  try {
    const answer = await fetch(`/dashboard?${new URLSearchParams({ project, id })}`, { method: "POST" })
    if (!answer.ok) return { status: "failed", message: (await answer.text()) || `evie's server answered ${answer.status}.` }
    return (await answer.json()) as DashboardOpened
  } catch (cause) {
    return { status: "failed", message: `evie's server could not be reached: ${cause instanceof Error ? cause.message : String(cause)}` }
  }
}

// What a live dashboard's tab shows: nothing yet while it opens, its page,
// or why it could not be opened. `load` counts the times the page has been
// loaded from the start, so that the frame loads it again when it changes.
export type LiveShown = { status: "opening" } | { status: "ready"; url: string; load: number } | Exclude<DashboardOpened, { status: "ready" }>

// What the tab shows once the dashboard has been opened, or asked for again.
// The page stays as it is while its app keeps running. It is loaded again
// when its app had to be started again, as after it crashed or was stopped
// to make room for another, since the page drawn is of the app that stopped.
export function afterOpening(shown: LiveShown, opened: DashboardOpened): LiveShown {
  if (opened.status !== "ready") return opened
  if (shown.status !== "ready") return { status: "ready", url: opened.url, load: 0 }
  if (!opened.started && opened.url === shown.url) return shown
  return { status: "ready", url: opened.url, load: shown.load + 1 }
}
