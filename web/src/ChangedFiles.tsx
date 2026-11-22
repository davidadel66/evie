import { useState } from "react"
import type { ChangedFile } from "../../src/core/types.ts"
import { Button } from "./components/ui/button.tsx"
import { changedFilesLine } from "./diff.ts"
import { openDiff } from "./diff-shown.ts"
import { CaretRight } from "./icons.tsx"

// How many lines were added and removed, as a diff says it: "+12 −4", in
// the diff's own green and red beside the signs that say it too.
export function Counts({ added, removed }: { added: number; removed: number }) {
  return (
    <span className="shrink-0 font-mono text-code whitespace-nowrap">
      <span className="text-diff-added">+{added}</span> <span className="text-diff-removed">−{removed}</span>
      <span className="sr-only">
        {" "}
        lines: {added} added, {removed} removed
      </span>
    </span>
  )
}

// The end of a completed turn that changed files, as T3 Code ends one: one
// quiet line, "3 changed files +12 −4", which opens to list them, and Open
// diff, which opens the diff panel on the turn. A file opens it at that file.
export function TurnChangedFiles({ turn, files }: { turn: number; files: ChangedFile[] }) {
  const [open, setOpen] = useState(false)
  const line = changedFilesLine(files)
  return (
    <div className="flex flex-col">
      <div className="flex min-h-control items-center gap-2">
        <button type="button" aria-expanded={open} onClick={() => setOpen(!open)} className="flex min-h-control min-w-0 items-center gap-2 text-body text-text-muted hover:text-text">
          {line.files}
          <Counts added={line.added} removed={line.removed} />
          <CaretRight className={`size-icon-sm shrink-0 transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
        </button>
        <Button variant="quiet" onClick={() => openDiff({ scope: `turn:${turn}` })} className="ml-auto">
          Open diff
        </Button>
      </div>
      {open && (
        <ul className="flex flex-col border-l border-line pl-3">
          {files.map((file) => (
            <li key={file.path}>
              <button type="button" title={file.path} onClick={() => openDiff({ scope: `turn:${turn}`, file: file.path })} className="flex min-h-control w-full min-w-0 items-center gap-3 text-left">
                <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{file.path}</span>
                {file.status !== "changed" && <span className="shrink-0 text-caption text-text-muted">{file.status === "added" ? "Added" : "Removed"}</span>}
                <Counts added={file.added} removed={file.removed} />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
