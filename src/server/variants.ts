import { promisify } from "node:util"
import { brotliCompress, constants, gzip } from "node:zlib"

// A file the server sends, kept both as it is and compressed, so that each
// request can be sent the smallest copy it can read. Each copy is a variant
// of the same contents, named by the encoding HTTP gives it: `identity` is
// the file as it is, `br` compressed with brotli and `gzip` with gzip. Every
// browser evie serves reads brotli; gzip is for a client that does not.
// `tag` is a hash of the contents, from which each variant's ETag is made.
// The variants are never changed once made, and each answer is sent a copy.
export type Variants = { type: string; tag: string; identity: Uint8Array; br?: Uint8Array; gzip?: Uint8Array }
export type Encoding = "identity" | "br" | "gzip"

// The encodings, in the order a file's variants are written out, as the web
// app's build writes them for the server to read back.
export const ENCODINGS: readonly Encoding[] = ["identity", "br", "gzip"]

// The encodings a file may be sent in, in the order they are preferred:
// brotli makes the smaller file.
const COMPRESSED = ["br", "gzip"] as const

// What is worth compressing: text, such as the page, scripts, stylesheets,
// JSON and SVG. Fonts and images are compressed already, and a second pass
// would only cost time. A file of 1 KB or less gains too little.
const TEXT = /^(text\/|application\/(javascript|json)|image\/svg\+xml)/
const SMALL = 1024

// Brotli's quality 5 of 11, which a start can afford: it compresses all of
// the web app's files in about 0.1 s, one after another, where 11 takes 9 s,
// for files about 8% larger. Gzip's level 6 is its usual default.
const BROTLI = { params: { [constants.BROTLI_PARAM_QUALITY]: 5 } }
const GZIP = { level: 6 }
const brotli = promisify(brotliCompress)
const gzipped = promisify(gzip)

// A file's variants: it as it is, and compressed with brotli and with gzip
// if it is text of more than 1 KB, each kept only when it is smaller. The
// compressing runs off the main thread, so many files are compressed at
// once. The process keeps much of the compressor's working memory for good:
// about 400 MB, measured, for all of the web app's files at once. So the web
// app's build makes their variants in its own process, which then exits.
export async function variantsOf(bytes: Uint8Array, type: string): Promise<Variants> {
  const file: Variants = { type, tag: Bun.hash(bytes).toString(16).padStart(16, "0"), identity: bytes }
  if (!TEXT.test(type) || bytes.length <= SMALL) return file
  const [br, gz] = await Promise.all([brotli(bytes, BROTLI), gzipped(bytes, GZIP)])
  if (br.length < bytes.length) file.br = br
  if (gz.length < bytes.length) file.gzip = gz
  return file
}

// The route that sends a file: each request is sent the variant its
// Accept-Encoding allows, saying that the answer varies by that header, so
// that a cache never hands a brotli file to a client that cannot read it.
// Each variant has an ETag of its own. A browser that has a variant already
// sends its ETag back in If-None-Match, and is answered 304, with no body:
// Bun's static routes do this by themselves, but cannot choose a variant.
// `cache` is the file's Cache-Control.
export function fileRoute(file: Variants, cache: string): (request: Request) => Response {
  const varies = COMPRESSED.some((encoding) => file[encoding] !== undefined)
  return (request) => {
    const encoding = chosen(file, request.headers.get("accept-encoding"))
    const etag = `"${file.tag}${encoding === "identity" ? "" : `-${encoding}`}"`
    const headers: Record<string, string> = { "Cache-Control": cache, ETag: etag, ...(varies && { Vary: "Accept-Encoding" }) }
    if (has(request.headers.get("if-none-match"), etag)) return new Response(null, { status: 304, headers })
    return new Response(file[encoding], { headers: { ...headers, "Content-Type": file.type, ...(encoding !== "identity" && { "Content-Encoding": encoding }) } })
  }
}

// The variant to send: brotli, else gzip, if the file has it and the
// request's Accept-Encoding names it without refusing it with a weight of
// 0, as `br;q=0` does; else the file as it is. A `*` is not taken to ask
// for brotli: only a request that names it is sent it.
function chosen(file: Variants, acceptEncoding: string | null): Encoding {
  const accepted = new Set<string>()
  for (const each of (acceptEncoding ?? "").toLowerCase().split(",")) {
    const [name, ...parameters] = each.split(";").map((part) => part.trim())
    const weight = parameters.find((parameter) => parameter.startsWith("q="))
    if (name && (weight === undefined || Number(weight.slice(2)) > 0)) accepted.add(name)
  }
  return COMPRESSED.find((encoding) => file[encoding] !== undefined && accepted.has(encoding)) ?? "identity"
}

// Whether the browser has the variant whose ETag is given: If-None-Match
// lists the ETags it has, or says `*` for any. They are compared as HTTP
// asks for this header, so one a proxy marked weak, as W/"...", still counts.
function has(ifNoneMatch: string | null, etag: string): boolean {
  if (ifNoneMatch === null) return false
  return ifNoneMatch.split(",").some((each) => {
    const tag = each.trim()
    return tag === "*" || tag.replace(/^W\//, "") === etag
  })
}
