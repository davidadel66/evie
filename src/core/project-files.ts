import { realpathSync } from "node:fs"
import { readdir } from "node:fs/promises"
import { isAbsolute, join, relative, sep } from "node:path"
import { runGit } from "./git.ts"
import { inGit } from "./projects.ts"
import { FILE_LISTING_LIMIT } from "./types.ts"

// The files of a project the owner can mention with @ in a client's message
// box, by their paths from its folder, with slashes. In a git repository
// they are the files git lists, tracked or not, and not those it ignores;
// outside one, every file, the nearest first. Never anything in git's own
// folder, node_modules, the project's .evie folder, evie's data folder, or
// a folder of keys such as .ssh, which evie's tools may not read either.
// At most `limit` of them are walked, and git is given ten seconds, so that
// a folder with a great many files, such as one's home folder, is listed
// quickly; `more` says there were more.

const LEFT_OUT = [".git", "node_modules", ".evie", ".ssh", ".aws", ".gnupg"]
const GIT_WITHIN = 10_000

export type Listing = { files: string[]; more: boolean }

export async function listProjectFiles(
  folder: string,
  options: { dataFolder: string; limit?: number; gitWithin?: number },
): Promise<Listing> {
  const limit = options.limit ?? FILE_LISTING_LIMIT
  const real = realOrSame(folder)
  const data = relative(real, realOrSame(options.dataFolder))
  // The data folder, as a path from the project's folder, when it is inside it.
  const dataInside = data !== "" && data !== ".." && !data.startsWith(`..${sep}`) && !isAbsolute(data) ? data.split(sep).join("/") : undefined
  const leftOut = (path: string) => path.split("/").some((name) => LEFT_OUT.includes(name)) || (dataInside !== undefined && (path === dataInside || path.startsWith(`${dataInside}/`)))
  if ((await inGit(folder)) !== true) return walked(folder, limit, leftOut)
  return (await listedByGit(folder, limit, options.gitWithin ?? GIT_WITHIN, leftOut)) ?? walked(folder, limit, leftOut)
}

// What git lists in the folder, run as every git evie runs is, with none of
// the commands a repository's settings name. A listing cut short by its
// time limit may be missing some. When git will not list them, as for a
// repository it does not trust, nothing: the folder is walked instead.
async function listedByGit(folder: string, limit: number, within: number, leftOut: (path: string) => boolean): Promise<Listing | undefined> {
  const run = await runGit(folder, ["ls-files", "--cached", "--others", "--exclude-standard", "-z"], { timeout: within, most: { separator: "\0", count: limit } })
  if (run.code !== 0 && !run.stopped) return undefined
  // A listing cut short may end part way through a path, which is dropped.
  const said = run.stdout.toString("utf8")
  const whole = run.stopped ? said.slice(0, said.lastIndexOf("\0") + 1) : said
  const listed = [...new Set(whole.split("\0"))].filter((path) => path !== "" && !leftOut(path))
  return { files: listed.slice(0, limit), more: run.stopped || listed.length > limit }
}

// Every file in the folder, a level at a time, each level by name, without
// following links. A folder that cannot be read is passed over.
async function walked(folder: string, limit: number, leftOut: (path: string) => boolean): Promise<Listing> {
  const files: string[] = []
  let seen = 0
  const waiting = [""]
  while (waiting.length > 0) {
    const inside = waiting.shift()!
    const entries = await readdir(join(folder, inside), { withFileTypes: true }).catch(() => [])
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      const path = inside === "" ? entry.name : `${inside}/${entry.name}`
      if (leftOut(path) || !(entry.isFile() || entry.isDirectory())) continue
      if (seen === limit) return { files, more: true }
      seen += 1
      if (entry.isFile()) files.push(path)
      else waiting.push(path)
    }
  }
  return { files, more: false }
}

function realOrSame(path: string): string {
  try {
    return realpathSync(path)
  } catch {
    return path
  }
}
