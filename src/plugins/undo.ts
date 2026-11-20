import { createHash } from "node:crypto"
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises"
import { dirname, isAbsolute, join, relative, resolve } from "node:path"
import { dataFolder } from "../core/evie.ts"
import type { CheckedToolCall, HookContext, Plugin } from "../core/types.ts"
import { patchFiles } from "./files/paths.ts"

// The built-in undo plugin. `/undo` puts files back as they were before
// Evie's last turn that changed any, and can be run again to go back a turn
// at a time.
//
// It works by keeping a copy. Just before a file tool runs (`write`, `edit`
// or `patch`), each file it is about to change is copied into the data
// folder, once per turn, or noted as not existing yet. `/undo` puts the
// copies back. It covers what the file tools change and nothing else: a
// file changed by a shell command is not put back.

export type UndoOptions = {
  // The folder Evie keeps its state in. Defaults to the usual place.
  dataFolder?: string
}

const FILE_TOOLS = ["write", "edit", "patch"]

// What is kept about one file in one turn. `before` and `after` are a mark
// of the file's contents, or "absent" when there was no file. `copy` names
// the saved copy of what it held before.
type Kept = { path: string; before: string; after?: string; copy?: string }
type Record = { files: Kept[]; ranCommands?: boolean }

const ABSENT = "absent"
const markOf = (contents: Buffer) => createHash("sha256").update(contents).digest("hex")
async function readOrAbsent(path: string): Promise<Buffer | undefined> {
  try {
    return await readFile(path)
  } catch {
    // Not there, or not a file that can be read: either way there is nothing to copy.
    return undefined
  }
}

export function undo(options: UndoOptions = {}): Plugin {
  const root = join(options.dataFolder ?? dataFolder({}), "undo")
  const turnFolder = (session: string, turn: number) => join(root, session, String(turn))
  const turnOf = (context: HookContext) => context.events.filter((event) => event.type === "turn_start").length

  async function load(folder: string): Promise<Record> {
    try {
      return JSON.parse(await readFile(join(folder, "record.json"), "utf8")) as Record
    } catch {
      return { files: [] }
    }
  }
  async function save(folder: string, record: Record): Promise<void> {
    await mkdir(folder, { recursive: true })
    await writeFile(join(folder, "record.json"), JSON.stringify(record))
  }

  return {
    name: "undo",
    setup(host) {
      const project = host.project?.folder ?? null
      // The files a call is about to change, in full. The file tools take a
      // path that is not a full one from the project folder.
      const filesOf = (call: CheckedToolCall): string[] => {
        const given = call.name === "patch" ? patchFiles(String(call.input.patchText ?? "")) : typeof call.input.path === "string" ? [call.input.path] : []
        return given.flatMap((path) => (isAbsolute(path) ? [resolve(path)] : project === null ? [] : [resolve(project, path)]))
      }

      // Before a file tool runs: keep a copy of each file it names, unless
      // this turn already has one. The first copy in a turn is the one that
      // holds what the file was before the turn.
      host.on("before_tool", async (call, context) => {
        if (!FILE_TOOLS.includes(call.name)) return
        const folder = turnFolder(context.session, turnOf(context))
        const record = await load(folder)
        for (const path of filesOf(call)) {
          if (record.files.some((kept) => kept.path === path)) continue
          const contents = await readOrAbsent(path)
          const kept: Kept = { path, before: contents ? markOf(contents) : ABSENT }
          if (contents) {
            kept.copy = String(record.files.length)
            await mkdir(folder, { recursive: true })
            await writeFile(join(folder, kept.copy), contents)
          }
          record.files.push(kept)
        }
        if (record.files.length > 0) await save(folder, record)
      })

      // After it has run: note what each file holds now. Undo uses this to
      // tell a file Evie left alone from one the owner has changed since.
      host.on("after_tool", async (call, _text, context) => {
        const folder = turnFolder(context.session, turnOf(context))
        if (call.name === "shell") {
          const record = await load(folder)
          if (!record.ranCommands) await save(folder, { ...record, ranCommands: true })
          return
        }
        if (!FILE_TOOLS.includes(call.name)) return
        const record = await load(folder)
        const touched = filesOf(call)
        for (const kept of record.files) {
          if (!touched.includes(kept.path)) continue
          const contents = await readOrAbsent(kept.path)
          kept.after = contents ? markOf(contents) : ABSENT
        }
        await save(folder, record)
      })

      host.commands.add({
        name: "undo",
        description: "Put files back as they were before Evie's last turn that changed any. Run it again to go back further.",
        async run(_args, context) {
          if (context.busy) return "Evie is in the middle of a turn. Undo when it has finished."
          const sessionFolder = join(root, context.session)
          const turns = (await readdir(sessionFolder).catch(() => [] as string[])).map(Number).filter(Number.isInteger).sort((a, b) => b - a)

          // The most recent turn that changed a file.
          for (const turn of turns) {
            const folder = turnFolder(context.session, turn)
            const record = await load(folder)
            const changed = record.files.filter((kept) => kept.after !== undefined && kept.after !== kept.before)
            if (changed.length === 0) continue

            const name = (path: string) => (project !== null && !relative(project, path).startsWith("..") ? relative(project, path) : path)
            const restored: string[] = []
            const removed: string[] = []
            const leftAlone: string[] = []
            for (const kept of changed) {
              const now = await readOrAbsent(kept.path)
              // Changed again since Evie left it, so it is the owner's now.
              if ((now ? markOf(now) : ABSENT) !== kept.after) {
                leftAlone.push(name(kept.path))
              } else if (kept.copy !== undefined) {
                await mkdir(dirname(kept.path), { recursive: true })
                await writeFile(kept.path, await readFile(join(folder, kept.copy)))
                restored.push(name(kept.path))
              } else {
                await rm(kept.path, { force: true })
                removed.push(name(kept.path))
              }
            }
            // An undone turn is forgotten, so the next undo goes back one more.
            await rm(folder, { recursive: true, force: true })

            const list = (files: string[]) => files.join(", ")
            const done = [
              restored.length > 0 ? `Put back: ${list(restored)}.` : "",
              removed.length > 0 ? `Removed, because the turn created ${removed.length === 1 ? "it" : "them"}: ${list(removed)}.` : "",
              leftAlone.length > 0 ? `Left alone, because ${leftAlone.length === 1 ? "it has" : "they have"} changed since: ${list(leftAlone)}.` : "",
            ].filter((sentence) => sentence !== "")
            const commands = record.ranCommands ? " Commands run through the shell in that turn were not undone." : ""

            if (restored.length + removed.length > 0) {
              context.tell(
                `The owner undid the file changes of an earlier turn. ${done.join(" ")}${commands} These files no longer hold ` +
                  `what you wrote, so read a file again before you change it.`,
              )
            }
            return `${done.join(" ")}${commands}`
          }
          return "There is nothing to undo: no turn in this session has changed a file with the file tools."
        },
      })
    },
  }
}
