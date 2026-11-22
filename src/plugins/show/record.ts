// How the show tool's result records what it showed, in the words the
// model reads, and read back by those that draw it: the web app under
// evie's reply, and the terminal on one line. The result is the record:
// the session's log keeps it as the call's result, and nothing else.
// Nothing here reaches the file system, so the web app can load it.
//
// A result that showed files opens with a line of its own, then names each
// file on a line, in the order shown:
//
//   Shown to the owner under your reply, in this order:
//   - ".evie/shown/2026-11-22/chart.png" (image/png, 33 bytes), copied from out/chart.png
//   - ".evie/files/plan.md" (text/markdown, 120 bytes), already in Storage
//
// The path is in quotes, as JSON writes a string, so that no name can end
// its line or its quotes early. One that starts `.evie/` is from the
// project's folder, in its Storage; any other is a full path, to a file
// kept for the session in the data folder.

// A file as it was shown: where it is kept, as the model was told it, and
// the type it is shown as, such as "image/png".
export type ShownFile = { path: string; type: string }

// What the result says of each file: also its size, in bytes, and either
// the path it was copied from, as the model named it, or that it was in
// Storage already and is shown in place.
export type Showing = ShownFile & { size: number; from?: string }

const OPENING = "Shown to the owner under your reply"
const COPIES = "Each copy stays as it is now. To show a later version, call show again."

export function shownText(files: readonly Showing[]): string {
  const lines = files.map((file) => `- ${JSON.stringify(file.path)} (${file.type}, ${sizeIn(file.size)}), ${file.from === undefined ? "already in Storage" : `copied from ${file.from}`}`)
  const copied = files.some((file) => file.from !== undefined)
  return [`${OPENING}${files.length === 1 ? ":" : ", in this order:"}`, ...lines, ...(copied ? [COPIES] : [])].join("\n")
}

// The files a show call's result names, in order, or none for a result
// that showed nothing, as one that refused.
export function shownIn(result: string): ShownFile[] {
  const [first = "", ...rest] = result.split("\n")
  if (!first.startsWith(OPENING)) return []
  return rest.flatMap((line) => {
    const named = /^- ("(?:[^"\\]|\\.)*") \(([\w.+-]+\/[\w.+-]+), /.exec(line)
    if (named === null) return []
    const path: unknown = JSON.parse(named[1]!)
    return typeof path === "string" ? [{ path, type: named[2]! }] : []
  })
}

// Where a file shown is kept: in the project's Storage, by its path inside
// the `.evie` folder, or in the data folder, by its full path.
export function placeOf(file: ShownFile): { storage: string } | { kept: string } {
  return file.path.startsWith(".evie/") ? { storage: file.path.slice(".evie/".length) } : { kept: file.path }
}

// A size in bytes, in short, as an attachment's is said: 4 bytes, 120 KB, 2.1 MB.
function sizeIn(bytes: number): string {
  if (bytes < 1024) return `${bytes} byte${bytes === 1 ? "" : "s"}`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1).replace(/\.0$/, "")} MB`
}
