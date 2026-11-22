import type { BunPlugin } from "bun"
import { dirname, join, resolve, sep } from "node:path"
import { fileRoute, variantsOf } from "./variants.ts"

// Shiki's grammars and themes, sent to the page as Shiki ships them, not
// built into the web app.
//
// The diff library colours code with Shiki. A grammar is what tells Shiki
// how to colour one language, and a theme which colour each kind of word
// gets. Shiki lists its 242 grammars and 65 themes, and the diff library
// lists Shiki's themes again, each reached by an `import()` of its own, such
// as `import("@shikijs/langs/typescript")`. The bundler builds each into a
// file of the web app, and the files they share into more: about 400 of the
// app's 500 files, built at every start of the server, for languages the
// owner may never look at. evie colours with one theme of its own, and needs
// one grammar for each language shown.
//
// Shiki ships each one already as a module a browser can load, which imports
// any other it needs from beside it, as HTML's imports `./javascript.mjs`. So
// the build leaves them out (`shikiFiles`): in the three modules that list
// them, it rewrites each such import to an address the server answers with
// Shiki's own file (`shikiRoute`), such as `/shiki/4.6.0/langs/typescript.mjs`.
// The page downloads a grammar the first time a diff shows its language, and
// none other. The address carries Shiki's version, so a browser may keep a
// file for good, and an upgrade of Shiki changes every address.

type Kind = "langs" | "themes"

// The three modules that list them, by the end of their paths: Shiki's list
// of grammars, whose name carries a hash of its contents, Shiki's list of
// themes, and the diff library's list of Shiki's themes.
const LISTS = /\/node_modules\/(shiki\/dist\/(langs-bundle-full-[^/]+|themes)\.mjs|@pierre\/theming\/dist\/collections\/shiki\.js)$/

// An import of one of them, as they write it: `import("@shikijs/langs/typescript")`.
const IMPORT = /\bimport\(\s*(["'])@shikijs\/(langs|themes)\/([^"']*)\1\s*\)/g

// A name that may be read from disk: Shiki's are all lowercase letters,
// digits and dashes. Nothing else is, such as `..` or a `/`.
const PLAIN = /^[a-z0-9-]+$/

const SCRIPT = "text/javascript;charset=utf-8"
const FOR_GOOD = "public, max-age=31536000, immutable"

// Where Shiki's files are, found once, when first asked: the Shiki the diff
// library imports, and the two folders it takes its grammars and themes
// from, `@shikijs/langs/dist` and `@shikijs/themes/dist`.
let found: { address: string; folders: Record<Kind, string> } | undefined
function shiki() {
  if (found) return found
  const library = dirname(Bun.resolveSync("@pierre/diffs", import.meta.dir))
  const folder = dirname(Bun.resolveSync("shiki/package.json", library))
  const { version } = require(join(folder, "package.json")) as { version: string }
  const distOf = (kind: Kind) => dirname(Bun.resolveSync(`@shikijs/${kind}`, folder))
  found = { address: `/shiki/${version}/`, folders: { langs: distOf("langs"), themes: distOf("themes") } }
  return found
}

// The address Shiki's files are served under, such as `/shiki/4.6.0/`.
export const shikiAddress = () => shiki().address

// The plugin for the web app's build that leaves Shiki's grammars and
// themes out, and imports each from its address instead, which it marks as
// outside the build. The build fails, saying why, when what it finds is not
// what it expects, as after an upgrade of Shiki that moves its files, rather
// than leave the diffs in one colour, or every grammar built in again:
// - a list of them with no import to rewrite;
// - an import, in a list, of a file other than the one the server sends at
//   its address, as of another copy of Shiki's grammars;
// - a grammar or theme built into the app all the same, as by a module that
//   lists them that the build does not know.
export const shikiFiles: BunPlugin = {
  name: "shiki-files",
  setup(build) {
    const { address, folders } = shiki()
    const problems: string[] = []
    build.onResolve({ filter: /^\/shiki\// }, (args) => ({ path: args.path, external: true }))
    build.onLoad({ filter: LISTS }, async (args) => {
      let imports = 0
      // Each import is rewritten, so that the build goes on to say every
      // problem it finds, rather than stop at the first it cannot resolve.
      const contents = (await Bun.file(args.path).text()).replace(IMPORT, (_whole, _quote, kind: Kind, name: string) => {
        imports += 1
        const file = join(folders[kind], `${name}.mjs`)
        if (!PLAIN.test(name) || resolved(`@shikijs/${kind}/${name}`, dirname(args.path)) !== file) {
          problems.push(`${args.path} imports "@shikijs/${kind}/${name}", which is not ${file}, the file the server sends at ${address}${kind}/${name}.mjs.`)
        }
        return `import(${JSON.stringify(`${address}${kind}/${name}.mjs`)})`
      })
      if (imports === 0) problems.push(`${args.path} lists none of Shiki's grammars or themes in the way the build rewrites, as \`import("@shikijs/langs/<name>")\`.`)
      return { contents, loader: "js" }
    })
    build.onEnd((result) => {
      if (!result.success) return
      // The metafile names each file built in by its path from the folder
      // the build runs in.
      for (const input of Object.keys(result.metafile?.inputs ?? {})) {
        const path = resolve(input)
        if (Object.values(folders).some((folder) => path.startsWith(folder + sep))) problems.push(`${path}, one of Shiki's grammars or themes, was built into the web app: a module the build does not rewrite imports it.`)
      }
      if (!result.metafile) problems.push("The web app's build needs its metafile, to check that none of Shiki's grammars or themes is built in.")
      if (problems.length > 0) throw new Error(`Shiki's grammars and themes could not be left out of the web app:\n${problems.join("\n")}`)
    })
  },
}

// Where a module imports a name from, if it can be found.
function resolved(name: string, from: string): string | undefined {
  try {
    return Bun.resolveSync(name, from)
  } catch {
    return undefined
  }
}

// The route that sends Shiki's files, at `/shiki/<version>/langs/<name>.mjs`
// and `/shiki/<version>/themes/<name>.mjs`. Only a plain name is read, and
// only from Shiki's two folders; any other answers 404, and a diff whose
// grammar cannot be loaded is drawn in plain text. Each file is read and
// compressed the first time it is asked for, and kept, so that it is never
// compressed twice. The files are compressed one at a time, so that a page
// asking for several at once makes the server's memory grow no more than
// for one: the compressor's working memory stays with the server, which
// runs all the time. Every file is sent as `fileRoute` sends a built file,
// in the encoding the browser takes, and kept by the browser for good,
// since its address changes with Shiki's version.
export function shikiRoute(): (request: Request) => Promise<Response> {
  const { address, folders } = shiki()
  const kept = new Map<string, Promise<((request: Request) => Response) | undefined>>()
  let compressing: Promise<unknown> = Promise.resolve()
  const read = async (path: string) => {
    const file = Bun.file(path)
    if (!(await file.exists())) return undefined
    return fileRoute(await variantsOf(await file.bytes(), SCRIPT), FOR_GOOD)
  }
  return async (request) => {
    const asked = new URL(request.url).pathname
    const [, kind, name] = (asked.startsWith(address) && asked.slice(address.length).match(FILE)) || []
    if (kind === undefined || name === undefined) return notThere()
    const path = join(folders[kind as Kind], `${name}.mjs`)
    let route = kept.get(path)
    if (route === undefined) {
      route = compressing.then(() => read(path))
      compressing = route.catch(() => undefined)
      kept.set(path, route)
    }
    const sent = await route.catch((cause) => {
      kept.delete(path)
      throw cause
    })
    if (sent) return sent(request)
    // A name with no file is not kept, so that names asked for that are not
    // there do not add up.
    kept.delete(path)
    return notThere()
  }
}

// A file asked for, after the address: `langs/<name>.mjs` or `themes/<name>.mjs`, with a plain name.
const FILE = /^(langs|themes)\/([a-z0-9-]+)\.mjs$/

const notThere = () => new Response("Shiki has no grammar or theme by that name.", { status: 404 })
