import { lstat, mkdir, readFile, rm, writeFile } from "node:fs/promises"
import { join } from "node:path"
import { storageFolder } from "./storage.ts"
import type { ProjectInstructions } from "./types.ts"

// What a project tells the model about how to work in it, as files. The
// instructions plugin adds them to each request, and a client's Settings tab
// shows and edits them through `read_instructions` and `write_instructions`.
//
// There are two sources, both in the project. The repository's own file,
// `AGENTS.md`, or failing that `CLAUDE.md`, belongs to whoever wrote the
// repository, often a team, so evie reads it and never writes it. The
// owner's notes for the project are evie's own file, `.evie/instructions.md`,
// which evie writes; they are given to the model after the repository's
// file, so on a conflict they win.

// The names a repository's file may have, in the order they are looked for.
export const REPO_FILES = ["AGENTS.md", "CLAUDE.md"]
// The owner's notes, inside the project's `.evie` folder.
export const NOTES_FILE = "instructions.md"
// The most of one file that is used.
export const MAX_BYTES = 32 * 1024

export type Instructions = { path: string; text: string; cut: boolean }

// A file's text, or undefined when it is missing or has nothing in it. A
// file over the limit is cut at the end of a line, with a note saying so.
export async function readInstructions(path: string): Promise<Instructions | undefined> {
  const bytes = await readFile(path).catch(() => undefined)
  if (!bytes) return undefined
  let text = bytes.subarray(0, MAX_BYTES).toString("utf8")
  if (bytes.length > MAX_BYTES) {
    const lastBreak = text.lastIndexOf("\n")
    text = `${(lastBreak > 0 ? text.slice(0, lastBreak) : text.replace(/�+$/, "")).trimEnd()}\n\n[This file is longer than 32 KiB, so the rest was left out. Read ${path} if you need it.]`
  }
  text = text.trim()
  return text === "" ? undefined : { path, text, cut: bytes.length > MAX_BYTES }
}

// The file that holds the owner's notes for a project, or nothing when the
// project's `.evie` folder is one evie does not use (a link, or the data
// folder's own), the rule Storage keeps to.
export function notesPath(project: string, dataFolder: string): string | undefined {
  const evie = storageFolder(project, dataFolder)
  return evie === undefined ? undefined : join(evie, NOTES_FILE)
}

// The repository's file in use, the one the model is given: the first of
// `REPO_FILES` that has something in it. `ignored` names the later ones that
// also have something, which are not used.
export async function repoInstructions(project: string): Promise<{ used?: Instructions; ignored: string[] }> {
  let used: Instructions | undefined
  const ignored: string[] = []
  for (const name of REPO_FILES) {
    const found = await readInstructions(join(project, name))
    if (!found) continue
    if (used === undefined) used = found
    else ignored.push(name)
  }
  return { ...(used !== undefined && { used }), ignored }
}

// What the Settings tab shows: the repository's file, as the model is given
// it, and the owner's notes whole, as they are written.
export async function readProjectInstructions(project: string, dataFolder: string): Promise<ProjectInstructions> {
  const { used, ignored } = await repoInstructions(project)
  const file = notesPath(project, dataFolder)
  const notes = file === undefined ? "" : await readFile(file, "utf8").catch(() => "")
  return {
    ...(used !== undefined && { repo: { name: used.path.slice(project.length + 1), text: used.text, cut: used.cut } }),
    ignored,
    notes,
    canWrite: file !== undefined,
  }
}

// Saves the owner's notes. Nothing but spaces removes the file, so that a
// project with no notes has no file. A refusal says what to do next.
export async function writeProjectInstructions(project: string, dataFolder: string, text: string): Promise<void> {
  if (typeof text !== "string") throw new Error("`text` is the notes to keep, as a string.")
  const file = notesPath(project, dataFolder)
  if (file === undefined) {
    throw new Error("This project's .evie folder is not one evie keeps files in (it is a link, or is inside evie's data folder), so its notes cannot be saved. Make .evie an ordinary folder of the project's own.")
  }
  if (Buffer.byteLength(text) > MAX_BYTES) {
    throw new Error(`These notes are longer than 32 KiB, which is the most evie gives the model from one file. Shorten them and save again.`)
  }
  const there = await lstat(file).catch(() => undefined)
  if (there?.isSymbolicLink()) throw new Error(`${file} is a link, which evie does not write through. Delete the link, then save again.`)
  if (text.trim() === "") {
    await rm(file, { force: true })
    return
  }
  await mkdir(join(file, ".."), { recursive: true })
  await writeFile(file, text.endsWith("\n") ? text : `${text}\n`)
}
