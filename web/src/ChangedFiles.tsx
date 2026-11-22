import { useState } from "react"
import type { ChangedFile } from "../../src/core/types.ts"
import { changedFilesLine, filesTree, type TreeRow } from "./diff.ts"
import { openDiff } from "./diff-shown.ts"
import { CaretRight, CaretUpDown, FileText, Folder, FolderOpen, GitDiff } from "./icons.tsx"
import { cn } from "./lib/utils.ts"

// How many lines were added and removed, as T3 Code's diff says it:
// "+12 −4" in mono, the added in green and the removed in red, or muted
// when none were; nothing when no line was. A screen reader hears them in
// words.
export function Stat({ added, removed, className }: { added: number; removed: number; className?: string }) {
  if (added === 0 && removed === 0) return null
  return (
    <span role="group" aria-label={`${added} added, ${removed} removed`} className={cn("inline-flex shrink-0 gap-1 font-mono font-normal whitespace-nowrap tabular-nums", className)}>
      <span aria-hidden="true" className={added > 0 ? "text-diff-added" : "text-text-muted"}>
        +{added}
      </span>
      <span aria-hidden="true" className={removed > 0 ? "text-diff-removed" : "text-text-muted"}>
        −{removed}
      </span>
    </span>
  )
}

// The end of a completed turn that changed files, as T3 Code's
// changed-files card, drawn as the owner's drawing has it: a rounded card
// that says how many files and lines changed, with a button that folds or
// opens all its folders and Open diff, which opens the diff on the turn at
// its first file; and under them the files as a tree, each folder open at
// first. A file opens the diff at that file, and a folder folds.
export function TurnChangedFiles({ turn, files }: { turn: number; files: ChangedFile[] }) {
  const [folded, setFolded] = useState<ReadonlySet<string>>(new Set())
  const rows = filesTree(files, folded)
  const folders = filesTree(files, new Set()).flatMap((row) => (row.kind === "folder" ? [row.path] : []))
  const line = changedFilesLine(files)
  const allOpen = folded.size === 0
  // The diff lists the files in order of their paths, as git does.
  const first = files.map((file) => file.path).sort(byPath)[0]
  return (
    <div className="@container mt-4 rounded-md bg-text/4">
      <div className="flex items-center justify-between gap-2 px-3 py-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-caption font-medium text-text">
          <span>{line.files}</span>
          <Stat added={line.added} removed={line.removed} />
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {folders.length > 0 && (
            <button
              type="button"
              aria-label={allOpen ? "Collapse all folders" : "Expand all folders"}
              title={allOpen ? "Collapse all folders" : "Expand all folders"}
              onClick={() => setFolded(allOpen ? new Set(folders) : new Set())}
              className="flex size-control-xs items-center justify-center rounded-chip text-text-muted hover:bg-text/8 hover:text-text"
            >
              <CaretUpDown className="size-icon-xs" />
            </button>
          )}
          <button
            type="button"
            aria-label="Open diff"
            title="Open the full diff"
            onClick={() => openDiff({ scope: `turn:${turn}`, ...(first !== undefined && { file: first }) })}
            className="flex h-control-xs items-center gap-1.5 rounded-chip px-2 text-caption font-medium text-text-muted hover:bg-text/8 hover:text-text"
          >
            <GitDiff className="size-icon-xs" />
            {/* The word shows where the card has the room, as T3 Code's. */}
            <span className="hidden @[24rem]:inline">Open diff</span>
          </button>
        </div>
      </div>
      <div className="flex flex-col px-2 pb-2">
        <FilesTreeRows
          rows={rows}
          fold={(path) =>
            setFolded((was) => {
              const now = new Set(was)
              if (!now.delete(path)) now.add(path)
              return now
            })
          }
          pick={(path) => openDiff({ scope: `turn:${turn}`, file: path })}
        />
      </div>
    </div>
  )
}

// Paths in the order git lists them, by their characters' codes, so that
// "README.md" comes before "recipes/".
export const byPath = (a: string, b: string) => (a < b ? -1 : a > b ? 1 : 0)

// The rows of a tree of changed files, as T3 Code's card draws them: 28px
// rows, 14px further in at each level; a folder with its caret, its icon,
// its name in small mono and the lines it changed, which folds or opens it;
// a file with its icon, its name and its lines, which `pick` opens. The
// file `current` is marked as the one shown.
export function FilesTreeRows({ rows, fold, pick, current }: { rows: readonly TreeRow[]; fold(path: string): void; pick(path: string): void; current?: string | undefined }) {
  const folders = rows.some((row) => row.kind === "folder")
  return rows.map((row) => {
    const indent = { paddingLeft: `${8 + row.depth * 14}px` }
    const look = "group flex h-control-sm w-full min-w-0 shrink-0 items-center gap-2 rounded-chip pr-2 text-left hover:bg-text/8"
    if (row.kind === "folder") {
      return (
        <button key={`folder:${row.path}`} type="button" aria-expanded={row.open} title={row.path} style={indent} onClick={() => fold(row.path)} className={look}>
          <CaretRight className={cn("size-icon-caret shrink-0 text-text-muted transition-transform motion-reduce:transition-none", row.open && "rotate-90")} />
          {row.open ? <FolderOpen className="size-icon-caret shrink-0 text-text-muted" /> : <Folder className="size-icon-caret shrink-0 text-text-muted" />}
          <span className="min-w-0 truncate font-mono text-2xs text-text-muted group-hover:text-text">{row.name}</span>
          <TreeStat counts={row.counts} />
        </button>
      )
    }
    return (
      <button
        key={`file:${row.path}`}
        type="button"
        title={row.path}
        aria-current={row.path === current ? "true" : undefined}
        style={indent}
        onClick={() => pick(row.path)}
        className={cn(look, row.path === current && "bg-text/12")}
      >
        {folders && <span aria-hidden="true" className="size-icon-caret shrink-0" />}
        <FileText className="size-icon-caret shrink-0 text-text-muted" />
        <span className="min-w-0 truncate font-mono text-caption text-text/85 group-hover:text-text">{row.name}</span>
        <TreeStat counts={row.counts} />
      </button>
    )
  })
}

function TreeStat({ counts }: { counts: { added: number; removed: number } | undefined }) {
  if (counts === undefined || (counts.added === 0 && counts.removed === 0)) return null
  return (
    <span role="group" aria-label={`${counts.added} added, ${counts.removed} removed`} className="ml-auto grid shrink-0 grid-cols-[4ch_4ch] gap-2 text-right font-mono text-3xs tabular-nums">
      <span aria-hidden="true" className={counts.added > 0 ? "text-diff-added" : "text-text-muted"}>
        +{counts.added}
      </span>
      <span aria-hidden="true" className={counts.removed > 0 ? "text-diff-removed" : "text-text-muted"}>
        −{counts.removed}
      </span>
    </span>
  )
}
