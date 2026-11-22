import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { App } from "./App.tsx"
import { openConnection } from "./connection.ts"

// A sign-in link opens the page with a code after # in its address. The
// address is put back to the page's own at once, so that the code is not
// left in the address bar, a bookmark or a screenshot. It works once anyway.
const code = new URLSearchParams(location.hash.slice(1)).get("code") ?? undefined
if (location.hash !== "" || location.pathname !== "/") history.replaceState(null, "", "/")

// Kept across hot updates while working on the web app, so that saving a
// file opens no second connection. In the built app, `data` is an empty
// object, so both are made once, as the page loads.
const connection = (import.meta.hot.data.connection ??= openConnection({ url: location.origin, ...(code !== undefined && { code }) }))
const root = (import.meta.hot.data.root ??= createRoot(document.getElementById("root")!))
root.render(
  <StrictMode>
    <App connection={connection} />
  </StrictMode>,
)
