import type { Stats } from "node:fs"
import { stat } from "node:fs/promises"
import { isAbsolute, relative, resolve } from "node:path"
import type { ToolContext } from "../../core/types.ts"

// Where a path given by the model points. A relative path is taken from the
// project folder. The general session has no project folder, so there a path
// has to be absolute.
export function resolvePath(path: string, context: ToolContext): { absolute: string } | { failure: string } {
  if (isAbsolute(path)) return { absolute: resolve(path) }
  if (context.folder === null) {
    return { failure: `This session is not in a project, so there is no folder to take "${path}" from. Give the full path.` }
  }
  return { absolute: resolve(context.folder, path) }
}

// How a file is named back to the model: relative to the project folder when
// it is inside it, in full otherwise.
export function displayPath(absolute: string, context: ToolContext): string {
  if (context.folder === null) return absolute
  const inside = relative(context.folder, absolute)
  return inside === "" || inside.startsWith("..") || isAbsolute(inside) ? absolute : inside
}

// What is at a path, or undefined when nothing is. Any other problem, such as
// no permission, is thrown, so it is not mistaken for a missing file.
export async function statOrMissing(absolute: string): Promise<Stats | undefined> {
  try {
    return await stat(absolute)
  } catch (cause) {
    const code = (cause as { code?: string }).code
    if (code === "ENOENT" || code === "ENOTDIR") return undefined
    throw cause
  }
}

// The files a patch names. The patch tool reads each line with the space
// around it taken off, so the same is done here.
export const patchFiles = (patchText: string) =>
  [...patchText.matchAll(/^[ \t]*\*\*\* (?:Add File|Update File|Delete File|Move to):[ \t]*(.+?)[ \t]*$/gm)].map((found) => found[1]!)
