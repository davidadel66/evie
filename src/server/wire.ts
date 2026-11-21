import type { Command, EvieEvent } from "../core/types.ts"

// What travels between Evie's server and a client. Each WebSocket message is
// one of these, as JSON text. They carry the core's own commands and events
// unchanged, so a client works the same over the network as in one process.

// From a client: a command, with an id the client chose. The reply to it
// names the same id. A command sent again with the same id, after the line
// dropped, is answered with the first one's reply, not carried out again.
export type ClientMessage = { id: string; command: Command }

// From the server: the reply to one command, with what the command returned,
// or the message of the error it failed with; or an event. A command that
// returns nothing has no `result`.
export type ServerMessage = { reply: string; result?: unknown } | { reply: string; error: string } | { event: EvieEvent }

// Where things are on the server. `socket` is the WebSocket. `check` answers
// over plain HTTP whether a token would be let in, since a WebSocket that is
// refused tells the client only that it failed. `login` trades a token for
// a cookie, for a browser.
export const PATHS = { socket: "/ws", check: "/check", login: "/login" } as const

// The close code for a connection whose token was taken back. Codes from
// 4000 are the application's own. A client closed with it stops trying.
export const TAKEN_BACK = 4001

// A host as it is written in an address. An IPv6 address, such as ::1, has
// colons of its own, so it goes in brackets: http://[::1]:3843.
export const hostInAddress = (host: string) => (host.includes(":") && !host.startsWith("[") ? `[${host}]` : host)
