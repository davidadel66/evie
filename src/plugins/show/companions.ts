import { realpathSync } from "node:fs"
import { readFile, stat } from "node:fs/promises"
import { dirname, isAbsolute, relative, resolve, sep } from "node:path"
import { companionType, SHOWN_LIMIT } from "./kinds.ts"

// Finds companion files referenced by relative path from an HTML mockup, a
// glTF (.gltf) scene, or an OBJ (.obj) model, so that showing the main file
// copies its local styles, scripts, images, `.bin` buffers and textures
// beside it into Storage.
//
// Only relative paths staying within the main file's own folder tree (never
// climbing out with `..`, never absolute, never URLs with a scheme, and
// never following a symlink outside the root folder) are collected.

const MAX_COMPANIONS = 64
const MAX_COMPANION_TEXT = 2 * 1024 * 1024

export type Companion = {
  // Path relative to the main file's directory, using `/`.
  relative: string
  // Resolved real path on disk.
  real: string
  // File size in bytes.
  size: number
}

// Collects all safe relative companion files reachable from `mainReal`
// (starting from its own folder `rootDir`). Files that do not exist, climb
// outside `rootDir`, are symlinks pointing outside `rootDir`, or exceed the
// total size budget are skipped silently (so a missing optional asset does
// not prevent showing the page).
export async function findCompanions(mainReal: string, mainType: string): Promise<Companion[]> {
  const rootDir = realpathSync(dirname(mainReal))
  const found = new Map<string, Companion>()
  const visited = new Set<string>([mainReal])
  let totalBytes = 0

  const enqueue = async (fromDir: string, ref: string): Promise<Companion | undefined> => {
    const clean = cleanRef(ref)
    if (clean === undefined) return undefined
    const candidate = resolve(fromDir, clean)
    const rel = relative(rootDir, candidate)
    if (rel === "" || rel === ".." || rel.startsWith(`..${sep}`) || isAbsolute(rel)) return undefined
    let real: string
    try {
      real = realpathSync(candidate)
    } catch {
      return undefined
    }
    if (visited.has(real)) return undefined
    const realRel = relative(rootDir, real)
    if (realRel === "" || realRel === ".." || realRel.startsWith(`..${sep}`) || isAbsolute(realRel)) return undefined
    const posixRel = realRel.split(sep).join("/")
    if (found.has(posixRel) || found.size >= MAX_COMPANIONS) return undefined
    if (companionType(real) === undefined) return undefined
    const facts = await stat(real).catch(() => undefined)
    if (!facts?.isFile() || facts.size > SHOWN_LIMIT || totalBytes + facts.size > SHOWN_LIMIT) return undefined
    visited.add(real)
    totalBytes += facts.size
    const entry: Companion = { relative: posixRel, real, size: facts.size }
    found.set(posixRel, entry)
    return entry
  }

  const queue: { real: string; kind: "html" | "css" | "js" | "gltf" | "obj" | "mtl" }[] = []
  if (mainType === "text/html") queue.push({ real: mainReal, kind: "html" })
  else if (mainType === "model/gltf+json") queue.push({ real: mainReal, kind: "gltf" })
  else if (mainType === "model/obj") queue.push({ real: mainReal, kind: "obj" })

  while (queue.length > 0 && found.size < MAX_COMPANIONS) {
    const current = queue.shift()!
    const facts = await stat(current.real).catch(() => undefined)
    if (!facts?.isFile() || facts.size > MAX_COMPANION_TEXT) continue
    const text = await readFile(current.real, "utf8").catch(() => undefined)
    if (text === undefined) continue
    const fromDir = dirname(current.real)
    const refs = extractRefs(text, current.kind)
    for (const ref of refs) {
      const added = await enqueue(fromDir, ref)
      if (!added) continue
      const nextKind = kindForRecursion(added.real)
      if (nextKind !== undefined) queue.push({ real: added.real, kind: nextKind })
    }
  }

  return [...found.values()]
}

function kindForRecursion(path: string): "css" | "js" | "mtl" | undefined {
  const lower = path.toLowerCase()
  if (lower.endsWith(".css")) return "css"
  if (lower.endsWith(".js") || lower.endsWith(".mjs")) return "js"
  if (lower.endsWith(".mtl")) return "mtl"
  return undefined
}

// Strips query string and fragment, decodes URI escapes, and rejects URLs
// with a scheme, protocol-relative URLs, root-relative paths, or anchors.
export function cleanRef(raw: string): string | undefined {
  const trimmed = raw.trim()
  if (trimmed === "" || trimmed.startsWith("#") || trimmed.startsWith("/") || trimmed.startsWith("//")) return undefined
  if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(trimmed)) return undefined
  const cut = trimmed.split(/[?#]/, 1)[0]!
  if (cut === "") return undefined
  try {
    return decodeURIComponent(cut)
  } catch {
    return cut
  }
}

export function extractRefs(text: string, kind: "html" | "css" | "js" | "gltf" | "obj" | "mtl"): string[] {
  switch (kind) {
    case "html":
      return extractHtmlRefs(text)
    case "css":
      return extractCssRefs(text)
    case "js":
      return extractJsRefs(text)
    case "gltf":
      return extractGltfRefs(text)
    case "obj":
      return extractObjRefs(text)
    case "mtl":
      return extractMtlRefs(text)
  }
}

function extractHtmlRefs(html: string): string[] {
  const refs: string[] = []
  // src="..." / href="..." / poster="..."
  for (const match of html.matchAll(/\b(?:src|href|poster)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+))/gi)) {
    const val = match[1] ?? match[2] ?? match[3]
    if (val) refs.push(val)
  }
  // srcset="a.png 1x, b.png 2x"
  for (const match of html.matchAll(/\bsrcset\s*=\s*(?:"([^"]*)"|'([^']*)')/gi)) {
    const srcset = match[1] ?? match[2] ?? ""
    for (const part of srcset.split(",")) {
      const url = part.trim().split(/\s+/, 1)[0]
      if (url) refs.push(url)
    }
  }
  // Inline <style> blocks
  for (const match of html.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/gi)) {
    refs.push(...extractCssRefs(match[1]!))
  }
  // Inline <script> blocks
  for (const match of html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/gi)) {
    refs.push(...extractJsRefs(match[1]!))
  }
  return refs
}

function extractCssRefs(css: string): string[] {
  const refs: string[] = []
  for (const match of css.matchAll(/url\(\s*(?:"([^"]*)"|'([^']*)'|([^)"'\s]+))\s*\)/gi)) {
    const val = match[1] ?? match[2] ?? match[3]
    if (val) refs.push(val)
  }
  for (const match of css.matchAll(/@import\s+(?:"([^"]*)"|'([^']*)')/gi)) {
    const val = match[1] ?? match[2]
    if (val) refs.push(val)
  }
  return refs
}

function extractJsRefs(js: string): string[] {
  const refs: string[] = []
  // Static import/export from "./..."
  for (const match of js.matchAll(/\b(?:import|export)\b[^"'`]*?\bfrom\s*["']([^"']+)["']/g)) {
    refs.push(match[1]!)
  }
  // Side-effect import "./..."
  for (const match of js.matchAll(/\bimport\s*["']([^"']+)["']/g)) {
    refs.push(match[1]!)
  }
  // Dynamic import("./...") or fetch("./...")
  for (const match of js.matchAll(/\b(?:import|fetch)\s*\(\s*["']([^"']+)["']/g)) {
    refs.push(match[1]!)
  }
  return refs
}

function extractGltfRefs(jsonText: string): string[] {
  try {
    const parsed = JSON.parse(jsonText) as { buffers?: { uri?: unknown }[]; images?: { uri?: unknown }[] }
    const refs: string[] = []
    for (const item of [...(parsed.buffers ?? []), ...(parsed.images ?? [])]) {
      if (typeof item?.uri === "string") refs.push(item.uri)
    }
    return refs
  } catch {
    return []
  }
}

function extractObjRefs(obj: string): string[] {
  const refs: string[] = []
  for (const line of obj.split(/\r?\n/)) {
    const match = /^\s*mtllib\s+(.+)$/i.exec(line)
    if (match) refs.push(match[1]!.trim())
  }
  return refs
}

function extractMtlRefs(mtl: string): string[] {
  const refs: string[] = []
  for (const line of mtl.split(/\r?\n/)) {
    const match = /^\s*(?:map_[A-Za-z0-9_]+|bump|disp|decal|refl)\s+(.+)$/i.exec(line)
    if (!match) continue
    // Texture map directives may carry flags before the filename; the path is the last token.
    const tokens = match[1]!.trim().split(/\s+/)
    const last = tokens.at(-1)
    if (last) refs.push(last)
  }
  return refs
}
