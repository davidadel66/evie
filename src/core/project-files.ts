import { readdir } from "node:fs/promises"
import { join } from "node:path"
import { inGit } from "./projects.ts"

// The files of a project the owner can mention with @ in a client's message
// box, by their paths from its folder, with slashes. In a git repository
// they are the files git lists, tracked or not, and not those it ignores;
// outside one, every file, the nearest first. Never anything in git's own
// folder or in node_modules. At most `limit` of them are walked, so that a
// folder with a great many files, such as one's home folder, is listed
// quickly; `more` says there were more.
export const FILE_LISTING_LIMIT = 10_000

const LEFT_OUT = [".git", "node_modules"]
// How long git may take to list them.
const GIT_WITHIN = 10_000

export async function listProjectFiles(folder: string, limit = FILE_LISTING_LIMIT): Promise<{ files: string[]; more: boolean }> {
  return (await inGit(folder)) === true ? listedByGit(folder, limit) : walked(folder, limit)
}

const leftOut = (path: string) => path.split("/").some((name) => LEFT_OUT.includes(name))

// What git lists in the folder, read as it comes and stopped once there is
// one more than the limit. Git is asked about the folder alone: git's own
// settings in the server's environment, such as GIT_DIR, are left out.
async function listedByGit(folder: string, limit: number): Promise<{ files: string[]; more: boolean }> {
  const env = Object.fromEntries(Object.entries(process.env).filter(([name]) => !name.startsWith("GIT_")))
  const git = Bun.spawn(["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"], {
    cwd: folder,
    env,
    stdin: "ignore",
    stdout: "pipe",
    stderr: "ignore",
    timeout: GIT_WITHIN,
  })
  const files = new Set<string>()
  const decoder = new TextDecoder()
  let buffer = ""
  let more = false
  reading: for await (const bytes of git.stdout) {
    buffer += decoder.decode(bytes, { stream: true })
    let end: number
    while ((end = buffer.indexOf("\0")) !== -1) {
      const path = buffer.slice(0, end)
      buffer = buffer.slice(end + 1)
      if (path === "" || leftOut(path)) continue
      if (files.size === limit && !files.has(path)) {
        more = true
        git.kill()
        break reading
      }
      files.add(path)
    }
  }
  await git.exited
  return { files: [...files], more }
}

// Every file in the folder, a level at a time, each level by name, without
// following links. A folder that cannot be read is passed over.
async function walked(folder: string, limit: number): Promise<{ files: string[]; more: boolean }> {
  const files: string[] = []
  let seen = 0
  const waiting = [""]
  while (waiting.length > 0) {
    const inside = waiting.shift()!
    const entries = await readdir(join(folder, inside), { withFileTypes: true }).catch(() => [])
    for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name))) {
      if (LEFT_OUT.includes(entry.name) || !(entry.isFile() || entry.isDirectory())) continue
      if (seen === limit) return { files, more: true }
      seen += 1
      const path = inside === "" ? entry.name : `${inside}/${entry.name}`
      if (entry.isFile()) files.push(path)
      else waiting.push(path)
    }
  }
  return { files, more: false }
}
