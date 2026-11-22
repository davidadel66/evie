import { expect, test } from "bun:test"
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
