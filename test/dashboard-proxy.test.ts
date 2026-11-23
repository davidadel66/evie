import { afterEach, describe, expect, test } from "bun:test"
import { createDashboardProxy } from "../src/server/dashboard-proxy.ts"

const cleanups: (() => void)[] = []
afterEach(() => {
  for (const cleanup of cleanups.splice(0).reverse()) cleanup()
})

type SeenRequest = {
  method: string
  pathname: string
  search: string
  headers: Record<string, string>
  body: string
}

function startUpstream(handler?: (req: Request, seen: SeenRequest) => Response | Promise<Response>) {
  const seen: SeenRequest[] = []
  const server = Bun.serve({
    hostname: "127.0.0.1",
    port: 0,
    async fetch(req) {
      const url = new URL(req.url)
      const headers: Record<string, string> = {}
      req.headers.forEach((v, k) => {
        headers[k.toLowerCase()] = v
      })
      const body = req.method === "GET" || req.method === "HEAD" ? "" : await req.text()
      const entry: SeenRequest = {
        method: req.method,
        pathname: url.pathname,
        search: url.search,
        headers,
        body,
      }
      seen.push(entry)
      if (handler) return handler(req, entry)
      return new Response("<h1>Dashboard</h1>", {
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "X-Frame-Options": "DENY",
          "Content-Security-Policy": "default-src 'none'",
        },
      })
    },
  })
  cleanups.push(() => server.stop(true))
  const origin = `http://127.0.0.1:${server.port}`
  return { server, origin, seen }
}

function tokenChecker(valid: Set<string>, options?: { fail?: () => boolean }) {
  return (token: string) => {
    if (options?.fail?.()) throw new Error("database locked")
    return valid.has(token) ? "laptop" : undefined
  }
}

const SANDBOX_CSP = "frame-ancestors 'self'; sandbox allow-scripts allow-forms allow-downloads allow-popups"

describe("dashboard-proxy", () => {
  test("GET HTML returns upstream body with CORS, cache, nosniff, and sandbox CSP", async () => {
    const { origin } = startUpstream((req) => {
      const url = new URL(req.url)
      if (url.pathname === "/data.json") {
        return Response.json({ ok: true })
      }
      return new Response("<h1>Hello</h1>", {
        headers: {
          "Content-Type": "text/html; charset=utf-8",
          "X-Frame-Options": "SAMEORIGIN",
          "Content-Security-Policy": "frame-ancestors 'none'",
        },
      })
    })
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const minted = proxy.mint("tok-1", origin)
    expect(minted.url).toBe(`/dashboard/${minted.passId}/`)

    const res = await proxy.handle(new Request(`http://evie.local${minted.url}`))
    expect(res.status).toBe(200)
    expect(await res.text()).toBe("<h1>Hello</h1>")
    expect(res.headers.get("access-control-allow-origin")).toBe("*")
    expect(res.headers.get("cache-control")).toBe("private, no-cache")
    expect(res.headers.get("x-content-type-options")).toBe("nosniff")
    expect(res.headers.get("content-security-policy")).toBe(SANDBOX_CSP)
    expect(res.headers.get("x-frame-options")).toBeNull()

    // Non-HTML responses do not receive the HTML sandbox CSP.
    const jsonRes = await proxy.handle(new Request(`http://evie.local${minted.url}data.json`))
    expect(jsonRes.status).toBe(200)
    expect(await jsonRes.json()).toEqual({ ok: true })
    expect(jsonRes.headers.get("content-security-policy")).toBeNull()
    expect(jsonRes.headers.get("access-control-allow-origin")).toBe("*")
    expect(jsonRes.headers.get("cache-control")).toBe("private, no-cache")
    expect(jsonRes.headers.get("x-content-type-options")).toBe("nosniff")
  })

  test("GET without trailing slash returns 308 preserving query string", async () => {
    const { origin, seen } = startUpstream()
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const { passId } = proxy.mint("tok-1", origin)

    const res = await proxy.handle(new Request(`http://evie.local/dashboard/${passId}?tab=metrics&range=7d`))
    expect(res.status).toBe(308)
    expect(res.headers.get("location")).toBe(`/dashboard/${passId}/?tab=metrics&range=7d`)
    expect(seen).toHaveLength(0)

    const plain = await proxy.handle(new Request(`http://evie.local/dashboard/${passId}`))
    expect(plain.status).toBe(308)
    expect(plain.headers.get("location")).toBe(`/dashboard/${passId}/`)
  })

  test("POST with JSON body and query string forwards to upstream and returns JSON", async () => {
    const { origin, seen } = startUpstream((_req, entry) => {
      return Response.json({
        receivedMethod: entry.method,
        receivedPath: entry.pathname,
        receivedSearch: entry.search,
        receivedType: entry.headers["content-type"],
        receivedBody: JSON.parse(entry.body),
      })
    })
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const { url } = proxy.mint("tok-1", `${origin}/app/`)

    const res = await proxy.handle(
      new Request(`http://evie.local${url}api/query?limit=5`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ metric: "cpu", host: "web-1" }),
      }),
    )
    expect(res.status).toBe(200)
    expect(await res.json()).toEqual({
      receivedMethod: "POST",
      receivedPath: "/app/api/query",
      receivedSearch: "?limit=5",
      receivedType: "application/json",
      receivedBody: { metric: "cpu", host: "web-1" },
    })
    expect(seen).toHaveLength(1)
  })

  test("strips evie_token_* cookies and Authorization header while preserving other cookies", async () => {
    const { origin, seen } = startUpstream()
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const { url } = proxy.mint("tok-1", origin)

    const res1 = await proxy.handle(
      new Request(`http://evie.local${url}`, {
        headers: {
          Cookie: "evie_token_3843=secret-a; theme=dark; evie_token_9999=secret-b; locale=en",
          Authorization: "Bearer super-secret",
          Origin: "http://evie.local",
        },
      }),
    )
    expect(res1.status).toBe(200)
    expect(seen[0]?.headers["cookie"]).toBe("theme=dark; locale=en")
    expect(seen[0]?.headers["authorization"]).toBeUndefined()
    expect(seen[0]?.headers["origin"]).toBeUndefined()

    // When only evie_token_* cookies are present, Cookie header is omitted entirely.
    const res2 = await proxy.handle(
      new Request(`http://evie.local${url}`, {
        headers: {
          Cookie: "evie_token_3843=secret-a",
        },
      }),
    )
    expect(res2.status).toBe(200)
    expect(seen[1]?.headers["cookie"]).toBeUndefined()
  })

  test("OPTIONS preflight returns 204 with CORS allow headers for valid pass and 404 for invalid pass", async () => {
    const { origin, seen } = startUpstream()
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const { url } = proxy.mint("tok-1", origin)

    const validRes = await proxy.handle(
      new Request(`http://evie.local${url}api/items`, {
        method: "OPTIONS",
        headers: {
          "Access-Control-Request-Method": "POST",
          "Access-Control-Request-Headers": "Content-Type, X-Custom",
        },
      }),
    )
    expect(validRes.status).toBe(204)
    expect(validRes.headers.get("access-control-allow-origin")).toBe("*")
    expect(validRes.headers.get("access-control-allow-methods")).toBe("GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
    expect(validRes.headers.get("access-control-allow-headers")).toBe("Content-Type, X-Custom")
    expect(validRes.headers.get("access-control-max-age")).toBe("600")
    expect(seen).toHaveLength(0)

    const invalidRes = await proxy.handle(
      new Request("http://evie.local/dashboard/nonexistent/api/items", {
        method: "OPTIONS",
        headers: {
          "Access-Control-Request-Method": "POST",
        },
      }),
    )
    expect(invalidRes.status).toBe(404)
  })

  test("rewrites relative and same-origin 302 redirects under pass prefix", async () => {
    let upstreamOrigin = ""
    const { origin } = startUpstream((req) => {
      const path = new URL(req.url).pathname
      if (path === "/rel") {
        return new Response(null, { status: 302, headers: { Location: "/next?a=1" } })
      }
      if (path === "/abs") {
        return new Response(null, { status: 302, headers: { Location: `${upstreamOrigin}/other?b=2#sec` } })
      }
      if (path === "/ext") {
        return new Response(null, { status: 302, headers: { Location: "https://example.com/outside" } })
      }
      if (path === "/proto-rel") {
        return new Response(null, { status: 302, headers: { Location: "//example.com/outside" } })
      }
      return new Response("ok")
    })
    upstreamOrigin = origin

    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const { passId, url } = proxy.mint("tok-1", origin)

    const rel = await proxy.handle(new Request(`http://evie.local${url}rel`))
    expect(rel.status).toBe(302)
    expect(rel.headers.get("location")).toBe(`/dashboard/${passId}/next?a=1`)

    const abs = await proxy.handle(new Request(`http://evie.local${url}abs`))
    expect(abs.status).toBe(302)
    expect(abs.headers.get("location")).toBe(`/dashboard/${passId}/other?b=2#sec`)

    const ext = await proxy.handle(new Request(`http://evie.local${url}ext`))
    expect(ext.status).toBe(302)
    expect(ext.headers.get("location")).toBe("https://example.com/outside")

    const protoRel = await proxy.handle(new Request(`http://evie.local${url}proto-rel`))
    expect(protoRel.status).toBe(302)
    expect(protoRel.headers.get("location")).toBe("//example.com/outside")
  })

  test("revoked token immediately returns 404 and checkToken error returns 503", async () => {
    const { origin } = startUpstream()
    const valid = new Set(["tok-1"])
    let failing = false
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(valid, { fail: () => failing }),
    })
    const { url } = proxy.mint("tok-1", origin)

    const ok = await proxy.handle(new Request(`http://evie.local${url}`))
    expect(ok.status).toBe(200)

    // Transient DB failure returns 503 without evicting the pass.
    failing = true
    const err = await proxy.handle(new Request(`http://evie.local${url}`))
    expect(err.status).toBe(503)
    expect(await err.text()).toBe("The token could not be checked just now. Try again.")

    failing = false
    const recovered = await proxy.handle(new Request(`http://evie.local${url}`))
    expect(recovered.status).toBe(200)

    // Revoking token immediately invalidates the pass.
    valid.delete("tok-1")
    const revoked = await proxy.handle(new Request(`http://evie.local${url}`))
    expect(revoked.status).toBe(404)
  })

  test("pass expiry and touch() sliding TTL", async () => {
    const { origin } = startUpstream()
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1", "tok-2"])),
      passLasts: 120,
    })

    const kept = proxy.mint("tok-1", origin)
    const untouched = proxy.mint("tok-2", origin)

    // Touch with wrong token fails.
    expect(proxy.touch(kept.passId, "tok-2")).toBe(false)

    // Slide kept pass forward midway through its TTL.
    await Bun.sleep(70)
    expect(proxy.touch(kept.passId, "tok-1")).toBe(true)

    // Wait past the initial 120ms expiry (total ~145ms), but before kept's extended expiry (~190ms).
    await Bun.sleep(75)
    const keptRes = await proxy.handle(new Request(`http://evie.local${kept.url}`))
    expect(keptRes.status).toBe(200)

    const expiredRes = await proxy.handle(new Request(`http://evie.local${untouched.url}`))
    expect(expiredRes.status).toBe(404)
    expect(proxy.touch(untouched.passId, "tok-2")).toBe(false)
  })

  test("mint reuses pass for same token + upstream, separates different tokens, and enforces maxPasses LRU", async () => {
    const { origin } = startUpstream()
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1", "tok-2", "tok-3"])),
      maxPasses: 2,
    })

    const first = proxy.mint("tok-1", origin)
    const reused = proxy.mint("tok-1", origin)
    expect(reused).toEqual(first)

    const second = proxy.mint("tok-2", origin)
    expect(second.passId).not.toBe(first.passId)

    // Re-minting first refreshes its LRU order, so second becomes the oldest.
    const refreshed = proxy.mint("tok-1", origin)
    expect(refreshed.passId).toBe(first.passId)

    // Minting a third pass evicts `second` (oldest), keeping `first` and `third`.
    const third = proxy.mint("tok-3", origin)
    expect(third.passId).not.toBe(first.passId)
    expect(third.passId).not.toBe(second.passId)

    expect((await proxy.handle(new Request(`http://evie.local${first.url}`))).status).toBe(200)
    expect((await proxy.handle(new Request(`http://evie.local${second.url}`))).status).toBe(404)
    expect((await proxy.handle(new Request(`http://evie.local${third.url}`))).status).toBe(200)
  })

  test("upstream down returns 502 with plain message", async () => {
    // Grab an ephemeral port and immediately close it so connections are refused.
    const dead = Bun.serve({ hostname: "127.0.0.1", port: 0, fetch: () => new Response("bye") })
    const deadOrigin = `http://127.0.0.1:${dead.port}`
    dead.stop(true)

    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })
    const { url } = proxy.mint("tok-1", deadOrigin)

    const res = await proxy.handle(new Request(`http://evie.local${url}`))
    expect(res.status).toBe(502)
    expect(await res.text()).toBe("This dashboard is not answering.")
  })

  test("unmatched prefix or empty passId returns 404", async () => {
    const proxy = createDashboardProxy({
      prefix: "/dashboard",
      checkToken: tokenChecker(new Set(["tok-1"])),
    })

    expect((await proxy.handle(new Request("http://evie.local/other/abc/"))).status).toBe(404)
    expect((await proxy.handle(new Request("http://evie.local/dashboard/"))).status).toBe(404)
    expect((await proxy.handle(new Request("http://evie.local/dashboard//foo"))).status).toBe(404)
  })
})
