import { readFile, stat } from "node:fs/promises"
import { join } from "node:path"
import { onThisComputer } from "../server/wire.ts"
import { ownEvie, storageFileAt } from "./storage.ts"

// A project's configured dashboards, read from `<project>/.evie/evie.json`.
// Each entry is either a static file inside `.evie/`, a live local web app
// reached on a loopback address, or an invalid entry kept in place with the
// reason it was refused so the owner can see what to fix. Untrusted projects
// expose no dashboards.

export type DashboardEntry =
  | { id: string; name: string; kind: "static"; path: string }
  | { id: string; name: string; kind: "live"; url: string; start?: string; idle?: number | "never" }
  | { id: string; name: string; kind: "invalid"; error: string }

const VALID_ID = /^[a-z0-9][a-z0-9-]*$/

// Reads and validates the `dashboards` list from `<projectFolder>/.evie/evie.json`.
// Never throws on bad config: missing or unreadable config yields `[]`, and
// individual malformed entries become `{ kind: "invalid", error }`.
export async function readDashboards(projectFolder: string, dataFolder: string, trusted: boolean): Promise<DashboardEntry[]> {
  if (!trusted) return []
  const config = await readProjectConfig(projectFolder)
  if (config === undefined || config.dashboards === undefined) return []
  if (!Array.isArray(config.dashboards)) {
    return [{ id: "dashboards", name: "Dashboards", kind: "invalid", error: 'In .evie/evie.json, "dashboards" must be a list.' }]
  }

  const usedIds = new Set<string>()
  const entries: DashboardEntry[] = []
  for (const raw of config.dashboards) {
    entries.push(await parseEntry(raw, projectFolder, dataFolder, usedIds))
  }
  return entries
}

// Reads `<projectFolder>/.evie/evie.json` as a plain object, or nothing when
// it is absent, unreadable, invalid JSON, or not a plain object.
async function readProjectConfig(projectFolder: string): Promise<Record<string, unknown> | undefined> {
  let raw: string
  try {
    raw = await readFile(join(projectFolder, ".evie", "evie.json"), "utf8")
  } catch {
    return undefined
  }
  try {
    const parsed: unknown = JSON.parse(raw)
    return isPlainObject(parsed) ? parsed : undefined
  } catch {
    return undefined
  }
}

async function parseEntry(
  raw: unknown,
  projectFolder: string,
  dataFolder: string,
  usedIds: Set<string>,
): Promise<DashboardEntry> {
  if (!isPlainObject(raw)) {
    const name = "Dashboard"
    const id = nextId(undefined, name, usedIds)
    return { id, name, kind: "invalid", error: "Each dashboard entry must be an object." }
  }

  const rawName = typeof raw.name === "string" ? raw.name.trim() : ""
  const name = rawName !== "" ? rawName : "Dashboard"
  const id = nextId(raw.id, name, usedIds)

  if (rawName === "") {
    return { id, name, kind: "invalid", error: 'Each dashboard must have a non-empty "name".' }
  }

  const hasUrl = raw.url !== undefined
  const hasPath = raw.path !== undefined
  if (hasUrl && hasPath) {
    return { id, name, kind: "invalid", error: 'A dashboard must specify either "url" or "path", not both.' }
  }
  if (!hasUrl && !hasPath) {
    return { id, name, kind: "invalid", error: 'A dashboard must specify either "url" or "path".' }
  }

  if (hasUrl) {
    return parseLiveEntry(id, name, raw)
  }
  return parseStaticEntry(id, name, raw.path, projectFolder, dataFolder)
}

function parseLiveEntry(id: string, name: string, raw: Record<string, unknown>): DashboardEntry {
  if (typeof raw.url !== "string" || raw.url.trim() === "") {
    return { id, name, kind: "invalid", error: 'A live dashboard "url" must be a non-empty loopback http(s) URL.' }
  }
  const url = raw.url.trim()
  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return { id, name, kind: "invalid", error: `Live dashboard URL "${url}" is not a valid loopback http(s) URL.` }
  }
  if ((parsed.protocol !== "http:" && parsed.protocol !== "https:") || !onThisComputer(url)) {
    return {
      id,
      name,
      kind: "invalid",
      error: `Live dashboard URL "${url}" is not allowed: live dashboards must use a loopback http(s) URL (127.0.0.1, localhost, or [::1]).`,
    }
  }

  let start: string | undefined
  if (raw.start !== undefined) {
    if (typeof raw.start !== "string" || raw.start.trim() === "") {
      return { id, name, kind: "invalid", error: 'When set, a live dashboard "start" command must be a non-empty string.' }
    }
    start = raw.start.trim()
  }

  let idle: number | "never" | undefined
  if (raw.idle !== undefined) {
    if (raw.idle === "never") {
      idle = "never"
    } else if (typeof raw.idle === "number" && Number.isFinite(raw.idle) && raw.idle > 0) {
      idle = raw.idle
    } else {
      return { id, name, kind: "invalid", error: 'When set, "idle" must be "never" or a positive number of minutes.' }
    }
  }

  const entry: Extract<DashboardEntry, { kind: "live" }> = { id, name, kind: "live", url }
  if (start !== undefined) entry.start = start
  if (idle !== undefined) entry.idle = idle
  return entry
}

async function parseStaticEntry(
  id: string,
  name: string,
  rawPath: unknown,
  projectFolder: string,
  dataFolder: string,
): Promise<DashboardEntry> {
  if (typeof rawPath !== "string" || rawPath.trim() === "") {
    return { id, name, kind: "invalid", error: 'A static dashboard "path" must be a non-empty path inside .evie/.' }
  }
  const path = rawPath.trim()

  // Verify the `.evie` folder itself is valid and surface the specific reason if refused.
  try {
    ownEvie(projectFolder, dataFolder)
  } catch (cause) {
    const error = cause instanceof Error ? cause.message : String(cause)
    return { id, name, kind: "invalid", error }
  }

  const resolved = await storageFileAt(projectFolder, path, dataFolder)
  if (resolved === undefined) {
    return {
      id,
      name,
      kind: "invalid",
      error: `Dashboard path "${path}" is missing or outside this project's .evie folder.`,
    }
  }
  const facts = await stat(resolved).catch(() => undefined)
  if (!facts?.isFile()) {
    return { id, name, kind: "invalid", error: `Dashboard path "${path}" is not a file in this project's .evie folder.` }
  }

  return { id, name, kind: "static", path }
}

function nextId(rawId: unknown, name: string, used: Set<string>): string {
  const base =
    typeof rawId === "string" && VALID_ID.test(rawId)
      ? rawId
      : slugify(name)
  if (!used.has(base)) {
    used.add(base)
    return base
  }
  let suffix = 2
  while (used.has(`${base}-${suffix}`)) suffix++
  const unique = `${base}-${suffix}`
  used.add(unique)
  return unique
}

function slugify(name: string): string {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
  return slug !== "" ? slug : "dashboard"
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}
