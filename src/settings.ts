import { readFile } from "node:fs/promises"
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

// Reads `NAME=value` lines. Blank lines, comments and anything else are skipped.
function parse(text: string): Record<string, string> {
  const settings: Record<string, string> = {}
  for (const line of text.split("\n")) {
    const match = /^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$/.exec(line)
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
