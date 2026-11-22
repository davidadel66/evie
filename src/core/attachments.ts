import { closeSync, lstatSync, openSync, readSync, realpathSync } from "node:fs"
import { mkdir, readFile, writeFile } from "node:fs/promises"
import { basename, dirname, extname, isAbsolute, join, relative, sep } from "node:path"
import { ownEvie } from "./storage.ts"
import { ATTACHMENT_LIMIT, type Attachment, type Attachments, type Project } from "./types.ts"

// The files the owner attaches to messages. In a project they are kept in
// its `.evie/attachments/` folder, under the day they came, such as
// `.evie/attachments/2026-11-22/plan.pdf`, where the workspace's Storage
// shows them beside the folder's other files and its backup keeps them. A
// session with no project keeps them in the data folder, under the
// session, as `attachments/<session>/2026-11-22/plan.pdf`, and they go
// with the session when it is deleted. So does a session whose project's
// `.evie` folder Storage would not show: one that is a link, or that is,
// holds or is inside the data folder, which holds evie's keys.
//
// A file is never replaced: a name taken already that day gets a number,
// as `plan-2.pdf`. Square brackets in a name are kept as round ones, so
// that a name cannot close the line the model is told it in. Each is kept
// at its path as it really is, with every link followed, and nothing is
// written through a link. What a file is comes from its bytes, never from
// what a browser said: a PNG, JPEG, GIF or WebP picture by its signature,
// and anything else is a plain file, by its name's ending, never an image.

// What the core does with them besides: `check` takes the paths of the
// files a message names, as a client sent them, and gives back what the
// message records of each, read from the file itself, refusing any that is
// not a file kept for the session.
// `image` gives an attachment's bytes, in base64, when it is an image a
// model can be shown, or nothing.
export type AttachmentStore = Attachments & {
  check(session: string, given: unknown): Attachment[] | undefined
  image(attachment: Attachment): Promise<string | undefined>
}

// The images a model can be shown, by their type, as OpenRouter lists them:
// https://openrouter.ai/docs/guides/overview/multimodal/image-understanding
export const IMAGE_TYPES: readonly string[] = ["image/png", "image/jpeg", "image/webp", "image/gif"]
// The largest image a model is shown as one: one whose bytes in base64 are
// at most 5 MB, the most some of the models take. A larger one is given by
// its path, so that it cannot make every request of the session fail.
export const IMAGE_LIMIT = (5 * 1024 * 1024 * 3) / 4
// The most pixels on a side of an image a model is shown as one, as some
// of the models refuse a larger one.
export const IMAGE_SIDE = 8000

export function createAttachments(options: {
  // The data folder.
  home: string
  // The project a session is in now, null for none, or undefined for a
  // session there is not.
  projectOf(session: string): Project | null | undefined
  // The files attached to a session's messages, as they record them.
  attached(session: string): readonly Attachment[]
}): AttachmentStore {
  // The folder a session's files are kept in now: its project's own
  // `.evie` folder, by the rule Storage keeps to, or the data folder's.
  function rootFor(session: string): string {
    const project = options.projectOf(session)
    if (project === undefined) throw new Error(`There is no session with the id "${session}".`)
    const own = join(realOrNothing(options.home) ?? options.home, "attachments", session)
    if (project === null) return own
    let evie: string | undefined
    try {
      evie = ownEvie(project.folder, options.home)
    } catch {
      return own
    }
    if (evie !== undefined) return join(evie, "attachments")
    const folder = realOrNothing(project.folder)
    if (folder === undefined) throw new Error(`This session's project folder, ${project.folder}, is not there, so nothing can be attached in it.`)
    return join(folder, ".evie", "attachments")
  }

  // Why a file of this name cannot be attached in the session, or nothing.
  function refusal(session: string, name: string): string | undefined {
    if (!plain(name)) return `${JSON.stringify(name)} is not a plain file name. A file is attached by its name alone, such as "plan.pdf".`
    try {
      rootFor(session)
      return undefined
    } catch (cause) {
      return cause instanceof Error ? cause.message : String(cause)
    }
  }

  return {
    refusal,
    async save(session, name, bytes) {
      const refused = refusal(session, name)
      if (refused !== undefined) throw new Error(refused)
      if (bytes.byteLength > ATTACHMENT_LIMIT) throw new Error(`${name} is larger than the ${ATTACHMENT_LIMIT / 1024 / 1024} MB a file attached can be.`)
      const root = rootFor(session)
      const day = await folderAt(root, today())
      const kept = name.replaceAll("[", "(").replaceAll("]", ")")
      for (let count = 1; ; count++) {
        const path = join(day, count === 1 ? kept : numbered(kept, count))
        try {
          await writeFile(path, bytes, { flag: "wx" })
          return { path, name: basename(path), type: kindOf(path, bytes), size: bytes.byteLength }
        } catch (cause) {
          if ((cause as { code?: string }).code !== "EEXIST") throw cause
        }
      }
    },
    async open(session, path) {
      if (options.projectOf(session) === undefined) return undefined
      const recorded = options.attached(session).find((each) => each.path === path)
      return recorded !== undefined && isKept(path, recorded.size) ? { file: path, type: recorded.type } : undefined
    },
    async image(attachment) {
      if (!IMAGE_TYPES.includes(attachment.type) || attachment.size > IMAGE_LIMIT || !isKept(attachment.path, attachment.size)) return undefined
      const bytes = await readFile(attachment.path).catch(() => undefined)
      if (bytes === undefined || bytes.byteLength > IMAGE_LIMIT) return undefined
      const picture = pictureIn(bytes)
      if (picture?.type !== attachment.type || picture.width > IMAGE_SIDE || picture.height > IMAGE_SIDE) return undefined
      return bytes.toString("base64")
    },
    // Read at once, not in steps, so that messages are taken in the order
    // they were sent.
    check(session, given) {
      if (given === undefined) return undefined
      if (!Array.isArray(given) || !given.every((path) => typeof path === "string")) {
        throw new Error("`attachments` are the paths of files kept for the session that go with the message, as the attachments route gave them back.")
      }
      const root = rootFor(session)
      return given.map((path: string) => {
        if (!within(path, root) || !isKept(path)) throw new Error(`${path} is not a file kept for this session, so it cannot go with the message. Attach it again.`)
        return { path, name: basename(path), type: kindOf(path, opening(path)), size: lstatSync(path).size }
      })
    },
  }
}

// Whether a file is one evie kept, as it kept it: there, at its path as it
// really is, with no link on the way, and of the size it was kept at.
function isKept(path: string, size?: number): boolean {
  try {
    const facts = lstatSync(path)
    return facts.isFile() && realpathSync(path) === path && (size === undefined || facts.size === size)
  } catch {
    return false
  }
}

// A folder for the day inside `root`, made when it is missing, as `root`
// is and the folders above it, up to the first that is there. Each is made
// one at a time, and one that is a link, or not a folder, is refused before
// anything is made inside it.
async function folderAt(root: string, day: string): Promise<string> {
  const folder = join(root, day)
  const missing: string[] = []
  let there = folder
  for (; realOrNothing(there) === undefined; there = dirname(there)) missing.unshift(there)
  const refuse = () => new Error("This project's .evie/attachments folder leads through a link to somewhere else, so nothing is attached there. Remove the link, and attach it again.")
  if (realOrNothing(there) !== there) throw refuse()
  for (const each of missing) await mkdir(each).catch((cause: { code?: string }) => (cause.code === "EEXIST" ? undefined : Promise.reject(cause)))
  if (realOrNothing(folder) !== folder || !lstatSync(folder).isDirectory()) throw refuse()
  return folder
}

// A plain file name: one with no folder in it, so that it cannot lead out
// of the folder it is kept in, and nothing a file system or a reader would
// trip on.
const plain = (name: string) =>
  typeof name === "string" && name !== "." && name !== ".." && name.trim() !== "" && name.length <= 200 && !/[/\\\x00-\x1f\x7f]/.test(name)

// What a file is: a picture of a kind a model can be shown, by its bytes,
// or else a plain file, by its name's ending, but never an image, since
// its bytes say it is not one of those.
function kindOf(path: string, bytes: Uint8Array): string {
  const picture = pictureIn(bytes)
  if (picture !== undefined) return picture.type
  const byName = Bun.file(path).type.split(";")[0]!.trim()
  return byName === "" || byName.startsWith("image/") ? "application/octet-stream" : byName
}

// The first bytes of a file, enough to say what picture it is and its size.
function opening(path: string): Uint8Array {
  const bytes = new Uint8Array(OPENING)
  const file = openSync(path, "r")
  try {
    return bytes.subarray(0, readSync(file, bytes, 0, OPENING, 0))
  } finally {
    closeSync(file)
  }
}
const OPENING = 64 * 1024

// A picture's kind and its size in pixels, read from the opening of its
// file: PNG, JPEG, GIF and WebP, as each one's format says. Anything else
// is no picture of these.
export function pictureIn(bytes: Uint8Array): { type: string; width: number; height: number } | undefined {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  const text = (at: number, length: number) => String.fromCharCode(...bytes.subarray(at, at + length))
  const has = (length: number) => bytes.byteLength >= length
  if (has(24) && [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a].every((byte, at) => bytes[at] === byte) && text(12, 4) === "IHDR") {
    return { type: "image/png", width: view.getUint32(16), height: view.getUint32(20) }
  }
  if (has(10) && (text(0, 6) === "GIF87a" || text(0, 6) === "GIF89a")) {
    return { type: "image/gif", width: view.getUint16(6, true), height: view.getUint16(8, true) }
  }
  if (has(30) && text(0, 4) === "RIFF" && text(8, 4) === "WEBP") {
    const chunk = text(12, 4)
    const three = (at: number) => bytes[at]! | (bytes[at + 1]! << 8) | (bytes[at + 2]! << 16)
    if (chunk === "VP8X") return { type: "image/webp", width: three(24) + 1, height: three(27) + 1 }
    if (chunk === "VP8L" && bytes[20] === 0x2f) {
      const bits = view.getUint32(21, true)
      return { type: "image/webp", width: (bits & 0x3fff) + 1, height: ((bits >> 14) & 0x3fff) + 1 }
    }
    if (chunk === "VP8 " && bytes[23] === 0x9d && bytes[24] === 0x01 && bytes[25] === 0x2a) {
      return { type: "image/webp", width: view.getUint16(26, true) & 0x3fff, height: view.getUint16(28, true) & 0x3fff }
    }
    return undefined
  }
  if (has(4) && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) {
    // The segments after the start, each a marker and its length, up to a
    // frame, which gives the size.
    for (let at = 2; at + 9 <= bytes.byteLength; ) {
      if (bytes[at] !== 0xff) return undefined
      const marker = bytes[at + 1]!
      if (marker === 0xff) {
        at += 1
        continue
      }
      const frame = marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc
      if (frame) return { type: "image/jpeg", width: view.getUint16(at + 7), height: view.getUint16(at + 5) }
      at += 2 + view.getUint16(at + 2)
    }
    return undefined
  }
  return undefined
}

// Today's date where evie runs, as the owner reads it, such as 2026-11-22.
function today(): string {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`
}

// A name with a number before its ending, as `plan-2.pdf`.
function numbered(name: string, count: number): string {
  const ending = extname(name)
  return `${name.slice(0, name.length - ending.length)}-${count}${ending}`
}

function realOrNothing(path: string): string | undefined {
  try {
    return realpathSync(path)
  } catch {
    return undefined
  }
}

// Whether a path is a folder or inside it.
function within(path: string, folder: string): boolean {
  const from = relative(folder, path)
  return from !== ".." && !from.startsWith(`..${sep}`) && !isAbsolute(from)
}
