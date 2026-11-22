import type { BuildMetafile, HTMLBundle } from "bun"
import { mkdtemp, readdir, rm, stat } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import page from "../../web/index.html"
import type { Built } from "./build-web-app.ts"

// The web app, for the server to serve: its page, and the files the page
// loads, each by the path it is served at. The page is either built already,
// or in development Bun's own import of it, which Bun bundles again on each
// request and updates in the open browser as its files change.
export type WebApp = { page: Blob; files: Record<string, Blob> } | { page: HTMLBundle }

// The web app as it is built, with Bun's metafile, which names each file
// built (by the path it was written at, such as `./chunk-1a2b3c4d.js`), the
// files in the repository it was built from (such as `web/index.html`), and
// the chunks each imports. The page is finished from it as it is built, so
// the server is handed the page and its files without it: about 0.8 MB of
// JSON it has no more use for, and would keep for as long as it runs.
export type BuiltWebApp = { page: Blob; files: Record<string, Blob>; metafile: BuildMetafile }

// The page Bun's bundler starts from: it follows the page's script and
// stylesheet, and everything they import.
const PAGE = join(import.meta.dir, "..", "..", "web", "index.html")

// The script that builds it, in a process of its own.
const BUILD = join(import.meta.dir, "build-web-app.ts")

// What the name of each build's temporary folder starts with.
const TEMPORARY = "evie-web-app-"

// Builds the web app, minified, as `evie serve` does when it starts, so there
// is no build step. Bun's bundler keeps its working memory, about 300 MB, for
// as long as the process that ran it lives, so the build runs in a process
// of its own, which exits once it is done, and the server keeps only the
// files built.
//
// That process is the same Bun as the server, whatever `bun` the path
// finds. It is started in the repository's folder, whatever folder the
// server was started in: Bun reads the repository's own bunfig.toml, and its
// .env if it has one, and none from the server's folder, and the metafile
// names files by their paths in the repository. It writes into a temporary
// folder, from which the files are read into memory, and the folder is
// removed, whether the build worked or not. A build that fails says why,
// each of the bundler's messages as `file:line: message`. The page is then
// given, in its head, what the bundler does not write itself (`headLinks`).
//
// `page`, to build another page, and `temporary`, the folder the temporary
// folder is made in, are for tests.
export async function buildWebApp(options: { page?: string; temporary?: string } = {}): Promise<BuiltWebApp> {
  const temporary = options.temporary ?? tmpdir()
  const folder = await mkdtemp(join(temporary, TEMPORARY))
  // While it builds, the folders earlier builds left are removed.
  const tidying = removeLeftFolders(temporary)
  try {
    const [list, all] = [join(folder, "files.json"), join(folder, "files")]
    const child = Bun.spawn([process.execPath, BUILD, options.page ?? PAGE, list, all], {
      cwd: REPOSITORY,
      stdin: "ignore",
      stdout: "ignore",
      stderr: "pipe",
    })
    const [problem, code] = await Promise.all([new Response(child.stderr).text(), child.exited])
    if (code !== 0) throw new Error(problem.trim() || `The web app's build stopped with ${child.signalCode ?? `exit code ${code}`}, saying nothing.`)
    // Each file, by the path it is served at, cut from the one file the
    // build wrote them all into, in the order of its list.
    const built: Built = await Bun.file(list).json()
    const bytes = await Bun.file(all).bytes()
    const files: Record<string, Blob> = {}
    let at = 0
    for (const file of built.files) {
      files[servedAt(file.path)] = new Blob([bytes.subarray(at, at + file.size)], { type: file.type })
      at += file.size
    }
    const { "/index.html": builtPage, ...loaded } = files
    if (!builtPage) throw new Error("Bun's bundler built the web app without its page.")
    const page = new Blob([withHeadLinks(await builtPage.text(), headLinks(built.metafile))], { type: builtPage.type })
    return { page, files: loaded, metafile: built.metafile }
  } finally {
    await Promise.all([rm(folder, { recursive: true, force: true }), tidying])
  }
}

// The path a built file is served at: the bundler names each by the path it
// was written at, such as `./chunk-1a2b3c4d.js`, and the page names it from
// the root, `/chunk-1a2b3c4d.js`.
const servedAt = (path: string) => path.replace(/^\./, "")

// A link for the built page's head, by its attributes, such as
// `{ rel: "modulepreload", href: "/chunk-1a2b3c4d.js" }`. An attribute with
// no value, such as `crossorigin`, is given as "".
export type HeadLink = { rel: string; href: string; [attribute: string]: string }

// What the built page's head is given that Bun's bundler does not write
// itself, worked out from the build's metafile once, as the server starts.
// This is the one place the built page is added to.
export function headLinks(metafile: BuildMetafile): HeadLink[] {
  return chunkPreloads(metafile)
}

// A `modulepreload` link for each chunk the page's script imports, and each
// chunk those import in turn. The bundler splits out of the script what more
// than one part of the app shares, and the script imports it; without the
// links, the browser learns of those chunks only once it has the whole
// script, which on a phone's network is another round of requests before
// anything is drawn. With them, it asks for the chunks alongside the script.
// What is imported with `import()`, such as the diff view and what only it
// uses, loads when it is needed, so it is not linked.
//
// Bun 1.4.0 adds no such links, and builds this app's script with chunks it
// imports at once, so on 1.4.0 these links are what preloads them. Bun
// 1.4.5 builds the script with what it would import at once folded into it,
// so for this app the list is empty. Where a build does import chunks at
// once, 1.4.5's `modulePreload` option, on by default, writes such links
// itself, and `withHeadLinks` then adds none again.
function chunkPreloads(metafile: BuildMetafile): HeadLink[] {
  const { outputs } = metafile
  // The page's own scripts: the outputs built from the same file as the page.
  const page = outputs["./index.html"]?.entryPoint
  const scripts = Object.keys(outputs).filter((path) => page !== undefined && outputs[path]!.entryPoint === page && path.endsWith(".js"))
  const imported = new Set<string>()
  const follow = (path: string) => {
    for (const each of outputs[path]?.imports ?? []) {
      if (each.kind !== "import-statement" || imported.has(each.path)) continue
      imported.add(each.path)
      follow(each.path)
    }
  }
  for (const script of scripts) follow(script)
  // `crossorigin`, as the bundler writes on the page's script, so that a
  // chunk is preloaded in the same way as the script's import of it fetches
  // it, and the browser uses the one it preloaded.
  return [...imported].map((path) => ({ rel: "modulepreload", href: servedAt(path), crossorigin: "" }))
}

// The page with each link added just before `</head>`, but for one it has
// already: one of the same kind (`rel`) for the same address, however it is
// written, as the bundler may write it itself. What is in a comment is not
// the page's own.
export function withHeadLinks(page: string, links: HeadLink[]): string {
  // Each kind a link is, with its address as the page resolves it, so
  // `./chunk.js` is `/chunk.js`.
  const kinds = (link: { rel?: string; href?: string }) => {
    const address = URL.parse(link.href ?? "", "http://page/")?.href ?? link.href
    return (link.rel ?? "").toLowerCase().split(/\s+/).filter(Boolean).map((rel) => `${rel} ${address}`)
  }
  // The page's links and the end of its head, in order, with each comment
  // passed over whole.
  const found = [...page.matchAll(/<!--[\s\S]*?-->|<link\b([^>]*)>|<\/head\s*>/gi)]
  const had = new Set(found.flatMap((match) => (match[1] === undefined ? [] : kinds(attributesOf(match[1])))))
  const added: HeadLink[] = []
  for (const link of links) {
    if (kinds(link).every((kind) => had.has(kind))) continue
    for (const kind of kinds(link)) had.add(kind)
    added.push(link)
  }
  if (added.length === 0) return page
  const end = found.find((match) => /^<\/head/i.test(match[0]))?.index
  if (end === undefined) throw new Error("The built page has no </head> to add its links before.")
  return page.slice(0, end) + added.map(tagOf).join("") + page.slice(end)
}

// A link written as a tag, such as `<link rel="modulepreload" href="/chunk-1a2b3c4d.js" crossorigin>`.
function tagOf(link: HeadLink): string {
  const quoted = (value: string) => `"${value.replaceAll("&", "&amp;").replaceAll('"', "&quot;")}"`
  return `<link${Object.entries(link).map(([name, value]) => (value === "" ? ` ${name}` : ` ${name}=${quoted(value)}`)).join("")}>`
}

// A tag's attributes, by name, from the text after its name, whether each
// value is quoted with " or ', unquoted, or not given.
function attributesOf(text: string): Record<string, string> {
  const attributes: Record<string, string> = {}
  for (const match of text.matchAll(/([^\s"'<>/=]+)(?:\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>`]+)))?/g)) {
    attributes[match[1]!.toLowerCase()] = match[2] ?? match[3] ?? match[4] ?? ""
  }
  return attributes
}

// Removes the temporary folders that builds left behind. A server stopped
// while it builds, as by Ctrl-C as it starts, stops before it can remove its
// folder. A build takes about half a second, so a folder untouched for an
// hour is no build's still running, such as another server's.
async function removeLeftFolders(temporary: string) {
  const hourAgo = Date.now() - 60 * 60 * 1000
  for (const name of await readdir(temporary).catch(() => [])) {
    if (!name.startsWith(TEMPORARY)) continue
    try {
      const folder = join(temporary, name)
      if ((await stat(folder)).mtimeMs < hourAgo) await rm(folder, { recursive: true, force: true })
    } catch {
      // Gone already, or another user's: left as it is.
    }
  }
}

// The web app for working on it: Bun bundles the page on each request and
// keeps the open page up to date as its files are saved, keeping the state
// of the components on screen. Bun takes the Tailwind plugin for this from
// `bunfig.toml`, which it reads from the folder it was started in, so the
// server must be started in the repository's folder, which is `REPOSITORY`.
export const webAppForDevelopment = (): WebApp => ({ page })
export const REPOSITORY = join(import.meta.dir, "..", "..")
