import { createContext } from "react"
import { typeByName } from "../../src/plugins/show/kinds.ts"
import type { ShownFile } from "../../src/plugins/show/record.ts"
import { PATHS } from "../../src/server/wire.ts"

// What evie showed, as the page draws it, apart from its screen: each file
// the show tool's details name, with its name, the folder it is kept in,
// and the address the page fetches it from; which viewer draws it; and the
// images that full screen moves through.

// A file shown, as the page draws it: known by `key`, which stays the same
// as the page reads more and draws again; its path and folder as the model
// was told them, such as ".evie/shown/2026-11-22/chart.png"; its type and
// name; the address it is fetched from, which a file in Storage has only
// once its project is known; and its place in its project's Storage, to
// open it there, when it has one.
export type Shown = { key: string; path: string; type: string; name: string; folder: string; address?: string; storage?: { project: string; path: string } }

// The viewer that draws each type, by the type the server sends the file
// as. A kind is added by its type and its viewer here, and its viewer in
// Shown.tsx.
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

// A file the tool recorded in a session, as the page draws it, known by
// `key`. A file in Storage is fetched from the project the session was in
// when it was shown, or none while that is not known.
export function shownOf(file: ShownFile, key: string, session: string, project: string | null | undefined): Shown {
  const told = file.storage === "project" ? `.evie/${file.path}` : file.path
  const cut = told.lastIndexOf("/") + 1
  const named = { key, path: told, type: file.type, name: told.slice(cut), folder: told.slice(0, cut) }
  if (file.storage === "session") return { ...named, address: addressOf({ session, path: file.path }) }
  if (project === null || project === undefined) return named
  return { ...named, address: addressOf({ project, path: file.path }), storage: { project, path: file.path } }
}

// The address a Storage file is fetched from: one in a project's `.evie`
// folder, by its path inside it, or one kept for a session in the data
// folder, by its full path.
export const addressOf = (asked: { project: string; path: string } | { session: string; path: string }) => `${PATHS.storage}?${new URLSearchParams(asked)}`

// A file of a project's Storage, by its path inside the `.evie` folder,
// as Storage opens it with the viewer of its kind, chosen by its name's
// ending, or nothing when its name names no kind evie shows, which Storage
// reads as it reads any file. The route reads its kind again from its
// bytes, and gives nothing for bytes of no kind evie shows.
export function storageShown(project: string, path: string): Shown | undefined {
  const type = typeByName(path)
  return type === undefined ? undefined : shownOf({ storage: "project", path, type, size: 0 }, `storage:${path}`, "", project)
}

// Where full screen stands in the images it moves through, by the key of
// the one it shows, which holds as more are read: its place, counted from
// 1, and how many there are; and the key of the one `by` places away,
// round the ends. One no longer among them stands alone.
export function placeIn(images: readonly Shown[], key: string): { at: number; count: number } {
  const at = images.findIndex((image) => image.key === key)
  return at === -1 ? { at: 1, count: 1 } : { at: at + 1, count: images.length }
}
export function stepFrom(images: readonly Shown[], key: string, by: number): string {
  const at = images.findIndex((image) => image.key === key)
  if (at === -1) return key
  return images[(((at + by) % images.length) + images.length) % images.length]!.key
}

// The images shown in the conversation, in order, which full screen moves
// through, as the conversation gives them to each file's frame. Without
// it, full screen shows the one file.
export const ShownGallery = createContext<readonly Shown[]>([])

// Every image shown among what the page draws, in order, for full screen
// to move through.
export function galleryOf(items: readonly { kind: string; file?: Shown }[]): Shown[] {
  return items.flatMap((item) => (item.kind === "shown" && item.file !== undefined && isImage(item.file) ? [item.file] : []))
}
