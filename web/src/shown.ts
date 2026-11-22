import { placeOf, type ShownFile } from "../../src/plugins/show/record.ts"
import { PATHS } from "../../src/server/wire.ts"

// What evie showed, as the page draws it, apart from its screen: each file
// the show tool recorded, with its name, the folder it is kept in, and the
// address the page fetches it from; which viewer draws it; and the images
// that full screen moves through.

// A file shown: as the tool recorded it, with its name and its folder as
// the tool told them, such as ".evie/shown/2026-11-22/"; the address it is
// fetched from, which a file in Storage has only once its project is known;
// and its place in its project's Storage, to open it there, when it has one.
export type Shown = ShownFile & { name: string; folder: string; address?: string; storage?: { project: string; path: string } }

// The viewer that draws each type, by the type the server sends the file
// as. A kind is added by its type and its viewer here, and its viewer in
// Viewers.tsx.
export type Viewer = "image" | "svg" | "pdf" | "markdown"
const VIEWERS: Record<string, Viewer> = {
  "image/png": "image",
  "image/jpeg": "image",
  "image/gif": "image",
  "image/webp": "image",
  "image/svg+xml": "svg",
  "application/pdf": "pdf",
  "text/markdown": "markdown",
}
export const viewerOf = (type: string): Viewer | undefined => VIEWERS[type]

// The viewers whose files are images, which full screen moves through.
const IMAGES: readonly Viewer[] = ["image", "svg"]
export const isImage = (file: { type: string }) => IMAGES.includes(viewerOf(file.type)!)

// A file the tool recorded in a session, as the page draws it. A file in
// Storage is fetched from the project the session was in when it was
// shown, or none while that is not known.
export function shownOf(file: ShownFile, session: string, project: string | null | undefined): Shown {
  const cut = file.path.lastIndexOf("/") + 1
  const named = { ...file, name: file.path.slice(cut), folder: file.path.slice(0, cut) }
  const place = placeOf(file)
  if (!("storage" in place)) return { ...named, address: addressOf({ session, path: file.path }) }
  if (project === null || project === undefined) return named
  return { ...named, address: addressOf({ project, path: place.storage }), storage: { project, path: place.storage } }
}

// The address a Storage file is fetched from: one in a project's `.evie`
// folder, by its path inside it, or one kept for a session in the data
// folder, by its full path.
export const addressOf = (asked: { project: string; path: string } | { session: string; path: string }) => `${PATHS.storage}?${new URLSearchParams(asked)}`

// The type a file in Storage is shown as, from its name's ending, before
// it is fetched: the route reads its kind again from its bytes, and gives
// nothing for bytes of no kind evie shows.
const BY_ENDING: Record<string, string> = {
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
  svg: "image/svg+xml",
  pdf: "application/pdf",
  md: "text/markdown",
  markdown: "text/markdown",
}

// A file of a project's Storage, by its path inside the `.evie` folder,
// as Storage opens it with the viewer of its kind, or nothing when its
// name names no kind evie shows, which Storage reads as it reads any file.
export function storageShown(project: string, path: string): Shown | undefined {
  const type = BY_ENDING[path.slice(path.lastIndexOf(".") + 1).toLowerCase()]
  if (type === undefined || !path.includes(".")) return undefined
  return shownOf({ path: `.evie/${path}`, type }, "", project)
}

// Every image shown among what the page draws, in order, for full screen
// to move through.
export function galleryOf(items: readonly { kind: string; file?: Shown }[]): Shown[] {
  return items.flatMap((item) => (item.kind === "shown" && item.file !== undefined && isImage(item.file) ? [item.file] : []))
}
