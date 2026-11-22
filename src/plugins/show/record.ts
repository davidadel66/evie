import type { Json } from "../../core/types.ts"

// What the show tool records of the files it showed. Its result is the
// record: the text, written for the model, and the details, for clients,
// which the session's log keeps beside it and never sends to the model.
// The web app draws the files under evie's reply from the details, and the
// terminal names them, and neither reads the text. Nothing here reaches
// the file system, so the web app can load it.

// A file as it was shown: where it is kept, in the project's Storage, by
// its path inside the `.evie` folder, such as "shown/2026-11-22/chart.png",
// or for a session with no project, in the data folder, by its full path;
// the type it is shown as, such as "image/png"; its size, in bytes; and the
// path it was copied from, as the model named it, or none for a file that
// was in Storage already and is shown where it is.
export type ShownFile = { storage: "project" | "session"; path: string; type: string; size: number; from?: string }

// A show call's details: the files it showed, in order.
export type ShowDetails = { files: ShownFile[] }

// The text the model reads: each file by the path it can use, from the
// project's folder or in full, its type and size, and where it came from.
//
//   Shown to the owner under your reply, in this order:
//   - ".evie/shown/2026-11-22/chart.png" (image/png, 33 bytes), copied from out/chart.png
//   - ".evie/files/plan.md" (text/markdown, 120 bytes), already in Storage
export function shownText(files: readonly ShownFile[]): string {
  const told = (file: ShownFile) => (file.storage === "project" ? `.evie/${file.path}` : file.path)
  const lines = files.map((file) => `- ${JSON.stringify(told(file))} (${file.type}, ${sizeIn(file.size)}), ${file.from === undefined ? "already in Storage" : `copied from ${file.from}`}`)
  const copied = files.some((file) => file.from !== undefined)
  return [`Shown to the owner under your reply${files.length === 1 ? ":" : ", in this order:"}`, ...lines, ...(copied ? ["Each copy stays as it is now. To show a later version, call show again."] : [])].join("\n")
}

// The files a show call's details name, in order, as a client reads them
// from the log: none for a call that showed nothing, and any entry not of
// this shape left out.
export function shownFiles(details: Json | undefined): ShownFile[] {
  const files = typeof details === "object" && details !== null && !Array.isArray(details) ? details.files : undefined
  if (!Array.isArray(files)) return []
  return files.flatMap((file): ShownFile[] => {
    if (typeof file !== "object" || file === null || Array.isArray(file)) return []
    const { storage, path, type, size, from } = file
    if ((storage !== "project" && storage !== "session") || typeof path !== "string" || typeof type !== "string" || typeof size !== "number") return []
    return [{ storage, path, type, size, ...(typeof from === "string" && { from }) }]
  })
}

// A size in bytes, in short, as an attachment's is said: 4 bytes, 120 KB, 2.1 MB.
function sizeIn(bytes: number): string {
  if (bytes < 1024) return `${bytes} byte${bytes === 1 ? "" : "s"}`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1).replace(/\.0$/, "")} MB`
}
