import { createContext } from "react"
import type { ConnectionState } from "./connection.ts"

// How the connection stands, as the page's frame gives it to the views
// inside it, for the line that says the connection is down.
export const ConnectionShown = createContext<ConnectionState | undefined>(undefined)
