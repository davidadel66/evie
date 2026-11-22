import type { ConnectionState } from "./connection.ts"

// Whether the page is connected to evie's server, always in a word. Colour
// is kept for what it means elsewhere: connecting and reconnecting look as
// Working does, with a small spinner, and the rest is plain text.
export function ConnectionLight({ state }: { state: ConnectionState }) {
  if (state.status === "connected") {
    return (
      <span role="status" className="flex items-center gap-2 text-caption text-text-muted">
        <span aria-hidden="true" className="size-2 rounded-full bg-text-muted" />
        Connected
      </span>
    )
  }
  if (state.status === "signed out") {
    return (
      <span role="status" className="text-caption text-text">
        Signed out
      </span>
    )
  }
  return (
    <span role="status" className="flex items-center gap-2 text-caption text-status-working">
      <span aria-hidden="true" className="size-3 animate-spin rounded-full border-2 border-current border-t-transparent motion-reduce:animate-none" />
      {state.status === "connecting" ? "Connecting" : "Reconnecting"}
    </span>
  )
}
