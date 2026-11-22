import { realpathSync } from "node:fs"
import { open, realpath, stat } from "node:fs/promises"
import { isAbsolute, join, relative, sep } from "node:path"
import { KIND_HEAD, shownType } from "./kinds.ts"

// Where the show plugin keeps what a session shows with no project, and
// how a file shown is found and known again: shared by the plugin, which
// keeps them; the guard, which lets a session show again what it showed;
// and the server's route, which gives them to the page.

// A session with no project keeps what it shows in the data folder, under
// the session, as `shown/<session>/2026-11-22/chart.png`. So does a session
// whose project's `.evie` folder Storage would not show. The plugin deletes
// it with the session.
export const ownShownFolder = (dataFolder: string, session: string) => join(dataFolder, "shown", session)

// A file the show tool kept for a session in the data folder, by its full
// path, as the tool told it: where it really is, or nothing when it is
// anywhere but in the session's own folder, as that folder really is. The
// folder is refused as a whole when it, or the folder of every session's,
// is a link.
export async function keptFileAt(dataFolder: string, session: string, path: string): Promise<string | undefined> {
  const data = realNow(dataFolder) ?? dataFolder
  const own = ownShownFolder(data, session)
  if (!isAbsolute(path) || realNow(join(data, "shown")) !== join(data, "shown") || realNow(own) !== own) return undefined
  const at = await realpath(path).catch(() => undefined)
  return at !== undefined && within(at, own) ? at : undefined
}

// The type a file is shown as, read from its name and the opening of its
// bytes, or nothing when it is not a file, or of no kind evie shows.
export async function typeOfFile(at: string): Promise<string | undefined> {
  const facts = await stat(at).catch(() => undefined)
  if (!facts?.isFile()) return undefined
  const file = await open(at, "r").catch(() => undefined)
  if (file === undefined) return undefined
  try {
    const head = new Uint8Array(Math.min(facts.size, KIND_HEAD))
    const { bytesRead } = await file.read(head, 0, head.byteLength, 0)
    return shownType(at, head.subarray(0, bytesRead))
  } finally {
    await file.close()
  }
}

function realNow(path: string): string | undefined {
  try {
    return realpathSync(path)
  } catch {
    return undefined
  }
}

// Whether a path is inside a folder. A path inside it may have a name that
// starts with two dots, such as `..cache`.
function within(path: string, folder: string): boolean {
  const from = relative(folder, path)
  return from !== "" && from !== ".." && !from.startsWith(`..${sep}`) && !isAbsolute(from)
}
