import { lstatSync, realpathSync } from "node:fs"
import { mkdir, readFile, writeFile } from "node:fs/promises"
import { dirname, extname, isAbsolute, join, relative, sep } from "node:path"
import { ATTACHMENT_LIMIT, type Attachment, type Attachments, type Project, type StoredEvent } from "./types.ts"

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
// as `plan-2.pdf`. Each is kept at its path as it really is, with every
// link followed, and nothing is written through a link.

// What the core does with them besides: `check` takes the attachments a
// message names, as a client sent them, and gives back what the message
// records of them, refusing any that is not a file kept for the session.
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

export function createAttachments(options: {
  // The data folder.
  home: string
  // The project a session is in now, null for none, or undefined for a
  // session there is not.
  projectOf(session: string): Project | null | undefined
  // A session's events, in which its messages record what was attached.
  events(session: string): readonly StoredEvent[]
}): AttachmentStore {
  // The folder a session's files are kept in now.
  function rootFor(session: string): string {
    const project = options.projectOf(session)
    if (project === undefined) throw new Error(`There is no session with the id "${session}".`)
    const data = realOrNothing(options.home) ?? options.home
    const own = join(data, "attachments", session)
    if (project === null) return own
    const folder = realOrNothing(project.folder)
    if (folder === undefined) throw new Error(`This session's project folder, ${project.folder}, is not there, so nothing can be attached in it.`)
    const evie = join(folder, ".evie")
    const real = realOrNothing(evie)
    if (real !== undefined && real !== evie) return own
    if (within(evie, data) || within(data, evie)) return own
    return join(evie, "attachments")
  }

  return {
    async save(session, name, type, bytes) {
      if (!plain(name)) throw new Error(`${JSON.stringify(name)} is not a plain file name. A file is attached by its name alone, such as "plan.pdf".`)
      if (bytes.byteLength > ATTACHMENT_LIMIT) throw new Error(`${name} is larger than the ${ATTACHMENT_LIMIT / 1024 / 1024} MB a file attached can be.`)
      const root = rootFor(session)
      const day = await folderAt(root, today())
      for (let count = 1; ; count++) {
        const path = join(day, count === 1 ? name : numbered(name, count))
        try {
          await writeFile(path, bytes, { flag: "wx" })
          return { path, name, type: typeOf(type), size: bytes.byteLength }
        } catch (cause) {
          if ((cause as { code?: string }).code !== "EEXIST") throw cause
        }
      }
    },
    async open(session, path) {
      if (options.projectOf(session) === undefined) return undefined
      const recorded = options
        .events(session)
        .flatMap((event) => (event.type === "user_message" ? (event.body.attachments ?? []) : []))
        .find((each) => each.path === path)
      return recorded !== undefined && isKept(path, recorded.size) ? { file: path, type: recorded.type } : undefined
    },
    async image(attachment) {
      if (!IMAGE_TYPES.includes(attachment.type) || attachment.size > IMAGE_LIMIT || !isKept(attachment.path, attachment.size)) return undefined
      const bytes = await readFile(attachment.path).catch(() => undefined)
      return bytes !== undefined && bytes.byteLength <= IMAGE_LIMIT ? bytes.toString("base64") : undefined
    },
    // Read at once, not in steps, so that messages are taken in the order
    // they were sent.
    check(session, given) {
      if (given === undefined) return undefined
      if (!Array.isArray(given)) throw new Error("`attachments` are the files kept for the session that go with the message, as the attachments route gave them back.")
      const root = rootFor(session)
      return given.map((each: Partial<Attachment> | null) => {
        const { path, name, type, size } = each ?? {}
        if (typeof path !== "string" || typeof name !== "string" || typeof type !== "string" || typeof size !== "number") {
          throw new Error("Each attachment has the `path`, `name`, `type` and `size` the attachments route gave back for it.")
        }
        if (!within(path, root) || !isKept(path, size)) throw new Error(`${name} is not a file kept for this session, so it cannot go with the message. Attach it again.`)
        return { path, name, type, size }
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

// A file's type as it was sent, such as "image/png", without anything after
// it; one that is not a type is a file of no known type.
function typeOf(sent: string): string {
  const type = sent.split(";")[0]!.trim().toLowerCase()
  return /^[\w.+-]+\/[\w.+-]+$/.test(type) ? type : "application/octet-stream"
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
