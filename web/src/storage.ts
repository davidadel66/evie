import type { Evie, StorageEntry, StorageFile } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { storageShown } from "./shown.ts"

// A workspace's Storage, apart from its screen: the files in its project's
// `.evie` folder, as a tree that starts with every folder open, listed a
// folder at a time, each folder's own folders as it is listed; the owner
// can fold a folder and open it again. And the one file open, read to be
// shown. A file or folder is deleted as the owner asks, once they have seen
// how much goes. The top is listed once connected, and every folder listed is
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
  // How much deleting a file or folder takes, by its path, or nothing when
  // it cannot be told, as when the line is down.
  count(path: string): Promise<Counted | undefined>
  // Deletes a file, or a folder with everything in it, for good: nothing,
  // once it is gone, or why the server refused, in its words.
  remove(path: string): Promise<string | undefined>
  close(): void
}

// How much a delete takes: the files and the folders, a file being one file.
export type Counted = { files: number; folders: number }

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
    async count(path) {
      const evie = clientOf()
      if (evie === undefined) return undefined
      return evie.send({ type: "count_storage", project, path }).catch(() => undefined)
    },
    // Once it is gone, its row goes at once, and its own listings and
    // folds, and those of every folder in it; its folder is listed again,
    // whatever the server said, since it may have changed meanwhile.
    async remove(path) {
      const evie = clientOf()
      if (evie === undefined) return NOT_CONNECTED
      const holder = path.includes("/") ? path.slice(0, path.lastIndexOf("/")) : ""
      try {
        await evie.send({ type: "delete_storage", project, path })
      } catch (cause) {
        if (!closed) list(holder)
        return wordsOf(cause)
      }
      if (closed) return undefined
      const gone = (folder: string) => folder === path || folder.startsWith(`${path}/`)
      const folders = new Map([...state.folders].filter(([folder]) => !gone(folder)))
      const held = folders.get(holder)
      if (held?.entries !== undefined) folders.set(holder, { ...held, entries: held.entries.filter((entry) => entry.path !== path) })
      set({ folders, closed: new Set([...state.closed].filter((folder) => !gone(folder))) })
      list(holder)
      return undefined
    },
    close() {
      closed = true
      unfollow()
    },
  }
}

const NOT_CONNECTED = "Not connected to evie's server yet. Wait until it is connected."
const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))

// What the dialog that asks before a delete says of it, after its path: how
// much goes, once counted; and a warning when what goes is used elsewhere,
// as the files attached to messages and those shown in replies are, which
// those messages then cannot show, and as the workspace's own settings file is.
export function deleteWords(entry: StorageEntry, counted: Counted | undefined): { goes: string; warning?: string } {
  const goes = entry.kind === "file" ? " goes." : counted === undefined ? " goes with everything in it." : countWords(counted)
  const used = ["attachments", "shown"].some((folder) => entry.path === folder || entry.path.startsWith(`${folder}/`))
  if (used) return { goes, warning: `Earlier messages show ${entry.kind === "file" ? "this file" : "files from this folder"}, and will no longer be able to.` }
  if (entry.path === "evie.json") return { goes, warning: "This is the workspace's own settings file. Without it, its backup stops and it loads no plugins of its own." }
  return { goes }
}

function countWords({ files, folders }: Counted): string {
  const parts = [...(files > 0 ? [plural(files, "file")] : []), ...(folders > 0 ? [plural(folders, "folder")] : [])]
  return parts.length === 0 ? " goes. Nothing is in it." : ` goes with everything in it: ${parts.join(" and ")}.`
}

const plural = (count: number, word: string) => `${count} ${word}${count === 1 ? "" : "s"}`
