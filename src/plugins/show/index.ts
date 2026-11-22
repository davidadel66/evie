import { closeSync, constants, openSync, readSync, realpathSync } from "node:fs"
import { copyFile, rm, stat } from "node:fs/promises"
import { basename, isAbsolute, join, relative, sep } from "node:path"
import { folderAt, numbered, today } from "../../core/attachments.ts"
import { dataFolder } from "../../core/evie.ts"
import { ownEvie } from "../../core/storage.ts"
import type { Plugin, Tool, ToolContext, ToolOutput } from "../../core/types.ts"
import { displayPath, resolvePath } from "../files/paths.ts"
import { ownShownFolder } from "./files.ts"
import { KIND_HEAD, namedFor, SHOWN_KINDS_WORDS, SHOWN_LIMIT, shownType } from "./kinds.ts"
import { shownText, type ShownFile } from "./record.ts"

// The built-in show plugin. It adds one tool, show, with which evie shows
// the owner something it made, such as a picture, a diagram, a document or
// a design: the page draws each file under evie's reply, and the terminal
// names it. Nothing is shown without the call. Each file is kept in the
// workspace's Storage: one from elsewhere is copied into the project's
// `.evie/shown/<date>/` folder, a new copy each call, so that an earlier
// reply keeps the version it showed; one already in Storage is shown where
// it is. With no project, they are kept for the session in the data folder
// (files.ts), and deleted with it. The tool's details list the files, for
// clients; its text tells the model the same (record.ts).

export type ShowOptions = {
  // The folder evie keeps its state in. Defaults to the usual place.
  dataFolder?: string
}

export function show(options: ShowOptions = {}): Plugin {
  const home = options.dataFolder ?? dataFolder({})
  // Where a session keeps what it shows: its project's `.evie` folder, by
  // the rule Storage keeps to, or with no project, or one whose `.evie`
  // Storage would not show, its own folder in the data folder.
  function storageFor(session: string, folder: string | null): { root: string; project: boolean } {
    const own = { root: ownShownFolder(realOrNothing(home) ?? home, session), project: false }
    if (folder === null) return own
    let evie: string | undefined
    try {
      evie = ownEvie(folder, home)
    } catch {
      return own
    }
    if (evie !== undefined) return { root: evie, project: true }
    // A project's `.evie` folder not made yet is made where the project really is.
    const project = realOrNothing(folder)
    return project === undefined ? own : { root: join(project, ".evie"), project: true }
  }

  const tool: Tool = {
    name: "show",
    description:
      "Show the owner something you made, such as an image, a diagram, a document or a design, drawn under your reply " +
      "where they can open it full screen. Use it when seeing the file says more than describing it. Give the path of " +
      "each file, in the order to show them. A relative path is taken from the project folder.",
    inputSchema: {
      type: "object",
      properties: { paths: { type: "array", items: { type: "string" }, minItems: 1, description: "The files to show, in order" } },
      required: ["paths"],
    },
    async run(input, context): Promise<ToolOutput> {
      const paths = input.paths
      if (!Array.isArray(paths) || paths.length === 0 || !paths.every((path) => typeof path === "string")) {
        return 'Not shown: `paths` is a list of the files to show, at least one, such as ["out/chart.png"].'
      }
      if (paths.length > MOST_FILES) return `Not shown: show takes at most ${MOST_FILES} files at a time. Show the ones that matter most, or call it again for the rest.`
      // Every file is checked before any is kept, so that a call shows all
      // of its files, in order, or none.
      const checked = await Promise.all(paths.map((path) => check(path, context)))
      const refused = checked.filter((each): each is { refused: string } => "refused" in each)
      if (refused.length === 1 && paths.length === 1) return `Not shown: ${refused[0]!.refused}`
      if (refused.length > 0) {
        return ["Not shown: nothing was shown, since some of these files cannot be.", ...refused.map((each) => `- ${each.refused}`), "Call show again with the files that can be shown."].join("\n")
      }
      const storage = storageFor(context.session, context.folder)
      const place = (path: string): Pick<ShownFile, "storage" | "path"> => (storage.project ? { storage: "project", path: relative(storage.root, path).split(sep).join("/") } : { storage: "session", path })
      const files: ShownFile[] = []
      // The copies this call has made, removed again if a later one fails,
      // so that a call that shows nothing keeps nothing.
      const copies: string[] = []
      try {
        for (const file of checked as Checked[]) {
          if (inStorage(file.real, storage.root)) {
            files.push({ ...place(file.real), type: file.type, size: file.size })
            continue
          }
          const day = await folderAt(storage.project ? join(storage.root, "shown") : storage.root, today(), () => new LinkOut())
          const kept = await copyInto(day, namedFor(basename(file.real), file.type), file.real)
          copies.push(kept)
          files.push({ ...place(kept), type: file.type, size: file.size, from: file.named })
        }
      } catch (cause) {
        await Promise.all(copies.map((copy) => rm(copy, { force: true }).catch(() => {})))
        if (cause instanceof LinkOut) return `Not shown: ${cause.message}`
        // The file system refusing, such as for want of room or permission,
        // is a failure the model can act on.
        if (cause instanceof Error && typeof (cause as { code?: unknown }).code === "string") {
          return `Not shown: the files could not be kept in Storage (${cause.message}). Check that there is room and that evie may write there, then call show again, or tell the owner the paths instead.`
        }
        throw cause
      }
      return { text: shownText(files), details: { files } }
    },
  }

  return {
    name: "show",
    setup(host) {
      host.tools.add(tool)
      // What a session kept with no project goes with it.
      host.on("session_deleted", (session) => rm(ownShownFolder(home, session), { recursive: true, force: true }))
    },
  }
}

// The most files one call shows.
const MOST_FILES = 20

// A file to be shown, once checked: its path as the model named it, as it
// really is, its type and its size.
type Checked = { named: string; real: string; type: string; size: number }

// Checks a file the model named: that it is there, a file, no larger than
// the limit, and of a kind that can be shown. Or why it cannot be, in
// words for the model, saying what to do instead.
async function check(path: string, context: ToolContext): Promise<Checked | { refused: string }> {
  const target = resolvePath(path, context)
  if ("failure" in target) return { refused: target.failure }
  const named = displayPath(target.absolute, context)
  const unreadable = (cause: unknown) => ({ refused: `${named} could not be read (${cause instanceof Error ? cause.message : String(cause)}). Check that evie may read it, or tell the owner its path instead.` })
  let facts: Awaited<ReturnType<typeof stat>>
  try {
    facts = await stat(target.absolute)
  } catch (cause) {
    const code = (cause as { code?: string }).code
    if (code === "ENOENT" || code === "ENOTDIR") return { refused: `${named} is not there. Check the path, or make the file first.` }
    return unreadable(cause)
  }
  if (facts.isDirectory()) return { refused: `${named} is a folder. Give the path of each file in it to show.` }
  if (!facts.isFile()) return { refused: `${named} is not a file that can be shown. Save what it holds to a file of a kind evie shows, or tell the owner its path instead.` }
  if (facts.size > SHOWN_LIMIT) return { refused: `${named} is ${megabytes(facts.size)}, more than the ${megabytes(SHOWN_LIMIT)} a file shown can be. Make it smaller, or tell the owner its path instead.` }
  let real: string
  let head: Uint8Array
  try {
    real = realpathSync(target.absolute)
    head = headOf(real, facts.size)
  } catch (cause) {
    // Gone since it was looked at, or not to be read.
    if ((cause as { code?: string }).code === "ENOENT") return { refused: `${named} is not there any more. Check the path, or make the file again.` }
    return unreadable(cause)
  }
  const type = shownType(real, head)
  if (type === undefined) return { refused: `${named} is not a kind of file evie can show: evie shows ${SHOWN_KINDS_WORDS}. Save it as one of those, or tell the owner its path instead.` }
  return { named, real, type, size: facts.size }
}

const megabytes = (size: number) => `${(size / 1024 / 1024).toFixed(1).replace(/\.0$/, "")} MB`

// Whether a file, as it really is, is in Storage: inside its folder, and
// not in the folder's own git repository, which backs it up.
function inStorage(real: string, storage: string): boolean {
  const inside = relative(storage, real)
  return inside !== "" && !inside.startsWith(`..${sep}`) && inside !== ".." && !isAbsolute(inside) && inside.split(sep)[0] !== ".git"
}

// The opening of a file, as much as its kind is told from.
function headOf(path: string, size: number): Uint8Array {
  const head = new Uint8Array(Math.min(size, KIND_HEAD))
  const file = openSync(path, "r")
  try {
    return head.subarray(0, readSync(file, head, 0, head.byteLength, 0))
  } finally {
    closeSync(file)
  }
}

// Copies a file into a folder under the name given, or with a number when
// that name is taken: copying fails when a name is taken, so that no file
// there is replaced. Gives back the copy's path.
async function copyInto(folder: string, name: string, from: string): Promise<string> {
  for (let count = 1; ; count++) {
    const path = join(folder, count === 1 ? name : numbered(name, count))
    try {
      await copyFile(from, path, constants.COPYFILE_EXCL)
      return path
    } catch (cause) {
      if ((cause as { code?: string }).code !== "EEXIST") throw cause
    }
  }
}

// Why nothing is kept in a folder that leads through a link: what it leads
// to is not the workspace's.
class LinkOut extends Error {
  constructor() {
    super("The folder shown files are kept in leads through a link to somewhere else, so nothing is shown there. Remove the link, and show it again.")
  }
}

function realOrNothing(path: string): string | undefined {
  try {
    return realpathSync(path)
  } catch {
    return undefined
  }
}
