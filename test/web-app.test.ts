import { expect, test } from "bun:test"
import tailwind from "bun-plugin-tailwind"
import { chmod, mkdir, mkdtemp, readdir, realpath, rm, utimes, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { brotliDecompressSync, gunzipSync } from "node:zlib"
import { builtWebApp } from "../src/programs/serve.ts"
import { buildWebApp } from "../src/server/web-app.ts"

// The web app as `evie serve` builds it when it starts: by Bun's bundler, in
// a process of its own, with Tailwind's plugin.

// A new empty folder, removed once the test is done with it.
async function inFolder<T>(prefix: string, use: (folder: string) => Promise<T>): Promise<T> {
  const folder = await mkdtemp(join(tmpdir(), prefix))
  try {
    return await use(folder)
  } finally {
    await rm(folder, { recursive: true, force: true })
  }
}

// A file built, as text.
const text = (bytes: Uint8Array) => new TextDecoder().decode(bytes)

test("the build hands over its metafile, which names the page's script by its path in the repository, and each chunk it imports is a file built beside it", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")

  const script = [...text(app.page.identity).matchAll(/<script[^>]* src="([^"]+)"/g)].map((match) => match[1]!)
  const entries = Object.entries(app.metafile.outputs).filter(([path, output]) => output.entryPoint === "web/index.html" && path.endsWith(".js"))
  expect(entries.map(([path]) => path.replace(/^\./, ""))).toEqual(script)
  const imports = entries[0]![1].imports.filter((each) => each.kind === "import-statement")
  expect(imports.length).toBeGreaterThan(0)
  for (const each of imports) expect(Object.keys(app.files)).toContain(each.path.replace(/^\./, ""))
})

test("every file the web app builds over 1 KB, the diff library's worker among them, is kept compressed with brotli and with gzip, each of which decodes to the file", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")

  // The worker is built, though the metafile does not list it.
  const worker = Object.keys(app.files).find((path) => /^\/worker-portable-[^/]+\.js$/.test(path))
  expect(worker).toBeDefined()
  expect(Object.keys(app.metafile.outputs).map((path) => path.replace(/^\./, ""))).not.toContain(worker)
  const files = [app.page, ...Object.values(app.files)]
  for (const file of files) {
    const plain = file.identity
    if (plain.length <= 1024) {
      expect([file.br, file.gzip]).toEqual([undefined, undefined])
      continue
    }
    expect(brotliDecompressSync(file.br!)).toEqual(Buffer.from(plain))
    expect(gunzipSync(file.gzip!)).toEqual(Buffer.from(plain))
  }
  expect(app.files[worker!]!.br!.length).toBeLessThan(app.files[worker!]!.identity.length / 2)
})

test("a server started in another folder, with another bun first on its path, builds the web app with its own Bun, and leaves nothing in the temporary folder", async () => {
  await inFolder("evie-elsewhere-", async (elsewhere) => {
    // A bun that only fails, first on the path of a Bun started in this
    // folder, which builds the web app as the server does.
    await mkdir(join(elsewhere, "bin"))
    await writeFile(join(elsewhere, "bin", "bun"), "#!/bin/sh\necho 'not the Bun that runs the server' >&2\nexit 1\n")
    await chmod(join(elsewhere, "bin", "bun"), 0o755)
    const temporary = join(elsewhere, "temporary")
    await mkdir(temporary)
    await writeFile(
      join(elsewhere, "build.ts"),
      `const { buildWebApp } = await import(${JSON.stringify(join(import.meta.dir, "..", "src", "server", "web-app.ts"))})\n` +
        `const app = await buildWebApp({ temporary: ${JSON.stringify(temporary)} })\n` +
        "const pages = Object.values(app.metafile.outputs).map((output) => output.entryPoint).filter((path) => path?.endsWith('index.html'))\n" +
        "console.log(JSON.stringify({ page: new TextDecoder().decode(app.page.identity), pages: [...new Set(pages)] }))\n",
    )

    const server = Bun.spawn([process.execPath, "build.ts"], { cwd: elsewhere, env: { ...process.env, PATH: `${join(elsewhere, "bin")}:${process.env.PATH}` }, stdout: "pipe", stderr: "pipe" })
    const [said, problem, code] = await Promise.all([new Response(server.stdout).text(), new Response(server.stderr).text(), server.exited])

    expect({ code, problem }).toEqual({ code: 0, problem: "" })
    const built = JSON.parse(said) as { page: string; pages: string[] }
    expect(built.page).toContain('<script type="module"')
    // Named from the repository, where the build was started.
    expect(built.pages).toEqual(["web/index.html"])
    expect(await readdir(temporary)).toEqual([])
  })
})

test("a build that fails names the file and the line, evie serve goes on without the page, and nothing is left in the temporary folder", async () => {
  await inFolder("evie-broken-page-", async (folder) => {
    await writeFile(join(folder, "index.html"), '<!doctype html><html><body><script type="module" src="./main.tsx"></script></body></html>\n')
    await writeFile(join(folder, "main.tsx"), "const shown = (1 +\nconsole.log(shown)\n")
    const temporary = join(folder, "temporary")
    await mkdir(temporary)

    const built = await builtWebApp(async () => ({ buildWebApp: () => buildWebApp({ page: join(folder, "index.html"), temporary }) }))

    // The bundler names the file by its real path: on macOS the temporary
    // folder is reached through a link.
    const file = join(await realpath(folder), "main.tsx")
    expect(built).toEqual({ problem: `The web app could not be built, so it is not served:\n${file}:2: Expected ")" but found end of file` })
    expect(await readdir(temporary)).toEqual([])
  })
})

test("a temporary folder left by a server stopped while it built is removed by the next build, and one still being built into is not", async () => {
  await inFolder("evie-left-build-", async (folder) => {
    await writeFile(join(folder, "index.html"), '<!doctype html><html><body><script type="module" src="./main.ts"></script></body></html>\n')
    await writeFile(join(folder, "main.ts"), 'console.log("evie")\n')
    const temporary = join(folder, "temporary")
    // Left two hours ago, as by Ctrl-C while the server started; another
    // server's, building now; and a folder that is not a build's.
    const twoHoursAgo = new Date(Date.now() - 2 * 60 * 60 * 1000)
    for (const name of ["evie-web-app-left", "evie-web-app-building", "something-else"]) await mkdir(join(temporary, name, "files"), { recursive: true })
    for (const name of ["evie-web-app-left", "something-else"]) await utimes(join(temporary, name), twoHoursAgo, twoHoursAgo)

    const app = await buildWebApp({ page: join(folder, "index.html"), temporary })

    expect("files" in app).toBe(true)
    expect((await readdir(temporary)).sort()).toEqual(["evie-web-app-building", "something-else"])
  })
})

test("the web app builds into a page whose script and stylesheet are built beside it, and that asks no other site for anything", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")

  const page = text(app.page.identity)
  // The empty icon, `data:,`, is carried in the page and loads nothing.
  const loads = [...page.matchAll(/(?:src|href)="(?!data:,")([^"]+)"/g)].map((match) => match[1]!)
  expect(loads.map((path) => path.split(".").pop()).sort()).toEqual(["css", "js"])
  for (const path of loads) expect(Object.keys(app.files)).toContain(path)

  const style = text(app.files[loads.find((path) => path.endsWith(".css"))!]!.identity)
  const script = text(app.files[loads.find((path) => path.endsWith(".js"))!]!.identity)
  // The fonts are evie's own, carried in the stylesheet, not fetched from Google.
  expect(style).toMatch(/@font-face\{font-family:Geist;[^}]*url\(data:font\/woff2/)
  expect(style).toMatch(/@font-face\{font-family:Geist Mono;[^}]*url\(data:font\/woff2/)
  for (const text of [page, style]) expect(text).not.toMatch(/(?:url\(|src="|href=")https?:/)
  // React's production build, minified: its development build warns about this.
  expect(script).not.toContain("Download the React DevTools")
})

test("the stylesheet holds the design system's colours for each of its three themes, and the screens' classes name them", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = text(Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.identity)

  // Dusk is the default, and each theme sets the same names.
  expect(style).toMatch(/:root,\[data-theme=dusk\]\{[^}]*--bg:#131312;--bg-side:#0e0e0d;/)
  expect(style).toMatch(/\[data-theme=moss\]\{[^}]*--bg:#111310;--bg-side:#0d0f0c;/)
  expect(style).toMatch(/\[data-theme=oxblood\]\{[^}]*--bg:#141011;[^}]*--status-needs:var\(--accent\)/)
  // A class names a token, and takes its value from the theme it is in.
  expect(style).toContain(".bg-bg-side{background-color:var(--bg-side)}")
  expect(style).toContain(".text-text-muted{color:var(--text-muted)}")
})

test("evie serve goes on without the web app when its module cannot be loaded, as when a package is missing, and says why", async () => {
  const built = await builtWebApp(() => Promise.reject(new Error("Cannot find package 'bun-plugin-tailwind'")))

  expect(built).toEqual({ problem: "The web app could not be built, so it is not served:\nCannot find package 'bun-plugin-tailwind'" })
})

test("the spacing scale has only the design system's steps and named sizes, so a class off the scale is not made", async () => {
  // A stylesheet beside the app's, asking Tailwind for classes as if a
  // screen used them.
  const folder = await mkdtemp(join(tmpdir(), "evie-classes-"))
  try {
    const sheet = join(folder, "classes.css")
    const styles = join(import.meta.dir, "..", "web", "src", "styles.css")
    await writeFile(sheet, `@import "${styles}";\n@source inline("p-1 p-5 p-7 p-8 w-session-list p-9 h-10 m-12");\n`)
    const built = await Bun.build({ entrypoints: [sheet], plugins: [tailwind] })
    const style = await built.outputs[0]!.text()

    for (const made of ["p-1", "p-5", "p-7", "p-8", "w-session-list"]) expect(style).toContain(`.${made} {`)
    for (const offScale of ["p-9", "h-10", "m-12"]) expect(style).not.toContain(`.${offScale} {`)
  } finally {
    await rm(folder, { recursive: true, force: true })
  }
})

test("below 640px the controls grow a step, as T3 Code's, and what is typed into a text box is 16px, so that iOS does not zoom the page in when one is tapped", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = text(Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.identity)

  for (const desktop of ["--spacing-control:32px", "--spacing-control-sm:28px", "--spacing-icon-control:16px", "--text-control:14px", "--text-field:14px", "--spacing-gutter:20px"]) {
    expect(style).toContain(`${desktop};`)
  }
  const phone = style.match(/@media \(max-width:639\.9+px\)\{:root\{([^}]*)\}\}/)?.[1]?.split(";")
  expect(phone).toEqual(expect.arrayContaining(["--spacing-control:36px", "--spacing-control-sm:32px", "--spacing-icon-control:18px", "--text-control:16px", "--text-field:16px", "--spacing-gutter:12px"]))
  // The text, the rows and the headers keep their sizes.
  expect(phone?.some((each) => /--(text-(body|caption|label)|spacing-(row-height|work-row|header-height))/.test(each))).toBe(false)
  expect(style).toContain(".text-field{font-size:var(--text-field);")
})

test("the page puts itself in the theme this browser picked before it is first drawn", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const page = text(app.page.identity)

  const themed = page.indexOf('localStorage.getItem("evie.theme")')
  expect(themed).toBeGreaterThan(-1)
  expect(themed).toBeLessThan(page.indexOf('rel="stylesheet"'))
})

test("the page names an empty icon, so that the browser does not ask for /favicon.ico and get a 404 on every load", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const page = text(app.page.identity)

  const icon = page.match(/<link\b[^>]*\brel="?icon"?[^>]*>/)?.[0]
  expect(icon).toMatch(/\bhref="?data:,"?[\s/>]/)
})

test("a transition has a length and an easing, which Tailwind's cleared defaults left out, and none for someone who asks for less motion", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = text(Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.identity)

  // Minified, 150ms is written .15s.
  expect(style).toMatch(/--default-transition-duration:(150ms|\.15s)/)
  expect(style).toMatch(/--default-transition-timing-function:cubic-bezier\(\.4,0,\.2,1\)/)
  expect(style).toMatch(/\.transition-transform\{[^}]*transition-duration:var\(--tw-duration,var\(--default-transition-duration\)\)/)
  expect(style).toMatch(/@media \(prefers-reduced-motion:reduce\)\{[^@]*\.motion-reduce\\:transition-none\{transition-property:none\}/)
})

test("the focus ring and the spinner's ring are drawn at the design system's indicator width, and lines at its line width", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = text(Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.identity)

  expect(style).toMatch(/--indicator-width:2px/)
  expect(style).toMatch(/--line-width:1px/)
  expect(style).toMatch(/:focus-visible\{outline:var\(--indicator-width\) ?solid var\(--focus-ring\)/)
  expect(style).toContain(".border-\\(length\\:--indicator-width\\){border-style:var(--tw-border-style);border-width:var(--indicator-width)}")
  expect(style).not.toMatch(/\.border-2\{/)
})

test("the screens opened rarely or later, and the drawings of every icon but the folder, are not in the script the page loads first, but in files of their own", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const page = await app.page.text()

  // The page's script and every file it imports before it runs: not those
  // it imports with import(), which load when asked for.
  const first = new Set<string>()
  const follow = async (path: string) => {
    if (first.has(path)) return
    first.add(path)
    const script = await app.files[path]!.text()
    for (const match of script.matchAll(/\bimport\s*([^("'`;]*?)\s*["'](\/[^"']+\.js)["']/g)) await follow(match[2]!)
  }
  await follow(page.match(/<script type="module"[^>]*src="([^"]+)"/)![1]!)
  const firstScript = (await Promise.all([...first].map((path) => app.files[path]!.text()))).join("\n")
  const others = Object.entries(app.files).filter(([path]) => path.endsWith(".js") && !first.has(path))
  const laterScripts = (await Promise.all(others.map(([, file]) => file.text()))).join("\n")

  // Words of the settings, the command palette, the session details, the
  // right panel, the workspace view, its side chat and storage, the icon
  // picker, and an icon's name.
  for (const words of ["Restart to apply", "Command palette", "Choose what to show", "The diff view could not be loaded", "This workspace is not in the list", "Ask evie to change something here", "Backed up privately", "No matching icon", "cooking-pot"]) {
    expect({ words, first: firstScript.includes(words), later: laterScripts.includes(words) }).toEqual({ words, first: false, later: true })
  }
})
