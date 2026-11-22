import type { HTMLBundle } from "bun"
import tailwind from "bun-plugin-tailwind"
import { join } from "node:path"
import page from "../../web/index.html"

// The web app, for the server to serve: its page, and the files the page
// loads, each by the path it is served at. The page is either built already,
// or in development Bun's own import of it, which Bun bundles again on each
// request and updates in the open browser as its files change.
export type WebApp = { page: Blob; files: Record<string, Blob> } | { page: HTMLBundle }

// The page Bun's bundler starts from: it follows the page's script and
// stylesheet, and everything they import.
const PAGE = join(import.meta.dir, "..", "..", "web", "index.html")

// Builds the web app in memory, minified, as `evie serve` does when it
// starts, so there is no build step and nothing written to disk. Tailwind's
// plugin is handed to the bundler here, in code, so the build is the same in
// whatever folder the server is started.
export async function buildWebApp(): Promise<WebApp> {
  const built = await Bun.build({
    entrypoints: [PAGE],
    plugins: [tailwind],
    minify: true,
    // The page is served at more than one path, such as /sign-in, so it
    // names its files from the root.
    publicPath: "/",
    // React's production build, without its checks and warnings for developers.
    define: { "process.env.NODE_ENV": JSON.stringify("production") },
  })
  const files: Record<string, Blob> = {}
  for (const output of built.outputs) files[output.path.replace(/^\./, "")] = new Blob([await output.arrayBuffer()], { type: output.type })
  const { "/index.html": builtPage, ...loaded } = files
  if (!builtPage) throw new Error("Bun's bundler built the web app without its page.")
  return { page: builtPage, files: loaded }
}

// The web app for working on it: Bun bundles the page on each request and
// keeps the open page up to date as its files are saved, keeping the state
// of the components on screen. Bun takes the Tailwind plugin for this from
// `bunfig.toml`, which it reads from the folder it was started in, so the
// server must be started in the repository's folder.
export const webAppForDevelopment = (): WebApp => ({ page })
