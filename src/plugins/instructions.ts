import { join } from "node:path"
import { dataFolder } from "../core/evie.ts"
import { notesPath, readInstructions, repoInstructions, type Instructions } from "../core/instructions.ts"
import type { Plugin } from "../core/types.ts"

// The built-in plugin for instruction files. The owner writes how they want
// Evie to work, once, in a file; this adds it to what the model is told.
//
// There are three files: one in the data folder, for every project, and two
// for a project: the repository's own `AGENTS.md` (or `CLAUDE.md`), and the
// owner's notes for the project in `.evie/instructions.md` (see
// `core/instructions.ts`). They are added to the end of the system prompt, in
// that order, so the notes win on a conflict. The system prompt is the start
// of every request, so as long as the files do not change, the start of each
// request matches the last and the provider's prompt cache keeps working.
//
// The files are read again for each request, so an edit takes effect at
// once. That restarts the prompt cache once.

const GLOBAL_FILE = "AGENTS.md"

// The owner's own instruction file, in the data folder.
export const ownInstructionsFile = (home: string) => join(home, GLOBAL_FILE)

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
          const { used } = await repoInstructions(host.project.folder)
          if (used) {
            sections.push(`# Instructions for this project\n\nThese were written for this project. Follow them. ${have(used)}\n\n${used.text}`)
          }
          const file = notesPath(host.project.folder, home)
          const notes = file === undefined ? undefined : await readInstructions(file)
          if (notes) {
            sections.push(`# The owner's notes for this project\n\nThe owner wrote these for this project alone, in evie. Follow them; where they differ from the project's own instructions, the notes win. ${have(notes)}\n\n${notes.text}`)
          }
        }

        if (sections.length === 0) return
        // Added after whatever is already there, such as the model's own prompt.
        return { ...request, system: [request.system, ...sections].filter((part) => part).join("\n\n") }
      })
    },
  }
}
