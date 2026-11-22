import { useSyncExternalStore } from "react"
import type { Connection } from "./connection.ts"
import { Frame } from "./Frame.tsx"
import type { Sessions } from "./sessions.ts"
import { SignIn } from "./SignIn.tsx"

// The web app: the sign-in page while the browser is signed out, and the
// frame of the three columns otherwise, with the list of sessions, redrawn
// as the connection changes.
// Until the server's first answer it is a blank page, so that a browser not
// signed in does not see the frame for a moment before the sign-in page.
export function App({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const state = useSyncExternalStore(connection.subscribe, connection.state)
  if (state.status === "connecting" && state.reason === undefined) return <div className="min-h-dvh bg-bg" />
  if (state.status === "signed out") return <SignIn connection={connection} state={state} />
  return <Frame state={state} connection={connection} sessions={sessions} />
}
