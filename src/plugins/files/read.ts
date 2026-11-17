import { readdir, readFile, stat } from "node:fs/promises"
import { basename, dirname, extname } from "node:path"
import type { Tool } from "../../core/types.ts"
import { displayPath, resolvePath } from "./paths.ts"

const DEFAULT_LIMIT = 2000
const MAX_LINE_LENGTH = 2000
const MAX_LISTED = 200

export const read: Tool = {
  name: "read",
  description:
    "Read a file. Returns its lines, each prefixed with its line number and a colon, such as `12: `; the prefix is " +
    "not part of the file. A relative path is taken from the project folder. By default the first 2000 lines are " +
    "returned; use offset and limit to read another part of a long file. Prefer one larger read over many small " +
    "ones. Reading a folder lists what is in it.",
  inputSchema: {
    type: "object",
    properties: {
      path: { type: "string", description: "The file to read" },
      offset: { type: "integer", description: "The line number to start from; the first line is 1" },
      limit: { type: "integer", description: "How many lines to read. Defaults to 2000" },
    },
    required: ["path"],
  },

  async run(input, context) {
    const path = input.path as string
    const target = resolvePath(path, context)
    if ("failure" in target) return target.failure
    const shown = displayPath(target.absolute, context)

    const info = await stat(target.absolute).catch(() => undefined)
    if (!info) return `File not found: ${path}.${await suggestions(target.absolute)}`
    if (info.isDirectory()) return listFolder(target.absolute, shown)

    const bytes = await readFile(target.absolute)
    if (bytes.subarray(0, 8192).includes(0)) return `${shown} looks like a binary file, so it is not shown.`

    const lines = bytes.toString("utf8").replaceAll("\r\n", "\n").split("\n")
    // A file that ends with a line break does not have an extra empty line.
    if (lines.at(-1) === "") lines.pop()
    if (lines.length === 0) return `[${shown} is empty.]`

    const offset = Math.max(1, (input.offset as number | undefined) ?? 1)
    const limit = Math.max(1, (input.limit as number | undefined) ?? DEFAULT_LIMIT)
    if (offset > lines.length) {
      return `${shown} has only ${lines.length} line${lines.length === 1 ? "" : "s"}, so there is nothing at line ${offset}.`
    }

    const last = Math.min(lines.length, offset + limit - 1)
    const numbered = lines.slice(offset - 1, last).map((line, index) => {
      const text = line.length > MAX_LINE_LENGTH ? `${line.slice(0, MAX_LINE_LENGTH)}… [line cut at ${MAX_LINE_LENGTH} characters]` : line
      return `${offset + index}: ${text}`
    })
    if (last < lines.length) {
      numbered.push("", `[Showing lines ${offset} to ${last} of ${lines.length}. Read on with offset: ${last + 1}.]`)
    }
    return numbered.join("\n")
  },
}

async function listFolder(absolute: string, shown: string): Promise<string> {
  const entries = await readdir(absolute, { withFileTypes: true })
  if (entries.length === 0) return `${shown} is a folder, and it is empty.`
  const names = entries
    .map((entry) => (entry.isDirectory() ? `${entry.name}/` : entry.name))
    .sort((a, b) => a.localeCompare(b))
  const more = names.length > MAX_LISTED ? `\n[${names.length - MAX_LISTED} more not shown.]` : ""
  return `${shown} is a folder. It contains:\n${names.slice(0, MAX_LISTED).join("\n")}${more}`
}

// Up to three names in the same folder that the model may have meant.
async function suggestions(absolute: string): Promise<string> {
  const folder = dirname(absolute)
  const entries = await readdir(folder).catch(() => undefined)
  if (!entries) return ` The folder ${folder} does not exist either.`

  const wanted = basename(absolute).toLowerCase()
  const wantedStem = stem(wanted)
  const scored = entries
    .map((name) => {
      const lower = name.toLowerCase()
      if (lower === wanted) return { name, score: 0 }
      if (stem(lower) === wantedStem) return { name, score: 1 }
      if (lower.includes(wantedStem) || wantedStem.includes(stem(lower))) return { name, score: 2 }
      const distance = editDistance(lower, wanted)
      return distance <= 2 ? { name, score: 2 + distance } : undefined
    })
    .filter((candidate) => candidate !== undefined)
    .sort((a, b) => a.score - b.score || a.name.localeCompare(b.name))
    .slice(0, 3)
  if (scored.length === 0) return ""
  return ` Did you mean: ${scored.map((candidate) => candidate.name).join(", ")}?`
}

const stem = (name: string) => (extname(name) ? name.slice(0, -extname(name).length) : name)

// How many single-character changes turn one name into the other.
function editDistance(a: string, b: string): number {
  let previous = Array.from({ length: b.length + 1 }, (_, index) => index)
  for (let i = 1; i <= a.length; i++) {
    const current = [i]
    for (let j = 1; j <= b.length; j++) {
      current[j] = Math.min(previous[j]! + 1, current[j - 1]! + 1, previous[j - 1]! + (a[i - 1] === b[j - 1] ? 0 : 1))
    }
    previous = current
  }
  return previous[b.length]!
}
