import { cn } from "./lib/utils.ts"

// A folder's path on one line, cut at its start when it is too long, so
// that its end, the folder's own name, shows; the whole path on hover.
export function FolderPath({ folder, className }: { folder: string; className?: string }) {
  return (
    <span title={folder} dir="rtl" className={cn("block min-w-0 truncate font-mono text-code", className)}>
      <bdi dir="ltr">{folder}</bdi>
    </span>
  )
}
