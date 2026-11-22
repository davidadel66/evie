import { expect, test } from "bun:test"
import tailwind from "bun-plugin-tailwind"
import { mkdtemp, rm, writeFile } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { builtWebApp } from "../src/programs/serve.ts"
import { buildWebApp } from "../src/server/web-app.ts"

// The web app as `evie serve` builds it when it starts: by Bun's bundler, in
// memory, with Tailwind's plugin.

test("the web app builds into a page whose script and stylesheet are built beside it, and that asks no other site for anything", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")

  const page = await app.page.text()
  const loads = [...page.matchAll(/(?:src|href)="([^"]+)"/g)].map((match) => match[1]!)
  expect(loads.map((path) => path.split(".").pop()).sort()).toEqual(["css", "js"])
  for (const path of loads) expect(Object.keys(app.files)).toContain(path)

  const style = await app.files[loads.find((path) => path.endsWith(".css"))!]!.text()
  const script = await app.files[loads.find((path) => path.endsWith(".js"))!]!.text()
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
  const style = await Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.text()

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

test("what is typed into a text box is 14px, as T3 Code's, and 16px at phone width, so that iOS does not zoom the page in when one is tapped", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = await Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.text()

  expect(style).toMatch(/--text-field:14px;/)
  expect(style).toMatch(/@media \(max-width:767\.9+px\)\{:root\{[^}]*--text-field:16px[;}]/)
  expect(style).toContain(".text-field{font-size:var(--text-field);")
})

test("the page puts itself in the theme this browser picked before it is first drawn", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const page = await app.page.text()

  const themed = page.indexOf('localStorage.getItem("evie.theme")')
  expect(themed).toBeGreaterThan(-1)
  expect(themed).toBeLessThan(page.indexOf('rel="stylesheet"'))
})

test("a transition has a length and an easing, which Tailwind's cleared defaults left out, and none for someone who asks for less motion", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = await Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.text()

  // Minified, 150ms is written .15s.
  expect(style).toMatch(/--default-transition-duration:(150ms|\.15s)/)
  expect(style).toMatch(/--default-transition-timing-function:cubic-bezier\(\.4,0,\.2,1\)/)
  expect(style).toMatch(/\.transition-transform\{[^}]*transition-duration:var\(--tw-duration,var\(--default-transition-duration\)\)/)
  expect(style).toMatch(/@media \(prefers-reduced-motion:reduce\)\{[^@]*\.motion-reduce\\:transition-none\{transition-property:none\}/)
})

test("the focus ring and the spinner's ring are drawn at the design system's indicator width, and lines at its line width", async () => {
  const app = await buildWebApp()
  if (!("files" in app)) throw new Error("buildWebApp gave the page to bundle, not the page built")
  const style = await Object.values(app.files).find((file) => file.type.startsWith("text/css"))!.text()

  expect(style).toMatch(/--indicator-width:2px/)
  expect(style).toMatch(/--line-width:1px/)
  expect(style).toMatch(/:focus-visible\{outline:var\(--indicator-width\) ?solid var\(--focus-ring\)/)
  expect(style).toContain(".border-\\(length\\:--indicator-width\\){border-style:var(--tw-border-style);border-width:var(--indicator-width)}")
  expect(style).not.toMatch(/\.border-2\{/)
})
