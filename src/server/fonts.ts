import type { BunPlugin } from "bun"

// The web app's fonts: Geist and Geist Mono, from the @fontsource-variable
// packages (SIL Open Font License 1.1, in LICENSES/), one variable file for
// each font and alphabet, which carries every weight. The stylesheet,
// `web/src/fonts.css`, names each by the address the server serves it at:
// its name in its package and the package's version, since a browser keeps
// it for good, so that a font from a newer package has an address of its
// own.
//
// They are served as files, rather than put into the stylesheet as Bun's
// bundler does by itself, because the page draws nothing until it has the
// stylesheet: inside it, all four fonts, as base64, made it three times as
// large, though the extended Latin files hold only letters that English
// text does not use. As files, each is fetched only when a letter of its
// own is on the page. The Latin files are preloaded: the page asks for them
// as it starts, alongside its script, and not only once the stylesheet is
// read and text laid out, so that they have come by the time text is first
// drawn, and it is not drawn in a fallback font first.
export type Font = { address: string; file: string; preload: boolean }

export const FONTS: readonly Font[] = [
  font("@fontsource-variable/geist", "geist-latin-wght-normal", true),
  font("@fontsource-variable/geist", "geist-latin-ext-wght-normal", false),
  font("@fontsource-variable/geist-mono", "geist-mono-latin-wght-normal", true),
  font("@fontsource-variable/geist-mono", "geist-mono-latin-ext-wght-normal", false),
]

// A package's font, by its name in the package's `files` folder.
function font(from: string, name: string, preload: boolean): Font {
  const { version } = require(`${from}/package.json`) as { version: string }
  return { address: `/fonts/${name}-${version}.woff2`, file: require.resolve(`${from}/files/${name}.woff2`), preload }
}

// The bundler's plugin for the fonts' addresses in the stylesheet, which it
// would otherwise look for as files, find none, and fail. With `external`,
// as the server builds the web app, each is left as it is, for the server to
// serve. Without, as Bun bundles the page on each request for `evie serve
// --dev`, which takes this plugin from bunfig.toml, each is taken to be the
// font's file, which the bundler then puts into the stylesheet. An address
// under /fonts/ that is no font's, such as one with a version that is not
// the package's, still fails the build, which names the file and the line.
export function fontsPlugin(external: boolean): BunPlugin {
  return {
    name: "evie-fonts",
    setup(build) {
      build.onResolve({ filter: /^\/fonts\// }, ({ path }) => {
        const font = FONTS.find((font) => font.address === path)
        if (font === undefined) return undefined
        return external ? { path, external: true } : { path: font.file }
      })
    },
  }
}

// For bunfig.toml, which names this file.
export default fontsPlugin(false)
