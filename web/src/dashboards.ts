import type { DashboardEntry, DashboardOpenResult, Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { addressOf as storageAddressOf, previewMintOf } from "./shown.ts"

// A workspace's dashboards, apart from its screen: listed from the project's
// `.evie/evie.json` once connected, listed again whenever the line comes back
// (since nothing pushes config edits), and kept alive while a live dashboard
// is open through a periodic heartbeat.

export const HEARTBEAT_MS = 30_000

export type DashboardsState = {
  listed: boolean
  dashboards: DashboardEntry[]
  problem?: string
}

export type Dashboards = {
  state(): DashboardsState
  subscribe(listener: () => void): () => void
  refresh(): void
  close(): void
}

export function openDashboards(connection: Connection, project: string): Dashboards {
  let state: DashboardsState = { listed: false, dashboards: [] }
  const listeners = new Set<() => void>()
  let closed = false
  let asked = 0

  function set(next: DashboardsState) {
    state = next
    for (const listener of [...listeners]) listener()
  }

  const clientOf = (): Evie | undefined => {
    const now = connection.state()
    return now.status === "connected" ? now.evie : undefined
  }

  function list() {
    const evie = clientOf()
    if (evie === undefined) return
    const ask = ++asked
    evie.send({ type: "list_dashboards", project }).then(
      ({ dashboards }) => {
        if (closed || ask !== asked) return
        set({ listed: true, dashboards })
      },
      (cause: unknown) => {
        if (closed || ask !== asked) return
        set({ listed: true, dashboards: state.dashboards, problem: wordsOf(cause) })
      },
    )
  }

  // List immediately once connected, and re-list every time the connection
  // returns after a drop so changes to `.evie/evie.json` appear.
  let connected: Evie | undefined
  function follow() {
    const evie = clientOf()
    if (evie !== undefined && evie !== connected) {
      connected = evie
      list()
    }
    if (evie === undefined) connected = undefined
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    refresh() {
      list()
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

// The POST URL that mints a scoped preview pass for a static dashboard file
// inside a project's `.evie` folder.
export function dashboardPreviewMintUrl(project: string, path: string): string {
  return previewMintOf(storageAddressOf({ project, path }))
}

// Opens a dashboard over the connection's current client. Returns undefined
// when the connection is not currently connected.
export async function openDashboard(connection: Connection, project: string, id: string): Promise<DashboardOpenResult | undefined> {
  const now = connection.state()
  if (now.status !== "connected") return undefined
  return now.evie.send({ type: "open_dashboard", project, id })
}

export type DashboardHeartbeat = {
  // Sends one touch immediately, as when returning from a backgrounded tab
  // or laptop sleep.
  touchNow(): void
  stop(): void
}

// Keeps a live dashboard's managed process and scoped proxy pass alive while
// it is open. Every `intervalMs` (30s by default), sends `touch_dashboard`.
// When the server answers `{ ok: false }` (such as after laptop sleep expired
// the pass), invokes `onExpired` once and stops itself so the view can re-open.
export function startDashboardHeartbeat(
  connection: Connection,
  target: { project: string; id: string; passId?: string },
  onExpired: () => void,
  intervalMs = HEARTBEAT_MS,
): DashboardHeartbeat {
  let stopped = false
  let inFlight = false

  function touch() {
    if (stopped || inFlight) return
    const now = connection.state()
    if (now.status !== "connected") return
    inFlight = true
    now.evie
      .send({
        type: "touch_dashboard",
        project: target.project,
        id: target.id,
        ...(target.passId !== undefined && { passId: target.passId }),
      })
      .then(
        (res) => {
          inFlight = false
          if (stopped) return
          if (!res.ok) {
            stop()
            onExpired()
          }
        },
        () => {
          inFlight = false
        },
      )
  }

  const timer = setInterval(touch, intervalMs)

  function stop() {
    if (stopped) return
    stopped = true
    clearInterval(timer)
  }

  return { touchNow: touch, stop }
}

const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
