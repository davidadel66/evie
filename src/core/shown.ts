import { join } from "node:path"

// What evie can show the owner, drawn in the conversation and opened from
// Storage: the kinds of file, each with the type it is given to the page
// as. A kind is read from the file's bytes, never from what a name claims:
// a picture and a PDF by the signature they open with, and a kind that is
// text, such as Markdown, from its name's ending once its bytes are text.
// The show tool keeps to this list, and so does the route that gives a
// Storage file to the page, so that nothing else reaches it. A kind is
// added here by its test and its type, and the page draws it by its type.

// The largest file that is shown: 25 MB. The route gives a file from the
// disk as it reads it, so it holds none of it in memory; what sets the
// limit is that every call keeps a new copy in the project's `.evie`
// folder, which the sync plugin backs up to GitHub, which warns at 50 MB a
// file. It is more than the 10 MB a file attached can be, since a PDF
// with pictures, or a 3D model with its textures, is often larger.
export const SHOWN_LIMIT = 25 * 1024 * 1024

// How much of a file is read to tell its kind: enough for a picture's
// signature, a PDF's header, which may come after up to 1 KB of anything,
// and to see that a text file is text and, for SVG, has its <svg> tag.
export const KIND_HEAD = 64 * 1024

// Each kind: its type, and whether a file of this name, opening with these
// bytes, is one. Pictures first, as their bytes alone decide.
const KINDS: { type: string; is(name: string, head: Uint8Array): boolean }[] = [
  { type: "image/png", is: (_, head) => opens(head, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]) },
  { type: "image/jpeg", is: (_, head) => opens(head, [0xff, 0xd8, 0xff]) },
  { type: "image/gif", is: (_, head) => ascii(head, 0, 6) === "GIF87a" || ascii(head, 0, 6) === "GIF89a" },
  { type: "image/webp", is: (_, head) => ascii(head, 0, 4) === "RIFF" && ascii(head, 8, 4) === "WEBP" },
  { type: "application/pdf", is: (_, head) => ascii(head, 0, 1024).includes("%PDF-") },
  { type: "image/svg+xml", is: (name, head) => endsWith(name, ".svg") && textOf(head)?.includes("<svg") === true },
  { type: "text/markdown", is: (name, head) => endsWith(name, ".md", ".markdown") && textOf(head) !== undefined },
]

// The type a file is shown as, from its name and the opening of its
// bytes, or nothing when it is no kind that can be shown.
export function shownType(name: string, head: Uint8Array): string | undefined {
  return KINDS.find((kind) => kind.is(name, head))?.type
}

// The kinds that can be shown, in words for the model, as a refusal says them.
export const SHOWN_KINDS_WORDS = "PNG, JPEG, GIF and WebP pictures, SVG, PDF and Markdown (.md)"

// Where a session with no project keeps what it shows: in the data folder,
// under the session, as `shown/<session>/2026-11-22/chart.png`, deleted
// with it. A session whose project's `.evie` folder Storage would not show
// keeps them there too.
export const ownShownFolder = (dataFolder: string, session: string) => join(dataFolder, "shown", session)

const opens = (head: Uint8Array, signature: number[]) => head.byteLength >= signature.length && signature.every((byte, at) => head[at] === byte)
const ascii = (head: Uint8Array, at: number, length: number) => String.fromCharCode(...head.subarray(at, at + length))
const endsWith = (name: string, ...endings: string[]) => endings.some((ending) => name.toLowerCase().endsWith(ending))

// The opening of a file as text, in UTF-8, or nothing when it is not: a
// zero byte, as a program's files hold, or bytes that are not UTF-8. A
// character cut in two at the end of the opening is not counted against it.
function textOf(head: Uint8Array): string | undefined {
  if (head.includes(0)) return undefined
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(head, { stream: true })
  } catch {
    return undefined
  }
}
