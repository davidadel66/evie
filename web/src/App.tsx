import { useSyncExternalStore } from "react"
import type { Connection } from "./connection.ts"
import { Frame } from "./Frame.tsx"
import { SignIn } from "./SignIn.tsx"

// The web app: the sign-in page while the browser is signed out, and the
// frame of the three columns otherwise, redrawn as the connection changes.
export function App({ connection }: { connection: Connection }) {
  const state = useSyncExternalStore(connection.subscribe, connection.state)
  if (state.status === "signed out") return <SignIn connection={connection} state={state} />
  return <Frame state={state} />
}
