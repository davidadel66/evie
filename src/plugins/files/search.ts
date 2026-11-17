import { dirname, basename, resolve } from "node:path"
import type { Tool, ToolContext } from "../../core/types.ts"
import { displayPath, resolvePath, statOrMissing } from "./paths.ts"

// grep and glob. Both run ripgrep, a fast search program, and both stop at
// 100 results so a broad search cannot flood the conversation.

const MAX_RESULTS = 100
// Hidden files are searched, since settings often live in them; git's own
// folder is not.
const COMMON = ["--color", "never", "--hidden", "--glob", "!.git", "--sort", "path"]

export function searchTools(ripgrep: string): Tool[] {
  const grep: Tool = {
    name: "grep",
    description:
      "Search the contents of files for a regular expression. Returns the matching lines, grouped by file, each " +
      "with its line number. Searches the project folder, or the file or folder given as path. Use include to " +
      "search only files whose names match a pattern, such as *.ts. Files that git ignores are skipped. At most " +
      "100 matches are returned.",
    inputSchema: {
      type: "object",
      properties: {
        pattern: { type: "string", description: "The regular expression to search for" },
        path: { type: "string", description: "The file or folder to search. Defaults to the project folder" },
        include: { type: "string", description: "Search only files whose names match this pattern, such as *.ts" },
      },
      required: ["pattern"],
    },

    async run(input, context) {
      const pattern = input.pattern as string
      const where = await searchRoot(input.path as string | undefined, context)
      if ("failure" in where) return where.failure

      const include = input.include === undefined ? [] : ["--glob", input.include as string]
      const found = await runRipgrep(
        ripgrep,
        [...COMMON, "--line-number", "--no-heading", "--with-filename", "--null", "--max-columns", "500",
          "--max-columns-preview", ...include, "--regexp", pattern, ...where.target],
        where.folder,
        context.signal,
      )
      if (found.missing) return missing("grep")
      if (found.error) {
        if (found.error.includes("regex parse error")) return `The pattern "${pattern}" is not a valid regular expression:\n${found.error}`
        return `grep could not run: ${found.error}`
      }
      if (found.lines.length === 0) return `No matches for "${pattern}".`

      // Each line is the file, a zero byte, then the line number and the text.
      const groups = new Map<string, string[]>()
      for (const line of found.lines) {
        const split = line.indexOf("\0")
        const file = displayPath(resolve(where.folder, line.slice(0, split)), context)
        const [, number, text] = /^(\d+):(.*)$/s.exec(line.slice(split + 1)) ?? []
        groups.set(file, [...(groups.get(file) ?? []), `  ${number}: ${text}`])
      }
      const body = [...groups].map(([file, lines]) => `${file}:\n${lines.join("\n")}`).join("\n\n")
      return `${heading(found, "match", "matches", ` in ${count(groups.size, "file", "files")}`)}\n\n${body}${more(found, "matches")}`
    },
  }

  const glob: Tool = {
    name: "glob",
    description:
      "Find files by name. The pattern is a glob such as **/*.test.ts or src/**/*.json. Searches the project " +
      "folder, or the folder given as path. Files that git ignores are skipped. At most 100 files are returned.",
    inputSchema: {
      type: "object",
      properties: {
        pattern: { type: "string", description: "The glob pattern that file names must match" },
        path: { type: "string", description: "The folder to search. Defaults to the project folder" },
      },
      required: ["pattern"],
    },

    async run(input, context) {
      const pattern = input.pattern as string
      const where = await searchRoot(input.path as string | undefined, context)
      if ("failure" in where) return where.failure

      const found = await runRipgrep(ripgrep, [...COMMON, "--files", "--glob", pattern], where.folder, context.signal)
      if (found.missing) return missing("glob")
      if (found.error) return `glob could not run: ${found.error}`
      if (found.lines.length === 0) return `No files match "${pattern}".`

      const names = found.lines.map((line) => displayPath(resolve(where.folder, line), context))
      return `${heading(found, "file", "files", "")}\n\n${names.join("\n")}${more(found, "files")}`
    },
  }

  return [grep, glob]
}

// ---- Wording ----------------------------------------------------------------------

const count = (amount: number, one: string, many: string) => `${amount} ${amount === 1 ? one : many}`

const heading = (found: Found, one: string, many: string, rest: string) =>
  found.more
    ? `Found more than ${MAX_RESULTS} ${many}. Showing the first ${MAX_RESULTS}.`
    : `Found ${count(found.lines.length, one, many)}${rest}.`

const more = (found: Found, many: string) =>
  found.more ? `\n\n[There are more ${many}. Narrow the pattern or the path to see the rest.]` : ""

const missing = (tool: string) =>
  `${tool} needs ripgrep, and it is not installed. Install it (for example \`brew install ripgrep\`), then try again.`

// ---- Where to search ----------------------------------------------------------------

// The folder ripgrep runs in, and what it is told to search there: nothing
// for the whole folder, or one file's name.
async function searchRoot(
  path: string | undefined,
  context: ToolContext,
): Promise<{ folder: string; target: string[] } | { failure: string }> {
  if (path === undefined) {
    if (context.folder === null) {
      return { failure: "This session is not in a project, so there is no folder to search. Give the full path of a folder as path." }
    }
    return { folder: context.folder, target: [] }
  }
  const resolved = resolvePath(path, context)
  if ("failure" in resolved) return resolved
  const info = await statOrMissing(resolved.absolute)
  if (!info) return { failure: `There is nothing at "${path}" to search.` }
  if (info.isDirectory()) return { folder: resolved.absolute, target: [] }
  return { folder: dirname(resolved.absolute), target: [basename(resolved.absolute)] }
}

// ---- Running ripgrep ------------------------------------------------------------------

type Found = { lines: string[]; more: boolean; missing?: true; error?: string }

// Runs ripgrep and reads its output a line at a time, stopping it as soon as
// there is one result more than will be shown.
async function runRipgrep(binary: string, args: string[], folder: string, signal: AbortSignal): Promise<Found> {
  let child: ReturnType<typeof Bun.spawn<"ignore", "pipe", "pipe">>
  try {
    child = Bun.spawn([binary, ...args], { cwd: folder, stdin: "ignore", stdout: "pipe", stderr: "pipe", signal })
  } catch {
    return { lines: [], more: false, missing: true }
  }

  const lines: string[] = []
  const decoder = new TextDecoder()
  let buffer = ""
  let more = false
  reading: for await (const bytes of child.stdout) {
    buffer += decoder.decode(bytes, { stream: true })
    let end: number
    while ((end = buffer.indexOf("\n")) !== -1) {
      if (lines.length === MAX_RESULTS) {
        more = true
        child.kill()
        break reading
      }
      lines.push(buffer.slice(0, end))
      buffer = buffer.slice(end + 1)
    }
  }

  const status = await child.exited
  // Status 1 means nothing was found. Anything above that is a real problem,
  // unless ripgrep was stopped here because there was enough.
  if (!more && status > 1) {
    const problem = (await new Response(child.stderr).text()).trim()
    return { lines: [], more: false, error: problem || `ripgrep ended with status ${status}` }
  }
  return { lines, more }
}
