import { createHash } from "node:crypto"
import { realpathSync } from "node:fs"
import { lstat, mkdir, readFile, readdir, rename, rm, writeFile } from "node:fs/promises"
import { basename, dirname, isAbsolute, join, relative, resolve } from "node:path"
import { dataFolder } from "../core/evie.ts"
import { contentsAt, LARGEST, TOO_LARGE } from "../core/diff.ts"
import type { CheckedToolCall, Contents, HookContext, KeptFile, Plugin } from "../core/types.ts"
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
// of the file's contents: "absent" when there was no file, and "unknown"
// when it could not be read. `copy` names the saved copy of what it held
// before. `mixed` is set when something other than Evie's file tools changed
// the file part way through the turn, such as the owner or a command.
type Kept = { path: string; before: string; after?: string; copy?: string; mixed?: true }
type Record = { files: Kept[]; ranCommands?: boolean }

const ABSENT = "absent"
const UNKNOWN = "unknown"
const markOf = (contents: Buffer) => createHash("sha256").update(contents).digest("hex")

// What is at a path now: the contents of a file, that there is nothing, or
// that it could not be read. Only a file that is known not to exist counts
// as absent, so a file that merely could not be read is never taken for one
// the turn created.
async function stateOf(path: string): Promise<{ mark: string; contents?: Buffer }> {
  try {
    const contents = await readFile(path)
    return { mark: markOf(contents), contents }
  } catch (cause) {
    const code = (cause as { code?: string }).code
    return { mark: code === "ENOENT" || code === "ENOTDIR" ? ABSENT : UNKNOWN }
  }
}

// One name for a file however it was spelled: links followed, and on a disk
// that ignores case, the case it is really stored in. A path that does not
// exist yet is followed as far as it does exist.
function oneName(absolute: string): string {
  try {
    return realpathSync.native(absolute)
  } catch {
    const parent = dirname(absolute)
    return parent === absolute ? absolute : join(oneName(parent), basename(absolute))
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
    // Replaced in one step, so a record is never left half written.
    await writeFile(join(folder, "record.json.new"), JSON.stringify(record))
    await rename(join(folder, "record.json.new"), join(folder, "record.json"))
  }

  return {
    name: "undo",
    setup(host) {
      const project = host.project?.folder ?? null
      // The files a call is about to change, in full. The file tools take a
      // path that is not a full one from the project folder.
      const filesOf = (call: CheckedToolCall): string[] => {
        const given = call.name === "patch" ? patchFiles(String(call.input.patchText ?? "")) : typeof call.input.path === "string" ? [call.input.path] : []
        const full = given.flatMap((path) => (isAbsolute(path) ? [resolve(path)] : project === null ? [] : [resolve(project, path)]))
        return [...new Set(full.map(oneName))]
      }

      // Before a file tool runs: keep a copy of each file it names, unless
      // this turn already has one. The first copy in a turn is the one that
      // holds what the file was before the turn.
      host.on("before_tool", async (call, context) => {
        if (!FILE_TOOLS.includes(call.name)) return
        const folder = turnFolder(context.session, turnOf(context))
        const record = await load(folder)
        for (const path of filesOf(call)) {
          const now = await stateOf(path)
          const already = record.files.find((kept) => kept.path === path)
          if (already) {
            // It no longer holds what Evie last left there: the owner, or a
            // command, has changed it during the turn. Undo will not touch it.
            if (already.after !== undefined && already.after !== now.mark) already.mixed = true
            continue
          }
          const kept: Kept = { path, before: now.mark }
          if (now.contents) {
            kept.copy = String(record.files.length)
            await mkdir(folder, { recursive: true })
            await writeFile(join(folder, kept.copy), now.contents)
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
          kept.after = (await stateOf(kept.path)).mark
        }
        if (record.files.length > 0) await save(folder, record)
      })

      // A session deleted for good takes its copies with it.
      host.on("session_deleted", (session) => rm(join(root, session), { recursive: true, force: true }))

      // What each turn changed, for a client to show as diffs: the files
      // whose contents the turn changed, read from its record alone; and
      // for one turn, each file's copy from before it, and what it left in
      // the file. That is the file as it is now, while it still holds what
      // the turn left, or else the copy a later turn kept of it before
      // changing it, when that copy is what this turn left. Otherwise it is
      // not known. A turn undone is gone, with its copies.
      const keptTurns = async (session: string) => {
        const numbers = (await readdir(join(root, session)).catch(() => [] as string[])).map(Number).filter(Number.isInteger).sort((a, b) => a - b)
        return Promise.all(numbers.map(async (turn) => ({ turn, folder: turnFolder(session, turn), record: await load(turnFolder(session, turn)) })))
      }
      // A copy's contents, or too large, for one over the size a diff shows, which is not read.
      const copyOf = async (folder: string, file: Kept): Promise<Contents> => {
        if (file.copy === undefined) return null
        const read = await contentsAt(join(folder, file.copy))
        return read === null ? undefined : read
      }
      const changedIn = (record: Record) => record.files.filter((file) => file.after !== file.before)
      host.changes.add({
        name: "undo",
        async turns(session) {
          return (await keptTurns(session)).flatMap(({ turn, record }) => {
            const paths = changedIn(record).map((file) => file.path)
            return paths.length === 0 ? [] : [{ turn, paths }]
          })
        },
        async files(session, turn) {
          const kept = await keptTurns(session)
          const at = kept.findIndex((each) => each.turn === turn)
          if (at === -1) return []
          const { folder, record } = kept[at]!
          const later = kept.slice(at + 1)
          const files: KeptFile[] = []
          for (const file of changedIn(record)) {
            const before = file.before === UNKNOWN ? undefined : await copyOf(folder, file)
            files.push({ path: file.path, before, after: await leftBy(file, later) })
          }
          return files
        },
      })
      async function leftBy(file: Kept, later: { folder: string; record: Record }[]): Promise<Contents> {
        if (file.after === ABSENT) return null
        if (file.after === undefined || file.after === UNKNOWN) return undefined
        // A file over the size a diff shows is not read, even to tell whether it still holds what the turn left.
        if (((await lstat(file.path).catch(() => undefined))?.size ?? 0) > LARGEST) return TOO_LARGE
        const now = await stateOf(file.path)
        if (now.mark === file.after) return now.contents
        for (const { folder, record } of later) {
          const next = record.files.find((each) => each.path === file.path && each.before === file.after)
          if (next) return copyOf(folder, next)
        }
        return undefined
      }

      host.commands.add({
        name: "undo",
        description: "Put files back as they were before evie's last turn that changed any. Run it again to go back further.",
        async run(_args, context) {
          if (context.busy) return "evie is in the middle of a turn. Undo when it has finished."
          const sessionFolder = join(root, context.session)
          const turns = (await readdir(sessionFolder).catch(() => [] as string[])).map(Number).filter(Number.isInteger).sort((a, b) => b - a)

          // The most recent turn that changed a file.
          for (const turn of turns) {
            const folder = turnFolder(context.session, turn)
            const record = await load(folder)
            // A file with no "after" was being changed when Evie was stopped, so
            // whether it changed is not known.
            const changed = record.files.filter((kept) => kept.after !== kept.before)
            if (changed.length === 0) continue

            const name = (path: string) => (project !== null && !relative(project, path).startsWith("..") ? relative(project, path) : path)
            const restored: string[] = []
            const removed: string[] = []
            const leftAlone: string[] = []
            const unsure: string[] = []
            const failed: string[] = []
            const waiting: Kept[] = []
            for (const kept of changed) {
              // A turn that started while this was running may be changing files now.
              if (context.busy) {
                waiting.push(kept)
                continue
              }
              const now = await stateOf(kept.path)
              try {
                if (kept.after === undefined) {
                  if (now.mark !== kept.before) unsure.push(name(kept.path))
                } else if (kept.mixed || kept.before === UNKNOWN || now.mark !== kept.after) {
                  // Changed by someone else during the turn or since, so it is not Evie's to put back.
                  leftAlone.push(name(kept.path))
                } else if (kept.copy !== undefined) {
                  await mkdir(dirname(kept.path), { recursive: true })
                  await writeFile(kept.path, await readFile(join(folder, kept.copy)))
                  restored.push(name(kept.path))
                } else {
                  await rm(kept.path, { force: true })
                  removed.push(name(kept.path))
                }
              } catch (cause) {
                failed.push(`${name(kept.path)} (${cause instanceof Error ? cause.message : String(cause)})`)
                waiting.push(kept)
              }
            }

            // What was dealt with is forgotten, so the next undo goes back one
            // more turn. What could not be put back is kept to try again, and
            // copies Evie is unsure about are set aside, not thrown away.
            let setAside = ""
            if (waiting.length > 0) {
              await save(folder, { ...record, files: waiting })
            } else if (unsure.length > 0) {
              setAside = `${folder}-kept`
              await rename(folder, setAside)
            } else {
              await rm(folder, { recursive: true, force: true })
            }

            const list = (files: string[]) => files.join(", ")
            const done = [
              restored.length > 0 ? `Put back: ${list(restored)}.` : "",
              removed.length > 0 ? `Removed, because the turn created ${removed.length === 1 ? "it" : "them"}: ${list(removed)}.` : "",
              leftAlone.length > 0 ? `Left alone, because something other than evie's file tools changed ${leftAlone.length === 1 ? "it" : "them"}: ${list(leftAlone)}.` : "",
              unsure.length > 0 ? `Left alone, because evie was stopped part way through changing ${unsure.length === 1 ? "it" : "them"}: ${list(unsure)}. What was there before is kept in ${setAside}.` : "",
              failed.length > 0 ? `Could not be put back: ${list(failed)}. Run /undo again once that is sorted out.` : "",
              waiting.length > failed.length ? "Stopped part way, because a new turn started. Run /undo again when it has finished." : "",
            ].filter((sentence) => sentence !== "")
            const commands = record.ranCommands ? " Commands run through the shell in that turn were not undone." : ""

            if (restored.length + removed.length > 0) {
              try {
                context.tell(
                  // The first sentences are the ones about files that went back.
                  `The owner undid the file changes of an earlier turn. ${done.slice(0, (restored.length > 0 ? 1 : 0) + (removed.length > 0 ? 1 : 0)).join(" ")}${commands} ` +
                    `These files no longer hold what you wrote, so read a file again before you change it.`,
                )
              } catch {
                // Evie is closing. The files are back, which is what was asked for.
              }
            }
            return `${done.join(" ")}${commands}`
          }
          return "There is nothing to undo: no turn in this session has changed a file with the file tools."
        },
      })
    },
  }
}
