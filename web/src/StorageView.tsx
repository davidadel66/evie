import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject, StorageEntry } from "../../src/core/types.ts"
import type { Connection } from "./connection.ts"
import { CaretLeft, CaretRight, File, FileImage, FolderSimple } from "./icons.tsx"
import { ageWords } from "./session-list.ts"
import { openStorage, type Storage, type StorageState } from "./storage.ts"
import { addressOfWorkspace, sizeWords, type WorkspacePlace } from "./workspace.ts"

// The workspace's Storage tab: the files in its project's .evie folder, as
// a list of rows, a folder opening in place under its own row, and whether
// the folder is backed up. A file opens in the workspace, read only, in
// place of the list. The files attached to messages are here, in the
// folder's attachments folder.
export function StorageView({ connection, project, place, now }: { connection: Connection; project: ListedProject; place: WorkspacePlace; now: number }) {
  const [storage, setStorage] = useState<{ project: string; storage: Storage }>()
  useEffect(() => {
    const opened = openStorage(connection, project.id)
    setStorage({ project: project.id, storage: opened })
    return () => opened.close()
  }, [connection, project.id])
  if (storage?.project !== project.id) return null
  return <Shown storage={storage.storage} project={project} place={place} now={now} />
}

function Shown({ storage, project, place, now }: { storage: Storage; project: ListedProject; place: WorkspacePlace; now: number }) {
  const state = useSyncExternalStore(storage.subscribe, storage.state)
  useEffect(() => storage.show(place.file), [storage, place.file])
  const { file: _, ...here } = place
  const back = addressOfWorkspace({ ...here, tab: "storage" })
  if (place.file !== undefined) return <FileShown state={state} path={place.file} back={back} now={now} />
  const top = state.folders.get("")
  return (
    <div className="flex flex-col gap-8 px-4 py-6 wide:px-7">
      <section aria-label="Files" className="flex flex-col">
        <div className="flex items-baseline justify-between gap-4 px-3 pb-2 text-caption text-text-muted">
          <h2>Files in .evie</h2>
          <span>{project.syncs ? "Backed up privately" : "Kept on this computer"}</span>
        </div>
        {top?.problem !== undefined ? (
          <Quiet>{top.problem}</Quiet>
        ) : top?.entries === undefined ? (
          <Listing />
        ) : top.entries.length === 0 ? (
          <Quiet>Nothing in the .evie folder yet. What evie keeps for this workspace will be here.</Quiet>
        ) : (
          <ul className="flex flex-col">
            <Entries entries={top.entries} depth={0} state={state} storage={storage} place={place} now={now} />
            <More count={top.more ?? 0} indent={0} />
          </ul>
        )}
      </section>
    </div>
  )
}

// The rows of a folder's entries, each folder's own entries under it while it is open.
function Entries({ entries, depth, state, storage, place, now }: { entries: StorageEntry[]; depth: number; state: StorageState; storage: Storage; place: WorkspacePlace; now: number }) {
  // Each level in by one more step of the icon and its gap.
  const indent = { paddingLeft: `${12 + depth * 24}px` }
  return entries.map((entry) => {
    const details = (
      <span className="flex shrink-0 items-center gap-4 text-caption text-text-muted">
        {entry.kind === "file" && <span className="font-mono text-code">{sizeWords(entry.size)}</span>}
        <span className="min-w-8 text-right">{ageWords(entry.changed, now)}</span>
      </span>
    )
    if (entry.kind === "file") {
      const Icon = /\.(png|jpe?g|gif|webp|svg)$/i.test(entry.name) ? FileImage : File
      return (
        <li key={entry.path}>
          <a href={addressOfWorkspace({ ...place, tab: "storage", file: entry.path })} style={indent} className="flex h-row-height items-center gap-2 border-t border-line pr-3">
            <span className="size-icon-sm shrink-0" />
            <Icon className="size-icon-sm shrink-0 text-text-muted" />
            <span className="min-w-0 flex-1 truncate text-body font-medium text-text">{entry.name}</span>
            {details}
          </a>
        </li>
      )
    }
    const open = state.open.has(entry.path)
    const inside = state.folders.get(entry.path)
    return (
      <li key={entry.path} className="flex flex-col">
        <button type="button" aria-expanded={open} onClick={() => storage.toggle(entry.path)} style={indent} className="flex h-row-height items-center gap-2 border-t border-line pr-3 text-left">
          <CaretRight className={`size-icon-sm shrink-0 text-text-muted transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
          <FolderSimple className="size-icon-sm shrink-0 text-text-muted" />
          <span className="min-w-0 flex-1 truncate text-body font-medium text-text">{entry.name}</span>
          {details}
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

// A file open in the workspace, read only: its path, size and when it
// changed, and its text, or the image, with the way back to the list.
function FileShown({ state, path, back, now }: { state: StorageState; path: string; back: string; now: number }) {
  const file = state.file?.path === path ? state.file : undefined
  const read = file?.read
  return (
    <div className="flex min-w-0 flex-col">
      <div className="flex min-h-row-height items-center gap-2 border-b border-line py-1 pr-4 pl-2 wide:pr-7 wide:pl-4">
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
      <div className="px-4 py-6 wide:px-7">
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
  if (indent === undefined) return <p className="flex h-row-height items-center border-t border-line px-3 text-caption text-text-muted">Listing</p>
  return (
    <li style={{ paddingLeft: `${12 + indent * 24}px` }} className="flex h-row-height items-center border-t border-line text-caption text-text-muted">
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
  if (indent === undefined) return <p className="border-t border-line px-3 pt-3 text-caption break-words text-text-muted">{children}</p>
  return (
    <li style={{ paddingLeft: `${12 + indent * 24}px` }} className="flex min-h-row-height items-center border-t border-line pr-3 text-caption break-words text-text-muted">
      {children}
    </li>
  )
}

// Not yet available, as the design system says it: a quiet badge.
export function Later() {
  return <span className="rounded-sm bg-surface-raised px-2 text-caption text-text-muted">Later</span>
}
