import { afterEach, expect, test } from "bun:test"
import { mkdir, mkdtemp, readdir, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { dirname, join } from "node:path"
import { brotliDecompressSync, gunzipSync } from "node:zlib"
import { createHighlighterCore } from "shiki/core"
import { createJavaScriptRegexEngine } from "shiki/engine/javascript"
import { builtWebApp } from "../src/programs/serve.ts"
import { shikiAddress } from "../src/server/shiki-files.ts"
import { buildWebApp, type BuiltWebApp } from "../src/server/web-app.ts"
import { scriptedProvider } from "./support/scripted-provider.ts"
import { serveEvie } from "./support/served.ts"

// Shiki's grammars and themes, which colour the code in a diff, left out of
// the web app's build and served from Shiki's own files when the page first
// asks for one.

const cleanups: (() => Promise<void>)[] = []
afterEach(async () => {
  for (const cleanup of cleanups.splice(0).reverse()) await cleanup()
})

// The web app, built once for the tests that read it or serve it.
let built: Promise<BuiltWebApp> | undefined
const webApp = () => (built ??= buildWebApp())

// The server, serving the web app.
async function served() {
  const served = await serveEvie([scriptedProvider([]).plugin], { app: await webApp() })
  cleanups.push(served.stop)
  return served.server
}

// A new empty folder, removed once the test is done.
async function newFolder(prefix: string) {
  const folder = await mkdtemp(join(tmpdir(), prefix))
  cleanups.push(() => rm(folder, { recursive: true, force: true }))
  return folder
}

// The text of every script built.
const scriptsOf = (app: BuiltWebApp) =>
  Object.values(app.files)
    .filter((file) => file.type.startsWith("text/javascript"))
    .map((file) => new TextDecoder().decode(file.identity))
    .join("\n")

// The Shiki the diff library uses, and the names its lists import with
// `import("@shikijs/langs/<name>")` and `import("@shikijs/themes/<name>")`.
const shiki = dirname(Bun.resolveSync("shiki/package.json", dirname(Bun.resolveSync("@pierre/diffs", import.meta.dir))))
const importedIn = async (file: string, kind: string) => new Set([...(await Bun.file(file).text()).matchAll(new RegExp(`import\\("@shikijs/${kind}/([^"]+)"\\)`, "g"))].map((match) => match[1]!))

test("the web app is built without Shiki's grammars and themes: each is imported from the server's address for it, which carries Shiki's version, and every language is kept", async () => {
  const app = await webApp()
  const { version } = await Bun.file(join(shiki, "package.json")).json()
  expect(shikiAddress()).toBe(`/shiki/${version}/`)

  // No file of Shiki's grammars or themes is built in.
  expect(Object.keys(app.metafile.inputs).filter((path) => /\/@shikijs\/(langs|themes)\//.test(`/${path}`))).toEqual([])
  // Each is imported from its address: every grammar Shiki lists, and
  // every theme of the diff library's list of Shiki's.
  const scripts = scriptsOf(app)
  const address = shikiAddress().replaceAll(".", "\\.")
  const addressed = (kind: string) => new Set([...scripts.matchAll(new RegExp(`import\\(["']${address}${kind}/([a-z0-9-]+)\\.mjs["']\\)`, "g"))].map((match) => match[1]!))
  const grammars = (await readdir(join(shiki, "dist"))).find((name) => /^langs-bundle-full-.+\.mjs$/.test(name))!
  expect(addressed("langs")).toEqual(await importedIn(join(shiki, "dist", grammars), "langs"))
  expect(addressed("langs").size).toBeGreaterThan(200)
  const theming = dirname(Bun.resolveSync("@pierre/theming/themes", dirname(Bun.resolveSync("@pierre/diffs", import.meta.dir))))
  expect(addressed("themes")).toEqual(await importedIn(join(theming, "collections", "shiki.js"), "themes"))
  expect(scripts).not.toMatch(/import\(["']@shikijs\//)
})

test("a grammar the web app imports loads from the server, compressed, with the grammars it imports beside it, and colours code in several colours with a theme served the same way", async () => {
  const server = await served()
  const folder = await newFolder("evie-shiki-")
  expect(scriptsOf(await webApp())).toContain(`"${shikiAddress()}langs/html.mjs"`)

  // Each file as the browser has it, with each it imports from beside it,
  // saved in a folder of their own, by name.
  const save = async (url: URL) => {
    const name = url.pathname.split("/").at(-1)!
    if (await Bun.file(join(folder, name)).exists()) return
    const answer = await fetch(url, { headers: { "Accept-Encoding": "gzip, deflate, br, zstd" } })
    expect([url.pathname, answer.status, answer.headers.get("content-encoding"), answer.headers.get("cache-control")]).toEqual([url.pathname, 200, "br", "public, max-age=31536000, immutable"])
    const source = await answer.text()
    await writeFile(join(folder, name), source)
    for (const match of source.matchAll(/\bfrom\s+["'](\.\/[^"']+)["']/g)) await save(new URL(match[1]!, url))
  }
  await save(new URL(`${shikiAddress()}langs/html.mjs`, server.url))
  await save(new URL(`${shikiAddress()}themes/github-dark.mjs`, server.url))
  // HTML's grammar imports JavaScript's and CSS's, from beside it.
  expect(await readdir(folder)).toEqual(expect.arrayContaining(["html.mjs", "javascript.mjs", "css.mjs", "github-dark.mjs"]))

  const { default: html } = await import(join(folder, "html.mjs"))
  const { default: theme } = await import(join(folder, "github-dark.mjs"))
  const highlighter = await createHighlighterCore({ langs: [html], themes: [theme], engine: createJavaScriptRegexEngine() })
  const code = '<p class="note">Water at dawn</p>\n<script>const beds = 3; console.log("beds", beds)</script>\n'
  const colours = new Set(highlighter.codeToTokensBase(code, { lang: "html", theme: "github-dark" }).flat().map((token) => token.color))
  expect(colours.size).toBeGreaterThanOrEqual(3)
  highlighter.dispose()
})

test("a grammar is sent with brotli, gzip or as it is, as asked, and answers 304 to the tag of what the browser has; asked for many times at once, each answer is the same", async () => {
  const server = await served()
  const url = `${server.url}${shikiAddress()}langs/python.mjs`
  const plain = Buffer.from(await Bun.file(join(dirname(Bun.resolveSync("@shikijs/langs", shiki)), "python.mjs")).bytes())
  const asked = (headers: Record<string, string>) => fetch(url, { headers, decompress: false })

  const answers = await Promise.all([asked({ "Accept-Encoding": "br" }), asked({ "Accept-Encoding": "br" }), asked({ "Accept-Encoding": "gzip" }), asked({ "Accept-Encoding": "identity" })])
  const [brotli, again, gzip, neither] = answers
  expect(answers.map((answer) => [answer.status, answer.headers.get("content-type"), answer.headers.get("cache-control"), answer.headers.get("vary")])).toEqual(
    answers.map(() => [200, "text/javascript;charset=utf-8", "public, max-age=31536000, immutable", "Accept-Encoding"]),
  )
  expect(brotliDecompressSync(Buffer.from(await brotli!.arrayBuffer()))).toEqual(plain)
  expect(brotliDecompressSync(Buffer.from(await again!.arrayBuffer()))).toEqual(plain)
  expect(gunzipSync(Buffer.from(await gzip!.arrayBuffer()))).toEqual(plain)
  expect(Buffer.from(await neither!.arrayBuffer())).toEqual(plain)
  expect(again!.headers.get("etag")).toBe(brotli!.headers.get("etag")!)

  const kept = await asked({ "Accept-Encoding": "br", "If-None-Match": brotli!.headers.get("etag")! })
  expect([kept.status, await kept.text()]).toEqual([304, ""])
})

test("only a plain name in Shiki's two folders is read: a name not there, one not plain, another version of Shiki's, or a path out of the folders answers 404", async () => {
  const server = await served()

  for (const path of [
    "langs/no-such-language.mjs",
    "themes/no-such-theme.mjs",
    // On a disk that ignores case, this file is there.
    "langs/TypeScript.mjs",
    "langs/typescript.d.mts",
    "langs/typescript",
    "others/typescript.mjs",
    "langs/..%2Fpackage.json",
    "langs/..%2F..%2F..%2Fshiki%2Fpackage.json",
    "langs/%2e%2e%2f%2e%2e%2fthemes%2fnord.mjs",
    "..%2F..%2Fpackage.json",
  ]) {
    const answer = await fetch(`${server.url}${shikiAddress()}${path}`)
    expect([path, answer.status]).toEqual([path, 404])
  }
  expect((await fetch(`${server.url}/shiki/0.0.1/langs/typescript.mjs`)).status).toBe(404)
  // A theme that is there is sent.
  expect((await fetch(`${server.url}${shikiAddress()}themes/nord.mjs`)).status).toBe(200)
})

test("the build fails, and says why, when a list of Shiki's has nothing to rewrite, or imports a grammar from elsewhere than the server sends it from, as after an upgrade of Shiki that moves its files", async () => {
  const folder = await newFolder("evie-shiki-lists-")
  const temporary = join(folder, "temporary")
  await mkdir(temporary)
  // Shiki's list of themes, as the build finds it in a page's node_modules:
  // with no import to rewrite, and importing a theme that the server's
  // copy of Shiki's themes does not hold.
  const list = join(folder, "node_modules", "shiki", "dist", "themes.mjs")
  await mkdir(dirname(list), { recursive: true })
  await writeFile(join(folder, "index.html"), '<!doctype html><html><body><script type="module" src="./main.ts"></script></body></html>\n')
  await writeFile(join(folder, "main.ts"), 'import { bundledThemesInfo } from "./node_modules/shiki/dist/themes.mjs"\nconsole.log(bundledThemesInfo)\n')
  const build = () => builtWebApp(async () => ({ buildWebApp: () => buildWebApp({ page: join(folder, "index.html"), temporary }) }))

  await writeFile(list, "export const bundledThemesInfo = []\n")
  const empty = await build()
  expect(empty).toEqual({ problem: expect.stringContaining("node_modules/shiki/dist/themes.mjs lists none of Shiki's grammars or themes") })

  await writeFile(list, 'export const bundledThemesInfo = [{ id: "nord", import: () => import("@shikijs/themes/nord") }]\n')
  const elsewhere = await build()
  expect(elsewhere).toEqual({ problem: expect.stringContaining('node_modules/shiki/dist/themes.mjs imports "@shikijs/themes/nord"') })
  expect(await readdir(temporary)).toEqual([])
})

test("the build fails when one of Shiki's grammars would be built into the web app, as when a module the build does not rewrite imports one", async () => {
  const folder = await newFolder("evie-shiki-built-in-")
  const grammar = join(dirname(Bun.resolveSync("@shikijs/langs", shiki)), "typescript.mjs")
  await writeFile(join(folder, "index.html"), '<!doctype html><html><body><script type="module" src="./main.ts"></script></body></html>\n')
  await writeFile(join(folder, "main.ts"), `import(${JSON.stringify(grammar)}).then((grammar) => console.log(grammar))\n`)

  const built = await builtWebApp(async () => ({ buildWebApp: () => buildWebApp({ page: join(folder, "index.html") }) }))

  expect(built).toEqual({ problem: expect.stringContaining("typescript.mjs, one of Shiki's grammars or themes, was built into the web app") })
})
