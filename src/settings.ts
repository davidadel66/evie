import { readFile, rename, rm, writeFile } from "node:fs/promises"
import { join } from "node:path"
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
// key's first line is replaced where it is and any later one taken out, so
// that the file holds it once; a key not there yet goes on a line of its
// own at the end. Every other line is kept as it is.
export function withKey(text: string, name: string, value: string | null): string {
  const lines = text.split("\n")
  const at = lines.findIndex((line) => LINE.exec(line)?.[1] === name)
  if (at === -1) {
    if (value === null) return text
    return `${text === "" || text.endsWith("\n") ? text : `${text}\n`}${name}=${value}\n`
  }
  const kept = lines.filter((line, index) => index <= at || LINE.exec(line)?.[1] !== name)
  if (value === null) kept.splice(at, 1)
  else kept[at] = `${name}=${value}`
  return kept.join("\n")
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

// Sets a key in the data folder's `.env` file, or with null takes it out.
// The file is replaced in one step, so it is never left half written, by
// one only the owner's user can read. Changes are made one at a time.
let writing: Promise<void> = Promise.resolve()
export function writeKey(home: string, name: string, value: string | null): Promise<void> {
  const written = writing.then(async () => {
    const file = join(home, ".env")
    const text = await readFile(file, "utf8").catch((cause: { code?: string }) => (cause.code === "ENOENT" ? "" : Promise.reject(cause)))
    await rm(`${file}.new`, { force: true })
    await writeFile(`${file}.new`, withKey(text, name, value), { mode: 0o600 })
    await rename(`${file}.new`, file)
  })
  writing = written.catch(() => {})
  return written
}
