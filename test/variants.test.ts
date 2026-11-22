import { expect, test } from "bun:test"
import { brotliDecompressSync, gunzipSync } from "node:zlib"
import { fileRoute, variantsOf, type Variants } from "../src/server/variants.ts"

// A file the server sends as it is and compressed, and the route that sends
// each request the variant its Accept-Encoding allows.

// A script of a few KB, as the bundler would write one.
const script = new TextEncoder().encode(Array.from({ length: 200 }, (_, i) => `export const value${i} = ${i} * 2;\n`).join(""))
const SCRIPT = "text/javascript;charset=utf-8"
const IMMUTABLE = "public, max-age=31536000, immutable"

// Asks the route for its file, with the request headers given.
const ask = (file: Variants, headers: Record<string, string> = {}, cache = IMMUTABLE) => fileRoute(file, cache)(new Request("http://evie.test/file.js", { headers }))

test("a file of text over 1 KB is kept compressed with brotli and with gzip, each of which decodes to the file", async () => {
  const file = await variantsOf(script, SCRIPT)

  expect(file.type).toBe(SCRIPT)
  expect(file.identity).toEqual(script)
  expect(brotliDecompressSync(file.br!)).toEqual(Buffer.from(script))
  expect(gunzipSync(file.gzip!)).toEqual(Buffer.from(script))
  expect(file.br!.length).toBeLessThan(script.length)
  expect(file.gzip!.length).toBeLessThan(script.length)
})

test("a font, an image and a file of 1 KB or less are kept only as they are: fonts and images are compressed already, and a small file gains too little", async () => {
  for (const [bytes, type] of [
    [script, "font/woff2"],
    [script, "image/png"],
    [script.subarray(0, 1024), SCRIPT],
  ] as const) {
    const file = await variantsOf(bytes, type)
    expect([file.br, file.gzip]).toEqual([undefined, undefined])
    expect(file.identity).toEqual(bytes)
  }
})

test("a request that accepts br is sent brotli, one that accepts only gzip is sent gzip, and one that accepts neither the file as it is, each saying it varies by Accept-Encoding", async () => {
  const file = await variantsOf(script, SCRIPT)

  const asked = async (acceptEncoding: string | undefined) => {
    const answer = await ask(file, acceptEncoding === undefined ? {} : { "Accept-Encoding": acceptEncoding })
    const body = new Uint8Array(await answer.arrayBuffer())
    return {
      encoding: answer.headers.get("content-encoding"),
      vary: answer.headers.get("vary"),
      type: answer.headers.get("content-type"),
      cache: answer.headers.get("cache-control"),
      body: answer.headers.get("content-encoding") === "br" ? brotliDecompressSync(body) : answer.headers.get("content-encoding") === "gzip" ? gunzipSync(body) : Buffer.from(body),
    }
  }
  const same = { vary: "Accept-Encoding", type: SCRIPT, cache: IMMUTABLE, body: Buffer.from(script) }

  // As Chrome asks, and in another order.
  expect(await asked("gzip, deflate, br, zstd")).toEqual({ encoding: "br", ...same })
  expect(await asked("br;q=0.5, gzip;q=1")).toEqual({ encoding: "br", ...same })
  expect(await asked("gzip, deflate")).toEqual({ encoding: "gzip", ...same })
  // Brotli refused by a weight of 0, written with spaces or without.
  expect(await asked("br;q=0, gzip")).toEqual({ encoding: "gzip", ...same })
  expect(await asked("br; q = 0, gzip")).toEqual({ encoding: "gzip", ...same })
  // Neither asked for: a `*` is not taken to ask for brotli.
  for (const neither of [undefined, "", "identity", "deflate, zstd", "*", "br;q=0, gzip;q=0.000"]) expect(await asked(neither)).toEqual({ encoding: null, ...same })
})

test("each variant has its own ETag, and a request sending it back is answered 304 with no body, still saying it varies", async () => {
  const file = await variantsOf(script, SCRIPT)
  const tags = { br: (await ask(file, { "Accept-Encoding": "br" })).headers.get("etag"), gzip: (await ask(file, { "Accept-Encoding": "gzip" })).headers.get("etag"), identity: (await ask(file)).headers.get("etag") }

  expect(new Set(Object.values(tags)).size).toBe(3)
  for (const tag of Object.values(tags)) expect(tag).toMatch(/^"[^"]+"$/)
  for (const [encoding, tag] of [
    ["br", tags.br],
    ["gzip", tags.gzip],
    ["identity", tags.identity],
  ] as const) {
    const again = await ask(file, { "Accept-Encoding": encoding, "If-None-Match": tag! })
    expect(again.status).toBe(304)
    expect(await again.text()).toBe("")
    expect({ etag: again.headers.get("etag"), vary: again.headers.get("vary"), cache: again.headers.get("cache-control"), encoding: again.headers.get("content-encoding") }).toEqual({
      etag: tag,
      vary: "Accept-Encoding",
      cache: IMMUTABLE,
      encoding: null,
    })
  }
  // One of a list, and one a proxy marked weak, match too.
  expect((await ask(file, { "Accept-Encoding": "br", "If-None-Match": `"other", ${tags.br}` })).status).toBe(304)
  expect((await ask(file, { "Accept-Encoding": "br", "If-None-Match": `W/${tags.br}` })).status).toBe(304)
  // The tag of another variant, or of another file, gets the file.
  const other = await ask(file, { "Accept-Encoding": "br", "If-None-Match": tags.gzip! })
  expect([other.status, other.headers.get("content-encoding"), other.headers.get("etag")]).toEqual([200, "br", tags.br])
  expect((await ask(file, { "Accept-Encoding": "br", "If-None-Match": '"not-this-file"' })).status).toBe(200)
  // A 304 is only for reading: a HEAD gets one, a POST does not.
  const asking = (method: string) => fileRoute(file, IMMUTABLE)(new Request("http://evie.test/file.js", { method, headers: { "Accept-Encoding": "br", "If-None-Match": tags.br! } }))
  expect([asking("HEAD").status, asking("POST").status]).toEqual([304, 200])
})

test("a file kept only as it is says nothing of Accept-Encoding, and still has an ETag that answers 304", async () => {
  const font = await variantsOf(script, "font/woff2")

  const answer = await ask(font, { "Accept-Encoding": "gzip, deflate, br, zstd" })
  expect([answer.headers.get("content-encoding"), answer.headers.get("vary"), answer.headers.get("content-type")]).toEqual([null, null, "font/woff2"])
  expect(new Uint8Array(await answer.arrayBuffer())).toEqual(script)
  const again = await ask(font, { "Accept-Encoding": "br", "If-None-Match": answer.headers.get("etag")! }, "no-cache")
  expect([again.status, again.headers.get("cache-control"), await again.text()]).toEqual([304, "no-cache", ""])
})

test("a file's ETag changes with its contents", async () => {
  const one = await ask(await variantsOf(script, SCRIPT))
  const changed = script.slice()
  changed[0] = "E".charCodeAt(0)
  const two = await ask(await variantsOf(changed, SCRIPT))

  expect(one.headers.get("etag")).not.toBe(two.headers.get("etag"))
})
