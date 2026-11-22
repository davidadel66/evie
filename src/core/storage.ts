import { readdir, readFile, realpath, stat } from "node:fs/promises"
import { extname, isAbsolute, join, relative, sep } from "node:path"
import type { StorageEntry, StorageFile } from "./types.ts"

// A workspace's Storage: what its project's `.evie/` folder holds, as a
// client shows it, listed a folder at a time and read a file at a time.
// Nothing outside that folder is listed or read: a path is refused when it
// climbs out of it, is a full path, or leads out through a link.

// The largest file that is read, 1 MB, so that one reply never carries
// more: a server keeps its latest replies to each device for a while.
export const STORAGE_FILE_LIMIT = 1024 * 1024

// The images that are read as such, by their ending, with their type.
const IMAGES: Record<string, string> = {
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".jpeg": "image/jpeg",
  ".gif": "image/gif",
  ".webp": "image/webp",
  ".svg": "image/svg+xml",
}

// The folder's own git repository, which the sync plugin keeps there to
// back it up: it is not one of the owner's files.
const OWN_GIT = ".git"

// What one folder holds, folders first, then files, each by name. `folder`
// is a path inside the `.evie/` folder, such as "files", or nothing for the
// folder itself. A `.evie/` folder not made yet holds nothing. An entry that
// leads out of it, or nowhere, through a link is left out.
export async function listStorage(project: string, folder = ""): Promise<StorageEntry[]> {
  const evie = join(project, ".evie")
  const top = await realOrNothing(evie)
  if (top === undefined && folder === "") return []
  const at = await inside(evie, top, folder, "folder")
  const kind = await stat(at).catch(() => undefined)
  if (!kind?.isDirectory()) throw new Error(`There is no folder ${folder} in this project's .evie folder.`)
  const listed: StorageEntry[] = []
  for (const name of await readdir(at)) {
    if (folder === "" && name === OWN_GIT) continue
    const real = await realOrNothing(join(at, name))
    if (real === undefined || !within(real, top!)) continue
    const facts = await stat(real).catch(() => undefined)
    if (!facts || (!facts.isFile() && !facts.isDirectory())) continue
    const path = folder === "" ? name : `${trimmed(folder)}/${name}`
    const changed = Math.floor(facts.mtimeMs)
    listed.push(facts.isDirectory() ? { name, path, kind: "folder", changed } : { name, path, kind: "file", size: facts.size, changed })
  }
  const byName = (a: StorageEntry, b: StorageEntry) => a.name.localeCompare(b.name, undefined, { sensitivity: "base", numeric: true })
  return [...listed.filter((entry) => entry.kind === "folder").sort(byName), ...listed.filter((entry) => entry.kind === "file").sort(byName)]
}

// One file, as text when it is text, or as an image when its ending names
// one, and no larger than the limit. Anything else is refused, saying why.
export async function readStorage(project: string, path: string): Promise<StorageFile> {
  const evie = join(project, ".evie")
  const at = await inside(evie, await realOrNothing(evie), path, "file")
  const facts = await stat(at).catch(() => undefined)
  if (!facts?.isFile()) throw new Error(`There is no file ${path} in this project's .evie folder.`)
  if (facts.size > STORAGE_FILE_LIMIT) {
    throw new Error(`${path} is larger than the 1 MB Storage shows. Open it on the computer evie runs on instead.`)
  }
  const bytes = await readFile(at)
  const shown = { path: trimmed(path), size: facts.size, changed: Math.floor(facts.mtimeMs) }
  const image = IMAGES[extname(at).toLowerCase()]
  if (image !== undefined) return { ...shown, kind: "image", type: image, data: bytes.toString("base64") }
  const text = asText(bytes)
  if (text === undefined) throw new Error(`${path} is neither text nor an image, so Storage cannot show it.`)
  return { ...shown, kind: "text", text }
}

// The full path of a folder or file named inside the `.evie/` folder, as it
// really is, once it is sure to be inside it. `top` is the `.evie/` folder
// as it really is, or nothing when there is none yet.
async function inside(evie: string, top: string | undefined, path: string, what: "folder" | "file"): Promise<string> {
  if (typeof path !== "string") throw new Error(`\`${what === "folder" ? "folder" : "path"}\` is a path inside the project's .evie folder, such as "files/notes.md".`)
  const outside = new Error(`${path} is outside this project's .evie folder. Storage holds only what is inside it.`)
  if (isAbsolute(path) || !within(join(evie, path), evie)) throw outside
  if (top === undefined) throw new Error(`There is no ${what} ${path} in this project's .evie folder.`)
  const real = await realOrNothing(join(evie, path))
  if (real === undefined) throw new Error(`There is no ${what} ${path} in this project's .evie folder.`)
  if (!within(real, top)) throw outside
  return real
}

// A path with its links resolved, or nothing when it leads nowhere.
const realOrNothing = (path: string) => realpath(path).catch(() => undefined)

// Whether a path is a folder or inside it. A path inside it may have a name
// that starts with two dots, such as `..cache`.
function within(path: string, folder: string): boolean {
  const from = relative(folder, path)
  return from !== ".." && !from.startsWith(`..${sep}`) && !isAbsolute(from)
}

// A path as a list gives it: slashes between its names, none at either end.
const trimmed = (path: string) => path.split(/[\\/]+/).filter((name) => name !== "" && name !== ".").join("/")

// Text in UTF-8, as most text files are: bytes that are not, or that hold
// a zero byte, as a program's files do, are not text.
function asText(bytes: Uint8Array): string | undefined {
  if (bytes.includes(0)) return undefined
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes)
  } catch {
    return undefined
  }
}
