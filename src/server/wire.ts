import type { Command, EvieEvent } from "../core/types.ts"

// What travels between Evie's server and a client. Each WebSocket message is
// one of these, as JSON text. They carry the core's own commands and events
// unchanged, so a client works the same over the network as in one process.

// From a client: a command, with an id the client chose, which must be
// unique for its device, such as 128 random bits. The reply to it names the
// same id. A command sent again with the same id, after the line dropped, is
// answered with the first one's reply, not carried out again, while the
// server keeps running: it remembers, in its memory, the replies to the last
// 200 commands of each device, however long ago they were sent, but for
// those that read a workspace's Storage or list a project's files, which
// are read again. `once` says
// the client will never send this command again, as with its own catch-up
// after a drop, whose reply can be large: its reply is not remembered, and
// it takes none of the 200 places.
export type ClientMessage = { id: string; command: Command; once?: true }

// From the server: the reply to one command, with what the command returned,
// or the message of the error it failed with; or an event; or a keepalive,
// which carries nothing and is sent every so often so that a client can tell
// a line that is quiet from one that has died without closing. A command
// that returns nothing has no `result`.
export type ServerMessage = { reply: string; result?: unknown } | { reply: string; error: string } | { event: EvieEvent } | { keepalive: true }

// Where things are on the server. `socket` is the WebSocket. `check` answers
// over plain HTTP whether a token would be let in, since a WebSocket that is
// refused tells the client only that it failed. `login` trades a token for
// a cookie, for a browser, or the code of a sign-in link for one. A device
// with a token asks `signInLink` for such a link, as `evie web` does.
// `page` is the web app, which is also served at `signIn`, the address a
// sign-in link opens. `attachments` takes in a file attached to a message,
// sent as the body of a POST with its type, and named in the address as
// `?session=<id>&name=<file name>`, and gives back the attachment as JSON;
// and gives one back, for `GET ?session=<id>&path=<its path>`.
export const PATHS = {
  socket: "/ws",
  check: "/check",
  login: "/login",
  signInLink: "/sign-in-link",
  page: "/",
  signIn: "/sign-in",
  attachments: "/attachments",
} as const

// The close code for a connection whose token was taken back. Codes from
// 4000 are the application's own. A client closed with it stops trying.
export const TAKEN_BACK = 4001

// Whether a web address, such as a page's, is on the computer it is read
// on: at 127.0.0.1, localhost or ::1. The server and its web page both go
// by it, for what only the computer evie runs on may change.
export const onThisComputer = (address: string) => URL.canParse(address) && ["127.0.0.1", "localhost", "[::1]"].includes(new URL(address).hostname)

// A host as it is written in an address. An IPv6 address, such as ::1, has
// colons of its own, so it goes in brackets: http://[::1]:3843.
export const hostInAddress = (host: string) => (host.includes(":") && !host.startsWith("[") ? `[${host}]` : host)
