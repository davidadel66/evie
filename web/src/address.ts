// The page's address: what is selected in the list is kept after # in it,
// so that a reload, a bookmark or a second tab opens on it. That is a
// session, as `#session=<id>`, or a new session not yet started, a draft,
// as `#draft=<project id>`, or `#draft=none` for one with no project. A
// draft is made in the browser only: the session is made on the server
// when its first message is sent. A sign-in link brings a code there
// instead, which is taken out as soon as the page opens. With a session, a
// subagent of it whose conversation is open in the middle is kept beside
// it, as `&subagent=<id>`.

export type Selected = { session: string; subagent?: string } | { draft: string | null }

// What an address's part after # selects, or undefined for nothing.
export function selectedIn(hash: string): Selected | undefined {
  const named = new URLSearchParams(hash.replace(/^#/, ""))
  const session = named.get("session")
  const subagent = named.get("subagent")
  if (session !== null && session !== "") return { session, ...(subagent !== null && subagent !== "" && { subagent }) }
  const draft = named.get("draft")
  if (draft !== null && draft !== "") return { draft: draft === NONE ? null : draft }
  return undefined
}

// The part after # that selects a session or a draft.
export function addressOf(selected: Selected): string {
  if ("session" in selected) return `#${new URLSearchParams({ session: selected.session, ...(selected.subagent !== undefined && { subagent: selected.subagent }) })}`
  return `#${new URLSearchParams({ draft: selected.draft ?? NONE })}`
}
// A draft with no project. No project's id can be this: ids are UUIDs.
const NONE = "none"

// The address the page is put at as it opens, whatever path it was opened
// at, such as /sign-in, from its part after #: its root, keeping only what
// is selected, so that a sign-in code is not left in the address bar, a
// bookmark or a screenshot.
export function addressAsOpened(hash: string): string {
  const selected = selectedIn(hash)
  return `/${selected === undefined ? "" : addressOf(selected)}`
}
