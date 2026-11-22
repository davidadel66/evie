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
