import type { Evie, ProjectInstructions } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// A workspace's instructions, apart from its screen: what the server says
// of them once connected, read again each time the line comes back, and the
// owner's notes as they are being written. What is written and not yet saved
// is kept until it is saved, whatever is read meanwhile, so that a read after
// a reconnect does not take it away.

export type InstructionsState = {
  // As the server last gave them, or none until they are read.
  read?: ProjectInstructions
  // The notes as written, once they differ from what was read.
  draft?: string
  // Why they could not be read, in the server's words.
  problem?: string
}

export type ProjectInstructionsView = {
  state(): InstructionsState
  subscribe(listener: () => void): () => void
  edit(text: string): void
  // Saves the notes: nothing, once they are kept, or why the server refused,
  // in its words.
  save(): Promise<string | undefined>
  close(): void
}

const NOT_CONNECTED = "evie's server is not connected, so the notes could not be saved. Try again once it is."

export function openProjectInstructions(connection: Connection, project: string): ProjectInstructionsView {
  let state: InstructionsState = {}
  const listeners = new Set<() => void>()
  let closed = false
  let asked = 0
  let followed: Evie | undefined

  function set(next: InstructionsState) {
    state = next
    for (const listener of [...listeners]) listener()
  }
  const clientOf = (): Evie | undefined => {
    const now = connection.state()
    return now.status === "connected" ? now.evie : undefined
  }

  function load(evie: Evie) {
    const ask = ++asked
    evie.send({ type: "read_instructions", project }).then(
      (read) => {
        if (closed || ask !== asked) return
        const { problem: _, draft, ...rest } = state
        // A draft the same as what the server now has is no draft.
        set({ ...rest, read, ...(draft !== undefined && draft !== read.notes && { draft }) })
      },
      (cause: unknown) => !closed && ask === asked && set({ ...state, problem: wordsOf(cause) }),
    )
  }

  function follow() {
    const evie = clientOf()
    if (evie === followed) return
    followed = evie
    if (evie !== undefined) load(evie)
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    edit(text) {
      set({ ...state, draft: text })
    },
    async save() {
      const evie = clientOf()
      if (evie === undefined) return NOT_CONNECTED
      const text = state.draft
      if (text === undefined) return undefined
      try {
        await evie.send({ type: "write_instructions", project, text })
      } catch (cause) {
        return wordsOf(cause)
      }
      // Kept unless it was edited again while it was being sent.
      if (!closed && state.draft === text) {
        const { draft: _, ...rest } = state
        set(rest)
      }
      if (!closed) load(evie)
      return undefined
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
