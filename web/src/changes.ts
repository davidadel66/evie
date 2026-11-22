import type { Commands, Evie } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation } from "./conversation.ts"
import type { Scope } from "./diff.ts"

// What a session changed, as the web app follows it, apart from its
// screens: the turns that changed files and the session's files, listed
// once the session is open, and again each time a turn ends or a note is
// added, as undo adds one when it puts files back; and each comparison the
// diff panel shows, read when it is asked for.

export type Listed = Commands["list_changes"]["output"]
export type ChangesState = { listed: Listed | undefined; problem: string | undefined }

export type Changes = {
  state(): ChangesState
  subscribe(listener: () => void): () => void
  // A comparison's files, with their diffs, or an error saying why it
  // could not be read. `base` is the branch to compare the branch with.
  read(scope: Scope, base?: string): Promise<Commands["read_changes"]["output"]>
}

const NOT_CONNECTED = "Not connected to evie's server."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

function openChanges(connection: Connection, session: string, conversation: Conversation): Changes & { close(): void } {
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

  // What the list was last read for: the client, and the number of the
  // last event that can change it. Read one at a time, and once more after
  // one asked for while another was out.
  let readOn: Evie | undefined
  let readAt = -1
  let listing = false
  let again = false
  async function list() {
    const now = conversation.state()
    const evie = client()
    if (closed || !now.opened || evie === undefined) return
    const at = now.events.findLast((event) => event.type === "turn_end" || event.type === "note")?.seq ?? 0
    if (evie === readOn && at === readAt) return
    if (listing) {
      again = true
      return
    }
    listing = true
    readOn = evie
    readAt = at
    try {
      const listed = await evie.send({ type: "list_changes", session })
      if (!closed) set({ listed, problem: undefined })
    } catch (cause) {
      // Tried again with the next turn's end, or when the line is back.
      readOn = undefined
      if (!closed) set({ problem: `What changed could not be listed: ${wordsOf(cause)}` })
    } finally {
      listing = false
      if (again) {
        again = false
        void list()
      }
    }
  }
  let closed = false
  const unfollow = conversation.subscribe(() => void list())
  const unconnect = connection.subscribe(() => void list())
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
      if (scope === "latest") {
        const latest = state.listed?.latest ?? (await evie.send({ type: "list_changes", session })).latest
        return latest === 0 ? { files: [] } : evie.send({ type: "read_changes", session, of: "turn", turn: latest })
      }
      if (scope.startsWith("turn:")) return evie.send({ type: "read_changes", session, of: "turn", turn: Number(scope.slice("turn:".length)) })
      if (scope === "branch") return evie.send({ type: "read_changes", session, of: "branch", ...(base !== undefined && { base }) })
      return evie.send({ type: "read_changes", session, of: scope as "session" | "uncommitted" })
    },
    close() {
      closed = true
      unfollow()
      unconnect()
    },
  }
}

// A session's changes are followed once on a connection however many parts
// of the page show them: the conversation's changed-files lines, the
// project card and the diff panel. Each holds them while it shows them;
// once none does, they are no longer followed.
const held = new WeakMap<Connection, Map<string, { changes: ReturnType<typeof openChanges>; release(): void; holders: number }>>()

export function holdChanges(connection: Connection, session: string): { changes: Changes; release(): void } {
  const open = held.get(connection) ?? held.set(connection, new Map()).get(connection)!
  let entry = open.get(session)
  if (entry === undefined) {
    const conversation = holdConversation(connection, session)
    entry = { changes: openChanges(connection, session, conversation.conversation), release: conversation.release, holders: 0 }
    open.set(session, entry)
  }
  const kept = entry
  kept.holders += 1
  let released = false
  return {
    changes: kept.changes,
    release() {
      if (released) return
      released = true
      kept.holders -= 1
      if (kept.holders > 0) return
      open.delete(session)
      kept.changes.close()
      kept.release()
    },
  }
}
