import type { Evie, ProjectUsage, UsagePeriod } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// A workspace's usage, apart from its screen: read once connected, and
// again each time the line comes back and whenever the owner asks, since
// nothing tells the page that a reply was made.

export type UsageState = { read?: ProjectUsage; problem?: string }

export type ProjectUsageView = {
  state(): UsageState
  subscribe(listener: () => void): () => void
  refresh(): void
  close(): void
}

export function openProjectUsage(connection: Connection, project: string): ProjectUsageView {
  let state: UsageState = {}
  const listeners = new Set<() => void>()
  let closed = false
  let asked = 0
  let followed: Evie | undefined

  function set(next: UsageState) {
    state = next
    for (const listener of [...listeners]) listener()
  }
  const clientOf = (): Evie | undefined => {
    const now = connection.state()
    return now.status === "connected" ? now.evie : undefined
  }

  function load(evie: Evie) {
    const ask = ++asked
    evie.send({ type: "read_usage", project }).then(
      (read) => !closed && ask === asked && set({ read }),
      (cause: unknown) => !closed && ask === asked && set({ ...state, problem: cause instanceof Error ? cause.message : String(cause) }),
    )
  }

  function follow() {
    const evie = clientOf()
    if (evie === followed) return
    followed = evie
    if (evie !== undefined) load(evie)
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
      const evie = clientOf()
      if (evie !== undefined) load(evie)
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

// A count of tokens in short words: 950, 12.3k, 2.1M.
export function tokenWords(count: number): string {
  if (count < 1000) return String(Math.round(count))
  if (count < 1_000_000) return `${trim(count / 1000)}k`
  return `${trim(count / 1_000_000)}M`
}
// Three figures at most, with no needless ".0".
const trim = (value: number) => (value >= 100 ? String(Math.round(value)) : value.toFixed(value >= 10 ? 1 : 2).replace(/\.?0+$/, ""))

// A cost in dollars: cents once it is a cent, and the first figures after
// the zeros when it is less, so that a small cost is not shown as $0.00.
export function costWords(dollars: number): string {
  if (dollars === 0) return "$0.00"
  if (dollars < 0.01) return `$${dollars.toFixed(6).replace(/(\.\d*?[1-9]\d?)\d*$/, "$1").replace(/0+$/, "")}`
  return `$${dollars.toFixed(2)}`
}

// How much of what was sent was read from the provider's cache, as a
// percentage, or nothing when no tokens were sent.
export function cachedWords(period: UsagePeriod): string | undefined {
  return period.inputTokens === 0 ? undefined : `${Math.round((period.cachedTokens / period.inputTokens) * 100)}%`
}
