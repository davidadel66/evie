import type { Commands, Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation } from "./conversation.ts"
import type { Scope } from "./diff.ts"

// What a session changed, as the web app follows it, apart from its
// screens: the turns that changed files and the session's files, listed
// once the session is open, and again each time a turn ends or a note is
// added, as undo adds one when it puts files back; and each comparison the
// diff panel shows, and a file's whole sides in it, read when asked for.

export type Listed = Commands["list_changes"]["output"]
export type ChangesState = { listed: Listed | undefined; problem: string | undefined }

export type Changes = {
  state(): ChangesState
  subscribe(listener: () => void): () => void
  // A comparison's files, with their diffs, or an error saying why it
  // could not be read. `base` is the branch to compare the branch with.
  read(scope: Scope, base?: string): Promise<Commands["read_changes"]["output"]>
  // One file's whole sides, before and after, as the same comparison has
  // them, by its path and, for a file renamed, the path it had before.
  sides(scope: Scope, path: string, from?: string, base?: string): Promise<Commands["read_file_sides"]["output"]>
}

const NOT_CONNECTED = "Not connected to evie's server."
type Comparison = { of: "turn"; turn: number } | { of: "session" | "uncommitted" } | { of: "branch"; base?: string }
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

function openChanges(connection: Connection, session: string, conversation: Conversation): Changes & { close(): void; whole(wanted: boolean): void } {
  let state: ChangesState = { listed: undefined, problem: undefined }
  const listeners = new Set<() => void>()
  const set = (next: Partial<ChangesState>) => {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }
  const client = (): Evie | undefined => {
    const now = connection.state()
    return now.status === "connected" ? now.evie : undefined
  }

  // What the list was last asked for: on which client, at the number of
  // the last event that can change it, and whether with the whole
  // session's files. It is asked for again only when one of them changes,
  // or the line comes back, whether or not it could be read: one that
  // failed is not asked for again as each word of a reply streams in.
  // Read one at a time, and once more after one asked for while another
  // was out.
  let readOn: Evie | undefined
  let readAt = -1
  let readWhole = false
  let wantWhole = false
  let listing = false
  let again = false
  async function list() {
    const now = conversation.state()
    const evie = client()
    if (closed || !now.opened || evie === undefined) return
    const at = now.events.findLast((event) => event.type === "turn_end" || event.type === "note")?.seq ?? 0
    if (evie === readOn && at === readAt && (readWhole || !wantWhole)) return
    if (listing) {
      again = true
      return
    }
    listing = true
    readOn = evie
    readAt = at
    readWhole = wantWhole
    try {
      const listed = await evie.send({ type: "list_changes", session, ...(wantWhole && { whole: true }) })
      if (!closed) set({ listed, problem: undefined })
    } catch (cause) {
      // What was listed before is kept, and the list is asked for again
      // at the next turn's end, or when the line is back.
      if (!closed) set({ problem: `What changed could not be listed: ${wordsOf(cause)}` })
    } finally {
      listing = false
      if (again) {
        again = false
        void list()
      }
    }
  }
  // What a comparison is, as the server's commands name it: the latest
  // turn is the session's last, as the list says it, and undefined before
  // the session has one.
  async function comparison(evie: Evie, scope: Scope, base: string | undefined): Promise<Comparison | undefined> {
    if (scope === "latest") {
      const latest = state.listed?.latest ?? (await evie.send({ type: "list_changes", session })).latest
      return latest === 0 ? undefined : { of: "turn", turn: latest }
    }
    if (scope.startsWith("turn:")) return { of: "turn", turn: Number(scope.slice("turn:".length)) }
    if (scope === "branch") return { of: "branch", ...(base !== undefined && { base }) }
    return { of: scope as "session" | "uncommitted" }
  }
  let closed = false
  let status = connection.state().status
  const unfollow = conversation.subscribe(() => void list())
  const unconnect = connection.subscribe(() => {
    const now = connection.state().status
    // Back after a drop: what was missed is listed again.
    if (now === "connected" && status !== "connected") readOn = undefined
    status = now
    void list()
  })
  void list()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    async read(scope, base) {
      const evie = client()
      if (evie === undefined) throw new Error(NOT_CONNECTED)
      const of = await comparison(evie, scope, base)
      return of === undefined ? { files: [] } : evie.send({ type: "read_changes", session, ...of })
    },
    async sides(scope, path, from, base) {
      const evie = client()
      if (evie === undefined) throw new Error(NOT_CONNECTED)
      const of = await comparison(evie, scope, base)
      if (of === undefined) return { unavailable: `${path} is not among the files this comparison changed.` }
      return evie.send({ type: "read_file_sides", session, ...of, path, ...(from !== undefined && { from }) })
    },
    close() {
      closed = true
      unfollow()
      unconnect()
    },
    whole(wanted) {
      wantWhole = wanted
      void list()
    },
  }
}

// A session's changes are followed once on a connection however many parts
// of the page show them: the conversation's changed-files lines, the
// project card and the diff panel. Each holds them while it shows them;
// once none does, they are no longer followed. The whole session's files
// are listed while a holder asks for them (`whole`), as the card does.
const held = new WeakMap<Connection, Map<string, { changes: ReturnType<typeof openChanges>; release(): void; holders: number; whole: number }>>()

export function holdChanges(connection: Connection, session: string, options: { whole?: boolean } = {}): { changes: Changes; release(): void } {
  const open = held.get(connection) ?? held.set(connection, new Map()).get(connection)!
  let entry = open.get(session)
  if (entry === undefined) {
    const conversation = holdConversation(connection, session)
    entry = { changes: openChanges(connection, session, conversation.conversation), release: conversation.release, holders: 0, whole: 0 }
    open.set(session, entry)
  }
  const kept = entry
  kept.holders += 1
  if (options.whole && ++kept.whole === 1) kept.changes.whole(true)
  let released = false
  return {
    changes: kept.changes,
    release() {
      if (released) return
      released = true
      kept.holders -= 1
      if (options.whole && --kept.whole === 0) kept.changes.whole(false)
      if (kept.holders > 0) return
      open.delete(session)
      kept.changes.close()
      kept.release()
    },
  }
}
