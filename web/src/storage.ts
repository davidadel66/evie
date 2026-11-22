import type { Evie, StorageEntry, StorageFile } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { storageShown } from "./shown.ts"

// A workspace's Storage, apart from its screen: the files in its project's
// `.evie` folder, as a tree that starts with every folder open, listed a
// folder at a time, each folder's own folders as it is listed; the owner
// can fold a folder and open it again. And the one file open, read to be
// shown. The top is listed once connected, and every folder listed is
// listed again when the line comes back, since nothing tells the page that
// a file changed meanwhile.

// A folder as listed, with how many more entries it holds than a listing
// gives, or why it could not be listed, or neither while it is being listed.
export type FolderListing = { entries?: StorageEntry[]; more?: number; problem?: string }

export type StorageState = {
  // Each folder listed, by its path in the `.evie` folder, the top as "".
  folders: ReadonlyMap<string, FolderListing>
  // The folders the owner has folded.
  closed: ReadonlySet<string>
  // The file being shown: `framed` when it is of a kind evie shows and the
  // server gives it, for the frame it is shown in under a reply to draw
  // itself; or as read, or why it could not be; or none of these while it
  // is looked at.
  file?: { path: string; read?: StorageFile; problem?: string; framed?: true }
}

export type Storage = {
  state(): StorageState
  subscribe(listener: () => void): () => void
  // Folds a folder, or opens it again.
  toggle(folder: string): void
  // Reads a file to be shown, or with nothing shows none.
  show(path: string | undefined): void
  close(): void
}

export function openStorage(connection: Connection, project: string): Storage {
  let state: StorageState = { folders: new Map(), closed: new Set() }
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
  // the new list comes. Then each folder in it not listed yet is listed.
  function list(folder: string) {
    const evie = clientOf()
    if (evie === undefined) return
    if (!state.folders.has(folder)) setFolder(folder, {})
    evie.send({ type: "list_storage", project, ...(folder !== "" && { folder }) }).then(
      (listed) => {
        if (closed) return
        setFolder(folder, listed)
        for (const entry of listed.entries) if (entry.kind === "folder" && !state.folders.has(entry.path)) list(entry.path)
      },
      (cause: unknown) => !closed && setFolder(folder, { problem: wordsOf(cause) }),
    )
  }

  // Only the latest file asked for is shown, however the reads come back.
  // A file of a kind evie shows, which the server gives, is drawn by its
  // frame, which fetches it whole, so it is not read here too; one it does
  // not give, as when its bytes are not of the kind its name says, is read
  // as any file is.
  let asked = 0
  function read(path: string) {
    const ask = ++asked
    set({ file: { path } })
    const shown = storageShown(project, path)
    if (shown?.address === undefined) return readOver(path, ask)
    connection.fetchFile(shown.address, "HEAD").then(
      (answer) => !closed && ask === asked && (answer.ok ? set({ file: { path, framed: true } }) : readOver(path, ask)),
      () => !closed && ask === asked && readOver(path, ask),
    )
  }
  function readOver(path: string, ask: number) {
    const evie = clientOf()
    if (evie === undefined) return
    evie.send({ type: "read_storage", project, path }).then(
      (read) => !closed && ask === asked && set({ file: { path, read } }),
      (cause: unknown) => !closed && ask === asked && set({ file: { path, problem: wordsOf(cause) } }),
    )
  }

  // Each time the page is connected anew: the top, every folder listed, and
  // the file shown.
  let connected: Evie | undefined
  function follow() {
    const evie = clientOf()
    if (evie !== undefined && evie !== connected) {
      connected = evie
      for (const folder of new Set(["", ...state.folders.keys()])) list(folder)
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
      const folded = new Set(state.closed)
      if (!folded.delete(folder)) folded.add(folder)
      set({ closed: folded })
      if (!folded.has(folder) && !state.folders.has(folder)) list(folder)
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
