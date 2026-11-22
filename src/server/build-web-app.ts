import type { BuildMetafile } from "bun"
import tailwind from "bun-plugin-tailwind"
import { fontsPlugin } from "./fonts.ts"
import { ENCODINGS, variantsOf, type Encoding, type Variants } from "./variants.ts"

// The web app's build, which `buildWebApp` runs in a process of its own:
//
//   bun build-web-app.ts <page> <file for the list> <file for the files>
//
// Bun's bundler keeps its working memory, about 300 MB for this app, for as
// long as the process that ran it lives, and the server runs all the time.
// Built here, the memory goes when this process exits. So does the
// compressor's, which makes each file's compressed variants here too.
//
// It writes every file built into one file, one after another, each as its
// variants in the order of `ENCODINGS`, which is quicker to write and to
// read back than each on its own, and a list of them, as JSON: each file's
// path, type, the hash of its contents, and the size of each variant it
// has, in that order, and Bun's metafile. If the build fails, it says why
// on stderr, one line for each of the bundler's messages, `file:line:
// message`, and exits with code 1.
export type Built = { files: { path: string; type: string; tag: string; sizes: Partial<Record<Encoding, number>> }[]; metafile: BuildMetafile }

const [page, list, files] = process.argv.slice(2)
if (page === undefined || list === undefined || files === undefined) {
  console.error("Usage: bun build-web-app.ts <page> <file for the list> <file for the files>")
  process.exit(2)
}

try {
  const built = await Bun.build({
    entrypoints: [page],
    // The plugins are handed to the bundler here, in code, so the build does
    // not depend on a bunfig.toml in the folder it is started in. The fonts
    // the stylesheet names are left out of it, for the server to serve as
    // files: see `FONTS`.
    plugins: [fontsPlugin(true), tailwind],
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
  // Every file built, the diff library's worker among them, though the
  // metafile does not list it.
  const outputs = await Promise.all(built.outputs.map(async (output) => ({ path: output.path, file: await variantsOf(new Uint8Array(await output.arrayBuffer()), output.type) })))
  await Bun.write(files, new Blob(outputs.flatMap(({ file }) => variantsIn(file).map(([, variant]) => variant))))
  const listed: Built = {
    files: outputs.map(({ path, file }) => ({ path, type: file.type, tag: file.tag, sizes: Object.fromEntries(variantsIn(file).map(([encoding, variant]) => [encoding, variant.length])) })),
    metafile: built.metafile!,
  }
  await Bun.write(list, JSON.stringify(listed))
} catch (cause) {
  console.error(described(cause))
  process.exit(1)
}

// The variants a file has, each with its encoding, in the order they are written.
function variantsIn(file: Variants): [Encoding, Uint8Array][] {
  return ENCODINGS.flatMap((encoding) => {
    const variant = file[encoding]
    return variant ? [[encoding, variant]] : []
  })
}

// Why the build failed. The bundler fails with "Bundle failed", and what
// went wrong where in the errors it carries.
function described(cause: unknown): string {
  const errors = cause instanceof AggregateError ? (cause.errors as Partial<BuildMessage>[]) : []
  const where = (error: Partial<BuildMessage>) => (error.position ? `${error.position.file}:${error.position.line}: ` : "")
  if (errors.length > 0) return errors.map((error) => `${where(error)}${error.message}`).join("\n")
  return cause instanceof Error ? cause.message : String(cause)
}
