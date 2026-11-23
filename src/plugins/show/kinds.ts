// What evie can show the owner, drawn in the conversation and opened from
// Storage: the kinds of file, each with the type it is given to the page
// as and the endings of its name. A kind is read from the file's bytes,
// never from what a name claims: a picture and a PDF by the signature they
// open with, and a kind that is text, such as Markdown, from its name's
// ending once its bytes are text. A kind is added here, by its test, its
// type and its endings, and the page draws it by its type (web/src/shown.ts).
//
// This is the show plugin's list, kept apart from the plugin so that three
// can share it without one copying it: the plugin, which shows only these;
// the server's route that gives Storage files to the page, which gives
// only these, as these types; and the page, which picks a viewer for a file
// in Storage by its ending. So it reaches nothing else: no file system, no
// state.

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

// The kinds of file evie can show, in order: pictures, PDF, SVG, Markdown,
// HTML mockups, Mermaid diagrams and 3D models (glTF, GLB, STL and OBJ).
// Pictures and GLB come first, as their bytes alone decide.
const KINDS: { type: string; endings: readonly string[]; is(name: string, head: Uint8Array): boolean }[] = [
  { type: "image/png", endings: [".png"], is: (_, head) => opens(head, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]) },
  { type: "image/jpeg", endings: [".jpg", ".jpeg"], is: (_, head) => opens(head, [0xff, 0xd8, 0xff]) },
  { type: "image/gif", endings: [".gif"], is: (_, head) => ascii(head, 0, 6) === "GIF87a" || ascii(head, 0, 6) === "GIF89a" },
  { type: "image/webp", endings: [".webp"], is: (_, head) => ascii(head, 0, 4) === "RIFF" && ascii(head, 8, 4) === "WEBP" },
  { type: "application/pdf", endings: [".pdf"], is: (_, head) => ascii(head, 0, 1024).includes("%PDF-") },
  { type: "image/svg+xml", endings: [".svg"], is: (name, head) => endsWith(name, [".svg"]) && textOf(head)?.includes("<svg") === true },
  { type: "text/markdown", endings: [".md", ".markdown"], is: (name, head) => endsWith(name, [".md", ".markdown"]) && textOf(head) !== undefined },
  { type: "text/html", endings: [".html", ".htm"], is: (name, head) => endsWith(name, [".html", ".htm"]) && textOf(head) !== undefined },
  { type: "text/vnd.mermaid", endings: [".mmd", ".mermaid"], is: (name, head) => endsWith(name, [".mmd", ".mermaid"]) && textOf(head) !== undefined },
  { type: "model/gltf-binary", endings: [".glb"], is: (_, head) => ascii(head, 0, 4) === "glTF" },
  { type: "model/gltf+json", endings: [".gltf"], is: (name, head) => endsWith(name, [".gltf"]) && isGltfJson(head) },
  { type: "model/stl", endings: [".stl"], is: (name, head) => endsWith(name, [".stl"]) && isStl(head) },
  { type: "model/obj", endings: [".obj"], is: (name, head) => endsWith(name, [".obj"]) && textOf(head) !== undefined },
]

// Companion files that an HTML mockup or a glTF model may reference by
// relative path and load inside its own preview pass: styles, scripts,
// fonts, images, JSON, WebAssembly, binary buffers and textures.
const COMPANION_TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".htm": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".ico": "image/x-icon",
  ".woff": "font/woff",
  ".woff2": "font/woff2",
  ".ttf": "font/ttf",
  ".otf": "font/otf",
  ".bin": "application/octet-stream",
  ".ktx2": "image/ktx2",
  ".mtl": "text/plain; charset=utf-8",
  ".wasm": "application/wasm",
  ".txt": "text/plain; charset=utf-8",
  ".csv": "text/csv; charset=utf-8",
  ".xml": "application/xml; charset=utf-8",
  ".gltf": "model/gltf+json",
  ".glb": "model/gltf-binary",
  ".stl": "model/stl",
  ".obj": "model/obj",
}

// The HTTP Content-Type for a companion file inside a shown preview folder.
export function companionType(name: string): string | undefined {
  const lower = name.toLowerCase()
  const dot = lower.lastIndexOf(".")
  return dot === -1 ? undefined : COMPANION_TYPES[lower.slice(dot)]
}

// Whether a shown kind can reference relative companion files beside it.
export const hasCompanions = (type: string) => type === "text/html" || type === "model/gltf+json" || type === "model/obj"

// The type a file is shown as, from its name and the opening of its
// bytes, or nothing when it is no kind that can be shown.
export function shownType(name: string, head: Uint8Array): string | undefined {
  return KINDS.find((kind) => kind.is(name, head))?.type
}

// The type a file's name says it is, by its ending, before its bytes are
// read, or nothing for a name of no kind evie shows.
export function typeByName(name: string): string | undefined {
  return KINDS.find((kind) => endsWith(name, kind.endings))?.type
}

// A name with the ending of its kind: as it is when it has one already,
// such as `chart.PNG`, or with the kind's first added, as `photo.jpg`, so
// that whatever reads the kind from the name again, as Storage does, reads
// the same kind.
export function namedFor(name: string, type: string): string {
  const kind = KINDS.find((each) => each.type === type)
  return kind === undefined || endsWith(name, kind.endings) ? name : `${name}${kind.endings[0]}`
}

// The kinds that can be shown, in words for the model, as a refusal says them.
export const SHOWN_KINDS_WORDS =
  "PNG, JPEG, GIF and WebP pictures, SVG, PDF, Markdown (.md), HTML mockups (.html), Mermaid diagrams (.mmd) and 3D models (.glb, .gltf, .stl, .obj)"

const opens = (head: Uint8Array, signature: number[]) => head.byteLength >= signature.length && signature.every((byte, at) => head[at] === byte)
const ascii = (head: Uint8Array, at: number, length: number) => String.fromCharCode(...head.subarray(at, at + length))
const endsWith = (name: string, endings: readonly string[]) => endings.some((ending) => name.toLowerCase().endsWith(ending))

// A glTF JSON file starts as UTF-8 text whose first non-space byte is `{`.
function isGltfJson(head: Uint8Array): boolean {
  const text = textOf(head)
  return text !== undefined && text.trimStart().startsWith("{")
}

// An STL file is either ASCII ("solid ...") or binary (an 80-byte header plus
// a 4-byte triangle count, at least 84 bytes long).
function isStl(head: Uint8Array): boolean {
  if (head.byteLength >= 84) return true
  const text = textOf(head)
  return text !== undefined && text.trimStart().startsWith("solid")
}

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
