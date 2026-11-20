import { readFile, readdir } from "node:fs/promises"
import { join } from "node:path"

// Agents and skills are files the owner writes: a few lines of header and a
// body of instructions. This reads them. There are two places they live:
// the data folder, for every project, and a project's own `.evie` folder,
// which is used only when the owner has trusted the project, because an
// agent or a skill is a standing instruction.

// A file split into its header and its body. The header is the lines
// between two lines of `---` at the top, each `name: value`. A file with no
// header is all body.
export function splitHeader(given: string): { header: Record<string, string>; body: string } {
  // A mark some editors put at the start of a file, and blank lines before
  // the header, are not part of it.
  const text = given.replace(/^\uFEFF/, "").replace(/^(?:[ \t]*\r?\n)+/, "")
  const found = /^---[ \t]*\r?\n([\s\S]*?)\r?\n---[ \t]*(?:\r?\n|$)/.exec(text)
  if (!found) return { header: {}, body: text.trim() }
  const header: Record<string, string> = {}
  for (const line of found[1]!.split(/\r?\n/)) {
    const at = line.indexOf(":")
    if (at === -1) continue
    const value = line.slice(at + 1).trim().replace(/^(["'])(.*)\1$/, "$2")
    if (value !== "") header[line.slice(0, at).trim().toLowerCase()] = value
  }
  return { header, body: text.slice(found[0].length).trim() }
}

// The two folders a kind of definition is read from, the owner's first. The
// project's is left out when there is no project, or it is not trusted.
export function foldersFor(kind: "agents" | "skills", dataFolder: string, project: string | null, trusted: (folder: string) => boolean): string[] {
  return [join(dataFolder, kind), ...(project !== null && trusted(project) ? [join(project, ".evie", kind)] : [])]
}

// A name the owner can type and a model can repeat: letters, digits, dashes.
export const tidyName = (name: string) => name.trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "")

export const entriesOf = (folder: string) => readdir(folder, { withFileTypes: true }).catch(() => [])
export const textOf = (file: string) => readFile(file, "utf8").catch(() => undefined)

// Whether a definition's `mode` holds it to reading only. Only "auto", or
// no mode at all, leaves it free to change things. Anything else, a
// misspelling included, holds it to reading: a slip in the file must not
// quietly give an agent more than its owner meant.
export function readsOnly(mode: string | undefined): boolean {
  if (mode === undefined) return false
  return mode.replace(/\s+#.*$/, "").trim().toLowerCase() !== "auto"
}
