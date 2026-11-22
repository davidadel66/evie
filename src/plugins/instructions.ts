import { readFile } from "node:fs/promises"
import { join } from "node:path"
import { dataFolder } from "../core/evie.ts"
import type { Plugin } from "../core/types.ts"

// The built-in plugin for instruction files. The owner writes how they want
// Evie to work, once, in a file; this adds it to what the model is told.
//
// There are two files: one in the data folder, for every project, and one in
// a project's own folder. They are added to the end of the system prompt, in
// that order, before every request. The system prompt is the start of every
// request, so as long as the files do not change, the start of each request
// matches the last and the provider's prompt cache keeps working.
//
// The files are read again for each request, so an edit takes effect at
// once. That restarts the prompt cache once.

// The names a project's file may have, in the order they are looked for.
const PROJECT_FILES = ["AGENTS.md", "CLAUDE.md"]
const GLOBAL_FILE = "AGENTS.md"

// The owner's own instruction file, in the data folder.
export const ownInstructionsFile = (home: string) => join(home, GLOBAL_FILE)
// The most of one file that is used.
const MAX_BYTES = 32 * 1024

export type InstructionsOptions = {
  // The folder the owner's own instruction file is in. Defaults to the usual data folder.
  dataFolder?: string
}

export function instructions(options: InstructionsOptions = {}): Plugin {
  const home = options.dataFolder ?? dataFolder({})

  return {
    name: "instructions",
    setup(host) {
      host.on("before_request", async (request) => {
        const sections: string[] = []

        // Each section says that the file's text is already here. Without
        // that, a model asked about its instructions goes and reads the file
        // again, which wastes a step and fills the conversation with a copy.
        const have = (file: Instructions) =>
          `The contents of ${file.path} are given here ${file.cut ? "up to a size limit" : "in full"}, so you already have them: do not read that file to find them.`

        const fromOwner = await readInstructions(ownInstructionsFile(home))
        if (fromOwner) {
          sections.push(`# Instructions from the owner\n\nThe owner wrote these for every project. Follow them. ${have(fromOwner)}\n\n${fromOwner.text}`)
        }

        if (host.project) {
          for (const name of PROJECT_FILES) {
            const fromProject = await readInstructions(join(host.project.folder, name))
            if (!fromProject) continue
            sections.push(`# Instructions for this project\n\nThese were written for this project. Follow them. ${have(fromProject)}\n\n${fromProject.text}`)
            break
          }
        }

        if (sections.length === 0) return
        // Added after whatever is already there, such as the model's own prompt.
        return { ...request, system: [request.system, ...sections].filter((part) => part).join("\n\n") }
      })
    },
  }
}

type Instructions = { path: string; text: string; cut: boolean }

// A file's text, or undefined when it is missing or has nothing in it. A
// file over the limit is cut at the end of a line, with a note saying so.
async function readInstructions(path: string): Promise<Instructions | undefined> {
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
