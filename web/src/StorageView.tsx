import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject, StorageEntry } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { CaretLeft, File, FileImage, FolderSimple } from "./icons.tsx"
import { ageWords } from "./session-list.ts"
import { openStorage, type Storage, type StorageState } from "./storage.ts"
import { addressOfWorkspace, sizeWords, type WorkspacePlace } from "./workspace.ts"

// The workspace's Storage tab: the files in its project's .evie folder,
// the files attached to messages among them, in its attachments folder, as
// a tree of rows: each with its icon, its name and how long ago it changed,
// a folder's entries opening in place under its row, a step further in. A
// file opens in the workspace, read only, in place of the tree.
export function StorageView({ connection, project, place, now }: { connection: Connection; project: ListedProject; place: WorkspacePlace; now: number }) {
  const [storage, setStorage] = useState<{ project: string; storage: Storage }>()
  useEffect(() => {
    const opened = openStorage(connection, project.id)
    setStorage({ project: project.id, storage: opened })
    return () => opened.close()
  }, [connection, project.id])
  if (storage?.project !== project.id) return null
  return <Shown storage={storage.storage} place={place} now={now} />
}

function Shown({ storage, place, now }: { storage: Storage; place: WorkspacePlace; now: number }) {
  const state = useSyncExternalStore(storage.subscribe, storage.state)
  useEffect(() => storage.show(place.file), [storage, place.file])
  const { file: _, ...here } = place
  const back = addressOfWorkspace({ ...here, tab: "storage" })
  if (place.file !== undefined) return <FileShown state={state} path={place.file} back={back} now={now} />
  const top = state.folders.get("")
  return (
    <section aria-label="Storage" className="flex flex-col px-gutter py-5">
      {top?.problem !== undefined ? (
        <Quiet>{top.problem}</Quiet>
      ) : top?.entries === undefined ? (
        <Listing />
      ) : top.entries.length === 0 ? (
        <Quiet>Nothing in the .evie folder yet. What evie keeps for this workspace, and the files attached to messages, will be here.</Quiet>
      ) : (
        <ul className="flex flex-col">
          <Entries entries={top.entries} depth={0} state={state} storage={storage} place={place} now={now} />
          <More count={top.more ?? 0} indent={0} />
        </ul>
      )}
    </section>
  )
}

// The rows of a folder's entries, each folder's own entries under it while
// it is open. A folder opens and closes as its row is picked.
function Entries({ entries, depth, state, storage, place, now }: { entries: StorageEntry[]; depth: number; state: StorageState; storage: Storage; place: WorkspacePlace; now: number }) {
  const indent = indentOf(depth)
  return entries.map((entry) => {
    const age = <span className="shrink-0 text-caption text-text-muted">{ageWords(entry.changed, now)}</span>
    if (entry.kind === "file") {
      const Icon = /\.(png|jpe?g|gif|webp|svg)$/i.test(entry.name) ? FileImage : File
      return (
        <li key={entry.path}>
          <a href={addressOfWorkspace({ ...place, tab: "storage", file: entry.path })} style={indent} className="flex h-row-height items-center gap-2 border-t border-line">
            <Icon className="size-icon-sm shrink-0 text-text-muted" />
            <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{entry.name}</span>
            {age}
          </a>
        </li>
      )
    }
    const open = state.open.has(entry.path)
    const inside = state.folders.get(entry.path)
    return (
      <li key={entry.path} className="flex flex-col">
        <button type="button" aria-expanded={open} onClick={() => storage.toggle(entry.path)} style={indent} className="flex h-row-height items-center gap-2 border-t border-line text-left">
          <FolderSimple className="size-icon-sm shrink-0 text-text-muted" />
          <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{entry.name}</span>
          {age}
        </button>
        {open && (
          <ul className="flex flex-col">
            {inside?.problem !== undefined ? (
              <Quiet indent={depth + 1}>{inside.problem}</Quiet>
            ) : inside?.entries === undefined ? (
              <Listing indent={depth + 1} />
            ) : inside.entries.length === 0 ? (
              <Quiet indent={depth + 1}>Nothing in this folder.</Quiet>
            ) : (
              <>
                <Entries entries={inside.entries} depth={depth + 1} state={state} storage={storage} place={place} now={now} />
                <More count={inside.more ?? 0} indent={depth + 1} />
              </>
            )}
          </ul>
        )}
      </li>
    )
  })
}

// Each level in by one step: the icon and its gap, and as much again.
const indentOf = (depth: number) => ({ paddingLeft: `${depth * 24}px` })

// A file open in the workspace, read only: its path, size and when it
// changed, and its text, or the image, with the way back to the list.
function FileShown({ state, path, back, now }: { state: StorageState; path: string; back: string; now: number }) {
  const file = state.file?.path === path ? state.file : undefined
  const read = file?.read
  return (
    <div className="flex min-w-0 flex-col">
      <div className="flex min-h-row-height items-center gap-2 border-b border-line py-1 pr-gutter wide:pl-2">
        <a href={back} className="inline-flex h-control shrink-0 items-center gap-2 rounded-md px-3 text-body text-text-muted hover:text-text">
          <CaretLeft className="size-icon-sm" />
          Storage
        </a>
        <span title={path} className="min-w-0 flex-1 truncate font-mono text-code text-text">
          {path}
        </span>
        {read !== undefined && (
          <span className="shrink-0 text-caption text-text-muted">
            {sizeWords(read.size)} · {ageWords(read.changed, now)}
          </span>
        )}
      </div>
      <div className="px-gutter py-4">
        {file?.problem !== undefined ? (
          <p className="text-body break-words text-text-muted">{file.problem}</p>
        ) : read === undefined ? (
          <p className="flex min-h-control items-center text-body text-text-muted">Reading the file</p>
        ) : read.kind === "image" ? (
          <img src={`data:${read.type};base64,${read.data}`} alt={path} className="max-w-full" />
        ) : (
          <pre className="font-mono text-code break-words whitespace-pre-wrap text-text">{read.text}</pre>
        )}
      </div>
    </div>
  )
}

// A folder being listed, in quiet words: at the top, or as a row of an open folder.
function Listing({ indent }: { indent?: number }) {
  if (indent === undefined) return <p className="text-caption text-text-muted">Listing</p>
  return (
    <li style={indentOf(indent)} className="flex h-row-height items-center border-t border-line text-caption text-text-muted">
      Listing
    </li>
  )
}

// How many more entries a folder holds than its listing gives.
function More({ count, indent }: { count: number; indent: number }) {
  if (count === 0) return null
  return <Quiet indent={indent}>{`${count} more, not listed here.`}</Quiet>
}

function Quiet({ children, indent }: { children: ReactNode; indent?: number }) {
  if (indent === undefined) return <p className="text-caption break-words text-text-muted">{children}</p>
  return (
    <li style={indentOf(indent)} className="flex min-h-row-height items-center border-t border-line text-caption break-words text-text-muted">
      {children}
    </li>
  )
}

// Not yet available, as the design system says it: a quiet badge.
export function Later() {
  return <span className="rounded-sm bg-surface-raised px-2 text-caption text-text-muted">Later</span>
}
