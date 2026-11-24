import { realpathSync } from "node:fs"
import { readdir, readFile, realpath, stat } from "node:fs/promises"
import { extname, isAbsolute, join, relative, resolve, sep } from "node:path"
import type { StorageEntry, StorageFile } from "./types.ts"

// A workspace's Storage: what its project's `.evie/` folder holds, as a
// client shows it, listed a folder at a time and read a file at a time.
// Nothing outside that folder is listed or read: a path is refused when it
// climbs out of it, is a full path, or leads out through a link. A `.evie`
// that is itself a link is refused as a whole, since where it leads is not
// the project's, and so is one that is, holds, or is inside evie's data
// folder, which holds its keys. The folder's own git repository, which
// backs it up, is not one of its files, and is refused too.

// The largest file that is read, 1 MB, so that one reply is never more than
// about 1.4 MB, an image's bytes in base64.
const STORAGE_FILE_LIMIT = 1024 * 1024
// The most entries one listing of a folder gives, by name, so that a folder
// of thousands of files is not one huge reply.
const STORAGE_LISTING_LIMIT = 500

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

// What one folder holds, folders first, then files, each by name, up to
// the limit, and how many more it holds. `folder` is a path inside the
// `.evie/` folder, such as "files", or nothing for the folder itself. A
// `.evie/` folder not made yet holds nothing. An entry that leads out of
// it, or nowhere, through a link is left out.
export async function listStorage(project: string, folder: string | undefined, dataFolder: string): Promise<{ entries: StorageEntry[]; more: number }> {
  const evie = await ownEvie(project, dataFolder)
  const named = inside(folder ?? "", "folder")
  if (evie === undefined && named === "") return { entries: [], more: 0 }
  const at = await real(evie, named, folder ?? "", "folder")
  const kind = await stat(at).catch(() => undefined)
  if (!kind?.isDirectory()) throw new Error(`There is no folder ${folder} in this project's .evie folder.`)
  const listed: StorageEntry[] = []
  for (const name of await readdir(at)) {
    if (named === "" && name === OWN_GIT) continue
    const target = await realOrNothing(join(at, name))
    if (target === undefined || !within(target, evie!)) continue
    const facts = await stat(target).catch(() => undefined)
    if (!facts || (!facts.isFile() && !facts.isDirectory())) continue
    const path = named === "" ? name : `${named}/${name}`
    const changed = Math.floor(facts.mtimeMs)
    listed.push(facts.isDirectory() ? { name, path, kind: "folder", changed } : { name, path, kind: "file", size: facts.size, changed })
  }
  const byName = (a: StorageEntry, b: StorageEntry) => a.name.localeCompare(b.name, undefined, { sensitivity: "base", numeric: true })
  const sorted = [...listed.filter((entry) => entry.kind === "folder").sort(byName), ...listed.filter((entry) => entry.kind === "file").sort(byName)]
  return { entries: sorted.slice(0, STORAGE_LISTING_LIMIT), more: Math.max(0, sorted.length - STORAGE_LISTING_LIMIT) }
}

// One file, as text when it is text, or as an image when its ending names
// one, and no larger than the limit. Anything else is refused, saying why.
export async function readStorage(project: string, path: string, dataFolder: string): Promise<StorageFile> {
  const evie = await ownEvie(project, dataFolder)
  const named = inside(path, "file")
  const at = await real(evie, named, path, "file")
  const facts = await stat(at).catch(() => undefined)
  if (!facts?.isFile()) throw new Error(`There is no file ${path} in this project's .evie folder.`)
  if (facts.size > STORAGE_FILE_LIMIT) {
    throw new Error(`${path} is larger than the 1 MB Storage shows. Open it on the computer evie runs on instead.`)
  }
  const bytes = await readFile(at)
  const shown = { path: named, size: facts.size, changed: Math.floor(facts.mtimeMs) }
  const image = IMAGES[extname(at).toLowerCase()]
  if (image !== undefined) return { ...shown, kind: "image", type: image, data: bytes.toString("base64") }
  const text = asText(bytes)
  if (text === undefined) throw new Error(`${path} is neither text nor an image, so Storage cannot show it.`)
  return { ...shown, kind: "text", text }
}

// A file of a project's Storage to give to the page, by its path inside
// the `.evie/` folder: where it really is, or nothing when it is not there,
// or is anywhere that `readStorage` refuses, which includes a link into the
// folder's own git repository. What the file is, and so whether it may be
// given, is for the one giving it to decide.
export async function storageFileAt(project: string, path: string, dataFolder: string): Promise<string | undefined> {
  let at: string
  let evie: string | undefined
  try {
    evie = ownEvie(project, dataFolder)
    at = await real(evie, inside(path, "file"), path, "file")
  } catch {
    return undefined
  }
  return relative(evie!, at).split(sep)[0] === OWN_GIT ? undefined : at
}

// The project's `.evie/` folder as it really is, or nothing when there is
// none yet. One that is a link, or that is, holds, or is inside the data
// folder, or would be once made, is refused, saying why. It is the one rule
// for what is a project's own: attachments are kept by it too. It reads at
// once, not in steps, for a caller that must not wait.
export function ownEvie(project: string, dataFolder: string): string | undefined {
  const folder = realNow(project)
  if (folder === undefined) return undefined
  const evie = join(folder, ".evie")
  const data = realNow(dataFolder) ?? dataFolder
  if (within(evie, data) || within(data, evie)) {
    throw new Error("This project's .evie folder is, holds, or is inside evie's data folder, which holds its keys, so Storage does not show it.")
  }
  const target = realNow(evie)
  if (target === undefined) return undefined
  if (target !== evie) throw new Error(`This project's .evie folder is a link to ${target}, so Storage does not show it: it shows only a .evie folder of the project's own.`)
  return evie
}

// The `.evie/` folder a plugin keeps a project's files in, so that Storage
// shows them: the folder as it really is, or where it is to be made, in the
// project as it really is, when it is not made yet. Nothing for a project
// that is not there, or whose `.evie` Storage would not show.
export function storageFolder(project: string, dataFolder: string): string | undefined {
  let evie: string | undefined
  try {
    evie = ownEvie(project, dataFolder)
  } catch {
    return undefined
  }
  if (evie !== undefined) return evie
  const folder = realNow(project)
  return folder === undefined ? undefined : join(folder, ".evie")
}

// A path with its links resolved, at once, or nothing when it leads nowhere.
function realNow(path: string): string | undefined {
  try {
    return realpathSync(path)
  } catch {
    return undefined
  }
}

// A path named inside the `.evie/` folder, written plainly: with slashes,
// none at either end, and no `.` or `..` in it. One that climbs out of the
// folder, or is a full path, is refused, and so is the folder's own git
// repository.
function inside(path: string, what: "folder" | "file"): string {
  if (typeof path !== "string") throw new Error(`\`${what === "folder" ? "folder" : "path"}\` is a path inside the project's .evie folder, such as "files/notes.md".`)
  const plain = relative(ROOT, resolve(ROOT, path))
  if (isAbsolute(path) || plain === ".." || plain.startsWith(`..${sep}`)) throw new Error(`${path} is outside this project's .evie folder. Storage holds only what is inside it.`)
  const names = plain === "" ? [] : plain.split(sep)
  if (names[0] === OWN_GIT) throw new Error(`${path} is the .evie folder's own git repository, which backs it up, and not one of its files.`)
  return names.join("/")
}
// Where a path is resolved against to write it plainly: any full path will do.
const ROOT = resolve("/evie-storage")

// A folder or file named inside the `.evie/` folder, as it really is, once
// it is sure to be inside it. `evie` is the `.evie/` folder as it really is,
// or nothing when there is none yet. `given` is the path as it was asked for.
async function real(evie: string | undefined, named: string, given: string, what: "folder" | "file"): Promise<string> {
  if (evie === undefined) throw new Error(`There is no ${what} ${given} in this project's .evie folder.`)
  const target = await realOrNothing(join(evie, named))
  if (target === undefined) throw new Error(`There is no ${what} ${given} in this project's .evie folder.`)
  if (!within(target, evie)) throw new Error(`${given} is outside this project's .evie folder. Storage holds only what is inside it.`)
  return target
}

// A path with its links resolved, or nothing when it leads nowhere.
const realOrNothing = (path: string) => realpath(path).catch(() => undefined)

// Whether a path is a folder or inside it. A path inside it may have a name
// that starts with two dots, such as `..cache`.
function within(path: string, folder: string): boolean {
  const from = relative(folder, path)
  return from !== ".." && !from.startsWith(`..${sep}`) && !isAbsolute(from)
}

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
