import { randomBytes } from "node:crypto"
import { lstat, open, readFile, readlink, rename, rm, stat, type FileHandle } from "node:fs/promises"
import { basename, dirname, join, resolve } from "node:path"
import { dataFolder } from "./core/evie.ts"

export type Settings = Record<string, string | undefined>

// Evie's settings: the real environment, plus a `.env` file in the data
// folder for anything the environment does not set. The file is where keys
// can be kept outside any repository.
//
// The file's values are returned to Evie only. They are not put into the
// process's own environment, so commands Evie runs later do not inherit them.
export async function loadSettings(environment: Settings): Promise<Settings> {
  const file = join(dataFolder(environment), ".env")
  const text = await readFile(file, "utf8").catch(() => "")
  return { ...parse(text), ...defined(environment) }
}

// A `NAME=value` line, as the file is read.
const LINE = /^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$/

// Reads `NAME=value` lines. Blank lines, comments and anything else are skipped.
function parse(text: string): Record<string, string> {
  const settings: Record<string, string> = {}
  for (const line of text.split("\n")) {
    const match = LINE.exec(line)
    if (!match) continue
    const [, name, raw] = match
    const quoted = /^(["'])(.*)\1$/.exec(raw!)
    settings[name!] = quoted ? quoted[2]! : raw!
  }
  return settings
}

function defined(environment: Settings): Record<string, string> {
  const settings: Record<string, string> = {}
  for (const [name, value] of Object.entries(environment)) {
    if (value !== undefined) settings[name] = value
  }
  return settings
}

// The file's text with a key set to a value, or with null taken out. The
// key's first line is replaced where it is, keeping an `export` it has, and
// any later one taken out, so that the file holds it once; a key not there
// yet goes on a line of its own at the end. Every other line is kept as it
// is, and so are the file's line endings.
export function withKey(text: string, name: string, value: string | null): string {
  const end = text.includes("\r\n") ? "\r\n" : "\n"
  const lines = text.split(end)
  const at = lines.findIndex((line) => LINE.exec(line)?.[1] === name)
  if (at === -1) {
    if (value === null) return text
    return `${text === "" || text.endsWith(end) ? text : `${text}${end}`}${name}=${value}${end}`
  }
  const kept = lines.filter((line, index) => index <= at || LINE.exec(line)?.[1] !== name)
  if (value === null) kept.splice(at, 1)
  else kept[at] = `${/^\s*export\s/.test(kept[at]!) ? "export " : ""}${name}=${value}`
  return kept.join(end)
}

// The keys the data folder's `.env` file sets, as it is now.
export async function keysInFile(home: string): Promise<Record<string, string>> {
  return parse(await readFile(join(home, ".env"), "utf8").catch(() => ""))
}

// Why a key's value cannot go in the file as it is, or undefined when it
// can. The file is read a line at a time, so the value must be one line,
// and read back as it was written.
export function keyValueProblem(name: string, value: string): string | undefined {
  if (value === "") return "Give the key's value, or clear it."
  if (/[\r\n]/.test(value)) return "A key's value is one line, and this one has a line break in it."
  if (parse(`${name}=${value}`)[name] !== value) return "A key's value is one line, with no spaces or quotes around it."
  return undefined
}

// Sets a key in the data folder's `.env` file, or with null takes it out,
// in a file only the owner's user can read. Changes are made one at a time.
let writing: Promise<void> = Promise.resolve()
export function writeKey(home: string, name: string, value: string | null): Promise<void> {
  const written = writing.then(async () => {
    const file = join(home, ".env")
    const text = await readFile(file, "utf8").catch((cause: { code?: string }) => (cause.code === "ENOENT" ? "" : Promise.reject(cause)))
    await replaceFile(file, withKey(text, name, value), 0o600)
  })
  writing = written.catch(() => {})
  return written
}

// Replaces a settings file's text in one step, so that it is never read
// half written: the text is written to a file beside it, made new under a
// name of its own, flushed to the disk, and moved into its place; when any
// of that fails, the file beside it is removed, so no copy is left. A file
// that is a link is replaced where the link leads, even when nothing is
// there yet, so the link stays. Replacements of one file are made one at a
// time. The file keeps its mode, or takes `mode`, as 0o600 for one only the
// owner's user can read.
const replacing = new Map<string, Promise<void>>()
export function replaceFile(file: string, text: string, mode?: number): Promise<void> {
  const replaced = (replacing.get(file) ?? Promise.resolve()).then(async () => {
    const target = await whereItLeads(file)
    const keep = mode ?? (await stat(target).then((found) => found.mode & 0o777, () => undefined))
    const beside = join(dirname(target), `.${basename(target)}.${randomBytes(8).toString("hex")}.new`)
    let handle: FileHandle | undefined
    try {
      handle = await open(beside, "wx", keep ?? 0o666)
      if (keep !== undefined) await handle.chmod(keep)
      await handle.writeFile(text)
      await handle.sync()
      await handle.close()
      handle = undefined
      await rename(beside, target)
    } catch (cause) {
      await handle?.close().catch(() => {})
      await rm(beside, { force: true })
      throw cause
    }
  })
  const settled = replaced.catch(() => {})
  replacing.set(file, settled)
  void settled.then(() => replacing.get(file) === settled && replacing.delete(file))
  return replaced
}

// Where writing to a file lands: the file itself, or for a link where it
// leads, followed one link at a time, so that a link to a file not there
// yet leads to where that file will be. A link that leads round in a circle
// is refused.
const MOST_LINKS = 40
async function whereItLeads(file: string): Promise<string> {
  let path = file
  for (let links = 0; links < MOST_LINKS; links++) {
    const found = await lstat(path).catch(() => undefined)
    if (!found?.isSymbolicLink()) return path
    path = resolve(dirname(path), await readlink(path))
  }
  throw new Error(`${file} is a link that leads round in a circle, so it was not written.`)
}
