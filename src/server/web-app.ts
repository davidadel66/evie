import type { BuildMetafile, HTMLBundle } from "bun"
import { mkdtemp, readdir, rm, stat } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import page from "../../web/index.html"

// The web app, for the server to serve: its page, and the files the page
// loads, each by the path it is served at. The page is either built already,
// or in development Bun's own import of it, which Bun bundles again on each
// request and updates in the open browser as its files change. Built, it
// comes with Bun's metafile, which names each file built (by the path it was
// written at, such as `./chunk-1a2b3c4d.js`), the files in the repository it
// was built from (such as `web/index.html`), and the chunks each imports.
export type WebApp = { page: Blob; files: Record<string, Blob>; metafile: BuildMetafile } | { page: HTMLBundle }

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
// server was started in, so that it takes no bunfig.toml or .env from there,
// and the metafile names files by their paths in the repository. It writes
// into a temporary folder; the files are read from it into memory, and the
// folder is removed, whether the build worked or not. A build that fails
// says why, each of the bundler's messages as `file:line: message`.
//
// `page`, to build another page, and `temporary`, the folder the temporary
// folder is made in, are for tests.
export async function buildWebApp(options: { page?: string; temporary?: string } = {}): Promise<WebApp> {
  const temporary = options.temporary ?? tmpdir()
  const folder = await mkdtemp(join(temporary, TEMPORARY))
  // While it builds, the folders earlier builds left are removed.
  const tidying = removeLeftFolders(temporary)
  try {
    const built = join(folder, "files")
    const metafile = join(folder, "metafile.json")
    const child = Bun.spawn([process.execPath, BUILD, options.page ?? PAGE, built, metafile], {
      cwd: REPOSITORY,
      stdin: "ignore",
      stdout: "ignore",
      stderr: "pipe",
    })
    const [problem, code] = await Promise.all([new Response(child.stderr).text(), child.exited])
    if (code !== 0) throw new Error(problem.trim() || `The web app's build stopped with exit code ${code}, saying nothing.`)
    const files: Record<string, Blob> = {}
    for await (const path of new Bun.Glob("**/*").scan(built)) {
      const file = Bun.file(join(built, path))
      files[`/${path}`] = new Blob([await file.bytes()], { type: file.type })
    }
    const { "/index.html": builtPage, ...loaded } = files
    if (!builtPage) throw new Error("Bun's bundler built the web app without its page.")
    return { page: builtPage, files: loaded, metafile: await Bun.file(metafile).json() }
  } finally {
    await Promise.all([rm(folder, { recursive: true, force: true }), tidying])
  }
}

// Removes the temporary folders that builds left behind. A server stopped
// while it builds, as by Ctrl-C as it starts, stops before it can remove its
// folder. A build takes about a second, so a folder untouched for an hour is
// no build's still running, such as another server's.
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
