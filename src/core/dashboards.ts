import { stat } from "node:fs/promises"
import { ownEvie, storageFileAt } from "./storage.ts"
import type { DashboardEntry } from "./types.ts"

// A workspace's dashboards, from the `dashboards` list in its project's
// `.evie/evie.json`, in order. Each is a live app, reached at a `url` on
// this computer, or a static HTML file, by its `path` inside `.evie/`. An
// entry that cannot be used is kept in its place, with why, so that its tab
// says what to fix.

// The hosts of an address on this computer, the only ones a live dashboard may have.
const ON_THIS_COMPUTER = ["127.0.0.1", "localhost", "[::1]"]
const ID = /^[a-z0-9][a-z0-9-]*$/

// The entries of a project's `dashboards` setting, as its settings file gives
// it: undefined when the file sets none or the project is not trusted.
export async function readDashboards(setting: unknown, projectFolder: string, dataFolder: string): Promise<DashboardEntry[]> {
  if (setting === undefined) return []
  if (!Array.isArray(setting)) return [{ id: "dashboards", name: "Dashboards", kind: "invalid", error: 'In .evie/evie.json, "dashboards" must be a list.' }]
  const ids = new Set<string>()
  const entries: DashboardEntry[] = []
  for (const entry of setting) entries.push(await read(entry, ids, projectFolder, dataFolder))
  return entries
}

async function read(entry: unknown, ids: Set<string>, projectFolder: string, dataFolder: string): Promise<DashboardEntry> {
  const given = typeof entry === "object" && entry !== null && !Array.isArray(entry) ? (entry as Record<string, unknown>) : undefined
  const named = typeof given?.name === "string" && given.name.trim() !== "" ? given.name.trim() : undefined
  const name = named ?? "Dashboard"
  const id = unique(typeof given?.id === "string" && ID.test(given.id) ? given.id : slug(name), ids)
  const invalid = (error: string): DashboardEntry => ({ id, name, kind: "invalid", error })
  if (given === undefined) return invalid('Each entry under "dashboards" must be an object, such as { "name": "Overview", "path": "dashboards/overview.html" }.')
  if (named === undefined) return invalid('Each dashboard needs a "name", for its tab.')
  if ((given.url === undefined) === (given.path === undefined)) return invalid('A dashboard has either a "url", for a live app, or a "path", for an HTML file in .evie, and not both.')

  if (given.url !== undefined) {
    const url = typeof given.url === "string" ? given.url.trim() : ""
    const parsed = URL.canParse(url) ? new URL(url) : undefined
    if (parsed === undefined || !["http:", "https:"].includes(parsed.protocol) || !ON_THIS_COMPUTER.includes(parsed.hostname)) {
      return invalid(`"${url}" is not an address on this computer. A live dashboard's "url" starts with http://127.0.0.1, http://localhost or http://[::1].`)
    }
    if (given.start !== undefined && (typeof given.start !== "string" || given.start.trim() === "")) return invalid('A dashboard\'s "start" is the command that starts its app, such as "bun run dev".')
    return { id, name, kind: "live", url, ...(typeof given.start === "string" && { start: given.start.trim() }) }
  }

  const path = typeof given.path === "string" ? given.path.trim() : ""
  try {
    ownEvie(projectFolder, dataFolder)
  } catch (cause) {
    return invalid(cause instanceof Error ? cause.message : String(cause))
  }
  const file = path === "" ? undefined : await storageFileAt(projectFolder, path, dataFolder)
  if (file === undefined || !(await stat(file).catch(() => undefined))?.isFile()) {
    return invalid(`There is no file "${path}" in this project's .evie folder. A static dashboard's "path" is an HTML file inside it.`)
  }
  return { id, name, kind: "static", path }
}

// An id for the tab's address, from its name, made unique by a number after it.
function unique(base: string, ids: Set<string>): string {
  let id = base
  for (let next = 2; ids.has(id); next++) id = `${base}-${next}`
  ids.add(id)
  return id
}

const slug = (name: string) =>
  name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "") || "dashboard"
