// The page's address: the session selected in the list is kept after # in
// it, as `#session=<id>`, so that a reload, a bookmark or a second tab opens
// on the same session. A sign-in link brings a code there instead, which is
// taken out as soon as the page opens.

// The session an address's part after # selects, or undefined for none.
export function selectedIn(hash: string): string | undefined {
  const session = new URLSearchParams(hash.replace(/^#/, "")).get("session")
  return session === null || session === "" ? undefined : session
}

// The part after # that selects a session.
export const addressOf = (session: string) => `#${new URLSearchParams({ session })}`

// The address the page is put at as it opens, whatever path it was opened
// at, such as /sign-in, from its part after #: its root, keeping only the
// selected session, so that a sign-in code is not left in the address bar,
// a bookmark or a screenshot.
export function addressAsOpened(hash: string): string {
  const session = selectedIn(hash)
  return `/${session === undefined ? "" : addressOf(session)}`
}
