import { randomBytes } from "node:crypto"

export type DashboardProxyOptions = {
  prefix: string
  checkToken(token: string): string | undefined
  passLasts?: number
  maxPasses?: number
}

export type DashboardProxy = {
  mint(token: string, upstreamUrl: string): { passId: string; url: string }
  touch(passId: string, token: string): boolean
  handle(request: Request): Promise<Response>
}

type DashboardPass = {
  id: string
  upstream: URL
  token: string
  until: number
}

const DEFAULT_PASS_LASTS = 60 * 60_000
const DEFAULT_MAX_PASSES = 200

const DROP_REQUEST_HEADERS = new Set([
  "host",
  "connection",
  "keep-alive",
  "transfer-encoding",
  "upgrade",
  "te",
  "proxy-connection",
  "proxy-authenticate",
  "proxy-authorization",
  "authorization",
  "origin",
  "accept-encoding",
])

const DROP_RESPONSE_HEADERS = new Set([
  "connection",
  "keep-alive",
  "transfer-encoding",
  "x-frame-options",
  "content-security-policy",
])

const SANDBOX_CSP = "frame-ancestors 'self'; sandbox allow-scripts allow-forms allow-downloads allow-popups"

// Strips Evie's own sign-in cookies (`evie_token_*`) from a Cookie header so
// an embedded dashboard never receives Evie credentials.
function stripEvieCookies(header: string): string | undefined {
  const kept: string[] = []
  for (const part of header.split(";")) {
    const trimmed = part.trim()
    if (trimmed === "") continue
    const eq = trimmed.indexOf("=")
    const name = (eq === -1 ? trimmed : trimmed.slice(0, eq)).trim()
    if (name.startsWith("evie_token_")) continue
    kept.push(trimmed)
  }
  return kept.length > 0 ? kept.join("; ") : undefined
}

// Rewrites relative and same-origin redirect locations under the pass prefix
// so dashboard navigation stays inside its capability URL.
function rewriteLocation(location: string, prefix: string, passId: string, upstreamOrigin: string): string {
  if (location.startsWith("/") && !location.startsWith("//")) {
    return `${prefix}/${passId}${location}`
  }
  if (URL.canParse(location)) {
    const parsed = new URL(location)
    if (parsed.origin === upstreamOrigin) {
      return `${prefix}/${passId}${parsed.pathname}${parsed.search}${parsed.hash}`
    }
  }
  return location
}

// A scoped reverse proxy for dashboard upstreams. Each pass is a capability
// URL tied to one signed-in device's token and one upstream base URL; if that
// device's token is revoked or the pass expires, the proxy stops answering at
// once. Evie credentials are stripped before forwarding and HTML responses are
// sandboxed.
export function createDashboardProxy(options: DashboardProxyOptions): DashboardProxy {
  const prefix = options.prefix.replace(/\/+$/, "")
  const passLasts = options.passLasts ?? DEFAULT_PASS_LASTS
  const maxPasses = options.maxPasses ?? DEFAULT_MAX_PASSES
  const passes = new Map<string, DashboardPass>()

  function mint(token: string, upstreamUrl: string): { passId: string; url: string } {
    const now = Date.now()
    const upstream = new URL(upstreamUrl)
    for (const [id, pass] of passes) {
      if (pass.until <= now) passes.delete(id)
    }
    for (const [id, pass] of passes) {
      if (pass.token === token && pass.upstream.href === upstream.href) {
        pass.until = now + passLasts
        passes.delete(id)
        passes.set(id, pass)
        return { passId: id, url: `${prefix}/${id}/` }
      }
    }
    while (passes.size >= maxPasses) {
      const oldest = passes.keys().next().value
      if (oldest === undefined) break
      passes.delete(oldest)
    }
    const id = randomBytes(32).toString("base64url")
    passes.set(id, { id, upstream, token, until: now + passLasts })
    return { passId: id, url: `${prefix}/${id}/` }
  }

  function touch(passId: string, token: string): boolean {
    const pass = passes.get(passId)
    if (pass === undefined) return false
    const now = Date.now()
    if (pass.until <= now) {
      passes.delete(passId)
      return false
    }
    if (pass.token !== token) return false
    pass.until = now + passLasts
    passes.delete(passId)
    passes.set(passId, pass)
    return true
  }

  async function handle(request: Request): Promise<Response> {
    const url = new URL(request.url)
    const lead = `${prefix}/`
    if (!url.pathname.startsWith(lead)) return new Response("Not found.", { status: 404 })
    const afterPrefix = url.pathname.slice(lead.length)
    const slash = afterPrefix.indexOf("/")
    const passId = slash === -1 ? afterPrefix : afterPrefix.slice(0, slash)
    const rest = slash === -1 ? "" : afterPrefix.slice(slash)
    if (passId === "") return new Response("Not found.", { status: 404 })

    const pass = passes.get(passId)
    if (pass === undefined || pass.until <= Date.now()) {
      passes.delete(passId)
      return new Response("Not found.", { status: 404 })
    }

    let valid: string | undefined
    try {
      valid = options.checkToken(pass.token)
    } catch {
      return new Response("The token could not be checked just now. Try again.", { status: 503 })
    }
    if (valid === undefined) {
      passes.delete(passId)
      return new Response("Not found.", { status: 404 })
    }

    if (rest === "") {
      return new Response(null, {
        status: 308,
        headers: { Location: `${prefix}/${passId}/${url.search}` },
      })
    }

    if (request.method === "OPTIONS" && request.headers.has("access-control-request-method")) {
      return new Response(null, {
        status: 204,
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS",
          "Access-Control-Allow-Headers": request.headers.get("access-control-request-headers") ?? "*",
          "Access-Control-Max-Age": "600",
        },
      })
    }

    const basePath = pass.upstream.pathname.replace(/\/+$/, "")
    const targetUrl = `${pass.upstream.origin}${basePath}${rest}${url.search}`

    const outHeaders = new Headers()
    request.headers.forEach((value, key) => {
      const lower = key.toLowerCase()
      if (DROP_REQUEST_HEADERS.has(lower)) return
      if (lower === "cookie") {
        const cleaned = stripEvieCookies(value)
        if (cleaned !== undefined) outHeaders.set("cookie", cleaned)
        return
      }
      outHeaders.append(key, value)
    })

    let upstreamRes: Response
    try {
      const init: RequestInit = {
        method: request.method,
        headers: outHeaders,
        redirect: "manual",
      }
      if (request.method !== "GET" && request.method !== "HEAD" && request.body !== null) {
        init.body = request.body
      }
      upstreamRes = await fetch(targetUrl, init)
    } catch {
      return new Response("This dashboard is not answering.", { status: 502 })
    }

    const respHeaders = new Headers()
    upstreamRes.headers.forEach((value, key) => {
      if (DROP_RESPONSE_HEADERS.has(key.toLowerCase())) return
      respHeaders.append(key, value)
    })

    if (upstreamRes.status >= 300 && upstreamRes.status < 400) {
      const location = respHeaders.get("location")
      if (location !== null) {
        respHeaders.set("location", rewriteLocation(location, prefix, passId, pass.upstream.origin))
      }
    }

    respHeaders.set("Access-Control-Allow-Origin", "*")
    respHeaders.set("Cache-Control", "private, no-cache")
    if (!respHeaders.has("x-content-type-options")) {
      respHeaders.set("X-Content-Type-Options", "nosniff")
    }

    const contentType = respHeaders.get("content-type")
    if (contentType !== null && contentType.toLowerCase().startsWith("text/html")) {
      respHeaders.set("Content-Security-Policy", SANDBOX_CSP)
    }

    return new Response(upstreamRes.body, {
      status: upstreamRes.status,
      statusText: upstreamRes.statusText,
      headers: respHeaders,
    })
  }

  return { mint, touch, handle }
}
