import type { BuildMetafile } from "bun"
import tailwind from "bun-plugin-tailwind"

// The web app's build, which `buildWebApp` runs in a process of its own:
//
//   bun build-web-app.ts <page> <file for the list> <file for the files>
//
// Bun's bundler keeps its working memory, about 300 MB for this app, for as
// long as the process that ran it lives, and the server runs all the time.
// Built here, the memory goes when this process exits.
//
// It writes every file built into one file, one after another, which is
// quicker to write and to read back than each file on its own, and a list
// of them, as JSON: each file's path, type and size, in that order, and
// Bun's metafile. If the build fails, it says why on stderr, one line for
// each of the bundler's messages, `file:line: message`, and exits with code 1.
export type Built = { files: { path: string; type: string; size: number }[]; metafile: BuildMetafile }

const [page, list, files] = process.argv.slice(2)
if (page === undefined || list === undefined || files === undefined) {
  console.error("Usage: bun build-web-app.ts <page> <file for the list> <file for the files>")
  process.exit(2)
}

try {
  const built = await Bun.build({
    entrypoints: [page],
    // Tailwind's plugin is handed to the bundler here, in code, so the build
    // does not depend on a bunfig.toml in the folder it is started in.
    plugins: [tailwind],
    minify: true,
    // The page is served at more than one path, such as /sign-in, so it
    // names its files from the root.
    publicPath: "/",
    // React's production build, without its checks and warnings for developers.
    define: { "process.env.NODE_ENV": JSON.stringify("production") },
    // What the page imports only when it needs it, such as the diff's
    // library and each language it colours, is built into files of their
    // own, which the page loads then, so that the rest of the page does not
    // wait for them.
    splitting: true,
    metafile: true,
  })
  const outputs = await Promise.all(built.outputs.map(async (output) => ({ path: output.path, type: output.type, bytes: new Uint8Array(await output.arrayBuffer()) })))
  await Bun.write(files, new Blob(outputs.map((output) => output.bytes)))
  const listed: Built = { files: outputs.map(({ path, type, bytes }) => ({ path, type, size: bytes.length })), metafile: built.metafile! }
  await Bun.write(list, JSON.stringify(listed))
} catch (cause) {
  console.error(described(cause))
  process.exit(1)
}

// Why the build failed. The bundler fails with "Bundle failed", and what
// went wrong where in the errors it carries.
function described(cause: unknown): string {
  const errors = cause instanceof AggregateError ? (cause.errors as Partial<BuildMessage>[]) : []
  const where = (error: Partial<BuildMessage>) => (error.position ? `${error.position.file}:${error.position.line}: ` : "")
  if (errors.length > 0) return errors.map((error) => `${where(error)}${error.message}`).join("\n")
  return cause instanceof Error ? cause.message : String(cause)
}
