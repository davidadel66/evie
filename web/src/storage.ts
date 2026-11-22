import type { Evie, StorageEntry, StorageFile } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"

// A workspace's Storage, apart from its screen: the files in its project's
// `.evie` folder, listed a folder at a time as the owner opens folders in
// place, and the one file open, read to be shown. The top is listed once
// connected, and what is open is listed again when the line comes back,
// since nothing tells the page that a file changed meanwhile.

// A folder as listed, with how many more entries it holds than a listing
// gives, or why it could not be listed, or neither while it is being listed.
export type FolderListing = { entries?: StorageEntry[]; more?: number; problem?: string }

export type StorageState = {
  // Each folder listed, by its path in the `.evie` folder, the top as "".
  folders: ReadonlyMap<string, FolderListing>
  // The folders open in place.
  open: ReadonlySet<string>
  // The file being shown, as read, or why it could not be, or neither while it is read.
  file?: { path: string; read?: StorageFile; problem?: string }
}

export type Storage = {
  state(): StorageState
  subscribe(listener: () => void): () => void
  // Opens a folder in place, listing it, or closes it when it is open.
  toggle(folder: string): void
  // Reads a file to be shown, or with nothing shows none.
  show(path: string | undefined): void
  close(): void
}

export function openStorage(connection: Connection, project: string): Storage {
  let state: StorageState = { folders: new Map(), open: new Set() }
  const listeners = new Set<() => void>()
  let closed = false
  function set(next: Partial<StorageState>) {
    state = { ...state, ...next }
    for (const listener of [...listeners]) listener()
  }
  const setFolder = (folder: string, listing: FolderListing) => set({ folders: new Map(state.folders).set(folder, listing) })

  const clientOf = (): Evie | undefined => {
    const now = connection.state()
    return now.status === "connected" ? now.evie : undefined
  }

  // A folder is listed again over what it showed before, which stays until
  // the new list comes.
  function list(folder: string) {
    const evie = clientOf()
    if (evie === undefined) return
    if (!state.folders.has(folder)) setFolder(folder, {})
    evie.send({ type: "list_storage", project, ...(folder !== "" && { folder }) }).then(
      (listed) => !closed && setFolder(folder, listed),
      (cause: unknown) => !closed && setFolder(folder, { problem: wordsOf(cause) }),
    )
  }

  // Only the latest file asked for is shown, however the reads come back.
  let asked = 0
  function read(path: string) {
    const evie = clientOf()
    const ask = ++asked
    set({ file: { path } })
    if (evie === undefined) return
    evie.send({ type: "read_storage", project, path }).then(
      (read) => !closed && ask === asked && set({ file: { path, read } }),
      (cause: unknown) => !closed && ask === asked && set({ file: { path, problem: wordsOf(cause) } }),
    )
  }

  // Each time the page is connected anew: the top, every folder open, and
  // the file shown.
  let connected: Evie | undefined
  function follow() {
    const evie = clientOf()
    if (evie !== undefined && evie !== connected) {
      connected = evie
      for (const folder of ["", ...state.open]) list(folder)
      if (state.file !== undefined) read(state.file.path)
    }
    if (evie === undefined) connected = undefined
  }
  const unfollow = connection.subscribe(follow)
  follow()

  return {
    state: () => state,
    subscribe(listener) {
      listeners.add(listener)
      return () => void listeners.delete(listener)
    },
    toggle(folder) {
      const open = new Set(state.open)
      if (open.delete(folder)) return set({ open })
      set({ open: open.add(folder) })
      list(folder)
    },
    show(path) {
      if (path === undefined) {
        asked++
        const { file: _, ...rest } = state
        state = rest
        for (const listener of [...listeners]) listener()
        return
      }
      read(path)
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
