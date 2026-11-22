import { cn } from "./lib/utils.ts"

// A folder's path on one line, cut at its start when it is too long, so
// that its end, the folder's own name, shows; the whole path on hover.
// `shown` is the path as written, such as from the home folder, `~/code`.
export function FolderPath({ folder, shown = folder, className }: { folder: string; shown?: string; className?: string }) {
  return (
    <span title={folder} dir="rtl" className={cn("block min-w-0 truncate font-mono text-code", className)}>
      <bdi dir="ltr">{shown}</bdi>
    </span>
  )
}
