import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { App } from "./App.tsx"
import { addressAsOpened } from "./address.ts"
import { openConnection } from "./connection.ts"
import { openSessions } from "./sessions.ts"

// A sign-in link opens the page with a code after # in its address. The
// address is put back to the page's own at once, keeping only the session
// selected in the list, so that the code is not left in the address bar, a
// bookmark or a screenshot. It works once anyway.
const code = new URLSearchParams(location.hash.slice(1)).get("code") ?? undefined
const opened = addressAsOpened(location.hash)
if (location.pathname + location.hash !== opened) history.replaceState(null, "", opened)

// Kept across hot updates while working on the web app, so that saving a
// file opens no second connection and lists the sessions no second time.
// In the built app, `data` is an empty object, so each is made once, as the
// page loads.
const connection = (import.meta.hot.data.connection ??= openConnection({ url: location.origin, ...(code !== undefined && { code }) }))
const sessions = (import.meta.hot.data.sessions ??= openSessions(connection))
const root = (import.meta.hot.data.root ??= createRoot(document.getElementById("root")!))
root.render(
  <StrictMode>
    <App connection={connection} sessions={sessions} />
  </StrictMode>,
)
