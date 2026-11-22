import type { ConnectionState } from "./connection.ts"

// How each state of the connection looks: always a word, with a dot beside
// it. Olive is activity; maroon is only for what waits for the owner, which
// signing in does.
const LOOKS: Record<ConnectionState["status"], { word: string; dot: string; text: string }> = {
  connecting: { word: "Connecting", dot: "bg-status-working", text: "text-status-working" },
  connected: { word: "Connected", dot: "bg-accent", text: "text-text-muted" },
  reconnecting: { word: "Reconnecting", dot: "bg-status-working", text: "text-status-working" },
  "signed out": { word: "Signed out", dot: "bg-status-needs", text: "text-status-needs" },
}

// Whether the page is connected to evie's server. Pointed at, it says why
// the line is down, or why the browser was signed out.
export function ConnectionLight({ state }: { state: ConnectionState }) {
  const look = LOOKS[state.status]
  return (
    <span role="status" title={"reason" in state ? state.reason : undefined} className={`flex items-center gap-2 text-caption ${look.text}`}>
      <span aria-hidden="true" className={`size-2 rounded-full ${look.dot}`} />
      {look.word}
    </span>
  )
}
